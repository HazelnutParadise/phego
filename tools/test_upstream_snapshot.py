#!/usr/bin/env python3
import hashlib
import io
import json
import tarfile
import tempfile
import unittest

import tools.upstream_snapshot as upstream_snapshot


class TestUpstreamSnapshot(unittest.TestCase):
    VERSION = "8.2.0"
    TAG = "php-8.2.0"
    COMMIT = "a" * 40
    ANNOUNCEMENT = "https://www.php.net/releases/8_2_0.php"
    ROOT = "php-8.2.0"

    def release_meta(self, **overrides):
        meta = {
            "version": self.VERSION,
            "tag": self.TAG,
            "commit": self.COMMIT,
            "announcement": self.ANNOUNCEMENT,
        }
        meta.update(overrides)
        return meta

    def regular(self, name, data, mtime=0):
        info = tarfile.TarInfo(name=name)
        info.size = len(data)
        info.mtime = mtime
        return {"info": info, "data": data}

    def directory(self, name):
        info = tarfile.TarInfo(name=name)
        info.type = tarfile.DIRTYPE
        info.mtime = 0
        return {"info": info, "data": b""}

    def symlink(self, name, target):
        info = tarfile.TarInfo(name=name)
        info.type = tarfile.SYMTYPE
        info.linkname = target
        info.mtime = 0
        return {"info": info, "data": b""}

    def hardlink(self, name, target):
        info = tarfile.TarInfo(name=name)
        info.type = tarfile.LNKTYPE
        info.linkname = target
        info.mtime = 0
        return {"info": info, "data": b""}

    def fifo(self, name):
        info = tarfile.TarInfo(name=name)
        info.type = tarfile.FIFOTYPE
        info.mtime = 0
        return {"info": info, "data": b""}

    def make_tar(self, members):
        with tempfile.TemporaryFile() as handle:
            with tarfile.open(fileobj=handle, mode="w:gz") as archive:
                for member in members:
                    info = member["info"]
                    payload = io.BytesIO(member["data"]) if info.isreg() else None
                    archive.addfile(info, payload)
            handle.seek(0)
            return handle.read()

    def standard_members(self, root=None, prefix=""):
        root = root or self.ROOT
        return [
            self.regular(f"{root}/Zend/zend_language_parser.y", (prefix + "parser\n").encode()),
            self.regular(f"{root}/Zend/zend_language_scanner.l", (prefix + "scanner\n").encode()),
            self.regular(f"{root}/ext/foo/foo.stub.php", (prefix + "stub\n").encode()),
            self.regular(f"{root}/tests/basic/001.phpt", (prefix + "test\n").encode()),
            self.regular(f"{root}/NEWS", (prefix + "news\n").encode()),
        ]

    def archive(self, members):
        data = self.make_tar(members)
        return data, hashlib.sha256(data).hexdigest()

    def capture(self, archive, sha, meta=None):
        meta = meta or self.release_meta()
        return upstream_snapshot.capture(
            archive, sha, meta["version"], meta["tag"], meta["commit"], meta["announcement"]
        )

    def capture_valid(self, members=None, meta=None):
        meta = meta or self.release_meta()
        archive, sha = self.archive(members or self.standard_members())
        return self.capture(archive, sha, meta), meta

    def clone(self, snapshot):
        return json.loads(json.dumps(snapshot))

    def test_capture_signature_and_basic_shape(self):
        archive, sha = self.archive(self.standard_members())
        result = self.capture(archive, sha)
        self.assertIsInstance(result, dict)
        self.assertEqual(result.get("schema_version"), 1)
        self.assertIn("release", result)
        self.assertIn("files", result)
        release = result["release"]
        self.assertEqual(release["version"], self.VERSION)
        self.assertEqual(release["tag"], self.TAG)
        self.assertEqual(release["commit"], self.COMMIT)
        self.assertEqual(release["archive_sha256"], sha)
        self.assertEqual(release["announcement"], self.ANNOUNCEMENT)
        self.assertIsInstance(result["files"], dict)
        self.assertTrue(result["files"])

    def test_capture_categories(self):
        archive, sha = self.archive(self.standard_members())
        files = self.capture(archive, sha)["files"]
        expected = {
            "Zend/zend_language_parser.y": "parser",
            "Zend/zend_language_scanner.l": "parser",
            "ext/foo/foo.stub.php": "stub",
            "tests/basic/001.phpt": "test",
            "NEWS": "release_note",
        }
        self.assertEqual(set(files), set(expected))
        for path, category in expected.items():
            self.assertEqual(files[path]["category"], category)
            self.assertRegex(files[path]["sha256"], r"^[0-9a-f]{64}$")

    def test_capture_file_digests_match_content(self):
        contents = {
            "Zend/zend_language_parser.y": b"%token T_FOO\n",
            "Zend/zend_language_scanner.l": b"%%\n",
            "ext/foo/foo.stub.php": b"function foo();\n",
            "tests/basic/001.phpt": b"--TEST--\ntest\n",
            "NEWS": b"PHP 8.2.0\n",
        }
        members = [self.regular(f"{self.ROOT}/{path}", data) for path, data in contents.items()]
        archive, sha = self.archive(members)
        result = self.capture(archive, sha)
        self.assertEqual(result["release"]["archive_sha256"], sha)
        for path, data in contents.items():
            self.assertEqual(result["files"][path]["sha256"], hashlib.sha256(data).hexdigest())

    def test_capture_accepts_either_parser_file(self):
        for name in ("zend_language_parser.y", "zend_language_scanner.l"):
            with self.subTest(name=name):
                members = [
                    member
                    for member in self.standard_members()
                    if "Zend/zend_language_" not in member["info"].name
                ]
                members.append(self.regular(f"{self.ROOT}/Zend/{name}", b"parser content\n"))
                archive, sha = self.archive(members)
                result = self.capture(archive, sha)
                self.assertEqual(result["files"][f"Zend/{name}"]["category"], "parser")

    def test_capture_accepts_either_release_note_file(self):
        for name in ("NEWS", "UPGRADING"):
            with self.subTest(name=name):
                members = [
                    member
                    for member in self.standard_members()
                    if member["info"].name != f"{self.ROOT}/NEWS"
                ]
                members.append(self.regular(f"{self.ROOT}/{name}", b"release notes\n"))
                archive, sha = self.archive(members)
                result = self.capture(archive, sha)
                self.assertEqual(result["files"][name]["category"], "release_note")

    def test_capture_rejects_bad_tag(self):
        archive, sha = self.archive(self.standard_members())
        for version, tag in (
            (self.VERSION, "v8.2.0"),
            (self.VERSION, "php-8.3.0"),
            (self.VERSION, "php-8.2.0-extra"),
        ):
            with self.subTest(tag=tag):
                with self.assertRaises(ValueError):
                    self.capture(archive, sha, self.release_meta(version=version, tag=tag))

    def test_capture_rejects_bad_commit(self):
        archive, sha = self.archive(self.standard_members())
        for commit in ("abc123", "g" * 40, "a" * 39, "a" * 41):
            with self.subTest(commit=commit):
                with self.assertRaises(ValueError):
                    self.capture(archive, sha, self.release_meta(commit=commit))

    def test_capture_rejects_bad_announcement(self):
        archive, sha = self.archive(self.standard_members())
        for announcement in (
            "http://www.php.net/releases/8_2_0.php",
            "ftp://www.php.net/releases/8_2_0.php",
            "https://example.com/releases/8_2_0.php",
            "https://www.php.net.evil.example/releases/8_2_0.php",
        ):
            with self.subTest(announcement=announcement):
                with self.assertRaises(ValueError):
                    self.capture(archive, sha, self.release_meta(announcement=announcement))

    def test_capture_rejects_bad_expected_sha(self):
        archive, _ = self.archive(self.standard_members())
        for expected in ("abc123", "g" * 64, "0" * 63, "0" * 65):
            with self.subTest(expected=expected):
                with self.assertRaises(ValueError):
                    self.capture(archive, expected)

    def test_capture_verifies_archive_sha256(self):
        archive, expected_sha = self.archive(self.standard_members())
        bad = bytearray(archive)
        bad[0] ^= 0xFF
        with self.assertRaises(ValueError):
            self.capture(bytes(bad), expected_sha)

    def test_capture_rejects_absolute_paths(self):
        members = self.standard_members()
        members[0] = self.regular(f"/{self.ROOT}/Zend/zend_language_parser.y", b"parser\n")
        archive, sha = self.archive(members)
        with self.assertRaises(ValueError):
            self.capture(archive, sha)

    def test_capture_rejects_dotdot(self):
        for name in (
            f"{self.ROOT}/../Zend/zend_language_parser.y",
            f"{self.ROOT}/Zend/../Zend/zend_language_parser.y",
        ):
            with self.subTest(name=name):
                members = self.standard_members()
                members[0] = self.regular(name, b"parser\n")
                archive, sha = self.archive(members)
                with self.assertRaises(ValueError):
                    self.capture(archive, sha)

    def test_capture_rejects_duplicate_paths(self):
        members = self.standard_members()
        members.append(self.regular(f"{self.ROOT}/Zend/zend_language_parser.y", b"duplicate\n"))
        archive, sha = self.archive(members)
        with self.assertRaises(ValueError):
            self.capture(archive, sha)

    def test_capture_rejects_links_and_nonregular_files(self):
        parser_path = f"{self.ROOT}/Zend/zend_language_parser.y"
        cases = {
            "symlink": self.symlink(parser_path, "zend_language_scanner.l"),
            "hardlink": self.hardlink(parser_path, f"{self.ROOT}/Zend/zend_language_scanner.l"),
            "fifo": self.fifo(parser_path),
        }
        for label, replacement in cases.items():
            with self.subTest(kind=label):
                members = [
                    member for member in self.standard_members() if member["info"].name != parser_path
                ]
                members.append(replacement)
                archive, sha = self.archive(members)
                with self.assertRaises(ValueError):
                    self.capture(archive, sha)

    def test_capture_rejects_wrong_root(self):
        for wrong_root in ("php-8.1.0", "php-8.2.1"):
            with self.subTest(root=wrong_root):
                archive, sha = self.archive(self.standard_members(root=wrong_root))
                with self.assertRaises(ValueError):
                    self.capture(archive, sha)
        mixed = self.standard_members()
        mixed[0] = self.regular("php-8.3.0/Zend/zend_language_parser.y", b"parser\n")
        archive, sha = self.archive(mixed)
        with self.assertRaises(ValueError):
            self.capture(archive, sha)

    def test_capture_rejects_missing_category(self):
        def relative(name):
            return name[len(self.ROOT) + 1:]

        matchers = {
            "parser": lambda name: relative(name).startswith("Zend/zend_language_"),
            "stub": lambda name: relative(name).endswith(".stub.php"),
            "test": lambda name: relative(name).endswith(".phpt"),
            "release_note": lambda name: relative(name) == "NEWS" or relative(name).endswith("/NEWS"),
        }
        for category, matches in matchers.items():
            with self.subTest(category=category):
                members = [member for member in self.standard_members() if not matches(member["info"].name)]
                archive, sha = self.archive(members)
                with self.assertRaises(ValueError):
                    self.capture(archive, sha)

    def test_capture_allows_directories(self):
        members = [
            self.directory(f"{self.ROOT}/"),
            self.directory(f"{self.ROOT}/Zend/"),
            self.directory(f"{self.ROOT}/ext/"),
            *self.standard_members(),
        ]
        archive, sha = self.archive(members)
        files = self.capture(archive, sha)["files"]
        self.assertEqual(
            set(files),
            {
                "Zend/zend_language_parser.y",
                "Zend/zend_language_scanner.l",
                "ext/foo/foo.stub.php",
                "tests/basic/001.phpt",
                "NEWS",
            },
        )

    def test_capture_is_independent_of_tar_order(self):
        members = self.standard_members()
        archive_a, sha_a = self.archive(members)
        archive_b, sha_b = self.archive(list(reversed(members)))
        self.assertNotEqual(sha_a, sha_b)
        files_a = self.capture(archive_a, sha_a)["files"]
        files_b = self.capture(archive_b, sha_b)["files"]
        self.assertEqual(files_a, files_b)
        self.assertEqual(json.dumps(files_a, sort_keys=True), json.dumps(files_b, sort_keys=True))

    def test_compare_basic_shape(self):
        old, _ = self.capture_valid()
        new = self.clone(old)
        result = upstream_snapshot.compare(old, new)
        self.assertIsInstance(result, dict)
        self.assertEqual(result.get("schema_version"), 1)
        self.assertEqual(result["old"], old["release"])
        self.assertEqual(result["new"], new["release"])
        self.assertEqual(result["changes"], [])
        self.assertFalse(result["announcement_changed"])
        self.assertEqual(result["compatibility"], "not_assessed")

    def test_compare_detects_add_remove_modify_sorted(self):
        old, _ = self.capture_valid()
        old["files"]["tests/basic/002.phpt"] = {"category": "test", "sha256": "a" * 64}
        new = self.clone(old)
        new["files"]["Zend/zend_language_parser.y"]["sha256"] = "b" * 64
        new["files"]["ext/bar/bar.stub.php"] = {"category": "stub", "sha256": "c" * 64}
        del new["files"]["tests/basic/001.phpt"]
        result = upstream_snapshot.compare(old, new)
        changes = result["changes"]
        self.assertEqual(len(changes), 3)
        paths = [change["path"] for change in changes]
        self.assertEqual(paths, sorted(paths))
        by_path = {change["path"]: change for change in changes}
        self.assertEqual(by_path["Zend/zend_language_parser.y"]["kind"], "modified")
        self.assertEqual(by_path["ext/bar/bar.stub.php"]["kind"], "added")
        self.assertEqual(by_path["tests/basic/001.phpt"]["kind"], "removed")
        self.assertEqual(by_path["Zend/zend_language_parser.y"]["old_sha256"], old["files"]["Zend/zend_language_parser.y"]["sha256"])
        self.assertEqual(by_path["Zend/zend_language_parser.y"]["new_sha256"], "b" * 64)
        self.assertEqual(by_path["ext/bar/bar.stub.php"]["new_sha256"], "c" * 64)
        self.assertEqual(by_path["tests/basic/001.phpt"]["old_sha256"], old["files"]["tests/basic/001.phpt"]["sha256"])
        for change in changes:
            self.assertEqual(change["category"], new["files"].get(change["path"], old["files"].get(change["path"]))["category"])
            for key in ("path", "category", "kind", "old_sha256", "new_sha256"):
                self.assertIn(key, change)

    def test_compare_covers_all_categories_and_change_kinds(self):
        old_contents = {
            "Zend/zend_language_parser.y": b"parser old\n",
            "Zend/zend_language_scanner.l": b"scanner\n",
            "ext/legacy/removed.stub.php": b"removed stub\n",
            "ext/keep/keep.stub.php": b"keep stub old\n",
            "tests/old/001.phpt": b"--TEST--\nold\n",
            "tests/keep/002.phpt": b"--TEST--\nkeep old\n",
            "NEWS": b"news old\n",
            "UPGRADING": b"upgrading removed\n",
        }
        new_contents = {
            "Zend/zend_language_parser.y": b"parser new\n",
            "Zend/zend_language_scanner.l": b"scanner\n",
            "ext/keep/keep.stub.php": b"keep stub new\n",
            "ext/new/added.stub.php": b"added stub\n",
            "tests/keep/002.phpt": b"--TEST--\nkeep new\n",
            "tests/new/003.phpt": b"--TEST--\nnew\n",
            "NEWS": b"news new\n",
        }

        def build(contents):
            return [self.regular(f"{self.ROOT}/{path}", data) for path, data in contents.items()]

        old, _ = self.capture_valid(build(old_contents))
        new, _ = self.capture_valid(build(new_contents))
        result = upstream_snapshot.compare(old, new)
        changes = result["changes"]
        expected = {
            "NEWS": ("release_note", "modified"),
            "UPGRADING": ("release_note", "removed"),
            "Zend/zend_language_parser.y": ("parser", "modified"),
            "ext/keep/keep.stub.php": ("stub", "modified"),
            "ext/legacy/removed.stub.php": ("stub", "removed"),
            "ext/new/added.stub.php": ("stub", "added"),
            "tests/keep/002.phpt": ("test", "modified"),
            "tests/new/003.phpt": ("test", "added"),
            "tests/old/001.phpt": ("test", "removed"),
        }
        paths = [change["path"] for change in changes]
        self.assertEqual(paths, sorted(expected))
        self.assertEqual(set(paths), set(expected))
        self.assertEqual({change["category"] for change in changes}, {"parser", "stub", "test", "release_note"})
        self.assertEqual({change["kind"] for change in changes}, {"added", "removed", "modified"})
        self.assertNotIn("Zend/zend_language_scanner.l", paths)
        for change in changes:
            category, kind = expected[change["path"]]
            self.assertEqual(change["category"], category)
            self.assertEqual(change["kind"], kind)
            if change["path"] in old_contents:
                self.assertEqual(
                    change["old_sha256"], hashlib.sha256(old_contents[change["path"]]).hexdigest()
                )
            if change["path"] in new_contents:
                self.assertEqual(
                    change["new_sha256"], hashlib.sha256(new_contents[change["path"]]).hexdigest()
                )

    def test_compare_announcement_changed(self):
        old, _ = self.capture_valid()
        new = self.clone(old)
        new["release"]["announcement"] = "https://www.php.net/releases/8_3_0.php"
        result = upstream_snapshot.compare(old, new)
        self.assertTrue(result["announcement_changed"])

    def test_compare_rejects_invalid_snapshot(self):
        good, _ = self.capture_valid()

        def set_schema(snapshot):
            snapshot["schema_version"] = 2

        def remove_schema(snapshot):
            del snapshot["schema_version"]

        def remove_release(snapshot):
            del snapshot["release"]

        def remove_version(snapshot):
            del snapshot["release"]["version"]

        def set_tag(snapshot):
            snapshot["release"]["tag"] = "v8.2.0"

        def set_commit(snapshot):
            snapshot["release"]["commit"] = "abc123"

        def set_announcement(snapshot):
            snapshot["release"]["announcement"] = "http://www.php.net/releases/8_2_0.php"

        def set_category(snapshot):
            snapshot["files"]["NEWS"]["category"] = "unknown"

        def remove_category(snapshot):
            del snapshot["files"]["NEWS"]["category"]

        def set_digest(snapshot):
            snapshot["files"]["NEWS"]["sha256"] = "zz"

        def remove_digest(snapshot):
            del snapshot["files"]["NEWS"]["sha256"]

        def set_files_list(snapshot):
            snapshot["files"] = []

        def remove_all_files(snapshot):
            snapshot["files"] = {}

        def remove_category_files(snapshot):
            snapshot["files"] = {path: item for path, item in snapshot["files"].items() if item["category"] != "test"}

        cases = {
            "schema version": set_schema,
            "missing schema version": remove_schema,
            "missing release": remove_release,
            "missing version": remove_version,
            "bad tag": set_tag,
            "bad commit": set_commit,
            "bad announcement": set_announcement,
            "bad category": set_category,
            "missing category": remove_category,
            "bad digest": set_digest,
            "missing digest": remove_digest,
            "files not mapping": set_files_list,
            "empty files": remove_all_files,
            "missing category": remove_category_files,
        }
        for label, mutate in cases.items():
            with self.subTest(case=label):
                candidate = self.clone(good)
                mutate(candidate)
                with self.assertRaises(ValueError):
                    upstream_snapshot.compare(good, candidate)

        for label, mutate in cases.items():
            with self.subTest(case=f"old {label}"):
                candidate = self.clone(good)
                mutate(candidate)
                with self.assertRaises(ValueError):
                    upstream_snapshot.compare(candidate, good)


if __name__ == "__main__":
    unittest.main()
