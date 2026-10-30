"""Regression contract for PHP 8.5.12 PHPT CLEAN handling.

Behavioral cases use the digest-pinned linux/386 PHP image. This suite tests
runner compatibility; PHP's own string-truthiness and nonzero-exit semantics
are not independently established here.
"""
import importlib.util
import os
from pathlib import Path
import re
import signal
import shutil
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
import unittest
import uuid
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))


def load_tool(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


session = load_tool("phego_phpt_session_clean", TOOLS / "phpt_session.py")
executor = load_tool("phego_phpt_execute_clean", TOOLS / "phpt_execute.py")
IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
EXECUTABLE = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def case(**kwargs):
    args = dict(image=IMAGE, executable=EXECUTABLE, platform=PLATFORM, release=RELEASE,
                source=b"<?php echo 'file'; ?>", skipif=b"", fixtures={}, timeout=10,
                output_limit=4096)
    args.update(kwargs)
    return session.run_case(**args)


class PHPTCleanInputTests(unittest.TestCase):
    def test_clean_type_and_phase_source_bound_are_checked_before_docker(self):
        for clean in ("<?php ?>", b"x" * (1024 * 1024 + 1)):
            with self.subTest(clean=type(clean).__name__), mock.patch.object(session.subprocess, "Popen") as popen:
                with self.assertRaises((TypeError, ValueError)):
                    case(clean=clean)
                popen.assert_not_called()

    def test_direct_clean_rejects_unverified_release_before_docker(self):
        for release in ("8.5.11", "8.6.0"):
            with self.subTest(release=release), mock.patch.object(session.subprocess, "Popen") as popen:
                with self.assertRaises(ValueError):
                    case(release=release, clean=b"<?php ?>")
                popen.assert_not_called()

    def test_clean_reserved_workspace_path_and_ancestor_conflicts_are_rejected(self):
        for fixtures in ({".phego-clean.php": b"clobber"},
                         {".phego-clean.php/child": b"clobber"},
                         {".phego-clean.php": b"x", "other": b"y"}):
            with self.subTest(fixtures=fixtures), mock.patch.object(session.subprocess, "Popen") as popen:
                with self.assertRaises(ValueError):
                    case(clean=b"<?php ?>", fixtures=fixtures)
                popen.assert_not_called()

    def test_symlink_fixture_source_is_rejected_without_touching_target(self):
        with tempfile.TemporaryDirectory(prefix="phego-clean-link-") as directory:
            root = Path(directory)
            target = root / "outside.txt"
            link = root / "fixture-link"
            target.write_bytes(b"must remain unchanged")
            link.symlink_to(target)
            with self.assertRaises(ValueError):
                executor._load_fixtures(["fixture.txt=" + str(link)])
            self.assertEqual(target.read_bytes(), b"must remain unchanged")

    def test_empty_clean_is_absent_but_whitespace_clean_is_php_trimmed(self):
        # Empty section takes the ordinary single-file route. A truthy section
        # with surrounding trim bytes is staged only after PHP trim semantics.
        phase = session.PhaseObservation
        profile = phase("completed", b"8.5.12\n4\n", 0)
        program = phase("completed", b"", 0)
        with mock.patch.object(executor, "execute_php", side_effect=(profile, program)) as execute, \
             mock.patch.object(executor, "run_case") as run_case:
            result = executor.run({"TEST": b"empty clean", "FILE": b"<?php ?>", "EXPECT": b"", "CLEAN": b""},
                                  IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 128)
        self.assertEqual(execute.call_count, 2)  # profile and FILE only
        run_case.assert_not_called()
        with mock.patch.object(executor, "run_case", return_value=SimpleNamespace(
                status="completed", profile=phase("completed", b"8.5.12\n4", 0),
                skipif=phase("completed", b"", 0), program=phase("completed", b"", 0),
                clean_phase=phase("completed", b"", 0), effects_complete=False,
                cleanup_confirmed=True)) as run_case:
            executor.run({"TEST": b"trim clean", "FILE": b"<?php ?>", "EXPECT": b"", "CLEAN": b" \t\n <?php ?> \r\n"},
                         IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 128)
            self.assertEqual(run_case.call_args.kwargs["clean"], b"<?php ?>")

    def test_php_section_not_empty_zero_edge_and_trimmed_staging(self):
        phase = session.PhaseObservation
        with mock.patch.object(executor, "execute_php", side_effect=(
                phase("completed", b"8.5.12\n4\n", 0), phase("completed", b"", 0))) as execute, \
             mock.patch.object(executor, "run_case") as run_case:
            result = executor.run({"TEST": b"zero clean", "FILE": b"<?php ?>", "EXPECT": b"", "CLEAN": b"0"},
                                  IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 128)
        self.assertEqual(execute.call_count, 2)
        run_case.assert_not_called()

        observation = SimpleNamespace(
            status="completed", profile=phase("completed", b"8.5.12\n4", 0),
            skipif=phase("completed", b"", 0), program=phase("completed", b"", 0),
            clean_phase=phase("completed", b"", 0), effects_complete=False,
            cleanup_confirmed=True,
        )
        with mock.patch.object(executor, "run_case", return_value=observation) as run_case:
            executor.run({"TEST": b"zero newline clean", "FILE": b"<?php ?>", "EXPECT": b"", "CLEAN": b"0\n"},
                         IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 128)
            self.assertEqual(run_case.call_args.kwargs["clean"], b"0")


class PHPTCleanExecutionTests(unittest.TestCase):
    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_runs_after_file_in_same_workspace_and_removes_file_fixture(self):
        sections = {
            "TEST": b"cleanup fixture lifecycle",
            "FILE": b"<?php file_put_contents('from-file.txt', 'created'); echo 'file-ok'; ?>",
            "EXPECT": b"file-ok",
            "CLEAN": b"<?php if (file_get_contents('from-file.txt') !== 'created') exit(7); unlink('from-file.txt'); ?>",
        }
        observation = case(source=sections["FILE"], clean=sections["CLEAN"])
        self.assertEqual(observation.program.status, "completed", observation)
        self.assertEqual(observation.clean_phase.status, "completed", observation)
        self.assertEqual(observation.clean_phase.exit_code, 0, observation)
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "pass", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_ini_is_file_only_while_env_reaches_file_and_clean(self):
        sections = {
            "TEST": b"CLEAN settings",
            "INI": b"precision=17",
            "ENV": b"PHEGO_CLEAN_VALUE=both",
            "FILE": b"<?php echo ini_get('precision'), '|', getenv('PHEGO_CLEAN_VALUE'); ?>",
            "EXPECT": b"17|both",
            "CLEAN": b"<?php if (ini_get('precision') !== '14' || getenv('PHEGO_CLEAN_VALUE') !== 'both') { echo 'CLEAN_SETTINGS_MISMATCH'; exit(9); } ?>",
        }
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "pass", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_skipif_skip_stops_before_clean(self):
        sections = {
            "TEST": b"skip before cleanup",
            "SKIPIF": b"<?php echo 'skip'; ?>",
            "FILE": b"<?php echo 'must-not-run'; ?>",
            "EXPECT": b"must-not-run",
            "CLEAN": b"<?php file_put_contents('/work/clean-ran', 'bad'); ?>",
        }
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertNotEqual(result.get("status"), "pass")
        skipped = case(skipif=b"<?php echo 'skip'; ?>", clean=b"<?php echo 'must-not-run'; ?>")
        self.assertEqual(skipped.skipif.status, "skipped_unknown", skipped)
        self.assertIsNone(skipped.clean_phase, skipped)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_output_after_matching_file_is_runner_gap(self):
        sections = {"TEST": b"visible cleanup output", "FILE": b"<?php echo 'ok'; ?>",
                    "EXPECT": b"ok", "CLEAN": b"<?php echo 'cleanup-visible'; ?>"}
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "runner_gap", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_stdout_uses_php_string_truthiness(self):
        for output, expected in ((b"0", "pass"), (b"0\n", "runner_gap")):
            sections = {"TEST": b"cleanup output truthiness", "FILE": b"<?php echo 'ok'; ?>",
                        "EXPECT": b"ok", "CLEAN": b"<?php echo " + repr(output.decode()).encode() + b"; ?>"}
            with self.subTest(output=output):
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
                self.assertEqual(result["status"], expected, result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_nonzero_exit_without_output_keeps_file_success_classification(self):
        sections = {"TEST": b"CLEAN nonzero without output", "FILE": b"<?php echo 'ok'; ?>",
                    "EXPECT": b"ok", "CLEAN": b"<?php exit(7); ?>"}
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "pass", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_file_cannot_replace_staged_clean_script_with_symlink(self):
        target_source = b"<?php echo 'symlink-target-executed'; file_put_contents('target-was-touched', 'yes'); ?>"
        source = (
            b"<?php file_put_contents('/work/clean-target.php', " + repr(target_source.decode()).encode() + b"); "
            b"unlink('/work/.phego-clean.php'); "
            b"symlink('/work/clean-target.php', '/work/.phego-clean.php'); echo 'file-ok'; ?>"
        )
        observation = case(source=source, clean=b"<?php echo 'intended-clean'; ?>")
        self.assertEqual(observation.program.status, "completed", observation)
        self.assertIsNotNone(observation.clean_phase, observation)
        self.assertNotEqual(observation.clean_phase.status, "completed", observation)
        self.assertNotIn(b"symlink-target-executed", observation.clean_phase.output)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_file_expect_mismatch_still_runs_clean(self):
        sections = {"TEST": b"cleanup after mismatch",
                    "FILE": b"<?php file_put_contents('marker', 'yes'); echo 'wrong'; ?>",
                    "EXPECT": b"right",
                    "CLEAN": b"<?php if (!is_file('marker')) exit(3); unlink('marker'); ?>"}
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "fail", result)
        direct = case(source=sections["FILE"], clean=sections["CLEAN"])
        self.assertEqual(direct.program.status, "completed", direct)
        self.assertEqual(direct.clean_phase.status, "completed", direct)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_timeout_or_stderr_and_missing_clean_observation_are_not_success(self):
        for clean in (b"<?php while (true) {} ?>", b"<?php fwrite(STDERR, 'cleanup-error'); ?>"):
            sections = {"TEST": b"bad cleanup", "FILE": b"<?php echo 'ok'; ?>", "EXPECT": b"ok", "CLEAN": clean}
            with self.subTest(clean=clean[:30]):
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 1, 4096)
                self.assertNotEqual(result["status"], "pass", result)

        timed_out = case(clean=b"<?php while (true) {} ?>", timeout=1)
        self.assertEqual(timed_out.clean_phase.status, "timeout", timed_out)
        self.assertNotEqual(timed_out.status, "completed", timed_out)
        self.assertTrue(timed_out.cleanup_confirmed, timed_out)

        stderr_case = case(clean=b"<?php fwrite(STDERR, 'cleanup-error'); ?>")
        self.assertEqual(stderr_case.clean_phase.status, "runner_gap", stderr_case)
        self.assertNotEqual(stderr_case.status, "completed", stderr_case)
        self.assertTrue(stderr_case.cleanup_confirmed, stderr_case)

        phase_type = session.PhaseObservation
        missing = session.CaseObservation(
            "completed", phase_type("completed", b"8.5.12\n4", 0),
            phase_type("completed", b"", 0), phase_type("completed", b"ok", 0),
            False, True,
        )
        sections = {"TEST": b"missing CLEAN observation", "FILE": b"<?php echo 'ok'; ?>",
                    "EXPECT": b"ok", "CLEAN": b"<?php ?>"}
        with mock.patch.object(executor, "run_case", return_value=missing):
            result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 4096)
        self.assertEqual(result["status"], "runner_gap", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_clean_background_process_is_runner_gap_and_container_is_removed(self):
        container_token = uuid.uuid4().hex
        label_token = uuid.uuid4().hex
        exact_label = "phego.phpt-session=" + label_token
        identifiers = iter((uuid.UUID(hex=container_token), uuid.UUID(hex=label_token)))
        with mock.patch.object(session.uuid, "uuid4", side_effect=lambda: next(identifiers)):
            observation = case(
                source=b"<?php echo 'file'; ?>",
                clean=b"<?php exec('sleep 20 >/dev/null 2>&1 &'); ?>",
            )
        self.assertEqual(observation.clean_phase.status, "runner_gap", observation)
        self.assertIn("CLEAN left additional container processes", observation.clean_phase.reason)
        self.assertEqual(observation.status, "runner_gap", observation)
        self.assertTrue(observation.cleanup_confirmed, observation)

        listed = subprocess.run(
            ["docker", "ps", "-aq", "--filter", "label=" + exact_label],
            capture_output=True, text=True, timeout=5, check=False,
        )
        self.assertEqual(listed.returncode, 0, listed.stderr)
        self.assertEqual(listed.stdout.split(), [], "owned CLEAN container remained after run_case")

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_sigterm_during_clean_removes_exact_owned_container(self):
        with tempfile.TemporaryDirectory(prefix="phego-clean-sigterm-") as directory:
            root = Path(directory)
            worker = root / "worker.py"
            worker.write_text("""import importlib.util, sys, uuid
path = sys.argv[1]
spec = importlib.util.spec_from_file_location('clean_cancel_session', path)
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
fixed_ids = iter((uuid.UUID(hex=sys.argv[6]), uuid.UUID(hex=sys.argv[7])))
module.uuid.uuid4 = lambda: next(fixed_ids)
module.run_case(
    image=sys.argv[2], executable=sys.argv[3], platform=sys.argv[4], release=sys.argv[5],
    source=b\"<?php file_put_contents('file-marker', 'ready'); ?>\", skipif=b\"\", fixtures={}, timeout=30,
    output_limit=4096, clean=b\"<?php while (true) {} ?>\",
)
""")
            container_token = uuid.uuid4().hex
            label_token = uuid.uuid4().hex
            expected_name = "/phego-phpt-" + container_token
            exact_label = "phego.phpt-session=" + label_token
            list_args = ["docker", "ps", "-aq", "--filter", "label=" + exact_label]
            child = subprocess.Popen(
                [sys.executable, str(worker), str(TOOLS / "phpt_session.py"), IMAGE,
                 EXECUTABLE, PLATFORM, RELEASE, container_token, label_token],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            )
            owned_id = None
            try:
                deadline = time.monotonic() + 25
                while time.monotonic() < deadline and child.poll() is None:
                    listed = subprocess.run(list_args, capture_output=True, text=True,
                                            timeout=3, check=False)
                    self.assertEqual(listed.returncode, 0, listed.stderr)
                    found = listed.stdout.split()
                    self.assertLessEqual(len(found), 1, "unique label matched multiple containers")
                    if found:
                        candidate = found[0]
                        name = subprocess.run(
                            ["docker", "inspect", "-f", "{{.Name}}", candidate],
                            capture_output=True, text=True, timeout=3, check=False,
                        )
                        self.assertEqual(name.returncode, 0, name.stderr)
                        self.assertEqual(name.stdout.strip(), expected_name)
                        top = subprocess.run(
                            ["docker", "top", candidate, "-eo", "pid,args"],
                            capture_output=True, text=True, timeout=3, check=False,
                        )
                        if top.returncode == 0 and "/work/.phego-clean.php" in top.stdout:
                            owned_id = candidate
                            break
                    time.sleep(0.1)
                self.assertIsNotNone(owned_id, "CLEAN PHP process did not start before deadline")
                child.send_signal(signal.SIGTERM)
                try:
                    child.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    self.fail("SIGTERM did not stop run_case within the bounded wait")
                self.assertEqual(child.returncode, -signal.SIGTERM)
                deadline = time.monotonic() + 8
                removed = False
                while time.monotonic() < deadline:
                    listed = subprocess.run(list_args, capture_output=True, text=True,
                                            timeout=3, check=False)
                    self.assertEqual(listed.returncode, 0,
                                     "Docker label query failed while confirming cleanup: " + listed.stderr)
                    remaining = listed.stdout.split()
                    self.assertLessEqual(len(remaining), 1,
                                         "unique label matched multiple containers during cleanup")
                    if not remaining:
                        removed = True
                        break
                    self.assertEqual(remaining[0], owned_id,
                                     "exact label resolved to a different container during cleanup")
                    time.sleep(0.1)
                self.assertTrue(removed, "owned container remained or cleanup confirmation timed out")
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=3)
                if owned_id is None:
                    remaining = subprocess.run(list_args, capture_output=True, text=True,
                                               timeout=3, check=False)
                    if remaining.returncode == 0:
                        found = remaining.stdout.split()
                        if len(found) == 1:
                            candidate = found[0]
                            name = subprocess.run(
                                ["docker", "inspect", "-f", "{{.Name}}", candidate],
                                capture_output=True, text=True, timeout=3, check=False,
                            )
                            if name.returncode == 0 and name.stdout.strip() == expected_name:
                                owned_id = candidate
                if owned_id is not None:
                    name = subprocess.run(
                        ["docker", "inspect", "-f", "{{.Name}}", owned_id],
                        capture_output=True, text=True, timeout=3, check=False,
                    )
                    if name.returncode == 0 and name.stdout.strip() == expected_name:
                        subprocess.run(["docker", "rm", "--force", owned_id],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                       timeout=5, check=False)


if __name__ == "__main__":
    unittest.main()
