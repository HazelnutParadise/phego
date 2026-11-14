#!/usr/bin/env python3
import contextlib
import io
import json
import os
import tarfile
import tempfile
import unittest
from unittest import mock

import tools.upstream_diff as upstream_diff
import tools.upstream_snapshot as upstream_snapshot


NEWS = """PHP                                                                        NEWS
|||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||||
22 Oct 2026, PHP 8.5.12

- Core:
  . Fixed GHSA-w8hw-w9fx-j79w, GHSA-6wmf-4rv5-p3rp (Avoid state corruption via
    bailout). (CVE-2026-109618, CVE-2026-109619) (Arnaud)
  . Fixed bug GH-23644 (Optimizer crash). (ndossche)
  . Fixed GHSA-w8hw-w9fx-j79w again, GHSA-w8hw-w9fx-j79w. (CVE-2026-109619, CVE-2026-109619)

- PDO_MySQL:
  . Fixed GHSA-aaaa-bbbb-cccc (quoting). (CVE-2026-12345)

24 Sep 2026, PHP 8.5.11

- Core:
  . Fixed bug GH-1 (old). (x)
"""


class TestImpact(unittest.TestCase):
    def test_news_and_source_paths_have_consistent_impact(self):
        for section in upstream_diff.NEWS_IMPACT:
            prefix = {"core": "Zend/", "fpm": "sapi/fpm/", "cli": "sapi/cli/"}.get(
                section, f"ext/{section}/")
            with self.subTest(section=section):
                self.assertEqual(upstream_diff.impact(prefix + "x.c"),
                                 upstream_diff.impact(section))

    def test_longest_prefix_wins(self):
        self.assertEqual(upstream_diff.impact("Zend/zend_language_parser.y"), ["W-09", "W-11"])
        self.assertEqual(upstream_diff.impact("Zend/zend_language_scanner.l"), ["W-09"])
        self.assertEqual(upstream_diff.impact("Zend/tests/basic/001.phpt"), ["W-11", "W-17"])
        self.assertEqual(upstream_diff.impact("Zend/zend_compile.c"), ["W-11", "W-17"])
        self.assertEqual(upstream_diff.impact("ext/pdo_mysql/pdo_mysql.c"), ["W-31"])
        self.assertEqual(upstream_diff.impact("ext/pdo/pdo.c"), ["W-31"])
        self.assertEqual(upstream_diff.impact("ext/xmlreader/reader.c"), ["W-33"])
        self.assertEqual(upstream_diff.impact("ext/standard/string.c"), ["W-29"])
        self.assertEqual(upstream_diff.impact("sapi/fpm/fpm/fpm.c"), ["W-25"])
        self.assertEqual(upstream_diff.impact("sapi/cli/php_cli.c"), ["W-35"])
        self.assertEqual(upstream_diff.impact("ext/foo/foo.c"), ["W-52"])

    def test_news_sections_are_case_insensitive(self):
        for section in ("Core", "core", "CORE"):
            with self.subTest(section=section):
                self.assertEqual(upstream_diff.impact(section), ["W-11", "W-17"])
        self.assertEqual(upstream_diff.impact("PDO_MySQL"), ["W-31"])
        self.assertEqual(upstream_diff.impact("pdo_mysql"), ["W-31"])
        self.assertEqual(upstream_diff.impact("Pdo_MySql"), ["W-31"])
        self.assertEqual(upstream_diff.impact("MYSQLND"), ["W-31"])
        self.assertEqual(upstream_diff.impact("Standard"), ["W-29"])
        self.assertEqual(upstream_diff.impact("Opcache"), ["W-15"])
        self.assertEqual(upstream_diff.impact("FPM"), ["W-25"])
        self.assertEqual(upstream_diff.impact("CLI"), ["W-35"])

    def test_unmatched_paths_and_sections_are_unmapped(self):
        for value in ("sapi/apache2handler/sapi_apache2.c", "engineering/README.md", "SAPI", "Unknown"):
            with self.subTest(value=value):
                self.assertEqual(upstream_diff.impact(value), ["unmapped"])

    def test_results_are_sorted(self):
        for value in ("Zend/zend_language_parser.y", "Zend/tests/basic/001.phpt",
                      "ext/standard/string.c", "Core", "PDO_MySQL", "unknown/path.c"):
            with self.subTest(value=value):
                result = upstream_diff.impact(value)
                self.assertEqual(result, sorted(result))


class TestParseStubs(unittest.TestCase):
    def test_doc_comments_keep_token_separation_inside_declarations(self):
        source = """<?php
function/** @readonly */f(): void {}
class C { public const X = 1/** @virtual */+2; }
"""
        self.assertEqual(upstream_diff.parse_stubs(source), {
            'function f': 'function f(): void',
            'class C': 'class C', 'const C::X': 'public const X = 1 +2',
        })

    def test_ignored_doc_comments_do_not_consume_tokens(self):
        source = '<?php\n/** plain */ /** @refcount 1 */ function f(): void {}\n'
        with mock.patch.object(upstream_diff, 'MAX_STUB_TOKENS', 9):
            self.assertEqual(upstream_diff.parse_stubs(source),
                             {'function f': 'function f(): void'})

    def test_unlisted_doc_comments_inside_declarations_are_ignored(self):
        source = """<?php
function /** normal */ f(/** @var ignored */ int $x): void {}
class C { public const X = /** @cvalue ignored */ 1; }
"""
        expected = """<?php
function f( int $x): void {}
class C { public const X = 1; }
"""
        self.assertEqual(upstream_diff.parse_stubs(source), upstream_diff.parse_stubs(expected))
        self.assertIn('function f', upstream_diff.parse_stubs(source))

    def test_enum_case_attributes_and_whitelisted_doc_tags_are_retained(self):
        source = """<?php
enum E {
    /** @readonly */
    #[A(1)]
    case X;
}
"""
        self.assertEqual(upstream_diff.parse_stubs(source), {
            'enum E': 'enum E', 'case E::X': '[@readonly] #[A(1)] case X',
        })

    def test_parameter_attribute_content_changes_without_spacing_artifact(self):
        source = '<?php\nfunction f(#[A(1)] string $value): void {}\n'
        expected = 'function f(#[A(1)] string $value): void'
        self.assertEqual(upstream_diff.parse_stubs(source), {'function f': expected})
        changed = source.replace('A(1)', 'A(2)')
        report = upstream_diff.diff_releases({'ext/demo/x.stub.php': source},
                                            {'ext/demo/x.stub.php': changed}, '8.5.12')
        self.assertEqual(len(report['stubs']['changed']), 1)
        self.assertEqual(report['stubs']['changed'][0]['new'], expected.replace('A(1)', 'A(2)'))

    def test_doc_tag_names_are_exact_and_apply_to_class_property_and_constant(self):
        source = """<?php
/** @strict-properties */
class C {
    /** @readonly */ public string $name;
    /** @virtual */ public const X = 1;
    /** @readonly-extra */ public function f(): void {}
}
"""
        self.assertEqual(upstream_diff.parse_stubs(source), {
            'class C': '[@strict-properties] class C',
            'property C::$name': '[@readonly] public string $name',
            'const C::X': '[@virtual] public const X = 1',
            'method C::f': 'public function f(): void',
        })

    def test_attribute_comment_brackets_are_preserved(self):
        text = """<?php
#[A(/* ] */ value: [1, 2])]
function f(#[B("]")] string $x): void {}
"""
        self.assertEqual(upstream_diff.parse_stubs(text), {
            "function f": '#[A(/* ] */ value: [1, 2])] function f(#[B("]")] string $x): void',
        })

    def test_functions_and_semicolon_namespace(self):
        text = """namespace Foo\\Bar;

function strlen(string $string): int;
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {"function Foo\\Bar\\strlen": "function strlen(string $string): int"},
        )

    def test_braced_namespace(self):
        text = """namespace Foo {
    function a(): void;
}

namespace Bar {
    class C {}
}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {
                "function Foo\\a": "function a(): void",
                "class Bar\\C": "class C",
            },
        )

    def test_classes_interfaces_traits_and_enums(self):
        text = """namespace Foo;

class Child extends Base implements A, B
{
    public static function make(int $x): self;
    const KIND = 'child';
}

interface I extends J {}

trait T {}

enum E: string implements I {}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {
                "class Foo\\Child": "class Child extends Base implements A, B",
                "method Foo\\Child::make": "public static function make(int $x): self",
                "const Foo\\Child::KIND": "const KIND = 'child'",
                "interface Foo\\I": "interface I extends J",
                "trait Foo\\T": "trait T",
                "enum Foo\\E": "enum E: string implements I",
            },
        )

    def test_global_and_namespaced_constants(self):
        text = """const TOP = 1;

namespace Foo;

const IN_NS = 2;
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {
                "const TOP": "const TOP = 1",
                "const Foo\\IN_NS": "const IN_NS = 2",
            },
        )

    def test_comments_and_unlisted_doc_tags_do_not_count(self):
        with_metadata = """<?php
namespace Foo;

/** Function doc. */
/** @link https://www.php.net/f */
function f(int $a): void {}

// line comment
/** @undocumentable */
class K
{
    /* block comment */
    /** @refcount 1 */
    const X = 1;

    /** @compile-time-eval */
    public function m(): void {}
}

/** @frameless-function {"arity": 0} */ function g(): void {}
"""
        without_metadata = """<?php
namespace Foo;

function f(int $a): void {}

class K
{
    const X = 1;

    public function m(): void {}
}

function g(): void {}
"""
        expected = {
            "function Foo\\f": "function f(int $a): void",
            "class Foo\\K": "class K",
            "const Foo\\K::X": "const X = 1",
            "method Foo\\K::m": "public function m(): void",
            "function Foo\\g": "function g(): void",
        }
        self.assertEqual(upstream_diff.parse_stubs(with_metadata), expected)
        self.assertEqual(upstream_diff.parse_stubs(without_metadata), expected)

    def test_multiline_declarations_collapse_whitespace(self):
        text = """namespace Foo;

function long(int $a,
              string $b): array;

class Wide extends Base
    implements A, B
{
}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {
                "function Foo\\long": "function long(int $a, string $b): array",
                "class Foo\\Wide": "class Wide extends Base implements A, B",
            },
        )

    def test_signature_stops_before_body(self):
        text = """namespace Foo;

function body(int $x): void
{
    return;
}

class C
{
    public function m(): void { }
}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(text),
            {
                "function Foo\\body": "function body(int $x): void",
                "class Foo\\C": "class C",
                "method Foo\\C::m": "public function m(): void",
            },
        )


class TestParseStubsReview(unittest.TestCase):
    """Cases found by the PR #105 review, each checked against php-8.5.12 sources."""

    def test_open_tag_does_not_hide_the_first_declaration(self):
        symbols = upstream_diff.parse_stubs(
            "<?php\n\n/** @generate-class-entries */\n\n"
            "final class Closure\n{\n    private function __construct() {}\n"
            "    public static function bind(Closure $closure): ?Closure {}\n}\n"
        )
        self.assertEqual(symbols["class Closure"], "final class Closure")
        self.assertIn("method Closure::bind", symbols)
        self.assertNotIn("function bind", symbols)

    def test_heredoc_is_a_string(self):
        symbols = upstream_diff.parse_stubs(
            "<?php\nnamespace N;\nconst X = <<<TXT\n} ; function fake(): void {}\nTXT;\n"
            "function real(): void {}\n"
        )
        self.assertEqual(sorted(symbols), ["const N\\X", "function N\\real"])
        self.assertEqual(symbols["const N\\X"],
                         "const X = <<<TXT\n} ; function fake(): void {}\nTXT")

    def test_whitespace_inside_strings_is_kept(self):
        two = upstream_diff.parse_stubs('<?php function f(string $x = "a  b"):  void {}')
        one = upstream_diff.parse_stubs('<?php function f(string $x = "a b"): void {}')
        self.assertEqual(two["function f"], 'function f(string $x = "a  b"): void')
        self.assertNotEqual(two, one)

    def test_enum_cases(self):
        symbols = upstream_diff.parse_stubs(
            "<?php\nenum RoundingMode {\n    case HalfAwayFromZero;\n    case HalfTowardsZero;\n}\n"
            "enum Suit: string { case Hearts = 'H'; public function color(): string { switch (1) { case 1: return ''; } } }\n"
        )
        self.assertEqual(symbols["case RoundingMode::HalfAwayFromZero"], "case HalfAwayFromZero")
        self.assertEqual(symbols["case Suit::Hearts"], "case Hearts = 'H'")
        self.assertEqual(sorted(k for k in symbols if k.startswith("case ")),
                         ["case RoundingMode::HalfAwayFromZero",
                          "case RoundingMode::HalfTowardsZero", "case Suit::Hearts"])

    def test_every_constant_of_a_list(self):
        symbols = upstream_diff.parse_stubs(
            "<?php namespace N; interface I { public const int A = 1, B = f(2, 3); }\nconst G = [1, 2], H = 3;"
        )
        self.assertEqual(symbols["const N\\I::A"], "public const int A = 1")
        self.assertEqual(symbols["const N\\I::B"], "public const int B = f(2, 3)")
        self.assertEqual(symbols["const N\\G"], "const G = [1, 2]")
        self.assertEqual(symbols["const N\\H"], "const H = 3")

    def test_same_symbol_in_several_stub_files_is_compared_per_file(self):
        apache = "<?php function apache_request_headers(): array {}"
        fpm = "<?php function apache_request_headers(): array {}"
        old = {"sapi/apache2handler/php_functions.stub.php": apache, "sapi/fpm/fpm/fpm_main.stub.php": fpm}
        new = dict(old)
        new["sapi/apache2handler/php_functions.stub.php"] = apache.replace("array", "array|false")
        changed = upstream_diff.diff_releases(old, new, "8.5.12")["stubs"]["changed"]
        self.assertEqual([(entry["symbol"], entry["file"]) for entry in changed],
                         [("function apache_request_headers", "sapi/apache2handler/php_functions.stub.php")])

    def test_conditional_declarations_keep_every_branch(self):
        text = ("<?php\n#if FOO\nfunction f(int $x): void {}\n#else\n"
                "function f(string $x): void {}\n#endif\nfunction g(): void {}\n")
        symbols = upstream_diff.parse_stubs(text)
        self.assertEqual(symbols["function f"],
                         "[if FOO] function f(int $x): void\n[if FOO / else] function f(string $x): void")
        self.assertEqual(symbols["function g"], "function g(): void")
        changed = upstream_diff.parse_stubs(text.replace("int $x", "float $x"))
        self.assertNotEqual(changed["function f"], symbols["function f"])


class TestGrammarSymbols(unittest.TestCase):
    def test_existing_rule_bodies_and_full_source_changes(self):
        parser = """%{
int helper = 1;
%}
%token T_STRING T_NAME
%left T_STRING
%start program
%% /* Rules */
program:
    name { /* action */ }
;
name: T_STRING ;
name: T_NAME ;
%%
int main(void) { return helper; }
"""
        scanner = "%{\n#include \"parser.h\"\n%}\n%%\n[a-z]+ { return T_STRING; }\n%%\nint yywrap(void) { return 1; }\n"
        old = {"Zend/zend_language_parser.y": parser,
               "Zend/zend_language_scanner.l": scanner}
        cases = (
            (parser, [], False),
            (parser.replace("name {", "name    {"), [], True),
            (parser.replace("/* action */", "/* changed */"), ["program"], True),
            (parser.replace("name: T_STRING ;\nname: T_NAME ;", "name: T_NAME ;\nname: T_STRING ;"), ["name"], True),
            (parser.replace("%left", "%right"), [], True),
            (parser.replace("%left", "%precedence"), [], True),
            (parser.replace("helper = 1", "helper = 2"), [], True),
            (parser.replace("return helper", "return 0"), [], True),
        )
        for changed, rules, parser_changed in cases:
            with self.subTest(parser=changed):
                report = upstream_diff.diff_releases(old, dict(old, **{
                    "Zend/zend_language_parser.y": changed}), "8.5.12")
                self.assertEqual(report["grammar"]["rules_changed"], rules)
                self.assertIs(report["grammar"]["parser_changed"], parser_changed)
                self.assertIs(report["grammar"]["scanner_changed"], False)
                self.assertEqual(report["affected_work_packages"],
                                 ["W-09", "W-11"] if parser_changed else [])
        report = upstream_diff.diff_releases(old, dict(old, **{
            "Zend/zend_language_scanner.l": scanner.replace("[a-z]", "[a-zA-Z]")}), "8.5.12")
        self.assertEqual(report["grammar"]["rules_changed"], [])
        self.assertIs(report["grammar"]["parser_changed"], False)
        self.assertIs(report["grammar"]["scanner_changed"], True)
        self.assertEqual(report["affected_work_packages"], ["W-09"])

    def test_rules_separator_allows_trailing_comment(self):
        parser_y = """%{
int yylex(void);
void yyerror(const char *message);
%}
%token T_STRING
%start program
%% /* Rules */
program:
    T_STRING { /* preserve rule actions */ }
;
%% /* C epilogue */
void yyerror(const char *message) {}
"""
        self.assertEqual(upstream_diff.grammar_symbols(parser_y),
                         {"tokens": ["T_STRING"], "rules": ["program"]})

    def test_tokens_and_rules(self):
        parser_y = """%token <ident> T_STRING "identifier"
%token T_ECHO T_IF
%token <num> T_LNUMBER "integer"
%token T_ZEXTRA
%token T_AEXTRA

prologue_not_a_rule:

%%
start:
    top_statement_list
;
top_statement:
    statement
;
  indented_not_a_rule:
;
duplicate_rule:
    x
;
duplicate_rule:
    y
;
%%
after_second_section:
    z
;
"""
        self.assertEqual(
            upstream_diff.grammar_symbols(parser_y),
            {
                "tokens": ["T_AEXTRA", "T_ECHO", "T_IF", "T_LNUMBER", "T_STRING", "T_ZEXTRA"],
                "rules": ["duplicate_rule", "start", "top_statement"],
            },
        )


class TestNewsEntries(unittest.TestCase):
    def test_entries_of_one_release(self):
        entries = upstream_diff.news_entries(NEWS, "8.5.12")
        self.assertEqual(
            entries,
            [
                {
                    "section": "Core",
                    "text": (
                        "Fixed GHSA-w8hw-w9fx-j79w, GHSA-6wmf-4rv5-p3rp "
                        "(Avoid state corruption via bailout). "
                        "(CVE-2026-109618, CVE-2026-109619) (Arnaud)"
                    ),
                    "security": True,
                    "ids": [
                        "GHSA-w8hw-w9fx-j79w",
                        "GHSA-6wmf-4rv5-p3rp",
                        "CVE-2026-109618",
                        "CVE-2026-109619",
                    ],
                },
                {
                    "section": "Core",
                    "text": "Fixed bug GH-23644 (Optimizer crash). (ndossche)",
                    "security": False,
                    "ids": [],
                },
                {
                    "section": "Core",
                    "text": (
                        "Fixed GHSA-w8hw-w9fx-j79w again, GHSA-w8hw-w9fx-j79w. "
                        "(CVE-2026-109619, CVE-2026-109619)"
                    ),
                    "security": True,
                    "ids": ["GHSA-w8hw-w9fx-j79w", "CVE-2026-109619"],
                },
                {
                    "section": "PDO_MySQL",
                    "text": "Fixed GHSA-aaaa-bbbb-cccc (quoting). (CVE-2026-12345)",
                    "security": True,
                    "ids": ["GHSA-aaaa-bbbb-cccc", "CVE-2026-12345"],
                },
            ],
        )

    def test_only_the_requested_release_is_read(self):
        self.assertEqual(
            upstream_diff.news_entries(NEWS, "8.5.11"),
            [
                {
                    "section": "Core",
                    "text": "Fixed bug GH-1 (old). (x)",
                    "security": False,
                    "ids": [],
                }
            ],
        )

    def test_missing_version_raises_value_error(self):
        for version in ("8.4.0", "8.5.1"):
            with self.subTest(version=version):
                with self.assertRaises(ValueError):
                    upstream_diff.news_entries(NEWS, version)


class TestReadArchive(unittest.TestCase):
    VERSION = "8.2.0"
    ROOT = "php-8.2.0"

    def tar_xz(self, files, directories=()):
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:xz") as archive:
            for name in directories:
                info = tarfile.TarInfo(name=name)
                info.type = tarfile.DIRTYPE
                info.mtime = 0
                archive.addfile(info)
            for name, data in files:
                info = tarfile.TarInfo(name=name)
                info.size = len(data)
                info.mtime = 0
                archive.addfile(info, io.BytesIO(data))
        return buffer.getvalue()

    def test_reads_only_parser_stubs_tests_and_news(self):
        contents = {
            "Zend/zend_language_parser.y": b"%token T_FOO\n",
            "Zend/zend_language_scanner.l": b"%%\n",
            "ext/foo/foo.stub.php": b"function foo(): void;\n",
            "tests/basic/001.phpt": b"--TEST--\nbasic\n",
            "NEWS": b"PHP 8.2.0\n",
        }
        extras = {
            "ext/foo/foo.c": b"int foo;\n",
            "README.md": b"readme\n",
            "ext/foo/notes.txt": b"notes\n",
        }
        files = [(f"{self.ROOT}/{path}", data) for path, data in contents.items()]
        files += [(f"{self.ROOT}/{path}", data) for path, data in extras.items()]
        directories = [f"{self.ROOT}/", f"{self.ROOT}/Zend/", f"{self.ROOT}/ext/foo/"]
        result = upstream_diff.read_archive(self.tar_xz(files, directories), self.VERSION)
        self.assertEqual(set(result), set(contents))
        for path, data in contents.items():
            expected = data if path.endswith(".phpt") else data.decode("utf-8")
            self.assertEqual(result[path], expected)

    def test_phpt_bytes_are_compared_exactly(self):
        old = upstream_diff.read_archive(self.tar_xz([(f"{self.ROOT}/t.phpt", b"--EXPECT--\n\xff\n")]), self.VERSION)
        new = upstream_diff.read_archive(self.tar_xz([(f"{self.ROOT}/t.phpt", b"--EXPECT--\n\xfe\n")]), self.VERSION)
        report = upstream_diff.diff_releases(old, new, self.VERSION)
        self.assertEqual([entry["path"] for entry in report["tests"]["modified"]], ["t.phpt"])

    def test_oversized_pax_metadata_is_rejected(self):
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:xz", format=tarfile.PAX_FORMAT) as archive:
            info = tarfile.TarInfo(name=f"{self.ROOT}/t.phpt")
            info.pax_headers = {"comment": "x" * (upstream_snapshot.MAX_METADATA + 1)}
            info.size = 1
            archive.addfile(info, io.BytesIO(b"x"))
        with self.assertRaises(ValueError):
            upstream_diff.read_archive(buffer.getvalue(), self.VERSION)

    def test_invalid_utf8_is_replaced(self):
        path = "ext/foo/foo.stub.php"
        data = self.tar_xz([(f"{self.ROOT}/{path}", b"function foo(): void; \xff")])
        text = upstream_diff.read_archive(data, self.VERSION)[path]
        self.assertTrue(text.startswith("function foo(): void;"))
        self.assertIn("\ufffd", text)

    def test_limits_and_unsafe_paths_raise_value_error(self):
        simple = self.tar_xz([(f"{self.ROOT}/ext/foo/foo.stub.php", b"function foo(): void;\n")])
        cases = {
            "archive too large": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", 1),
                simple,
            ),
            "member too large": (
                mock.patch.object(upstream_diff, "MAX_FILE", 4),
                self.tar_xz([(f"{self.ROOT}/ext/foo/foo.stub.php", b"function foo(): void;\n")]),
            ),
            "absolute path": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", upstream_diff.MAX_ARCHIVE),
                self.tar_xz([(f"/{self.ROOT}/Zend/zend_language_parser.y", b"%token T_A\n")]),
            ),
            "dotdot": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", upstream_diff.MAX_ARCHIVE),
                self.tar_xz([(f"{self.ROOT}/../Zend/zend_language_parser.y", b"%token T_A\n")]),
            ),
            "nested dotdot": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", upstream_diff.MAX_ARCHIVE),
                self.tar_xz([(f"{self.ROOT}/Zend/../Zend/zend_language_parser.y", b"%token T_A\n")]),
            ),
            "outside root": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", upstream_diff.MAX_ARCHIVE),
                self.tar_xz([("other/Zend/zend_language_parser.y", b"%token T_A\n")]),
            ),
            "wrong root": (
                mock.patch.object(upstream_diff, "MAX_ARCHIVE", upstream_diff.MAX_ARCHIVE),
                self.tar_xz([("php-8.2.1/Zend/zend_language_parser.y", b"%token T_A\n")]),
            ),
            "too many members": (
                mock.patch.object(upstream_diff, "MAX_MEMBERS", 2),
                self.tar_xz([(f"{self.ROOT}/t{i}.phpt", b"x") for i in range(3)]),
            ),
            "unpacks too large": (
                mock.patch.object(upstream_diff, "MAX_UNPACKED", 5),
                self.tar_xz([(f"{self.ROOT}/README", b"abc"), (f"{self.ROOT}/LICENSE", b"abc")]),
            ),
            "keeps too much": (
                mock.patch.object(upstream_diff, "MAX_KEPT", 5),
                self.tar_xz([(f"{self.ROOT}/a.phpt", b"abc"), (f"{self.ROOT}/b.phpt", b"abc")]),
            ),
        }
        for label, (patched, data) in cases.items():
            with self.subTest(case=label):
                with patched:
                    with self.assertRaises(ValueError):
                        upstream_diff.read_archive(data, self.VERSION)


class TestDiffReleases(unittest.TestCase):
    NEW_VERSION = "8.5.12"

    OLD_PARSER = """%token T_KEEP
%token T_OLD
%token T_ZOLD

%%

rule_keep:
    x
;

rule_removed:
    y
;

rule_zremoved:
    z
;

%%
"""

    NEW_PARSER = """%token T_KEEP
%token T_ZNEW
%token T_NEW

%%

rule_keep:
    x
;

rule_added:
    y
;

rule_zadded:
    z
;

%%
"""

    OLD_STUB = """namespace Std;

function alpha(int $x): void;
function beta(): string;
class Delta {}
/** old docs */
class Gamma {}
const LIMIT = 3;
"""

    NEW_STUB = """namespace Std;

function alpha(int $x): int;
function gamma(): void;
/** new docs */
class Gamma {}
const LIMIT = 3;
"""

    NEW_CURL_STUB = """namespace Zzz;

function zeta(): void;
"""

    def fixtures(self):
        old_files = {
            "Zend/zend_language_parser.y": self.OLD_PARSER,
            "Zend/zend_language_scanner.l": "scanner old\n",
            "ext/standard/standard.stub.php": self.OLD_STUB,
            "Zend/tests/001.phpt": "old 001\n",
            "Zend/tests/002.phpt": "old 002\n",
            "NEWS": NEWS,
        }
        new_files = {
            "Zend/zend_language_parser.y": self.NEW_PARSER,
            "Zend/zend_language_scanner.l": "scanner new\n",
            "ext/standard/standard.stub.php": self.NEW_STUB,
            "ext/curl/curl.stub.php": self.NEW_CURL_STUB,
            "Zend/tests/001.phpt": "new 001\n",
            "Zend/tests/003.phpt": "new 003\n",
            "tests/unmapped/009.phpt": "new 009\n",
            "NEWS": NEWS,
        }
        return old_files, new_files

    def report(self):
        old_files, new_files = self.fixtures()
        return upstream_diff.diff_releases(old_files, new_files, self.NEW_VERSION)

    def test_report_shape_and_stubs(self):
        report = self.report()
        self.assertEqual(
            set(report),
            {"schema_version", "new_version", "stubs", "grammar", "tests", "security",
             "affected_work_packages"},
        )
        self.assertEqual(report["schema_version"], 2)
        self.assertEqual(report["new_version"], self.NEW_VERSION)
        self.assertEqual(
            report["stubs"],
            {
                "added": [
                    {
                        "symbol": "function Std\\gamma",
                        "file": "ext/standard/standard.stub.php",
                        "old": None,
                        "new": "function gamma(): void",
                        "work_packages": ["W-29"],
                    },
                    {
                        "symbol": "function Zzz\\zeta",
                        "file": "ext/curl/curl.stub.php",
                        "old": None,
                        "new": "function zeta(): void",
                        "work_packages": ["W-32"],
                    },
                ],
                "removed": [
                    {
                        "symbol": "class Std\\Delta",
                        "file": "ext/standard/standard.stub.php",
                        "old": "class Delta",
                        "new": None,
                        "work_packages": ["W-29"],
                    },
                    {
                        "symbol": "function Std\\beta",
                        "file": "ext/standard/standard.stub.php",
                        "old": "function beta(): string",
                        "new": None,
                        "work_packages": ["W-29"],
                    },
                ],
                "changed": [
                    {
                        "symbol": "function Std\\alpha",
                        "file": "ext/standard/standard.stub.php",
                        "old": "function alpha(int $x): void",
                        "new": "function alpha(int $x): int",
                        "work_packages": ["W-29"],
                    },
                ],
            },
        )

    def test_doc_comment_only_change_is_not_a_stub_change(self):
        stubs = self.report()["stubs"]
        symbols = [entry["symbol"] for group in stubs.values() for entry in group]
        self.assertNotIn("class Std\\Gamma", symbols)
        self.assertNotIn("const Std\\LIMIT", symbols)

    def test_grammar_changes(self):
        self.assertEqual(
            self.report()["grammar"],
            {
                "tokens_added": ["T_NEW", "T_ZNEW"],
                "tokens_removed": ["T_OLD", "T_ZOLD"],
                "rules_added": ["rule_added", "rule_zadded"],
                "rules_removed": ["rule_removed", "rule_zremoved"],
                "rules_changed": [],
                "parser_changed": True,
                "scanner_changed": True,
            },
        )

    def test_test_changes(self):
        self.assertEqual(
            self.report()["tests"],
            {
                "added": [
                    {
                        "path": "Zend/tests/003.phpt",
                        "work_packages": ["W-11", "W-17"],
                    },
                    {
                        "path": "tests/unmapped/009.phpt",
                        "work_packages": ["unmapped"],
                    },
                ],
                "removed": [
                    {
                        "path": "Zend/tests/002.phpt",
                        "work_packages": ["W-11", "W-17"],
                    },
                ],
                "modified": [
                    {
                        "path": "Zend/tests/001.phpt",
                        "work_packages": ["W-11", "W-17"],
                    },
                ],
            },
        )

    def test_security_and_affected_work_packages(self):
        report = self.report()
        self.assertEqual(
            report["security"],
            [
                {
                    "section": "Core",
                    "text": (
                        "Fixed GHSA-w8hw-w9fx-j79w, GHSA-6wmf-4rv5-p3rp "
                        "(Avoid state corruption via bailout). "
                        "(CVE-2026-109618, CVE-2026-109619) (Arnaud)"
                    ),
                    "security": True,
                    "ids": [
                        "GHSA-w8hw-w9fx-j79w",
                        "GHSA-6wmf-4rv5-p3rp",
                        "CVE-2026-109618",
                        "CVE-2026-109619",
                    ],
                    "work_packages": ["W-11", "W-17"],
                    "assessment": "needs_assessment",
                },
                {
                    "section": "Core",
                    "text": (
                        "Fixed GHSA-w8hw-w9fx-j79w again, GHSA-w8hw-w9fx-j79w. "
                        "(CVE-2026-109619, CVE-2026-109619)"
                    ),
                    "security": True,
                    "ids": ["GHSA-w8hw-w9fx-j79w", "CVE-2026-109619"],
                    "work_packages": ["W-11", "W-17"],
                    "assessment": "needs_assessment",
                },
                {
                    "section": "PDO_MySQL",
                    "text": "Fixed GHSA-aaaa-bbbb-cccc (quoting). (CVE-2026-12345)",
                    "security": True,
                    "ids": ["GHSA-aaaa-bbbb-cccc", "CVE-2026-12345"],
                    "work_packages": ["W-31"],
                    "assessment": "needs_assessment",
                },
            ],
        )
        affected = report["affected_work_packages"]
        self.assertEqual(
            affected,
            ["W-09", "W-11", "W-17", "W-29", "W-31", "W-32", "unmapped"],
        )
        self.assertEqual(affected, sorted(affected))


class TestMain(unittest.TestCase):
    OLD_VERSION = "8.5.11"
    NEW_VERSION = "8.5.12"

    def tar_xz(self, files):
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:xz") as archive:
            for name, data in files:
                info = tarfile.TarInfo(name=name)
                info.size = len(data)
                info.mtime = 0
                archive.addfile(info, io.BytesIO(data))
        return buffer.getvalue()

    def write_archive(self, directory, name, files):
        path = os.path.join(directory, name)
        with open(path, "wb") as handle:
            handle.write(self.tar_xz(files))
        return path

    def test_success_writes_json_and_returns_zero(self):
        with tempfile.TemporaryDirectory() as directory:
            old_path = self.write_archive(
                directory,
                "old.tar.xz",
                [
                    (
                        f"php-{self.OLD_VERSION}/ext/standard/standard.stub.php",
                        b"namespace Std;\nfunction alpha(): void;\n",
                    ),
                    (f"php-{self.OLD_VERSION}/NEWS", NEWS.encode()),
                ],
            )
            new_path = self.write_archive(
                directory,
                "new.tar.xz",
                [
                    (
                        f"php-{self.NEW_VERSION}/ext/standard/standard.stub.php",
                        b"namespace Std;\nfunction alpha(): int;\n",
                    ),
                    (f"php-{self.NEW_VERSION}/NEWS", NEWS.encode()),
                ],
            )
            stdout = io.StringIO()
            stderr = io.StringIO()
            with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                status = upstream_diff.main(
                    [
                        old_path,
                        new_path,
                        "--old-version",
                        self.OLD_VERSION,
                        "--new-version",
                        self.NEW_VERSION,
                    ]
                )
            self.assertEqual(status, 0)
            self.assertEqual(stderr.getvalue(), "")
            report = json.loads(stdout.getvalue())
            self.assertEqual(report["schema_version"], 2)
            self.assertEqual(report["new_version"], self.NEW_VERSION)
            self.assertEqual(
                [entry["symbol"] for entry in report["stubs"]["changed"]],
                ["function Std\\alpha"],
            )

    def test_missing_file_and_value_error_write_stderr_and_return_two(self):
        with tempfile.TemporaryDirectory() as directory:
            missing_path = os.path.join(directory, "missing.tar.xz")
            invalid_path = self.write_archive(
                directory,
                "invalid.tar.xz",
                [("wrong-root/Zend/zend_language_parser.y", b"%token T_A\n")],
            )
            cases = {
                "missing file": [missing_path, missing_path],
                "value error": [invalid_path, invalid_path],
            }
            for label, archives in cases.items():
                with self.subTest(case=label):
                    stdout = io.StringIO()
                    stderr = io.StringIO()
                    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                        status = upstream_diff.main(
                            [
                                archives[0],
                                archives[1],
                                "--old-version",
                                self.OLD_VERSION,
                                "--new-version",
                                self.NEW_VERSION,
                            ]
                        )
                    self.assertEqual(status, 2)
                    self.assertTrue(stderr.getvalue().strip())
                    self.assertEqual(stdout.getvalue(), "")


if __name__ == "__main__":
    unittest.main()
