"""REQ-018/020/021; T-035/036/039-042: verify oracle identity and isolation."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("w13_reference", Path(__file__).resolve().parents[1] / "tests/semantics/w13-objects/reference.py")


class ReferenceRunnerTest(unittest.TestCase):
    def run_oracle(self, directory, width=64, stderr=b"", pull=False):
        module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(module)
        responses = [subprocess.CompletedProcess([], int(pull), b"", b"")]
        if pull:
            responses.append(subprocess.CompletedProcess([], 0, b"", b"pull progress"))
        responses.append(subprocess.CompletedProcess([], 0, json.dumps({"php": "8.5.12", "int_size": width // 8, "cases": {}}).encode(), stderr))
        for _ in range(2):
            responses.append(subprocess.CompletedProcess([],0,b"shutdown\n",b""))
        for kind in ("declared", "dynamic"):
            data={"php":"8.5.12","int_size":width//8,"cases":{"lazy-rollback-write-"+kind:{"result":["isolated",kind],"diagnostics":[]}}}
            responses.append(subprocess.CompletedProcess([],0,json.dumps(data).encode(),b""))
        with patch.object(sys, "argv", ["reference.py", "--width", str(width), "--output-dir", directory]), \
             patch.object(module.subprocess, "run", side_effect=responses) as run, \
             contextlib.redirect_stdout(io.StringIO()):
            module.main()
        return run.call_args_list

    def test_pinned_widths_and_isolation(self):
        for width in (32, 64):
            with self.subTest(width=width), tempfile.TemporaryDirectory() as directory:
                calls = self.run_oracle(directory, width)
                command = calls[-1].args[0]
                self.assertIn("--read-only", command)
                self.assertEqual(command[command.index("--network") + 1], "none")
                self.assertIn("--cap-drop", command)
                self.assertIn("no-new-privileges", command)
                self.assertTrue(any("@sha256:" in arg for arg in command))
                data = json.loads((Path(directory) / f"w13-run-{width}.json").read_text())
                self.assertEqual(data["integer_width"], width)
                self.assertEqual(data["stderr"], "")

    def test_pull_stderr_is_separate(self):
        with tempfile.TemporaryDirectory() as directory:
            calls = self.run_oracle(directory, pull=True)
            self.assertEqual(calls[1].args[0][1], "pull")

    def test_php_stderr_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(RuntimeError, "Unexpected PHP stderr"):
                self.run_oracle(directory, stderr=b"PHP error")
            self.assertFalse((Path(directory) / "w13-ref-64.json").exists())

    def test_shutdown_fixtures_recorded(self):
        with tempfile.TemporaryDirectory() as directory:
            self.run_oracle(directory)
            data=json.loads((Path(directory)/"w13-ref-64.json").read_text())
            shutdown={k:v for k,v in data["cases"].items() if k.startswith("shutdown-")}
            self.assertEqual(shutdown,{
                "shutdown-handle-reuse":{"result":"shutdown\n","diagnostics":[]},
                "shutdown-late-hole":{"result":"shutdown\n","diagnostics":[]},
            })

    def test_isolated_cases_preserve_observations(self):
        with tempfile.TemporaryDirectory() as directory:
            self.run_oracle(directory)
            data=json.loads((Path(directory)/"w13-ref-64.json").read_text())
            self.assertEqual(data["cases"]["lazy-rollback-write-declared"],{"result":["isolated","declared"],"diagnostics":[]})
            self.assertEqual(data["cases"]["lazy-rollback-write-dynamic"],{"result":["isolated","dynamic"],"diagnostics":[]})
            metadata=json.loads((Path(directory)/"w13-run-64.json").read_text())
            self.assertEqual([r["fixture"] for r in metadata["runs"]][-2:],["isolated/case_lazy_rollback_declared.php","isolated/case_lazy_rollback_dynamic.php"])

    def test_timeout_removes_container(self):
        module=importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(module)
        calls=[]
        def run(command,**kwargs):
            calls.append(command)
            if command[1]=="run":
                if "--cidfile" in command:
                    Path(command[command.index("--cidfile")+1]).write_text("f"*64)
                raise subprocess.TimeoutExpired(command,120)
            return subprocess.CompletedProcess(command,0,b"",b"")
        with tempfile.TemporaryDirectory() as directory,patch.object(sys,"argv",["reference.py","--width","64","--output-dir",directory]),patch.object(module.subprocess,"run",side_effect=run):
            with self.assertRaises(subprocess.TimeoutExpired):module.main()
            self.assertIn(["docker","rm","-f","f"*64],calls)
