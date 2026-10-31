package frontend

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func lex(t *testing.T, src string, opts ...func(*LexOptions)) []Token {
	t.Helper()
	o := LexOptions{ShortOpenTag: true, IntWidth: 64, MaxBytes: 1 << 20}
	for _, f := range opts {
		f(&o)
	}
	toks, _, err := Lex([]byte(src), o)
	if err != nil {
		t.Fatal(err)
	}
	return toks
}

func kinds(toks []Token) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Kind.String())
	}
	return b.String()
}

func TestTemplateAndTags(t *testing.T) {
	got := kinds(lex(t, "<p><?= $x ?>\n</p><?php echo 1;"))
	want := "T_INLINE_HTML T_OPEN_TAG_WITH_ECHO T_WHITESPACE T_VARIABLE T_WHITESPACE T_CLOSE_TAG T_INLINE_HTML T_OPEN_TAG T_ECHO T_WHITESPACE T_LNUMBER ;"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	noShort := func(o *LexOptions) { o.ShortOpenTag = false }
	if got := kinds(lex(t, "<? echo 1;", noShort)); got != "T_INLINE_HTML" {
		t.Fatalf("short tag disabled: %s", got)
	}
}

func TestIntegerWidthDecidesNumberKind(t *testing.T) {
	for _, c := range []struct {
		src   string
		width int
		want  Kind
	}{
		{"2147483647", 32, TLNumber}, {"2147483648", 32, TDNumber}, {"2147483648", 64, TLNumber},
		{"9223372036854775807", 64, TLNumber}, {"9223372036854775808", 64, TDNumber},
		{"0x7fffffff", 32, TLNumber}, {"0x80000000", 32, TDNumber}, {"0b1_01", 64, TLNumber},
		{"0o17", 64, TLNumber}, {"017", 64, TLNumber}, {"1_000.5e3", 64, TDNumber}, {".5", 64, TDNumber},
	} {
		w := func(o *LexOptions) { o.IntWidth = c.width }
		toks := lex(t, "<?php "+c.src, w)
		if len(toks) != 2 || toks[1].Kind != c.want || toks[1].Text != c.src {
			t.Errorf("%s/%d: %v", c.src, c.width, toks)
		}
	}
}

func TestInterpolationAndHeredoc(t *testing.T) {
	got := kinds(lex(t, "<?php \"a $b[0] {$c->d} ${e} $f->g\";\n$x = <<<EOT\n  v $y\n  EOT;\n"))
	want := `T_OPEN_TAG " T_ENCAPSED_AND_WHITESPACE T_VARIABLE [ T_NUM_STRING ] T_ENCAPSED_AND_WHITESPACE ` +
		`T_CURLY_OPEN T_VARIABLE T_OBJECT_OPERATOR T_STRING } T_ENCAPSED_AND_WHITESPACE ` +
		`T_DOLLAR_OPEN_CURLY_BRACES T_STRING_VARNAME } T_ENCAPSED_AND_WHITESPACE T_VARIABLE T_OBJECT_OPERATOR T_STRING " ; ` +
		`T_WHITESPACE T_VARIABLE T_WHITESPACE = T_WHITESPACE T_START_HEREDOC T_ENCAPSED_AND_WHITESPACE T_VARIABLE ` +
		`T_ENCAPSED_AND_WHITESPACE T_END_HEREDOC ; T_WHITESPACE`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestEveryByteBelongsToOneTokenAndLinesAdvance(t *testing.T) {
	src := "<?php\n/* a\r\nb */\r$x = 'unterminated\n"
	toks := lex(t, src)
	var b strings.Builder
	for _, tok := range toks {
		b.WriteString(tok.Text)
	}
	if b.String() != src {
		t.Fatalf("tokens do not reconstruct the source: %q", b.String())
	}
	if last := toks[len(toks)-1]; last.Line != 4 || last.Kind != TEncapsedAndWhitespace {
		t.Fatalf("last token %v", last)
	}
}

func TestLimitsAreCheckedBeforeLexing(t *testing.T) {
	if _, _, err := Lex([]byte("<?php 1;"), LexOptions{IntWidth: 64, MaxBytes: 4}); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	if _, _, err := Lex([]byte("<?php 1;"), LexOptions{IntWidth: 64, MaxBytes: 100, MaxTokens: 2}); !errors.Is(err, ErrTooManyTokens) {
		t.Fatalf("token limit: %v", err)
	}
	if _, _, err := Lex(nil, LexOptions{IntWidth: 16, MaxBytes: 1}); err == nil {
		t.Fatal("invalid width accepted")
	}
	if toks, _, err := Lex(nil, LexOptions{IntWidth: 32, MaxBytes: 1}); err != nil || len(toks) != 0 {
		t.Fatalf("empty source: %v %v", toks, err)
	}
}

func TestUnterminatedCommentIsDiagnosedWithLine(t *testing.T) {
	_, diags, err := Lex([]byte("<?php\n\n/* open"), LexOptions{IntWidth: 64, MaxBytes: 100})
	if err != nil || len(diags) != 1 || diags[0].Line != 3 {
		t.Fatalf("diags %v err %v", diags, err)
	}
}

func FuzzLex(f *testing.F) {
	for _, s := range []string{"<?php \"$a[", "<?php <<<X\n${", "<?php `{$a->", "<?php #[", "<?= ?>", "<?php (int)0x", "\xff<?php"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		toks, _, err := Lex(src, LexOptions{ShortOpenTag: true, IntWidth: 64, MaxBytes: 1 << 16})
		if err != nil {
			if errors.Is(err, ErrInputTooLarge) {
				return
			}
			t.Fatal(err)
		}
		n := 0
		for _, tok := range toks {
			n += len(tok.Text)
		}
		if n != len(src) {
			t.Fatalf("tokens cover %d of %d bytes", n, len(src))
		}
	})
}

func TestHaltCompilerTakesExactlyThreeTokens(t *testing.T) {
	// PHP 8.5.12 token_get_all keeps lexing three significant tokens after
	// __halt_compiler, even when they are not "();".
	got := kinds(lex(t, "<?php __halt_compiler);\n\n?>\ndata"))
	want := "T_OPEN_TAG T_HALT_COMPILER ) ; T_WHITESPACE T_CLOSE_TAG T_INLINE_HTML"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestHeredocBraceIndexMemoryFollowsBraceCount(t *testing.T) {
	// One interpolation in a large heredoc must not allocate per source byte.
	body := strings.Repeat("x", 4<<20)
	src := []byte("<?php $s = <<<EOT\n{$a}" + body + "\nEOT;\n")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: len(src)}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > uint64(3*len(src)) {
		t.Fatalf("lexing a %d-byte heredoc allocated %d bytes", len(src), grown)
	}
}

func TestBraceHeavyHeredocStopsAtTokenBudget(t *testing.T) {
	src := []byte("<?php $s = <<<EOT\n{$a[" + strings.Repeat("{", 1<<20) + "\nEOT;\n")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, _, err := Lex(src, LexOptions{IntWidth: 64, MaxBytes: len(src), MaxTokens: 20})
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooManyTokens) {
		t.Fatalf("err = %v", err)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > uint64(4*len(src)) {
		t.Fatalf("brace prescan allocated %d bytes for a %d-byte input", grown, len(src))
	}
}
