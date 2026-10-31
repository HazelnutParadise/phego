package frontend

import (
	"errors"
	"testing"
)

func TestRejectEmptyConstructsAndStandaloneParenthesizedIntersection(t *testing.T) {
	for _, source := range []string{"unset();", "#[ ] function f() {}", "function f(): (A&B) {}", "function f((A&B) $x) {}"} {
		t.Run(source, func(t *testing.T) {
			_, err := parse(t, "<?php\n"+source)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 {
				t.Fatalf("expected syntax error on line 2, got %v", err)
			}
		})
	}
	for _, source := range []string{"unset($x);", "unset($x,);", "#[A] function f() {}", "#[A,] function f() {}", "function f(): (A&B)|C {}", "function f(): A&B {}", "function f(): A|(B&C) {}"} {
		t.Run(source, func(t *testing.T) {
			if _, err := parse(t, "<?php "+source); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}
