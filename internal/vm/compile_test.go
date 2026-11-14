package vm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

const testFile = "/w/test.php"

// lowerSource parses and lowers src for PHP 8.5.12 at the given width.
func lowerSource(src string, width int, file string) (*ir.Unit, error) {
	prof, err := profile.Resolve("8.5.12", width, false)
	if err != nil {
		return nil, err
	}
	tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: width, MaxBytes: len(src) + 1})
	if err != nil {
		return nil, err
	}
	return ir.Lower(tree, prof, ir.Options{File: file})
}

func buildOptions() bytecode.Options {
	return bytecode.Options{Limits: bytecode.DefaultLimits(), Supported: 0}
}

// compileWidth lowers and compiles src; the image is not yet built.
func compileWidth(t testing.TB, src string, width int, file string) (bytecode.Image, error) {
	t.Helper()
	unit, err := lowerSource(src, width, file)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	return Compile(context.Background(), unit, []byte(src))
}

// compile compiles src for the 64-bit profile and requires Build to accept
// the image.
func compile(t testing.TB, src string) bytecode.Image {
	t.Helper()
	img, err := compileWidth(t, src, 64, testFile)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := bytecode.Build(context.Background(), img, buildOptions()); err != nil {
		t.Fatalf("Build rejected the image: %v\n%s", err, Disassemble(img))
	}
	return img
}

// ops lists the instructions of function fn whose opcode is op, with their pc.
func ops(img bytecode.Image, fn int, op bytecode.Opcode) []int {
	var pcs []int
	for pc, in := range img.Functions[fn].Code {
		if in.Op == op {
			pcs = append(pcs, pc)
		}
	}
	return pcs
}

// funcIndex finds a function by its declared name.
func funcIndex(t testing.TB, img bytecode.Image, name string) uint32 {
	t.Helper()
	for i, f := range img.Functions {
		if f.Name == name {
			return uint32(i)
		}
	}
	t.Fatalf("no function %q", name)
	return 0
}

// callOrder lists the callee names of the direct calls in fn, in code order.
func callOrder(img bytecode.Image, fn int) []string {
	var out []string
	for _, in := range img.Functions[fn].Code {
		if in.Op == bytecode.OpCall {
			out = append(out, img.Functions[in.B].Name)
		}
	}
	return out
}

func firstPC(t testing.TB, img bytecode.Image, fn int, match func(bytecode.Instruction) bool) int {
	t.Helper()
	for pc, in := range img.Functions[fn].Code {
		if match(in) {
			return pc
		}
	}
	t.Fatalf("no matching instruction in function %d:\n%s", fn, Disassemble(img))
	return -1
}

// localReg returns the Location register the prologue binds to local name.
func localReg(t testing.TB, img bytecode.Image, fn int, name string) uint32 {
	t.Helper()
	f := img.Functions[fn]
	for i, c := range f.LocalNames {
		if img.Constants[c].Bytes == name {
			for _, in := range f.Code {
				if in.Op == bytecode.OpLocal && in.B == uint32(i) {
					return in.A
				}
			}
		}
	}
	t.Fatalf("no local %q", name)
	return 0
}

func callTo(img bytecode.Image, name string) func(bytecode.Instruction) bool {
	return func(in bytecode.Instruction) bool {
		return in.Op == bytecode.OpCall && img.Functions[in.B].Name == name
	}
}

const fgh = "function f() { return 1; }\nfunction g() { return 2; }\nfunction h() { return 3; }\n"

func TestImageShape(t *testing.T) {
	src := "<?php\nfunction add($a, $b) {\n    $c = $a + $b;\n    return $c;\n}\n$x = add(1, 2);\necho $x;\n"
	img := compile(t, src)
	if img.Entry != 0 || len(img.Functions) != 2 || img.Functions[0].Name != "{main}" || img.Functions[1].Name != "add" {
		t.Fatalf("functions: %+v", img.Functions)
	}
	if img.Required != 0 || img.Identity.GuestPlatform != "php-8.5.12" || img.Identity.Target != "portable-go" {
		t.Fatalf("identity: %+v", img.Identity)
	}
	if len(img.Sources) != 1 || img.Sources[0].Name != testFile || img.Sources[0].Bytes != uint32(len(src)) || img.Sources[0].Lines != uint32(strings.Count(src, "\n")+1) {
		t.Fatalf("source: %+v", img.Sources)
	}
	add := img.Functions[1]
	if add.Params != 2 || add.Locals != 3 || add.ReturnKind != bytecode.Value || len(add.LocalNames) != 3 {
		t.Fatalf("add frame: params %d locals %d names %v", add.Params, add.Locals, add.LocalNames)
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := img.Constants[add.LocalNames[i]].Bytes; got != want {
			t.Fatalf("local %d named %q, want %q", i, got, want)
		}
	}
	// Prologue: one Local per local, then Store for each parameter.
	for i := 0; i < 3; i++ {
		if in := add.Code[i]; in.Op != bytecode.OpLocal || in.B != uint32(i) {
			t.Fatalf("prologue pc %d: %+v", i, in)
		}
	}
	for i := 0; i < 2; i++ {
		in := add.Code[3+i]
		if in.Op != bytecode.OpStore || in.B != uint32(i) || in.A != localReg(t, img, 1, []string{"a", "b"}[i]) {
			t.Fatalf("parameter store %d: %+v", i, in)
		}
		if add.Positions[3+i].Line != 2 {
			t.Fatalf("prologue line %d, want the declaration line 2", add.Positions[3+i].Line)
		}
	}
	for fi, f := range img.Functions {
		for pc, p := range f.Positions {
			if p.Source != 1 || p.Line == 0 || p.Start != 0 || p.End != 0 {
				t.Fatalf("function %d pc %d position %+v", fi, pc, p)
			}
		}
		last := f.Code[len(f.Code)-1]
		if last.Op != bytecode.OpReturn {
			t.Fatalf("function %d does not end with Return", fi)
		}
	}
	main := img.Functions[0]
	if got := main.Positions[len(main.Code)-1].Line; got != img.Sources[0].Lines {
		t.Fatalf("main implicit return on line %d, want the last line %d", got, img.Sources[0].Lines)
	}
	if callOrder(img, 0)[0] != "add" {
		t.Fatalf("call order %v", callOrder(img, 0))
	}
}

func TestConstantsAreDeduplicated(t *testing.T) {
	img := compile(t, "<?php\necho 1, 1, 'a', 'a', 1.5, 1.5, -0.0, 0.0, true, true, null, null;\n$x = 1;\n")
	seen := map[bytecode.Constant]bool{}
	for _, c := range img.Constants {
		if seen[c] {
			t.Fatalf("duplicate constant %+v", c)
		}
		seen[c] = true
	}
}

func TestIntConstantsRespectThe32BitProfile(t *testing.T) {
	src := "<?php\necho 2147483647, -2147483647 - 1, 2147483648, 9223372036854775807;\n"
	img, err := compileWidth(t, src, 32, testFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bytecode.Build(context.Background(), img, buildOptions()); err != nil {
		t.Fatal(err)
	}
	for _, c := range img.Constants {
		if c.Kind == bytecode.Int && int64(int32(c.Bits)) != int64(c.Bits) {
			t.Fatalf("int constant %d outside the 32-bit range", int64(c.Bits))
		}
	}
}

func TestDeterministicEncoding(t *testing.T) {
	src := "<?php\nfunction f($a) { return $a . 'x'; }\n$a = [1, 'k' => [2, 3]];\nfor ($i = 0; $i < 3; $i++) { echo f($i), $a['k'][1] ?? 'n'; }\nswitch ($i) { case 1: echo 1; default: echo 2; }\n"
	var prev []byte
	for i := 0; i < 3; i++ {
		img := compile(t, src)
		p, err := bytecode.Build(context.Background(), img, buildOptions())
		if err != nil {
			t.Fatal(err)
		}
		b, err := p.Encode(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if prev != nil && !bytes.Equal(prev, b) {
			t.Fatal("two compilations encode differently")
		}
		prev = b
	}
}

func TestCompileHonoursCancellation(t *testing.T) {
	unit, err := lowerSource("<?php\necho 1;\n", 64, testFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compile(ctx, unit, []byte("<?php\necho 1;\n")); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestCompileRejectsNilUnit(t *testing.T) {
	if _, err := Compile(context.Background(), nil, nil); err == nil {
		t.Fatal("nil unit compiled")
	}
}

// --- evaluation order ---

func TestOrderBinaryOperandsLeftToRight(t *testing.T) {
	img := compile(t, "<?php\n"+fgh+"echo f() + g() * h();\n")
	if got := strings.Join(callOrder(img, 0), ","); got != "f,g,h" {
		t.Fatalf("call order %s", got)
	}
	mul := firstPC(t, img, 0, func(in bytecode.Instruction) bool {
		return in.Op == bytecode.OpBinary && in.Aux == uint16(bytecode.Multiply)
	})
	add := firstPC(t, img, 0, func(in bytecode.Instruction) bool {
		return in.Op == bytecode.OpBinary && in.Aux == uint16(bytecode.Add)
	})
	code := img.Functions[0].Code
	if mul > add || code[add].B != code[firstPC(t, img, 0, callTo(img, "f"))].A || code[add].C != code[mul].A {
		t.Fatalf("operands of + are wrong:\n%s", Disassemble(img))
	}
}

func TestOrderArrayWriteKeyThenValue(t *testing.T) {
	img := compile(t, "<?php\n"+fgh+"$a = [];\n$a[f()] = g();\n")
	code := img.Functions[0].Code
	cf, cg := firstPC(t, img, 0, callTo(img, "f")), firstPC(t, img, 0, callTo(img, "g"))
	set := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpArraySet })
	if !(cf < cg && cg < set) || code[set].A != localReg(t, img, 0, "a") || code[set].B != code[cf].A || code[set].C != code[cg].A {
		t.Fatalf("wrong $a[f()] = g():\n%s", Disassemble(img))
	}
}

func TestOrderCompoundAssignReadsVariableAfterValue(t *testing.T) {
	img := compile(t, "<?php\n"+fgh+"$x += f();\n")
	code := img.Functions[0].Code
	lx := localReg(t, img, 0, "x")
	cf := firstPC(t, img, 0, callTo(img, "f"))
	load := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpLoad && in.B == lx })
	add := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpBinary })
	store := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpStore && in.A == lx })
	if !(cf < load && load < add && add < store) || code[load].Aux != 0 || code[add].B != code[load].A || code[add].C != code[cf].A || code[store].B != code[add].A {
		t.Fatalf("wrong $x += f():\n%s", Disassemble(img))
	}
}

func TestOrderCallArgumentsPushedAfterAllAreEvaluated(t *testing.T) {
	img := compile(t, "<?php\nfunction f($a, $b) { return 1; }\n"+fgh[len("function f() { return 1; }\n"):]+"echo f(g(), h() ? 1 : 2);\n")
	code := img.Functions[0].Code
	pushes := ops(img, 0, bytecode.OpPush)
	branch := ops(img, 0, bytecode.OpBranch)
	cf := firstPC(t, img, 0, callTo(img, "f"))
	if got := strings.Join(callOrder(img, 0), ","); got != "g,h,f" {
		t.Fatalf("call order %s", got)
	}
	if len(pushes) != 2 || len(branch) != 1 || pushes[0] < branch[0] || pushes[1] != pushes[0]+1 || cf != pushes[1]+1 {
		t.Fatalf("pushes must form one run right before the call:\n%s", Disassemble(img))
	}
	if code[pushes[0]].A != code[firstPC(t, img, 0, callTo(img, "g"))].A {
		t.Fatalf("first argument is not g()'s result:\n%s", Disassemble(img))
	}
}

func TestOrderShortCircuit(t *testing.T) {
	img := compile(t, "<?php\nfunction a() { return 1; }\nfunction b() { return 2; }\nfunction c() { return 3; }\n$r = a() && b() || c();\n")
	if got := strings.Join(callOrder(img, 0), ","); got != "a,b,c" {
		t.Fatalf("call order %s", got)
	}
	code := img.Functions[0].Code
	ca, cb, cc := firstPC(t, img, 0, callTo(img, "a")), firstPC(t, img, 0, callTo(img, "b")), firstPC(t, img, 0, callTo(img, "c"))
	// a's result is tested before b runs, the && result before c runs, and
	// a false a() skips b().
	br := ops(img, 0, bytecode.OpBranch)
	if len(br) != 4 || !(ca < br[0] && br[0] < cb && cb < br[1] && br[1] < br[2] && br[2] < cc && cc < br[3]) {
		t.Fatalf("branches in wrong places:\n%s", Disassemble(img))
	}
	if code[br[0]].A != code[ca].A || code[br[0]].B != uint32(cb) || int(code[br[0]].C) <= br[1] || int(code[br[0]].C) >= br[2] {
		t.Fatalf("a() false must skip b():\n%s", Disassemble(img))
	}
	if code[br[2]].B <= uint32(cc) || code[br[2]].C != uint32(cc) {
		t.Fatalf("a true && result must skip c():\n%s", Disassemble(img))
	}
}

func TestOrderCoalesceReadsQuietlyBeforeFallback(t *testing.T) {
	img := compile(t, "<?php\n"+fgh+"echo $x ?? f();\n")
	lx := localReg(t, img, 0, "x")
	load := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpLoad && in.B == lx })
	br := ops(img, 0, bytecode.OpBranch)
	cf := firstPC(t, img, 0, callTo(img, "f"))
	if img.Functions[0].Code[load].Aux != 1 || len(br) != 1 || !(load < br[0] && br[0] < cf) {
		t.Fatalf("wrong $x ?? f():\n%s", Disassemble(img))
	}
}

func TestOrderNestedTernary(t *testing.T) {
	img := compile(t, "<?php\n"+fgh+"echo f() ? (g() ? 'a' : 'b') : h();\n")
	if got := strings.Join(callOrder(img, 0), ","); got != "f,g,h" {
		t.Fatalf("call order %s", got)
	}
	br := ops(img, 0, bytecode.OpBranch)
	code := img.Functions[0].Code
	if len(br) != 2 || code[br[0]].A != code[firstPC(t, img, 0, callTo(img, "f"))].A || code[br[1]].A != code[firstPC(t, img, 0, callTo(img, "g"))].A {
		t.Fatalf("wrong nesting:\n%s", Disassemble(img))
	}
	// h() is only on the false edge of the outer branch.
	if int(code[br[0]].C) > firstPC(t, img, 0, callTo(img, "h")) || int(code[br[0]].C) < br[1] {
		t.Fatalf("outer false edge must skip the inner ternary:\n%s", Disassemble(img))
	}
}

func TestOrderGreaterSwapsOperandsButNotEvaluation(t *testing.T) {
	for _, tc := range []struct {
		op  string
		aux bytecode.BinaryOp
	}{{">", bytecode.Less}, {">=", bytecode.LessOrEqual}} {
		img := compile(t, "<?php\n"+fgh+"echo f() "+tc.op+" g();\n")
		code := img.Functions[0].Code
		cf, cg := firstPC(t, img, 0, callTo(img, "f")), firstPC(t, img, 0, callTo(img, "g"))
		cmp := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpBinary })
		if !(cf < cg) || code[cmp].Aux != uint16(tc.aux) || code[cmp].B != code[cg].A || code[cmp].C != code[cf].A {
			t.Fatalf("%s:\n%s", tc.op, Disassemble(img))
		}
	}
}

// PHP reads a plain variable operand when the operator runs, so
// $k + $k++ + $k sees the incremented $k in the first addition (17 for 5).
func TestOrderVariableOperandIsReadWhenTheOperatorRuns(t *testing.T) {
	img := compile(t, "<?php\n$k = 5;\n$r = $k + $k++ + $k;\n")
	lk := localReg(t, img, 0, "k")
	inc := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpIncDec })
	load := firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpLoad && in.B == lk })
	if load < inc {
		t.Fatalf("$k must be read after $k++:\n%s", Disassemble(img))
	}
	// Arguments are sent as they are evaluated: f($i, $i++, $i) reads $i first.
	img = compile(t, "<?php\nfunction f($a, $b, $c) { return 1; }\n$i = 1;\nf($i, $i++, $i);\n")
	li := localReg(t, img, 0, "i")
	inc = firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpIncDec })
	load = firstPC(t, img, 0, func(in bytecode.Instruction) bool { return in.Op == bytecode.OpLoad && in.B == li })
	if load > inc {
		t.Fatalf("first argument must be read before $i++:\n%s", Disassemble(img))
	}
}

// --- functions ---

func TestFunctionCalls(t *testing.T) {
	src := "<?php\necho later(1);\nfunction later($n) { return $n <= 0 ? 0 : later($n - 1); }\n" +
		"function foo() { return 1; }\necho FOO();\necho later();\necho later(1, 2);\necho missing(1);\n"
	img := compile(t, src)
	later, foo := funcIndex(t, img, "later"), funcIndex(t, img, "foo")
	code := img.Functions[0].Code
	calls := ops(img, 0, bytecode.OpCall)
	if len(calls) != 2 || code[calls[0]].B != later || code[calls[0]].C != 1 || code[calls[1]].B != foo || code[calls[1]].C != 0 {
		t.Fatalf("direct calls:\n%s", Disassemble(img))
	}
	if rec := ops(img, int(later), bytecode.OpCall); len(rec) != 1 || img.Functions[later].Code[rec[0]].B != later {
		t.Fatalf("recursion is not a direct call:\n%s", Disassemble(img))
	}
	// Arity mismatches and unknown names go through CallValue with the name.
	var names []string
	var argc []uint32
	for _, pc := range ops(img, 0, bytecode.OpCallValue) {
		in := code[pc]
		names = append(names, constantFor(img, 0, pc, in.B))
		argc = append(argc, in.C)
	}
	// An unknown name throws before its arguments are evaluated: argc 0.
	if strings.Join(names, ",") != "later,later,missing" || argc[0] != 0 || argc[1] != 2 || argc[2] != 0 {
		t.Fatalf("CallValue names %v argc %v:\n%s", names, argc, Disassemble(img))
	}
}

func TestNamespacedFunctionResolution(t *testing.T) {
	src := "<?php\nnamespace A;\nfunction f() { return 1; }\necho f();\necho \\A\\f();\necho g();\n"
	img := compile(t, src)
	f := funcIndex(t, img, "A\\f")
	if calls := ops(img, 0, bytecode.OpCall); len(calls) != 2 || img.Functions[0].Code[calls[0]].B != f || img.Functions[0].Code[calls[1]].B != f {
		t.Fatalf("namespaced calls:\n%s", Disassemble(img))
	}
	cv := ops(img, 0, bytecode.OpCallValue)
	if len(cv) != 1 || constantFor(img, 0, cv[0], img.Functions[0].Code[cv[0]].B) != "A\\g" {
		t.Fatalf("unknown namespaced name:\n%s", Disassemble(img))
	}
	// The global fallback of an unqualified name resolves to a global function.
	img = compile(t, "<?php\nnamespace { function g() { return 1; } }\nnamespace B { echo g(); }\n")
	if calls := ops(img, 0, bytecode.OpCall); len(calls) != 1 || img.Functions[img.Functions[0].Code[calls[0]].B].Name != "g" {
		t.Fatalf("fallback:\n%s", Disassemble(img))
	}
}

// constantFor finds the string constant last loaded into register reg
// before pc.
func constantFor(img bytecode.Image, fn, pc int, reg uint32) string {
	code := img.Functions[fn].Code
	for i := pc - 1; i >= 0; i-- {
		if code[i].Op == bytecode.OpConst && code[i].A == reg {
			return img.Constants[code[i].B].Bytes
		}
	}
	return "?"
}

func TestPrintInsideExpressionIsSupported(t *testing.T) {
	img := compile(t, "<?php\n$r = 1 + print 'x';\necho 'a' . print 'b';\n")
	if len(ops(img, 0, bytecode.OpOutput)) != 3 {
		t.Fatalf("print:\n%s", Disassemble(img))
	}
}

// --- unsupported constructs ---

func TestUnsupportedConstructs(t *testing.T) {
	cases := []struct {
		feature, src string
		line         int
	}{
		{"typed parameter", "echo 1;\nfunction f(int $a) {}", 3},
		{"variadic parameter", "echo 1;\nfunction f(...$a) {}", 3},
		{"return type", "echo 1;\nfunction f(): int { return 1; }", 3},
		{"return by reference", "echo 1;\nfunction &f() { return $x; }", 3},
		{"function attribute", "echo 1;\n#[Foo]\nfunction f() {}", 3},
		{"parameter attribute", "echo 1;\nfunction f(#[\\SensitiveParameter] $a) {}", 3},
		{"closure", "echo 1;\n$f = function () {};", 3},
		{"arrow function", "echo 1;\n$f = fn() => 1;", 3},
		{"class declaration", "echo 1;\nclass A {}", 3},
		{"new", "echo 1;\n$o = new A;", 3},
		{"try", "echo 1;\ntry { echo 2; } finally { echo 3; }", 3},
		{"throw", "echo 1;\nthrow $e;", 3},
		{"static variable", "echo 1;\nfunction f() { static $x = 0; }", 3},
		{"global statement", "echo 1;\nfunction f() { global $x; }", 3},
		{"builtin function define()", "echo 1;\ndefine('A', 1);", 3},
		{"builtin function constant()", "echo 1;\necho constant('A');", 3},
		{"the constant STDIN (a resource)", "echo 1;\necho STDIN;", 3},
		{"the constant STDIN (a resource)", "namespace A;\necho 1;\necho STDIN;", 4},
		{"the platform-dependent constant CHAR_MAX", "namespace A;\necho 1;\nfunction f($d = CHAR_MAX) {}", 4},
		{"the constant STDERR (a resource)", "namespace A;\necho 1;\nconst Y = STDERR;", 4},
		{"constant attribute", "echo 1;\n#[\\Deprecated]\nconst X = 1;", 3},
		{"the platform-dependent constant CHAR_MAX", "echo 1;\necho CHAR_MAX;", 3},
		{"the constant STDERR (a resource)", "echo 1;\nconst A = STDERR;", 3},
		{"include", "echo 1;\ninclude 'x.php';", 3},
		{"require_once", "echo 1;\nrequire_once 'x.php';", 3},
		{"eval", "echo 1;\neval('echo 1;');", 3},
		{"property access", "echo 1;\necho match ($o->p) { 1 => 2, default => 3 };", 3},
		{"class constant", "echo 1;\necho match ($x) { A::B => 2, default => 3 };", 3},
		{"property access", "echo 1;\n[$a, $o->p] = $c;", 3},
		{"static property access", "echo 1;\n['k' => A::$s] = $c;", 3},
		{"property access", "echo 1;\n[&$o->p] = $c;", 3},
		{"variable variables", "echo 1;\n$$n = 1;", 3},
		{"named arguments", "echo 1;\nfunction f($a) {}\nf(a: 1);", 4},
		{"argument unpacking", "echo 1;\nfunction f($a) {}\nf(...$x);", 4},
		{"array spread", "echo 1;\n$a = [...$x];", 3},
		{"dynamic call", "echo 1;\n$f = 'g';\n$f();", 4},
		{"property access", "echo 1;\necho \"{$o->p}\";", 3},
		{"method call", "echo 1;\n$o->m();", 3},
		{"static method call", "echo 1;\nA::m();", 3},
		{"class constant", "echo 1;\necho A::B;", 3},
		{"static property access", "echo 1;\necho A::$b;", 3},
		{"goto", "echo 1;\ngoto a;\na:\necho 2;", 3},
		{"declare", "declare(ticks=1);\necho 1;", 2},
		{"yield", "echo 1;\nfunction g() { yield 1; }", 3},
		{"clone", "echo 1;\n$b = clone $a;", 3},
		{"instanceof", "echo 1;\n$b = $a instanceof A;", 3},
		{"shell execution", "echo 1;\n$b = `ls`;", 3},
		{"conditional function declaration", "echo 1;\nif ($c) { function f() {} }", 3},
		{"superglobal", "echo 1;\necho $_GET;", 3},
		{"$this", "echo 1;\necho $this;", 3},
		{"property access", "echo 1;\necho @$o->p;", 3},
		{"property access", "echo 1;\necho empty($o->p);", 3},
		{"static property access", "echo 1;\necho empty(A::$p);", 3},
		{"reference to the result of a call", "echo 1;\nfunction f() {}\n$a = &f();", 4},
		{"passing a value that is not a variable by reference", "echo 1;\nfunction f(&$a) {}\nf(1);", 4},
		{"value of an assignment by reference between two elements", "echo 1;\n$x = ($a[0] = &$b[0]);", 3},
		{"property access", "echo 1;\n$a = &$o->p;", 3},
		{"by-reference array element", "echo 1;\n$a = [&$b];", 3},
		{"first-class callable", "echo 1;\nfunction f() {}\n$g = f(...);", 4},
		{"object cast", "echo 1;\n$o = (object)$a;", 3},
		{"__halt_compiler", "echo 1;\n__halt_compiler();", 3},
		{"array write through a non-variable", "echo 1;\nfunction f() {}\nf()[0] = 1;", 4},
		{"pipe operator", "echo 1;\nfunction f($a) {}\necho 1 |> f(...);", 4},
		{"builtin function strlen()", "echo 1;\necho strlen('x');", 3},
		{"builtin function count()", "echo 1;\necho count([1]);", 3},
		{"builtin function var_dump()", "echo 1;\nvar_dump(1);", 3},
		{"builtin function printf()", "echo 1;\nprintf('x');", 3},
		{"builtin function STRLEN()", "echo 1;\necho STRLEN('x');", 3},
		{"builtin function strlen()", "namespace A;\necho 1;\necho strlen('x');", 4},
		{"builtin function strlen()", "namespace A;\necho 1;\necho \\strlen('x');", 4},
		{"builtin function func_get_args()", "function f() { return func_get_args(); }", 2},
	}
	if len(cases) < 30 {
		t.Fatal("too few cases")
	}
	for _, tc := range cases {
		t.Run(tc.feature, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			img, err := compileWidth(t, src, 64, testFile)
			var ue *UnsupportedError
			if !errors.As(err, &ue) {
				t.Fatalf("got image %v, err %v; want UnsupportedError", len(img.Functions), err)
			}
			if ue.Feature != tc.feature || ue.File != testFile || ue.Line != tc.line {
				t.Fatalf("got %+v, want %q on line %d", ue, tc.feature, tc.line)
			}
			if !reflectZeroImage(img) {
				t.Fatal("an unsupported program returned a partial image")
			}
			if want := testFile + ":" + itoa(tc.line) + ": unsupported PHP construct: " + tc.feature; ue.Error() != want {
				t.Fatalf("Error() = %q, want %q", ue.Error(), want)
			}
		})
	}
}

// A hoisted declaration of a builtin's name is PHP's compile-time fatal
// error, reported before any unsupported construct. The pinned PHP 8.5.12
// image printed, for each source (php -n, display_errors=1):
//
//	"<?php\nfunction strlen() {}\n"            Fatal error: Cannot redeclare function strlen() in /w/r1.php on line 2
//	"<?php\necho \"a\";\nfunction\n STRLEN\n (\n) {}\n"  Fatal error: Cannot redeclare function STRLEN() in /w/r2.php on line 3
//	"<?php\nfunction var_dump($x) {}\n"        Fatal error: Cannot redeclare function var_dump() in /w/r5.php on line 2
//
// each followed by "Stack trace:\n#0 {main}" and exit code 255, with no
// output from the echo. Declared in a namespace, strlen is allowed: the
// namespaced function ran and printed 5.
func TestCompileErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src, msg string
		line           int
	}{
		{"lower case", "<?php\nfunction strlen() {}\n", "Cannot redeclare function strlen()", 2},
		{"declared spelling on the keyword line", "<?php\necho \"a\";\nfunction\n STRLEN\n (\n) {}\n", "Cannot redeclare function STRLEN()", 3},
		{"other builtin", "<?php\nfunction var_dump($x) {}\n", "Cannot redeclare function var_dump()", 2},
		{"ahead of an unsupported construct", "<?php\nforeach ($a as $b) {}\nfunction f() { foreach ($a as $b) {} }\nfunction Count() {}\n", "Cannot redeclare function Count()", 4},
		{"first declaration in order", "<?php\nfunction printf() {}\nfunction strlen() {}\n", "Cannot redeclare function printf()", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, err := compileWidth(t, tc.src, 64, testFile)
			var ce *CompileError
			if !errors.As(err, &ce) {
				t.Fatalf("got image %v, err %v; want CompileError", len(img.Functions), err)
			}
			if *ce != (CompileError{Message: tc.msg, File: testFile, Line: tc.line}) {
				t.Fatalf("got %+v, want %q on line %d", ce, tc.msg, tc.line)
			}
			if want := testFile + ":" + itoa(tc.line) + ": " + tc.msg; ce.Error() != want {
				t.Fatalf("Error() = %q, want %q", ce.Error(), want)
			}
			if !reflectZeroImage(img) {
				t.Fatal("a failed compilation returned a partial image")
			}
		})
	}
	img, err := compileWidth(t, "<?php\nnamespace N;\nfunction strlen() { return 5; }\necho strlen(), \"\\n\";\n", 64, testFile)
	if err != nil {
		t.Fatalf("namespaced strlen: %v", err)
	}
	if out, _ := runMini(t, img); out != "5\n" {
		t.Fatalf("namespaced strlen printed %q, want \"5\\n\"", out)
	}
}

func reflectZeroImage(img bytecode.Image) bool {
	return img.Functions == nil && img.Constants == nil && img.Sources == nil && img.Identity == (bytecode.Identity{})
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestUnsupportedReportsTheFirstInSourceOrder(t *testing.T) {
	cases := []struct {
		src     string
		feature string
		line    int
	}{
		// A function body before the main code that writes a property.
		{"<?php\nfunction f() { return $this; }\necho 1;\n$o->p = 1;\n", "$this", 2},
		// The main code before a later function.
		{"<?php\necho 1;\n$o->p = 1;\nfunction f() { return $this; }\n", "property access", 3},
		// Within a statement, the left operand first.
		{"<?php\necho $this . $o->p;\n", "$this", 2},
	}
	for _, tc := range cases {
		_, err := compileWidth(t, tc.src, 64, testFile)
		var ue *UnsupportedError
		if !errors.As(err, &ue) || ue.Feature != tc.feature || ue.Line != tc.line {
			t.Fatalf("%q: got %v, want %s on line %d", tc.src, err, tc.feature, tc.line)
		}
	}
}

// --- the W-17 reference fixtures ---

// Fixtures that need a construct outside the compiled subset. They are
// reported, not compiled by approximation. Stage 2 compiles the three that
// stage 1 listed here (element compound assignment and nested unset).
var outsideSubset = map[string]UnsupportedError{}

// Fixtures whose PHP result is a parse error, which the frontend reports.
var parseFatal = map[string]string{
	"fatal_const_conditional": `syntax error, unexpected token "const"`,
}

// Fixtures whose PHP result is a compile-time fatal error, which ir.Lower
// reports before the compiler runs.
var compileFatal = map[string]string{
	"fatal_redeclare_function":                 "Cannot redeclare function SAME() (previously declared in /w/cases/fatal_redeclare_function.php:3)",
	"fatal_param_default_redeclared_param":     "Redefinition of parameter $a",
	"fatal_list_spread":                        "Spread operator is not supported in assignments",
	"fatal_list_empty":                         "Cannot use empty list",
	"fatal_list_mixed_keys":                    "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_list_mix_syntax":                    "Cannot mix [] and list()",
	"fatal_list_ref_non_referenceable":         "Cannot assign reference to non referenceable value",
	"fatal_list_foreach_key_list":              "Cannot use list as key element",
	"fatal_list_lines_mixed_after_ref":         "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_list_lines_spread_after_ref":        "Spread operator is not supported in assignments",
	"fatal_list_lines_empty_after_ref_element": "Cannot use empty array entries in keyed array assignment",
	"fatal_list_lines_mixed_after_ref_element": "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_list_lines_mixed_after_element":     "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_list_lines_foreach_mixed_after_ref": "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_list_lines_mixed_after_var":         "Cannot mix keyed and unkeyed array entries in assignments",
	"fatal_const_redeclare_builtin":            "Cannot redeclare constant 'true'",
}

func TestReferenceFixturesCompile(t *testing.T) {
	files, err := filepath.Glob("../../tests/vm/w17/cases/*.php")
	if err != nil || len(files) != 329 {
		t.Fatalf("want 329 fixtures, got %d (%v)", len(files), err)
	}
	for _, width := range []int{64, 32} {
		for _, path := range files {
			name := strings.TrimSuffix(filepath.Base(path), ".php")
			t.Run(name+"/"+itoa(width), func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				file := "/w/cases/" + name + ".php"
				unit, err := lowerSource(string(src), width, file)
				if msg, ok := parseFatal[name]; ok {
					var se *frontend.SyntaxError
					if !errors.As(err, &se) || se.Message != msg || se.Compile {
						t.Fatalf("want parse error %q, got %v", msg, err)
					}
					return
				}
				if msg, ok := compileFatal[name]; ok {
					var ce *ir.CompileError
					if !errors.As(err, &ce) || ce.Message != msg {
						t.Fatalf("want compile fatal %q, got %v", msg, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("lower: %v", err)
				}
				img, err := Compile(context.Background(), unit, src)
				if want, ok := outsideSubset[name]; ok {
					var ue *UnsupportedError
					if !errors.As(err, &ue) || ue.Feature != want.Feature || ue.Line != want.Line || ue.File != file {
						t.Fatalf("want unsupported %+v, got %v", want, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Compile: %v", err)
				}
				if _, err := bytecode.Build(context.Background(), img, buildOptions()); err != nil {
					t.Fatalf("Build: %v\n%s", err, Disassemble(img))
				}
				if img.Identity.Profile.IntWidth != width {
					t.Fatalf("profile width %d", img.Identity.Profile.IntWidth)
				}
			})
		}
	}
}

func TestRegisterUseIsBoundedForLongChains(t *testing.T) {
	// A left-deep chain reuses its registers: 20 000 operands stay small.
	img := compile(t, "<?php\n$x = 1;\necho $x"+strings.Repeat(" . $x", 20000)+";\n")
	if n := len(img.Functions[0].Registers); n > 8 {
		t.Fatalf("left-deep chain uses %d registers", n)
	}
	// Nested calls keep each level's result and arguments live; past the
	// verifier's 4096 registers the program is reported, never emitted.
	src := "<?php\necho " + strings.Repeat("g(1, 1, ", 1500) + "1" + strings.Repeat(")", 1500) + ";\nfunction g($a, $b, $c) { return 1; }\n"
	_, err := compileWidth(t, src, 64, testFile)
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.Feature != "expression too large" || ue.Line != 2 {
		t.Fatalf("got %v, want expression too large on line 2", err)
	}
}

func TestBuiltinListIsSortedLowerCaseAndUnique(t *testing.T) {
	if len(builtinFunctions) < 1000 {
		t.Fatalf("only %d builtin names", len(builtinFunctions))
	}
	for i, n := range builtinFunctions {
		if n != strings.ToLower(n) || n == "" || (i > 0 && builtinFunctions[i-1] >= n) {
			t.Fatalf("entry %d %q breaks the sorted lower-case unique order", i, n)
		}
	}
	for _, n := range []string{"strlen", "count", "var_dump", "printf", "func_get_args"} {
		if !isBuiltin(n) || !isBuiltin(strings.ToUpper(n)) {
			t.Fatalf("%s is not recognised", n)
		}
	}
	for _, n := range []string{"foreach", "isset", "empty", "missing_function", "a\\strlen"} {
		if isBuiltin(n) {
			t.Fatalf("%s is reported as a builtin function", n)
		}
	}
}

// A unit function shadows a builtin of the same short name in its
// namespace: PHP resolves the namespaced name first.
func TestUnitFunctionShadowsBuiltinFallback(t *testing.T) {
	img := compile(t, "<?php\nnamespace A;\nfunction strlen($s) { return 1; }\necho strlen('x');\n")
	if calls := ops(img, 0, bytecode.OpCall); len(calls) != 1 || img.Functions[img.Functions[0].Code[calls[0]].B].Name != "A\\strlen" {
		t.Fatalf("namespaced function not called directly:\n%s", Disassemble(img))
	}
}

// PHP throws "Call to undefined function" at INIT_FCALL_BY_NAME, before any
// argument is evaluated. Oracle: "undefined_fn(print("x"))" printed no "x";
// in namespace ns, "f(...)" reported "ns\f()" and "\other\G(...)" "other\G()".
func TestUndefinedFunctionEvaluatesNoArguments(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{"<?php\nfunction f() { return 1; }\nundefined_fn(print('x'), f(), $u);\n", "undefined_fn"},
		{"<?php\nnamespace ns;\nf(print('y'));\n", "ns\\f"},
		{"<?php\nnamespace ns;\n\\other\\G(print('z'));\n", "other\\G"},
	} {
		img := compile(t, tc.src)
		code := img.Functions[0].Code
		cv := ops(img, 0, bytecode.OpCallValue)
		if len(cv) != 1 || code[cv[0]].C != 0 || constantFor(img, 0, cv[0], code[cv[0]].B) != tc.name {
			t.Fatalf("%q:\n%s", tc.src, Disassemble(img))
		}
		for _, op := range []bytecode.Opcode{bytecode.OpOutput, bytecode.OpCall, bytecode.OpPush, bytecode.OpLoad} {
			if len(ops(img, 0, op)) != 0 {
				t.Fatalf("%q evaluates an argument (%s):\n%s", tc.src, opNames[op], Disassemble(img))
			}
		}
		if p := img.Functions[0].MaxStack; p != 0 {
			t.Fatalf("MaxStack %d", p)
		}
	}
	// A unit function with the wrong arity still evaluates every argument.
	img := compile(t, "<?php\nfunction two($a, $b) { return 1; }\ntwo(print('a'));\ntwo(1, 2, print('b'));\n")
	if len(ops(img, 0, bytecode.OpOutput)) != 2 || len(ops(img, 0, bytecode.OpPush)) != 4 {
		t.Fatalf("arity mismatch must evaluate arguments:\n%s", Disassemble(img))
	}
}
