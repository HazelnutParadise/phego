"""Tests for comparing traceable execution snapshots."""
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from compare_evidence import compare


class CompareEvidenceTests(unittest.TestCase):
    PROFILE = {"release": "8.5.12", "platform": "linux/arm64", "build": "cli"}
    COMMIT = "a" * 40

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        registry = self.root / "docs/data/phego-acceptance-tests.json"
        registry.parent.mkdir(parents=True)
        registry.write_text(json.dumps({"items": [{"id": f"T-{index:03d}"} for index in range(1, 11)]}))

    def tearDown(self):
        self.temp.cleanup()

    def evidence_record(self, test_id, status, *, profile=None, commit=None, contents=None):
        if status == "not_run":
            return {"test_id": test_id, "status": status}
        if contents is None:
            contents = f"{test_id}:{status}:{profile or self.PROFILE}:{commit or self.COMMIT}\n".encode()
        evidence_path = f"evidence/{test_id}-{status}-{hashlib.sha256(contents).hexdigest()[:8]}.log"
        evidence_file = self.root / evidence_path
        evidence_file.parent.mkdir(exist_ok=True)
        evidence_file.write_bytes(contents)
        return {
            "test_id": test_id,
            "status": status,
            "commit": commit or self.COMMIT,
            "command": "python3 -m unittest synthetic_case",
            "profile": dict(profile or self.PROFILE),
            "exit_code": 0 if status == "pass" else 1 if status in ("fail", "semantic_difference") else None,
            "evidence": {"path": evidence_path, "sha256": hashlib.sha256(contents).hexdigest()},
        }

    def snapshot(self, name, records):
        path = self.root / name
        path.write_text(json.dumps({"records": records}))
        return path

    def test_empty_snapshots_preserve_the_full_registry(self):
        previous = self.snapshot("previous.json", [])
        current = self.snapshot("current.json", [])
        result = compare(self.root, previous, current)

        self.assertEqual(result["product_acceptance"], "not_claimed")
        self.assertEqual(result["changes"], {
            f"T-{index:03d}": {"previous": "not_run", "current": "not_run"}
            for index in range(1, 11)
        })
        self.assertEqual(result["previous"]["results"]["T-001"], "not_run")
        self.assertEqual(result["current"]["results"]["T-010"], "not_run")

    def test_initial_snapshot_keeps_omitted_tests_not_run(self):
        current = self.snapshot("current.json", [self.evidence_record("T-001", "pass")])
        previous = self.snapshot("previous.json", [])

        result = compare(self.root, previous, current)

        self.assertEqual(result["changes"]["T-001"], {"previous": "not_run", "current": "pass"})
        self.assertEqual(result["current"]["results"]["T-002"], "not_run")
        self.assertEqual(result["product_acceptance"], "not_claimed")

    def test_executed_profiles_must_match_but_commits_may_differ(self):
        previous = self.snapshot("previous.json", [self.evidence_record("T-001", "fail")])
        changed_profile = dict(self.PROFILE, platform="linux/amd64")
        current = self.snapshot("current.json", [self.evidence_record("T-001", "fail", profile=changed_profile)])
        with self.assertRaises(ValueError):
            compare(self.root, previous, current)

        newer_commit = "b" * 40
        current = self.snapshot("current.json", [self.evidence_record("T-001", "fail", commit=newer_commit)])
        result = compare(self.root, previous, current)
        self.assertEqual(result["previous"]["execution_records"][0]["commit"], self.COMMIT)
        self.assertEqual(result["current"]["execution_records"][0]["commit"], newer_commit)

    def test_fail_to_pass_retains_both_records_without_certifying_acceptance(self):
        before = self.evidence_record("T-001", "fail")
        after = self.evidence_record("T-001", "pass", commit="b" * 40)
        previous = self.snapshot("previous.json", [before])
        current = self.snapshot("current.json", [after])

        result = compare(self.root, previous, current)

        self.assertEqual(result["changes"]["T-001"], {"previous": "fail", "current": "pass"})
        self.assertEqual(result["previous"]["execution_records"][0]["evidence"], before["evidence"])
        self.assertEqual(result["current"]["execution_records"][0]["evidence"], after["evidence"])
        self.assertEqual(result["product_acceptance"], "not_claimed")

    def test_pass_to_fail_is_reported_as_a_regression(self):
        previous = self.snapshot("previous.json", [self.evidence_record("T-001", "pass")])
        current = self.snapshot("current.json", [self.evidence_record("T-001", "fail", commit="b" * 40)])

        result = compare(self.root, previous, current)

        self.assertEqual(result["changes"]["T-001"], {"previous": "pass", "current": "fail"})
        self.assertEqual(result["current"]["results"]["T-001"], "fail")

    def test_prior_failure_and_execution_states_cannot_be_erased_or_downgraded(self):
        protected_previous = ("pass", "fail", "crash", "timeout", "runner_error", "semantic_difference")
        forbidden_current = (None, "not_run", "skip", "environment_skip", "unsupported")
        for old_status in protected_previous:
            for new_status in forbidden_current:
                with self.subTest(previous=old_status, current=new_status):
                    old = self.evidence_record("T-001", old_status)
                    previous = self.snapshot("previous.json", [old])
                    records = [] if new_status is None else [self.evidence_record("T-001", new_status)]
                    current = self.snapshot("current.json", records)
                    with self.assertRaises(ValueError):
                        compare(self.root, previous, current)

    def test_invalid_summaries_unknown_ids_and_duplicate_ids_are_rejected(self):
        good = self.evidence_record("T-001", "fail")
        previous = self.snapshot("previous.json", [good])
        bad = dict(good, evidence={"path": good["evidence"]["path"], "sha256": "0" * 64})
        current = self.snapshot("current.json", [bad])
        with self.assertRaises(ValueError):
            compare(self.root, previous, current)

        unknown = self.evidence_record("T-099", "pass")
        current = self.snapshot("current.json", [unknown])
        with self.assertRaises(ValueError):
            compare(self.root, previous, current)

        duplicate = self.evidence_record("T-001", "fail")
        current = self.snapshot("current.json", [duplicate, duplicate])
        with self.assertRaises(ValueError):
            compare(self.root, previous, current)

        duplicate_registry = self.root / "docs/data/phego-acceptance-tests.json"
        duplicate_registry.write_text(json.dumps({"items": [{"id": "T-001"}, {"id": "T-001"}]}))
        with self.assertRaises(ValueError):
            compare(self.root, previous, previous)

    def test_cli_success_prints_both_reports_and_not_claimed(self):
        previous = self.snapshot("previous.json", [self.evidence_record("T-001", "fail")])
        current = self.snapshot("current.json", [self.evidence_record("T-001", "pass", commit="b" * 40)])
        script = Path(__file__).with_name("compare_evidence.py")

        result = subprocess.run(
            [sys.executable, str(script), "--root", str(self.root), "--previous", str(previous), "--current", str(current)],
            text=True, capture_output=True, check=False,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertIn("previous", output)
        self.assertIn("current", output)
        self.assertEqual(output["changes"]["T-001"], {"previous": "fail", "current": "pass"})
        self.assertEqual(output["product_acceptance"], "not_claimed")

    def test_cli_invalid_profile_exits_with_error_status(self):
        previous = self.snapshot("previous.json", [self.evidence_record("T-001", "pass")])
        profile = dict(self.PROFILE, build="native")
        current = self.snapshot("current.json", [self.evidence_record("T-001", "pass", profile=profile)])
        script = Path(__file__).with_name("compare_evidence.py")

        result = subprocess.run(
            [sys.executable, str(script), "--root", str(self.root), "--previous", str(previous), "--current", str(current)],
            text=True, capture_output=True, check=False,
        )

        self.assertEqual(result.returncode, 2)
        self.assertIn("compare_evidence: error:", result.stderr)
        self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
