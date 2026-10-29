#!/usr/bin/env python3
"""Check capability claims against the recorded test outcomes without certifying them."""
from __future__ import annotations
import argparse
import json
import os
import stat
import sys
import tempfile
from pathlib import Path
import traceability


_MAX_INPUT = 1024 * 1024
_MAX_EVIDENCE = 64 * 1024 * 1024
_REGISTRY = Path("docs/data/phego-acceptance-tests.json")
_PROFILE_FIELDS = {"release", "platform", "build"}
_KINDS = {"function", "class", "constant", "extension", "ini", "sapi"}
_STATUSES = {"supported", "unsupported", "unverified", "policy_denied", "semantic_difference", "failed"}


def _read_regular(path: Path, label: str, limit: int = _MAX_INPUT) -> bytes:
    """Read one bounded regular file without following a symlink."""
    descriptor = None
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        if not stat.S_ISREG(os.fstat(descriptor).st_mode):
            raise ValueError(f"{label} must be a regular file")
        chunks = []
        size = 0
        while True:
            block = os.read(descriptor, min(65536, limit + 1 - size))
            if not block:
                break
            chunks.append(block)
            size += len(block)
            if size > limit:
                raise ValueError(f"{label} exceeds {limit} bytes")
        return b"".join(chunks)
    except OSError as exc:
        raise ValueError(f"cannot safely read {label}: {exc}") from exc
    finally:
        if descriptor is not None:
            os.close(descriptor)


def _read_beneath(root: Path, relative: str, limit: int) -> bytes:
    """Read a bounded regular evidence file using no-follow directory handles."""
    path = Path(relative)
    if path.is_absolute() or ".." in path.parts or not path.parts:
        raise ValueError("evidence path must stay within root")
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
            raise ValueError(f"evidence must be a regular file: {relative}")
        if os.fstat(fd).st_size > limit:
            raise ValueError(f"evidence exceeds {limit} bytes: {relative}")
        chunks = []
        size = 0
        while True:
            block = os.read(fd, min(65536, limit + 1 - size))
            if not block:
                break
            chunks.append(block)
            size += len(block)
            if size > limit:
                raise ValueError(f"evidence exceeds {limit} bytes: {relative}")
        return b"".join(chunks)
    except OSError as exc:
        raise ValueError(f"cannot safely read evidence file {relative}: {exc}") from exc
    finally:
        for descriptor in reversed(descriptors):
            os.close(descriptor)


def _write_snapshot_file(path: Path, contents: bytes, label: str) -> None:
    """Create a snapshot file atomically, rejecting filesystem aliases."""
    descriptor = None
    try:
        descriptor = os.open(
            path,
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
            0o600,
        )
        view = memoryview(contents)
        while view:
            written = os.write(descriptor, view)
            view = view[written:]
    except FileExistsError as exc:
        raise ValueError(f"snapshot path collision for {label}: {path}") from exc
    except OSError as exc:
        raise ValueError(f"cannot create snapshot file for {label}: {path}: {exc}") from exc
    finally:
        if descriptor is not None:
            os.close(descriptor)


def _traceability_snapshot(root: Path, results_bytes: bytes | None) -> tuple[dict, set[str]]:
    """Run the existing verifier against immutable, bounded input snapshots."""
    registry_bytes = _read_beneath(root, _REGISTRY.as_posix(), _MAX_INPUT)
    records = []
    if results_bytes is not None:
        payload = _json_bytes(results_bytes, "results")
        if isinstance(payload, dict) and isinstance(payload.get("records"), list):
            records = payload["records"]
    evidence = {}
    total_evidence_size = 0
    registry_key = _REGISTRY.as_posix()
    for record in records:
        if isinstance(record, dict) and record.get("status") != "not_run":
            item = record.get("evidence")
            if isinstance(item, dict) and isinstance(item.get("path"), str):
                relative = item["path"]
                path = Path(relative)
                if path.is_absolute() or ".." in path.parts or not path.parts:
                    raise ValueError("evidence path must stay within root")
                normalized = path.as_posix()
                if normalized == registry_key:
                    raise ValueError("evidence path conflicts with acceptance registry")
                if normalized not in evidence:
                    remaining = _MAX_EVIDENCE - total_evidence_size
                    if remaining < 0:
                        raise ValueError(f"total evidence exceeds {_MAX_EVIDENCE} bytes")
                    contents = _read_beneath(root, normalized, remaining)
                    evidence[normalized] = contents
                    total_evidence_size += len(contents)

    with tempfile.TemporaryDirectory() as directory:
        snapshot_root = Path(directory) / "root"
        registry_path = snapshot_root / _REGISTRY
        try:
            registry_path.parent.mkdir(parents=True)
            _write_snapshot_file(registry_path, registry_bytes, "acceptance registry")
            for relative, contents in evidence.items():
                target = snapshot_root / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                _write_snapshot_file(target, contents, f"evidence {relative}")
            results_path = None
            if results_bytes is not None:
                results_path = Path(directory) / "results.json"
                _write_snapshot_file(results_path, results_bytes, "results")
        except OSError as exc:
            raise ValueError(f"cannot construct validation snapshot: {exc}") from exc
        checked = traceability.report(snapshot_root, results_path)
        return checked, set(checked["results"])


def _json_bytes(raw: bytes, label: str) -> object:
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"{label} contains duplicate JSON key: {key}")
            result[key] = value
        return result

    try:
        return json.loads(raw.decode("utf-8"), object_pairs_hook=unique)
    except (UnicodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ValueError(f"invalid {label} JSON: {exc}") from exc


def _string(value: object, maximum: int, label: str, nonblank: bool = True) -> bool:
    return isinstance(value, str) and len(value) <= maximum and (not nonblank or bool(value.strip()))


def report(root: Path, capabilities: Path, results: Path | None) -> dict:
    """Validate versioned claims, profile, IDs, and recorded passing tests.

    The input is an object with schema_version=1, profile (release, platform,
    build), and claims. Each claim has kind, name, status, test_ids, and reason.
    Preserve distinct unsupported, unverified, policy_denied, semantic_difference
    and failed states. Supported claims require nonempty known test IDs with
    recorded pass results for the same profile. Reject duplicate identities,
    duplicate/unknown IDs, missing or malformed inputs, and unbounded input.
    Return claims unchanged and explicitly mark verification and acceptance as
    not verified/not claimed. Raise ValueError on invalid input.
    """
    capability = _json_bytes(_read_regular(capabilities, "capabilities input"), "capabilities")
    if not isinstance(capability, dict) or set(capability) != {"schema_version", "profile", "claims"}:
        raise ValueError("capabilities must contain exactly schema_version, profile, and claims")
    if type(capability["schema_version"]) is not int or capability["schema_version"] != 1:
        raise ValueError("schema_version must be integer 1")
    profile = capability["profile"]
    if not isinstance(profile, dict) or set(profile) != _PROFILE_FIELDS:
        raise ValueError("profile must contain exactly release, platform, and build")
    for field, value in profile.items():
        if not _string(value, 256, f"profile.{field}"):
            raise ValueError(f"profile.{field} must be a nonblank string of at most 256 characters")
    claims = capability["claims"]
    if not isinstance(claims, list) or len(claims) > 10000:
        raise ValueError("claims must be an array with at most 10000 items")

    identities = set()
    required = {"kind", "name", "status", "test_ids", "reason"}
    for index, claim in enumerate(claims):
        label = f"claim {index}"
        if not isinstance(claim, dict) or set(claim) != required:
            raise ValueError(f"{label} must contain exactly kind, name, status, test_ids, and reason")
        if not isinstance(claim["kind"], str) or claim["kind"] not in _KINDS:
            raise ValueError(f"{label} has invalid kind")
        if not _string(claim["name"], 256, f"{label}.name"):
            raise ValueError(f"{label}.name must be a nonblank string of at most 256 characters")
        status = claim["status"]
        if not isinstance(status, str) or status not in _STATUSES:
            raise ValueError(f"{label} has invalid status")
        reason = claim["reason"]
        if not _string(reason, 4096, f"{label}.reason", nonblank=status != "supported"):
            raise ValueError(f"{label}.reason must be a string of at most 4096 characters and nonblank unless supported")
        ids = claim["test_ids"]
        if not isinstance(ids, list) or len(ids) > 10000:
            raise ValueError(f"{label}.test_ids must be an array of at most 10000 IDs")
        if any(not _string(test_id, 256, f"{label}.test_ids") for test_id in ids):
            raise ValueError(f"{label}.test_ids must contain nonblank strings of at most 256 characters")
        if len(set(ids)) != len(ids):
            raise ValueError(f"{label}.test_ids contains duplicate IDs")
        if status == "supported" and not ids:
            raise ValueError(f"{label}: supported claims require at least one test ID")
        identity = (claim["kind"], claim["name"])
        if identity in identities:
            raise ValueError(f"duplicate claim identity: {claim['kind']} {claim['name']}")
        identities.add(identity)

    checked = None
    if results is not None:
        result_bytes = _read_regular(results, "results input")
        checked, registered = _traceability_snapshot(root, result_bytes)
    elif any(claim["status"] == "supported" for claim in claims):
        raise ValueError("results are required for supported claims")
    else:
        _, registered = _traceability_snapshot(root, None)
    for claim in claims:
        ids = claim["test_ids"]
        unknown = [test_id for test_id in ids if test_id not in registered]
        if unknown:
            raise ValueError(f"unknown acceptance test ID: {unknown[0]}")
        if claim["status"] == "supported":
            if checked is None:
                raise ValueError("results are required for supported claims")
            for test_id in ids:
                if checked["results"].get(test_id) != "pass":
                    raise ValueError(f"{test_id}: supported claim requires a recorded pass")
                record = next(record for record in checked["execution_records"] if record["test_id"] == test_id)
                if record["profile"] != profile:
                    raise ValueError(f"{test_id}: execution profile does not match claim profile")

    return {
        "schema_version": 1,
        "profile": profile,
        "claims": claims,
        "verification_status": "not_verified",
        "product_acceptance": "not_claimed",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tools/capabilities.py --capabilities capabilities.json --results records.json. Invalid input exits 2; a valid report does not establish product acceptance.")
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="project root (default: repository containing this tool)")
    parser.add_argument("--capabilities", type=Path, required=True, help="versioned capability claims JSON")
    parser.add_argument("--results", type=Path, help="execution records JSON; required for supported claims")
    args = parser.parse_args()
    try:
        output = report(args.root, args.capabilities, args.results)
    except (OSError, ValueError) as exc:
        print(f"capabilities: error: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(output, ensure_ascii=True, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
