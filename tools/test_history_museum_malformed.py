import unittest

from history_museum import discover

INDEX_URL = "https://museum.php.net/php2/"


class MalformedMuseumIndexTest(unittest.TestCase):
    def test_rejects_duplicate_href_attributes(self):
        pages = (
            b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz" href="php-9.9.tar.gz">php</a></td></tr></tbody></table>',
            b'<table id="list"><tbody><tr><td class="link"><a href="php-9.9.tar.gz" href="php-2.0.tar.gz">php</a></td></tr></tbody></table>',
        )
        for page in pages:
            with self.subTest(page=page), self.assertRaises(ValueError):
                discover(page, INDEX_URL)

    def test_rejects_missing_cell_and_anchor_end_tags(self):
        pages = (
            b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php</a></tr></tbody></table>',
            b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php</td></tr></tbody></table>',
        )
        for page in pages:
            with self.subTest(page=page), self.assertRaises(ValueError):
                discover(page, INDEX_URL)

    def test_rejects_missing_row_and_body_end_tags(self):
        pages = (
            b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php</a></td></tbody></table>',
            b'<table id="list"><tbody><tr><td class="link"><a href="php-2.0.tar.gz">php</a></td></tr></table>',
        )
        for page in pages:
            with self.subTest(page=page), self.assertRaises(ValueError):
                discover(page, INDEX_URL)

    def test_rejects_nested_table_in_list_body(self):
        page = (
            b'<table id="list"><tbody>'
            b'<tr><td class="link"><a href="php-1.0.tar.gz">php1</a></td></tr>'
            b'<tr><td><table><tr><td class="link"><a href="php-2.0.tar.gz">php2</a></td></tr></table></td></tr>'
            b'<tr><td class="link"><a href="php-3.0.tar.gz">php3</a></td></tr>'
            b'</tbody></table>'
        )
        with self.assertRaises(ValueError):
            discover(page, INDEX_URL)

    def test_rejects_anchor_outside_link_cell(self):
        page = (
            b'<table id="list"><tbody>'
            b'<tr><td class="link"><a href="php-2.0.tar.gz">php2</a></td></tr>'
            b'<tr><a href="php-3.0.tar.gz">php3</a></tr>'
            b'</tbody></table>'
        )
        with self.assertRaises(ValueError):
            discover(page, INDEX_URL)

    def test_rejects_mismatched_cell_closing_tags(self):
        for cell in (b'<td class="link"><a href="php-2.0.tar.gz">php</a></th>',
                     b'<th>header</td><td class="link"><a href="php-2.0.tar.gz">php</a></td>'):
            page = b'<table id="list"><tbody><tr>' + cell + b'</tr></tbody></table>'
            with self.subTest(cell=cell), self.assertRaises(ValueError):
                discover(page, INDEX_URL)

    def test_accepts_normal_museum_index(self):
        page = (
            b'<!DOCTYPE html><html><head><title>Index of /php2</title></head><body>'
            b'<table id="list"><thead><tr><th>Name</th><th>Last modified</th></tr></thead><tbody>'
            b'<tr><td class="link"><a href="../?C=N&amp;O=D">Parent Directory</a></td><td>2024-01-01</td></tr>'
            b'<tr><td class="link"><a href="php-2.0.tar.gz">php-2.0.tar.gz</a></td><td>1997-01-01</td></tr>'
            b'<tr><td class="link"><a href="php-2.0b10.tar.gz">php-2.0b10.tar.gz</a></td><td>1997-01-01</td></tr>'
            b'<tr><td class="link"><a href="php-2.0.tar.gz.asc">php-2.0.tar.gz.asc</a></td><td>1997-01-01</td></tr>'
            b'</tbody></table></body></html>'
        )
        result = discover(page, INDEX_URL)
        self.assertEqual(
            [item["filename"] for item in result["candidates"]],
            ["php-2.0.tar.gz", "php-2.0b10.tar.gz"],
        )
        self.assertEqual(
            [item["filename"] for item in result["other_files"]],
            ["php-2.0.tar.gz.asc"],
        )


if __name__ == "__main__":
    unittest.main()
