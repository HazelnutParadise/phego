"""PHPT parsing preserves fixtures and refuses ambiguous tests."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("phego_phpt_runner", ROOT / "tools/phpt_runner.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)

class PHPTParsingTests(unittest.TestCase):
    def test_existing_pass_fixture_retains_test_and_expectation(self):
        sections = runner.parse_phpt((ROOT / "tests/runner/phpt-fixtures/pass.phpt").read_bytes(), "pass.phpt")
        self.assertEqual(sections["TEST"], b"Known passing result\n")
        self.assertEqual(sections["EXPECT"], b"phego fixture\n")
        self.assertIn(b"<?php", sections["FILE"])

    def test_missing_expectation_is_reported_with_path(self):
        with self.assertRaisesRegex(runner.PHPTError, r"bork.phpt.*EXPECT"):
            runner.parse_phpt((ROOT / "tests/runner/phpt-fixtures/bork.phpt").read_bytes(), "bork.phpt")

    def test_duplicate_section_reports_location(self):
        data = b"--TEST--\nname\n--FILE--\n<?php ?>\n--EXPECT--\nx\n--EXPECT--\ny\n"
        with self.assertRaisesRegex(runner.PHPTError, r"sample.phpt:7.*EXPECT"):
            runner.parse_phpt(data, "sample.phpt")

    def test_section_content_is_byte_preserving_with_crlf_and_invalid_utf8(self):
        data = b"--TEST--\r\nname\r\n--FILE--\r\n<?php ?>\r\n--EXPECT--\r\n\xff\r\n"
        self.assertEqual(runner.parse_phpt(data, "bytes.phpt")["EXPECT"], b"\xff\r\n")

    def test_fileeof_can_be_followed_by_expectation(self):
        # PHP 8.5.12 run-tests.php parses section markers after FILEEOF.
        data = b"--TEST--\nname\n--FILEEOF--\n<?php echo 1; ?>\n--EXPECT--\n1\n"
        sections = runner.parse_phpt(data, "eof.phpt")
        self.assertEqual(sections["FILEEOF"], b"<?php echo 1; ?>\n")
        self.assertEqual(sections["EXPECT"], b"1\n")

    def test_external_formatted_expectation_is_recognized(self):
        data = b"--TEST--\nname\n--FILE--\n<?php ?>\n--EXPECTF_EXTERNAL--\nexpected.txt\n"
        self.assertIn("EXPECTF_EXTERNAL", runner.parse_phpt(data, "external.phpt"))

    def test_first_section_must_be_test(self):
        data = b"--FILE--\n<?php ?>\n--TEST--\nname\n--EXPECT--\n\n"
        with self.assertRaisesRegex(runner.PHPTError, r"first.*TEST"):
            runner.parse_phpt(data, "order.phpt")

    def test_phpdbg_and_redirect_allow_no_file_section(self):
        for data in (b"--TEST--\nname\n--PHPDBG--\nrun\n--EXPECT--\noutput\n",
                     b"--TEST--\nname\n--REDIRECTTEST--\n<?php ?>\n"):
            with self.subTest(data=data):
                self.assertIn("TEST", runner.parse_phpt(data, "special.phpt"))

    def test_conflicting_expectations_and_leading_data_rejected(self):
        samples = [b"prefix\n--TEST--\nx\n--FILE--\ny\n--EXPECT--\nz\n", b"--TEST--\nx\n--FILE--\ny\n--EXPECT--\nz\n--EXPECTF--\nz\n"]
        for sample in samples:
            with self.subTest(sample=sample), self.assertRaises(runner.PHPTError):
                runner.parse_phpt(sample, "bad.phpt")

if __name__ == "__main__": unittest.main()
