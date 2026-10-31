package frontend

import "testing"

func TestDestructuringAssignmentMatchesPHP(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{"array($a) = $b;", false},
		{"[$a] += $b;", false},
		{"list($a) += $b;", false},
		{"[$a] = &$b;", false},
		{"list($a) = &$b;", false},
		{"[$a] = $b;", true},
		{"list($a) = $b;", true},
		{"array($a)[0] += $b;", true},
		{"[$a][0] = $b;", true},
		{"array($a)->x = $b;", true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: got %v; PHP accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
