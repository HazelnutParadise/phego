package frontend

import (
	"errors"
	"testing"
)

func TestForeachTargetsFollowPHPParsing(t *testing.T) {
	for _, target := range []string{"1", "(1)", "($x)", "$x + 1", "array($x)", "C::K", "$x++", "++$x", "$x = 1", "&[$x]", "&list($x)", "new C()", "null", "1 => $y"} {
		t.Run("invalid/"+target, func(t *testing.T) {
			_, err := parse(t, "<?php\nforeach ($a as "+target+") {}")
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 {
				t.Fatalf("expected syntax error on line 2, got %v", err)
			}
		})
	}
	for _, target := range []string{"$x", "$x[0]", "$o->p", "C::$x", "[$x]", "list($x)", "&$x", "foo()", "$x => $y", "&$x => &$y", "$x?->p", "[1]"} {
		t.Run("valid/"+target, func(t *testing.T) {
			if _, err := parse(t, "<?php foreach ($a as "+target+") {}"); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}
