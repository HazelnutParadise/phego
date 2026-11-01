#!/usr/bin/env python3
"""Compare two execution-record snapshots without certifying product acceptance."""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

from traceability import report as traceability_report


_NONRUN_CLASSIFICATIONS = {"skip", "environment_skip", "unsupported"}
_PROTECTED_OUTCOMES = {
    "pass",
    "fail",
    "crash",
    "timeout",
    "runner_error",
    "semantic_difference",
    "unsupported",
}


def compare(root: Path, previous: Path, current: Path) -> dict:
    """Compare validated snapshots, preserving both records and every test ID.

    Reject mismatched profiles and changes that hide prior failures, runner
    gaps, semantic differences or passes as a non-executed classification.
    Allow a failure to be rerun at a newer commit but retain its prior record.
    Neither a valid digest nor a reported pass is product certification.
    Invalid snapshots or unsafe transitions raise ValueError.
    """
    previous_report = traceability_report(root, previous)
    current_report = traceability_report(root, current)

    previous_results = previous_report["results"]
    current_results = current_report["results"]
    previous_ids = set(previous_results)
    current_ids = set(current_results)
    if previous_ids != current_ids:
        missing = sorted(previous_ids - current_ids)
        added = sorted(current_ids - previous_ids)
        details = []
        if missing:
            details.append(f"missing from current report: {', '.join(missing)}")
        if added:
            details.append(f"added to current report: {', '.join(added)}")
        raise ValueError("snapshot test ID sets do not match (" + "; ".join(details) + ")")

    previous_profile = previous_report["profile"]
    current_profile = current_report["profile"]
    if previous_profile is not None and current_profile is not None and previous_profile != current_profile:
        raise ValueError("snapshots must use the same exact profile")

    changes = {}
    for test_id in previous_results:
        old_status = previous_results[test_id]
        new_status = current_results[test_id]
        if old_status != "not_run" and new_status == "not_run":
            raise ValueError(f"{test_id}: previously classified result cannot become not_run")
        if (
            old_status in _PROTECTED_OUTCOMES
            and new_status in _NONRUN_CLASSIFICATIONS
            and new_status != old_status
        ):
            raise ValueError(f"{test_id}: {old_status} cannot be reclassified as {new_status}")
        if old_status in {"fail", "crash", "timeout", "semantic_difference", "unsupported"} and new_status == "runner_error":
            raise ValueError(f"{test_id}: {old_status} cannot be hidden by runner_error")
        changes[test_id] = {"previous": old_status, "current": new_status}

    return {
        "changes": changes,
        "current": current_report,
        "previous": previous_report,
        "product_acceptance": "not_claimed",
    }


def main() -> int:
    parser = argparse.ArgumentParser(
        description=__doc__,
        epilog="Example: python3 tools/compare_evidence.py --previous before.json --current after.json. "
        "Invalid inputs or prohibited transitions exit 2; output does not certify test outcomes.",
    )
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1], help="project root")
    parser.add_argument("--previous", type=Path, required=True, help="previous execution-record JSON")
    parser.add_argument("--current", type=Path, required=True, help="current execution-record JSON")
    args = parser.parse_args()
    try:
        output = compare(args.root, args.previous, args.current)
    except (OSError, TypeError, ValueError) as exc:
        print(f"compare_evidence: error: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(output, ensure_ascii=False, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
