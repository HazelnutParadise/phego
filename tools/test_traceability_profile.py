"""Profile and revision isolation for traceability records."""
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from traceability import report


class ProfileTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root / "docs/data").mkdir(parents=True)
        (self.root / "docs/data/phego-acceptance-tests.json").write_text(
            json.dumps({"items": [{"id": "T-001"}, {"id": "T-002"}]}))
        (self.root / "result.log").write_bytes(b"observed result\n")
        self.path = self.root / "records.json"

    def record(self, test_id, status):
        return {"test_id": test_id, "status": status, "commit": "a" * 40,
                "command": "test", "profile": {"release": "8.5.12",
                "platform": "linux/amd64", "build": "default"}, "exit_code": 1,
                "evidence": {"path": "result.log", "sha256": hashlib.sha256(
                    b"observed result\n").hexdigest()}}

    def check(self, records):
        self.path.write_text(json.dumps({"records": records}))
        return report(self.root, self.path)

    def test_mixed_profiles_rejected(self):
        a, b = self.record("T-001", "fail"), self.record("T-002", "fail")
        b["profile"]["platform"] = "linux/arm64"
        with self.assertRaises(ValueError):
            self.check([a, b])

    def test_mixed_commits_rejected(self):
        a, b = self.record("T-001", "fail"), self.record("T-002", "fail")
        b["commit"] = "b" * 40
        with self.assertRaises(ValueError):
            self.check([a, b])

    def test_report_preserves_provenance_and_evidence(self):
        record = self.record("T-001", "fail")
        result = self.check([record])
        self.assertEqual(result["execution_records"], [record])
        self.assertEqual(result["profile"], record["profile"])
        self.assertEqual(result["commit"], record["commit"])

    def test_excessively_nested_json_reports_invalid_input(self):
        self.path.write_text("[" * 1100 + "0" + "]" * 1100)
        with self.assertRaises(ValueError):
            report(self.root, self.path)
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("traceability.py")),
                                 "--root", str(self.root), "--results", str(self.path)],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("Traceback", result.stderr)

    def test_duplicate_json_keys_are_rejected(self):
        self.path.write_text('{"records": [], "records": []}')
        with self.assertRaises(ValueError):
            report(self.root, self.path)

    def test_timeout_can_record_absent_exit_code(self):
        record = self.record("T-001", "timeout")
        record["exit_code"] = None
        self.assertEqual(self.check([record])["results"]["T-001"], "timeout")

    def test_crash_and_timeout_are_separate_from_fail(self):
        result = self.check([self.record("T-001", "crash"),
                             self.record("T-002", "timeout")])
        self.assertEqual(result["counts"]["crash"], 1)
        self.assertEqual(result["counts"]["timeout"], 1)
        self.assertEqual(result["counts"]["pass"], 0)


if __name__ == "__main__":
    unittest.main()
