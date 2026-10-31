package frontend

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, src string) (*Node, error) {
	t.Helper()
	return Parse([]byte(src), LexOptions{ShortOpenTag: true, IntWidth: 64, MaxBytes: 1 << 22})
}

func TestParseBuildsLocatedTree(t *testing.T) {
	root, err := parse(t, "<?php\nnamespace App;\n\nfinal class A extends B implements C {\n  public function f(int $x = 1): static { return $x ** 2 + 1; }\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 2 || root.Children[0].Kind != "namespace" || root.Children[1].Kind != "class" || root.Children[1].Line != 4 {
		t.Fatalf("unexpected tree: %+v", root.Children)
	}
}

// shape prints operators and their grouping, e.g. (- (** $a 2)).
func shape(n *Node) string {
	if n == nil {
		return "_"
	}
	switch n.Kind {
	case "variable", "literal", "name":
		return n.Text
	case "binary", "unary", "assign":
		parts := []string{n.Text}
		for _, c := range n.Children {
			parts = append(parts, shape(c))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	parts := []string{n.Kind}
	for _, c := range n.Children {
		parts = append(parts, shape(c))
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func TestOperatorPrecedenceMatchesPHP(t *testing.T) {
	for src, want := range map[string]string{
		"-$a ** 2;":           "(- (** $a 2))",
		"!$a instanceof B;":   "(! (instanceof $a B))",
		"$a = $b and $c;":     "(and (= $a $b) $c)",
		"$a . $b + $c;":       "(. $a (+ $b $c))",
		"!$a = $b;":           "(! (= $a $b))",
		"$a ?? $b ?? $c;":     "(?? $a (?? $b $c))",
		"$a |> $f . 'x';":     "(|> $a (. $f 'x'))",
		"print $a and $b;":    "(and (print $a) $b)",
		"$a + $b = 1;":        "(+ $a (= $b 1))",
		"$a ? $b : $c ?: $d;": "(ternary (ternary $a $b $c) _ $d)",
	} {
		root, err := parse(t, "<?php "+src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := shape(root.Children[0].Children[0]); got != want {
			t.Errorf("%s: got %s, want %s", src, got, want)
		}
	}
}

func TestSyntaxErrorsAreLocated(t *testing.T) {
	for src, line := range map[string]int{
		"<?php\n\n1 < 2 < 3;":                    3,
		"<?php\nfunction echo() {}":              2,
		"<?php\n$x = <<<EOT\n  a\n b\n  EOT;":    4,
		"<?php\n(void) $a + 1;\n$b = (void) $a;": 3,
		"<?php\nif (1) {\n":                      3,
		"<?php\nclass A { public public $a; }":   2,
	} {
		_, err := parse(t, src)
		var se *SyntaxError
		if !errors.As(err, &se) || se.Line != line {
			t.Errorf("%q: error %v, want line %d", src, err, line)
		}
	}
}

func TestImportNamesFollowUseGrammar(t *testing.T) {
	// PHP 8.5.12 rejects namespace-relative imports and accepts \X.
	for src, ok := range map[string]bool{
		`<?php use namespace\X;`: false, `<?php use function namespace\f;`: false,
		`<?php use const namespace\C;`: false, `<?php use static;`: false,
		`<?php use \X;`: true, `<?php use A\B as C, function f;`: false, `<?php use A\{B, function f};`: true,
	} {
		_, err := parse(t, src)
		if (err == nil) != ok {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func TestBinaryPrefixedStringsAreCallable(t *testing.T) {
	for _, src := range []string{`<?php b"strlen"("x");`, `<?php B'strlen'("x");`} {
		if _, err := parse(t, src); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func TestDeepNestingIsBoundedNotFatal(t *testing.T) {
	_, err := parse(t, "<?php "+strings.Repeat("(", MaxNesting+10)+"1"+strings.Repeat(")", MaxNesting+10)+";")
	var se *SyntaxError
	if !errors.As(err, &se) || !strings.Contains(se.Message, "nesting") {
		t.Fatalf("deep nesting: %v", err)
	}
	_, err = parse(t, "<?php "+strings.Repeat("$", MaxNesting+10)+"x;")
	if !errors.As(err, &se) || !strings.Contains(se.Message, "nesting") {
		t.Fatalf("deep variable variables: %v", err)
	}
	if _, err := parse(t, "<?php "+strings.Repeat("$", 9000)+"x;"); err != nil {
		t.Fatalf("variable-variable depth PHP accepts was rejected: %v", err)
	}
	if _, err := parse(t, "<?php "+strings.Repeat("~", 7676)+"1;"); err != nil {
		t.Fatalf("depth PHP accepts was rejected: %v", err)
	}
}

func TestLexerTokenLimitCountsFinalStep(t *testing.T) {
	// The halt compiler step emits its trailing inline HTML in the same step.
	src := []byte("<?php __halt_compiler(); data")
	toks, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: 100, MaxTokens: len(toks) - 1}); !errors.Is(err, ErrTooManyTokens) {
		t.Fatalf("limit %d: %v", len(toks)-1, err)
	}
}

func TestLexerDiagnosesUnterminatedString(t *testing.T) {
	for _, src := range []string{"<?php \"abc $x", "<?php `ls", "<?php <<<EOT\nabc $x"} {
		_, diags, err := Lex([]byte(src), LexOptions{IntWidth: 64, MaxBytes: 100})
		if err != nil || len(diags) == 0 {
			t.Errorf("%q: diags %v err %v", src, diags, err)
		}
	}
}

func TestHeredocInterpolationLexesInLinearTime(t *testing.T) {
	small := "<?php $s = <<<EOT\n" + strings.Repeat("$x ", 10000) + "\nEOT;\n"
	large := "<?php $s = <<<EOT\n" + strings.Repeat("$x ", 80000) + "\nEOT;\n"
	measure := func(src string) time.Duration {
		start := time.Now()
		if _, _, err := Lex([]byte(src), LexOptions{IntWidth: 64, MaxBytes: 1 << 22}); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	measure(small)
	a, b := measure(small), measure(large)
	// 8x the input must not take anywhere near 64x the time.
	if b > 30*a+50*time.Millisecond {
		t.Fatalf("lexing looks superlinear: %v for 1x, %v for 8x", a, b)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"<?php class A { public function __construct(public readonly int $x) {} }",
		"<?php $a = fn($x) => match($x) { 1, 2 => 'a', default => throw new E };",
		"<?php foreach ($a as [$k, [$v]]): endforeach;", "<?php enum E: string { case A = 'a'; }",
		"<?php $x = <<<EOT\n  {$a->b[1]}\n  EOT;", "<?php #[A(1)] function &f(...$a): ?static {}"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		_, err := Parse(src, LexOptions{ShortOpenTag: true, IntWidth: 64, MaxBytes: 1 << 16})
		var se *SyntaxError
		if err != nil && !errors.As(err, &se) && !errors.Is(err, ErrInputTooLarge) {
			t.Fatal(err)
		}
		if se != nil && se.Line < 1 {
			t.Fatalf("error without a line: %v", se)
		}
	})
}
