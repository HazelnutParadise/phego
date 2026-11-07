package diag

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const tomlSource = "[policy]\n\n[[policy.files]]\n  path = \"app\"\n  root = \"/srv/app\"\n"

func sources(m map[string]string) func(string) ([]byte, bool) {
	return func(name string) ([]byte, bool) {
		s, ok := m[name]
		return []byte(s), ok
	}
}

func TestRenderGolden(t *testing.T) {
	cases := []struct {
		name string
		d    Diagnostic
		src  map[string]string
	}{
		{
			name: "primary-with-source",
			d: Diagnostic{
				Code: "PHG0123", Severity: Error, Message: "mount path is not absolute",
				Primary: Span{File: "phego.toml", Line: 4, Column: 10, Length: 5, Label: `expected a path starting with "/"`},
				Notes:   []string{"mounts are matched against absolute PHP paths"},
				Help:    []string{`write "/app"`},
			},
			src: map[string]string{"phego.toml": tomlSource},
		},
		{
			name: "field-path-no-source",
			d: Diagnostic{
				Code: "PHG0101", Severity: Error, Message: "Limits.Output must not be negative",
				Primary: Span{File: "Limits.Output"},
				Notes:   []string{"got -1"},
				Help:    []string{"use 0 for the default"},
			},
		},
		{
			name: "missing-source-text",
			d: Diagnostic{
				Code: "PHG0201", Severity: Error, Message: "register read before definition",
				Primary: Span{File: "app.php", Line: 12, Column: 3, Length: 4},
			},
		},
		{
			name: "secondary-same-and-other-line",
			d: Diagnostic{
				Code: "PHG0202", Severity: Warning, Message: "handler regions cross",
				Primary:   Span{File: "a.php", Line: 3, Column: 1, Length: 3, Label: "this region"},
				Secondary: []Span{{File: "a.php", Line: 2, Column: 5, Length: 2, Label: "starts inside this one"}, {File: "a.php", Line: 3, Column: 7, Length: 1, Label: "ends here"}},
			},
			src: map[string]string{"a.php": "<?php\ntry {\nfoo(); }\n"},
		},
		{
			name: "secondary-other-file",
			d: Diagnostic{
				Code: "PHG0203", Severity: Error, Message: "duplicate function name",
				Primary:   Span{File: "b.php", Line: 1, Column: 10, Length: 1, Label: "second definition"},
				Secondary: []Span{{File: "a.php", Line: 1, Column: 10, Length: 1, Label: "first definition"}},
			},
			src: map[string]string{"a.php": "function f() {}\n", "b.php": "function f() {}\n"},
		},
		{
			name: "tabs-and-multibyte",
			d: Diagnostic{
				Code: "PHG0124", Severity: Error, Message: "bad value",
				Primary: Span{File: "x.php", Line: 1, Column: 4, Length: 6, Label: "here"},
			},
			src: map[string]string{"x.php": "\té值值x\n"},
		},
		{
			name: "zero-length-and-past-end",
			d: Diagnostic{
				Code: "PHG0125", Severity: Error, Message: "unexpected end",
				Primary: Span{File: "y.php", Line: 1, Column: 4, Length: 0, Label: "expected more here"},
			},
			src: map[string]string{"y.php": "abc"},
		},
		{
			name: "line-past-end-of-file",
			d: Diagnostic{
				Code: "PHG0126", Severity: Error, Message: "position outside source",
				Primary: Span{File: "y.php", Line: 9, Column: 1, Length: 1},
			},
			src: map[string]string{"y.php": "abc\n"},
		},
		{
			name: "column-from-offset",
			d: Diagnostic{
				Code: "PHG0130", Severity: Error, Message: "unknown opcode",
				Primary: Span{File: "a.php", Line: 2, Offset: 6, Length: 4, Label: "this instruction"},
			},
			src: map[string]string{"a.php": "<?php\necho 1;\n"},
		},
		{
			name: "invalid-utf8-crlf-and-controls",
			d: Diagnostic{
				Code: "PHG0127", Severity: Error, Message: "bad\x1b[31m byte",
				Primary: Span{File: "z.php", Line: 2, Column: 3, Length: 1, Label: "\x07here"},
			},
			src: map[string]string{"z.php": "<?php\r\na\xffb\x1b[2Jc\r\n"},
		},
		{
			name: "long-line-window",
			d: Diagnostic{
				Code: "PHG0128", Severity: Error, Message: "value too long",
				Primary: Span{File: "l.php", Line: 1, Column: 301, Length: 3, Label: "here"},
			},
			src: map[string]string{"l.php": strings.Repeat("a", 300) + "XYZ" + strings.Repeat("b", 300)},
		},
		{
			name: "many-secondary",
			d: func() Diagnostic {
				d := Diagnostic{Code: "PHG0129", Severity: Error, Message: "too many", Primary: Span{File: "m.php", Line: 1, Column: 1, Length: 1}}
				for i := 0; i < 20; i++ {
					d.Secondary = append(d.Secondary, Span{File: "m.php", Line: 2, Column: i + 1, Length: 1})
				}
				return d
			}(),
			src: map[string]string{"m.php": "x\n" + strings.Repeat("y", 20) + "\n"},
		},
	}
	for _, c := range cases {
		for _, color := range []bool{false, true} {
			name := c.name
			if color {
				name += ".color"
			}
			t.Run(name, func(t *testing.T) {
				var b bytes.Buffer
				if err := Render(&b, c.d, Options{Color: color, Source: sources(c.src)}); err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join("testdata", name+".txt")
				if *update {
					if err := os.WriteFile(golden, b.Bytes(), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if b.String() != string(want) {
					t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
				}
				if !color && strings.Contains(b.String(), "\x1b") {
					t.Fatal("plain output contains escape codes")
				}
			})
		}
	}
}

func TestOneLine(t *testing.T) {
	d := Diagnostic{Code: "PHG0123", Severity: Error, Message: "mount path is not absolute", Primary: Span{File: "phego.toml", Line: 4, Column: 10}}
	if got, want := d.OneLine(), "error[PHG0123]: mount path is not absolute (phego.toml:4:10)"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	d.Primary = Span{File: "Limits.Output"}
	if got, want := d.OneLine(), "error[PHG0123]: mount path is not absolute (Limits.Output)"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSeverityText(t *testing.T) {
	for _, s := range []Severity{Error, Warning} {
		b, err := s.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var got Severity
		if err := got.UnmarshalText(b); err != nil || got != s {
			t.Fatalf("%v round trip gave %v, %v", s, got, err)
		}
	}
	var s Severity
	if s.UnmarshalText([]byte("fatal")) == nil {
		t.Fatal("unknown severity accepted")
	}
}

func TestJSONMatchesTerminal(t *testing.T) {
	d := Diagnostic{Code: "PHG0123", Message: "mount path is not absolute",
		Primary: Span{File: "phego.toml", Line: 4, Column: 10, Length: 5, Label: "here"},
		Notes:   []string{"why"}, Help: []string{"fix"}}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"PHG0123","severity":"error","message":"mount path is not absolute","primary":{"file":"phego.toml","line":4,"column":10,"length":5,"label":"here"},"notes":["why"],"help":["fix"]}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	var back Diagnostic
	if err := json.Unmarshal(b, &back); err != nil || back.OneLine() != d.OneLine() {
		t.Fatalf("round trip %+v, %v", back, err)
	}
}
