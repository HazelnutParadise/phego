"""Classify supplied PHPT observations without claiming they were executed."""
import base64
import json
import re
import subprocess
import sys
from dataclasses import dataclass
from typing import Optional

@dataclass(frozen=True)
class Observation:
    status: str
    output: bytes = b""
    exit_code: Optional[int] = None
    reason: str = ""

def match_format(expected: bytes, actual: bytes, *, path_separator=None) -> Optional[bool]:
    """Evaluate supported EXPECTF tokens in a bounded helper process."""
    if len(expected) > 1024 * 1024 or len(actual) > 1024 * 1024:
        return None
    tokens = {ord("s"): rb"[^\r\n]+", ord("S"): rb"[^\r\n]*",
              ord("a"): rb".+?", ord("A"): rb".*?", ord("w"): rb"\s*",
              ord("i"): rb"[+-]?\d+", ord("d"): rb"\d+",
              ord("x"): rb"[0-9a-fA-F]+", ord("c"): rb".", ord("0"): rb"\x00",
              ord("f"): rb"[+-]?(?:\d+|(?=\.\d))(?:\.\d+)?(?:[Ee][+-]?\d+)?"}
    pattern = bytearray(); i = 0
    while i < len(expected):
        if expected[i] == ord("%") and i + 1 < len(expected):
            token = expected[i + 1]
            if token == ord("e"):
                if path_separator != b"/":
                    return None
                pattern.extend(re.escape(path_separator)); i += 2; continue
            if token == ord("r"):
                return None  # Raw PCRE needs profile-aware handling.
            if token in tokens:
                pattern.extend(tokens[token]); i += 2; continue
        pattern.extend(re.escape(expected[i:i + 1])); i += 1
    payload = json.dumps({"pattern": base64.b64encode(pattern).decode("ascii"),
                          "actual": base64.b64encode(actual).decode("ascii")}).encode("ascii")
    code = "import base64,json,re,sys; v=json.load(sys.stdin); p=base64.b64decode(v['pattern']); a=base64.b64decode(v['actual']); print('1' if re.fullmatch(p,a,re.DOTALL) else '0')"
    try:
        run = subprocess.run([sys.executable, "-I", "-B", "-c", code], input=payload,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=2, check=False)
    except (OSError, subprocess.TimeoutExpired):
        return None
    if run.returncode != 0 or run.stdout not in (b"0\n", b"1\n"):
        return None
    return run.stdout == b"1\n"


def classify(sections: dict, observation: Observation, *, normalize_output=True, headers_match=True,
             path_separator=None) -> dict:
    """Return a distinct result for pass, failure, skip, unsupported and runner gaps."""
    result = {"status": "runner_gap", "basis": "supplied_observation", "exit_code": observation.exit_code,
              "reason": observation.reason}
    if observation.status in {"environment_skip", "unsupported", "runner_gap", "timeout"}:
        result["status"] = observation.status
        return result
    if observation.status != "completed" or type(observation.exit_code) is not int or observation.exit_code < 0:
        result["reason"] = "missing, invalid, or signal-terminated observation"
        return result
    if not isinstance(sections, dict) or any(not isinstance(k, str) or not isinstance(v, bytes) for k, v in sections.items()):
        result["reason"] = "invalid PHPT sections"
        return result
    if "REDIRECTTEST" in sections:
        result["reason"] = "redirected tests are not yet supported"
        return result
    variants = {"EXPECT", "EXPECTF", "EXPECTREGEX", "EXPECT_EXTERNAL", "EXPECTF_EXTERNAL", "EXPECTREGEX_EXTERNAL"}
    if len(variants.intersection(sections)) != 1:
        result["reason"] = "expected exactly one PHPT expectation section"
        return result
    if set(sections).intersection({"EXPECTREGEX", "EXPECT_EXTERNAL", "EXPECTF_EXTERNAL", "EXPECTREGEX_EXTERNAL"}):
        result["reason"] = "expectation format is not yet implemented"
        return result
    expected = sections.get("EXPECTF", sections.get("EXPECT"))
    if not isinstance(expected, bytes) or not isinstance(observation.output, bytes):
        result["reason"] = "missing or invalid output expectation"
        return result
    # PHP 8.5.12's reference runner trims surrounding PHP whitespace and
    # normalizes CRLF before comparing plain EXPECT. Exit codes remain evidence.
    trim = b" \t\n\r\x00\x0b"
    actual = observation.output.strip(trim)
    if normalize_output:
        actual = actual.replace(b"\r\n", b"\n")
    expected = expected.strip(trim).replace(b"\r\n", b"\n")
    matched = match_format(expected, actual, path_separator=path_separator) if "EXPECTF" in sections else actual == expected
    if matched is None:
        if b"%e" in expected and b"%r" not in expected and path_separator != b"/":
            result["reason"] = "EXPECTF %e requires a verified PHP 8.5.12 Linux platform profile"
        else:
            result["reason"] = "unsupported or timed-out EXPECTF pattern"
        return result
    matched = matched and headers_match
    if "XFAIL" in sections:
        result["status"] = "xpass" if matched else "xfail"
    else:
        result["status"] = "pass" if matched else "fail"
    return result
