package phego

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// W-17 stage 1: REQ-025, REQ-026; T-009, T-010. The references under
// tests/vm/w17 were produced by PHP 8.5.12 in pinned images
// (tests/vm/w17/README.md). They are oracle data: this test runs every case
// in every mode through the public API and compares the bytes.

type w17Reference struct {
	IntSize int       `json:"int_size"` // bytes, as PHP_INT_SIZE
	Cases   []w17Case `json:"cases"`
}

type w17Case struct {
	Case         string  `json:"case"`
	SourceSHA256 string  `json:"source_sha256"`
	Display      w17Run  `json:"display"`
	Log          w17Run  `json:"log"`
	Both         w17Run  `json:"both"`
	ReturnJSON   *string `json:"return_json"`
}

type w17Run struct {
	Stdout   string `json:"stdout_b64"`
	Stderr   string `json:"stderr_b64"`
	ExitCode int    `json:"exit_code"`
}

// w17Unsupported lists the cases this build's first VM stage cannot run,
// with the located error Compile must return. They count as unsupported in
// the summary, never as passes.
var w17Unsupported = map[string]UnsupportedError{
	"compound_assign":           {Feature: "compound assignment to array element", File: "/w/cases/compound_assign.php", Line: 61},
	"compound_assign_undefined": {Feature: "compound assignment to array element", File: "/w/cases/compound_assign_undefined.php", Line: 34},
	"isset_unset":               {Feature: "nested array unset", File: "/w/cases/isset_unset.php", Line: 22},
}

func TestW17PHP8512Programs(t *testing.T) {
	dir := filepath.Join("tests", "vm", "w17")
	files, err := filepath.Glob(filepath.Join(dir, "cases", "*.php"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".php"))
	}
	for _, profile := range []struct {
		name, file string
		intSize    int
	}{
		{"64", "reference-64.json", 64},
		{"amd64", "reference-64-amd64.json", 64},
		{"32", "reference-32.json", 32},
	} {
		t.Run(profile.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, profile.file))
			if err != nil {
				t.Fatal(err)
			}
			var ref w17Reference
			if err := json.Unmarshal(raw, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.IntSize*8 != profile.intSize {
				t.Fatalf("%s has PHP_INT_SIZE %d, want %d bits", profile.file, ref.IntSize, profile.intSize)
			}
			var refNames []string
			for _, c := range ref.Cases {
				refNames = append(refNames, c.Case)
			}
			if !slices.Equal(refNames, names) {
				t.Fatalf("%s covers %v, the fixtures are %v", profile.file, refNames, names)
			}
			e, err := NewEngine(Config{IntSize: profile.intSize})
			if err != nil {
				t.Fatal(err)
			}
			var pass, unsupported, fail int
			for _, c := range ref.Cases {
				src, err := os.ReadFile(filepath.Join(dir, "cases", c.Case+".php"))
				if err != nil {
					t.Fatal(err)
				}
				if sum := sha256.Sum256(src); hex.EncodeToString(sum[:]) != c.SourceSHA256 {
					t.Fatalf("%s: the fixture changed after %s was generated; regenerate the reference", c.Case, profile.file)
				}
				for _, mode := range []struct {
					name         string
					want         w17Run
					display, log string
				}{
					{"display", c.Display, "1", "0"},
					{"log", c.Log, "0", "1"},
					{"both", c.Both, "1", "1"},
				} {
					ok := t.Run(c.Case+"/"+mode.name, func(t *testing.T) {
						want, isUnsupported := w17Unsupported[c.Case]
						got := w17Execute(t, e, c.Case, string(src), WithINI("display_errors", mode.display), WithINI("log_errors", mode.log),
							WithINI("html_errors", "0"), WithINI("error_reporting", "-1"))
						if isUnsupported {
							var ue *UnsupportedError
							if !errors.As(got.err, &ue) || *ue != want {
								t.Fatalf("want the located %#v, got %#v", want, got.err)
							}
							unsupported++
							return
						}
						if got.err != nil && got.compileFatal == nil && got.exitCode != 255 {
							t.Fatalf("run failed: %v", got.err)
						}
						w17Compare(t, "stdout", w17Decode(t, mode.want.Stdout), got.stdout)
						w17Compare(t, "stderr", w17Decode(t, mode.want.Stderr), got.stderr)
						if got.exitCode != mode.want.ExitCode {
							t.Errorf("exit code %d, want %d (error %v)", got.exitCode, mode.want.ExitCode, got.err)
						}
						if c.ReturnJSON != nil {
							if j, err := w17JSON(got.ret); err != nil || j != *c.ReturnJSON {
								t.Errorf("return value %s (%v), want %s", j, err, *c.ReturnJSON)
							}
						}
					})
					if _, isUnsupported := w17Unsupported[c.Case]; ok && !isUnsupported {
						pass++
					} else if !ok {
						fail++
					}
				}
			}
			t.Logf("profile %s: %d pass, %d unsupported, %d fail of %d", profile.name, pass, unsupported, fail, 3*len(ref.Cases))
			if pass+unsupported+fail != 3*len(ref.Cases) {
				t.Errorf("counted %d runs, want %d", pass+unsupported+fail, 3*len(ref.Cases))
			}
		})
	}
}

type w17Observed struct {
	stdout, stderr []byte
	exitCode       int
	ret            Value
	err            error
	compileFatal   *PHPError
}

// w17Execute compiles and runs one case as php -n /w/cases/<case>.php does.
// A compile-time fatal error is rendered the way PHP 8.5.12 prints it.
func w17Execute(t *testing.T, e *Engine, name, src string, opts ...RunOption) w17Observed {
	t.Helper()
	ctx := context.Background()
	p, err := e.Compile(ctx, "/w/cases/"+name+".php", src)
	var pe *PHPError
	if errors.As(err, &pe) && pe.Kind == CompileError {
		var rc runConfig
		rc.ini = defaultINI
		for _, opt := range opts {
			if err := opt(&rc); err != nil {
				t.Fatal(err)
			}
		}
		obs := w17Observed{exitCode: 255, err: err, compileFatal: pe}
		// PHP 8.5's fatal_error_backtraces (on by default) appends the
		// backtrace after the location; while the file is compiled it is
		// {main} alone.
		text := "Fatal error: " + pe.Message + " in " + pe.File + " on line " + strconv.Itoa(pe.Line) + "\nStack trace:\n#0 {main}\n"
		if rc.ini.display {
			obs.stdout = []byte("\n" + text)
		}
		if rc.ini.log {
			obs.stderr = []byte("PHP " + strings.Replace(text, ": ", ":  ", 1))
		}
		return obs
	}
	if err != nil {
		return w17Observed{err: err}
	}
	res, err := e.Run(ctx, p, opts...)
	return w17Observed{stdout: res.Output, stderr: w17LogLines(res.Diagnostics), exitCode: res.ExitCode, ret: res.Return, err: err}
}

// w17LogLines renders the log channel as the PHP CLI writes it to stderr
// when error_log is empty: "PHP Level:  message in file on line N".
func w17LogLines(ds []Diagnostic) []byte {
	var b []byte
	for _, d := range ds {
		b = fmt.Appendf(b, "PHP %s:  %s in %s on line %d\n", d.Level, d.Message, d.File, d.Line)
	}
	return b
}

func w17Decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// w17Compare reports the first differing byte with some context.
func w17Compare(t *testing.T, what string, want, got []byte) {
	t.Helper()
	if string(want) == string(got) {
		return
	}
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	from := max(0, i-40)
	t.Errorf("%s differs at byte %d\n  php:   %q\n  phego: %q", what, i, want[from:min(len(want), i+60)], got[from:min(len(got), i+60)])
}

// w17JSON is json_encode($v, JSON_PRESERVE_ZERO_FRACTION) of PHP 8.5.12 for
// the values the return_* cases produce.
func w17JSON(v Value) (string, error) {
	if v.IsNull() {
		return "null", nil
	}
	if b, ok := v.Bool(); ok {
		return strconv.FormatBool(b), nil
	}
	if i, ok := v.Int64(); ok {
		return strconv.FormatInt(i, 10), nil
	}
	if f, ok := v.Float64(); ok {
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return "", errors.New("json_encode cannot encode INF or NAN")
		}
		s := strconv.FormatFloat(f, 'g', -1, 64)
		if strings.ContainsAny(s, "e") {
			return "", fmt.Errorf("exponent float %s is not encoded by this helper", s)
		}
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s, nil
	}
	if b, ok := v.Bytes(); ok {
		return w17JSONString(b)
	}
	entries, _ := v.Entries()
	list := true
	for i, en := range entries {
		if k, ok := en.Key.Int64(); !ok || k != int64(i) {
			list = false
		}
	}
	var parts []string
	for _, en := range entries {
		val, err := w17JSON(en.Value)
		if err != nil {
			return "", err
		}
		if list {
			parts = append(parts, val)
			continue
		}
		key, ok := en.Key.String()
		if !ok {
			k, _ := en.Key.Int64()
			key = strconv.FormatInt(k, 10)
		}
		ks, err := w17JSONString([]byte(key))
		if err != nil {
			return "", err
		}
		parts = append(parts, ks+":"+val)
	}
	if list {
		return "[" + strings.Join(parts, ",") + "]", nil
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func w17JSONString(b []byte) (string, error) {
	if !utf8.Valid(b) {
		return "", errors.New("json_encode rejects invalid UTF-8")
	}
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range string(b) {
		switch {
		case r == '"' || r == '\\' || r == '/':
			out.WriteString(`\` + string(r))
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\r':
			out.WriteString(`\r`)
		case r == '\t':
			out.WriteString(`\t`)
		case r == '\b':
			out.WriteString(`\b`)
		case r == '\f':
			out.WriteString(`\f`)
		case r < 0x20 || r >= 0x80:
			if r > 0xffff {
				r -= 0x10000
				fmt.Fprintf(&out, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			} else {
				fmt.Fprintf(&out, `\u%04x`, r)
			}
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String(), nil
}
