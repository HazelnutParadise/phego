"""Execution orchestration never mistakes an invalid oracle profile for a pass."""
import importlib.util
from pathlib import Path
import sys
import unittest
from unittest.mock import call, patch

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

spec = importlib.util.spec_from_file_location("phego_phpt_execute", TOOLS / "phpt_execute.py")
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)

IMAGE = "fixture@sha256:" + "a" * 64
PLATFORM = "linux/amd64"
RELEASE = "8.5.12"
EXECUTABLE = "/usr/bin/php"
FILE_SOURCE = b"<?php echo \"secret php source\"; ?>\x00\xff\r\n"
PROGRAM_OUTPUT = b"private program output\n"
PROFILE_OUTPUT = b"8.5.12\n8\n"
BASE_SECTIONS = {
    "TEST": b"classification fixture",
    "FILE": FILE_SOURCE,
    "EXPECT": b"private program output\n",
}


def completed(output=b"", exit_code=0):
    return executor.Observation("completed", output, exit_code)


class PHPTExecutionTests(unittest.TestCase):
    def invoke(self, sections=None, profile=completed(PROFILE_OUTPUT), program=completed(PROGRAM_OUTPUT), **kwargs):
        observations = [profile, program]

        def fake_execute(*args):
            if not observations:
                self.fail("unexpected sandbox invocation")
            return observations.pop(0)

        with patch.object(executor, "execute_php", side_effect=fake_execute) as mocked:
            result = executor.run(
                BASE_SECTIONS if sections is None else sections,
                IMAGE,
                EXECUTABLE,
                kwargs.get("platform", PLATFORM),
                kwargs.get("release", RELEASE),
                kwargs.get("timeout", 5),
                kwargs.get("output_limit", 1024),
            )
        return result, mocked

    def test_profile_runs_first_then_exact_original_file_bytes(self):
        result, mocked = self.invoke()

        self.assertEqual(mocked.call_count, 2)
        profile_call, file_call = mocked.call_args_list
        self.assertEqual(profile_call.args[:2], (IMAGE, EXECUTABLE))
        self.assertIn(b"PHP_VERSION", profile_call.args[2])
        self.assertIn(b"PHP_INT_SIZE", profile_call.args[2])
        self.assertEqual(file_call.args[:3], (IMAGE, EXECUTABLE, FILE_SOURCE))
        self.assertEqual(file_call.args[3:], (5, 1024, PLATFORM))
        self.assertEqual(result["status"], "pass")

    def test_exact_release_and_platform_integer_width_are_required_before_file(self):
        for profile_output, platform, release in (
            (b"8.5.11\n8\n", PLATFORM, RELEASE),
            (b"8.5.12\n4\n", PLATFORM, RELEASE),
            (b"8.5.12\n8\n", "linux/386", RELEASE),
            (b"8.5.12\n8\n", PLATFORM, "8.5.11"),
        ):
            with self.subTest(profile_output=profile_output, platform=platform, release=release):
                result, mocked = self.invoke(
                    profile=completed(profile_output), platform=platform, release=release
                )
                self.assertNotEqual(result["status"], "pass")
                self.assertEqual(mocked.call_count, 1, "FILE must not run after a profile mismatch")

    def test_profile_mismatch_identifies_the_field(self):
        version, _ = self.invoke(profile=completed(b"8.5.11\n8\n"))
        width, _ = self.invoke(profile=completed(b"8.5.12\n4\n"))
        malformed, _ = self.invoke(profile=completed(b"\xff8.5.12\n8\n"))
        self.assertIn("release", version["reason"])
        self.assertNotIn("width", version["reason"])
        self.assertIn("width", width["reason"])
        self.assertNotIn("release", width["reason"])
        self.assertIn("malformed", malformed["reason"])

    def test_malformed_or_nonzero_profile_never_runs_file(self):
        for observation in (
            completed(b"8.5.12\nnot-an-int\n"),
            completed(b"8.5.12\n8\nextra\n"),
            completed(PROFILE_OUTPUT, exit_code=1),
            executor.Observation("completed", b"8.5.12\n8\n", None),
        ):
            with self.subTest(profile=observation):
                result, mocked = self.invoke(profile=observation)
                self.assertNotEqual(result["status"], "pass")
                self.assertEqual(mocked.call_count, 1)

    def test_invalid_profile_encodings_are_runner_gaps_without_echo_or_file_execution(self):
        invalid_profiles = (
            b"\xff8.5.12\n8\n",
            b"8.5.12\n0008\n",
            b"8.5.12\n999999999999999999999999999999999999999999999999\n",
            "non-byte profile output",
        )
        for profile_output in invalid_profiles:
            observation = completed(profile_output)
            with self.subTest(output_type=type(profile_output).__name__), patch.object(
                executor, "execute_php", return_value=observation
            ) as mocked:
                result = executor.run(BASE_SECTIONS, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 1024)
                self.assertEqual(result["status"], "runner_gap")
                mocked.assert_called_once()
                rendered = repr(result)
                if isinstance(profile_output, bytes):
                    self.assertNotIn(repr(profile_output), rendered)
                    self.assertNotIn(profile_output.decode("utf-8", "replace"), rendered)
                else:
                    self.assertNotIn(profile_output, rendered)

    def test_profile_timeout_cleanup_failure_and_output_limit_never_run_file(self):
        blocked = (
            executor.Observation("timeout", reason="probe timed out"),
            executor.Observation("runner_error", reason="container cleanup could not be confirmed"),
            executor.Observation("output_limit", b"8.5.12\n8\n"),
            executor.Observation("runner_error", reason="container operation failed"),
        )
        for observation in blocked:
            with self.subTest(status=observation.status, reason=observation.reason):
                result, mocked = self.invoke(profile=observation)
                self.assertNotEqual(result["status"], "pass")
                self.assertEqual(mocked.call_count, 1)

    def test_file_timeout_cleanup_failure_and_output_limit_never_pass(self):
        blocked = (
            executor.Observation("timeout", reason="program timed out"),
            executor.Observation("runner_error", reason="container cleanup could not be confirmed"),
            executor.Observation("output_limit", PROGRAM_OUTPUT),
        )
        for observation in blocked:
            with self.subTest(status=observation.status):
                result, mocked = self.invoke(program=observation)
                self.assertNotEqual(result["status"], "pass")
                self.assertEqual(mocked.call_count, 2)

    def test_only_test_file_expect_expectf_and_xfail_sections_start_sandbox(self):
        unsupported = (
            "INI", "ENV", "ARGS", "CLEAN", "STDIN", "EXPECTREGEX",
            "EXPECT_EXTERNAL", "EXPECTF_EXTERNAL", "EXPECTREGEX_EXTERNAL",
            "REDIRECTTEST", "UNKNOWN_SECTION",
        )
        for name in unsupported:
            sections = dict(BASE_SECTIONS)
            sections[name] = b"unsupported content"
            with self.subTest(section=name), patch.object(executor, "execute_php") as mocked:
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 1024)
                self.assertEqual(result["status"], "runner_gap")
                self.assertEqual(result["basis"], "not_observed")
                self.assertIn(name, result["reason"])
                mocked.assert_not_called()

    def test_missing_or_conflicting_required_sections_are_runner_gaps_without_sandbox(self):
        invalid = (
            {"FILE": FILE_SOURCE, "EXPECT": b"x"},
            {"TEST": b"x", "FILE": FILE_SOURCE},
            {**BASE_SECTIONS, "EXPECTF": b"%d"},
        )
        for sections in invalid:
            with self.subTest(keys=tuple(sections)), patch.object(executor, "execute_php") as mocked:
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 1024)
                self.assertEqual(result["status"], "runner_gap")
                mocked.assert_not_called()

    def test_expect_expectf_and_xfail_classifications_are_preserved(self):
        cases = (
            ({}, PROGRAM_OUTPUT, "pass"),
            ({}, b"mismatch", "fail"),
            ({"XFAIL": b"known", }, b"mismatch", "xfail"),
            ({"XFAIL": b"known", }, PROGRAM_OUTPUT, "xpass"),
            ({}, b"private program output", "pass"),
        )
        for extra, output, expected_status in cases:
            sections = dict(BASE_SECTIONS)
            sections.update(extra)
            with self.subTest(expected_status=expected_status, xfail="XFAIL" in extra):
                result, mocked = self.invoke(sections=sections, program=completed(output))
                self.assertEqual(result["status"], expected_status)
                self.assertEqual(mocked.call_count, 2)

        sections = dict(BASE_SECTIONS)
        sections.pop("EXPECT")
        sections["EXPECTF"] = b"private %s output\n"
        result, _ = self.invoke(sections=sections, program=completed(PROGRAM_OUTPUT))
        self.assertEqual(result["status"], "pass")

    def test_nonzero_file_exit_is_retained_and_cannot_hide_output_mismatch(self):
        result, mocked = self.invoke(program=completed(PROGRAM_OUTPUT, exit_code=7))
        self.assertEqual(result["status"], "pass")
        self.assertEqual(result.get("exit_code"), 7)
        self.assertEqual(mocked.call_count, 2)

        result, _ = self.invoke(program=completed(b"wrong", exit_code=7))
        self.assertEqual(result["status"], "fail")
        self.assertEqual(result.get("exit_code"), 7)

    def test_result_does_not_echo_php_source_or_observed_output(self):
        result, _ = self.invoke()
        rendered = repr(result)
        self.assertNotIn(FILE_SOURCE.decode("utf-8", "replace"), rendered)
        self.assertNotIn("secret php source", rendered)
        self.assertNotIn(PROGRAM_OUTPUT.decode(), rendered)
        self.assertNotIn("8.5.12\n8\n", rendered)


if __name__ == "__main__":
    unittest.main()
