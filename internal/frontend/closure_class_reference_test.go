package frontend

import (
	"errors"
	"testing"
)

func TestClosureUseRequiresCapture(t *testing.T) {
	_, err := parse(t, "<?php\n$f = function() use () {};")
	var syntax *SyntaxError
	if !errors.As(err, &syntax) || syntax.Line != 2 {
		t.Fatalf("expected located syntax error: %v", err)
	}
	for _, source := range []string{"$f = function() {};", "$f = function() use ($x) {};", "$f = function() use ($x,) {};"} {
		if _, err := parse(t, "<?php "+source); err != nil {
			t.Fatalf("valid PHP rejected %q: %v", source, err)
		}
	}
}

func TestDynamicClassReferencePreservesNullsafeAccess(t *testing.T) {
	for _, tc := range []struct {
		source, want string
		child        int
	}{
		{"$x instanceof $o?->className;", "nullsafe_prop", 1},
		{"$x instanceof $o->className;", "prop", 1},
		{"new $o?->className;", "nullsafe_prop", 0},
		{"new $o->className;", "prop", 0},
	} {
		t.Run(tc.source, func(t *testing.T) {
			root, err := parse(t, "<?php "+tc.source)
			if err != nil {
				t.Fatal(err)
			}
			expr := root.Children[0].Children[0]
			if got := expr.Children[tc.child].Kind; got != tc.want {
				t.Fatalf("class reference: got %s, want %s", got, tc.want)
			}
		})
	}
}
