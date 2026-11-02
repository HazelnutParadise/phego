package frontend

import "testing"

func TestAttributedExpressionsContinueOperators(t *testing.T) {
	for _, src := range []string{
		"#[A] function() {} instanceof Closure;",
		"#[A] static function() {} instanceof Closure;",
		"#[A] fn() => 1 instanceof Closure;",
		"#[A] function() {} ? 1 : 2;",
	} {
		if _, err := parse(t, "<?php "+src); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestForClausesRejectTrailingComma(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{"for ($x=0,; ; ) {}", false},
		{"for (; $x,; ) {}", false},
		{"for (;; $x,) {}", false},
		{"for (;;) {}", true},
		{"for ($x=0,$y=0; $x<1,$y<2; $x++,$y++) {}", true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: %v; accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
