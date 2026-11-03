"""Check synthetic PHPT reference observations without claiming runner acceptance."""
from __future__ import annotations

import gzip
import hashlib
import json
import re
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "tests/runner/phpt-fixtures"
MANIFEST = FIXTURES / "reference-expectations.json"


class PHPTReferenceEvidenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))

    def test_every_fixture_is_pinned_and_remains_unrun_by_phego(self) -> None:
        entries = self.manifest["items"]
        self.assertEqual(len(entries), 8)
        self.assertEqual({entry["file"] for entry in entries}, {path.name for path in FIXTURES.glob("*.phpt")})
        self.assertEqual(self.manifest["product_acceptance"], "not_claimed")
        for entry in entries:
            with self.subTest(file=entry["file"]):
                actual = hashlib.sha256((FIXTURES / entry["file"]).read_bytes()).hexdigest()
                self.assertEqual(actual, entry["sha256"])
                self.assertEqual(entry["phego_status"], "not_run")

    def test_each_reference_log_preserves_all_results_and_exit(self) -> None:
        expected = {entry["file"]: entry["reference_status"] for entry in self.manifest["items"]}
        logs = self.manifest["reference"]["runs"]
        self.assertEqual(len(logs), 2)
        for log in logs:
            with self.subTest(log=log["path"]):
                raw = (ROOT / log["path"]).read_bytes()
                self.assertEqual(hashlib.sha256(raw).hexdigest(), log["sha256"])
                unpacked = gzip.decompress(raw)
                self.assertEqual(hashlib.sha256(unpacked).hexdigest(), log["uncompressed_sha256"])
                text = unpacked.decode("utf-8")
                text = re.sub(r"\x1b\[[0-9;]*m", "", text).replace("\r", "\n")
                observed = re.findall(r"(?m)^(PASS|FAIL|SKIP|XFAIL|WARN|BORK) [^\n]*\[/tmp/fixtures/([^\]]+)\]", text)
                self.assertEqual(len(observed), 8)
                self.assertEqual({filename: status for status, filename in observed}, expected)
                self.assertIn("REFERENCE_RUNNER_EXIT=1", text)


if __name__ == "__main__":
    unittest.main()
