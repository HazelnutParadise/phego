#!/usr/bin/env python3
"""Replay frozen adversarial W-12 sequences against PHP and Go (REQ-007/016/017; T-013/014/031-034)."""
import argparse
import base64
import gzip
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile

GO_TEST = r'''package runtime
import("testing";"os";"encoding/json";"reflect")
func TestW12ReviewCorpus(t *testing.T) {
 b,err:=os.ReadFile(os.Getenv("PHEGO_W12_REVIEW_REFERENCE"));if err!=nil{t.Fatal(err)}
 var ref struct{PHP string `json:"php"`;Width int `json:"int_size"`;Cases []struct{Name string;Operations []any;Events []any}}
 if err:=json.Unmarshal(b,&ref);err!=nil{t.Fatal(err)}
 if ref.PHP!="8.5.12"||ref.Width!=8||len(ref.Cases)==0{t.Fatal("invalid reference profile")}
 for _,c:=range ref.Cases{t.Run(c.Name,func(t *testing.T){
  h,err:=newHeap(64);if err!=nil{t.Fatal(err)};m:=w12Machine{h:h,vars:map[string]*value{},events:[]any{}}
  if err:=m.run(w12Input(c.Operations).([]any),-1);err!=nil{t.Fatal(err)}
  data,err:=json.Marshal(m.events);if err!=nil{t.Fatal(err)};var got []any
  if err:=json.Unmarshal(data,&got);err!=nil{t.Fatal(err)}
  if !reflect.DeepEqual(got,c.Events){want,err:=json.Marshal(c.Events);if err!=nil{t.Fatal(err)};t.Errorf("Go %s\nPHP %s",data,want)}
 })}
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tests/semantics/w12-arrays/review_corpus.py --corpus foreach --output-dir /tmp/w12-foreach-review")
    parser.add_argument("--corpus", choices=("general", "foreach"), required=True, help="Frozen original review inputs: 500 general or 700 foreach sequences")
    parser.add_argument("--output-dir", type=Path, required=True, help="New directory for actual PHP output and replay report; existing directories are rejected")
    args = parser.parse_args()
    source_dir = Path(__file__).resolve().parent
    root = source_dir.parents[2]
    spec = importlib.util.spec_from_file_location("w12_reference", source_dir / "reference.py")
    reference = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(reference)
    packed = (source_dir / ("review-" + args.corpus + ".json.gz")).read_bytes()
    cases = gzip.decompress(packed)
    count = len(json.loads(cases))
    if count != {"general": 500, "foreach": 700}[args.corpus]:
        raise RuntimeError("Frozen corpus count changed")
    source = (source_dir / "matrix.php").read_text()
    marker = "$out = ['php'=>PHP_VERSION"
    if source.count(marker) != 1:
        raise RuntimeError("Fixture entry point changed; review corpus insertion first")
    source = source.replace(marker, "$cases=json_decode(base64_decode('" + base64.b64encode(cases).decode() + "'),true,512,JSON_THROW_ON_ERROR);\n" + marker)
    args.output_dir.mkdir(parents=True, exist_ok=False)
    platform, image = reference.IMAGES[64]
    # Acquisition output is separate from the reference's diagnostic stream.
    subprocess.run(["docker", "pull", "--platform", platform, image], capture_output=True, timeout=120, check=True)
    # REQ-017; T-033/T-034: stdin has no fixture directory. Mount the same
    # read-only sources as reference.py so sorting.php remains resolvable.
    command = ["docker", "run", "--pull", "never", "--rm", "-i", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "128m", "--cpus", "1", "--mount", f"type=bind,src={source_dir},dst=/fixtures,readonly", "--workdir", "/fixtures", "--platform", platform, image, "php", "-n"]
    php = subprocess.run(command, input=source.encode(), capture_output=True, timeout=120)
    report = {"corpus": args.corpus, "cases": count, "platform": platform, "image": image, "command": command, "php_exit_code": php.returncode, "php_stderr": php.stderr.decode(errors="replace"), "input_sha256": hashlib.sha256(packed).hexdigest(), "fixture_sha256": hashlib.sha256((source_dir / "matrix.php").read_bytes()).hexdigest(), "output_sha256": hashlib.sha256(php.stdout).hexdigest()}
    output = args.output_dir / "reference.json"
    output.write_bytes(php.stdout)
    if php.returncode == 0 and not php.stderr:
        data = json.loads(php.stdout)
        if data["php"] != "8.5.12" or data["int_size"] != 8 or len(data["cases"]) != count:
            raise RuntimeError("Reference profile or case count mismatch")
        with tempfile.TemporaryDirectory(prefix="w12-overlay-") as temporary:
            temporary = Path(temporary)
            test = temporary / "review_test.go"
            test.write_text(GO_TEST)
            overlay = temporary / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(root / "internal/runtime/w12_review_overlay_test.go"): str(test)}}))
            env = os.environ.copy()
            env["PHEGO_W12_REVIEW_REFERENCE"] = str(output.resolve())
            go = subprocess.run(["go", "test", "-overlay", str(overlay), "./internal/runtime", "-run", "^TestW12ReviewCorpus$", "-count=1"], cwd=root, env=env, capture_output=True, timeout=120)
            report.update(go_exit_code=go.returncode, go_stdout=go.stdout.decode(errors="replace"), go_stderr=go.stderr.decode(errors="replace"))
    (args.output_dir / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report))
    if report.get("go_exit_code") != 0:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
