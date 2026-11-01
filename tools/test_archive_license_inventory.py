"""Behavioral tests for the archive license inventory capture command.

Fixtures are tiny locally generated archives. No upstream archive or toolchain
is involved; archive SHA-256 values in each lock are computed from the fixture.
"""

import hashlib
import io
import json
import os
import subprocess
import sys
import stat
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path


REPO = Path(__file__).resolve().parents[1]
CLI = REPO / "tools" / "archive_license_inventory.py"
# Capture contract limits. Candidate files are bounded separately from the
# inventory-wide entry count; non-candidate binaries need no per-file read cap.
MAX_MEMBER_BYTES = 16 * 1024 * 1024
MAX_MEMBER_COUNT = 100_000


class ArchiveLicenseInventoryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def make_tar(self, members):
        path = self.root / "fixture.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            for name, data, kind in members:
                info = tarfile.TarInfo(name)
                if kind == "symlink":
                    info.type = tarfile.SYMTYPE
                    info.linkname = data.decode()
                    archive.addfile(info)
                elif kind == "hardlink":
                    info.type = tarfile.LNKTYPE
                    info.linkname = data.decode()
                    archive.addfile(info)
                else:
                    info.size = len(data)
                    archive.addfile(info, io.BytesIO(data))
        return path

    def make_zip(self, members):
        path = self.root / "fixture.zip"
        with zipfile.ZipFile(path, "w") as archive:
            for name, data in members:
                archive.writestr(name, data, compress_type=zipfile.ZIP_STORED)
        return path

    def damaged_zip_crc(self, members):
        path = self.make_zip(members)
        raw = bytearray(path.read_bytes())
        # Flip a byte in the stored member payload without updating its CRC.
        marker = members[0][1]
        offset = raw.find(marker)
        self.assertGreaterEqual(offset, 0)
        raw[offset] ^= 1
        path.write_bytes(raw)
        return path

    def damaged_empty_zip_central_crc(self):
        path = self.make_zip([("go/LICENSE", b"")])
        raw = bytearray(path.read_bytes())
        central_directory = raw.find(b"PK\x01\x02")
        self.assertGreaterEqual(central_directory, 0)
        # Central-directory CRC-32 is four bytes at offset 16. The empty
        # member's correct CRC is zero, so corrupt only this metadata field.
        raw[central_directory + 16:central_directory + 20] = b"\x01\x00\x00\x00"
        path.write_bytes(raw)
        return path

    def lock_for(self, archive, *, version="1.2.3", platform="linux/amd64", digest=None,
                 archive_url=None):
        lock = self.root / "lock.json"
        suffix = "windows-amd64.zip" if platform.startswith("windows/") else f"linux-{platform.split('/')[1]}.tar.gz"
        archive_url = archive_url or f"https://go.dev/dl/go{version}.{suffix}"
        lock.write_text(json.dumps({"go": {
            "version": version,
            "platform": platform,
            "archive_url": archive_url,
            "archive_sha256": digest or hashlib.sha256(archive.read_bytes()).hexdigest(),
        }}), encoding="utf-8")
        return lock

    def run_capture(self, archive, *, version="1.2.3", locked_platform="linux/amd64",
                    platform="linux/amd64", digest=None, archive_url=None):
        # Assert implementation presence before invoking it: missing CLI is
        # never allowed to masquerade as a successful rejection test.
        self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
        lock = self.lock_for(archive, version=version, platform=locked_platform,
                             digest=digest, archive_url=archive_url)
        return subprocess.run(
            [sys.executable, str(CLI), "capture", "--lock", str(lock),
             "--component", "go", "--platform", platform, "--archive", str(archive)],
            cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    def assert_rejected(self, archive, **kwargs):
        result = self.run_capture(archive, **kwargs)
        self.assertNotEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        self.assertEqual(result.stdout, b"", "failed capture must not emit partial JSON")
        self.assertNotIn(b"Traceback", result.stderr)

    def test_rejects_non_object_lock_roots_without_traceback(self):
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        for root in ([], None):
            with self.subTest(root=root):
                self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
                lock = self.root / "invalid-lock.json"
                lock.write_text(json.dumps(root), encoding="utf-8")
                result = subprocess.run(
                    [sys.executable, str(CLI), "capture", "--lock", str(lock),
                     "--component", "go", "--platform", "linux/amd64",
                     "--archive", str(archive)],
                    cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    check=False, timeout=5,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, b"")
                self.assertNotIn(b"Traceback", result.stderr)

    @unittest.skipUnless(hasattr(os, "mkfifo"), "FIFO creation is unavailable")
    def test_rejects_lock_fifo_within_timeout_without_traceback(self):
        self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        lock = self.root / "lock.fifo"
        os.mkfifo(lock)
        result = subprocess.run(
            [sys.executable, str(CLI), "capture", "--lock", str(lock),
             "--component", "go", "--platform", "linux/amd64", "--archive", str(archive)],
            cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            check=False, timeout=5,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b"")
        self.assertNotIn(b"Traceback", result.stderr)

    def test_rejects_deeply_nested_lock_json_without_traceback(self):
        self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        lock = self.root / "deep-lock.json"
        lock.write_text("[" * 2000 + "0" + "]" * 2000, encoding="utf-8")
        result = subprocess.run(
            [sys.executable, str(CLI), "capture", "--lock", str(lock),
             "--component", "go", "--platform", "linux/amd64", "--archive", str(archive)],
            cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            check=False, timeout=5,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b"")
        self.assertNotIn(b"Traceback", result.stderr)

    @unittest.skipUnless(hasattr(os, "mkfifo"), "FIFO creation is unavailable")
    def test_rejects_archive_fifo_within_timeout_without_traceback(self):
        self.assertTrue(CLI.is_file(), f"CLI is not implemented: {CLI}")
        fifo = self.root / "archive.fifo"
        os.mkfifo(fifo)
        lock = self.root / "lock.json"
        lock.write_text(json.dumps({"go": {
            "version": "1.2.3",
            "platform": "linux/amd64",
            "archive_url": "https://go.dev/dl/go1.2.3.linux-amd64.tar.gz",
            "archive_sha256": "0" * 64,
        }}), encoding="utf-8")
        result = subprocess.run(
            [sys.executable, str(CLI), "capture", "--lock", str(lock),
             "--component", "go", "--platform", "linux/amd64", "--archive", str(fifo)],
            cwd=REPO, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            check=False, timeout=5,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b"")
        self.assertNotIn(b"Traceback", result.stderr)

    def test_tar_capture_is_partial_deterministic_and_reports_candidate_bytes(self):
        archive = self.make_tar([
            ("go/LICENSE", b"BSD license\n", "file"),
            ("go/README", b"not a license\n", "file"),
        ])
        first = self.run_capture(archive)
        self.assertEqual(first.returncode, 0, first.stderr.decode("utf-8", "replace"))
        result = json.loads(first.stdout)
        self.assertEqual(result.get("status"), "partial")
        rendered = json.dumps(result, sort_keys=True)
        self.assertIn("go/LICENSE", rendered)
        self.assertIn(hashlib.sha256(b"BSD license\n").hexdigest(), rendered)
        self.assertIn('"size": 12', rendered)
        self.assertEqual(first.stdout, self.run_capture(archive).stdout)

    def test_zip_capture_reports_candidate(self):
        archive = self.make_zip([("go/LICENSE.txt", b"MIT license")])
        result = self.run_capture(archive, locked_platform="windows/amd64",
                                  platform="windows/amd64")
        self.assertEqual(result.returncode, 0, result.stderr.decode("utf-8", "replace"))
        rendered = json.dumps(json.loads(result.stdout), sort_keys=True)
        self.assertIn("go/LICENSE.txt", rendered)
        self.assertIn(hashlib.sha256(b"MIT license").hexdigest(), rendered)

    def test_rejects_archive_checksum_mismatch(self):
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        self.assert_rejected(archive, digest="0" * 64)

    def test_rejects_locked_platform_mismatch(self):
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        self.assert_rejected(archive, locked_platform="linux/arm64")

    def test_rejects_version_platform_archive_url_mismatch(self):
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        self.assert_rejected(
            archive, locked_platform="linux/amd64",
            archive_url="https://go.dev/dl/go1.2.3.linux-arm64.tar.gz",
        )

    def test_rejects_duplicate_member_paths(self):
        archive = self.make_tar([
            ("go/LICENSE", b"one", "file"),
            ("go/LICENSE", b"two", "file"),
        ])
        self.assert_rejected(archive)

    def test_rejects_absolute_and_parent_traversal_paths(self):
        for name in ("/etc/LICENSE", "../LICENSE", "go/../../LICENSE"):
            with self.subTest(name=name):
                self.assert_rejected(self.make_tar([(name, b"lic", "file")]))

    def test_rejects_non_utf8_member_path(self):
        archive = self.root / "invalid-name.tar.gz"
        with tarfile.open(archive, "w:gz", encoding="utf-8", errors="surrogateescape") as tar:
            info = tarfile.TarInfo("go/LIC\udcffENSE")
            info.size = 1
            tar.addfile(info, io.BytesIO(b"x"))
        self.assert_rejected(archive)

    def test_rejects_truncated_tar_archive(self):
        archive = self.make_tar([("go/LICENSE", b"lic", "file")])
        archive.write_bytes(archive.read_bytes()[:-20])
        self.assert_rejected(archive)

    def test_rejects_tar_gzip_trailing_data(self):
        for suffix in (b"nonzero trailing bytes",):
            with self.subTest(suffix=suffix):
                archive = self.make_tar([("go/LICENSE", b"lic", "file")])
                archive.write_bytes(archive.read_bytes() + suffix)
                self.assert_rejected(archive)

        with self.subTest(suffix="second tar.gz stream"):
            archive = self.make_tar([("go/LICENSE", b"lic", "file")])
            second = self.root / "second.tar.gz"
            with tarfile.open(second, "w:gz") as tar:
                info = tarfile.TarInfo("go/NOTICE")
                info.size = 3
                tar.addfile(info, io.BytesIO(b"two"))
            archive.write_bytes(archive.read_bytes() + second.read_bytes())
            self.assert_rejected(archive)

    def test_enforces_uncompressed_size_limit(self):
        archive = self.make_tar([("go/LICENSE", b"x" * (MAX_MEMBER_BYTES + 1), "file")])
        self.assert_rejected(archive)

    def test_enforces_member_count_limit(self):
        archive = self.make_tar([
            (f"go/file-{index:05d}", b"x", "file") for index in range(MAX_MEMBER_COUNT + 1)
        ])
        self.assert_rejected(archive)

    def test_rejects_symlink_and_hardlink_candidates(self):
        for kind in ("symlink", "hardlink"):
            with self.subTest(kind=kind):
                archive = self.make_tar([("go/LICENSE", b"target", kind)])
                self.assert_rejected(archive)

    def test_rejects_zip_crc_corruption(self):
        archive = self.damaged_zip_crc([("go/LICENSE", b"original license")])
        self.assert_rejected(archive, locked_platform="windows/amd64", platform="windows/amd64")

    def test_rejects_zip_candidate_with_special_file_mode(self):
        for file_type in (stat.S_IFIFO, stat.S_IFCHR, stat.S_IFBLK):
            with self.subTest(file_type=file_type):
                archive = self.root / "special.zip"
                info = zipfile.ZipInfo("go/LICENSE")
                info.create_system = 3
                info.external_attr = (file_type | 0o600) << 16
                with zipfile.ZipFile(archive, "w") as zipped:
                    zipped.writestr(info, b"license")
                self.assert_rejected(
                    archive, locked_platform="windows/amd64", platform="windows/amd64"
                )

    def test_rejects_empty_zip_candidate_with_corrupt_central_crc(self):
        archive = self.damaged_empty_zip_central_crc()
        self.assert_rejected(archive, locked_platform="windows/amd64", platform="windows/amd64")


if __name__ == "__main__":
    unittest.main()
