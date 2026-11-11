import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock

import tools.upstream_diff as upstream_diff
import tools.upstream_snapshot as upstream_snapshot
import tools.upstream_update as upstream_update
import tools.upstream_watch as upstream_watch


ROOT = Path(__file__).resolve().parents[1]
OLD_VERSION = "8.5.11"
NEW_VERSION = "8.5.12"
PARSER = "Zend/zend_language_parser.y"
SCANNER = "Zend/zend_language_scanner.l"
STUB = "ext/standard/example.stub.php"
CHANGED_TEST = "Zend/tests/update.phpt"
REMOVED_TEST = "Zend/tests/removed.phpt"
ADDED_TEST = "Zend/tests/added.phpt"
IMPLEMENTATION = "ext/standard/example.c"
UNKNOWN = "build/notes.txt"


def _release(version, archive):
    return {
        "version": version,
        "tag": "php-" + version,
        "commit": hashlib.sha1(version.encode("ascii")).hexdigest(),
        "archive_sha256": hashlib.sha256(archive).hexdigest(),
        "announcement": "https://www.php.net/releases/" + version.replace(".", "_") + ".php",
    }


def _tar(entries, root=None):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w") as archive:
        for path, data in entries:
            info = tarfile.TarInfo((root or "php-" + NEW_VERSION) + "/" + path)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
    return output.getvalue()


def _required_sources(version, *, stub=None, news=None):
    return {
        PARSER: b"%token T_STRING\n%%\nprogram: expression ;\n%%\n",
        SCANNER: b"%%\n[a-z]+ { return T_STRING; }\n%%\n",
        STUB: stub or b"<?php\nnamespace Example;\nfunction stable(int $value): int;\n",
        "NEWS": news or (
            f"1 Nov 2026, PHP {version}\n\n- Core:\n. Updated engine behavior.\n"
        ).encode("ascii"),
        CHANGED_TEST: b"--TEST--\nold output \xff\n",
    }


def _archive_pair(old_files=None, new_files=None):
    old_entries = _required_sources(OLD_VERSION)
    if old_files:
        old_entries.update(old_files)
    new_entries = _required_sources(NEW_VERSION)
    if new_files:
        new_entries.update(new_files)
    old = _tar(sorted(old_entries.items()), "php-" + OLD_VERSION)
    new = _tar(sorted(new_entries.items()), "php-" + NEW_VERSION)
    return old, new, _release(OLD_VERSION, old), _release(NEW_VERSION, new)


def _watch_record(version, digest, *, suffix="1", announcement_sha="e"):
    return {
        "version": version,
        "tag": "php-" + version,
        "tag_object": suffix * 40,
        "commit": hashlib.sha1(version.encode("ascii")).hexdigest(),
        "sha256": digest,
        "announcement": "https://www.php.net/releases/" + version.replace(".", "_") + ".php",
        "announcement_sha256": announcement_sha * 64,
        "release_tags": ["release"],
    }


def _snapshot(series, *, news_sha="f"):
    return {
        "schema_version": 3,
        "news_sha256": news_sha * 64,
        "news": [{"id": "2026-11-05-1", "time": "2026-11-05T21:12:54+00:00",
                  "title": "PHP 8.6.0 RC5 available for testing", "sha256": "d" * 64}],
        "series": series,
    }


def _pair_archives():
    before = _required_sources(OLD_VERSION)
    after = _required_sources(
        NEW_VERSION,
        stub=(
            b"<?php\nnamespace Example;\n"
            b"function stable(string $value): string;\n"
            b"function added(bool $enabled): bool;\n"
        ),
        news=(
            f"1 Nov 2026, PHP {NEW_VERSION}\n\n"
            "- Core:\n. Fixed CVE-2026-12345 in execution state.\n"
        ).encode("ascii"),
    )
    before.update({
        REMOVED_TEST: b"--TEST--\nremoved\n",
        IMPLEMENTATION: b"int old_behavior(void) { return 1; }\n",
        UNKNOWN: b"old note\n",
    })
    after.update({
        PARSER: b"%token T_STRING T_NAME\n%%\nprogram: expression | T_NAME ;\n%%\n",
        SCANNER: b"%%\n[a-z]+ { return T_STRING; }\n[0-9]+ { return T_NAME; }\n%%\n",
        CHANGED_TEST: b"--TEST--\nnew output \xfe\n",
        ADDED_TEST: b"--TEST--\nadded \xff\n",
        IMPLEMENTATION: b"int new_behavior(void) { return 2; }\n",
        UNKNOWN: b"new note\n",
    })
    return _archive_pair(before, after)


class TestUpstreamUpdateClassify(unittest.TestCase):
    def test_t129_reports_additions_removals_modifications_deterministically(self):
        old, new, old_release, new_release = _pair_archives()

        first = upstream_update.classify(old, new, old_release, new_release)
        second = upstream_update.classify(old, new, old_release, new_release)

        self.assertEqual(first, second)
        self.assertEqual(first["schema_version"], 1)
        self.assertEqual(first["old"], upstream_snapshot._release(old_release))
        self.assertEqual(first["new"], upstream_snapshot._release(new_release))
        self.assertEqual(first["source_verification"], "sha256_verified")
        self.assertEqual(first["compatibility"], "not_assessed")
        self.assertEqual(
            first["classification"],
            upstream_diff.diff_releases(
                upstream_diff.read_archive(old, OLD_VERSION),
                upstream_diff.read_archive(new, NEW_VERSION),
                NEW_VERSION,
            ),
        )

        files = {item["path"]: item for item in first["files"]}
        self.assertTrue(all(
            set(item) == {
                "path", "category", "kind", "old_sha256", "new_sha256",
                "work_packages", "review",
            }
            for item in first["files"]
        ))
        self.assertTrue(all(
            set(item) == {
                "path", "sha256", "release", "archive_sha256",
                "work_packages", "status",
            }
            for item in first["regression_candidates"]
        ))
        self.assertEqual(files[PARSER]["kind"], "modified")
        self.assertEqual(files[PARSER]["category"], "parser")
        self.assertEqual(files[PARSER]["review"], "needs_behavior_regression")
        self.assertEqual(files[SCANNER]["kind"], "modified")
        self.assertEqual(files[SCANNER]["category"], "parser")
        self.assertEqual(files[SCANNER]["review"], "needs_behavior_regression")
        self.assertEqual(files[STUB]["kind"], "modified")
        self.assertEqual(files[STUB]["review"], "needs_behavior_regression")
        self.assertEqual(files[REMOVED_TEST]["kind"], "removed")
        self.assertEqual(files[ADDED_TEST]["kind"], "added")
        self.assertEqual(files[UNKNOWN]["category"], "unclassified")
        self.assertEqual(files[UNKNOWN]["review"], "needs_behavior_regression")
        self.assertEqual(files[IMPLEMENTATION]["category"], "implementation")
        self.assertEqual(files[IMPLEMENTATION]["review"], "needs_behavior_regression")
        self.assertEqual(files["NEWS"]["review"], "needs_assessment")
        self.assertEqual(files[CHANGED_TEST]["review"], "pending")
        self.assertEqual(files[CHANGED_TEST]["kind"], "modified")
        self.assertIsNone(files[ADDED_TEST]["old_sha256"])
        self.assertIsNone(files[REMOVED_TEST]["new_sha256"])
        self.assertEqual(
            first["affected_work_packages"],
            sorted(
                set(first["classification"]["affected_work_packages"])
                | {
                    package
                    for item in first["files"]
                    for package in upstream_diff.impact(item["path"])
                }
            ),
        )

    def test_t130_c_implementation_change_stays_unassessed_with_identical_stub(self):
        old, new, old_release, new_release = _archive_pair(
            {IMPLEMENTATION: b"int run(void) { return 1; }\n"},
            {IMPLEMENTATION: b"int run(void) { return 0; }\n"},
        )

        report = upstream_update.classify(old, new, old_release, new_release)
        implementation = next(item for item in report["files"] if item["path"] == IMPLEMENTATION)

        self.assertEqual(report["compatibility"], "not_assessed")
        self.assertEqual(implementation["review"], "needs_behavior_regression")
        self.assertNotEqual(implementation["old_sha256"], implementation["new_sha256"])
        self.assertNotIn(STUB, {item["path"] for item in report["files"]})
        self.assertEqual(report["classification"]["stubs"]["changed"], [])

    def test_raw_stub_alias_change_is_not_hidden_by_equal_parsed_signatures(self):
        old_stub = (
            b"<?php\nnamespace Example;\nuse Vendor\\OldName as Alias;\n"
            b"function stable(int $value): int;\n"
        )
        new_stub = (
            b"<?php\nnamespace Example;\nuse Vendor\\NewName as Alias;\n"
            b"function stable(int $value): int;\n"
        )
        old, new, old_release, new_release = _archive_pair(
            {STUB: old_stub}, {STUB: new_stub}
        )

        report = upstream_update.classify(old, new, old_release, new_release)
        stub = next(item for item in report["files"] if item["path"] == STUB)

        self.assertEqual(stub["kind"], "modified")
        self.assertEqual(stub["old_sha256"], hashlib.sha256(old_stub).hexdigest())
        self.assertEqual(stub["new_sha256"], hashlib.sha256(new_stub).hexdigest())
        self.assertEqual(stub["review"], "needs_behavior_regression")

    def test_phpt_candidate_is_bound_to_exact_file_and_new_archive(self):
        old, new, old_release, new_release = _pair_archives()
        report = upstream_update.classify(old, new, old_release, new_release)
        candidates = {item["path"]: item for item in report["regression_candidates"]}
        candidate = candidates[CHANGED_TEST]
        changed = next(item for item in report["files"] if item["path"] == CHANGED_TEST)

        self.assertEqual(candidate["sha256"], changed["new_sha256"])
        self.assertEqual(candidate["sha256"], hashlib.sha256(b"--TEST--\nnew output \xfe\n").hexdigest())
        self.assertEqual(candidate["release"], NEW_VERSION)
        self.assertEqual(candidate["archive_sha256"], new_release["archive_sha256"])
        self.assertEqual(candidate["status"], "pending")
        self.assertEqual(candidate["work_packages"], upstream_diff.impact(CHANGED_TEST))
        self.assertIn(ADDED_TEST, candidates)

    def test_implementation_extensions_are_classified_and_unknown_files_retained(self):
        old, new, old_release, new_release = _archive_pair(
            {"src/engine.c": b"old\n", "src/api.h": b"old\n",
             "src/engine.inc": b"old\n", "src/assembly.S": b"old\n",
             "src/native.cpp": b"old\n", UNKNOWN: b"old"},
            {"src/engine.c": b"new\n", "src/api.h": b"new\n",
             "src/engine.inc": b"new\n", "src/assembly.S": b"new\n",
             "src/native.cpp": b"new\n", UNKNOWN: b"new"},
        )

        report = upstream_update.classify(old, new, old_release, new_release)
        files = {item["path"]: item for item in report["files"]}

        for path in (
            "src/engine.c", "src/api.h", "src/engine.inc",
            "src/assembly.S", "src/native.cpp",
        ):
            self.assertEqual(files[path]["category"], "implementation")
            self.assertEqual(files[path]["review"], "needs_behavior_regression")
        self.assertIn(UNKNOWN, files)
        self.assertEqual(files[UNKNOWN]["category"], "unclassified")

    def test_rejects_empty_archive_and_each_missing_required_source(self):
        empty = _tar([], "php-" + OLD_VERSION)
        old_release = _release(OLD_VERSION, empty)
        new, _, new_release = _archive_pair()[1:]
        with self.assertRaises(ValueError):
            upstream_update.classify(empty, new, old_release, new_release)

        required = (PARSER, SCANNER, STUB, "NEWS", CHANGED_TEST)
        complete = _required_sources(OLD_VERSION)
        for path in required:
            with self.subTest(missing=path):
                entries = {name: data for name, data in complete.items() if name != path}
                archive = _tar(sorted(entries.items()), "php-" + OLD_VERSION)
                archive_release = _release(OLD_VERSION, archive)
                with self.assertRaises(ValueError):
                    upstream_update.classify(archive, new, archive_release, new_release)

    def test_rejects_wrong_root_and_unsafe_paths(self):
        valid_new, _, new_release = _archive_pair()[1:]
        wrong_root = _tar(
            sorted(_required_sources(OLD_VERSION).items()), "php-8.5.10"
        )
        with self.assertRaises(ValueError):
            upstream_update.classify(
                wrong_root, valid_new, _release(OLD_VERSION, wrong_root), new_release
            )

        unsafe = _tar(
            [("../escape", b"no"), *sorted(_required_sources(OLD_VERSION).items())],
            "php-" + OLD_VERSION,
        )
        with self.assertRaises(ValueError):
            upstream_update.classify(
                unsafe, valid_new, _release(OLD_VERSION, unsafe), new_release
            )

        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w") as archive:
            for path, data in sorted(_required_sources(OLD_VERSION).items()):
                info = tarfile.TarInfo("php-" + OLD_VERSION + "/" + path)
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            link = tarfile.TarInfo("php-" + OLD_VERSION + "/linked.phpt")
            link.type = tarfile.SYMTYPE
            link.linkname = "../outside"
            archive.addfile(link)
        symlink_archive = output.getvalue()
        with self.assertRaises(ValueError):
            upstream_update.classify(
                symlink_archive,
                valid_new,
                _release(OLD_VERSION, symlink_archive),
                new_release,
            )

    def test_digest_mismatch_is_rejected_before_archive_decoding(self):
        old_data = b"not a tar archive"
        new, _, new_release = _archive_pair()[1:]
        incorrect_lock = _release(OLD_VERSION, b"some other archive")

        with self.assertRaisesRegex(ValueError, "sha256"):
            upstream_update.classify(old_data, new, incorrect_lock, new_release)

    def test_archive_resource_limit_is_enforced(self):
        oversized = b"x" * 2048
        old_entries = _required_sources(OLD_VERSION)
        old_entries["ignored.bin"] = oversized
        old = _tar(sorted(old_entries.items()), "php-" + OLD_VERSION)
        new, _, new_release = _archive_pair()[1:]

        with mock.patch.object(upstream_diff, "MAX_UNPACKED", 1024):
            with self.assertRaises(ValueError):
                upstream_update.classify(old, new, _release(OLD_VERSION, old), new_release)

    def test_release_identity_must_match_archive_root_and_lock(self):
        old, new, old_release, new_release = _archive_pair()
        wrong_lock = dict(old_release, version="8.5.10", tag="php-8.5.10")

        with self.assertRaises(ValueError):
            upstream_update.classify(old, new, wrong_lock, new_release)


class TestUpstreamUpdateObserveChanges(unittest.TestCase):
    def test_unchanged_snapshots_do_not_load_archives(self):
        record = _watch_record(OLD_VERSION, "a" * 64)
        snapshot = _snapshot({"8.5": record})
        calls = []

        result = upstream_update.observe_changes(snapshot, snapshot, calls.append)

        self.assertEqual(result["schema_version"], 1)
        self.assertEqual(result["metadata"], upstream_watch.compare(snapshot, snapshot))
        self.assertEqual(result["metadata"]["status"], "unchanged")
        self.assertEqual(result["compatibility"], "not_assessed")
        self.assertEqual(result["comparisons"], [])
        self.assertEqual(calls, [])
        self.assertEqual(result["status"], "unchanged")

    def test_metadata_only_changes_are_visible_without_archive_loads(self):
        old = _snapshot({"8.5": _watch_record(OLD_VERSION, "a" * 64)})
        new = _snapshot({
            "8.5": _watch_record(OLD_VERSION, "a" * 64, announcement_sha="b")
        })
        calls = []

        result = upstream_update.observe_changes(old, new, calls.append)

        self.assertEqual(calls, [])
        self.assertEqual(result["metadata"]["status"], "pending_review")
        self.assertIn(
            {"series": "8.5", "field": "announcement_sha256",
             "old": "e" * 64, "new": "b" * 64},
            result["metadata"]["changes"],
        )
        self.assertEqual(result["comparisons"], [])
        self.assertEqual(result["status"], "pending_review")
        self.assertEqual(result["compatibility"], "not_assessed")

    def test_news_only_change_reports_entries_without_archive_loads(self):
        records = {"8.5": _watch_record(OLD_VERSION, "a" * 64)}
        old = _snapshot(records)
        new = _snapshot(records, news_sha="b")
        new["news"] = [{"id": "2026-11-06-1", "time": "2026-11-06T10:00:00+00:00",
                        "title": "PHP 8.6.0 RC6 available for testing", "sha256": "c" * 64}] + old["news"]
        calls = []

        result = upstream_update.observe_changes(old, new, calls.append)

        self.assertEqual(calls, [])
        self.assertEqual(result["comparisons"], [])
        self.assertEqual(result["metadata"]["status"], "pending_review")
        self.assertEqual(result["metadata"]["news_added"], ["2026-11-06-1"])
        self.assertEqual(result["metadata"]["news_removed"], [])
        self.assertEqual(result["status"], "pending_review")

    def test_changed_release_loads_exact_old_and_new_identities(self):
        old_archive, new_archive, old_release, new_release = _pair_archives()
        old = _snapshot({
            "8.5": _watch_record(OLD_VERSION, old_release["archive_sha256"])
        })
        new = _snapshot({
            "8.5": _watch_record(NEW_VERSION, new_release["archive_sha256"], suffix="b")
        })
        archives = {OLD_VERSION: old_archive, NEW_VERSION: new_archive}
        calls = []

        def load_archive(release):
            calls.append(release)
            return archives[release["version"]]

        result = upstream_update.observe_changes(old, new, load_archive)

        self.assertEqual(
            calls,
            [
                upstream_snapshot._release(old_release),
                upstream_snapshot._release(new_release),
            ],
        )
        self.assertEqual(result["comparisons"][0]["series"], "8.5")
        self.assertEqual(result["comparisons"][0]["status"], "classified")
        self.assertEqual(result["comparisons"][0]["report"]["compatibility"], "not_assessed")
        self.assertEqual(result["status"], "pending_review")

    def test_archive_digest_change_loads_each_exact_digest_release(self):
        old_entries = _required_sources(OLD_VERSION)
        old_entries[IMPLEMENTATION] = b"int run(void) { return 1; }\n"
        new_entries = dict(old_entries)
        new_entries[IMPLEMENTATION] = b"int run(void) { return 2; }\n"
        old_archive = _tar(sorted(old_entries.items()), "php-" + OLD_VERSION)
        new_archive = _tar(sorted(new_entries.items()), "php-" + OLD_VERSION)
        old_release = _release(OLD_VERSION, old_archive)
        new_release = _release(OLD_VERSION, new_archive)
        old = _snapshot({
            "8.5": _watch_record(OLD_VERSION, old_release["archive_sha256"])
        })
        new = _snapshot({
            "8.5": _watch_record(OLD_VERSION, new_release["archive_sha256"], suffix="b")
        })
        calls = []

        def load_archive(release):
            calls.append(release)
            return old_archive if release["archive_sha256"] == old_release["archive_sha256"] else new_archive

        result = upstream_update.observe_changes(old, new, load_archive)

        self.assertEqual(
            calls,
            [
                upstream_snapshot._release(old_release),
                upstream_snapshot._release(new_release),
            ],
        )
        self.assertEqual(result["comparisons"][0]["status"], "classified")

    def test_loader_exception_blocks_one_series_without_discarding_another(self):
        old_archive_85, new_archive_85, old_release_85, new_release_85 = _pair_archives()
        old_84_entries = _required_sources("8.4.33")
        old_84_entries[IMPLEMENTATION] = b"int run(void) { return 1; }\n"
        new_84_entries = _required_sources("8.4.34")
        new_84_entries[IMPLEMENTATION] = b"int run(void) { return 2; }\n"
        old_archive_84 = _tar(
            sorted(old_84_entries.items()), "php-8.4.33"
        )
        new_archive_84 = _tar(
            sorted(new_84_entries.items()), "php-8.4.34"
        )
        old_release_84 = _release("8.4.33", old_archive_84)
        new_release_84 = _release("8.4.34", new_archive_84)
        old_records = {
            "8.4": _watch_record("8.4.33", old_release_84["archive_sha256"], suffix="c"),
            "8.5": _watch_record(OLD_VERSION, old_release_85["archive_sha256"]),
        }
        new_records = {
            "8.4": _watch_record("8.4.34", new_release_84["archive_sha256"], suffix="d"),
            "8.5": _watch_record(NEW_VERSION, new_release_85["archive_sha256"], suffix="b"),
        }
        old = _snapshot(old_records)
        new = _snapshot(new_records, news_sha="a")
        calls = []

        def load_archive(release):
            calls.append(release)
            if release["version"] == "8.4.34":
                raise OSError("offline fixture unavailable")
            if release["version"] == "8.4.33":
                return old_archive_84
            return old_archive_85 if release["version"] == OLD_VERSION else new_archive_85

        result = upstream_update.observe_changes(old, new, load_archive)

        comparisons = {item["series"]: item for item in result["comparisons"]}
        self.assertEqual(comparisons["8.4"]["status"], "blocked")
        self.assertIn("offline fixture unavailable", comparisons["8.4"]["error"])
        self.assertEqual(comparisons["8.5"]["status"], "classified")
        self.assertEqual(result["metadata"]["status"], "pending_review")
        self.assertEqual(result["status"], "blocked")
        self.assertEqual(len(calls), 4)

    def test_added_and_removed_series_are_blocked_without_inventing_pairs(self):
        old = _snapshot({
            "8.4": _watch_record("8.4.33", "c" * 64, suffix="c"),
            "8.5": _watch_record(OLD_VERSION, "a" * 64),
        })
        new = _snapshot({
            "8.5": _watch_record(OLD_VERSION, "a" * 64),
            "8.6": _watch_record("8.6.0", "b" * 64, suffix="b"),
        })
        calls = []

        result = upstream_update.observe_changes(old, new, calls.append)

        comparisons = {item["series"]: item for item in result["comparisons"]}
        self.assertEqual(comparisons["8.4"]["status"], "blocked")
        self.assertEqual(comparisons["8.6"]["status"], "blocked")
        self.assertEqual(result["status"], "blocked")
        self.assertEqual(calls, [])


class TestUpstreamUpdateCLI(unittest.TestCase):
    def _invoke_pair(self, directory, old, new, lock_bytes):
        old_path = Path(directory) / "old.tar"
        new_path = Path(directory) / "new.tar"
        lock_path = Path(directory) / "lock.json"
        old_path.write_bytes(old)
        new_path.write_bytes(new)
        lock_path.write_bytes(lock_bytes)
        return subprocess.run(
            [
                sys.executable,
                "-m",
                "tools.upstream_update",
                "pair",
                "--lock",
                str(lock_path),
                str(old_path),
                str(new_path),
            ],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_pair_command_emits_json_for_two_exact_locked_archives(self):
        old, new, old_release, new_release = _pair_archives()
        lock = json.dumps({"old": old_release, "new": new_release}).encode("utf-8")

        with tempfile.TemporaryDirectory() as directory:
            result = self._invoke_pair(directory, old, new, lock)

        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["source_verification"], "sha256_verified")
        self.assertEqual(report["compatibility"], "not_assessed")

    def test_pair_command_rejects_wrong_hash_without_a_json_report(self):
        old, new, old_release, new_release = _archive_pair()
        wrong_lock = dict(old_release, archive_sha256="0" * 64)
        lock = json.dumps({"old": wrong_lock, "new": new_release}).encode("utf-8")

        with tempfile.TemporaryDirectory() as directory:
            result = self._invoke_pair(directory, old, new, lock)

        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout.strip(), "")

    def test_pair_command_rejects_duplicate_or_malformed_lock_without_report(self):
        old, new, _, _ = _archive_pair()
        malformed_locks = (
            b'{"old": {}, "old": {}, "new": {}}',
            b'{"old": {}, "new": []}',
        )
        for lock in malformed_locks:
            with self.subTest(lock=lock):
                with tempfile.TemporaryDirectory() as directory:
                    result = self._invoke_pair(directory, old, new, lock)
                self.assertEqual(result.returncode, 2)
                self.assertEqual(result.stdout.strip(), "")


if __name__ == "__main__":
    unittest.main()
