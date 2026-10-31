#!/usr/bin/env python3
"""Observe W-03 local checks without certifying product acceptance."""
from __future__ import annotations

import argparse
import hashlib
import json
import platform
import re
import os
import selectors
from pathlib import Path
import signal
import stat
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone


_WORKER = r'''
import json, sys, unittest
from pathlib import Path
root, output, encoded = sys.argv[1:]
sys.path.insert(0, str(Path(root)))
sys.path.insert(0, str(Path(root) / 'tools'))
names = json.loads(encoded)
def emit(value):
    with Path(output).open('a', encoding='utf-8') as stream:
        stream.write(json.dumps(value) + '\n')
class Observed(unittest.TestResult):
    def __init__(self):
        super().__init__()
        self.events = []
        self.outcomes = {}
    def mark(self, test, outcome):
        parent = getattr(test, 'test_case', test)
        key = parent.id()
        if self.outcomes.get(key) != 'fail':
            self.outcomes[key] = outcome
    def startTest(self, test):
        super().startTest(test)
        self.events.append([test.id(), 'start'])
        emit([test.id(), 'start'])
    def addSuccess(self, test):
        super().addSuccess(test)
        if test.id() not in self.outcomes:
            self.mark(test, 'pass')
    def addFailure(self, test, err):
        super().addFailure(test, err)
        self.mark(test, 'fail')
    def addError(self, test, err):
        super().addError(test, err)
        self.mark(test, 'fail')
    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.mark(test, 'skip')
    def addSubTest(self, test, subtest, err):
        super().addSubTest(test, subtest, err)
        if err is not None:
            self.mark(test, 'fail')
    def addExpectedFailure(self, test, err):
        super().addExpectedFailure(test, err)
        self.mark(test, 'expected_failure')
    def addUnexpectedSuccess(self, test):
        super().addUnexpectedSuccess(test)
        self.mark(test, 'unexpected_success')
    def stopTest(self, test):
        self.events.append([test.id(), self.outcomes.get(test.id(), 'error')])
        self.events.append([test.id(), 'stop'])
        emit(self.events[-2])
        emit(self.events[-1])
        super().stopTest(test)

result = Observed()
unittest.defaultTestLoader.loadTestsFromNames(names).run(result)
for test, detail in result.failures + result.errors:
    print(test.id() + ': ' + detail, file=sys.stderr)
emit({'tests_run': result.testsRun, 'successful': result.wasSuccessful(), 'skipped': len(result.skipped), 'expected_failures': len(result.expectedFailures)})
'''


_GUARDIAN = r'''
import os, resource, signal, subprocess, sys
from pathlib import Path
marker, code, *args = sys.argv[1:]
result = 127
try:
    if os.getpgrp() != os.getpid():
        raise RuntimeError('guardian is not the process group leader')
    limit = 128 * 1024 * 1024
    resource.setrlimit(resource.RLIMIT_FSIZE, (limit, limit))
    child = subprocess.Popen([sys.executable, '-I', '-B', '-c', code, *args], stdin=subprocess.DEVNULL)
    result = child.wait()
finally:
    try:
        Path(marker).write_text(str(result), encoding='ascii')
    finally:
        if os.getpgrp() == os.getpid():
            os.killpg(os.getpgrp(), signal.SIGKILL)
'''


def _guarded_run(code: str, args: list[str], cwd: Path, env: dict, timeout: float) -> dict:
    # The guardian remains the group leader until it kills the complete group.
    # This avoids signalling a recycled process-group ID after reaping it.
    result = {'status': 'error', 'code': None, 'stdout': b'', 'stderr': b'', 'stdout_size': 0}
    with tempfile.TemporaryDirectory(prefix='phego-local-guardian-') as td:
        marker = Path(td) / 'exit.txt'
        with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
            proc = subprocess.Popen(
                [sys.executable, '-I', '-B', '-c', _GUARDIAN, str(marker), code, *args],
                cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr,
                start_new_session=True,
            )
            try:
                try:
                    guardian_code = proc.wait(timeout=timeout)
                except subprocess.TimeoutExpired:
                    result['status'] = 'timeout'
                    guardian_code = None
            finally:
                if proc.poll() is None:
                    try:
                        os.killpg(proc.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    proc.wait()
            stdout.seek(0, os.SEEK_END)
            result['stdout_size'] = stdout.tell()
            stdout.seek(max(0, result['stdout_size'] - 65536))
            result['stdout'] = stdout.read()
            stderr.seek(0, os.SEEK_END)
            stderr.seek(max(0, stderr.tell() - 65536))
            result['stderr'] = stderr.read()
            if result['status'] == 'timeout':
                return result
            if guardian_code != -signal.SIGKILL or not marker.is_file() or marker.stat().st_size > 32:
                return result
            try:
                result['code'] = int(marker.read_text(encoding='ascii'))
            except (OSError, UnicodeError, ValueError):
                return result
            result['status'] = 'completed'
            return result


def _observe_cases(root: Path, names: list[str], timeout: float) -> dict:
    """Execute an explicitly selected local suite; input test code is trusted."""
    result = {'status': 'error', 'cases': {}, 'diagnostics': '', 'product_acceptance': 'not_claimed'}
    if (not isinstance(names, list) or not names or any(not isinstance(n, str) or not n for n in names)
            or len(set(names)) != len(names) or not isinstance(timeout, (int, float))
            or isinstance(timeout, bool) or timeout <= 0):
        return result
    try:
        root = Path(root).resolve(strict=True)
        with tempfile.TemporaryDirectory(prefix='phego-local-observer-') as td:
            output = Path(td) / 'observed.json'
            env = {'PATH': os.defpath, 'HOME': td, 'TMPDIR': td, 'PYTHONDONTWRITEBYTECODE': '1'}
            observed = _guarded_run(_WORKER, [str(root), str(output), json.dumps(names)], Path(td), env, timeout)
            result['diagnostics'] = (observed['stdout'] + observed['stderr']).decode('utf-8', 'replace')
            timed_out = observed['status'] == 'timeout'
            if not timed_out and (observed['status'] != 'completed' or observed['code'] != 0):
                return result
            if not output.is_file() or output.stat().st_size > 1024 * 1024:
                if timed_out:
                    result['status'] = 'timeout'
                return result
            lines = output.read_text(encoding='utf-8').splitlines()
            entries = []
            for line in lines:
                try:
                    entries.append(json.loads(line))
                except json.JSONDecodeError:
                    if not timed_out:
                        raise
                    break
            summary = entries.pop() if entries and isinstance(entries[-1], dict) else None
            events = entries
            if timed_out:
                result['status'] = 'timeout'
                result['events'] = events
                for i in range(0, len(events) - 2, 3):
                    start, outcome, stop = events[i:i + 3]
                    if (all(isinstance(x, list) and len(x) == 2 for x in (start, outcome, stop))
                            and start[0] == outcome[0] == stop[0] and start[0] in names
                            and start[1] == 'start' and stop[1] == 'stop'):
                        result['cases'][start[0]] = outcome[1]
                    else:
                        break
                return result
            if (not isinstance(summary, dict) or summary.get('tests_run') != len(names)
                    or type(summary.get('successful')) is not bool
                    or type(summary.get('skipped')) is not int
                    or type(summary.get('expected_failures')) is not int
                    or not isinstance(events, list) or len(events) != 3 * len(names)):
                return result
            observed = {}
            for i in range(0, len(events), 3):
                start, outcome, stop = events[i:i + 3]
                if (not all(isinstance(x, list) and len(x) == 2 for x in (start, outcome, stop))
                        or start[0] != outcome[0] or start[0] != stop[0]
                        or start[1] != 'start' or stop[1] != 'stop'
                        or start[0] not in names or start[0] in observed):
                    return result
                observed[start[0]] = outcome[1]
            result['cases'] = observed
            result['events'] = events
            if (summary['successful'] and summary['skipped'] == 0 and summary['expected_failures'] == 0
                    and set(observed) == set(names) and all(v == 'pass' for v in observed.values())):
                result['status'] = 'pass'
            else:
                result['status'] = 'fail'
            return result
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        return result


_MAX_FILE = 32 * 1024 * 1024
_MAX_TOTAL = 256 * 1024 * 1024
_MAX_FILES = 10000


def _git(root: Path, *args: str) -> bytes:
    env = {k: v for k, v in os.environ.items() if not k.upper().startswith('GIT_')}
    env.update({'GIT_NO_REPLACE_OBJECTS': '1', 'GIT_CONFIG_NOSYSTEM': '1',
                'GIT_CONFIG_GLOBAL': os.devnull, 'GIT_TERMINAL_PROMPT': '0'})
    try:
        p = subprocess.run(['git', '-C', str(root), *args], env=env, stdin=subprocess.DEVNULL,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError('cannot inspect Git objects') from exc
    if p.returncode:
        raise ValueError('Git object is unavailable')
    return p.stdout


def _snapshot_commit(root: Path, commit: str, destination: Path) -> str:
    """Copy verified regular blobs from one exact commit into a private directory."""
    if not isinstance(commit, str) or not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise ValueError('commit must be a full lowercase SHA-1')
    deadline = time.monotonic() + 300
    root = Path(root).resolve(strict=True)
    destination = Path(destination).resolve(strict=True)
    if Path(_git(root, 'rev-parse', '--show-toplevel').decode().strip()).resolve() != root:
        raise ValueError('root must be the Git worktree root')
    if _git(root, 'cat-file', '-t', commit).strip() != b'commit':
        raise ValueError('revision is not a commit')
    tree = _git(root, 'rev-parse', commit + '^{tree}').decode('ascii').strip()
    raw = _git(root, 'ls-tree', '-rz', '--full-tree', commit)
    if len(raw) > 8 * 1024 * 1024:
        raise ValueError('tree listing exceeds limit')
    entries = [x for x in raw.split(b'\0') if x]
    if not entries or len(entries) > _MAX_FILES:
        raise ValueError('invalid file count')
    total = 0
    seen = set()
    checked = []
    for entry in entries:
        if time.monotonic() >= deadline:
            raise ValueError('snapshot timed out')
        try:
            meta, path_bytes = entry.split(b'\t', 1)
            mode, kind, oid = meta.decode('ascii').split(' ')
            name = path_bytes.decode('utf-8', 'strict')
        except (ValueError, UnicodeError) as exc:
            raise ValueError('invalid tree entry') from exc
        parts = name.split('/')
        if (mode not in ('100644', '100755') or kind != 'blob'
                or not re.fullmatch(r'[0-9a-f]{40}', oid) or not name
                or '\\' in name or any(part in ('', '.', '..') for part in parts)
                or name in seen):
            raise ValueError('unsafe or unsupported committed path')
        seen.add(name)
        checked.append((mode, oid, parts))

    proc = None
    selector = selectors.DefaultSelector()
    try:
        env = {k: v for k, v in os.environ.items() if not k.upper().startswith('GIT_')}
        env.update({'GIT_NO_REPLACE_OBJECTS': '1', 'GIT_CONFIG_NOSYSTEM': '1',
                    'GIT_CONFIG_GLOBAL': os.devnull, 'GIT_TERMINAL_PROMPT': '0'})
        try:
            proc = subprocess.Popen(
                ['git', '-C', str(root), 'cat-file', '--batch'], env=env,
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                bufsize=0,
            )
        except OSError as exc:
            raise ValueError('cannot start Git batch reader') from exc
        assert proc.stdin is not None and proc.stdout is not None
        in_fd, out_fd = proc.stdin.fileno(), proc.stdout.fileno()
        os.set_blocking(in_fd, False)
        os.set_blocking(out_fd, False)
        def ready(fd: int, event: int) -> None:
            for key in list(selector.get_map().values()):
                selector.unregister(key.fd)
            selector.register(fd, event)
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise ValueError('snapshot timed out')
                if selector.select(remaining):
                    return

        def write_request(payload: bytes) -> None:
            offset = 0
            while offset < len(payload):
                ready(in_fd, selectors.EVENT_WRITE)
                try:
                    offset += os.write(in_fd, payload[offset:])
                except BlockingIOError:
                    continue
                except OSError as exc:
                    raise ValueError('Git batch reader failed') from exc

        def read_exact(size: int) -> bytes:
            chunks = bytearray()
            while len(chunks) < size:
                ready(out_fd, selectors.EVENT_READ)
                try:
                    chunk = os.read(out_fd, min(65536, size - len(chunks)))
                except BlockingIOError:
                    continue
                except OSError as exc:
                    raise ValueError('Git batch reader failed') from exc
                if not chunk:
                    raise ValueError('truncated Git batch response')
                chunks.extend(chunk)
            return bytes(chunks)

        def read_header() -> bytes:
            line = bytearray()
            while len(line) <= 128:
                char = read_exact(1)
                if char == b'\n':
                    return bytes(line)
                line.extend(char)
            raise ValueError('invalid Git batch header')

        for mode, oid, parts in checked:
            if time.monotonic() >= deadline:
                raise ValueError('snapshot timed out')
            write_request(oid.encode('ascii') + b'\n')
            header = read_header().split(b' ')
            if len(header) != 3 or header[0] != oid.encode('ascii') or header[1] != b'blob':
                raise ValueError('invalid Git batch header')
            if not header[2].isdigit():
                raise ValueError('invalid blob size')
            size = int(header[2])
            total += size
            if size > _MAX_FILE or total > _MAX_TOTAL:
                raise ValueError('committed tree exceeds size limit')
            data = read_exact(size)
            if read_exact(1) != b'\n':
                raise ValueError('invalid Git batch framing')
            if hashlib.sha1(b'blob ' + str(size).encode() + b'\0' + data).hexdigest() != oid:
                raise ValueError('committed blob does not match its object ID')
            target = destination.joinpath(*parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            try:
                with target.open('xb') as f:
                    f.write(data)
            except OSError as exc:
                raise ValueError('cannot create snapshot file') from exc
            if mode == '100755':
                target.chmod(0o755)
        proc.stdin.close()
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ValueError('snapshot timed out')
        try:
            code = proc.wait(timeout=remaining)
        except subprocess.TimeoutExpired as exc:
            raise ValueError('snapshot timed out') from exc
        if code != 0:
            raise ValueError('Git batch reader failed')
    finally:
        selector.close()
        if proc is not None:
            for stream in (proc.stdin, proc.stdout):
                if stream is not None and not stream.closed:
                    stream.close()
            if proc.poll() is None:
                proc.kill()
            proc.wait()
    if time.monotonic() >= deadline:
        raise ValueError('snapshot timed out')
    return tree


_CORE_CASES = {
    'test_work_status.WorkStatusTest.test_requirement_with_missing_test_is_rejected',
    'test_work_status.WorkStatusTest.test_dependency_cycle_in_plan_is_rejected',
    'test_w03_local_gate.ObservedExecutionTests.test_missing_or_empty_or_duplicate_cases_are_rejected',
}
_SCOPE = 'W-03 local tool checks; not product acceptance'


def _read_manifest(path: Path) -> list[str]:
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError('duplicate manifest key')
            result[key] = value
        return result
    try:
        data = path.read_bytes()
        if len(data) > 1024 * 1024:
            raise ValueError('manifest exceeds limit')
        obj = json.loads(data.decode('utf-8'), object_pairs_hook=unique)
    except (OSError, UnicodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ValueError('cannot read check manifest') from exc
    if (not isinstance(obj, dict) or set(obj) != {'schema_version', 'scope', 'cases'}
            or type(obj['schema_version']) is not int or obj['schema_version'] != 1
            or obj['scope'] != _SCOPE or not isinstance(obj['cases'], list)):
        raise ValueError('invalid check manifest')
    names = obj['cases']
    if (not names or len(names) > _MAX_FILES
            or any(not isinstance(n, str) or not re.fullmatch(r'[A-Za-z_]\w*(?:\.[A-Za-z_]\w*){2,}', n, re.ASCII) for n in names)
            or len(set(names)) != len(names) or not _CORE_CASES.issubset(names)):
        raise ValueError('check manifest is incomplete or ambiguous')
    return names


_DISCOVER = r'''
import json, sys, unittest
from pathlib import Path
root = Path(sys.argv[1])
sys.path.insert(0, str(root))
suite = unittest.defaultTestLoader.discover(str(root / 'tools'), pattern='test_*.py')
def flatten(group):
    for item in group:
        if isinstance(item, unittest.TestSuite):
            yield from flatten(item)
        else:
            yield item.id()
print(json.dumps(list(flatten(suite))))
'''


def _discover_cases(root: Path, timeout: int) -> list[str]:
    env = {'PATH': os.defpath, 'HOME': str(root), 'TMPDIR': tempfile.gettempdir(), 'PYTHONDONTWRITEBYTECODE': '1'}
    try:
        observed = _guarded_run(_DISCOVER, [str(root)], root, env, min(timeout, 120))
    except OSError as exc:
        raise ValueError('test discovery failed') from exc
    if observed['status'] != 'completed' or observed['code'] != 0 or observed['stdout_size'] > 65536:
        raise ValueError('test discovery failed or timed out')
    try:
        value = json.loads(observed['stdout'].decode('utf-8'))
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise ValueError('invalid test discovery output') from exc
    if not isinstance(value, list) or any(not isinstance(n, str) or '_FailedTest' in n for n in value):
        raise ValueError('test discovery contains an import failure')
    return value


def _snapshot_fingerprint(root: Path) -> str:
    entries = []
    total = 0
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs:
            path = Path(directory) / name
            metadata = path.lstat()
            if not stat.S_ISDIR(metadata.st_mode):
                raise ValueError('snapshot contains an unsupported directory')
            entries.append((path.relative_to(root).as_posix() + '/', '', metadata.st_mode & 0o777))
        for name in files:
            path = Path(directory) / name
            fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
            with os.fdopen(fd, 'rb') as stream:
                metadata = os.fstat(stream.fileno())
                if not stat.S_ISREG(metadata.st_mode) or metadata.st_size > _MAX_FILE:
                    raise ValueError('snapshot contains an unsupported or oversized file')
                data = stream.read(_MAX_FILE + 1)
                if len(data) > _MAX_FILE:
                    raise ValueError('snapshot file exceeds size limit')
            total += len(data)
            if total > _MAX_TOTAL:
                raise ValueError('snapshot exceeds total size limit')
            entries.append((path.relative_to(root).as_posix(), hashlib.sha256(data).hexdigest(), metadata.st_mode & 0o777))
        if len(entries) > _MAX_FILES:
            raise ValueError('snapshot exceeds entry count limit')
    return hashlib.sha256(json.dumps(sorted(entries), separators=(',', ':')).encode()).hexdigest()


class _Cancelled(BaseException):
    def __init__(self, signum: int):
        self.signum = signum


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog=(
        'Example: python3 tools/w03_local_gate.py --commit <full-commit-sha>. '
        'Runs trusted committed test code, not a security sandbox. '
        'Pass means only that the declared W-03 tool checks ran successfully.'))
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1], help='Git worktree root')
    parser.add_argument('--commit', required=True, help='full lowercase SHA-1 of the trusted source commit')
    parser.add_argument('--timeout', type=int, default=1800, help='test-suite timeout in seconds (1-3600; default 1800)')
    args = parser.parse_args()
    if not 1 <= args.timeout <= 3600:
        parser.error('--timeout must be between 1 and 3600')
    previous = {}
    def cancel(signum, _frame):
        raise _Cancelled(signum)
    for sig in (signal.SIGINT, signal.SIGTERM):
        previous[sig] = signal.signal(sig, cancel)
    try:
        with tempfile.TemporaryDirectory(prefix='phego-w03-gate-') as td:
            snapshot = Path(td) / 'source'
            snapshot.mkdir()
            tree = _snapshot_commit(args.root, args.commit, snapshot)
            tool = snapshot / 'tools/w03_local_gate.py'
            if tool.read_bytes() != Path(__file__).read_bytes():
                raise ValueError('running gate differs from selected commit')
            manifest = snapshot / 'tools/w03-local-checks.json'
            names = _read_manifest(manifest)
            before = _snapshot_fingerprint(snapshot)
            discovered = _discover_cases(snapshot, args.timeout)
            if len(discovered) != len(set(discovered)) or set(discovered) != set(names):
                raise ValueError('discovered cases do not match committed manifest')
            if _snapshot_fingerprint(snapshot) != before:
                raise ValueError('snapshot changed during discovery')
            start = datetime.now(timezone.utc).isoformat()
            outcome = _observe_cases(snapshot, names, args.timeout)
            end = datetime.now(timezone.utc).isoformat()
            if _snapshot_fingerprint(snapshot) != before:
                raise ValueError('snapshot changed during checks')
            paths = ('docs/data/phego-requirements.json', 'docs/data/phego-acceptance-tests.json',
                     'docs/data/phego-work-packages.json', 'docs/implementation/github-index.json')
            result = {
                'scope': _SCOPE, 'commit': args.commit, 'tree': tree,
                'tool_sha256': hashlib.sha256(tool.read_bytes()).hexdigest(),
                'manifest_sha256': hashlib.sha256(manifest.read_bytes()).hexdigest(),
                'planning_sha256': {p: hashlib.sha256((snapshot / p).read_bytes()).hexdigest() for p in paths},
                'python_version': sys.version, 'python_executable_sha256': hashlib.sha256(Path(sys.executable).read_bytes()).hexdigest(),
                'os': platform.system(), 'architecture': platform.machine(),
                'started_utc': start, 'finished_utc': end, 'expected_count': len(names),
                'observation': outcome, 'package_acceptance': 'not_claimed', 'product_acceptance': 'not_claimed',
            }
            print(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2))
            return 0 if outcome['status'] == 'pass' else 1
    except _Cancelled as exc:
        print('w03_local_gate: interrupted', file=sys.stderr)
        return 128 + exc.signum
    except (OSError, ValueError, UnicodeError) as exc:
        print('w03_local_gate: error: ' + str(exc), file=sys.stderr)
        return 2
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)


if __name__ == '__main__':
    raise SystemExit(main())
