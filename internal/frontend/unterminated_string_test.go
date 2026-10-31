package frontend

import (
	"strings"
	"testing"
)

func TestUnterminatedStringsHaveLocatedDiagnostic(t *testing.T) {
	for _, src := range []string{
		"<?php\n'unfinished", "<?php\n\"$x", "<?php\n<<<E\nunfinished",
		"<?php\n<<<'E'\nunfinished", "<?php\n\"{$x", "<?php\n<<<E\n{$x",
	} {
		t.Run(src, func(t *testing.T) {
			tokens, diags, err := Lex([]byte(src), LexOptions{IntWidth: 64, MaxBytes: 1024})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range diags {
				if strings.Contains(strings.ToLower(d.Message), "unterminated") && d.Line >= 2 {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing located unterminated diagnostic: %v", diags)
			}
			var out strings.Builder
			for _, token := range tokens {
				out.WriteString(token.Text)
			}
			if out.String() != src {
				t.Fatalf("token text changed: %q", out.String())
			}
		})
	}
	for _, src := range []string{"<?php 'complete';", "<?php \"$x\";", "<?php <<<E\ncomplete\nE;"} {
		_, diags, err := Lex([]byte(src), LexOptions{IntWidth: 64, MaxBytes: 1024})
		if err != nil || len(diags) != 0 {
			t.Fatalf("valid string diagnosed: %v %v", diags, err)
		}
	}
}
