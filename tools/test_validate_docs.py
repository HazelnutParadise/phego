#!/usr/bin/env python3
"""Regression tests for the documentation validator, not the PHP engine."""
from __future__ import annotations
import json
import shutil
import tempfile
import unittest
from pathlib import Path
from validate_docs import validate

SOURCE = Path(__file__).resolve().parents[1]

class DocumentationValidatorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="phego-doc-check-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "repo"
        shutil.copytree(SOURCE, self.root, ignore=shutil.ignore_patterns(".git", "__pycache__"))

    def mutate_json(self, filename, transform) -> None:
        path = self.root / "docs/data" / filename
        value = json.loads(path.read_text())
        transform(value)
        path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")

    def assert_rejected(self, expected_check: str) -> None:
        report = validate(self.root)
        self.assertEqual(report["status"], "fail")
        self.assertTrue(any(expected_check in e for e in report["errors"]), report["errors"])
        self.assertEqual(report["product_test_status"], "not_run")

    def test_valid_package(self) -> None:
        report = validate(self.root)
        self.assertEqual(report["status"], "pass", report["errors"])
        self.assertEqual(report["counts"]["requirements"], 101)
        self.assertFalse(report["engine_implemented"])

    def test_dependency_cycle(self) -> None:
        self.mutate_json("phego-work-packages.json", lambda o: o["items"][0]["dependencies"].append("W-60"))
        self.assert_rejected("acyclic work-package dependencies")

    def test_unknown_requirement(self) -> None:
        self.mutate_json("phego-acceptance-tests.json", lambda o: o["items"][0].update(requirement="REQ-999"))
        self.assert_rejected("bidirectional requirement/test/work traceability")

    def test_false_product_pass(self) -> None:
        self.mutate_json("phego-acceptance-tests.json", lambda o: o["items"][0].update(status="passed"))
        self.assert_rejected("unexecuted planning state")

    def test_duplicate_requirement(self) -> None:
        self.mutate_json("phego-requirements.json", lambda o: o["items"][1].update(id="REQ-001"))
        self.assert_rejected("unique IDs")

    def test_fake_source_lock(self) -> None:
        self.mutate_json("php-history-policy.json", lambda o: o["required_generations"][0].update(source_lock_status="certified"))
        self.assert_rejected("planning catalogs and references")

    def test_readme_language(self) -> None:
        p = self.root / "README.md"
        p.write_text(p.read_text() + "\n測試。\n")
        self.assert_rejected("English README")

    def test_broken_internal_link(self) -> None:
        p = self.root / "README.md"
        p.write_text(p.read_text() + "\n[Broken](missing-file.md)\n")
        self.assert_rejected("local Markdown links")

    def test_missing_meme_slogan(self) -> None:
        p = self.root / "README.md"
        p.write_text(p.read_text().replace("The safest PHP in the world", "A PHP engine."))
        self.assert_rejected("requested slogan")

    def test_missing_mit_license(self) -> None:
        (self.root / "LICENSE").write_text("Unlicensed\n")
        self.assert_rejected("MIT license identity")

    def test_undefined_source(self) -> None:
        p = self.root / "docs/spec/specification.zh-TW.md"
        p.write_text(p.read_text() + "\n[R99]\n")
        self.assert_rejected("source references resolve")

if __name__ == "__main__":
    unittest.main()
