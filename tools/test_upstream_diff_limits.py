import bz2
import gzip
import hashlib
import io
import lzma
import tarfile
import unittest
from unittest import mock

import tools.upstream_diff as upstream_diff
import tools.upstream_snapshot as upstream_snapshot


VERSION = "8.5.12"
NEWS = b"PHP NEWS\n\n22 Oct 2026, PHP 8.5.12\n"


def _tar_bytes():
    files = {
        "NEWS": NEWS,
        "Zend/zend_language_parser.y": b"%token T_STRING\n",
        "Zend/tests/limits.phpt": b"--TEST--\nlimits\n",
        "ext/limits/limits.stub.php": b"<?php\nfunction limits_test(): void;\n",
    }
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w") as archive:
        for path, content in files.items():
            member = tarfile.TarInfo(f"php-{VERSION}/{path}")
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
    return buffer.getvalue()


def _compress_xz(data, dictionary_size=8 * 1024 * 1024):
    return lzma.compress(
        data,
        format=lzma.FORMAT_XZ,
        filters=[{"id": lzma.FILTER_LZMA2, "dict_size": dictionary_size}],
    )


def _compress_alone(data, dictionary_size=8 * 1024 * 1024):
    return lzma.compress(
        data,
        format=lzma.FORMAT_ALONE,
        filters=[{"id": lzma.FILTER_LZMA1, "dict_size": dictionary_size}],
    )


def _capture(archive):
    return upstream_snapshot.capture(
        archive,
        hashlib.sha256(archive).hexdigest(),
        VERSION,
        "php-" + VERSION,
        "0" * 40,
        "https://www.php.net/releases/" + VERSION + ".php",
    )


class TestUpstreamDecoderLimits(unittest.TestCase):
    def test_read_archive_rejects_xz_dictionary_over_memory_cap(self):
        archive = _compress_xz(_tar_bytes())
        with mock.patch.object(upstream_snapshot, "MAX_LZMA_MEMORY", 4 * 1024 * 1024, create=True):
            with self.assertRaises(ValueError):
                upstream_diff.read_archive(archive, VERSION)

    def test_snapshot_capture_rejects_xz_dictionary_over_memory_cap(self):
        archive = _compress_xz(_tar_bytes())
        with mock.patch.object(upstream_snapshot, "MAX_LZMA_MEMORY", 4 * 1024 * 1024, create=True):
            with self.assertRaises(ValueError):
                _capture(archive)

    def test_lzma_alone_dictionary_over_memory_cap_is_rejected(self):
        archive = _compress_alone(_tar_bytes())
        with mock.patch.object(upstream_snapshot, "MAX_LZMA_MEMORY", 4 * 1024 * 1024, create=True):
            for operation in (
                lambda: upstream_diff.read_archive(archive, VERSION),
                lambda: _capture(archive),
            ):
                with self.subTest(operation=operation):
                    with self.assertRaises(ValueError):
                        operation()

    def test_valid_xz_archive_remains_readable(self):
        archive = _compress_xz(_tar_bytes(), dictionary_size=1024 * 1024)
        self.assertEqual(upstream_diff.read_archive(archive, VERSION)["NEWS"], NEWS.decode())
        snapshot = _capture(archive)
        self.assertIn("NEWS", snapshot["files"])

    def test_tar_gzip_bz2_xz_and_legacy_lzma_share_news(self):
        raw = _tar_bytes()
        archives = {
            "tar": raw,
            "gzip": gzip.compress(raw, mtime=0),
            "bz2": bz2.compress(raw),
            "xz": _compress_xz(raw, dictionary_size=1024 * 1024),
            "legacy-lzma": _compress_alone(raw, dictionary_size=8 * 1024 * 1024),
        }
        for kind, archive in archives.items():
            with self.subTest(kind=kind):
                self.assertEqual(upstream_diff.read_archive(archive, VERSION)["NEWS"], NEWS.decode())
                self.assertIn("NEWS", _capture(archive)["files"])

    def test_truncated_xz_footer_and_trailing_garbage_are_rejected(self):
        valid_xz = _compress_xz(_tar_bytes(), dictionary_size=1024 * 1024)
        invalid_archives = {
            "truncated-footer": valid_xz[:-1],
            "trailing-garbage": valid_xz + b"not-xz",
        }
        for kind, archive in invalid_archives.items():
            for operation in (
                lambda: upstream_diff.read_archive(archive, VERSION),
                lambda: _capture(archive),
            ):
                with self.subTest(kind=kind, operation=operation):
                    with self.assertRaises(ValueError):
                        operation()

    def test_nested_xz_is_not_decompressed_twice(self):
        nested = _compress_xz(_compress_xz(_tar_bytes(), dictionary_size=1024 * 1024),
                              dictionary_size=1024 * 1024)
        with self.assertRaises(ValueError):
            upstream_diff.read_archive(nested, VERSION)
        with self.assertRaises(ValueError):
            _capture(nested)

    def test_concatenated_xz_streams_with_padding_preserve_tar_boundaries(self):
        raw = _tar_bytes()
        archive = (_compress_xz(raw[:512], dictionary_size=1024 * 1024)
                   + b"\0" * 4
                   + _compress_xz(raw[512:], dictionary_size=1024 * 1024))
        self.assertEqual(upstream_diff.read_archive(archive, VERSION)["NEWS"], NEWS.decode())
        self.assertIn("NEWS", _capture(archive)["files"])


    def test_member_padding_remains_within_archive_budget(self):
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            for index in range(100):
                member = tarfile.TarInfo(f"php-{VERSION}/ignored-{index}.bin")
                member.size = 1
                archive.addfile(member, io.BytesIO(b"x"))
        archive = _compress_xz(buffer.getvalue(), dictionary_size=1024 * 1024)
        with mock.patch.object(upstream_diff, "MAX_UNPACKED", 100), \
                mock.patch.object(upstream_diff, "MAX_MEMBERS", 100), \
                mock.patch.object(upstream_snapshot, "MAX_METADATA", 0), \
                mock.patch.object(upstream_snapshot, "MAX_METADATA_HEADERS", 0):
            self.assertEqual(upstream_diff.read_archive(archive, VERSION), {})


class TestUpstreamStubLimits(unittest.TestCase):
    def test_condition_depth_accepts_128_and_rejects_129(self):
        with mock.patch.object(upstream_diff, "MAX_CONDITION_DEPTH", 128, create=True):
            accepted = "#if A\n" * 128 + "function bounded(): void;\n" + "#endif\n" * 128
            rejected = "#if A\n" * 129 + "function too_deep(): void;\n" + "#endif\n" * 129
            self.assertIn("function bounded", upstream_diff.parse_stubs(accepted))
            with self.assertRaises(ValueError):
                upstream_diff.parse_stubs(rejected)

    def test_condition_bytes_bound_if_expression_and_growing_elif_chain(self):
        with mock.patch.object(upstream_diff, "MAX_CONDITION_BYTES", 64, create=True):
            long_if = "#if " + "x" * 70 + "\nfunction long_if(): void;\n#endif\n"
            growing_elif = (
                "#if A\n"
                + "".join("#elif " + (letter * 20) + "\n" for letter in "BCDE")
                + "function long_elif(): void;\n#endif\n"
            )
            for text in (long_if, growing_elif):
                with self.subTest(text=text[:16]):
                    with self.assertRaises(ValueError):
                        upstream_diff.parse_stubs(text)

    def test_signature_bytes_bound_distinct_and_repeated_symbols(self):
        def declaration(name, size=70):
            return f"function {name}(string $value = '{'x' * size}'): void;\n"

        with mock.patch.object(upstream_diff, "MAX_SIGNATURE_BYTES", 128, create=True):
            distinct = "<?php\n" + declaration("alpha") + declaration("beta")
            repeated = (
                "<?php\n"
                + "#if FEATURE_A\n" + declaration("repeated") + "#endif\n"
                + "#if FEATURE_B\n" + declaration("repeated") + "#endif\n"
            )
            for text in (distinct, repeated):
                with self.subTest(text=text[:32]):
                    with self.assertRaises(ValueError):
                        upstream_diff.parse_stubs(text)
            ordinary_conditional = "#if FEATURE\nfunction conditional(): void;\n#endif\n"
            self.assertEqual(
                upstream_diff.parse_stubs(ordinary_conditional),
                {"function conditional": "[if FEATURE] function conditional(): void"},
            )

    def test_diff_releases_bounds_aggregate_stub_bytes_across_files(self):
        first = "<?php\n" + (
            "function first(string $value = '" + "a" * 65 + "'): void;\n"
        )
        second = "<?php\n" + (
            "function second(string $value = '" + "b" * 65 + "'): void;\n"
        )
        self.assertLess(len(first.encode()), 128)
        self.assertLess(len(second.encode()), 128)
        new_files = {
            "ext/first/first.stub.php": first,
            "ext/second/second.stub.php": second,
        }
        with mock.patch.object(upstream_diff, "MAX_STUB_BYTES", 256, create=True):
            for path, text in new_files.items():
                self.assertEqual(len(upstream_diff.diff_releases({}, {path: text}, VERSION)["stubs"]["added"]), 1)
            with self.assertRaises(ValueError):
                upstream_diff.diff_releases({}, new_files, VERSION)


if __name__ == "__main__":
    unittest.main()
