#!/usr/bin/env python3
"""Every upstream security fix in a committed diff report has an assessment."""
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
STATUSES = {"affected", "not_affected", "needs_regression", "unknown"}


class TestSecurityAssessments(unittest.TestCase):
    def load(self):
        data = json.loads((ROOT / "engineering/compatibility/security-assessments.json").read_text())
        self.assertEqual(data["schema_version"], 1)
        return data["assessments"]

    def test_records_are_complete(self):
        for record in self.load():
            with self.subTest(ids=record.get("ids")):
                self.assertIn(record["status"], STATUSES)
                self.assertTrue(record["ids"])
                self.assertTrue(record["work_packages"])
                self.assertGreater(len(record["evidence"].strip()), 40, "evidence must explain the judgement")
                self.assertRegex(record["assessed_at"], r"^\d{4}-\d{2}-\d{2}$")

    def test_every_reported_fix_is_assessed(self):
        assessed = {(r["release"], i) for r in self.load() for i in r["ids"]}
        reports = sorted((ROOT / "engineering/compatibility").glob("upstream-diff-*.json"))
        self.assertTrue(reports, "no committed upstream diff report")
        for path in reports:
            report = json.loads(path.read_text())
            for entry in report["security"]:
                self.assertEqual(entry["assessment"], "needs_assessment", path.name)
                for ident in entry["ids"]:
                    with self.subTest(report=path.name, id=ident):
                        self.assertIn((report["new_version"], ident), assessed)


if __name__ == "__main__":
    unittest.main()
