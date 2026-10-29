#!/usr/bin/env python3
"""Validate work-package status claims without certifying acceptance."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
import tempfile
from pathlib import Path, PurePosixPath
from urllib.parse import unquote, urlsplit


_STATUSES = {"not_started", "in_progress", "blocked", "claimed_complete"}
_COMMIT_RE = re.compile(r"[0-9a-f]{40}\Z")
_SHA256_RE = re.compile(r"[0-9a-f]{64}\Z")
_MAX_BLOB_SIZE = 8 * 1024 * 1024
_MAX_LEDGER_SIZE = 1024 * 1024
_MAX_EVENTS = 10_000
_MAX_PLAN_FILE_SIZE = 64 * 1024 * 1024
_MAX_PLAN_TOTAL_SIZE = 512 * 1024 * 1024
_MAX_PLAN_FILES = 10_000
_GIT_TIMEOUT = 10


def _read_regular(path: Path, limit: int, label: str) -> bytes:
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(fd, "rb") as stream:
            metadata = os.fstat(stream.fileno())
            if not stat.S_ISREG(metadata.st_mode):
                raise ValueError(f"{label} must be a regular file")
            if metadata.st_size > limit:
                raise ValueError(f"{label} exceeds size limit")
            data = stream.read(limit + 1)
            if len(data) > limit:
                raise ValueError(f"{label} exceeds size limit")
            return data
    except OSError as exc:
        raise ValueError(f"cannot read {label}: {exc}") from exc


def _load_json(path: Path, label: str) -> object:
    try:
        data = _read_regular(path, _MAX_PLAN_FILE_SIZE, label)
        return json.loads(data.decode("utf-8"), object_pairs_hook=_unique_object)
    except (UnicodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ValueError(f"cannot read {label}: {exc}") from exc


def _read_root_file(root: Path, relative: Path) -> bytes:
    descriptors = []
    try:
        current = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        descriptors.append(current)
        for part in relative.parts[:-1]:
            current = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=current)
            descriptors.append(current)
        fd = os.open(relative.parts[-1], os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW, dir_fd=current)
        descriptors.append(fd)
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > _MAX_PLAN_FILE_SIZE:
            raise ValueError(f"linked planning input must be a bounded regular file: {relative}")
        with os.fdopen(os.dup(fd), "rb") as stream:
            data = stream.read(_MAX_PLAN_FILE_SIZE + 1)
        if len(data) > _MAX_PLAN_FILE_SIZE:
            raise ValueError(f"linked planning input exceeds file size limit: {relative}")
        return data
    except OSError as exc:
        raise ValueError(f"cannot read linked planning input {relative}: {exc}") from exc
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)


def _snapshot_tree(root: Path, destination: Path) -> None:
    count = 0
    total = 0
    def raise_walk_error(error):
        raise error
    try:
        for directory, directories, files, dirfd in os.fwalk(root, topdown=True, onerror=raise_walk_error, follow_symlinks=False):
            directories[:] = [name for name in directories if name not in {".git", "__pycache__"}]
            for name in directories:
                mode = os.stat(name, dir_fd=dirfd, follow_symlinks=False).st_mode
                if not stat.S_ISDIR(mode):
                    raise ValueError(f"planning input directory is not a directory: {name}")
            target = destination / Path(directory).relative_to(root)
            target.mkdir(parents=True, exist_ok=True)
            for name in files:
                if name in {".git", "__pycache__"}:
                    continue
                count += 1
                if count > _MAX_PLAN_FILES:
                    raise ValueError("planning tree exceeds file count limit")
                metadata = os.stat(name, dir_fd=dirfd, follow_symlinks=False)
                if not stat.S_ISREG(metadata.st_mode):
                    raise ValueError(f"planning input must be a regular file: {name}")
                relative = (Path(directory).relative_to(root) / name).as_posix()
                required_content = name.endswith(".md") or relative in {
                    "LICENSE", "docs/data/phego-requirements.json", "docs/data/phego-acceptance-tests.json",
                    "docs/data/phego-work-packages.json", "docs/data/phego-sources.json",
                    "docs/data/php-history-policy.json", "docs/data/competitor-catalog.json",
                    "docs/data/application-catalog.json", "docs/implementation/github-index.json",
                }
                if not required_content:
                    # The documentation validator only checks that these paths
                    # exist; do not read unrelated files that may contain secrets.
                    (target / name).touch()
                    continue
                fd = os.open(name, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW, dir_fd=dirfd)
                with os.fdopen(fd, "rb") as stream:
                    metadata = os.fstat(stream.fileno())
                    if not stat.S_ISREG(metadata.st_mode) or metadata.st_size > _MAX_PLAN_FILE_SIZE:
                        raise ValueError(f"planning input must be a bounded regular file: {name}")
                    data = stream.read(_MAX_PLAN_FILE_SIZE + 1)
                if len(data) > _MAX_PLAN_FILE_SIZE:
                    raise ValueError(f"planning input exceeds file size limit: {name}")
                total += len(data)
                if total > _MAX_PLAN_TOTAL_SIZE:
                    raise ValueError("planning tree exceeds total size limit")
                (target / name).write_bytes(data)
        # The documentation validator checks chapter anchors even in a linked
        # non-Markdown file. Read only those explicitly linked targets.
        copied_links = set()
        for markdown in destination.rglob("*.md"):
            body = re.sub(r"```.*?```", "", markdown.read_text(encoding="utf-8"), flags=re.S)
            for link in re.findall(r"(?<!!)\[[^\]]*\]\(([^\s)]+)\)", body):
                split = urlsplit(link)
                if split.scheme or link.startswith("//") or not split.fragment.startswith("chapter-"):
                    continue
                target_path = (markdown.parent / unquote(split.path)).resolve() if split.path else markdown
                try:
                    relative = target_path.relative_to(destination)
                except ValueError as exc:
                    raise ValueError(f"planning link escapes project: {link}") from exc
                if not target_path.is_file() or target_path.suffix == ".md" or relative in copied_links:
                    continue
                data = _read_root_file(root, relative)
                total += len(data)
                if total > _MAX_PLAN_TOTAL_SIZE:
                    raise ValueError("planning tree exceeds total size limit")
                target_path.write_bytes(data)
                copied_links.add(relative)
    except OSError as exc:
        raise ValueError(f"cannot snapshot planning tree: {exc}") from exc


def _plan_fingerprints(root: Path) -> dict[str, str]:
    paths = ("docs/data/phego-requirements.json", "docs/data/phego-acceptance-tests.json",
             "docs/data/phego-work-packages.json", "docs/implementation/github-index.json")
    return {name: hashlib.sha256(_read_regular(root / name, _MAX_PLAN_FILE_SIZE, name)).hexdigest() for name in paths}


def _unique_object(pairs: list[tuple[str, object]]) -> dict:
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def _git(root: Path, *args: str, binary: bool = False) -> bytes | str:
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env["GIT_NO_REPLACE_OBJECTS"] = "1"
    env["GIT_CONFIG_NOSYSTEM"] = "1"
    env["GIT_CONFIG_GLOBAL"] = os.devnull
    try:
        result = subprocess.run(
            ["git", *args], cwd=root, env=env, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, timeout=_GIT_TIMEOUT, check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError(f"git command failed: {exc}") from exc
    if result.returncode:
        detail = result.stderr.decode("utf-8", "replace").strip()
        raise ValueError(f"git {' '.join(args)} failed: {detail}")
    return result.stdout if binary else result.stdout.decode("utf-8", "strict")


def _validate_planning(root: Path) -> None:
    # The existing documentation validator reads the whole tree. Reject links
    # and special files first, and isolate it with a timeout to bound races.
    for directory, directories, files in os.walk(root, followlinks=False):
        directories[:] = [name for name in directories if name not in {".git", "__pycache__"}]
        for name in directories + files:
            path = Path(directory) / name
            mode = path.lstat().st_mode
            if not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)):
                raise ValueError(f"planning input must be a regular file or directory: {path}")
    command = [sys.executable, "-I", str(Path(__file__).with_name("validate_docs.py")),
               "--root", str(root), "--no-write"]
    try:
        result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=15, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ValueError(f"planning validation failed or timed out: {exc}") from exc
    if result.returncode:
        try:
            report = json.loads(result.stdout)
            errors = report.get("errors", []) if isinstance(report, dict) else []
        except (ValueError, TypeError):
            errors = []
        detail = "; ".join(str(item) for item in errors) if isinstance(errors, list) else ""
        raise ValueError("invalid planning records" + (": " + detail if detail else ""))


def _registry_ids(root: Path, filename: str, label: str) -> set[str]:
    data = _load_json(root / "docs/data" / filename, label)
    if not isinstance(data, dict) or not isinstance(data.get("items"), list):
        raise ValueError(f"{label} must contain an items array")
    result: set[str] = set()
    for item in data["items"]:
        if not isinstance(item, dict) or not isinstance(item.get("id"), str) or not item["id"]:
            raise ValueError(f"{label} contains an item without a valid id")
        if item["id"] in result:
            raise ValueError(f"duplicate id in {label}: {item['id']}")
        result.add(item["id"])
    return result


def _dependencies(value: object, package: str, field: str) -> list[str]:
    if not isinstance(value, list) or any(not isinstance(x, str) or not x for x in value):
        raise ValueError(f"{package}: {field} must be a list of IDs")
    if len(set(value)) != len(value):
        raise ValueError(f"{package}: duplicate {field}")
    return value


def _load_plan(root: Path) -> tuple[dict[str, dict], dict[str, dict[str, list[str]]]]:
    plan = _load_json(root / "docs/data/phego-work-packages.json", "work-package plan")
    index = _load_json(root / "docs/implementation/github-index.json", "GitHub integration index")
    if not isinstance(plan, dict) or not isinstance(plan.get("items"), list):
        raise ValueError("work-package plan must contain an items array")
    if not isinstance(index, dict) or not isinstance(index.get("items"), list):
        raise ValueError("GitHub integration index must contain an items array")
    requirements = _registry_ids(root, "phego-requirements.json", "requirements registry")
    tests = _registry_ids(root, "phego-acceptance-tests.json", "acceptance registry")

    def by_id(items: list, label: str) -> dict[str, dict]:
        out = {}
        for item in items:
            if not isinstance(item, dict) or not isinstance(item.get("id"), str) or not item["id"]:
                raise ValueError(f"{label} contains an item without a valid id")
            if item["id"] in out:
                raise ValueError(f"duplicate ID in {label}: {item['id']}")
            out[item["id"]] = item
        return out

    packages = by_id(plan["items"], "work-package plan")
    indexed = by_id(index["items"], "GitHub integration index")
    if set(packages) != set(indexed):
        raise ValueError("work-package IDs differ between plan and GitHub index")
    prereqs = {}
    for package, item in packages.items():
        raw = _dependencies(item.get("dependencies"), package, "dependencies")
        integration = _dependencies(indexed[package].get("integration_dependencies"), package, "integration_dependencies")
        idx_raw = _dependencies(indexed[package].get("dependencies"), package, "indexed dependencies")
        if raw != idx_raw:
            raise ValueError(f"{package}: original dependencies differ from GitHub index")
        for dep in raw + integration:
            if dep not in packages:
                raise ValueError(f"{package}: unknown dependency {dep}")
        for field, valid_ids in (("requirements", requirements), ("tests", tests)):
            values = item.get(field)
            if not isinstance(values, list) or any(not isinstance(v, str) or v not in valid_ids for v in values):
                raise ValueError(f"{package}: invalid {field} IDs")
            if len(set(values)) != len(values):
                raise ValueError(f"{package}: duplicate {field} IDs")
        prereqs[package] = {"dependencies": raw, "integration_dependencies": integration}

    visiting: set[str] = set()
    visited: set[str] = set()
    def visit(package: str) -> None:
        if package in visiting:
            raise ValueError(f"dependency cycle includes {package}")
        if package in visited:
            return
        visiting.add(package)
        for dep in prereqs[package]["dependencies"] + prereqs[package]["integration_dependencies"]:
            visit(dep)
        visiting.remove(package)
        visited.add(package)
    for package in packages:
        visit(package)
    return packages, prereqs


def _validate_evidence(root: Path, evidence: object, cache: dict) -> tuple[str, str, str]:
    if not isinstance(evidence, dict) or set(evidence) != {"commit", "path", "sha256"}:
        raise ValueError("evidence must contain exactly commit, path, and sha256")
    commit, path, digest = evidence["commit"], evidence["path"], evidence["sha256"]
    if not isinstance(commit, str) or not _COMMIT_RE.fullmatch(commit):
        raise ValueError("evidence commit must be a 40-character lowercase Git hash")
    if not isinstance(digest, str) or not _SHA256_RE.fullmatch(digest):
        raise ValueError("evidence sha256 must be 64 lowercase hex characters")
    if not isinstance(path, str) or not path or "\\" in path or "\x00" in path or re.match(r"[A-Za-z]:", path):
        raise ValueError("evidence path must be a safe relative path")
    pure = PurePosixPath(path)
    if pure.is_absolute() or any(part in {"", ".", ".."} for part in path.split("/")):
        raise ValueError("evidence path must be a safe relative path")
    key = (commit, path, digest)
    if key in cache:
        return key

    if "repository_root" not in cache:
        git_root = Path(_git(root, "rev-parse", "--show-toplevel").strip()).resolve()
        if git_root != root:
            raise ValueError("project root is not the Git worktree root")
        cache["repository_root"] = True
    if _git(root, "cat-file", "-t", commit).strip() != "commit":
        raise ValueError("evidence commit must identify a commit object")
    spec = f":(literal){path}"
    raw = _git(root, "ls-tree", "-z", "--full-tree", commit, "--", spec, binary=True)
    entries = raw.split(b"\0")
    matches = []
    for entry in entries:
        if not entry:
            continue
        try:
            metadata, entry_path = entry.split(b"\t", 1)
            mode, kind, object_id = metadata.decode("ascii").split(" ")
        except (ValueError, UnicodeDecodeError) as exc:
            raise ValueError("invalid git ls-tree response") from exc
        if entry_path.decode("utf-8", "surrogateescape") == path:
            matches.append((mode, kind, object_id))
    if len(matches) != 1:
        raise ValueError(f"evidence path is not a tracked file at commit: {path}")
    mode, kind, object_id = matches[0]
    if kind != "blob" or mode not in {"100644", "100755"}:
        raise ValueError(f"evidence must be a regular file, not a symlink: {path}")
    size_text = _git(root, "cat-file", "-s", object_id)
    try:
        size = int(size_text.strip())
    except ValueError as exc:
        raise ValueError("invalid git blob size") from exc
    if size < 0 or size > _MAX_BLOB_SIZE:
        raise ValueError(f"evidence blob exceeds 8 MiB: {path}")
    content = _git(root, "cat-file", "blob", object_id, binary=True)
    actual = hashlib.sha256(content).hexdigest()
    if actual != digest:
        raise ValueError(f"evidence sha256 does not match committed blob: {path}")
    cache[key] = object_id
    return key


def _transition_allowed(current: str, target: str) -> bool:
    allowed = {
        "not_started": {"in_progress", "blocked", "claimed_complete"},
        "in_progress": {"blocked", "claimed_complete"},
        "blocked": {"in_progress"},
        "claimed_complete": {"in_progress"},
    }
    return target in allowed[current]


def status_report(root: Path, ledger: Path | None) -> dict:
    """Validate planning and status claims; never certify verification or acceptance."""
    try:
        resolved_root = root.resolve(strict=True)
    except OSError as exc:
        raise ValueError(f"project root is unavailable: {root}") from exc
    if not resolved_root.is_dir():
        raise ValueError(f"project root is not a directory: {root}")
    # Validate and use the same private snapshot; original worktree changes
    # cannot replace a checked JSON record before it is loaded.
    with tempfile.TemporaryDirectory(prefix="phego-work-status-") as temporary:
        snapshot = Path(temporary).resolve()
        _snapshot_tree(resolved_root, snapshot)
        fingerprints = _plan_fingerprints(snapshot)
        _validate_planning(snapshot)
        if _plan_fingerprints(snapshot) != fingerprints:
            raise ValueError("planning inputs changed during validation")
        packages, prerequisites = _load_plan(snapshot)
        if _plan_fingerprints(snapshot) != fingerprints:
            raise ValueError("planning inputs changed during loading")
    statuses = {package: "not_started" for package in packages}
    events: list[dict] = []
    evidence_cache = {}
    stale_claims: set[str] = set()
    dependents = {package: set() for package in packages}
    for package, links in prerequisites.items():
        for dependency in links["dependencies"] + links["integration_dependencies"]:
            dependents[dependency].add(package)
    if ledger is not None:
        try:
            fd = os.open(ledger, os.O_RDONLY | os.O_NONBLOCK)
            with os.fdopen(fd, "rb") as stream:
                if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                    raise ValueError("status ledger must be a regular file")
                ledger_bytes = stream.read(_MAX_LEDGER_SIZE + 1)
            if len(ledger_bytes) > _MAX_LEDGER_SIZE:
                raise ValueError("status ledger exceeds 1 MiB")
            data = json.loads(ledger_bytes.decode("utf-8"), object_pairs_hook=_unique_object)
        except (OSError, UnicodeError, json.JSONDecodeError, RecursionError) as exc:
            raise ValueError(f"cannot read status ledger: {exc}") from exc
        if not isinstance(data, dict) or set(data) != {"events"} or not isinstance(data["events"], list):
            raise ValueError("status ledger must be an object containing only an events array")
        if len(data["events"]) > _MAX_EVENTS:
            raise ValueError("status ledger exceeds 10000 events")
        for number, event in enumerate(data["events"], 1):
            if not isinstance(event, dict) or set(event) != {"id", "status", "reason", "evidence"}:
                raise ValueError(f"event {number} must contain exactly id, status, reason, and evidence")
            package, target, reason = event["id"], event["status"], event["reason"]
            if not isinstance(package, str) or package not in packages:
                raise ValueError(f"event {number}: unknown work-package ID {package!r}")
            if not isinstance(target, str) or target not in _STATUSES:
                raise ValueError(f"{package}: invalid status {target!r}")
            if target == "not_started":
                raise ValueError(f"{package}: not_started is the plan default, not an event status")
            if not isinstance(reason, str) or not reason.strip():
                raise ValueError(f"{package}: reason must be nonempty")
            _validate_evidence(resolved_root, event["evidence"], evidence_cache)
            current = statuses[package]
            if not _transition_allowed(current, target):
                raise ValueError(f"{package}: invalid status transition {current} -> {target}")
            if current == "claimed_complete" and target == "in_progress":
                pending = list(dependents[package])
                visited = set()
                while pending:
                    dependent = pending.pop()
                    if dependent in visited:
                        continue
                    visited.add(dependent)
                    if statuses[dependent] == "claimed_complete":
                        stale_claims.add(dependent)
                    pending.extend(dependents[dependent])
                stale_claims.discard(package)
            if target == "claimed_complete":
                missing = [dep for dep in prerequisites[package]["dependencies"] + prerequisites[package]["integration_dependencies"] if statuses[dep] != "claimed_complete" or dep in stale_claims]
                if missing:
                    raise ValueError(f"{package}: prerequisites are not claimed_complete: {', '.join(missing)}")
            statuses[package] = target
            events.append({"id": package, "status": target, "reason": reason, "evidence": dict(event["evidence"])})

    if stale_claims:
        raise ValueError("downstream completion claims require reopening: " + ", ".join(sorted(stale_claims)))
    for package, status in statuses.items():
        if status == "claimed_complete":
            incomplete = [dep for dep in prerequisites[package]["dependencies"] + prerequisites[package]["integration_dependencies"] if statuses[dep] != "claimed_complete"]
            if incomplete:
                raise ValueError(f"{package}: completed downstream claim has reopened prerequisites: {', '.join(incomplete)}")
    return {
        "statuses": statuses,
        "prerequisites": prerequisites,
        "events": events,
        "planning_input_sha256": fingerprints,
        "status_basis": "planning_defaults_and_unverified_claims",
        "verification_status": "not_verified",
        "claim_notice": "工作包狀態是有 Git 證據的主張，尚未獨立驗證。",
        "product_acceptance": "not_claimed",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tools/work_status.py --ledger claims.json. Invalid input exits 2; output does not certify acceptance.")
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="project root")
    parser.add_argument("--ledger", type=Path, help="optional JSON status-claim ledger")
    args = parser.parse_args()
    try:
        result = status_report(args.root, args.ledger)
    except (OSError, TypeError, ValueError) as exc:
        print(f"work_status: error: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
