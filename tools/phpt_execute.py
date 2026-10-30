"""Execute a supported PHPT case in the restricted reference sandbox.

TEST, FILE, EXPECT or EXPECTF, optional XFAIL, SKIPIF, CLEAN and partial INI, ENV, STDIN and CGI requests are supported. CGI cases require --cgi-executable.
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
import phpt_cgi

MAX_INPUT = 8 *  1024 * 1024
_PROFILE_SOURCE = b'<?php echo PHP_VERSION, "\\n", PHP_INT_SIZE, "\\n"; ?>'
_SUPPORTED = {"TEST", "FILE", "EXPECT", "EXPECTF", "XFAIL", "SKIPIF", "CLEAN", "INI", "ENV", "STDIN", "CGI", "GET", "POST", "COOKIE", "EXPECTHEADERS"}
_INT_WIDTH = {"linux/386": 4, "linux/amd64": 8, "linux/arm64": 8}


def _result(status, reason="", **extra):
    result = {"status": status, "reason": reason, "execution": "reference_executed",
              "product_acceptance": "not_claimed", "basis": "restricted_reference_observation"}
    result.update(extra)
    return result


def _gap(reason):
    return _result("runner_gap", reason, execution="not_run", basis="not_observed")


def _normalize_file_section(sections, release):
    """Apply the verified PHP 8.5.12 run-tests FILE/FILEEOF rules locally."""
    if "FILE" in sections and "FILEEOF" in sections:
        return None, "FILE and FILEEOF sections conflict"
    file_eof = "FILEEOF" in sections
    if file_eof:
        source = sections["FILEEOF"]
        if len(source) > 1024 * 1024:
            return None, "FILE source exceeds sandbox limit"
        if release != "8.5.12":
            return None, "FILEEOF is unsupported for the requested PHP release"
    else:
        source = sections.get("FILE")
    if source is None:
        return None, "required sections missing or conflicting"
    if len(source) > 1024 * 1024:
        return None, "FILE source exceeds sandbox limit"
    if release == "8.5.12":
        # run-tests.php readFile stops after a line beginning at column zero
        # with ===DONE=== followed only by PCRE whitespace, retaining marker.
        chunks = source.split(b"\n")
        for index, line in enumerate(chunks):
            if re.fullmatch(rb"===DONE===[\t\r\f\v ]*", line):
                source = b"\n".join(chunks[:index + 1])
                if index < len(chunks) - 1:
                    source += b"\n"
                break
    if file_eof:
        source = source.rstrip(b"\r\n")
    normalized = dict(sections)
    normalized.pop("FILEEOF", None)
    normalized["FILE"] = source
    return normalized, ""


def run(sections, image, executable, platform, release, timeout, output_limit, *, fixtures=None, cgi_executable=None):
    """Run a narrowly supported case and return a sanitized result record."""
    if not isinstance(sections, dict) or any(not isinstance(k, str) or not isinstance(v, bytes) for k, v in sections.items()):
        return _gap("invalid PHPT sections")
    if "STDIN" in sections:
        if release != "8.5.12":
            return _gap("PHPT STDIN is supported only for PHP 8.5.12")
        if len(sections["STDIN"]) > 1024 * 1024:
            return _gap("PHPT STDIN exceeds 1 MiB limit")
    request_active = phpt_cgi.requested(sections)
    cgi_needed = phpt_cgi.needs_cgi(sections)
    path_separator = b"/" if release == "8.5.12" and platform in _INT_WIDTH else None
    # PHP 8.5.12 run-tests.php initializes CGI-style request variables for
    # every FILE phase, including ordinary CLI cases with no request sections.
    cli_request_active = release == "8.5.12"
    if request_active:
        if release != "8.5.12":
            return _gap("PHPT request sections are supported only for PHP 8.5.12")
        try:
            phpt_cgi.request(sections)
        except (ValueError, TypeError) as error:
            known = {"CGI query or cookie exceeds 64 KiB", "CGI query or cookie contains NUL", "CGI POST exceeds 1 MiB", "CGI query would disable command options"}
            return _gap(str(error) if str(error) in known else "invalid CGI request")
    if cgi_needed and (not isinstance(cgi_executable, str) or not cgi_executable.startswith("/") or "\x00" in cgi_executable):
        return _gap("CGI executable must be provided as an absolute path")
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
    unsupported = set(sections) - (_SUPPORTED | {"FILEEOF"})
    if unsupported:
        if any(not re.fullmatch(r"[A-Z][A-Z0-9_]{0,63}", name) for name in unsupported):
            return _gap("invalid PHPT section name")
        return _gap("unsupported PHPT section: " + ", ".join(sorted(unsupported)))
    sections, file_error = _normalize_file_section(sections, release)
    if file_error:
        return _gap(file_error)
    if not {"TEST", "FILE"}.issubset(sections) or ("EXPECT" in sections) == ("EXPECTF" in sections):
        return _gap("required sections missing or conflicting")
    clean_requested = "CLEAN" in sections and sections["CLEAN"] not in (b"", b"0")
    if clean_requested and release != "8.5.12":
        return _gap("CLEAN is supported only for PHP 8.5.12")
    if not isinstance(release, str) or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", release):
        return _gap("exact PHP release is required")
    if platform not in _INT_WIDTH:
        return _gap("unsupported platform profile")
    if not isinstance(sections["FILE"], bytes) or len(sections["FILE"]) > 1024 * 1024:
        return _gap("FILE source exceeds sandbox limit")
    if cli_request_active or request_active or clean_requested or "SKIPIF" in sections or fixtures is not None or ini is not None or env is not None or "STDIN" in sections:
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
            if cli_request_active:
                session_kwargs["request_sections"] = {k: sections[k] for k in ("GET", "POST", "COOKIE") if k in sections}
            if cgi_needed:
                session_kwargs["cgi_executable"] = cgi_executable
            if "STDIN" in sections:
                session_kwargs["stdin"] = sections["STDIN"]
            if clean_requested:
                if len(sections["CLEAN"]) > 1024 * 1024:
                    return _gap("CLEAN source exceeds sandbox limit")
                # PHP trims CLEAN when staging its source file, while PHP's
                # own string conversion determines whether the section runs.
                session_kwargs["clean"] = sections["CLEAN"].strip(b" \x00\t\n\r\v")
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
            clean_phase = getattr(case, "clean_phase", None)
            cgi_profile = getattr(case, "cgi_profile", None)
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
            cgi_status = getattr(cgi_profile, "status", None)
            cgi_output = getattr(cgi_profile, "output", None)
            cgi_code = getattr(cgi_profile, "exit_code", None)
            clean_status = getattr(clean_phase, "status", None)
            clean_output = getattr(clean_phase, "output", None)
            clean_exit_code = getattr(clean_phase, "exit_code", None)
            if case_status == "cleanup_uncertain" or cleanup_confirmed is False:
                diagnostic = "PHP case cleanup could not be confirmed"
            elif profile_status == "profile_mismatch":
                match = (re.fullmatch(rb"([0-9]{1,20}\.[0-9]{1,20}\.[0-9]{1,20})\n([48])", profile_output)
                         if isinstance(profile_output, bytes) else None)
                if match is None:
                    diagnostic = "PHP profile output is malformed"
                elif match.group(1) != release.encode("ascii"):
                    diagnostic = "PHP release does not match requested release"
                elif match.group(2) != str(_INT_WIDTH[platform]).encode("ascii"):
                    diagnostic = "PHP integer width does not match requested platform"
                else:
                    diagnostic = "PHP profile could not be verified"
            elif cgi_needed and cgi_status != "completed":
                diagnostic = "CGI profile could not be verified"
            elif case_status == "profile_mismatch":
                diagnostic = "PHP profile does not match the requested release or platform"
            elif case_status == "skipped_unknown" or skipif_status == "skipped_unknown":
                diagnostic = "SKIPIF requested a skip with an unknown cause"
            elif clean_requested and clean_status == "timeout":
                diagnostic = "CLEAN phase timed out"
            elif clean_requested and clean_status == "output_limit":
                diagnostic = "CLEAN phase exceeded the output limit"
            elif "timeout" in (case_status, profile_status, skipif_status, program_status):
                diagnostic = "PHP case phase timed out"
            elif "output_limit" in (case_status, profile_status, skipif_status, program_status):
                diagnostic = "PHP case phase exceeded the output limit"
            elif clean_requested and clean_status != "completed":
                diagnostic = "CLEAN phase did not produce a completed observation"
            elif clean_requested and (not isinstance(clean_output, bytes) or type(clean_exit_code) is not int
                                      or not 0 <= clean_exit_code <= 124):
                diagnostic = "CLEAN phase observation is incomplete or ambiguous"
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
                and (not cgi_needed or (cgi_status == "completed" and type(cgi_code) is int and cgi_code == 0
                     and cgi_output == b"cgi-fcgi\n" + release.encode("ascii") + b"\n" + str(_INT_WIDTH[platform]).encode("ascii")))
                and skipif_status == "completed"
                and isinstance(skipif_output, bytes)
                and program_status == "completed"
                and isinstance(program_output, bytes)
                and type(program_exit_code) is int
                and 0 <= program_exit_code <= 124
                and (not clean_requested or
                     (clean_status == "completed" and isinstance(clean_output, bytes)
                      and type(clean_exit_code) is int and 0 <= clean_exit_code <= 124))
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
        if cgi_needed:
            body, headers_match, had_headers = phpt_cgi.response(program_output, sections.get("EXPECTHEADERS", b""), _INT_WIDTH[platform] * 8)
            if headers_match is None:
                return _result("runner_gap", "CGI header numeric comparison is not supported", execution="reference_executed", effects_complete=False)
            classified = classify(sections, Observation(program_status, body, program_exit_code, ""),
                                  normalize_output=not had_headers, headers_match=headers_match,
                                  path_separator=path_separator)
        else:
            classified = classify(sections, Observation(program_status, program_output, program_exit_code, ""),
                                  path_separator=path_separator)
        status = classified.get("status", "runner_gap")
        if clean_requested and status in {"pass", "xpass"} and clean_output not in (b"", b"0"):
            return _result("runner_gap", "CLEAN produced truthy output", execution="reference_executed",
                           effects_complete=False)
        reason = "" if status in {"pass", "fail", "xfail", "xpass"} else "PHP observation could not be classified by supported PHPT expectations"
        if status == "runner_gap" and classified.get("reason") == "EXPECTF %e requires a verified PHP 8.5.12 Linux platform profile":
            reason = classified["reason"]
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
                                               ""), path_separator=path_separator)
    status = classified.get("status", "runner_gap")
    reason = "" if status in {"pass", "fail", "xfail", "xpass"} else "observation could not be classified"
    if status == "runner_gap" and classified.get("reason") == "EXPECTF %e requires a verified PHP 8.5.12 Linux platform profile":
        reason = classified["reason"]
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
    parser.add_argument("--cgi-executable", help="absolute path to reference php-cgi when a case requires CGI")
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
    run_options = {"fixtures": fixtures}
    if args.cgi_executable is not None:
        run_options["cgi_executable"] = args.cgi_executable
    result = run(sections, args.image, args.executable, args.platform, args.release, args.timeout, args.output_limit, **run_options)
    result["input_sha256"] = hashlib.sha256(data).hexdigest()
    print(json.dumps(result, sort_keys=True))
    return 0 if result["status"] in {"pass", "xfail"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
