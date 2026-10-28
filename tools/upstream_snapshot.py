#!/usr/bin/env python3
"""Inventory verified PHP release archives and compare candidate changes."""
import argparse
import hashlib
import io
import json
import re
import sys
import tarfile
from pathlib import Path
from urllib.parse import urlsplit

MAX_ARCHIVE = 100 * 1024 * 1024
MAX_FILE = 32 * 1024 * 1024
MAX_EXPANDED = 2 * 1024 * 1024 * 1024
MAX_ENTRIES = 100000
MAX_SNAPSHOT = 32 * 1024 * 1024
MAX_METADATA = 1024 * 1024
MAX_METADATA_HEADERS = 256
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
HEX40 = re.compile(r"[0-9a-f]{40}\Z")
VERSION = re.compile(r"[0-9]+(?:\.[0-9]+){1,2}\Z")


def _release(release):
    if not isinstance(release, dict):
        raise ValueError("release must be an object")
    version = release.get("version")
    tag = release.get("tag")
    commit = release.get("commit")
    digest = release.get("archive_sha256")
    announcement = release.get("announcement")
    if not isinstance(version, str) or not VERSION.fullmatch(version):
        raise ValueError("invalid release version")
    if tag != "php-" + version:
        raise ValueError("tag does not match version")
    if not isinstance(commit, str) or not HEX40.fullmatch(commit):
        raise ValueError("invalid commit")
    if not isinstance(digest, str) or not HEX64.fullmatch(digest):
        raise ValueError("invalid archive sha256")
    if not isinstance(announcement, str):
        raise ValueError("invalid announcement")
    try:
        url = urlsplit(announcement)
        valid = (url.scheme == "https" and url.netloc in ("www.php.net", "php.net")
                 and bool(url.path) and not url.fragment and not url.query)
    except ValueError:
        valid = False
    if not valid:
        raise ValueError("invalid announcement URL")
    return {"version": version, "tag": tag, "commit": commit,
            "archive_sha256": digest, "announcement": announcement}


def _category(path):
    if path in ("Zend/zend_language_parser.y", "Zend/zend_language_scanner.l"):
        return "parser"
    if path.endswith(".stub.php"):
        return "stub"
    if path.endswith(".phpt"):
        return "test"
    if path in ("NEWS", "UPGRADING"):
        return "release_note"
    return None


def _safe_path(path):
    return (isinstance(path, str) and bool(path) and not path.startswith("/")
            and "\\" not in path and all(part not in ("", ".", "..") for part in path.split("/")))


class _BoundedTarInfo(tarfile.TarInfo):
    @classmethod
    def fromtarfile(cls, archive):
        try:
            return super().fromtarfile(archive)
        except (tarfile.InvalidHeaderError, tarfile.TruncatedHeaderError,
                tarfile.EmptyHeaderError) as exc:
            # TarFile.next otherwise treats malformed trailing headers as EOF.
            raise tarfile.SubsequentHeaderError(str(exc)) from exc

    def _proc_sparse(self, archive):
        raise ValueError("sparse archive entries are unsupported")

    def _proc_gnusparse_00(self, *args):
        raise ValueError("sparse archive entries are unsupported")

    def _proc_gnusparse_01(self, *args):
        raise ValueError("sparse archive entries are unsupported")

    def _proc_gnusparse_10(self, *args):
        raise ValueError("sparse archive entries are unsupported")

    # tarfile reads PAX and GNU extension payloads before yielding a member.
    def _check_metadata(self, archive):
        count = getattr(archive, "_phego_metadata_count", 0) + 1
        size = getattr(archive, "_phego_metadata_size", 0) + self.size
        if self.size < 0 or count > MAX_METADATA_HEADERS or size > MAX_METADATA:
            raise ValueError("archive metadata limit exceeded")
        archive._phego_metadata_count = count
        archive._phego_metadata_size = size

    def _proc_pax(self, archive):
        self._check_metadata(archive)
        return super()._proc_pax(archive)

    def _proc_gnulong(self, archive):
        self._check_metadata(archive)
        return super()._proc_gnulong(archive)


def capture(archive, expected_sha256, version, tag, commit, announcement):
    """Return a deterministic snapshot of a digest-verified source archive."""
    release = _release({"version": version, "tag": tag, "commit": commit,
                        "archive_sha256": expected_sha256, "announcement": announcement})
    if not isinstance(archive, bytes) or len(archive) > MAX_ARCHIVE:
        raise ValueError("invalid or oversized archive")
    if hashlib.sha256(archive).hexdigest() != expected_sha256:
        raise ValueError("archive sha256 mismatch")
    root = "php-" + version
    seen, files, expanded = set(), {}, 0
    try:
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r|*", tarinfo=_BoundedTarInfo) as stream:
            for index, member in enumerate(stream, 1):
                if index > MAX_ENTRIES:
                    raise ValueError("too many archive entries")
                name = member.name.rstrip("/") if member.isdir() else member.name
                if not _safe_path(name) or not (name == root or name.startswith(root + "/")):
                    raise ValueError("invalid archive path")
                if name in seen:
                    raise ValueError("duplicate archive path")
                seen.add(name)
                if member.isdir():
                    continue
                if not member.isfile() or name == root:
                    raise ValueError("non-regular archive entry")
                expanded += member.size
                if member.size < 0 or member.size > MAX_FILE or expanded > MAX_EXPANDED:
                    raise ValueError("archive expansion limit exceeded")
                path = name[len(root) + 1:]
                category = _category(path)
                if category is None:
                    continue
                handle = stream.extractfile(member)
                if handle is None:
                    raise ValueError("cannot read archive entry")
                data = handle.read(MAX_FILE + 1)
                if len(data) != member.size:
                    raise ValueError("truncated archive entry")
                files[path] = {"category": category, "sha256": hashlib.sha256(data).hexdigest()}
    except (tarfile.TarError, EOFError, OSError, TypeError) as exc:
        raise ValueError("invalid archive") from exc
    if {item["category"] for item in files.values()} != {"parser", "stub", "test", "release_note"}:
        raise ValueError("missing required source category")
    return {"schema_version": 1, "release": release, "files": dict(sorted(files.items()))}


def _snapshot(snapshot):
    if not isinstance(snapshot, dict) or type(snapshot.get("schema_version")) is not int or snapshot["schema_version"] != 1:
        raise ValueError("invalid snapshot schema")
    release = _release(snapshot.get("release"))
    files = snapshot.get("files")
    if not isinstance(files, dict) or len(files) > MAX_ENTRIES:
        raise ValueError("invalid snapshot files")
    for path, item in files.items():
        category = _category(path) if _safe_path(path) else None
        if not isinstance(item, dict) or set(item) != {"category", "sha256"} or category is None or item.get("category") != category:
            raise ValueError("invalid snapshot path or category")
        digest = item.get("sha256")
        if not isinstance(digest, str) or not HEX64.fullmatch(digest):
            raise ValueError("invalid snapshot digest")
    if {item["category"] for item in files.values()} != {"parser", "stub", "test", "release_note"}:
        raise ValueError("missing required snapshot category")
    return release, files


def compare(old, new):
    """Return file changes and release metadata, without judging compatibility."""
    old_release, old_files = _snapshot(old)
    new_release, new_files = _snapshot(new)
    changes = []
    for path in sorted(old_files.keys() | new_files.keys()):
        before, after = old_files.get(path), new_files.get(path)
        if before == after:
            continue
        changes.append({"path": path, "category": (after or before)["category"],
                        "kind": "added" if before is None else "removed" if after is None else "modified",
                        "old_sha256": before["sha256"] if before else None,
                        "new_sha256": after["sha256"] if after else None})
    return {"schema_version": 1, "old": old_release, "new": new_release,
            "changes": changes,
            "announcement_changed": old_release["announcement"] != new_release["announcement"],
            "compatibility": "not_assessed"}


def _read(path, limit):
    with Path(path).open("rb") as source:
        data = source.read(limit + 1)
    if len(data) > limit:
        raise ValueError("input exceeds size limit")
    return data


def _unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    snap = sub.add_parser("capture")
    for name in ("archive", "sha256", "version", "tag", "commit", "announcement"):
        snap.add_argument("--" + name, required=True)
    diff = sub.add_parser("compare")
    diff.add_argument("old")
    diff.add_argument("new")
    args = parser.parse_args(argv)
    try:
        if args.command == "capture":
            result = capture(_read(args.archive, MAX_ARCHIVE), args.sha256, args.version,
                             args.tag, args.commit, args.announcement)
        else:
            old = json.loads(_read(args.old, MAX_SNAPSHOT), object_pairs_hook=_unique_pairs)
            new = json.loads(_read(args.new, MAX_SNAPSHOT), object_pairs_hook=_unique_pairs)
            result = compare(old, new)
        print(json.dumps(result, ensure_ascii=True, sort_keys=True, separators=(",", ":")))
    except (OSError, ValueError, UnicodeError, RecursionError) as exc:
        print("Error: " + str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
