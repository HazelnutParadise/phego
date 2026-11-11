"""Respect a stricter interpreter integer-conversion limit with a clear error."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]

class InterpreterLimitTests(unittest.TestCase):
    @unittest.skipUnless(hasattr(sys, "get_int_max_str_digits"), "interpreter has no configurable integer conversion limit")
    def test_cli_reports_stricter_interpreter_limit(self):
        base = {"test_id":"x", "test_sha256":"a"*64, "release":"8.5.12", "platform":"linux/arm64", "build":"b", "engine":"e", "status":"completed", "stdout_base64":"", "stderr_base64":"", "exit_code":0, "effects":{}}
        raw = json.dumps(base).replace('"effects": {}', '"effects": {"n": ' + '9'*1000 + '}')
        env = dict(os.environ, PYTHONINTMAXSTRDIGITS="640")
        with tempfile.TemporaryDirectory() as tmp:
            a, b = Path(tmp)/"a.json", Path(tmp)/"b.json"
            a.write_text(raw); b.write_text(raw)
            result = subprocess.run([sys.executable, str(ROOT/"tools/phpt_compare.py"), str(a), str(b)], capture_output=True, text=True, env=env, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertIn("interpreter", result.stderr.lower())
        self.assertNotIn("9999999999", result.stderr)

    @unittest.skipUnless(hasattr(sys, "get_int_max_str_digits"), "interpreter has no configurable integer conversion limit")
    def test_direct_api_reports_stricter_interpreter_limit(self):
        script = r"""
import importlib.util, sys
s=importlib.util.spec_from_file_location('d',sys.argv[1]); d=importlib.util.module_from_spec(s); sys.modules['d']=d; s.loader.exec_module(d)
for exit_code, effects in [(10**999, {}), (0, {'n': 10**999})]:
 v=d.Observation('x','a'*64,'8.5.12','linux/arm64','b','e','completed',b'',b'',exit_code,effects)
 try: d.compare(v,v)
 except ValueError as exc:
  if 'interpreter' not in str(exc): sys.exit(1)
  print(str(exc))
 else: sys.exit(1)
"""
        env = dict(os.environ, PYTHONINTMAXSTRDIGITS="640")
        result = subprocess.run([sys.executable, "-c", script, str(ROOT/"tools/phpt_diff.py")], capture_output=True, text=True, env=env, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("interpreter", result.stdout.lower())

if __name__ == "__main__": unittest.main()
