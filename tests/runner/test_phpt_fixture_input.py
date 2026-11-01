"""CLI fixture inputs are explicit, bounded, regular files with safe names."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
TOOLS = ROOT / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))
spec = importlib.util.spec_from_file_location("phego_phpt_execute_fixture_input", TOOLS / "phpt_execute.py")
executor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(executor)

IMAGE = "fixture@sha256:" + "a" * 64
BASE = b"--TEST--\nfixture input\n--FILE--\n<?php echo 'ok'; ?>\n--EXPECT--\nok\n"


def successful_run(sections, image, executable, platform, release, timeout, output_limit, *, fixtures=None):
    return {"status": "pass", "reason": "", "execution": "reference_executed"}


class PHPTFixtureInputTests(unittest.TestCase):
    def invoke(self, path, *fixtures):
        args = ["--image", IMAGE, "--platform", "linux/amd64", "--release", "8.5.12"]
        for fixture in fixtures:
            args.extend(("--fixture", fixture))
        args.append(str(path))
        output = __import__("io").StringIO()
        with patch("sys.stdout", output), patch.object(executor, "run", side_effect=successful_run) as run:
            code = executor.main(args)
        return code, json.loads(output.getvalue()), run

    def test_fixture_bytes_and_destination_forward_for_normal_and_skipif_cases(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            payload = b"\x00\xffbinary\r\n"
            source = root / "source=part.bin"
            source.write_bytes(payload)
            for skipif in (False, True):
                case = root / ("skip.phpt" if skipif else "plain.phpt")
                data = BASE.replace(b"--FILE--", b"--SKIPIF--\n<?php echo ''; ?>\n--FILE--") if skipif else BASE
                case.write_bytes(data)
                with self.subTest(skipif=skipif):
                    code, result, run = self.invoke(case, "data/input.bin=" + str(source))
                    self.assertEqual(code, 0)
                    self.assertEqual(result["status"], "pass")
                    run.assert_called_once()
                    self.assertEqual(run.call_args.kwargs["fixtures"], {"data/input.bin": payload})
                    self.assertEqual(run.call_args.args[0].get("SKIPIF") is not None, skipif)

    def test_multiple_fixtures_and_equals_in_source_paths_are_forwarded(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            first = root / "source=one.bin"
            second = root / "source=two=part.bin"
            first.write_bytes(b"\x00first")
            second.write_bytes(b"second\xff\n")
            case = root / "case.phpt"
            case.write_bytes(BASE)
            code, result, run = self.invoke(
                case, "fixtures/one=" + str(first), "fixtures/two=" + str(second)
            )
            self.assertEqual(code, 0)
            self.assertEqual(result["status"], "pass")
            run.assert_called_once()
            self.assertEqual(run.call_args.kwargs["fixtures"], {
                "fixtures/one": b"\x00first",
                "fixtures/two": b"second\xff\n",
            })

    def test_no_fixture_flag_preserves_none_and_cli_help_documents_repetition(self):
        with tempfile.TemporaryDirectory() as td:
            case = Path(td) / "case.phpt"
            case.write_bytes(BASE)
            code, _, run = self.invoke(case)
            self.assertEqual(code, 0)
            self.assertIsNone(run.call_args.kwargs["fixtures"])
        with patch("sys.stdout", __import__("io").StringIO()) as stdout, self.assertRaises(SystemExit) as raised:
            executor.main(["--help"])
        self.assertEqual(raised.exception.code, 0)
        self.assertIn("--fixture NAME=PATH", stdout.getvalue())
        self.assertIn("repeat", stdout.getvalue())

    def test_fixture_reader_rejects_unsafe_names_duplicates_and_phase_conflicts(self):
        invalid = (
            ["=PATH"], ["missing-separator"], ["/absolute=PATH"], ["../escape=PATH"],
            ["a/../escape=PATH"], ["a//b=PATH"], [r"a\b=PATH"],
            ["same=PATH", "same=PATH"],
            ["node=PATH", "node/child=PATH"], ["node/child=PATH", "node=PATH"],
            [".phego-profile.php=PATH"], [".phego-profile.php/child=PATH"],
            [".phego-skipif.php=PATH"], [".phego-file.php=PATH"],
        )
        with tempfile.TemporaryDirectory() as td:
            source = Path(td) / "source"
            source.write_bytes(b"fixture-secret-bytes")
            self.assertEqual(executor._load_fixtures(["valid/name=" + str(source)]),
                             {"valid/name": b"fixture-secret-bytes"})
            for case in invalid:
                specs = [item.replace("PATH", str(source)) for item in case]
                with self.subTest(specs=case), self.assertRaises(ValueError):
                    executor._load_fixtures(specs)
            # Destination validation must happen before attempting to open PATH.
            with patch.object(executor.os, "open", side_effect=AssertionError("source opened")) as open_file:
                with self.assertRaises(ValueError):
                    executor._load_fixtures(["bad\x00destination=" + str(source)])
                open_file.assert_not_called()

    def test_rejects_missing_symlink_fifo_count_and_aggregate_size(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            ordinary = root / "file"
            ordinary.write_bytes(b"x")
            self.assertEqual(executor._load_fixtures(["valid=" + str(ordinary)]), {"valid": b"x"})
            missing = root / "missing"
            link = root / "link"
            link.symlink_to(ordinary)
            fifo = root / "pipe"
            os.mkfifo(fifo)
            for name, path in (("missing", missing), ("link", link), ("pipe", fifo)):
                with self.subTest(path=name), self.assertRaises((OSError, ValueError)):
                    executor._load_fixtures([f"{name}={path}"])
            # A FIFO must be rejected without waiting for a writer.
            for count in (129,):
                specs = [f"f{i}={ordinary}" for i in range(count)]
                with self.assertRaises(ValueError):
                    executor._load_fixtures(specs)
            large = root / "large"
            with large.open("wb") as stream:
                stream.truncate(8 * 1024 * 1024)
            with self.assertRaises(ValueError):
                executor._load_fixtures([f"one={large}", f"two={large}"])

    def test_cli_rejection_is_sanitized_and_identifies_fixture(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            case = root / "case.phpt"
            case.write_bytes(BASE)
            secret_path = root / "host-secret-path"
            secret_path.write_bytes(b"host-secret-fixture-bytes")
            valid_code, valid_result, valid_run = self.invoke(case, "ok=" + str(secret_path))
            self.assertEqual(valid_code, 0)
            self.assertEqual(valid_result["status"], "pass")
            self.assertEqual(valid_run.call_args.kwargs["fixtures"], {"ok": b"host-secret-fixture-bytes"})
            code, result, run = self.invoke(case, "../bad=" + str(secret_path))
            self.assertEqual(code, 2)
            self.assertEqual(result["status"], "runner_gap")
            self.assertIn("fixture destination must be a safe relative path", result["reason"])
            self.assertNotIn(str(root), repr(result))
            self.assertNotIn("host-secret-fixture-bytes", repr(result))
            run.assert_not_called()

    def test_missing_safe_open_flags_are_reported_as_fixture_errors_without_run(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            case = root / "case.phpt"
            case.write_bytes(BASE)
            source = root / "fixture"
            source.write_bytes(b"fixture")
            for flag in ("O_NOFOLLOW", "O_NONBLOCK"):
                with self.subTest(flag=flag):
                    output = __import__("io").StringIO()
                    args = ["--image", IMAGE, "--platform", "linux/amd64", "--release", "8.5.12",
                            "--fixture", "safe=" + str(source), str(case)]
                    with patch("sys.stdout", output), patch.object(executor, "_read_phpt", return_value=BASE), \
                         patch.object(executor, "run", side_effect=successful_run) as run:
                        with patch.object(os, flag, create=True):
                            delattr(os, flag)
                            code = executor.main(args)
                    self.assertEqual(code, 2)
                    self.assertNotIn("Traceback", output.getvalue())
                    result = json.loads(output.getvalue())
                    self.assertEqual(result["status"], "runner_gap")
                    self.assertIn("fixture", result["reason"])
                    run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
