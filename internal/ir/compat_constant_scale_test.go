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
// 3da26d5..c98cc07 they failed with "constant evaluation resource limit
// exceeded" or, for the alternating chain, took 23 s at depth 9000.
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
		"alternating concat and add": "$x = " + strings.Repeat("(", 9000) + "1" + strings.Repeat(" . '') + 0", 9000) + ";",
		"logical and of variables":   "if (" + join(3000, variable, " && ") + ") { echo 1; }",
		"logical and of literals":    "$x = " + join(3000, func(int) string { return "true" }, " && ") + ";",
		"logical or":                 "$x = " + join(3000, variable, " || ") + ";",
		"large literal array":        "$x = [" + join(70000, func(i int) string { return fmt.Sprint(i) }, ",") + "];",
		"large variable array":       "$x = [" + join(70000, func(int) string { return "$a" }, ",") + "];",
		"constant concat chain":      "const X = " + join(80, segment, " .\n") + ";",
		"array concat chain":         "$x = ['k' => " + join(80, segment, " .\n") + "];",
		"class constant concat":      "class A { const X = " + join(80, segment, " .\n") + "; }",
		"concat left of and":         "if (" + join(80, segment, " .\n") + " && $y) {}",
		"long add chain name":        "$o = new stdClass; $o->{(1e999-1e999)" + strings.Repeat(" + 0", 2500) + "};",
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
