"""Execute a supported PHPT case in the restricted reference sandbox.

TEST, FILE, EXPECT or EXPECTF, optional XFAIL, SKIPIF and partial INI and ENV are supported.
Other sections produce runner_gap before a container is started. The caller
must provide an exact release and platform. Verify PHP_VERSION and PHP_INT_SIZE
in the pinned image before running the case. SKIPIF and FILE share a case
workspace; --fixture NAME=PATH explicitly supplies files to that workspace.
Incomplete observations or unknown skip causes are runner gaps. This does not claim product acceptance.
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
from phpt_session import run_case
from phpt_ini import parse_ini
from phpt_env import parse_env

MAX_INPUT = 8 *  1024 * 1024
_PROFILE_SOURCE = b'<?php echo PHP_VERSION, "\\n", PHP_INT_SIZE, "\\n"; ?>'
_SUPPORTED = {"TEST", "FILE", "EXPECT", "EXPECTF", "XFAIL", "SKIPIF", "INI", "ENV"}
_INT_WIDTH = {"linux/386": 4, "linux/amd64": 8, "linux/arm64": 8}


def _result(status, reason="", **extra):
    result = {"status": status, "reason": reason, "execution": "reference_executed",
              "product_acceptance": "not_claimed", "basis": "restricted_reference_observation"}
    result.update(extra)
    return result


def _gap(reason):
    return _result("runner_gap", reason, execution="not_run", basis="not_observed")


def run(sections, image, executable, platform, release, timeout, output_limit, *, fixtures=None):
    """Run a narrowly supported case and return a sanitized result record."""
    if not isinstance(sections, dict) or any(not isinstance(k, str) or not isinstance(v, bytes) for k, v in sections.items()):
        return _gap("invalid PHPT sections")
    ini = None
    env = None
    if "INI" in sections:
        try:
            ini = parse_ini(sections["INI"], release)
        except TypeError:
            return _gap("PHPT INI section could not be parsed")
        except ValueError as error:
            reasons = {
                "unsupported PHP release": "PHPT INI parser does not support the requested PHP release",
                "INI input must be bytes": "PHPT INI input has an invalid type",
                "INI input exceeds byte limit": "PHPT INI input exceeds 64 KiB limit",
                "INI input contains NUL byte": "PHPT INI input contains a NUL byte",
                "INI input contains shell expansion syntax": "PHPT INI input contains unsupported shell expansion syntax",
                "INI input exceeds line limit": "PHPT INI input exceeds 256 lines limit",
                "malformed INI setting name": "PHPT INI input contains a malformed setting name",
                "unsupported INI substitution": "PHPT INI input contains an unsupported substitution",
            }
            return _gap(reasons.get(str(error), "PHPT INI input is invalid or unsupported"))
    if "ENV" in sections:
        try:
            env = parse_env(sections["ENV"], release)
        except (TypeError, ValueError) as error:
            message = str(error)
            # Parser diagnostics carry names only for reserved variables; values
            # are never included in public runner results.
            if message.startswith("reserved ENV variable name:"):
                return _gap(message)
            messages = {
                "unsupported PHP release": "PHPT ENV parser does not support the requested PHP release",
                "ENV input must be bytes": "PHPT ENV input has an invalid type",
                "ENV input exceeds 64 KiB": "PHPT ENV input exceeds 64 KiB limit",
                "ENV input contains a NUL byte": "PHPT ENV input contains a NUL byte",
                "ENV input exceeds 256 lines": "PHPT ENV input exceeds 256 lines limit",
                "invalid ENV variable name": "PHPT ENV input contains an invalid variable name",
            }
            return _gap(messages.get(message, "PHPT ENV input is invalid or unsupported"))
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
    if "SKIPIF" in sections or fixtures is not None or ini is not None or env is not None:
        if len(sections.get("SKIPIF", b"")) > 1024 * 1024:
            return _gap("SKIPIF source exceeds sandbox limit")
        if fixtures is None:
            fixtures = {}
        if not isinstance(fixtures, dict):
            return _gap("invalid fixture mapping")
        try:
            session_kwargs = dict(
                image=image,
                executable=executable,
                platform=platform,
                release=release,
                source=sections["FILE"],
                skipif=sections.get("SKIPIF", b""),
                fixtures=fixtures,
                timeout=timeout,
                output_limit=output_limit,
            )
            if ini is not None:
                session_kwargs["ini"] = ini
            if env is not None:
                session_kwargs["env"] = env
            case = run_case(**session_kwargs)
        except Exception:
            return _result("runner_gap", "PHP case session could not be completed",
                           execution="not_completed", effects_complete=False)

        try:
            profile = getattr(case, "profile", None)
            skipif = getattr(case, "skipif", None)
            program = getattr(case, "program", None)
            case_status = getattr(case, "status", None)
            cleanup_confirmed = getattr(case, "cleanup_confirmed", None)
            profile_status = getattr(profile, "status", None)
            profile_output = getattr(profile, "output", None)
            profile_exit_code = getattr(profile, "exit_code", None)
            skipif_status = getattr(skipif, "status", None)
            skipif_output = getattr(skipif, "output", None)
            program_status = getattr(program, "status", None)
            program_output = getattr(program, "output", None)
            program_exit_code = getattr(program, "exit_code", None)
            if case_status == "cleanup_uncertain" or cleanup_confirmed is False:
                diagnostic = "PHP case cleanup could not be confirmed"
            elif case_status == "profile_mismatch" or profile_status == "profile_mismatch":
                diagnostic = "PHP profile does not match the requested release or platform"
            elif case_status == "skipped_unknown" or skipif_status == "skipped_unknown":
                diagnostic = "SKIPIF requested a skip with an unknown cause"
            elif "timeout" in (case_status, profile_status, skipif_status, program_status):
                diagnostic = "PHP case phase timed out"
            elif "output_limit" in (case_status, profile_status, skipif_status, program_status):
                diagnostic = "PHP case phase exceeded the output limit"
            else:
                diagnostic = ""
            if diagnostic:
                return _result("runner_gap", diagnostic, execution="not_completed",
                               effects_complete=False)
            complete = (
                case_status == "completed"
                and cleanup_confirmed is True
                and getattr(case, "effects_complete", None) is False
                and profile_status == "completed"
                and profile_output == release.encode("ascii") + b"\n" + str(_INT_WIDTH[platform]).encode("ascii")
                and type(profile_exit_code) is int
                and profile_exit_code == 0
                and skipif_status == "completed"
                and isinstance(skipif_output, bytes)
                and program_status == "completed"
                and isinstance(program_output, bytes)
                and type(program_exit_code) is int
                and 0 <= program_exit_code <= 124
            )
        except Exception:
            complete = False
        if not complete:
            return _result("runner_gap", "PHP case session did not produce a complete observation",
                           execution="not_completed", effects_complete=False)
        skip_token = skipif_output.strip(b" \x00\t\n\r\v")
        if skip_token:
            # SKIPIF skip reasons are intentionally unknown; other directives
            # are not supported by this executor.
            return _result("runner_gap", "SKIPIF did not produce a classifiable observation",
                           execution="not_completed", effects_complete=False)
        classified = classify(
            sections,
            Observation(program_status, program_output, program_exit_code, ""),
        )
        status = classified.get("status", "runner_gap")
        reason = "" if status in {"pass", "fail", "xfail", "xpass"} else "PHP observation could not be classified by supported PHPT expectations"
        return _result(status, reason, execution="reference_executed", effects_complete=False,
                       exit_code=program_exit_code, image=image, platform=platform, release=release)

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


def _load_fixtures(specifications):
    """Load explicit NAME=PATH fixtures within session bounds."""
    if not isinstance(specifications, (list, tuple)):
        raise ValueError("fixture must use NAME=PATH format")
    if len(specifications) > 128:
        raise ValueError("fixture count exceeds the limit of 128")

    fixtures = {}
    sources = []
    reserved = (".phego-profile.php", ".phego-skipif.php", ".phego-file.php")
    for specification in specifications:
        if not isinstance(specification, str) or "=" not in specification:
            raise ValueError("fixture must use NAME=PATH format")
        name, path = specification.split("=", 1)
        parts = name.split("/")
        if (not name or "\x00" in name or name.startswith("/") or "\\" in name
                or any(part in {"", ".", ".."} for part in parts)):
            raise ValueError("fixture destination must be a safe relative path")
        if any(name == item or name.startswith(item + "/") or item.startswith(name + "/")
               for item in reserved):
            raise ValueError("fixture destination conflicts with an internal phase file")
        if name in fixtures:
            raise ValueError("fixture destination is duplicated")
        for existing in fixtures:
            if name.startswith(existing + "/") or existing.startswith(name + "/"):
                raise ValueError("fixture destination conflicts with a fixture directory")
        if not path:
            raise ValueError("fixture source path is empty")
        fixtures[name] = None
        sources.append((name, path))

    if not hasattr(os, "O_NOFOLLOW") or not hasattr(os, "O_NONBLOCK"):
        raise ValueError("fixture safe opening is unavailable on this platform")

    total = 0
    for name, path in sources:
        remaining = MAX_INPUT - total
        try:
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        except FileNotFoundError:
            raise ValueError("fixture source file does not exist") from None
        except (OSError, ValueError):
            raise ValueError("fixture source file cannot be opened safely") from None
        with os.fdopen(fd, "rb") as stream:
            metadata = os.fstat(stream.fileno())
            if not stat.S_ISREG(metadata.st_mode):
                raise ValueError("fixture source must be a regular file")
            if metadata.st_size > remaining:
                raise ValueError("fixture files exceed the 8 MiB total limit")
            data = stream.read(remaining + 1)
        if len(data) > remaining:
            raise ValueError("fixture files exceed the 8 MiB total limit")
        fixtures[name] = data
        total += len(data)
    return fixtures


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True, help="digest-pinned reference image")
    parser.add_argument("--executable", default="/usr/bin/php")
    parser.add_argument("--platform", required=True, choices=tuple(_INT_WIDTH))
    parser.add_argument("--release", required=True, help="exact PHP release, for example 8.5.12")
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output-limit", type=int, default=1024 * 1024)
    parser.add_argument("--fixture", action="append", metavar="NAME=PATH", help="copy NAME=PATH into the case workspace; repeat up to 128 times, total at most 8 MiB")
    parser.add_argument("path", help="one PHPT input file")
    args = parser.parse_args(argv)
    try:
        data = _read_phpt(args.path)
        sections = parse_phpt(data, "input")
    except (OSError, ValueError, PHPTError):
        print(json.dumps(_gap("PHPT input could not be safely read or parsed"), sort_keys=True))
        return 2
    try:
        fixtures = _load_fixtures(args.fixture) if args.fixture is not None else None
    except (OSError, ValueError) as error:
        # Exception text may contain user-controlled paths or fixture names.
        diagnostic = str(error)
        known = {
            "fixture must use NAME=PATH format", "fixture count exceeds the limit of 128", "fixture destination must be a safe relative path",
            "fixture destination conflicts with an internal phase file", "fixture destination is duplicated",
            "fixture destination conflicts with a fixture directory", "fixture source path is empty", "fixture source file does not exist",
            "fixture source file cannot be opened safely", "fixture source must be a regular file", "fixture files exceed the 8 MiB total limit",
            "fixture safe opening is unavailable on this platform",
        }
        reason = diagnostic if diagnostic in known else "fixture input error"
        print(json.dumps(_gap(reason), sort_keys=True))
        return 2
    result = run(sections, args.image, args.executable, args.platform, args.release, args.timeout, args.output_limit, fixtures=fixtures)
    result["input_sha256"] = hashlib.sha256(data).hexdigest()
    print(json.dumps(result, sort_keys=True))
    return 0 if result["status"] in {"pass", "xfail"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
