"""The executor routes SKIPIF cases through the public shared-session API."""
import importlib.util
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

spec = importlib.util.spec_from_file_location("phego_phpt_execute_session", TOOLS / "phpt_execute.py")
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)

IMAGE = "fixture@sha256:" + "a" * 64
PLATFORM = "linux/amd64"
RELEASE = "8.5.12"
EXECUTABLE = "/usr/bin/php"
SOURCE = b"<?php echo \"private FILE\"; ?>\x00\xff\r\n"
SKIPIF = b"<?php /* exact SKIPIF */ ?>\x00\xff\r\n"
FIXTURES = {"nested/data.bin": b"\x00private fixture\xff"}
OUTPUT = b"private output\n"


def observation(status="completed", output=b"", exit_code=0, cleanup_confirmed=True):
    return executor.run_case.__globals__["CaseObservation"](
        status,
        executor.run_case.__globals__["PhaseObservation"]("completed", b"8.5.12\n8", 0),
        executor.run_case.__globals__["PhaseObservation"]("completed", b"", 0),
        executor.run_case.__globals__["PhaseObservation"](status, output, exit_code),
        False,
        cleanup_confirmed,
    )


class PHPTExecutionSessionTests(unittest.TestCase):
    def sections(self, **extra):
        sections = {"TEST": b"session", "FILE": SOURCE, "EXPECT": OUTPUT}
        sections.update(extra)
        return sections

    def invoke(self, case_result=None, **run_options):
        with patch.object(executor, "execute_php") as profile, patch.object(
            executor, "run_case", return_value=case_result
        ) as session:
            result = executor.run(self.sections(SKIPIF=SKIPIF), IMAGE, EXECUTABLE,
                                  PLATFORM, RELEASE, 5, 1024,
                                  **run_options)
        return result, profile, session

    def test_skipif_uses_public_session_and_passes_profile_source_and_fixture_bytes_unchanged(self):
        result, profile, session = self.invoke(observation(output=OUTPUT), fixtures=FIXTURES)

        profile.assert_not_called()
        session.assert_called_once()
        kwargs = session.call_args.kwargs
        self.assertEqual(kwargs, {
            "image": IMAGE, "executable": EXECUTABLE, "platform": PLATFORM,
            "release": RELEASE, "source": SOURCE, "skipif": SKIPIF,
            "fixtures": FIXTURES, "timeout": 5, "output_limit": 1024,
        })
        self.assertIs(kwargs["source"], SOURCE)
        self.assertIs(kwargs["skipif"], SKIPIF)
        self.assertIs(kwargs["fixtures"], FIXTURES)
        self.assertEqual((result["status"], result["effects_complete"], result["product_acceptance"]),
                         ("pass", False, "not_claimed"))

    def test_skipif_unknown_cause_is_runner_gap_and_file_is_not_claimed_as_run(self):
        case_observation = executor.run_case.__globals__["CaseObservation"](
            "skipped_unknown",
            executor.run_case.__globals__["PhaseObservation"]("completed", b"8.5.12\n8", 0),
            executor.run_case.__globals__["PhaseObservation"]("skipped_unknown", b"SKIP unknown", 0),
            None,
            False,
            True,
            "skip cause unknown",
        )
        result, _, session = self.invoke(case_observation)
        self.assertEqual(result["status"], "runner_gap")
        self.assertNotIn(result["status"], {"environment_skip", "unsupported"})
        self.assertIsNone(result.get("exit_code"))
        session.assert_called_once()

    def test_complete_program_observation_uses_expect_expectf_and_xfail(self):
        cases = (
            ({}, OUTPUT, "pass"),
            ({}, b"different", "fail"),
            ({"XFAIL": b"known defect"}, b"different", "xfail"),
            ({"XFAIL": b"known defect"}, OUTPUT, "xpass"),
        )
        for extra, output, expected in cases:
            with self.subTest(expected=expected):
                sections = self.sections(SKIPIF=SKIPIF, **extra)
                session_result = observation(output=output)
                with patch.object(executor, "execute_php") as profile, patch.object(
                    executor, "run_case", return_value=session_result
                ):
                    result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE,
                                          5, 1024, fixtures=FIXTURES)
                    profile.assert_not_called()
                self.assertEqual(result["status"], expected)
                self.assertFalse(result["effects_complete"])
                self.assertEqual(result["product_acceptance"], "not_claimed")

        sections = self.sections(SKIPIF=SKIPIF)
        sections.pop("EXPECT")
        sections["EXPECTF"] = b"private %s\n"
        with patch.object(executor, "execute_php") as profile, patch.object(
            executor, "run_case", return_value=observation(output=OUTPUT)
        ):
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE,
                                  5, 1024, fixtures=FIXTURES)
            profile.assert_not_called()
        self.assertEqual(result["status"], "pass")

    def test_bad_profile_and_bad_or_incomplete_session_results_cannot_pass(self):
        mismatch = executor.run_case.__globals__["CaseObservation"](
            "profile_mismatch",
            executor.run_case.__globals__["PhaseObservation"]("profile_mismatch", b"8.5.11\n8", 0),
            None, None, False, True, "PHP profile mismatch",
        )
        result, profile, session = self.invoke(mismatch, fixtures=FIXTURES)
        self.assertNotEqual(result["status"], "pass")
        profile.assert_not_called()
        session.assert_called_once()

        incomplete = (
            observation(status="timeout", output=OUTPUT),
            observation(status="output_limit", output=OUTPUT),
            observation(status="cleanup_uncertain", output=OUTPUT, cleanup_confirmed=False),
            {"status": "completed", "program": "malformed", "effects_complete": True,
             "cleanup_confirmed": True},
        )
        for result_value in incomplete:
            with self.subTest(result=result_value):
                result, _, _ = self.invoke(result_value, fixtures=FIXTURES)
                self.assertNotEqual(result["status"], "pass")

    def test_unsupported_sections_stop_before_profile_or_session(self):
        with patch.object(executor, "execute_php") as profile, patch.object(executor, "run_case") as session:
            result = executor.run(self.sections(SKIPIF=SKIPIF, CLEAN=b"secret"), IMAGE,
                                  EXECUTABLE, PLATFORM, RELEASE, 5, 1024,
                                  fixtures=FIXTURES)
        self.assertEqual(result["status"], "runner_gap")
        self.assertEqual(result["basis"], "not_observed")
        profile.assert_not_called()
        session.assert_not_called()
        self.assertNotIn("secret", repr(result))

    def test_fixtures_without_skipif_use_shared_session(self):
        sections = self.sections()
        with patch.object(executor, "execute_php") as execute, patch.object(
            executor, "run_case", return_value=observation(output=OUTPUT)
        ) as session:
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE,
                                  5, 1024, fixtures=FIXTURES)
        execute.assert_not_called()
        self.assertEqual(session.call_args.kwargs["skipif"], b"")
        self.assertIs(session.call_args.kwargs["fixtures"], FIXTURES)
        self.assertEqual(result["status"], "pass")
        self.assertFalse(result["effects_complete"])

    def test_cases_without_skipif_keep_single_file_executor_behavior(self):
        sections = self.sections()
        with patch.object(executor, "execute_php", side_effect=(
            type("Profile", (), {"status": "completed", "output": b"8.5.12\n8\n", "exit_code": 0})(),
            type("File", (), {"status": "completed", "output": OUTPUT, "exit_code": 0})(),
        )) as execute, patch.object(executor, "run_case") as session:
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 1024)
        self.assertEqual(result["status"], "pass")
        self.assertEqual(execute.call_count, 2)
        session.assert_not_called()


if __name__ == "__main__":
    unittest.main()
