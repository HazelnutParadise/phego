"""Regression coverage for batched, verified Git snapshot reads."""
import os
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

import w03_local_gate


class SnapshotBatchTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.root = base / 'repo'
        self.root.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Snapshot test')
        self.git('config', 'user.email', 'snapshot@example.invalid')
        self.destination = base / 'snapshot'
        self.destination.mkdir()
        self.wrapper_dir = base / 'bin'
        self.wrapper_dir.mkdir()
        self.log = base / 'git-invocations'
        self.real_git = subprocess.check_output(['which', 'git'], text=True).strip()
        wrapper = self.wrapper_dir / 'git'
        wrapper.write_text(
            '#!/usr/bin/env python3\n'
            'import os, subprocess, sys, time\n'
            f"with open({str(self.log)!r}, 'a', encoding='utf-8') as f: f.write(' '.join(sys.argv[1:]) + '\\n')\n"
            "args = sys.argv[1:]\n"
            "is_cat_file = 'cat-file' in args\n"
            "hold = os.environ.get('PHEGO_TEST_BATCH_HOLD')\n"
            "if is_cat_file and '--batch' in args and hold:\n"
            "    if hold == 'header':\n"
            "        with open(os.environ['PHEGO_TEST_BATCH_PID'], 'w', encoding='ascii') as f: f.write(str(os.getpid()) + ' header')\n"
            "        while True: time.sleep(60)\n"
            "    request = sys.stdin.buffer.readline().strip().decode('ascii')\n"
            "    root = args[args.index('-C') + 1]\n"
            f"    size = int(subprocess.check_output([{self.real_git!r}, '-C', root, 'cat-file', '-s', request], text=True).strip())\n"
            "    data = subprocess.check_output([" + repr(self.real_git) + ", '-C', root, 'cat-file', 'blob', request])\n"
            "    sys.stdout.buffer.write((request + ' blob ' + str(size) + '\\n').encode('ascii')); sys.stdout.buffer.flush()\n"
            "    if hold == 'body':\n"
            "        sys.stdout.buffer.write(data[:max(1, len(data) // 2)]); sys.stdout.buffer.flush()\n"
            "        with open(os.environ['PHEGO_TEST_BATCH_PID'], 'w', encoding='ascii') as f: f.write(str(os.getpid()) + ' body')\n"
            "        while True: time.sleep(60)\n"
            "    sys.stdout.buffer.write(data + b'\\n'); sys.stdout.buffer.flush()\n"
            "    with open(os.environ['PHEGO_TEST_BATCH_PID'], 'w', encoding='ascii') as f: f.write(str(os.getpid()) + ' exit')\n"
            "    while True: time.sleep(60)\n"
            "if is_cat_file and '--batch' in args:\n"
            "    fault = os.environ.get('PHEGO_TEST_BATCH_FAULT')\n"
            "    if fault == 'abort': sys.exit(41)\n"
            "    if fault == 'bad-header': sys.stdout.buffer.write(b'not-a-batch-header\\n'); sys.exit(0)\n"
            "    if fault in ('truncated', 'corrupt-data'):\n"
            "        request = sys.stdin.buffer.readline().strip().decode('ascii')\n"
            "        root = args[args.index('-C') + 1]\n"
            f"        size = int(subprocess.check_output([{self.real_git!r}, '-C', root, 'cat-file', '-s', request], text=True).strip())\n"
            "        sys.stdout.buffer.write((request + ' blob ' + str(size) + '\\n').encode('ascii'))\n"
            "        if fault == 'truncated': sys.stdout.buffer.write(b'x' * max(0, size - 1))\n"
            "        else: sys.stdout.buffer.write(b'X' * size + b'\\n')\n"
            "        sys.stdout.buffer.flush(); sys.exit(0)\n"
            "if is_cat_file and os.environ.get('PHEGO_TEST_CAT_FILE_FAIL'): sys.exit(41)\n"
            f"os.execv({self.real_git!r}, [{self.real_git!r}, *args])\n",
            encoding='utf-8',
        )
        wrapper.chmod(0o755)
        self.env = os.environ.copy()
        self.env['PATH'] = str(self.wrapper_dir) + os.pathsep + self.env.get('PATH', '')
        self.commit = None

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], text=True).strip()

    def commit_files(self, files):
        for name, (data, mode) in files.items():
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
            target.chmod(mode)
        self.git('add', '--all')
        self.git('commit', '-qm', 'fixture')
        self.commit = self.git('rev-parse', 'HEAD')

    def snapshot(self):
        with patch.dict(os.environ, self.env):
            return w03_local_gate._snapshot_commit(self.root, self.commit, self.destination)

    def test_reads_many_binary_blobs_with_bounded_cat_file_processes(self):
        payloads = {
            'binary.bin': (b'\x00first\n\xff\x00last\n', 0o644),
            'nested/deeper/newline.bin': (b'line one\nline two\n\x00', 0o644),
            'duplicate-a.bin': (b'repeated\x00blob\n', 0o644),
            'duplicate-b.bin': (b'repeated\x00blob\n', 0o644),
            'nested/run-me': (b'#!/bin/sh\nexit 0\n', 0o755),
        }
        for index in range(36):
            payloads[f'nested/group-{index // 6}/file-{index:02}.dat'] = (
                f'file={index}\n'.encode() + bytes([index, 0, 255]), 0o644)
        self.commit_files(payloads)
        expected_tree = self.git('rev-parse', f'{self.commit}^{{tree}}')

        self.assertEqual(self.snapshot(), expected_tree)

        for name, (data, mode) in payloads.items():
            target = self.destination / name
            self.assertEqual(target.read_bytes(), data, name)
            self.assertEqual(target.stat().st_mode & 0o777, mode, name)
        invocations = self.log.read_text(encoding='utf-8').splitlines()
        cat_file_processes = [line for line in invocations if 'cat-file' in line]
        self.assertLessEqual(
            len(cat_file_processes), 3,
            f'{len(payloads)} blobs caused {len(cat_file_processes)} cat-file processes',
        )

    def test_snapshots_selected_commit_not_worktree_or_later_commit(self):
        self.commit_files({'selected.txt': (b'from selected commit\n', 0o644)})
        selected = self.commit
        (self.root / 'selected.txt').write_bytes(b'uncommitted change')
        (self.root / 'later.txt').write_bytes(b'later commit only')
        self.git('add', '--all')
        self.git('commit', '-qm', 'later')

        self.assertEqual(
            w03_local_gate._snapshot_commit(self.root, selected, self.destination),
            self.git('rev-parse', f'{selected}^{{tree}}'),
        )
        self.assertEqual((self.destination / 'selected.txt').read_bytes(), b'from selected commit\n')
        self.assertFalse((self.destination / 'later.txt').exists())

    def test_rejects_non_blob_tree_entry_without_writing_snapshot(self):
        self.commit_files({'regular.txt': (b'ok', 0o644)})
        # A committed symlink is a tree entry, but is intentionally unsupported.
        os.symlink('regular.txt', self.root / 'link')
        self.git('add', 'link')
        self.git('commit', '-qm', 'symlink')
        self.commit = self.git('rev-parse', 'HEAD')
        with self.assertRaises(ValueError):
            self.snapshot()
        self.assertFalse((self.destination / 'regular.txt').exists())

    def test_rejects_oversized_blob_without_partial_success(self):
        # Keep the fixture sparse on disk; Git still stores a real oversized blob.
        self.commit_files({'large.bin': (b'x' * (w03_local_gate._MAX_FILE + 1), 0o644)})
        with self.assertRaises(ValueError):
            self.snapshot()
        self.assertFalse((self.destination / 'large.bin').exists())

    def test_rejects_corrupt_blob_object_instead_of_reporting_success(self):
        self.commit_files({'data.bin': (b'original\x00bytes\n', 0o644)})
        oid = self.git('rev-parse', f'{self.commit}:data.bin')
        object_path = self.root / '.git' / 'objects' / oid[:2] / oid[2:]
        # Damage the stored loose object after recording a valid commit/tree.
        object_path.chmod(0o644)
        object_path.write_bytes(b'not a zlib-compressed Git object')
        with self.assertRaises(ValueError):
            self.snapshot()
        self.assertFalse((self.destination / 'data.bin').exists())

    def test_rejects_corrupt_or_early_terminated_cat_file_result(self):
        self.commit_files({'data.bin': (b'verified payload\x00\n', 0o644)})
        for variable in ('PHEGO_TEST_CAT_FILE_FAIL',):
            with self.subTest(fault=variable), patch.dict(os.environ, self.env):
                os.environ[variable] = '1'
                with self.assertRaises(ValueError):
                    w03_local_gate._snapshot_commit(self.root, self.commit, self.destination)
                self.assertFalse((self.destination / 'data.bin').exists())
                os.environ.pop(variable, None)
                for item in self.destination.iterdir():
                    if item.is_file():
                        item.unlink()
                    elif item.is_dir():
                        import shutil
                        shutil.rmtree(item)

    def test_rejects_invalid_truncated_corrupt_and_aborted_batch_responses(self):
        self.commit_files({'data.bin': (b'payload\x00\n', 0o644)})
        for fault in ('bad-header', 'truncated', 'corrupt-data', 'abort'):
            with self.subTest(fault=fault), patch.dict(os.environ, self.env):
                self.log.write_text('', encoding='utf-8')
                os.environ['PHEGO_TEST_BATCH_FAULT'] = fault
                error = None
                try:
                    w03_local_gate._snapshot_commit(self.root, self.commit, self.destination)
                except ValueError as exc:
                    error = exc
                invocations = self.log.read_text(encoding='utf-8').splitlines()
                self.assertTrue(
                    any('cat-file' in line and '--batch' in line for line in invocations),
                    f'{fault} injection was not reached because cat-file --batch was not invoked',
                )
                self.assertIsNotNone(error, f'{fault} batch response was accepted as successful')
                self.assertFalse((self.destination / 'data.bin').exists())
                os.environ.pop('PHEGO_TEST_BATCH_FAULT', None)
                for item in self.destination.iterdir():
                    if item.is_file():
                        item.unlink()
                    elif item.is_dir():
                        import shutil
                        shutil.rmtree(item)

    def test_sigterm_during_live_batch_stream_reaps_git_and_cancels_snapshot(self):
        self.commit_files({'data.bin': (b'payload used for streamed cancellation\x00\n', 0o644)})
        for stage in ('header', 'body', 'exit'):
            with self.subTest(stage=stage):
                pid_file = Path(self.tmp.name) / f'git-batch-{stage}.pid'
                pid_file.unlink(missing_ok=True)
                env = self.env.copy()
                env['PHEGO_TEST_BATCH_HOLD'] = stage
                env['PHEGO_TEST_BATCH_PID'] = str(pid_file)
                env['PYTHONPATH'] = str(Path(__file__).resolve().parent) + os.pathsep + env.get('PYTHONPATH', '')
                script = (
                    'import signal, sys\n'
                    'import w03_local_gate\n'
                    'def cancel(signum, _frame): raise w03_local_gate._Cancelled(signum)\n'
                    'signal.signal(signal.SIGTERM, cancel)\n'
                    'try:\n'
                    f'    tree = w03_local_gate._snapshot_commit({str(self.root)!r}, {self.commit!r}, {str(self.destination)!r})\n'
                    '    print("SNAPSHOT_SUCCESS", tree, flush=True)\n'
                    'except w03_local_gate._Cancelled as exc:\n'
                    '    print("SNAPSHOT_CANCELLED", flush=True)\n'
                    '    sys.exit(128 + exc.signum)\n'
                )
                process = subprocess.Popen(
                    [sys.executable, '-c', script], cwd=str(self.root), env=env,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                    start_new_session=True,
                )
                git_pid = None
                try:
                    deadline = time.monotonic() + 8
                    while time.monotonic() < deadline and process.poll() is None:
                        if pid_file.exists():
                            try:
                                pid_parts = pid_file.read_text(encoding='ascii').split()
                                git_pid = int(pid_parts[0])
                                if len(pid_parts) == 2 and pid_parts[1] == stage:
                                    break
                                git_pid = None
                            except (OSError, ValueError):
                                pass
                        time.sleep(0.02)
                    self.assertIsNotNone(git_pid, f'{stage} stage did not start before deadline')
                    os.kill(process.pid, signal.SIGTERM)
                    stdout, stderr = process.communicate(timeout=5)
                    self.assertEqual(process.returncode, 128 + signal.SIGTERM, stderr)
                    self.assertIn('SNAPSHOT_CANCELLED', stdout)
                    self.assertNotIn('SNAPSHOT_SUCCESS', stdout)
                    if stage != 'exit':
                        self.assertFalse((self.destination / 'data.bin').exists())

                    reap_deadline = time.monotonic() + 3
                    while time.monotonic() < reap_deadline:
                        try:
                            os.kill(git_pid, 0)
                        except ProcessLookupError:
                            break
                        time.sleep(0.02)
                    else:
                        self.fail(f'{stage} cat-file process {git_pid} was not reaped')
                finally:
                    owned_group = False
                    if process.poll() is None:
                        owned_group = True
                    elif git_pid is not None:
                        try:
                            owned_group = os.getpgid(git_pid) == process.pid
                        except ProcessLookupError:
                            pass
                    if owned_group:
                        try:
                            os.killpg(process.pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                    if process.poll() is None:
                        try:
                            process.communicate(timeout=3)
                        except subprocess.TimeoutExpired:
                            process.kill()
                            process.wait(timeout=3)
                    else:
                        process.communicate(timeout=3)
                    for item in self.destination.iterdir():
                        if item.is_file():
                            item.unlink()
                        elif item.is_dir():
                            import shutil
                            shutil.rmtree(item)

    def test_expired_snapshot_deadline_does_not_report_success(self):
        self.commit_files({f'file-{i}.txt': (b'contents', 0o644) for i in range(4)})
        original_monotonic = w03_local_gate.time.monotonic
        for values in ([0.0, 301.0], [0.0, 0.0, 0.0, 301.0]):
            with self.subTest(clock_values=values), patch(
                'w03_local_gate.time.monotonic', side_effect=values
            ), self.assertRaises(ValueError):
                self.snapshot()
            for item in self.destination.iterdir():
                if item.is_file():
                    item.unlink()
                elif item.is_dir():
                    import shutil
                    shutil.rmtree(item)
            self.assertFalse(any(self.destination.iterdir()))
        self.assertIsNotNone(original_monotonic)


if __name__ == '__main__':
    unittest.main()
