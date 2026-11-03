#!/usr/bin/env python3
"""Generate isolated PHP 8.5.12 object oracles (REQ-012/018-021; T-023/024/035-042)."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

IMAGES = {
    64: ("linux/arm64", "php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9"),
    32: ("linux/386", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049"),
}


def run_php(source, platform, image, fixture):
    with tempfile.TemporaryDirectory(prefix="phego-w13-oracle-") as directory:
        cidfile = Path(directory) / "cid"
        command = ["docker", "run", "--pull", "never", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
                   "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "128m", "--cpus", "1",
                   "--cidfile", str(cidfile), "--platform", platform, "-v", str(source) + ":/w:ro", image, "php", "-n", "/w/" + fixture]
        try:
            result = subprocess.run(command, capture_output=True, timeout=120, check=True)
            if result.stderr:
                raise RuntimeError("Unexpected PHP stderr: " + result.stderr.decode(errors="replace"))
            return result, command
        finally:
            if cidfile.exists():
                container = cidfile.read_text().strip()
                if container:
                    subprocess.run(["docker", "rm", "-f", container], capture_output=True, timeout=30, check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tests/semantics/w13-objects/reference.py --width 64 --output-dir /tmp/w13-reference")
    parser.add_argument("--width", type=int, choices=(32, 64), required=True, help="Guest integer width; 64 requires an arm64 host or configured emulation")
    parser.add_argument("--output-dir", type=Path, required=True, help="Directory for reference JSON and execution metadata")
    args = parser.parse_args()
    source = Path(__file__).resolve().parent
    platform, image = IMAGES[args.width]
    # Image acquisition has Docker's own stderr (pull progress). Keep it
    # separate from the PHP stderr channel that must remain empty.
    preparation = ["docker", "image", "inspect", image]
    inspected = subprocess.run(preparation, capture_output=True, timeout=30)
    if inspected.returncode:
        preparation = ["docker", "pull", "--platform", platform, image]
        subprocess.run(preparation, capture_output=True, timeout=120, check=True)
    result, command = run_php(source, platform, image, "matrix.php")
    data = json.loads(result.stdout)
    if data["php"] != "8.5.12" or data["int_size"] * 8 != args.width:
        raise RuntimeError("Reference version/platform mismatch")
    runs = [{"fixture": "matrix.php", "command": command, "exit_code": result.returncode,
             "stdout_sha256": hashlib.sha256(result.stdout).hexdigest(), "stderr": result.stderr.decode()}]
    for fixture in sorted((source / "shutdown").glob("case_*.php")):
        relative = fixture.relative_to(source).as_posix()
        observed, auxiliary_command = run_php(source, platform, image, relative)
        case = "shutdown-" + fixture.stem.removeprefix("case_").replace("_", "-")
        data["cases"][case] = {"result": observed.stdout.decode(), "diagnostics": []}
        runs.append({"fixture": relative, "command": auxiliary_command, "exit_code": observed.returncode,
                     "stdout_sha256": hashlib.sha256(observed.stdout).hexdigest(), "stderr": observed.stderr.decode()})
    for fixture in sorted((source / "isolated").glob("case_*.php")):
        relative = fixture.relative_to(source).as_posix()
        observed, auxiliary_command = run_php(source, platform, image, relative)
        single = json.loads(observed.stdout)
        if single["php"] != "8.5.12" or single["int_size"] * 8 != args.width:
            raise RuntimeError("Isolated reference version/platform mismatch")
        if len(single["cases"]) != 1 or data["cases"].keys() & single["cases"].keys():
            raise RuntimeError("Isolated reference must add exactly one new case")
        data["cases"].update(single["cases"])
        runs.append({"fixture": relative, "command": auxiliary_command, "exit_code": observed.returncode,
                     "stdout_sha256": hashlib.sha256(observed.stdout).hexdigest(), "stderr": observed.stderr.decode()})
    output = (json.dumps(data, indent=2) + "\n").encode()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    name = "w13-ref-%d.json" % args.width
    (args.output_dir / name).write_bytes(output)
    metadata = {
        "requirements": ["REQ-012", "REQ-018", "REQ-019", "REQ-020", "REQ-021"],
        "tests": ["T-023", "T-024", "T-035", "T-036", "T-037", "T-038", "T-039", "T-040", "T-041", "T-042"],
        "image_preparation": preparation, "command": command, "runs": runs, "exit_code": result.returncode, "stderr": result.stderr.decode(),
        "image": image, "platform": platform, "php": data["php"], "integer_width": args.width,
        "cases": len(data["cases"]), "fixture_sha256": hashlib.sha256((source / "matrix.php").read_bytes()).hexdigest(),
        "fixture_sources_sha256": {p.relative_to(source).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob("*.php"))},
        "output": name, "output_sha256": hashlib.sha256(output).hexdigest(),
        "diagnostic_scope": "PHP severity and message; source locations belong to W-17 integration",
    }
    (args.output_dir / ("w13-run-%d.json" % args.width)).write_text(json.dumps(metadata, indent=2) + "\n")
    print(json.dumps(metadata))


if __name__ == "__main__":
    main()
