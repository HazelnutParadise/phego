"""Execute a single PHPT PHP source inside a disposable restricted container."""
import json
import os
import re
import selectors
import subprocess
import tempfile
import time
import uuid
from pathlib import Path
from dataclasses import dataclass
from typing import Optional

@dataclass(frozen=True)
class ExecutionResult:
    status: str
    output: bytes = b""
    exit_code: Optional[int] = None
    reason: str = ""

def execute_php(image: str, executable: str, source: bytes, timeout: float, output_limit: int, platform: str) -> ExecutionResult:
    """Run one source with network, privilege, memory, process and output limits.

    Input must use a digest-pinned image and absolute container executable.
    Timeout, overflow and unconfirmed container cleanup never return completed.
    """
    if not isinstance(image, str) or not re.fullmatch(r"[A-Za-z0-9._:/-]+@sha256:[0-9a-f]{64}", image):
        raise ValueError("image must be pinned by SHA-256 digest")
    if not isinstance(executable, str) or not re.fullmatch(r"/[A-Za-z0-9_./-]+", executable) or ".." in executable.split("/"):
        raise ValueError("executable must be an absolute container path")
    if platform not in ("linux/amd64", "linux/arm64", "linux/386"):
        raise ValueError("platform must be linux/amd64, linux/arm64, or linux/386")
    if not isinstance(source, bytes) or len(source) > 1024 * 1024:
        raise ValueError("source must be bytes no larger than 1 MiB")
    if isinstance(timeout, bool) or not isinstance(timeout, (int, float)) or not 0 < timeout <= 60:
        raise ValueError("timeout must be between zero and 60 seconds")
    if type(output_limit) is not int or not 1 <= output_limit <= 4 * 1024 * 1024:
        raise ValueError("output limit must be between 1 byte and 4 MiB")

    name = "phego-phpt-" + uuid.uuid4().hex
    result = ExecutionResult("runner_error", reason="container execution did not complete")
    output = bytearray()
    with tempfile.TemporaryDirectory(prefix="phego-phpt-") as tmp:
        source_file = Path(tmp) / "test.php"
        source_file.write_bytes(source)
        source_file.chmod(0o444)
        command = ["docker", "create", "--name", name, "--pull", "never", "--platform", platform,
                   "--network", "none", "--read-only", "--user", "65534:65534",
                   "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                   "--memory", "256m", "--memory-swap", "256m", "--cpus", "1",
                   "--pids-limit", "64", "--log-driver", "none", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=32m",
                   "--mount", f"type=bind,src={source_file},dst=/work/test.php,readonly",
                   "--workdir", "/work", "--entrypoint", executable, image, "-n", "/work/test.php"]
        try:
            try:
                create = subprocess.run(command, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE, timeout=min(timeout, 10), check=False)
                container_id = create.stdout.decode("ascii", "replace").strip()
                if create.returncode or not re.fullmatch(r"[0-9a-f]{64}", container_id):
                    result = ExecutionResult("runner_error", reason="container creation failed or returned an invalid ID")
                else:
                    proc = subprocess.Popen(["docker", "start", "-a", name], stdin=subprocess.DEVNULL,
                                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
                    deadline = time.monotonic() + timeout
                    status = "completed"
                    try:
                        with selectors.DefaultSelector() as selector:
                            selector.register(proc.stdout, selectors.EVENT_READ)
                            while selector.get_map():
                                remaining = deadline - time.monotonic()
                                if remaining <= 0:
                                    status = "timeout"; break
                                events = selector.select(remaining)
                                if not events:
                                    status = "timeout"; break
                                chunk = os.read(proc.stdout.fileno(), min(65536, output_limit - len(output) + 1))
                                if not chunk:
                                    selector.unregister(proc.stdout); break
                                output.extend(chunk)
                                if len(output) > output_limit:
                                    del output[output_limit:]; status = "output_limit"; break
                        if status != "completed": proc.kill()
                        try: code = proc.wait(timeout=5)
                        except subprocess.TimeoutExpired:
                            proc.kill(); proc.wait(); status = "runner_error"; code = None
                    finally:
                        if proc.poll() is None: proc.kill(); proc.wait()
                        proc.stdout.close()
                    if status == "completed":
                        inspect = subprocess.run(["docker", "inspect", "--format", "{{json .State}}", name],
                                                 stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                                 stderr=subprocess.PIPE, timeout=10, check=False)
                        try:
                            state = json.loads(inspect.stdout)
                            exit_code = state["ExitCode"]
                            valid = (state["Status"] == "exited" and type(exit_code) is int and exit_code >= 0
                                     and not state["Error"] and state["OOMKilled"] is False and code == exit_code)
                        except (ValueError, TypeError, KeyError):
                            valid = False
                        if inspect.returncode or not valid:
                            result = ExecutionResult("runner_error", bytes(output), reason="cannot verify container completion")
                        else:
                            result = ExecutionResult("completed", bytes(output), exit_code)
                    else:
                        result = ExecutionResult(status, bytes(output), reason="execution did not finish within limits")
            except (OSError, subprocess.SubprocessError) as exc:
                result = ExecutionResult("runner_error", bytes(output), reason=f"container operation failed: {type(exc).__name__}")
        finally:
            # Use a unique name even when create's result is uncertain; never
            # declare successful cleanup when removal cannot be confirmed.
            try:
                removal = subprocess.run(["docker", "rm", "-f", name], stdin=subprocess.DEVNULL,
                                         stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10, check=False)
                if removal.returncode != 0:
                    result = ExecutionResult("runner_error", result.output, reason="container cleanup could not be confirmed")
            except (OSError, subprocess.SubprocessError):
                result = ExecutionResult("runner_error", result.output, reason="container cleanup could not be confirmed")
    return result
