package vm

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 2: REQ-025, REQ-026; T-009, T-010. The instruction streams of
// element writes, foreach, parameter defaults and string offsets. The
// end-to-end versions, with the bytes PHP 8.5.12 printed, are the root
// package's TestW17Stage2Probes; the orders and lines asserted here are the
// ones those oracle runs showed.

// bodyOps names the instructions of function fn after its Locals.
func bodyOps(img bytecode.Image, fn int) []string {
	var out []string
	for _, in := range img.Functions[fn].Code {
		if in.Op != bytecode.OpLocal {
			out = append(out, name(opNames[:], int(in.Op)))
		}
	}
	return out
}

// mainOps names the instructions of {main} without its Locals and the
// implicit "return null" at its end.
func mainOps(t *testing.T, img bytecode.Image) string {
	t.Helper()
	ops := bodyOps(img, 0)
	if n := len(ops); n < 2 || ops[n-2] != "Const" || ops[n-1] != "Return" {
		t.Fatalf("no implicit return:\n%s", Disassemble(img))
	}
	return strings.Join(ops[:len(ops)-2], " ")
}

func instructions(img bytecode.Image, fn int, op bytecode.Opcode) []bytecode.Instruction {
	var out []bytecode.Instruction
	for _, in := range img.Functions[fn].Code {
		if in.Op == op {
			out = append(out, in)
		}
	}
	return out
}

// elementRegister is the function's one Location register beyond its locals.
func elementRegister(t *testing.T, img bytecode.Image, fn int) uint32 {
	t.Helper()
	f := img.Functions[fn]
	var extra []uint32
	for r, k := range f.Registers {
		if k == bytecode.Location && (uint32(r) < f.Params || uint32(r) >= f.Params+f.Locals) {
			extra = append(extra, uint32(r))
		}
	}
	if len(extra) != 1 || extra[0] != uint32(len(f.Registers)-1) {
		t.Fatalf("element registers %v of %d registers, want only the last\n%s", extra, len(f.Registers), Disassemble(img))
	}
	return extra[0]
}

func TestStage2ElementWriteShapes(t *testing.T) {
	const (
		W  = uint16(bytecode.FetchWrite)
		RW = uint16(bytecode.FetchReadWrite)
		U  = uint16(bytecode.FetchUnset)
		KL = uint16(bytecode.FetchKeyLocation)
		AP = uint16(bytecode.FetchAppend)
		ID = uint16(bytecode.FetchForIncDec)
		AO = uint16(bytecode.FetchForAssignOp)
	)
	for _, tc := range []struct {
		name, src, ops string
		fetch          []uint16 // the Aux of each FetchDim, in order
		locals         int
	}{
		{"nested assignment", "$a[1][2] = $v;", "Const FetchDim Const ArraySet", []uint16{W}, 2},
		{"three levels", "$a[1][$k][3] = 4;", "Const Const FetchDim FetchDim Const ArraySet", []uint16{W, W | KL}, 2},
		{"append at every level", "$a[][] = 1;", "Const FetchDim ArrayAppend", []uint16{W | AP}, 1},
		{"compound assignment", "$a[$k] += $v;", "FetchDim Load Load Binary Store", []uint16{RW | KL | AO}, 3},
		{"nested compound assignment", `$a[1]["x"] .= "s";`, "Const Const FetchDim Const FetchDim Load Binary Store", []uint16{RW, RW | AO}, 1},
		{"compound append", "$a[] -= 1;", "Const FetchDim Load Binary Store", []uint16{RW | AP | AO}, 1},
		{"increment", "$a[$i]++;", "FetchDim IncDec", []uint16{RW | KL | ID}, 2},
		{"nested decrement", "--$a[1][2];", "Const FetchDim Const FetchDim IncDec", []uint16{RW, RW | ID}, 1},
		{"increment of an appended element", "$a[]++;", "FetchDim IncDec", []uint16{RW | AP | ID}, 1},
		{"nested unset", "unset($a[1][$k]);", "Const FetchDim ArrayUnset", []uint16{U}, 2},
		{"unset three levels", `unset($a["x"]["y"]["z"]);`, "Const FetchDim Const FetchDim Const ArrayUnset", []uint16{U, U}, 1},
		{"single level stays one instruction", "$a[$k] = $v;", "ArraySet", nil, 3},
		{"single unset stays one instruction", "unset($a[$k]);", "ArrayUnset", nil, 2},
		{"coalesce assignment", "$a[$k] ??= 2;", "Load Load ArrayGet Const Binary Branch Move Jump Const ArraySet Move", nil, 2},
		{"nested coalesce assignment", "$a[1][2] ??= $v;", "Load Const ArrayGet Const ArrayGet Const Binary Branch Move Jump Const FetchDim Const ArraySet Move", []uint16{W}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got := mainOps(t, img); got != tc.ops {
				t.Fatalf("instructions %q, want %q\n%s", got, tc.ops, Disassemble(img))
			}
			f := img.Functions[0]
			// Element writes need no hidden local: only the PHP variables.
			if int(f.Locals) != tc.locals || len(f.LocalNames) != tc.locals {
				t.Fatalf("%d locals, want %d\n%s", f.Locals, tc.locals, Disassemble(img))
			}
			fetches := instructions(img, 0, bytecode.OpFetchDim)
			if len(fetches) != len(tc.fetch) {
				t.Fatalf("%d FetchDim, want %d\n%s", len(fetches), len(tc.fetch), Disassemble(img))
			}
			if len(fetches) == 0 {
				for _, k := range f.Registers[f.Params+f.Locals:] {
					if k == bytecode.Location {
						t.Fatalf("an element register without FetchDim\n%s", Disassemble(img))
					}
				}
				return
			}
			// Every level reuses the one element register: the first fetch
			// goes through the variable, each later one through the element.
			e := elementRegister(t, img, 0)
			for i, in := range fetches {
				if in.Aux != tc.fetch[i] || in.A != e || (i == 0) == (in.B == e) {
					t.Fatalf("FetchDim %d = %+v, want Aux %d into register %d\n%s", i, in, tc.fetch[i], e, Disassemble(img))
				}
			}
		})
	}
}

// The value of an element assignment comes from the write itself: its result
// register, since the result of a string offset write is the byte written.
func TestStage2ElementWriteResult(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		flags     bytecode.ArrayWriteFlags
		sameAsC   bool
	}{
		{"variable value", "echo $a[$k] = $v;", bytecode.KeyLocation | bytecode.ValueLocation, false},
		{"expression value", "echo $a[$k] = $v + 1;", bytecode.KeyLocation, true},
		{"nested", "$x = $a[1][2] = $v;", bytecode.ValueLocation, false},
		{"chained", `$x = $a["k"] = $b["j"] = 3;`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			sets := instructions(img, 0, bytecode.OpArraySet)
			if len(sets) == 0 {
				t.Fatalf("no ArraySet\n%s", Disassemble(img))
			}
			in := sets[0]
			r, ok := bytecode.ArraySetResult(in.Aux)
			if !ok || bytecode.ArrayWriteFlags(in.Aux&3) != tc.flags || (r == in.C) != tc.sameAsC || img.Functions[0].Registers[r] != bytecode.Value {
				t.Fatalf("ArraySet %+v: result %d (%v), want flags %d\n%s", in, r, ok, tc.flags, Disassemble(img))
			}
			// No quiet read-back of the value variable.
			for _, l := range instructions(img, 0, bytecode.OpLoad) {
				if l.Aux == 1 {
					t.Fatalf("a quiet Load reads the value back\n%s", Disassemble(img))
				}
			}
		})
	}
	t.Run("a discarded value names no result", func(t *testing.T) {
		img := compile(t, "<?php\n$a[$k] = $v;\n$a[1][2] = 3;\n")
		for _, in := range instructions(img, 0, bytecode.OpArraySet) {
			if _, ok := bytecode.ArraySetResult(in.Aux); ok {
				t.Fatalf("ArraySet %+v has a result\n%s", in, Disassemble(img))
			}
		}
	})
	t.Run("an append reads a variable value back", func(t *testing.T) {
		img := compile(t, "<?php\necho $a[1][] = $v;\n")
		if got := mainOps(t, img); got != "Const FetchDim ArrayAppend Load Output" {
			t.Fatalf("instructions %q\n%s", got, Disassemble(img))
		}
	})
}

// $a[k]...[kn] = $a reads $a before any level is fetched, as PHP copies it;
// the keys' expressions come before the value's, and the fetches after both.
func TestStage2ElementWriteOrder(t *testing.T) {
	img := compile(t, "<?php\n$a[1][2] = $a;\n")
	if got := mainOps(t, img); got != "Load Const FetchDim Const ArraySet" {
		t.Fatalf("self assignment: %q\n%s", got, Disassemble(img))
	}
	img = compile(t, "<?php\n"+fgh+"$a[f()][$k][g()] = h();\n")
	if got := strings.Join(callOrder(img, 0), " "); got != "f g h" {
		t.Fatalf("calls %q, want f g h", got)
	}
	last := firstPC(t, img, 0, callTo(img, "h"))
	for pc, in := range img.Functions[0].Code {
		if in.Op == bytecode.OpFetchDim && pc < last {
			t.Fatalf("a level is fetched before the value is evaluated\n%s", Disassemble(img))
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"keys, value, then the write", `function t($x) { echo "[$x]"; return $x; } $b[t(1)][t(2)] = t(3); echo $b[1][2];`, "[1][2][3]3"},
		{"a key variable is read when its level is fetched", `$k = 1; $c = []; $c[$k][$k = 2] = $k; echo isset($c[2][2]) ? "22" : "12";`, "22"},
		{"compound: keys, value, then the element", `function t($x) { echo "[$x]"; return $x; } $a = [1 => [2 => "o"]]; $a[t(1)][t(2)] .= t("v"); echo $a[1][2];`, "[1][2][v]ov"},
		{"compound reads its key once", `$i = 0; $b = [5, 6, 7]; $b[$i++] += 10; echo $b[0], $b[1], $i;`, "1561"},
		{"increment of an element", `$a = [5, 5]; $x = $a[0]++; $y = ++$a[1]; echo $x, $y, $a[0], $a[1];`, "5666"},
		{"coalesce assignment evaluates its key once", `function t($x) { echo "[$x]"; return $x; } $a = []; $a[t("p")] ??= t(7); $a[t("p")] ??= t(8); echo $a["p"];`, "[p][7][p]7"},
		{"coalesce assignment reads a key variable again for the write", `$k = "a"; $a = []; $a[$k] ??= ($k = "b"); echo isset($a["a"]) ? "a" : "-", isset($a["b"]) ? "b" : "-";`, "-b"},
		{"nested unset", `$a = ["n" => ["x" => 1, "y" => 2]]; unset($a["n"]["x"]); unset($a["no"]["q"]); echo isset($a["n"]["x"]) ? "x" : "-", isset($a["n"]["y"]) ? "y" : "-", isset($a["no"]) ? "made" : "none";`, "-ynone"},
		{"copies stay separate", `$a = [[1, 2]]; $b = $a; $a[0][0] = 9; echo $b[0][0], $a[0][0];`, "19"},
		{"the result of an element assignment", `$x = $a[1][2] = "v"; echo $x, $a[1][2];`, "vv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

func TestStage2ForeachShape(t *testing.T) {
	img := compile(t, "<?php\nforeach ($a as $k => $v) { echo $v; }\n")
	if got := mainOps(t, img); got != "Load IterInit IterNext IterKey Load Output Jump Drop" {
		t.Fatalf("instructions %q\n%s", got, Disassemble(img))
	}
	f := img.Functions[0]
	if f.Locals != 3 {
		t.Fatalf("%d locals, want $a, $k and $v only\n%s", f.Locals, Disassemble(img))
	}
	var init, next, key, jump, drop bytecode.Instruction
	var nextPC, dropPC int
	for pc, in := range f.Code {
		switch in.Op {
		case bytecode.OpIterInit:
			init = in
		case bytecode.OpIterNext:
			next, nextPC = in, pc
		case bytecode.OpIterKey:
			key = in
		case bytecode.OpJump:
			jump = in
		case bytecode.OpDrop:
			drop, dropPC = in, pc
		}
	}
	lv, _ := localIndexReg(img, 0, "v")
	lk, _ := localIndexReg(img, 0, "k")
	switch {
	case next.Aux != uint16(bytecode.IterLocation) || next.B != lv || key.Aux != uint16(bytecode.IterLocation) || key.A != lk:
		t.Fatalf("the loop variables are not written by the iterator instructions\n%s", Disassemble(img))
	case next.A != init.A || key.B != init.A || drop.A != init.A || f.Registers[init.A] != bytecode.Value:
		t.Fatalf("the instructions do not share one iterator register\n%s", Disassemble(img))
	case int(jump.A) != nextPC:
		t.Fatalf("the back edge does not return to IterNext\n%s", Disassemble(img))
	case int(next.C) != dropPC || int(init.C) != dropPC+1:
		t.Fatalf("exhaustion must reach the Drop and a non-iterable subject skip it\n%s", Disassemble(img))
	}
	for _, tc := range []struct{ name, src, ops string }{
		{"value only", "foreach ($a as $v) {}", "Load IterInit IterNext Jump Drop"},
		{"element targets", "foreach ($a as $k[0] => $v[]) {}", "Load IterInit IterNext ArrayAppend IterKey Const ArraySet Jump Drop"},
		{"literal subject", "foreach ([1] as $v) {}", "NewArray Const ArrayAdd IterInit IterNext Jump Drop"},
		{"break and continue", "foreach ($a as $v) { if ($v) break; continue; }", "Load IterInit IterNext Load Branch Jump Jump Jump Jump Drop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got := mainOps(t, img); got != tc.ops {
				t.Fatalf("instructions %q, want %q\n%s", got, tc.ops, Disassemble(img))
			}
		})
	}
	// Leaving an inner foreach for an outer loop drops the inner iterator:
	// "break 2" and "continue 2" each emit a Drop before their Jump.
	img = compile(t, "<?php\nforeach ($a as $x) { foreach ($b as $y) { if ($y) break 2; if ($x) continue 2; } }\n")
	drops := 0
	code := img.Functions[0].Code
	inner := instructions(img, 0, bytecode.OpIterInit)[1].A
	for pc, in := range code {
		// The Drop that is the taken branch of an if, followed by the Jump
		// out of the inner loop.
		if in.Op == bytecode.OpDrop && in.A == inner && code[pc-1].Op == bytecode.OpBranch && code[pc+1].Op == bytecode.OpJump {
			drops++
		}
	}
	if drops != 2 || len(instructions(img, 0, bytecode.OpDrop)) != 4 {
		t.Fatalf("%d Drop of the inner iterator on the way out, %d Drop in all; want 2 and 4\n%s", drops, len(instructions(img, 0, bytecode.OpDrop)), Disassemble(img))
	}
	for _, tc := range []struct{ name, src, want string }{
		{"values and keys", `foreach (["x" => 1, 5 => "five"] as $k => $v) { echo $k, "=", $v, ";"; }`, "x=1;5=five;"},
		{"the array as it was", `$a = [1, 2, 3]; foreach ($a as $v) { $a[] = $v; echo $v; } echo "|"; foreach ($a as $v) echo $v;`, "123|123123"},
		{"break and continue levels", `foreach ([1, 2, 3] as $i) { foreach (["a", "b", "c"] as $j) { if ($j == "b") continue 2; if ($i == 3) break 2; echo $i, $j, " "; } } echo "|", $i, $j;`, "1a 2a |3a"},
		{"continue 2 from a switch", `foreach ([1, 2, 3, 4] as $i) { switch ($i) { case 2: continue 2; case 4: break 2; } echo $i; }`, "13"},
		{"the key is assigned last", `foreach (["k1" => "v1"] as $x => $x) {} echo $x;`, "k1"},
		{"element targets", `foreach (["x" => 1, "y" => 2] as $ks[] => $out[]) {} echo $ks[0], $ks[1], $out[0], $out[1];`, "xy12"},
		{"in a function", `function sum($xs) { $t = 0; foreach ($xs as $x) { $t += $x; } return $t; } echo sum([1, 2, 3, 4]);`, "10"},
		{"return from inside", `function first($xs) { foreach ($xs as $k => $x) { if ($x == 3) return $k; } return "none"; } echo first([1, 3]), first([2]);`, "1none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

func TestStage2ParameterDefaults(t *testing.T) {
	img := compile(t, "<?php\nfunction f(\n  $a,\n  $b = 5,\n  $c = [1, 2]\n) { return $b; }\necho f(1), f(1, 2), f(1, 2, 3), f(1, 2, 3, 4);\n")
	fi := int(funcIndex(t, img, "f"))
	f := img.Functions[fi]
	if got := strings.Join(bodyOps(img, fi), " "); got != "Store ParamDefault Const Store ParamDefault NewArray Const ArrayAdd Const ArrayAdd Move Store Load Return Const Return" {
		t.Fatalf("prologue %q\n%s", got, Disassemble(img))
	}
	if f.Params != 3 || f.Locals != 3 {
		t.Fatalf("params %d, locals %d\n%s", f.Params, f.Locals, Disassemble(img))
	}
	// Each ParamDefault names its parameter register and jumps to the Store
	// that receives it; a literal default is loaded into that register.
	param := uint32(1)
	for pc, in := range f.Code {
		if in.Op != bytecode.OpParamDefault {
			continue
		}
		target := f.Code[in.B]
		if in.A != param || target.Op != bytecode.OpStore || target.B != param || int(in.B) <= pc {
			t.Fatalf("ParamDefault %+v at %d\n%s", in, pc, Disassemble(img))
		}
		if param == 1 {
			if c := f.Code[pc+1]; c.Op != bytecode.OpConst || c.A != 1 || int(in.B) != pc+2 {
				t.Fatalf("the literal default is not loaded into the parameter register\n%s", Disassemble(img))
			}
		}
		param++
	}
	if param != 3 {
		t.Fatalf("%d ParamDefault, want 2", param-1)
	}
	// The receiving instructions carry their parameter's line (3, 4, 5), the
	// default's code included, whatever lines the default spans.
	var lines []uint32
	for pc, in := range f.Code {
		if in.Op == bytecode.OpLoad {
			break
		}
		if in.Op != bytecode.OpLocal {
			lines = append(lines, f.Positions[pc].Line)
		}
	}
	want := []uint32{3, 4, 4, 4, 5, 5, 5, 5, 5, 5, 5, 5}
	if len(lines) != len(want) {
		t.Fatalf("prologue lines %v, want %v\n%s", lines, want, Disassemble(img))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("prologue lines %v, want %v\n%s", lines, want, Disassemble(img))
		}
	}
	// A call with as many arguments as parameters is direct; other counts go
	// through CallValue, which the run time binds.
	if n := len(instructions(img, 0, bytecode.OpCall)); n != 1 {
		t.Fatalf("%d direct calls, want 1\n%s", n, Disassemble(img))
	}
	if n := len(instructions(img, 0, bytecode.OpCallValue)); n != 3 {
		t.Fatalf("%d CallValue, want 3\n%s", n, Disassemble(img))
	}
	if got, _ := runMini(t, img); got != "5222" {
		t.Fatalf("output %q, want 5222", got)
	}
	multi := compile(t, "<?php\nfunction g(\n  $b\n   =\n   [1, 2][7],\n  $c = \"x\"\n   . \"y\"\n) {}\n")
	g := multi.Functions[funcIndex(t, multi, "g")]
	for pc, in := range g.Code {
		if in.Op == bytecode.OpLocal || in.Op == bytecode.OpReturn || (in.Op == bytecode.OpConst && pc >= len(g.Code)-2) {
			continue
		}
		line := g.Positions[pc].Line
		if wantLine := map[uint32]uint32{0: 3, 1: 6}[paramOf(g, pc)]; line != wantLine {
			t.Fatalf("pc %d (%s) is on line %d, want its parameter's line %d\n%s", pc, name(opNames[:], int(in.Op)), line, wantLine, Disassemble(multi))
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"defaults by position", `function h($a = 1, $b = 2) { return "$a$b"; } echo h(), h(9), h(9, 8), h(9, 8, 7);`, "12929898"},
		{"null does not take the default", `function h($a = "d") { return $a === null ? "null" : $a; } echo h(null), h();`, "nulld"},
		{"a default array is fresh on every call", `function h($a = [1]) { $a[] = 2; return isset($a[2]) ? "leak" : "ok"; } echo h(), h();`, "okok"},
		{"recursion", `function fact($n, $acc = 1) { return $n <= 1 ? $acc : fact($n - 1, $acc * $n); } echo fact(5);`, "120"},
		{"constant expressions", `function h($x = 1 + 2 * 3, $y = "a" . "b", $z = [1, 2][1]) { return $x . $y . $z; } echo h();`, "7ab2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

// paramOf is the parameter whose prologue code holds pc: the code up to and
// including the Store of parameter i belongs to it.
func paramOf(f bytecode.Function, pc int) uint32 {
	p := uint32(0)
	for i := 0; i < pc; i++ {
		if in := f.Code[i]; in.Op == bytecode.OpStore && in.B < f.Params {
			p++
		}
	}
	return p
}

func TestStage2StringOffsets(t *testing.T) {
	// A string literal base is read like any other value.
	img := compile(t, "<?php\necho \"abc\"[1], $s[0], $s[-1] ?? \"d\", isset($s[2]);\n")
	gets := instructions(img, 0, bytecode.OpArrayGet)
	if len(gets) != 3 || gets[0].Aux != 0 || gets[1].Aux != 0 || gets[2].Aux != 1 || len(instructions(img, 0, bytecode.OpArrayIsset)) != 1 {
		t.Fatalf("reads %+v\n%s", gets, Disassemble(img))
	}
	for _, tc := range []struct{ name, src, want string }{
		{"offsets", `$s = "hello"; echo $s[0], $s[4], $s[-1], "abc"[1], "abc"[-3];`, "hooba"},
		{"nested and interpolated", `$a = ["k" => "str"]; $s = "xyz"; echo $a["k"][1], "$s[0]{$s[2]}", $s[1][0];`, "txzy"},
		{"in a loop", `$s = "abc"; $o = ""; for ($i = 0; isset($s[$i]); $i++) { $o = $s[$i] . $o; } echo $o;`, "cba"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

// PHP prints each diagnostic on the line of the opline that raises it. The
// lines below are the ones the pinned PHP 8.5.12 image printed for these
// sources (root package: s2_cas_lines, s2_nested_lines, s2_incdec_dim_lines,
// s2_coalesce_dim_lines, s2_fe_lines).
func TestStage2Lines(t *testing.T) {
	lineOf := func(t *testing.T, img bytecode.Image, match func(bytecode.Instruction) bool) []uint32 {
		t.Helper()
		var lines []uint32
		for pc, in := range img.Functions[0].Code {
			if match(in) {
				lines = append(lines, img.Functions[0].Positions[pc].Line)
			}
		}
		if len(lines) == 0 {
			t.Fatalf("no instruction selected\n%s", Disassemble(img))
		}
		return lines
	}
	is := func(op bytecode.Opcode) func(bytecode.Instruction) bool {
		return func(in bytecode.Instruction) bool { return in.Op == op }
	}
	equal := func(t *testing.T, img bytecode.Image, what string, got []uint32, want ...uint32) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s on lines %v, want %v\n%s", what, got, want, Disassemble(img))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s on lines %v, want %v\n%s", what, got, want, Disassemble(img))
			}
		}
	}
	t.Run("compound assignment", func(t *testing.T) {
		// $u1's line for the fetch and for the read of $u2 on line 6.
		img := compile(t, "<?php\n$a = [];\n$a\n[$u1]\n+=\n$u2;\n")
		equal(t, img, "FetchDim", lineOf(t, img, is(bytecode.OpFetchDim)), 4)
		equal(t, img, "the read of $u2", selectedLines(t, img, sel{"{main}", readsVar("u2")}), 4)
		equal(t, img, "Binary", lineOf(t, img, is(bytecode.OpBinary)), 4)
	})
	t.Run("nested assignment", func(t *testing.T) {
		img := compile(t, "<?php\n$a = [];\n$a\n[$u1]\n[$u2]\n=\n$u3;\n")
		equal(t, img, "FetchDim", lineOf(t, img, is(bytecode.OpFetchDim)), 4)
		equal(t, img, "ArraySet", lineOf(t, img, is(bytecode.OpArraySet)), 5)
	})
	t.Run("nested unset", func(t *testing.T) {
		img := compile(t, "<?php\nunset(\n$a\n[$u4]\n[$u5]\n);\n")
		equal(t, img, "FetchDim", lineOf(t, img, is(bytecode.OpFetchDim)), 4)
		equal(t, img, "ArrayUnset", lineOf(t, img, is(bytecode.OpArrayUnset)), 5)
	})
	t.Run("increment and decrement", func(t *testing.T) {
		img := compile(t, "<?php\n$a\n[$u1]\n++;\n--\n$a\n[$u2];\n")
		equal(t, img, "FetchDim", lineOf(t, img, is(bytecode.OpFetchDim)), 3, 7)
		equal(t, img, "IncDec", lineOf(t, img, is(bytecode.OpIncDec)), 3, 7)
	})
	t.Run("coalesce assignment", func(t *testing.T) {
		// Both passes and the read of the value on line 6 carry the key's line.
		img := compile(t, "<?php\n$a\n [$u1]\n ??=\n\n $u2;\n")
		equal(t, img, "ArrayGet", lineOf(t, img, is(bytecode.OpArrayGet)), 3)
		equal(t, img, "ArraySet", lineOf(t, img, is(bytecode.OpArraySet)), 3)
		equal(t, img, "the reads of $u1", selectedLines(t, img, sel{"{main}", readsVar("u1")}), 3, 3)
		equal(t, img, "the read of $u2", selectedLines(t, img, sel{"{main}", readsVar("u2")}), 3)
	})
	t.Run("foreach", func(t *testing.T) {
		// FE_RESET_R on the subject's line; an element target on its own.
		img := compile(t, "<?php\nforeach (\n  $undef\n  as\n  $arr[$uk]\n) {\n}\n")
		equal(t, img, "IterInit", lineOf(t, img, is(bytecode.OpIterInit)), 3)
		equal(t, img, "the read of $undef", selectedLines(t, img, sel{"{main}", readsVar("undef")}), 3)
		equal(t, img, "ArraySet", lineOf(t, img, is(bytecode.OpArraySet)), 5)
	})
}

// A chain of element levels is compiled with a loop and one element
// register, whatever its depth: 20 000 levels need no Go stack and no more
// registers than one level.
func TestStage2DeepElementChainsAreIterative(t *testing.T) {
	const depth = 20000
	chain := strings.Repeat("[0]", depth)
	for _, src := range []string{"$a" + chain + " = 1;", "unset($a" + chain + ");", "$a" + chain + "++;", "$a" + chain + " .= 'x';"} {
		img := compile(t, "<?php\n"+src+"\n")
		f := img.Functions[0]
		if n := len(instructions(img, 0, bytecode.OpFetchDim)); n < depth-1 {
			t.Fatalf("%d FetchDim for %d levels", n, depth)
		}
		if len(f.Registers) > 8 {
			t.Fatalf("%d registers for a chain of %d levels", len(f.Registers), depth)
		}
	}
}

// What stage 2 still refuses, with the construct's own line.
func TestStage2Unsupported(t *testing.T) {
	for _, tc := range []struct {
		feature, src string
		line         int
	}{
		{"foreach by reference", "echo 1;\nforeach ($a as &$v) {}", 3},
		{"foreach by reference", "echo 1;\nforeach ($a as $k => &$v) {}", 3},
		{"list() destructuring", "echo 1;\nforeach ($a as [$x, $y]) {}", 3},
		{"list() destructuring", "echo 1;\nforeach ($a as $k => list($x)) {}", 3},
		{"property access", "echo 1;\nforeach ($a as $o->p) {}", 3},
		{"property access", "echo 1;\nforeach ($a as\n $o->p[1]) {}", 4},
		{"array write through a non-variable", "echo 1;\nfunction f() {}\nf()[0][1] = 1;", 4},
		{"array write through a non-variable", "echo 1;\nfunction f() {}\nf()[0] .= 1;", 4},
		{"array write through a non-variable", "echo 1;\nfunction f() {}\nf()[0]++;", 4},
		{"property access", "echo 1;\nunset($o->p[1][2]);", 3},
		{"property access", "echo 1;\n$o->p[1] ??= 2;", 3},
		{"static property access", "echo 1;\nA::$b[1][2] = 3;", 3},
		{"constant lookup", "echo 1;\nfunction f($a = 1,\n  $b = PHP_EOL) {}", 4},
		{"class constant", "echo 1;\nfunction f($a = self::X) {}", 3},
		{"typed parameter", "echo 1;\nfunction f(int $a = 1) {}", 3},
		{"by-reference parameter", "echo 1;\nfunction f(&$a = 1) {}", 3},
	} {
		t.Run(tc.feature, func(t *testing.T) {
			img, err := compileWidth(t, "<?php\n"+tc.src+"\n", 64, testFile)
			var ue *UnsupportedError
			if !errors.As(err, &ue) || ue.Feature != tc.feature || ue.File != testFile || ue.Line != tc.line {
				t.Fatalf("got %v, want %q on line %d", err, tc.feature, tc.line)
			}
			if !reflectZeroImage(img) {
				t.Fatal("an unsupported program returned a partial image")
			}
		})
	}
	// "$a[] ??= v" is PHP's compile-time fatal error, as the pinned image
	// printed for s2_coalesce_append and s2_coalesce_append_mid.
	for _, src := range []string{"echo \"a\\n\";\n$c = [];\n$c[] ??= 4;", "echo \"a\\n\";\n$c = [];\n$c[][1] ??= 4;"} {
		_, err := compileWidth(t, "<?php\n"+src+"\n", 64, testFile)
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Message != "Cannot use [] for reading" || ce.Line != 4 || ce.File != testFile {
			t.Fatalf("%q: got %v, want the compile error on line 4", src, err)
		}
	}
}
