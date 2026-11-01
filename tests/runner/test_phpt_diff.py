"""Comparison preserves failures, missing execution and exact profile identity."""
import dataclasses
import importlib.util
from pathlib import Path
import sys
import unittest
ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("phego_phpt_diff", ROOT / "tools/phpt_diff.py")
diff = importlib.util.module_from_spec(spec); sys.modules[spec.name] = diff; spec.loader.exec_module(diff)

class DifferenceTests(unittest.TestCase):
    def reference(self):
        return diff.Observation("example.phpt", "a"*64, "8.5.12", "linux/arm64", "reference-cli", "reference", "completed", b"ok\n", b"", 0, {"files":{"out":"abc"}})
    def candidate(self):
        return dataclasses.replace(self.reference(), engine="phego", build="pure-go")

    def test_identical_observations_match_but_never_claim_execution(self):
        result=diff.compare(self.reference(), self.candidate())
        self.assertEqual(result["status"], "match")
        self.assertEqual(result["basis"], "supplied_observations")
        self.assertEqual(result["product_acceptance"], "not_claimed")

    def test_differences_preserve_all_changed_fields(self):
        candidate=dataclasses.replace(self.candidate(), stdout=b"bad", stderr=b"warning", exit_code=3, effects={"files":{"out":"different"}})
        result=diff.compare(self.reference(), candidate)
        self.assertEqual(result["status"], "mismatch")
        self.assertEqual(set(result["differences"]), {"stdout", "stderr", "exit_code", "effects"})

    def test_side_effect_types_are_not_coerced(self):
        ref = dataclasses.replace(self.reference(), effects={"value":1})
        candidate = dataclasses.replace(self.candidate(), effects={"value":True})
        self.assertEqual(diff.compare(ref, candidate)["differences"], ["effects"])

    def test_profile_and_test_identity_must_match(self):
        for changes in ({"test_sha256":"b"*64}, {"test_id":"other.phpt"}, {"release":"8.5.11"}, {"platform":"linux/amd64"}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                diff.compare(self.reference(), dataclasses.replace(self.candidate(), **changes))

    def test_unexecuted_or_unsupported_cannot_be_match(self):
        for state in ("not_run", "unsupported", "environment_skip", "runner_gap", "timeout"):
            with self.subTest(state=state):
                actual=diff.compare(self.reference(), dataclasses.replace(self.candidate(), status=state, exit_code=None))
                self.assertEqual(actual["status"], "incomparable")
                self.assertEqual(actual["candidate_status"], state)

    def test_incomplete_case_retains_partial_output_and_signal_code(self):
        candidate = dataclasses.replace(self.candidate(), status="timeout", stdout=b"partial", exit_code=-9)
        result = diff.compare(self.reference(), candidate)
        self.assertEqual(result["status"], "incomparable")
        self.assertEqual(result["candidate"]["exit_code"], -9)
        self.assertEqual(result["candidate"]["stdout_base64"], "cGFydGlhbA==")

    def test_normalization_is_explicit_and_bounded(self):
        candidate=dataclasses.replace(self.candidate(), stdout=b"ok\r\n")
        self.assertEqual(diff.compare(self.reference(), candidate)["status"], "mismatch")
        actual=diff.compare(self.reference(), candidate, normalize_crlf=True)
        self.assertEqual(actual["status"], "match")
        self.assertEqual(actual["normalizations"], ["crlf_to_lf"])

    def test_effect_size_limit_accepts_boundary_and_rejects_overflow(self):
        import json
        limit = 1024 * 1024
        overhead = len(json.dumps({"s":""}, ensure_ascii=True))
        effects = {"s":"x" * (limit - overhead)}
        ref = dataclasses.replace(self.reference(), effects=effects)
        cand = dataclasses.replace(self.candidate(), effects=effects)
        self.assertEqual(diff.compare(ref, cand)["status"], "match")
        effects = {"s":effects["s"] + "x"}
        with self.assertRaises(ValueError):
            diff.compare(dataclasses.replace(ref, effects=effects), cand)

    def test_shared_nested_effects_are_bounded_before_serialization(self):
        import subprocess
        script = r"""
import importlib.util, sys
spec = importlib.util.spec_from_file_location('d', sys.argv[1])
d = importlib.util.module_from_spec(spec); sys.modules['d']=d; spec.loader.exec_module(d)
v=[]
for _ in range(35): v=[v,v]
a=d.Observation('x','a'*64,'8.5.12','linux/arm64','a','a','completed',b'',b'',0,{'v':v})
try: d.compare(a,a)
except ValueError: sys.exit(0)
sys.exit(1)
"""
        result = subprocess.run([sys.executable, "-I", "-c", script, str(ROOT / "tools/phpt_diff.py")],
                                capture_output=True, timeout=4)
        self.assertEqual(result.returncode, 0, result.stderr.decode(errors="replace"))

    def test_invalid_sha_and_missing_exit_are_rejected(self):
        for changes in ({"test_sha256":"not-a-hash"}, {"exit_code":None}, {"engine":""}, {"status":"unknown"}, {"status":[]}, {"effects":{1:"value"}}, {"effects":{"value":(1,2)}}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                diff.compare(self.reference(), dataclasses.replace(self.candidate(), **changes))

if __name__ == "__main__": unittest.main()
