#!/usr/bin/env python3
"""Contract tests for the isolated PHP/FI 2.0 reference helper."""
from __future__ import annotations

import hashlib
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

import history_fi2

_identity = patch.object(history_fi2, "_host_identity", return_value=(1000, 1000))


def setUpModule():
    # Tests must not depend on the account running them; root is covered by
    # its own rejection test.
    _identity.start()


def tearDownModule():
    _identity.stop()


class SnapshotTests(unittest.TestCase):
    def test_copies_regular_file_and_returns_sha256(self):
        data = b"historical reference input\x00"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, target = root / "source", root / "snapshot"
            source.write_bytes(data)

            digest = history_fi2.snapshot(source, target, len(data), hashlib.sha256(data).hexdigest())

            self.assertEqual(digest, hashlib.sha256(data).hexdigest())
            self.assertEqual(target.read_bytes(), data)

    def test_rejects_symlink_directory_oversize_digest_mismatch_and_existing_target(self):
        data = b"bounded source"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            regular, target = root / "regular", root / "target"
            regular.write_bytes(data)
            symlink = root / "link"
            symlink.symlink_to(regular)
            subdirectory = root / "directory"
            subdirectory.mkdir()
            cases = (
                (symlink, root / "symlink-copy", len(data), None),
                (subdirectory, root / "directory-copy", len(data), None),
                (regular, root / "oversize-copy", len(data) - 1, None),
                (regular, root / "mismatch-copy", len(data), "0" * 64),
            )
            for source, destination, maximum, digest in cases:
                with self.subTest(source=source.name), self.assertRaises((OSError, ValueError)):
                    history_fi2.snapshot(source, destination, maximum, digest)
                self.assertFalse(destination.exists(), "a rejected snapshot must not appear successful")

            target.write_bytes(b"keep existing data")
            with self.assertRaises((OSError, ValueError)):
                history_fi2.snapshot(regular, target, len(data))
            self.assertEqual(target.read_bytes(), b"keep existing data")

    @unittest.skipUnless(hasattr(os, "mkfifo"), "FIFO sources require POSIX")
    def test_fifo_source_is_rejected_without_blocking(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fifo = root / "source.fifo"
            os.mkfifo(fifo)
            code = (
                "import sys\n"
                "from pathlib import Path\n"
                "import history_fi2\n"
                "history_fi2._host_identity = lambda: (1000, 1000)\n"
                "source, target = map(Path, sys.argv[1:])\n"
                "try:\n"
                "    history_fi2.snapshot(source, target, 1024)\n"
                "except (OSError, ValueError):\n"
                "    raise SystemExit(0)\n"
                "raise SystemExit(3)\n"
            )
            environment = os.environ.copy()
            environment["PYTHONPATH"] = str(Path(history_fi2.__file__).resolve().parent)
            try:
                result = subprocess.run(
                    [sys.executable, "-c", code, str(fifo), str(root / "snapshot")],
                    capture_output=True,
                    text=True,
                    timeout=1.0,
                    env=environment,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.fail("snapshot blocked while opening a FIFO source")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((root / "snapshot").exists())

    def test_failed_snapshot_write_does_not_delete_replacement(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, target = root / "source", root / "snapshot"
            source.write_bytes(b"source")

            def replace_and_fail(_fd, _data):
                target.rename(root / "original")
                target.write_bytes(b"replacement")
                raise OSError("injected write failure")

            with patch.object(history_fi2.os, "write", side_effect=replace_and_fail):
                with self.assertRaises(OSError):
                    history_fi2.snapshot(source, target, 100)
            self.assertEqual(target.read_bytes(), b"replacement")


class BuildCommandTests(unittest.TestCase):
    def _paths(self, root: Path, name: str = "input file.phtml"):
        archive = root / "archive.tar.gz"
        program = root / name
        cidfile = root / "container.id"
        archive.touch()
        program.touch()
        return archive, program, cidfile

    def test_rejects_unsupported_platform_timeout_types_and_comma_mount_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive, program, cidfile = self._paths(root)
            for platform in ("linux/amd64", "darwin/arm64", ""):
                with self.subTest(platform=platform), self.assertRaises(ValueError):
                    history_fi2.build_command(archive, program, cidfile, 30, platform)
            for timeout in (0, -1, 1801, True, False):
                with self.subTest(timeout=timeout), self.assertRaises(ValueError):
                    history_fi2.build_command(archive, program, cidfile, timeout, "linux/arm64")
            for timeout in (1, 1800):
                history_fi2.build_command(archive, program, cidfile, timeout, "linux/arm64")
            comma_root = root / "comma,dir"
            comma_root.mkdir()
            comma_archive, comma_program, comma_cidfile = self._paths(comma_root)
            with self.assertRaises(ValueError):
                history_fi2.build_command(comma_archive, comma_program, comma_cidfile, 30, "linux/arm64")

    def test_root_host_identity_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory, "php.tar.gz")
            program = Path(directory, "program.phtml")
            archive.write_bytes(b"a")
            program.write_bytes(b"p")
            for identity in ((0, 1000), (1000, 0)):
                with self.subTest(identity=identity), \
                     patch.object(history_fi2, "_host_identity", return_value=identity), \
                     self.assertRaises(ValueError):
                    history_fi2.build_command(archive, program, Path(directory, "c.cid"), 30, "linux/arm64")

    def test_successful_argv_has_isolation_limits_and_keeps_paths_out_of_shell(self):
        with tempfile.TemporaryDirectory(prefix="fi2 paths ") as directory:
            root = Path(directory)
            archive, program, cidfile = self._paths(root)
            argv = history_fi2.build_command(archive, program, cidfile, 17, "linux/arm64")

        self.assertIsInstance(argv, list)
        self.assertTrue(all(isinstance(argument, str) for argument in argv))
        self.assertEqual(argv[0], "docker")
        self.assertIn(history_fi2.IMAGE, argv)
        self.assertIn("--platform", argv)
        self.assertEqual(argv[argv.index("--platform") + 1], "linux/arm64")
        self.assertIn("--network", argv)
        self.assertEqual(argv[argv.index("--network") + 1], "none")
        self.assertIn("--read-only", argv)
        self.assertIn("--cidfile", argv)
        self.assertIn("--cap-drop", argv)
        self.assertEqual(argv[argv.index("--cap-drop") + 1], "ALL")

        def option_value(option: str) -> str | None:
            for index, argument in enumerate(argv):
                if argument == option and index + 1 < len(argv):
                    return argv[index + 1]
                if argument.startswith(option + "="):
                    return argument.split("=", 1)[1]
            return None

        self.assertTrue(option_value("--user"), "container must run as a non-root user")
        self.assertNotIn(option_value("--user"), ("0", "root", "0:0", "root:root"))
        for option in ("--cpus", "--memory", "--pids-limit"):
            self.assertTrue(option_value(option), f"missing resource limit {option}")
        self.assertTrue(any(argument == "--mount" for argument in argv))
        mount_indices = [index for index, argument in enumerate(argv) if argument == "--mount"]
        self.assertEqual(len(mount_indices), 2)
        mount_values = [argv[index + 1] for index in mount_indices]
        self.assertTrue(all("readonly" in value or ",ro" in value for value in mount_values))
        self.assertTrue(any("timeout" in argument for argument in argv))
        self.assertTrue(any(argument in ("17", "17s") for argument in argv))

        user_paths = (str(archive), str(program), str(cidfile))
        self.assertTrue(all(any(path in argument for argument in argv) for path in user_paths))
        shell_scripts = [argument for argument in argv if argument.startswith("set -") or "#!/bin/" in argument]
        for path in user_paths:
            self.assertFalse(any(path in script for script in shell_scripts))


class ExecuteAndMainTests(unittest.TestCase):
    def _inputs(self, root: Path):
        archive, program = root / "source.tar.gz", root / "case.phtml"
        archive.write_bytes(b"archive")
        program.write_bytes(b"input")
        return archive, program

    def _run_execute(self, archive, program, process, cleanup_result=0):
        container_id = "a" * 64
        cidfile = archive.parent / "container.cid"
        command = ["docker", "run", "--cidfile", str(cidfile)]
        fake_subprocess = Mock()

        def start(argv, **kwargs):
            cidfile.write_text(container_id, encoding="ascii")
            return process

        fake_subprocess.Popen.side_effect = start
        fake_subprocess.run.return_value = Mock(returncode=cleanup_result)
        fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired
        with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
             patch.object(history_fi2, "build_command", return_value=command), \
             patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
            result = history_fi2.execute(archive, program, 30, "linux/arm64")
        return result, fake_subprocess, container_id

    def test_execute_cleans_started_container_by_cidfile_id_and_preserves_nonzero_result(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            process = Mock()
            process.returncode = 23
            process.communicate.return_value = (b"", b"reference failed")
            process.wait.return_value = 23
            container_id = "a" * 64
            cidfile = archive.parent / "container.cid"
            fake_subprocess = Mock()

            def start(argv, **kwargs):
                cidfile.write_text(container_id, encoding="ascii")
                return process

            fake_subprocess.Popen.side_effect = start
            fake_subprocess.run.return_value = Mock(returncode=0)
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired
            try:
                with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                     patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(cidfile)]), \
                     patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                    result = history_fi2.execute(archive, program, 30, "linux/arm64")
            except (OSError, RuntimeError, ValueError) as error:
                self.assertNotIn("success", str(error).lower())
                result = None
            self.assertTrue(fake_subprocess.Popen.called, "execute must attempt to start Docker")
            self.assertTrue(cidfile.exists(), "the started-container substitute must write its cidfile")
            self.assertEqual(cidfile.read_text(encoding="ascii"), container_id)
            if result is not None:
                self.assertEqual(result, 23)
            cleanup_argvs = [call.args[0] for call in fake_subprocess.run.call_args_list
                             if call.args and isinstance(call.args[0], list) and "rm" in call.args[0]]
            self.assertTrue(cleanup_argvs, "started containers must be cleaned up even when execute reports an error")
            self.assertTrue(any(container_id in argv for argv in cleanup_argvs),
                            "cleanup must target the actual cidfile ID")

    def test_execute_rejects_missing_cidfile_without_guessing_a_container_id(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            process = Mock()
            process.returncode = 0
            process.communicate.return_value = (b"", b"")
            fake_subprocess = Mock()
            fake_subprocess.Popen.return_value = process
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired
            with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                 patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(Path(directory) / "missing.cid")]), \
                 patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                with self.assertRaises((OSError, RuntimeError, ValueError)):
                    history_fi2.execute(archive, program, 30, "linux/arm64")
            self.assertTrue(fake_subprocess.Popen.called)
            cleanup_argvs = [call.args[0] for call in fake_subprocess.run.call_args_list
                             if call.args and isinstance(call.args[0], list) and "rm" in call.args[0]]
            self.assertFalse(cleanup_argvs, "unknown container identity must not be guessed")

    def test_execute_reports_docker_start_failure_without_cleanup_guess(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            fake_subprocess = Mock()
            fake_subprocess.Popen.side_effect = OSError("docker unavailable")
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired
            with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                 patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(Path(directory) / "container.cid")]), \
                 patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                with self.assertRaises(OSError):
                    history_fi2.execute(archive, program, 30, "linux/arm64")
            self.assertTrue(fake_subprocess.Popen.called)
            self.assertFalse(fake_subprocess.run.called)

    def test_execute_cleans_cidfile_container_when_communicate_raises_oserror(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            container_id = "f" * 64
            cidfile = Path(directory) / "communicate-error.cid"
            process = Mock()
            process.communicate.side_effect = OSError("pipe read failed")
            fake_subprocess = Mock()
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired

            def start(argv, **kwargs):
                cidfile.write_text(container_id, encoding="ascii")
                return process

            fake_subprocess.Popen.side_effect = start
            fake_subprocess.run.return_value = Mock(returncode=0)
            with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                 patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(cidfile)]), \
                 patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                with self.assertRaises(OSError):
                    history_fi2.execute(archive, program, 30, "linux/arm64")
            cleanup_argvs = [call.args[0] for call in fake_subprocess.run.call_args_list
                             if call.args and isinstance(call.args[0], list) and "rm" in call.args[0]]
            self.assertTrue(cleanup_argvs, "a communicate failure must still clean the started container")
            self.assertTrue(any(container_id in argv for argv in cleanup_argvs))

    def test_execute_uses_timeout_expired_and_cleanup_failure_is_not_success(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            timeout_process = Mock()
            timeout_process.communicate.side_effect = subprocess.TimeoutExpired("docker", 1)
            timeout_process.wait.return_value = -9
            fake_subprocess = Mock()
            container_id = "c" * 64
            cidfile = Path(directory) / "container.cid"

            def start(argv, **kwargs):
                cidfile.write_text(container_id, encoding="ascii")
                return timeout_process

            fake_subprocess.Popen.side_effect = start
            fake_subprocess.run.return_value = Mock(returncode=1)
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired
            with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                 patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(cidfile)]), \
                 patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                with self.assertRaises((OSError, RuntimeError, ValueError, subprocess.TimeoutExpired)):
                    history_fi2.execute(archive, program, 1, "linux/arm64")
            self.assertTrue(fake_subprocess.Popen.called)
            cleanup_argvs = [call.args[0] for call in fake_subprocess.run.call_args_list
                             if call.args and isinstance(call.args[0], list) and "rm" in call.args[0]]
            self.assertTrue(cleanup_argvs, "timed-out container must be cleaned up")
            self.assertTrue(any(container_id in argv for argv in cleanup_argvs))

    def test_execute_cleanup_failure_is_not_reported_as_success(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            successful_process = Mock()
            successful_process.returncode = 0
            successful_process.communicate.return_value = (b"output", b"")
            cleanup_failure_subprocess = Mock()
            cleanup_failure_subprocess.Popen.return_value = successful_process
            cleanup_failure_subprocess.run.return_value = Mock(returncode=1)
            cleanup_failure_subprocess.TimeoutExpired = subprocess.TimeoutExpired
            cleanup_failure_cid = Path(directory) / "cleanup-failure.cid"
            cleanup_id = "d" * 64

            def start_with_cleanup_id(argv, **kwargs):
                cleanup_failure_cid.write_text(cleanup_id, encoding="ascii")
                return successful_process

            cleanup_failure_subprocess.Popen.side_effect = start_with_cleanup_id
            with patch.object(history_fi2, "snapshot", return_value="b" * 64), \
                 patch.object(history_fi2, "build_command", return_value=["docker", "run", "--cidfile", str(cleanup_failure_cid)]), \
                 patch.object(history_fi2, "subprocess", cleanup_failure_subprocess, create=True):
                with self.assertRaises((OSError, RuntimeError, ValueError)):
                    history_fi2.execute(archive, program, 1, "linux/arm64")
            self.assertTrue(cleanup_failure_subprocess.Popen.called)
            cleanup_argvs = [call.args[0] for call in cleanup_failure_subprocess.run.call_args_list
                             if call.args and isinstance(call.args[0], list) and "rm" in call.args[0]]
            self.assertTrue(cleanup_argvs)
            self.assertTrue(any(cleanup_id in argv for argv in cleanup_argvs))

    def test_execute_stdout_excludes_fake_docker_cleanup_noise(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive, program = self._inputs(root)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            docker = fake_bin / "docker"
            cid = "7" * 64
            cleanup_record = root / "cleanup.txt"
            docker.write_text(
                "#!/usr/bin/env python3\n"
                "import os, sys\n"
                "args = sys.argv[1:]\n"
                "cid = " + repr(cid) + "\n"
                "if args and args[0] == 'rm':\n"
                "    if args != ['rm', '-f', cid]:\n"
                "        raise SystemExit(42)\n"
                "    open(os.environ['FI2_CLEANUP_RECORD'], 'w').write(args[2])\n"
                "    print('CLEANUP-NOISE', flush=True)\n"
                "    raise SystemExit(0)\n"
                "if not args or args[0] != 'run':\n"
                "    raise SystemExit(41)\n"
                "try:\n"
                "    index = args.index('--cidfile')\n"
                "    open(args[index + 1], 'w').write(cid)\n"
                "except (ValueError, IndexError):\n"
                "    raise SystemExit(40)\n"
                "sys.stdout.write('PROGRAM-OUTPUT\\n')\n"
                "sys.stdout.flush()\n",
                encoding="utf-8",
            )
            docker.chmod(0o755)
            environment = os.environ.copy()
            environment["PATH"] = str(fake_bin) + os.pathsep + environment.get("PATH", "")
            environment["FI2_CLEANUP_RECORD"] = str(cleanup_record)
            environment["PYTHONPATH"] = str(Path(history_fi2.__file__).resolve().parent)
            outcome = root / "execute-result.txt"
            script = (
                "import hashlib, sys\n"
                "from pathlib import Path\n"
                "import history_fi2\n"
                "history_fi2._host_identity = lambda: (1000, 1000)\n"
                "def fake_snapshot(source, target, max_bytes, expected_sha256=None):\n"
                "    data = Path(source).read_bytes()\n"
                "    Path(target).write_bytes(data)\n"
                "    return hashlib.sha256(data).hexdigest()\n"
                "history_fi2.snapshot = fake_snapshot\n"
                "try:\n"
                "    result = history_fi2.execute(Path(sys.argv[1]), Path(sys.argv[2]), 30, 'linux/arm64')\n"
                "except Exception as error:\n"
                "    Path(sys.argv[3]).write_text('error:' + str(error), encoding='utf-8')\n"
                "else:\n"
                "    Path(sys.argv[3]).write_text('result:' + str(result), encoding='utf-8')\n"
            )
            try:
                result = subprocess.run(
                    [sys.executable, "-c", script, str(archive), str(program), str(outcome)],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=6.0,
                    env=environment,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.fail("fake Docker stdout isolation exceeded the outer process limit")

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("PROGRAM-OUTPUT", result.stdout)
            self.assertNotIn("CLEANUP-NOISE", result.stdout)
            self.assertTrue(cleanup_record.exists())
            self.assertEqual(cleanup_record.read_text(encoding="ascii"), cid)
            self.assertIn("result:0", outcome.read_text(encoding="utf-8"))

    def test_execute_keeps_both_mount_snapshots_until_cleanup_callback(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, program = self._inputs(Path(directory))
            process = Mock()
            process.returncode = -15
            process.poll.return_value = None
            process.communicate.side_effect = [OSError("wait failed"), (b"", b"")]
            command_holder = []
            cleanup_id = "8" * 64
            observed_mounts = []
            cleanup_argvs = []
            fake_subprocess = Mock()
            fake_subprocess.TimeoutExpired = subprocess.TimeoutExpired

            def snapshot_to_target(source, target, max_bytes, expected_sha256=None):
                data = Path(source).read_bytes()
                Path(target).write_bytes(data)
                return hashlib.sha256(data).hexdigest()

            def start(argv, **kwargs):
                run_command = command_holder[-1]
                cidfile = Path(run_command[run_command.index("--cidfile") + 1])
                cidfile.write_text(cleanup_id, encoding="ascii")
                return process

            def cleanup(argv, **kwargs):
                cleanup_argvs.append(argv)
                mount_sources = []
                for index, arg in enumerate(command_holder[-1][:-1]):
                    if arg == "--mount":
                        value = command_holder[-1][index + 1]
                        source = value.split("source=", 1)[1].split(",", 1)[0]
                        mount_sources.append(Path(source))
                observed_mounts.extend(path.exists() for path in mount_sources)
                return Mock(returncode=0)

            original_build_command = history_fi2.build_command

            def record_build_command(*args):
                argv = original_build_command(*args)
                command_holder.append(argv)
                return argv

            fake_subprocess.Popen.side_effect = start
            fake_subprocess.run.side_effect = cleanup
            with patch.object(history_fi2, "snapshot", side_effect=snapshot_to_target), \
                 patch.object(history_fi2, "build_command", side_effect=record_build_command), \
                 patch.object(history_fi2, "subprocess", fake_subprocess, create=True):
                with self.assertRaises(OSError):
                    history_fi2.execute(archive, program, 30, "linux/arm64")
            self.assertEqual(observed_mounts, [True, True],
                             "source and program snapshots must remain through container removal")
            self.assertTrue(cleanup_argvs, "wait failure must still remove the container")
            self.assertTrue(any(argv[:3] == ["docker", "rm", "-f"] and cleanup_id in argv
                                for argv in cleanup_argvs))

    def test_execute_signal_during_cleanup_fails_after_fake_container_is_removed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive, program = self._inputs(root)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            docker = fake_bin / "docker"
            cid = "6" * 64
            cleanup_started = root / "cleanup-started.txt"
            cleanup_done = root / "cleanup-done.txt"
            docker.write_text(
                "#!/usr/bin/env python3\n"
                "import os, signal, sys, time\n"
                "args = sys.argv[1:]\n"
                "cid = " + repr(cid) + "\n"
                "if args and args[0] == 'rm':\n"
                "    if args != ['rm', '-f', cid]:\n"
                "        raise SystemExit(42)\n"
                "    open(os.environ['FI2_CLEANUP_STARTED'], 'w').write(args[2])\n"
                "    os.kill(int(os.environ['FI2_EXEC_PID']), signal.SIGTERM)\n"
                "    time.sleep(0.2)\n"
                "    open(os.environ['FI2_CLEANUP_DONE'], 'w').write(args[2])\n"
                "    raise SystemExit(0)\n"
                "if not args or args[0] != 'run':\n"
                "    raise SystemExit(41)\n"
                "try:\n"
                "    index = args.index('--cidfile')\n"
                "    open(args[index + 1], 'w').write(cid)\n"
                "except (ValueError, IndexError):\n"
                "    raise SystemExit(40)\n",
                encoding="utf-8",
            )
            docker.chmod(0o755)
            environment = os.environ.copy()
            environment["PATH"] = str(fake_bin) + os.pathsep + environment.get("PATH", "")
            environment["FI2_CLEANUP_STARTED"] = str(cleanup_started)
            environment["FI2_CLEANUP_DONE"] = str(cleanup_done)
            environment["PYTHONPATH"] = str(Path(history_fi2.__file__).resolve().parent)
            outcome = root / "signal-result.txt"
            script = (
                "import hashlib, os, sys\n"
                "from pathlib import Path\n"
                "import history_fi2\n"
                "history_fi2._host_identity = lambda: (1000, 1000)\n"
                "def fake_snapshot(source, target, max_bytes, expected_sha256=None):\n"
                "    data = Path(source).read_bytes()\n"
                "    Path(target).write_bytes(data)\n"
                "    return hashlib.sha256(data).hexdigest()\n"
                "history_fi2.snapshot = fake_snapshot\n"
                "os.environ['FI2_EXEC_PID'] = str(os.getpid())\n"
                "try:\n"
                "    rc = history_fi2.execute(Path(sys.argv[1]), Path(sys.argv[2]), 30, 'linux/arm64')\n"
                "except RuntimeError as error:\n"
                "    Path(sys.argv[3]).write_text('error:' + str(error), encoding='utf-8')\n"
                "else:\n"
                "    Path(sys.argv[3]).write_text('success:' + str(rc), encoding='utf-8')\n"
            )
            try:
                result = subprocess.run(
                    [sys.executable, "-c", script, str(archive), str(program), str(outcome)],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=6.0,
                    env=environment,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.fail("SIGTERM during fake cleanup did not finish within the outer process limit")

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(cleanup_started.exists())
            self.assertEqual(cleanup_started.read_text(encoding="ascii"), cid)
            self.assertTrue(cleanup_done.exists(), "cleanup must complete after the signal")
            self.assertEqual(cleanup_done.read_text(encoding="ascii"), cid)
            outcome_text = outcome.read_text(encoding="utf-8")
            self.assertTrue(outcome_text.startswith("error:"), outcome_text)
            self.assertIn("signal", outcome_text.lower())

    def test_execute_signal_during_first_snapshot_stops_before_second_snapshot_or_docker(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive, program = self._inputs(root)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            docker = fake_bin / "docker"
            cid = "5" * 64
            docker_calls = root / "docker-calls.txt"
            docker.write_text(
                "#!/usr/bin/env python3\n"
                "import os, sys\n"
                "args = sys.argv[1:]\n"
                "with open(os.environ['FI2_DOCKER_CALLS'], 'a') as calls:\n"
                "    calls.write(' '.join(args) + '\\n')\n"
                "cid = " + repr(cid) + "\n"
                "if args and args[0] == 'rm':\n"
                "    if args != ['rm', '-f', cid]:\n"
                "        raise SystemExit(42)\n"
                "    raise SystemExit(0)\n"
                "if not args or args[0] != 'run':\n"
                "    raise SystemExit(41)\n"
                "try:\n"
                "    index = args.index('--cidfile')\n"
                "    open(args[index + 1], 'w').write(cid)\n"
                "except (ValueError, IndexError):\n"
                "    raise SystemExit(40)\n",
                encoding="utf-8",
            )
            docker.chmod(0o755)
            environment = os.environ.copy()
            environment["PATH"] = str(fake_bin) + os.pathsep + environment.get("PATH", "")
            environment["FI2_DOCKER_CALLS"] = str(docker_calls)
            environment["FI2_SNAPSHOT_CALLS"] = str(root / "snapshot-calls.txt")
            environment["PYTHONPATH"] = str(Path(history_fi2.__file__).resolve().parent)
            outcome = root / "signal-result.txt"
            script = (
                "import hashlib, os, signal, sys\n"
                "from pathlib import Path\n"
                "import history_fi2\n"
                "history_fi2._host_identity = lambda: (1000, 1000)\n"
                "def snapshot_and_signal(source, target, max_bytes, expected_sha256=None):\n"
                "    calls = Path(os.environ['FI2_SNAPSHOT_CALLS'])\n"
                "    count = len(calls.read_text().splitlines()) if calls.exists() else 0\n"
                "    with calls.open('a') as stream:\n"
                "        stream.write(str(source) + '\\n')\n"
                "    data = Path(source).read_bytes()\n"
                "    Path(target).write_bytes(data)\n"
                "    if count == 0:\n"
                "        os.kill(os.getpid(), signal.SIGTERM)\n"
                "    return hashlib.sha256(data).hexdigest()\n"
                "history_fi2.snapshot = snapshot_and_signal\n"
                "try:\n"
                "    rc = history_fi2.execute(Path(sys.argv[1]), Path(sys.argv[2]), 30, 'linux/arm64')\n"
                "except RuntimeError as error:\n"
                "    Path(sys.argv[3]).write_text('error:' + str(error), encoding='utf-8')\n"
                "else:\n"
                "    Path(sys.argv[3]).write_text('success:' + str(rc), encoding='utf-8')\n"
            )
            try:
                result = subprocess.run(
                    [sys.executable, "-c", script, str(archive), str(program), str(outcome)],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=6.0,
                    env=environment,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.fail("SIGTERM during the first snapshot exceeded the outer process limit")

            self.assertEqual(result.returncode, 0, result.stderr)
            outcome_text = outcome.read_text(encoding="utf-8")
            snapshot_calls_path = root / "snapshot-calls.txt"
            snapshot_calls = snapshot_calls_path.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(snapshot_calls), 1,
                             f"SIGTERM during first snapshot continued into later work: {snapshot_calls!r}")
            self.assertTrue(outcome_text.startswith("error:"), outcome_text)
            self.assertIn("interrupted", outcome_text.lower())
            self.assertFalse(docker_calls.exists(), "SIGTERM during snapshot must prevent Docker startup")

    def test_execute_stops_bounded_fake_docker_on_excessive_output_and_cleans_it(self):
        output_limit = history_fi2._OUTPUT_LIMIT
        fake_output_cap = 16 * 1024 * 1024
        self.assertGreater(fake_output_cap, output_limit)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive, program = self._inputs(root)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            docker = fake_bin / "docker"
            progress = root / "progress.txt"
            cleanup_record = root / "cleanup.txt"
            cid = "9" * 64
            docker.write_text(
                "#!/usr/bin/env python3\n"
                "import os, signal, sys, time\n"
                "args = sys.argv[1:]\n"
                "cid = " + repr(cid) + "\n"
                "if args and args[0] == 'rm':\n"
                "    if len(args) != 3 or args[1] != '-f' or args[2] != cid:\n"
                "        raise SystemExit(42)\n"
                "    open(os.environ['FI2_CLEANUP_RECORD'], 'w').write(args[2])\n"
                "    raise SystemExit(0)\n"
                "if not args or args[0] != 'run':\n"
                "    raise SystemExit(41)\n"
                "try:\n"
                "    index = args.index('--cidfile')\n"
                "    open(args[index + 1], 'w').write(cid)\n"
                "except (ValueError, IndexError):\n"
                "    raise SystemExit(40)\n"
                "count = 0\n"
                "def record_and_exit(signum, frame):\n"
                "    open(os.environ['FI2_PROGRESS'], 'w').write(str(count))\n"
                "    raise SystemExit(0)\n"
                "signal.signal(signal.SIGTERM, record_and_exit)\n"
                "chunk = b'x' * (64 * 1024)\n"
                "limit = 16 * 1024 * 1024\n"
                "while count < limit:\n"
                "    sys.stdout.buffer.write(chunk)\n"
                "    sys.stdout.buffer.flush()\n"
                "    count += len(chunk)\n"
                "    open(os.environ['FI2_PROGRESS'], 'w').write(str(count))\n"
                "    time.sleep(0.01)\n"
                "raise SystemExit(0)\n",
                encoding="utf-8",
            )
            docker.chmod(0o755)
            environment = os.environ.copy()
            environment["PATH"] = str(fake_bin) + os.pathsep + environment.get("PATH", "")
            environment["FI2_PROGRESS"] = str(progress)
            environment["FI2_CLEANUP_RECORD"] = str(cleanup_record)

            outcome = root / "execute-outcome.txt"
            execute_script = (
                "import hashlib, sys\n"
                "from pathlib import Path\n"
                "import history_fi2\n"
                "history_fi2._host_identity = lambda: (1000, 1000)\n"
                "def fake_snapshot(source, target, max_bytes, expected_sha256=None):\n"
                "    data = Path(source).read_bytes()\n"
                "    Path(target).write_bytes(data)\n"
                "    return hashlib.sha256(data).hexdigest()\n"
                "history_fi2.snapshot = fake_snapshot\n"
                "try:\n"
                "    history_fi2.execute(Path(sys.argv[1]), Path(sys.argv[2]), 30, 'linux/arm64')\n"
                "except RuntimeError as error:\n"
                "    Path(sys.argv[3]).write_text(str(error), encoding='utf-8')\n"
                "else:\n"
                "    Path(sys.argv[3]).write_text('unexpected success', encoding='utf-8')\n"
            )
            environment["PYTHONPATH"] = str(Path(history_fi2.__file__).resolve().parent)
            try:
                result = subprocess.run(
                    [sys.executable, "-c", execute_script, str(archive), str(program), str(outcome)],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=8.0,
                    env=environment,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.fail("excessive fake Docker output was not stopped within the outer process limit")

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("output exceeded", outcome.read_text(encoding="utf-8"))

            self.assertTrue(cleanup_record.exists(), "fake Docker must receive cleanup for its own container")
            self.assertEqual(cleanup_record.read_text(encoding="ascii"), cid)
            emitted = int(progress.read_text(encoding="ascii"))
            self.assertGreaterEqual(emitted, output_limit)
            self.assertLess(emitted, fake_output_cap,
                            "execute must stop the producer before its bounded output cap")


if __name__ == "__main__":
    unittest.main()
