"""PHPT CGI behavior against the pinned PHP 8.5.12 reference runner.

The expected request setup and output rules follow php-src php-8.5.12
run-tests.php and sapi/cgi/cgi_main.c. CGI behavior is opt-in through the
verified php-cgi binary and must run in the same disposable case session as
the CLI profile and execution. Docker-backed checks require the digest-pinned
linux/386 PHP image used by the neighboring runner tests.
"""
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import unittest
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


session = load_tool("phego_phpt_cgi_session", TOOLS / "phpt_session.py")
executor = load_tool("phego_phpt_cgi_execute", TOOLS / "phpt_execute.py")
cgi_support = load_tool("phego_phpt_cgi_support", TOOLS / "phpt_cgi.py")
IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
CLI = "/usr/local/bin/php"
CGI = "/usr/local/bin/php-cgi"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def base_sections(source=b"<?php echo 'body'; ?>", expected=b"body"):
    return {"TEST": b"CGI execution", "FILE": source, "EXPECT": expected}


class PHPTCGINumericComparisonTests(unittest.TestCase):
    def test_64_bit_integer_comparisons_preserve_int64_precision(self):
        # Zend compares two in-range IS_LONG values exactly. Do not route these
        # 64-bit integer spellings through binary floating-point first.
        self.assertFalse(cgi_support._numeric_equal(
            b"9007199254740992", b"9007199254740993", 64,
        ))
        self.assertTrue(cgi_support._numeric_equal(
            b"9223372036854775807", b"09223372036854775807", 64,
        ))


class PHPTCGRoutingTests(unittest.TestCase):
    def run_sections(self, sections, **kwargs):
        args = dict(image=IMAGE, executable=CLI, platform=PLATFORM, release=RELEASE,
                    timeout=10, output_limit=4096)
        args.update(kwargs)
        return executor.run(sections, **args)

    def test_cgi_selection_uses_exact_run_tests_nonempty_predicate(self):
        cases = (
            ({"GET": b""}, False), ({"GET": b"0"}, False),
            ({"GET": b"0\n"}, True), ({"POST": b"0"}, False),
            ({"COOKIE": b"0"}, False), ({"EXPECTHEADERS": b""}, False),
            ({"CGI": b""}, True),
        )
        for extra, needs_cgi in cases:
            sections = {**base_sections(), **extra}
            observation = executor.Observation("completed", b"body", 0)
            profile = executor.Observation("completed", b"8.5.12\n4\n", 0)
            with self.subTest(extra=extra), \
                 mock.patch.object(executor, "run_case") as run_case, \
                 mock.patch.object(executor, "execute_php", side_effect=(profile, observation)):
                result = self.run_sections(sections, cgi_executable=CGI if needs_cgi else None)
            if needs_cgi:
                run_case.assert_called_once()

    def test_missing_cgi_binary_is_runner_gap_before_any_sandbox_when_required(self):
        with mock.patch.object(executor, "run_case") as mocked, \
             mock.patch.object(executor, "execute_php") as cli:
            result = self.run_sections({**base_sections(), "GET": b"a=b"}, cgi_executable=None)
        self.assertEqual(result["status"], "runner_gap")
        self.assertEqual(result["execution"], "not_run")
        mocked.assert_not_called()
        cli.assert_not_called()

    def test_invalid_or_mismatched_cgi_profile_never_executes_php_test(self):
        for cgi in ("relative/php-cgi", "/usr/local/bin/php-cgi\0bad"):
            with self.subTest(cgi=cgi), mock.patch.object(executor, "run_case") as mocked, \
                 mock.patch.object(executor, "execute_php") as cli:
                result = self.run_sections({**base_sections(), "GET": b"a=b"}, cgi_executable=cgi)
            self.assertEqual(result["status"], "runner_gap")
            self.assertEqual(result["execution"], "not_run")
            mocked.assert_not_called()
            cli.assert_not_called()

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_absolute_cli_binary_is_rejected_by_cgi_profile_not_path_validation(self):
        result = self.run_sections({**base_sections(), "GET": b"a=b"}, cgi_executable=CLI)
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertIn("profile", result.get("reason", ""))

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_missing_php_cgi_inside_image_is_a_profile_gap(self):
        result = self.run_sections(
            {**base_sections(), "GET": b"a=b"},
            cgi_executable="/usr/local/bin/php-cgi-that-does-not-exist",
        )
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertEqual(result["execution"], "not_completed")

    def test_cgi_required_sections_and_unsupported_request_modes_are_explicit_gaps(self):
        for name in ("POST_RAW", "PUT", "GZIP_POST", "DEFLATE_POST", "ARGS"):
            sections = {**base_sections(), name: b"payload"}
            with self.subTest(section=name), mock.patch.object(executor, "run_case") as mocked:
                result = self.run_sections(sections, cgi_executable=CGI)
            self.assertEqual(result["status"], "runner_gap")
            self.assertEqual(result["execution"], "not_run")
            self.assertIn(name, result.get("reason", ""))
            mocked.assert_not_called()

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_supported_empty_cgi_section_runs_reference_case(self):
        result = self.run_sections({**base_sections(), "CGI": b""}, cgi_executable=CGI)
        self.assertNotEqual(result["status"], "runner_gap", result)

    def test_cli_exposes_the_explicit_cgi_executable_option(self):
        result = subprocess.run(
            [sys.executable, str(TOOLS / "phpt_execute.py"), "--help"],
            capture_output=True, text=True, timeout=10, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--cgi-executable", result.stdout)

    def test_leading_hyphen_query_that_decodes_to_option_is_rejected_preexecution(self):
        # CGI treats a QUERY_STRING without '=' as command-line options after
        # URL decoding. Keep this hostile shape outside the subprocess boundary.
        for query in (b"-%2dd", b"%20-x", b"%09--help"):
            sections = {**base_sections(), "GET": query}
            with self.subTest(query=query), mock.patch.object(executor, "run_case") as mocked:
                result = self.run_sections(sections, cgi_executable=CGI)
            self.assertEqual(result["status"], "runner_gap")
            self.assertEqual(result["execution"], "not_run")
            mocked.assert_not_called()

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_numeric_headers_use_the_verified_php_loose_comparison(self):
        cases = (
            (b"08", b"8", "pass"),
            (b"1e2", b"100", "pass"),
            (b"1e2", b"101", "fail"),
            (b"2147483647", b"02147483647", "pass"),
            (b"2147483648", b"02147483648", "pass"),
            (b"9007199254740992", b"9007199254740993", "fail"),
            (b"9007199254740992", b"09007199254740992", "fail"),
            (b"0.1", b"0.10000000000000001", "pass"),
            (b"1e999", b"01e999", "fail"),
            (b"999999999999999999999999999999999999",
             b"1000000000000000000000000000000000000", "fail"),
        )
        for actual, expected, status in cases:
            with self.subTest(actual=actual, expected=expected):
                source = b"<?php header('X-Count: " + actual + b"'); echo 'body'; ?>"
                sections = {**base_sections(source), "EXPECTHEADERS": b"X-Count: " + expected}
                result = self.run_sections(sections, cgi_executable=CGI)
                self.assertEqual(result["status"], status, result)

    @unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
    def test_form_feed_in_numeric_header_matches_php_whitespace_comparison(self):
        sources = (
            b"<?php header('X-Count: '.chr(12).'1'); echo 'body'; ?>",
            b"<?php header('X-Count: '.'1'.chr(12)); echo 'body'; ?>",
            b"<?php header('X-Count: '.chr(12).' \t1\v '.chr(12)); echo 'body'; ?>",
        )
        for source in sources:
            with self.subTest(source=source):
                sections = {**base_sections(source), "EXPECTHEADERS": b"X-Count: 1"}
                result = self.run_sections(sections, cgi_executable=CGI)
                self.assertEqual(result["status"], "pass", result)


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTCGIExecutionTests(unittest.TestCase):
    def run_sections(self, sections, **kwargs):
        args = dict(image=IMAGE, executable=CLI, platform=PLATFORM, release=RELEASE,
                    timeout=15, output_limit=8192, cgi_executable=CGI)
        args.update(kwargs)
        return executor.run(sections, **args)

    def test_get_post_cookie_and_query_edge_cases_round_trip_through_php_cgi(self):
        cases = (
            (b"name=a+b&zero=0", b"", b"", b"a b||GET|"),
            (b"", b"name=a+b&zero=0", b"", b"|a b|POST|"),
            (b" name=query ", b" name=body ", b" session=abc \t", b"query|body|POST|session=abc"),
            (b"zero=0", b"", b"zero=0", b"0||GET|zero=0"),
        )
        source = (
            b"<?php echo ($_GET['name'] ?? $_GET['zero'] ?? ''), '|', "
            b"($_POST['name'] ?? $_POST['zero'] ?? ''), '|', $_SERVER['REQUEST_METHOD'], '|', "
            b"($_SERVER['HTTP_COOKIE'] ?? ''); ?>"
        )
        for get, post, cookie, expected in cases:
            sections = {**base_sections(source, expected), "GET": get, "POST": post, "COOKIE": cookie}
            with self.subTest(get=get, post=post, cookie=cookie):
                result = self.run_sections(sections)
                self.assertEqual(result["status"], "pass", result)

    def test_php_nonempty_false_values_use_cli_but_zero_newline_uses_cgi(self):
        source = b"<?php echo PHP_SAPI; ?>"
        for value, sapi in ((b"", b"cli"), (b"0", b"cli"), (b"0\n", b"cgi-fcgi")):
            result = self.run_sections({**base_sections(source, sapi), "GET": value})
            self.assertEqual(result["status"], "pass", result)

    def test_zero_get_cookie_and_post_keep_official_cli_environment_values(self):
        source = (
            b"<?php echo PHP_SAPI, '|'; "
            b"foreach (['QUERY_STRING', 'REQUEST_METHOD', 'HTTP_COOKIE'] as $i => $name) { "
            b"if ($i) echo '|'; $value = getenv($name); "
            b"echo $value === false ? '<unset>' : $value; } ?>"
        )
        cases = (
            ("GET", b"cli|0|GET|<unset>"),
            ("COOKIE", b"cli|<unset>|GET|0"),
            ("POST", b"cli|<unset>|GET|<unset>"),
        )
        for name, expected in cases:
            sections = {**base_sections(source, expected), name: b"0"}
            with self.subTest(section=name):
                result = self.run_sections(sections, cgi_executable=None)
                self.assertEqual(result["status"], "pass", result)

    def test_cgi_skipif_and_cli_clean_share_one_session_with_ini_env_stdin_fixture(self):
        sections = {
            "TEST": b"CGI phase profile and shared isolated inputs",
            "CGI": b"",
            "INI": b"precision=17\n",
            "ENV": b"PHEGO_CGI_ENV=ready",
            "STDIN": b"stdin-bytes",
            "SKIPIF": (b"<?php if (PHP_SAPI !== 'cgi-fcgi' || ini_get('precision') !== '14' "
                        b"|| getenv('PHEGO_CGI_ENV') !== 'ready') exit(1); ?>"),
            "FILE": (b"<?php echo PHP_SAPI, '|', ini_get('precision'), '|', "
                     b"getenv('PHEGO_CGI_ENV'), '|', file_get_contents('php://stdin'), '|', "
                     b"file_get_contents(__DIR__ . '/fixture.txt'); ?>"),
            "CLEAN": b"<?php if (PHP_SAPI !== 'cli') echo 'unexpected-clean-sapi'; ?>",
            "EXPECT": b"cgi-fcgi|17|ready|stdin-bytes|visible",
        }
        result = self.run_sections(sections, fixtures={"fixture.txt": b"visible"})
        self.assertEqual(result["status"], "pass", result)

    def test_post_raw_binary_content_length_and_body_are_byte_accurate(self):
        body = b"a=1\x00b=\xff\r\nZ"
        source = (b"<?php echo $_SERVER['CONTENT_TYPE'], '|', $_SERVER['CONTENT_LENGTH'], '|', "
                  b"bin2hex(file_get_contents('php://input')); ?>")
        # A declared POST content type uses urlencoded mode; non-UTF8 bytes
        # remain intact in php://input even when PHP cannot parse fields.
        sections = {**base_sections(source, b"application/x-www-form-urlencoded|" +
                                   str(len(body)).encode() + b"|" + body.hex().encode()),
                    "POST": body}
        result = self.run_sections(sections)
        self.assertEqual(result["status"], "pass", result)

    def test_nonempty_post_body_takes_precedence_over_phpt_stdin(self):
        body = b"from-post=1"
        sections = {
            **base_sections(
                b"<?php echo file_get_contents('php://input'); ?>",
                body,
            ),
            "POST": body,
            "STDIN": b"from-stdin=2",
        }
        result = self.run_sections(sections)
        self.assertEqual(result["status"], "pass", result)

    def test_headers_case_sensitive_last_duplicate_wins_and_subset_is_allowed(self):
        source = (b"<?php header('X-Check: first', false); header('X-Check: final', false); "
                  b"header('X-Extra: yes'); echo 'body'; ?>")
        sections = {**base_sections(source), "CGI": b"",
                    "EXPECTHEADERS": b"X-Check: final"}
        result = self.run_sections(sections)
        self.assertEqual(result["status"], "pass", result)
        missing = {**sections, "EXPECTHEADERS": b"X-Missing: no"}
        self.assertEqual(self.run_sections(missing)["status"], "fail")
        earlier_duplicate = {**sections, "EXPECTHEADERS": b"X-Check: first"}
        self.assertEqual(self.run_sections(earlier_duplicate)["status"], "fail")
        case_mismatch = {**sections, "EXPECTHEADERS": b"x-check: final"}
        self.assertEqual(self.run_sections(case_mismatch)["status"], "fail")

    def test_header_mismatch_obeys_xfail(self):
        source = b"<?php header('X-Actual: yes'); echo 'body'; ?>"
        mismatch = {**base_sections(source), "EXPECTHEADERS": b"X-Actual: no"}
        self.assertEqual(self.run_sections(mismatch)["status"], "fail")
        mismatch["XFAIL"] = b"known CGI header mismatch"
        self.assertEqual(self.run_sections(mismatch)["status"], "xfail")

    def test_cgi_body_keeps_internal_crlf_while_cli_normalizes_it(self):
        source = b"<?php echo \"line\\r\\nnext\"; ?>"
        # PHP run-tests.php strips CGI headers and trims the body without
        # applying its earlier CLI CRLF normalization. Thus LF expectation
        # mismatches the body's retained internal CRLF bytes.
        mismatch = {**base_sections(source, b"line\nnext"), "CGI": b""}
        self.assertEqual(self.run_sections(mismatch)["status"], "fail")

    def test_cli_and_cgi_both_run_when_cgi_is_requested_and_outputs_are_distinct(self):
        sections = {**base_sections(b"<?php echo PHP_SAPI; ?>", b"cgi-fcgi"), "CGI": b""}
        result = self.run_sections(sections)
        self.assertEqual(result["status"], "pass", result)

    def test_run_case_captures_independent_cgi_profile_in_same_session(self):
        observation = session.run_case(
            image=IMAGE, executable=CLI, cgi_executable=CGI, platform=PLATFORM,
            release=RELEASE, source=b"<?php echo PHP_SAPI; ?>", skipif=b"",
            fixtures={}, timeout=10, output_limit=4096,
        )
        self.assertEqual(observation.profile.status, "completed", observation)
        self.assertEqual(observation.cgi_profile.status, "completed", observation)
        self.assertEqual(observation.cgi_profile.output, b"cgi-fcgi\n8.5.12\n4", observation)
        boundary = re.search(rb"\r?\n\r?\n", observation.program.output)
        self.assertIsNotNone(boundary, observation.program)
        self.assertIn(b"Content-type:", observation.program.output[:boundary.start()])
        self.assertEqual(observation.program.output[boundary.end():], b"cgi-fcgi", observation.program)

    def test_reserved_cgi_environment_names_cannot_override_request_values(self):
        sections = {**base_sections(b"<?php echo $_SERVER['REQUEST_METHOD']; ?>", b"GET"),
                    "GET": b"a=b", "ENV": b"REQUEST_METHOD=DELETE"}
        result = self.run_sections(sections)
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertIn("REQUEST_METHOD", result.get("reason", ""))
        for secret in (b"DELETE", b"body"):
            self.assertNotIn(secret.decode(), repr(result))

    def test_cgi_parameters_do_not_appear_in_host_docker_argv(self):
        sentinel = b"PHEGO_CGI_ARGV_SENTINEL_36A9"
        sections = {**base_sections(b"<?php echo 'ok'; ?>", b"ok"),
                    "GET": b"token=" + sentinel, "COOKIE": b"secret=" + sentinel}
        real_popen = session.subprocess.Popen
        host_argv = []

        def recording_popen(argv, *args, **kwargs):
            host_argv.append(argv)
            return real_popen(argv, *args, **kwargs)

        with mock.patch.object(session.subprocess, "Popen", side_effect=recording_popen):
            result = self.run_sections(sections)
        self.assertEqual(result["status"], "pass", result)
        self.assertTrue(host_argv, "expected Docker subprocesses to be observed")
        self.assertTrue(all(sentinel not in repr(argv).encode("utf-8") for argv in host_argv))

    def test_cgi_timeout_confirms_the_owned_session_was_cleaned_up(self):
        observation = session.run_case(
            image=IMAGE, executable=CLI, cgi_executable=CGI, platform=PLATFORM,
            release=RELEASE, source=b"<?php while (true) {} ?>", skipif=b"",
            fixtures={}, timeout=2, output_limit=4096,
        )
        self.assertEqual(observation.status, "timeout", observation)
        self.assertEqual(observation.program.status, "timeout", observation)
        self.assertTrue(observation.cleanup_confirmed, observation)


if __name__ == "__main__":
    unittest.main()
