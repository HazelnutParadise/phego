#!/usr/bin/env python3
"""Generate isolated PHP 8.5.12 array oracles (REQ-007/016/017; T-013/014/031-034)."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

# Profiles are (integer width, architecture); see tests/semantics/w13-objects.
IMAGES = {
    (64, "arm64"): ("linux/arm64", "php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9"),
    (32, "386"): ("linux/386", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049"),
    (64, "amd64"): ("linux/amd64", "php@sha256:35fd7ee3ab5af1ed1f23f6bb01a3464c10ad0bb813a020b29b770115b852d06f"),
}
DEFAULT_ARCH = {64: "arm64", 32: "386"}


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tests/semantics/w12-arrays/reference.py --width 64 --output-dir /tmp/w12-reference")
    parser.add_argument("--width", type=int, choices=(32, 64), required=True, help="Guest integer width")
    parser.add_argument("--arch", choices=("arm64", "386", "amd64"), help="Guest architecture: arm64 (default for 64), 386 (default for 32) or amd64; other hosts need emulation")
    parser.add_argument("--output-dir", type=Path, required=True, help="Directory for reference JSON and execution metadata")
    args = parser.parse_args()
    source = Path(__file__).resolve().parent
    arch = args.arch or DEFAULT_ARCH[args.width]
    if (args.width, arch) not in IMAGES:
        parser.error("no pinned image for %d-bit %s" % (args.width, arch))
    platform, image = IMAGES[(args.width, arch)]
    # The default architecture keeps the historical file names.
    suffix = "%d" % args.width if arch == DEFAULT_ARCH[args.width] else "%d-%s" % (args.width, arch)
    # Image acquisition has Docker's own stderr (pull progress). Keep it
    # separate from the PHP stderr channel that must remain empty.
    preparation = ["docker", "image", "inspect", image]
    inspected = subprocess.run(preparation, capture_output=True, timeout=30)
    if inspected.returncode:
        preparation = ["docker", "pull", "--platform", platform, image]
        subprocess.run(preparation, capture_output=True, timeout=120, check=True)
    command = ["docker", "run", "--pull", "never", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
               "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "128m", "--cpus", "1",
               "--platform", platform, "-v", str(source) + ":/w:ro", image, "php", "-n", "/w/matrix.php"]
    result = subprocess.run(command, capture_output=True, timeout=120, check=True)
    data = json.loads(result.stdout)
    if data["php"] != "8.5.12" or data["int_size"] * 8 != args.width:
        raise RuntimeError("Reference version/platform mismatch")
    if result.stderr:
        raise RuntimeError("Unexpected PHP stderr: " + result.stderr.decode(errors="replace"))
    args.output_dir.mkdir(parents=True, exist_ok=True)
    name = "w12-ref-%s.json" % suffix
    (args.output_dir / name).write_bytes(result.stdout)
    metadata = {
        "requirements": ["REQ-007", "REQ-016", "REQ-017"],
        "tests": ["T-013", "T-014", "T-031", "T-032", "T-033", "T-034"],
        "image_preparation": preparation, "command": command, "exit_code": result.returncode, "stderr": result.stderr.decode(),
        "image": image, "platform": platform, "php": data["php"], "integer_width": args.width, "architecture": arch,
        "cases": len(data["cases"]), "fixture_sha256": hashlib.sha256((source / "matrix.php").read_bytes()).hexdigest(),
        "fixture_sources_sha256": {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.glob("*.php"))},
        "output": name, "output_sha256": hashlib.sha256(result.stdout).hexdigest(),
        "diagnostic_scope": "PHP severity and message; source locations belong to W-17 integration",
    }
    (args.output_dir / ("w12-run-%s.json" % suffix)).write_text(json.dumps(metadata, indent=2) + "\n")
    print(json.dumps(metadata))


if __name__ == "__main__":
    main()
