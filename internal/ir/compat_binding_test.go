package ir

import "testing"

// These declarations are accepted by PHP 8.5.12 php -l: their inheritance
// checks cannot run until the declaration is executed and dependencies bind.
func TestCompatibilityDeferredClassBinding(t *testing.T) {
	for name, source := range map[string]string{
		"trait parent":              `<?php trait T {} class A { use T; final function f() {} } class B extends A { function f() {} }`,
		"interface parent":          `<?php interface I {} class A implements I { final function f() {} } class B extends A { function f() {} }`,
		"unresolved parent":         `<?php class A extends Missing { final function f() {} } class B extends A { function f() {} }`,
		"interface implementation":  `<?php interface I { function f(?int $x); } class A implements I { function f(int $x) {} }`,
		"unknown variance":          `<?php class A { function f(): X {} } class B extends A { function f(): Y {} final function g() {} } class C extends B { function g() {} }`,
		"unknown property variance": `<?php class A { public X $p; } class B extends A { public Y $p; final function f() {} } class C extends B { function f() {} }`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := lowerSource(source, 64); err != nil {
				t.Fatalf("PHP accepts compilation; got %v", err)
			}
		})
	}
}

func TestCompatibilityDeclarationLocations(t *testing.T) {
	for name, tc := range map[string]struct {
		source  string
		line    int
		message string
	}{
		"method": {"<?php\nclass C {\n#[Override]\npublic\nfunction\nf() {}\n}", 5, `C::f() has #[\Override] attribute, but no matching parent method exists`},
		"hook":   {"<?php\nclass C {\npublic $p {\n#[Override]\nget => 1;\n}\n}", 5, `C::$p::get() has #[\Override] attribute, but no matching parent method exists`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := lowerSource(tc.source, 64)
			ce, ok := err.(*CompileError)
			if !ok || ce.Line != tc.line || ce.Message != tc.message {
				t.Fatalf("got %v, want line%d %s", err, tc.line, tc.message)
			}
		})
	}
}
