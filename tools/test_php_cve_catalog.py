#!/usr/bin/env python3
"""W-41 (REQ-067; T-133, T-134): the PHP CVE catalog, its cases and README block agree."""
import json
import unittest

import php_cve_catalog as cat

PAGE = b"""<section class="version" id="8.5.13"><!-- {{{ 8.5.13 -->
<h3>Version 8.5.13</h3>
<ul><li>Core:
  <ul>
    <li>Fixed GHSA-2fjv-p558-g82m (Use-after-free). (CVE-2026-129182)</li>
  </ul>
</li>
<li>Standard:
  <ul>
    <li>Fixed bug <a href="x">#1</a> (unserialize() use after free). (CVE-2015-0231)</li>
  </ul>
</li></ul>
</section>
"""


class TestCommittedCatalog(unittest.TestCase):
    def test_catalog_cases_and_readme_agree(self):
        doc, problems = cat.check()
        self.assertEqual(problems, [])
        self.assertEqual(doc["totals"]["entries"], len(doc["entries"]))

    def test_every_entry_is_complete(self):
        doc = json.loads(cat.CATALOG.read_text())
        ids = [e["id"] for e in doc["entries"]]
        self.assertEqual(len(ids), len(set(ids)), "duplicate identifiers")
        for e in doc["entries"]:
            with self.subTest(id=e["id"]):
                self.assertIn(e["status"], cat.STATUSES)
                self.assertIn(e["component"], cat.COMPONENTS)
                self.assertIn(e["defect_class"], cat.CLASSES)
                if e["status"] in ("tested", "pending"):
                    self.assertEqual(e["work_package"], cat.COMPONENTS[e["component"]]["wp"])
                else:
                    self.assertIsNone(e["work_package"])
                    self.assertTrue(e.get("note"), "an entry Phego does not track needs its reason")
                self.assertEqual(bool(e["tests"]), e["status"] == "tested")

    def test_tested_components_are_implemented(self):
        doc = json.loads(cat.CATALOG.read_text())
        for e in doc["entries"]:
            if e["status"] == "tested":
                with self.subTest(id=e["id"]):
                    self.assertTrue(cat.COMPONENTS[e["component"]]["implemented"])


class TestParsing(unittest.TestCase):
    def test_sections_and_ids(self):
        found = cat.parse_changelog("8", PAGE)
        self.assertEqual(found["CVE-2026-129182"][0]["section"], "core")
        self.assertEqual(found["GHSA-2fjv-p558-g82m"][0]["release"], "8.5.13")
        self.assertEqual(found["CVE-2015-0231"][0]["section"], "standard")

    def test_identifier_outside_an_item_fails(self):
        page = PAGE.replace(b"</section>", b"<p>Also CVE-2026-999999.</p>\n</section>")
        with self.assertRaisesRegex(ValueError, "CVE-2026-999999"):
            cat.parse_changelog("8", page)

    def test_page_without_versions_fails(self):
        with self.assertRaisesRegex(ValueError, "no version sections"):
            cat.parse_changelog("8", b"<html>CVE-2026-1</html>")


class TestClassification(unittest.TestCase):
    def test_function_beats_broad_section(self):
        rec = {"changelog": [{"section": "core", "text": "Use after free in unserialize()"}], "summary": "", "cwe": []}
        self.assertEqual(cat.classify(rec)[:2], ("serialization", "memory-safety"))

    def test_specific_section_wins(self):
        rec = {"changelog": [{"section": "fpm", "text": "env_path_info underflow"}], "summary": "", "cwe": ["CWE-120"]}
        self.assertEqual(cat.classify(rec), ("fastcgi", "memory-safety", "changelog-section"))

    def test_memory_text_beats_generic_cwe(self):
        rec = {"changelog": [], "summary": "Use-after-free vulnerability in substr_replace", "cwe": ["CWE-399"]}
        self.assertEqual(cat.classify(rec)[1], "memory-safety")


class TestReadme(unittest.TestCase):
    def test_splice_inserts_then_replaces(self):
        text = "# T\n\n## Project status\n\nx\n"
        once = cat.splice_readme(text, cat.README_BEGIN + "\nA\n" + cat.README_END + "\n")
        twice = cat.splice_readme(once, cat.README_BEGIN + "\nB\n" + cat.README_END + "\n")
        self.assertIn("\nB\n", twice)
        self.assertNotIn("\nA\n", twice)
        self.assertEqual(twice.count(cat.README_BEGIN), 1)
        self.assertTrue(twice.rstrip().endswith("x"))


if __name__ == "__main__":
    unittest.main()
