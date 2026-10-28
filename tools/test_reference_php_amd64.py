#!/usr/bin/env python3
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from reference_php import build_command


class TestReferencePhpAmd64(unittest.TestCase):
    def _archive_path(self, td, name="source.tar.xz"):
        archive = Path(td) / name
        archive.touch()
        return archive

    def _make_compat(self):
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

    def test_explicit_amd64_must_select_amd64_platform_and_c_digest_only(self):
        compat = self._make_compat()
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            argv = build_command(self._archive_path(td), 900, platform="linux/amd64")
        self.assertIn("--platform", argv)
        self.assertEqual(argv[argv.index("--platform") + 1], "linux/amd64")
        self.assertIn("buildpack-deps@sha256:" + "c" * 64, argv)
        self.assertNotIn("buildpack-deps@sha256:" + "b" * 64, argv)
        # same no-network/read-only/nonroot/cap-drop/timeout safety controls
        self.assertEqual(argv[argv.index("--network") + 1], "none")
        self.assertIn("--read-only", argv)
        self.assertIn("--user", argv)
        self.assertIn("--cap-drop", argv)
        self.assertEqual(argv[argv.index("--cap-drop") + 1], "ALL")
        self.assertIn("timeout", argv)

    def test_invalid_platform_raises(self):
        compat = self._make_compat()
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/s390x")

    def test_absent_additional_reference_builds_raises_before_docker(self):
        compat = self._make_compat()
        compat.pop("additional_reference_builds")
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")

    def test_duplicated_additional_reference_builds_raises(self):
        compat = self._make_compat()
        compat["additional_reference_builds"] = [
            {
                "platform": "linux/amd64",
                "image": "buildpack-deps@sha256:" + "c" * 64,
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            },
            {
                "platform": "linux/amd64",
                "image": "buildpack-deps@sha256:" + "d" * 64,
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            },
        ]
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")

    def test_malformed_additional_reference_builds_raises(self):
        compat = self._make_compat()
        compat["additional_reference_builds"] = [
            {
                "platform": "linux/amd64",
                "image": "not-pinned",
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            }
        ]
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")

    def test_unpinned_additional_reference_builds_raises(self):
        compat = self._make_compat()
        compat["additional_reference_builds"] = [
            {
                "platform": "linux/amd64",
                "image": "buildpack-deps:latest",
                "configure_options": ["--disable-all", "--enable-cli", "--without-pear"],
            }
        ]
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")

    def test_mismatched_additional_reference_builds_raises(self):
        compat = self._make_compat()
        compat["additional_reference_builds"] = [
            {
                "platform": "linux/amd64",
                "image": "buildpack-deps@sha256:" + "c" * 64,
                "configure_options": ["--enable-debug"],
            }
        ]
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=compat
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")

    def test_changing_arm64_to_amd64_must_fail(self):
        compat = self._make_compat()
        bad = dict(compat)
        bad["reference_build"] = dict(compat["reference_build"])
        bad["reference_build"]["platform"] = "linux/amd64"
        with tempfile.TemporaryDirectory() as td, patch(
            "reference_php._load_compat", return_value=bad
        ):
            with self.assertRaises((ValueError, OSError)):
                build_command(self._archive_path(td), 900, platform="linux/amd64")
