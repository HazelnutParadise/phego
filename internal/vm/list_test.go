package vm

import (
	"errors"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 4: REQ-025, REQ-026; T-009, T-010. The instruction streams of
// destructuring. The end-to-end versions, with the bytes PHP 8.5.12 printed,
// are tests/vm/w17/cases/{list,foreach_list,fatal_list}_*.php in the root
// package's TestW17PHP8512Programs; the shapes asserted here are those of
// zend_compile_assign (ZEND_AST_ARRAY), zend_compile_list_assign and
// zend_compile_foreach in PHP 8.5.12's zend_compile.c.

func TestStage4DestructuringShapes(t *testing.T) {
	const (
		W   = uint16(bytecode.FetchWrite)
		REF = uint16(bytecode.FetchForRef)
		VAL = uint16(bytecode.FetchContainerValue)
		KL  = uint16(bytecode.FetchKeyLocation)
	)
	for _, tc := range []struct {
		name, src, ops string
		fetch          []uint16
	}{
		// A variable right-hand side is copied first (QM_ASSIGN), then each
		// element is read with FETCH_LIST_R and assigned.
		{"variable source", "[$a, $b] = $x;", "Load Const ArrayGet Store Const ArrayGet Store", nil},
		// Any other right-hand side is evaluated once.
		{"literal source", "[$a, $b] = [1, 2];", "NewArray Const ArrayAdd Const ArrayAdd Const ArrayGet Store Const ArrayGet Store", nil},
		// A skipped position reads nothing.
		{"skipped position", "[, $b] = $x;", "Load Const ArrayGet Store", nil},
		// Keys are evaluated right before their element's fetch, between
		// the assignments of the earlier elements.
		{"keyed", "['k' => $a, $j => $b] = $x;", "Load Const ArrayGet Store Load ArrayGet Store", nil},
		// A nested pattern is read from the fetched element.
		{"nested", "[[$a], $b] = $x;", "Load Const ArrayGet Const ArrayGet Store Const ArrayGet Store", nil},
		// An element target has its keys evaluated after the fetch, then
		// the write (ASSIGN_DIM).
		{"element target", "[$a[1], $a[]] = $x;", "Load Const ArrayGet Const ArraySet Const ArrayGet ArrayAppend", nil},
		// With a by-reference element the right-hand side becomes a
		// reference (MAKE_REF); the bound element is fetched from it
		// (FETCH_LIST_W) and made a reference, the plain element is read
		// through it, and the reference is released at the end.
		{"by reference", "[&$a, $b] = $x;", "MakeRef Const FetchDim MakeRef Bind Const ArrayGet Store Drop", []uint16{W | REF | VAL}},
		// An element right-hand side is fetched by reference first.
		{"by reference from an element", "[&$a] = $x[0];", "Const FetchDim MakeRef Const FetchDim MakeRef Bind Drop", []uint16{W | REF, W | REF | VAL}},
		// A nested pattern with a reference is fetched for writing and
		// made a reference too; its VAR is freed after it.
		{"nested by reference", "[[&$a]] = $x;", "MakeRef Const FetchDim MakeRef Const FetchDim MakeRef Bind Drop Drop", []uint16{W | REF | VAL, W | REF | VAL}},
		// A plain variable key is read by the fetch itself.
		{"by reference variable key", "[$k => &$a] = $x;", "MakeRef FetchDim MakeRef Bind Drop", []uint16{W | REF | VAL | KL}},
		// An element target of a by-reference element: its keys, its
		// fetches by reference, then ASSIGN_REF.
		{"by reference element target", "[&$t[1]] = $x;", "MakeRef Const FetchDim MakeRef Const FetchDim Bind Drop", []uint16{W | REF | VAL, W | REF}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got := mainOps(t, img); got != tc.ops {
				t.Fatalf("ops = %q, want %q\n%s", got, tc.ops, Disassemble(img))
			}
			var fetch []uint16
			for _, in := range instructions(img, 0, bytecode.OpFetchDim) {
				fetch = append(fetch, in.Aux)
			}
			if len(fetch) != len(tc.fetch) {
				t.Fatalf("fetch aux = %v, want %v", fetch, tc.fetch)
			}
			for i := range fetch {
				if fetch[i] != tc.fetch[i] {
					t.Fatalf("fetch aux = %v, want %v", fetch, tc.fetch)
				}
			}
			for _, in := range instructions(img, 0, bytecode.OpArrayGet) {
				if in.Aux != bytecode.ArrayGetList {
					t.Fatalf("ArrayGet aux %d, want a list read\n%s", in.Aux, Disassemble(img))
				}
			}
		})
	}
}

// foreach with a pattern reads the element into a temporary that the pattern
// is destructured from; a pattern with a reference makes the loop iterate by
// reference and releases the element's reference after the assignments.
func TestStage4ForeachPatternShapes(t *testing.T) {
	for _, tc := range []struct{ name, src, ops string }{
		{"by value", "foreach ($r as [$a, $b]) {}", "Load IterInit IterNext Const ArrayGet Store Const ArrayGet Store Jump Drop"},
		{"with key", "foreach ($r as $k => ['id' => $id]) {}", "Load IterInit IterNext Const ArrayGet Store IterKey Jump Drop"},
		{"by reference", "foreach ($r as [&$a, $b]) {}", "IterInit IterNext Const FetchDim MakeRef Bind Const ArrayGet Store Drop Jump Drop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got := mainOps(t, img); got != tc.ops {
				t.Fatalf("ops = %q, want %q\n%s", got, tc.ops, Disassemble(img))
			}
		})
	}
}

// The lines PHP's compiler leaves behind: the first fetch carries the line
// after the right-hand side, a variable assignment moves to the variable's
// line, an element target or a by-reference target leaves the fetch's line,
// and a copied variable source is read on the pattern's first line.
func TestStage4DestructuringLines(t *testing.T) {
	src := "<?php\n[\n$a\n,\n$b[\n0\n]\n,\n$c\n]\n=\n[\n]\n;\n[\n$d\n]\n=\n$u\n;\n$x = [1];\n[\n&$e\n,\n$f\n]\n=\n$x\n;\n"
	img := compile(t, src)
	var gets, loads []uint32
	f := img.Functions[0]
	for i, in := range f.Code {
		switch in.Op {
		case bytecode.OpArrayGet:
			gets = append(gets, f.Positions[i].Line)
		case bytecode.OpLoad:
			loads = append(loads, f.Positions[i].Line)
		}
	}
	// $a: after the empty literal (its closing bracket, line 13); $b[0]:
	// after $a's assignment (line 3); $c: after the element write, which
	// leaves the fetch's line (3); $d: the pattern's line (16); $f: after
	// the by-reference binding, which leaves the fetch's line, where
	// MAKE_REF read $x (line 28).
	want := []uint32{13, 3, 3, 16, 28}
	if len(gets) != len(want) {
		t.Fatalf("fetch lines %v, want %v\n%s", gets, want, Disassemble(img))
	}
	for i := range want {
		if gets[i] != want[i] {
			t.Fatalf("fetch lines %v, want %v\n%s", gets, want, Disassemble(img))
		}
	}
	// QM_ASSIGN of $u on the pattern's first line.
	if len(loads) != 1 || loads[0] != 16 {
		t.Fatalf("load lines %v, want [16]\n%s", loads, Disassemble(img))
	}
}

// The value of a destructuring assignment by reference from an element and
// the object or static-property targets stay refused at Compile.
func TestStage4Unsupported(t *testing.T) {
	for _, tc := range []struct {
		feature, src string
		line         int
	}{
		{"value of a destructuring assignment by reference from an element", "echo 1;\n$y = ([&$a] = $x[0]);", 3},
		{"property access", "echo 1;\n[$o->p] = $x;", 3},
		{"property access", "echo 1;\n[&$o->p] = $x;", 3},
		{"static property access", "echo 1;\nforeach ($r as [A::$p]) {}", 3},
		{"reference to the result of a call", "echo 1;\nfunction f() { return [1]; }\n[&$a] = f();", 4},
	} {
		t.Run(tc.feature, func(t *testing.T) {
			_, err := compileWidth(t, "<?php\n"+tc.src+"\n", 64, testFile)
			var ue *UnsupportedError
			if !errors.As(err, &ue) || ue.Feature != tc.feature || ue.Line != tc.line {
				t.Fatalf("got %v, want %q on line %d", err, tc.feature, tc.line)
			}
		})
	}
	// The value of a by-reference destructuring from a variable is the
	// variable, read quietly after the binding.
	img := compile(t, "<?php\n$y = ([&$a] = $x);\n")
	if got := mainOps(t, img); got != "MakeRef Const FetchDim MakeRef Bind Drop Load Store" {
		t.Fatalf("ops = %q\n%s", got, Disassemble(img))
	}
}
