package ir

import "testing"

// Regressions for the PR #129 review: method arity and variance, magic method
// return types, hook backing detection, NaN string conversion, constant
// member names and parenthesized class operands. The fixture records
// php -n -l for both integer widths.
func TestCompatibilityReviewOracle(t *testing.T) {
	checkCompileOracle(t, "testdata/php8512-review-compile.json.gz", 572)
}

// php -n -l cannot show what these operands do at run time. The pinned
// PHP 8.5.12 images throw or warn while executing the names kept here, and
// (A)::f() calls a method of the class named by constant A.
func TestCompatibilityReviewRuntimeOperands(t *testing.T) {
	lower := func(t *testing.T, src string, width int) *Node {
		t.Helper()
		u, err := lowerSource("<?php "+src, width)
		if err != nil {
			t.Fatal(err)
		}
		return firstExpr(t, u)
	}
	for _, tc := range []struct {
		src   string
		width int
		text  string // folded member name, or "" when the name stays an expression
	}{
		{"$o->{[9223372036854775807 => 1, 2]};", 64, ""},
		{"$o->{[9223372036854775807 => 1, 2]};", 32, ""},
		{"$o->{[2147483647 => 1, 2]};", 32, ""},
		{"$o->{[1.5 => 1]};", 64, ""},
		{"$o->{[null => 1]};", 64, ""},
		{"$o->{[(1e999-1e999) => 1]};", 64, ""},
		{"$o?->{[1.5 => 1]};", 64, ""},
		{"$o->{[9223372036854775807 => 1, 2]}();", 64, ""},
		{"A::${[1.5 => 1]};", 64, ""},
		{"$o->{[2147483647 => 1, 2]};", 64, "Array"},
		{"$o->{[...[1, 2]]};", 64, "Array"},
		{"$o->{1e999-1e999};", 32, "NAN"},
		{"A::${1e999-1e999};", 64, "NAN"},
	} {
		n := lower(t, tc.src, tc.width)
		if n.Text != tc.text || (tc.text == "") != (n.Kids[1] != nil) {
			t.Errorf("width=%d %s: name %q, expression %v; want %q", tc.width, tc.src, n.Text, n.Kids[1] != nil, tc.text)
		}
	}
	for _, tc := range []struct {
		src string
		op  Op
	}{
		{"(A)::f();", OpStaticCall},
		{"(A)::$p;", OpStaticProp},
		{"(A)::C;", OpClassConst},
		{"new (A);", OpNew},
		{"(A)::class;", OpClassName},
	} {
		n := lower(t, tc.src, 64)
		var operand *Node
		if len(n.Kids) != 0 {
			operand = n.Kids[0]
		}
		if n.Op != tc.op || n.Name != nil || operand == nil || operand.Op != OpConst || operand.Name.Resolved != "A" {
			t.Errorf("%s: got %s with class name %v and operand %v; want constant A", tc.src, n.Op, n.Name, operand)
		}
	}
	if n := lower(t, "A::f();", 64); n.Name == nil || n.Name.Resolved != "A" || n.Kids[0] != nil {
		t.Errorf("A::f(): got name %v and operand %v", n.Name, n.Kids[0])
	}
	// $x instanceof (A) also reads constant A.
	if n := lower(t, "$x instanceof (A);", 64); n.Op != OpInstanceof || n.Name != nil || n.Kids[1] == nil || n.Kids[1].Op != OpConst {
		t.Errorf("instanceof (A): got name %v and operand %v", n.Name, n.Kids[1])
	}
	// A string the parser holds is a fully qualified class name for ::class.
	u, err := lowerSource("<?php namespace N; ('\\A')::class;", 64)
	if err != nil {
		t.Fatal(err)
	}
	if n := firstExpr(t, u); n.Op != OpLiteral || n.Value.Str != "A" {
		t.Errorf("('\\A')::class: got %s %v", n.Op, n.Value)
	}
	// ${[1]} converts its constant name while compiling.
	if n := lower(t, "${[1]};", 64); n.Op != OpVarVar || n.Kids[0].Op != OpLiteral || n.Kids[0].Value.Str != "Array" {
		t.Errorf("${[1]}: got %s with name %v", n.Op, n.Kids[0])
	}
}
