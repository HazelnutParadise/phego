package frontend

import "testing"

func TestHeredocTreePreservesClosingMarkerIndentation(t *testing.T) {
	for _, tc := range []struct{ source, marker string }{
		{"<?php $s = <<<E\n  hello\n  E;", "  E"},
		{"<?php $s = <<<E\n  hello\nE;", "E"},
		{"<?php $s = <<<'E'\n\thello\n\tE;", "\tE"},
	} {
		root, err := parse(t, tc.source)
		if err != nil {
			t.Fatal(err)
		}
		n := root.Children[0].Children[0].Children[1]
		if n.Kind != "heredoc" || len(n.Children) == 0 {
			t.Fatalf("unexpected tree: %+v", n)
		}
		end := n.Children[len(n.Children)-1]
		if end.Kind != "heredoc_end" || end.Text != tc.marker || end.Line != 3 {
			t.Errorf("marker for %q: %+v", tc.source, end)
		}
	}
}

func TestPropertyHookRequiresSingleProperty(t *testing.T) {
	for _, tc := range []struct {
		source   string
		accepted bool
	}{
		{"class C { public $a, $b { get; } }", false},
		{"class C { public int $a, $b { get => 1; } }", false},
		{"class C { public $a, $b; }", true},
		{"class C { public $a { get; } }", true},
		{"class C { public int $a { get => 1; } }", true},
	} {
		_, err := parse(t, "<?php "+tc.source)
		if (err == nil) != tc.accepted {
			t.Errorf("%q: %v, accepted=%t", tc.source, err, tc.accepted)
		}
	}
}
