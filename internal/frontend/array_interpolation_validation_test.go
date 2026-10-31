package frontend

import (
	"errors"
	"testing"
)

func TestArrayReferencesAndCurlyInterpolationRequireVariables(t *testing.T) {
	invalid := []string{`[&1];`, `["k"=>&1];`, `[&$a ** 2];`, `"{$a + 1}";`, `"{$a++}";`, `"{$a::C}";`}
	for _, source := range invalid {
		t.Run("invalid/"+source, func(t *testing.T) {
			_, err := parse(t, "<?php\n"+source)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 {
				t.Fatalf("expected syntax error on line 2, got %v", err)
			}
		})
	}
	valid := []string{`[&$a];`, `["k"=>&$a];`, `[&foo()];`, `[&$a[1+2]];`, `"{$a}";`, `"{$a[1+2]}";`, `"{$a->foo()}";`, `"{$a::foo()}";`, `"{$a[foo()]}";`}
	for _, source := range valid {
		t.Run("valid/"+source, func(t *testing.T) {
			if _, err := parse(t, "<?php "+source); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}
