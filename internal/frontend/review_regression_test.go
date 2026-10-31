package frontend

import "testing"

func TestReviewPHPReferenceRegressions(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{"$s = <<<E\n{$a /* \" */}\nE;", true},
		{"$s = <<<E\n{$a /* } { */}\nE;", true},
		{"$s = <<<E\n{$a // }\n}\nE;", true},
		{"new A::$classes[0];", true},
		{"$x instanceof A::$holder->className;", true},
		{"new A::$holder?->className;", true},
		{"unset([$a]);", false},
		{"unset(list($a));", false},
		{"unset([$a][0]);", true},
		{"unset(foo());", true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: got %v; accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
