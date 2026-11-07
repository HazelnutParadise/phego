package ir

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// The fixture records php -n -l, so these assertions concern compilation only.
func TestCompatibilityConstantOracle(t *testing.T) {
	checkCompileOracle(t, "testdata/php8512-constant-compile.json.gz", 9362)
}

func TestCompatibilitySyntaxConstantOracle(t *testing.T) {
	checkCompileOracle(t, "testdata/php8512-syntax-constant-compile.json.gz", 1400)
}

func checkCompileOracle(t *testing.T, file string, count int) {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var cases []struct {
		Width  int    `json:"width"`
		Source string `json:"source"`
		Events []struct {
			Level   string `json:"level"`
			Message string `json:"message"`
			Line    int    `json:"line"`
		} `json:"events"`
	}
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != count {
		t.Fatalf("got %d reference cases", len(cases))
	}
	failures := 0
	for _, tc := range cases {
		if tc.Width != 32 && tc.Width != 64 {
			t.Fatalf("bad profile: %d", tc.Width)
		}
		unit, err := lowerSource(tc.Source, tc.Width)
		var ce *CompileError
		if err != nil && !errors.As(err, &ce) {
			t.Fatal(err)
		}
		message, line := "", 0
		var warnings []Diagnostic
		for _, e := range tc.Events {
			if e.Level == "Fatal error" {
				message, line = e.Message, e.Line
			} else {
				warnings = append(warnings, Diagnostic{Line: e.Line, Level: e.Level, Message: e.Message})
			}
		}
		gotMessage, gotLine := "", 0
		if ce != nil {
			gotMessage, gotLine = ce.Message, ce.Line
		}
		different := gotMessage != message || gotLine != line
		if ce == nil && message == "" {
			got, _ := json.Marshal(unit.Diagnostics)
			want, _ := json.Marshal(warnings)
			different = different || string(got) != string(want)
		}
		if different {
			failures++
			t.Errorf("width=%d %s: got line%d %q; want line%d %q", tc.Width, tc.Source, gotLine, gotMessage, line, message)
		}
	}
	t.Logf("%d cases, %d differences", len(cases), failures)
}
