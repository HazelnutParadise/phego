"""Reference checks for PHP 8.5.12 PHPT EXPECTREGEX / EXPECTF."""
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


session = load_tool("phego_phpt_pcre_session", TOOLS / "phpt_session.py")
pcre = load_tool("phego_phpt_pcre_matcher", TOOLS / "phpt_pcre.py")
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


def observed(output, *, status="complete", code=0, cleanup=True, effects=True, reason=""):
    phase = session.PhaseObservation(status, output, code, reason)
    return session.CaseObservation(
        status, None, None, phase, effects, cleanup, reason,
    )


class PHPTPCREContractTests(unittest.TestCase):
    def match(self, expected, actual, *, formatted=False, release=RELEASE, platform=PLATFORM,
              timeout=10):
        return pcre.match_reference(expected, actual, formatted, IMAGE, EXECUTABLE,
                                    platform, release, timeout)

    def test_rejects_non_bytes_and_non_boolean_formatted_before_starting_reference(self):
        invalid = (
            ("pattern", b"text", False),
            (b"pattern", "text", False),
            (b"pattern", b"text", 1),
        )
        for expected, actual, formatted in invalid:
            with self.subTest(expected_type=type(expected).__name__, actual_type=type(actual).__name__), \
                    mock.patch.object(pcre, "run_case", create=True) as run_case:
                matched, diagnostic = self.match(expected, actual, formatted=formatted)
                self.assertIsNone(matched)
                self.assertIsInstance(diagnostic, str)
                run_case.assert_not_called()

    def test_each_input_over_one_mib_is_a_gap_without_echoing_bytes(self):
        secret = b"PCRE_PRIVATE_PAYLOAD_8d3c"
        for expected, actual in ((b"x" * (1024 * 1024 + 1), b""),
                                 (b"", secret + b"x" * (1024 * 1024))):
            with self.subTest(expected_length=len(expected), actual_length=len(actual)), \
                    mock.patch.object(pcre, "run_case", create=True) as run_case:
                matched, diagnostic = self.match(expected, actual)
                self.assertIsNone(matched)
                self.assertNotIn(secret.decode(), diagnostic)
                self.assertNotIn("x" * 64, diagnostic)
                run_case.assert_not_called()

    def test_unknown_release_is_gap_without_starting_reference(self):
        with mock.patch.object(pcre, "run_case", create=True) as run_case:
            matched, diagnostic = self.match(b"x", b"x", release="8.4.0")
        self.assertIsNone(matched)
        self.assertNotIn("8.4.0", diagnostic)
        run_case.assert_not_called()

    def test_incomplete_observation_is_gap_and_does_not_disclose_output(self):
        secret = b"OBSERVED_PRIVATE_DATA_241a"
        incomplete = (
            observed(secret, status="timeout", code=None, reason="execution timeout"),
            observed(secret, cleanup=False, reason="cleanup not confirmed"),
            session.CaseObservation("runner_gap", None, None, None, False, True,
                                    "observation unavailable"),
        )
        for observation in incomplete:
            with self.subTest(status=observation.status, cleanup=observation.cleanup_confirmed), \
                    mock.patch.object(pcre, "run_case", create=True, return_value=observation) as run_case:
                matched, diagnostic = self.match(b"secret", secret)
                self.assertIsNone(matched)
                self.assertNotIn(secret.decode(), diagnostic)
                self.assertIsInstance(diagnostic, str)
                run_case.assert_called_once()

    def test_invalid_pcre_is_gap_and_diagnostic_does_not_echo_pattern_or_output(self):
        pattern = b"PCRE_PATTERN_SECRET_["
        output = b"PCRE_OUTPUT_SECRET_9b7"
        with mock.patch.object(pcre, "run_case", create=True, return_value=observed(output)) as run_case:
            matched, diagnostic = self.match(pattern, output)
        self.assertIsNone(matched)
        self.assertNotIn(pattern.decode(), diagnostic)
        self.assertNotIn(output.decode(), diagnostic)
        run_case.assert_called_once()

    def test_timeout_is_gap_without_leaking_timeout_output(self):
        secret = b"TIMEOUT_PRIVATE_OUTPUT_891"
        error = subprocess.TimeoutExpired("reference", 0.01, output=secret, stderr=secret)
        with mock.patch.object(pcre, "run_case", create=True, side_effect=error) as run_case:
            matched, diagnostic = self.match(b"x", secret, timeout=0.01)
        self.assertIsNone(matched)
        self.assertNotIn(secret.decode(), diagnostic)
        run_case.assert_called_once()

    def test_exception_types_become_safe_gap_diagnostics(self):
        secret = b"EXCEPTION_PRIVATE_BYTES"
        cases = (TypeError(secret.decode()), ValueError(secret.decode()), OSError(secret.decode()))
        for error in cases:
            with self.subTest(error_type=type(error).__name__), \
                    mock.patch.object(pcre, "run_case", create=True, side_effect=error) as run_case:
                matched, diagnostic = self.match(b"x", b"x")
                self.assertIsNone(matched)
                self.assertNotIn(secret.decode(), diagnostic)
                run_case.assert_called_once()


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTPCREReferenceTests(unittest.TestCase):
    def match(self, expected, actual, *, formatted=False):
        return pcre.match_reference(expected, actual, formatted, IMAGE, EXECUTABLE,
                                    PLATFORM, RELEASE, 10)

    def assertMatch(self, expected, actual, *, formatted=False):
        matched, diagnostic = self.match(expected, actual, formatted=formatted)
        self.assertIs(matched, True, diagnostic)

    def assertMismatch(self, expected, actual, *, formatted=False):
        matched, diagnostic = self.match(expected, actual, formatted=formatted)
        self.assertIs(matched, False, diagnostic)

    def test_expectregex_alternation_is_unanchored(self):
        self.assertMatch(b"a|b", b"ac")
        self.assertMismatch(b"a|b", b"z")

    def test_invalid_regex_delimiter_and_character_class_are_reported_as_gaps(self):
        for pattern in (b"/", b"["):
            with self.subTest(pattern=pattern):
                matched, diagnostic = self.match(pattern, b"anything")
                self.assertIsNone(matched, diagnostic)
                self.assertNotIn(pattern.decode(), diagnostic)

    def test_expectregex_preserves_binary_nul_newline_and_case_sensitivity(self):
        self.assertMatch(b"A\\x00B", b"A\x00B")
        self.assertMatch(b"first\\nsecond", b"first\nsecond")
        self.assertMismatch(b"abc", b"ABC")

    def test_expectf_raw_regex_pair_expands_percent_d(self):
        self.assertMatch(b"id=%r%d%r;", b"id=123;", formatted=True)

    def test_unpaired_expectf_raw_marker_is_literal_text(self):
        self.assertMatch(b"value=%r", b"value=%r", formatted=True)

    def test_expectf_linux_path_separator_is_slash(self):
        self.assertMatch(b"dir%efile", b"dir/file", formatted=True)


if __name__ == "__main__":
    unittest.main()
