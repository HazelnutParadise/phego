"""Regression checks for the additive diagnostics requirement."""
import json
import shutil
import tempfile
import unittest
from pathlib import Path
from validate_docs import validate
from traceability import report

SOURCE = Path(__file__).resolve().parents[1]

class DiagnosticRegistryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="phego-diagnostic-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "repo"
        shutil.copytree(SOURCE, self.root, ignore=shutil.ignore_patterns(".git", "__pycache__"))

    def test_additive_requirement_and_unrun_scenarios(self):
        requirements = json.loads((self.root / "docs/data/phego-requirements.json").read_text())["items"]
        tests = json.loads((self.root / "docs/data/phego-acceptance-tests.json").read_text())["items"]
        self.assertEqual({x["id"] for x in requirements}, {f"REQ-{i:03d}" for i in range(1, 102)})
        self.assertEqual({x["id"] for x in tests}, {f"T-{i:03d}" for i in range(1, 203)})
        status = report(self.root, None)
        for test in ("T-201", "T-202"):
            self.assertEqual(status["results"][test], "not_run")
        self.assertEqual(validate(self.root)["status"], "pass")

    def test_new_id_accepts_evidence_but_unknown_and_duplicate_do_not(self):
        import hashlib
        evidence = self.root / "local-evidence.txt"
        evidence.write_text("observed test evidence\n")
        result_file = self.root / "local-results.json"
        record = {"test_id":"T-201", "status":"pass", "commit":"a"*40,
                  "command":"example test", "profile":{"release":"8.5.12", "platform":"linux/amd64", "build":"pure-go"},
                  "exit_code":0, "evidence":{"path":"local-evidence.txt", "sha256":hashlib.sha256(evidence.read_bytes()).hexdigest()}}
        result_file.write_text(json.dumps({"records":[record]}))
        self.assertEqual(report(self.root, result_file)["results"]["T-201"], "pass")
        for records in ([record, record], [{**record, "test_id":"T-203"}]):
            result_file.write_text(json.dumps({"records":records}))
            with self.assertRaises(ValueError): report(self.root, result_file)

    def test_missing_or_duplicate_owner_is_rejected(self):
        path = self.root / "docs/implementation/github-index.json"
        original = json.loads(path.read_text())
        for mode in ("missing", "duplicate"):
            with self.subTest(mode=mode):
                value = json.loads(json.dumps(original))
                if mode == "missing": del value["acceptance_owner"]["T-201"]
                else:
                    next(x for x in value["items"] if x["id"] == "W-08")["acceptance_owner_for"].append("T-201")
                path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
                result = validate(self.root)
                self.assertEqual(result["status"], "fail")
                self.assertTrue(any("acceptance owner" in e for e in result["errors"]), result["errors"])

    def test_duplicate_owner_json_key_is_rejected(self):
        path = self.root / "docs/implementation/github-index.json"
        original = path.read_text()
        marker = '"T-201": "W-42"'
        self.assertEqual(original.count(marker), 1)
        path.write_text(original.replace(marker, '"T-201": "W-08", "T-201": "W-42"'))
        result = validate(self.root)
        self.assertEqual(result["status"], "fail")
        self.assertTrue(any("acceptance owner" in e for e in result["errors"]))

    def test_index_relationships_cannot_diverge_from_plan(self):
        path = self.root / "docs/implementation/github-index.json"
        original = json.loads(path.read_text())
        for field, value in (("requirements", "REQ-101"), ("tests", "T-201")):
            with self.subTest(field=field):
                data = json.loads(json.dumps(original))
                next(x for x in data["items"] if x["id"] == "W-42")[field].remove(value)
                path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
                result = validate(self.root)
                self.assertEqual(result["status"], "fail")
                self.assertTrue(any("acceptance owner" in e for e in result["errors"]))

    def test_missing_reverse_work_package_link_is_rejected(self):
        path = self.root / "docs/data/phego-work-packages.json"
        value = json.loads(path.read_text())
        next(x for x in value["items"] if x["id"] == "W-09")["requirements"].remove("REQ-101")
        path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
        result = validate(self.root)
        self.assertEqual(result["status"], "fail")
        self.assertTrue(any("traceability" in e for e in result["errors"]))

if __name__ == "__main__": unittest.main()
