#!/usr/bin/env python3
"""Re-run the committed PHP 8.5.12 compilation fixtures without changing them."""
import argparse
import gzip
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]
IMAGES = {
    32: ("linux/386", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049"),
    64: ("linux/amd64", "php@sha256:35fd7ee3ab5af1ed1f23f6bb01a3464c10ad0bb813a020b29b770115b852d06f"),
}


def parse_output(output, width, count):
    banner, separator, body = output.partition("\n")
    if not separator or banner != f"PROFILE 8.5.12 {width // 8} Linux":
        raise ValueError("reference version/platform mismatch")
    events = [[] for _ in range(count)]

    def diagnostic(match):
        level, message, number, line = match.groups()
        number = int(number)
        if number >= count:
            raise ValueError("unexpected diagnostic file")
        events[number].append({"level": level, "message": message, "line": int(line)})
        return ""

    # PHP lint prints only the top-level frame for a compile failure. The
    # fixture records severity/message/location; reject any other extra output.
    body = re.sub(r"(?m)^(Warning|Deprecated|Notice|Fatal error|Parse error): (.*?) in /w/corpus/(\d+)\.php on line (\d+)\r?\n(?:Stack trace:\r?\n#0 \{main\}\r?\n)?",
                  diagnostic, body, flags=re.S)
    outcomes = {}

    def outcome(match):
        status, number = match.groups()
        number = int(number)
        if number >= count or number in outcomes:
            raise ValueError("duplicate or unexpected reference outcome")
        outcomes[number] = status == "Errors parsing"
        return ""

    body = re.sub(r"(?m)^(No syntax errors detected in|Errors parsing) /w/corpus/(\d+)\.php\r?\n", outcome, body)
    if body.strip() or len(outcomes) != count:
        raise ValueError(f"incomplete or unrecognized reference output: {len(outcomes)}/{count} outcomes; {body.strip()[:300]!r}")
    for number, failed in outcomes.items():
        if failed != any(e["level"] in ("Fatal error", "Parse error") for e in events[number]):
            raise ValueError("reference status disagrees with diagnostics")
    return events


def collect(cases, width, timeout):
    platform, image = IMAGES[width]
    inspected = subprocess.run(["docker", "image", "inspect", image], capture_output=True, timeout=30)
    if inspected.returncode:
        subprocess.run(["docker", "pull", "--platform", platform, image], capture_output=True, timeout=120, check=True)
    with tempfile.TemporaryDirectory(prefix="phego-compile-oracle-") as directory:
        root = Path(directory)
        (root / "corpus").mkdir()
        for number, case in enumerate(cases):
            (root / "corpus" / f"{number:05d}.php").write_text(case["source"])
        (root / "run.php").write_text('''<?php
echo "PROFILE ",PHP_VERSION," ",PHP_INT_SIZE," ",PHP_OS,"\\n";
$files=glob('/w/corpus/*.php');sort($files);
$cmd='php -n -d display_errors=1 -d log_errors=0 -d error_reporting=-1 -d html_errors=0 -l ';
foreach(array_chunk($files,200) as $chunk) {
    echo shell_exec($cmd.implode(' ',array_map('escapeshellarg',$chunk)).' 2>&1');
}
''')
        name = "phego-compile-oracle-" + uuid.uuid4().hex
        command = ["docker", "run", "--pull=never", "--rm", "--name", name,
                   "--user", f"{os.getuid()}:{os.getgid()}",
                   "--network=none", "--read-only", "--cap-drop=ALL",
                   "--security-opt=no-new-privileges", "--pids-limit=32", "--memory=256m", "--cpus=1",
                   "--platform", platform, "-v", directory + ":/w:ro", image, "php", "-n", "/w/run.php"]
        try:
            result = subprocess.run(command, capture_output=True, timeout=timeout, check=True)
        finally:
            removed = subprocess.run(["docker", "rm", "-f", name], capture_output=True, timeout=15)
            if removed.returncode and b"No such container" not in removed.stderr:
                raise RuntimeError("cannot confirm reference container cleanup: " + name)
        if result.stderr:
            raise ValueError("unexpected reference runner stderr: " + result.stderr.decode(errors="replace"))
        return parse_output(result.stdout.decode(), width, len(cases))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture", action="append", type=Path,
                        help="gzip JSON fixture; repeat to select files (default: both committed fixtures)")
    parser.add_argument("--width", type=int, choices=(32, 64), action="append",
                        help="reference integer width; repeat to select both (default: both)")
    parser.add_argument("--timeout", type=int, default=180, help="seconds per isolated reference run (default: 180)")
    args = parser.parse_args(argv)
    if args.timeout <= 0:
        parser.error("--timeout must be positive")
    fixtures = args.fixture or [ROOT / "internal/ir/testdata" / name for name in (
        "php8512-constant-compile.json.gz", "php8512-syntax-constant-compile.json.gz")]
    try:
        total = 0
        for fixture in fixtures:
            snapshot = fixture.read_bytes()
            cases = json.loads(gzip.decompress(snapshot))
            if not cases or any(c["width"] not in (32, 64) for c in cases):
                raise ValueError("empty fixture or invalid profile")
            for width in sorted(set(args.width or (32, 64))):
                selected = [c for c in cases if c["width"] == width]
                if not selected:
                    raise ValueError("fixture has no cases for requested width")
                actual = collect(selected, width, args.timeout)
                for number, (case, events) in enumerate(zip(selected, actual)):
                    if events != case["events"]:
                        raise ValueError(f"{fixture.name} int{width} case {number}: {events!r} != {case['events']!r}")
                total += len(selected)
                print(f"PASS {fixture.name}: {len(selected)} cases, PHP 8.5.12 int{width}", flush=True)
            if fixture.read_bytes() != snapshot:
                raise ValueError("fixture changed during reference execution")
        print(f"PASS {total} reference cases; compilation evidence only")
        return 0
    except subprocess.CalledProcessError as exc:
        print(f"compile reference failed: {exc}", file=sys.stderr)
        for stream, output in (("stdout", exc.stdout), ("stderr", exc.stderr)):
            if output:
                detail = output[:4096].decode(errors="replace")
                suffix = " [truncated]" if len(output) > 4096 else ""
                print(f"{stream}: {detail}{suffix}", file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError, RuntimeError, subprocess.SubprocessError) as exc:
        print(f"compile reference failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
