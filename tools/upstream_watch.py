#!/usr/bin/env python3
"""Observe official PHP release metadata and compare candidate changes."""
import argparse
import hashlib
import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit

CATALOG = "https://www.php.net/releases/index.php?json"
PHP = "https://www.php.net"
GITHUB = "https://api.github.com/repos/php/php-src/git"
MAX_RESPONSE = 2 * 1024 * 1024
MAX_SERIES = 30
SERIES = re.compile(r"[0-9]+\.[0-9]+\Z")
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+\Z")
SHA40 = re.compile(r"[0-9a-f]{40}\Z")
SHA64 = re.compile(r"[0-9a-f]{64}\Z")
FIELDS = {"version", "tag", "tag_object", "commit", "sha256", "announcement", "announcement_sha256", "release_tags"}


def _unique_pairs(pairs):
    data = {}
    for key, value in pairs:
        if key in data:
            raise ValueError("duplicate JSON key")
        data[key] = value
    return data


def _json(fetch, url):
    data = fetch(url)
    if not isinstance(data, bytes) or len(data) > MAX_RESPONSE:
        raise ValueError("invalid or oversized response")
    try:
        value = json.loads(data, object_pairs_hook=_unique_pairs)
    except (ValueError, UnicodeError, RecursionError) as exc:
        raise ValueError("invalid JSON response") from exc
    if not isinstance(value, dict):
        raise ValueError("expected JSON object")
    return value


def _content(body):
    """Extract the official content section, excluding rotating site chrome."""
    start = b'<section id="layout-content">'
    end = b'</section><!-- layout-content -->'
    if not isinstance(body, bytes) or len(body) > MAX_RESPONSE or body.count(start) != 1 or body.count(end) != 1:
        raise ValueError("invalid source content section")
    left = body.index(start)
    right = body.index(end)
    if right <= left + len(start):
        raise ValueError("empty or misplaced source content")
    return body[left:right]


def _record(series, record):
    if not isinstance(series, str) or not SERIES.fullmatch(series):
        raise ValueError("invalid release series")
    if not isinstance(record, dict) or set(record) != FIELDS:
        raise ValueError("invalid release record fields")
    version = record["version"]
    if not isinstance(version, str) or not VERSION.fullmatch(version) or version.rsplit(".", 1)[0] != series:
        raise ValueError("release version does not match series")
    if record["tag"] != "php-" + version:
        raise ValueError("release tag does not match version")
    for key in ("tag_object", "commit"):
        if not isinstance(record[key], str) or not SHA40.fullmatch(record[key]):
            raise ValueError("invalid git object")
    for key in ("sha256", "announcement_sha256"):
        if not isinstance(record[key], str) or not SHA64.fullmatch(record[key]):
            raise ValueError("invalid release digest")
    if record["announcement"] != PHP + "/releases/" + version.replace(".", "_") + ".php":
        raise ValueError("invalid announcement URL")
    tags = record["release_tags"]
    if not isinstance(tags, list) or len(tags) > 30 or not all(isinstance(x, str) and re.fullmatch(r"[a-zA-Z0-9_-]{1,40}", x) for x in tags) or tags != sorted(set(tags)):
        raise ValueError("invalid release tags")
    return record


def _snapshot(snapshot):
    if not isinstance(snapshot, dict) or set(snapshot) != {"schema_version", "series", "news_sha256"} or type(snapshot["schema_version"]) is not int or snapshot["schema_version"] != 2:
        raise ValueError("invalid snapshot schema")
    news_digest = snapshot["news_sha256"]
    if not isinstance(news_digest, str) or not SHA64.fullmatch(news_digest):
        raise ValueError("invalid news digest")
    series = snapshot["series"]
    if not isinstance(series, dict) or not 0 < len(series) <= MAX_SERIES:
        raise ValueError("invalid series collection")
    for name, record in series.items():
        _record(name, record)
    return series


def observe(fetch):
    """Return an official-source metadata snapshot from a bounded fetcher."""
    catalog = _json(fetch, CATALOG)
    supported = set()
    for major, info in catalog.items():
        if not isinstance(major, str) or not major.isdigit() or not isinstance(info, dict):
            raise ValueError("invalid release catalog")
        versions = info.get("supported_versions")
        if not isinstance(versions, list):
            raise ValueError("invalid supported versions")
        for series in versions:
            if not isinstance(series, str) or not SERIES.fullmatch(series) or series.split(".")[0] != major:
                raise ValueError("invalid supported release series")
            if series in supported:
                raise ValueError("duplicate supported release series")
            supported.add(series)
    if not 0 < len(supported) <= MAX_SERIES:
        raise ValueError("invalid supported release count")
    news_digest = hashlib.sha256(_content(fetch(PHP + "/"))).hexdigest()
    records = {}
    for series in sorted(supported):
        releases = _json(fetch, PHP + "/releases/index.php?json&version=" + series + "&max=1")
        if len(releases) != 1:
            raise ValueError("expected exactly one latest release")
        version, details = next(iter(releases.items()))
        if not isinstance(version, str) or not VERSION.fullmatch(version) or version.rsplit(".", 1)[0] != series or not isinstance(details, dict):
            raise ValueError("invalid latest release")
        sources = details.get("source")
        if not isinstance(sources, list) or len(sources) > 100:
            raise ValueError("invalid source list")
        matches = [item for item in sources if isinstance(item, dict) and item.get("filename") == "php-" + version + ".tar.xz"]
        if len(matches) != 1 or not isinstance(matches[0].get("sha256"), str) or not SHA64.fullmatch(matches[0]["sha256"]):
            raise ValueError("missing or invalid source digest")
        announcement_path = "/releases/" + version.replace(".", "_") + ".php"
        notice = details.get("announcement")
        if notice is not True and (not isinstance(notice, dict) or notice.get("English") != announcement_path):
            raise ValueError("invalid announcement metadata")
        announcement_url = PHP + announcement_path
        tags = details.get("tags")
        if not isinstance(tags, list) or not all(isinstance(x, str) for x in tags) or len(tags) != len(set(tags)):
            raise ValueError("invalid release tags")
        ref = _json(fetch, GITHUB + "/ref/tags/php-" + version).get("object")
        if not isinstance(ref, dict) or ref.get("type") not in ("tag", "commit") or not isinstance(ref.get("sha"), str) or not SHA40.fullmatch(ref["sha"]):
            raise ValueError("invalid tag object")
        tag_object = ref["sha"]
        if ref["type"] == "tag":
            obj = _json(fetch, GITHUB + "/tags/" + tag_object).get("object")
            if not isinstance(obj, dict) or obj.get("type") != "commit" or not isinstance(obj.get("sha"), str) or not SHA40.fullmatch(obj["sha"]):
                raise ValueError("invalid tagged commit")
            commit = obj["sha"]
        else:
            commit = tag_object
        notice_body = _content(fetch(announcement_url))
        version_marker = rb"PHP " + re.escape(version.encode("ascii")) + rb"(?![0-9]|\.[0-9])"
        if re.search(version_marker, notice_body) is None:
            raise ValueError("invalid announcement body")
        record = {"version": version, "tag": "php-" + version, "tag_object": tag_object,
                  "commit": commit, "sha256": matches[0]["sha256"], "announcement": announcement_url,
                  "announcement_sha256": hashlib.sha256(notice_body).hexdigest(), "release_tags": sorted(tags)}
        records[series] = _record(series, record)
    return {"schema_version": 2, "news_sha256": news_digest, "series": records}


def compare(old, new):
    """List candidate changes without claiming semantic compatibility."""
    before, after = _snapshot(old), _snapshot(new)
    changes = []
    for series in sorted(before.keys() | after.keys()):
        if series not in before or series not in after:
            changes.append({"series": series, "field": "series", "old": before.get(series), "new": after.get(series)})
            continue
        for field in sorted(FIELDS):
            if before[series][field] != after[series][field]:
                changes.append({"series": series, "field": field, "old": before[series][field], "new": after[series][field]})
    if old["news_sha256"] != new["news_sha256"]:
        changes.append({"series": "global", "field": "news_sha256", "old": old["news_sha256"], "new": new["news_sha256"]})
    return {"schema_version": 1, "status": "pending_review" if changes else "unchanged",
            "compatibility": "not_assessed", "changes": changes}


class _SameHostRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        old, new = urlsplit(req.full_url), urlsplit(newurl)
        if new.scheme != "https" or old.hostname != new.hostname or new.port not in (None, 443) or new.username or new.password:
            raise ValueError("redirect rejected")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def fetch(url):
    parts = urlsplit(url)
    if parts.scheme != "https" or parts.hostname not in ("www.php.net", "api.github.com") or parts.port not in (None, 443) or parts.username or parts.password:
        raise ValueError("source URL rejected")
    headers = {"User-Agent": "Phego-upstream-watch/1", "Accept": "application/vnd.github+json"}
    token = os.environ.get("GITHUB_TOKEN")
    if parts.hostname == "api.github.com" and token:
        if len(token) > 4096 or not token.isascii() or any(ord(c) <= 32 or ord(c) == 127 for c in token):
            raise ValueError("invalid GitHub token")
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.build_opener(_SameHostRedirect()).open(request, timeout=20) as response:
        data = response.read(MAX_RESPONSE + 1)
    if len(data) > MAX_RESPONSE:
        raise ValueError("source response exceeds size limit")
    return data


def _read_snapshot(path):
    with Path(path).open("rb") as source:
        data = source.read(MAX_RESPONSE + 1)
    if len(data) > MAX_RESPONSE:
        raise ValueError("snapshot exceeds size limit")
    return json.loads(data, object_pairs_hook=_unique_pairs)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("observe")
    diff = commands.add_parser("compare")
    diff.add_argument("old")
    diff.add_argument("new")
    args = parser.parse_args(argv)
    try:
        result = observe(fetch) if args.command == "observe" else compare(_read_snapshot(args.old), _read_snapshot(args.new))
        print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    except (OSError, ValueError, UnicodeError, RecursionError) as exc:
        print("Error: " + str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
