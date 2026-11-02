package frontend

import (
	"errors"
	"strings"
	"testing"
)

func TestInvalidLegacyOctalIsLocatedWithoutChangingToken(t *testing.T) {
	for _, tc := range []struct {
		literal string
		kind    Kind
	}{
		{"08", TLNumber}, {"09", TLNumber}, {"0_8", TLNumber},
		{"000000000000000000000000000000000000000008", TLNumber},
		{"07777777777777777777777777777777777777778", TDNumber},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			src := []byte("<?php\n" + tc.literal + ";")
			opts := LexOptions{IntWidth: 32, MaxBytes: 1024}
			tokens, diags, err := Lex(src, opts)
			if err != nil || len(tokens) < 2 || tokens[1].Kind != tc.kind || tokens[1].Text != tc.literal {
				t.Fatalf("token changed: %v, %v", tokens, err)
			}
			if len(diags) != 1 || !diags[0].Fatal || diags[0].Line != 2 || !strings.Contains(diags[0].Message, "Invalid numeric literal") {
				t.Fatalf("expected located invalid literal diagnostic, got %v", diags)
			}
			_, err = Parse(src, opts)
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Line != 2 || !strings.Contains(syntax.Message, "Invalid numeric literal") {
				t.Fatalf("expected located parser error, got %v", err)
			}
		})
	}
}

func TestValidOctalAndFloatingPointRemainAccepted(t *testing.T) {
	for _, literal := range []string{"077", "08.0", "08e1", "0o17", "0x89", "0b101"} {
		t.Run(literal, func(t *testing.T) {
			src := []byte("<?php " + literal + ";")
			opts := LexOptions{IntWidth: 32, MaxBytes: 1024}
			_, diagnostics, err := Lex(src, opts)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("unexpected diagnostic: %v %v", diagnostics, err)
			}
			if _, err := Parse(src, opts); err != nil {
				t.Fatalf("valid number rejected: %v", err)
			}
		})
	}
}
