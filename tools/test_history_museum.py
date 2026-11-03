import hashlib
import unittest

from history_museum import discover


class MuseumDiscoveryTest(unittest.TestCase):
    def test_distinct_files_and_source_digest_are_preserved(self):
        page = b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php-2.0.tar.gz</a></td></tr><tr><td class="link"><a href="php-2.0b10.tar.gz">php-2.0b10.tar.gz</a></td></tr></tbody></table>'
        result = discover(page, "https://museum.php.net/php2/")
        self.assertEqual(result["source_sha256"], hashlib.sha256(page).hexdigest())
        self.assertEqual([x["filename"] for x in result["candidates"]], ["php-2.0.tar.gz", "php-2.0b10.tar.gz"])
        self.assertTrue(all(x["identity_status"] == "unverified" for x in result["candidates"]))
        self.assertEqual(result["inventory_status"], "incomplete")

    def test_rejects_changed_or_empty_index(self):
        for page in (b"", b"<html>server error</html>", b'<table id="list"><tbody></tbody></table>'):
            with self.subTest(page=page), self.assertRaises(ValueError):
                discover(page, "https://museum.php.net/php2/")

    def test_rejects_untrusted_index_url(self):
        page = b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php-2.0.tar.gz</a></td></tr></tbody></table>'
        for url in ("https://evil.example/php2/", "http://museum.php.net/php2/", "https://museum.php.net/php2/?x=1"):
            with self.subTest(url=url), self.assertRaises(ValueError):
                discover(page, url)

    def test_preserves_non_archive_files_separately(self):
        page = b'<table id="list"><tbody><tr><td class="link"><a href="../?C=N&amp;O=D">Parent</a></td></tr><tr><td class="link"><a href="php-2.0.tar.gz">php</a></td></tr><tr><td class="link"><a href="php-2.0.tar.gz.asc">signature</a></td></tr><tr><td class="link"><a href="php-2.0.diff.gz">patch</a></td></tr></tbody></table>'
        result = discover(page, "https://museum.php.net/php2/")
        self.assertEqual([x["filename"] for x in result["candidates"]], ["php-2.0.tar.gz"])
        self.assertEqual([x["filename"] for x in result["other_files"]], ["php-2.0.tar.gz.asc", "php-2.0.diff.gz"])

    def test_rejects_unexpected_archive_link(self):
        page = b'<table id="list"><tbody><tr><td class="link"><a href="https://evil.example/a.tar.gz">a.tar.gz</a></td></tr></tbody></table>'
        with self.assertRaises(ValueError):
            discover(page, "https://museum.php.net/php2/")


if __name__ == "__main__":
    unittest.main()
