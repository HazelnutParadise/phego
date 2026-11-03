package frontend

import (
	"errors"
	"testing"
)

func TestComplexPostfixReceiversRequireParentheses(t *testing.T) {
	invalid := []struct {
		source string
		line   int
	}{
		{"match($x){default=>1}[0];", 2}, {"match($x){default=>1}->x;", 2},
		{"match($x){default=>1}::X;", 2}, {"function(){}();", 2},
		{"function(){}->x;", 2}, {"<<<E\nx\nE[0];", 4},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.source, func(t *testing.T) {
			_, err := parse(t, "<?php\n"+tc.source)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != tc.line {
				t.Fatalf("expected syntax error on line %d, got %v", tc.line, err)
			}
		})
	}
	valid := []string{"(match($x){default=>1})[0];", "(match($x){default=>1})->x;", "(match($x){default=>1})::X;", "(function(){})();", "(function(){})->x;", "\"hello $x\"[0];", "\"hello $x\"->x;", "\"hello $x\"::X;", "(<<<E\nx\nE)[0];"}
	for _, source := range valid {
		t.Run("valid/"+source, func(t *testing.T) {
			if _, err := parse(t, "<?php "+source); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}
