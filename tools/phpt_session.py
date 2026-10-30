"""Run a PHPT case in a single disposable, bounded reference environment.

The input fixture manifest is explicit and bounded; no ambient host directories
are copied. Profile, SKIPIF and FILE run in separate PHP processes within the
same case workspace. A generic SKIPIF skip has unknown cause and is not
classified as environment_skip or unsupported without independent evidence.
Raw phase observations and incomplete effects remain distinct from acceptance.
This module does not claim Phego product acceptance.
"""
from dataclasses import dataclass
from typing import Optional
import io
import os
import re
import selectors
import signal
import subprocess
import sys
import tarfile
import threading
import time
import uuid
import warnings


@dataclass(frozen=True)
class PhaseObservation:
    status: str
    output: bytes = b""
    exit_code: Optional[int] = None
    reason: str = ""


@dataclass(frozen=True)
class CaseObservation:
    status: str
    profile: Optional[PhaseObservation]
    skipif: Optional[PhaseObservation]
    program: Optional[PhaseObservation]
    effects_complete: bool
    cleanup_confirmed: bool
    reason: str = ""


def run_case(image, executable, platform, release, source, skipif, fixtures, timeout, output_limit):
    """Execute a case and preserve bounded observations and cleanup status.

    Validate all inputs before creating resources, reject unsafe or ambiguous
    fixture paths, and execute each phase in one isolated workspace. Verify the
    exact image profile before test code, stop on failure or uncertain cleanup,
    preserve raw observation bytes and never invent a skip reason or effects.
    Host cancellation must stop the container and confirm cleanup separately.
    """
    # Validate everything before crossing the Docker resource boundary.
    if not isinstance(image, str) or not re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", image):
        raise ValueError("image must be pinned by sha256 digest")
    if not isinstance(executable, str) or not executable.startswith("/") or "\x00" in executable:
        raise ValueError("executable must be an absolute path")
    if platform not in ("linux/386", "linux/amd64", "linux/arm64", "linux/arm/v7", "linux/arm64/v8"):
        raise ValueError("unsupported platform")
    if not isinstance(release, str) or not re.fullmatch(r"[0-9]{1,20}\.[0-9]{1,20}\.[0-9]{1,20}", release):
        raise ValueError("release must be an exact version")
    if not isinstance(source, bytes) or not isinstance(skipif, bytes):
        raise TypeError("source and skipif must be bytes")
    if len(source) > 1024 * 1024 or len(skipif) > 1024 * 1024:
        raise ValueError("phase source exceeds 1 MiB")
    if not isinstance(fixtures, dict):
        raise TypeError("fixtures must be a mapping")
    if len(fixtures) > 128:
        raise ValueError("too many fixtures")
    total = 0
    fixture_names = set()
    for name, data in fixtures.items():
        if not isinstance(name, str) or not name or "\x00" in name or "\\" in name:
            raise ValueError("invalid fixture path")
        parts = name.split("/")
        if any(p in ("", ".", "..") for p in parts) or name.startswith("/"):
            raise ValueError("unsafe fixture path")
        if not isinstance(data, bytes):
            raise TypeError("fixture values must be bytes")
        fixture_names.add(name)
        total += len(data)
    phase_names = (".phego-profile.php", ".phego-skipif.php", ".phego-file.php")
    for name in fixture_names:
        if any(name == phase or name.startswith(phase + "/") or phase.startswith(name + "/")
               for phase in phase_names):
            raise ValueError("fixture path conflicts with an internal PHPT phase file")
        parents = name.split("/")[:-1]
        if any("/".join(parents[:i]) in fixture_names for i in range(1, len(parents) + 1)):
            raise ValueError("fixture file conflicts with another fixture directory")
    if total > 8 * 1024 * 1024:
        raise ValueError("fixtures exceed 8 MiB")
    if isinstance(timeout, bool) or not isinstance(timeout, (int, float)) or not 0 < timeout <= 120:
        raise ValueError("timeout must be in (0, 120]")
    if isinstance(output_limit, bool) or not isinstance(output_limit, int) or not 0 < output_limit <= 1024 * 1024:
        raise ValueError("output_limit must be in (0, 1 MiB]")
    sigterm_event = [None]

    def observation(status, output=b"", code=None, reason=""):
        return PhaseObservation(status, output, code, reason)

    def docker(args, payload=None, limit=output_limit, seconds=timeout):
        if sigterm_event[0] is not None:
            raise _InterruptedBySigterm(*sigterm_event[0])
        try:
            proc = subprocess.Popen(args, stdin=subprocess.PIPE if payload is not None else subprocess.DEVNULL,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0)
        except OSError as exc:
            return None, b"", b"", "runner_gap", str(exc)
        sel = selectors.DefaultSelector()
        if payload is not None:
            os.set_blocking(proc.stdin.fileno(), False)
            sel.register(proc.stdin, selectors.EVENT_WRITE, "stdin")
        os.set_blocking(proc.stdout.fileno(), False)
        os.set_blocking(proc.stderr.fileno(), False)
        sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
        sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
        out, err = bytearray(), bytearray()
        streams = {"stdout": out, "stderr": err}
        pending = memoryview(payload) if payload is not None else None
        deadline = time.monotonic() + seconds
        try:
            while sel.get_map():
                if sigterm_event[0] is not None:
                    if proc.poll() is None:
                        proc.kill(); proc.wait()
                    raise _InterruptedBySigterm(*sigterm_event[0])
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    proc.kill(); proc.wait()
                    return None, bytes(out), bytes(err), "timeout", "phase timed out"
                events = sel.select(min(remaining, .1))
                for key, _ in events:
                    if key.data == "stdin":
                        try:
                            pending = pending[os.write(key.fd, pending[:65536]):]
                        except BlockingIOError:
                            continue
                        except (BrokenPipeError, OSError):
                            pending = memoryview(b"")
                        if not pending:
                            sel.unregister(key.fileobj)
                            key.fileobj.close()
                        continue
                    target = streams[key.data]
                    try:
                        chunk = os.read(key.fd, min(65536, limit + 1 - len(target)))
                    except BlockingIOError:
                        continue
                    if not chunk:
                        sel.unregister(key.fileobj); key.fileobj.close(); continue
                    target.extend(chunk)
                    if len(target) > limit:
                        proc.kill(); proc.wait()
                        return None, bytes(out[:limit]), bytes(err[:limit]), "output_limit", "phase output exceeded limit"
            code = proc.wait(timeout=max(.01, deadline-time.monotonic()))
            if sigterm_event[0] is not None:
                raise _InterruptedBySigterm(*sigterm_event[0])
            return code, bytes(out), bytes(err), "", ""
        except subprocess.TimeoutExpired:
            proc.kill(); proc.wait()
            return None, bytes(out), bytes(err), "timeout", "phase timed out"
        finally:
            sel.close()
            for stream in (proc.stdin, proc.stdout, proc.stderr):
                if stream is not None and not stream.closed:
                    stream.close()
            if proc.poll() is None:
                proc.kill(); proc.wait()

    def cleanup_by_label(label_value, absence_is_clean=False):
        last_reason = "no container with the creation label was found; daemon outcome remains uncertain"
        for attempt in range(5):
            code, output, error, failure, why = docker(
                ["docker", "ps", "-aq", "--filter", "label=" + label_value], seconds=30, limit=4096)
            if failure or code != 0:
                return False, why or error.decode("utf-8", "replace") or "cannot verify owned container cleanup"
            ids = output.decode("ascii", "ignore").splitlines()
            if ids:
                if len(ids) != 1 or not re.fullmatch(r"[0-9a-f]{12,64}", ids[0]):
                    return False, "owned container listing was ambiguous"
                rm_code, rm_output, rm_error, rm_failure, rm_why = docker(
                    ["docker", "rm", "--force", ids[0]], seconds=30, limit=4096)
                cleaned = not rm_failure and rm_code == 0 and rm_output.strip().decode("ascii", "ignore") == ids[0]
                return cleaned, "" if cleaned else rm_why or rm_error.decode("utf-8", "replace") or "owned container removal not confirmed"
            if attempt < 4:
                time.sleep(.1)
        return absence_is_clean, last_reason

    def process_snapshot(container_id):
        code, output, error, failure, why = docker(
            ["docker", "top", container_id, "-eo", "pid,args"], seconds=10, limit=8192)
        if failure or code != 0 or error:
            return None, why or error.decode("utf-8", "replace") or "cannot inspect container processes"
        pids = set()
        for line in output.decode("ascii", "replace").splitlines()[1:]:
            fields = line.split(None, 1)
            if not fields or not fields[0].isdigit():
                return None, "container process list was malformed"
            pids.add(fields[0])
        return pids, ""

    if not shutil_which("docker"):
        return CaseObservation("runner_gap", None, None, None, False, False, "docker CLI unavailable")
    if threading.current_thread() is not threading.main_thread():
        return CaseObservation("runner_gap", None, None, None, False, False,
                               "SIGTERM cleanup cannot be secured outside the main thread")
    container = "phego-phpt-" + uuid.uuid4().hex
    label = "phego.phpt-session=" + uuid.uuid4().hex
    create = ["docker", "create", "--pull", "never", "--name", container, "--label", label, "--platform", platform,
              "--network", "none", "--read-only", "--user", "65534:65534",
              "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
              "--pids-limit", "64", "--memory", "256m", "--memory-swap", "256m",
              "--cpus", "1", "--tmpfs", "/work:rw,noexec,nosuid,nodev,size=16m,uid=65534,gid=65534,mode=0700",
              image, "/bin/sh", "-c", "exec sleep 2147483647"]
    class _InterruptedBySigterm(BaseException):
        def __init__(self, signum, frame):
            self.signum = signum
            self.frame = frame

    previous_sigterm = None
    sigterm_handler_installed = False
    previous_sigterm = signal.getsignal(signal.SIGTERM)
    if previous_sigterm != signal.SIG_IGN:
        def interrupt_create(signum, frame):
            sigterm_event[0] = (signum, frame)
        signal.signal(signal.SIGTERM, interrupt_create)
        sigterm_handler_installed = True
    try:
        code, out, err, failure, why = docker(create, seconds=30, limit=4096)
        if sigterm_event[0] is not None:
            raise _InterruptedBySigterm(*sigterm_event[0])
        if failure or code != 0:
            status = failure or "runner_gap"
            cleaned, cleanup_reason = cleanup_by_label(label)
            if not cleaned:
                status = "cleanup_uncertain"
            if sigterm_handler_installed:
                signal.signal(signal.SIGTERM, previous_sigterm)
                sigterm_handler_installed = False
            phase = observation(status, err, code, why or "container creation failed")
            return CaseObservation(status, phase, None, None, False, cleaned,
                                   cleanup_reason or why or "container creation failed")
        container_id = out.decode("ascii", "ignore").strip()
        if not container_id or not re.fullmatch(r"[0-9a-f]{12,64}", container_id):
            cleaned, cleanup_reason = cleanup_by_label(label)
            if sigterm_handler_installed:
                signal.signal(signal.SIGTERM, previous_sigterm)
                sigterm_handler_installed = False
            return CaseObservation("runner_gap" if cleaned else "cleanup_uncertain", None, None, None, False, cleaned,
                                   "Docker returned no verifiable container id" if cleaned else
                                   cleanup_reason or "created container removal not confirmed")
    except BaseException as create_error:
        interrupted = create_error if isinstance(create_error, _InterruptedBySigterm) else None
        if interrupted is not None:
            sigterm_event[0] = None
        if sigterm_handler_installed:
            signal.signal(signal.SIGTERM, signal.SIG_IGN)
        try:
            cleaned, cleanup_reason = cleanup_by_label(label)
        except BaseException as cleanup_error:
            cleaned, cleanup_reason = False, "cleanup attempt interrupted: " + type(cleanup_error).__name__
        if sigterm_handler_installed:
            signal.signal(signal.SIGTERM, previous_sigterm)
            sigterm_handler_installed = False
        if not cleaned:
            warnings.warn("PHPT Docker create was interrupted and cleanup is uncertain: " + cleanup_reason,
                          RuntimeWarning)
        if interrupted is not None:
            if previous_sigterm == signal.SIG_DFL:
                os.kill(os.getpid(), signal.SIGTERM)
            elif callable(previous_sigterm):
                previous_sigterm(interrupted.signum, interrupted.frame)
            return CaseObservation("cleanup_uncertain" if not cleaned else "runner_gap", None, None, None,
                                   False, cleaned, cleanup_reason or "SIGTERM interrupted container creation")
        raise

    phases = {"profile": None, "skipif": None, "program": None}
    status, reason = "runner_gap", "container cleanup not confirmed"
    interrupted_sigterm = None
    cleanup_interruption = None
    try:
        code, _, err, failure, why = docker(["docker", "start", container_id], seconds=30, limit=4096)
        if failure or code != 0 or err:
            phases["profile"] = observation("runner_gap", err, code, why or "container start failed")
        else:
            baseline_pids, process_error = process_snapshot(container_id)
            if baseline_pids is None:
                phases["profile"] = observation("runner_gap", b"", None, process_error)
            # Tar is sent over stdin; fixture names and bytes never appear in argv.
            archive = io.BytesIO()
            with tarfile.open(fileobj=archive, mode="w") as tar:
                for name, data in fixtures.items():
                    info = tarfile.TarInfo("./" + name); info.size = len(data); info.mode = 0o600
                    tar.addfile(info, io.BytesIO(data))
            code, _, err, failure, why = docker(["docker", "exec", "-i", "-w", "/work", container_id, "tar", "-x", "-f", "-"], archive.getvalue(), limit=4096, seconds=30)
            if phases["profile"] is not None:
                pass
            elif failure or code != 0 or err:
                phases["profile"] = observation("runner_gap", err, code, why or "fixture staging failed")
            else:
                phase_archive = io.BytesIO()
                phase_sources = {
                    ".phego-profile.php": b"<?php echo PHP_VERSION, \"\\n\", PHP_INT_SIZE; ?>",
                    ".phego-skipif.php": skipif,
                }
                with tarfile.open(fileobj=phase_archive, mode="w") as tar:
                    for name, data in phase_sources.items():
                        info = tarfile.TarInfo(name); info.size = len(data); info.mode = 0o444
                        tar.addfile(info, io.BytesIO(data))
                code, _, err, failure, why = docker(
                        ["docker", "exec", "-i", "-w", "/work", container_id, "tar", "-x", "-f", "-"],
                    phase_archive.getvalue(), limit=4096, seconds=30)
                if failure or code != 0 or err:
                    phases["profile"] = observation("runner_gap", err, code, why or "phase staging failed")
                else:
                    expected_int_size = b"4" if platform in ("linux/386", "linux/arm/v7") else b"8"
                    code, output, err, failure, why = docker(
                        ["docker", "exec", "-i", "-w", "/work", container_id, executable, "-n", "/work/.phego-profile.php"],
                        limit=output_limit, seconds=timeout)
                    profile_ok = output == release.encode("ascii") + b"\n" + expected_int_size
                    if failure:
                        phases["profile"] = observation(failure, output, code, why)
                    elif code in (125, 126, 127):
                        phases["profile"] = observation("runner_gap", output, code, "Docker exec failure is ambiguous")
                    elif err:
                        phases["profile"] = observation("runner_gap", output, code, "profile wrote to stderr: " + repr(err))
                    elif not profile_ok:
                        phases["profile"] = observation("profile_mismatch", output, code, "PHP release or integer size mismatch")
                    elif code != 0:
                        phases["profile"] = observation("runner_gap", output, code, "profile exited unsuccessfully")
                    else:
                        phases["profile"] = observation("completed", output, code)
                    current_pids, process_error = process_snapshot(container_id)
                    if current_pids is None or current_pids != baseline_pids:
                        phases["profile"] = observation("runner_gap", phases["profile"].output,
                                                         phases["profile"].exit_code,
                                                         process_error or "profile left additional container processes")
                if phases["profile"].status == "completed":
                    code, output, err, failure, why = docker(["docker", "exec", "-i", "-w", "/work", container_id, executable, "-n", "/work/.phego-skipif.php"], limit=output_limit, seconds=timeout)
                    if failure:
                        phases["skipif"] = observation(failure, output, code, why)
                    elif code in (125, 126, 127):
                        phases["skipif"] = observation("runner_gap", output, code, "Docker exec failure is ambiguous")
                    elif err:
                        phases["skipif"] = observation("runner_gap", output, code, "SKIPIF wrote to stderr: " + repr(err))
                    else:
                        phases["skipif"] = observation("completed" if code == 0 else "runner_gap", output, code,
                                                         "SKIPIF exited unsuccessfully" if code != 0 else "")
                        if code == 0:
                            current_pids, process_error = process_snapshot(container_id)
                            if current_pids is None or current_pids != baseline_pids:
                                phases["skipif"] = observation("runner_gap", output, code,
                                                               process_error or "SKIPIF left additional container processes")
                        if phases["skipif"].status == "completed":
                            # PHP's default trim() removes space, NUL, tab, LF, CR and VT.
                            token = output.strip(b" \x00\t\n\r\v")
                            if not token:
                                file_path = "/work/.phego-file.php"
                                no_clobber = "test ! -e " + file_path + " && test ! -L " + file_path + " && umask 077 && set -C && cat > " + file_path
                                code, _, err, failure, why = docker(
                                    ["docker", "exec", "-i", "-w", "/work", container_id, "sh", "-c", no_clobber],
                                    source, limit=4096, seconds=30)
                                if failure:
                                    phases["program"] = observation(failure, b"", code, why)
                                elif code != 0 or err:
                                    phases["program"] = observation("runner_gap", b"", code,
                                                                     "FILE phase path already exists or source staging failed: " + repr(err))
                                else:
                                    code, output, err, failure, why = docker(
                                        ["docker", "exec", "-i", "-w", "/work", container_id, executable, "-n", file_path],
                                        limit=output_limit, seconds=timeout)
                                    if failure:
                                        phases["program"] = observation(failure, output, code, why)
                                    elif code in (125, 126, 127):
                                        phases["program"] = observation("runner_gap", output, code, "Docker exec failure is ambiguous")
                                    elif err:
                                        phases["program"] = observation("runner_gap", output, code, "FILE wrote to stderr: " + repr(err))
                                    elif not isinstance(code, int) or code < 0 or code > 124:
                                        phases["program"] = observation("runner_gap", output, code,
                                                                         "FILE exit status may represent Docker failure or signal termination")
                                    else:
                                        phases["program"] = observation("completed", output, code,
                                                                         "" if code == 0 else "FILE exited with status " + str(code))
                                current_pids, process_error = process_snapshot(container_id)
                                if current_pids is None or current_pids != baseline_pids:
                                    phases["program"] = observation("runner_gap", phases["program"].output,
                                                                     phases["program"].exit_code,
                                                                     process_error or "FILE left additional container processes")
                            elif token[:4].lower() == b"skip":
                                phases["skipif"] = observation("skipped_unknown", output, 0, "SKIPIF requested skip; cause unknown")
                            else:
                                phases["skipif"] = observation("runner_gap", output, 0,
                                                               "non-empty SKIPIF directive is unsupported; FILE was not run")
                status = (phases["program"].status if phases["program"] else
                          phases["skipif"].status if phases["skipif"] and phases["skipif"].status != "completed" else
                          phases["profile"].status if phases["profile"] else "runner_gap")
        reason = ""
    except BaseException as exc:
        # Preserve cancellation semantics while still attempting owned-container cleanup.
        status, reason = "runner_gap", "execution interrupted: " + type(exc).__name__
        if isinstance(exc, _InterruptedBySigterm):
            interrupted_sigterm = exc
        else:
            raise
    finally:
        pending_exception = sys.exc_info()[1]
        if sigterm_event[0] is not None:
            interrupted_sigterm = _InterruptedBySigterm(*sigterm_event[0])
            sigterm_event[0] = None
            if sigterm_handler_installed:
                signal.signal(signal.SIGTERM, signal.SIG_IGN)
        try:
            rm_code, rm_out, rm_err, rm_failure, rm_why = docker(["docker", "rm", "--force", container_id], seconds=30, limit=4096)
            cleaned = not rm_failure and rm_code == 0 and rm_out.strip() == container_id.encode()
        except BaseException as cleanup_error:
            cleaned = False
            rm_why, rm_err = "cleanup interrupted: " + type(cleanup_error).__name__, b""
            if isinstance(cleanup_error, _InterruptedBySigterm):
                interrupted_sigterm = cleanup_error
                sigterm_event[0] = None
                if sigterm_handler_installed:
                    signal.signal(signal.SIGTERM, signal.SIG_IGN)
                try:
                    rm_code, rm_out, rm_err, rm_failure, rm_why = docker(
                        ["docker", "rm", "--force", container_id], seconds=30, limit=4096)
                    cleaned = not rm_failure and rm_code == 0 and rm_out.strip() == container_id.encode()
                except BaseException as retry_error:
                    cleaned = False
                    rm_why, rm_err = "cleanup retry interrupted: " + type(retry_error).__name__, b""
            elif pending_exception is None:
                cleanup_interruption = cleanup_error
        if sigterm_event[0] is not None and interrupted_sigterm is None:
            interrupted_sigterm = _InterruptedBySigterm(*sigterm_event[0])
            sigterm_event[0] = None
        if sigterm_handler_installed:
            signal.signal(signal.SIGTERM, previous_sigterm)
            sigterm_handler_installed = False
        if not cleaned:
            status, reason = "cleanup_uncertain", rm_why or rm_err.decode("utf-8", "replace") or "container removal not confirmed"
    if cleanup_interruption is not None and interrupted_sigterm is None:
        raise cleanup_interruption
    if interrupted_sigterm is None and sigterm_event[0] is not None:
        interrupted_sigterm = _InterruptedBySigterm(*sigterm_event[0])
    if interrupted_sigterm is not None:
        if previous_sigterm == signal.SIG_DFL:
            os.kill(os.getpid(), signal.SIGTERM)
        elif callable(previous_sigterm):
            previous_sigterm(interrupted_sigterm.signum, interrupted_sigterm.frame)
        return CaseObservation(status, phases["profile"], phases["skipif"], phases["program"], False,
                               cleaned, reason or "SIGTERM interrupted PHPT execution")
    return CaseObservation(status, phases["profile"], phases["skipif"], phases["program"], False, cleaned, reason)


def shutil_which(name):
    import shutil
    return shutil.which(name)
