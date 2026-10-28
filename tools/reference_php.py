#!/usr/bin/env python3
"""Rebuild a pinned PHP reference engine in a disposable restricted container."""
import argparse
import hashlib
import json
import re
import signal
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
        image = arm64_image
    elif platform == "linux/amd64":
        prof = _select_additional_profile(compat, platform)
        image = prof["image"]
    else:
        raise ValueError("unsupported platform")

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
        "actual=$(sha256sum /input/php.tar.xz | awk '{print $1}'); "
        "[ \"$actual\" = \"$expected\" ] || { echo 'sha256 mismatch' >&2; exit 1; }; "
        "mkdir -p /tmp/build; cd /tmp/build; "
        "tar -xf /input/php.tar.xz; "
        "set -- php-*; [ -d \"$1\" ] || { echo 'no php source directory found' >&2; exit 1; }; "
        "cd \"$1\"; "
        "./configure --disable-all --enable-cli --without-pear; make -j2; "
        "echo '=== php -n -v ==='; ./sapi/cli/php -n -v; "
        "echo '=== binary sha256 ==='; sha256sum ./sapi/cli/php; "
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

    state = {"sigterm": False, "proc": None}

    def handle_sigterm(signum, frame):
        state["sigterm"] = True
        active_proc = state["proc"]
        if active_proc is not None and active_proc.poll() is None:
            try:
                active_proc.terminate()
            except ProcessLookupError:
                pass

    previous_sigterm_handler = signal.signal(signal.SIGTERM, handle_sigterm)
    try:
        with tempfile.TemporaryDirectory(prefix="phego-reference-") as td:
            cidfile = Path(td) / "container.cid"
            image_index = argv.index("--mount") + 2
            argv[image_index:image_index] = ["--cidfile", str(cidfile)]
            result = 1
            cleanup_failed = False
            try:
                proc = subprocess.Popen(argv)
                state["proc"] = proc

                def cancel_after_sigterm():
                    if proc.poll() is None and not state["sigterm"]:
                        proc.terminate()
                    try:
                        proc.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        proc.kill()
                        proc.wait()
                    return 143

                # SIGTERM may arrive while Popen is creating the child, before
                # its handle can be stored by the signal handler.
                if state["sigterm"] and proc.poll() is None:
                    proc.terminate()
                if state["sigterm"]:
                    result = cancel_after_sigterm()
                else:
                    deadline = time.monotonic() + args.timeout + 60
                    try:
                        while True:
                            if state["sigterm"]:
                                result = cancel_after_sigterm()
                                break
                            remaining = deadline - time.monotonic()
                            if remaining <= 0:
                                raise subprocess.TimeoutExpired(argv, args.timeout + 60)
                            try:
                                result = proc.wait(timeout=min(remaining, 0.5))
                                if state["sigterm"]:
                                    result = cancel_after_sigterm()
                                break
                            except subprocess.TimeoutExpired:
                                if state["sigterm"]:
                                    result = cancel_after_sigterm()
                                    break
                                if time.monotonic() >= deadline:
                                    raise
                    except subprocess.TimeoutExpired:
                        print("Error: host wait timeout expired", file=sys.stderr)
                        proc.kill()
                        proc.wait()
                        result = 124
                    except KeyboardInterrupt:
                        print("Error: interrupted", file=sys.stderr)
                        proc.terminate()
                        try:
                            proc.wait(timeout=5)
                        except subprocess.TimeoutExpired:
                            proc.kill()
                            proc.wait()
                        result = 130
            except FileNotFoundError as e:
                print(f"Error: {e}", file=sys.stderr)
                result = 127
            except OSError as e:
                print(f"Error: failed to start Docker: {e}", file=sys.stderr)
                result = 126 if isinstance(e, PermissionError) else 1
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
                elif result == 0 or result in (124, 130) or cidfile_created or state["sigterm"]:
                    print("Error: container cleanup state unknown; Docker cidfile is missing, unreadable, or invalid", file=sys.stderr)
                    result = 1
                    cleanup_failed = True
                if state["sigterm"] and not cleanup_failed:
                    result = 143
            return result
    finally:
        signal.signal(signal.SIGTERM, previous_sigterm_handler)


if __name__ == "__main__":
    raise SystemExit(main())
