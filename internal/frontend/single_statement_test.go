package frontend

import (
	"errors"
	"testing"
)

func TestSingleStatementBodiesRejectDeclarations(t *testing.T) {
	for _, tc := range []struct {
		source   string
		line     int
		accepted bool
	}{
		{"if(true)\nfunction f() {}", 2, false},
		{"while(false)\nclass C {}", 2, false},
		{"if(true)\n#[A]\nfunction f() {}", 3, false},
		{"declare(ticks=1) function f() {}", 1, false},
		{"if(true) function() {};", 0, true},
		{"if(true) { function f() {} }", 0, true},
		{"if(true): function f() {} endif;", 0, true},
		{"switch(0) { case 0: function f() {} }", 0, true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if tc.accepted && err != nil {
			t.Errorf("%q: %v", tc.source, err)
		}
		if !tc.accepted {
			var se *SyntaxError
			if !errors.As(err, &se) || se.Line != tc.line {
				t.Errorf("%q: error=%v, want line %d", tc.source, err, tc.line)
			}
		}
	}
}

func TestAttributesMustFollowNewForAnonymousClasses(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{"#[A] new C();", false},
		{"#[A] new class {};", false},
		{"new #[A] class {};", true},
		{"#[A] function() {};", true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: %v; accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
