"""Extract unverified archive candidates from a saved official museum index."""
import argparse
import hashlib
from html.parser import HTMLParser
import json
import re
from pathlib import Path
from urllib.parse import urljoin

MAX_INDEX_BYTES = 4 * 1024 * 1024
INDEX_URL = re.compile(r"https://museum\.php\.net/php(?:1|2|3|4|5|7|8)/\Z")
SAFE_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._+-]*\Z")
FILENAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9._+-]*\.(?:tar\.gz|tar\.bz2|tar\.xz|zip)\Z")


class IndexParser(HTMLParser):
    """Parse the museum table while rejecting malformed table structure."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.seen_table = False
        self.table_open = False
        self.body_open = False
        self.section = None
        self.row_open = False
        self.cell_open = False
        self.cell_tag = None
        self.link_cell = False
        self.anchor_open = False
        self.links = []

    @staticmethod
    def _attributes(attrs):
        names = [name.lower() for name, _ in attrs]
        if len(names) != len(set(names)):
            raise ValueError("duplicate HTML attribute")
        return dict(attrs)

    def handle_starttag(self, tag, attrs):
        attrs = self._attributes(attrs)
        if tag == "table":
            if self.table_open:
                raise ValueError("nested museum index table")
            if attrs.get("id") == "list":
                if self.seen_table:
                    raise ValueError("duplicate museum index table")
                self.seen_table = self.table_open = True
            return
        if not self.table_open:
            return
        if tag in ("thead", "tbody", "tfoot"):
            if self.section is not None or self.row_open or self.cell_open or self.anchor_open:
                raise ValueError("invalid nested table section")
            self.section = tag
            if tag == "tbody":
                self.body_open = True
            return
        if tag == "tr":
            if self.section not in ("thead", "tbody", "tfoot") or self.row_open or self.cell_open or self.anchor_open:
                raise ValueError("invalid table row")
            self.row_open = True
            return
        if tag in ("td", "th"):
            if not self.row_open or self.cell_open or self.anchor_open:
                raise ValueError("invalid table cell")
            self.cell_open = True
            self.cell_tag = tag
            self.link_cell = tag == "td" and "link" in attrs.get("class", "").split()
            return
        if tag == "a":
            if self.anchor_open or (self.body_open and not self.link_cell):
                raise ValueError("anchor outside museum link cell")
            self.anchor_open = True
            if self.body_open:
                self.links.append(attrs.get("href", ""))
            return

    def handle_startendtag(self, tag, attrs):
        if tag in ("table", "thead", "tbody", "tfoot", "tr", "td", "th", "a"):
            raise ValueError("self-closing structural HTML tag")
        self._attributes(attrs)

    def handle_endtag(self, tag):
        if tag == "table":
            if self.table_open:
                if self.section is not None or self.row_open or self.cell_open or self.anchor_open:
                    raise ValueError("incomplete museum index table")
                self.table_open = False
            return
        if not self.table_open:
            return
        if tag in ("thead", "tbody", "tfoot"):
            if self.section != tag or self.row_open or self.cell_open or self.anchor_open:
                raise ValueError("invalid table section close")
            if tag == "tbody":
                self.body_open = False
            self.section = None
        elif tag == "tr":
            if not self.row_open or self.cell_open or self.anchor_open:
                raise ValueError("invalid table row close")
            self.row_open = False
        elif tag in ("td", "th"):
            if not self.cell_open or self.cell_tag != tag or self.anchor_open:
                raise ValueError("invalid table cell close")
            self.cell_open = False
            self.cell_tag = None
            self.link_cell = False
        elif tag == "a":
            if not self.anchor_open:
                raise ValueError("invalid anchor close")
            self.anchor_open = False


def discover(page, index_url):
    if not isinstance(page, bytes) or len(page) > MAX_INDEX_BYTES:
        raise ValueError("index must be bytes and at most 4 MiB")
    if not isinstance(index_url, str) or not INDEX_URL.fullmatch(index_url):
        raise ValueError("index URL must be an official PHP museum generation directory")
    try:
        content = page.decode("utf-8", "strict")
    except UnicodeDecodeError as exc:
        raise ValueError("index is not UTF-8") from exc
    parser = IndexParser()
    parser.feed(content)
    parser.close()
    if (not parser.seen_table or parser.table_open or parser.body_open or parser.section is not None
            or parser.row_open or parser.cell_open or parser.anchor_open):
        raise ValueError("museum index table missing or incomplete")
    names = []
    other_names = []
    seen = set()
    for link in parser.links:
        if re.fullmatch(r"\.\./(?:\?C=[NSM]&O=[AD])?", link):
            continue
        if not SAFE_NAME.fullmatch(link):
            raise ValueError("unexpected index link: " + repr(link))
        if link in seen:
            raise ValueError("duplicate index link: " + link)
        seen.add(link)
        (names if FILENAME.fullmatch(link) else other_names).append(link)
    if not names:
        raise ValueError("museum index contains no archive candidates")
    return {"index_url": index_url, "source_sha256": hashlib.sha256(page).hexdigest(),
            "inventory_status": "incomplete", "candidates": [
                {"filename": name, "url": urljoin(index_url, name),
                 "identity_status": "unverified", "source_status": "not_retrieved"}
                for name in names],
            "other_files": [{"filename": name, "url": urljoin(index_url, name)}
                            for name in other_names]}


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="Example: python3 tools/history_museum.py --index-url https://museum.php.net/php2/ index.html")
    parser.add_argument("--index-url", required=True, help="official museum generation directory URL")
    parser.add_argument("index", type=Path, help="saved UTF-8 directory index, at most 4 MiB")
    args = parser.parse_args()
    try:
        with args.index.open("rb") as source:
            page = source.read(MAX_INDEX_BYTES + 1)
        result = discover(page, args.index_url)
    except (OSError, ValueError) as exc:
        parser.exit(2, f"error: cannot inspect museum index: {exc}\n")
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
