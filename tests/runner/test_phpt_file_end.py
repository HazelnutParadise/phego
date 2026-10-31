"""Regression tests for PHP 8.5.12 FILE/FILEEOF end markers."""
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
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


parser = load_tool("phego_phpt_file_end_parser", TOOLS / "phpt_runner.py")
executor = load_tool("phego_phpt_file_end_executor", TOOLS / "phpt_execute.py")
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


def run_phpt(data, *, release=RELEASE):
    sections = parser.parse_phpt(data, "file-end.phpt")
    return executor.run(
        sections, IMAGE, EXECUTABLE, PLATFORM, release, 15, 8192,
    )


class PHPTFileEndInputTests(unittest.TestCase):
    def test_unsupported_file_sections_are_named_before_execution(self):
        for name in ('FILE_EXTERNAL', 'REDIRECTTEST'):
            with self.subTest(name=name), mock.patch.object(executor, 'run_case') as run_case:
                result = executor.run({'TEST': b'x', name: b'content', 'EXPECT': b'x'},
                                      IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 8192)
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertIn(name, result['reason'])
                run_case.assert_not_called()

    def test_oversized_file_is_rejected_before_marker_truncation(self):
        sections = {'TEST': b'x', 'FILE': b'===DONE===\n' + b'x' * (1024 * 1024), 'EXPECT': b'x'}
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 8192)
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertIn('limit', result['reason'])
        run_case.assert_not_called()

    def test_section_marker_prefix_is_recognized_and_duplicate_rejected(self):
        data = (b'--TEST-- trailing text\nname\n--FILE-- suffix\n<?php ?>\n'
                b'--INI-- suffix\nprecision=17\n--EXPECT-- trailing\nx\n')
        parsed = parser.parse_phpt(data, 'prefix.phpt')
        self.assertEqual(parsed['FILE'], b'<?php ?>\n')
        self.assertEqual(parsed['INI'], b'precision=17\n')
        duplicate = data + b'--EXPECT-- suffix\ny\n'
        with self.assertRaises(parser.PHPTError):
            parser.parse_phpt(duplicate, 'duplicate.phpt')

    def test_unknown_markers_do_not_become_false_passes(self):
        for marker in (b'--UNKNOWN-- suffix', b'--_UNKNOWN-- suffix', b'--UNKNOWN2--'):
            data = (b'--TEST--\nx\n--FILE--\n<?php ?>\n===DONE===\n' + marker +
                    b'\nignored\n--EXPECT--\n===DONE===\n')
            with self.subTest(marker=marker):
                result = run_phpt(data)
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertEqual(result['execution'], 'not_run')

    def test_binary_suffix_after_section_marker_is_not_file_content(self):
        data = (b'--TEST--\nx\n--FILE--\n<?php ?>\n--INI--\x00\xff\n'
                b'precision=17\n--EXPECT--\nx\n')
        parsed = parser.parse_phpt(data, 'binary.phpt')
        self.assertEqual(parsed['FILE'], b'<?php ?>\n')
        self.assertEqual(parsed['INI'], b'precision=17\n')

    def test_conflicting_file_and_fileeof_are_rejected_before_execution(self):
        sections = {'TEST': b'x', 'FILE': b'one', 'FILEEOF': b'two', 'EXPECT': b'two'}
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 8192)
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertEqual(result['execution'], 'not_run')
        run_case.assert_not_called()

    def test_lone_cr_does_not_make_a_new_marker_line(self):
        source = b"<?php echo 'a'; ?>\r===DONE===\ntail\n"
        sections = {'TEST': b'x', 'FILE': source, 'EXPECT': b'irrelevant'}
        with mock.patch.object(executor, 'run_case', side_effect=RuntimeError('capture')) as run_case:
            executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 8192)
        self.assertEqual(run_case.call_args.kwargs['source'], source)
        self.assertEqual(sections['FILE'], source)

    def test_fileeof_trims_after_done_truncation(self):
        source = b"<?php echo 'a'; ?>\r\n===DONE===\r\nignored\r\n"
        sections = {'TEST': b'x', 'FILEEOF': source, 'EXPECT': b'irrelevant'}
        with mock.patch.object(executor, 'run_case', side_effect=RuntimeError('capture')) as run_case:
            executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 8192)
        self.assertEqual(run_case.call_args.kwargs['source'], b"<?php echo 'a'; ?>\r\n===DONE===")
        self.assertEqual(sections['FILEEOF'], source)

    def test_unconfirmed_release_keeps_file_eof_as_runner_gap(self):
        data = (b"--TEST--\nFILEEOF requires a confirmed release\n"
                b"--FILEEOF--\n<?php echo 'must not run'; ?>\n"
                b"--EXPECT--\nmust not run\n")
        result = run_phpt(data, release="8.5.11")
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertEqual(result["execution"], "not_run")


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTFileEndExecutionTests(unittest.TestCase):
    def assert_phpt_passes(self, data):
        result = run_phpt(data)
        self.assertEqual(result["status"], "pass", result)

    def test_done_marker_does_not_hide_following_suffixed_ini_section(self):
        data = (b'--TEST--\nsection marker suffix\n--FILE--\n'
                b'<?php echo ini_get("precision"), "\\n"; ?>\n===DONE===\n'
                b'--INI-- suffix\nprecision=17\n--EXPECT--\n14\n===DONE===\n')
        result = run_phpt(data)
        self.assertEqual(result['status'], 'fail', result)

    def test_lf_done_marker_keeps_marker_and_ignores_tail_until_next_section(self):
        # PHP's official runner retains the marker and ignores the tail.
        data = (b"--TEST--\nFILE marker tail\n--FILE--\n"
                b'<?php echo "before\\n"; ?>\n'
                b"===DONE===\nwrong tail\n"
                b"--EXPECT--\nbefore\n===DONE===\n")
        self.assert_phpt_passes(data)

    def test_crlf_marker_with_trailing_horizontal_whitespace_stops_file_tail(self):
        data = (b"--TEST--\nCRLF marker line\n--FILE--\r\n"
                b'<?php echo "before\\n"; ?>\r\n'
                b"===DONE=== \t\r\nignored tail\r\n"
                b"--EXPECT--\r\nbefore\r\n===DONE===\r\n")
        self.assert_phpt_passes(data)

    def test_similar_marker_text_does_not_stop_file_content(self):
        data = (b"--TEST--\nNon-matching marker text remains FILE output\n--FILE--\n"
                b'<?php echo "before\\n"; ?>\n'
                b"===DONE===suffix\nordinary tail\n"
                b"--EXPECT--\nbefore\n===DONE===suffix\nordinary tail\n")
        self.assert_phpt_passes(data)

    def test_file_eof_converts_and_trims_terminal_crlf_before_execution(self):
        data = (b"--TEST--\nFILEEOF conversion\n--FILEEOF--\n"
                b"<?php echo 'from-file-eof'; ?>\r\n\r\n"
                b"--EXPECT--\nfrom-file-eof\n")
        self.assert_phpt_passes(data)

    def test_file_eof_also_obeys_done_marker_and_preserves_marker_line(self):
        data = (b"--TEST--\nFILEEOF marker line\n--FILEEOF--\n"
                b'<?php echo "from-file-eof\\n"; ?>\r\n'
                b"===DONE===\r\nignored tail\r\n"
                b"--EXPECT--\nfrom-file-eof\n===DONE===\n")
        self.assert_phpt_passes(data)


if __name__ == "__main__":
    unittest.main()
