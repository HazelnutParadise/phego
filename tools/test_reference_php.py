#!/usr/bin/env python3
import hashlib
import io
import re
import signal
import subprocess
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from unittest.mock import Mock, patch

from reference_php import build_command, main, verify_archive

_EXPECTED_PROFILE = {
    "extensions": ["Core", "Reflection", "SPL", "Zend OPcache", "date", "hash", "json", "lexbor", "pcre", "random", "standard", "uri"],
    "zend_extensions": ["Zend OPcache"],
    "session_available": False,
    "session_upload_progress": False,
}

class TestVerifyArchive(unittest.TestCase):
    def test_matching_regular_archive_is_accepted(self):
        data = b"reference php source archive"
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(data)
            verify_archive(archive, hashlib.sha256(data).hexdigest())

    def test_missing_archive_raises_oserror(self):
        with tempfile.TemporaryDirectory() as td:
            missing = Path(td) / "missing.tar.xz"
            with self.assertRaises(OSError):
                verify_archive(missing, "0" * 64)

    def test_directory_raises_oserror(self):
        with tempfile.TemporaryDirectory() as td:
            directory = Path(td) / "directory"
            directory.mkdir()
            with self.assertRaises(OSError):
                verify_archive(directory, "0" * 64)

    def test_sha256_mismatch_raises_valueerror(self):
        data = b"archive contents"
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(data)
            correct = hashlib.sha256(data).hexdigest()
            wrong = ("0" if correct[0] != "0" else "1") + correct[1:]
            with self.assertRaises(ValueError):
                verify_archive(archive, wrong)

    def test_malformed_digest_raises_valueerror(self):
        data = b"archive contents"
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(data)
            for digest in ("not-a-digest", "g" * 64, "0" * 63, "0" * 65):
                with self.subTest(digest=digest):
                    with self.assertRaises(ValueError):
                        verify_archive(archive, digest)


class TestBuildCommand(unittest.TestCase):
    def _archive_path(self, td, name="source.tar.xz"):
        archive = Path(td) / name
        archive.touch()
        return archive

    def test_returns_docker_argv(self):
        with tempfile.TemporaryDirectory() as td:
            argv = build_command(self._archive_path(td), 900)
        self.assertIsInstance(argv, list)
        self.assertTrue(argv)
        self.assertTrue(all(isinstance(arg, str) for arg in argv))
        self.assertEqual(argv[0], "docker")

    def test_unpinned_image_is_rejected(self):
        lock = {"schema_version": 1, "php": {"version": "8.5.12", "archive_sha256": "0" * 64},
                "reference_build": {"image": "--privileged", "platform": "linux/arm64",
                                    "expected_profile": _EXPECTED_PROFILE,
                                    "configure_options": ["--disable-all", "--enable-cli", "--without-pear"]}}
        with tempfile.TemporaryDirectory() as td, patch("reference_php._load_compat", return_value=lock):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900)

    def test_uppercase_digest_is_normalized_for_container_verification(self):
        lock = {
            "schema_version": 1,
            "php": {"version": "8.5.12", "archive_sha256": "A" * 64},
            "reference_build": {"image": "buildpack-deps@sha256:" + "0" * 64,
                                "platform": "linux/arm64",
                                "expected_profile": _EXPECTED_PROFILE,
                                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"]},
        }
        with tempfile.TemporaryDirectory() as td, patch("reference_php._load_compat", return_value=lock):
            argv = build_command(self._archive_path(td), 900)
        self.assertIn("expected=" + "a" * 64, argv[-1])

    def test_unsupported_lock_profile_is_rejected(self):
        base = {
            "schema_version": 1,
            "php": {"version": "8.5.12", "archive_sha256": "0" * 64},
            "reference_build": {
                "image": "buildpack-deps@sha256:" + "0" * 64,
                "platform": "linux/arm64",
                "expected_profile": _EXPECTED_PROFILE,
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            },
        }
        for field, value in (("schema_version", 999), ("platform", "linux/amd64"),
                             ("configure_options", ["--enable-debug"])):
            lock = {**base, "reference_build": dict(base["reference_build"])}
            (lock if field == "schema_version" else lock["reference_build"])[field] = value
            with self.subTest(field=field), tempfile.TemporaryDirectory() as td, \
                 patch("reference_php._load_compat", return_value=lock):
                with self.assertRaises((ValueError, OSError)):
                    build_command(self._archive_path(td), 900)

    def test_nonpositive_timeout_raises_valueerror(self):
        with tempfile.TemporaryDirectory() as td:
            archive = self._archive_path(td)
            for timeout in (0, -1, -900):
                with self.subTest(timeout=timeout):
                    with self.assertRaises(ValueError):
                        build_command(archive, timeout)

    def test_timeout_is_enforced_inside_the_container(self):
        with tempfile.TemporaryDirectory() as td:
            argv = build_command(self._archive_path(td), 300)
        self.assertIn("timeout", argv)
        self.assertTrue(any(arg in ("300", "300s") for arg in argv))

    def test_bind_path_with_colon_is_not_split(self):
        with tempfile.TemporaryDirectory() as td:
            archive = self._archive_path(td, "source:archive.tar.xz")
            argv = build_command(archive, 300)
            self.assertIn("--mount", argv)
            self.assertTrue(any(str(archive) in arg and ",readonly" in arg for arg in argv))

    def test_command_has_networkless_readonly_and_bounded_isolation(self):
        with tempfile.TemporaryDirectory() as td:
            argv = build_command(self._archive_path(td), 900)

        self.assertIn("--network", argv)
        self.assertEqual(argv[argv.index("--network") + 1], "none")
        self.assertIn("--read-only", argv)

        self.assertTrue(
            any(arg == "--tmpfs" or arg.startswith("--tmpfs=") for arg in argv),
            "container must provide a temporary filesystem",
        )
        self.assertTrue(
            any(arg in ("--user", "-u") or arg.startswith("--user=") for arg in argv),
            "container must run as a non-root user",
        )
        user_index = next(
            i for i, arg in enumerate(argv)
            if arg in ("--user", "-u") or arg.startswith("--user=")
        )
        user = argv[user_index].split("=", 1)[1] if "=" in argv[user_index] else argv[user_index + 1]
        self.assertNotIn(user, ("0", "root", "0:0", "root:root"))

        def option_value(option):
            for index, arg in enumerate(argv):
                if arg == option and index + 1 < len(argv):
                    return argv[index + 1]
                if arg.startswith(option + "="):
                    return arg.split("=", 1)[1]
            return None

        cpus = option_value("--cpus")
        self.assertIsNotNone(cpus, "CPU usage must be limited")
        self.assertGreater(float(cpus), 0)

        memory = option_value("--memory")
        self.assertIsNotNone(memory, "memory usage must be limited")
        self.assertRegex(memory, re.compile(r"^[1-9][0-9]*(?:[bkmg](?:i?b)?)?$", re.IGNORECASE))

        pids_limit = option_value("--pids-limit")
        self.assertIsNotNone(pids_limit, "process count must be limited")
        self.assertGreater(int(pids_limit), 0)

        self.assertIn("--cap-drop", argv)
        self.assertEqual(argv[argv.index("--cap-drop") + 1], "ALL")
        self.assertIn("--security-opt", argv)
        security_options = [argv[i + 1] for i, arg in enumerate(argv[:-1]) if arg == "--security-opt"]
        self.assertTrue(any("no-new-privileges" in option for option in security_options))

        self.assertTrue(
            any(re.search(r"@sha256:[0-9a-fA-F]{64}$", arg) for arg in argv),
            "image must be pinned by a complete sha256 digest",
        )

    def test_source_bind_is_readonly_and_paths_with_spaces_stay_one_argument(self):
        with tempfile.TemporaryDirectory() as td:
            archive = self._archive_path(td, "source archive.tar.xz")
            path = str(archive)
            argv = build_command(archive, 300)

        matching = [arg for arg in argv if path in arg]
        self.assertTrue(matching, "archive path must be included in the Docker command")
        self.assertTrue(
            any(":ro" in arg or ",readonly" in arg or ",ro" in arg for arg in matching),
            "archive bind mount must be read-only",
        )
        self.assertNotIn(path.split()[0], argv)
        self.assertNotIn(path.split()[-1], argv)


class TestContainerCleanup(unittest.TestCase):
    def test_success_with_invalid_cidfile_is_reported_as_failure(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(b"test")

            def fake_docker(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text("invalid-cid")
                proc = Mock()
                proc.wait.return_value = 0
                return proc

            errors = io.StringIO()
            with patch("sys.argv", ["reference_php.py", str(archive)]), \
                 patch("reference_php.verify_archive"), \
                 patch("reference_php.subprocess.Popen", side_effect=fake_docker), \
                 redirect_stderr(errors):
                self.assertEqual(main(), 1)
            self.assertIn("cleanup state unknown", errors.getvalue())

    def test_removal_failure_changes_success_to_failure(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(b"test")
            container_id = "a" * 64

            def fake_docker(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                proc = Mock()
                proc.wait.return_value = 0
                return proc

            errors = io.StringIO()
            with patch("sys.argv", ["reference_php.py", str(archive)]), \
                 patch("reference_php.verify_archive"), \
                 patch("reference_php.subprocess.Popen", side_effect=fake_docker), \
                 patch("reference_php.subprocess.run", return_value=Mock(returncode=1)) as remove, \
                 redirect_stderr(errors):
                self.assertEqual(main(), 1)
            remove.assert_called_once()
            self.assertEqual(remove.call_args.args[0], ["docker", "rm", "-f", container_id])
            self.assertIs(remove.call_args.kwargs["check"], False)
            self.assertGreater(remove.call_args.kwargs["timeout"], 0)
            self.assertIn("failed to clean up", errors.getvalue())

    def test_sigterm_cleans_up_the_started_container(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(b"test")
            container_id = "b" * 64
            previous_handler = signal.getsignal(signal.SIGTERM)

            def fake_docker(argv):
                Path(argv[argv.index("--cidfile") + 1]).write_text(container_id)
                proc = Mock()
                proc.poll.return_value = None
                def wait(*args, **kwargs):
                    if not proc.terminate.called:
                        signal.getsignal(signal.SIGTERM)(signal.SIGTERM, None)
                    if not proc.kill.called:
                        raise subprocess.TimeoutExpired("docker", kwargs.get("timeout", 0))
                    return 0
                proc.wait.side_effect = wait
                processes.append(proc)
                return proc

            processes = []
            with patch("sys.argv", ["reference_php.py", str(archive)]), \
                 patch("reference_php.verify_archive"), \
                 patch("reference_php.subprocess.Popen", side_effect=fake_docker), \
                 patch("reference_php.subprocess.run", return_value=Mock(returncode=0)) as remove, \
                 redirect_stderr(io.StringIO()):
                self.assertEqual(main(), 143)
            remove.assert_called_once()
            self.assertEqual(remove.call_args.args[0], ["docker", "rm", "-f", container_id])
            self.assertEqual(signal.getsignal(signal.SIGTERM), previous_handler)
            processes[0].terminate.assert_called_once()
            processes[0].kill.assert_called_once()
            self.assertTrue(any(call.kwargs.get("timeout") == 5 for call in processes[0].wait.call_args_list))


if __name__ == "__main__":
    unittest.main()
