#!/usr/bin/env python3
"""Regression tests for consuming only the verified PHP archive copy."""
import hashlib
import os
import re
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from reference_php import build_command


class TestReferencePhpArchiveRace(unittest.TestCase):
    def _compat(self):
        return {
            "schema_version": 1,
            "php": {"archive_sha256": "a" * 64},
            "reference_build": {
                "platform": "linux/arm64",
                "image": "buildpack-deps@sha256:" + "b" * 64,
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            },
            "additional_reference_builds": [
                {
                    "platform": "linux/amd64",
                    "image": "buildpack-deps@sha256:" + "c" * 64,
                    "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
                }
            ],
        }

    def _script(self, platform, archive):
        with patch("reference_php._load_compat", return_value=self._compat()):
            argv = build_command(archive, 900, platform=platform)
        return argv[-1]

    def _mapped_script_with_snapshot(self, root, archive_bytes):
        mounted = root / "mounted.tar.xz"
        private = root / "private.tar.xz"
        mounted.write_bytes(archive_bytes)
        compat = self._compat()
        compat["php"]["archive_sha256"] = hashlib.sha256(archive_bytes).hexdigest()
        with patch("reference_php._load_compat", return_value=compat):
            argv = build_command(mounted, 900, platform="linux/arm64")
        script = argv[-1]
        copy = re.search(r"\bcp\s+(?:--\s+)?/input/php\.tar\.xz\s+(/tmp/[^\s;]+)", script)
        self.assertIsNotNone(copy, "test requires the container's private archive snapshot step")
        script = script.replace("/input/php.tar.xz", str(mounted))
        script = script.replace(copy.group(1), str(private))
        script = script.replace("/tmp/build", str(root / "build"))
        return script, mounted, private

    @staticmethod
    def _write_tool(path, contents):
        path.write_text(contents, encoding="utf-8")
        path.chmod(0o755)

    def test_each_supported_profile_copies_then_verifies_and_extracts_one_private_archive(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "php.tar.xz"
            archive.touch()
            for platform in ("linux/arm64", "linux/amd64"):
                with self.subTest(platform=platform):
                    script = self._script(platform, archive)
                    copy = re.search(
                        r"\bcp\s+(?:--\s+)?/input/php\.tar\.xz\s+(/tmp/[^\s;]+)", script
                    )
                    self.assertIsNotNone(copy, "the bind-mounted archive must be snapshotted into /tmp")
                    private_archive = copy.group(1)
                    copied_at = copy.start()
                    hash_match = re.search(
                        r"actual=\$\(sha256sum\s+([^\s|)]+)", script
                    )
                    self.assertIsNotNone(hash_match, "the script must hash its build input")
                    tar_match = re.search(r"\btar\s+-xf\s+([^\s;]+)", script)
                    self.assertIsNotNone(tar_match, "the script must extract its build input")
                    self.assertEqual(hash_match.group(1), private_archive)
                    self.assertEqual(tar_match.group(1), private_archive)
                    self.assertLess(copied_at, hash_match.start())
                    self.assertLess(hash_match.start(), tar_match.start())
                    mounted_paths = list(re.finditer(r"/input/php\.tar\.xz", script))
                    self.assertEqual(len(mounted_paths), 1, "the bind mount must be read only once")
                    self.assertIn(mounted_paths[0].group(0), copy.group(0))

    def test_copy_failure_never_reaches_tar(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            script, _, _ = self._mapped_script_with_snapshot(root, b"archive")
            cp_called = root / "cp-called"
            tar_called = root / "tar-called"
            self._write_tool(
                bin_dir / "cp",
                f"#!/bin/sh\nprintf called > {str(cp_called)!r}\nexit 7\n",
            )
            self._write_tool(
                bin_dir / "tar",
                f"#!/bin/sh\nprintf called > {str(tar_called)!r}\nexit 0\n",
            )
            env = dict(os.environ)
            env["PATH"] = str(bin_dir) + os.pathsep + env.get("PATH", "")

            result = subprocess.run(
                ["sh", "-c", script], cwd=root, env=env, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(cp_called.exists(), "the copy failure double must run")
            self.assertFalse(tar_called.exists(), "tar must not run after the private copy fails")

    def test_digest_mismatch_never_reaches_tar(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            script, _, _ = self._mapped_script_with_snapshot(root, b"archive")
            tar_called = root / "tar-called"
            self._write_tool(bin_dir / "sha256sum", "#!/bin/sh\nprintf '%064d  %s\\n' 0 \"$1\"\n")
            self._write_tool(
                bin_dir / "tar",
                f"#!/bin/sh\nprintf called > {str(tar_called)!r}\nexit 0\n",
            )
            env = dict(os.environ)
            env["PATH"] = str(bin_dir) + os.pathsep + env.get("PATH", "")

            result = subprocess.run(
                ["sh", "-c", script], cwd=root, env=env, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("sha256 mismatch", result.stderr)
            self.assertFalse(tar_called.exists(), "tar must not run after a digest mismatch")

    def test_mount_rewrite_after_digest_does_not_change_extracted_bytes(self):
        original = b"verified archive bytes"
        rewritten = b"attacker replacement bytes"
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            mounted = root / "mounted.tar.xz"
            private = root / "private.tar.xz"
            build_dir = root / "build"
            bin_dir = root / "bin"
            observed = root / "extracted-input.bin"
            mounted.write_bytes(original)
            bin_dir.mkdir()

            compat = self._compat()
            compat["php"]["archive_sha256"] = hashlib.sha256(original).hexdigest()
            with patch("reference_php._load_compat", return_value=compat):
                argv = build_command(mounted, 900, platform="linux/arm64")
            script = argv[-1]
            copy = re.search(r"\bcp\s+(?:--\s+)?/input/php\.tar\.xz\s+(/tmp/[^\s;]+)", script)
            if copy is None:
                self.fail("container command does not snapshot the host-mounted archive")
            container_copy = copy.group(1)
            script = script.replace("/input/php.tar.xz", str(mounted))
            script = script.replace(container_copy, str(private))
            script = script.replace("/tmp/build", str(build_dir))

            sha_tool = bin_dir / "sha256sum"
            sha_tool.write_text(
                "#!/usr/bin/env python3\n"
                "import hashlib, pathlib, sys\n"
                f"mounted = pathlib.Path({str(mounted)!r})\n"
                f"replacement = {rewritten!r}\n"
                "for name in sys.argv[1:]:\n"
                "    data = pathlib.Path(name).read_bytes()\n"
                "    print(hashlib.sha256(data).hexdigest(), name)\n"
                "    if pathlib.Path(name).name == 'private.tar.xz':\n"
                "        mounted.write_bytes(replacement)\n",
                encoding="utf-8",
            )
            sha_tool.chmod(0o755)

            tar_tool = bin_dir / "tar"
            tar_tool.write_text(
                "#!/usr/bin/env python3\n"
                "import pathlib, sys\n"
                "archive = pathlib.Path(sys.argv[sys.argv.index('-xf') + 1])\n"
                f"pathlib.Path({str(observed)!r}).write_bytes(archive.read_bytes())\n"
                "source = pathlib.Path('php-8.5.12')\n"
                "source.mkdir(parents=True, exist_ok=True)\n"
                "configure = source / 'configure'\n"
                "configure.write_text('#!/bin/sh\\nexit 0\\n')\n"
                "configure.chmod(0o755)\n"
                "cli = source / 'sapi' / 'cli'\n"
                "cli.mkdir(parents=True, exist_ok=True)\n"
                "php = cli / 'php'\n"
                "php.write_text('#!/bin/sh\\nexit 0\\n')\n"
                "php.chmod(0o755)\n"
                "(source / 'run-tests.php').write_text('')\n",
                encoding="utf-8",
            )
            tar_tool.chmod(0o755)
            make_tool = bin_dir / "make"
            make_tool.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            make_tool.chmod(0o755)

            env = dict(os.environ)
            env["PATH"] = str(bin_dir) + os.pathsep + env.get("PATH", "")
            result = subprocess.run(
                ["sh", "-c", script], cwd=root, env=env, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(mounted.read_bytes(), rewritten, "the test double must rewrite the mount after hashing")
            self.assertEqual(observed.read_bytes(), original, "extraction must consume the verified snapshot")


if __name__ == "__main__":
    unittest.main()
