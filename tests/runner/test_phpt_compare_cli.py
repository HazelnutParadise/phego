"""Black-box contract tests for the supplied-observation comparison CLI."""
import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
TOOL = ROOT / "tools/phpt_compare.py"
FIELDS = {
    "test_id", "test_sha256", "release", "platform", "build", "engine",
    "status", "stdout_base64", "stderr_base64", "exit_code", "effects",
}


class PHPTCompareCLITests(unittest.TestCase):
    def observation(self, **changes):
        value = {
            "test_id": "example.phpt",
            "test_sha256": "a" * 64,
            "release": "8.5.12",
            "platform": "linux/arm64",
            "build": "reference-cli",
            "engine": "reference",
            "status": "completed",
            "stdout_base64": base64.b64encode(b"ok\n").decode("ascii"),
            "stderr_base64": "",
            "exit_code": 0,
            "effects": {"files": {"out": "abc"}},
        }
        value.update(changes)
        self.assertEqual(set(value), FIELDS)
        return value

    def write(self, directory, name, value):
        path = Path(directory) / name
        if isinstance(value, bytes):
            path.write_bytes(value)
        else:
            path.write_text(json.dumps(value, allow_nan=True), encoding="utf-8")
        return path

    def run_tool(self, first, second, *args, timeout=5):
        return subprocess.run(
            [sys.executable, str(TOOL), *args, str(first), str(second)],
            capture_output=True, text=True, timeout=timeout,
        )

    def assert_error(self, result):
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertIn("phpt-compare: error:", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_help_documents_crlf_opt_in(self):
        result = subprocess.run([sys.executable, str(TOOL), "--help"], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0)
        self.assertIn("--normalize-crlf", result.stdout)
        self.assertIn("observation JSON", result.stdout)

    def test_identical_completed_observations_match_with_provenance_limits(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation())
            right = self.write(tmp, "right.json", self.observation(engine="phego", build="pure-go"))
            result = self.run_tool(left, right)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["status"], "match")
        self.assertEqual(report["basis"], "supplied_observations")
        self.assertEqual(report["product_acceptance"], "not_claimed")
        self.assertEqual(report["normalizations"], [])

    def test_changed_outputs_mismatch_with_exit_one(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation())
            right = self.write(tmp, "right.json", self.observation(stdout_base64=base64.b64encode(b"bad\n").decode()))
            result = self.run_tool(left, right)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "mismatch")

    def test_unsupported_and_not_run_are_incomparable_with_exit_three(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation())
            for status in ("unsupported", "not_run"):
                with self.subTest(status=status):
                    right = self.write(tmp, "right.json", self.observation(status=status, exit_code=None))
                    result = self.run_tool(left, right)
                    self.assertEqual(result.returncode, 3, result.stderr)
                    self.assertEqual(json.loads(result.stdout)["status"], "incomparable")

    def test_crlf_normalization_is_opt_in(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation(stdout_base64=base64.b64encode(b"a\nb\n").decode()))
            right = self.write(tmp, "right.json", self.observation(stdout_base64=base64.b64encode(b"a\r\nb\r\n").decode()))
            ordinary = self.run_tool(left, right)
            normalized = self.run_tool(left, right, "--normalize-crlf")
        self.assertEqual(ordinary.returncode, 1)
        self.assertEqual(json.loads(ordinary.stdout)["status"], "mismatch")
        self.assertEqual(normalized.returncode, 0, normalized.stderr)
        report = json.loads(normalized.stdout)
        self.assertEqual(report["status"], "match")
        self.assertEqual(report["normalizations"], ["crlf_to_lf"])

    def test_profile_mismatch_is_input_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation())
            right = self.write(tmp, "right.json", self.observation(release="8.5.11"))
            result = self.run_tool(left, right)
        self.assert_error(result)
        self.assertIn("release", result.stderr)

    def test_missing_and_unknown_fields_are_rejected(self):
        for label, changes in (("missing", {"effects": None}), ("unknown", {"extra": "no"})):
            with self.subTest(label=label), tempfile.TemporaryDirectory() as tmp:
                value = self.observation()
                if label == "missing":
                    del value["effects"]
                else:
                    value.update(changes)
                left = self.write(tmp, "left.json", value)
                right = self.write(tmp, "right.json", self.observation())
                self.assert_error(self.run_tool(left, right))

    def test_invalid_base64_is_rejected_strictly(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation(stdout_base64="%%%="))
            right = self.write(tmp, "right.json", self.observation())
            self.assert_error(self.run_tool(left, right))

    def test_duplicate_json_keys_and_nan_are_rejected(self):
        for label, raw in (
            ("duplicate", b'{"test_id":"one","test_id":"two"}'),
            ("nan", json.dumps(self.observation()).replace('"exit_code": 0', '"exit_code": NaN').encode()),
        ):
            with self.subTest(label=label), tempfile.TemporaryDirectory() as tmp:
                left = self.write(tmp, "left.json", raw)
                right = self.write(tmp, "right.json", self.observation())
                self.assert_error(self.run_tool(left, right))

    def test_input_over_eight_mib_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation())
            large = Path(tmp) / "large.json"
            with large.open("wb") as stream:
                stream.truncate(8 * 1024 * 1024 + 1)
            result = self.run_tool(left, large)
        self.assert_error(result)
        self.assertIn("8 MiB", result.stderr)

    def test_symlink_is_rejected_when_symlink_creation_is_available(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = self.write(tmp, "target.json", self.observation())
            link = Path(tmp) / "link.json"
            try:
                link.symlink_to(target)
            except (NotImplementedError, OSError) as exc:
                self.skipTest(f"symlink creation unavailable: {exc}")
            other = self.write(tmp, "other.json", self.observation())
            self.assert_error(self.run_tool(link, other))

    def test_fifo_is_rejected_without_blocking_when_supported(self):
        with tempfile.TemporaryDirectory() as tmp:
            if not hasattr(os, "mkfifo"):
                self.skipTest("FIFO creation unavailable")
            left = Path(tmp) / "input.fifo"
            try:
                os.mkfifo(left)
            except (NotImplementedError, OSError) as exc:
                self.skipTest(f"FIFO creation unavailable: {exc}")
            right = self.write(tmp, "right.json", self.observation())
            self.assert_error(self.run_tool(left, right, timeout=2))


    def test_invalid_input_identifies_path_and_cause(self):
        with tempfile.TemporaryDirectory() as tmp:
            left = self.write(tmp, "left.json", self.observation(stdout_base64="%%%="))
            right = self.write(tmp, "right.json", self.observation())
            result = self.run_tool(left, right)
        self.assert_error(result)
        self.assertIn("left.json", result.stderr)
        self.assertIn("base64", result.stderr.lower())
        self.assertIn("stdout", result.stderr.lower())

    def test_duplicate_key_in_otherwise_valid_observation_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            raw = json.dumps(self.observation())[:-1] + ', "test_id": "different.phpt"}'
            left = self.write(tmp, "left.json", raw.encode())
            right = self.write(tmp, "right.json", self.observation())
            result = self.run_tool(left, right)
        self.assert_error(result)
        self.assertIn("duplicate", result.stderr.lower())
        self.assertIn("test_id", result.stderr)


    def test_untrusted_unknown_field_is_not_echoed_in_diagnostics(self):
        with tempfile.TemporaryDirectory() as tmp:
            value = self.observation()
            value["SECRET_MARKER_9Q" + "x" * 10000 + "\x1b[31m"] = "value"
            left = self.write(tmp, "left.json", value)
            right = self.write(tmp, "right.json", self.observation())
            result = self.run_tool(left, right)
        self.assert_error(result)
        self.assertIn("unknown", result.stderr.lower())
        self.assertNotIn("SECRET_MARKER_9Q", result.stderr)
        self.assertNotIn("\x1b", result.stderr)
        self.assertLess(len(result.stderr.encode()), 4096)

    def test_unknown_duplicate_key_is_not_echoed_in_diagnostics(self):
        with tempfile.TemporaryDirectory() as tmp:
            raw = json.dumps(self.observation())[:-1] + ', "SECRET_MARKER_9Q": 1, "SECRET_MARKER_9Q": 2}'
            left = self.write(tmp, "left.json", raw.encode())
            right = self.write(tmp, "right.json", self.observation())
            result = self.run_tool(left, right)
        self.assert_error(result)
        self.assertIn("duplicate", result.stderr.lower())
        self.assertNotIn("SECRET_MARKER_9Q", result.stderr)


    def test_bad_observation_describes_specific_problem(self):
        cases = [({"test_sha256": "invalid"}, "sha-256"),
                 ({"exit_code": None}, "exit code"),
                 ({"effects": []}, "effects")]
        for changes, detail in cases:
            with self.subTest(changes=changes), tempfile.TemporaryDirectory() as tmp:
                left = self.write(tmp, "left.json", self.observation(**changes))
                right = self.write(tmp, "right.json", self.observation())
                result = self.run_tool(left, right)
                self.assert_error(result)
                self.assertIn(detail, result.stderr.lower())


    def test_unknown_option_does_not_echo_untrusted_terminal_content(self):
        option = "--SECRET_MARKER_9Q" + "x" * 10000 + "\x1b[31m"
        result = subprocess.run([sys.executable, str(TOOL), option, "a", "b"],
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("SECRET_MARKER_9Q", result.stderr)
        self.assertNotIn("\x1b", result.stderr)
        self.assertLess(len(result.stderr.encode()), 4096)
        self.assertIn("argument", result.stderr.lower())


if __name__ == "__main__":
    unittest.main()
