"""Contract tests for traceability report validation and its command line."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import traceability


PROJECT_ROOT = Path(__file__).resolve().parents[1]
REGISTRY = PROJECT_ROOT / "docs/data/phego-acceptance-tests.json"
TEST_IDS = [item["id"] for item in json.loads(REGISTRY.read_text())["items"]]
COMMIT = "a" * 40


class ReportContractTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        (self.root / "docs/data").mkdir(parents=True)
        (self.root / "docs/data/phego-acceptance-tests.json").write_text(
            json.dumps({"items": [{"id": test_id} for test_id in TEST_IDS]})
        )
        self.evidence = self.root / "evidence.txt"
        self.evidence.write_text("reproducible evidence\n")
        self.results = self.root / "results.json"

    def tearDown(self) -> None:
        self.temp.cleanup()

    def record(self, test_id: str | None = None, status: str = "pass") -> dict:
        record = {
            "test_id": test_id or TEST_IDS[0],
            "status": status,
            "commit": COMMIT,
            "command": "go test ./...",
            "profile": {"release": "8.4", "platform": "linux/amd64", "build": "pure-go"},
            "exit_code": 0,
            "evidence": {
                "path": "evidence.txt",
                "sha256": hashlib.sha256(self.evidence.read_bytes()).hexdigest(),
            },
        }
        if status == "not_run":
            record.pop("commit")
            record.pop("command")
            record.pop("profile")
            record.pop("exit_code")
            record.pop("evidence")
        return record

    def call_report(self, records: list[dict] | None = None) -> dict:
        if records is not None:
            self.results.write_text(json.dumps({"records": records}))
        return traceability.report(self.root, self.results if records is not None else None)

    def assert_invalid(self, records: list[dict]) -> None:
        with self.assertRaises((ValueError, OSError, TypeError)):
            self.call_report(records)

    def test_missing_results_defaults_every_registered_test_to_not_run(self) -> None:
        report = traceability.report(self.root, None)
        self.assertEqual(set(report["results"]), set(TEST_IDS))
        self.assertTrue(all(value == "not_run" for value in report["results"].values()))
        self.assertEqual(report["product_acceptance"], "not_claimed")
        self.assertEqual(report["counts"].get("not_run"), len(TEST_IDS))
        self.assertEqual(sum(report["counts"].values()), len(TEST_IDS))

    def test_counts_include_all_outcomes_and_never_treat_other_status_as_pass(self) -> None:
        statuses = ["pass", "fail", "skip", "unsupported", "runner_error", "not_run"]
        records = [self.record(test_id, status) for test_id, status in zip(TEST_IDS, statuses)]
        for item in records:
            if item["status"] == "pass":
                item["exit_code"] = 0
        report = self.call_report(records)
        self.assertEqual(report["product_acceptance"], "not_claimed")
        for status in statuses[:-1]:
            self.assertEqual(report["counts"][status], 1)
        self.assertEqual(report["counts"]["not_run"], len(TEST_IDS) - 5)
        self.assertEqual(sum(report["counts"].values()), len(TEST_IDS))
        for test_id, status in zip(TEST_IDS, statuses):
            self.assertEqual(report["results"][test_id], status)

    def test_valid_nonzero_exit_is_allowed_for_nonpass_outcomes(self) -> None:
        record = self.record(status="fail")
        record["exit_code"] = 7
        self.assertEqual(self.call_report([record])["results"][TEST_IDS[0]], "fail")

    def test_rejects_unknown_duplicate_ids_and_unknown_status(self) -> None:
        self.assert_invalid([self.record("T-DOES-NOT-EXIST")])
        self.assert_invalid([self.record(), self.record()])
        self.assert_invalid([self.record(status="success")])

    def test_rejects_missing_or_malformed_provenance_fields(self) -> None:
        mutations = [
            lambda r: r.pop("commit"), lambda r: r.update(commit="A" * 40),
            lambda r: r.update(commit="a" * 39), lambda r: r.update(command=""),
            lambda r: r.update(command=4), lambda r: r.update(profile={"release": "8.4", "platform": "linux/amd64"}),
            lambda r: r.update(profile={"release": "", "platform": "linux/amd64", "build": "pure-go"}),
            lambda r: r.update(profile={"release": "8.4", "platform": None, "build": "pure-go"}),
            lambda r: r.update(exit_code=True), lambda r: r.update(exit_code="0"),
            lambda r: r.update(evidence={"path": "evidence.txt"}),
            lambda r: r.update(evidence={"path": "evidence.txt", "sha256": "0" * 64}),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                record = self.record()
                mutate(record)
                self.assert_invalid([record])

    def test_pass_requires_zero_exit_code(self) -> None:
        record = self.record()
        record["exit_code"] = 1
        self.assert_invalid([record])

    def test_not_run_cannot_claim_execution_evidence(self) -> None:
        record = self.record(status="not_run")
        record["evidence"] = {"path": "evidence.txt", "sha256": hashlib.sha256(self.evidence.read_bytes()).hexdigest()}
        self.assert_invalid([record])

    def test_rejects_bad_results_container_and_record_shapes(self) -> None:
        for payload in ([], {}, {"records": {}}, {"records": [{"test_id": TEST_IDS[0]}]}, {"records": [None]}):
            with self.subTest(payload=payload):
                self.results.write_text(json.dumps(payload))
                with self.assertRaises((ValueError, OSError, TypeError)):
                    traceability.report(self.root, self.results)

    def test_rejects_absolute_parent_and_symlink_evidence(self) -> None:
        for bad_path in (str(self.evidence), "../outside.txt"):
            record = self.record()
            record["evidence"]["path"] = bad_path
            self.assert_invalid([record])
        outside = self.root.parent / (self.root.name + "-outside")
        outside.write_text("outside\n")
        link = self.root / "linked-evidence.txt"
        try:
            link.symlink_to(outside)
            record = self.record()
            record["evidence"] = {"path": link.name, "sha256": hashlib.sha256(outside.read_bytes()).hexdigest()}
            self.assert_invalid([record])
        finally:
            outside.unlink(missing_ok=True)

    def test_rejects_directory_and_missing_evidence(self) -> None:
        directory = self.root / "directory"
        directory.mkdir()
        for evidence_path in ("directory", "missing.txt"):
            record = self.record()
            record["evidence"]["path"] = evidence_path
            self.assert_invalid([record])


class CommandLineTests(unittest.TestCase):
    def test_cli_returns_zero_for_valid_results_and_nonzero_for_invalid_results(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "docs/data").mkdir(parents=True)
            (root / "docs/data/phego-acceptance-tests.json").write_text(
                json.dumps({"items": [{"id": "T-001"}]})
            )
            evidence = root / "evidence.txt"
            evidence.write_text("ok")
            result_file = root / "results.json"
            valid = {
                "records": [{
                    "test_id": "T-001", "status": "pass", "commit": COMMIT,
                    "command": "check", "profile": {"release": "8.4", "platform": "linux", "build": "go"},
                    "exit_code": 0,
                    "evidence": {"path": "evidence.txt", "sha256": hashlib.sha256(b"ok").hexdigest()},
                }]
            }
            result_file.write_text(json.dumps(valid))
            cmd = [sys.executable, str(PROJECT_ROOT / "tools/traceability.py"), "--root", str(root), "--results", str(result_file)]
            success = subprocess.run(cmd, capture_output=True, text=True)
            self.assertEqual(success.returncode, 0, success.stderr)
            result_file.write_text(json.dumps({"records": [{"test_id": "T-001", "status": "pass"}]}))
            failure = subprocess.run(cmd, capture_output=True, text=True)
            self.assertNotEqual(failure.returncode, 0)


if __name__ == "__main__":
    unittest.main()
