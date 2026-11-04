"""The compile oracle must reject incomplete, unknown and mismatched observations."""
import unittest

from verify_compile_fixtures import parse_output


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


if __name__ == '__main__':
    unittest.main()
