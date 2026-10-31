"""End-to-end PHPT PCRE integration against the pinned reference image."""
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import unittest
from unittest import mock

TOOLS = Path(__file__).resolve().parents[2] / "tools"
sys.path.insert(0, str(TOOLS))
import phpt_execute as executor

IMAGE = os.environ.get("PHEGO_PHPT_SESSION_IMAGE", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049")
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
AVAILABLE = bool(re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE) and shutil.which("docker") and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False).returncode == 0)

class PHPTPCREClassificationTests(unittest.TestCase):
    def classify(self, sections, output=b"body", **options):
        return executor._classify_reference(sections, executor.Observation("completed", output, 0),
                    IMAGE, "/usr/local/bin/php", PLATFORM, options.pop("release", "8.5.12"), 10, **options)

    def test_normalization_and_expected_failure_or_header_mismatch(self):
        with mock.patch.object(executor, "match_reference", return_value=(True, "")) as matcher:
            result = self.classify({"EXPECTREGEX": b" x\r\ny ", "XFAIL": b"known"},
                                   b" x\r\ny ", headers_match=False)
        self.assertEqual(result["status"], "xfail")
        self.assertEqual(matcher.call_args.args[:3], (b"x\ny", b"x\ny", False))
        with mock.patch.object(executor, "match_reference", return_value=(True, "")):
            result = self.classify({"EXPECTF": b"%r.*%r", "XFAIL": b"known"})
        self.assertEqual(result["status"], "xpass")

    def test_failures_and_unverified_version_cannot_pass(self):
        sections = {"EXPECTREGEX": b"secret"}
        for outcome in ((None, "PHPT PCRE invalid pattern"), (1, "")):
            with self.subTest(outcome=outcome), mock.patch.object(executor, "match_reference", return_value=outcome):
                self.assertEqual(self.classify(sections)["status"], "runner_gap")
        with mock.patch.object(executor, "match_reference", side_effect=ValueError("secret")):
            result = self.classify(sections)
            self.assertEqual(result["status"], "runner_gap")
            self.assertNotIn("secret", result["reason"])
        with mock.patch.object(executor, "match_reference") as matcher:
            result = self.classify(sections, release="8.5.11")
            self.assertEqual(result["status"], "runner_gap")
            matcher.assert_not_called()

@unittest.skipUnless(AVAILABLE, "pinned PHP 8.5.12 image and Docker required")
class PHPTPCREExecutionTests(unittest.TestCase):
    def run_sections(self, sections, fixtures=None):
        return executor.run(sections, IMAGE, "/usr/local/bin/php", PLATFORM, "8.5.12", 15, 8192, fixtures=fixtures)

    def test_unanchored_alternation_and_external_expectation(self):
        source = b"<?php echo 'ac'; ?>"
        for section, fixtures in (({"EXPECTREGEX": b"a|b"}, None),
                                  ({"EXPECTREGEX_EXTERNAL": b"expected.txt"}, {"expected.txt": b"a|b"})):
            with self.subTest(section=section):
                result = self.run_sections({"TEST": b"regex", "FILE": source, **section}, fixtures)
                self.assertEqual(result["status"], "pass", result)
                self.assertEqual(result["product_acceptance"], "not_claimed")

    def test_raw_alternatives_are_grouped_and_float_token_is_exact(self):
        cases = ((b"%rfoo|bar%rEND", b"<?php echo 'fooZ'; ?>"),
                 (b"%r%r%f", b"<?php echo '2.'; ?>"))
        for pattern, source in cases:
            with self.subTest(pattern=pattern):
                result = self.run_sections({"TEST": b"regex", "FILE": source, "EXPECTF": pattern})
                self.assertEqual(result["status"], "fail", result)

    def test_raw_expectf_and_invalid_regex(self):
        for expectation, status in (({"EXPECTF": b"a%r%d%r"}, "pass"),
                                    ({"EXPECTREGEX": b"[private"}, "runner_gap")):
            with self.subTest(expectation=expectation):
                result = self.run_sections({"TEST": b"regex", "FILE": b"<?php echo 'a42'; ?>", **expectation})
                self.assertEqual(result["status"], status, result)
                self.assertNotIn("private", result["reason"])

if __name__ == "__main__":
    unittest.main()
