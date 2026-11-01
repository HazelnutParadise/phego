#!/usr/bin/env python3
"""Verify locked PHP sources, classify all changed files, and queue regressions.

The pair command accepts a reviewed {old,new} release lock. The watch command
uses validated W-02 observation snapshots. Neither command executes source or
certifies PHP compatibility. JSON goes to stdout; invalid inputs exit 2.
"""
import argparse
import hashlib
import io
import json
import os
import stat
import sys
import time
import urllib.request

try:
    from tools import upstream_diff, upstream_snapshot, upstream_watch
except ImportError:  # run as tools/upstream_update.py
    import upstream_diff
    import upstream_snapshot
    import upstream_watch


MAX_JSON = 2 * 1024 * 1024
MAX_REPORT = 32 * 1024 * 1024
MAX_FETCH_SECONDS = 180
_HASH_CHUNK = 64 * 1024


def _unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def _read_bounded(path, limit):
    flags = os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW
    descriptor = os.open(os.fspath(path), flags)
    try:
        if not stat.S_ISREG(os.fstat(descriptor).st_mode):
            raise ValueError("input must be a regular file")
        with os.fdopen(descriptor, "rb") as source:
            descriptor = -1
            data = source.read(limit + 1)
        if len(data) > limit:
            raise ValueError("input exceeds size limit")
        return data
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def _read_json(path):
    data = _read_bounded(path, MAX_JSON)
    try:
        value = json.loads(data, object_pairs_hook=_unique_pairs)
    except (ValueError, UnicodeError, RecursionError) as exc:
        raise ValueError("invalid JSON input") from exc
    if not isinstance(value, dict):
        raise ValueError("JSON input must be an object")
    return value


def _manifest_category(path):
    category = upstream_snapshot._category(path)
    if category is not None:
        return category
    if path.endswith((".c", ".h", ".inc", ".S", ".cpp")):
        return "implementation"
    return "unclassified"


def _all_file_manifest(data, version):
    root = "php-" + version
    files = {}
    with upstream_snapshot._open_archive(
            data, upstream_diff.MAX_UNPACKED, upstream_diff.MAX_MEMBERS) as stream:
        for member in stream:
            if not member.isfile():
                continue
            if not member.name.startswith(root + "/"):
                raise ValueError("archive root does not match release version")
            path = member.name[len(root) + 1:]
            if not path or path in files:
                raise ValueError("invalid or duplicate archive path")
            handle = stream.extractfile(member)
            if handle is None:
                raise ValueError("cannot read archive entry")
            digest = hashlib.sha256()
            size = 0
            with handle:
                while True:
                    chunk = handle.read(_HASH_CHUNK)
                    if not chunk:
                        break
                    size += len(chunk)
                    if size > member.size:
                        raise ValueError("archive entry exceeds declared size")
                    digest.update(chunk)
            if size != member.size:
                raise ValueError("truncated archive entry")
            files[path] = {
                "category": _manifest_category(path),
                "sha256": digest.hexdigest(),
            }
    return dict(sorted(files.items()))


def _validated_sources(data, release):
    if not isinstance(data, bytes) or len(data) > upstream_diff.MAX_ARCHIVE:
        raise ValueError("invalid or oversized archive")
    if hashlib.sha256(data).hexdigest() != release["archive_sha256"]:
        raise ValueError("archive sha256 mismatch")


def _source_manifest(data, release):
    content = upstream_diff.read_archive(data, release["version"])
    manifest = _all_file_manifest(data, release["version"])
    required = (
        "Zend/zend_language_parser.y",
        "Zend/zend_language_scanner.l",
        "NEWS",
    )
    if any(path not in manifest for path in required):
        raise ValueError("archive is missing required parser, scanner, or NEWS source")
    if ("Zend/zend_language_parser.y" not in content or "NEWS" not in content
            or not any(path.endswith(".stub.php") for path in content)
            or not any(path.endswith(".phpt") for path in content)):
        raise ValueError("archive is missing required parser, NEWS, stub, or PHPT content")
    if (not any(item["category"] == "stub" for item in manifest.values())
            or not any(item["category"] == "test" for item in manifest.values())):
        raise ValueError("archive is missing required stub or PHPT files")
    return content, manifest


def classify(old_data, new_data, old_release, new_release):
    """Return a deterministic source-verified report, or raise ValueError.

    Validate release identities and both SHA-256 values before decoding. Require
    parser, scanner, NEWS, stub and PHPT inputs. Track every regular file by hash,
    retain the existing detailed classifier, mark non-test changes needs_review,
    and produce source-bound pending PHPT regression candidates. Compatibility
    always remains not_assessed, including identical or same-signature sources.
    Apply upstream_diff archive/resource limits; never extract or execute files.
    """
    old_identity = upstream_snapshot._release(old_release)
    new_identity = upstream_snapshot._release(new_release)
    _validated_sources(old_data, old_identity)
    _validated_sources(new_data, new_identity)

    old_content, old_manifest = _source_manifest(old_data, old_identity)
    new_content, new_manifest = _source_manifest(new_data, new_identity)
    classification = upstream_diff.diff_releases(
        old_content, new_content, new_identity["version"])

    changes = []
    regression_candidates = []
    affected = set(classification["affected_work_packages"])
    for path in sorted(old_manifest.keys() | new_manifest.keys()):
        before = old_manifest.get(path)
        after = new_manifest.get(path)
        if before is not None and after is not None and before["sha256"] == after["sha256"]:
            continue
        current = after or before
        category = current["category"]
        kind = "added" if before is None else "removed" if after is None else "modified"
        work_packages = upstream_diff.impact(path)
        review = {
            "release_note": "needs_assessment",
            "test": "pending",
        }.get(category, "needs_behavior_regression")
        change = {
            "path": path,
            "category": category,
            "kind": kind,
            "old_sha256": before["sha256"] if before is not None else None,
            "new_sha256": after["sha256"] if after is not None else None,
            "work_packages": work_packages,
            "review": review,
        }
        changes.append(change)
        affected.update(work_packages)
        if category == "test" and kind in ("added", "modified"):
            regression_candidates.append({
                "path": path,
                "sha256": after["sha256"],
                "release": new_identity["version"],
                "archive_sha256": new_identity["archive_sha256"],
                "work_packages": work_packages,
                "status": "pending",
            })

    return {
        "schema_version": 1,
        "old": old_identity,
        "new": new_identity,
        "source_verification": "sha256_verified",
        "compatibility": "not_assessed",
        "classification": classification,
        "files": changes,
        "regression_candidates": regression_candidates,
        "affected_work_packages": sorted(affected),
    }


def _watch_release(record):
    return upstream_snapshot._release({
        "version": record["version"],
        "tag": record["tag"],
        "commit": record["commit"],
        "archive_sha256": record["sha256"],
        "announcement": record["announcement"],
    })


def _json_size(value, limit=MAX_REPORT):
    encoder = json.JSONEncoder(
        ensure_ascii=True, sort_keys=True, separators=(",", ":"))
    size = 0
    for chunk in encoder.iterencode(value):
        size += len(chunk)
        if size > limit:
            raise ValueError("serialized report exceeds size limit")
    return size


def _error_text(error):
    text = str(error)
    return text if len(text) <= 1024 else text[:1024] + "..."


def observe_changes(baseline, observed, load_archive):
    """Classify changed release pairs from valid W-02 snapshots.

    load_archive(release) -> bytes is called only for changed version/digest pairs
    present in both snapshots. Preserve all metadata changes, additions/removals,
    and per-series failures as blocked results. A failed pair must not discard
    successful pairs; no-change input performs no downloads. Compatibility is
    not_assessed for every result. Do not infer an old release for a new series.
    """
    metadata = upstream_watch.compare(baseline, observed)
    old_series = upstream_watch._snapshot(baseline)
    new_series = upstream_watch._snapshot(observed)
    comparisons = []
    capacity_base = {
        "schema_version": 1,
        "status": "pending_review",
        "compatibility": "not_assessed",
        "metadata": metadata,
        "comparisons": [],
    }
    # Keep the array delimiters in the budget; entries are inserted inside them.
    accumulated_size = _json_size(capacity_base)

    def append_comparison(entry, reserved_bytes):
        nonlocal accumulated_size
        entry_size = _json_size(entry)
        next_size = accumulated_size + (1 if comparisons else 0) + entry_size
        if next_size + reserved_bytes > MAX_REPORT:
            entry = {
                "series": entry["series"],
                "status": "blocked",
                "error": "accumulated report exceeds size limit",
            }
            entry_size = _json_size(entry)
            next_size = accumulated_size + (1 if comparisons else 0) + entry_size
        if next_size > MAX_REPORT:
            raise ValueError("watch report exceeds size limit")
        comparisons.append(entry)
        accumulated_size = next_size

    all_series = sorted(old_series.keys() | new_series.keys())
    # Reserve an error record for each remaining series. A Unicode code point
    # can occupy twelve ASCII bytes in JSON, including a surrogate pair.
    error_budgets = [_json_size({"series": series, "status": "blocked",
                                 "error": "\U00010000" * 1027}) + 1
                     for series in all_series]
    for index, series in enumerate(all_series):
        reserved_bytes = sum(error_budgets[index + 1:])
        before = old_series.get(series)
        after = new_series.get(series)
        if before is None:
            append_comparison({
                "series": series,
                "status": "blocked",
                "error": "release series added without a baseline",
            }, reserved_bytes)
            continue
        if after is None:
            append_comparison({
                "series": series,
                "status": "blocked",
                "error": "release series removed from observation",
            }, reserved_bytes)
            continue
        if (before["version"] == after["version"]
                and before["sha256"] == after["sha256"]):
            continue

        old_release = _watch_release(before)
        new_release = _watch_release(after)
        try:
            old_archive = load_archive(old_release)
            new_archive = load_archive(new_release)
            report = classify(old_archive, new_archive, old_release, new_release)
            append_comparison({
                "series": series,
                "status": "classified",
                "report": report,
            }, reserved_bytes)
        except (OSError, ValueError) as exc:
            append_comparison({
                "series": series,
                "status": "blocked",
                "error": _error_text(exc),
            }, reserved_bytes)

    status = ("blocked" if any(item["status"] == "blocked" for item in comparisons)
              else metadata["status"])
    return {
        "schema_version": 1,
        "status": status,
        "compatibility": "not_assessed",
        "metadata": metadata,
        "comparisons": comparisons,
    }


def _set_response_timeout(response, timeout):
    raw = getattr(getattr(response, "fp", None), "raw", None)
    sock = getattr(raw, "_sock", None)
    if sock is not None:
        sock.settimeout(timeout)


def _fetch_archive(release):
    url = ("https://www.php.net/distributions/php-"
           + release["version"] + ".tar.xz")
    request = urllib.request.Request(
        url,
        headers={
            "User-Agent": "Phego-upstream-update/1",
            "Accept": "application/x-xz",
        },
    )
    deadline = time.monotonic() + MAX_FETCH_SECONDS
    opener = urllib.request.build_opener(upstream_watch._SameHostRedirect())
    with opener.open(request, timeout=30) as response:
        body = bytearray()
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("archive fetch deadline exceeded")
            _set_response_timeout(response, min(30, remaining))
            # read() can keep receiving tiny fragments without returning to the
            # deadline check. read1() performs at most one underlying read.
            chunk = response.read1(min(_HASH_CHUNK, upstream_diff.MAX_ARCHIVE + 1 - len(body)))
            if not chunk:
                return bytes(body)
            body.extend(chunk)
            if len(body) > upstream_diff.MAX_ARCHIVE:
                raise ValueError("source archive exceeds size limit")


def _serialize_report(report):
    encoder = json.JSONEncoder(
        ensure_ascii=True, sort_keys=True, separators=(",", ":"))
    output = io.StringIO()
    size = 0
    for chunk in encoder.iterencode(report):
        size += len(chunk)
        if size > MAX_REPORT:
            raise ValueError("serialized report exceeds size limit")
        output.write(chunk)
    return output.getvalue()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command")
    pair = sub.add_parser("pair", help="Verify two archives against a reviewed release lock")
    pair.add_argument("--lock", required=True, help="JSON containing old/new release identities")
    pair.add_argument("old_archive")
    pair.add_argument("new_archive")
    watch = sub.add_parser("watch", help="Classify version/digest changes in W-02 snapshots")
    watch.add_argument("baseline")
    watch.add_argument("observed")
    watch.add_argument("--archive-dir", help="Offline cache: <sha256>.tar.xz; missing files block")
    args = parser.parse_args(argv)
    if args.command is None:
        parser.print_help()
        return 0
    try:
        if args.command == "pair":
            lock = _read_json(args.lock)
            if "old" not in lock or "new" not in lock:
                raise ValueError("release lock must contain old and new records")
            old_release = upstream_snapshot._release(lock["old"])
            new_release = upstream_snapshot._release(lock["new"])
            old_archive = _read_bounded(args.old_archive, upstream_diff.MAX_ARCHIVE)
            new_archive = _read_bounded(args.new_archive, upstream_diff.MAX_ARCHIVE)
            result = classify(old_archive, new_archive, old_release, new_release)
            output = _serialize_report(result)
            sys.stdout.write(output + "\n")
            return 0

        baseline = _read_json(args.baseline)
        observed = _read_json(args.observed)
        if args.archive_dir is None:
            load_archive = _fetch_archive
        else:
            def load_archive(release):
                path = os.path.join(
                    args.archive_dir, release["archive_sha256"] + ".tar.xz")
                return _read_bounded(path, upstream_diff.MAX_ARCHIVE)

        result = observe_changes(baseline, observed, load_archive)
        output = _serialize_report(result)
        sys.stdout.write(output + "\n")
        return 2 if result["status"] == "blocked" else 0
    except (OSError, ValueError, UnicodeError, RecursionError) as exc:
        print("upstream_update: " + _error_text(exc), file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
