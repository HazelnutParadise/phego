import hashlib
import io
import json
import tarfile
import unittest
from unittest.mock import patch

import upstream_snapshot as source


def archive_with_pax(size):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w:gz", format=tarfile.PAX_FORMAT) as tar:
        for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS"):
            info = tarfile.TarInfo("php-8.2.0/" + path)
            info.size = 1
            if path == "NEWS":
                info.pax_headers = {"comment": "x" * size}
            tar.addfile(info, io.BytesIO(b"x"))
    data = output.getvalue()
    return data, hashlib.sha256(data).hexdigest()


def snapshot(data, digest):
    return source.capture(data, digest, "8.2.0", "php-8.2.0", "a" * 40,
                          "https://www.php.net/releases/8_2_0.php")


class TestLimits(unittest.TestCase):
    def test_metadata_must_be_bounded_before_parsing(self):
        data, digest = archive_with_pax(2048)
        with patch.object(source, "MAX_METADATA", 1024, create=True):
            with self.assertRaises(ValueError):
                snapshot(data, digest)

    def test_metadata_within_limit_is_accepted(self):
        data, digest = archive_with_pax(20)
        with patch.object(source, "MAX_METADATA", 1024, create=True):
            self.assertEqual(len(snapshot(data, digest)["files"]), 4)

    def test_gnu_longname_metadata_is_bounded(self):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:gz", format=tarfile.GNU_FORMAT) as tar:
            for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS", "x" * 200):
                info = tarfile.TarInfo("php-8.2.0/" + path)
                info.size = 1
                tar.addfile(info, io.BytesIO(b"x"))
        data = output.getvalue()
        with patch.object(source, "MAX_METADATA", 64):
            with self.assertRaisesRegex(ValueError, "metadata limit"):
                snapshot(data, hashlib.sha256(data).hexdigest())

    def test_metadata_header_count_is_bounded(self):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w:gz", format=tarfile.PAX_FORMAT) as tar:
            for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS"):
                info = tarfile.TarInfo("php-8.2.0/" + path)
                info.size = 1
                info.pax_headers = {"comment": "short"}
                tar.addfile(info, io.BytesIO(b"x"))
        data = output.getvalue()
        with patch.object(source, "MAX_METADATA_HEADERS", 1):
            with self.assertRaisesRegex(ValueError, "metadata limit"):
                snapshot(data, hashlib.sha256(data).hexdigest())

    def test_truncated_gzip_is_a_validation_error(self):
        data = b"\x1f\x8b\x08"
        with self.assertRaises(ValueError):
            snapshot(data, hashlib.sha256(data).hexdigest())

    def test_corrupt_header_after_valid_files_is_rejected(self):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w") as tar:
            for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS"):
                info = tarfile.TarInfo("php-8.2.0/" + path)
                info.size = 1
                tar.addfile(info, io.BytesIO(b"x"))
        data = bytearray(output.getvalue())
        data[4096:4608] = b"X" * 512
        with self.assertRaises(ValueError):
            snapshot(bytes(data), hashlib.sha256(data).hexdigest())

    def test_sparse_entries_are_rejected_before_processing(self):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w") as tar:
            for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS"):
                info = tarfile.TarInfo("php-8.2.0/" + path)
                info.size = 1
                if path == "NEWS":
                    info.type = tarfile.GNUTYPE_SPARSE
                tar.addfile(info, io.BytesIO(b"x"))
        data = output.getvalue()
        with self.assertRaises(ValueError):
            snapshot(data, hashlib.sha256(data).hexdigest())

    def test_pax_sparse_variants_are_rejected(self):
        variants = ({"GNU.sparse.map": "0,1"}, {"GNU.sparse.size": "1"},
                    {"GNU.sparse.major": "1", "GNU.sparse.minor": "0"})
        for headers in variants:
            with self.subTest(headers=headers):
                output = io.BytesIO()
                with tarfile.open(fileobj=output, mode="w:gz", format=tarfile.PAX_FORMAT) as tar:
                    for path in ("Zend/zend_language_parser.y", "a.stub.php", "a.phpt", "NEWS"):
                        info = tarfile.TarInfo("php-8.2.0/" + path)
                        info.size = 1
                        if path == "NEWS":
                            info.pax_headers = headers
                        tar.addfile(info, io.BytesIO(b"x"))
                data = output.getvalue()
                with self.assertRaisesRegex(ValueError, "sparse archive"):
                    snapshot(data, hashlib.sha256(data).hexdigest())

    def test_unknown_file_field_is_rejected(self):
        data, digest = archive_with_pax(20)
        old = snapshot(data, digest)
        new = json.loads(json.dumps(old))
        new["files"]["NEWS"]["note"] = True
        with self.assertRaises(ValueError):
            source.compare(old, new)
