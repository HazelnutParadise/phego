"""Compare supplied PHPT execution observations without asserting provenance."""
import base64
import json
import re
from dataclasses import dataclass
from typing import Optional

MAX_NUMERIC_CHARS = 4096
# 10**4096 needs fewer than 13609 bits; reject larger values before str().
MAX_NUMERIC_BITS = 13608


def _bounded_integer(value):
    if value.bit_length() > MAX_NUMERIC_BITS:
        raise ValueError("integer exceeds 4096 characters")
    try:
        length = len(str(value))
    except ValueError as exc:
        raise ValueError("integer exceeds the interpreter integer conversion limit") from exc
    if length > MAX_NUMERIC_CHARS:
        raise ValueError("integer exceeds 4096 characters")


@dataclass(frozen=True)
class Observation:
    test_id: str
    test_sha256: str
    release: str
    platform: str
    build: str
    engine: str
    status: str
    stdout: bytes
    stderr: bytes
    exit_code: Optional[int]
    effects: dict

def compare(reference: Observation, candidate: Observation, *, normalize_crlf: bool = False) -> dict:
    """Compare exact-profile observations; unexecuted cases remain incomparable."""
    if type(normalize_crlf) is not bool:
        raise ValueError("normalize_crlf must be a boolean")
    allowed = {"completed", "not_run", "unsupported", "environment_skip", "runner_gap", "timeout"}
    def snapshot(observation):
        if not isinstance(observation, Observation):
            raise ValueError("observation has invalid type")
        for field in ("test_id", "release", "platform", "build", "engine"):
            value = getattr(observation, field)
            if not isinstance(value, str) or not value.strip() or len(value) > 512:
                raise ValueError(f"invalid observation {field}")
        if not isinstance(observation.test_sha256, str) or not re.fullmatch(r"[0-9a-f]{64}", observation.test_sha256):
            raise ValueError("test SHA-256 must be 64 lowercase hexadecimal characters")
        if not isinstance(observation.status, str) or observation.status not in allowed:
            raise ValueError("invalid observation status")
        if observation.status == "completed" and (type(observation.exit_code) is not int or observation.exit_code < 0):
            raise ValueError("completed observation requires a nonnegative exit code")
        if observation.status != "completed" and observation.exit_code is not None and type(observation.exit_code) is not int:
            raise ValueError("partial exit code must be an integer or absent")
        if type(observation.exit_code) is int:
            _bounded_integer(observation.exit_code)
        if not isinstance(observation.stdout, bytes) or not isinstance(observation.stderr, bytes):
            raise ValueError("output must be bytes")
        if len(observation.stdout) + len(observation.stderr) > 4 * 1024 * 1024:
            raise ValueError("combined output exceeds 4 MiB")
        if not isinstance(observation.effects, dict):
            raise ValueError("effects must be a JSON object")
        try:
            size = 0
            def charge(amount):
                nonlocal size
                size += amount
                if size > 1024 * 1024:
                    raise ValueError("effects exceed 1 MiB")
            def validate_keys(value, depth=0):
                if depth > 100:
                    raise ValueError("effects nesting exceeds limit")
                if isinstance(value, dict):
                    charge(2 + max(0, len(value) - 1) * 2)
                    for key, child in value.items():
                        if not isinstance(key, str) or len(key) > 1024 * 1024:
                            raise ValueError("effects object keys must be bounded strings")
                        charge(len(json.dumps(key, ensure_ascii=True)) + 2)
                        validate_keys(child, depth + 1)
                elif isinstance(value, list):
                    charge(2 + max(0, len(value) - 1) * 2)
                    for child in value: validate_keys(child, depth + 1)
                elif value is None or type(value) in (str, int, float, bool):
                    if isinstance(value, str) and len(value) > 1024 * 1024:
                        raise ValueError("effects string exceeds limit")
                    if type(value) is int:
                        _bounded_integer(value)
                    charge(len(json.dumps(value, ensure_ascii=True, allow_nan=False)))
                else:
                    raise ValueError("effects contain a non-JSON value")
            validate_keys(observation.effects)
            encoded = json.dumps(observation.effects, ensure_ascii=True, sort_keys=True, allow_nan=False).encode("ascii")
            if len(encoded) > 1024 * 1024:
                raise ValueError("effects exceed 1 MiB")
            effects = json.loads(encoded)
        except (TypeError, ValueError, RecursionError) as exc:
            if str(exc) == "integer exceeds the interpreter integer conversion limit":
                raise ValueError("effects integer exceeds the interpreter integer conversion limit") from exc
            raise ValueError("effects must be a bounded JSON object") from exc
        return {"test_id":observation.test_id, "test_sha256":observation.test_sha256,
                "release":observation.release, "platform":observation.platform, "build":observation.build,
                "engine":observation.engine, "status":observation.status, "exit_code":observation.exit_code,
                "stdout_base64":base64.b64encode(observation.stdout).decode("ascii"),
                "stderr_base64":base64.b64encode(observation.stderr).decode("ascii"), "effects":effects}
    ref, cand = snapshot(reference), snapshot(candidate)
    for key in ("test_id", "test_sha256", "release", "platform"):
        if ref[key] != cand[key]:
            raise ValueError(f"observation {key} differs")
    result = {"status":"incomparable", "basis":"supplied_observations", "product_acceptance":"not_claimed",
              "reference_status":ref["status"], "candidate_status":cand["status"],
              "reference":ref, "candidate":cand, "differences":[],
              "normalizations":["crlf_to_lf"] if normalize_crlf else []}
    if ref["status"] != "completed" or cand["status"] != "completed":
        return result
    for key in ("stdout", "stderr"):
        left, right = getattr(reference, key), getattr(candidate, key)
        if normalize_crlf:
            left, right = left.replace(b"\r\n", b"\n"), right.replace(b"\r\n", b"\n")
        if left != right:
            result["differences"].append(key)
    if ref["exit_code"] != cand["exit_code"]:
        result["differences"].append("exit_code")
    if json.dumps(ref["effects"], sort_keys=True, ensure_ascii=True) != json.dumps(cand["effects"], sort_keys=True, ensure_ascii=True):
        result["differences"].append("effects")
    result["status"] = "mismatch" if result["differences"] else "match"
    return result
