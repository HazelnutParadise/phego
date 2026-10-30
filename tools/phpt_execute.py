"""Execute a supported PHPT case in the restricted reference sandbox.

Only TEST, FILE, EXPECT or EXPECTF, and optional XFAIL are supported. Any
other section produces runner_gap before a container is started. The caller
must provide an exact release and platform. Probe PHP_VERSION and PHP_INT_SIZE
in the same pinned image before running the case; a mismatched or malformed
profile is a runner_gap, never a pass. This does not claim product acceptance.
"""
import argparse
import hashlib
import json
import os
import re
import stat
import sys

from phpt_runner import PHPTError, parse_phpt
from phpt_sandbox import execute_php
from phpt_results import Observation, classify

MAX_INPUT = 8 *  1024 * 1024
_PROFILE_SOURCE = b'<?php echo PHP_VERSION, "\\n", PHP_INT_SIZE, "\\n"; ?>'
_SUPPORTED = {"TEST", "FILE", "EXPECT", "EXPECTF", "XFAIL"}
_INT_WIDTH = {"linux/386": 4, "linux/amd64": 8, "linux/arm64": 8}


def _result(status, reason="", **extra):
    result = {"status": status, "reason": reason, "execution": "reference_executed",
              "product_acceptance": "not_claimed", "basis": "restricted_reference_observation"}
    result.update(extra)
    return result


def _gap(reason):
    return _result("runner_gap", reason, execution="not_run", basis="not_observed")


def run(sections, image, executable, platform, release, timeout, output_limit):
    """Run a narrowly supported case and return a sanitized result record."""
    if not isinstance(sections, dict) or any(not isinstance(k, str) or not isinstance(v, bytes) for k, v in sections.items()):
        return _gap("invalid PHPT sections")
    unsupported = set(sections) - _SUPPORTED
    if unsupported:
        if any(not re.fullmatch(r"[A-Z][A-Z0-9_]{0,63}", name) for name in unsupported):
            return _gap("invalid PHPT section name")
        return _gap("unsupported PHPT section: " + ", ".join(sorted(unsupported)))
    if not {"TEST", "FILE"}.issubset(sections) or ("EXPECT" in sections) == ("EXPECTF" in sections):
        return _gap("required sections missing or conflicting")
    if not isinstance(release, str) or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", release):
        return _gap("exact PHP release is required")
    if platform not in _INT_WIDTH:
        return _gap("unsupported platform profile")
    if not isinstance(sections["FILE"], bytes) or len(sections["FILE"]) > 1024 * 1024:
        return _gap("FILE source exceeds sandbox limit")
    try:
        profile = execute_php(image, executable, _PROFILE_SOURCE, timeout, output_limit, platform)
    except (OSError, ValueError, TypeError):
        return _gap("PHP profile could not be verified")
    if (not isinstance(getattr(profile, "status", None), str)
            or not isinstance(getattr(profile, "output", None), bytes)
            or type(getattr(profile, "exit_code", None)) is not int):
        return _gap("PHP profile returned an invalid observation")
    if profile.status != "completed" or profile.exit_code != 0:
        return _gap("PHP profile did not complete successfully")
    match = re.fullmatch(rb"([0-9]{1,20}\.[0-9]{1,20}\.[0-9]{1,20})\n([48])\n", profile.output)
    if not match:
        return _gap("PHP profile output is malformed")
    if match.group(1) != release.encode("ascii"):
        return _gap("PHP release does not match requested release")
    if match.group(2) != str(_INT_WIDTH[platform]).encode("ascii"):
        return _gap("PHP integer width does not match requested platform")
    try:
        observation = execute_php(image, executable, sections["FILE"], timeout, output_limit, platform)
    except (OSError, ValueError, TypeError):
        return _result("runner_gap", "PHP execution could not be completed", execution="not_completed")
    if (not isinstance(getattr(observation, "status", None), str)
            or not isinstance(getattr(observation, "output", None), bytes)
            or (getattr(observation, "exit_code", None) is not None
                and type(observation.exit_code) is not int)):
        return _result("runner_gap", "PHP execution returned an invalid observation", execution="not_completed")
    if observation.status in {"timeout", "output_limit", "runner_error"}:
        return _result("runner_gap", "PHP execution did not produce a complete bounded observation", execution="not_completed")
    classified = classify(sections, Observation(observation.status, observation.output, observation.exit_code,
                                               ""))
    status = classified.get("status", "runner_gap")
    reason = "" if status in {"pass", "fail", "xfail", "xpass"} else "observation could not be classified"
    return _result(status, reason, exit_code=observation.exit_code,
                   image=image, platform=platform, release=release)


def _read_phpt(path):
    if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_NONBLOCK"):
        raise ValueError("safe file opening is unavailable")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        metadata = os.fstat(stream.fileno())
        if not stat.S_ISREG(metadata.st_mode):
            raise ValueError("input must be a regular file")
        if metadata.st_size > MAX_INPUT:
            raise ValueError("input exceeds 8 MiB")
        data = stream.read(MAX_INPUT + 1)
    if len(data) > MAX_INPUT:
        raise ValueError("input exceeds 8 MiB")
    return data


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="digest-pinned reference image")
    parser.add_argument("--executable", default="/usr/bin/php")
    parser.add_argument("--platform", required=True, choices=tuple(_INT_WIDTH))
    parser.add_argument("--release", required=True, help="exact PHP release, for example 8.5.12")
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output-limit", type=int, default=1024 * 1024)
    parser.add_argument("path", help="one PHPT input file")
    args = parser.parse_args(argv)
    try:
        data = _read_phpt(args.path)
        sections = parse_phpt(data, "input")
    except (OSError, ValueError, PHPTError):
        print(json.dumps(_gap("PHPT input could not be safely read or parsed"), sort_keys=True))
        return 2
    result = run(sections, args.image, args.executable, args.platform, args.release, args.timeout, args.output_limit)
    result["input_sha256"] = hashlib.sha256(data).hexdigest()
    print(json.dumps(result, sort_keys=True))
    return 0 if result["status"] in {"pass", "xfail"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
