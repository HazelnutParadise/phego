package runtime

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// opsReference is the output of tests/semantics/w10-ops/matrix.php run with
// `php -n` in the pinned PHP 8.5.12 images.
type opsReference struct {
	PHP      string                       `json:"php"`
	IntSize  int                          `json:"int_size"`
	Operands []string                     `json:"operands"`
	Unary    map[string][]opsReferenceRun `json:"unary"`
	Binary   map[string][]opsReferenceRun `json:"binary"`
}

type opsReferenceRun struct {
	R string   `json:"r"`
	E []string `json:"e"`
	D [][2]any `json:"d"`
}

// nativeOpsReferenceEnv names a directory holding ref-32.json and
// ref-64.json produced on hardware that runs both reference images natively.
// The semantics-ops workflow sets it on an x86-64 runner.
const nativeOpsReferenceEnv = "PHEGO_NATIVE_OPS_REFERENCE"

// loadOpsReference returns the reference for width, read from the native
// directory when it is set and from the committed testdata otherwise.
func loadOpsReference(t *testing.T, width int) opsReference {
	t.Helper()
	var r io.Reader
	native := os.Getenv(nativeOpsReferenceEnv) != ""
	if native {
		f, err := os.Open(filepath.Join(os.Getenv(nativeOpsReferenceEnv), fmt.Sprintf("ref-%d.json", width)))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		r = f
	} else {
		f, err := os.Open(fmt.Sprintf("testdata/php8512-ops-%d.json.gz", width))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		z, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		r = z
	}
	var ref opsReference
	if err := json.NewDecoder(r).Decode(&ref); err != nil {
		t.Fatal(err)
	}
	if ref.PHP != "8.5.12" || ref.IntSize*8 != width {
		t.Fatalf("reference is PHP %s with %d-byte integers, want 8.5.12 at %d bits", ref.PHP, ref.IntSize, width)
	}
	return ref
}

// decodeOperand builds a value from matrix.php's exact encoding.
func decodeOperand(t *testing.T, h *heap, enc string) value {
	t.Helper()
	kind, body, _ := strings.Cut(enc, ":")
	switch kind {
	case "N":
		return nullValue()
	case "b":
		return boolValue(body == "1")
	case "i":
		i, err := strconv.ParseInt(body, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return h.number(i)
	case "d":
		raw, err := hex.DecodeString(body)
		if err != nil {
			t.Fatal(err)
		}
		return floatValue(math.Float64frombits(binary.BigEndian.Uint64(raw)))
	case "s":
		raw, err := hex.DecodeString(body)
		if err != nil {
			t.Fatal(err)
		}
		return value{kind: valueString, text: string(raw)}
	case "a":
		var pairs [][2]string
		if err := json.Unmarshal([]byte(body), &pairs); err != nil {
			t.Fatal(err)
		}
		arr := h.newArrayValue()
		for _, p := range pairs {
			if err := h.setElement(&arr, decodeOperand(t, h, p[0]), decodeOperand(t, h, p[1])); err != nil {
				t.Fatal(err)
			}
		}
		return arr
	}
	t.Fatalf("unknown operand encoding %q", enc)
	return value{}
}

// encodeValue mirrors matrix.php's enc().
func encodeValue(v value) string {
	switch v = v.deref(); v.kind {
	case valueNull, valueUndefined:
		return "N"
	case valueBool:
		if v.boolean {
			return "b:1"
		}
		return "b:0"
	case valueInt:
		return "i:" + strconv.FormatInt(v.integer, 10)
	case valueFloat:
		var raw [8]byte
		binary.BigEndian.PutUint64(raw[:], math.Float64bits(v.floating))
		return "d:" + hex.EncodeToString(raw[:])
	case valueString:
		return "s:" + hex.EncodeToString([]byte(v.text))
	case valueArray:
		pairs := [][2]string{}
		pos := 0
		for b, ok := v.array.next(&pos); ok; b, ok = v.array.next(&pos) {
			k := value{kind: valueInt, integer: b.key.integer}
			if b.key.isString {
				k = value{kind: valueString, text: b.key.text}
			}
			pairs = append(pairs, [2]string{encodeValue(k), encodeValue(b.val)})
		}
		raw, _ := json.Marshal(pairs)
		return "a:" + string(raw)
	}
	return "?"
}

func boolResult(b bool, err error) (value, error) { return boolValue(b), err }

func intResult(c int, err error) (value, error) { return value{kind: valueInt, integer: int64(c)}, err }

func php8512Binary(h *heap, op string, a, b value) (value, error) {
	switch op {
	case "+", "-", "*", "/", "**":
		return h.arith(op, a, b)
	case "%":
		return h.mod(a, b)
	case ".":
		return h.concat(a, b)
	case "==":
		return boolResult(h.looseEqual(a, b))
	case "!=":
		eq, err := h.looseEqual(a, b)
		return boolValue(!eq), err
	case "===":
		return boolResult(identical(a, b))
	case "<", "<=", ">", ">=":
		x, y := a, b
		if op[0] == '>' {
			x, y = b, a
		}
		c, err := h.compare(x, y)
		return boolValue(c < 0 || (len(op) == 2 && c == 0)), err
	case "<=>":
		return intResult(h.compare(a, b))
	case "&", "|", "^":
		return h.bitwise(op, a, b)
	case "<<", ">>":
		return h.shift(op, a, b)
	}
	panic("unknown operator " + op)
}

func php8512Unary(h *heap, op string, a value) (value, error) {
	switch op {
	case "(int)":
		return h.number(h.toInt(a)), nil
	case "(float)":
		return floatValue(h.toFloat(a)), nil
	case "(string)":
		s, err := h.toString(a)
		return value{kind: valueString, text: s}, err
	case "(bool)":
		return boolValue(h.toBool(a)), nil
	case "neg":
		return h.arith("*", a, h.number(-1))
	case "pos":
		return h.arith("*", a, h.number(1))
	case "~":
		return h.bitwiseNot(a)
	case "!":
		return boolValue(!h.toBool(a)), nil
	}
	panic("unknown operator " + op)
}

func describeRun(r opsReferenceRun) string {
	return fmt.Sprintf("result=%q throw=%q diagnostics=%v", r.R, r.E, r.D)
}

func actualRun(h *heap, v value, err error) opsReferenceRun {
	var run opsReferenceRun
	if err != nil {
		th, ok := err.(*Throwable)
		if !ok {
			run.E = []string{"go-error", err.Error()}
		} else {
			run.E = []string{th.Class, th.Message}
		}
	} else {
		run.R = encodeValue(v)
	}
	for _, d := range h.diagnostics {
		run.D = append(run.D, [2]any{float64(d.level), d.message})
	}
	return run
}

// canonicalNaN maps every NaN encoding to one form. The sign and payload of a
// NaN come from the host CPU and math library, not from PHP semantics, and
// PHP exposes them only through pack().
func canonicalNaN(enc string) string {
	if raw, err := hex.DecodeString(strings.TrimPrefix(enc, "d:")); err == nil && strings.HasPrefix(enc, "d:") && len(raw) == 8 {
		if math.IsNaN(math.Float64frombits(binary.BigEndian.Uint64(raw))) {
			return "d:NaN"
		}
	}
	return enc
}

// ulpDistance returns how many float64 steps separate two finite float
// encodings, or -1 when either is not a finite float.
func ulpDistance(a, b string) int64 {
	decode := func(enc string) (float64, bool) {
		raw, err := hex.DecodeString(strings.TrimPrefix(enc, "d:"))
		if !strings.HasPrefix(enc, "d:") || err != nil || len(raw) != 8 {
			return 0, false
		}
		f := math.Float64frombits(binary.BigEndian.Uint64(raw))
		return f, !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	x, okx := decode(a)
	y, oky := decode(b)
	if !okx || !oky || math.Signbit(x) != math.Signbit(y) {
		return -1
	}
	d := int64(math.Float64bits(x)) - int64(math.Float64bits(y))
	if d < 0 {
		d = -d
	}
	return d
}

// i386PowDifferences is the number of 32-bit ** results that differ from
// PHP 8.5.12 by at most 64 units in the last place. The linux/386 PHP build
// calls glibc's i386 pow, which computes with x87 instructions and is not
// rounded to nearest; Phego rounds pow to nearest, which matches the 64-bit
// glibc builds exactly. The i386 reference is byte-identical whether it runs
// natively on x86-64 or under QEMU, so this is a real libm difference of the
// 32-bit profile, recorded as a known gap in docs/implementation/W-10.md.
const i386PowDifferences = 51

func sameRun(a, b opsReferenceRun) bool {
	return canonicalNaN(a.R) == canonicalNaN(b.R) && slices.Equal(a.E, b.E) && slices.EqualFunc(a.D, b.D, func(x, y [2]any) bool { return x == y })
}

// TestPHP8512OperatorsMatchReference checks every conversion and operator
// result, thrown error, and diagnostic against PHP 8.5.12 for both integer
// widths.
func TestPHP8512OperatorsMatchReference(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprintf("int%d", width), func(t *testing.T) {
			ref := loadOpsReference(t, width)
			h, err := newHeap(width)
			if err != nil {
				t.Fatal(err)
			}
			operands := make([]value, len(ref.Operands))
			for i, enc := range ref.Operands {
				operands[i] = decodeOperand(t, h, enc)
			}
			failures := 0
			report := func(name string, want, got opsReferenceRun) {
				failures++
				if failures <= 40 {
					t.Errorf("%s\n  php:   %s\n  phego: %s", name, describeRun(want), describeRun(got))
				}
			}
			checked := 0
			for op, runs := range ref.Unary {
				for i, want := range runs {
					h.diagnostics = nil
					v, err := php8512Unary(h, op, operands[i])
					if got := actualRun(h, v, err); !sameRun(want, got) {
						report(fmt.Sprintf("%s %s", op, ref.Operands[i]), want, got)
					}
					checked++
				}
			}
			n := len(operands)
			powDiffs := 0
			for op, runs := range ref.Binary {
				for idx, want := range runs {
					h.diagnostics = nil
					v, err := php8512Binary(h, op, operands[idx/n], operands[idx%n])
					got := actualRun(h, v, err)
					if sameRun(want, got) {
						checked++
						continue
					}
					if d := ulpDistance(want.R, got.R); width == 32 && op == "**" && d > 0 && d <= 64 && len(want.D) == len(got.D) {
						powDiffs++
						continue
					}
					report(fmt.Sprintf("%s %s %s", ref.Operands[idx/n], op, ref.Operands[idx%n]), want, got)
					checked++
				}
			}
			if failures > 0 {
				t.Fatalf("%d cases differ from PHP 8.5.12", failures)
			}
			if want := map[int]int{32: i386PowDifferences, 64: 0}[width]; powDiffs != want {
				t.Fatalf("%d ** results are within 64 ULP of the i386 libm result, want exactly %d", powDiffs, want)
			}
			t.Logf("%d cases match PHP 8.5.12; %d ** results differ from the i386 libm result by at most 64 ULP", checked, powDiffs)
		})
	}
}

func php8512StringOp(h *heap, op string, s value) (value, error) {
	switch op {
	case "(int)", "(float)":
		return php8512Unary(h, op, s)
	case "+0":
		return h.arith("+", s, h.number(0))
	case "%7":
		return h.mod(s, h.number(7))
	case "<=>1":
		return intResult(h.compare(s, value{kind: valueString, text: "1"}))
	case "==1.0":
		return boolResult(h.looseEqual(s, value{kind: valueString, text: "1.0"}))
	case "<=>int":
		return intResult(h.compare(s, h.number(10)))
	case "==0":
		return boolResult(h.looseEqual(s, h.number(0)))
	}
	panic("unknown string operation " + op)
}

// TestPHP8512NumericStringsMatchReference runs a generated corpus of strings
// built from the bytes that matter to numeric recognition (see
// tools/w10_numeric_strings.py) through conversions and comparisons.
func TestPHP8512NumericStringsMatchReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/php8512-numeric-strings.txt")
	if err != nil {
		t.Fatal(err)
	}
	inputs := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprintf("int%d", width), func(t *testing.T) {
			f, err := os.Open(fmt.Sprintf("testdata/php8512-numeric-strings-%d.jsonl.gz", width))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			z, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			dec := json.NewDecoder(z)
			h, err := newHeap(width)
			if err != nil {
				t.Fatal(err)
			}
			failures, checked := 0, 0
			for _, line := range inputs {
				var row map[string]opsReferenceRun
				if err := dec.Decode(&row); err != nil {
					t.Fatalf("reference ended early at %q: %v", line, err)
				}
				text, err := hex.DecodeString(line)
				if err != nil {
					t.Fatal(err)
				}
				s := value{kind: valueString, text: string(text)}
				for op, want := range row {
					h.diagnostics = nil
					v, err := php8512StringOp(h, op, s)
					if got := actualRun(h, v, err); !sameRun(want, got) {
						if failures++; failures <= 40 {
							t.Errorf("%q %s\n  php:   %s\n  phego: %s", text, op, describeRun(want), describeRun(got))
						}
					}
					checked++
				}
			}
			if dec.More() {
				t.Fatal("reference has more rows than inputs")
			}
			if failures > 0 {
				t.Fatalf("%d of %d cases differ from PHP 8.5.12", failures, checked)
			}
			t.Logf("%d cases match PHP 8.5.12", checked)
		})
	}
}
