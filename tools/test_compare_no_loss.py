"""Regression checks for lost unsupported and skipped results."""
import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from compare_evidence import compare


class NoLossTests(unittest.TestCase):
    def test_previous_results_are_not_silently_removed_or_reclassified(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            (root / "docs/data").mkdir(parents=True)
            (root / "docs/data/phego-acceptance-tests.json").write_text(json.dumps({"items": [{"id": "T-001"}]}))
            (root / "log").write_bytes(b"synthetic\n")
            base = {"test_id": "T-001", "commit": "a" * 40, "command": "test", "profile":
                    {"release": "8.5.12", "platform": "linux/arm64", "build": "cli"},
                    "exit_code": None, "evidence": {"path": "log", "sha256": hashlib.sha256(b"synthetic\n").hexdigest()}}
            previous, current = root / "old.json", root / "new.json"
            for old, new in (("unsupported", "skip"), ("unsupported", "environment_skip"),
                             ("unsupported", "runner_error"), ("fail", "runner_error"), ("skip", "not_run")):
                with self.subTest(old=old, new=new):
                    prior = dict(base, status=old, exit_code=1 if old == "fail" else None)
                    previous.write_text(json.dumps({"records": [prior]}))
                    current.write_text(json.dumps({"records": [] if new == "not_run" else [dict(base, status=new)]}))
                    with self.assertRaises(ValueError):
                        compare(root, previous, current)
            previous.write_text(json.dumps({"records": [dict(base, status="pass", exit_code=0)]}))
            current.write_text(json.dumps({"records": [dict(base, status="runner_error")]}))
            result = compare(root, previous, current)
            self.assertEqual(result["changes"]["T-001"], {"previous": "pass", "current": "runner_error"})


if __name__ == "__main__":
    unittest.main()
