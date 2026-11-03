#!/usr/bin/env python3
"""Rebuild a pinned PHP reference engine in a disposable restricted container."""
import argparse
import hashlib
import json
import re
import signal
import shlex
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Optional


_HEX64_RE = re.compile(r"^[0-9a-fA-F]{64}$")
_CONTAINER_ID_RE = re.compile(r"^[0-9a-f]{64}$")
_PINNED_IMAGE_RE = re.compile(
    r"^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]+)?"
    r"(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*@sha256:[0-9a-fA-F]{64}$"
)
_CONFIGURE_OPTIONS = ["--disable-all", "--enable-cli", "--without-pear"]
_VERSION_RE = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+\Z")

# Executed inside the reference container against JSON emitted by the freshly
# built PHP binary. The same code can be exercised with controlled inputs.
_PROFILE_VALIDATOR = r'''
import json, re, sys

def reject(message):
    print("PHP profile mismatch: " + message, file=sys.stderr)
    raise SystemExit(1)

try:
    if len(sys.argv) != 4:
        reject("expected profile is required")
    version, platform = sys.argv[1:3]
    configured = json.loads(sys.argv[3])
    if not isinstance(configured, dict) or set(configured) != {
            "extensions", "zend_extensions", "session_available", "session_upload_progress"}:
        reject("invalid expected profile")
    raw = sys.stdin.read(1024 * 1024 + 1)
    if len(raw) > 1024 * 1024:
        reject("profile exceeds size limit")
    profile = json.loads(raw)
    if not isinstance(profile, dict):
        reject("profile must be an object")
    machines = {"linux/arm64": "aarch64", "linux/amd64": "x86_64"}
    expected = {"version": version, "int_size": 8, "sapi": "cli", "os_family": "Linux",
                "machine": machines.get(platform), "ini_loaded": "", "ini_scanned": ""}
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) or platform not in machines:
        reject("invalid expected version or platform")
    for key, value in expected.items():
        if type(profile.get(key)) is not type(value) or profile.get(key) != value:
            reject(key)
    for key in ("extensions", "zend_extensions"):
        values = profile.get(key)
        if not isinstance(values, list) or len(values) > 1024 or any(
                not isinstance(item, str) or not item or len(item) > 1024 for item in values):
            reject(key)
        expected_values = configured.get(key)
        if (not isinstance(expected_values, list) or len(expected_values) > 1024 or
                any(not isinstance(item, str) or not item or len(item) > 1024 for item in expected_values) or
                len(set(expected_values)) != len(expected_values) or len(set(values)) != len(values) or
                sorted(values) != sorted(expected_values)):
            reject(key)
        profile[key] = sorted(values)
    for key in ("session_available", "session_upload_progress"):
        wanted = configured.get(key)
        if key == "session_available" and type(wanted) is not bool:
            reject(key)
        if key == "session_upload_progress" and type(wanted) not in (bool, str):
            reject(key)
        if type(profile.get(key)) is not type(wanted) or profile.get(key) != wanted:
            reject(key)
    print(json.dumps(profile, sort_keys=True, separators=(",", ":")))
except (ValueError, TypeError, UnicodeError) as exc:
    reject(str(exc))
'''


def _validate_lock_profile(compat: dict) -> None:
    if type(compat.get("schema_version")) is not int or compat["schema_version"] != 1:
        raise ValueError("unsupported compat.lock.json schema_version")
    reference_build = compat.get("reference_build")
    if not isinstance(reference_build, dict):
        raise ValueError("invalid reference_build in compat.lock.json")
    if reference_build.get("platform") != "linux/arm64":
        raise ValueError("unsupported reference_build.platform; expected linux/arm64")
    if reference_build.get("configure_options") != _CONFIGURE_OPTIONS:
        raise ValueError("unsupported reference_build.configure_options")


def verify_archive(path: Path, expected_sha256: str) -> None:
    """Reject missing, non-regular, or mismatched source archives."""
    if not isinstance(expected_sha256, str) or not _HEX64_RE.fullmatch(expected_sha256):
        raise ValueError("expected_sha256 must be a 64-hex digit digest")
    p = Path(path)
    if not p.exists():
        raise OSError(f"archive not found: {p}")
    if not p.is_file():
        raise OSError(f"archive is not a regular file: {p}")
    h = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    if h.hexdigest().lower() != expected_sha256.lower():
        raise ValueError("sha256 mismatch")


def _load_compat() -> dict:
    compat_path = Path(__file__).resolve().parent.parent / "compat.lock.json"
    try:
        with compat_path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except Exception as e:
        raise OSError(f"failed to read compat.lock.json: {e}") from e
    if not isinstance(data, dict):
        raise OSError("invalid compat.lock.json")
    return data


def _load_reference_image(compat: Optional[dict] = None) -> str:
    try:
        source = _load_compat() if compat is None else compat
        image = source["reference_build"]["image"]
    except Exception as e:
        raise OSError(f"missing reference_build.image in compat.lock.json: {e}") from e
    if not isinstance(image, str) or not _PINNED_IMAGE_RE.fullmatch(image):
        raise OSError("reference_build.image must be a valid image name pinned by a 64-hex sha256 digest")
    return image


def build_command(archive: Path, timeout_seconds: int, platform: str = "linux/arm64") -> list[str]:
    """Return a Docker command for a bounded, networkless reference build."""
    return _build_command(archive, timeout_seconds, _load_compat(), platform=platform)


def _select_additional_profile(compat: dict, platform: str) -> dict:
    additional = compat.get("additional_reference_builds")
    if not isinstance(additional, list):
        raise ValueError("missing additional_reference_builds")
    matches = []
    for p in additional:
        if not isinstance(p, dict) or not isinstance(p.get("platform"), str) or not p["platform"]:
            raise ValueError("additional_reference_builds entries need a platform")
        if p["platform"] == platform:
            matches.append(p)
    if len(matches) != 1:
        raise ValueError("ambiguous or missing additional reference profile")
    prof = matches[0]
    if prof.get("configure_options") != _CONFIGURE_OPTIONS:
        raise ValueError("unsupported additional_reference_builds.configure_options")
    image = prof.get("image")
    if not isinstance(image, str) or not _PINNED_IMAGE_RE.fullmatch(image):
        raise ValueError("additional_reference_builds.image must be a valid image name pinned by a 64-hex sha256 digest")
    return prof


def _build_command(archive: Path, timeout_seconds: int, compat: dict, platform: str = "linux/arm64") -> list[str]:
    if not isinstance(timeout_seconds, int) or isinstance(timeout_seconds, bool) or timeout_seconds <= 0:
        raise ValueError("timeout_seconds must be a positive integer")
    # The additional profile extends a valid lock; it cannot redefine the
    # schema or replace the canonical arm64 build.
    _validate_lock_profile(compat)
    arm64_image = _load_reference_image(compat)
    if platform == "linux/arm64":
        prof = compat["reference_build"]
        image = arm64_image
    elif platform == "linux/amd64":
        prof = _select_additional_profile(compat, platform)
        image = prof["image"]
    else:
        raise ValueError("unsupported platform")
    expected_profile = prof.get("expected_profile")
    if not isinstance(expected_profile, dict) or set(expected_profile) != {
            "extensions", "zend_extensions", "session_available", "session_upload_progress"}:
        raise ValueError("platform expected_profile is required")
    for key in ("extensions", "zend_extensions"):
        values = expected_profile[key]
        if (not isinstance(values, list) or len(values) > 1024 or
                any(not isinstance(item, str) or not item or len(item) > 1024 for item in values) or
                len(set(values)) != len(values)):
            raise ValueError(f"invalid expected_profile.{key}")
    if type(expected_profile["session_available"]) is not bool or type(
            expected_profile["session_upload_progress"]) not in (bool, str):
        raise ValueError("invalid expected_profile session values")

    try:
        archive_abs = Path(archive).resolve(strict=True)
    except OSError as e:
        raise OSError(f"cannot resolve archive path: {e}") from e
    if not archive_abs.is_file():
        raise OSError(f"archive is not a regular file: {archive_abs}")
    archive_str = str(archive_abs)
    if any(c in archive_str for c in (",", '"', "\n", "\r")):
        raise ValueError("resolved archive path contains characters unsafe for Docker --mount")

    try:
        expected_sha = compat["php"]["archive_sha256"]
    except Exception as e:
        raise OSError(f"missing php.archive_sha256 in compat.lock.json: {e}") from e
    if not isinstance(expected_sha, str) or not _HEX64_RE.fullmatch(expected_sha):
        raise ValueError("php.archive_sha256 must be a 64-hex digit digest")
    expected_sha = expected_sha.lower()
    version = compat.get("php", {}).get("version")
    if not isinstance(version, str) or not _VERSION_RE.fullmatch(version):
        raise ValueError("php.version must be an exact numeric release version")

    argv = [
        "docker", "run", "--pull", "never", "--hostname", "phego-reference",
        "--network", "none", "--read-only", "--tmpfs", "/tmp:rw,nosuid,nodev,exec,size=2g",
        "--user", "65534:65534", "--cpus", "2", "--memory", "2g", "--pids-limit", "256",
        "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--platform", platform,
        "--mount", f"type=bind,source={archive_str},target=/input/php.tar.xz,readonly",
        image,
        "timeout", "--kill-after=5s", f"{timeout_seconds}s", "sh", "-c",
    ]
    script = (
        "set -eu; "
        "export SOURCE_DATE_EPOCH=1792540800 TZ=UTC LC_ALL=C; "
        f"expected={expected_sha}; "
        "cp /input/php.tar.xz /tmp/phego-php.tar.xz; "
        "actual=$(sha256sum /tmp/phego-php.tar.xz | awk '{print $1}'); "
        "[ \"$actual\" = \"$expected\" ] || { echo 'sha256 mismatch' >&2; exit 1; }; "
        "mkdir -p /tmp/build; cd /tmp/build; "
        "tar -xf /tmp/phego-php.tar.xz; "
        "set -- php-*; [ -d \"$1\" ] || { echo 'no php source directory found' >&2; exit 1; }; "
        "cd \"$1\"; "
        "./configure --disable-all --enable-cli --without-pear; make -j2; "
        "echo '=== php -n -v ==='; ./sapi/cli/php -n -v; "
        "echo '=== PHP_PROFILE ==='; "
        + "./sapi/cli/php -n -r " + shlex.quote(
            '$p=["version"=>PHP_VERSION,"int_size"=>PHP_INT_SIZE,"sapi"=>PHP_SAPI,'
            '"os_family"=>PHP_OS_FAMILY,"machine"=>php_uname("m"),'
            '"ini_loaded"=>(string)(php_ini_loaded_file() ?: ""),'
            '"ini_scanned"=>(string)(php_ini_scanned_files() ?: ""),'
            '"extensions"=>get_loaded_extensions(),"zend_extensions"=>get_loaded_extensions(true),'
            '"session_available"=>extension_loaded("session"),'
            '"session_upload_progress"=>ini_get("session.upload_progress.enabled")];'
            'echo json_encode($p, JSON_THROW_ON_ERROR);'
        ) + " > /tmp/phego-profile.json; "
        + "_PROFILE_VALIDATOR=" + shlex.quote(_PROFILE_VALIDATOR) + "; "
        + "python3 -c \"$_PROFILE_VALIDATOR\" " + shlex.quote(version) + " " + shlex.quote(platform)
        + " " + shlex.quote(json.dumps(expected_profile, sort_keys=True, separators=(",", ":")))
        + " < /tmp/phego-profile.json; "
        + "echo '=== binary sha256 ==='; sha256sum ./sapi/cli/php; "
        "TEST_PHP_EXECUTABLE=./sapi/cli/php ./sapi/cli/php -n run-tests.php -P tests/basic/001.phpt"
    )
    argv.append(script)
    return argv


def _cidfile_state(cidfile: Path) -> tuple[bool, Optional[str]]:
    """Return whether Docker created the cidfile and its validated container ID."""
    try:
        value = cidfile.read_text(encoding="ascii").strip()
    except FileNotFoundError:
        return False, None
    except (UnicodeError, OSError):
        return True, None
    return True, value if _CONTAINER_ID_RE.fullmatch(value) else None


def _remove_container(container_id: str) -> None:
    try:
        proc = subprocess.run(["docker", "rm", "-f", container_id], check=False, timeout=15)
    except Exception as e:
        raise RuntimeError(f"failed to clean up reference container {container_id}: {e}") from e
    if proc.returncode != 0:
        raise RuntimeError(f"failed to clean up reference container {container_id} (docker rm -f exit {proc.returncode})")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="downloaded official PHP source archive")
    parser.add_argument("--timeout", type=int, default=900, help="container build time limit in seconds")
    parser.add_argument("--platform", choices=["linux/arm64", "linux/amd64"], default="linux/arm64")
    args = parser.parse_args()

    try:
        compat = _load_compat()
        expected_sha = compat["php"]["archive_sha256"]
        verify_archive(args.archive, expected_sha)
        argv = _build_command(args.archive, args.timeout, compat, platform=args.platform)
    except (OSError, ValueError, KeyError, TypeError) as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    state = {"sigint": False, "sigterm": False, "proc": None}

    def handle_signal(signum, _frame):
        # Handlers only record cancellation. The main loop owns child
        # termination, and the same handlers defer signals through cleanup.
        if signum == signal.SIGINT:
            state["sigint"] = True
        elif signum == signal.SIGTERM:
            state["sigterm"] = True

    previous_sigint = signal.signal(signal.SIGINT, handle_signal)
    previous_sigterm = signal.signal(signal.SIGTERM, handle_signal)
    cleanup_failed = False
    result = 1
    try:
        with tempfile.TemporaryDirectory(prefix="phego-reference-") as td:
            cidfile = Path(td) / "container.cid"
            image_index = argv.index("--mount") + 2
            argv[image_index:image_index] = ["--cidfile", str(cidfile)]
            process_started = False
            try:
                if not state["sigint"] and not state["sigterm"]:
                    proc = subprocess.Popen(argv)
                    process_started = True
                    state["proc"] = proc
                    deadline = time.monotonic() + args.timeout + 60
                    while proc.poll() is None:
                        if state["sigint"] or state["sigterm"]:
                            break
                        remaining = deadline - time.monotonic()
                        if remaining <= 0:
                            raise subprocess.TimeoutExpired(argv, args.timeout + 60)
                        try:
                            proc.wait(timeout=min(remaining, 0.5))
                        except subprocess.TimeoutExpired:
                            pass
                    if state["sigint"] or state["sigterm"]:
                        try:
                            proc.terminate()
                        except OSError:
                            pass
                        try:
                            proc.wait(timeout=5)
                        except subprocess.TimeoutExpired:
                            proc.kill()
                            proc.wait(timeout=5)
                        result = 143 if state["sigterm"] else 130
                    else:
                        result = proc.wait()
                        if result == 124:
                            print("Error: container build timed out", file=sys.stderr)
            except FileNotFoundError as e:
                print(f"Error: {e}", file=sys.stderr)
                result = 127
            except OSError as e:
                print(f"Error: failed to start Docker: {e}", file=sys.stderr)
                result = 126 if isinstance(e, PermissionError) else 1
            except subprocess.TimeoutExpired:
                print("Error: host wait timeout expired", file=sys.stderr)
                proc.kill()
                proc.wait(timeout=5)
                result = 124
            finally:
                state["proc"] = None
                cidfile_created, container_id = _cidfile_state(cidfile)
                if container_id:
                    try:
                        _remove_container(container_id)
                    except RuntimeError as e:
                        print(f"Error: {e}", file=sys.stderr)
                        result = 1
                        cleanup_failed = True
                elif process_started:
                    print("Error: container cleanup state unknown; Docker cidfile is missing, unreadable, or invalid", file=sys.stderr)
                    result = 1
                    cleanup_failed = True
                if cleanup_failed:
                    result = 1
                elif state["sigterm"]:
                    result = 143
                elif state["sigint"]:
                    result = 130
    finally:
        try:
            signal.signal(signal.SIGTERM, previous_sigterm)
        finally:
            signal.signal(signal.SIGINT, previous_sigint)

    # A deferred signal can arrive during handler restoration.
    if cleanup_failed:
        return 1
    if state["sigterm"]:
        return 143
    if state["sigint"]:
        return 130
    return result


if __name__ == "__main__":
    raise SystemExit(main())
