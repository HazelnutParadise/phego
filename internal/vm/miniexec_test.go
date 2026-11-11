package vm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// miniVM executes compiled images over a small value domain (null, bool,
// int, string and int/string-keyed arrays) so tests can check control flow
// and evaluation order by output. It is a test oracle for the compiler's
// instruction streams, not a PHP implementation: anything outside its
// domain fails the test instead of guessing.
type miniVM struct {
	img   bytecode.Image
	out   strings.Builder
	steps int
	exit  bool
}

type undefined struct{}

type miniArray struct {
	keys []any
	vals map[any]any
	next int64
}

func (a *miniArray) clone() *miniArray {
	c := &miniArray{keys: append([]any(nil), a.keys...), vals: make(map[any]any, len(a.vals)), next: a.next}
	for k, v := range a.vals {
		c.vals[k] = copyValue(v)
	}
	return c
}

func copyValue(v any) any {
	if a, ok := v.(*miniArray); ok {
		return a.clone()
	}
	return v
}

func (a *miniArray) set(k, v any) {
	if _, ok := a.vals[k]; !ok {
		a.keys = append(a.keys, k)
	}
	a.vals[k] = v
	if i, ok := k.(int64); ok && i >= a.next {
		a.next = i + 1
	}
}

func (a *miniArray) unset(k any) {
	if _, ok := a.vals[k]; !ok {
		return
	}
	delete(a.vals, k)
	for i, x := range a.keys {
		if x == k {
			a.keys = append(a.keys[:i], a.keys[i+1:]...)
			break
		}
	}
}

type miniFail string

func failf(format string, args ...any) { panic(miniFail(fmt.Sprintf(format, args...))) }

func key(v any) any {
	switch x := v.(type) {
	case int64:
		return x
	case string:
		if i, err := strconv.ParseInt(x, 10, 64); err == nil && strconv.FormatInt(i, 10) == x {
			return i
		}
		return x
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case nil:
		return ""
	}
	failf("array key %T outside the mini domain", v)
	return nil
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case string:
		return x != "" && x != "0"
	case *miniArray:
		return len(x.keys) > 0
	}
	failf("truthiness of %T", v)
	return false
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "1"
		}
		return ""
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return x
	}
	failf("string conversion of %T", v)
	return ""
}

func num(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case int64:
		return x
	case string:
		i, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			failf("numeric conversion of %q", x)
		}
		return i
	}
	failf("numeric conversion of %T", v)
	return 0
}

func looseEqual(a, b any) bool {
	switch {
	case isBool(a) || isBool(b) || a == nil || b == nil:
		return truthy(a) == truthy(b)
	}
	ai, aInt := a.(int64)
	bi, bInt := b.(int64)
	as, aStr := a.(string)
	bs, bStr := b.(string)
	switch {
	case aInt && bInt:
		return ai == bi
	case aStr && bStr:
		return as == bs
	case aInt && bStr:
		return strconv.FormatInt(ai, 10) == bs
	case aStr && bInt:
		return as == strconv.FormatInt(bi, 10)
	}
	failf("loose comparison of %T and %T", a, b)
	return false
}

func isBool(v any) bool { _, ok := v.(bool); return ok }

func (m *miniVM) binary(op bytecode.BinaryOp, a, b any) any {
	switch op {
	case bytecode.Add:
		return num(a) + num(b)
	case bytecode.Subtract:
		return num(a) - num(b)
	case bytecode.Multiply:
		return num(a) * num(b)
	case bytecode.Modulo:
		return num(a) % num(b)
	case bytecode.Concat:
		return str(a) + str(b)
	case bytecode.Equal:
		return looseEqual(a, b)
	case bytecode.NotEqual:
		return !looseEqual(a, b)
	case bytecode.Identical, bytecode.NotIdentical:
		same := fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b) && a == b
		if _, ok := a.(*miniArray); ok {
			failf("identity of arrays")
		}
		return same == (op == bytecode.Identical)
	case bytecode.Less:
		return num(a) < num(b)
	case bytecode.LessOrEqual:
		return num(a) <= num(b)
	case bytecode.BoolXor:
		return truthy(a) != truthy(b)
	}
	failf("binary op %d", op)
	return nil
}

type miniFrame struct {
	regs   []any
	locals []any
	stack  []any
}

func (m *miniVM) call(fi uint32, args []any) any {
	f := m.img.Functions[fi]
	fr := &miniFrame{regs: make([]any, len(f.Registers)), locals: make([]any, f.Locals)}
	for i := range fr.locals {
		fr.locals[i] = undefined{}
	}
	copy(fr.regs, args)
	pc := 0
	loc := func(r uint32) int { return fr.regs[r].(int) }
	for {
		m.steps++
		if m.steps > 1_000_000 {
			failf("step limit")
		}
		in := f.Code[pc]
		pc++
		switch in.Op {
		case bytecode.OpCheckpoint:
		case bytecode.OpConst:
			c := m.img.Constants[in.B]
			switch c.Kind {
			case bytecode.Null:
				fr.regs[in.A] = nil
			case bytecode.Bool:
				fr.regs[in.A] = c.Bits == 1
			case bytecode.Int:
				fr.regs[in.A] = int64(c.Bits)
			case bytecode.String:
				fr.regs[in.A] = c.Bytes
			default:
				failf("float constant")
			}
		case bytecode.OpMove:
			fr.regs[in.A] = copyValue(fr.regs[in.B])
		case bytecode.OpLocal:
			fr.regs[in.A] = int(in.B)
		case bytecode.OpLoad:
			v := fr.locals[loc(in.B)]
			if _, ok := v.(undefined); ok {
				if in.Aux == 0 {
					failf("undefined variable read")
				}
				v = nil
			}
			fr.regs[in.A] = copyValue(v)
		case bytecode.OpStore:
			fr.locals[loc(in.A)] = copyValue(fr.regs[in.B])
		case bytecode.OpBinary:
			fr.regs[in.A] = m.binary(bytecode.BinaryOp(in.Aux), fr.regs[in.B], fr.regs[in.C])
		case bytecode.OpUnary:
			switch bytecode.UnaryOp(in.Aux) {
			case bytecode.Not:
				fr.regs[in.A] = !truthy(fr.regs[in.B])
			case bytecode.Negate:
				fr.regs[in.A] = -num(fr.regs[in.B])
			default:
				failf("unary %d", in.Aux)
			}
		case bytecode.OpCast:
			switch bytecode.CastKind(in.Aux) {
			case bytecode.CastBool:
				fr.regs[in.A] = truthy(fr.regs[in.B])
			case bytecode.CastString:
				fr.regs[in.A] = str(fr.regs[in.B])
			case bytecode.CastInt:
				fr.regs[in.A] = num(fr.regs[in.B])
			default:
				failf("cast %d", in.Aux)
			}
		case bytecode.OpIncDec:
			l := loc(in.B)
			old := fr.locals[l]
			if _, ok := old.(undefined); ok {
				failf("increment of undefined")
			}
			var nv any
			switch bytecode.IncDecOp(in.Aux) {
			case bytecode.PreInc, bytecode.PostInc:
				if old == nil {
					nv = int64(1)
				} else {
					nv = old.(int64) + 1
				}
			default:
				if old == nil {
					nv = nil
				} else {
					nv = old.(int64) - 1
				}
			}
			fr.locals[l] = nv
			if in.Aux == uint16(bytecode.PreInc) || in.Aux == uint16(bytecode.PreDec) {
				fr.regs[in.A] = nv
			} else {
				fr.regs[in.A] = old
			}
		case bytecode.OpJump:
			pc = int(in.A)
		case bytecode.OpBranch:
			if truthy(fr.regs[in.A]) {
				pc = int(in.B)
			} else {
				pc = int(in.C)
			}
		case bytecode.OpPush:
			fr.stack = append(fr.stack, fr.regs[in.A])
		case bytecode.OpCall:
			n := len(fr.stack) - int(in.C)
			args := append([]any(nil), fr.stack[n:]...)
			fr.stack = fr.stack[:n]
			v := m.call(in.B, args)
			if m.exit {
				return nil
			}
			fr.regs[in.A] = v
		case bytecode.OpReturn:
			return fr.regs[in.A]
		case bytecode.OpOutput:
			m.out.WriteString(str(fr.regs[in.A]))
		case bytecode.OpExit:
			if in.Aux == 1 {
				if s, ok := fr.regs[in.A].(string); ok {
					m.out.WriteString(s)
				}
			}
			m.exit = true
			return nil
		case bytecode.OpNewArray:
			fr.regs[in.A] = &miniArray{vals: map[any]any{}}
		case bytecode.OpArrayGet:
			a, ok := fr.regs[in.B].(*miniArray)
			if !ok {
				failf("array read on %T", fr.regs[in.B])
			}
			v, ok := a.vals[key(fr.regs[in.C])]
			if !ok && in.Aux == 0 {
				failf("undefined key")
			}
			fr.regs[in.A] = copyValue(v)
		case bytecode.OpArraySet, bytecode.OpArrayAppend:
			l := loc(in.A)
			a, ok := fr.locals[l].(*miniArray)
			if !ok {
				if _, undef := fr.locals[l].(undefined); !undef && fr.locals[l] != nil {
					failf("array write on %T", fr.locals[l])
				}
				a = &miniArray{vals: map[any]any{}}
				fr.locals[l] = a
			}
			if in.Op == bytecode.OpArraySet {
				a.set(key(fr.regs[in.B]), copyValue(fr.regs[in.C]))
			} else {
				a.set(a.next, copyValue(fr.regs[in.B]))
			}
		case bytecode.OpArrayUnset:
			if a, ok := fr.locals[loc(in.A)].(*miniArray); ok {
				a.unset(key(fr.regs[in.B]))
			}
		case bytecode.OpArrayIsset:
			a, ok := fr.regs[in.B].(*miniArray)
			v, has := any(nil), false
			if ok {
				v, has = a.vals[key(fr.regs[in.C])]
			}
			fr.regs[in.A] = has && v != nil
		case bytecode.OpIsset:
			v := fr.locals[loc(in.B)]
			_, undef := v.(undefined)
			fr.regs[in.A] = !undef && v != nil
		case bytecode.OpUnset:
			fr.locals[loc(in.A)] = undefined{}
		default:
			failf("opcode %d outside the mini VM", in.Op)
		}
	}
}

// run executes the image and returns its output and whether it exited.
func runMini(t *testing.T, img bytecode.Image) (out string, exited bool) {
	t.Helper()
	m := &miniVM{img: img}
	defer func() {
		if r := recover(); r != nil {
			if f, ok := r.(miniFail); ok {
				t.Fatalf("mini VM: %s\n%s", f, Disassemble(img))
			}
			panic(r)
		}
	}()
	m.call(img.Entry, nil)
	return m.out.String(), m.exit
}

func TestRunControlFlow(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"nested break and continue", `for ($i = 0; $i < 3; $i++) {
    for ($j = 0; $j < 3; $j++) {
        if ($j == 1) { continue 2; }
        if ($i == 2) { break 2; }
        echo $i, $j, " ";
    }
}
echo "end";`, "00 10 end"},
		{"switch fall-through, default in the middle", `function sw($v) {
    switch ($v) {
        case 1: echo "one ";
        default: echo "def ";
        case 2: echo "two "; break;
        case 3: echo "three ";
    }
    echo "|";
}
sw(1); sw(2); sw(3); sw(9);`, "one def two |two |three |def two |"},
		{"continue inside switch inside a loop", `for ($i = 0; $i < 3; $i++) {
    switch ($i) { case 1: continue 2; default: echo "s", $i; }
    echo "a", $i;
}`, "s0a0s2a2"},
		{"continue targeting switch acts as break", `for ($i = 0; $i < 2; $i++) {
    switch ($i) { case 0: echo "z"; continue; case 1: echo "o"; }
    echo $i;
}`, "z0o1"},
		{"do-while runs once", `$k = 10; do { echo $k; $k++; } while ($k < 3); echo "|";
$k = 0; do { $k++; if ($k == 2) { continue; } echo $k; } while ($k < 4);`, "10|134"},
		{"for with empty parts and comma lists", `for ($a = 0, $b = 6; $a < $b; $a += 2, $b -= 1) { echo $a, $b, " "; }
for (;;) { echo "once"; break; }
$n = 0; for (; $n < 2;) { echo $n++; }
for ($i = 0, $j = 0; $i < 2, $j < 3; $i++, $j++) { echo $j; }`, "06 25 once01012"},
		{"while with break 1", `$i = 0; while (true) { $i++; if ($i > 3) { break 1; } echo $i; }`, "123"},
		{"if elseif else", `function c($n) { if ($n < 0) { return "neg"; } elseif ($n == 0) { return "zero"; } else if ($n < 5) { return "small"; } else { return "big"; } }
echo c(-1), c(0), c(3), c(9);`, "negzerosmallbig"},
		{"short-circuit and evaluation order", `function t($l, $v) { echo "[", $l, "]"; return $v; }
$r = t("a", 0) && t("b", 1); echo $r ? "T" : "F";
$r = t("a", 1) || t("b", 1); echo $r ? "T" : "F";
$r = t("a", 0) || t("b", 0) || t("c", 1); echo $r ? "T" : "F";
echo t("x", "") ?: t("y", "fb");
$n = null; echo $n ?? t("d", "def"); echo $undef ?? "u";
echo t("l", 1) + t("r", 2) * t("m", 3);`, "[a]F[a]T[a][b][c]T[x][y]fb[d]defu[l][r][m]7"},
		{"variables read when the operator runs", `$k = 5; echo $k + $k++ + $k, " ";
$i = 1; function three($a, $b, $c) { return $a . $b . $c; } echo three($i, $i++, $i), " ";
$j = 1; echo three($j, ++$j, $j), " ";
$arr = []; $n = 0; $arr[$n] = $n++; echo $arr[1], $n, " ";
$e = 1; $e += $e += 1; echo $e, " ";
$x = 1; echo $x . ($x = 2) . $x;`, "17 112 122 01 4 222"},
		{"greater-than keeps left-to-right evaluation", `function t($l, $v) { echo $l; return $v; }
echo t("a", 2) > t("b", 1) ? "T" : "F", t("c", 1) >= t("d", 1) ? "T" : "F";`, "abTcdT"},
		{"recursion and hoisting", `echo fact(5);
function fact($n) { return $n <= 1 ? 1 : $n * fact($n - 1); }
function FOO() { return "foo"; } echo foo();`, "120foo"},
		{"exit in an expression stops output", `function stop() { exit(9); } echo "a", "b", stop(), "c";`, "ab"},
		{"arrays", `$a = [1, "k" => [2, 3], 5 => "x", "y"]; echo $a[0], $a["k"][1], $a[5], $a[6];
$a[] = "z"; echo $a[7]; unset($a[0]); echo isset($a[0]) ? "s" : "u", isset($a["k"][0], $a[5]) ? "s" : "u";
$b = $a; $b["k"] = 0; echo $a["k"][0];`, "13xyzus2"},
		{"isset and unset of variables", `$a = 1; $b = null; echo isset($a) ? 1 : 0, isset($a, $b) ? 1 : 0, isset($c) ? 1 : 0;
unset($a, $b); echo isset($a) ? 1 : 0;`, "1000"},
		{"compound assignments and coalesce assignment", `$s = "a"; $s .= "b"; $s .= 1; $n = 2; $n *= 3; $n -= 1; $q ??= "set"; $q ??= "not"; echo $s, $n, $q;`, "ab15set"},
		{"interpolation", `$x = "v"; $i = 7; $a = ["k" => "K"]; echo "a $x b", "$i", "{$a['k']}$x";`, "a v b7Kv"},
		{"print returns 1", `$r = print "p"; echo $r, print "q";`, "p1q1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

// Fixtures whose behaviour stays inside the mini VM's domain run against
// the PHP 8.5.12 reference output.
func TestRunReferenceFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../tests/vm/w17/reference-64.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Case    string `json:"case"`
			Display struct {
				Stdout string `json:"stdout_b64"`
				Exit   int    `json:"exit_code"`
			} `json:"display"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"loops_basic": true, "loops_break_continue_levels": true, "short_circuit_order": true,
		"func_arg_eval_order": true, "exit_in_expression": true, "exit_in_function": true,
		"exit_plain": true, "close_tag_eats_newline": true, "inline_html_basic": true,
		"inline_html_trailing_newlines": true, "empty_file_no_php": true, "return_none": true,
	}
	ran := 0
	for _, c := range ref.Cases {
		if !want[c.Case] {
			continue
		}
		ran++
		t.Run(c.Case, func(t *testing.T) {
			src, err := os.ReadFile("../../tests/vm/w17/cases/" + c.Case + ".php")
			if err != nil {
				t.Fatal(err)
			}
			img, err := compileWidth(t, string(src), 64, "/w/cases/"+c.Case+".php")
			if err != nil {
				t.Fatal(err)
			}
			stdout, _ := base64.StdEncoding.DecodeString(c.Display.Stdout)
			if got, _ := runMini(t, img); got != string(stdout) {
				t.Fatalf("output %q\nwant   %q", got, stdout)
			}
		})
	}
	if ran != len(want) {
		t.Fatalf("ran %d of %d fixtures", ran, len(want))
	}
}
