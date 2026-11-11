"""REQ-017; T-034: Docker progress must not be mistaken for PHP diagnostics."""
import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("w12_reference", Path(__file__).with_name("reference.py"))
REFERENCE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REFERENCE)


class ReferenceRunnerTest(unittest.TestCase):
    def test_uncached_image_pull_has_its_own_stderr(self):
        with tempfile.TemporaryDirectory() as directory:
            responses = [subprocess.CompletedProcess([], 1, b"", b"missing image"),
                         subprocess.CompletedProcess([], 0, b"digest verified", b"pull progress"),
                         subprocess.CompletedProcess([], 0, json.dumps({"php": "8.5.12", "int_size": 8, "cases": []}).encode(), b"")]
            with patch.object(sys, "argv", ["reference.py", "--width", "64", "--output-dir", directory]), \
                 patch.object(REFERENCE.subprocess, "run", side_effect=responses) as run, \
                 contextlib.redirect_stdout(io.StringIO()):
                REFERENCE.main()
            self.assertEqual(run.call_args_list[1].args[0][1], "pull")
            self.assertEqual(run.call_args_list[2].args[0][1:4], ["run", "--pull", "never"])
            self.assertEqual(json.loads((Path(directory) / "w12-run-64.json").read_text())["stderr"], "")
            sources = json.loads((Path(directory) / "w12-run-64.json").read_text())["fixture_sources_sha256"]
            self.assertEqual(set(sources), {"matrix.php", "sorting.php"})
            self.assertEqual(sources["sorting.php"], hashlib.sha256(Path(__file__).with_name("sorting.php").read_bytes()).hexdigest())

    def test_php_stderr_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            responses = [subprocess.CompletedProcess([], 0, b"image", b""),
                         subprocess.CompletedProcess([], 0, b'{"php":"8.5.12","int_size":8}', b"PHP failure")]
            with patch.object(sys, "argv", ["reference.py", "--width", "64", "--output-dir", directory]), \
                 patch.object(REFERENCE.subprocess, "run", side_effect=responses):
                with self.assertRaisesRegex(RuntimeError, "Unexpected PHP stderr"):
                    REFERENCE.main()
            self.assertFalse((Path(directory) / "w12-ref-64.json").exists())
