#!/usr/bin/env python3
import json
import unittest
from urllib.error import URLError

import tools.upstream_watch as uw


def _article(ident, title, body="<p>Body.</p>", time=None):
    """One news entry in the markup the php.net homepage serves (checked 2026-11-07)."""
    time = time or ident[:10] + "T10:00:00+00:00"
    return (
        '<article class="newsentry">\n  <header class="title">\n'
        f'    <time datetime="{time}">01 Jan 2026</time>\n'
        '    <h2 class="newstitle">\n'
        f'      <a href="https://www.php.net/archive/{ident[:4]}.php#{ident}" id="{ident}">{title}</a>\n'
        '    </h2>\n  </header>\n  <div class="newscontent">\n    <div>\n'
        f'      {body}\n    </div>\n  \n  </div>\n</article>'
    )


def _home(articles, banner=""):
    return (banner + '<section id="layout-content">\n<div class=\'home-content\'>' + "".join(articles)
            + '<p class="archive"><a href="/archive/">Older News Entries</a></p></div>\n'
            '</section><!-- layout-content -->').encode("utf-8")


# The homepage keeps the latest 25 entries, newest first.
NEWS_IDS = [f"2026-10-{day:02d}-1" for day in range(30, 5, -1)]
NEWS_ARTICLES = [_article(ident, "News " + ident) for ident in NEWS_IDS]


def _news_list():
    return uw._news(uw._content(_home(NEWS_ARTICLES)))


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
        if url == "https://www.php.net/":
            return _home(NEWS_ARTICLES)
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
        return f'<section id="layout-content">PHP {ver} announcement</section><!-- layout-content -->'.encode("utf-8")

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
        self.assertEqual(snap.get("schema_version"), 3)
        self.assertEqual([entry["id"] for entry in snap["news"]], NEWS_IDS)
        self.assertEqual(snap["news"][0]["title"], "News 2026-10-30-1")
        self.assertEqual(snap["news"][0]["time"], "2026-10-30T10:00:00+00:00")
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
                   "https://www.php.net/releases/8_3_12.php": b'<section id="layout-content">PHP 8.3.120 announcement</section><!-- layout-content -->' }
        with self.assertRaises(ValueError):
            uw.observe(FakeFetch(mapping))

    def _home_mapping(self, home):
        version = "8.3.12"
        return {CATALOG_URL: b({"8": {"supported_versions": ["8.3"]}}),
                "https://www.php.net/releases/index.php?json&version=8.3&max=1": b(self._series_resp("8.3", version)),
                "https://api.github.com/repos/php/php-src/git/ref/tags/php-8.3.12": b({"object": {"type": "commit", "sha": "c" * 40}}),
                "https://www.php.net/releases/8_3_12.php": self._announce_html(version),
                "https://www.php.net/": home}

    def _observe_home(self, home):
        return uw.observe(FakeFetch(self._home_mapping(home)))

    def test_news_content_changes_but_rotating_banner_does_not(self):
        old = self._observe_home(_home(NEWS_ARTICLES, banner="banner A"))
        same = self._observe_home(_home(NEWS_ARTICLES, banner="banner B"))
        self.assertEqual(uw.compare(old, same)["status"], "unchanged")
        spaced = _home([article + "\n" for article in NEWS_ARTICLES])
        changed = uw.compare(old, self._observe_home(spaced))
        self.assertEqual(changed["status"], "pending_review")
        self.assertEqual([entry["field"] for entry in changed["changes"]], ["news_sha256"])
        self.assertEqual((changed["news_added"], changed["news_removed"], changed["news_changed"]), ([], [], []))

    def test_news_entry_added_and_oldest_pushed_off(self):
        old = self._observe_home(_home(NEWS_ARTICLES))
        newest = _article("2026-11-05-1", "PHP 8.6.0 RC5 available for testing", time="2026-11-05T21:12:54+00:00")
        new = self._observe_home(_home([newest] + NEWS_ARTICLES[:-1]))
        self.assertEqual(len(new["news"]), 25)
        result = uw.compare(old, new)
        self.assertEqual(result["status"], "pending_review")
        self.assertEqual(result["news_added"], ["2026-11-05-1"])
        self.assertEqual(result["news_removed"], [NEWS_IDS[-1]])
        self.assertEqual(result["news_changed"], [])
        entries = {entry["id"]: entry for entry in result["changes"] if entry["field"] == "news"}
        self.assertIsNone(entries["2026-11-05-1"]["old"])
        self.assertEqual(entries["2026-11-05-1"]["new"]["title"], "PHP 8.6.0 RC5 available for testing")
        self.assertEqual(entries["2026-11-05-1"]["new"]["time"], "2026-11-05T21:12:54+00:00")
        self.assertEqual(entries[NEWS_IDS[-1]]["old"]["title"], "News " + NEWS_IDS[-1])
        self.assertIsNone(entries[NEWS_IDS[-1]]["new"])
        self.assertEqual(result["changes"][-1]["field"], "news_sha256")

    def test_news_entry_edited_in_place(self):
        old = self._observe_home(_home(NEWS_ARTICLES))
        edited = list(NEWS_ARTICLES)
        edited[3] = _article(NEWS_IDS[3], "News " + NEWS_IDS[3], body="<p>Corrected body.</p>")
        result = uw.compare(old, self._observe_home(_home(edited)))
        self.assertEqual((result["news_added"], result["news_removed"], result["news_changed"]), ([], [], [NEWS_IDS[3]]))
        change = result["changes"][0]
        self.assertEqual(change["field"], "news")
        self.assertEqual(change["old"]["title"], change["new"]["title"])
        self.assertNotEqual(change["old"]["sha256"], change["new"]["sha256"])
        retitled = list(NEWS_ARTICLES)
        retitled[0] = _article(NEWS_IDS[0], "Security fix &amp; update")
        result = uw.compare(old, self._observe_home(_home(retitled)))
        self.assertEqual(result["news_changed"], [NEWS_IDS[0]])
        self.assertEqual(result["changes"][0]["new"]["title"], "Security fix & update")

    def test_news_reorder_is_caught_by_digest(self):
        old = self._observe_home(_home(NEWS_ARTICLES))
        result = uw.compare(old, self._observe_home(_home([NEWS_ARTICLES[1], NEWS_ARTICLES[0]] + NEWS_ARTICLES[2:])))
        self.assertEqual(result["status"], "pending_review")
        self.assertEqual([entry["field"] for entry in result["changes"]], ["news_sha256"])

    def test_malformed_article_markup_fails_instead_of_returning_fewer_entries(self):
        good = NEWS_ARTICLES[0]
        variants = {
            "no articles": [],
            "item class": [good.replace('class="newsentry"', 'class="newsItem"')] + NEWS_ARTICLES[1:],
            "missing id": [good.replace(f' id="{NEWS_IDS[0]}"', "")] + NEWS_ARTICLES[1:],
            "attribute order": [good.replace(f'<a href="https://www.php.net/archive/2026.php#{NEWS_IDS[0]}" id="{NEWS_IDS[0]}">',
                                             f'<a id="{NEWS_IDS[0]}" href="https://www.php.net/archive/2026.php#{NEWS_IDS[0]}">')] + NEWS_ARTICLES[1:],
            "link mismatch": [good.replace(f"2026.php#{NEWS_IDS[0]}", f"2026.php#{NEWS_IDS[1]}")] + NEWS_ARTICLES[1:],
            "year mismatch": [good.replace("archive/2026.php", "archive/2025.php")] + NEWS_ARTICLES[1:],
            "bad id": [_article("2026-10-30", "Undated")] + NEWS_ARTICLES[1:],
            "bad time": [good.replace("2026-10-30T10:00:00+00:00", "yesterday")] + NEWS_ARTICLES[1:],
            "markup in title": [good.replace("News 2026-10-30-1", "<em>News</em>")] + NEWS_ARTICLES[1:],
            "empty title": [good.replace("News 2026-10-30-1", " ")] + NEWS_ARTICLES[1:],
            "missing time": [good.replace('<time datetime="2026-10-30T10:00:00+00:00">01 Jan 2026</time>', "")] + NEWS_ARTICLES[1:],
            "unterminated": [good[:-len("</article>")]] + NEWS_ARTICLES[1:],
            "nested": [good.replace("<p>Body.</p>", NEWS_ARTICLES[1])] + NEWS_ARTICLES[2:],
            "markup between": [good, "<hr>"] + NEWS_ARTICLES[1:],
            "stray close": [good, "</article>"] + NEWS_ARTICLES[1:],
            "duplicate id": [good, good] + NEWS_ARTICLES[1:],
            "too many": [_article(f"2026-01-01-{n}", f"News {n}") for n in range(1, uw.MAX_NEWS + 2)],
            "impossible date": [_article("2026-02-30-1", "Leap")] + NEWS_ARTICLES[1:],
            "format character in title": [good.replace("News 2026-10-30-1", "News &#x202E;1")] + NEWS_ARTICLES[1:],
        }
        for name, articles in variants.items():
            with self.subTest(name):
                with self.assertRaises(ValueError):
                    self._observe_home(_home(articles))
        # An entry in other markup before the first or after the last article,
        # or any change to the frame, must fail rather than be skipped.
        page = _home(NEWS_ARTICLES)
        frames = {
            "entry before first": page.replace(b"<div class='home-content'>",
                                               b"<div class='home-content'><div class=\"newsentry\"><h2>Security notice</h2></div>"),
            "uppercase entry after last": page.replace(b'<p class="archive">',
                                                      b'<ARTICLE class="newsentry">x</ARTICLE><p class="archive">'),
            "frame text": page.replace(b"Older News Entries", b"Older news"),
            "missing frame": page.replace(b"<div class='home-content'>", b""),
        }
        for name, home in frames.items():
            with self.subTest(name):
                with self.assertRaises(ValueError):
                    self._observe_home(home)
        with self.assertRaises(ValueError):
            self._observe_home(_home(NEWS_ARTICLES).replace(b"News 2026-10-30-1", b"News \xff"))

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
        snap = {"schema_version": 3, "news_sha256": "f" * 64, "news": _news_list(), "series": {}}
        for sname, rec in series_map.items():
            snap["series"][sname] = dict(rec)
        # ensure sorted
        return snap

    def _valid_snap(self):
        snap = self._mk_snap({"8.1": {
            "version": "8.1.30", "tag": "php-8.1.30", "tag_object": "a" * 40,
            "commit": "a" * 40, "sha256": "1" * 64,
            "announcement": "https://www.php.net/releases/8_1_30.php",
            "announcement_sha256": "2" * 64, "release_tags": ["security"]}})
        self.assertEqual(uw.compare(snap, snap)["status"], "unchanged")
        return snap

    def test_compare_unchanged(self):
        old = {
            "schema_version": 3,
            "news_sha256": "f" * 64,
            "news": _news_list(),
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
        self.assertEqual(res["schema_version"], 2)
        self.assertEqual(res["status"], "unchanged")
        self.assertEqual(res["compatibility"], "not_assessed")
        self.assertEqual(res["changes"], [])
        self.assertEqual((res["news_added"], res["news_removed"], res["news_changed"]), ([], [], []))

    def test_news_change_is_separate_candidate(self):
        old = {"schema_version": 3, "news_sha256": "f" * 64, "news": _news_list(), "series": {"8.1": {
            "version": "8.1.30", "tag": "php-8.1.30", "tag_object": "a" * 40,
            "commit": "a" * 40, "sha256": "1" * 64,
            "announcement": "https://www.php.net/releases/8_1_30.php",
            "announcement_sha256": "2" * 64, "release_tags": ["security"]}}}
        new = json.loads(json.dumps(old))
        new["news_sha256"] = "e" * 64
        result = uw.compare(old, new)
        self.assertEqual(result["status"], "pending_review")
        self.assertEqual(result["changes"][0]["field"], "news_sha256")

    def test_dynamic_banner_is_not_part_of_announcement_content(self):
        content = b'<section id="layout-content">PHP 8.1.30 release</section><!-- layout-content -->'
        self.assertEqual(uw._content(b'banner one' + content), uw._content(b'banner two' + content))
        with self.assertRaises(ValueError):
            uw._content(b'missing section')

    def test_compare_release_changes_emit_events_sorted(self):
        old = {
            "schema_version": 3,
            "news_sha256": "f" * 64,
            "news": _news_list(),
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
        old = {"schema_version": 3, "news_sha256": "f" * 64, "news": _news_list(), "series": {"8.1": record("8.1.30")}}
        new = {"schema_version": 3, "news_sha256": "f" * 64, "news": _news_list(), "series": {"8.2": record("8.2.25")}}
        res = uw.compare(old, new)
        self.assertEqual(res["status"], "pending_review")
        fields = [c["field"] for c in res["changes"]]
        self.assertIn("series", fields)

    def test_compare_invalid_shape_rejected(self):
        bad = {"schema_version": 3, "series": {}}
        with self.assertRaises(ValueError):
            uw.compare(bad, bad)
        bad2 = {"schema_version": 1}
        with self.assertRaises(ValueError):
            uw.compare(bad2, bad2)
        bad3 = {"schema_version": 1, "series": "x"}
        with self.assertRaises(ValueError):
            uw.compare(bad3, bad3)

    def test_schema_2_snapshot_is_rejected(self):
        old = self._valid_snap()
        del old["news"]
        old["schema_version"] = 2
        with self.assertRaises(ValueError):
            uw.compare(old, self._valid_snap())

    def test_invalid_news_list_rejected(self):
        entry = _news_list()[0]
        variants = {
            "empty": [],
            "not a list": {"id": entry["id"]},
            "duplicate id": [entry, dict(entry)],
            "extra field": [dict(entry, body="x")],
            "missing field": [{key: value for key, value in entry.items() if key != "time"}],
            "bad id": [dict(entry, id="../2026")],
            "bad time": [dict(entry, time="2026-11-05")],
            "bad digest": [dict(entry, sha256="F" * 64)],
            "padded title": [dict(entry, title=" News")],
            "control title": [dict(entry, title="News\nline")],
            "c1 control title": [dict(entry, title="News\x85line")],
            "bidi title": [dict(entry, title="News\u202eline")],
            "impossible id date": [dict(entry, id="2026-99-99-1")],
            "impossible time": [dict(entry, time="2026-11-05T25:00:00+00:00")],
            "too many": [dict(entry, id=f"2026-01-01-{n}") for n in range(1, uw.MAX_NEWS + 2)],
        }
        for name, news in variants.items():
            with self.subTest(name):
                snap = self._valid_snap()
                snap["news"] = news
                with self.assertRaises(ValueError):
                    uw.compare(snap, self._valid_snap())

    def test_compare_unrecognized_fields_rejected(self):
        record = {"version": "8.1.30", "tag": "php-8.1.30", "tag_object": "a" * 40,
                  "commit": "a" * 40, "sha256": "1" * 64,
                  "announcement": "https://www.php.net/releases/8_1_30.php",
                  "announcement_sha256": "2" * 64, "release_tags": ["security"], "extra": 1}
        old = {"schema_version": 3, "news_sha256": "f" * 64, "news": _news_list(), "series": {"8.1": record}}
        with self.assertRaises(ValueError):
            uw.compare(old, old)


if __name__ == "__main__":
    unittest.main()
