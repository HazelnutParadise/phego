package runtime

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 1: REQ-025, REQ-026; T-009, T-010. Element writes that read
// their key or value variable themselves (bytecode.ArrayWriteFlags) and the
// array-literal instruction ArrayAdd, on hand-built images. The expected
// diagnostics are what the pinned PHP 8.5.12 image printed for the PHP shown
// with each case (php -n, error_reporting=-1); the same programs run end to
// end in the root package's TestW17ReviewProbes.

// vmShow renders an exported value compactly: N, b:1, i:5, s:"x", [k=>v,...].
func vmShow(v any) string {
	switch x := v.(type) {
	case nil:
		return "N"
	case bool:
		if x {
			return "b:1"
		}
		return "b:0"
	case int64:
		return fmt.Sprintf("i:%d", x)
	case []byte:
		return fmt.Sprintf("s:%q", x)
	case []SnapshotEntry:
		parts := make([]string, len(x))
		for i, e := range x {
			k := fmt.Sprint(e.Key)
			if s, ok := e.Key.(string); ok {
				k = fmt.Sprintf("%q", s)
			}
			parts[i] = k + "=>" + vmShow(e.Value)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return fmt.Sprintf("?%T", v)
}

// vmDiagnostics lists the logged diagnostics before any uncaught throwable,
// which the case checks separately.
func vmDiagnostics(res RunResult) []string {
	var out []string
	for _, d := range res.Diagnostics {
		if d.Level != "Fatal error" {
			out = append(out, d.Level+": "+d.Message)
		}
	}
	return out
}

type vmArrayCase struct {
	name   string
	php    string // the PHP the image encodes
	locals []string
	build  func(m *vmFn) uint32
	diags  []string
	end    RunEnd
	ret    string // the returned value when the run completes
	thrown string // "Class: message" of the uncaught throwable
}

func runArrayCases(t *testing.T, cases []vmArrayCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 2, tc.locals...)
			m.at(3)
			r := tc.build(m)
			m.ret(r)
			cfg := vmConfig(t, 64)
			cfg.DisplayErrors = false
			res := a.program().Run(cfg)
			if got := vmDiagnostics(res); strings.Join(got, "\n") != strings.Join(tc.diags, "\n") {
				t.Errorf("%s\ndiagnostics %q\nwant        %q", tc.php, got, tc.diags)
			}
			if res.End != tc.end {
				t.Fatalf("%s: end %v (%v), want %v", tc.php, res.End, res.Err, tc.end)
			}
			switch tc.end {
			case Completed:
				if got := vmShow(res.Return); got != tc.ret {
					t.Errorf("%s: returned %s, want %s", tc.php, got, tc.ret)
				}
			case UncaughtThrowable:
				if got := res.Throwable.Class + ": " + res.Throwable.Message; got != tc.thrown {
					t.Errorf("%s: threw %q, want %q", tc.php, got, tc.thrown)
				}
			}
		})
	}
}

const (
	vmNullOffset = "Deprecated: Using null as an array offset is deprecated, use an empty string instead"
	vmFalseArray = "Deprecated: Automatic conversion of false to array is deprecated"
)

func TestVMElementWriteReadsVariablesInPHPOrder(t *testing.T) {
	runArrayCases(t, []vmArrayCase{
		{name: "false container, undefined key and value", php: `$f = false; $f[$uk] = $uv; return $f;`,
			locals: []string{"f", "uk", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("f", m.k(false))
				m.setVars("f", "uk", 0, "uv", 0)
				return m.load("f")
			},
			diags: []string{vmFalseArray, "Warning: Undefined variable $uk", vmNullOffset, "Warning: Undefined variable $uv"},
			end:   Completed, ret: `[""=>N]`},
		{name: "undefined container", php: `$a[$uk] = $uv; return $a;`, locals: []string{"a", "uk", "uv"},
			build: func(m *vmFn) uint32 { m.setVars("a", "uk", 0, "uv", 0); return m.load("a") },
			diags: []string{"Warning: Undefined variable $uk", vmNullOffset, "Warning: Undefined variable $uv"},
			end:   Completed, ret: `[""=>N]`},
		{name: "float key converts before the value is read", php: `$a = []; $a[1.5] = $uv; return $a;`, locals: []string{"a", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.newArray())
				m.setVars("a", "", m.k(1.5), "uv", 0)
				return m.load("a")
			},
			diags: []string{"Deprecated: Implicit conversion from float 1.5 to int loses precision", "Warning: Undefined variable $uv"},
			end:   Completed, ret: `[1=>N]`},
		{name: "scalar container reads neither key nor value", php: `$s = 5; $s[$uk] = $uv;`, locals: []string{"s", "uk", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k(5))
				m.setVars("s", "uk", 0, "uv", 0)
				return m.k(nil)
			},
			end: UncaughtThrowable, thrown: "Error: Cannot use a scalar value as an array"},
		{name: "illegal key reads no value", php: `$a = []; $a[[]] = $uv;`, locals: []string{"a", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.newArray())
				m.setVars("a", "", m.newArray(), "uv", 0)
				return m.k(nil)
			},
			end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on array"},
		{name: "null container is an array before its key is read", php: `$n = null; $n[$n] = 1;`, locals: []string{"n"},
			build: func(m *vmFn) uint32 {
				m.store("n", m.k(nil))
				m.setVars("n", "n", 0, "", m.k(1))
				return m.k(nil)
			},
			end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on array"},
		{name: "false container is an array before its key is read", php: `$f = false; $f[$f] = 1;`, locals: []string{"f"},
			build: func(m *vmFn) uint32 {
				m.store("f", m.k(false))
				m.setVars("f", "f", 0, "", m.k(1))
				return m.k(nil)
			},
			diags: []string{vmFalseArray},
			end:   UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on array"},
		{name: "append to a string reads no value", php: `$s = "abc"; $s[] = $uv;`, locals: []string{"s", "uv"},
			build: func(m *vmFn) uint32 { m.store("s", m.k("abc")); m.pushVar("s", "uv"); return m.k(nil) },
			end:   UncaughtThrowable, thrown: "Error: [] operator not supported for strings"},
		{name: "append to true reads no value", php: `$t = true; $t[] = $uv;`, locals: []string{"t", "uv"},
			build: func(m *vmFn) uint32 { m.store("t", m.k(true)); m.pushVar("t", "uv"); return m.k(nil) },
			end:   UncaughtThrowable, thrown: "Error: Cannot use a scalar value as an array"},
		{name: "append overflow reads the value first", php: `$a = [PHP_INT_MAX => 1]; $a[] = $uv;`, locals: []string{"a", "uv"},
			build: func(m *vmFn) uint32 {
				arr := m.newArray()
				m.add(arr, m.k(int64(math.MaxInt64)), m.k(1))
				m.store("a", arr)
				m.pushVar("a", "uv")
				return m.k(nil)
			},
			diags: []string{"Warning: Undefined variable $uv"},
			end:   UncaughtThrowable, thrown: "Error: Cannot add element to the array as the next element is already occupied"},
		{name: "false container append", php: `$g = false; $g[] = $uv; return $g;`, locals: []string{"g", "uv"},
			build: func(m *vmFn) uint32 { m.store("g", m.k(false)); m.pushVar("g", "uv"); return m.load("g") },
			diags: []string{vmFalseArray, "Warning: Undefined variable $uv"},
			end:   Completed, ret: `[0=>N]`},
		// The compiler never passes the container itself as the value (PHP
		// copies it first); an image that does stores the array as it was
		// before the write, as PHP's copy does, never a recursive one.
		{name: "value location naming the container", php: `$a = [1]; $a["k"] = $a; $a[] = $a; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k(1)))
				m.setVars("a", "", m.k("k"), "a", 0)
				m.pushVar("a", "a")
				return m.load("a")
			},
			end: Completed, ret: `[0=>i:1,"k"=>[0=>i:1],1=>[0=>i:1,"k"=>[0=>i:1]]]`},
	})
}

func TestVMArrayAdd(t *testing.T) {
	runArrayCases(t, []vmArrayCase{
		{name: "undefined key variable is the empty string", php: `return [$uk => 1];`, locals: []string{"uk"},
			build: func(m *vmFn) uint32 { arr := m.newArray(); m.addVarKey(arr, "uk", m.k(1)); return arr },
			diags: []string{"Warning: Undefined variable $uk"},
			end:   Completed, ret: `[""=>i:1]`},
		{name: "null key variable", php: `$n = null; return [$n => 2];`, locals: []string{"n"},
			build: func(m *vmFn) uint32 {
				m.store("n", m.k(nil))
				arr := m.newArray()
				m.addVarKey(arr, "n", m.k(2))
				return arr
			},
			diags: []string{vmNullOffset},
			end:   Completed, ret: `[""=>i:2]`},
		{name: "literal keys", php: `return [null => 1, true => 2, false => 3, 1.5 => 4, "08" => 5, "8" => 6, 2.0 => 7, "x" => 8, "x" => 9];`,
			build: func(m *vmFn) uint32 {
				arr := m.newArray()
				for i, k := range []any{nil, true, false, 1.5, "08", "8", 2.0, "x", "x"} {
					m.add(arr, m.k(k), m.k(i+1))
				}
				return arr
			},
			diags: []string{vmNullOffset, "Deprecated: Implicit conversion from float 1.5 to int loses precision"},
			end:   Completed, ret: `[""=>i:1,1=>i:4,0=>i:3,"08"=>i:5,8=>i:6,2=>i:7,"x"=>i:9]`},
		{name: "next index after a negative key", php: `return [-5 => 1, 2];`,
			build: func(m *vmFn) uint32 {
				arr := m.newArray()
				m.add(arr, m.k(-5), m.k(1))
				m.emit(bytecode.OpArrayAdd, 0, arr, m.k(2), 0)
				return arr
			},
			end: Completed, ret: `[-5=>i:1,-4=>i:2]`},
		{name: "array key", php: `$k = []; return [$k => 1];`, locals: []string{"k"},
			build: func(m *vmFn) uint32 {
				m.store("k", m.newArray())
				arr := m.newArray()
				m.addVarKey(arr, "k", m.k(1))
				return arr
			},
			end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on array"},
		{name: "next element occupied", php: `return [PHP_INT_MAX => 1, 2];`,
			build: func(m *vmFn) uint32 {
				arr := m.newArray()
				m.add(arr, m.k(int64(math.MaxInt64)), m.k(1))
				m.emit(bytecode.OpArrayAdd, 0, arr, m.k(2), 0)
				return arr
			},
			end: UncaughtThrowable, thrown: "Error: Cannot add element to the array as the next element is already occupied"},
		{name: "adds to its own register only", php: `$b = [1]; $c = $b; $b[] = 2 (in a register)`,
			build: func(m *vmFn) uint32 {
				arr := m.arrayOf(m.k(1))
				other := m.reg(bytecode.Value)
				m.emit(bytecode.OpMove, 0, other, arr, 0)
				m.emit(bytecode.OpArrayAdd, 0, arr, m.k(2), 0)
				return m.arrayOf(arr, other)
			},
			end: Completed, ret: `[0=>[0=>i:1,1=>i:2],1=>[0=>i:1]]`},
		{name: "array added to itself is a copy", php: `$a = [1]; $a[] = $a (in a register)`,
			build: func(m *vmFn) uint32 {
				arr := m.arrayOf(m.k(1))
				m.emit(bytecode.OpArrayAdd, 0, arr, arr, 0)
				return arr
			},
			end: Completed, ret: `[0=>i:1,1=>[0=>i:1]]`},
		{name: "non-array register", php: `(not produced by the compiler)`,
			build: func(m *vmFn) uint32 {
				r := m.k(1)
				m.emit(bytecode.OpArrayAdd, 0, r, m.k(2), 0)
				return r
			},
			end: Unsupported},
	})
}
