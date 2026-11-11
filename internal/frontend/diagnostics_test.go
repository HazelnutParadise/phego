package frontend

import (
	"errors"
	"testing"
)

// These results were observed with PHP 8.5.12 token_get_all(TOKEN_PARSE).
// Keep competing failures together to check which diagnostic wins.
func TestPHP8512SyntaxDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, source, message string
		line                  int
		compile               bool
	}{
		{"anonymous-modifier-line", "<?php new abstract\nfinal class {}; echo ;", "Cannot use the abstract modifier on an anonymous class", 1, true},
		{"trait-set-alias", "<?php class C { use T { f as public\npublic(set); } }", "syntax error, unexpected token \"public(set)\", expecting \";\"", 2, false},
		{"public-bracket", "<?php class C { public public ]", "Multiple access type modifiers are not allowed", 1, true},
		{"class-modifier", "<?php abstract abstract class C {} echo ;", "Multiple abstract modifiers are not allowed", 1, true},
		{"anonymous-modifier", "<?php new abstract class {} ; echo ;", "Cannot use the abstract modifier on an anonymous class", 1, true},
		{"method-modifier", "<?php class C { readonly function f(){} } echo ;", "Cannot use the readonly modifier on a method", 1, true},
		{"constant-modifier", "<?php class C { public public const X=1; } echo ;", "Multiple access type modifiers are not allowed", 1, true},
		{"parameter-modifier", "<?php function f(public public $x) {} echo ;", "Multiple access type modifiers are not allowed", 1, true},
		{"hook-modifier", "<?php class C { public $x { public get; } } echo ;", "Cannot use the public modifier on a property hook", 1, true},
		{"trait-modifier", "<?php class C { use T { f as readonly; } } echo ;", "Cannot use the readonly modifier on a method", 1, true},
		{"identifier-operand", "<?php class C { const echo = (function(){ ?><?= 1; })(); } echo ;", "syntax error, unexpected token \";\"", 1, false},
		{"encoding-later-error", "<?php declare(encoding='UTF-8'); echo ;", "syntax error, unexpected token \";\"", 1, false},
		{"unicode-pending", "<?php class C { public public \"\\u{110000}\"; }", "Multiple access type modifiers are not allowed", 1, true},
		{"nul-string", "<?php echo 1 'A\x00B';", "syntax error, unexpected single-quoted string \"A\"", 1, false},
		{"long-nul-string", "<?php echo 1 'A\x00BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB';", "syntax error, unexpected single-quoted string \"A...\"", 1, false},
		{"binary-string", "<?php echo 1 b\"a\\\\b\";", "syntax error, unexpected quoted string \"b\"a\\\\b\", expecting \",\" or \";\"", 1, false},
		{"caller-class C { public public", "<?php class C { public public <<<X\n$a\n X;", "Multiple access type modifiers are not allowed", 2, true},
		{"expected-expr", "<?php echo ;", "syntax error, unexpected token \";\"", 1, false},
		{"first-syntax", "<?php echo ;\n [", "syntax error, unexpected token \";\"", 1, false},
		{"first-bracket", "<?php echo ) ;\n [", "Unmatched ')'", 1, false},
		{"first-heredoc", "<?php echo ;\n<<<X\n$a\n X;", "syntax error, unexpected token \";\"", 1, false},
		{"modifier-before-syntax", "<?php class C { public public function f() {} }\necho ;", "Multiple access type modifiers are not allowed", 1, true},
		{"syntax-before-modifier", "<?php echo ;\nclass C { public public function f() {} }", "syntax error, unexpected token \";\"", 1, false},
		{"class-before-syntax", "<?php abstract abstract class C {}\necho ;", "Multiple abstract modifiers are not allowed", 1, true},
		{"halt-namespace", "<?php\nnamespace foo {\n __halt_compiler();\n\n?>\n", "Unclosed '{' on line 2", 3, false},
		{"halt-inner", "<?php\nfunction f() {\n __halt_compiler();\n\n?>\n", "__HALT_COMPILER() can only be used from the outermost scope", 3, true},
		{"wrong-variable-token", "<?php\n$x = [200] =& $a[100];", "syntax error, unexpected token \"&\"", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, width := range []int{32, 64} {
				_, err := Parse([]byte(tc.source), LexOptions{IntWidth: width, MaxBytes: 1 << 20})
				var syntax *SyntaxError
				if !errors.As(err, &syntax) {
					t.Fatalf("width %d: expected syntax error, got %v", width, err)
				}
				if syntax.Message != tc.message || syntax.Line != tc.line || syntax.Compile != tc.compile || syntax.CallerLocation {
					t.Errorf("width %d: got %#v, want line %d compile=%t: %s", width, syntax, tc.line, tc.compile, tc.message)
				}
			}
		})
	}
}
