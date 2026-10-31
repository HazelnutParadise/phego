"""Regression contract for PHP 8.5.12 PHPT ENV handling.

Known runner-owned names requiring exclusion and a ``runner_gap`` report are
REQUEST_METHOD, QUERY_STRING, PATH_TRANSLATED, SCRIPT_FILENAME,
TEST_PHP_EXTRA_ARGS, REDIRECT_STATUS, HTTP_COOKIE, CONTENT_TYPE,
CONTENT_LENGTH. PHP run-tests.php clears or rewrites these around test execution.
This test deliberately makes no claims about other CGI/process names.
Behavioral execution uses the repository's digest-pinned linux/386 PHP image.
"""
import importlib.util
import os
from pathlib import Path
import re
import signal
import tempfile
import time
import shutil
import subprocess
import sys
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


session = load_tool("phego_phpt_session_env", TOOLS / "phpt_session.py")
executor = load_tool("phego_phpt_execute_env", TOOLS / "phpt_execute.py")

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


def parse(data, release=RELEASE):
    parser = load_tool("phego_phpt_env", TOOLS / "phpt_env.py")
    return parser.parse_env(data, release)


class PHPTEnvParserTests(unittest.TestCase):
    def test_php_8512_split_trim_first_equals_and_duplicate_position(self):
        self.assertEqual(parse(b"\n A=first\nB=\"two words\"\nA=last=part\nEMPTY=\nno-equals\n=empty-name\n"),
                         (b"A=last=part", b'B="two words"', b"EMPTY="))

    def test_pwd_substitution_and_whitespace_trim_match_run_tests(self):
        self.assertEqual(parse(b" PHEGO_PATH={PWD}/sub dir \n"),
                         (b"PHEGO_PATH=/work/sub dir",))

    def test_limits_nul_unsafe_names_and_release_are_rejected(self):
        invalid = (
            (b"X=" + b"v" * (64 * 1024), RELEASE),
            (b"A=1\n" * 257, RELEASE),
            (b"A=x\0y", RELEASE),
            (b"BAD-NAME=x", RELEASE),
            (b"1BAD=x", RELEASE),
            (b"A=x", "8.5.11"),
        )
        for data, release in invalid:
            with self.subTest(data=data[:24], release=release), self.assertRaises((ValueError, TypeError)):
                parse(data, release)

    def test_type_errors_are_rejected(self):
        with self.assertRaises((ValueError, TypeError)):
            parse("A=x")


class PHPTEnvExecutionTests(unittest.TestCase):
    def run_case(self, env, *, skipif=b"", source=None):
        if source is None:
            source = b"<?php echo getenv('PHEGO_ENV_VALUE'), '|', getenv('PHEGO_ENV_EMPTY'), '|', getenv('PHEGO_ENV_PWD'); ?>"
        return session.run_case(
            image=IMAGE, executable=EXECUTABLE, platform=PLATFORM, release=RELEASE,
            source=source, skipif=skipif, fixtures={}, timeout=10, output_limit=4096,
            env=env,
        )

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_env_reaches_skipif_and_file_without_changing_profile(self):
        env = parse(b"PHEGO_ENV_VALUE=hello world\nPHEGO_ENV_EMPTY=\nPHEGO_ENV_PWD={PWD}")
        skipif = b"<?php if (getenv('PHEGO_ENV_VALUE') !== 'hello world') exit(1); ?>"
        source = b"<?php echo getenv('PHEGO_ENV_VALUE'), '|', getenv('PHEGO_ENV_EMPTY'), '|', getenv('PHEGO_ENV_PWD'); ?>"
        result = self.run_case(env, skipif=skipif, source=source)
        self.assertEqual(result.status, "completed")
        self.assertEqual(result.profile.output, b"8.5.12\n" + (b"4" if PLATFORM == "linux/386" else b"8"))
        self.assertEqual(result.program.output, b"hello world||/work")

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_shell_normalizes_pwd_and_ifs_before_skipif_and_file(self):
        env = b"PWD=/false-pwd\nIFS=abc\nPPID=999999"
        skipif = (
            b"<?php if (getenv('PWD') !== '/work' || "
            b"bin2hex(getenv('IFS')) !== '20090a' || "
            b"getenv('PPID') === '999999') exit(1); ?>"
        )
        source = (
            b"<?php echo getenv('PWD'), '|', bin2hex(getenv('IFS')), '|', "
            b"getenv('PPID'); ?>"
        )
        sections = {
            "TEST": b"shell-adjusted ENV values",
            "ENV": env,
            "SKIPIF": skipif,
            "FILE": source,
            "EXPECTF": b"/work|20090a|%d",
        }
        # executor.run routes through run_case while preserving a sanitized
        # runner_gap if ENV normalization rejects a shell-owned variable.
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertEqual(result["status"], "pass", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_env_isolated_between_cases(self):
        first = self.run_case(parse(b"PHEGO_ENV_VALUE=first"))
        second = self.run_case(parse(b"PHEGO_ENV_VALUE=second"))
        self.assertEqual(first.program.output, b"first||")
        self.assertEqual(second.program.output, b"second||")

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_host_docker_argv_never_contains_env_values(self):
        sentinel = b"PHEGO_ENV_ARGV_SENTINEL_59C9"
        env = parse(b"PHEGO_ENV_VALUE=" + sentinel)
        source = b"<?php echo getenv('PHEGO_ENV_VALUE'); ?>"
        real_popen = session.subprocess.Popen
        host_argv = []

        def recording_popen(argv, *args, **kwargs):
            host_argv.append(argv)
            return real_popen(argv, *args, **kwargs)

        with mock.patch.object(session.subprocess, "Popen", side_effect=recording_popen):
            observation = self.run_case(env, source=source)
        self.assertEqual(observation.status, "completed", observation.reason)
        self.assertTrue(observation.cleanup_confirmed)
        self.assertEqual(observation.program.output, sentinel)
        self.assertTrue(host_argv, "expected Docker subprocesses to be observed")
        self.assertTrue(all(sentinel not in repr(argv).encode("utf-8") for argv in host_argv))

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_executor_combines_env_and_ini_across_skipif_and_file_without_argv_leak(self):
        sentinel = b"PHEGO_ENV_SHELL_SENTINEL_816B"
        literal = b"quote' ; $(printf " + sentinel + b") `echo shell`"
        env_data = b"PHEGO_ENV_EMPTY=\nPHEGO_ENV_LITERAL=" + literal
        sections = {
            "TEST": b"ENV and INI session",
            "ENV": env_data,
            "INI": b"precision=17\n",
            "SKIPIF": (
                b"<?php if (getenv('PHEGO_ENV_EMPTY') !== false || "
                b"getenv('PHEGO_ENV_LITERAL') !== " + repr(literal).encode("ascii") +
                b" || ini_get('precision') !== '14') exit(1); ?>"
            ),
            "FILE": (
                b"<?php echo getenv('PHEGO_ENV_EMPTY'), '|', "
                b"getenv('PHEGO_ENV_LITERAL'), '|', ini_get('precision'); ?>"
            ),
            "EXPECT": b"|" + literal + b"|17",
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
        self.assertTrue(all(sentinel not in repr(argv).encode("utf-8") for argv in host_argv))
        self.assertTrue(all(literal not in repr(argv).encode("utf-8") for argv in host_argv))

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_large_single_quote_ini_with_env_stays_within_input_bound_and_executes(self):
        quote_count = 40000
        ini_value = b"'" * quote_count
        ini_data = b'error_log="' + ini_value + b'"\n'
        env_data = b"PHEGO_ENV_LARGE_INI=present"
        self.assertLess(len(ini_data) + len(env_data), 64 * 1024)
        sections = {
            "TEST": b"large quoted INI and ENV",
            "ENV": env_data,
            "INI": ini_data,
            "FILE": b"<?php echo getenv('PHEGO_ENV_LARGE_INI'), '|', strlen(ini_get('error_log')), '|ok'; ?>",
            "EXPECT": b"present|" + str(quote_count * 2).encode("ascii") + b"|ok",
        }
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 30, 4096)
        self.assertEqual(result["status"], "pass", result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_env_case_timeout_confirms_container_cleanup(self):
        observation = self.run_case(
            parse(b"PHEGO_ENV_TIMEOUT=bounded"),
            source=b"<?php while (true) {} ?>",
        )
        self.assertEqual(observation.status, "timeout", observation)
        self.assertIsNotNone(observation.program)
        self.assertEqual(observation.program.status, "timeout")
        self.assertTrue(observation.cleanup_confirmed)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_sigterm_during_env_file_execution_removes_owned_container(self):
        with tempfile.TemporaryDirectory(prefix="phego-env-sigterm-") as directory:
            root = Path(directory)
            worker = root / "worker.py"
            worker.write_text("""import importlib.util, sys, uuid
path = sys.argv[1]
spec = importlib.util.spec_from_file_location('env_cancel_session', path)
module = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = module
spec.loader.exec_module(module)
fixed_ids = iter((uuid.UUID(hex=sys.argv[6]), uuid.UUID(hex=sys.argv[7])))
module.uuid.uuid4 = lambda: next(fixed_ids)
module.run_case(
    image=sys.argv[2], executable=sys.argv[3], platform=sys.argv[4], release=sys.argv[5],
    source=b"<?php while (true) {} ?>", skipif=b"", fixtures={}, timeout=30,
    output_limit=4096, env=(b"PHEGO_ENV_CANCEL=present",),
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
                        if top.returncode == 0 and "/work/.phego-file.php" in top.stdout:
                            owned_id = candidate
                            break
                    time.sleep(0.1)
                self.assertIsNotNone(owned_id, "ENV FILE PHP process did not start before deadline")
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

    def test_env_limits_report_numeric_bounds_without_values_or_docker(self):
        secret = b"PHEGO_ENV_LIMIT_SENTINEL_71F3"
        cases = (
            (b"PHEGO_ENV_SECRET=" + secret + b"x" * (64 * 1024), "64 KiB"),
            (b"PHEGO_ENV_SECRET=" + secret + b"=x\n" * 257, "256 lines"),
        )
        for env_data, bound in cases:
            sections = {
                "TEST": b"oversized ENV",
                "ENV": env_data,
                "FILE": b"<?php echo 'must not run'; ?>",
                "EXPECT": b"must not run",
            }
            with self.subTest(bound=bound), mock.patch.object(session.subprocess, "Popen") as popen:
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 5, 4096)
            self.assertEqual(result["status"], "runner_gap")
            self.assertEqual(result["execution"], "not_run")
            self.assertIn(bound, result["reason"])
            self.assertNotIn(secret.decode(), repr(result))
            popen.assert_not_called()

    def test_direct_unsafe_env_is_rejected_before_docker(self):
        for env in ((b"BAD-NAME=x",), (b"A=x\0y",), (b"1BAD=x",)):
            with self.subTest(env=env), mock.patch.object(session.subprocess, "Popen") as popen:
                with self.assertRaises((ValueError, TypeError)):
                    session.run_case(
                        image=IMAGE, executable=EXECUTABLE, platform=PLATFORM, release=RELEASE,
                        source=b"<?php ?>", skipif=b"", fixtures={}, timeout=1, output_limit=128,
                        env=env,
                    )
                popen.assert_not_called()

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_optind_that_breaks_reference_shell_cannot_be_reported_as_pass(self):
        sections = {
            "TEST": b"unsafe shell startup ENV",
            "ENV": b"OPTIND=SENTINEL_OPTIND",
            "FILE": b"<?php echo 'must not pass'; ?>",
            "EXPECT": b"must not pass",
        }
        result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM, RELEASE, 15, 4096)
        self.assertNotEqual(result["status"], "pass", result)

    def test_runner_owned_cgi_and_process_names_are_excluded_as_runner_gap(self):
        names = (b"REQUEST_METHOD", b"QUERY_STRING", b"PATH_TRANSLATED",
                 b"SCRIPT_FILENAME", b"TEST_PHP_EXTRA_ARGS", b"REDIRECT_STATUS",
                 b"HTTP_COOKIE", b"CONTENT_TYPE", b"CONTENT_LENGTH")
        for name in names:
            sections = {"TEST": b"env", "FILE": b"<?php echo 'ran'; ?>",
                        "EXPECT": b"ran", "ENV": name + b"=unsafe-to-assume"}
            with self.subTest(name=name):
                result = executor.run(sections, IMAGE, EXECUTABLE, PLATFORM,
                                      RELEASE, 2, 128)
                self.assertEqual(result["status"], "runner_gap")
                self.assertIn(name.decode(), result.get("reason", ""))
                self.assertEqual(result.get("execution"), "not_run")
