package frontend

import (
	"errors"
	"strings"
	"testing"
)

func TestPHP8512EncodingDirectiveTiming(t *testing.T) {
	for _, tc := range []struct {
		name, source, message string
		line                  int
		compile               bool
	}{
		{"later-syntax-error", "<?php\ndeclare(encoding=1+1, ticks=);", `syntax error, unexpected token ")"`, 2, false},
		{"later-newline", "<?php\ndeclare(encoding=1+1,\nticks=1);", "Encoding must be a literal", 3, true},
		{"body-next-line", "<?php\ndeclare(encoding=1+1)\n;", "Encoding must be a literal", 2, true},
	} {
		for _, tail := range []string{"", strings.Repeat(";", 80)} {
			t.Run(tc.name+"/"+tail, func(t *testing.T) {
				for _, width := range []int{32, 64} {
					root, err := Parse([]byte(tc.source+tail), LexOptions{IntWidth: width, MaxBytes: 1 << 20})
					var se *SyntaxError
					if !errors.As(err, &se) || se.Message != tc.message || se.Line != tc.line || se.Compile != tc.compile || root != nil {
						t.Fatalf("int%d: got %#v, want line %d compile=%t: %s", width, err, tc.line, tc.compile, tc.message)
					}
				}
			})
		}
	}
}
