#!/usr/bin/env python3
"""Check W-01's invariant index against the specification, not engine behavior."""
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class ContractDocumentTests(unittest.TestCase):
    def test_invariant_checklist_matches_specification(self):
        specification = (ROOT / "docs/spec/specification.zh-TW.md").read_text(encoding="utf-8")
        chapter = specification.split('## 03　不可違反的系統不變條件', 1)[1].split(
            '<a id="chapter-04"></a>', 1
        )[0]
        source_ids = re.findall(r"^I-\d{2}(?=：)", chapter, re.MULTILINE)
        checklist = (ROOT / "docs/decisions/W-01-contract.md").read_text(encoding="utf-8")
        listed_ids = re.findall(r"^\| (I-\d{2}) \|", checklist, re.MULTILINE)
        self.assertTrue(source_ids, "the specification must contain invariant IDs")
        self.assertEqual(len(source_ids), len(set(source_ids)), "duplicate specification ID")
        self.assertEqual(len(listed_ids), len(set(listed_ids)), "duplicate checklist ID")
        self.assertEqual(set(listed_ids), set(source_ids), "checklist differs from specification")


if __name__ == "__main__":
    unittest.main()
