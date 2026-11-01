package frontend

import (
	"errors"
	"testing"
)

func TestPostfixReceiverSyntaxMatchesPHP(t *testing.T) {
	for _, source := range []string{"1[0];", "1->x;", "1::X;", "2.0[0];", "2.0->x;", "2.0::X;", "__FILE__::X;", "__LINE__::X;"} {
		t.Run("invalid/"+source, func(t *testing.T) {
			_, err := parse(t, "<?php\n"+source)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 {
				t.Fatalf("expected syntax error on line 2, got %v", err)
			}
		})
	}
	for _, source := range []string{"(1)[0];", "(1)->x;", "(1)::X;", "'abc'[0];", "'abc'->x;", "'abc'::X;", "[1][0];", "FOO::X;", "__FILE__[0];", "__FILE__->x;", "foo()[0];", "foo()->x;", "foo()::X;"} {
		t.Run("valid/"+source, func(t *testing.T) {
			if _, err := parse(t, "<?php "+source); err != nil {
				t.Fatalf("valid PHP rejected: %v", err)
			}
		})
	}
}
