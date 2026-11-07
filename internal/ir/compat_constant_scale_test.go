package ir

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// PHP 8.5.12 compiles each of these in linear time (php -n -l reports no
// errors for all of them). Constant evaluation must neither reject them as a
// host limit nor re-evaluate nested operands for every enclosing probe. On
// 3da26d5..0ba11a8 they failed with "constant evaluation resource limit
// exceeded" or took more than 10 s (for example 23 s for the alternating
// concatenation chain on c98cc07, and 11.7 s for 700 nested closures around
// a 200,000-element array on a828a53).
func TestConstantEvaluationScalesLinearly(t *testing.T) {
	join := func(n int, item func(int) string, sep string) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = item(i)
		}
		return strings.Join(parts, sep)
	}
	variable := func(i int) string { return fmt.Sprintf("$a%d", i) }
	segment := func(int) string { return "'" + strings.Repeat("q", 60) + "'" }
	cases := map[string]string{
		"alternating concat and add":   "$x = " + strings.Repeat("(", 9000) + "1" + strings.Repeat(" . '') + 0", 9000) + ";",
		"logical and of variables":     "if (" + join(3000, variable, " && ") + ") { echo 1; }",
		"logical and of literals":      "$x = " + join(3000, func(int) string { return "true" }, " && ") + ";",
		"logical or":                   "$x = " + join(3000, variable, " || ") + ";",
		"large literal array":          "$x = [" + join(70000, func(i int) string { return fmt.Sprint(i) }, ",") + "];",
		"large variable array":         "$x = [" + join(70000, func(int) string { return "$a" }, ",") + "];",
		"constant concat chain":        "const X = " + join(80, segment, " .\n") + ";",
		"array concat chain":           "$x = ['k' => " + join(80, segment, " .\n") + "];",
		"class constant concat":        "class A { const X = " + join(80, segment, " .\n") + "; }",
		"concat left of and":           "if (" + join(80, segment, " .\n") + " && $y) {}",
		"long add chain name":          "$o = new stdClass; $o->{(1e999-1e999)" + strings.Repeat(" + 0", 2500) + "};",
		"alternating logical and add":  "const X = " + strings.Repeat("(", 3000) + "1" + strings.Repeat(" && 1) + 1)", 1500) + ";",
		"short ternary chain":          "const X = " + strings.Repeat("0 ?: ", 3000) + "1;",
		"unary chain name":             "$o = new stdClass; $o->{" + strings.Repeat("!", 3000) + "1} = 1;",
		"unary chain left of and":      "if (" + strings.Repeat("!", 3000) + "1 && $z) {}",
		"union member name":            "$o = new stdClass; $o->{" + join(300, func(i int) string { return fmt.Sprintf("[%d=>%d]", i, i) }, " + ") + "} = 1;",
		"nested spread":                "$x = " + strings.Repeat("[...", 40) + "[" + join(1000, func(i int) string { return fmt.Sprint(i) }, ",") + "]" + strings.Repeat("]", 40) + ";",
		"nested closures and array":    "$f = " + strings.Repeat("function () { return ", 700) + "[" + join(200000, func(i int) string { return fmt.Sprint(i) }, ",") + "]" + strings.Repeat("; }", 700) + ";",
		"parse-time class name concat": "function f() { return (" + join(50000, segment, " . ") + ")::f(); }",
		"coalesce chain":               "$a = [" + join(3000, variable, " ?? ") + "];",
		"power chain":                  "$a = [" + join(3000, func(int) string { return "1" }, " ** ") + "];",
		"many union constants": join(30, func(k int) string {
			return fmt.Sprintf("const C%d = %s;", k, join(256, func(i int) string { return fmt.Sprintf("['c%d_%d' => 1]", k, i) }, " + "))
		}, "\n"),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			for _, width := range []int{32, 64} {
				start := time.Now()
				if _, err := lowerSource("<?php\n"+src, width); err != nil {
					t.Fatalf("int%d: %v", width, err)
				}
				if elapsed := time.Since(start); elapsed > 10*time.Second {
					t.Fatalf("int%d: took %v", width, elapsed)
				}
			}
		})
	}
}

// The alternating chain above is the shape whose probes were quadratic; it
// must also emit PHP's compile-time warning for a NaN operand exactly once.
func TestConstantLongChainKeepsNaNWarning(t *testing.T) {
	u, err := lowerSource("<?php\n$o = new stdClass;\n$o->{(1e999-1e999)"+strings.Repeat(" + 0", 2500)+"};", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Diagnostics) != 1 || u.Diagnostics[0].Message != "unexpected NAN value was coerced to string" || u.Diagnostics[0].Line != 3 {
		t.Fatalf("got %+v", u.Diagnostics)
	}
}
