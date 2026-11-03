#!/usr/bin/env python3
"""Compare evidence schema and traceability validation using synthetic inputs.

Requires jsonschema 4.25.1. Runs offline once that test dependency is available.
"""
import argparse
import copy
import hashlib
import json
import tempfile
from pathlib import Path

import traceability

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--schema", type=Path, default=ROOT / "docs/data/evidence-records.schema.json", help="schema to check")
    args = parser.parse_args()
    try:
        import jsonschema
    except ImportError:
        parser.error("the test dependency jsonschema 4.25.1 is required")
    try:
        schema = json.loads(args.schema.read_text(encoding="utf-8"))
        jsonschema.Draft202012Validator.check_schema(schema)
    except (OSError, UnicodeError, ValueError, jsonschema.exceptions.SchemaError) as exc:
        parser.error(f"invalid schema: {exc}")
    validator = jsonschema.Draft202012Validator(schema)
    with tempfile.TemporaryDirectory(prefix="phego-schema-") as temporary:
        root = Path(temporary)
        (root / "docs/data").mkdir(parents=True)
        (root / "docs/data/phego-acceptance-tests.json").write_text(json.dumps({"items": [{"id": "T-001"}]}))
        data = b"synthetic evidence\n"
        (root / "evidence.log").write_bytes(data)
        valid = {"test_id": "T-001", "status": "pass", "commit": "a" * 40, "command": "test",
                 "profile": {"release": "8.5.12", "platform": "linux/arm64", "build": "cli"},
                 "exit_code": 0, "evidence": {"path": "evidence.log", "sha256": hashlib.sha256(data).hexdigest()}}
        cases = {}
        for status in ("pass", "fail", "crash", "timeout", "unsupported", "skip", "environment_skip", "runner_error", "semantic_difference"):
            record = copy.deepcopy(valid)
            record["status"] = status
            if status in ("crash", "timeout", "unsupported", "skip", "environment_skip", "runner_error"):
                record["exit_code"] = None
            cases["valid_" + status] = {"records": [record]}
        cases["valid_not_run"] = {"records": [{"test_id": "T-001", "status": "not_run"}]}
        for name, field, value in (("bad_commit", "commit", "X" * 40), ("bad_pass_exit", "exit_code", 1),
                                   ("bad_profile", "profile", {"release": "8.5.12"}),
                                   ("bad_evidence", "evidence", {"path": "evidence.log", "sha256": "zzz"}),
                                   ("blank_command", "command", "   ")):
            record = copy.deepcopy(valid)
            record[field] = value
            cases[name] = {"records": [record]}
        cases["bad_unrun"] = {"records": [{"test_id": "T-001", "status": "not_run", "command": "test"}]}
        cases["bad_outcome"] = {"records": [dict(valid, status="unknown")]}
        mismatches = []
        for name, payload in cases.items():
            result_path = root / "records.json"
            result_path.write_text(json.dumps(payload))
            try:
                traceability.report(root, result_path)
                accepted = True
            except (TypeError, ValueError):
                accepted = False
            expected = name.startswith("valid_")
            if validator.is_valid(payload) != expected or accepted != expected:
                mismatches.append(name)
        if mismatches:
            print("schema/validator mismatch: " + ", ".join(mismatches))
            return 1
        print(f"Matched {len(cases)} synthetic cases; product acceptance not claimed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
