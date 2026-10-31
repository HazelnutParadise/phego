package frontend

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

func nestedHeredoc(depth int) string {
	return "<?php $s = " + strings.Repeat("<<<E\n{$a[", depth) + "'x'" + strings.Repeat("]}\nE\n", depth) + ";"
}

func TestNestedHeredocTokensMatchPHP(t *testing.T) {
	src := nestedHeredoc(3)
	tokens := lex(t, src)
	want := "T_OPEN_TAG T_VARIABLE T_WHITESPACE = T_WHITESPACE T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ " +
		"T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ " +
		"T_CONSTANT_ENCAPSED_STRING ] } T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC T_WHITESPACE " +
		"] } T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC T_WHITESPACE ] } T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC T_WHITESPACE ;"
	if got := kinds(tokens); got != want {
		t.Fatalf("tokens differ: %s", got)
	}
	var out strings.Builder
	for _, token := range tokens {
		out.WriteString(token.Text)
	}
	if out.String() != src {
		t.Fatal("tokens do not preserve source")
	}
	if _, err := parse(t, src); err != nil {
		t.Fatalf("PHP accepts nested heredoc: %v", err)
	}
}

func TestNestedHeredocDoesNotRescanQuadratically(t *testing.T) {
	measure := func(depth int) time.Duration {
		src := []byte(nestedHeredoc(depth))
		start := time.Now()
		if _, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: 1 << 22}); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	measure(500)
	small := measure(1000)
	large := measure(10000)
	if large > 30*small+100*time.Millisecond {
		t.Fatalf("nested heredoc scanning scales superlinearly: 1000=%v 10000=%v", small, large)
	}
}

func TestBraceCacheDoesNotTreatUnindexedBraceAsMatched(t *testing.T) {
	l := &lexer{src: "{}'{}"}
	if got := l.skipBraces(0); got != 1 {
		t.Fatalf("first brace: got %d", got)
	}
	if got := l.skipBraces(3); got != 4 {
		t.Fatalf("brace skipped while indexing: got %d, want 4", got)
	}
}

func TestBraceCacheMatchesOriginalScanner(t *testing.T) {
	reference := func(src string, start int) int {
		depth := 0
		for i := start; i < len(src); i++ {
			switch c := src[i]; c {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return i
				}
			case '\'', '"':
				for i++; i < len(src) && src[i] != c; i++ {
					if src[i] == '\\' {
						i++
					}
				}
			}
		}
		return len(src)
	}
	rng := rand.New(rand.NewSource(20261031))
	alphabet := []byte("{}abc'\"\\$\n")
	for trial := 0; trial < 2000; trial++ {
		data := make([]byte, rng.Intn(120)+1)
		for i := range data {
			data[i] = alphabet[rng.Intn(len(alphabet))]
		}
		src := string(data)
		l := &lexer{src: src}
		if start := strings.IndexByte(src, '{'); start >= 0 {
			l.skipBraces(start)
		}
		for i := len(src) - 1; i >= 0; i-- {
			if src[i] != '{' {
				continue
			}
			if got, want := l.skipBraces(i), reference(src, i); got != want {
				t.Fatalf("source %q start %d: got %d, want %d", src, i, got, want)
			}
		}
	}
}

func TestNestedHeredocAfterQuotedTextDoesNotRescanQuadratically(t *testing.T) {
	source := func(depth int) string {
		return "<?php $q = <<<Q\n{$x}\n'\nQ;\n" + strings.TrimPrefix(nestedHeredoc(depth), "<?php ")
	}
	if _, err := parse(t, source(3)); err != nil {
		t.Fatalf("PHP accepts nested heredoc after quoted text: %v", err)
	}
	want := "T_OPEN_TAG T_VARIABLE T_WHITESPACE = T_WHITESPACE T_START_HEREDOC T_CURLY_OPEN T_VARIABLE } " +
		"T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC ; T_WHITESPACE T_VARIABLE T_WHITESPACE = T_WHITESPACE " +
		"T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ " +
		"T_START_HEREDOC T_CURLY_OPEN T_VARIABLE [ T_CONSTANT_ENCAPSED_STRING ] } " +
		"T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC T_WHITESPACE ] } T_ENCAPSED_AND_WHITESPACE " +
		"T_END_HEREDOC T_WHITESPACE ] } T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC T_WHITESPACE ;"
	if got := kinds(lex(t, source(3))); got != want {
		t.Fatalf("PHP token sequence differs: %s", got)
	}
	measure := func(depth int) time.Duration {
		src := []byte(source(depth))
		start := time.Now()
		if _, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: 1 << 22}); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	measure(500)
	small, large := measure(1000), measure(10000)
	if large > 30*small+100*time.Millisecond {
		t.Fatalf("nested heredoc after quoted text scales superlinearly: 1000=%v 10000=%v", small, large)
	}
}
