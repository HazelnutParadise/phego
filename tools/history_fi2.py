#!/usr/bin/env python3
"""Build PHP/FI 2.0 in a restricted container and run one local reference input."""
from __future__ import annotations
import argparse
import hashlib
import os
import re
import signal
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ARCHIVE_SHA256 = "018f706477f4531115178c4b89b7cef1894c5b51cd27e5f9b4d9f5bb44cbeb82"
IMAGE = "buildpack-deps@sha256:2d7fd9aab90c5444e6c4f5c59fce9ab6ba6bce7b8709ef74fcd1f618cd2e867b"
ARCHIVE_LIMIT = 8 * 1024 * 1024
INPUT_LIMIT = 1024 * 1024
_CID_RE = re.compile(r"^[0-9a-f]{64}$")
_OUTPUT_LIMIT = 4 * 1024 * 1024
_OUTPUT_LIMITER = (
    "import os, resource, sys; "
    f"resource.setrlimit(resource.RLIMIT_FSIZE, ({_OUTPUT_LIMIT}, {_OUTPUT_LIMIT})); "
    "os.execvp(sys.argv[1], sys.argv[1:])"
)


def snapshot(source: Path, target: Path, max_bytes: int, expected_sha256: str | None = None) -> str:
    """Copy a bounded regular file to a private snapshot; return its SHA-256.

    Reject symlinks, special files, oversize input, changed reads and digest mismatch.
    Never overwrite target. Error messages identify the source and failure.
    """
    src = Path(source)
    dst = Path(target)
    if type(max_bytes) is not int or max_bytes < 0:
        raise ValueError(f"{src}: invalid size limit {max_bytes!r}")
    flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
    try:
        fd = os.open(src, flags)
    except OSError as exc:
        raise OSError(f"{src}: cannot open source without following links: {exc}") from exc
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode):
            raise ValueError(f"{src}: source is not a regular file")
        if before.st_size > max_bytes:
            raise ValueError(f"{src}: size {before.st_size} exceeds limit {max_bytes}")
        chunks, total = [], 0
        while True:
            chunk = os.read(fd, min(1024 * 1024, max_bytes + 1 - total))
            if not chunk:
                break
            total += len(chunk)
            if total > max_bytes:
                raise ValueError(f"{src}: source exceeds limit {max_bytes}")
            chunks.append(chunk)
        after = os.fstat(fd)
        identity = lambda st: (st.st_dev, st.st_ino, st.st_mode, st.st_size, st.st_mtime_ns, st.st_ctime_ns)
        if identity(before) != identity(after) or total != before.st_size:
            raise ValueError(f"{src}: source changed while being read")
        data = b"".join(chunks)
    finally:
        os.close(fd)
    digest = hashlib.sha256(data).hexdigest()
    if expected_sha256 is not None:
        if not isinstance(expected_sha256, str) or not re.fullmatch(r"[0-9a-fA-F]{64}", expected_sha256):
            raise ValueError(f"{src}: expected SHA-256 must be 64 hexadecimal characters")
        if digest.lower() != expected_sha256.lower():
            raise ValueError(f"{src}: SHA-256 mismatch (expected {expected_sha256}, got {digest})")
    try:
        outfd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except OSError as exc:
        raise OSError(f"{src}: cannot create snapshot {dst}: {exc}") from exc
    try:
        view = memoryview(data)
        while view:
            written = os.write(outfd, view)
            if written <= 0:
                raise OSError("short write")
            view = view[written:]
        os.fchmod(outfd, 0o600)
    except OSError as exc:
        os.close(outfd)
        raise OSError(f"{src}: failed writing snapshot {dst}; partial file may remain: {exc}") from exc
    else:
        os.close(outfd)
    return digest


def build_command(archive: Path, program: Path, cidfile: Path, timeout: int, platform: str) -> list[str]:
    """Build an argv, never a shell command, for the locked isolated reference.

    Accept only platform linux/arm64 and positive bounded timeout. Mount only the
    two snapshots read-only, use a cidfile, disable network, and bound resources.
    The native script builds PHP/FI 2.0 and runs the supplied copied input.
    """
    if platform != "linux/arm64":
        raise ValueError(f"unsupported platform: {platform!r}; expected linux/arm64")
    if type(timeout) is not int or not 1 <= timeout <= 1800:
        raise ValueError("timeout must be an integer from 1 through 1800")
    uid, gid = os.getuid(), os.getgid()
    if uid == 0 or gid == 0:
        raise ValueError("host uid:gid must be non-zero to preserve private snapshot permissions")
    paths = []
    for label, path in (("archive", archive), ("program", program), ("cidfile", cidfile)):
        try: value = str(Path(path).resolve(strict=(label != "cidfile")))
        except OSError as exc: raise OSError(f"{label} path {path}: {exc}") from exc
        if any(c in value for c in ',"\n\r'):
            raise ValueError(f"{label} path contains a character unsafe for Docker mount/cidfile: {value}")
        paths.append(value)
    a, p, c = paths
    script = ("set -eu; mkdir -p /tmp/build /tmp/docroot; cd /tmp/build; "
        "actual=$(sha256sum /input/php.tar.gz | awk '{print $1}'); "
        f"[ \"$actual\" = {ARCHIVE_SHA256} ] || {{ echo 'PHP/FI 2.0 archive SHA-256 mismatch' >&2; exit 20; }}; "
        "tar -xf /input/php.tar.gz; [ -d php-2.0/src ]; "
        "cd php-2.0; "
        "printf '%s\\n' n n n n n n n n n n /tmp/docroot y n n n n n '' '' n | ./install; "
        "make -C src -j2; cp /input/case.phtml /tmp/docroot/case.phtml; "
        "cd /tmp/docroot; QUERY_STRING= timeout --kill-after=5s " + str(timeout) +
        "s /tmp/build/php-2.0/src/php.cgi -q case.phtml")
    return ["docker", "run", "--pull", "never", "--cidfile", c, "--platform", platform,
        "--network", "none", "--read-only", "--user", f"{uid}:{gid}", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--pids-limit", "256", "--memory", "2g",
        "--cpus", "2", "--tmpfs", "/tmp:rw,nosuid,nodev,exec,size=2g",
        "--mount", f"type=bind,source={a},target=/input/php.tar.gz,readonly",
        "--mount", f"type=bind,source={p},target=/input/case.phtml,readonly",
        IMAGE, "timeout", "--kill-after=5s", f"{timeout}s", "sh", "-c", script]


def execute(archive: Path, program: Path, timeout: int, platform: str) -> int:
    """Snapshot inputs, run Docker, and remove its container on every exit path.

    A clean reference run returns its exit code. Missing Docker, invalid input,
    timeout, interrupted operation or uncertain cleanup fail explicitly; never
    report reference behavior as product acceptance. stdout is raw program output
    plus clearly labelled build/exit evidence; stderr reports failures.
    """
    if type(timeout) is not int or not 1 <= timeout <= 1800:
        raise ValueError("timeout must be an integer from 1 through 1800")
    if platform != "linux/arm64": raise ValueError(f"unsupported platform: {platform!r}")
    state = {"signal": None}
    previous = {}
    proc = None
    argv = None
    cid_root = tempfile.TemporaryDirectory(prefix="phego-fi2-cid-")
    cid = Path(cid_root.name) / "container.cid"
    snapshot_root = tempfile.TemporaryDirectory(prefix="phego-fi2-")
    timed_out = False
    output_exceeded = False
    rc = None
    def handler(signum, _frame): state["signal"] = signum
    for sig in (signal.SIGINT, signal.SIGTERM): previous[sig] = signal.signal(sig, handler)
    try:
        td = snapshot_root.name
        root = Path(td); ar = root / "archive.tar.gz"; pr = root / "case.phtml"
        snapshot(archive, ar, ARCHIVE_LIMIT, ARCHIVE_SHA256)
        if state["signal"]: raise RuntimeError(f"PHP/FI reference interrupted by signal {state['signal']}")
        snapshot(program, pr, INPUT_LIMIT)
        if state["signal"]: raise RuntimeError(f"PHP/FI reference interrupted by signal {state['signal']}")
        argv = build_command(ar, pr, cid, timeout, platform)
        with tempfile.TemporaryFile() as stdout_file, tempfile.TemporaryFile() as stderr_file:
            launch_argv = [sys.executable, "-c", _OUTPUT_LIMITER, *argv]
            proc = subprocess.Popen(launch_argv, stdout=stdout_file, stderr=stderr_file)
            started = time.monotonic()
            while proc.poll() is None:
                if state["signal"]:
                    break
                if time.monotonic() - started >= timeout:
                    timed_out = True
                    break
                if os.fstat(stdout_file.fileno()).st_size >= _OUTPUT_LIMIT or os.fstat(stderr_file.fileno()).st_size >= _OUTPUT_LIMIT:
                    output_exceeded = True
                    break
                try:
                    proc.communicate(timeout=0.2)
                except subprocess.TimeoutExpired:
                    continue
            if state["signal"] or timed_out or output_exceeded:
                try: proc.terminate()
                except OSError: pass
                try: proc.communicate(timeout=2)
                except subprocess.TimeoutExpired:
                    try: proc.kill()
                    except OSError: pass
                    try: proc.communicate(timeout=2)
                    except subprocess.TimeoutExpired: pass
            else:
                proc.communicate()
            rc = proc.returncode
            stdout_file.seek(0); out = stdout_file.read(_OUTPUT_LIMIT + 1)
            stderr_file.seek(0); err = stderr_file.read(_OUTPUT_LIMIT + 1)
            if len(out) >= _OUTPUT_LIMIT or len(err) >= _OUTPUT_LIMIT:
                output_exceeded = True
        if timed_out: raise RuntimeError(f"PHP/FI reference timed out after {timeout}s (docker exit {rc})")
        if state["signal"]: raise RuntimeError(f"PHP/FI reference interrupted by signal {state['signal']} (docker exit {rc})")
        if output_exceeded: raise RuntimeError(f"reference output exceeded {_OUTPUT_LIMIT} bytes in stdout or stderr (docker exit {rc}); output was stopped")
    finally:
        # A failed wait/read must not leave the docker client or its container behind.
        if proc is not None and proc.poll() is None:
            try: proc.terminate()
            except OSError: pass
            try: proc.communicate(timeout=2)
            except Exception:
                try: proc.kill()
                except OSError: pass
                try: proc.wait(timeout=2)
                except Exception: pass
        try:
            cid_path = cid
            if argv is not None:
                try: cid_path = Path(argv[argv.index("--cidfile") + 1])
                except (ValueError, IndexError): pass
            if proc is not None:
                container_id = None
                try:
                    raw = cid_path.read_text(encoding="ascii").strip()
                    if _CID_RE.fullmatch(raw): container_id = raw
                except (OSError, UnicodeError): pass
                if not container_id:
                    raise RuntimeError("Docker was started but cidfile has no valid 64-character lowercase ID; cleanup state is unknown")
                try: removed = subprocess.run(["docker", "rm", "-f", container_id], check=False, timeout=15, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                except Exception as exc: raise RuntimeError(f"failed to clean up container {container_id}: {exc}") from exc
                if removed.returncode != 0:
                    raise RuntimeError(f"failed to clean up container {container_id} (docker rm -f exit {removed.returncode})")
        finally:
            try:
                snapshot_root.cleanup()
            finally:
                cid_root.cleanup()
                for sig, old in previous.items(): signal.signal(sig, old)
            if state["signal"]: raise RuntimeError(f"PHP/FI reference interrupted by signal {state['signal']}")
    if out: sys.stdout.buffer.write(out)
    print(f"\n[PHP/FI build and engine exit code: {rc}]")
    if err: sys.stderr.buffer.write(err)
    return rc


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tools/history_fi2.py --platform linux/arm64 php-2.0.tar.gz example.phtml")
    parser.add_argument("archive", type=Path, help="official php-2.0.tar.gz archive")
    parser.add_argument("program", type=Path, help="PHP/FI input file")
    parser.add_argument("--platform", required=True, choices=["linux/arm64"], help="reference guest platform")
    parser.add_argument("--timeout", type=int, default=300, help="maximum run time in seconds (1-1800, default: 300)")
    args = parser.parse_args(argv)
    try:
        return execute(args.archive, args.program, args.timeout, args.platform)
    except (OSError, ValueError, RuntimeError) as exc:
        print(f"error: PHP/FI reference failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
