"""Build the PHP release inventory from saved official museum and release data.

Every discovered version keeps its own record. Names that cannot be mapped to
an exact version stay unresolved instead of being merged into a nearby release.
"""
import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

from history_museum import MAX_INDEX_BYTES, discover

MAX_RELEASES_BYTES = 16 * 1024 * 1024
RELEASES_URL = "https://www.php.net/releases/index.php?json&version={major}&max=1000"
EXTENSION = re.compile(r"\.(?:tar\.gz|tar\.bz2|tar\.xz|zip|exe|msi)\Z")
PREFIXES = (("php-debug-pack-", "debug-pack"), ("php-devel-pack-", "devel-pack"),
            ("php-test-pack-", "test-pack"), ("pecl-", "pecl"), ("php-", "php"), ("php", "php"))
CORE_VERSION = re.compile(r"\d+\.\d+(?:\.\d+)*[A-Za-z0-9]*\Z")
VERSION_KEY = re.compile(r"\d+\.\d+\.\d+")
PRERELEASE = re.compile(r"(alpha|beta|RC|rc|b\d|a\d|dev)", re.I)

# Releases known only from historical announcements. They have no retrievable
# archive in the official sources read by this tool and remain blockers.
ANNOUNCED_WITHOUT_ARCHIVE = [
    {"name": "Personal Home Page Tools", "version": "1.0", "generation": "php1",
     "announced": "1995-06-08",
     "announcement": "https://groups.google.com/g/comp.infosystems.www.authoring.cgi/c/PyJ25gZ6z7A/m/M9FkTUVDfcwJ",
     "note": "First public release; no archive is listed by museum.php.net or php.net."},
]
# Versions listed only in Rasmus Lerdorf's 2002 history slides. They are leads,
# not confirmed public releases, and remain blockers until a source is found.
SLIDE_LEADS = [("1.90", "1995-09-18"), ("1.91", "1995-10-09"), ("1.92", "1995-10-17")]
SLIDES_URL = "https://talks.php.net/show/oscon2002/5"
# Archive names whose exact version was established from their contents.
KNOWN_IDENTITY = {
    "php-108.tar.gz": ("1.0.8", "docs/implementation/evidence/W-49-php-108-source.json"),
}
# Versions whose official archive was retrieved and hashed in recorded evidence.
RETRIEVED = {"2.0": "docs/implementation/evidence/W-49-php-200-source.json",
             "1.0.8": "docs/implementation/evidence/W-49-php-108-source.json"}


def _read(path, limit):
    data = Path(path).read_bytes()
    if len(data) > limit:
        raise ValueError(f"{path}: input exceeds {limit} bytes")
    return data


def classify_name(filename):
    """Return (version, artifact) for an archive name, or (None, None).

    The version keeps prerelease words such as "dev-zend2-alpha1"; Windows
    build revisions ("5.2.9-2") and platform words describe the artifact, not
    a separate PHP release.
    """
    ext = EXTENSION.search(filename)
    if not ext:
        return None, None
    rest = filename[:ext.start()]
    for prefix, kind in PREFIXES:
        if rest.startswith(prefix):
            rest = rest[len(prefix):]
            break
    else:
        return None, None
    tokens = rest.split("-")
    if not CORE_VERSION.fullmatch(tokens[0]):
        return None, None
    version, build, platform = [tokens[0]], [], []
    for token in tokens[1:]:
        low = token.lower()
        if token.isdigit() and not platform:
            build.append(token)
        elif re.fullmatch(r"(?:alpha|beta|rc|dev|pl)[a-z0-9]*|zend\d+", low):
            version.append(token)
        else:
            platform.append(token)
    artifact = {"kind": kind, "platform": "-".join(platform) or "source"}
    if build:
        artifact["build"] = "-".join(build)
    return "-".join(version), artifact


def build(museum, releases):
    """museum: [(url, path)]; releases: [(major, path)]."""
    records, inputs, unresolved = {}, [], []

    def record(version, generation):
        r = records.setdefault(version, {
            "version": version, "generation": generation,
            "release_type": "prerelease" if PRERELEASE.search(version) else "release",
            "artifacts": [], "published_checksums": False, "source_status": "not_retrieved",
            "evidence_status": "listed_unverified"})
        return r

    for url, path in museum:
        data = _read(path, MAX_INDEX_BYTES)
        inputs.append({"url": url, "sha256": hashlib.sha256(data).hexdigest()})
        listing = discover(data, url)
        generation = url.rstrip("/").rsplit("/", 1)[1]
        for c in listing["candidates"]:
            version, artifact = classify_name(c["filename"])
            if c["filename"] in KNOWN_IDENTITY:
                version, artifact = KNOWN_IDENTITY[c["filename"]][0], {"kind": "php", "platform": "source"}
            if version is None:
                unresolved.append({"generation": generation, "filename": c["filename"], "url": c["url"],
                                   "reason": "archive name does not state an exact version"})
                continue
            record(version, generation)["artifacts"].append(
                {"filename": c["filename"], **artifact, "url": c["url"], "listed_by": "museum"})

    for major, path in releases:
        data = _read(path, MAX_RELEASES_BYTES)
        url = RELEASES_URL.format(major=major)
        inputs.append({"url": url, "sha256": hashlib.sha256(data).hexdigest()})
        doc = json.loads(data)
        for key, entry in doc.items():
            if VERSION_KEY.fullmatch(key):
                record(key, f"php{major}").setdefault("date", entry.get("date"))
            for kind in ("source", "windows"):
                for item in entry.get(kind, []):
                    if "filename" not in item:
                        if VERSION_KEY.fullmatch(key) and isinstance(item.get("link"), str):
                            record(key, f"php{major}")["artifacts"].append(
                                {"filename": None, "kind": "link", "platform": kind, "url": item["link"], "listed_by": "releases"})
                        else:
                            unresolved.append({"generation": f"php{major}", "filename": None, "url": url,
                                               "reason": f"release entry {key!r} has no archive name"})
                        continue
                    version, artifact = classify_name(item["filename"])
                    if version is None:
                        unresolved.append({"generation": f"php{major}", "filename": item["filename"],
                                           "url": url, "reason": "archive name does not state an exact version"})
                        continue
                    r = record(version, f"php{major}")
                    checksum = {k: item[k] for k in ("sha256", "md5") if k in item}
                    if checksum:
                        r["published_checksums"] = True
                    match = next((a for a in r["artifacts"] if a["filename"] == item["filename"]), None)
                    if match is None:
                        match = {"filename": item["filename"], **artifact, "url": None, "listed_by": "releases"}
                        r["artifacts"].append(match)
                    else:
                        match["listed_by"] = "museum+releases"
                    match.update(checksum)
                    if "date" in entry and "date" not in r:
                        r["date"] = entry["date"]

    out = []
    for announced in ANNOUNCED_WITHOUT_ARCHIVE:
        out.append({**announced, "release_type": "release", "artifacts": [], "published_checksums": False,
                    "source_status": "missing", "evidence_status": "blocked_missing_source"})
    for version, date in SLIDE_LEADS:
        if version not in records:
            out.append({"name": "PHP/FI", "version": version, "generation": "php1", "announced": date,
                        "announcement": SLIDES_URL, "release_type": "release", "artifacts": [],
                        "published_checksums": False, "source_status": "missing",
                        "evidence_status": "lead_unconfirmed"})
    for version in sorted(records, key=_sort_key):
        r = records[version]
        if version in RETRIEVED:
            r["source_status"], r["source_evidence"] = "retrieved_hash_recorded", RETRIEVED[version]
        if version in {v for v, _ in KNOWN_IDENTITY.values()}:
            r["identity_evidence"] = next(e for v, e in KNOWN_IDENTITY.values() if v == version)
        r["artifacts"].sort(key=lambda a: (a["filename"] or "", a["url"] or ""))
        out.append(r)
    return {
        "schema_version": 1,
        "scope": "release_inventory_candidates",
        "inventory_status": "incomplete",
        "inputs": inputs,
        "counts": {"versions": len(out), "unresolved_names": len(unresolved),
                   "missing_source": sum(r["source_status"] == "missing" for r in out),
                   "prereleases": sum(r["release_type"] == "prerelease" for r in out)},
        "releases": out,
        "unresolved": unresolved,
        "limits": [
            "Archives were listed, not downloaded or hash-checked; listed_unverified is not a source lock.",
            "Prereleases are only those named by these official listings; snapshot servers, git tags, and mailing-list announcements are not yet cross-checked.",
            "Unresolved names keep their raw filename and are not aliased to a nearby version.",
        ],
    }


def _sort_key(version):
    parts = re.findall(r"\d+|[A-Za-z]+", version)
    return [(0, int(p)) if p.isdigit() else (1, p.lower()) for p in parts]


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0],
                                epilog="Example: python3 tools/history_inventory.py --museum https://museum.php.net/php2/=php2.html "
                                       "--releases 8=releases-8.json -o inventory.json")
    p.add_argument("--museum", action="append", default=[], metavar="URL=PATH",
                   help="saved museum generation index and its official URL (repeatable)")
    p.add_argument("--releases", action="append", default=[], metavar="MAJOR=PATH",
                   help="saved php.net releases JSON for a major version (repeatable)")
    p.add_argument("-o", "--output", required=True, help="inventory JSON to write")
    args = p.parse_args(argv)
    try:
        museum = [tuple(x.split("=", 1)) for x in args.museum]
        releases = [tuple(x.split("=", 1)) for x in args.releases]
        if any(len(x) != 2 for x in museum + releases) or not all(m.isdigit() for m, _ in releases):
            raise ValueError("expected URL=PATH for --museum and MAJOR=PATH for --releases")
        result = build(museum, releases)
    except (OSError, ValueError, KeyError, TypeError) as e:
        print(f"error: {e}", file=sys.stderr)
        return 2
    Path(args.output).write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"{result['counts']['versions']} versions, {result['counts']['unresolved_names']} unresolved names")
    return 0


if __name__ == "__main__":
    sys.exit(main())
