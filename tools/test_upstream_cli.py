import contextlib
import io
import tempfile
import unittest
from pathlib import Path

from upstream_snapshot import main


class TestUpstreamCLI(unittest.TestCase):
    def test_duplicate_json_key_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            p = Path(directory) / "snapshot.json"
            p.write_text('{"schema_version":1,"schema_version":1}')
            out, err = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                result = main(["compare", str(p), str(p)])
            self.assertEqual(result, 1)
            self.assertEqual(out.getvalue(), "")
            self.assertIn("duplicate JSON key", err.getvalue())

    def test_missing_archive_fails_without_output(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = str(Path(directory) / "missing.tar.xz")
            out, err = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                result = main(["capture", "--archive", archive, "--sha256", "a" * 64,
                               "--version", "8.5.12", "--tag", "php-8.5.12",
                               "--commit", "b" * 40, "--announcement",
                               "https://www.php.net/releases/8_5_12.php"])
            self.assertEqual(result, 1)
            self.assertEqual(out.getvalue(), "")
            self.assertTrue(err.getvalue().startswith("Error:"))


if __name__ == "__main__":
    unittest.main()
