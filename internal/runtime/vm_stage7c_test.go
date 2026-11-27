package runtime

import (
	"context"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026.

// stage7cProgram builds the program of a with handlers on {main}.
func stage7cProgram(t *testing.T, a *vmAsm, handlers []bytecode.Handler) *Program {
	t.Helper()
	image := a.image()
	image.Functions[0].Handlers = handlers
	bp, err := bytecode.Build(context.Background(), image, bytecode.Options{Limits: bytecode.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProgram(bp)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// In a function without liveness facts no register is released early. The
// exception a catch receives, the value a Return hands to a finally block
// and the exception a finally block suspends still have one holder each
// (the catch register until it is dropped, and the pending completion):
// the returned array is not changed by the finally block's write to the
// variable, and the run ends without a leaked or doubly released value.
// The verifier does not let the handler read a register the range wrote,
// so the registers themselves cannot be printed here.
func TestStage7CUnwindingLeavesNoHolderInRegisters(t *testing.T) {
	t.Run("catch", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2, "e")
		withoutLiveness(m)
		ex := m.reg(bytecode.Value)
		start := m.pc()
		m.bin(bytecode.Modulo, m.k(1), m.k(0)) // throws DivisionByZeroError
		leave := m.emit(bytecode.OpLeave, 0, 0, 0, 0)
		target := m.pc()
		m.store("e", ex)
		m.emit(bytecode.OpDrop, 0, ex, 0, 0)
		m.echoK("[]")
		leave2 := m.emit(bytecode.OpLeave, 0, 0, 0, 0)
		after := m.pc()
		m.unset("e") // the last holder: nothing else keeps the exception
		m.echoK("done")
		m.retNull()
		m.code[leave].A, m.code[leave2].A = after, after
		p := stage7cProgram(t, a, []bytecode.Handler{{Start: start, End: target, Target: target, BodyEnd: after, Register: ex, Kind: bytecode.Catch}})
		res := p.Run(vmConfig(t, 64))
		if res.End != Completed || res.Err != nil || string(res.Output) != "[]done" {
			t.Fatalf("%v %v %q", res.End, res.Err, res.Output)
		}
	})
	t.Run("return through finally", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2, "a")
		withoutLiveness(m)
		m.store("a", m.arrayOf(m.k(1)))
		start := m.pc()
		r := m.load("a")
		m.ret(r)
		target := m.pc()
		m.echoK("[]")
		m.push("a", m.k(2)) // the returned array is not the variable's any more
		m.emit(bytecode.OpEndFinally, 0, 0, 0, 0)
		end := m.pc()
		m.retNull()
		p := stage7cProgram(t, a, []bytecode.Handler{{Start: start, End: target, Target: target, BodyEnd: end, Kind: bytecode.Finally}})
		res := p.Run(vmConfig(t, 64))
		if res.End != Completed || res.Err != nil || string(res.Output) != "[]" || encodeSnapshot(res.Return) != `a:[["i:0","i:1"]]` {
			t.Fatalf("%v %v %q %s", res.End, res.Err, res.Output, encodeSnapshot(res.Return))
		}
	})
	t.Run("throw through finally", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		withoutLiveness(m)
		start := m.pc()
		m.bin(bytecode.Modulo, m.k(1), m.k(0))
		leave := m.emit(bytecode.OpLeave, 0, 0, 0, 0)
		target := m.pc()
		m.echoK("finally")
		m.emit(bytecode.OpEndFinally, 0, 0, 0, 0)
		end := m.pc()
		m.retNull()
		m.code[leave].A = end
		p := stage7cProgram(t, a, []bytecode.Handler{{Start: start, End: target, Target: target, BodyEnd: end, Kind: bytecode.Finally}})
		res := p.Run(vmConfig(t, 64))
		if res.End != UncaughtThrowable || res.Throwable == nil || res.Throwable.Class != "DivisionByZeroError" || string(res.Output)[:7] != "finally" {
			t.Fatalf("%v %v %q", res.End, res.Err, res.Output)
		}
	})
}

// Hand-written bytecode cannot confuse the pending completions: an
// EndFinally whose finally block suspended nothing ends the run with an
// internal error instead of continuing somewhere.
func TestStage7CEndFinallyNeedsItsCompletion(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	start := m.pc()
	leave := m.emit(bytecode.OpLeave, 0, 0, 0, 0)
	target := m.pc()
	m.echoK("f")
	m.emit(bytecode.OpEndFinally, 0, 0, 0, 0)
	end := m.pc()
	m.echoK("after")
	m.retNull()
	m.code[leave].A = end
	p := stage7cProgram(t, a, []bytecode.Handler{{Start: start, End: target, Target: target, BodyEnd: end, Kind: bytecode.Finally}})
	res := p.Run(vmConfig(t, 64))
	if res.End != Completed || string(res.Output) != "fafter" {
		t.Fatalf("%v %v %q", res.End, res.Err, res.Output)
	}
	// The verifier refuses every way into a finally body but its handler,
	// so the missing completion is set up directly: the same frame with
	// nothing pending.
	s := &vmState{}
	f := &vmFrame{fn: &p.funcs[p.entry]}
	s.frames = []*vmFrame{f}
	f.pc = int(target) + 2
	for pc := range f.fn.code {
		if f.fn.code[pc].op == bytecode.OpEndFinally {
			if _, _, _, err := s.endFinally(f, pc); err == nil {
				t.Fatal("EndFinally without a pending completion did not fail")
			}
		}
	}
}
