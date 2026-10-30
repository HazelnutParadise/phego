"""PHPT STDIN contract tests for the pinned PHP 8.5.12 reference runner."""
import importlib.util
import hashlib
import base64
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
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


session = load_tool("phego_phpt_stdin_session", TOOLS / "phpt_session.py")
executor = load_tool("phego_phpt_stdin_execute", TOOLS / "phpt_execute.py")
IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
EXECUTABLE = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
STDIN_LIMIT = 1024 * 1024
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def call_case(**overrides):
    args = dict(
        image=IMAGE, executable=EXECUTABLE, platform=PLATFORM, release=RELEASE,
        source=b"<?php echo bin2hex(file_get_contents('php://stdin')); ?>",
        skipif=b"", fixtures={}, timeout=10, output_limit=4096,
    )
    args.update(overrides)
    return session.run_case(**args)


def run_executor_with_outer_deadline(sections, *, timeout, output_limit, outer_timeout=20):
    """Run executor under a deadline and clean only its uniquely identified container."""
    with tempfile.TemporaryDirectory(prefix="phego-stdin-outer-") as directory:
        root = Path(directory)
        container_token = uuid.uuid4().hex
        label_token = uuid.uuid4().hex
        expected_name = "/phego-phpt-" + container_token
        list_args = ["docker", "ps", "-aq", "--filter",
                     "label=phego.phpt-session=" + label_token]
        payload_path = root / "sections.json"
        payload_path.write_text(json.dumps({name: base64.b64encode(data).decode("ascii")
                                            for name, data in sections.items()}))
        worker = root / "executor_worker.py"
        worker.write_text("""import base64, importlib.util, json, pathlib, sys, uuid
sys.path.insert(0, str(pathlib.Path(sys.argv[1]).parent))
spec = importlib.util.spec_from_file_location('bounded_stdin_execute', sys.argv[1])
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
fixed_ids = iter((uuid.UUID(hex=sys.argv[9]), uuid.UUID(hex=sys.argv[10])))
module.run_case.__globals__['uuid'].uuid4 = lambda: next(fixed_ids)
sections = {name: base64.b64decode(data) for name, data in json.loads(pathlib.Path(sys.argv[2]).read_text()).items()}
result = module.run(sections, sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6],
                    float(sys.argv[7]), int(sys.argv[8]))
print(json.dumps(result))
""")
        child = subprocess.Popen(
            [sys.executable, str(worker), str(TOOLS / "phpt_execute.py"), str(payload_path),
             IMAGE, EXECUTABLE, PLATFORM, RELEASE, str(timeout), str(output_limit),
             container_token, label_token],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        timed_out = False
        stdout = stderr = ""

        def query_owned():
            listed = subprocess.run(list_args, capture_output=True, text=True,
                                    timeout=3, check=False)
            if listed.returncode != 0:
                raise AssertionError("exact Docker label query failed: " + listed.stderr)
            matches = listed.stdout.split()
            if len(matches) > 1:
                raise AssertionError("unique outer-worker label matched multiple containers")
            if not matches:
                return None
            candidate = matches[0]
            name = subprocess.run(
                ["docker", "inspect", "-f", "{{.Name}}", candidate],
                capture_output=True, text=True, timeout=3, check=False,
            )
            if name.returncode != 0 or name.stdout.strip() != expected_name:
                raise AssertionError("exact label resolved name " + repr(name.stdout.strip())
                                     + "; expected " + repr(expected_name))
            return candidate

        try:
            try:
                stdout, stderr = child.communicate(timeout=outer_timeout)
            except subprocess.TimeoutExpired:
                timed_out = True
                child.kill()
                stdout, stderr = child.communicate(timeout=3)
        finally:
            if child.poll() is None:
                child.kill()
                child.wait(timeout=3)
            deadline = time.monotonic() + 8
            removed = False
            while time.monotonic() < deadline:
                candidate = query_owned()
                if candidate is None:
                    removed = True
                    break
                subprocess.run(["docker", "rm", "--force", candidate],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                               timeout=5, check=False)
                time.sleep(0.1)
            if not removed:
                raise AssertionError("outer-worker container cleanup could not be confirmed")
        if timed_out:
            raise AssertionError("bounded executor worker exceeded outer deadline of "
                                 + str(outer_timeout) + " seconds")
        if child.returncode != 0:
            raise AssertionError("bounded executor worker failed: " + stderr)
        return json.loads(stdout)


class PHPTStdinValidationTests(unittest.TestCase):
    def test_direct_stdin_rejects_wrong_type_and_over_limit_before_docker(self):
        real_popen = session.subprocess.Popen
        for value in ("text", bytearray(b"bytes"), b"x" * (STDIN_LIMIT + 1)):
            with self.subTest(value_type=type(value).__name__, size=len(value)):
                with mock.patch.object(session.subprocess, "Popen", wraps=real_popen) as popen:
                    with self.assertRaises((TypeError, ValueError)):
                        call_case(stdin=value)
                    popen.assert_not_called()

    def test_unsupported_release_rejects_stdin_before_docker(self):
        with mock.patch.object(session.subprocess, "Popen") as popen:
            with self.assertRaises((TypeError, ValueError)):
                call_case(release="8.5.11", stdin=b"secret")
            popen.assert_not_called()

    def test_stdin_content_is_not_returned_in_input_diagnostics(self):
        sentinel = b"PHEGO_STDIN_DIAGNOSTIC_SECRET_7a1e"
        with mock.patch.object(session.subprocess, "Popen") as popen:
            with self.assertRaises((TypeError, ValueError)) as caught:
                call_case(stdin=sentinel + b"\0" + b"x" * STDIN_LIMIT)
        self.assertNotIn(sentinel.decode(), str(caught.exception))
        popen.assert_not_called()

    def test_executor_rejects_unsupported_release_and_oversized_stdin_before_docker(self):
        sentinel = b"PHEGO_OVERSIZED_STDIN_SECRET_d83b"
        base = {"TEST": b"invalid STDIN is rejected before execution",
                "FILE": b"<?php echo 'must not run'; ?>", "EXPECT": b"must not run"}
        with mock.patch.object(session.subprocess, "Popen") as popen:
            unsupported = executor.run(
                {**base, "STDIN": sentinel}, IMAGE, EXECUTABLE, PLATFORM,
                "8.5.11", 10, 4096,
            )
            self.assertNotEqual(unsupported["status"], "pass", unsupported)
            self.assertIn("STDIN", unsupported["reason"])
            oversized = executor.run(
                {**base, "STDIN": sentinel + b"x" * (STDIN_LIMIT + 1)},
                IMAGE, EXECUTABLE, PLATFORM, RELEASE, 10, 4096,
            )
            self.assertNotEqual(oversized["status"], "pass", oversized)
            self.assertIn("STDIN", oversized["reason"])
            self.assertTrue(
                "1 MiB" in oversized["reason"] or str(STDIN_LIMIT) in oversized["reason"],
                oversized,
            )
            self.assertNotIn(sentinel.decode("ascii"), oversized["reason"])
            popen.assert_not_called()

    def test_fixture_cannot_clobber_internal_file_phase_name(self):
        with mock.patch.object(session.subprocess, "Popen") as popen:
            result = executor.run(
                {"TEST": b"reserved runner script fixture", "FILE": b"<?php echo 'fake'; ?>",
                 "EXPECT": b"fake", "STDIN": b"input", "ENV": b"PHEGO_STDIN_ENV=present"},
                IMAGE, EXECUTABLE, PLATFORM, RELEASE, 10, 4096,
                fixtures={".phego-stdin-run.sh": b"must not replace the runner script"},
            )
            self.assertNotEqual(result["status"], "pass", result)
            popen.assert_not_called()


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTStdinExecutionTests(unittest.TestCase):
    def test_binary_bytes_round_trip_without_newline_and_with_ini_env_clean_fixture(self):
        raw = b"\x00\xffline\r\nsecond"
        sections = {
            "TEST": b"STDIN stays raw through the FILE phase",
            "STDIN": raw,
            "INI": b"precision=17\n",
            "ENV": b"PHEGO_STDIN_ENV=ready",
            "SKIPIF": b"<?php if (fread(STDIN, 1) !== '') exit(1); ?>",
            "FILE": (
                b"<?php echo bin2hex(file_get_contents('php://stdin')), '|', "
                b"ini_get('precision'), '|', getenv('PHEGO_STDIN_ENV'), '|', "
                b"file_get_contents(__DIR__.'/fixture.txt'); ?>"
            ),
            "CLEAN": (
                b"<?php if (fread(STDIN, 1) !== '') { echo 'PHEGO_CLEAN_STDIN_NOT_EOF'; exit(2); } "
                b"file_put_contents('cleaned', 'yes'); ?>"
            ),
            "EXPECT": b"" + raw.hex().encode() + b"|17|ready|fixture",
        }
        result = executor.run(
            sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096,
            fixtures={"fixture.txt": b"fixture"},
        )
        self.assertEqual(result["status"], "pass", result)

    def test_empty_stdin_is_eof_and_binary_output_remains_empty(self):
        result = executor.run(
            {"TEST": b"empty STDIN", "FILE": b"<?php var_export(file_get_contents('php://stdin')); ?>",
             "EXPECT": b"''", "STDIN": b""},
            IMAGE, EXECUTABLE, PLATFORM, RELEASE, 10, 4096,
        )
        self.assertEqual(result["status"], "pass", result)

    def test_large_bounded_stdin_round_trip_with_bounded_output(self):
        data = bytes(range(256)) * (STDIN_LIMIT // 256)
        result = executor.run(
            {"TEST": b"bounded STDIN", "FILE": b"<?php echo hash('sha256', file_get_contents('php://stdin')); ?>",
             "EXPECT": hashlib.sha256(data).hexdigest().encode("ascii"), "STDIN": data},
            IMAGE, EXECUTABLE, PLATFORM, RELEASE, 20, 4096,
        )
        self.assertEqual(result["status"], "pass", result)
        self.assertLessEqual(len(result["reason"].encode("utf-8")), 4096)

    def test_file_can_close_stdin_early_and_complete(self):
        result = executor.run(
            {"TEST": b"FILE closes STDIN", "FILE": b"<?php fclose(STDIN); echo 'closed'; ?>",
             "EXPECT": b"closed", "STDIN": b"x" * STDIN_LIMIT},
            IMAGE, EXECUTABLE, PLATFORM, RELEASE, 10, 4096,
        )
        self.assertEqual(result["status"], "pass", result)

    def test_stdin_is_reserved_for_file_and_not_leaked_in_host_docker_argv(self):
        sentinel = b"PHEGO_STDIN_ARGV_SECRET_f2c9\x00\xff"
        sections = {
            "TEST": b"STDIN must reach FILE only",
            "INI": b"precision=17",
            "ENV": b"PHEGO_STDIN_ARGV_ENV=not-on-argv",
            "SKIPIF": b"<?php if (fread(STDIN, 1) !== '') exit(1); ?>",
            "FILE": b"<?php echo bin2hex(file_get_contents('php://stdin')), '|', ini_get('precision'), '|', getenv('PHEGO_STDIN_ARGV_ENV'); ?>",
            "CLEAN": (
                b"<?php if (fread(STDIN, 1) !== '') { echo 'PHEGO_CLEAN_STDIN_NOT_EOF'; exit(2); } ?>"
            ),
            "STDIN": sentinel,
            "EXPECT": sentinel.hex().encode() + b"|17|not-on-argv",
        }
        real_popen = session.subprocess.Popen
        host_argv = []

        def recording_popen(argv, *args, **kwargs):
            host_argv.append(argv)
            return real_popen(argv, *args, **kwargs)

        with mock.patch.object(session.subprocess, "Popen", side_effect=recording_popen):
            result = executor.run(
                sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096,
            )
        self.assertEqual(result["status"], "pass", result)
        self.assertTrue(host_argv, "expected Docker subprocesses to be observed")
        for argv in host_argv:
            rendered = repr(argv).encode("utf-8")
            self.assertNotIn(sentinel, rendered)
            self.assertNotIn(b"PHEGO_STDIN_ARGV_SECRET", rendered)
            self.assertNotIn(b"f2c9", rendered)
            self.assertNotIn(b"not-on-argv", rendered)

    def test_parse_and_execute_keep_cr_before_env_marker_inside_stdin(self):
        payload = b"abc\r--ENV--\nPHEGO_VALUE=oops\n"
        phpt = (
            b"--TEST--\nCR in STDIN is data, not a section boundary\n"
            b"--FILE--\n<?php echo bin2hex(file_get_contents('php://stdin')); ?>\n"
            b"--EXPECT--\n" + payload.hex().encode("ascii") + b"\n"
            b"--STDIN--\n" + payload
        )
        sections = executor.parse_phpt(phpt, "stdin-cr-boundary.phpt")
        self.assertEqual(sections["STDIN"], payload)
        self.assertNotIn("ENV", sections)
        result = executor.run(
            sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096,
        )
        self.assertEqual(result["status"], "pass", result)

    def test_skipif_symlink_at_internal_runner_script_path_is_not_overwritten_or_passed(self):
        sections = {
            "TEST": b"SKIPIF cannot pre-create the internal STDIN runner script",
            "ENV": b"PHEGO_STDIN_SYMLINK_CASE=1",
            "SKIPIF": (
                b"<?php if (!symlink('fixture.txt', '.phego-stdin-run.sh')) "
                b"{ echo 'PHEGO_SYMLINK_SETUP_FAILED'; exit(1); } ?>"
            ),
            "FILE": b"<?php echo is_link('.phego-stdin-run.sh') ? 'still-link' : 'missing'; ?>",
            "EXPECT": b"missing",
            "STDIN": b"payload",
        }
        result = executor.run(
            sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096,
            fixtures={"fixture.txt": b"fixture"},
        )
        self.assertNotEqual(result["status"], "pass", result)

    def test_file_runs_after_internal_stdin_script_has_been_removed(self):
        sections = {
            "TEST": b"internal STDIN runner script is removed before FILE",
            "ENV": b"PHEGO_STDIN_REMOVAL_CHECK=ready",
            "SKIPIF": b"<?php if (fread(STDIN, 1) !== '') exit(1); ?>",
            "FILE": b"<?php echo getenv('PHEGO_STDIN_REMOVAL_CHECK'), '|', file_exists('.phego-stdin-run.sh') ? 'present' : 'removed'; ?>",
            "EXPECT": b"ready|removed",
            "STDIN": b"payload",
        }
        result = executor.run(
            sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096,
        )
        self.assertEqual(result["status"], "pass", result)

    def test_stdin_with_ini_env_file_timeout_is_bounded_and_reported(self):
        sections = {
            "TEST": b"STDIN plus INI and ENV FILE timeout",
            "STDIN": b"payload",
            "INI": b"precision=17",
            "ENV": b"PHEGO_STDIN_TIMEOUT=1",
            "FILE": b"<?php while (true) {} ?>",
            "EXPECT": b"",
        }
        result = run_executor_with_outer_deadline(
            sections, timeout=2, output_limit=4096, outer_timeout=15,
        )
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertIn("timed out", result["reason"])

    def test_stdin_with_ini_env_file_output_limit_is_bounded_and_reported(self):
        sections = {
            "TEST": b"STDIN plus INI and ENV FILE output limit",
            "STDIN": b"payload",
            "INI": b"precision=17",
            "ENV": b"PHEGO_STDIN_OUTPUT_LIMIT=1",
            "FILE": b"<?php while (true) { echo str_repeat('x', 8192); } ?>",
            "EXPECT": b"",
        }
        result = run_executor_with_outer_deadline(
            sections, timeout=5, output_limit=256, outer_timeout=15,
        )
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertIn("output limit", result["reason"])

    def test_sigterm_during_stdin_ini_env_file_cleans_only_exact_owned_container(self):
        with tempfile.TemporaryDirectory(prefix="phego-stdin-sigterm-") as directory:
            root = Path(directory)
            worker = root / "worker.py"
            worker.write_text("""import importlib.util, sys, uuid
spec = importlib.util.spec_from_file_location('stdin_cancel_session', sys.argv[1])
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
fixed_ids = iter((uuid.UUID(hex=sys.argv[6]), uuid.UUID(hex=sys.argv[7])))
module.uuid.uuid4 = lambda: next(fixed_ids)
module.run_case(
    image=sys.argv[2], executable=sys.argv[3], platform=sys.argv[4], release=sys.argv[5],
    source=b"<?php while (true) {} ?>", skipif=b"", fixtures={}, timeout=30,
    output_limit=4096, ini=(b"precision=17",), env=(b"PHEGO_STDIN_CANCEL=1",),
    stdin=b"cancel-payload",
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

            def query_owned():
                listed = subprocess.run(list_args, capture_output=True, text=True,
                                        timeout=3, check=False)
                self.assertEqual(listed.returncode, 0,
                                 "exact Docker label query failed: " + listed.stderr)
                matches = listed.stdout.split()
                self.assertLessEqual(len(matches), 1, "unique label matched multiple containers")
                if not matches:
                    return None
                candidate = matches[0]
                name = subprocess.run(
                    ["docker", "inspect", "-f", "{{.Name}}", candidate],
                    capture_output=True, text=True, timeout=3, check=False,
                )
                self.assertEqual(name.returncode, 0, name.stderr)
                self.assertEqual(name.stdout.strip(), expected_name)
                return candidate

            try:
                deadline = time.monotonic() + 20
                while time.monotonic() < deadline and child.poll() is None:
                    candidate = query_owned()
                    if candidate:
                        top = subprocess.run(
                            ["docker", "top", candidate, "-eo", "pid,args"],
                            capture_output=True, text=True, timeout=3, check=False,
                        )
                        if top.returncode == 0 and "/work/.phego-file.php" in top.stdout:
                            owned_id = candidate
                            break
                    time.sleep(0.1)
                self.assertIsNotNone(owned_id, "FILE PHP process did not start before outer deadline")
                child.send_signal(signal.SIGTERM)
                try:
                    child.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    self.fail("SIGTERM did not stop the runner within its outer deadline")
                self.assertEqual(child.returncode, -signal.SIGTERM)

                deadline = time.monotonic() + 8
                gone = False
                while time.monotonic() < deadline:
                    remaining = query_owned()
                    if remaining is None:
                        gone = True
                        break
                    self.assertEqual(remaining, owned_id,
                                     "exact label resolved to a different container during cleanup")
                    time.sleep(0.1)
                self.assertTrue(gone, "same unique Docker label did not become empty")
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait(timeout=3)
                # Emergency cleanup is limited to this test's exact label and name.
                try:
                    candidate = query_owned()
                except (AssertionError, subprocess.SubprocessError):
                    candidate = None
                if candidate is not None:
                    subprocess.run(["docker", "rm", "--force", candidate],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                   timeout=5, check=False)
