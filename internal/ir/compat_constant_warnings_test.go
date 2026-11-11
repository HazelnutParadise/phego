package ir

import "testing"

func TestCompatibilityConstantWarnings(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		lines        []int
	}{
		{"const-initial-line", "<?php\nconst X = ((-1)**0.5) ? 1 : f();", []int{2}},
		{"const-next-item-line", "<?php\nconst X =\n1,\nY =\n((-1)**0.5) ? 1 : 2;", []int{2}},
		{"class-const-next-item-line", "<?php\nclass C {\nconst X =\n1,\nY =\n((-1)**0.5) ? 1 : 2;}", []int{3}},
		{"property-next-item-line", "<?php\nclass C {\npublic $x =\n1,\n$y =\n((-1)**0.5) ? 1 : 2;}", []int{3}},
		{"static-initializer", "<?php\nfunction f(){\nstatic $x = ((-1)**0.5) ? 1 : 2;}", []int{3}},
		{"static-dynamic-initializer", "<?php\nfunction f(){\nstatic $x = f();}", nil},
		{"case00", "<?php\n(1e999-1e999) || true;", []int{2, 2}},
		{"case00multiline", "<?php\n\n(1e999\n-1e999) || true;", []int{4, 4}},
		{"case01", "<?php\n(1e999-1e999) && true;", []int{2}},
		{"case01multiline", "<?php\n\n(1e999\n-1e999) && true;", []int{4}},
		{"case02", "<?php\nfalse || (1e999-1e999);", []int{2}},
		{"case02multiline", "<?php\n\nfalse || (1e999\n-1e999);", []int{4}},
		{"case03", "<?php\ntrue && (1e999-1e999);", []int{2}},
		{"case03multiline", "<?php\n\ntrue && (1e999\n-1e999);", []int{4}},
		{"case04", "<?php\n$x && (1e999-1e999);", []int{}},
		{"case04multiline", "<?php\n\n$x && (1e999\n-1e999);", []int{}},
		{"case05", "<?php\n(1e999-1e999) xor false;", []int{2}},
		{"case05multiline", "<?php\n\n(1e999\n-1e999) xor false;", []int{4}},
		{"case06", "<?php\n(1e999-1e999) ? 1 : 2;", []int{}},
		{"case06multiline", "<?php\n\n(1e999\n-1e999) ? 1 : 2;", []int{}},
		{"case07", "<?php\n[(1e999-1e999) || true];", []int{2}},
		{"case07multiline", "<?php\n\n[(1e999\n-1e999) || true];", []int{3}},
		{"case08", "<?php\n[(1e999-1e999) && true];", []int{2}},
		{"case08multiline", "<?php\n\n[(1e999\n-1e999) && true];", []int{3}},
		{"case09", "<?php\n[false || (1e999-1e999)];", []int{2}},
		{"case09multiline", "<?php\n\n[false || (1e999\n-1e999)];", []int{3}},
		{"case10", "<?php\n[true && (1e999-1e999)];", []int{2}},
		{"case10multiline", "<?php\n\n[true && (1e999\n-1e999)];", []int{3}},
		{"case11", "<?php\n[(1e999-1e999) ? 1 : 2];", []int{2}},
		{"case11multiline", "<?php\n\n[(1e999\n-1e999) ? 1 : 2];", []int{3}},
		{"case12", "<?php\n[(1e999-1e999) ?: 2];", []int{2}},
		{"case12multiline", "<?php\n\n[(1e999\n-1e999) ?: 2];", []int{3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{32, 64} {
				u, err := lowerSource(tc.source, width)
				if err != nil {
					t.Fatal(err)
				}
				if len(u.Diagnostics) != len(tc.lines) {
					t.Fatalf("int%d: got %#v; want lines %v", width, u.Diagnostics, tc.lines)
				}
				for i, line := range tc.lines {
					d := u.Diagnostics[i]
					if d.Line != line || d.Level != "Warning" || d.Message != "unexpected NAN value was coerced to bool" {
						t.Fatalf("int%d: warning%d %#v", width, i, d)
					}
				}
			}
		})
	}
}
