#!/usr/bin/env python3
"""Behavioral contract tests for bootstrapped Go source builds."""
import hashlib
import copy
import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from reference_go import build_command
from test_reference_go import SCRIPT, valid_lock


ROOT = Path(__file__).resolve().parents[1]


def source_lock():
    lock = valid_lock()
    lock["go"]["source_build"] = {
        "version": lock["go"]["version"],
        "archive_url": "https://go.dev/dl/go" + lock["go"]["version"] + ".src.tar.gz",
        "archive_sha256": "e" * 64,
    }
    return lock


class SourceCommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bootstrap = self.root / "bootstrap.tar.gz"
        self.source = self.root / "source.tar.gz"
        self.bootstrap.write_bytes(b"bootstrap fixture")
        self.source.write_bytes(b"source fixture")
        self.lock = source_lock()
        self.lock["go"]["archive_sha256"] = hashlib.sha256(self.bootstrap.read_bytes()).hexdigest()
        self.lock["go"]["source_build"]["archive_sha256"] = hashlib.sha256(self.source.read_bytes()).hexdigest()

    def command(self, **kwargs):
        return build_command(self.bootstrap, "linux/arm64", 900, self.lock, **kwargs)

    def test_four_argument_api_remains_the_binary_archive_mode(self):
        argv = build_command(self.bootstrap, "linux/arm64", 900, valid_lock())
        self.assertEqual(argv[0], "docker")
        self.assertIn("/input/go.tar.gz", argv[-1])
        self.assertNotIn("GOROOT_BOOTSTRAP", argv[-1])

    def test_source_metadata_is_required_complete_and_tied_to_go_version(self):
        valid_argv = build_command(self.bootstrap, "linux/arm64", 900, self.lock,
                                   source_archive=self.source)
        self.assertEqual(valid_argv[0], "docker", "valid locked metadata and both archive digests must be accepted")
        cases = (
            (lambda source: None, "missing go.source_build"),
            (lambda source: source.pop("version"), "source archive URL and version must match go.version"),
            (lambda source: source.pop("archive_url"), "source archive URL and version must match go.version"),
            (lambda source: source.pop("archive_sha256"), "source archive_sha256 must be a 64-hex digit digest"),
            (lambda source: source.update(version="1.27.4"), "source archive URL and version must match go.version"),
            (lambda source: source.update(archive_url="https://example.invalid/go.src.tar.gz"), "source archive URL and version must match go.version"),
            (lambda source: source.update(archive_url="https://go.dev/dl/go1.27.4.src.tar.gz"), "source archive URL and version must match go.version"),
            (lambda source: source.update(archive_sha256="z" * 64), "source archive_sha256 must be a 64-hex digit digest"),
        )
        for change, expected_error in cases:
            lock = copy.deepcopy(self.lock)
            if expected_error == "missing go.source_build":
                lock["go"].pop("source_build")
            else:
                change(lock["go"]["source_build"])
            with self.subTest(expected_error=expected_error, source_build=lock["go"].get("source_build")):
                with self.assertRaisesRegex(ValueError, re.escape(expected_error)):
                    build_command(self.bootstrap, "linux/arm64", 900, lock, source_archive=self.source)

    def test_source_mode_requires_existing_regular_archives_and_verified_digests(self):
        missing = self.root / "missing.tar.gz"
        with self.assertRaises((OSError, ValueError)):
            build_command(self.bootstrap, "linux/arm64", 900, self.lock, source_archive=missing)
        with self.assertRaises((OSError, ValueError)):
            build_command(self.bootstrap, "linux/arm64", 900, self.lock, source_archive=self.root)
        for archive in (self.bootstrap, self.source):
            archive.write_bytes(b"changed after lock")
            with self.assertRaises((OSError, ValueError)):
                build_command(self.bootstrap, "linux/arm64", 900, self.lock, source_archive=self.source)
            # Reset both fixtures to their locked contents for the next side.
            self.bootstrap.write_bytes(b"bootstrap fixture")
            self.source.write_bytes(b"source fixture")

    def test_source_build_runs_in_restricted_container_with_two_gib_tmpfs(self):
        argv = self.command(source_archive=self.source)
        self.assertEqual(argv[0], "docker")
        self.assertEqual(argv[argv.index("--network") + 1], "none")
        self.assertIn("--read-only", argv)
        self.assertEqual(argv[argv.index("--user") + 1], "65534:65534")
        self.assertEqual(argv[argv.index("--cpus") + 1], "2")
        self.assertEqual(argv[argv.index("--memory") + 1], "3g")
        self.assertIn("size=2g", argv[argv.index("--tmpfs") + 1])
        script = argv[-1]
        for setting in ("CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off"):
            self.assertRegex(script, rf"(?:export\s+)?{setting}")
        self.assertIn("GOROOT_BOOTSTRAP", script)
        self.assertIn("make.bash", script)
        self.assertIn("go version", script)
        self.assertIn("probe", script)

    def test_each_mounted_archive_is_copied_then_verified_then_extracted_from_private_copy(self):
        argv = self.command(source_archive=self.source)
        mounts = [argv[i + 1] for i, value in enumerate(argv[:-1]) if value == "--mount"]
        self.assertEqual(len(mounts), 2, "bootstrap and source must have separate read-only mounts")
        script = argv[-1]
        for digest, target in ((self.lock["go"]["archive_sha256"], "/input/go.tar.gz"),
                               (self.lock["go"]["source_build"]["archive_sha256"], "/input/go.src.tar.gz")):
            target_name = re.escape(target)
            copy = re.search(r"\bcp\s+(?:--\s+)?" + target_name + r"\s+(/tmp/[^ ;]+)", script)
            self.assertIsNotNone(copy, f"copy mounted {target} to a private container path first")
            private_copy = copy.group(1)
            verify = re.search(r"sha256sum\s+([^ ;)]+)", script[copy.end():])
            self.assertIsNotNone(verify, f"verify the private copy of {target}")
            self.assertEqual(verify.group(1), private_copy)
            self.assertIn(digest, script[copy.end():], "the locked digest must be checked")
            extract = re.search(r"tar\s+-xzf\s+([^ ;]+)", script[copy.end() + verify.end():])
            self.assertIsNotNone(extract, f"extract {target} only after its private-copy digest check")
            self.assertEqual(extract.group(1), private_copy)

    def test_source_and_bootstrap_have_distinct_roots_and_exact_toolchain_checks(self):
        script = self.command(source_archive=self.source)[-1]
        self.assertRegex(script, r"GOROOT_BOOTSTRAP=[^ ;]+bootstrap[^ ;]*")
        self.assertRegex(script, r"source[^ ;]*/src/make\.bash|src/make\.bash")
        self.assertGreaterEqual(script.count("go version"), 2)
        self.assertIn("linux/arm64", script)
        self.assertIn(self.lock["go"]["version"], script)

    def test_amd64_source_build_command_checks_complete_version_and_target_platform(self):
        lock = source_lock()
        bootstrap = lock["go"]["additional_archives"][0]
        bootstrap["archive_sha256"] = hashlib.sha256(self.bootstrap.read_bytes()).hexdigest()
        lock["go"]["source_build"]["archive_sha256"] = hashlib.sha256(self.source.read_bytes()).hexdigest()
        argv = build_command(self.bootstrap, "linux/amd64", 900, lock, source_archive=self.source)
        script = argv[-1]
        self.assertIn("linux/amd64", argv)
        self.assertIn(bootstrap["probe_image"], argv)
        self.assertEqual(argv[argv.index("--memory") + 1], "3g")
        self.assertIn('go version go1.27.3 linux/amd64', script)
        self.assertGreaterEqual(script.count("go version"), 2)
        self.assertIn("rebuilt_version_platform_mismatch", script)
        self.assertIn("bootstrap_version_platform_mismatch", script)

    def test_generated_shell_with_controlled_tools_and_toolchain_stubs(self):
        """Run the emitted payload locally with fake cp/hash/tar/Go tools."""
        script = self.command(source_archive=self.source)[-1]
        sandbox = self.root / "shell-sandbox"
        temp_root, input_root, fake_bin = (sandbox / name for name in ("tmp", "input", "bin"))
        for directory in (temp_root, input_root, fake_bin):
            directory.mkdir(parents=True)
        (input_root / "go.tar.gz").write_bytes(self.bootstrap.read_bytes())
        (input_root / "go.src.tar.gz").write_bytes(self.source.read_bytes())
        events = sandbox / "events"
        rebuilt_go_file = sandbox / "rebuilt-go"
        rebuilt_go_file.write_text('''#!/bin/sh
if [ "$1" = version ]; then echo "go version $REBUILT_VERSION $REBUILT_PLATFORM"; exit 0; fi
if [ "$1" = build ]; then
  echo go-build >> "$EVENTS"
  printf '%s\\n' '#!/bin/sh' 'echo probe-ran >> "$EVENTS"' > "$3"
  chmod +x "$3"
  exit 0
fi
exit 2
''')
        rebuilt_go_file.chmod(0o755)

        stubs = {
            "cp": """import os,pathlib,shutil,sys
args=sys.argv[1:]
with open(os.environ['EVENTS'],'a') as f:f.write('cp '+pathlib.Path(args[-1]).name+'\\n')
shutil.copyfile(args[-2],args[-1])
""",
            "sha256sum": """import hashlib,os,pathlib,sys
p=pathlib.Path(sys.argv[-1])
with open(os.environ['EVENTS'],'a') as f:f.write('sha '+p.name+'\\n')
print(('0'*64 if os.environ.get('BAD_DIGEST')==p.name else hashlib.sha256(p.read_bytes()).hexdigest()),p)
""",
            "tar": """import os,pathlib,sys
args=sys.argv[1:]; archive=pathlib.Path(args[args.index('-xzf')+1]); dest=pathlib.Path(args[args.index('-C')+1])
with open(os.environ['EVENTS'],'a') as f:f.write('tar '+archive.name+'\\n')
go=dest/'go'
if 'bootstrap' in archive.name:
 p=go/'bin'/'go'; p.parent.mkdir(parents=True,exist_ok=True)
 p.write_text('#!/bin/sh\\nif [ "$1" = version ]; then echo "go version $BOOTSTRAP_VERSION $BOOTSTRAP_PLATFORM"; exit 0; fi\\nexit 2\\n'); p.chmod(0o755)
else:
 (go/'VERSION').parent.mkdir(parents=True,exist_ok=True); (go/'VERSION').write_text(os.environ['SOURCE_VERSION']+'\\n')
 (go/'bin').mkdir(parents=True,exist_ok=True)
 make=go/'src'/'make.bash'; make.parent.mkdir(parents=True,exist_ok=True)
 make.write_text('#!/bin/sh\\n'
   'echo make >> "$EVENTS"\\n'
   'if [ "$MAKE_FAIL" = 1 ]; then exit 19; fi\\n'
   'cp "$REBUILT_GO_FILE" "$(dirname "$0")/../bin/go"\\n'
   'chmod +x "$(dirname "$0")/../bin/go"\\n')
 make.chmod(0o755)
""",
        }
        for name, body in stubs.items():
            path = fake_bin / name
            path.write_text("#!" + sys.executable + "\n" + body)
            path.chmod(0o755)

        cases = (
            ("success", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.3", "", 0, True, True, "linux/arm64"),
            ("bootstrap version mismatch", "go1.27.4", "linux/arm64", "go1.27.3", "go1.27.3", "", 1, False, False, "linux/arm64"),
            ("bootstrap platform mismatch", "go1.27.3", "linux/amd64", "go1.27.3", "go1.27.3", "", 1, False, False, "linux/arm64"),
            ("bootstrap private-copy digest mismatch", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.3", "phego-bootstrap.tar.gz", 1, False, False, "linux/arm64"),
            ("source VERSION mismatch", "go1.27.3", "linux/arm64", "go1.27.4", "go1.27.3", "", 1, False, False, "linux/arm64"),
            ("make.bash failure", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.3", "make", 19, True, False, "linux/arm64"),
            ("rebuilt version mismatch", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.4", "", 1, True, False, "linux/arm64"),
            ("rebuilt platform mismatch", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.3", "", 1, True, False, "linux/amd64"),
            ("source private-copy digest mismatch", "go1.27.3", "linux/arm64", "go1.27.3", "go1.27.3", "phego-source.tar.gz", 1, False, False, "linux/arm64"),
        )
        for (label, bootstrap_version, bootstrap_platform, source_version, rebuilt_version,
             failure, status, make_expected, probe_expected, rebuilt_platform) in cases:
            with self.subTest(case=label):
                events.write_text("")
                env = dict(os.environ, PATH=str(fake_bin) + os.pathsep + os.environ.get("PATH", ""),
                           EVENTS=str(events), BOOTSTRAP_VERSION=bootstrap_version,
                           BOOTSTRAP_PLATFORM=bootstrap_platform,
                           SOURCE_VERSION=source_version, REBUILT_VERSION=rebuilt_version,
                           REBUILT_PLATFORM=rebuilt_platform,
                           MAKE_FAIL="1" if failure == "make" else "0",
                           BAD_DIGEST=failure if failure != "make" else "",
                           REBUILT_GO_FILE=str(rebuilt_go_file))
                mapped = re.sub(r"(?<![A-Za-z0-9_])/(input|tmp)(?=/|\b)",
                                lambda match: str(input_root if match.group(1) == "input" else temp_root), script)
                result = subprocess.run(["/bin/sh", "-c", mapped], cwd=sandbox, env=env,
                                        capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, status, result.stderr or result.stdout)
                observed = events.read_text().splitlines()
                self.assertEqual("make" in observed, make_expected)
                self.assertEqual("go-build" in observed, probe_expected)
                self.assertEqual("probe-ran" in observed, probe_expected)
                if probe_expected:
                    self.assertIn("make", observed)
                    self.assertTrue(result.stdout.strip().endswith("source_build_probe_ok"))
                else:
                    self.assertNotIn("go-build", observed, "failed validation/build must stop before probe build")


class SourceCliTests(unittest.TestCase):
    """Use the existing fake-Docker lifecycle pattern; no real Docker is run."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "project"
        (self.root / "tools").mkdir(parents=True)
        (self.root / "tools" / "reference_go.py").write_bytes(SCRIPT.read_bytes())
        self.bootstrap = self.root / "bootstrap.tar.gz"
        self.source = self.root / "source.tar.gz"
        self.bootstrap.write_bytes(b"bootstrap")
        self.source.write_bytes(b"source")
        self.lock = source_lock()
        self.lock["go"]["archive_sha256"] = hashlib.sha256(self.bootstrap.read_bytes()).hexdigest()
        self.lock["go"]["source_build"]["archive_sha256"] = hashlib.sha256(self.source.read_bytes()).hexdigest()
        (self.root / "compat.lock.json").write_text(json.dumps(self.lock))
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.events = self.root / "docker.jsonl"
        docker = self.bin / "docker"
        docker.write_text(
            "#!" + sys.executable + "\nimport json,os,pathlib,sys\n"
            "args=sys.argv[1:]\n"
            "with open(os.environ['DOCKER_EVENTS'],'a') as f:f.write(json.dumps(args)+'\\n')\n"
            "if args and args[0]=='run' and '--cidfile' in args:pathlib.Path(args[args.index('--cidfile')+1]).write_text('a'*64)\n"
            "sys.exit(0)\n"
        )
        docker.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ.get("PATH", ""), DOCKER_EVENTS=str(self.events))

    def invoke(self, *args):
        return subprocess.run([sys.executable, str(self.root / "tools" / "reference_go.py"), *map(str, args)],
                              cwd=self.root, env=self.env, capture_output=True, text=True, timeout=8)

    def test_help_explains_source_build_bootstrap_platform_default_and_examples(self):
        result = self.invoke("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        for text in (
            "--source-build SOURCE_ARCHIVE",
            "bootstrap",
            "default: linux/arm64",
            "Examples:",
            "--source-build go.src.tar.gz",
        ):
            with self.subTest(text=text):
                self.assertIn(text, result.stdout)
        self.assertFalse(self.events.exists(), "--help must not start Docker")

    def test_source_build_option_preserves_archive_positional_as_bootstrap(self):
        result = self.invoke("--timeout", "1", "--source-build", self.source, self.bootstrap)
        self.assertEqual(result.returncode, 0, result.stderr)
        rows = [json.loads(row) for row in self.events.read_text().splitlines()]
        run = rows[0]
        mounts = [run[i + 1] for i, value in enumerate(run[:-1]) if value == "--mount"]
        self.assertEqual(len(mounts), 2)
        self.assertTrue(any(str(self.bootstrap) in value for value in mounts))
        self.assertTrue(any(str(self.source) in value for value in mounts))

    def test_source_archive_hash_mismatch_stops_before_docker(self):
        self.source.write_bytes(b"changed source")
        result = self.invoke("--source-build", self.source, self.bootstrap)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists(), "both host files must verify before Docker starts")

    def test_bootstrap_hash_mismatch_stops_before_docker(self):
        self.bootstrap.write_bytes(b"changed bootstrap")
        result = self.invoke("--source-build", self.source, self.bootstrap)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists(), "both host files must verify before Docker starts")

    def test_legacy_positional_cli_still_launches_binary_probe(self):
        lock = valid_lock()
        lock["go"]["archive_sha256"] = hashlib.sha256(self.bootstrap.read_bytes()).hexdigest()
        (self.root / "compat.lock.json").write_text(json.dumps(lock))
        result = self.invoke("--timeout", "1", self.bootstrap)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.events.exists())


if __name__ == "__main__":
    unittest.main()
