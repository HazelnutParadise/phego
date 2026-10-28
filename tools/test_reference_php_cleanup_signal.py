#!/usr/bin/env python3
"""Regression coverage for PHP reference probe cleanup under signals."""
import hashlib
import json
import os
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = Path(__file__).with_name("reference_php.py")
CONTAINER_ID = "a" * 64


def valid_lock(archive):
    return {
        "schema_version": 1,
        "php": {"archive_sha256": hashlib.sha256(archive).hexdigest()},
        "reference_build": {
            "platform": "linux/arm64",
            "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            "image": "buildpack-deps@sha256:" + "d" * 64,
        },
    }


class CliCleanupSignalTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "project"
        (self.root / "tools").mkdir(parents=True)
        self.script = self.root / "tools" / "reference_php.py"
        self.script.write_bytes(SCRIPT.read_bytes())
        self.archive = self.root / "php.tar.xz"
        contents = b"fake locked PHP archive"
        self.archive.write_bytes(contents)
        (self.root / "compat.lock.json").write_text(json.dumps(valid_lock(contents)))
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.events = self.root / "docker-events.jsonl"
        self.docker = self.bin / "docker"
        self.docker.write_text(
            "#!" + sys.executable + "\n"
            "import json, os, pathlib, sys, time\n"
            "args=sys.argv[1:]\n"
            "with open(os.environ['DOCKER_EVENTS'], 'a') as f: f.write(json.dumps(args)+'\\n')\n"
            "if args and args[0]=='rm':\n"
            "    time.sleep(float(os.environ.get('FAKE_RM_DELAY','0')))\n"
            "    done=os.environ.get('FAKE_RM_DONE')\n"
            "    if done: pathlib.Path(done).write_text('done')\n"
            "    sys.exit(int(os.environ.get('FAKE_RM_STATUS','0')))\n"
            "cid=args[args.index('--cidfile')+1] if '--cidfile' in args else None\n"
            "if cid and os.environ.get('FAKE_NO_CIDFILE')!='1': pathlib.Path(cid).write_text(os.environ.get('FAKE_CONTAINER_ID','" + CONTAINER_ID + "'))\n"
            "mode=os.environ.get('FAKE_RUN_MODE','success')\n"
            "if mode=='sleep': time.sleep(60)\n"
            "sys.exit(0 if mode=='success' else 17)\n"
        )
        self.docker.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ.get("PATH", ""),
                        DOCKER_EVENTS=str(self.events))

    def start(self, **overrides):
        env = dict(self.env, **overrides)
        return subprocess.Popen([sys.executable, str(self.script), "--timeout", "1", str(self.archive)],
                                cwd=self.root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=True, start_new_session=True)

    def rows(self):
        return [json.loads(line) for line in self.events.read_text().splitlines()] if self.events.exists() else []

    def wait_for_cleanup(self, proc):
        deadline = time.monotonic() + 4
        while time.monotonic() < deadline:
            rows = self.rows()
            if len(rows) >= 2 and rows[-1][:2] == ["rm", "-f"]:
                return rows[-1]
            if proc.poll() is not None:
                break
            time.sleep(0.02)
        self.fail("fake docker rm did not start")

    def stop_group(self, proc):
        # The parent may already have exited while its fake Docker child is
        # still alive after an interrupted subprocess.run(). Always reap the
        # isolated process group, then collect the parent if it is still up.
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        if proc.poll() is None:
            try:
                proc.communicate(timeout=2)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.communicate(timeout=2)

    def test_parent_sigint_during_rm_waits_for_cleanup_and_returns_130(self):
        done = self.root / "rm-done"
        proc = self.start(FAKE_RM_DELAY="0.35", FAKE_RM_DONE=str(done))
        try:
            self.assertEqual(self.wait_for_cleanup(proc), ["rm", "-f", CONTAINER_ID])
            proc.send_signal(signal.SIGINT)
            _stdout, stderr = proc.communicate(timeout=4)
            self.assertEqual(proc.returncode, 130, stderr)
            self.assertTrue(done.exists(), "cleanup must finish after SIGINT reaches only the parent")
            self.assertEqual(self.rows()[-1], ["rm", "-f", CONTAINER_ID])
        finally:
            self.stop_group(proc)

    def test_process_group_sigint_during_rm_is_failure_with_container_id(self):
        proc = self.start(FAKE_RM_DELAY="2")
        try:
            self.assertEqual(self.wait_for_cleanup(proc), ["rm", "-f", CONTAINER_ID])
            os.killpg(proc.pid, signal.SIGINT)
            _stdout, stderr = proc.communicate(timeout=4)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn(CONTAINER_ID, stderr,
                          "interrupted rm must retain the verified container ID in its error")
        finally:
            self.stop_group(proc)

    def test_cleanup_failure_reports_container_id_and_restores_sigterm_handler(self):
        original = signal.getsignal(signal.SIGTERM)
        proc = self.start(FAKE_RM_STATUS="9")
        try:
            _stdout, stderr = proc.communicate(timeout=4)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn(CONTAINER_ID, stderr)
            self.assertEqual(self.rows()[-1], ["rm", "-f", CONTAINER_ID])
        finally:
            self.stop_group(proc)
        self.assertIs(signal.getsignal(signal.SIGTERM), original)

    def test_failed_docker_run_without_cidfile_reports_unknown_cleanup_without_rm(self):
        proc = self.start(FAKE_RUN_MODE="failure", FAKE_NO_CIDFILE="1")
        try:
            _stdout, stderr = proc.communicate(timeout=4)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn("cleanup", stderr.lower(),
                          "a started build without a cidfile must report unknown cleanup state")
            rows = self.rows()
            self.assertEqual(len(rows), 1, "must not attempt rm for an unverified container ID")
            self.assertEqual(rows[0][0], "run")
        finally:
            self.stop_group(proc)


class MainSignalStateTests(unittest.TestCase):
    """Exercise signal delivery at cidfile-read and handler restoration seams."""

    def invoke_main(self, signal_during_read=None, cleanup_error=None):
        import reference_php

        data = b"temporary archive"
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "php.tar.xz"
            archive.write_bytes(data)
            lock = valid_lock(data)
            proc = Mock()
            proc.poll.return_value = 0
            proc.wait.return_value = 0
            proc.returncode = 0
            original_handlers = {s: signal.getsignal(s) for s in (signal.SIGINT, signal.SIGTERM)}

            def fake_popen(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(CONTAINER_ID)
                return proc

            original_read = reference_php._cidfile_state
            removed = []

            def read_and_signal(path):
                result = original_read(path)
                if signal_during_read is not None:
                    signal_during_read()
                return result

            def fake_remove(cid):
                removed.append(cid)
                if cleanup_error:
                    raise RuntimeError("failed to clean up reference container " + CONTAINER_ID)

            with patch.object(reference_php, "_load_compat", return_value=lock), \
                 patch.object(reference_php, "verify_archive"), \
                 patch.object(reference_php, "_build_command", return_value=["docker", "run", "--mount", "x", "image"]), \
                 patch.object(reference_php.subprocess, "Popen", side_effect=fake_popen), \
                 patch.object(reference_php, "_cidfile_state", side_effect=read_and_signal), \
                 patch.object(reference_php, "_remove_container", side_effect=fake_remove), \
                 patch.object(sys, "argv", ["reference_php.py", str(archive)]):
                try:
                    result = reference_php.main()
                except KeyboardInterrupt:
                    result = "SIGINT escaped without an interrupt status"
            restored = {s: signal.getsignal(s) for s in original_handlers}
            return result, restored, original_handlers, removed

    def test_sigint_during_cidfile_read_is_deferred_until_cleanup_and_returns_130(self):
        seen = []

        def send_sigint():
            seen.append(True)
            signal.getsignal(signal.SIGINT)(signal.SIGINT, None)

        result, restored, original, removed = self.invoke_main(send_sigint)
        self.assertEqual(removed, [CONTAINER_ID],
                         "cidfile interruption must clean up using its validated container ID")
        self.assertEqual(result, 130)
        self.assertEqual(seen, [True])
        self.assertEqual(restored, original)

    def test_sigterm_takes_priority_over_sigint_and_both_handlers_are_restored(self):
        def send_both():
            signal.getsignal(signal.SIGTERM)(signal.SIGTERM, None)
            signal.getsignal(signal.SIGINT)(signal.SIGINT, None)

        result, restored, original, _removed = self.invoke_main(send_both)
        self.assertEqual(result, 143)
        self.assertEqual(restored, original)

    def test_sigterm_alone_returns_143_and_restores_handlers(self):
        def send_sigterm():
            signal.getsignal(signal.SIGTERM)(signal.SIGTERM, None)

        result, restored, original, _removed = self.invoke_main(send_sigterm)
        self.assertEqual(result, 143)
        self.assertEqual(restored, original)

    def test_cleanup_failure_restores_signal_handlers(self):
        result, restored, original, _removed = self.invoke_main(cleanup_error=True)
        self.assertNotEqual(result, 0)
        self.assertEqual(restored, original)

    def test_sigint_during_main_handler_restore_returns_130_after_cleanup(self):
        import reference_php

        data = b"archive for handler restoration race"
        container_id = "c" * 64
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "php.tar.xz"
            archive.write_bytes(data)
            lock = valid_lock(data)
            old_handlers = {s: signal.getsignal(s) for s in (signal.SIGINT, signal.SIGTERM)}
            installed = {}
            restored = set()
            proc = Mock()
            proc.poll.return_value = 0
            proc.wait.return_value = 0
            proc.returncode = 0

            def fake_popen(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                return proc

            def controlled_signal(signum, handler):
                if signum not in installed:
                    installed[signum] = handler
                    return old_handlers[signum]
                previous = installed[signum]
                if handler is old_handlers[signum]:
                    if signum == signal.SIGINT:
                        previous(signum, None)
                    installed[signum] = handler
                    restored.add(signum)
                    return previous
                installed[signum] = handler
                return previous

            with patch.object(reference_php, "_load_compat", return_value=lock), \
                 patch.object(reference_php, "verify_archive"), \
                 patch.object(reference_php, "_build_command", return_value=["docker", "run", "--mount", "x", "image"]), \
                 patch.object(reference_php.subprocess, "Popen", side_effect=fake_popen) as popen, \
                 patch.object(reference_php, "_remove_container") as remove, \
                 patch.object(reference_php.signal, "signal", side_effect=controlled_signal), \
                 patch.object(sys, "argv", ["reference_php.py", str(archive)]):
                result = reference_php.main()

            self.assertEqual(result, 130,
                             "SIGINT during handler restoration must override the precomputed success status")
            popen.assert_called_once()
            remove.assert_called_once_with(container_id)
            self.assertEqual(restored, {signal.SIGINT, signal.SIGTERM})
            self.assertIs(installed[signal.SIGINT], old_handlers[signal.SIGINT])
            self.assertIs(installed[signal.SIGTERM], old_handlers[signal.SIGTERM])

    def test_sigint_raised_by_restored_default_handler_does_not_skip_sigterm_restore(self):
        import reference_php

        data = b"archive for handler restoration ordering"
        container_id = "d" * 64
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "php.tar.xz"
            archive.write_bytes(data)
            lock = valid_lock(data)
            old_handlers = {s: signal.getsignal(s) for s in (signal.SIGINT, signal.SIGTERM)}
            installed = {}
            proc = Mock()
            proc.poll.return_value = 0
            proc.wait.return_value = 0
            proc.returncode = 0

            def fake_popen(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                return proc

            def controlled_signal(signum, handler):
                if signum not in installed:
                    installed[signum] = handler
                    return old_handlers[signum]
                previous = installed[signum]
                installed[signum] = handler
                if signum == signal.SIGINT and handler is old_handlers[signum]:
                    # Invoke the restored default handler synchronously. It
                    # raises KeyboardInterrupt and would skip later restores
                    # unless the cleanup uses nested/finally-safe restoration.
                    handler(signum, None)
                return previous

            with patch.object(reference_php, "_load_compat", return_value=lock), \
                 patch.object(reference_php, "verify_archive"), \
                 patch.object(reference_php, "_build_command", return_value=["docker", "run", "--mount", "x", "image"]), \
                 patch.object(reference_php.subprocess, "Popen", side_effect=fake_popen) as popen, \
                 patch.object(reference_php, "_remove_container") as remove, \
                 patch.object(reference_php.signal, "signal", side_effect=controlled_signal), \
                 patch.object(sys, "argv", ["reference_php.py", str(archive)]):
                with self.assertRaises(KeyboardInterrupt):
                    reference_php.main()

            popen.assert_called_once()
            remove.assert_called_once_with(container_id)
            self.assertIs(installed[signal.SIGINT], old_handlers[signal.SIGINT])
            self.assertIs(installed[signal.SIGTERM], old_handlers[signal.SIGTERM],
                          "SIGINT while restoring its default handler must not skip SIGTERM restoration")


if __name__ == "__main__":
    sys.path.insert(0, str(ROOT / "tools"))
    unittest.main()
