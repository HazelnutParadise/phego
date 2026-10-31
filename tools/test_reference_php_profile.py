#!/usr/bin/env python3
"""Tests for the exact PHP reference build profile and its extraction check."""
import copy
import json
import shlex
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import reference_php


_ARM64_PROFILE_EXPECTED = {
    "extensions": ["Core", "Reflection", "SPL", "Zend OPcache", "date", "hash", "json", "lexbor", "pcre", "random", "standard", "uri"],
    "zend_extensions": ["Zend OPcache"],
    "session_available": False,
    "session_upload_progress": False,
}
_AMD64_PROFILE_EXPECTED = {
    "extensions": ["Core", "Reflection", "SPL", "Zend OPcache", "date", "hash", "json", "lexbor", "pcre", "random", "standard", "uri"],
    "zend_extensions": ["Zend OPcache"],
    "session_available": False,
    "session_upload_progress": False,
}


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
            "extensions": list(_ARM64_PROFILE_EXPECTED["extensions"]),
            "zend_extensions": ["Zend OPcache"],
            "session_available": False,
            "session_upload_progress": False,
        }

    def validate(self, profile, version="8.5.12", platform="linux/arm64", expected=None):
        argv = [sys.executable, "-c", reference_php._PROFILE_VALIDATOR, version, platform]
        if expected is not None:
            argv.append(json.dumps(expected))
        return subprocess.run(
            argv,
            input=json.dumps(profile), text=True, capture_output=True, check=False,
        )

    def assert_profile_rejected(self, result, reason):
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(reason, result.stderr)

    def test_exact_arm64_profile_succeeds_and_sorts_extension_lists(self):
        self.profile["extensions"] = list(reversed(self.profile["extensions"]))
        result = self.validate(self.profile, expected=_ARM64_PROFILE_EXPECTED)
        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertEqual(output["extensions"], _ARM64_PROFILE_EXPECTED["extensions"])
        self.assertEqual(output["zend_extensions"], _ARM64_PROFILE_EXPECTED["zend_extensions"])

    def test_exact_amd64_profile_succeeds(self):
        profile = dict(self.profile, machine="x86_64")
        result = self.validate(profile, platform="linux/amd64", expected=_AMD64_PROFILE_EXPECTED)
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
                expected = _AMD64_PROFILE_EXPECTED if platform == "linux/amd64" else _ARM64_PROFILE_EXPECTED
                result = self.validate(profile, version, platform, expected)
                self.assertNotEqual(result.returncode, 0)

    def _profile_for_platform(self, platform):
        profile = dict(self.profile)
        if platform == "linux/amd64":
            profile["machine"] = "x86_64"
        profile.update(_ARM64_PROFILE_EXPECTED if platform == "linux/arm64" else _AMD64_PROFILE_EXPECTED)
        return profile

    def test_reference_extension_and_session_profiles_match_for_each_platform(self):
        for platform, expected in (("linux/arm64", _ARM64_PROFILE_EXPECTED),
                                   ("linux/amd64", _AMD64_PROFILE_EXPECTED)):
            with self.subTest(platform=platform):
                profile = self._profile_for_platform(platform)
                profile["extensions"] = list(reversed(profile["extensions"]))
                result = self.validate(profile, platform=platform, expected=expected)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_added_missing_and_duplicate_extensions_for_each_platform(self):
        for platform, expected in (("linux/arm64", _ARM64_PROFILE_EXPECTED),
                                   ("linux/amd64", _AMD64_PROFILE_EXPECTED)):
            for key in ("extensions", "zend_extensions"):
                baseline = self._profile_for_platform(platform)
                mutations = (
                    lambda p: p[key].append("unexpected"),
                    lambda p: p[key].pop(),
                    lambda p: p[key].append(p[key][0]),
                )
                for mutation in mutations:
                    profile = json.loads(json.dumps(baseline))
                    mutation(profile)
                    with self.subTest(platform=platform, key=key, profile=profile):
                        result = self.validate(profile, platform=platform, expected=expected)
                        self.assert_profile_rejected(result, key)

    def test_rejects_mismatched_missing_and_wrongly_typed_session_values(self):
        for platform, expected in (("linux/arm64", _ARM64_PROFILE_EXPECTED),
                                   ("linux/amd64", _AMD64_PROFILE_EXPECTED)):
            for key, value in (("session_available", True), ("session_upload_progress", True),
                               ("session_available", None), ("session_upload_progress", "false")):
                profile = self._profile_for_platform(platform)
                profile[key] = value
                with self.subTest(platform=platform, key=key, value=value):
                    result = self.validate(profile, platform=platform, expected=expected)
                    self.assert_profile_rejected(result, key)
            for key in ("session_available", "session_upload_progress"):
                profile = self._profile_for_platform(platform)
                del profile[key]
                with self.subTest(platform=platform, missing=key):
                    result = self.validate(profile, platform=platform, expected=expected)
                    self.assert_profile_rejected(result, key)

    def test_validator_rejects_missing_expected_profile(self):
        for platform in ("linux/arm64", "linux/amd64"):
            with self.subTest(platform=platform, missing_expected=True):
                result = self.validate(self._profile_for_platform(platform), platform=platform)
                self.assert_profile_rejected(result, "expected")


class TestBuildProfileGate(unittest.TestCase):
    def setUp(self):
        self.compat = reference_php._load_compat()

    def _lock_with_profiles(self):
        lock = copy.deepcopy(self.compat)
        lock["reference_build"]["expected_profile"] = copy.deepcopy(_ARM64_PROFILE_EXPECTED)
        amd64 = next(item for item in lock["additional_reference_builds"]
                     if item["platform"] == "linux/amd64")
        amd64["expected_profile"] = copy.deepcopy(_AMD64_PROFILE_EXPECTED)
        return lock

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
        lock = self._lock_with_profiles()
        with tempfile.TemporaryDirectory() as td:
            archive = Path(td) / "source.tar.xz"
            archive.touch()
            for platform in ("linux/arm64", "linux/amd64"):
                with self.subTest(platform=platform):
                    argv = reference_php._build_command(archive, 900, lock, platform)
                    script = argv[-1]
                    self.assertEqual(argv[argv.index("--platform") + 1], platform)
                    self.assertIn("PHP_PROFILE", script)
                    self.assertIn("ini_loaded", script)
                    self.assertIn("ini_scanned", script)
                    self.assertIn("zend_extensions", script)
                    self.assertIn("_PROFILE_VALIDATOR", script)
                    expected = lock["reference_build"]["expected_profile"] if platform == "linux/arm64" else next(
                        item["expected_profile"] for item in lock["additional_reference_builds"]
                        if item["platform"] == platform
                    )
                    embedded_profiles = []
                    for token in shlex.split(script):
                        try:
                            value = json.loads(token)
                        except (json.JSONDecodeError, TypeError):
                            continue
                        if isinstance(value, dict):
                            embedded_profiles.append(value)
                    self.assertIn(expected, embedded_profiles)

    def test_each_platform_requires_its_own_lock_expectation(self):
        for platform, missing_from in (("linux/arm64", "reference_build"),
                                       ("linux/amd64", "additional_reference_builds")):
            lock = self._lock_with_profiles()
            if missing_from == "reference_build":
                del lock["reference_build"]["expected_profile"]
            else:
                amd64 = next(item for item in lock["additional_reference_builds"]
                             if item["platform"] == platform)
                del amd64["expected_profile"]
            with self.subTest(platform=platform), tempfile.TemporaryDirectory() as td:
                archive = Path(td) / "source.tar.xz"
                archive.touch()
                with self.assertRaises((ValueError, OSError, KeyError, TypeError)):
                    reference_php._build_command(archive, 900, lock, platform)


if __name__ == "__main__":
    unittest.main()
