#!/usr/bin/env python3
import json
import unittest
from urllib.error import URLError

import tools.upstream_watch as uw


class FakeFetch:
    def __init__(self, mapping):
        self.mapping = mapping
        self.calls = []

    def __call__(self, url):
        self.calls.append(url)
        if url in self.mapping:
            val = self.mapping[url]
            if isinstance(val, Exception):
                raise val
            if isinstance(val, bytes):
                return val
            return json.dumps(val).encode("utf-8")
        raise URLError(f"unexpected {url}")


def b(obj):
    if isinstance(obj, bytes):
        return obj
    return json.dumps(obj).encode("utf-8")


CATALOG_URL = "https://www.php.net/releases/index.php?json"


class TestUpstreamWatchObserve(unittest.TestCase):
    def setUp(self):
        self.catalog = {
            "8": {"supported_versions": ["8.1", "8.2"]},
            "7": {"supported_versions": ["7.4"]},
        }

    def _series_resp(self, series, ver):
        vnnn = ver  # e.g. 8.2.10
        major_minor = ".".join(ver.split(".")[:2])
        self.assertEqual(major_minor, series)
        return {
            ver: {
                "source": [
                    {"filename": f"php-{ver}.tar.xz", "sha256": "a" * 64}
                ],
                "announcement": {"English": f"/releases/{ver.replace('.', '_')}.php"},
                "tags": ["security", "release"],
            }
        }

    def _tag_ref(self, ver):
        return {"object": {"type": "tag", "sha": "b" * 40}}

    def _tag_obj(self, sha):
        return {"object": {"type": "commit", "sha": sha}}

    def _commit(self, sha):
        return {"object": {"type": "commit", "sha": sha}}

    def _announce_html(self, ver):
        return f"<html>PHP {ver} announcement</html>".encode("utf-8")

    def test_observe_basic_snapshot(self):
        catalog = self.catalog
        mapping = {CATALOG_URL: b(catalog)}
        # series responses
        mapping["https://www.php.net/releases/index.php?json&version=8.1&max=1"] = b(
            self._series_resp("8.1", "8.1.30")
        )
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(
            self._series_resp("8.2", "8.2.25")
        )
        mapping["https://www.php.net/releases/index.php?json&version=7.4&max=1"] = b(
            self._series_resp("7.4", "7.4.33")
        )
        # tags
        for ver in ["8.1.30", "8.2.25", "7.4.33"]:
            mapping[f"https://api.github.com/repos/php/php-src/git/ref/tags/php-{ver}"] = b(
                self._tag_ref(ver)
            )
            sha = "b" * 40
            mapping[f"https://api.github.com/repos/php/php-src/git/tags/{sha}"] = b(
                self._tag_obj(sha)
            )
        # announcements
        mapping["https://www.php.net/releases/8_1_30.php"] = self._announce_html("8.1.30")
        mapping["https://www.php.net/releases/8_2_25.php"] = self._announce_html("8.2.25")
        mapping["https://www.php.net/releases/7_4_33.php"] = self._announce_html("7.4.33")

        f = FakeFetch(mapping)
        snap = uw.observe(f)
        self.assertIsInstance(snap, dict)
        self.assertEqual(snap.get("schema_version"), 1)
        self.assertIn("series", snap)
        series = snap["series"]
        self.assertEqual(list(series.keys()), sorted(series.keys()))
        for sname in ["7.4", "8.1", "8.2"]:
            rec = series[sname]
            for k in ["version", "tag", "tag_object", "commit", "sha256", "announcement", "announcement_sha256", "release_tags"]:
                self.assertIn(k, rec)
            self.assertEqual(rec["tag_object"], "b" * 40)
            self.assertEqual(rec["commit"], "b" * 40)
            self.assertEqual(rec["tag"], f"php-{rec['version']}")
            self.assertTrue(rec["announcement"].startswith("https://www.php.net/releases/"))
            self.assertEqual(len(rec["sha256"]), 64)
            self.assertTrue(all(c in "0123456789abcdef" for c in rec["sha256"]))
            self.assertEqual(rec["release_tags"], sorted(rec["release_tags"]))
        # tags are sorted in record
        for sname in series:
            self.assertEqual(series[sname]["release_tags"], sorted(series[sname]["release_tags"]))

    def test_boolean_announcement_from_php_release_api(self):
        version = "8.3.12"
        response = self._series_resp("8.3", version)
        response[version]["announcement"] = True
        mapping = {CATALOG_URL: b({"8": {"supported_versions": ["8.3"]}}),
                   "https://www.php.net/releases/index.php?json&version=8.3&max=1": b(response),
                   "https://api.github.com/repos/php/php-src/git/ref/tags/php-8.3.12": b({"object": {"type": "commit", "sha": "c" * 40}}),
                   "https://www.php.net/releases/8_3_12.php": self._announce_html(version)}
        result = uw.observe(FakeFetch(mapping))
        self.assertEqual(result["series"]["8.3"]["announcement"], "https://www.php.net/releases/8_3_12.php")

    def test_catalog_missing_supported_versions_is_rejected(self):
        catalog = {"8": {}, "7": {"supported_versions": ["7.4"]}}
        with self.assertRaises(ValueError):
            uw.observe(FakeFetch({CATALOG_URL: b(catalog)}))

    def test_duplicate_supported_series_is_rejected(self):
        catalog = {"8": {"supported_versions": ["8.3", "8.3"]}}
        with self.assertRaises(ValueError):
            uw.observe(FakeFetch({CATALOG_URL: b(catalog)}))

    def test_announcement_must_not_match_version_prefix(self):
        version = "8.3.12"
        mapping = {CATALOG_URL: b({"8": {"supported_versions": ["8.3"]}}),
                   "https://www.php.net/releases/index.php?json&version=8.3&max=1": b(self._series_resp("8.3", version)),
                   "https://api.github.com/repos/php/php-src/git/ref/tags/php-8.3.12": b({"object": {"type": "commit", "sha": "c" * 40}}),
                   "https://www.php.net/releases/8_3_12.php": b"<html>PHP 8.3.120 announcement</html>"}
        with self.assertRaises(ValueError):
            uw.observe(FakeFetch(mapping))

    def test_observe_lightweight_tag_records_same_sha(self):
        catalog = {"8": {"supported_versions": ["8.3"]}}
        mapping = {CATALOG_URL: b(catalog)}
        mapping["https://www.php.net/releases/index.php?json&version=8.3&max=1"] = b(
            self._series_resp("8.3", "8.3.12")
        )
        ver = "8.3.12"
        sha = "c" * 40
        mapping[f"https://api.github.com/repos/php/php-src/git/ref/tags/php-{ver}"] = b(
            {"object": {"type": "commit", "sha": sha}}
        )
        mapping["https://www.php.net/releases/8_3_12.php"] = self._announce_html(ver)
        f = FakeFetch(mapping)
        snap = uw.observe(f)
        rec = snap["series"]["8.3"]
        self.assertEqual(rec["tag_object"], sha)
        self.assertEqual(rec["commit"], sha)

    def test_observe_missing_archive_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        bad = {
            "8.2.5": {
                "source": [],
                "announcement": {"English": "/releases/8_2_5.php"},
                "tags": ["release"],
            }
        }
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(bad)
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)

    def test_observe_wrong_version_series_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        bad = {
            "8.1.0": {
                "source": [{"filename": "php-8.1.0.tar.xz", "sha256": "a" * 64}],
                "announcement": {"English": "/releases/8_1_0.php"},
                "tags": [],
            }
        }
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(bad)
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)

    def test_observe_malformed_digest_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        bad = {
            "8.2.1": {
                "source": [{"filename": "php-8.2.1.tar.xz", "sha256": "ZZZ"}],
                "announcement": {"English": "/releases/8_2_1.php"},
                "tags": [],
            }
        }
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(bad)
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)

    def test_observe_non_https_or_traversal_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        bad = {
            "8.2.1": {
                "source": [{"filename": "php-8.2.1.tar.xz", "sha256": "a" * 64}],
                "announcement": {"English": "http://evil.com/releases/8_2_1.php"},
                "tags": [],
            }
        }
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(bad)
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)

    def test_observe_missing_tag_object_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b(
            self._series_resp("8.2", "8.2.1")
        )
        mapping["https://api.github.com/repos/php/php-src/git/ref/tags/php-8.2.1"] = b(
            {"object": {}}
        )
        mapping["https://www.php.net/releases/8_2_1.php"] = self._announce_html("8.2.1")
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)

    def test_observe_propagates_oserror(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = OSError("net")
        f = FakeFetch(mapping)
        with self.assertRaises(OSError):
            uw.observe(f)

    def test_observe_invalid_json_fails(self):
        catalog = {"8": {"supported_versions": ["8.2"]}}
        mapping = {CATALOG_URL: b(catalog)}
        mapping["https://www.php.net/releases/index.php?json&version=8.2&max=1"] = b"{bad"
        f = FakeFetch(mapping)
        with self.assertRaises(ValueError):
            uw.observe(f)


class TestUpstreamWatchCompare(unittest.TestCase):
    def _mk_snap(self, series_map):
        snap = {"schema_version": 1, "series": {}}
        for sname, rec in series_map.items():
            snap["series"][sname] = dict(rec)
        # ensure sorted
        return snap

    def test_compare_unchanged(self):
        old = {
            "schema_version": 1,
            "series": {
                "8.1": {
                    "version": "8.1.30",
                    "tag": "php-8.1.30",
                    "tag_object": "a" * 40,
                    "commit": "a" * 40,
                    "sha256": "1" * 64,
                    "announcement": "https://www.php.net/releases/8_1_30.php",
                    "announcement_sha256": "2" * 64,
                    "release_tags": ["release", "security"],
                }
            },
        }
        new = json.loads(json.dumps(old))
        res = uw.compare(old, new)
        self.assertEqual(res["schema_version"], 1)
        self.assertEqual(res["status"], "unchanged")
        self.assertEqual(res["compatibility"], "not_assessed")
        self.assertEqual(res["changes"], [])

    def test_compare_release_changes_emit_events_sorted(self):
        old = {
            "schema_version": 1,
            "series": {
                "8.2": {
                    "version": "8.2.24",
                    "tag": "php-8.2.24",
                    "tag_object": "a" * 40,
                    "commit": "a" * 40,
                    "sha256": "1" * 64,
                    "announcement": "https://www.php.net/releases/8_2_24.php",
                    "announcement_sha256": "2" * 64,
                    "release_tags": ["release"],
                },
                "8.1": {
                    "version": "8.1.30",
                    "tag": "php-8.1.30",
                    "tag_object": "b" * 40,
                    "commit": "b" * 40,
                    "sha256": "3" * 64,
                    "announcement": "https://www.php.net/releases/8_1_30.php",
                    "announcement_sha256": "4" * 64,
                    "release_tags": ["release"],
                },
            },
        }
        new = json.loads(json.dumps(old))
        new["series"]["8.1"]["version"] = "8.1.31"
        new["series"]["8.1"]["tag"] = "php-8.1.31"
        new["series"]["8.1"]["announcement"] = "https://www.php.net/releases/8_1_31.php"
        new["series"]["8.1"]["tag_object"] = "c" * 40
        new["series"]["8.1"]["commit"] = "c" * 40
        new["series"]["8.1"]["sha256"] = "5" * 64
        new["series"]["8.2"]["announcement_sha256"] = "6" * 64
        new["series"]["8.2"]["release_tags"] = ["release", "security"]
        res = uw.compare(old, new)
        self.assertEqual(res["status"], "pending_review")
        changes = res["changes"]
        # sorted by series, field
        self.assertEqual(changes[0]["series"], "8.1")
        self.assertEqual(changes[1]["series"], "8.1")
        # all changed fields present
        fields = {(c["series"], c["field"]) for c in changes}
        self.assertIn(("8.1", "version"), fields)
        self.assertIn(("8.1", "tag"), fields)
        self.assertIn(("8.1", "tag_object"), fields)
        self.assertIn(("8.1", "commit"), fields)
        self.assertIn(("8.1", "sha256"), fields)
        self.assertIn(("8.2", "announcement_sha256"), fields)
        self.assertIn(("8.2", "release_tags"), fields)

    def test_compare_series_added_removed(self):
        def record(version):
            return {"version": version, "tag": "php-" + version, "tag_object": "a" * 40,
                    "commit": "b" * 40, "sha256": "c" * 64,
                    "announcement": "https://www.php.net/releases/" + version.replace(".", "_") + ".php",
                    "announcement_sha256": "d" * 64, "release_tags": ["security"]}
        old = {"schema_version": 1, "series": {"8.1": record("8.1.30")}}
        new = {"schema_version": 1, "series": {"8.2": record("8.2.25")}}
        res = uw.compare(old, new)
        self.assertEqual(res["status"], "pending_review")
        fields = [c["field"] for c in res["changes"]]
        self.assertIn("series", fields)

    def test_compare_invalid_shape_rejected(self):
        bad = {"schema_version": 2, "series": {}}
        with self.assertRaises(ValueError):
            uw.compare(bad, bad)
        bad2 = {"schema_version": 1}
        with self.assertRaises(ValueError):
            uw.compare(bad2, bad2)
        bad3 = {"schema_version": 1, "series": "x"}
        with self.assertRaises(ValueError):
            uw.compare(bad3, bad3)

    def test_compare_unrecognized_fields_rejected(self):
        old = {"schema_version": 1, "series": {"8.1": {"version": "8.1.30", "extra": 1}}}
        with self.assertRaises(ValueError):
            uw.compare(old, old)


if __name__ == "__main__":
    unittest.main()
