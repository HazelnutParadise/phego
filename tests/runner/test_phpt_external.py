"""PHP 8.5.12 external PHPT sections using only explicit fixture inputs."""
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
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


parser = load_tool("phego_phpt_external_parser", TOOLS / "phpt_runner.py")
executor = load_tool("phego_phpt_external_executor", TOOLS / "phpt_execute.py")
session = load_tool("phego_phpt_external_session", TOOLS / "phpt_session.py")
IMAGE = os.environ.get(
    "PHEGO_PHPT_SESSION_IMAGE",
    "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049",
)
CLI = "/usr/local/bin/php"
PLATFORM = os.environ.get("PHEGO_PHPT_SESSION_PLATFORM", "linux/386")
RELEASE = "8.5.12"
PHP_IMAGE_AVAILABLE = bool(
    re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", IMAGE)
    and shutil.which("docker")
    and subprocess.run(["docker", "image", "inspect", IMAGE], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, check=False).returncode == 0
)


def invoke_case(case_bytes, fixtures, *, release=RELEASE, patch_run=None):
    with tempfile.TemporaryDirectory(prefix="phego-external-") as directory:
        root = Path(directory)
        case = root / "case.phpt"
        case.write_bytes(case_bytes)
        args = ["--image", IMAGE, "--executable", CLI, "--platform", PLATFORM, "--release", release]
        for name, payload in fixtures.items():
            source = root / ("host-" + str(len(args)) + ".fixture")
            source.write_bytes(payload)
            args.extend(("--fixture", name + "=" + str(source)))
        args.append(str(case))
        output = io.StringIO()
        with mock.patch("sys.stdout", output):
            if patch_run is None:
                code = executor.main(args)
            else:
                with mock.patch.object(executor, "run", side_effect=patch_run):
                    code = executor.main(args)
        return code, json.loads(output.getvalue())


def external_case(file_section=b"nested/body.php\n", expect_section=b"nested/expect.bin\n",
                  *, expectation="EXPECT_EXTERNAL"):
    return (b"--TEST--\nexternal test\n--FILE_EXTERNAL--\n" + file_section
            + b"--" + expectation.encode("ascii") + b"--\n" + expect_section)


class PHPTExternalFixtureTests(unittest.TestCase):
    def test_fixture_reader_returns_immutable_exact_byte_snapshot(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "source=part.bin"
            original = b"\x00\xffline\r\n"
            path.write_bytes(original)
            snapshot = executor._load_fixtures(["nested/body.php=" + str(path)])
            path.write_bytes(b"changed after snapshot")
        self.assertEqual(snapshot, {"nested/body.php": original})

    def test_missing_oversized_and_unsafe_fixture_inputs_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ordinary = root / "fixture"
            ordinary.write_bytes(b"x")
            large = root / "large"
            with large.open("wb") as stream:
                stream.truncate(8 * 1024 * 1024 + 1)
            missing = root / "absent"
            cases = (
                (["nested/body.php=" + str(missing)], (OSError, ValueError)),
                (["../body.php=" + str(ordinary)], ValueError),
                (["/body.php=" + str(ordinary)], ValueError),
                (["nested//body.php=" + str(ordinary)], ValueError),
                (["nested/body.php=" + str(large)], ValueError),
            )
            for specifications, error in cases:
                with self.subTest(specifications=specifications), self.assertRaises(error):
                    executor._load_fixtures(specifications)

    def test_fixture_path_limits_apply_before_open_or_container_creation(self):
        cases = (("a" * 4097, "4096 bytes"), ("a" * 256, "255 bytes"),
                 ("é" * 128, "255 bytes"), ("/".join(["a"] * 65), "64 components"))
        for name, diagnostic in cases:
            with self.subTest(diagnostic=diagnostic, name_length=len(name)):
                with mock.patch.object(executor.os, "open") as open_file:
                    with self.assertRaisesRegex(ValueError, diagnostic):
                        executor._load_fixtures([name + "=/unused"])
                    open_file.assert_not_called()
                with mock.patch.object(session.subprocess, "Popen") as popen:
                    with self.assertRaisesRegex(ValueError, diagnostic):
                        session.run_case(IMAGE, CLI, PLATFORM, RELEASE, b"", b"", {name: b"x"}, 10, 4096)
                    popen.assert_not_called()
                with mock.patch.object(executor, "run_case") as run_case:
                    result = executor.run({'TEST': b'x', 'FILE_EXTERNAL': b'body.php', 'EXPECT': b''},
                                          IMAGE, CLI, PLATFORM, RELEASE, 10, 4096,
                                          fixtures={name: b'x'})
                    self.assertIn(diagnostic, result['reason'])
                    self.assertEqual(result['execution'], 'not_run')
                    run_case.assert_not_called()

    def test_subclasses_cannot_evade_fixture_resource_limits(self):
        class DishonestDict(dict):
            def __len__(self): return 0

        class DishonestName(str):
            def __len__(self): return 1
            def encode(self, *args, **kwargs): return b'safe'

        class DishonestBytes(bytes):
            def __len__(self): return 1

        cases = (DishonestDict({str(i): b'x' for i in range(129)}),
                 {DishonestName('x' * 5000): b'x'},
                 {'body.php': DishonestBytes(b'x' * (8 * 1024 * 1024 + 1))})
        for fixtures in cases:
            with self.subTest(kind=type(fixtures).__name__), mock.patch.object(session.subprocess, 'Popen') as popen:
                with self.assertRaises((TypeError, ValueError)):
                    session.run_case(IMAGE, CLI, PLATFORM, RELEASE, b'', b'', fixtures, 10, 4096)
                popen.assert_not_called()
            with mock.patch.object(executor, 'run_case') as run_case:
                result = executor.run({'TEST': b'x', 'FILE_EXTERNAL': b'body.php', 'EXPECT': b''},
                                      IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures=fixtures)
                self.assertEqual(result['status'], 'runner_gap')
                self.assertEqual(result['execution'], 'not_run')
                run_case.assert_not_called()

    def test_subclassed_phase_and_section_bytes_cannot_bypass_limits(self):
        class DishonestBytes(bytes):
            def __len__(self): return 1

        payload = DishonestBytes(b'x' * (1024 * 1024 + 1))
        with mock.patch.object(session.subprocess, 'Popen') as popen:
            with self.assertRaises((TypeError, ValueError)):
                session.run_case(IMAGE, CLI, PLATFORM, RELEASE, payload, b'', {}, 10, 4096)
            popen.assert_not_called()
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run({'TEST': b'x', 'FILE': payload, 'EXPECT': b''},
                                  IMAGE, CLI, PLATFORM, RELEASE, 10, 4096)
            self.assertEqual(result['status'], 'runner_gap')
            self.assertEqual(result['execution'], 'not_run')
            run_case.assert_not_called()

    def test_subclassed_bounded_containers_are_rejected(self):
        class DishonestList(list):
            def __len__(self): return 0
        class DishonestTuple(tuple):
            def __len__(self): return 0

        with mock.patch.object(executor.os, 'open') as open_file:
            with self.assertRaises(ValueError):
                executor._load_fixtures(DishonestList([str(i) + '=/unused' for i in range(129)]))
            open_file.assert_not_called()
        for option, values in (('args', [b'x'] * 4097),
                               ('ini', [b'a=b'] * 257), ('env', [b'A=b'] * 257)):
            with self.subTest(option=option), mock.patch.object(session.subprocess, 'Popen') as popen:
                with self.assertRaises((TypeError, ValueError)):
                    session.run_case(IMAGE, CLI, PLATFORM, RELEASE, b'', b'', {}, 10, 4096,
                                     **{option: DishonestTuple(values)})
                popen.assert_not_called()

    def test_external_reference_diagnostics_distinguish_missing_unsafe_and_encoding(self):
        cases = ((b'secret-missing.php', 'fixture was not supplied'),
                 (b'../secret.php', 'reference is unsafe'),
                 (b'private-\xff.php', 'reference encoding is unsupported'))
        for reference, diagnostic in cases:
            with self.subTest(diagnostic=diagnostic), mock.patch.object(executor, 'run_case') as run_case:
                result = executor.run({'TEST': b'x', 'FILE_EXTERNAL': reference, 'EXPECT': b''},
                                      IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures={})
                self.assertIn('FILE_EXTERNAL', result['reason'])
                self.assertIn(diagnostic, result['reason'])
                self.assertNotIn('secret', result['reason'])
                self.assertEqual(result['execution'], 'not_run')
                run_case.assert_not_called()

    def test_external_section_paths_must_resolve_only_to_supplied_safe_fixtures(self):
        data = external_case(file_section=b"../outside.php\n")
        with mock.patch.object(executor, "run_case") as run_case, \
             mock.patch.object(session.subprocess, "Popen") as popen:
            result = executor.run(parser.parse_phpt(data, "external.phpt"), IMAGE, CLI,
                                  PLATFORM, RELEASE, 10, 4096, fixtures={})
        self.assertEqual(result["status"], "runner_gap", result)
        self.assertEqual(result["execution"], "not_run", result)
        self.assertFalse(result["reason"].startswith("unsupported PHPT section:"), result)
        self.assertRegex(result["reason"].lower(), r"external|fixture|path")
        run_case.assert_not_called()
        popen.assert_not_called()

    def test_missing_external_fixture_names_the_section_without_disclosing_path(self):
        sections = parser.parse_phpt(external_case(), 'external.phpt')
        fixtures = {'nested/body.php': b"<?php echo 'body'; ?>"}
        before = dict(sections)
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures=fixtures)
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertEqual(result['execution'], 'not_run', result)
        self.assertIn('EXPECT_EXTERNAL', result['reason'])
        self.assertNotIn('nested/expect.bin', result['reason'])
        self.assertEqual(sections, before)
        run_case.assert_not_called()

    def test_direct_conflicting_source_and_expectations_never_execute(self):
        conflicts = (
            {'TEST': b'x', 'FILE': b'<?php ?>', 'FILE_EXTERNAL': b'secret.php', 'EXPECT': b'x'},
            {'TEST': b'x', 'FILE': b'<?php ?>', 'EXPECT': b'x', 'EXPECT_EXTERNAL': b'secret.txt'},
        )
        for sections in conflicts:
            with self.subTest(keys=tuple(sections)), mock.patch.object(executor, 'run_case') as run_case:
                result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096,
                                      fixtures={'secret.php': b'<?php ?>', 'secret.txt': b'x'})
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertEqual(result['execution'], 'not_run', result)
                run_case.assert_not_called()

    def test_double_dot_reference_resolves_as_php_without_using_wrong_fixture(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'body..php', 'EXPECT': b'right'}
        fixtures = {'body..php': b"<?php echo 'wrong'; ?>", 'bodyphp': b"<?php echo 'right'; ?>"}
        with mock.patch.object(executor, 'run_case', side_effect=RuntimeError('capture')) as run_case:
            executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures=fixtures)
        self.assertEqual(run_case.call_args.kwargs['source'], fixtures['bodyphp'])

    def test_double_dot_is_removed_before_php_trim(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'.. body.php ..', 'EXPECT': b'right'}
        fixtures = {' body.php ': b"<?php echo 'wrong'; ?>", 'body.php': b"<?php echo 'right'; ?>"}
        with mock.patch.object(executor, 'run_case', side_effect=RuntimeError('capture')) as run_case:
            executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures=fixtures)
        self.assertEqual(run_case.call_args.kwargs['source'], fixtures['body.php'])

    def test_oversized_external_reference_is_rejected_without_execution(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'a' * (64 * 1024 + 1), 'EXPECT': b''}
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures={})
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertEqual(result['execution'], 'not_run', result)
        self.assertIn('FILE_EXTERNAL', result['reason'])
        run_case.assert_not_called()

    def test_done_marker_truncates_reference_section_not_external_file(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'body.php\n===DONE===\ntail\n', 'EXPECT': b'right'}
        fixtures = {'body.php\n===DONE===': b"<?php echo 'right'; ?>",
                    'body.php\n===DONE===\ntail': b"<?php echo 'wrong'; ?>"}
        with mock.patch.object(executor, 'run_case', side_effect=RuntimeError('capture')) as run_case:
            executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096, fixtures=fixtures)
        self.assertEqual(run_case.call_args.kwargs['source'], fixtures['body.php\n===DONE==='])

    def test_form_feed_is_not_trimmed_from_external_name(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'\x0cbody.php\x0c', 'EXPECT': b'wrong'}
        with mock.patch.object(executor, 'run_case') as run_case:
            result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096,
                                  fixtures={'body.php': b"<?php echo 'wrong'; ?>"})
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertEqual(result['execution'], 'not_run', result)
        run_case.assert_not_called()

    def test_mutable_or_nonbyte_fixture_values_are_not_coerced(self):
        for value in (bytearray(b"<?php ?>"), 10):
            with self.subTest(value=type(value).__name__), mock.patch.object(executor, 'run_case') as run_case:
                sections = {'TEST': b'x', 'FILE_EXTERNAL': b'body.php', 'EXPECT': b''}
                result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 10, 4096,
                                      fixtures={'body.php': value})
                self.assertEqual(result['status'], 'runner_gap', result)
                self.assertEqual(result['execution'], 'not_run', result)
                self.assertIn('fixture', result['reason'].lower())
                run_case.assert_not_called()

    def test_redirect_without_file_preserves_unsupported_section_diagnostic(self):
        result = executor.run({'TEST': b'x', 'REDIRECTTEST': b'ignored'}, IMAGE, CLI,
                              PLATFORM, RELEASE, 10, 4096)
        self.assertEqual(result['status'], 'runner_gap', result)
        self.assertIn('REDIRECTTEST', result['reason'])

    def test_conflicting_source_or_expectation_forms_are_rejected(self):
        cases = (
            b"--TEST--\nx\n--FILE--\n<?php ?>\n--FILE_EXTERNAL--\na.php\n--EXPECT--\nx\n",
            b"--TEST--\nx\n--FILE_EXTERNAL--\na.php\n--EXPECT--\nx\n--EXPECT_EXTERNAL--\nx.txt\n",
        )
        for data in cases:
            with self.subTest(data=data), self.assertRaises(parser.PHPTError):
                parser.parse_phpt(data, "conflict.phpt")

    def test_unverified_release_and_expectregex_external_stay_gaps(self):
        external = parser.parse_phpt(external_case(), "external.phpt")
        fixtures = {"nested/body.php": b"<?php echo 'body'; ?>",
                    "nested/expect.bin": b"body"}
        with mock.patch.object(executor, "run_case") as run_case, \
             mock.patch.object(session.subprocess, "Popen") as popen:
            unsupported_release = executor.run(external, IMAGE, CLI, PLATFORM,
                                               "8.5.11", 10, 4096, fixtures=fixtures)
            regex_external = parser.parse_phpt(
                external_case(expectation="EXPECTREGEX_EXTERNAL"), "regex-external.phpt")
            unsupported_regex = executor.run(regex_external, IMAGE, CLI, PLATFORM,
                                             RELEASE, 10, 4096,
                                             fixtures={**fixtures, "nested/expect.bin": b"body"})
        for result in (unsupported_release, unsupported_regex):
            self.assertEqual(result["status"], "runner_gap", result)
            self.assertEqual(result["execution"], "not_run", result)
        self.assertIn("release", unsupported_release["reason"].lower())
        self.assertIn("EXPECTREGEX_EXTERNAL", unsupported_regex["reason"])
        run_case.assert_not_called()
        popen.assert_not_called()

    def test_external_fixture_errors_do_not_echo_host_paths_or_contents(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            case = root / "case.phpt"
            case.write_bytes(external_case())
            secret_path = root / "private-fixture"
            secret = b"HOST_EXTERNAL_SECRET_4f2a"
            secret_path.write_bytes(secret)
            output = io.StringIO()
            args = ["--image", IMAGE, "--platform", PLATFORM, "--release", RELEASE,
                    "--fixture", "../escape=" + str(secret_path), str(case)]
            with mock.patch("sys.stdout", output):
                code = executor.main(args)
            result = json.loads(output.getvalue())
        self.assertEqual(code, 2)
        self.assertEqual(result["status"], "runner_gap")
        self.assertNotIn(str(root), repr(result))
        self.assertNotIn(secret.decode("ascii"), repr(result))

    def test_inline_done_and_fileeof_normalization_remain_unchanged(self):
        inline = {"TEST": b"inline", "FILE": b"<?php ?>\n===DONE===\nignored\n",
                  "EXPECT": b""}
        normalized, error = executor._normalize_file_section(inline, RELEASE)
        self.assertEqual(error, "")
        self.assertEqual(normalized["FILE"], b"<?php ?>\n===DONE===\n")
        file_eof = {"TEST": b"file eof", "FILEEOF": b"<?php ?>\r\n",
                    "EXPECT": b""}
        normalized, error = executor._normalize_file_section(file_eof, RELEASE)
        self.assertEqual(error, "")
        self.assertEqual(normalized["FILE"], b"<?php ?>")


@unittest.skipUnless(PHP_IMAGE_AVAILABLE, "pinned PHP 8.5.12 linux/386 image and Docker are required")
class PHPTExternalExecutionTests(unittest.TestCase):
    def test_official_file_external_and_expect_external_preserve_marker_binary_and_crlf(self):
        # PHP 8.5.12 resolves external references after reading PHPT sections.
        source = (b'<?php // \xff\r\necho chr(255), "\\r\\n"; ?>\r\n'
                  b'===DONE===\r\n<?php echo "end"; ?>')
        expected = b"\xff\r\n===DONE===\r\nend"
        case = external_case(file_section=b"nested/body.php\n",
                             expect_section=b"nested/expect.bin\n")
        code, result = invoke_case(case, {"nested/body.php": source,
                                         "nested/expect.bin": expected})
        self.assertEqual(code, 0, result)
        self.assertEqual(result["status"], "pass", result)

    def test_external_name_trims_nul_like_php(self):
        sections = {'TEST': b'x', 'FILE_EXTERNAL': b'\x00body.php\x00', 'EXPECT': b'body'}
        result = executor.run(sections, IMAGE, CLI, PLATFORM, RELEASE, 15, 8192,
                              fixtures={'body.php': b"<?php echo 'body'; ?>"})
        self.assertEqual(result['status'], 'pass', result)

    def test_expectf_external_uses_external_formatted_bytes(self):
        case = external_case(file_section=b"nested/body.php\n",
                             expect_section=b"nested/format.txt\n",
                             expectation="EXPECTF_EXTERNAL")
        code, result = invoke_case(case, {"nested/body.php": b"<?php echo 'count=42'; ?>",
                                         "nested/format.txt": b"count=%d\r\n"})
        self.assertEqual(code, 0, result)
        self.assertEqual(result["status"], "pass", result)

    def test_external_fixture_source_and_contents_never_enter_host_docker_argv(self):
        source = b"<?php echo 'external-payload'; ?>"
        expected = b"external-payload"
        case = external_case()
        seen_argv = []
        real_popen = session.subprocess.Popen

        def recording_popen(argv, *args, **kwargs):
            seen_argv.append(tuple(argv))
            return real_popen(argv, *args, **kwargs)

        with mock.patch.object(session.subprocess, "Popen", side_effect=recording_popen):
            code, result = invoke_case(case, {"nested/body.php": source,
                                              "nested/expect.bin": expected})
        self.assertEqual(code, 0, result)
        self.assertEqual(result["status"], "pass", result)
        self.assertTrue(seen_argv)
        argv_text = "\n".join(str(argument) for argv in seen_argv for argument in argv)
        self.assertNotIn("external-payload", argv_text)
        self.assertNotIn("nested/body.php=", argv_text)
        self.assertNotIn("host-", argv_text)


if __name__ == "__main__":
    unittest.main()
