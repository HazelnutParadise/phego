"""PHP 8.5.12 ARGS compatibility and shell-boundary regression tests."""
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


parser = load_tool("phego_phpt_args_parser", TOOLS / "phpt_runner.py")
executor = load_tool("phego_phpt_args_executor", TOOLS / "phpt_execute.py")
session = load_tool("phego_phpt_args_session", TOOLS / "phpt_session.py")
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


def base_sections(source, expected, args=b'"two words" plain'):
    return {"TEST": b"PHP 8.5.12 ARGS reference", "ARGS": args,
            "FILE": source, "EXPECT": expected}


def run_phpt(sections, **overrides):
    options = dict(image=IMAGE, executable=CLI, platform=PLATFORM,
                   release=RELEASE, timeout=15, output_limit=8192)
    options.update(overrides)
    return executor.run(sections, **options)


class PHPTArgsSafetyTests(unittest.TestCase):
    def test_command_substitution_glob_and_pipeline_are_gaps_before_execution(self):
        # run-tests.php appends ARGS to a shell command after "--". These
        # constructs are not forwarded until their quoting/expansion behavior
        # can be reproduced safely.
        payloads = (b"$(touch /tmp/phego-args-marker)", b"*", b"first|second",
                    b"# ignored by shell", b"~", b'"a\\\nb"')
        for payload in payloads:
            with self.subTest(payload=payload), \
                 mock.patch.object(executor, "run_case") as run_case, \
                 mock.patch.object(session.subprocess, "Popen") as popen:
                result = run_phpt(base_sections(b"<?php echo 'unsafe'; ?>", b"unsafe", payload))
                self.assertEqual(result["status"], "runner_gap", result)
                self.assertEqual(result["execution"], "not_run", result)
                run_case.assert_not_called()
                popen.assert_not_called()

    def test_nul_and_overlong_args_are_gaps_before_execution(self):
        for payload in (b'a\x00b', b'a' * (65536 + 1)):
            with self.subTest(length=len(payload)), mock.patch.object(executor, 'run_case') as run_case:
                result = run_phpt(base_sections(b'<?php ?>', b'', payload))
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertEqual(result['execution'], 'not_run', result)
                run_case.assert_not_called()

    def test_too_many_args_are_rejected_before_session(self):
        with mock.patch.object(executor, 'run_case') as run_case:
            result = run_phpt(base_sections(b'<?php ?>', b'', b'x ' * 4097))
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertEqual(result['execution'], 'not_run', result)
        self.assertIn('4096', result['reason'])
        run_case.assert_not_called()

    def test_invalid_args_diagnostic_gives_byte_position_without_input(self):
        for payload, offset in ((b'hello $private', 7), (b"x 'private", 3),
                                (b'\xff$private', 2), (b'a\x00private', 2)):
            with self.subTest(offset=offset):
                result = run_phpt(base_sections(b'<?php ?>', b'', payload))
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertIn('ARGS', result['reason'])
                self.assertIn(f'byte {offset}', result['reason'])
                self.assertNotIn('private', result['reason'])

    def test_args_over_phpt_parser_limit_is_rejected(self):
        raw = (b"--TEST--\nlarge ARGS\n--ARGS--\n" + b"a" * (8 * 1024 * 1024)
               + b"\n--FILE--\n<?php ?>\n--EXPECT--\n\n")
        with self.assertRaisesRegex(parser.PHPTError, "8 MiB"):
            parser.parse_phpt(raw, "large-args.phpt")


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTArgsExecutionTests(unittest.TestCase):
    def test_official_quoted_arguments_and_multiple_arguments(self):
        # PHP 8.5.12's official runner invokes PHP with -f file -- <ARGS>.
        source = b"<?php echo json_encode(array_slice($argv, 1)); ?>"
        result = run_phpt(base_sections(source, b'["two words","plain"]'))
        self.assertEqual(result["status"], "pass", result)

    def test_empty_argument_and_shell_whitespace_are_preserved(self):
        source = b"<?php echo json_encode(array_slice($argv, 1)); ?>"
        result = run_phpt(base_sections(source, b'["first","","two words","last"]',
                                        b"first '' \"two words\" last"))
        self.assertEqual(result["status"], "pass", result)

    def test_leading_dash_and_unquoted_carriage_return_are_argument_bytes(self):
        source = b"<?php echo json_encode(array_map('bin2hex', array_slice($argv, 1))); ?>"
        result = run_phpt(base_sections(source, b'["2d6e","780d"]', b'-n x\r'))
        self.assertEqual(result['status'], 'pass', result)

    def test_non_utf8_argument_is_forwarded_as_bytes(self):
        # Encode argv bytes in PHP so the PHPT output remains UTF-8 JSON.
        source = b"<?php echo json_encode(array_map('bin2hex', array_slice($argv, 1))); ?>"
        result = run_phpt(base_sections(source, b'["616c706861","ff","7a"]',
                                        b"alpha \xff z"))
        self.assertEqual(result["status"], "pass", result)

    def test_stdin_env_ini_skipif_and_clean_coexist_with_args(self):
        source = (b"<?php echo json_encode([array_slice($argv, 1), "
                  b"file_get_contents('php://stdin'), getenv('PHEGO_ARGS_ENV'), "
                  b"ini_get('precision')]); ?>")
        sections = base_sections(source,
            b'[["two words","plain"],"stdin-data\\n","present","7"]')
        sections.update({
            "STDIN": b"stdin-data\n",
            "ENV": b"PHEGO_ARGS_ENV=present\n",
            "INI": b"precision=7\n",
            "SKIPIF": b"<?php if (PHP_VERSION !== '8.5.12') die('skip wrong release'); ?>",
            "CLEAN": b"<?php ?>",
        })
        result = run_phpt(sections)
        self.assertEqual(result["status"], "pass", result)

    def test_args_are_not_exposed_in_host_docker_argv(self):
        source = b"<?php echo json_encode(array_slice($argv, 1)); ?>"
        secret_arg = "two words"
        host_argv = []
        real_popen = session.subprocess.Popen

        def recording_popen(argv, *args, **kwargs):
            host_argv.append(tuple(argv))
            return real_popen(argv, *args, **kwargs)

        with mock.patch.object(session.subprocess, "Popen", side_effect=recording_popen):
            result = run_phpt(base_sections(source, b'["two words","plain"]'))
        self.assertEqual(result["status"], "pass", result)
        self.assertTrue(host_argv)
        self.assertTrue(all(secret_arg not in repr(argv) for argv in host_argv), host_argv)

    def test_cgi_get_executes_with_args_and_matches_official_output(self):
        source = b"<?php echo json_encode(['argv'=>$argv ?? null,'getopt'=>getopt('', ['name:'])]); ?>"
        sections = base_sections(source, b'{"argv":null,"getopt":false}', b'--name=hello')
        sections['CGI'] = b''
        result = run_phpt(sections, cgi_executable=CGI)
        self.assertEqual(result['status'], 'pass', result)

    def test_cgi_get_arguments_reach_container_script_but_not_host_argv(self):
        marker = b'PHEGO_CGI_ARG_MARKER_729164'
        sections = base_sections(b"<?php echo 'body'; ?>", b'body', marker)
        sections['CGI'] = b''
        payloads, host_argv = [], []
        original_write, original_popen = session.os.write, session.subprocess.Popen
        def record_write(fd, data):
            payloads.append(bytes(data))
            return original_write(fd, data)
        def record_popen(argv, *args, **kwargs):
            host_argv.append(tuple(argv))
            return original_popen(argv, *args, **kwargs)
        with mock.patch.object(session.os, 'write', side_effect=record_write), \
             mock.patch.object(session.subprocess, 'Popen', side_effect=record_popen):
            result = run_phpt(sections, cgi_executable=CGI)
        self.assertEqual(result['status'], 'pass', result)
        self.assertTrue(any(marker in payload for payload in payloads))
        self.assertTrue(all(marker not in repr(argv).encode() for argv in host_argv))

    def test_nonempty_post_ignores_even_shell_syntax_in_args(self):
        sections = base_sections(b"<?php echo $_SERVER['REQUEST_METHOD']; ?>", b'POST', b'$(false)')
        sections.update({'CGI': b'', 'POST': b'field=value'})
        result = run_phpt(sections, cgi_executable=CGI)
        self.assertEqual(result['status'], 'pass', result)

    def test_post_and_cgi_do_not_run_cli_args_as_cgi_query_options(self):
        # PHP 8.5.12 run-tests.php omits the ARGS suffix for nonempty POST.
        source = b"<?php echo json_encode([$_SERVER['REQUEST_METHOD'], array_slice($argv ?? [], 1)]); ?>"
        sections = base_sections(source, b'["POST",[]]')
        sections.update({"CGI": b"", "POST": b"field=value"})
        result = run_phpt(sections, cgi_executable=CGI)
        self.assertEqual(result["status"], "pass", result)


if __name__ == "__main__":
    unittest.main()
