import unittest
from unittest import mock

import tools.upstream_diff as upstream_diff


VERSION = "8.5.12"
PARSER_PATH = "Zend/zend_language_parser.y"
SCANNER_PATH = "Zend/zend_language_scanner.l"


class TestPropertyStubs(unittest.TestCase):
    def test_typed_properties_keep_modifiers_types_and_defaults(self):
        source = """<?php
namespace Review;

class TypedProperties
{
    public ?Foo $nullable = null;
    protected A|B $union = A::DEFAULT;
    private (A&B)|null $intersection = null;
}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(source),
            {
                "class Review\\TypedProperties": "class TypedProperties",
                "property Review\\TypedProperties::$nullable": "public ?Foo $nullable = null",
                "property Review\\TypedProperties::$union": "protected A|B $union = A::DEFAULT",
                "property Review\\TypedProperties::$intersection":
                    "private (A&B)|null $intersection = null",
            },
        )

    def test_hooks_skip_matched_bodies_and_do_not_create_properties_from_locals(self):
        source = """<?php
class Hooked
{
    public string $label {
        get => $this->label;
        set (string $value) {
            $local = trim($value);
            $this->label = $local;
        }
    }

    public function update(int $parameter): void
    {
        $bodyVariable = $parameter;
    }
}
"""
        symbols = upstream_diff.parse_stubs(source)
        self.assertEqual(
            symbols,
            {
                "class Hooked": "class Hooked",
                "property Hooked::$label": "public string $label",
                "method Hooked::update": "public function update(int $parameter): void",
            },
        )
        self.assertFalse(any("$parameter" in name or "$bodyVariable" in name
                             for name in symbols))

    def test_multiple_properties_split_only_at_top_level_commas(self):
        source = """<?php
class ManyProperties
{
    public array $items = ['outer' => ['left,right' => [1, 2]]],
        $empty = [],
        $nested = ['key' => ['value' => 3]];
}
"""
        self.assertEqual(
            upstream_diff.parse_stubs(source),
            {
                "class ManyProperties": "class ManyProperties",
                "property ManyProperties::$items":
                    "public array $items = ['outer' => ['left,right' => [1, 2]]]",
                "property ManyProperties::$empty": "public array $empty = []",
                "property ManyProperties::$nested":
                    "public array $nested = ['key' => ['value' => 3]]",
            },
        )


class TestStubMetadata(unittest.TestCase):
    ATTRIBUTE = (
        "#[Audit(payload: ['outer' => [1, 2]], "
        'label: "quoted ] bracket")]'
    )
    PARAMETER_ATTRIBUTE = (
        "#[Parameter(nested: ['a' => [1, 2]], "
        'text: "quoted ] bracket")]'
    )

    def test_attributes_are_kept_at_declarations_and_parameters(self):
        source = f"""<?php
#[Entity(data: ['x' => [1, 2]], note: "quoted ] bracket")]
class Decorated
{{
    {self.ATTRIBUTE}
    public string $value;

    {self.ATTRIBUTE}
    public function set( {self.PARAMETER_ATTRIBUTE} string $value): void {{}}
}}
"""
        symbols = upstream_diff.parse_stubs(source)
        self.assertEqual(
            symbols,
            {
                "class Decorated":
                    '#[Entity(data: [\'x\' => [1, 2]], note: "quoted ] bracket")] class Decorated',
                "property Decorated::$value":
                    f"{self.ATTRIBUTE} public string $value",
                "method Decorated::set":
                    # Signatures drop the space inside brackets.
                    f"{self.ATTRIBUTE} public function set({self.PARAMETER_ATTRIBUTE} "
                    "string $value): void",
            },
        )

    def test_only_documented_metadata_tags_are_included_and_metadata_order_is_canonical(self):
        tags = (
            "@tentative-return-type",
            "@readonly",
            "@virtual",
            "@strict-properties",
            "@not-serializable",
            "@prefer-ref",
            "@deprecated",
        )
        source = """<?php
#if FEATURE
class Canonical
{
    #[Audit(payload: ['x' => [1, 2]])]
    /**
     * @tentative-return-type
     * @readonly
     * @virtual
     * @strict-properties
     * @not-serializable
     * @prefer-ref
     * @return string
     * @refcount 1
     * @deprecated
     */
    public static function canonical(): void {}
}
#endif
"""
        signature = upstream_diff.parse_stubs(source)["method Canonical::canonical"]
        self.assertTrue(signature.startswith("[if FEATURE] "))
        expected_attribute = "#[Audit(payload: ['x' => [1, 2]])]"
        self.assertIn(expected_attribute, signature)
        attribute_at = signature.index(expected_attribute)
        modifier_at = signature.index("public static function canonical(): void")
        self.assertLess(attribute_at, modifier_at)
        for tag in tags:
            with self.subTest(tag=tag):
                tag_at = signature.index(f"[{tag}]")
                self.assertLess(signature.index("[if FEATURE]"), tag_at)
                self.assertLess(tag_at, attribute_at)
        for ignored in ("@return", "@refcount"):
            self.assertNotIn(f"[{ignored}", signature)

    def test_doc_and_attribute_source_order_does_not_change_prefix_order(self):
        doc_then_attribute = """<?php
class Ordered
{
/**
 * @tentative-return-type string
 */
#[Audit(value: [1, 2])]
public function ordered(): void {}
}
"""
        attribute_then_doc = """<?php
class Ordered
{
#[Audit(value: [1, 2])]
/**
 * @tentative-return-type string
 */
public function ordered(): void {}
}
"""
        expected = (
            "[@tentative-return-type string] "
            "#[Audit(value: [1, 2])] public function ordered(): void"
        )
        self.assertEqual(upstream_diff.parse_stubs(doc_then_attribute)["method Ordered::ordered"], expected)
        self.assertEqual(upstream_diff.parse_stubs(attribute_then_doc)["method Ordered::ordered"], expected)

    def test_attribute_argument_changes_are_reported_with_normalized_whitespace(self):
        original = """<?php
#[Audit(data: ['a' => [1, 2]], text: "two  spaces")]
function attributed(): void {}
"""
        whitespace_only = original.replace("[1, 2]", "[1,  2]")
        changed = original.replace("[1, 2]", "[1, 3]")
        before = upstream_diff.parse_stubs(original)
        expected_before = (
            '#[Audit(data: [\'a\' => [1, 2]], text: "two  spaces")] '
            "function attributed(): void"
        )
        expected_after = (
            '#[Audit(data: [\'a\' => [1, 3]], text: "two  spaces")] '
            "function attributed(): void"
        )
        self.assertEqual(before, {"function attributed": expected_before})
        self.assertEqual(before, upstream_diff.parse_stubs(whitespace_only))
        after = upstream_diff.parse_stubs(changed)
        self.assertEqual(after, {"function attributed": expected_after})

        report = upstream_diff.diff_releases(
            {"ext/demo/demo.stub.php": original},
            {"ext/demo/demo.stub.php": changed},
            VERSION,
        )
        self.assertEqual(
            report["stubs"]["changed"],
            [{
                "symbol": "function attributed",
                "file": "ext/demo/demo.stub.php",
                "old": expected_before,
                "new": expected_after,
                "work_packages": upstream_diff.impact("ext/demo/demo.stub.php"),
            }],
        )


class TestMetadataBudgets(unittest.TestCase):
    DECLARATION = """<?php
#if FEATURE
/** @tentative-return-type */
#[Budget(value: [1, 2])]
function budget(): void {}
#endif
"""
    SYMBOL = "function budget"
    SIGNATURE = (
        "[if FEATURE] [@tentative-return-type] "
        "#[Budget(value: [1, 2])] function budget(): void"
    )

    def test_attribute_is_preserved_at_exact_token_budget(self):
        source = """<?php
#[Budget(value: [1, 2])]
function budget(): void {}
"""
        with mock.patch.object(upstream_diff, "MAX_STUB_TOKENS", 10):
            self.assertEqual(
                upstream_diff.parse_stubs(source),
                {"function budget": "#[Budget(value: [1, 2])] function budget(): void"},
            )

    def test_attribute_adds_one_token_at_token_budget_boundary(self):
        source = """<?php
#[Budget(value: [1, 2])]
function budget(): void {}
"""
        with mock.patch.object(upstream_diff, "MAX_STUB_TOKENS", 9):
            with self.assertRaisesRegex(ValueError, "token"):
                upstream_diff.parse_stubs(source)

    def test_signature_budget_accepts_actual_metadata_output_bytes(self):
        total_bytes = len(self.SYMBOL.encode("utf-8")) + len(self.SIGNATURE.encode("utf-8"))
        with mock.patch.object(upstream_diff, "MAX_SIGNATURE_BYTES", total_bytes):
            self.assertEqual(
                upstream_diff.parse_stubs(self.DECLARATION),
                {self.SYMBOL: self.SIGNATURE},
            )

    def test_signature_budget_rejects_one_byte_below_actual_metadata_output(self):
        total_bytes = len(self.SYMBOL.encode("utf-8")) + len(self.SIGNATURE.encode("utf-8"))
        with mock.patch.object(upstream_diff, "MAX_SIGNATURE_BYTES", total_bytes - 1):
            with self.assertRaisesRegex(ValueError, "signature"):
                upstream_diff.parse_stubs(self.DECLARATION)

    def test_condition_budget_accepts_exact_condition_bytes_with_metadata(self):
        condition_bytes = len("if FEATURE".encode("utf-8"))
        with mock.patch.object(upstream_diff, "MAX_CONDITION_BYTES", condition_bytes):
            self.assertEqual(
                upstream_diff.parse_stubs(self.DECLARATION),
                {self.SYMBOL: self.SIGNATURE},
            )

    def test_condition_budget_rejects_one_byte_below_condition_bytes(self):
        condition_bytes = len("if FEATURE".encode("utf-8"))
        with mock.patch.object(upstream_diff, "MAX_CONDITION_BYTES", condition_bytes - 1):
            with self.assertRaisesRegex(ValueError, "condition"):
                upstream_diff.parse_stubs(self.DECLARATION)


class TestGrammarReportContracts(unittest.TestCase):
    def test_report_flags_use_raw_source_comparisons_and_schema_two(self):
        parser = """%{
int helper(void);
%}
%left T_PLUS
%%
program:
    T_PLUS
;
%%
int helper(void) { return 1; }
"""
        scanner = "%{\nint scan(void);\n%}\n%%\n. { return 1; }\n%%\n"
        cases = (
            ({PARSER_PATH: parser, SCANNER_PATH: scanner},
             {PARSER_PATH: parser, SCANNER_PATH: scanner}, False, False, []),
            ({PARSER_PATH: parser, SCANNER_PATH: scanner},
             {PARSER_PATH: parser + "\n", SCANNER_PATH: scanner}, True, False,
             upstream_diff.impact(PARSER_PATH)),
            ({PARSER_PATH: parser, SCANNER_PATH: scanner},
             {PARSER_PATH: parser, SCANNER_PATH: scanner + "\n"}, False, True,
             upstream_diff.impact(SCANNER_PATH)),
            ({PARSER_PATH: parser}, {PARSER_PATH: parser, SCANNER_PATH: ""}, False, True,
             upstream_diff.impact(SCANNER_PATH)),
            ({PARSER_PATH: parser, SCANNER_PATH: ""},
             {PARSER_PATH: parser}, False, True, upstream_diff.impact(SCANNER_PATH)),
        )
        for old_files, new_files, parser_changed, scanner_changed, affected in cases:
            with self.subTest(old=old_files, new=new_files):
                report = upstream_diff.diff_releases(old_files, new_files, VERSION)
                self.assertEqual(report["schema_version"], 2)
                self.assertIs(report["grammar"]["parser_changed"], parser_changed)
                self.assertIs(report["grammar"]["scanner_changed"], scanner_changed)
                self.assertEqual(report["affected_work_packages"], affected)


if __name__ == "__main__":
    unittest.main()


class TestSignatureComments(unittest.TestCase):
    """Adding or removing a comment inside a signature is not a change."""

    def test_comment_position_and_line_breaks_do_not_change_signatures(self):
        plain = upstream_diff.parse_stubs("<?php function m($a, $z = null) {}")
        for source in (
            "<?php function m(/** note */$a, $z = null) {}",
            "<?php function m( /** note */ $a, $z = null) {}",
            "<?php function m($a /* note */, $z = null) {}",
            "<?php function m($a, $z = /* note */ null) {}",
            "<?php function m($a, $z = null/** note */) {}",
            "<?php function m(\n    /** note */\n    $a,\n    $z = null\n) {}",
        ):
            with self.subTest(source=source):
                self.assertEqual(upstream_diff.parse_stubs(source), plain)
        self.assertEqual(plain, {"function m": "function m($a, $z = null)"})

    def test_comment_edits_report_no_change_but_real_edits_do(self):
        old = {"ext/demo/demo.stub.php": "<?php function m(/** first */ $z): int {}"}
        same = {"ext/demo/demo.stub.php": "<?php function m($z): int {}"}
        renamed = {"ext/demo/demo.stub.php": "<?php function m($y): int {}"}
        self.assertEqual(upstream_diff.diff_releases(old, same, VERSION)["stubs"]["changed"], [])
        changed = upstream_diff.diff_releases(old, renamed, VERSION)["stubs"]["changed"]
        self.assertEqual([(c["old"], c["new"]) for c in changed], [("function m($z): int", "function m($y): int")])


class TestBehaviorTags(unittest.TestCase):
    """gen_stub.php tags that change generated flags, aliases or constants."""

    def test_behavior_tag_edits_are_reported(self):
        cases = {
            "deprecated function": ("function f(): void {}", "/** @deprecated */ function f(): void {}"),
            "alias": ("/** @alias g */ function f(): void {}", "/** @alias h */ function f(): void {}"),
            "implementation alias": ("/** @implementation-alias g */ function f(): void {}", "function f(): void {}"),
            "constant value": ("/**\n * @var int\n * @cvalue E_ERROR\n */\nconst E = UNKNOWN;", "/**\n * @var int\n * @cvalue E_WARNING\n */\nconst E = UNKNOWN;"),
            "constant type": ("/**\n * @var int\n * @cvalue X\n */\nconst E = UNKNOWN;", "/**\n * @var string\n * @cvalue X\n */\nconst E = UNKNOWN;"),
            "file cache": ("/** @var int */\nconst E = 1;", "/**\n * @var int\n * @no-file-cache\n */\nconst E = 1;"),
        }
        for name, (old, new) in cases.items():
            with self.subTest(name=name):
                report = upstream_diff.diff_releases(
                    {"ext/demo/demo.stub.php": "<?php " + old},
                    {"ext/demo/demo.stub.php": "<?php " + new},
                    VERSION,
                )
                self.assertEqual(len(report["stubs"]["changed"]), 1, report["stubs"])


class TestOpcacheImpact(unittest.TestCase):
    def test_file_cache_jit_and_optimizer_paths_map_to_their_work_packages(self):
        for path, expected in {
            "ext/opcache/tests/file_cache_error.phpt": ["W-23"],
            "ext/opcache/tests/gh16979_check_file_cache_function.phpt": ["W-23"],
            "ext/opcache/zend_file_cache.c": ["W-23"],
            "ext/opcache/jit/zend_jit.c": ["W-47"],
            "ext/opcache/tests/jit/add_001.phpt": ["W-47"],
            "Zend/Optimizer/sccp.c": ["W-15"],
            "ext/opcache/tests/opt/dce_001.phpt": ["W-15"],
            "Zend/zend_compile.c": ["W-11", "W-17"],
        }.items():
            with self.subTest(path=path):
                self.assertEqual(upstream_diff.impact(path), expected)
