#!/usr/bin/env python3
"""Generate isolated PHP 8.5.12 array oracles (REQ-007/016/017; T-013/014/031-034)."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

IMAGES = {
    64: ("linux/arm64", "php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9"),
    32: ("linux/386", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049"),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tests/semantics/w12-arrays/reference.py --width 64 --output-dir /tmp/w12-reference")
    parser.add_argument("--width", type=int, choices=(32, 64), required=True, help="Guest integer width; 64 requires an arm64 host or configured emulation")
    parser.add_argument("--output-dir", type=Path, required=True, help="Directory for reference JSON and execution metadata")
    args = parser.parse_args()
    source = Path(__file__).resolve().parent
    platform, image = IMAGES[args.width]
    command = ["docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
               "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "128m", "--cpus", "1",
               "--platform", platform, "-v", str(source) + ":/w:ro", image, "php", "-n", "/w/matrix.php"]
    result = subprocess.run(command, capture_output=True, timeout=120, check=True)
    data = json.loads(result.stdout)
    if data["php"] != "8.5.12" or data["int_size"] * 8 != args.width:
        raise RuntimeError("Reference version/platform mismatch")
    if result.stderr:
        raise RuntimeError("Unexpected PHP stderr: " + result.stderr.decode(errors="replace"))
    args.output_dir.mkdir(parents=True, exist_ok=True)
    name = "w12-ref-%d.json" % args.width
    (args.output_dir / name).write_bytes(result.stdout)
    metadata = {
        "requirements": ["REQ-007", "REQ-016", "REQ-017"],
        "tests": ["T-013", "T-014", "T-031", "T-032", "T-033", "T-034"],
        "command": command, "exit_code": result.returncode, "stderr": result.stderr.decode(),
        "image": image, "platform": platform, "php": data["php"], "integer_width": args.width,
        "cases": len(data["cases"]), "fixture_sha256": hashlib.sha256((source / "matrix.php").read_bytes()).hexdigest(),
        "output": name, "output_sha256": hashlib.sha256(result.stdout).hexdigest(),
        "diagnostic_scope": "PHP severity and message; source locations belong to W-17 integration",
    }
    (args.output_dir / ("w12-run-%d.json" % args.width)).write_text(json.dumps(metadata, indent=2) + "\n")
    print(json.dumps(metadata))


if __name__ == "__main__":
    main()
