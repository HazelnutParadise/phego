"""Contract tests for capability claims and their evidence gate."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import capabilities
import traceability


PROJECT_ROOT = Path(__file__).resolve().parents[1]
SCRIPT = PROJECT_ROOT / "tools/capabilities.py"
PROFILE = {"release": "8.4", "platform": "linux/amd64", "build": "pure-go"}
COMMIT = "a" * 40
TEST_IDS = ["T-CAP-001", "T-CAP-002", "T-CAP-003"]
STATUSES = ("unsupported", "unverified", "policy_denied", "semantic_difference", "failed")


class CapabilityReportTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        (self.root / "docs/data").mkdir(parents=True)
        (self.root / "docs/data/phego-acceptance-tests.json").write_text(
            json.dumps({"items": [{"id": test_id} for test_id in TEST_IDS]}), encoding="utf-8"
        )
        self.evidence = self.root / "evidence.txt"
        self.evidence.write_text("capability evidence\n", encoding="utf-8")
        self.results = self.root / "results.json"
        self.capability_file = self.root / "capabilities.json"

    def tearDown(self) -> None:
        self.temp.cleanup()

    def claim(self, **changes: object) -> dict:
        item = {
            "kind": "function",
            "name": "strlen",
            "status": "supported",
            "test_ids": [TEST_IDS[0]],
            "reason": "Verified by the registered test.",
        }
        item.update(changes)
        return item

    def payload(self, claims: list[dict] | None = None, **changes: object) -> dict:
        value = {"schema_version": 1, "profile": dict(PROFILE), "claims": claims or []}
        value.update(changes)
        return value

    def write_capabilities(self, value: object) -> Path:
        self.capability_file.write_text(json.dumps(value, ensure_ascii=False), encoding="utf-8")
        return self.capability_file

    def write_raw_capabilities(self, raw: bytes) -> Path:
        self.capability_file.write_bytes(raw)
        return self.capability_file

    def record(self, test_id: str, status: str = "pass", profile: dict | None = None) -> dict:
        record = {
            "test_id": test_id,
            "status": status,
            "commit": COMMIT,
            "command": "python3 -m unittest discover -s tools",
            "profile": dict(profile or PROFILE),
            "exit_code": 0 if status == "pass" else 1,
            "evidence": {
                "path": "evidence.txt",
                "sha256": hashlib.sha256(self.evidence.read_bytes()).hexdigest(),
            },
        }
        if status == "not_run":
            for field in ("commit", "command", "profile", "exit_code", "evidence"):
                record.pop(field)
        return record

    def write_results(self, records: list[dict]) -> Path:
        self.results.write_text(json.dumps({"records": records}), encoding="utf-8")
        return self.results

    def assert_invalid(self, payload: object, results: Path | None = None) -> None:
        caps = self.write_capabilities(payload)
        with self.assertRaises((ValueError, OSError, TypeError)):
            capabilities.report(self.root, caps, results)

    def test_supported_claim_requires_and_accepts_traceability_pass_for_same_profile(self) -> None:
        results = self.write_results([self.record(TEST_IDS[0])])
        checked = traceability.report(self.root, results)
        self.assertEqual(checked["results"][TEST_IDS[0]], "pass")

        output = capabilities.report(self.root, self.write_capabilities(self.payload([self.claim()])), results)
        self.assertEqual(output["claims"], [self.claim()])
        self.assertEqual(output["profile"], PROFILE)
        self.assertEqual(output["verification_status"], "not_verified")
        self.assertEqual(output["product_acceptance"], "not_claimed")

    def test_each_open_status_is_preserved_with_required_reason(self) -> None:
        claims = [
            self.claim(name=f"feature_{status}", status=status, test_ids=[], reason=f"Still {status}.")
            for status in STATUSES
        ]
        output = capabilities.report(self.root, self.write_capabilities(self.payload(claims)), None)
        self.assertEqual(output["claims"], claims)
        self.assertEqual(output["verification_status"], "not_verified")
        self.assertEqual(output["product_acceptance"], "not_claimed")

    def test_empty_claim_list_is_valid_and_preserved(self) -> None:
        value = self.payload([])
        output = capabilities.report(self.root, self.write_capabilities(value), None)
        self.assertEqual(output["claims"], [])
        self.assertEqual(output["profile"], PROFILE)
        self.assertEqual(output["verification_status"], "not_verified")
        self.assertEqual(output["product_acceptance"], "not_claimed")

    def test_supported_claim_rejects_missing_results_and_nonpassing_records(self) -> None:
        caps = self.write_capabilities(self.payload([self.claim()]))
        with self.assertRaises((ValueError, OSError, TypeError)):
            capabilities.report(self.root, caps, None)
        for status in ("not_run", "fail"):
            with self.subTest(status=status):
                results = self.write_results([self.record(TEST_IDS[0], status)])
                with self.assertRaises((ValueError, OSError, TypeError)):
                    capabilities.report(self.root, caps, results)

    def test_supported_claim_rejects_profile_mismatch_and_bad_evidence_digest(self) -> None:
        other_profile = {**PROFILE, "release": "8.3"}
        with self.subTest(case="profile"):
            results = self.write_results([self.record(TEST_IDS[0], profile=other_profile)])
            with self.assertRaises((ValueError, OSError, TypeError)):
                capabilities.report(self.root, self.write_capabilities(self.payload([self.claim()])), results)

        with self.subTest(case="digest"):
            wrong = self.record(TEST_IDS[0])
            wrong["evidence"]["sha256"] = "0" * 64
            results = self.write_results([wrong])
            with self.assertRaises((ValueError, OSError, TypeError)):
                capabilities.report(self.root, self.write_capabilities(self.payload([self.claim()])), results)

    def test_input_object_and_exact_top_level_fields_are_required(self) -> None:
        for value in (
            [], None, 1,
            {"schema_version": 1, "profile": PROFILE, "claims": [], "extra": True},
            {"profile": PROFILE, "claims": []},
            {"schema_version": True, "profile": PROFILE, "claims": []},
        ):
            with self.subTest(value=value):
                self.assert_invalid(value)

    def test_profile_requires_exact_fields_nonblank_strings_and_max_length(self) -> None:
        bad_profiles = (
            {},
            {"release": "8.4", "platform": "linux/amd64"},
            {**PROFILE, "extra": "x"},
            {**PROFILE, "release": "   "},
            {**PROFILE, "build": None},
            {**PROFILE, "platform": "x" * 257},
        )
        for profile in bad_profiles:
            with self.subTest(profile=profile):
                self.assert_invalid(self.payload(profile=profile))

    def test_claim_requires_exact_fields_valid_types_and_known_enums(self) -> None:
        base = self.claim()
        bad_claims = []
        for field in ("kind", "name", "status", "test_ids", "reason"):
            item = dict(base)
            item.pop(field)
            bad_claims.append(item)
        for field, value in (("kind", "global"), ("name", "f" * 257), ("status", "partial"),
                             ("test_ids", "T-CAP-001"), ("test_ids", [1]), ("reason", None)):
            item = dict(base)
            item[field] = value
            bad_claims.append(item)
        item = dict(base)
        item["extra"] = 1
        bad_claims.append(item)
        for item in bad_claims:
            with self.subTest(item=item):
                self.assert_invalid(self.payload([item]))

    def test_required_nonblank_strings_and_reason_rules(self) -> None:
        for field in ("kind", "name"):
            item = self.claim()
            item[field] = " \t "
            with self.subTest(field=field):
                self.assert_invalid(self.payload([item]))
        self.assert_invalid(self.payload([self.claim(status="unverified", reason=" \n ", test_ids=[])]))
        self.assert_invalid(self.payload([self.claim(status="unsupported", reason=" \t ", test_ids=[])]))
        results = self.write_results([self.record(TEST_IDS[0])])
        output = capabilities.report(self.root, self.write_capabilities(self.payload([self.claim(reason="")])), results)
        self.assertEqual(output["claims"][0]["reason"], "")

    def test_supported_claim_requires_at_least_one_test_id(self) -> None:
        self.assert_invalid(self.payload([self.claim(test_ids=[])]), self.write_results([]))

    def test_test_ids_must_be_unique_known_and_registered(self) -> None:
        for ids in (["T-UNKNOWN"], [TEST_IDS[0], TEST_IDS[0]]):
            with self.subTest(ids=ids):
                item = self.claim(test_ids=ids)
                self.assert_invalid(self.payload([item]), self.write_results([self.record(TEST_IDS[0])]))

    def test_one_registered_test_can_cover_multiple_claims(self) -> None:
        claims = [self.claim(name="first"), self.claim(name="second")]
        results = self.write_results([self.record(TEST_IDS[0])])
        output = capabilities.report(self.root, self.write_capabilities(self.payload(claims)), results)
        self.assertEqual(output["claims"], claims)

    def test_kind_name_identity_cannot_repeat(self) -> None:
        claims = [self.claim(name="same", test_ids=[TEST_IDS[0]]),
                  self.claim(name="same", test_ids=[TEST_IDS[1]])]
        self.assert_invalid(self.payload(claims), self.write_results([
            self.record(TEST_IDS[0]), self.record(TEST_IDS[1])
        ]))

    def test_rejects_duplicate_json_keys_invalid_utf8_missing_file_and_size_limit(self) -> None:
        cases = (
            b'{"schema_version":1,"schema_version":1,"profile":{},"claims":[]}',
            b"\xff",
        )
        for raw in cases:
            with self.subTest(raw=raw[:20]):
                with self.assertRaises((ValueError, OSError, TypeError)):
                    capabilities.report(self.root, self.write_raw_capabilities(raw), None)
        with self.assertRaises((ValueError, OSError, TypeError)):
            capabilities.report(self.root, self.root / "missing.json", None)
        self.capability_file.write_bytes(b" " * (1024 * 1024 + 1))
        with self.assertRaises((ValueError, OSError, TypeError)):
            capabilities.report(self.root, self.capability_file, None)

    def test_rejects_more_than_ten_thousand_claims(self) -> None:
        claims = [self.claim(name=f"function_{index}", test_ids=[], status="unverified", reason="Not checked.")
                  for index in range(10001)]
        raw = json.dumps(self.payload(claims), separators=(",", ":")).encode("utf-8")
        self.assertLess(len(raw), 1024 * 1024)
        with self.assertRaises((ValueError, OSError, TypeError)):
            capabilities.report(self.root, self.write_raw_capabilities(raw), None)

    @unittest.skipUnless(hasattr(os, "mkfifo"), "requires POSIX FIFO")
    def test_fifo_and_symlink_inputs_are_rejected_without_waiting(self) -> None:
        fifo = self.root / "input.fifo"
        os.mkfifo(fifo)
        link = self.root / "input-link.json"
        link.symlink_to(self.write_capabilities(self.payload([])))
        for path in (fifo, link):
            with self.subTest(path=path.name):
                result = subprocess.run([sys.executable, str(SCRIPT), "--root", str(self.root),
                                         "--capabilities", str(path)], capture_output=True, text=True, timeout=3)
                self.assertEqual(result.returncode, 2, result.stderr)

    def test_results_are_read_from_the_original_bounded_snapshot(self) -> None:
        results = self.write_results([self.record(TEST_IDS[0])])
        caps = self.write_capabilities(self.payload([self.claim()]))
        original_report = traceability.report
        def mutate_original(root, snapshot):
            self.write_results([self.record(TEST_IDS[0], "fail")])
            return original_report(root, snapshot)
        with mock.patch.object(traceability, "report", side_effect=mutate_original):
            output = capabilities.report(self.root, caps, results)
        self.assertEqual(output["claims"][0]["status"], "supported")
        self.assertEqual(json.loads(results.read_text())["records"][0]["status"], "fail")

    def test_results_input_rejects_oversized_or_symlinked_files(self) -> None:
        caps = self.write_capabilities(self.payload([self.claim()]))
        self.results.write_bytes(b" " * (1024 * 1024 + 1))
        with self.assertRaises(ValueError):
            capabilities.report(self.root, caps, self.results)
        link = self.root / "results-link.json"
        link.symlink_to(self.results)
        with self.assertRaises(ValueError):
            capabilities.report(self.root, caps, link)

    def test_results_snapshot_does_not_overwrite_same_named_evidence(self) -> None:
        evidence = self.root / "results.json"
        evidence.write_bytes(b"actual evidence")
        record = self.record(TEST_IDS[0])
        record["evidence"] = {"path": "results.json", "sha256": hashlib.sha256(evidence.read_bytes()).hexdigest()}
        records = self.root / "separate-records.json"
        records.write_text(json.dumps({"records": [record]}))
        output = capabilities.report(self.root, self.write_capabilities(self.payload([self.claim()])), records)
        self.assertEqual(output["claims"][0]["status"], "supported")

    def test_total_evidence_size_is_bounded(self) -> None:
        size = 33 * 1024 * 1024
        digest = hashlib.sha256(b"\0" * size).hexdigest()
        records = []
        for index, test_id in enumerate(TEST_IDS[:2]):
            path = self.root / f"large-{index}.log"
            with path.open("wb") as stream:
                stream.truncate(size)
            record = self.record(test_id)
            record["evidence"] = {"path": path.name, "sha256": digest}
            records.append(record)
        claim = self.claim(test_ids=TEST_IDS[:2])
        with self.assertRaises(ValueError):
            capabilities.report(self.root, self.write_capabilities(self.payload([claim])), self.write_results(records))

    def test_registry_alias_cannot_replace_the_snapshot(self) -> None:
        registry = self.root / "docs/data/phego-acceptance-tests.json"
        alias = self.root / "DOCS/data/phego-acceptance-tests.json"
        if not alias.exists() or not os.path.samefile(registry, alias):
            self.skipTest("requires case-insensitive filesystem")
        replacement = json.dumps({"items": [{"id": "T-REPLACED"}]}).encode()
        record = self.record("T-REPLACED")
        record["evidence"] = {"path": "DOCS/data/phego-acceptance-tests.json", "sha256": hashlib.sha256(replacement).hexdigest()}
        caps = self.write_capabilities(self.payload([self.claim(test_ids=["T-REPLACED"])]))
        results = self.write_results([record])
        original = capabilities._read_beneath
        def replace_after_snapshot(root, relative, limit):
            value = original(root, relative, limit)
            if relative == "docs/data/phego-acceptance-tests.json":
                registry.write_bytes(replacement)
            return value
        with mock.patch.object(capabilities, "_read_beneath", side_effect=replace_after_snapshot):
            with self.assertRaises(ValueError):
                capabilities.report(self.root, caps, results)

    def test_registry_size_is_bounded(self) -> None:
        registry = self.root / "docs/data/phego-acceptance-tests.json"
        registry.write_bytes(b'{"items":[]}' + b" " * (1024 * 1024))
        with self.assertRaises(ValueError):
            capabilities.report(self.root, self.write_capabilities(self.payload([])), None)

    def test_cli_can_encode_json_with_unpaired_surrogate(self) -> None:
        value = self.payload(profile={**PROFILE, "build": "\ud800"})
        caps = self.write_raw_capabilities(json.dumps(value).encode("utf-8"))
        result = subprocess.run([sys.executable, str(SCRIPT), "--root", str(self.root),
                                 "--capabilities", str(caps)], capture_output=True, text=True,
                                env={**os.environ, "PYTHONIOENCODING": "utf-8:strict"}, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["profile"], value["profile"])

    @unittest.skipUnless(hasattr(os, "mkfifo"), "requires POSIX FIFO")
    def test_fifo_registry_is_rejected_without_waiting(self) -> None:
        caps = self.write_capabilities(self.payload([]))
        registry = self.root / "docs/data/phego-acceptance-tests.json"
        registry.unlink()
        os.mkfifo(registry)
        result = subprocess.run([sys.executable, str(SCRIPT), "--root", str(self.root),
                                 "--capabilities", str(caps)], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 2, result.stderr)

    def test_cli_exit_codes_for_valid_report_and_invalid_input(self) -> None:
        caps = self.write_capabilities(self.payload([]))
        valid = subprocess.run(
            [sys.executable, str(SCRIPT), "--root", str(self.root), "--capabilities", str(caps)],
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(valid.returncode, 0, valid.stderr)
        self.assertEqual(json.loads(valid.stdout)["product_acceptance"], "not_claimed")

        invalid_caps = self.write_capabilities(self.payload(schema_version=2))
        invalid = subprocess.run(
            [sys.executable, str(SCRIPT), "--root", str(self.root), "--capabilities", str(invalid_caps)],
            text=True, capture_output=True, check=False,
        )
        self.assertEqual(invalid.returncode, 2)
        self.assertIn("capabilities: error:", invalid.stderr)


if __name__ == "__main__":
    unittest.main()
