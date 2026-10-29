#!/usr/bin/env python3
"""Inspect execution records without claiming product certification."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import sys
from pathlib import Path


_OUTCOMES = (
    "pass",
    "fail",
    "crash",
    "timeout",
    "unsupported",
    "skip",
    "environment_skip",
    "runner_error",
    "semantic_difference",
    "not_run",
)
_PROVENANCE_FIELDS = {"commit", "command", "profile", "exit_code", "evidence"}
_PROFILE_FIELDS = ("release", "platform", "build")


def _load_json(path: Path, label: str) -> object:
    def unique_keys(pairs: list[tuple[str, object]]) -> dict:
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key: {key}")
            result[key] = value
        return result

    try:
        with path.open("r", encoding="utf-8") as stream:
            return json.load(stream, object_pairs_hook=unique_keys)
    except (OSError, UnicodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ValueError(f"cannot read {label} JSON at {path}: {exc}") from exc


def _registry_ids(root: Path) -> list[str]:
    registry_path = root / "docs/data/phego-acceptance-tests.json"
    registry = _load_json(registry_path, "acceptance registry")
    if not isinstance(registry, dict) or not isinstance(registry.get("items"), list):
        raise ValueError("acceptance registry must be an object with an items list")

    ids: list[str] = []
    seen: set[str] = set()
    for index, item in enumerate(registry["items"]):
        if not isinstance(item, dict) or not isinstance(item.get("id"), str) or not item["id"]:
            raise ValueError(f"acceptance registry item {index} has no valid id")
        test_id = item["id"]
        if test_id in seen:
            raise ValueError(f"duplicate test id in acceptance registry: {test_id}")
        seen.add(test_id)
        ids.append(test_id)
    return ids


def _evidence_digest(root: Path, evidence: object, test_id: str) -> str:
    if not isinstance(evidence, dict):
        raise ValueError(f"{test_id}: evidence must be an object")
    relative = evidence.get("path")
    digest = evidence.get("sha256")
    if not isinstance(relative, str) or not relative or not isinstance(digest, str):
        raise ValueError(f"{test_id}: evidence requires a relative path and sha256")
    if not re.fullmatch(r"[0-9a-f]{64}", digest):
        raise ValueError(f"{test_id}: evidence sha256 must be 64 lowercase hex characters")

    path = Path(relative)
    if path.is_absolute() or ".." in path.parts or not path.parts:
        raise ValueError(f"{test_id}: evidence path must stay within root")

    # Open each component relative to an already opened directory.  O_NOFOLLOW
    # prevents a concurrent symlink replacement from escaping the project root.
    descriptors = []
    try:
        current = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        descriptors.append(current)
        for part in path.parts[:-1]:
            current = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=current)
            descriptors.append(current)
        fd = os.open(path.parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=current)
        descriptors.append(fd)
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            raise ValueError(f"{test_id}: evidence must be a regular file: {relative}")
        digest_state = hashlib.sha256()
        while True:
            block = os.read(fd, 1024 * 1024)
            if not block:
                break
            digest_state.update(block)
        actual = digest_state.hexdigest()
    except OSError as exc:
        raise ValueError(f"{test_id}: cannot safely read evidence file {relative}: {exc}") from exc
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)
    if actual != digest:
        raise ValueError(f"{test_id}: evidence sha256 does not match {relative}")
    return actual


def _validate_record(record: object, known_ids: set[str], root: Path) -> tuple[str, str, tuple[str, str, str] | None, str | None]:
    if not isinstance(record, dict):
        raise ValueError("each execution record must be an object")
    test_id = record.get("test_id")
    status = record.get("status")
    if not isinstance(test_id, str) or test_id not in known_ids:
        raise ValueError(f"unknown or invalid test id: {test_id!r}")
    if status not in _OUTCOMES:
        raise ValueError(f"{test_id}: invalid status: {status!r}")

    if status == "not_run":
        if _PROVENANCE_FIELDS.intersection(record):
            raise ValueError(f"{test_id}: not_run records cannot include execution provenance")
        return test_id, status, None, None

    commit = record.get("commit")
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError(f"{test_id}: commit must be a 40-character lowercase hex hash")
    command = record.get("command")
    if not isinstance(command, str) or not command.strip():
        raise ValueError(f"{test_id}: command must be a nonempty string")
    profile = record.get("profile")
    if not isinstance(profile, dict) or set(profile) != set(_PROFILE_FIELDS):
        raise ValueError(f"{test_id}: profile must contain exactly release, platform, and build")
    profile_values: list[str] = []
    for field in _PROFILE_FIELDS:
        value = profile.get(field)
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"{test_id}: profile.{field} must be a nonempty string")
        profile_values.append(value)

    exit_code = record.get("exit_code")
    no_exit = {"crash", "timeout", "runner_error", "skip", "environment_skip", "unsupported"}
    if "exit_code" not in record or (exit_code is None and status not in no_exit) or (
        exit_code is not None and (isinstance(exit_code, bool) or not isinstance(exit_code, int))
    ):
        raise ValueError(f"{test_id}: exit_code must be an integer or null for an interrupted or unexecuted case")
    if status == "pass" and exit_code != 0:
        raise ValueError(f"{test_id}: pass requires exit_code 0")
    _evidence_digest(root, record.get("evidence"), test_id)
    return test_id, status, tuple(profile_values), commit


def report(root: Path, results: Path | None) -> dict:
    """Return traceable results or raise ValueError for invalid evidence.

    Load the existing planning registry; preserve all test IDs in the report.
    Reject unknown or duplicate IDs, invalid outcomes, missing provenance, and
    missing or mismatched evidence. Never infer product acceptance from records.
    """
    try:
        resolved_root = root.resolve(strict=True)
    except OSError as exc:
        raise ValueError(f"project root is unavailable: {root}") from exc
    if not resolved_root.is_dir():
        raise ValueError(f"project root is not a directory: {root}")
    ids = _registry_ids(resolved_root)
    statuses = {test_id: "not_run" for test_id in ids}

    records = []
    baseline_profile = None
    baseline_commit = None
    if results is not None:
        payload = _load_json(results, "results")
        if not isinstance(payload, dict) or not isinstance(payload.get("records"), list):
            raise ValueError("results JSON must be an object with a records list")
        known_ids = set(ids)
        seen: set[str] = set()
        records = payload["records"]
        for record in records:
            test_id, status, profile, commit = _validate_record(record, known_ids, resolved_root)
            if test_id in seen:
                raise ValueError(f"duplicate test id in results: {test_id}")
            seen.add(test_id)
            if status != "not_run":
                if baseline_profile is None:
                    baseline_profile, baseline_commit = profile, commit
                elif profile != baseline_profile:
                    raise ValueError(f"{test_id}: all executed records must use the same exact profile")
                elif commit != baseline_commit:
                    raise ValueError(f"{test_id}: all executed records must use the same commit")
            statuses[test_id] = status

    counts = {status: 0 for status in _OUTCOMES}
    for status in statuses.values():
        counts[status] += 1
    return {
        "counts": counts,
        "commit": baseline_commit,
        "profile": dict(zip(_PROFILE_FIELDS, baseline_profile)) if baseline_profile else None,
        "execution_records": records,
        "product_acceptance": "not_claimed",
        "results": statuses,
    }


def main() -> int:
    parser = argparse.ArgumentParser(
        description=__doc__,
        epilog="Examples: python3 tools/traceability.py; python3 tools/traceability.py --results records.json. "
        "Invalid input exits 2. Records are reported as supplied; digest checks do not certify execution.",
    )
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="project root (default: repository containing this tool)")
    parser.add_argument("--results", type=Path, help="JSON execution records to check (optional; omitted tests are not_run)")
    args = parser.parse_args()
    try:
        output = report(args.root, args.results)
    except (OSError, TypeError, ValueError) as exc:
        print(f"traceability: error: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(output, ensure_ascii=False, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
