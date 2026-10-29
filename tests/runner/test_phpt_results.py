"""Classifications cannot turn missing execution into a passing result."""
import importlib.util
from pathlib import Path
import sys
import unittest
ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("phego_phpt_results", ROOT / "tools/phpt_results.py")
results = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = results
spec.loader.exec_module(results)

class PHPTClassificationTests(unittest.TestCase):
    def test_success_and_mismatch_are_distinct(self):
        sec = {"EXPECT": b"hello\n"}
        self.assertEqual(results.classify(sec, results.Observation("completed", b"hello\n", 0))["status"], "pass")
        self.assertEqual(results.classify(sec, results.Observation("completed", b"wrong\n", 0))["status"], "fail")

    def test_nonexecuted_categories_cannot_pass(self):
        for state in ("environment_skip", "unsupported", "runner_gap", "timeout"):
            with self.subTest(state=state):
                actual = results.classify({"EXPECT": b""}, results.Observation(state, reason="test reason"))
                self.assertEqual(actual["status"], state)
                self.assertEqual(actual["basis"], "supplied_observation")

    def test_missing_exit_or_unknown_observation_is_runner_gap(self):
        for observation in (results.Observation("completed", b"hello"), results.Observation("invented", b"hello", 0)):
            self.assertEqual(results.classify({"EXPECT": b"hello"}, observation)["status"], "runner_gap")

    def test_xfail_and_unexpected_pass_remain_distinct(self):
        sec = {"EXPECT": b"good", "XFAIL": b"known failure"}
        self.assertEqual(results.classify(sec, results.Observation("completed", b"bad", 0))["status"], "xfail")
        self.assertEqual(results.classify(sec, results.Observation("completed", b"good", 0))["status"], "xpass")

    def test_formatted_expectation_compares_real_output(self):
        section = {"EXPECTF": b"items: %d\n"}
        self.assertEqual(results.classify(section, results.Observation("completed", b"items: 42\n", 0))["status"], "pass")
        self.assertEqual(results.classify(section, results.Observation("completed", b"items: hi\n", 0))["status"], "fail")

    def test_unsupported_raw_regex_is_a_runner_gap(self):
        section = {"EXPECTF": b"%r.+%r"}
        self.assertEqual(results.classify(section, results.Observation("completed", b"ok", 0))["status"], "runner_gap")

    def test_exit_code_is_retained_and_unhandled_expectation_is_not_pass(self):
        # PHP run-tests.php compares expected output even for fatal exits.
        actual = results.classify({"EXPECT": b"ok"}, results.Observation("completed", b"ok", 2))
        self.assertEqual(actual["status"], "pass")
        self.assertEqual(actual["exit_code"], 2)
        self.assertEqual(results.classify({"EXPECTREGEX": b".*"}, results.Observation("completed", b"ok", 0))["status"], "runner_gap")

if __name__ == "__main__": unittest.main()
