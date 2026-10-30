"""PHP 8.5.12 PHPT INI parsing and restricted-runner integration checks."""
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

from phpt_ini import parse_ini

spec = importlib.util.spec_from_file_location("phego_phpt_execute_ini", TOOLS / "phpt_execute.py")
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)

RELEASE = "8.5.12"
IMAGE = "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049"


class ParseINICompatibilityTests(unittest.TestCase):
    def test_php_line_splitting_trimming_and_duplicate_order(self):
        data = (b"  alpha \t = \t first \v\r\n"
                b"ignore this\r"
                b"beta=two\n"
                b"alpha=last\n"
                b"extension=one.so\n"
                b"extension=two.so\n"
                b"zend_extension=zend.so\n")
        self.assertEqual(parse_ini(data, RELEASE), (
            b"alpha=last", b"beta=two", b"extension=one.so",
            b"extension=two.so", b"zend_extension=zend.so",
        ))

    def test_interleaved_duplicates_keep_php_array_insertion_order(self):
        data = (b"extension=a\nprecision=7\nextension=b\n"
                b"zend_extension=x\nprecision=9\nzend_extension=y\n")
        self.assertEqual(parse_ini(data, RELEASE), (
            b"extension=a", b"extension=b", b"precision=9",
            b"zend_extension=x", b"zend_extension=y",
        ))

    def test_quoted_ini_bytes_are_preserved_without_shell_expansion(self):
        values = (
            b"space=\"two words\"", b"single='literal value'",
            b'double="a\\\"b"', b"equals=a=b=c",
        )
        parsed = parse_ini(b"\n".join(values), RELEASE)
        self.assertEqual(parsed, values)

    def test_shell_expansion_syntax_is_rejected_without_expansion(self):
        self.assertEqual(parse_ini(b"safe='two words'", RELEASE), (b"safe='two words'",))
        for data in (b"value=$HOME", b"value=$(touch marker)", b"value=`touch marker`" ):
            with self.subTest(data=data):
                with self.assertRaises(ValueError):
                    parse_ini(data, RELEASE)

    def test_pwd_is_replaced_and_forbidden_runtime_placeholders_are_rejected(self):
        self.assertEqual(parse_ini(b"include_path={PWD}/lib", RELEASE),
                         (b"include_path=/work/lib",))
        for placeholder in (b"{TMP}", b"{ENV:HOME}", b"{MAIL:sendmail}"):
            with self.subTest(placeholder=placeholder):
                with self.assertRaises(ValueError):
                    parse_ini(b"sendmail_path=" + placeholder, RELEASE)

    def test_rejects_nul_unknown_release_and_size_or_line_overflow(self):
        self.assertEqual(parse_ini(b"ok=1", RELEASE), (b"ok=1",))
        bad_inputs = (
            (b"ok=1\x00bad=2", RELEASE),
            (b"ok=1", "8.5.11"),
            (b"x=" + b"a" * (64 * 1024), RELEASE),
            (b"x=1\n" * 257, RELEASE),
        )
        for data, release in bad_inputs:
            with self.subTest(length=len(data), release=release):
                with self.assertRaises(ValueError):
                    parse_ini(data, release)

    def test_error_messages_distinguish_unsupported_version_nul_size_and_shell(self):
        self.assertEqual(parse_ini(b"safe=valid", RELEASE), (b"safe=valid",))
        bad = (
            (b"version-marker=1", "99.99.99"),
            (b"nul-marker=1\x00", RELEASE),
            (b"size-marker=" + b"s" * (64 * 1024), RELEASE),
            (b"shell-marker=$HOME", RELEASE),
        )
        messages = []
        for data, release in bad:
            with self.subTest(data=data[:24], release=release):
                with self.assertRaises(ValueError) as raised:
                    parse_ini(data, release)
                message = str(raised.exception)
                self.assertTrue(message)
                self.assertNotIn(data.split(b"=", 1)[0].decode("ascii", "ignore"), message)
                messages.append(message)
        self.assertEqual(len(set(messages)), len(messages))

    def test_malformed_and_unsafe_setting_names_are_rejected(self):
        self.assertEqual(parse_ini(b"safe_name=ok", RELEASE), (b"safe_name=ok",))
        for data in (b"=value", b"bad key=value", b"a;b=value", b"a\n=value"):
            with self.subTest(data=data):
                with self.assertRaises(ValueError):
                    parse_ini(data, RELEASE)


class INIRunnerIntegrationTests(unittest.TestCase):
    def observation(self, output=b"ok\n"):
        return executor.run_case.__globals__["CaseObservation"](
            "completed",
            executor.run_case.__globals__["PhaseObservation"]("completed", b"8.5.12\n4", 0),
            executor.run_case.__globals__["PhaseObservation"]("completed", b"", 0),
            executor.run_case.__globals__["PhaseObservation"]("completed", output, 0),
            False, True,
        )

    def test_ini_case_routes_to_session_with_parsed_immutable_settings_and_raw_skipif(self):
        ini = b"precision=14\nprecision=17\n"
        skipif = b"<?php echo ini_get('precision') === '14' ? 'SKIP bad' : ''; ?>\xff"
        sections = {"TEST": b"ini", "FILE": b"<?php echo ini_get('precision'); ?>",
                    "EXPECT": b"ok\n", "INI": ini, "SKIPIF": skipif}
        with patch.object(executor, "run_case", return_value=self.observation()) as session, \
             patch.object(executor, "execute_php") as one_phase:
            result = executor.run(sections, IMAGE, "/usr/bin/php", "linux/386", RELEASE, 5, 2048)
        one_phase.assert_not_called()
        self.assertEqual(result["status"], "pass")
        kwargs = session.call_args.kwargs
        self.assertEqual(kwargs["ini"], (b"precision=17",))
        self.assertIs(type(kwargs["ini"]), tuple)
        self.assertIs(kwargs["skipif"], skipif)

    def test_invalid_ini_is_rejected_before_session_without_echoing_source_or_output(self):
        valid_sections = {"TEST": b"valid", "FILE": b"<?php echo 'ok'; ?>",
                          "EXPECT": b"ok\n", "INI": b"precision=17\n"}
        with patch.object(executor, "run_case", return_value=self.observation()) as valid_session, \
             patch.object(executor, "execute_php") as valid_execute:
            valid_result = executor.run(valid_sections, IMAGE, "/usr/bin/php", "linux/386",
                                        RELEASE, 5, 2048)
        self.assertEqual(valid_result["status"], "pass")
        valid_session.assert_called_once()
        self.assertEqual(valid_session.call_args.kwargs["ini"], (b"precision=17",))
        valid_execute.assert_not_called()

        secret = b"smtp_password=private-marker\x00bad=1"
        sections = {"TEST": b"bad", "FILE": b"<?php echo 'output-marker'; ?>",
                    "EXPECT": b"", "INI": secret}
        with patch.object(executor, "run_case") as session, \
             patch.object(executor, "execute_php") as one_phase:
            result = executor.run(sections, IMAGE, "/usr/bin/php", "linux/386", RELEASE, 5, 2048)
        session.assert_not_called()
        one_phase.assert_not_called()
        self.assertEqual(result["status"], "runner_gap")
        self.assertNotIn("private-marker", repr(result))
        self.assertNotIn("output-marker", repr(result))

    def test_run_reports_sanitized_reasons_for_shell_size_and_release_errors(self):
        valid = {"TEST": b"invalid-ini", "FILE": b"<?php echo 'must not run'; ?>",
                 "EXPECT": b"", "INI": b"precision=17"}
        cases = (
            (b"error_log=sentinel-shell-$HOME", RELEASE, "shell", "sentinel-shell"),
            (b"oversize=" + b"x" * (64 * 1024), RELEASE, "limit", "oversize"),
            (b"release-sentinel=1", "8.5.11", "release", "release-sentinel"),
        )
        for ini, release, reason_token, secret in cases:
            with self.subTest(reason=reason_token):
                sections = dict(valid, INI=ini)
                with patch.object(executor, "run_case") as session, \
                     patch.object(executor, "execute_php") as execute:
                    result = executor.run(sections, IMAGE, "/usr/local/bin/php", "linux/386",
                                          release, 5, 2048)
                self.assertEqual(result["status"], "runner_gap")
                self.assertIn(reason_token, result["reason"].lower())
                self.assertNotIn(secret, repr(result))
                session.assert_not_called()
                execute.assert_not_called()

    def test_direct_session_rejects_untrusted_or_noncanonical_ini_before_docker(self):
        invalid = (
            ("release", "8.5.11", (b"precision=17",)),
            ("shell expansion", RELEASE, (b"error_log=$HOME",)),
            ("NUL byte", RELEASE, (b"error_log=bad\x00value",)),
            ("unsupported substitution", RELEASE, (b"include_path={TMP}",)),
            ("noncanonical duplicate", RELEASE, (b"precision=7", b"precision=9")),
        )
        session_namespace = executor.run_case.__globals__
        session_subprocess = session_namespace["subprocess"]
        for label, release, settings in invalid:
            with self.subTest(input=label), patch.object(
                session_subprocess, "Popen", side_effect=AssertionError("Docker must not be invoked")
            ) as popen:
                with self.assertRaises(ValueError):
                    executor.run_case(
                        image=IMAGE, executable="/usr/local/bin/php", platform="linux/386",
                        release=release, source=b"<?php echo 'must not run'; ?>", skipif=b"",
                        fixtures={}, timeout=5, output_limit=2048, ini=settings,
                    )
                popen.assert_not_called()


def _docker_available():
    if shutil.which("docker") is None:
        return False
    try:
        available = subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL,
                                   stderr=subprocess.DEVNULL, timeout=3).returncode == 0
        if not available:
            return False
        image = subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, timeout=3).returncode == 0
        return image
    except (OSError, subprocess.TimeoutExpired):
        return False


@unittest.skipUnless(_docker_available(), "Docker daemon unavailable")
class DockerINIIntegrationTests(unittest.TestCase):
    def run_php_case(self, file_source, expected, ini, skipif=b""):
        observation = executor.run_case(
            image=IMAGE, executable="/usr/local/bin/php", platform="linux/386", release=RELEASE,
            source=file_source, skipif=skipif, fixtures={}, timeout=15,
            output_limit=4096, ini=parse_ini(ini, RELEASE),
        )
        self.assertEqual(observation.status, "completed", observation.reason)
        self.assertTrue(observation.cleanup_confirmed)
        self.assertEqual(observation.program.output, expected)
        return observation

    def test_file_ini_does_not_change_skipif_defaults(self):
        observation = self.run_php_case(
            b"<?php echo ini_get('precision'); ?>", b"17",
            b"precision=17\n",
            b"<?php echo ini_get('precision') === '14' ? '' : 'SKIP skipif saw changed INI'; ?>",
        )
        self.assertEqual(observation.skipif.output, b"")

    def test_duplicate_settings_use_the_last_value(self):
        self.run_php_case(b"<?php echo ini_get('precision'); ?>", b"17",
                          b"precision=14\nprecision=17\n")

    def test_single_quote_in_double_quoted_ini_value_matches_run_tests_escaping(self):
        self.run_php_case(
            b"<?php echo bin2hex(ini_get('error_log')); ?>",
            b"615c2762",
            b"error_log=\"a'b\"\n",
        )

    def test_ini_value_reaches_php_without_appearing_in_host_docker_argv(self):
        sentinel = b"PHEGO_INI_ARGV_SENTINEL_4E70D2"
        setting = b"error_log=/tmp/" + sentinel
        session_subprocess = executor.run_case.__globals__["subprocess"]
        real_popen = session_subprocess.Popen
        host_argv = []

        def recording_popen(argv, *args, **kwargs):
            host_argv.append(argv)
            return real_popen(argv, *args, **kwargs)

        with patch.object(session_subprocess, "Popen", side_effect=recording_popen):
            observation = executor.run_case(
                image=IMAGE, executable="/usr/local/bin/php", platform="linux/386", release=RELEASE,
                source=b"<?php echo ini_get('error_log'); ?>", skipif=b"", fixtures={}, timeout=15,
                output_limit=4096, ini=(setting,),
            )
        self.assertEqual(observation.status, "completed", observation.reason)
        self.assertTrue(observation.cleanup_confirmed)
        self.assertEqual(observation.program.output, b"/tmp/" + sentinel)
        self.assertTrue(host_argv, "expected Docker subprocesses to be observed")
        self.assertTrue(all(sentinel not in repr(argv).encode("utf-8") for argv in host_argv))

    def test_malicious_shell_text_is_rejected_without_creating_a_marker(self):
        self.assertEqual(parse_ini(b"precision=17", RELEASE), (b"precision=17",))
        with tempfile.TemporaryDirectory(prefix="phego-phpt-ini-") as directory:
            marker = Path(directory) / "shell-marker"
            sections = {"TEST": b"malicious", "FILE": b"<?php echo 'must not run'; ?>",
                        "EXPECT": b"", "INI": b"error_log=`touch " + os.fsencode(marker) + b"`\n"}
            with patch.object(executor, "run_case") as session, \
                 patch.object(executor, "execute_php") as execute:
                result = executor.run(sections, IMAGE, "/usr/local/bin/php", "linux/386", RELEASE, 5, 4096)
            self.assertEqual(result["status"], "runner_gap")
            session.assert_not_called()
            execute.assert_not_called()
            self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
