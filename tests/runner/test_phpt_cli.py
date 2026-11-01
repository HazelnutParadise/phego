"""PHPT inspection is bounded and reports parse errors without executing code."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
ROOT = Path(__file__).resolve().parents[2]
TOOL = ROOT / "tools/phpt.py"
FIXTURE = ROOT / "tests/runner/phpt-fixtures/pass.phpt"

class PHPTCLITests(unittest.TestCase):
    def run_tool(self, *args):
        return subprocess.run([sys.executable, str(TOOL), *args], capture_output=True, text=True, timeout=10)

    def test_help_lists_supported_inspection(self):
        result = self.run_tool("--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("inspect", result.stdout)

    def test_inspect_valid_fixture_without_executing_it(self):
        result = self.run_tool("inspect", str(FIXTURE))
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "parsed")
        self.assertEqual(report["execution"], "not_run")
        self.assertEqual(report["product_acceptance"], "not_claimed")
        self.assertEqual(set(report["sections"]), {"TEST", "FILE", "EXPECT"})
        self.assertEqual(len(report["sha256"]), 64)

    def test_invalid_fixture_explains_missing_expectation(self):
        result = self.run_tool("inspect", str(FIXTURE.with_name("bork.phpt")))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("bork.phpt", result.stderr)
        self.assertIn("EXPECT", result.stderr)
        self.assertIn("help:", result.stderr)
        self.assertIn("--EXPECT--", result.stderr)
        self.assertEqual(result.stdout, "")

    def test_symlink_and_oversized_input_are_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); link = root / "link.phpt"; link.symlink_to(FIXTURE)
            result = self.run_tool("inspect", str(link))
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")
            large = root / "large.phpt"
            with large.open("wb") as f: f.truncate(8*1024*1024+1)
            result = self.run_tool("inspect", str(large))
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("8 MiB", result.stderr)

    def test_unavailable_safe_open_flags_fail_clearly(self):
        script = "import os,sys; delattr(os,sys.argv[3]); sys.path.insert(0,sys.argv[1]); sys.argv=['phpt.py','inspect',sys.argv[2]]; import phpt; raise SystemExit(phpt.main())"
        for flag in ("O_NOFOLLOW", "O_NONBLOCK"):
            with self.subTest(flag=flag):
                result = subprocess.run([sys.executable, "-I", "-c", script, str(ROOT / "tools"), str(FIXTURE), flag], capture_output=True, text=True, timeout=10)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("safe file opening is unavailable", result.stderr)
                self.assertNotIn("Traceback", result.stderr)

    def test_fifo_never_blocks(self):
        with tempfile.TemporaryDirectory() as tmp:
            fifo=Path(tmp)/"input.phpt"; os.mkfifo(fifo)
            result=self.run_tool("inspect", str(fifo))
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")

if __name__ == "__main__": unittest.main()
