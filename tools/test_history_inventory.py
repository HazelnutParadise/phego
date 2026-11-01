import json
import tempfile
import unittest
from pathlib import Path

import history_inventory as inv

MUSEUM = ('<table id="list"><tbody>'
          '<tr><td class="link"><a href="{0}">{0}</a></td></tr>'
          '<tr><td class="link"><a href="{1}">{1}</a></td></tr>'
          '</tbody></table>')


class ClassifyNameTests(unittest.TestCase):
    def test_versions_keep_prerelease_and_patch_levels(self):
        cases = {
            "php-2.0b10.tar.gz": "2.0b10",
            "php-4.0.4pl1RC1.tar.gz": "4.0.4pl1RC1",
            "php-4.3.0-dev-zend2-win32-alpha2.zip": "4.3.0-dev-zend2-alpha2",
            "php-5.2.9-2-nts-Win32.zip": "5.2.9",
            "php-8.5.12-Win32-vs17-x64.zip": "8.5.12",
            "php-debug-pack-5.2.9-Win32.zip": "5.2.9",
        }
        for name, version in cases.items():
            with self.subTest(name=name):
                self.assertEqual(inv.classify_name(name)[0], version)

    def test_installers_keep_museum_urls_and_dotless_names_stay_unresolved(self):
        out = BuildTests().build(["php-4.0.4-installer.exe", "php405-installer.exe"], {})
        by_version = {r["version"]: r for r in out["releases"]}
        artifact = by_version["4.0.4"]["artifacts"][0]
        self.assertEqual(artifact["platform"], "installer")
        self.assertTrue(artifact["url"].endswith("/php-4.0.4-installer.exe"))
        self.assertEqual([u["filename"] for u in out["unresolved"]], ["php405-installer.exe"])
        self.assertNotIn("4.0.5", by_version)

    def test_dates_are_not_copied_across_versions_in_one_entry(self):
        releases = {"3.0.x (latest)": {"date": "20 Oct 2000",
                                        "source": [{"filename": "php-3.0.18.tar.gz"}],
                                        "windows": [{"filename": "php-3.0.17-win32.zip"}]},
                    "4.0.1": {"date": "28 Jun 2000", "source": [{"filename": "php-4.0.1pl2.tar.gz"}]},
                    "8.5.12": {"source": [{"filename": "php-8.5.12.tar.xz", "date": "21 Oct 2026"}]}}
        out = BuildTests().build(["php-1.99s.tar.gz", "php-2.0.tar.gz"], releases)
        by_version = {r["version"]: r for r in out["releases"]}
        self.assertNotIn("date", by_version["3.0.18"])
        self.assertNotIn("date", by_version["3.0.17"])
        self.assertNotIn("date", by_version["4.0.1pl2"])
        self.assertEqual(by_version["4.0.1"]["date"], "28 Jun 2000")
        self.assertEqual(by_version["8.5.12"]["date"], "21 Oct 2026")
        self.assertEqual(by_version["3.0.18"]["artifacts"][0]["release_entry"], "3.0.x (latest)")

    def test_pre_suffix_is_a_prerelease(self):
        out = BuildTests().build(["php-4.3.0pre2-Win32.zip", "php-4.3.0.tar.gz"], {})
        by_version = {r["version"]: r for r in out["releases"]}
        self.assertEqual(by_version["4.3.0pre2"]["release_type"], "prerelease")
        self.assertEqual(by_version["4.3.0"]["release_type"], "release")

    def test_windows_rebuild_is_an_artifact_not_a_release(self):
        version, artifact = inv.classify_name("php-5.2.9-2-nts-Win32.zip")
        self.assertEqual((version, artifact["build"], artifact["platform"]), ("5.2.9", "2", "nts-Win32"))

    def test_names_without_exact_version_are_unresolved(self):
        for name in ("php-108.tar.gz", "README.txt", "php-latest.tar.gz", "phpx-1.0.tar.gz"):
            with self.subTest(name=name):
                self.assertEqual(inv.classify_name(name), (None, None))


class BuildTests(unittest.TestCase):
    def build(self, museum_files, releases):
        with tempfile.TemporaryDirectory() as d:
            m = Path(d, "m.html")
            m.write_text(MUSEUM.format(*museum_files), encoding="utf-8")
            r = Path(d, "r.json")
            r.write_text(json.dumps(releases), encoding="utf-8")
            return inv.build([("https://museum.php.net/php1/", m)], [("8", r)])

    def test_first_release_stays_missing_and_108_is_not_its_substitute(self):
        out = self.build(["php-108.tar.gz", "php-1.99s.tar.gz"], {})
        by_version = {r["version"]: r for r in out["releases"]}
        self.assertEqual(by_version["1.0"]["source_status"], "missing")
        self.assertEqual(by_version["1.0"]["artifacts"], [])
        self.assertEqual(by_version["1.0.8"]["artifacts"][0]["filename"], "php-108.tar.gz")
        self.assertIn("identity_evidence", by_version["1.0.8"])
        self.assertEqual(out["inventory_status"], "incomplete")

    def test_each_patch_release_has_its_own_record_with_checksums(self):
        releases = {
            "8.5.12": {"date": "21 Oct 2026", "source": [{"filename": "php-8.5.12.tar.xz", "sha256": "ab"}]},
            "8.5.11": {"source": [{"filename": "php-8.5.11.tar.xz", "sha256": "cd"}]},
            "5.5.6": {"source": [{"link": "http://windows.php.net/download/", "name": "Windows"}]},
        }
        out = self.build(["php-1.99s.tar.gz", "php-2.0.tar.gz"], releases)
        by_version = {r["version"]: r for r in out["releases"]}
        self.assertTrue(by_version["8.5.12"]["published_checksums"])
        self.assertEqual(by_version["8.5.11"]["artifacts"][0]["sha256"], "cd")
        self.assertEqual(by_version["5.5.6"]["artifacts"][0]["kind"], "link")
        self.assertNotIn("8.5", by_version)

    def test_oversized_or_invalid_inputs_are_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            big = Path(d, "big.json")
            big.write_bytes(b" " * (inv.MAX_RELEASES_BYTES + 1))
            with self.assertRaises(ValueError):
                inv.build([], [("8", big)])
            self.assertEqual(inv.main(["--releases", "x=" + str(big), "-o", str(Path(d, "o.json"))]), 2)


if __name__ == "__main__":
    unittest.main()
