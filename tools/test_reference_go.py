#!/usr/bin/env python3
"""Contract tests for the locked, isolated Go reference probe."""
import hashlib
import io
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from unittest.mock import Mock, patch

from reference_go import _remove_container, build_command


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = Path(__file__).with_name("reference_go.py")


def valid_lock():
    return {
        "schema_version": 1,
        "go": {
            "version": "1.27.3",
            "platform": "linux/arm64",
            "archive_url": "https://go.dev/dl/go1.27.3.linux-arm64.tar.gz",
            "archive_sha256": "a" * 64,
            "additional_archives": [{
                "platform": "linux/amd64",
                "archive_url": "https://go.dev/dl/go1.27.3.linux-amd64.tar.gz",
                "archive_sha256": "b" * 64,
                "probe_image": "debian@sha256:" + "c" * 64,
            }],
        },
        "reference_build": {
            "image": "buildpack-deps@sha256:" + "d" * 64,
            "platform": "linux/arm64",
        },
    }


class BuildCommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.archive = Path(self.temp.name) / "go archive.tar.gz"
        self.archive.write_bytes(b"archive")
        self.lock = valid_lock()

    def command(self, platform="linux/arm64", timeout=900, lock=None, archive=None):
        return build_command(archive or self.archive, platform, timeout,
                             self.lock if lock is None else lock)

    def test_selects_correct_locked_platform_and_image(self):
        for platform, image in (("linux/arm64", self.lock["reference_build"]["image"]),
                                ("linux/amd64", self.lock["go"]["additional_archives"][0]["probe_image"])):
            with self.subTest(platform=platform):
                argv = self.command(platform)
                self.assertEqual(argv[0], "docker")
                self.assertIn(image, argv)
                self.assertIn(platform, argv)

    def test_container_command_is_confined_and_compiles_without_network(self):
        argv = self.command()
        self.assertIn("--network", argv)
        self.assertEqual(argv[argv.index("--network") + 1], "none")
        self.assertIn("--read-only", argv)
        self.assertIn("--cap-drop", argv)
        self.assertEqual(argv[argv.index("--cap-drop") + 1], "ALL")
        self.assertIn("--security-opt", argv)
        self.assertTrue(any("no-new-privileges" in argv[i + 1]
                            for i, arg in enumerate(argv[:-1]) if arg == "--security-opt"))
        self.assertIn("--user", argv)
        self.assertNotIn(argv[argv.index("--user") + 1], ("0", "root", "0:0"))
        for option in ("--cpus", "--memory", "--pids-limit", "--platform", "--mount"):
            self.assertIn(option, argv)
        self.assertRegex(argv[argv.index("--cpus") + 1], r"^[1-9]")
        self.assertRegex(argv[argv.index("--memory") + 1], r"^[1-9][0-9]*(?:[bkmg](?:i?b)?)?$", re.I)
        self.assertGreater(int(argv[argv.index("--pids-limit") + 1]), 0)
        script = argv[-1]
        for setting in ("CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local"):
            self.assertIn(setting, script)
        self.assertIn("sha256sum", script)
        self.assertIn(self.lock["go"]["archive_sha256"], script)
        self.assertIn("go build", script)

    def test_go_home_and_build_cache_are_writable_in_read_only_container(self):
        script = self.command()[-1]
        self.assertRegex(script, r"(?:export\s+)?HOME=/tmp/[^ ;]+")
        self.assertRegex(script, r"(?:export\s+)?GOCACHE=/tmp/[^ ;]+")
        self.assertNotIn("/nonexistent", script)
        home = re.search(r"(?:export\s+)?HOME=(/tmp/[^ ;]+)", script)
        cache = re.search(r"(?:export\s+)?GOCACHE=(/tmp/[^ ;]+)", script)
        self.assertIsNotNone(home)
        self.assertIsNotNone(cache)
        self.assertIn(home.group(1), script)
        self.assertIn(cache.group(1), script)
        self.assertIn('mkdir -p "$HOME" "$GOCACHE"', script)

    def test_archive_is_copied_before_hash_check_and_only_verified_copy_is_extracted(self):
        script = self.command()[-1]
        copy = re.search(r"\bcp\s+(?:--\s+)?/input/go\.tar\.gz\s+(/tmp/[^ ;]+)", script)
        self.assertIsNotNone(copy, "copy the host-mounted archive into a private container path first")
        private_archive = copy.group(1)
        hash_check = re.search(r"sha256sum\s+([^ ;)]+)", script)
        self.assertIsNotNone(hash_check, "verify a digest for the private copy")
        self.assertEqual(hash_check.group(1), private_archive,
                         "digest verification must read the private copy")
        extract = re.search(r"tar\s+-xzf\s+([^ ;]+)", script)
        self.assertIsNotNone(extract, "extract the verified source archive")
        self.assertEqual(extract.group(1), private_archive,
                         "extract only the same copy whose digest was verified")
        self.assertLess(copy.start(), hash_check.start())
        self.assertLess(hash_check.start(), extract.start(),
                        "verify the private archive before extraction")

    def test_rejects_unsupported_platform_timeout_and_mount_delimiters(self):
        for platform in ("darwin/arm64", "linux/ppc64le", "windows/amd64"):
            with self.subTest(platform=platform), self.assertRaises((ValueError, OSError)):
                self.command(platform)
        for timeout in (0, -1, "900", True, 1.5):
            with self.subTest(timeout=timeout), self.assertRaises((ValueError, OSError)):
                self.command(timeout=timeout)
        for name in ("bad,name.tar.gz", "bad\nname.tar.gz"):
            bad = Path(self.temp.name) / name
            bad.touch()
            with self.subTest(name=name), self.assertRaises((ValueError, OSError)):
                self.command(archive=bad)

    def test_rejects_bad_or_ambiguous_lock_records(self):
        cases = []
        lock = valid_lock(); lock["schema_version"] = 9; cases.append(("schema version", lock, "linux/arm64"))
        lock = valid_lock(); del lock["go"]["archive_sha256"]; cases.append(("missing source digest", lock, "linux/arm64"))
        lock = valid_lock(); lock["go"]["archive_sha256"] = "x" * 64; cases.append(("malformed source digest", lock, "linux/arm64"))
        lock = valid_lock(); lock["go"]["version"] = "1.27.4"; cases.append(("version disagrees with archive URL", lock, "linux/arm64"))
        lock = valid_lock(); lock["go"]["additional_archives"][0]["archive_sha256"] = "z" * 64; cases.append(("malformed additional source digest", lock, "linux/amd64"))
        lock = valid_lock(); lock["go"]["additional_archives"][0]["archive_url"] = "https://go.dev/dl/go1.27.3.linux-arm64.tar.gz"; cases.append(("additional OS and CPU disagree", lock, "linux/amd64"))
        lock = valid_lock(); lock["go"]["additional_archives"].append(dict(lock["go"]["additional_archives"][0])); cases.append(("duplicate amd64", lock, "linux/amd64"))
        lock = valid_lock(); lock["go"]["additional_archives"] = []; cases.append(("missing amd64", lock, "linux/amd64"))
        lock = valid_lock(); lock["reference_build"]["image"] = "buildpack-deps:latest"; cases.append(("unpinned arm image", lock, "linux/arm64"))
        lock = valid_lock(); lock["go"]["additional_archives"][0]["probe_image"] = "debian@sha256:" + "c" * 63; cases.append(("short amd image digest", lock, "linux/amd64"))
        lock = valid_lock(); lock["go"]["platform"] = "darwin/arm64"; cases.append(("OS and CPU disagree", lock, "linux/arm64"))
        for label, lock, platform in cases:
            with self.subTest(case=label), self.assertRaises((ValueError, OSError, TypeError, KeyError)):
                self.command(platform, lock=lock)


class CleanupSignalRaceTests(unittest.TestCase):
    def test_sigint_during_handler_restoration_is_reported_as_interrupted(self):
        previous_handler = object()
        installed = {}
        calls = []

        def controlled_signal(signum, handler):
            calls.append((signum, handler))
            if len(calls) == 1:
                installed["handler"] = handler
                return previous_handler
            self.assertIs(handler, previous_handler, "the original SIGINT handler must be restored")
            # Deliver the signal in the restore operation, after the helper's
            # return value expression may already have been evaluated.
            installed["handler"](signum, None)
            return installed["handler"]

        container_id = "a" * 64
        with patch("reference_go.signal.signal", side_effect=controlled_signal), \
             patch("reference_go.subprocess.run", return_value=Mock(returncode=0)) as run:
            interrupted = _remove_container(container_id)

        self.assertTrue(interrupted, "SIGINT during handler restoration must not be reported as success")
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0][0], calls[1][0])
        self.assertIs(calls[1][1], previous_handler)
        run.assert_called_once_with(["docker", "rm", "-f", container_id], timeout=15, check=False)


class MainCleanupInterruptTests(unittest.TestCase):
    def test_sigint_while_reading_cidfile_still_cleans_container_and_returns_130(self):
        container_id = "b" * 64
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "go.tar.gz"
            data = b"archive"
            archive.write_bytes(data)
            lock = valid_lock()
            lock["go"]["archive_sha256"] = hashlib.sha256(data).hexdigest()
            previous_sigint = signal.getsignal(signal.SIGINT)
            previous_sigterm = signal.getsignal(signal.SIGTERM)
            proc = Mock()
            proc.poll.return_value = 0
            proc.returncode = 0

            def start_fake_container(argv):
                cidfile = Path(argv[argv.index("--cidfile") + 1])
                cidfile.write_text(container_id)
                return proc

            from reference_go import _cidfile_state
            calls = []

            def interrupt_during_cidfile_read(path):
                result = _cidfile_state(path)
                calls.append(path)
                if len(calls) == 1:
                    # Deliver SIGINT through the handler currently installed
                    # by main. The default handler must interrupt execution;
                    # only an installed deferral handler should return here.
                    handler = signal.getsignal(signal.SIGINT)
                    handler(signal.SIGINT, None)
                return result

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=start_fake_container) as popen, \
                 patch("reference_go._cidfile_state", side_effect=interrupt_during_cidfile_read), \
                 patch("reference_go._remove_container", return_value=False) as remove, \
                 patch("reference_go.subprocess.run") as run:
                try:
                    result = __import__("reference_go").main([str(archive)])
                except KeyboardInterrupt:
                    self.fail("default SIGINT handler escaped instead of deferring cleanup interruption")

            self.assertEqual(result, 130, "SIGINT during cidfile read must become an interrupt status")
            popen.assert_called_once()
            remove.assert_called_once_with(container_id)
            run.assert_not_called()
            self.assertEqual(len(calls), 1)
            self.assertIs(signal.getsignal(signal.SIGINT), previous_sigint,
                          "main must restore its previous SIGINT handler")
            self.assertIs(signal.getsignal(signal.SIGTERM), previous_sigterm,
                          "main must restore its previous SIGTERM handler")

    def test_sigint_while_restoring_main_handler_returns_130_after_cleanup(self):
        container_id = "c" * 64
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "go.tar.gz"
            data = b"archive for outer-finally signal race"
            archive.write_bytes(data)
            lock = valid_lock()
            lock["go"]["archive_sha256"] = hashlib.sha256(data).hexdigest()
            old_handlers = {
                signal.SIGINT: signal.getsignal(signal.SIGINT),
                signal.SIGTERM: signal.getsignal(signal.SIGTERM),
            }
            installed = {}
            restored = set()
            proc = Mock()
            proc.poll.return_value = 0
            proc.returncode = 0

            def start_fake_container(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                return proc

            def controlled_signal(signum, handler):
                if signum not in installed:
                    installed[signum] = handler
                    return old_handlers[signum]
                previous = installed[signum]
                if handler is old_handlers[signum]:
                    if signum == signal.SIGINT:
                        # The process has finished cleanup. Deliver Ctrl+C
                        # immediately before the original handler is restored.
                        previous(signum, None)
                    installed[signum] = handler
                    restored.add(signum)
                    return previous
                installed[signum] = handler
                return previous

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=start_fake_container) as popen, \
                 patch("reference_go._remove_container", return_value=False) as remove, \
                 patch("reference_go.subprocess.run") as run, \
                 patch("reference_go.signal.signal", side_effect=controlled_signal):
                result = __import__("reference_go").main([str(archive)])

            self.assertEqual(result, 130, "SIGINT during outer handler restoration must not return success")
            popen.assert_called_once()
            remove.assert_called_once_with(container_id)
            run.assert_not_called()
            self.assertEqual(restored, {signal.SIGINT, signal.SIGTERM})
            self.assertIs(installed[signal.SIGINT], old_handlers[signal.SIGINT])
            self.assertIs(installed[signal.SIGTERM], old_handlers[signal.SIGTERM])


class MainSignalBoundaryTests(unittest.TestCase):
    def _fixture(self, directory, label):
        archive = Path(directory) / (label + ".tar.gz")
        content = (label + " fixture").encode()
        archive.write_bytes(content)
        lock = valid_lock()
        lock["go"]["archive_sha256"] = hashlib.sha256(content).hexdigest()
        return archive, lock

    def _proc(self, active=False):
        proc = Mock()
        proc.returncode = None if active else 0
        proc.poll.return_value = None if active else 0
        proc.wait.return_value = -15 if active else 0
        return proc

    def test_sigint_inside_popen_is_observed_after_child_handle_is_assigned(self):
        container_id = "d" * 64
        with tempfile.TemporaryDirectory() as td:
            archive, lock = self._fixture(td, "popen-sigint")
            old_int = signal.getsignal(signal.SIGINT)
            old_term = signal.getsignal(signal.SIGTERM)
            proc = self._proc(active=True)

            def popen_before_return(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                signal.getsignal(signal.SIGINT)(signal.SIGINT, None)
                return proc

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=popen_before_return) as popen, \
                 patch("reference_go._remove_container", return_value=False) as remove:
                result = __import__("reference_go").main([str(archive)])

            self.assertEqual(result, 130)
            popen.assert_called_once()
            proc.terminate.assert_called_once()
            remove.assert_called_once_with(container_id)
            self.assertIs(signal.getsignal(signal.SIGINT), old_int)
            self.assertIs(signal.getsignal(signal.SIGTERM), old_term)

    def test_cancel_before_popen_skips_container_and_restores_handlers(self):
        with tempfile.TemporaryDirectory() as td:
            archive, lock = self._fixture(td, "prelaunch-cancel")
            old_handlers = {signal.SIGINT: signal.getsignal(signal.SIGINT),
                            signal.SIGTERM: signal.getsignal(signal.SIGTERM)}
            current, restored = {}, set()

            def controlled_signal(signum, handler):
                if signum not in current:
                    current[signum] = handler
                    if signum == signal.SIGTERM:
                        current[signal.SIGINT](signal.SIGINT, None)
                    return old_handlers[signum]
                previous = current[signum]
                current[signum] = handler
                if handler is old_handlers[signum]:
                    restored.add(signum)
                return previous

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen") as popen, \
                 patch("reference_go._remove_container") as remove, \
                 patch("reference_go.signal.signal", side_effect=controlled_signal):
                result = __import__("reference_go").main([str(archive)])

            self.assertEqual(result, 130)
            popen.assert_not_called()
            remove.assert_not_called()
            self.assertEqual(restored, {signal.SIGINT, signal.SIGTERM})
            self.assertIs(current[signal.SIGINT], old_handlers[signal.SIGINT])
            self.assertIs(current[signal.SIGTERM], old_handlers[signal.SIGTERM])

    def test_sigterm_takes_precedence_when_both_signals_arrive_during_run(self):
        container_id = "e" * 64
        with tempfile.TemporaryDirectory() as td:
            archive, lock = self._fixture(td, "dual-signal")
            old_int = signal.getsignal(signal.SIGINT)
            old_term = signal.getsignal(signal.SIGTERM)
            proc = self._proc(active=True)

            def popen_with_both_signals(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                signal.getsignal(signal.SIGINT)(signal.SIGINT, None)
                signal.getsignal(signal.SIGTERM)(signal.SIGTERM, None)
                return proc

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=popen_with_both_signals) as popen, \
                 patch("reference_go._remove_container", return_value=False) as remove:
                result = __import__("reference_go").main([str(archive)])

            self.assertEqual(result, 143, "SIGTERM status has precedence over SIGINT")
            popen.assert_called_once()
            proc.terminate.assert_called_once()
            remove.assert_called_once_with(container_id)
            self.assertIs(signal.getsignal(signal.SIGINT), old_int)
            self.assertIs(signal.getsignal(signal.SIGTERM), old_term)

    def test_cancel_cleanup_failure_overrides_signal_status_and_reports_cid(self):
        container_id = "f" * 64
        with tempfile.TemporaryDirectory() as td:
            archive, lock = self._fixture(td, "cancel-cleanup-failure")
            old_int = signal.getsignal(signal.SIGINT)
            old_term = signal.getsignal(signal.SIGTERM)
            proc = self._proc(active=True)

            def popen_then_cancel(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                signal.getsignal(signal.SIGINT)(signal.SIGINT, None)
                return proc

            errors = io.StringIO()
            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=popen_then_cancel) as popen, \
                 patch("reference_go.subprocess.run", return_value=Mock(returncode=9)) as remove, \
                 redirect_stderr(errors):
                result = __import__("reference_go").main([str(archive)])

            self.assertEqual(result, 1, "cleanup failure must override cancellation status")
            popen.assert_called_once()
            remove.assert_called_once_with(["docker", "rm", "-f", container_id], timeout=15, check=False)
            self.assertIn(container_id, errors.getvalue())
            self.assertIs(signal.getsignal(signal.SIGINT), old_int)
            self.assertIs(signal.getsignal(signal.SIGTERM), old_term)

    def test_sigint_after_restoring_sigint_still_restores_sigterm_handler(self):
        container_id = "9" * 64
        with tempfile.TemporaryDirectory() as td:
            archive, lock = self._fixture(td, "restore-handler-race")
            old_handlers = {signal.SIGINT: signal.getsignal(signal.SIGINT),
                            signal.SIGTERM: signal.getsignal(signal.SIGTERM)}
            current, restored = {}, set()
            proc = self._proc(active=False)

            def start_fake_container(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                return proc

            def controlled_signal(signum, handler):
                if signum not in current:
                    current[signum] = handler
                    return old_handlers[signum]
                previous = current[signum]
                if handler is old_handlers[signum]:
                    current[signum] = handler
                    restored.add(signum)
                    if signum == signal.SIGINT:
                        # The default SIGINT handler raises exactly in the gap
                        # before main attempts to restore the SIGTERM handler.
                        handler(signum, None)
                else:
                    current[signum] = handler
                return previous

            with patch("reference_go._load_compat", return_value=lock), \
                 patch("reference_go.verify_archive"), \
                 patch("reference_go.subprocess.Popen", side_effect=start_fake_container) as popen, \
                 patch("reference_go._remove_container", return_value=False) as remove, \
                 patch("reference_go.subprocess.run") as run, \
                 patch("reference_go.signal.signal", side_effect=controlled_signal):
                interrupted = False
                try:
                    __import__("reference_go").main([str(archive)])
                except KeyboardInterrupt:
                    interrupted = True

            self.assertTrue(interrupted, "the restored default SIGINT handler should deliver KeyboardInterrupt")
            popen.assert_called_once()
            remove.assert_called_once_with(container_id)
            run.assert_not_called()
            self.assertIn(signal.SIGINT, restored)
            self.assertIn(signal.SIGTERM, restored,
                          "SIGTERM handler must be restored even if SIGINT interrupts handler restoration")
            self.assertIs(current[signal.SIGINT], old_handlers[signal.SIGINT])
            self.assertIs(current[signal.SIGTERM], old_handlers[signal.SIGTERM])


class CliLifecycleTests(unittest.TestCase):
    """Exercise the real CLI in a disposable project with a fake Docker CLI."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "project"
        (self.root / "tools").mkdir(parents=True)
        (self.root / "tools" / "reference_go.py").write_bytes(SCRIPT.read_bytes())
        self.fixture = self.root / "go.tar.gz"
        self.fixture.write_bytes(b"locked test archive")
        lock = valid_lock()
        lock["go"]["archive_sha256"] = hashlib.sha256(self.fixture.read_bytes()).hexdigest()
        (self.root / "compat.lock.json").write_text(json.dumps(lock))
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
            "if cid and os.environ.get('FAKE_NO_CIDFILE')!='1': pathlib.Path(cid).write_text('a'*64)\n"
            "mode=os.environ.get('FAKE_RUN_MODE','success')\n"
            "if mode=='sleep': time.sleep(60)\n"
            "sys.exit({'success':0,'failure':17,'timeout':124}.get(mode, 17))\n"
        )
        self.docker.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ.get("PATH", ""),
                        DOCKER_EVENTS=str(self.events))

    def invoke(self, mode="success", **env_overrides):
        env = dict(self.env, FAKE_RUN_MODE=mode, **env_overrides)
        return subprocess.run([sys.executable, str(self.root / "tools" / "reference_go.py"),
                               "--timeout", "1", str(self.fixture)], cwd=self.root, env=env,
                              capture_output=True, text=True, timeout=8)

    def event_rows(self):
        if not self.events.exists():
            return []
        return [json.loads(line) for line in self.events.read_text().splitlines()]

    def assert_successful_probe_and_cleanup(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        rows = self.event_rows()
        self.assertEqual(len(rows), 2)
        self.assertEqual(rows[1], ["rm", "-f", "a" * 64])
        self.assertEqual(rows[0][0], "run")
        return result

    def test_successful_cli_build_tracks_and_removes_only_its_container(self):
        self.assert_successful_probe_and_cleanup()

    def test_docker_timeout_is_failure_and_container_is_removed(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        result = self.invoke("timeout")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("success", result.stdout.lower())
        rows = self.event_rows()
        self.assertEqual(rows[-1], ["rm", "-f", "a" * 64])

    def test_sigterm_returns_interrupted_status_and_cleans_container(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        proc = subprocess.Popen([sys.executable, str(self.root / "tools" / "reference_go.py"),
                                 "--timeout", "1", str(self.fixture)], cwd=self.root, env=dict(self.env, FAKE_RUN_MODE="sleep"),
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            deadline = __import__("time").monotonic() + 5
            while __import__("time").monotonic() < deadline:
                rows = self.event_rows()
                if rows and "--cidfile" in rows[0]:
                    cidfile = Path(rows[0][rows[0].index("--cidfile") + 1])
                    if cidfile.exists():
                        break
                __import__("time").sleep(0.02)
            self.assertTrue(self.event_rows() and cidfile.exists(), "fake Docker did not create a tracked container")
            proc.terminate()
            stdout, stderr = proc.communicate(timeout=8)
            self.assertEqual(proc.returncode, 143, stderr or stdout)
            self.assertEqual(self.event_rows()[-1], ["rm", "-f", "a" * 64])
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()

    def test_cleanup_failure_is_reported_even_after_successful_build(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        result = self.invoke(FAKE_RM_STATUS="9")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cleanup", result.stderr.lower())
        self.assertIn("a" * 64, result.stderr,
                      "cleanup failure must identify the verified container for follow-up")
        self.assertEqual(self.event_rows()[-1], ["rm", "-f", "a" * 64])

    def test_missing_cidfile_after_success_is_reported_as_unknown_cleanup(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        result = self.invoke(FAKE_NO_CIDFILE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cleanup", result.stderr.lower())
        self.assertEqual(len(self.event_rows()), 1, "must not remove an unverified container ID")

    def test_failed_docker_run_without_cidfile_reports_unknown_cleanup(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        result = self.invoke("failure", FAKE_NO_CIDFILE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cleanup", result.stderr.lower(),
                      "a failed Docker run without a cidfile still has unknown cleanup state")
        self.assertEqual(len(self.event_rows()), 1,
                         "must not try to remove a container using an unverified ID")

    def test_parent_sigint_during_cleanup_finishes_removing_tracked_container(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        done = self.root / "rm-finished"
        env = dict(self.env, FAKE_RUN_MODE="success", FAKE_RM_DELAY="1", FAKE_RM_DONE=str(done))
        proc = subprocess.Popen([sys.executable, str(self.root / "tools" / "reference_go.py"),
                                 "--timeout", "1", str(self.fixture)], cwd=self.root, env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            deadline = __import__("time").monotonic() + 5
            while __import__("time").monotonic() < deadline:
                rows = self.event_rows()
                if len(rows) >= 2 and rows[-1][:2] == ["rm", "-f"]:
                    break
                __import__("time").sleep(0.02)
            rows = self.event_rows()
            self.assertGreaterEqual(len(rows), 2, "fake Docker cleanup was never started")
            self.assertEqual(rows[-1], ["rm", "-f", "a" * 64])
            proc.send_signal(__import__("signal").SIGINT)
            stdout, stderr = proc.communicate(timeout=8)
            self.assertEqual(proc.returncode, 130, stderr or stdout)
            deadline = __import__("time").monotonic() + 3
            while not done.exists() and __import__("time").monotonic() < deadline:
                __import__("time").sleep(0.02)
            self.assertTrue(done.exists(), f"fake docker rm did not finish after SIGINT: {stderr}")
            self.assertEqual(self.event_rows()[-1], ["rm", "-f", "a" * 64])
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.communicate(timeout=3)

    def test_process_group_sigint_during_cleanup_is_reported_and_keeps_container_id(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        done = self.root / "group-rm-finished"
        env = dict(self.env, FAKE_RUN_MODE="success", FAKE_RM_DELAY="2", FAKE_RM_DONE=str(done))
        proc = subprocess.Popen([sys.executable, str(self.root / "tools" / "reference_go.py"),
                                 "--timeout", "1", str(self.fixture)], cwd=self.root, env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                                start_new_session=True)
        try:
            deadline = __import__("time").monotonic() + 5
            while __import__("time").monotonic() < deadline:
                rows = self.event_rows()
                if len(rows) >= 2 and rows[-1][:2] == ["rm", "-f"]:
                    break
                __import__("time").sleep(0.02)
            rows = self.event_rows()
            self.assertGreaterEqual(len(rows), 2, "fake Docker cleanup was never started")
            self.assertEqual(rows[-1], ["rm", "-f", "a" * 64])
            __import__("os").killpg(proc.pid, __import__("signal").SIGINT)
            stdout, stderr = proc.communicate(timeout=8)
            self.assertNotEqual(proc.returncode, 0, stdout)
            self.assertNotIn("probe ok", stdout.lower(), "interrupted cleanup must not report success")
            if not done.exists():
                self.assertIn("a" * 64, stderr,
                              "an interrupted rm must report the verified container ID")
            self.assertEqual(self.event_rows()[-1], ["rm", "-f", "a" * 64])
        finally:
            try:
                __import__("os").killpg(proc.pid, __import__("signal").SIGKILL)
            except ProcessLookupError:
                pass
            if proc.poll() is None:
                proc.communicate(timeout=3)

    def test_bad_archive_is_rejected_before_fake_docker_can_run(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        missing = self.root / "missing.tar.gz"
        result = subprocess.run([sys.executable, str(self.root / "tools" / "reference_go.py"), str(missing)],
                                cwd=self.root, env=self.env, capture_output=True, text=True, timeout=8)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists(), "Docker must not start for a missing archive")

    def test_nonpositive_cli_timeout_is_rejected_before_fake_docker_can_run(self):
        self.assert_successful_probe_and_cleanup()
        self.events.unlink()
        result = subprocess.run([sys.executable, str(self.root / "tools" / "reference_go.py"),
                                 "--timeout", "0", str(self.fixture)], cwd=self.root, env=self.env,
                                capture_output=True, text=True, timeout=8)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists(), "Docker must not start for an invalid timeout")


class SnapshotExecutionTests(unittest.TestCase):
    def test_hash_then_host_archive_rewrite_still_extracts_the_verified_snapshot(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            mounted = root / "input" / "go.tar.gz"
            mounted.parent.mkdir()
            original = b"archive bytes captured before verification"
            tampered = b"host changed the read-only mount after verification"
            mounted.write_bytes(original)
            tmp_root = root / "container-tmp"
            tmp_root.mkdir()
            private_copy = tmp_root / "phego-go.tar.gz"
            tar_log = root / "tar-input.json"
            fake_bin = root / "bin"
            fake_bin.mkdir()

            sha256sum = fake_bin / "sha256sum"
            sha256sum.write_text(
                "#!" + sys.executable + "\n"
                "import hashlib, os, pathlib, sys\n"
                "p=pathlib.Path(sys.argv[1])\n"
                "digest=hashlib.sha256(p.read_bytes()).hexdigest()\n"
                "if p.name != 'probe': pathlib.Path(os.environ['SOURCE_ARCHIVE']).write_bytes(os.environ['TAMPERED_BYTES'].encode())\n"
                "print(digest, p)\n"
            )
            sha256sum.chmod(0o755)

            fake_tar = fake_bin / "tar"
            fake_go_script = (
                "#!/bin/sh\n"
                "if [ \"$1\" = version ]; then echo \"go version go1.27.3 linux/arm64\"; exit 0; fi\n"
                "if [ \"$1\" = build ]; then while [ \"$1\" != -o ]; do shift; done; shift; printf '#!/bin/sh\\nexit 0\\n' > \"$1\"; chmod +x \"$1\"; exit 0; fi\n"
                "exit 2\n"
            )
            fake_tar.write_text(
                "#!" + sys.executable + "\n"
                "import hashlib, json, os, pathlib, sys\n"
                "args=sys.argv[1:]\n"
                "archive=pathlib.Path(args[args.index('-xzf')+1])\n"
                "data=archive.read_bytes()\n"
                "pathlib.Path(os.environ['TAR_LOG']).write_text(json.dumps({'path':str(archive),'sha256':hashlib.sha256(data).hexdigest(),'data':data.decode()}))\n"
                "go=pathlib.Path(os.environ['CONTAINER_TMP'])/'build/go/bin/go'\n"
                "go.parent.mkdir(parents=True, exist_ok=True)\n"
                f"go.write_text({fake_go_script!r})\n"
                "go.chmod(0o755)\n"
            )
            fake_tar.chmod(0o755)

            compat = valid_lock()
            compat["go"]["archive_sha256"] = hashlib.sha256(original).hexdigest()
            generated = build_command(mounted, "linux/arm64", 30, compat)[-1]
            script = generated.replace("/tmp", str(tmp_root)).replace("/input/go.tar.gz", str(mounted))
            env = dict(os.environ, PATH=str(fake_bin) + os.pathsep + os.environ.get("PATH", ""),
                       SOURCE_ARCHIVE=str(mounted), TAMPERED_BYTES=tampered.decode(),
                       TAR_LOG=str(tar_log), CONTAINER_TMP=str(tmp_root))
            result = subprocess.run(["/bin/sh", "-c", script], cwd=root, env=env,
                                    capture_output=True, text=True, timeout=10)

            self.assertEqual(result.returncode, 0, result.stderr or result.stdout)
            self.assertEqual(mounted.read_bytes(), tampered)
            extracted = json.loads(tar_log.read_text())
            self.assertEqual(extracted["path"], str(private_copy))
            self.assertEqual(extracted["sha256"], hashlib.sha256(original).hexdigest())
            self.assertEqual(extracted["data"], original.decode())

            # Prove the harness detects the old check-then-use race: hashing
            # the mounted input rewrites it before tar reads the same path.
            mounted.write_bytes(original)
            tar_log.unlink()
            regressed = generated.replace("cp /input/go.tar.gz /tmp/phego-go.tar.gz; ", "")
            regressed = regressed.replace("/tmp/phego-go.tar.gz", "/input/go.tar.gz")
            vulnerable = subprocess.run(["/bin/sh", "-c", regressed.replace("/tmp", str(tmp_root)).replace(
                "/input/go.tar.gz", str(mounted))], cwd=root, env=env,
                capture_output=True, text=True, timeout=10)
            self.assertEqual(vulnerable.returncode, 0, vulnerable.stderr or vulnerable.stdout)
            raced = json.loads(tar_log.read_text())
            self.assertEqual(raced["data"], tampered.decode())
            self.assertNotEqual(raced["sha256"], hashlib.sha256(original).hexdigest())


if __name__ == "__main__":
    unittest.main()
