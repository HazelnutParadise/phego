package runtime

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// The helpers behind Binary, Unary and Cast already match PHP 8.5.12 on 48
// operands (TestPHP8512OperatorsMatchReference, testdata/php8512-ops-*.json).
// These tests do not repeat that table: they run the VM on the stage-1 value
// set and require each Aux to reach the helper PHP's result came from, with
// every diagnostic located at the instruction's file and line.

// vmValueSet is {null,false,true,0,1,-1,1.5,"1","1.5","abc","",[],[1]} in the
// ops reference's operand encoding.
var vmValueSet = []string{"N", "b:0", "b:1", "i:0", "i:1", "i:-1", "d:3ff8000000000000", "s:31", "s:312e35", "s:616263", "s:", "a:[]", `a:[["i:0","i:1"]]`}

// operand emits a register holding the encoded operand.
func (f *vmFn) operand(t testing.TB, enc string) uint32 {
	kind, body, _ := strings.Cut(enc, ":")
	switch kind {
	case "N":
		return f.k(nil)
	case "b":
		return f.k(body == "1")
	case "i":
		i, err := strconv.ParseInt(body, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return f.k(i)
	case "d":
		raw, _ := hex.DecodeString(body)
		return f.k(math.Float64frombits(binary.BigEndian.Uint64(raw)))
	case "s":
		raw, _ := hex.DecodeString(body)
		return f.k(string(raw))
	case "a":
		var pairs [][2]string
		if err := json.Unmarshal([]byte(body), &pairs); err != nil {
			t.Fatal(err)
		}
		tmp := f.hidden()
		f.emit(bytecode.OpStore, 0, tmp, f.newArray(), 0)
		for _, p := range pairs {
			f.emit(bytecode.OpArraySet, 0, tmp, f.operand(t, p[0]), f.operand(t, p[1]))
		}
		out := f.reg(bytecode.Value)
		f.emit(bytecode.OpLoad, 0, out, tmp, 0)
		f.emit(bytecode.OpUnset, 0, tmp, 0, 0)
		return out
	}
	t.Fatalf("operand %q", enc)
	return 0
}

// encodeSnapshot is encodeValue for an exported Return.
func encodeSnapshot(v any) string {
	switch x := v.(type) {
	case nil:
		return "N"
	case bool:
		if x {
			return "b:1"
		}
		return "b:0"
	case int64:
		return "i:" + strconv.FormatInt(x, 10)
	case float64:
		var raw [8]byte
		binary.BigEndian.PutUint64(raw[:], math.Float64bits(x))
		return "d:" + hex.EncodeToString(raw[:])
	case []byte:
		return "s:" + hex.EncodeToString(x)
	case []SnapshotEntry:
		pairs := [][2]string{}
		for _, e := range x {
			k := ""
			switch key := e.Key.(type) {
			case int64:
				k = "i:" + strconv.FormatInt(key, 10)
			case string:
				k = "s:" + hex.EncodeToString([]byte(key))
			}
			pairs = append(pairs, [2]string{k, encodeSnapshot(e.Value)})
		}
		raw, _ := json.Marshal(pairs)
		return "a:" + string(raw)
	}
	return fmt.Sprintf("?%T", v)
}

const vmOpLine = 7

// vmRunOp runs one operation built by emit at line vmOpLine and returns the
// observation in the ops reference's form. Diagnostics must be located at
// vmTestFile:vmOpLine.
func vmRunOp(t *testing.T, width int, operands []string, emit func(f *vmFn, regs []uint32) uint32) opsReferenceRun {
	t.Helper()
	a := newAsm(t, width)
	m := a.fn("{main}", 0, 3)
	regs := make([]uint32, len(operands))
	for i, o := range operands {
		regs[i] = m.at(3).operand(t, o)
	}
	m.at(vmOpLine)
	r := emit(m, regs)
	m.at(9).ret(r)
	cfg := vmConfig(t, width)
	cfg.DisplayErrors = false
	res := a.program().Run(cfg)
	var run opsReferenceRun
	switch res.End {
	case Completed:
		run.R = encodeSnapshot(res.Return)
	case UncaughtThrowable:
		run.E = []string{res.Throwable.Class, res.Throwable.Message}
		if res.Throwable.File != vmTestFile || res.Throwable.Line != vmOpLine {
			t.Errorf("throwable located at %s:%d", res.Throwable.File, res.Throwable.Line)
		}
	default:
		t.Fatalf("run ended %v: %v", res.End, res.Err)
	}
	for _, d := range res.Diagnostics {
		if d.Level == "Fatal error" {
			continue
		}
		if d.File != vmTestFile || d.Line != vmOpLine {
			t.Errorf("diagnostic %q located at %s:%d, want line %d", d.Message, d.File, d.Line, vmOpLine)
		}
		run.D = append(run.D, [2]any{float64(diagnosticLevelBit(d.Level)), d.Message})
	}
	return run
}

func TestVMBinaryWiring(t *testing.T) {
	ops := map[bytecode.BinaryOp]string{
		bytecode.Add: "+", bytecode.Subtract: "-", bytecode.Multiply: "*", bytecode.Divide: "/", bytecode.Concat: ".",
		bytecode.Equal: "==", bytecode.Identical: "===", bytecode.Less: "<", bytecode.BitAnd: "&", bytecode.BitOr: "|",
		bytecode.BitXor: "^", bytecode.Modulo: "%", bytecode.Power: "**", bytecode.ShiftLeft: "<<", bytecode.ShiftRight: ">>",
		bytecode.NotEqual: "!=", bytecode.LessOrEqual: "<=", bytecode.Spaceship: "<=>",
	}
	for _, width := range []int{32, 64} {
		ref := loadOpsReference(t, width)
		index := map[string]int{}
		for i, o := range ref.Operands {
			index[o] = i
		}
		n := len(ref.Operands)
		lookup := func(op, x, y string) opsReferenceRun {
			return ref.Binary[op][index[x]*n+index[y]]
		}
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for op, sym := range ops {
				for _, x := range vmValueSet {
					for _, y := range vmValueSet {
						got := vmRunOp(t, width, []string{x, y}, func(f *vmFn, r []uint32) uint32 { return f.bin(op, r[0], r[1]) })
						if want := lookup(sym, x, y); !sameRun(want, got) {
							t.Errorf("%s %s %s\n  php:   %s\n  phego: %s", x, sym, y, describeRun(want), describeRun(got))
						}
					}
				}
			}
			// a > b and a >= b are Less and LessOrEqual with swapped operands;
			// a !== b is the negation of ===.
			for _, x := range vmValueSet {
				for _, y := range vmValueSet {
					for sym, op := range map[string]bytecode.BinaryOp{">": bytecode.Less, ">=": bytecode.LessOrEqual} {
						got := vmRunOp(t, width, []string{x, y}, func(f *vmFn, r []uint32) uint32 { return f.bin(op, r[1], r[0]) })
						if want := lookup(sym, x, y); !sameRun(want, got) {
							t.Errorf("%s %s %s\n  php:   %s\n  phego: %s", x, sym, y, describeRun(want), describeRun(got))
						}
					}
					want := lookup("===", x, y)
					if want.R == "b:1" {
						want.R = "b:0"
					} else if want.R == "b:0" {
						want.R = "b:1"
					}
					got := vmRunOp(t, width, []string{x, y}, func(f *vmFn, r []uint32) uint32 { return f.bin(bytecode.NotIdentical, r[0], r[1]) })
					if !sameRun(want, got) {
						t.Errorf("%s !== %s\n  php:   %s\n  phego: %s", x, y, describeRun(want), describeRun(got))
					}
				}
			}
		})
	}
}

func TestVMUnaryAndCastWiring(t *testing.T) {
	unary := map[bytecode.UnaryOp]string{bytecode.Not: "!", bytecode.Negate: "neg", bytecode.Plus: "pos", bytecode.BitNot: "~"}
	casts := map[bytecode.CastKind]string{bytecode.CastBool: "(bool)", bytecode.CastInt: "(int)", bytecode.CastFloat: "(float)", bytecode.CastString: "(string)"}
	for _, width := range []int{32, 64} {
		ref := loadOpsReference(t, width)
		index := map[string]int{}
		for i, o := range ref.Operands {
			index[o] = i
		}
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			// The full 48-operand set: the VM adds fast paths for these.
			for op, sym := range unary {
				for i, x := range ref.Operands {
					if !fitsWidth(x, width) {
						continue // a constant cannot hold it; PHP's literal would be a float
					}
					got := vmRunOp(t, width, []string{x}, func(f *vmFn, r []uint32) uint32 { return f.unary(op, r[0]) })
					if want := ref.Unary[sym][i]; !sameRun(want, got) {
						t.Errorf("%s %s\n  php:   %s\n  phego: %s", sym, x, describeRun(want), describeRun(got))
					}
				}
			}
			for k, sym := range casts {
				for _, x := range vmValueSet {
					got := vmRunOp(t, width, []string{x}, func(f *vmFn, r []uint32) uint32 { return f.cast(k, r[0]) })
					if want := ref.Unary[sym][index[x]]; !sameRun(want, got) {
						t.Errorf("%s %s\n  php:   %s\n  phego: %s", sym, x, describeRun(want), describeRun(got))
					}
				}
			}
		})
	}
}

// TestVMFastBinaryMatchesHelpers checks the interpreter's int and float fast
// paths against the general helpers on every int and float operand of the
// ops reference, including the overflow boundaries.
func TestVMFastBinaryMatchesHelpers(t *testing.T) {
	for _, width := range []int{32, 64} {
		ref := loadOpsReference(t, width)
		var operands []string
		for _, o := range ref.Operands {
			if strings.HasPrefix(o, "i:") || strings.HasPrefix(o, "d:") {
				operands = append(operands, o)
			}
		}
		for _, o := range []string{"i:2147483647", "i:-2147483648", "i:2147483646", "i:65536"} {
			if !slices.Contains(operands, o) {
				operands = append(operands, o)
			}
		}
		h, err := referenceHeap(width)
		if err != nil {
			t.Fatal(err)
		}
		s := &vmState{h: h}
		for op := bytecode.Add; op <= bytecode.BoolXor; op++ {
			for _, x := range operands {
				for _, y := range operands {
					a, b := decodeOperand(t, h, x), decodeOperand(t, h, y)
					if width == 32 && (a.kind == valueInt && !h.inRange(a.integer) || b.kind == valueInt && !h.inRange(b.integer)) {
						continue
					}
					fast, ok := s.fastBinary(op, a, b)
					if !ok {
						continue
					}
					h.diagnostics = nil
					slow, err := s.binary(op, a, b)
					if err != nil || len(h.diagnostics) != 0 || canonicalNaN(encodeValue(fast)) != canonicalNaN(encodeValue(slow)) {
						t.Errorf("%d: op %d %s %s: fast %s, helper %s %v %v", width, op, x, y, encodeValue(fast), encodeValue(slow), err, h.diagnostics)
					}
				}
			}
		}
	}
}

func fitsWidth(enc string, width int) bool {
	if !strings.HasPrefix(enc, "i:") || width == 64 {
		return true
	}
	i, err := strconv.ParseInt(enc[2:], 10, 64)
	return err == nil && i >= math.MinInt32 && i <= math.MaxInt32
}
