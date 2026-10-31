package frontend

import (
	"errors"
	"testing"
)

func TestReferenceAndIncrementOperandsFollowPHPParsing(t *testing.T) {
	for _, source := range []string{"++1;", "++[];", "++($x);", "$a = &1;", "$a = &($x);", "$a = &new C();", "$a = &[$x];", "$a = &$x = $y;"} {
		t.Run("invalid/"+source, func(t *testing.T) {
			_, err := parse(t, "<?php\n"+source)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 {
				t.Fatalf("expected syntax error on line 2, got %v", err)
			}
		})
	}
	for _, source := range []string{"++$x;", "++foo();", "++$x[0];", "$a = &$x;", "$a = &foo();", "$a = &$x[0];", "$a = &$x + 1;", "++$x?->p;"} {
		t.Run("valid/"+source, func(t *testing.T) {
			if _, err := parse(t, "<?php "+source); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}

func TestReferenceAssignmentPrecedesFollowingAddition(t *testing.T) {
	root, err := parse(t, "<?php $a = &$b + 1;")
	if err != nil {
		t.Fatal(err)
	}
	if got := shape(root.Children[0].Children[0]); got != "(+ (assign_ref $a $b) 1)" {
		t.Fatalf("wrong grouping: %s", got)
	}
}
