import hashlib
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class FrozenPlanTests(unittest.TestCase):
    def setUp(self):
        self.plan = json.loads((ROOT / "benchmarks/plan.json").read_text(encoding="utf-8"))

    def test_every_inventoried_competitor_has_every_cell(self):
        inventory = json.loads((ROOT / "docs/implementation/evidence/W-54-discovered-inventory.json").read_text(encoding="utf-8"))
        self.assertEqual(self.plan["competitors"], [item["id"] for item in inventory["items"]])
        expected = (len(self.plan["competitors"]) * len(self.plan["workloads"])
                    * len(self.plan["platforms"]) * len(self.plan["modes"]))
        self.assertEqual(len(self.plan["cells"]), expected)

    def test_no_cell_claims_a_result_without_evidence(self):
        for cell in self.plan["cells"]:
            if cell["status"] != "pending":
                self.assertTrue(cell["evidence"].strip(), cell)

    def test_workload_sources_and_oracles_match_recorded_hashes(self):
        listed = {entry["path"]: entry["sha256"] for entry in self.plan["files"]}
        for workload in self.plan["workloads"]:
            for ext in ("php", "expected"):
                path = f"benchmarks/workloads/{workload}.{ext}"
                self.assertEqual(listed[path], hashlib.sha256((ROOT / path).read_bytes()).hexdigest(), path)
            self.assertTrue((ROOT / f"benchmarks/workloads/{workload}.expected").read_text().strip())

    def test_coverage_gaps_stay_visible(self):
        gaps = json.loads((ROOT / "benchmarks/coverage-gaps.json").read_text(encoding="utf-8"))["gaps"]
        self.assertTrue(gaps)
        self.assertTrue(all(g["status"] in ("open", "closed") and g["description"] for g in gaps))


if __name__ == "__main__":
    unittest.main()
