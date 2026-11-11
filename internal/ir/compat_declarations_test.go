package ir

import (
	"errors"
	"testing"
)

// TestCompatibilityDeclarations checks declaration-time behavior against the
// PHP 8.5.12 compile oracle. Oracle IDs refer to compile-reference.jsonl in the
// W-11 corpus; legal controls were checked with the pinned php:8.5.12-cli.
func TestCompatibilityDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		source  string
		line    int
		message string
	}{
		{
			name:    "method modifier cannot override final method (oracle 112d5af5f3ed8f0d)",
			source:  `<?php class Foo { final function f() {} } class Bar extends Foo { function f() {} }`,
			line:    1,
			message: `Cannot override final method Foo::f()`,
		},
		{
			name:    "concrete class cannot declare abstract method (oracle 168796c2d79fc4cf)",
			source:  "<?php\nclass Test {\n    abstract function f();\n}\n",
			line:    3,
			message: `Class Test declares abstract method f() and must therefore be declared abstract`,
		},
		{
			name:    "magic method parameter type (oracle 055e2959f7138c87)",
			source:  `<?php class Foo { function __get(int $name) {} }`,
			line:    1,
			message: `Foo::__get(): Parameter #1 ($name) must be of type string when declared`,
		},
		{
			name:    "magic method return type (oracle 4d2cdbd3034bd345)",
			source:  "<?php\ntrait T {\n    public function __toString(): int { return 'ok'; }\n}\n",
			line:    3,
			message: `T::__toString(): Return type must be string when declared`,
		},
		{
			name:    "readonly property requires a type (oracle b8cbe4d6c36fd454)",
			source:  "<?php\nreadonly class Foo {\n    public $bar;\n}\n",
			line:    3,
			message: `Readonly property Foo::$bar must have type`,
		},
		{
			name:    "typed property default must match type (oracle 359d0b0b475f13cf)",
			source:  `<?php class Foo { public int $bar = "string"; }`,
			line:    1,
			message: `Cannot use string as default value for property Foo::$bar of type int`,
		},
		{
			name:    "enum cannot declare property (oracle ba37f9379d0922f6)",
			source:  "<?php\nenum Foo {\n    public $bar;\n}\n",
			line:    3,
			message: `Enum Foo cannot include properties`,
		},
		{
			name:    "trait alias cannot add abstract modifier (oracle e6a9ffbafaf64551)",
			source:  "<?php\ntrait T { function f() {} }\nclass C { use T { T::f as abstract; } }\n",
			line:    3,
			message: `Cannot use "abstract" as method modifier in trait alias`,
		},
		{
			name:    "typed constant rejects void (oracle 8887b061b61e7d1a)",
			source:  `<?php class A { public const void C = 1; }`,
			line:    1,
			message: `Class constant A::C cannot have type void`,
		},
		{
			name:    "typed constant value must match type (oracle ef8d755dd2544c79)",
			source:  `<?php class A { public const string C = 1; }`,
			line:    1,
			message: `Cannot use int as value for class constant A::C of type string`,
		},
		{
			name:    "duplicate property declaration (oracle 70533be120f8f65a)",
			source:  "<?php\nclass Test {\n    public $p;\n    public $p;\n}\n",
			line:    4,
			message: `Cannot redeclare Test::$p`,
		},
		{
			name:    "hook setter parameter must match property type (oracle 2b19af986ae71e18)",
			source:  "<?php\nclass Test {\n    public $prop {\n        set(string $prop) {}\n    }\n}\n",
			line:    4,
			message: `Type of parameter $prop of hook Test::$prop::set must be compatible with property type`,
		},
		{
			name:    "hook list must not be empty (oracle 411d521318541f6d)",
			source:  "<?php\nclass Test {\n    public $prop {}\n}\n",
			line:    3,
			message: `Property hook list must not be empty`,
		},
		{
			name:    "attribute target must allow function (oracle 6c6713b108b8270c)",
			source:  "<?php\n#[Attribute]\nfunction f() {}\n",
			line:    3,
			message: `Attribute "Attribute" cannot target function (allowed targets: class)`,
		},
		{
			name:    "same-file override signature must be compatible (oracle a16a31430a41a4ca)",
			source:  "<?php\nclass A { function foo(): A {} }\nclass B extends A { function foo(): stdClass {} }\n",
			line:    3,
			message: `Declaration of B::foo(): stdClass must be compatible with A::foo(): A`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := lowerSource(tc.source, 64)
			var ce *CompileError
			if !errors.As(err, &ce) || ce.Line != tc.line || ce.Message != tc.message {
				t.Fatalf("got %v, want compile error on line %d: %s", err, tc.line, tc.message)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		source string
	}{
		{
			name:   "final method without override",
			source: `<?php class Foo { final function f() {} }`,
		},
		{
			name:   "abstract interface method implemented",
			source: `<?php interface I { public function f(); } class C implements I { public function f() {} }`,
		},
		{
			name:   "magic method with valid declaration",
			source: `<?php class Foo { public function __get(string $name): mixed {} public function __toString(): string { return ''; } }`,
		},
		{
			name:   "readonly typed property initialized by constructor",
			source: `<?php class Foo { public readonly int $bar; function __construct() { $this->bar = 1; } }`,
		},
		{
			name:   "backed enum with cases",
			source: `<?php enum Foo: string { case A = 'a'; }`,
		},
		{
			name:   "trait alias with valid precedence",
			source: `<?php trait T { function f() {} } class C { use T { f as private g; } }`,
		},
		{
			name:   "typed constant with matching value",
			source: `<?php class A { public const int C = 1; }`,
		},
		{
			name:   "distinct class members",
			source: `<?php class Test { public $p; function f() {} }`,
		},
		{
			name:   "typed property hook setter",
			source: `<?php class Test { public string $prop { set(string $value) {} } }`,
		},
		{
			name:   "attribute applied to declared target",
			source: `<?php #[Attribute(Attribute::TARGET_CLASS)] class A {} #[A] class B {}`,
		},
		{
			name:   "compatible same-file inheritance",
			source: `<?php class A { function foo(): A {} } class B extends A { function foo(): B {} }`,
		},
		{
			name:   "conditional declaration defers missing parent resolution",
			source: `<?php if (false) { class ConditionalChild extends MissingParent {} }`,
		},
		{
			name:   "unresolved parent remains a compile-time declaration",
			source: `<?php class Child extends MissingParent {}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := lowerSource(tc.source, 64); err != nil {
				t.Fatalf("unexpected compile error: %v", err)
			}
		})
	}

	t.Run("magic visibility warning is ordered and nonfatal", func(t *testing.T) {
		u := lower(t, `<?php class Bar { private function __invoke() {} }`)
		want := []Diagnostic{{Line: 1, Level: "Warning", Message: "The magic method Bar::__invoke() must have public visibility"}}
		if len(u.Diagnostics) != len(want) || u.Diagnostics[0] != want[0] {
			t.Fatalf("got diagnostics %+v, want %+v", u.Diagnostics, want)
		}
	})
}
