"""Extract the PHP 8.5.12 lexer corpus from the locked php-src archive.

Writes one file per --FILE-- section of Zend/tests and ext/tokenizer/tests into
OUTPUT/corpus. The corpus is PHP-licensed test material and stays outside the
repository; only its digest is recorded.
"""
import argparse
import hashlib
import re
import sys
import tarfile
from pathlib import Path

ARCHIVE_SHA256 = "08bc2eba2a588654ade1357174aa646b761556c862dda0678345ee29ed481209"
PREFIXES = ("php-8.5.12/Zend/tests/", "php-8.5.12/ext/tokenizer/tests/")
MAX_SOURCE = 200000
FILE_SECTION = re.compile(rb"^--FILE--\r?\n(.*?)^--[A-Z_]+--\r?$", re.S | re.M)


def extract(archive, output):
    digest = hashlib.sha256(Path(archive).read_bytes()).hexdigest()
    if digest != ARCHIVE_SHA256:
        raise ValueError(f"archive sha256 {digest} does not match the locked PHP 8.5.12 source")
    corpus = Path(output) / "corpus"
    corpus.mkdir(parents=True, exist_ok=False)
    names = []
    with tarfile.open(archive, "r:xz") as tar:
        for member in tar:
            if not member.isfile() or not member.name.endswith(".phpt") or not member.name.startswith(PREFIXES):
                continue
            match = FILE_SECTION.search(tar.extractfile(member).read())
            if not match or len(match.group(1)) > MAX_SOURCE:
                continue
            name = hashlib.sha256(member.name.encode()).hexdigest()[:16] + ".php"
            (corpus / name).write_bytes(match.group(1))
            names.append(name)
    combined = hashlib.sha256()
    for name in sorted(names):
        combined.update(name.encode() + b"\0" + hashlib.sha256((corpus / name).read_bytes()).digest())
    return len(names), combined.hexdigest()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0],
                                     epilog="Example: python3 tools/frontend_corpus.py php-8.5.12.tar.xz /tmp/lexer-corpus")
    parser.add_argument("archive", help="locked php-8.5.12.tar.xz")
    parser.add_argument("output", help="new directory to create the corpus in")
    args = parser.parse_args(argv)
    try:
        count, digest = extract(args.archive, args.output)
    except (OSError, ValueError, tarfile.TarError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    print(f"{count} files, corpus sha256 {digest}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
