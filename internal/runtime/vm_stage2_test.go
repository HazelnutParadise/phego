package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 2: REQ-025, REQ-026; T-009, T-010. The format 1.2 instructions
// on hand-built images: FetchDim, the result register of ArraySet, string
// offsets, the foreach iterator and ParamDefault. The expected diagnostics
// are what the pinned PHP 8.5.12 image printed for the PHP shown with each
// case (php -n, error_reporting=-1); the same programs run end to end, from
// source, in the root package's TestW17Stage2Probes.

// elem is a Location register for an element: FetchDim defines it.
func (f *vmFn) elem() uint32 { return f.reg(bytecode.Location) }

// fetch is FetchDim: dst names container[key]. keyVar, when set, is a
// variable the instruction reads itself.
func (f *vmFn) fetch(dst, container uint32, flags bytecode.FetchFlags, keyVar string, key uint32) {
	if keyVar != "" {
		flags |= bytecode.FetchKeyLocation
		key = f.loc(keyVar)
	}
	if flags&bytecode.FetchAppend != 0 {
		key = 0
	}
	f.emit(bytecode.OpFetchDim, uint16(flags), dst, container, key)
}

// setAt is ArraySet through any Location, with an optional result register.
func (f *vmFn) setAt(container, key, v uint32, result bool) uint32 {
	if !result {
		f.emit(bytecode.OpArraySet, 0, container, key, v)
		return 0
	}
	r := f.reg(bytecode.Value)
	aux, ok := bytecode.ArraySetAux(0, r)
	if !ok {
		f.a.t.Fatalf("register %d cannot be an ArraySet result", r)
	}
	f.emit(bytecode.OpArraySet, aux, container, key, v)
	return r
}

func (f *vmFn) loadAt(l uint32) uint32 {
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpLoad, 1, r, l, 0)
	return r
}

const (
	vmScalarArray  = "Error: Cannot use a scalar value as an array"
	vmStringArray  = "Error: Cannot use string offset as an array"
	vmOffsetCast   = "Warning: String offset cast occurred"
	vmStringAppend = "Error: [] operator not supported for strings"
)

func TestVMFetchDim(t *testing.T) {
	const W, RW, U = bytecode.FetchWrite, bytecode.FetchReadWrite, bytecode.FetchUnset
	runArrayCases(t, []vmArrayCase{
		{name: "write creates the path", php: `$a[1][2] = 3; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "", m.k(1))
				m.setAt(e, m.k(2), m.k(3), false)
				return m.load("a")
			}, end: Completed, ret: `[1=>[2=>i:3]]`},
		{name: "three levels reuse one element register", php: `$a["x"]["y"]["z"] = 1; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "", m.k("x"))
				m.fetch(e, e, W, "", m.k("y"))
				m.setAt(e, m.k("z"), m.k(1), false)
				return m.load("a")
			}, end: Completed, ret: `["x"=>["y"=>["z"=>i:1]]]`},
		{name: "append in the middle", php: `$c[][] = 5; $c[0][] = 6; return $c;`, locals: []string{"c"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("c"), W|bytecode.FetchAppend, "", 0)
				m.emit(bytecode.OpArrayAppend, 0, e, m.k(5), 0)
				m.fetch(e, m.loc("c"), W, "", m.k(0))
				m.emit(bytecode.OpArrayAppend, 0, e, m.k(6), 0)
				return m.load("c")
			}, end: Completed, ret: `[0=>[0=>i:5,1=>i:6]]`},
		{name: "read-write warns for each missing key", php: `$a = []; $a["x"]["y"] .= "w"; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				e := m.elem()
				m.fetch(e, m.loc("a"), RW, "", m.k("x"))
				m.fetch(e, e, RW|bytecode.FetchForAssignOp, "", m.k("y"))
				m.emit(bytecode.OpStore, 0, e, m.bin(bytecode.Concat, m.loadAt(e), m.k("w")), 0)
				return m.load("a")
			},
			diags: []string{`Warning: Undefined array key "x"`, `Warning: Undefined array key "y"`},
			end:   Completed, ret: `["x"=>["y"=>s:"w"]]`},
		{name: "read-write on an undefined variable", php: `$u["p"]["q"] -= 1; return $u;`, locals: []string{"u"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("u"), RW, "", m.k("p"))
				m.fetch(e, e, RW|bytecode.FetchForAssignOp, "", m.k("q"))
				m.emit(bytecode.OpStore, 0, e, m.bin(bytecode.Subtract, m.loadAt(e), m.k(1)), 0)
				return m.load("u")
			},
			diags: []string{"Warning: Undefined variable $u", `Warning: Undefined array key "p"`, `Warning: Undefined array key "q"`},
			end:   Completed, ret: `["p"=>["q"=>i:-1]]`},
		{name: "write on an undefined variable is silent", php: `$u["p"]["q"] = 1; return $u;`, locals: []string{"u"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("u"), W, "", m.k("p"))
				m.setAt(e, m.k("q"), m.k(1), false)
				return m.load("u")
			}, end: Completed, ret: `["p"=>["q"=>i:1]]`},
		{name: "increment through an element", php: `$a = [5]; $x = $a[0]++; $a["n"]--; return [$x, $a];`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k(5)))
				e := m.elem()
				m.fetch(e, m.loc("a"), RW|bytecode.FetchForIncDec, "", m.k(0))
				x := m.reg(bytecode.Value)
				m.emit(bytecode.OpIncDec, uint16(bytecode.PostInc), x, e, 0)
				m.fetch(e, m.loc("a"), RW|bytecode.FetchForIncDec, "", m.k("n"))
				m.emit(bytecode.OpIncDec, uint16(bytecode.PostDec), m.reg(bytecode.Value), e, 0)
				return m.arrayOf(x, m.load("a"))
			},
			diags: []string{`Warning: Undefined array key "n"`, "Warning: Decrement on type null has no effect, this will change in the next major version of PHP"},
			end:   Completed, ret: `[0=>i:5,1=>[0=>i:6,"n"=>N]]`},
		{name: "read-write append creates null silently", php: `$a = []; $a[]++; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				e := m.elem()
				m.fetch(e, m.loc("a"), RW|bytecode.FetchAppend|bytecode.FetchForIncDec, "", 0)
				m.emit(bytecode.OpIncDec, uint16(bytecode.PostInc), m.reg(bytecode.Value), e, 0)
				return m.load("a")
			}, end: Completed, ret: `[0=>i:1]`},
		{name: "undefined key variable", php: `$a = []; $a[$u1][$u2] = $u3; return $a;`, locals: []string{"a", "u1", "u2", "u3"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "u1", 0)
				m.emit(bytecode.OpArraySet, uint16(bytecode.KeyLocation|bytecode.ValueLocation), e, m.loc("u2"), m.loc("u3"))
				return m.load("a")
			},
			diags: []string{"Warning: Undefined variable $u1", vmNullOffset, "Warning: Undefined variable $u2", vmNullOffset, "Warning: Undefined variable $u3"},
			end:   Completed, ret: `[""=>[""=>N]]`},
		{name: "false becomes an array at each level", php: `$a = [false]; $a[0][1] = 2; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k(false)))
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "", m.k(0))
				m.setAt(e, m.k(1), m.k(2), false)
				return m.load("a")
			}, diags: []string{vmFalseArray}, end: Completed, ret: `[0=>[1=>i:2]]`},
		{name: "false variable in write mode", php: `$f = false; $f[1][2] = 3; return $f;`, locals: []string{"f"},
			build: func(m *vmFn) uint32 {
				m.store("f", m.k(false))
				e := m.elem()
				m.fetch(e, m.loc("f"), W, "", m.k(1))
				m.setAt(e, m.k(2), m.k(3), false)
				return m.load("f")
			}, diags: []string{vmFalseArray}, end: Completed, ret: `[1=>[2=>i:3]]`},
		{name: "array key", php: `$a = []; $a[[]][$uk] = $uv;`, locals: []string{"a", "uk", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "", m.arrayOf())
				m.emit(bytecode.OpArraySet, uint16(bytecode.KeyLocation|bytecode.ValueLocation), e, m.loc("uk"), m.loc("uv"))
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on array"},
		{name: "append when the next index is taken", php: `$a = [PHP_INT_MAX => 1]; $a[][$uk] = $uv;`, locals: []string{"a", "uk", "uv"},
			build: func(m *vmFn) uint32 {
				lit := m.newArray()
				m.add(lit, m.k(int64(9223372036854775807)), m.k(1))
				m.store("a", lit)
				e := m.elem()
				m.fetch(e, m.loc("a"), W|bytecode.FetchAppend, "", 0)
				m.emit(bytecode.OpArraySet, uint16(bytecode.KeyLocation|bytecode.ValueLocation), e, m.loc("uk"), m.loc("uv"))
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "Error: Cannot add element to the array as the next element is already occupied"},
		// Scalars.
		{name: "scalar container", php: `$i = 5; $i[$u1][$u2] = $u3;`, locals: []string{"i", "u1"},
			build: func(m *vmFn) uint32 {
				m.store("i", m.k(5))
				m.fetch(m.elem(), m.loc("i"), W, "u1", 0)
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: vmScalarArray},
		{name: "scalar container, increment reads no key", php: `$i = 5; $i[$uk]++;`, locals: []string{"i", "uk"},
			build: func(m *vmFn) uint32 {
				m.store("i", m.k(5))
				m.fetch(m.elem(), m.loc("i"), RW|bytecode.FetchForIncDec, "uk", 0)
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: vmScalarArray},
		{name: "scalar container, assign-op reads the key first", php: `$i = 5; $i[$uk] .= "x";`, locals: []string{"i", "uk"},
			build: func(m *vmFn) uint32 {
				m.store("i", m.k(5))
				m.fetch(m.elem(), m.loc("i"), RW|bytecode.FetchForAssignOp, "uk", 0)
				return m.k(nil)
			}, diags: []string{"Warning: Undefined variable $uk"}, end: UncaughtThrowable, thrown: vmScalarArray},
		{name: "scalar container, unset", php: `$i = 5; unset($i[$u1][$u2]);`, locals: []string{"i", "u1"},
			build: func(m *vmFn) uint32 {
				m.store("i", m.k(5))
				m.fetch(m.elem(), m.loc("i"), U, "u1", 0)
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "Error: Cannot unset offset in a non-array variable"},
		// Strings.
		{name: "string container", php: `$s = "str"; $s[$u1][$u2] = $u3;`, locals: []string{"s", "u1"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("str"))
				m.fetch(m.elem(), m.loc("s"), W, "u1", 0)
				return m.k(nil)
			}, diags: []string{"Warning: Undefined variable $u1", vmOffsetCast}, end: UncaughtThrowable, thrown: vmStringArray},
		{name: "string container, increment", php: `$s = "abc"; $s[1.5]++;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), RW|bytecode.FetchForIncDec, "", m.k(1.5))
				return m.k(nil)
			}, diags: []string{vmOffsetCast}, end: UncaughtThrowable, thrown: "Error: Cannot increment/decrement string offsets"},
		{name: "string container, assign-op", php: `$s = "abc"; $s["1x"] .= $uv;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), RW|bytecode.FetchForAssignOp, "", m.k("1x"))
				return m.k(nil)
			}, diags: []string{`Warning: Illegal string offset "1x"`}, end: UncaughtThrowable, thrown: "Error: Cannot use assign-op operators with string offsets"},
		{name: "string container, append", php: `$s = "str"; $s[][0] = 1;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("str"))
				m.fetch(m.elem(), m.loc("s"), W|bytecode.FetchAppend, "", 0)
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: vmStringAppend},
		{name: "string container, array key", php: `$s = "abc"; $s[[]][0] = 1;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), W, "", m.arrayOf())
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type array on string"},
		{name: "string container, non-numeric key", php: `$s = "abc"; $s["x"]++;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), RW|bytecode.FetchForIncDec, "", m.k("x"))
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type string on string"},
		{name: "string container, unset with an array key", php: `$s = "abc"; unset($s[[]][0]);`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), U, "", m.arrayOf())
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: "Error: Cannot unset string offsets"},
		{name: "string container, unset with a leading-numeric key", php: `$s = "abc"; unset($s["1x"][0]);`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.fetch(m.elem(), m.loc("s"), U, "", m.k("1x"))
				return m.k(nil)
			}, end: UncaughtThrowable, thrown: vmStringArray},
		// Unset mode.
		{name: "unset creates nothing", php: `$a = ["n" => ["x" => 1]]; unset($a["n"]["x"]); unset($a["no"]["x"]); unset($a["n"]["zz"]["q"]); return $a;`,
			locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				inner := m.newArray()
				m.add(inner, m.k("x"), m.k(1))
				lit := m.newArray()
				m.add(lit, m.k("n"), inner)
				m.store("a", lit)
				e := m.elem()
				m.fetch(e, m.loc("a"), U, "", m.k("n"))
				m.emit(bytecode.OpArrayUnset, 0, e, m.k("x"), 0)
				m.fetch(e, m.loc("a"), U, "", m.k("no"))
				m.emit(bytecode.OpArrayUnset, 0, e, m.k("x"), 0)
				m.fetch(e, m.loc("a"), U, "", m.k("n"))
				m.fetch(e, e, U, "", m.k("zz"))
				m.emit(bytecode.OpArrayUnset, 0, e, m.k("q"), 0)
				return m.load("a")
			}, end: Completed, ret: `["n"=>[]]`},
		{name: "unset on an undefined variable", php: `unset($u["a"]["b"]); return isset($u);`, locals: []string{"u"},
			build: func(m *vmFn) uint32 {
				e := m.elem()
				m.fetch(e, m.loc("u"), U, "", m.k("a"))
				m.emit(bytecode.OpArrayUnset, 0, e, m.k("b"), 0)
				return m.isset("u")
			}, diags: []string{"Warning: Undefined variable $u"}, end: Completed, ret: `b:0`},
		{name: "unset through false", php: `$f = false; unset($f["a"]["b"]); return $f;`, locals: []string{"f"},
			build: func(m *vmFn) uint32 {
				m.store("f", m.k(false))
				e := m.elem()
				m.fetch(e, m.loc("f"), U, "", m.k("a"))
				m.emit(bytecode.OpArrayUnset, 0, e, m.k("b"), 0)
				return m.load("f")
			}, diags: []string{vmFalseArray}, end: Completed, ret: `b:0`},
		{name: "unset with undefined key variables", php: `$a = []; unset($a[$k1][$k2]); return $a;`, locals: []string{"a", "k1", "k2"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				e := m.elem()
				m.fetch(e, m.loc("a"), U, "k1", 0)
				m.emit(bytecode.OpArrayUnset, uint16(bytecode.KeyLocation), e, m.loc("k2"), 0)
				return m.load("a")
			}, diags: []string{"Warning: Undefined variable $k1", vmNullOffset, "Warning: Undefined variable $k2"}, end: Completed, ret: `[]`},
		{name: "unset of a string element's offset", php: `$s = ["str"]; unset($s[0][$uk]);`, locals: []string{"s", "uk"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.arrayOf(m.k("str")))
				e := m.elem()
				m.fetch(e, m.loc("s"), U, "", m.k(0))
				m.emit(bytecode.OpArrayUnset, uint16(bytecode.KeyLocation), e, m.loc("uk"), 0)
				return m.k(nil)
			}, diags: []string{"Warning: Undefined variable $uk"}, end: UncaughtThrowable, thrown: "Error: Cannot unset string offsets"},
		// Copy on write.
		{name: "a nested write separates each shared level", php: `$a = [[1, 2]]; $b = $a; $in = $a[0]; $a[0][0] = 9; return [$a, $b, $in];`,
			locals: []string{"a", "b", "in"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.arrayOf(m.k(1), m.k(2))))
				m.store("b", m.load("a"))
				m.store("in", m.get(m.load("a"), m.k(0), false))
				e := m.elem()
				m.fetch(e, m.loc("a"), W, "", m.k(0))
				m.setAt(e, m.k(0), m.k(9), false)
				return m.arrayOf(m.load("a"), m.load("b"), m.load("in"))
			}, end: Completed, ret: `[0=>[0=>[0=>i:9,1=>i:2]],1=>[0=>[0=>i:1,1=>i:2]],2=>[0=>i:1,1=>i:2]]`},
	})
}

func TestVMStringOffsetWrite(t *testing.T) {
	set := func(key, v any, result bool) func(m *vmFn) uint32 {
		return func(m *vmFn) uint32 {
			m.store("s", m.k("abc"))
			r := m.setAt(m.loc("s"), m.k(key), m.k(v), result)
			if result {
				return m.arrayOf(r, m.load("s"))
			}
			return m.load("s")
		}
	}
	const firstByte = "Warning: Only the first byte will be assigned to the string offset"
	runArrayCases(t, []vmArrayCase{
		{name: "in range", php: `$s = "abc"; $s[1] = "X";`, locals: []string{"s"}, build: set(1, "X", false), end: Completed, ret: `s:"aXc"`},
		{name: "past the end pads with spaces", php: `$s[5] = "!";`, locals: []string{"s"}, build: set(5, "!", false), end: Completed, ret: `s:"abc  !"`},
		{name: "negative", php: `$s[-1] = "z";`, locals: []string{"s"}, build: set(-1, "z", false), end: Completed, ret: `s:"abz"`},
		{name: "first byte only", php: `$s[0] = "long";`, locals: []string{"s"}, build: set(0, "long", false), diags: []string{firstByte}, end: Completed, ret: `s:"lbc"`},
		{name: "integer value", php: `$s[0] = 65;`, locals: []string{"s"}, build: set(0, 65, false), diags: []string{firstByte}, end: Completed, ret: `s:"6bc"`},
		{name: "true value", php: `$s[2] = true;`, locals: []string{"s"}, build: set(2, true, false), end: Completed, ret: `s:"ab1"`},
		{name: "numeric string key", php: `$s["2"] = "q";`, locals: []string{"s"}, build: set("2", "q", false), end: Completed, ret: `s:"abq"`},
		{name: "leading-numeric key", php: `$s["1x"] = "A";`, locals: []string{"s"}, build: set("1x", "A", false),
			diags: []string{`Warning: Illegal string offset "1x"`}, end: Completed, ret: `s:"aAc"`},
		{name: "float key", php: `$s[1.7] = "F";`, locals: []string{"s"}, build: set(1.7, "F", false), diags: []string{vmOffsetCast}, end: Completed, ret: `s:"aFc"`},
		{name: "null key", php: `$s[null] = "G";`, locals: []string{"s"}, build: set(nil, "G", false), diags: []string{vmOffsetCast}, end: Completed, ret: `s:"Gbc"`},
		{name: "before the start", php: `$s[-7] = "H";`, locals: []string{"s"}, build: set(-7, "H", false),
			diags: []string{"Warning: Illegal string offset -7"}, end: Completed, ret: `s:"abc"`},
		{name: "non-numeric key", php: `$s["x"] = "y";`, locals: []string{"s"}, build: set("x", "y", false),
			end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type string on string"},
		{name: "float string key", php: `$s["1.5"] = "A";`, locals: []string{"s"}, build: set("1.5", "A", false),
			end: UncaughtThrowable, thrown: "TypeError: Cannot access offset of type string on string"},
		{name: "empty value", php: `$s[1] = "";`, locals: []string{"s"}, build: set(1, "", false),
			end: UncaughtThrowable, thrown: "Error: Cannot assign an empty string to a string offset"},
		{name: "null value", php: `$s[1] = null;`, locals: []string{"s"}, build: set(1, nil, false),
			end: UncaughtThrowable, thrown: "Error: Cannot assign an empty string to a string offset"},
		{name: "result is the byte", php: `$r = $s[0] = "xyz"; return [$r, $s];`, locals: []string{"s"}, build: set(0, "xyz", true),
			diags: []string{firstByte}, end: Completed, ret: `[0=>s:"x",1=>s:"xbc"]`},
		{name: "result of an illegal offset is null", php: `$r = $s[-9] = "q"; return [$r, $s];`, locals: []string{"s"}, build: set(-9, "q", true),
			diags: []string{"Warning: Illegal string offset -9"}, end: Completed, ret: `[0=>N,1=>s:"abc"]`},
		{name: "result of an array element write is the value", php: `$a = []; $r = $a["k"] = 7; return [$r, $a];`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf())
				r := m.setAt(m.loc("a"), m.k("k"), m.k(7), true)
				return m.arrayOf(r, m.load("a"))
			}, end: Completed, ret: `[0=>i:7,1=>["k"=>i:7]]`},
		{name: "the value variable is read after the offset", php: `$s[$uk] = $uv;`, locals: []string{"s", "uk", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.emit(bytecode.OpArraySet, uint16(bytecode.KeyLocation|bytecode.ValueLocation), m.loc("s"), m.loc("uk"), m.loc("uv"))
				return m.k(nil)
			},
			diags: []string{"Warning: Undefined variable $uk", vmOffsetCast, "Warning: Undefined variable $uv"},
			end:   UncaughtThrowable, thrown: "Error: Cannot assign an empty string to a string offset"},
		{name: "an illegal offset does not read the value", php: `$s[-9] = $uv; return $s;`, locals: []string{"s", "uv"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.emit(bytecode.OpArraySet, uint16(bytecode.ValueLocation), m.loc("s"), m.k(-9), m.loc("uv"))
				return m.load("s")
			}, diags: []string{"Warning: Illegal string offset -9"}, end: Completed, ret: `s:"abc"`},
		{name: "array value", php: `$s[1] = [1]; return $s;`, locals: []string{"s"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.setAt(m.loc("s"), m.k(1), m.arrayOf(m.k(1)), false)
				return m.load("s")
			}, diags: []string{"Warning: Array to string conversion", firstByte}, end: Completed, ret: `s:"aAc"`},
		{name: "a string element of an array", php: `$a = ["str"]; $a[0][1] = "X"; return $a;`, locals: []string{"a"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k("str")))
				e := m.elem()
				m.fetch(e, m.loc("a"), bytecode.FetchWrite, "", m.k(0))
				m.setAt(e, m.k(1), m.k("X"), false)
				return m.load("a")
			}, end: Completed, ret: `[0=>s:"sXr"]`},
		{name: "a copy keeps its bytes", php: `$s = "abc"; $t = $s; $s[0] = "X"; return [$s, $t];`, locals: []string{"s", "t"},
			build: func(m *vmFn) uint32 {
				m.store("s", m.k("abc"))
				m.store("t", m.load("s"))
				m.setAt(m.loc("s"), m.k(0), m.k("X"), false)
				return m.arrayOf(m.load("s"), m.load("t"))
			}, end: Completed, ret: `[0=>s:"Xbc",1=>s:"abc"]`},
	})
	t.Run("an offset past the memory bound ends the run", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2, "s")
		m.store("s", m.k("abc"))
		m.echoK("a")
		m.setAt(m.loc("s"), m.k(int64(vmStringOffsetLimit)), m.k("x"), false)
		m.echoK("never")
		m.retNull()
		res := runAsm(t, a)
		var le *LimitError
		if res.End != LimitExceeded || !errors.As(res.Err, &le) || le.Resource != "memory" || string(res.Output) != "a" {
			t.Fatalf("%v %v %q", res.End, res.Err, res.Output)
		}
		if len(res.Diagnostics) != 0 {
			t.Fatalf("a host stop must not be reported as PHP: %v", res.Diagnostics)
		}
	})
}

func TestVMStringOffsetRead(t *testing.T) {
	type c struct {
		key   any
		quiet bool
		ret   string
		diags []string
		throw string
	}
	arr := struct{}{}
	uninit := func(n string) []string { return []string{"Warning: Uninitialized string offset " + n} }
	cases := []c{
		{0, false, `s:"a"`, nil, ""}, {5, false, `s:"f"`, nil, ""}, {-1, false, `s:"f"`, nil, ""}, {-6, false, `s:"a"`, nil, ""},
		{6, false, `s:""`, uninit("6"), ""}, {-7, false, `s:""`, uninit("-7"), ""},
		{"1", false, `s:"b"`, nil, ""}, {" 2", false, `s:"c"`, nil, ""}, {"3 ", false, `s:"d"`, nil, ""}, {"-1", false, `s:"f"`, nil, ""},
		{"1x", false, `s:"b"`, []string{`Warning: Illegal string offset "1x"`}, ""},
		{"0x1", false, `s:"a"`, []string{`Warning: Illegal string offset "0x1"`}, ""},
		{1.7, false, `s:"b"`, []string{vmOffsetCast}, ""}, {true, false, `s:"b"`, []string{vmOffsetCast}, ""},
		{false, false, `s:"a"`, []string{vmOffsetCast}, ""}, {nil, false, `s:"a"`, []string{vmOffsetCast}, ""},
		{-0.5, false, `s:"a"`, []string{vmOffsetCast}, ""},
		{1e3, false, `s:""`, []string{vmOffsetCast, "Warning: Uninitialized string offset 1000"}, ""},
		{1e30, false, `s:""`, []string{vmOffsetCast, "Warning: Uninitialized string offset 5076964154930102272"}, ""},
		{"9", false, `s:""`, uninit("9"), ""},
		{"x", false, "", nil, "TypeError: Cannot access offset of type string on string"},
		{"1.5", false, "", nil, "TypeError: Cannot access offset of type string on string"},
		{"", false, "", nil, "TypeError: Cannot access offset of type string on string"},
		{"1e1", false, "", nil, "TypeError: Cannot access offset of type string on string"},
		{arr, false, "", nil, "TypeError: Cannot access offset of type array on string"},
		// FETCH_DIM_IS, as ?? reads.
		{0, true, `s:"a"`, nil, ""}, {6, true, `N`, nil, ""}, {-7, true, `N`, nil, ""}, {"1", true, `s:"b"`, nil, ""},
		{"x", true, `N`, nil, ""}, {"1.0", true, `N`, nil, ""}, {"", true, `N`, nil, ""}, {" 1", true, `s:"b"`, nil, ""},
		{"1x", true, `s:"b"`, []string{`Warning: Illegal string offset "1x"`}, ""},
		{1.5, true, `s:"b"`, nil, ""}, {true, true, `s:"b"`, nil, ""}, {nil, true, `s:"a"`, nil, ""}, {1e30, true, `N`, nil, ""},
		{arr, true, "", nil, "TypeError: Cannot access offset of type array on string"},
	}
	var table []vmArrayCase
	for _, tc := range cases {
		end := Completed
		if tc.throw != "" {
			end = UncaughtThrowable
		}
		op := map[bool]string{false: `"abcdef"[%v]`, true: `"abcdef"[%v] ?? null`}[tc.quiet]
		table = append(table, vmArrayCase{name: fmt.Sprintf(op, tc.key), php: fmt.Sprintf("return "+op+";", tc.key),
			build: func(m *vmFn) uint32 {
				var key uint32
				if tc.key == arr {
					key = m.arrayOf()
				} else {
					key = m.k(tc.key)
				}
				return m.get(m.k("abcdef"), key, tc.quiet)
			}, diags: tc.diags, end: end, ret: tc.ret, thrown: tc.throw})
	}
	runArrayCases(t, table)
	t.Run("32-bit offsets", func(t *testing.T) {
		a := newAsm(t, 32)
		m := a.fn("{main}", 0, 2)
		m.ret(m.arrayOf(m.get(m.k("abc"), m.k(2147483648.0), false), m.get(m.k("abc"), m.k(-2147483649.0), false), m.get(m.k("abc"), m.k("2147483648"), true)))
		cfg := vmConfig(t, 32)
		cfg.DisplayErrors = false
		res := a.program().Run(cfg)
		want := []string{vmOffsetCast, "Warning: Uninitialized string offset -2147483648", vmOffsetCast, "Warning: Uninitialized string offset 2147483647"}
		if got := vmDiagnostics(res); strings.Join(got, "\n") != strings.Join(want, "\n") || vmShow(res.Return) != `[0=>s:"",1=>s:"",2=>N]` {
			t.Fatalf("%q\n%s (%v)", got, vmShow(res.Return), res.Err)
		}
	})
}

// foreachLoop emits foreach (subject as [$key =>] $value) { body } with the
// loop variables written by the iterator instructions themselves, and
// returns the pc of IterNext.
func (f *vmFn) foreachLoop(subject uint32, key, val string, body func()) uint32 {
	it := f.reg(bytecode.Value)
	init := f.emit(bytecode.OpIterInit, 0, it, subject, 0)
	head := f.pc()
	next := f.emit(bytecode.OpIterNext, uint16(bytecode.IterLocation), it, f.loc(val), 0)
	if key != "" {
		f.emit(bytecode.OpIterKey, uint16(bytecode.IterLocation), f.loc(key), it, 0)
	}
	body()
	f.jump(head)
	f.code[next].C = f.pc()
	f.emit(bytecode.OpDrop, 0, it, 0, 0)
	f.code[init].C = f.pc()
	return head
}

func TestVMForeach(t *testing.T) {
	runArrayCases(t, []vmArrayCase{
		{name: "values and keys in order", php: `$o = ""; foreach (["x" => 1, 5 => "five", "" => true] as $k => $v) { $o .= "[$k]=$v;"; } return $o;`,
			locals: []string{"o", "k", "v"},
			build: func(m *vmFn) uint32 {
				m.store("o", m.k(""))
				lit := m.newArray()
				m.add(lit, m.k("x"), m.k(1))
				m.add(lit, m.k(5), m.k("five"))
				m.add(lit, m.k(""), m.k(true))
				m.foreachLoop(lit, "k", "v", func() {
					s := m.bin(bytecode.Concat, m.bin(bytecode.Concat, m.bin(bytecode.Concat, m.k("["), m.load("k")), m.k("]=")), m.load("v"))
					m.store("o", m.bin(bytecode.Concat, m.load("o"), m.bin(bytecode.Concat, s, m.k(";"))))
				})
				return m.load("o")
			}, end: Completed, ret: `s:"[x]=1;[5]=five;[]=1;"`},
		{name: "an empty array leaves the variables alone", php: `foreach ([] as $k => $v) {} return [isset($k), isset($v)];`, locals: []string{"k", "v"},
			build: func(m *vmFn) uint32 {
				m.foreachLoop(m.arrayOf(), "k", "v", func() {})
				return m.arrayOf(m.isset("k"), m.isset("v"))
			}, end: Completed, ret: `[0=>b:0,1=>b:0]`},
		{name: "the loop iterates the array as it was", php: `$a = [1, 2, 3]; $o = ""; foreach ($a as $k => $v) { $a[] = $v * 10; $a[1] = "c"; unset($a[2]); $o .= $v; } return [$o, $a];`,
			locals: []string{"a", "o", "k", "v"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k(1), m.k(2), m.k(3)))
				m.store("o", m.k(""))
				m.foreachLoop(m.load("a"), "k", "v", func() {
					m.push("a", m.bin(bytecode.Multiply, m.load("v"), m.k(10)))
					m.set("a", m.k(1), m.k("c"))
					m.unsetKey("a", m.k(2))
					m.store("o", m.bin(bytecode.Concat, m.load("o"), m.load("v")))
				})
				return m.arrayOf(m.load("o"), m.load("a"))
			}, end: Completed, ret: `[0=>s:"123",1=>[0=>i:1,1=>s:"c",3=>i:10,4=>i:20,5=>i:30]]`},
		{name: "the array variable may be the loop variable", php: `$e = [1, 2]; $o = ""; foreach ($e as $e) { $o .= $e; } return [$o, $e];`,
			locals: []string{"e", "o"},
			build: func(m *vmFn) uint32 {
				m.store("e", m.arrayOf(m.k(1), m.k(2)))
				m.store("o", m.k(""))
				m.foreachLoop(m.load("e"), "", "e", func() { m.store("o", m.bin(bytecode.Concat, m.load("o"), m.load("e"))) })
				return m.arrayOf(m.load("o"), m.load("e"))
			}, end: Completed, ret: `[0=>s:"12",1=>i:2]`},
		{name: "the key is assigned after the value", php: `foreach (["k1" => "v1"] as $x => $x) {} return $x;`, locals: []string{"x"},
			build: func(m *vmFn) uint32 {
				lit := m.newArray()
				m.add(lit, m.k("k1"), m.k("v1"))
				m.foreachLoop(lit, "x", "x", func() {})
				return m.load("x")
			}, end: Completed, ret: `s:"k1"`},
		{name: "holes are skipped", php: `$a = [0, 1, 2, 3, 4]; unset($a[1], $a[3]); $o = ""; foreach ($a as $k => $v) { $o .= $k; } return $o;`,
			locals: []string{"a", "o", "k", "v"},
			build: func(m *vmFn) uint32 {
				m.store("a", m.arrayOf(m.k(0), m.k(1), m.k(2), m.k(3), m.k(4)))
				m.unsetKey("a", m.k(1))
				m.unsetKey("a", m.k(3))
				m.store("o", m.k(""))
				m.foreachLoop(m.load("a"), "k", "v", func() { m.store("o", m.bin(bytecode.Concat, m.load("o"), m.load("k"))) })
				return m.load("o")
			}, end: Completed, ret: `s:"024"`},
		{name: "value registers as destinations", php: `$o = ""; foreach ([7, 8] as $k => $v) { $o .= $k . $v; } return $o;`, locals: []string{"o"},
			build: func(m *vmFn) uint32 {
				m.store("o", m.k(""))
				it, v, k := m.reg(bytecode.Value), m.reg(bytecode.Value), m.reg(bytecode.Value)
				init := m.emit(bytecode.OpIterInit, 0, it, m.arrayOf(m.k(7), m.k(8)), 0)
				head := m.pc()
				next := m.emit(bytecode.OpIterNext, 0, it, v, 0)
				m.emit(bytecode.OpIterKey, 0, k, it, 0)
				m.store("o", m.bin(bytecode.Concat, m.load("o"), m.bin(bytecode.Concat, k, v)))
				m.jump(head)
				m.code[next].C = m.pc()
				m.emit(bytecode.OpDrop, 0, it, 0, 0)
				m.code[init].C = m.pc()
				return m.load("o")
			}, end: Completed, ret: `s:"0718"`},
	})
	for _, tc := range []struct {
		name string
		v    any
		typ  string
	}{{"null", nil, "null"}, {"int", 5, "int"}, {"string", "str", "string"}, {"false", false, "false"}, {"true", true, "true"}, {"float", 1.5, "float"}} {
		t.Run("not iterable: "+tc.name, func(t *testing.T) {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 2, "v")
			m.at(3)
			m.foreachLoop(m.k(tc.v), "", "v", func() { m.echoK("never") })
			m.ret(m.isset("v"))
			cfg := vmConfig(t, 64)
			cfg.DisplayErrors = false
			res := a.program().Run(cfg)
			want := "Warning: foreach() argument must be of type array|object, " + tc.typ + " given"
			if got := vmDiagnostics(res); len(got) != 1 || got[0] != want || res.End != Completed || vmShow(res.Return) != "b:0" || len(res.Output) != 0 {
				t.Fatalf("%q %v %s %q", got, res.End, vmShow(res.Return), res.Output)
			}
			if res.Diagnostics[0].Line != 3 {
				t.Fatalf("line %d, want the IterInit's line 3", res.Diagnostics[0].Line)
			}
		})
	}
	t.Run("an iterator instruction on another value is refused, not a fault", func(t *testing.T) {
		for name, build := range map[string]func(m *vmFn){
			"IterNext": func(m *vmFn) { m.emit(bytecode.OpIterNext, 0, m.k(5), m.reg(bytecode.Value), m.pc()+1) },
			"IterKey before IterNext": func(m *vmFn) {
				m.emit(bytecode.OpIterKey, 0, m.reg(bytecode.Value), m.arrayOf(m.k(1)), 0)
			},
		} {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 2)
			build(m)
			m.retNull()
			res := runAsm(t, a)
			var u *UnsupportedError
			if res.End != Unsupported || !errors.As(res.Err, &u) {
				t.Fatalf("%s: %v %v", name, res.End, res.Err)
			}
		}
	})
}

// sumLoop is `foreach ($arr as $v) { echo "x"; }` over the global $arr.
func sumLoop(t testing.TB) *Program {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "arr", "v")
	m.foreachLoop(m.load("arr"), "", "v", func() { m.echoK("x") })
	m.retNull()
	return a.program()
}

func vmList(n int) []SnapshotEntry {
	out := make([]SnapshotEntry, n)
	for i := range out {
		out[i] = SnapshotEntry{Key: int64(i), Value: int64(i)}
	}
	return out
}

func TestVMForeachChargesStepsBeforeEffects(t *testing.T) {
	p := sumLoop(t)
	// Local, Local, Load, IterInit, then IterNext, Const, Output, Jump per
	// element: the n-th echo is step 4 + 4n - 1.
	for _, steps := range []int64{1, 4, 5, 6, 7, 8, 11, 12, 100, 1023, 1024, 1025} {
		cfg := vmConfig(t, 64)
		cfg.Globals = []Global{{Name: "arr", Value: vmList(1000)}}
		cfg.Steps = steps
		res := p.Run(cfg)
		if res.End != LimitExceeded || !errors.Is(res.Err, ErrStepLimit) || res.Steps != steps {
			t.Fatalf("steps %d: %v %v after %d", steps, res.End, res.Err, res.Steps)
		}
		want := 0
		if steps >= 7 {
			want = int((steps - 3) / 4)
		}
		if len(res.Output) != want {
			t.Fatalf("steps %d: %d echoes, want %d", steps, len(res.Output), want)
		}
	}
}

func TestVMForeachPollsCancellationAtTheBackEdge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := vmConfig(t, 64)
	cfg.Context = ctx
	cfg.Globals = []Global{{Name: "arr", Value: vmList(100000)}}
	cfg.Output = &cancelWriter{cancel: cancel, n: 3}
	res := sumLoop(t).Run(cfg)
	if res.End != Cancelled || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("%v %v", res.End, res.Err)
	}
	// The third echo is step 15; its write or the Jump right after it
	// observes the cancellation.
	if res.Steps > 16 {
		t.Fatalf("cancellation observed after %d steps", res.Steps)
	}
}

// A foreach retains the array; it must not copy it, however large it is.
func TestVMForeachDoesNotCopyTheArray(t *testing.T) {
	const n = 50000
	loop := sumLoopSilent(t, true)
	plain := sumLoopSilent(t, false)
	run := func(p *Program) float64 {
		return testing.AllocsPerRun(3, func() {
			cfg := vmConfig(t, 64)
			cfg.Globals = []Global{{Name: "arr", Value: vmList(n)}}
			if res := p.Run(cfg); res.End != Completed {
				t.Fatalf("%v %v", res.End, res.Err)
			}
		})
	}
	with, without := run(loop), run(plain)
	if with-without > 64 {
		t.Fatalf("iterating %d elements made %.0f more allocations than loading the array (%.0f against %.0f)", n, with-without, with, without)
	}
}

// sumLoopSilent is `$n = 0; foreach ($arr as $v) { $n++; } return $n;`, or
// only the load of $arr without the loop.
func sumLoopSilent(t testing.TB, loop bool) *Program {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "arr", "v", "n")
	m.store("n", m.k(0))
	subject := m.load("arr")
	if loop {
		m.foreachLoop(subject, "", "v", func() { m.incdec(bytecode.PreInc, "n") })
	}
	m.ret(m.load("n"))
	return a.program()
}

// One IterNext over a long run of deleted elements still observes a stop.
func TestVMIterNextObservesAStopWhileSkippingHoles(t *testing.T) {
	a := &phpArray{buckets: make([]*bucket, 4*vmHoleCheck)}
	a.buckets[len(a.buckets)-1] = &bucket{key: intKey(1), val: nullValue()}
	a.live = 1
	polls := 0
	pos, b, stopped := nextLive(a, 0, func() bool { polls++; return false })
	if stopped || b == nil || pos != len(a.buckets) || polls != 3 {
		t.Fatalf("pos %d, found %v, stopped %v after %d polls", pos, b != nil, stopped, polls)
	}
	polls = 0
	pos, b, stopped = nextLive(a, 0, func() bool { polls++; return polls == 2 })
	if !stopped || b != nil || pos > 2*vmHoleCheck+1 {
		t.Fatalf("pos %d, found %v, stopped %v", pos, b != nil, stopped)
	}
	for _, start := range []int{-5, len(a.buckets), len(a.buckets) + 9} {
		if _, b, stopped := nextLive(a, start, func() bool { return false }); b != nil || stopped {
			t.Fatalf("position %d found an element", start)
		}
	}
}

func TestVMParamDefault(t *testing.T) {
	// function f($a, $b = "B", $c = [1]) { return [$a, $b, $c]; }
	build := func(t testing.TB) (*vmAsm, *vmFn) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		f := &vmFn{a: a, name: "f", params: 3, locs: map[string]uint32{}, index: map[string]uint32{}, line: 10}
		f.regs = []bytecode.RegisterKind{bytecode.Value, bytecode.Value, bytecode.Value}
		for i, l := range []string{"a", "b", "c"} {
			r := f.reg(bytecode.Location)
			f.names = append(f.names, a.str(l))
			f.locs[l], f.index[l] = r, uint32(i)
			f.emit(bytecode.OpLocal, 0, r, uint32(i), 0)
		}
		f.at(11).emit(bytecode.OpStore, 0, f.loc("a"), 0, 0)
		pd := f.at(12).emit(bytecode.OpParamDefault, 0, 1, 0, 0)
		f.emit(bytecode.OpConst, 0, 1, a.str("B"), 0)
		f.code[pd].B = f.pc()
		f.emit(bytecode.OpStore, 0, f.loc("b"), 1, 0)
		pd = f.at(13).emit(bytecode.OpParamDefault, 0, 2, 0, 0)
		lit := f.arrayOf(f.k(1))
		f.emit(bytecode.OpMove, 0, 2, lit, 0)
		f.code[pd].B = f.pc()
		f.emit(bytecode.OpStore, 0, f.loc("c"), 2, 0)
		f.at(14).ret(f.arrayOf(f.load("a"), f.load("b"), f.load("c")))
		a.funcs = append(a.funcs, f)
		return a, m
	}
	for _, tc := range []struct {
		args []any
		ret  string
	}{
		{[]any{1}, `[0=>i:1,1=>s:"B",2=>[0=>i:1]]`},
		{[]any{1, nil}, `[0=>i:1,1=>N,2=>[0=>i:1]]`},
		{[]any{1, 2, 3}, `[0=>i:1,1=>i:2,2=>i:3]`},
		{[]any{1, 2, 3, 4}, `[0=>i:1,1=>i:2,2=>i:3]`},
	} {
		t.Run(fmt.Sprintf("%d arguments", len(tc.args)), func(t *testing.T) {
			a, m := build(t)
			var args []uint32
			for _, v := range tc.args {
				args = append(args, m.k(v))
			}
			m.ret(m.callName("f", args...))
			if res := runAsm(t, a); res.End != Completed || vmShow(res.Return) != tc.ret || len(res.Diagnostics) != 0 {
				t.Fatalf("%v %s %v %v", res.End, vmShow(res.Return), res.Diagnostics, res.Err)
			}
		})
	}
	t.Run("too few arguments", func(t *testing.T) {
		a, m := build(t)
		m.at(5).ret(m.callName("f"))
		cfg := vmConfig(t, 64)
		cfg.DisplayErrors = false
		res := a.program().Run(cfg)
		want := "Too few arguments to function f(), 0 passed in " + vmTestFile + " on line 5 and at least 1 expected"
		if res.End != UncaughtThrowable || res.Throwable.Class != "ArgumentCountError" || res.Throwable.Message != want {
			t.Fatalf("%v %+v %v", res.End, res.Throwable, res.Err)
		}
		// PHP raises it at the RECV of the first missing parameter: its line.
		if res.Throwable.Line != 11 {
			t.Fatalf("thrown at line %d, want the parameter's line 11", res.Throwable.Line)
		}
		if got := res.Diagnostics[len(res.Diagnostics)-1].Message; !strings.Contains(got, "#0 "+vmTestFile+"(5): f()\n#1 {main}") {
			t.Fatalf("trace %q", got)
		}
	})
	t.Run("the backtrace shows the passed arguments only", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.at(5).ret(m.callName("f", m.k(7)))
		f := &vmFn{a: a, name: "f", params: 2, locs: map[string]uint32{}, index: map[string]uint32{}, line: 10}
		f.regs = []bytecode.RegisterKind{bytecode.Value, bytecode.Value}
		for i, l := range []string{"a", "b"} {
			r := f.reg(bytecode.Location)
			f.names = append(f.names, a.str(l))
			f.locs[l], f.index[l] = r, uint32(i)
			f.emit(bytecode.OpLocal, 0, r, uint32(i), 0)
		}
		for i, l := range []string{"a", "b"} {
			pd := f.emit(bytecode.OpParamDefault, 0, uint32(i), 0, 0)
			f.emit(bytecode.OpConst, 0, uint32(i), a.constant(bytecode.Constant{Kind: bytecode.Int, Bits: 1}), 0)
			f.code[pd].B = f.pc()
			f.emit(bytecode.OpStore, 0, f.loc(l), uint32(i), 0)
		}
		// $a = "changed"; $b = 5; $b[...] = ...: an uncaught Error.
		f.store("a", f.k("changed"))
		f.store("b", f.k(5))
		f.emit(bytecode.OpArraySet, 0, f.loc("b"), 0, 1)
		f.retNull()
		a.funcs = append(a.funcs, f)
		cfg := vmConfig(t, 64)
		cfg.DisplayErrors = false
		res := a.program().Run(cfg)
		if res.End != UncaughtThrowable {
			t.Fatalf("%v %v", res.End, res.Err)
		}
		if got := res.Diagnostics[len(res.Diagnostics)-1].Message; !strings.Contains(got, "(5): f('changed')\n#1 {main}") {
			t.Fatalf("trace %q", got)
		}
	})
}

// While the default of a later parameter is evaluated, the backtrace shows
// the arguments that were passed, which the prologue has already stored.
// PHP 8.5.12 printed "#0 ...(6): f('arg')" for
// function f($a, $b = 1 % 0, $c = 3) {} f("arg"); (root: s2_def_throw_trace).
func TestVMBacktraceDuringADefault(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.at(6).ret(m.callName("f", m.k("arg")))
	f := &vmFn{a: a, name: "f", params: 2, locs: map[string]uint32{}, index: map[string]uint32{}, line: 2}
	f.regs = []bytecode.RegisterKind{bytecode.Value, bytecode.Value}
	for i, l := range []string{"a", "b"} {
		r := f.reg(bytecode.Location)
		f.names = append(f.names, a.str(l))
		f.locs[l], f.index[l] = r, uint32(i)
		f.emit(bytecode.OpLocal, 0, r, uint32(i), 0)
	}
	f.emit(bytecode.OpStore, 0, f.loc("a"), 0, 0)
	pd := f.emit(bytecode.OpParamDefault, 0, 1, 0, 0)
	f.emit(bytecode.OpMove, 0, 1, f.bin(bytecode.Modulo, f.k(1), f.k(0)), 0)
	f.code[pd].B = f.pc()
	f.emit(bytecode.OpStore, 0, f.loc("b"), 1, 0)
	f.retNull()
	a.funcs = append(a.funcs, f)
	cfg := vmConfig(t, 64)
	cfg.DisplayErrors = false
	res := a.program().Run(cfg)
	if res.End != UncaughtThrowable || res.Throwable.Class != "DivisionByZeroError" || res.Throwable.Line != 2 {
		t.Fatalf("%v %+v %v", res.End, res.Throwable, res.Err)
	}
	if got := res.Diagnostics[len(res.Diagnostics)-1].Message; !strings.Contains(got, "#0 "+vmTestFile+"(6): f('arg')\n#1 {main}") {
		t.Fatalf("trace %q", got)
	}
}

// The element window of a frame is cleared when the frame is popped: a later
// frame in the same window reads an undefined location, never the pointer.
func TestVMElementLocationsAreClearedOnPop(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.echo(m.call(1))
	m.echo(m.call(2))
	m.retNull()
	w := a.fn("w", 0, 3, "a")
	e := w.elem()
	w.fetch(e, w.loc("a"), bytecode.FetchWrite, "", w.k(1))
	w.setAt(e, w.k(2), w.k("deep"), false)
	w.ret(w.k("w"))
	r := a.fn("r", 0, 4, "a")
	e = r.elem()
	r.fetch(e, r.loc("a"), bytecode.FetchWrite, "", r.k(1))
	r.ret(r.loadAt(e))
	p := a.program()
	if res := p.Run(vmConfig(t, 64)); res.End != Completed || string(res.Output) != "w" {
		t.Fatalf("verified run: %v %q %v", res.End, res.Output, res.Err)
	}
	// Skip r's FetchDim, as a corrupted VM would: its element register must
	// be undefined, not the element w fetched.
	f := &p.funcs[2]
	for pc := range f.code {
		if f.code[pc].op == bytecode.OpFetchDim {
			f.code[pc] = vmInstr{op: bytecode.OpCheckpoint}
		}
	}
	res := p.Run(vmConfig(t, 64))
	var ie *InternalError
	if res.End != Internal || !errors.As(res.Err, &ie) || ie.Function != "r" || !strings.Contains(res.Err.Error(), "location register") {
		t.Fatalf("%v %v", res.End, res.Err)
	}
}

// One Program, many goroutines: iterator positions and element locations
// belong to each run's frames. Run it with -race.
func TestVMStage2SharedProgramIsolation(t *testing.T) {
	a := newAsm(t, 64)
	// $o = []; foreach ($in as $k => $v) { $o[$k][] = $v . $tag; } return $o;
	m := a.fn("{main}", 0, 2, "in", "tag", "o", "k", "v")
	m.store("o", m.arrayOf())
	e := m.elem()
	m.foreachLoop(m.load("in"), "k", "v", func() {
		// The value first, as the compiler orders it: nothing but the
		// write may follow the fetch.
		v := m.bin(bytecode.Concat, m.load("v"), m.load("tag"))
		m.fetch(e, m.loc("o"), bytecode.FetchWrite, "k", 0)
		m.emit(bytecode.OpArrayAppend, 0, e, v, 0)
	})
	m.ret(m.load("o"))
	p := a.program()
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				tag := "g" + strconv.Itoa(g) + "i" + strconv.Itoa(i)
				cfg := vmConfig(t, 64)
				cfg.Globals = []Global{{Name: "tag", Value: []byte(tag)}, {Name: "in", Value: []SnapshotEntry{{Key: "a", Value: int64(g)}, {Key: "b", Value: int64(i)}}}}
				res := p.Run(cfg)
				want := fmt.Sprintf(`["a"=>[0=>s:"%d%s"],"b"=>[0=>s:"%d%s"]]`, g, tag, i, tag)
				if res.End != Completed || vmShow(res.Return) != want {
					errs <- fmt.Errorf("goroutine %d run %d: %v %s %v", g, i, res.End, vmShow(res.Return), res.Err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
