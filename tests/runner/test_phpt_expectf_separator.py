"""EXPECTF %e separator handling for the verified PHP 8.5.12 Linux profile."""
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))


def load_tool(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


results = load_tool("phego_expectf_separator_results", TOOLS / "phpt_results.py")
executor = load_tool("phego_expectf_separator_execute", TOOLS / "phpt_execute.py")
IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
EXECUTABLE = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def sections(source, expected):
    return {"TEST": b"EXPECTF separator profile", "FILE": source, "EXPECTF": expected}


class PHPTExpectFSeparatorUnitTests(unittest.TestCase):
    def test_e_matches_linux_separator(self):
        self.assertIs(results.match_format(b"a%eb", b"a/b", path_separator=b"/"), True)

    def test_e_does_not_match_a_backslash_on_linux(self):
        self.assertIs(results.match_format(b"a%eb", b"a\\b", path_separator=b"/"), False)

    def test_s_and_repeated_e_tokens_are_supported(self):
        self.assertIs(results.match_format(b"%s%e%s%e%s", b"one/two/three", path_separator=b"/"), True)

    def test_percent_before_e_token_remains_literal(self):
        self.assertIs(results.match_format(b"%%e", b"%/", path_separator=b"/"), True)
        self.assertIs(results.match_format(b"%%e", b"%e", path_separator=b"/"), False)

    def test_unverified_separator_stays_unknown(self):
        self.assertIsNone(results.match_format(b'a%eb', b'a\\b', path_separator=b'\\'))

    def test_raw_pattern_remains_reported_as_unsupported(self):
        result = results.classify({'EXPECTF': b'%r.*%r%e'}, results.Observation('completed', b'/ ', 0))
        self.assertEqual(result['status'], 'runner_gap')
        self.assertIn('unsupported', result['reason'])

    def test_missing_separator_profile_remains_unknown(self):
        self.assertIsNone(results.match_format(b"a%eb", b"a/b"))


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTExpectFSeparatorReferenceTests(unittest.TestCase):
    def run_phpt(self, source, expected, *, release=RELEASE, platform=PLATFORM):
        return executor.run(sections(source, expected), IMAGE, EXECUTABLE, platform,
                            release, 15, 8192)

    def test_reference_linux_e_matches_slash(self):
        result = self.run_phpt(b"<?php echo 'a/b'; ?>", b"a%eb")
        self.assertEqual(result["status"], "pass", result)

    def test_reference_linux_e_does_not_match_backslash(self):
        result = self.run_phpt(b"<?php echo 'a\\\\b'; ?>", b"a%eb")
        self.assertEqual(result["status"], "fail", result)

    def test_reference_supports_s_repeated_e_and_literal_percent_e(self):
        cases = (
            (b"<?php echo 'one/two/three'; ?>", b"%s%e%s%e%s", "pass"),
            (b"<?php echo '%/'; ?>", b"%%e", "pass"),
            (b"<?php echo '%e'; ?>", b"%%e", "fail"),
        )
        for source, expected, status in cases:
            with self.subTest(expected=expected):
                result = self.run_phpt(source, expected)
                self.assertEqual(result["status"], status, result)

    def test_matching_unverified_release_still_does_not_guess_separator(self):
        with mock.patch.object(executor, 'execute_php', side_effect=[
            results.Observation('completed', b'8.5.11\n4\n', 0),
            results.Observation('completed', b'a/b', 0),
        ]):
            result = self.run_phpt(b"<?php echo 'a/b'; ?>", b'a%eb', release='8.5.11')
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertIn('%e', result['reason'])
        self.assertIn('profile', result['reason'])

    def test_unavailable_profile_and_unverified_release_remain_gaps(self):
        with mock.patch.object(executor, "execute_php") as execute_php:
            no_platform = self.run_phpt(b"<?php echo 'a/b'; ?>", b"a%eb", platform=None)
            self.assertEqual(no_platform["status"], "runner_gap", no_platform)
            execute_php.assert_not_called()

            execute_php.side_effect = [
                results.Observation("completed", b"8.5.12\n4\n", 0),
            ]
            wrong_release = self.run_phpt(b"<?php echo 'a/b'; ?>", b"a%eb", release="8.5.11")
        self.assertEqual(wrong_release["status"], "runner_gap", wrong_release)
        self.assertEqual(wrong_release["execution"], "not_run")
