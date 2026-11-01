#!/usr/bin/env python3
"""Classify what changed between two PHP release archives.

tools/upstream_snapshot.py records which parser, stub, test and release-note
files changed. This tool reads those files and says what the change is:
functions, classes, methods and constants added, removed or changed in the
stubs; tokens and grammar rules added or removed; tests added, removed or
changed; and the security fixes listed in NEWS. Each change names the work
packages it may affect. Security fixes are never judged here: every one is
reported as "needs_assessment" until a maintainer records evidence.

Usage:
  python3 tools/upstream_diff.py OLD.tar.xz NEW.tar.xz --old-version 8.5.11 --new-version 8.5.12
"""
import argparse
import io
import json
import lzma
import re
import sys
import tarfile

MAX_ARCHIVE = 100 * 1024 * 1024
MAX_FILE = 32 * 1024 * 1024

# Source path prefixes and NEWS sections mapped to the work packages that
# implement them. The longest matching prefix wins; unmatched paths map to
# "unmapped" so they are reviewed rather than dropped.
PATH_IMPACT = {
    "Zend/zend_language_parser.y": ["W-09", "W-11"],
    "Zend/zend_language_scanner.l": ["W-09"],
    "Zend/tests/": ["W-11", "W-17"],
    "Zend/": ["W-11", "W-17"],
    "ext/standard/": ["W-29"],
    "ext/pcre/": ["W-30"],
    "ext/mbstring/": ["W-30"],
    "ext/pdo": ["W-31"],
    "ext/mysqli/": ["W-31"],
    "ext/openssl/": ["W-32"],
    "ext/curl/": ["W-32"],
    "ext/dom/": ["W-33"],
    "ext/xml": ["W-33"],
    "ext/intl/": ["W-33"],
    "ext/zip/": ["W-33"],
    "ext/session/": ["W-28"],
    "sapi/fpm/": ["W-25"],
    "sapi/cli/": ["W-35"],
    "ext/": ["W-52"],
}
NEWS_IMPACT = {
    "core": ["W-11", "W-17"], "standard": ["W-29"], "pcre": ["W-30"], "mbstring": ["W-30"],
    "pdo": ["W-31"], "mysqli": ["W-31"], "mysqlnd": ["W-31"], "openssl": ["W-32"], "curl": ["W-32"],
    "dom": ["W-33"], "xml": ["W-33"], "intl": ["W-33"], "zip": ["W-33"], "session": ["W-28"],
    "fpm": ["W-25"], "cli": ["W-35"], "opcache": ["W-15"],
}


_STUB_MODIFIERS = frozenset({
    "public", "protected", "private", "static", "final", "abstract", "readonly", "var",
})
_CLASS_KEYWORDS = frozenset({"class", "interface", "trait", "enum"})
_NEWS_HEADER = re.compile(
    r"^\s*[0-9]{1,2}\s+[A-Za-z]{3,9}\s+[0-9]{4},\s+PHP\s+(\S+)\s*$"
)
_NEWS_ID = re.compile(
    r"GHSA-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}|CVE-[0-9]{4}-[0-9]{4,}"
)


def impact(path_or_section):
    """Return the sorted work packages a source path or NEWS section affects.

    A path is matched against PATH_IMPACT by longest prefix. A NEWS section
    name (for example "Core" or "PDO_MySQL") is matched case-insensitively
    against NEWS_IMPACT, first as a whole and then by its part before "_".
    Anything unmatched returns ["unmapped"].
    """
    if not isinstance(path_or_section, str):
        return ["unmapped"]
    prefixes = [prefix for prefix in PATH_IMPACT if path_or_section.startswith(prefix)]
    if prefixes:
        return sorted(PATH_IMPACT[max(prefixes, key=len)])
    section = path_or_section.lower()
    if section in NEWS_IMPACT:
        return sorted(NEWS_IMPACT[section])
    head = section.split("_", 1)[0]
    if head in NEWS_IMPACT:
        return sorted(NEWS_IMPACT[head])
    return ["unmapped"]


def _skip_quoted(text, index):
    quote = text[index]
    index += 1
    while index < len(text):
        char = text[index]
        if char == "\\":
            index += 2
            continue
        index += 1
        if char == quote:
            break
    return index


def _skip_attribute(text, index):
    index += 1
    depth = 0
    while index < len(text):
        char = text[index]
        if char in "'\"":
            index = _skip_quoted(text, index)
            continue
        if char == "[":
            depth += 1
        elif char == "]":
            depth -= 1
            if depth == 0:
                return index + 1
        index += 1
    return index


def _strip_metadata(text):
    out = []
    index = 0
    length = len(text)
    while index < length:
        char = text[index]
        if char in "'\"":
            start = index
            index = _skip_quoted(text, index)
            out.append(text[start:index])
            continue
        if char == "/" and index + 1 < length and text[index + 1] == "/":
            index += 2
            while index < length and text[index] not in "\r\n":
                index += 1
            continue
        if char == "/" and index + 1 < length and text[index + 1] == "*":
            index += 2
            while index + 1 < length and not (text[index] == "*" and text[index + 1] == "/"):
                index += 1
            index = min(index + 2, length)
            out.append(" ")
            continue
        if char == "#":
            if index + 1 < length and text[index + 1] == "[":
                index = _skip_attribute(text, index)
                out.append(" ")
                continue
            index += 1
            while index < length and text[index] not in "\r\n":
                index += 1
            continue
        out.append(char)
        index += 1
    return "".join(out)


def _tokenize(text):
    tokens = []
    index = 0
    length = len(text)
    while index < length:
        char = text[index]
        if char.isspace():
            index += 1
            continue
        if char in "'\"":
            start = index
            index = _skip_quoted(text, index)
            tokens.append(("string", text[start:index], start, index))
            continue
        if char.isalpha() or char in "_\\":
            start = index
            while index < length and (text[index].isalnum() or text[index] in "_\\"):
                index += 1
            tokens.append(("ident", text[start:index], start, index))
            continue
        tokens.append(("symbol", char, index, index + 1))
        index += 1
    return tokens


def _collapse(text):
    return re.sub(r"\s+", " ", text).strip()


def _qualify(namespace, name):
    if name.startswith("\\"):
        return name[1:]
    return namespace + "\\" + name if namespace else name


def _statement_start(tokens, index):
    cursor = index - 1
    while cursor >= 0 and tokens[cursor][0] == "ident" and tokens[cursor][1] in _STUB_MODIFIERS:
        cursor -= 1
    if cursor < 0:
        return 0
    if tokens[cursor][0] == "symbol" and tokens[cursor][1] in (";", "{", "}"):
        return cursor + 1
    return None


def _matching_brace(tokens, index):
    depth = 0
    for cursor in range(index, len(tokens)):
        if tokens[cursor][0] != "symbol":
            continue
        if tokens[cursor][1] == "{":
            depth += 1
        elif tokens[cursor][1] == "}":
            depth -= 1
            if depth == 0:
                return cursor
    return len(tokens) - 1


def parse_stubs(text):
    """Return {symbol: signature} for one .stub.php file.

    Symbols are "function NAME", "class NAME", "interface NAME", "trait NAME",
    "enum NAME", "method CLASS::NAME" and "const NAME" (global) or
    "const CLASS::NAME", with names fully qualified by the file's namespace
    declarations ("namespace X;" or "namespace X { ... }") and no leading
    backslash. The signature is the declaration with comments removed and
    runs of whitespace collapsed to one space, ending before the body or the
    semicolon, for example "function strlen(string $string): int". Doc
    comments and attributes do not count, so editing them is no change.
    Class signatures include extends and implements lists.
    """
    cleaned = _strip_metadata(text)
    tokens = _tokenize(cleaned)
    symbols = {}
    scopes = []
    base_namespace = ""
    depth = 0

    def namespace_now():
        return scopes[-1]["namespace"] if scopes else base_namespace

    def class_now():
        return scopes[-1]["class"] if scopes else None

    index = 0
    while index < len(tokens):
        kind, value, start, _ = tokens[index]
        if kind == "symbol":
            if value == "{":
                depth += 1
            elif value == "}":
                depth -= 1
                while scopes and scopes[-1]["depth"] > depth:
                    scopes.pop()
            index += 1
            continue
        if kind != "ident":
            index += 1
            continue

        if value == "namespace":
            cursor = index + 1
            names = []
            while cursor < len(tokens) and tokens[cursor][0] == "ident":
                names.append(tokens[cursor][1])
                cursor += 1
            if cursor >= len(tokens):
                break
            name = "".join(names).strip("\\")
            terminator = tokens[cursor][1]
            if terminator == ";":
                if scopes:
                    scopes[-1]["namespace"] = name
                else:
                    base_namespace = name
                index = cursor + 1
                continue
            if terminator == "{":
                depth += 1
                scopes.append({"depth": depth, "namespace": name, "class": None})
                index = cursor + 1
                continue
            index += 1
            continue

        if value in _CLASS_KEYWORDS and class_now() is None:
            declaration = _statement_start(tokens, index)
            if declaration is None:
                index += 1
                continue
            cursor = index + 1
            name = None
            while cursor < len(tokens) and tokens[cursor][1] not in ("{", ";"):
                if name is None and tokens[cursor][0] == "ident":
                    name = tokens[cursor][1]
                cursor += 1
            if cursor >= len(tokens) or name is None:
                break
            terminator = tokens[cursor]
            signature = _collapse(cleaned[tokens[declaration][2]:terminator[2]])
            qualified = _qualify(namespace_now(), name)
            symbols[f"{value} {qualified}"] = signature
            if terminator[1] == "{":
                depth += 1
                scopes.append({"depth": depth, "namespace": namespace_now(), "class": qualified})
                index = cursor + 1
            else:
                index = cursor + 1
            continue

        if value == "function":
            declaration = _statement_start(tokens, index)
            if declaration is None:
                index += 1
                continue
            cursor = index + 1
            if cursor < len(tokens) and tokens[cursor][1] == "&":
                cursor += 1
            if cursor >= len(tokens) or tokens[cursor][0] != "ident":
                index += 1
                continue
            name = tokens[cursor][1]
            cursor += 1
            while cursor < len(tokens) and tokens[cursor][1] not in ("{", ";"):
                cursor += 1
            if cursor >= len(tokens):
                break
            terminator = tokens[cursor]
            signature = _collapse(cleaned[tokens[declaration][2]:terminator[2]])
            current_class = class_now()
            if current_class is not None:
                symbols[f"method {current_class}::{name}"] = signature
            else:
                symbols[f"function {_qualify(namespace_now(), name)}"] = signature
            if terminator[1] == "{":
                index = _matching_brace(tokens, cursor) + 1
            else:
                index = cursor + 1
            continue

        if value == "const":
            declaration = _statement_start(tokens, index)
            if declaration is None:
                index += 1
                continue
            cursor = index + 1
            equals = None
            while cursor < len(tokens) and tokens[cursor][1] != ";":
                if equals is None and tokens[cursor][0] == "symbol" and tokens[cursor][1] == "=":
                    equals = cursor
                cursor += 1
            if cursor >= len(tokens):
                break
            name = None
            if equals is not None:
                for back in range(equals - 1, index, -1):
                    if tokens[back][0] == "ident":
                        name = tokens[back][1]
                        break
            else:
                for ahead in range(index + 1, cursor):
                    if tokens[ahead][0] == "ident":
                        name = tokens[ahead][1]
                        break
            if name is None:
                index = cursor + 1
                continue
            signature = _collapse(cleaned[tokens[declaration][2]:tokens[cursor][2]])
            current_class = class_now()
            if current_class is not None:
                symbols[f"const {current_class}::{name}"] = signature
            else:
                symbols[f"const {_qualify(namespace_now(), name)}"] = signature
            index = cursor + 1
            continue

        index += 1

    return symbols


def grammar_symbols(parser_y):
    """Return {"tokens": [...], "rules": [...]} for zend_language_parser.y.

    Tokens are the names declared by %token lines (T_STRING, T_ECHO, ...);
    rules are the nonterminals defined in the grammar section between the
    first and second "%%" lines, each "name:" at the start of a line. Both
    lists are sorted and contain no duplicates.
    """
    lines = parser_y.splitlines()
    tokens = set()
    for line in lines:
        stripped = line.lstrip()
        if not stripped.startswith("%token"):
            continue
        rest = stripped[len("%token"):]
        rest = re.sub(r"<[^>]*>", " ", rest)
        rest = re.sub(r'"[^"]*"', " ", rest)
        tokens.update(re.findall(r"[A-Za-z_][A-Za-z0-9_]*", rest))
    rules = set()
    in_grammar = False
    separators = 0
    for line in lines:
        if line.strip() == "%%":
            separators += 1
            in_grammar = separators == 1
            continue
        if in_grammar:
            match = re.match(r"([A-Za-z_][A-Za-z0-9_]*)\s*:", line)
            if match:
                rules.add(match.group(1))
    return {"tokens": sorted(tokens), "rules": sorted(rules)}


def news_entries(news, version):
    """Return the NEWS entries of one release.

    The block starts at the line "<date>, PHP <version>" and ends at the next
    such header. Each "- Section:" line starts a section and each ". text"
    line starts an entry; continuation lines are joined with one space.
    Returns a list of {"section", "text", "security", "ids"} in file order.
    ids lists every GHSA-xxxx-xxxx-xxxx and CVE-YYYY-NNNN+ identifier in the
    entry, in order of appearance without duplicates; security is True when
    ids is not empty. A version missing from NEWS raises ValueError.
    """
    lines = news.splitlines()
    headers = []
    for index, line in enumerate(lines):
        match = _NEWS_HEADER.match(line)
        if match:
            headers.append((index, match.group(1)))
    start = None
    end = len(lines)
    for position, (index, found) in enumerate(headers):
        if found == version:
            start = index
            end = headers[position + 1][0] if position + 1 < len(headers) else len(lines)
            break
    if start is None:
        raise ValueError(f"version {version} not found in NEWS")

    raw_entries = []
    section = None
    current = None
    for line in lines[start + 1:end]:
        section_match = re.match(r"^\s*-\s*([^:]+):\s*$", line)
        if section_match:
            if current is not None:
                raw_entries.append(current)
                current = None
            section = section_match.group(1).strip()
            continue
        entry_match = re.match(r"^\s*\.\s*(.*)$", line)
        if entry_match:
            if current is not None:
                raw_entries.append(current)
            current = {"section": section, "chunks": [entry_match.group(1).strip()]}
            continue
        if current is not None:
            stripped = line.strip()
            if stripped:
                current["chunks"].append(stripped)
    if current is not None:
        raw_entries.append(current)

    entries = []
    for item in raw_entries:
        text = " ".join(chunk for chunk in item["chunks"] if chunk)
        ids = list(dict.fromkeys(_NEWS_ID.findall(text)))
        entries.append({
            "section": item["section"],
            "text": text,
            "security": bool(ids),
            "ids": ids,
        })
    return entries


def _safe_path(path):
    return (isinstance(path, str) and bool(path) and not path.startswith("/")
            and "\\" not in path and all(part not in ("", ".", "..") for part in path.split("/")))


def _wanted_path(path):
    return (path in ("Zend/zend_language_parser.y", "Zend/zend_language_scanner.l", "NEWS")
            or path.endswith(".stub.php") or path.endswith(".phpt"))


def read_archive(data, version):
    """Return {path: text} for the parser, stub, test and NEWS files.

    Paths are relative to the archive's "php-<version>/" root. Only
    Zend/zend_language_parser.y, Zend/zend_language_scanner.l, *.stub.php,
    *.phpt and NEWS are read; text is decoded as UTF-8 with replacement.
    Archives larger than MAX_ARCHIVE, members larger than MAX_FILE, paths
    outside the root, absolute paths and ".." components raise ValueError.
    """
    if not isinstance(data, (bytes, bytearray)):
        raise ValueError("archive must be bytes")
    if len(data) > MAX_ARCHIVE:
        raise ValueError("archive too large")
    root = "php-" + version
    files = {}
    seen = set()
    try:
        with tarfile.open(fileobj=io.BytesIO(data), mode="r|*") as stream:
            for member in stream:
                name = member.name.rstrip("/") if member.isdir() else member.name
                if not _safe_path(name) or not (name == root or name.startswith(root + "/")):
                    raise ValueError("invalid archive path")
                if name in seen:
                    raise ValueError("duplicate archive path")
                seen.add(name)
                if member.isdir():
                    continue
                if not member.isfile() or name == root:
                    raise ValueError("non-regular archive entry")
                if member.size < 0 or member.size > MAX_FILE:
                    raise ValueError("archive member too large")
                path = name[len(root) + 1:]
                if not _wanted_path(path):
                    continue
                handle = stream.extractfile(member)
                if handle is None:
                    raise ValueError("cannot read archive entry")
                raw = handle.read(MAX_FILE + 1)
                if len(raw) != member.size:
                    raise ValueError("truncated archive entry")
                files[path] = raw.decode("utf-8", errors="replace")
    except (tarfile.TarError, EOFError, OSError, TypeError, lzma.LZMAError) as exc:
        raise ValueError("invalid archive") from exc
    return files


def _stub_map(files):
    stubs = {}
    for path in sorted(files):
        if path.endswith(".stub.php"):
            for symbol, signature in parse_stubs(files[path]).items():
                stubs[symbol] = (path, signature)
    return stubs


def diff_releases(old_files, new_files, new_version):
    """Return the classified change report between two read archives.

    old_files and new_files are read_archive results. The report is
    {"schema_version": 1, "new_version", "stubs": {"added", "removed",
    "changed"}, "grammar": {"tokens_added", "tokens_removed", "rules_added",
    "rules_removed"}, "tests": {"added", "removed", "modified"}, "security":
    [...], "affected_work_packages": [...]}.

    Stub entries are {"symbol", "file", "old", "new", "work_packages"} (old
    or new is None for additions and removals), sorted by symbol. Test
    entries are {"path", "work_packages"} sorted by path. Security entries
    are the new release's security NEWS entries with "work_packages" and
    "assessment": "needs_assessment". affected_work_packages is the sorted
    union of every work package above, including "unmapped" when present.
    """
    old_stubs = _stub_map(old_files)
    new_stubs = _stub_map(new_files)
    added, removed, changed = [], [], []
    for symbol in sorted(set(old_stubs) | set(new_stubs)):
        before = old_stubs.get(symbol)
        after = new_stubs.get(symbol)
        if before is None:
            path, signature = after
            added.append({"symbol": symbol, "file": path, "old": None, "new": signature,
                          "work_packages": impact(path)})
        elif after is None:
            path, signature = before
            removed.append({"symbol": symbol, "file": path, "old": signature, "new": None,
                            "work_packages": impact(path)})
        elif before[1] != after[1]:
            changed.append({"symbol": symbol, "file": after[0], "old": before[1],
                            "new": after[1], "work_packages": impact(after[0])})

    old_grammar = grammar_symbols(old_files.get("Zend/zend_language_parser.y", ""))
    new_grammar = grammar_symbols(new_files.get("Zend/zend_language_parser.y", ""))
    old_tokens, new_tokens = set(old_grammar["tokens"]), set(new_grammar["tokens"])
    old_rules, new_rules = set(old_grammar["rules"]), set(new_grammar["rules"])
    grammar = {
        "tokens_added": sorted(new_tokens - old_tokens),
        "tokens_removed": sorted(old_tokens - new_tokens),
        "rules_added": sorted(new_rules - old_rules),
        "rules_removed": sorted(old_rules - new_rules),
    }

    old_tests = {path: text for path, text in old_files.items() if path.endswith(".phpt")}
    new_tests = {path: text for path, text in new_files.items() if path.endswith(".phpt")}
    tests_added, tests_removed, tests_modified = [], [], []
    for path in sorted(set(old_tests) | set(new_tests)):
        entry = {"path": path, "work_packages": impact(path)}
        before, after = old_tests.get(path), new_tests.get(path)
        if before is None:
            tests_added.append(entry)
        elif after is None:
            tests_removed.append(entry)
        elif before != after:
            tests_modified.append(entry)

    security = []
    if "NEWS" in new_files:
        for entry in news_entries(new_files["NEWS"], new_version):
            if not entry["security"]:
                continue
            item = dict(entry)
            item["work_packages"] = impact(entry["section"])
            item["assessment"] = "needs_assessment"
            security.append(item)

    affected = set()
    for group in (added, removed, changed, tests_added, tests_removed, tests_modified):
        for entry in group:
            affected.update(entry["work_packages"])
    for entry in security:
        affected.update(entry["work_packages"])
    for path in ("Zend/zend_language_parser.y", "Zend/zend_language_scanner.l"):
        if old_files.get(path) != new_files.get(path):
            affected.update(impact(path))

    return {
        "schema_version": 1,
        "new_version": new_version,
        "stubs": {"added": added, "removed": removed, "changed": changed},
        "grammar": grammar,
        "tests": {"added": tests_added, "removed": tests_removed, "modified": tests_modified},
        "security": security,
        "affected_work_packages": sorted(affected),
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0],
                                     epilog="Example: python3 tools/upstream_diff.py php-8.5.11.tar.xz php-8.5.12.tar.xz --old-version 8.5.11 --new-version 8.5.12")
    parser.add_argument("old_archive")
    parser.add_argument("new_archive")
    parser.add_argument("--old-version", required=True)
    parser.add_argument("--new-version", required=True)
    args = parser.parse_args(argv)
    try:
        with open(args.old_archive, "rb") as f:
            old = read_archive(f.read(MAX_ARCHIVE + 1), args.old_version)
        with open(args.new_archive, "rb") as f:
            new = read_archive(f.read(MAX_ARCHIVE + 1), args.new_version)
        report = diff_releases(old, new, args.new_version)
    except (OSError, ValueError) as exc:
        print(f"upstream_diff: {exc}", file=sys.stderr)
        return 2
    json.dump(report, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
