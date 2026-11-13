package runtime

import (
	"errors"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 3 review fixes: REQ-025, REQ-026; T-049, T-050. What the VM
// does with references that hand-written bytecode, or a function too large
// for liveness, can leave in registers.

// The reference MakeRef put in a register moves to the argument stack, so
// the register stops counting as an owner even where liveness does not
// release it (a function above vmLivenessWords). Before the fix the
// register kept the reference: reading it again printed 5, and the extra
// owner made a later array copy share the element instead of copying it.
func TestPushMovesAByReferenceArgument(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x")
	withoutLiveness(m)
	g := a.fn("g", 1, 1, "p")
	g.retNull()
	m.store("x", m.k(5))
	r := m.reg(bytecode.Value)
	m.emit(bytecode.OpMakeRef, 0, r, m.loc("x"), 0)
	m.call(1, r)
	m.echoK("[")
	m.echo(r) // read after the Push: the register is empty
	m.echoK("]")
	m.echo(m.load("x"))
	m.retNull()
	res := runAsm(t, a)
	if res.Err != nil || string(res.Output) != "[]5" {
		t.Fatalf("output %q, err %v; want \"[]5\"", res.Output, res.Err)
	}
}

// withoutLiveness makes f too large for vmLiveness (more than
// vmLivenessWords bitset words), so that no register is released early and
// what an instruction leaves in a register stays observable, as in a very
// large PHP function.
func withoutLiveness(f *vmFn) {
	for i := 0; i < 1024; i++ {
		f.reg(bytecode.Value)
	}
	for i := 0; i < 1<<16; i++ {
		f.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	}
}

// A copy of an iterator register that outlives its loop names no
// iteration, even after the iteration's slot is reused by another loop:
// it fails closed instead of walking the other loop's array.
func TestCopiedRefIteratorFailsClosed(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "a", "b", "v")
	withoutLiveness(m)
	m.store("a", m.arrayOf(m.k("a1"), m.k("a2")))
	m.store("b", m.arrayOf(m.k("b1"), m.k("b2")))
	it := m.reg(bytecode.Value)
	i1 := m.emit(bytecode.OpIterInit, uint16(bytecode.IterByRef), it, m.loc("a"), 0)
	cp := m.reg(bytecode.Value)
	m.emit(bytecode.OpMove, 0, cp, it, 0)
	m.emit(bytecode.OpDrop, 0, it, 0, 0) // ends the iteration over $a
	it2 := m.reg(bytecode.Value)
	i2 := m.emit(bytecode.OpIterInit, uint16(bytecode.IterByRef), it2, m.loc("b"), 0)
	n1 := m.emit(bytecode.OpIterNext, uint16(bytecode.IterByRef|bytecode.IterLocation), cp, m.loc("v"), 0)
	m.echo(m.load("v"))
	exit := m.pc()
	m.echoK("end")
	m.retNull()
	m.code[i1].C, m.code[i2].C, m.code[n1].C = exit, exit, exit
	res := runAsm(t, a)
	var ue *UnsupportedError
	if !errors.As(res.Err, &ue) || string(res.Output) != "" {
		t.Fatalf("output %q, err %v; want no output and an UnsupportedError", res.Output, res.Err)
	}
}

// Overwriting an iterator register ends its iteration, so the array no
// longer tracks a position for it and the slot is reused.
func TestOverwrittenRefIteratorIsReleased(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "a", "v")
	withoutLiveness(m)
	m.store("a", m.arrayOf(m.k(1), m.k(2)))
	it := m.reg(bytecode.Value)
	one := a.constant(bytecode.Constant{Kind: bytecode.Int, Bits: 1})
	// Five times: IterInit (jumps past the group), IterNext (exits to the
	// Const), Const over the iterator register.
	for i := 0; i < 5; i++ {
		start := uint32(len(m.code))
		m.emit(bytecode.OpIterInit, uint16(bytecode.IterByRef), it, m.loc("a"), start+3)
		m.emit(bytecode.OpIterNext, uint16(bytecode.IterByRef|bytecode.IterLocation), it, m.loc("v"), start+2)
		m.emit(bytecode.OpConst, 0, it, one, 0)
	}
	m.retNull()
	var live, slots, cursors int
	res := a.program().run(vmConfig(t, 64), func(s *vmState) {
		s.loop()
		for _, ri := range s.refIters {
			if ri.live {
				live++
			}
		}
		slots = len(s.refIters)
		cursors = len(s.frames[0].locals[0].deref().array.cursors)
	})
	if res.Err != nil || live != 0 || slots != 1 || cursors != 0 {
		t.Fatalf("err %v: %d live iterations in %d slots, %d cursors; want 0 in 1 slot, 0 cursors", res.Err, live, slots, cursors)
	}
}

// CallValue reads its callable through a reference like every other
// instruction that reads a Value.
func TestCallValueDereferencesItsCallable(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x")
	g := a.fn("g", 0, 1)
	g.echoK("g")
	g.retNull()
	m.store("x", m.k("g"))
	r := m.reg(bytecode.Value)
	m.emit(bytecode.OpMakeRef, 0, r, m.loc("x"), 0)
	m.emit(bytecode.OpCallValue, 0, m.reg(bytecode.Value), r, 0)
	m.retNull()
	res := runAsm(t, a)
	if res.Err != nil || string(res.Output) != "g" {
		t.Fatalf("output %q, err %v; want \"g\"", res.Output, res.Err)
	}
}
