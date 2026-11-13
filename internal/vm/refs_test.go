package vm

import (
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 3: REQ-025, REQ-026; T-009, T-010. The instruction streams of
// references. The end-to-end versions, with the bytes PHP 8.5.12 printed,
// are tests/vm/w17/cases/{ref,foreach_ref,param_ref}_*.php in the root
// package's TestW17PHP8512Programs; the shapes asserted here are those of
// zend_compile_assign_ref, SEND_REF and FE_RESET_RW/FE_FETCH_RW in PHP
// 8.5.12's zend_compile.c.

func TestStage3ReferenceShapes(t *testing.T) {
	const (
		W   = uint16(bytecode.FetchWrite)
		REF = uint16(bytecode.FetchForRef)
		AP  = uint16(bytecode.FetchAppend)
		KL  = uint16(bytecode.FetchKeyLocation)
	)
	for _, tc := range []struct {
		name, src, ops string
		fetch          []uint16
	}{
		// Both sides are variables: ASSIGN_REF alone.
		{"variables", "$a = &$b;", "Bind", nil},
		// A variable target: no MAKE_REF, the source element is fetched
		// with ZEND_FETCH_DIM_REF on its last level.
		{"variable target", "$a = &$b[1][2];", "Const FetchDim Const FetchDim Bind", []uint16{W, W | REF}},
		// A variable source: the target is fetched last, also by reference.
		{"variable source", "$a[1] = &$b;", "Const FetchDim Bind", []uint16{W | REF}},
		{"appending target", "$a[] = &$b;", "FetchDim Bind", []uint16{W | REF | AP}},
		// Both sides elements: MAKE_REF between the source and the target.
		{"two elements", "$a[1] = &$b[2];", "Const FetchDim MakeRef Const FetchDim Bind", []uint16{W | REF, W | REF}},
		// The target's key expressions run before the source is fetched.
		{"target keys first", "$a[$i + 1] = &$b[$j];", "Const Load Binary FetchDim MakeRef FetchDim Bind", []uint16{W | REF | KL, W | REF}},
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
		})
	}
}

func TestStage3ByReferenceArgumentsAndParameters(t *testing.T) {
	img := compile(t, "<?php\nfunction f($a, &$b, &$c = 1) {}\nf($x, $y, $z[1]);\n")
	// SEND_REF makes each by-reference argument a reference at its
	// position, between the evaluations of the other arguments.
	if got, want := mainOps(t, img), "Load MakeRef Const FetchDim MakeRef Push Push Push Call"; got != want {
		t.Fatalf("ops = %q, want %q\n%s", got, want, Disassemble(img))
	}
	// The callee binds a by-reference parameter, with a default or not.
	binds := instructions(img, 1, bytecode.OpBind)
	stores := instructions(img, 1, bytecode.OpStore)
	if len(binds) != 2 || len(stores) != 1 || binds[0].B != 1 || binds[1].B != 2 || stores[0].B != 0 {
		t.Fatalf("receive: binds %+v, stores %+v\n%s", binds, stores, Disassemble(img))
	}
	for _, b := range binds {
		if b.Aux != uint16(bytecode.BindValue) {
			t.Fatalf("receive bind aux %d\n%s", b.Aux, Disassemble(img))
		}
	}
}

func TestStage3ForeachByReferenceShapes(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		init      uint16
		next      uint16
	}{
		{"variable subject and target", "foreach ($a as &$v) {}", uint16(bytecode.IterByRef), uint16(bytecode.IterByRef | bytecode.IterLocation)},
		{"temporary subject", "foreach ([1] as &$v) {}", uint16(bytecode.IterByRef), uint16(bytecode.IterByRef | bytecode.IterLocation)},
		{"element target", "foreach ($a as &$b[0]) {}", uint16(bytecode.IterByRef), uint16(bytecode.IterByRef)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			inits, nexts := instructions(img, 0, bytecode.OpIterInit), instructions(img, 0, bytecode.OpIterNext)
			if len(inits) != 1 || len(nexts) != 1 || inits[0].Aux != tc.init || nexts[0].Aux != tc.next {
				t.Fatalf("init %+v next %+v\n%s", inits, nexts, Disassemble(img))
			}
		})
	}
	// An element target is fetched after the element is, then bound to it.
	img := compile(t, "<?php\nforeach ($a as &$b[0]) {}\n")
	if got, want := mainOps(t, img), "IterInit IterNext Const FetchDim Bind Jump Drop"; got != want {
		t.Fatalf("ops = %q, want %q\n%s", got, want, Disassemble(img))
	}
}
