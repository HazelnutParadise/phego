#!/usr/bin/env python3
"""Verify and probe a locked Go toolchain inside a restricted container."""
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
_PINNED_IMAGE_RE = re.compile(r"^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*@sha256:[0-9a-f]{64}$")
_VERSION_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+$")
_ARCHIVE_RE = re.compile(r"^https://go\.dev/dl/go([0-9]+\.[0-9]+\.[0-9]+)\.linux-(arm64|amd64)\.tar\.gz$")
_SUPPORTED_PLATFORMS = ("linux/arm64", "linux/amd64")


def _validate_lock_profile(compat: dict) -> None:
    if not isinstance(compat, dict):
        raise ValueError("invalid compat.lock.json")
    if type(compat.get("schema_version")) is not int or compat["schema_version"] != 1:
        raise ValueError("unsupported compat.lock.json schema_version")
    rb = compat.get("reference_build")
    if not isinstance(rb, dict) or rb.get("platform") != "linux/arm64":
        raise ValueError("reference_build.platform must be linux/arm64")


def verify_archive(path: Path, expected_sha256: str) -> None:
    if not isinstance(expected_sha256, str) or not _HEX64_RE.fullmatch(expected_sha256):
        raise ValueError("archive_sha256 must be a 64-hex digit digest")
    p = Path(path)
    if not p.is_file():
        raise OSError(f"archive is missing or not a regular file: {p}")
    h = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    if h.hexdigest() != expected_sha256.lower():
        raise ValueError("sha256 mismatch")


def _load_compat() -> dict:
    path = Path(__file__).resolve().parent.parent / "compat.lock.json"
    try:
        with path.open(encoding="utf-8") as f:
            data = json.load(f)
    except (OSError, ValueError) as e:
        raise OSError(f"failed to read compat.lock.json: {e}") from e
    if not isinstance(data, dict):
        raise ValueError("invalid compat.lock.json")
    return data


def _image(value: object, field: str) -> str:
    if not isinstance(value, str) or not _PINNED_IMAGE_RE.fullmatch(value):
        raise ValueError(f"{field} must be an image pinned by a 64-digit sha256 digest")
    return value


def _profile(compat: dict, platform: str) -> dict:
    _validate_lock_profile(compat)
    if platform not in _SUPPORTED_PLATFORMS:
        raise ValueError("unsupported platform")
    go = compat.get("go")
    if not isinstance(go, dict):
        raise ValueError("invalid go record")
    version = go.get("version")
    if not isinstance(version, str) or not _VERSION_RE.fullmatch(version):
        raise ValueError("go.version must be a complete semantic version")

    if platform == "linux/arm64":
        record = go
        if record.get("platform") != platform:
            raise ValueError("go.platform must be linux/arm64")
        image = _image(compat.get("reference_build", {}).get("image"), "reference_build.image")
    else:
        archives = go.get("additional_archives")
        if not isinstance(archives, list):
            raise ValueError("go.additional_archives must be a list")
        matches = [x for x in archives if isinstance(x, dict) and x.get("platform") == platform]
        if len(matches) != 1:
            raise ValueError("missing or ambiguous linux/amd64 Go archive")
        record = matches[0]
        image = _image(record.get("probe_image"), "go.additional_archives.probe_image")

    url, digest = record.get("archive_url"), record.get("archive_sha256")
    if not isinstance(url, str):
        raise ValueError("missing archive_url")
    m = _ARCHIVE_RE.fullmatch(url)
    expected_arch = platform.split("/")[1]
    if not m or m.group(1) != version or m.group(2) != expected_arch:
        raise ValueError("archive_url, version, and platform are inconsistent")
    if not isinstance(digest, str) or not _HEX64_RE.fullmatch(digest):
        raise ValueError("archive_sha256 must be a 64-hex digit digest")
    return {"image": image, "archive_url": url, "archive_sha256": digest.lower(), "version": version}


def build_command(archive: Path, platform: str, timeout_seconds: int, compat: dict) -> list[str]:
    if type(timeout_seconds) is not int or timeout_seconds <= 0:
        raise ValueError("timeout_seconds must be a positive integer")
    prof = _profile(compat, platform)
    try:
        path = Path(archive).resolve(strict=True)
    except OSError as e:
        raise OSError(f"cannot resolve archive path: {e}") from e
    if not path.is_file():
        raise OSError("archive must be a regular file")
    source = str(path)
    if any(c in source for c in (",", '"', "\n", "\r")):
        raise ValueError("archive path contains characters unsafe for Docker --mount")
    script = " ".join((
        "set -eu;",
        f"expected='{prof['archive_sha256']}';",
        "cp /input/go.tar.gz /tmp/phego-go.tar.gz;",
        "actual=$(sha256sum /tmp/phego-go.tar.gz | awk '{print $1}');",
        '[ "$actual" = "$expected" ] || { echo sha256_mismatch >&2; exit 1; };',
        "mkdir -p /tmp/build && tar -xzf /tmp/phego-go.tar.gz -C /tmp/build;",
        "[ -x /tmp/build/go/bin/go ] || { echo invalid_Go_archive >&2; exit 1; };",
        "export PATH=/tmp/build/go/bin:$PATH;",
        "export HOME=/tmp/phego-home GOCACHE=/tmp/phego-gocache;",
        'mkdir -p "$HOME" "$GOCACHE";',
        f"[ \"$(go version)\" = 'go version go{prof['version']} linux/{platform.split('/')[1]}' ] || {{ go version >&2; echo Go_version/platform_mismatch >&2; exit 1; }};",
        "printf 'package main\\nfunc main() {}\\n' > /tmp/build/probe.go;",
        "cd /tmp/build; CGO_ENABLED=0 GOPROXY=off GOTOOLCHAIN=local go build -o probe probe.go; ./probe;",
        "echo '=== go version ==='; go version; echo '=== binary sha256 ==='; sha256sum probe; echo 'probe ok'"
    ))
    return [
        "docker", "run", "--pull", "never", "--network", "none", "--read-only",
        "--tmpfs", "/tmp:rw,nosuid,nodev,exec,size=1g", "--user", "65534:65534",
        "--cpus", "2", "--memory", "2g", "--pids-limit", "256", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--platform", platform,
        "--mount", f"type=bind,source={source},target=/input/go.tar.gz,readonly",
        prof["image"], "timeout", "--kill-after=5s", f"{timeout_seconds}s", "sh", "-c", script,
    ]


def _cidfile_state(path: Path) -> tuple[bool, Optional[str]]:
    try:
        value = path.read_text(encoding="ascii").strip()
    except FileNotFoundError:
        return False, None
    except (OSError, UnicodeError):
        return True, None
    return True, value if _CONTAINER_ID_RE.fullmatch(value) else None


def _remove_container(container_id: str) -> bool:
    interrupted = False

    def _on_sigint(_signum, _frame):
        nonlocal interrupted
        interrupted = True

    old_handler = signal.signal(signal.SIGINT, _on_sigint)
    try:
        try:
            p = subprocess.run(["docker", "rm", "-f", container_id], timeout=15, check=False)
        except subprocess.TimeoutExpired as e:
            raise RuntimeError(f"failed to clean up reference container {container_id}: {e}") from e
        except Exception as e:
            raise RuntimeError(f"failed to clean up reference container {container_id}: {e}") from e
        if p.returncode:
            raise RuntimeError(f"cleanup failed for reference container {container_id} (docker rm exit {p.returncode})")
    finally:
        signal.signal(signal.SIGINT, old_handler)
    return interrupted


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--timeout", type=int, default=900)
    parser.add_argument("--platform", choices=_SUPPORTED_PLATFORMS, default="linux/arm64")
    args = parser.parse_args(argv)
    try:
        compat = _load_compat()
        prof = _profile(compat, args.platform)
        if type(args.timeout) is not int or args.timeout <= 0:
            raise ValueError("timeout must be a positive integer")
        verify_archive(args.archive, prof["archive_sha256"])
        command = build_command(args.archive, args.platform, args.timeout, compat)
    except (OSError, ValueError, KeyError, TypeError) as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    state = {"sigint": False, "sigterm": False, "proc": None}

    def on_signal(signum, _frame):
        # Flag only. Termination happens in the polling loop so cleanup can
        # defer further signals instead of raising KeyboardInterrupt.
        if signum == signal.SIGINT:
            state["sigint"] = True
        elif signum == signal.SIGTERM:
            state["sigterm"] = True

    old_sigint = signal.signal(signal.SIGINT, on_signal)
    old_sigterm = signal.signal(signal.SIGTERM, on_signal)
    try:
        with tempfile.TemporaryDirectory(prefix="phego-go-") as td:
            cidfile = Path(td) / "container.cid"
            command[command.index("--mount"):command.index("--mount")] = ["--cidfile", str(cidfile)]
            result, cleanup_failed, process_started = 1, False, False
            try:
                if not state["sigint"] and not state["sigterm"]:
                    proc = subprocess.Popen(command)
                    process_started = True
                    state["proc"] = proc
                    try:
                        deadline = time.monotonic() + args.timeout + 60
                        while proc.poll() is None:
                            if state["sigint"] or state["sigterm"]:
                                break
                            if time.monotonic() >= deadline:
                                raise subprocess.TimeoutExpired(command, args.timeout + 60)
                            try:
                                proc.wait(timeout=min(.25, max(.01, deadline - time.monotonic())))
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
                                proc.kill(); proc.wait()
                        else:
                            result = proc.returncode
                            if result == 124:
                                print("Error: container probe timed out", file=sys.stderr)
                    except subprocess.TimeoutExpired:
                        print("Error: host wait timeout expired", file=sys.stderr)
                        proc.kill(); proc.wait(); result = 124
                    except KeyboardInterrupt:
                        # A signal may still surface here if it races with
                        # Popen; treat it as a cancellation and clean up.
                        state["sigint"] = True
                        try:
                            proc.terminate()
                        except OSError:
                            pass
                        try: proc.wait(timeout=5)
                        except subprocess.TimeoutExpired: proc.kill(); proc.wait()
            except FileNotFoundError as e:
                print(f"Error: {e}", file=sys.stderr); result = 127
            except OSError as e:
                print(f"Error: failed to start Docker: {e}", file=sys.stderr); result = 1
            finally:
                state["proc"] = None
                created, container_id = _cidfile_state(cidfile)
                interrupted_during_cleanup = False
                if container_id:
                    try:
                        interrupted_during_cleanup = _remove_container(container_id)
                    except RuntimeError as e:
                        print(f"Error: {e}", file=sys.stderr)
                        result = 1
                        cleanup_failed = True
                elif process_started or created:
                    print("Error: container cleanup state unknown; cidfile is missing, unreadable, or invalid", file=sys.stderr)
                    result = 1
                    cleanup_failed = True
                if interrupted_during_cleanup:
                    state["sigint"] = True
                if cleanup_failed:
                    result = 1
                elif state["sigterm"]:
                    result = 143
                elif state["sigint"]:
                    result = 130
    finally:
        try:
            signal.signal(signal.SIGTERM, old_sigterm)
        finally:
            signal.signal(signal.SIGINT, old_sigint)

    # A deferred signal can arrive while the original handlers are restored.
    # Decide the final status only after both restoration calls have returned.
    if cleanup_failed:
        return 1
    if state["sigterm"]:
        return 143
    if state["sigint"]:
        return 130
    return result


if __name__ == "__main__":
    raise SystemExit(main())
