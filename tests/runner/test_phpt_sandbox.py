"""The executor bounds untrusted work and records cleanup failure."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("phego_phpt_sandbox", ROOT / "tools/phpt_sandbox.py")
sandbox = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = sandbox
spec.loader.exec_module(sandbox)
IMAGE = "fixture@sha256:" + "a" * 64
PLATFORM = "linux/amd64"
PLATFORMS = ("linux/amd64", "linux/arm64", "linux/386")

class SandboxTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name); self.log = self.root / "log.jsonl"
        fake = self.root / "docker"
        fake.write_text("#!" + sys.executable + "\n" + """import json, os, sys, time
with open(os.environ['PHPT_FAKE_LOG'], 'a') as f: f.write(json.dumps(sys.argv[1:]) + '\\n')
action = sys.argv[1]
if action == 'create':
    mount = sys.argv[sys.argv.index('--mount') + 1]
    src = next(part[4:] for part in mount.split(',') if part.startswith('src='))
    if os.environ.get('PHPT_FAKE_CAPTURE'):
        with open(os.environ['PHPT_FAKE_CAPTURE'], 'wb') as f: f.write(open(src, 'rb').read())
    print('a' * 64)
elif action == 'start':
    if os.environ.get('PHPT_FAKE_MODE') == 'timeout': time.sleep(5)
    elif os.environ.get('PHPT_FAKE_MODE') == 'overflow': sys.stdout.write('x' * 200000)
    elif os.environ.get('PHPT_FAKE_MODE') == 'nonzero':
        print('expected fatal'); sys.exit(2)
    elif os.environ.get('PHPT_FAKE_MODE') == 'not_started': sys.exit(1)
    elif os.environ.get('PHPT_FAKE_MODE') == 'attach_failure': sys.exit(1)
    else: print('actual output')
elif action == 'inspect':
    status = 'created' if os.environ.get('PHPT_FAKE_MODE') == 'not_started' else 'exited'
    code = 2 if os.environ.get('PHPT_FAKE_MODE') == 'nonzero' else 0
    print(json.dumps({'Status':status, 'ExitCode':code, 'Error':'', 'OOMKilled':os.environ.get('PHPT_FAKE_MODE') == 'oom'}))
elif action == 'rm' and os.environ.get('PHPT_FAKE_MODE') == 'cleanup_failure': sys.exit(1)
""")
        fake.chmod(0o755)
        self.env = patch.dict(os.environ, {"PATH": str(self.root) + os.pathsep + os.environ.get("PATH", ""), "PHPT_FAKE_LOG": str(self.log)}, clear=False)
        self.env.start(); self.addCleanup(self.env.stop)

    def actions(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def reset_actions(self):
        if self.log.exists(): self.log.unlink()

    def test_only_pinned_images_and_valid_limits_are_accepted(self):
        for image, timeout, limit in (("fixture:latest", 1, 10), (IMAGE, 0, 10), (IMAGE, 1, 0)):
            with self.subTest(image=image, timeout=timeout, limit=limit):
                with self.assertRaises(ValueError): sandbox.execute_php(image, "/usr/bin/php", b"<?php ?>", timeout, limit, PLATFORM)
        self.assertEqual(self.actions(), [])

    def test_source_and_timeout_output_bounds_are_unaffected_by_platform(self):
        for source, timeout, limit in ((b"x" * (1024 * 1024 + 1), 5, 1000), ("<?php ?>", 5, 1000), (b"<?php ?>", 0, 1000), (b"<?php ?>", 5, 0)):
            with self.subTest(source_type=type(source).__name__, source_size=len(source), timeout=timeout, limit=limit):
                with self.assertRaises(ValueError): sandbox.execute_php(IMAGE, "/usr/bin/php", source, timeout, limit, PLATFORM)
        self.assertEqual(self.actions(), [])

    def test_platform_is_required_and_validated(self):
        for platform in (None, "", "linux/amd64/v2", "linux/arm/v7", "linux/riscv64", "windows/amd64", "Linux/amd64"):
            with self.subTest(platform=platform):
                with self.assertRaises(ValueError): sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, platform)
        with self.assertRaises(TypeError): sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000)
        self.assertEqual(self.actions(), [])

    def test_supported_platforms_are_forwarded_verbatim_to_create(self):
        for platform in PLATFORMS:
            with self.subTest(platform=platform):
                self.reset_actions()
                result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, platform)
                self.assertEqual(result.status, "completed")
                create = self.actions()[0]
                self.assertEqual(create[0], "create")
                self.assertIn("--platform", create)
                self.assertEqual(create[create.index("--platform") + 1], platform)

    def test_success_is_observed_in_restricted_container_and_cleaned(self):
        result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php echo 1;", 5, 1000, PLATFORM)
        self.assertEqual(result.status, "completed"); self.assertEqual(result.exit_code, 0)
        self.assertIn(b"actual output", result.output)
        actions = self.actions(); self.assertEqual([a[0] for a in actions], ["create", "start", "inspect", "rm"])
        create = actions[0]
        for flag in ("--platform", "--network", "--read-only", "--user", "--cap-drop", "--security-opt", "--memory", "--pids-limit", "--tmpfs", "--log-driver"):
            self.assertIn(flag, create)
        self.assertEqual(create[create.index("--platform")+1], PLATFORM)
        self.assertEqual(create[create.index("--network")+1], "none")
        self.assertNotIn("<?php echo 1;", " ".join(create))

    def test_mounted_source_bytes_are_not_rewritten(self):
        capture = self.root / "captured.bin"
        sources = (b"<?php echo \"caf\xe9\"; ?>", b"<?php echo 'nul\x00byte'; ?>", bytes(range(256)))
        for source in sources:
            with self.subTest(source_prefix=source[:16]):
                if capture.exists(): capture.unlink()
                with patch.dict(os.environ, {"PHPT_FAKE_CAPTURE": str(capture)}):
                    result = sandbox.execute_php(IMAGE, "/usr/bin/php", source, 5, 1000, PLATFORM)
                self.assertEqual(result.status, "completed")
                self.assertTrue(capture.exists())
                self.assertEqual(capture.read_bytes(), source)

    def test_nonzero_program_exit_is_retained_but_unstarted_container_is_error(self):
        with patch.dict(os.environ, {"PHPT_FAKE_MODE":"nonzero"}):
            result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, PLATFORM)
        self.assertEqual(result.status, "completed"); self.assertEqual(result.exit_code, 2)
        for mode in ("not_started", "attach_failure"):
            with self.subTest(mode=mode), patch.dict(os.environ, {"PHPT_FAKE_MODE":mode}):
                result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, PLATFORM)
            self.assertEqual(result.status, "runner_error")

    def test_oom_termination_cannot_report_completed(self):
        with patch.dict(os.environ, {"PHPT_FAKE_MODE":"oom"}):
            result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, PLATFORM)
        self.assertEqual(result.status, "runner_error")
        self.assertEqual(self.actions()[-1][0], "rm")

    def test_timeout_and_output_limit_never_claim_completion(self):
        for mode, expected, timeout in (("timeout", "timeout", 2), ("overflow", "output_limit", 5)):
            with self.subTest(mode=mode), patch.dict(os.environ, {"PHPT_FAKE_MODE":mode}):
                result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", timeout, 1000, PLATFORM)
                self.assertEqual(result.status, expected)
                self.assertLessEqual(len(result.output), 1000)
                self.assertEqual(self.actions()[-1][0], "rm")

    def test_inspect_failure_preserves_observed_output_and_cleans_up(self):
        import subprocess
        original_run = sandbox.subprocess.run
        def fail_inspect(command, *args, **kwargs):
            if command[1] == "inspect":
                raise subprocess.TimeoutExpired(command, 1)
            return original_run(command, *args, **kwargs)
        with patch.object(sandbox.subprocess, "run", side_effect=fail_inspect):
            result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, PLATFORM)
        self.assertEqual(result.status, "runner_error")
        self.assertIn(b"actual output", result.output)
        self.assertEqual(self.actions()[-1][0], "rm")

    def test_unconfirmed_cleanup_cannot_report_completed(self):
        with patch.dict(os.environ, {"PHPT_FAKE_MODE":"cleanup_failure"}):
            result = sandbox.execute_php(IMAGE, "/usr/bin/php", b"<?php ?>", 5, 1000, PLATFORM)
        self.assertEqual(result.status, "runner_error")
        self.assertIn("cleanup", result.reason)

if __name__ == "__main__": unittest.main()
