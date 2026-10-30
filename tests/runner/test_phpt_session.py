"""Contract tests for one isolated PHPT session.

Input rejection tests are Docker independent. Behavioral checks run only with an
explicitly supplied, digest-pinned PHP 8.5.12 image; no fake container output is
used as evidence of execution.
"""
import importlib.util
import base64
import json
import os
from pathlib import Path
import re
import signal
import shutil
import subprocess
import sys
import time
from unittest.mock import patch
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("phego_phpt_session", ROOT / "tools/phpt_session.py")
session = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = session
spec.loader.exec_module(session)

IMAGE = os.environ.get("PHEGO_PHPT_SESSION_IMAGE", "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049")
EXECUTABLE = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = False
if re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE) and shutil.which("docker"):
    PHP_IMAGE_AVAILABLE = subprocess.run(
        ["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL, check=False,
    ).returncode == 0


def run(image=IMAGE, **changes):
    args = dict(image=image, executable=EXECUTABLE, platform=PLATFORM,
                release=RELEASE, source=b"<?php echo 'ok'; ?>", skipif=b"",
                fixtures={}, timeout=2, output_limit=4096)
    args.update(changes)
    return session.run_case(**args)


class PHPTSessionInputTests(unittest.TestCase):
    """Malformed input must be rejected before the Docker boundary is needed."""

    def test_rejects_unpinned_or_malformed_profile_and_limits(self):
        invalid = (
            {"image": "php:latest"},
            {"executable": "php"},
            {"platform": "windows/amd64"},
            {"release": "8.5"},
            {"release": "٨.٥.١٢"},
            {"source": "<?php ?>"},
            {"timeout": 0},
            {"output_limit": 0},
        )
        for changes in invalid:
            args = {"image": IMAGE}
            args.update(changes)
            with self.subTest(changes=changes), patch.dict(os.environ, {"PATH": ""}), self.assertRaises((ValueError, TypeError)):
                run(**args)

    def test_rejects_unsafe_fixture_names_and_types_before_execution(self):
        for fixtures in (
            {"../outside.php": b"x"},
            {"/etc/passwd": b"x"},
            {"a/../../b": b"x"},
            {"safe.php": "not bytes"},
        ):
            with self.subTest(fixtures=fixtures), patch.dict(os.environ, {"PATH": ""}), self.assertRaises((ValueError, TypeError)):
                run(image=IMAGE, fixtures=fixtures)

    def test_rejects_fixture_and_phase_size_overruns_before_execution(self):
        cases = (
            {"fixtures": {"large.bin": b"x" * (8 * 1024 * 1024 + 1)}},
            {"source": b"x" * (1024 * 1024 + 1)},
            {"skipif": b"x" * (1024 * 1024 + 1)},
        )
        for changes in cases:
            with self.subTest(field=next(iter(changes))), patch.dict(os.environ, {"PATH": ""}), self.assertRaises((ValueError, TypeError)):
                run(image=IMAGE, **changes)


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "set PHEGO_PHPT_SESSION_IMAGE to a digest-pinned PHP 8.5.12 image and install Docker")
class PHPTSessionDockerTests(unittest.TestCase):
    """These assert actual PHP/Docker behavior, never a fake boundary response."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def invoke(self, source, skipif=b"", fixtures=None, **changes):
        return run(source=source, skipif=skipif, fixtures=fixtures or {}, **changes)

    def test_profile_skipif_and_file_share_one_workspace_but_use_independent_php_processes(self):
        # The SKIPIF-created marker and listed fixture must both be visible to
        # FILE. A PHP process environment mutation must not survive SKIPIF.
        skipif = b"<?php file_put_contents('skip-marker', 'yes'); file_put_contents('skip-pid', (string)getmypid()); putenv('SESSION_MARKER=leak'); ?>"
        source = (b"<?php echo file_get_contents('skip-marker'), '|', "
                  b"file_get_contents('fixture.txt'), '|', "
                  b"(getenv('SESSION_MARKER') === false ? 'clean' : 'leak'), '|', "
                  b"(file_exists('not-listed.txt') ? 'ambient' : 'isolated'), '|', "
                  b"file_get_contents('skip-pid'), '|', getmypid(); ?>")
        result = self.invoke(source, skipif, {"fixture.txt": b"fixture"})
        self.assertEqual(result.profile.status, "completed")
        self.assertEqual(result.skipif.status, "completed")
        self.assertIsNotNone(result.program)
        fields = result.program.output.split(b"|")
        self.assertEqual(fields[:4], [b"yes", b"fixture", b"clean", b"isolated"])
        self.assertNotEqual(fields[4], fields[5])
        self.assertTrue(result.cleanup_confirmed)

    def test_profile_reports_runtime_version_and_integer_width_before_test_phases(self):
        expected_width = "4" if PLATFORM.endswith("/386") else "8"
        result = self.invoke(b"<?php echo 'ran'; ?>")
        self.assertEqual(result.profile.status, "completed")
        self.assertEqual(result.profile.output.splitlines(),
                         [RELEASE.encode(), expected_width.encode()])
        wrong_width_platform = "linux/amd64" if expected_width == "4" else "linux/386"
        mismatch = self.invoke(b"<?php echo 'must not run'; ?>",
                               b"<?php echo 'must not run'; ?>",
                               platform=wrong_width_platform)
        self.assertIsNone(mismatch.skipif)
        self.assertIsNone(mismatch.program)
        self.assertNotEqual(mismatch.status, "pass")

    def test_file_and_skipif_have_distinct_real_php_files_in_one_directory(self):
        # SKIPIF records its real script path without emitting non-empty output,
        # which run-tests would interpret as an invalid skip condition.
        skipif = (b"<?php file_put_contents(__DIR__ . '/skip-path.json', "
                  b"json_encode(['file' => __FILE__, 'dir' => __DIR__])); ?>")
        source = (b"<?php $skip = json_decode(file_get_contents(__DIR__ . '/skip-path.json'), true); "
                  b"echo json_encode(['skip' => $skip['file'], 'skip_dir' => $skip['dir'], "
                  b"'file' => __FILE__, 'dir' => __DIR__, "
                  b"'skip_exists' => file_exists($skip['file']), "
                  b"'file_exists' => file_exists(__FILE__)]); ?>")
        result = self.invoke(source, skipif)
        self.assertEqual(result.skipif.output, b"")
        details = json.loads(result.program.output)
        self.assertTrue(details["skip_exists"])
        self.assertTrue(details["file_exists"])
        self.assertEqual(details["skip_dir"], details["dir"])
        self.assertNotEqual(details["skip"], details["file"])
        for path in (details["skip"], details["file"]):
            self.assertNotEqual(path, "Standard input code")
            self.assertTrue(path.endswith(".php"))

    def test_require_fixture_resolves_relative_to_file_dir(self):
        source = b"<?php echo require __DIR__ . '/helper.php'; ?>"
        result = self.invoke(source, fixtures={"helper.php": b"<?php return 'required-from-fixture'; ?>"})
        self.assertEqual(result.program.output, b"required-from-fixture")

    def test_nonzero_file_exit_is_a_completed_reference_observation(self):
        result = self.invoke(b"<?php echo 'output-before-exit'; exit(7); ?>")
        self.assertEqual(result.program.status, "completed")
        self.assertEqual(result.program.output, b"output-before-exit")
        self.assertEqual(result.program.exit_code, 7)
        self.assertEqual(result.status, "completed")

    def test_non_skip_skipif_output_is_a_runner_gap_and_never_runs_file(self):
        for output in (b"Warning: fixture unavailable", b"XFAIL expected later",
                       b"arbitrary diagnostic"):
            with self.subTest(output=output):
                result = self.invoke(b"<?php echo 'must not run'; ?>",
                                     b"<?php echo '" + output + b"'; ?>")
                self.assertEqual(result.status, "runner_gap")
                self.assertIsNone(result.program)

    def test_php_output_is_preserved_as_original_bytes(self):
        result = self.invoke(b"<?php echo hex2bin('e900ff'); ?>")
        self.assertEqual(result.program.output, b"\xe9\x00\xff")

    def test_official_skipif_prefix_semantics_and_unknown_cause_stays_unknown(self):
        # run-tests trims SKIPIF output; empty means continue. A case-insensitive
        # leading four-byte SKIP token skips, including the word SKIPPER.
        proceed = self.invoke(b"<?php echo 'ran'; ?>", b"<?php echo '  '; ?>")
        self.assertIsNotNone(proceed.program)
        for output in (b"SKIP missing dependency", b"skipper by fixture"):
            with self.subTest(output=output):
                result = self.invoke(b"<?php echo 'must not run'; ?>",
                                     b"<?php echo '" + output + b"'; ?>")
                self.assertIsNone(result.program)
                self.assertNotIn(result.status, {"environment_skip", "unsupported", "pass"})

    def test_nonzero_or_malformed_skipif_does_not_run_file(self):
        result = self.invoke(b"<?php echo 'must not run'; ?>", b"<?php exit(2); ?>")
        self.assertIsNone(result.program)
        self.assertNotEqual(result.status, "pass")

    def test_release_mismatch_does_not_execute_skipif_or_file(self):
        result = self.invoke(b"<?php echo 'must not run'; ?>", b"<?php echo 'must not run'; ?>",
                             release="8.4.0")
        self.assertIsNone(result.skipif)
        self.assertIsNone(result.program)
        self.assertNotEqual(result.status, "pass")

    def test_timeout_and_output_overflow_never_claim_complete_effects(self):
        for source, timeout, limit in (
            (b"<?php while (true) {} ?>", 0.1, 4096),
            (b"<?php echo str_repeat('x', 1000000); ?>", 2, 64),
        ):
            with self.subTest(timeout=timeout, limit=limit):
                result = self.invoke(source, timeout=timeout, output_limit=limit)
                self.assertNotEqual(result.status, "pass")
                self.assertFalse(result.effects_complete)

    def test_infinite_output_is_bounded_under_an_outer_process_deadline(self):
        """A child process keeps a pipe stall or a stuck runner from hanging unittest."""
        worker = self.root / "run_session_worker.py"
        payload_path = self.root / "case.json"
        result_path = self.root / "result.json"
        worker.write_text("""import base64, importlib.util, json, sys
payload = json.load(open(sys.argv[1]))
payload['source'] = base64.b64decode(payload['source'])
payload['skipif'] = base64.b64decode(payload['skipif'])
payload['fixtures'] = {name: base64.b64decode(data) for name, data in payload['fixtures'].items()}
spec = importlib.util.spec_from_file_location('worker_session', payload.pop('module_path'))
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
try:
    observed = module.run_case(**payload)
    json.dump({'status': observed.status, 'effects_complete': observed.effects_complete,
               'output_size': len(observed.program.output) if observed.program else 0},
              open(sys.argv[2], 'w'))
except BaseException as exc:
    json.dump({'error': type(exc).__name__ + ': ' + str(exc)}, open(sys.argv[2], 'w'))
""")
        payload = dict(image=IMAGE, executable=EXECUTABLE, platform=PLATFORM,
                       release=RELEASE,
                       source=base64.b64encode(
                           b"<?php while (true) { echo str_repeat('x', 8192); } ?>"
                       ).decode("ascii"),
                       skipif=base64.b64encode(b"").decode("ascii"), fixtures={},
                       timeout=0.25, output_limit=64,
                       module_path=str(ROOT / "tools/phpt_session.py"))
        payload_path.write_text(json.dumps(payload))
        try:
            child = subprocess.run([sys.executable, str(worker), str(payload_path),
                                    str(result_path)], check=False, timeout=8,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        except subprocess.TimeoutExpired:
            self.fail("run_case exceeded the outer 8 second deadline")
        self.assertEqual(child.returncode, 0, child.stderr.decode("utf-8", "replace"))
        observation = json.loads(result_path.read_text())
        self.assertNotIn("error", observation)
        self.assertNotEqual(observation["status"], "pass")
        self.assertFalse(observation["effects_complete"])
        self.assertLessEqual(observation["output_size"], 64)

    def test_repeated_cases_do_not_leak_fixture_contents(self):
        first = self.invoke(b"<?php echo file_get_contents('secret.txt'); ?>",
                            fixtures={"secret.txt": b"first-case"})
        second = self.invoke(b"<?php echo file_exists('secret.txt') ? 'leaked' : 'clean'; ?>")
        self.assertEqual(first.program.output, b"first-case")
        self.assertEqual(second.program.output, b"clean")
        self.assertTrue(first.cleanup_confirmed)
        self.assertTrue(second.cleanup_confirmed)


class PHPTSessionCancellationTests(unittest.TestCase):
    """Use a temporary Docker CLI double; this test never contacts Docker."""

    def test_sigterm_during_create_uses_its_label_and_unknown_cleanup_is_not_success(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            docker = root / "docker"
            log = root / "docker.jsonl"
            ready = root / "create-ready"
            state = root / "state.json"
            result = root / "result.json"
            docker.write_text("#!" + sys.executable + "\n" + r'''import json, os, sys, time
args = sys.argv[1:]
action = args[0] if args else ""
with open(os.environ["FAKE_DOCKER_LOG"], "a") as stream:
    stream.write(json.dumps(args) + "\n")
if action == "create":
    label = args[args.index("--label") + 1]
    with open(os.environ["FAKE_DOCKER_STATE"], "w") as stream:
        json.dump({"label": label, "pid": os.getpid()}, stream)
    open(os.environ["FAKE_DOCKER_READY"], "w").write("created")
    print("abcdef123456", flush=True)
    time.sleep(60)
elif action == "ps":
    sys.stderr.write("container state unavailable")
    sys.exit(1)
elif action == "rm":
    print("abcdef123456")
else:
    print("unsupported fake docker action", file=sys.stderr)
    sys.exit(2)
''')
            docker.chmod(0o755)
            worker = root / "cancel_worker.py"
            worker.write_text("""import base64, importlib.util, json, signal, sys
payload = json.load(open(sys.argv[1]))
payload['source'] = base64.b64decode(payload['source'])
payload['skipif'] = base64.b64decode(payload['skipif'])
spec = importlib.util.spec_from_file_location('cancel_session', payload.pop('module_path'))
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
def interrupt(_signum, _frame):
    raise KeyboardInterrupt()
signal.signal(signal.SIGTERM, interrupt)
try:
    observed = module.run_case(**payload)
    report = {'status': observed.status, 'cleanup_confirmed': observed.cleanup_confirmed}
except BaseException as exc:
    report = {'status': 'interrupted', 'error': type(exc).__name__, 'cleanup_confirmed': False}
json.dump(report, open(sys.argv[2], 'w'))
""")
            payload = dict(image=IMAGE, executable=EXECUTABLE, platform=PLATFORM,
                           release=RELEASE,
                           source=base64.b64encode(b"<?php echo 'not run'; ?>").decode("ascii"),
                           skipif=base64.b64encode(b"").decode("ascii"), fixtures={},
                           timeout=2, output_limit=4096,
                           module_path=str(ROOT / "tools/phpt_session.py"))
            payload_file = root / "case.json"
            payload_file.write_text(json.dumps(payload))
            env = dict(os.environ)
            env.update({"PATH": str(root) + os.pathsep + env.get("PATH", ""),
                        "FAKE_DOCKER_LOG": str(log), "FAKE_DOCKER_READY": str(ready),
                        "FAKE_DOCKER_STATE": str(state)})
            child = subprocess.Popen([sys.executable, str(worker), str(payload_file), str(result)],
                                     env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                deadline = time.monotonic() + 5
                while not ready.exists() and child.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(ready.exists(), "fake container was not created before the deadline")
                state_record = json.loads(state.read_text())
                child.send_signal(signal.SIGTERM)
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait(timeout=2)
                    self.fail("cancelled run_case did not return before the outer timeout")
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=2)
            report = json.loads(result.read_text()) if result.exists() else {}
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            create = next(args for args in calls if args[0] == "create")
            label = create[create.index("--label") + 1]
            self.assertEqual(label, state_record["label"])
            label_lookups = [args for args in calls if args[0] == "ps" and "--filter" in args]
            self.assertTrue(label_lookups, "cancellation cleanup did not query containers by its unique label")
            self.assertIn("label=" + label, label_lookups[-1])
            self.assertNotEqual(report.get("status"), "pass")
            self.assertFalse(report.get("cleanup_confirmed", False))


if __name__ == "__main__":
    unittest.main()
