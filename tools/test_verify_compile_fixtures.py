"""The compile oracle must reject incomplete, unknown and mismatched observations."""
import contextlib
import gzip
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

from verify_compile_fixtures import collect, main, parse_output


class CompileFixtureTests(unittest.TestCase):
    def test_preserves_warnings_and_fatal_order(self):
        raw = ('PROFILE 8.5.12 8 Linux\n\nWarning: first in /w/corpus/00000.php on line 2\n'
               'Fatal error: bad in /w/corpus/00000.php on line 3\nStack trace:\n#0 {main}\nErrors parsing /w/corpus/00000.php\n'
               'No syntax errors detected in /w/corpus/00001.php\n')
        self.assertEqual(parse_output(raw, 64, 2), [[
            {"level": "Warning", "message": "first", "line": 2},
            {"level": "Fatal error", "message": "bad", "line": 3}], []])

    def test_missing_duplicate_and_foreign_results_fail(self):
        for body in ['', 'No syntax errors detected in /w/corpus/00000.php\n' * 2,
                     'No syntax errors detected in /w/corpus/99999.php\n',
                     'Segmentation fault\n', 'Errors parsing /w/corpus/00000.php\n']:
            with self.subTest(body=body), self.assertRaises(ValueError):
                parse_output('PROFILE 8.5.12 4 Linux\n' + body, 32, 1)

    def test_profile_mismatch_fails(self):
        for profile in ['8.5.11 4 Linux', '8.5.12 8 Linux', '8.5.12 4 Darwin']:
            with self.subTest(profile=profile), self.assertRaises(ValueError):
                parse_output('PROFILE ' + profile + '\nNo syntax errors detected in /w/corpus/00000.php\n', 32, 1)

    def test_multiline_message_is_preserved(self):
        raw = ('PROFILE 8.5.12 4 Linux\nFatal error: first\nsecond in /w/corpus/00000.php on line 7\n'
               'Errors parsing /w/corpus/00000.php\n')
        self.assertEqual(parse_output(raw, 32, 1)[0][0]['message'], 'first\nsecond')

    def test_private_workspace_remains_readable_without_root_capabilities(self):
        def docker(command, **kwargs):
            if command[1] == 'run':
                root = Path(command[command.index('-v') + 1].removesuffix(':/w:ro'))
                self.assertEqual(root.stat().st_mode & 0o777, 0o700)
                self.assertIn('--cap-drop=ALL', command)
                self.assertIn('--read-only', command)
                user = command[command.index('--user') + 1] if '--user' in command else '0:0'
                # Linux root without DAC_OVERRIDE cannot traverse another
                # owner's 0700 bind mount. Model a non-root CI runner.
                if user != '1001:1002':
                    raise subprocess.CalledProcessError(1, command, stderr=b'Could not open input file: /w/run.php')
                self.assertEqual((root / 'corpus/00000.php').read_text(), '<?php echo 1;')
                return subprocess.CompletedProcess(command, 0, b'PROFILE 8.5.12 4 Linux\nNo syntax errors detected in /w/corpus/00000.php\n', b'')
            return subprocess.CompletedProcess(command, 0, b'', b'')

        with mock.patch('os.getuid', return_value=1001), mock.patch('os.getgid', return_value=1002), \
                mock.patch('verify_compile_fixtures.subprocess.run', side_effect=docker):
            self.assertEqual(collect([{'source': '<?php echo 1;'}], 32, 30), [[]])

    def test_failed_runner_preserves_bounded_diagnostics_and_cleanup(self):
        commands = []

        def docker(command, **kwargs):
            commands.append(command)
            if command[1] == 'run':
                raise subprocess.CalledProcessError(1, command, output=b'OUT\xff' + b'x' * 5000,
                                                    stderr=b'Could not open input file: /w/run.php\n' + b'y' * 5000)
            return subprocess.CompletedProcess(command, 0, b'', b'')

        with tempfile.TemporaryDirectory() as directory:
            fixture = Path(directory) / 'fixture.json.gz'
            fixture.write_bytes(gzip.compress(json.dumps([{'source': '<?php echo 1;', 'width': 32, 'events': []}]).encode()))
            errors = io.StringIO()
            with mock.patch('verify_compile_fixtures.subprocess.run', side_effect=docker), contextlib.redirect_stderr(errors):
                self.assertEqual(main(['--fixture', str(fixture), '--width', '32']), 1)
        text = errors.getvalue()
        self.assertIn('OUT\ufffd', text)
        self.assertIn('Could not open input file: /w/run.php', text)
        self.assertEqual(text.count('[truncated]'), 2)
        self.assertLess(len(text), 9500)
        self.assertEqual(commands[-1][:3], ['docker', 'rm', '-f'])


if __name__ == '__main__':
    unittest.main()
