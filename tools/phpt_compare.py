#!/usr/bin/env python3
"""Compare supplied PHPT observations; this command does not execute tests."""
import argparse
import base64
import binascii
import json
import math
from decimal import Decimal, DecimalException
import os
import stat
import sys

from phpt_diff import Observation, compare


MAX_INPUT_BYTES = 8 * 1024 * 1024
FIELDS = {
    "test_id", "test_sha256", "release", "platform", "build", "engine",
    "status", "stdout_base64", "stderr_base64", "exit_code", "effects",
}


class InputError(ValueError):
    """An invalid observation input with a safe, actionable diagnostic."""


class SafeArgumentParser(argparse.ArgumentParser):
    """Report argument errors without echoing untrusted command-line text."""

    def error(self, message):
        if message.startswith("the following arguments are required:"):
            detail = "reference and candidate observation files are required"
        elif message.startswith("unrecognized arguments:"):
            detail = "unrecognized argument; use --help for valid options"
        else:
            detail = "invalid arguments; use --help for usage"
        self.exit(2, f"phpt-compare: error: {detail}\n")


def _safe_path(path):
    """Escape terminal controls and keep a path diagnostic bounded."""
    rendered = repr(os.fspath(path))
    if len(rendered) > 160:
        rendered = rendered[:157] + "..."
    return rendered


def _unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            if key in FIELDS:
                raise ValueError(f"duplicate JSON key {key}")
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def _reject_constant(value):
    raise ValueError("non-standard JSON constant is not allowed")


def _bounded_int(token):
    if len(token) > 4096:
        raise ValueError("JSON number exceeds 4096 characters")
    try:
        return int(token)
    except ValueError as exc:
        raise ValueError("JSON number exceeds the interpreter integer conversion limit") from exc


def _bounded_float(token):
    if len(token) > 4096:
        raise ValueError("JSON number exceeds 4096 characters")
    try:
        value = float(token)
        if not math.isfinite(value) or Decimal(token) != Decimal(repr(value)):
            raise ValueError("JSON number loses precision or is out of range")
        return value
    except (OverflowError, DecimalException) as exc:
        raise ValueError("JSON number loses precision or is out of range") from exc


def _read_input(path):
    if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_NONBLOCK"):
        raise InputError(f"{_safe_path(path)}: platform lacks required safe open flags O_NOFOLLOW and O_NONBLOCK")
    flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
    fd = None
    try:
        fd = os.open(path, flags)
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode):
            raise InputError(f"{_safe_path(path)}: input must be a regular file (symlinks and FIFOs are not accepted)")
        if info.st_size > MAX_INPUT_BYTES:
            raise InputError(f"{_safe_path(path)}: observation JSON exceeds 8 MiB maximum")
        chunks = []
        remaining = MAX_INPUT_BYTES + 1
        while remaining:
            chunk = os.read(fd, min(65536, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        data = b"".join(chunks)
        if len(data) > MAX_INPUT_BYTES:
            raise InputError(f"{_safe_path(path)}: observation JSON exceeds 8 MiB maximum")
        return data
    except InputError:
        raise
    except OSError as exc:
        cause = exc.strerror or "operating system error"
        raise InputError(f"{_safe_path(path)}: cannot safely read input: {cause}") from exc
    finally:
        if fd is not None:
            os.close(fd)


def load_observation(path):
    """Read a bounded observation JSON and convert its base64 output to bytes."""
    try:
        raw = _read_input(path)
        try:
            text = raw.decode("utf-8", errors="strict")
        except UnicodeDecodeError as exc:
            raise ValueError(f"malformed UTF-8 at byte {exc.start}") from exc
        try:
            value = json.loads(text, object_pairs_hook=_unique_object, parse_constant=_reject_constant, parse_int=_bounded_int, parse_float=_bounded_float)
        except (json.JSONDecodeError, ValueError, RecursionError) as exc:
            if isinstance(exc, json.JSONDecodeError):
                detail = f" at line {exc.lineno} column {exc.colno}"
            elif str(exc) == "duplicate JSON key":
                detail = ": duplicate JSON key"
            elif str(exc).startswith("duplicate JSON key ") and str(exc)[len("duplicate JSON key "):] in FIELDS:
                detail = f": {exc}"
            elif str(exc) == "non-standard JSON constant is not allowed":
                detail = ": non-standard JSON constant"
            elif str(exc) in ("JSON number exceeds 4096 characters", "JSON number loses precision or is out of range", "JSON number exceeds the interpreter integer conversion limit"):
                detail = f": {exc}"
            else:
                detail = ": invalid structure"
            raise ValueError("invalid JSON" + detail) from exc
        if not isinstance(value, dict):
            raise ValueError("top-level JSON value must be an object")
        missing, unknown = FIELDS - value.keys(), value.keys() - FIELDS
        if missing:
            raise ValueError("missing required field(s): " + ", ".join(sorted(missing)))
        if unknown:
            raise ValueError("unknown field(s)")
        for field in ("stdout_base64", "stderr_base64"):
            encoded = value[field]
            if not isinstance(encoded, str):
                raise ValueError(f"{field} must be a base64 string")
            try:
                value[field] = base64.b64decode(encoded, validate=True)
            except (binascii.Error, ValueError) as exc:
                raise ValueError(f"{field} contains invalid strict base64") from exc
        return Observation(
            test_id=value["test_id"], test_sha256=value["test_sha256"],
            release=value["release"], platform=value["platform"],
            build=value["build"], engine=value["engine"], status=value["status"],
            stdout=value["stdout_base64"], stderr=value["stderr_base64"],
            exit_code=value["exit_code"], effects=value["effects"],
        )
    except (InputError, ValueError, TypeError, RecursionError) as exc:
        raise InputError(f"{_safe_path(path)}: {exc}") from exc


def main():
    parser = SafeArgumentParser(
        description="Compare two supplied PHPT observation JSON files; this tool does not execute tests.",
        epilog=(
            "Each file is at most 8 MiB and contains exactly these fields: test_id, test_sha256, "
            "release, platform, build, engine, status, stdout_base64, stderr_base64, exit_code, effects.\n"
            "JSON numbers are limited to 4096 characters; decimals must survive float conversion.\n"
            "Output is a JSON comparison report. Exit codes: 0=match, 1=mismatch, 2=invalid input, "
            "3=incomparable. CRLF normalization is opt-in.\n"
            "Example: python3 tools/phpt_compare.py reference.json candidate.json"
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("reference", help="reference observation JSON file")
    parser.add_argument("candidate", help="candidate observation JSON file")
    parser.add_argument("--normalize-crlf", action="store_true", help="compare CRLF and LF as equivalent")
    args = parser.parse_args()
    try:
        reference = load_observation(args.reference)
        candidate = load_observation(args.candidate)
        try:
            result = compare(reference, candidate, normalize_crlf=args.normalize_crlf)
        except (ValueError, TypeError, RecursionError) as exc:
            detail = str(exc)
            # These messages are fixed in phpt_diff and contain no input data.
            safe_messages = {
                "normalize_crlf must be a boolean",
                "observation has invalid type",
                "test SHA-256 must be 64 lowercase hexadecimal characters",
                "invalid observation status",
                "completed observation requires a nonnegative exit code",
                "partial exit code must be an integer or absent",
                "output must be bytes",
                "combined output exceeds 4 MiB",
                "effects must be a JSON object",
                "effects must be a bounded JSON object",
                "integer exceeds 4096 characters",
                "integer exceeds the interpreter integer conversion limit",
                "effects integer exceeds the interpreter integer conversion limit",
            }
            safe_detail = detail if detail in safe_messages else "comparison failed"
            for field in FIELDS:
                if detail in (f"observation {field} differs", f"invalid observation {field}"):
                    safe_detail = detail
                    break
            raise InputError(
                f"{_safe_path(args.reference)} and {_safe_path(args.candidate)}: {safe_detail}"
            ) from exc
        print(json.dumps(result, ensure_ascii=True, sort_keys=True, indent=2))
        return {"match": 0, "mismatch": 1, "incomparable": 3}[result["status"]]
    except (OSError, ValueError, TypeError, RecursionError, KeyError) as exc:
        message = str(exc)
        # Escape any terminal control characters in diagnostics and cap the
        # total output even if a lower layer supplies an unexpectedly long error.
        message = "".join(ch if ch.isprintable() else f"\\u{ord(ch):04x}" for ch in message)
        print(f"phpt-compare: error: {message[:512]}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
