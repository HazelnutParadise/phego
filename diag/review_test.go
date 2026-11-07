package diag

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func render(t *testing.T, d Diagnostic, src string) string {
	t.Helper()
	var b bytes.Buffer
	if err := Render(&b, d, Options{Source: func(string) ([]byte, bool) { return []byte(src), true }}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// markerColumn is the display column of the first marker under source line n.
func markerColumn(out string, mark byte) int {
	for _, l := range strings.Split(out, "\n") {
		if i := strings.IndexByte(l, '|'); i >= 0 && i+2 < len(l) && strings.TrimSpace(l[:i]) == "" {
			if j := strings.IndexByte(l[i+2:], mark); j >= 0 && strings.Trim(l[i+2:i+2+j], " ") == "" {
				return j
			}
		}
	}
	return -1
}

func TestReviewColumnPastEndIsClamped(t *testing.T) {
	for _, col := range []int{math.MaxInt, 1 << 20, 5} {
		out := render(t, Diagnostic{Code: "X", Primary: Span{File: "f", Line: 1, Column: col, Length: 1}}, "abc\n")
		if len(out) > 200 || markerColumn(out, '^') != 3 {
			t.Fatalf("column %d: marker at %d, %d bytes:\n%s", col, markerColumn(out, '^'), len(out), out)
		}
	}
}

func TestReviewLongLineWindowFollowsPrimary(t *testing.T) {
	src := strings.Repeat("a", 379) + "XYZ" + strings.Repeat("b", 20)
	out := render(t, Diagnostic{Code: "X",
		Primary:   Span{File: "f", Line: 1, Column: 380, Length: 3, Label: "primary"},
		Secondary: []Span{{File: "f", Line: 1, Column: 2, Length: 1, Label: "far left"}}}, src)
	lines := strings.Split(out, "\n")
	var code string
	for _, l := range lines {
		if strings.HasPrefix(l, "1 | ") {
			code = l[4:]
		}
	}
	col := markerColumn(out, '^')
	if col < 0 || col+3 > len(code) || code[col:col+3] != "XYZ" {
		t.Fatalf("primary marker not under XYZ:\n%s", out)
	}
	if !strings.Contains(out, "note: f:1:2: far left") {
		t.Fatalf("secondary outside the window is not reported as a note:\n%s", out)
	}
}

func TestReviewBidiControlsAreReplaced(t *testing.T) {
	out := render(t, Diagnostic{Code: "X", Message: "a‮b", Primary: Span{File: "f⁦", Line: 1, Column: 1, Length: 1, Label: "‏"}}, "x‮y\n")
	if strings.ContainsAny(out, "‮⁦‏") {
		t.Fatalf("bidi control in output: %q", out)
	}
}

func TestReviewWidthsOfCombiningAndEmoji(t *testing.T) {
	for _, src := range []string{"éx", "\U0001F680x"} {
		out := render(t, Diagnostic{Code: "X", Primary: Span{File: "f", Line: 1, Column: len(src), Length: 1}}, src)
		want := 1
		if strings.HasPrefix(src, "\U0001F680") {
			want = 2
		}
		if got := markerColumn(out, '^'); got != want {
			t.Fatalf("%q: marker at %d, want %d\n%s", src, got, want, out)
		}
	}
}

// PHP and the Phego lexer count a lone \r as a line break.
func TestReviewLoneCarriageReturnIsALineBreak(t *testing.T) {
	out := render(t, Diagnostic{Code: "X", Primary: Span{File: "f", Line: 2, Column: 1, Length: 2}}, "ab\rcd\r\nef")
	if !strings.Contains(out, "2 | cd\n") {
		t.Fatalf("line 2 is not cd:\n%s", out)
	}
	out = render(t, Diagnostic{Code: "X", Primary: Span{File: "f", Line: 3, Offset: 7, Length: 2}}, "ab\rcd\r\nef")
	if !strings.Contains(out, "--> f:3:1") {
		t.Fatalf("offset after \\r\\n not resolved on line 3:\n%s", out)
	}
}

func TestReviewSourceReadOnce(t *testing.T) {
	calls := 0
	var b bytes.Buffer
	d := Diagnostic{Code: "X", Primary: Span{File: "f", Line: 1, Offset: 1, Length: 1}, Secondary: []Span{{File: "f", Line: 1, Column: 1, Length: 1}}}
	if err := Render(&b, d, Options{Source: func(string) ([]byte, bool) { calls++; return []byte("abc"), true }}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("Source called %d times", calls)
	}
}

// Without Column, Offset or Length the column is unknown, not 1.
func TestReviewUnknownColumnStaysUnknown(t *testing.T) {
	out := render(t, Diagnostic{Code: "X", Primary: Span{File: "f", Line: 1}}, "abc\n")
	if !strings.Contains(out, "--> f:1\n") || strings.Contains(out, "^") {
		t.Fatalf("invented a column:\n%s", out)
	}
}
