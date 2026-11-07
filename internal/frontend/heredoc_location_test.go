package frontend

import (
	"errors"
	"strings"
	"testing"
)

// PHP's token_get_all() resolves a first-token interpolation indentation
// failure from the caller, before parsing the interpolation. Source anchors
// here are our editor location contract, independently of PHP's caller line.
func TestHeredocCallerLocation(t *testing.T) {
	for _, tc := range []struct {
		body, marker string
		caller       bool
		line         int
		message      string
	}{
		{"$a", " X", true, 4, "Invalid body indentation level"},
		{"{$a}", " X", true, 4, "Invalid body indentation level"},
		{"${a}", " X", true, 4, "Invalid body indentation level"},
		{"{$a + }", " X", true, 4, "Invalid body indentation level"},
		{"${1+}", " X", true, 4, "Invalid body indentation level"},
		{"\n$a", " X", false, 4, "Invalid body indentation level"},
		{" text\n$a", " X", false, 4, "Invalid body indentation level"},
		{"text", " X", false, 3, "Invalid body indentation level"},
		{"{$a + }", "X", false, 3, `unexpected token "+"`},
	} {
		t.Run(tc.body+tc.marker, func(t *testing.T) {
			src := "<?php\n<<<X\n" + tc.body + "\n" + tc.marker + ";"
			_, err := parse(t, src)
			var se *SyntaxError
			if !errors.As(err, &se) || se.CallerLocation != tc.caller || se.Line != tc.line || !strings.Contains(se.Message, tc.message) {
				t.Fatalf("error = %#v, want caller=%t source line=%d message=%q", err, tc.caller, tc.line, tc.message)
			}
		})
	}
}

func TestHeredocCallerDiagnosticPrecedesBodySyntax(t *testing.T) {
	src := []byte("<?php\n<<<X\n{$a + }\n X;")
	tokens, diagnostics, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diagnostics {
		if d.CallerLocation {
			if !d.Fatal || d.Line != 4 || d.Token < 0 || d.Token >= len(tokens) || tokens[d.Token].Kind != TStartHeredoc {
				t.Fatalf("invalid caller diagnostic: %+v", d)
			}
			return
		}
	}
	t.Fatalf("missing caller diagnostic: %+v", diagnostics)
}
