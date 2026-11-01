"""Bounded, loss-aware JSON number comparisons for PHPT observations."""
import dataclasses
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("numeric_diff", ROOT / "tools/phpt_diff.py")
diff = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = diff
spec.loader.exec_module(diff)

class NumericBoundsTests(unittest.TestCase):
    def observation(self):
        return diff.Observation("x.phpt", "a" * 64, "8.5.12", "linux/arm64", "build", "engine", "completed", b"", b"", 0, {})

    def cli(self, token, candidate="0.1", field="effects", timeout=4):
        base = {"test_id":"x.phpt", "test_sha256":"a"*64, "release":"8.5.12", "platform":"linux/arm64", "build":"b", "engine":"e", "status":"completed", "stdout_base64":"", "stderr_base64":"", "exit_code":0, "effects":{}}
        raw = json.dumps(base)
        if field == "effects":
            left = raw.replace('"effects": {}', '"effects": {"n": ' + token + '}')
            right = raw.replace('"effects": {}', '"effects": {"n": ' + candidate + '}')
        else:
            left = raw.replace('"exit_code": 0', '"exit_code": ' + token)
            right = raw
        with tempfile.TemporaryDirectory() as tmp:
            a, b = Path(tmp)/"a.json", Path(tmp)/"b.json"
            a.write_text(left); b.write_text(right)
            return subprocess.run([sys.executable, str(ROOT/"tools/phpt_compare.py"), str(a), str(b)], capture_output=True, text=True, timeout=timeout)

    def test_float_values_are_not_silently_rounded(self):
        result = self.cli("9007199254740993.0", "9007199254740992.0")
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("precision", result.stderr.lower())
        for token in ("0.10000000000000001", "1e-324", "1e309", "1e999999999999999999999"):
            with self.subTest(token=token):
                self.assertEqual(self.cli(token).returncode, 2)

    def test_ordinary_decimals_remain_accepted(self):
        for token in ("0.1", "1.2300", "9007199254740992.0", "5e-324"):
            with self.subTest(token=token):
                result = self.cli(token, token)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout)["status"], "match")

    def test_decimal_token_length_and_large_integer_are_bounded(self):
        self.assertEqual(self.cli("0." + "0"*4094, "0.0").returncode, 0)
        for field in ("effects", "exit_code"):
            with self.subTest(field=field):
                self.assertEqual(self.cli("1" + "0"*4095, "1" + "0"*4095, field=field).returncode, 0 if field == "effects" else 1)
                result = self.cli("1" + "0"*4096, field=field)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("number", result.stderr.lower())
        self.assertEqual(self.cli("0." + "0"*4095).returncode, 2)
        self.assertEqual(self.cli("9"*1000000, timeout=2).returncode, 2)

    def test_direct_api_bounds_integer_effects_and_exit_codes(self):
        good = 10**4095
        huge = 10**4096
        for field in ("exit_code", "effects"):
            with self.subTest(field=field):
                changes = {field: good if field == "exit_code" else {"n": good}}
                obs = dataclasses.replace(self.observation(), **changes)
                self.assertEqual(diff.compare(obs, obs)["status"], "match")
                changes = {field: huge if field == "exit_code" else {"n": huge}}
                obs = dataclasses.replace(self.observation(), **changes)
                with self.assertRaises(ValueError): diff.compare(obs, obs)

    def test_number_type_and_signed_zero_remain_distinct(self):
        self.assertEqual(self.cli("1", "1.0").returncode, 1)
        self.assertEqual(self.cli("-0.0", "0.0").returncode, 1)

if __name__ == "__main__": unittest.main()
