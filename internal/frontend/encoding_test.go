package frontend

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// PHP 8.5.12 classifies literals while constructing its syntax tree, before
// general constant evaluation. Both pinned integer-width images agree.
func TestPHP8512EncodingLiterals(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		literal          bool
	}{
		{"string", `"x"`, true},
		{"parentheses", `(("x"))`, true},
		{"integer", `(1)`, true},
		{"float", `1.5`, true},
		{"infinity", `1e999`, true},
		{"concat", `"x" . "y"`, true},
		{"numeric-concat", `1 . 2`, true},
		{"nested-concat", `("x" . (1 . 2.5)) . "y"`, true},
		{"heredoc", "<<<X\nx\nX", true},
		{"empty-heredoc", "<<<X\nX", true},
		{"nowdoc", "<<<'X'\nx\nX", true},
		{"empty-nowdoc", "<<<'X'\nX", true},
		{"heredoc-concat", "(<<<X\nx\nX) . \"y\"", true},
		{"long-concat", strings.Repeat(`"x" . `, 10000) + `"y"`, true},
		{"positive", `+1`, false},
		{"negative", `-1`, false},
		{"arithmetic", `1+1`, false},
		{"cast", `(string)1`, false},
		{"true", `true`, false},
		{"false", `false`, false},
		{"null", `null`, false},
		{"constant", `NAME`, false},
		{"magic", `__FILE__`, false},
		{"class", `Foo::class`, false},
		{"array", `[]`, false},
		{"interpolation", `"x$y"`, false},
		{"heredoc-interpolation", "<<<X\nx$y\nX", false},
		{"backtick", "`x`", false},
		{"concat-bool", `"x" . true`, false},
		{"concat-unary", `"x" . +1`, false},
	} {
		for _, tail := range []struct {
			name, source string
			deep         bool
		}{
			{"short", "", false},
			{"replay", strings.Repeat(";", 80), false},
			{"exhaustion", strings.Repeat("if(1)", 2500) + ";", true},
		} {
			t.Run(tc.name+"/"+tail.name, func(t *testing.T) {
				source := "<?php\ndeclare(encoding=" + tc.expression + ");\n" + tail.source
				for _, width := range []int{32, 64} {
					root, err := Parse([]byte(source), LexOptions{IntWidth: width, MaxBytes: 1 << 20})
					if tc.literal && !tail.deep {
						if err != nil || root == nil {
							t.Fatalf("int%d: wanted a parsed tree, got %v", width, err)
						}
						continue
					}
					line := 2 + strings.Count(tc.expression, "\n")
					message, compile := "Encoding must be a literal", true
					if tc.literal {
						line++
						message, compile = "memory exhausted", false
					}
					var se *SyntaxError
					if !errors.As(err, &se) || se.Line != line || se.Message != message || se.Compile != compile || root != nil {
						t.Fatalf("int%d: got %#v, want %s", width, err, fmt.Sprintf("line %d compile=%t: %s", line, compile, message))
					}
				}
			})
		}
	}
}
