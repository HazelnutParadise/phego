package frontend

import "testing"

func TestGroupedImportsAreWholeDeclarations(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{`use A\{B}, C;`, false},
		{`use A, B\{C};`, false},
		{`use A\{B}, C\{D};`, false},
		{`use A\{B, C};`, true},
		{`use A, B;`, true},
		{`use A\{B, function c, const D,};`, true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: %v; PHP accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
