package frontend

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Boundaries and error priority observed with token_get_all(TOKEN_PARSE) in
// both pinned PHP 8.5.12 reference images. Successful parses must produce ASTs.
func TestPHP8512NestingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		source func(int) string
	}{
		{"parentheses", 9995, func(n int) string { return strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + ";" }},
		{"unary", 9996, func(n int) string { return strings.Repeat("~", n) + "1;" }},
		{"variables", 9997, func(n int) string { return strings.Repeat("$", n) + "x;" }},
		{"if", 2499, func(n int) string { return strings.Repeat("if(1)", n) + ";" }},
		{"closures", 768, func(n int) string {
			return strings.Repeat("(function(){return ", n) + "1" + strings.Repeat(";})()", n) + ";"
		}},
		{"mixed", 833, func(n int) string { return strings.Repeat("if(1){while(1){", n) + ";" + strings.Repeat("}}", n) }},
	} {
		for _, depth := range []int{tc.limit - 1, tc.limit, tc.limit + 1} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, depth), func(t *testing.T) {
				for _, width := range []int{32, 64} {
					root, err := Parse([]byte("<?php\n"+tc.source(depth)), LexOptions{IntWidth: width, MaxBytes: 1 << 20})
					if depth <= tc.limit {
						if err != nil || root == nil || root.Kind != "file" || len(root.Children) == 0 {
							t.Fatalf("int%d: accepted source did not produce a tree: %v", width, err)
						}
						continue
					}
					var se *SyntaxError
					if !errors.As(err, &se) || se.Message != "memory exhausted" || se.Line != 2 || se.Compile || root != nil {
						t.Fatalf("int%d: expected PHP parser exhaustion on line 2: %#v", width, err)
					}
				}
			})
		}
	}
}

func TestPHP8512NestingErrorPriority(t *testing.T) {
	deep := strings.Repeat("if(1)", 2500) + ";"
	for _, tc := range []struct {
		name, prefix, suffix, message string
		line                          int
		compile                       bool
	}{
		{"valid-encoding", "declare(encoding=\"x\");\n", "", "memory exhausted", 3, false},
		{"encoding-before", "declare(encoding=1+1);\n", "", "Encoding must be a literal", 2, true},
		{"encoding-after", "", "\ndeclare(encoding=1+1);", "memory exhausted", 2, false},
		{"modifier-before", "class C { public public $x; }\n", "", "Multiple access type modifiers are not allowed", 2, true},
		{"modifier-after", "", "\nclass C { public public $x; }", "memory exhausted", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{32, 64} {
				root, err := Parse([]byte("<?php\n"+tc.prefix+deep+tc.suffix), LexOptions{IntWidth: width, MaxBytes: 1 << 20})
				var se *SyntaxError
				if !errors.As(err, &se) || se.Message != tc.message || se.Line != tc.line || se.Compile != tc.compile || root != nil {
					t.Fatalf("int%d: got %#v, want line %d compile=%t: %s", width, err, tc.line, tc.compile, tc.message)
				}
			}
		})
	}
}
