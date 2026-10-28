import io
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from unittest.mock import Mock, patch

import reference_php


class TestAmd64CLI(unittest.TestCase):
    def setUp(self):
        self.lock = {"schema_version": 1, "php": {"version": "8.5.12", "archive_sha256": "a" * 64},
            "reference_build": {"platform": "linux/arm64", "image": "buildpack-deps@sha256:" + "b" * 64,
                                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"]},
            "additional_reference_builds": [{"platform": "linux/amd64", "image": "buildpack-deps@sha256:" + "c" * 64,
                                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"]}]}

    def run_cli(self, remove_exit):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.write_bytes(b"test")
            commands = []
            def fake_docker(argv):
                commands.append(argv)
                Path(argv[argv.index("--cidfile") + 1]).write_text("d" * 64)
                proc = Mock()
                proc.wait.return_value = 0
                return proc
            stderr = io.StringIO()
            with patch("sys.argv", ["reference_php.py", str(archive), "--platform", "linux/amd64"]), \
                 patch("reference_php._load_compat", return_value=self.lock), \
                 patch("reference_php.verify_archive"), \
                 patch("reference_php.subprocess.Popen", side_effect=fake_docker), \
                 patch("reference_php.subprocess.run", return_value=Mock(returncode=remove_exit)) as remove, \
                 redirect_stderr(stderr):
                result = reference_php.main()
            self.assertEqual(remove.call_args.args[0], ["docker", "rm", "-f", "d" * 64])
            self.assertEqual(commands[0][commands[0].index("--platform") + 1], "linux/amd64")
            self.assertIn("buildpack-deps@sha256:" + "c" * 64, commands[0])
            return result, stderr.getvalue()

    def test_amd64_rejects_invalid_schema_and_malformed_profile_list(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.touch()
            for kind in ("schema", "non_object", "missing_platform"):
                lock = dict(self.lock)
                if kind == "schema":
                    lock["schema_version"] = 99
                elif kind == "non_object":
                    lock["additional_reference_builds"] = [None, self.lock["additional_reference_builds"][0]]
                else:
                    lock["additional_reference_builds"] = [{"image": "missing-platform"}, self.lock["additional_reference_builds"][0]]
                with self.subTest(kind=kind), patch("reference_php._load_compat", return_value=lock):
                    with self.assertRaises((ValueError, OSError)):
                        reference_php.build_command(archive, 900, "linux/amd64")

    def test_amd64_cli_selects_profile_and_cleans_up(self):
        result, _ = self.run_cli(0)
        self.assertEqual(result, 0)

    def test_amd64_cli_reports_cleanup_failure(self):
        result, error = self.run_cli(1)
        self.assertEqual(result, 1)
        self.assertIn("failed to clean up", error)
