#!/usr/bin/env python3
"""Tests for the exact PHP reference build profile and its extraction check."""
import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import reference_php


class TestProfileValidator(unittest.TestCase):
    def setUp(self):
        self.profile = {
            "version": "8.5.12",
            "int_size": 8,
            "sapi": "cli",
            "os_family": "Linux",
            "machine": "aarch64",
            "ini_loaded": "",
            "ini_scanned": "",
            "extensions": ["Core", "date", "json"],
            "zend_extensions": ["Zend OPcache"],
        }

    def validate(self, profile, version="8.5.12", platform="linux/arm64"):
        return subprocess.run(
            [sys.executable, "-c", reference_php._PROFILE_VALIDATOR, version, platform],
            input=json.dumps(profile), text=True, capture_output=True, check=False,
        )

    def test_exact_arm64_profile_succeeds_and_sorts_extension_lists(self):
        self.profile["extensions"] = ["json", "Core", "date"]
        self.profile["zend_extensions"] = ["Zend OPcache", "xdebug"]
        result = self.validate(self.profile)
        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertEqual(output["extensions"], ["Core", "date", "json"])
        self.assertEqual(output["zend_extensions"], ["Zend OPcache", "xdebug"])

    def test_exact_amd64_profile_succeeds(self):
        profile = dict(self.profile, machine="x86_64")
        result = self.validate(profile, platform="linux/amd64")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_every_profile_mismatch(self):
        cases = [
            (dict(self.profile, version="8.5.11"), "8.5.12", "linux/arm64"),
            (dict(self.profile, int_size=4), "8.5.12", "linux/arm64"),
            (dict(self.profile, sapi="fpm-fcgi"), "8.5.12", "linux/arm64"),
            (dict(self.profile, os_family="Windows"), "8.5.12", "linux/arm64"),
            (dict(self.profile, machine="armv7l"), "8.5.12", "linux/arm64"),
            (dict(self.profile, ini_loaded="/etc/php.ini"), "8.5.12", "linux/arm64"),
            (dict(self.profile, ini_scanned="/etc/php/conf.d"), "8.5.12", "linux/arm64"),
            (dict(self.profile, extensions="Core"), "8.5.12", "linux/arm64"),
            (dict(self.profile, zend_extensions=[1]), "8.5.12", "linux/arm64"),
            (dict(self.profile, version=8.5), "8.5.12", "linux/arm64"),
            (dict(self.profile, int_size=True), "8.5.12", "linux/arm64"),
            (dict(self.profile, sapi=None), "8.5.12", "linux/arm64"),
            (dict(self.profile, os_family=[]), "8.5.12", "linux/arm64"),
            (dict(self.profile, machine={}), "8.5.12", "linux/arm64"),
            (dict(self.profile, ini_loaded=0), "8.5.12", "linux/arm64"),
            (dict(self.profile, ini_scanned=False), "8.5.12", "linux/arm64"),
            (dict(self.profile, extensions=None), "8.5.12", "linux/arm64"),
            (dict(self.profile, zend_extensions={}), "8.5.12", "linux/arm64"),
            (dict(self.profile, machine="aarch64"), "8.5.12", "linux/amd64"),
        ]
        for profile, version, platform in cases:
            with self.subTest(profile=profile, platform=platform):
                result = self.validate(profile, version, platform)
                self.assertNotEqual(result.returncode, 0)


class TestBuildProfileGate(unittest.TestCase):
    def setUp(self):
        self.compat = reference_php._load_compat()

    def test_missing_or_invalid_php_version_is_rejected_before_build_command(self):
        for value in (None, "", "8.5", "8.5.12-dev", 8.5):
            lock = copy.deepcopy(self.compat)
            if value is None:
                del lock["php"]["version"]
            else:
                lock["php"]["version"] = value
            with self.subTest(version=value), tempfile.TemporaryDirectory() as td:
                archive = Path(td) / "source.tar.xz"
                archive.touch()
                with self.assertRaises((ValueError, OSError, KeyError, TypeError)):
                    reference_php._build_command(archive, 900, lock)

    def test_arm64_and_amd64_commands_extract_and_check_the_profile(self):
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.touch()
            for platform in ("linux/arm64", "linux/amd64"):
                with self.subTest(platform=platform):
                    argv = reference_php._build_command(archive, 900, self.compat, platform)
                    script = argv[-1]
                    self.assertEqual(argv[argv.index("--platform") + 1], platform)
                    self.assertIn("PHP_PROFILE", script)
                    self.assertIn("ini_loaded", script)
                    self.assertIn("ini_scanned", script)
                    self.assertIn("zend_extensions", script)
                    self.assertIn("_PROFILE_VALIDATOR", script)


if __name__ == "__main__":
    unittest.main()
