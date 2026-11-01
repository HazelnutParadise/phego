import io
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import frontend_corpus


def archive(directory, members):
    path = Path(directory, "php.tar.xz")
    with tarfile.open(path, "w:xz") as tar:
        for name, data in members.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    return path


class CorpusTests(unittest.TestCase):
    def test_rejects_unlocked_archive(self):
        with tempfile.TemporaryDirectory() as d:
            path = archive(d, {"php-8.5.12/Zend/tests/a.phpt": b"--FILE--\n<?php\n--EXPECT--\n"})
            with self.assertRaises(ValueError):
                frontend_corpus.extract(path, Path(d, "out"))

    def test_extracts_only_file_sections_from_selected_tests(self):
        with tempfile.TemporaryDirectory() as d:
            path = archive(d, {
                "php-8.5.12/Zend/tests/a.phpt": b"--TEST--\nx\n--FILE--\n<?php echo 1;\n--EXPECT--\n1\n",
                "php-8.5.12/ext/standard/tests/b.phpt": b"--FILE--\n<?php\n--EXPECT--\n",
                "php-8.5.12/Zend/tests/c.phpt": b"--TEST--\nno file\n",
            })
            import hashlib
            with mock.patch.object(frontend_corpus, "ARCHIVE_SHA256", hashlib.sha256(path.read_bytes()).hexdigest()):
                count, digest = frontend_corpus.extract(path, Path(d, "out"))
            files = list(Path(d, "out", "corpus").iterdir())
            self.assertEqual((count, len(files)), (1, 1))
            self.assertEqual(files[0].read_bytes(), b"<?php echo 1;\n")
            self.assertEqual(len(digest), 64)
            self.assertEqual(frontend_corpus.main([str(path), str(Path(d, "out"))]), 2)


if __name__ == "__main__":
    unittest.main()
