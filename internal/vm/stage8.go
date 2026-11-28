package vm

import (
	"sort"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026. The unwind table of
// format 1.10 tells the VM what PHP frees when an exception or exit leaves
// code that is building a call or that a return has left
// (cleanup_unfinished_calls, cleanup_live_vars and ZEND_FREE_ON_RETURN).

// callArgs records the arguments of the call whose code starts at start
// and that the instruction just emitted makes: those in temporaries are
// freed before the other temporaries when an exception leaves the call
// unfinished. A by-reference argument is a Location and holds nothing.
func (fc *fnCompiler) callArgs(start int, args []uint32) {
	end := len(fc.code) - 1
	if start >= end {
		return
	}
	for i, r := range args {
		if r >= fc.base && r != elemReg {
			fc.unwinds = append(fc.unwinds, bytecode.Unwind{Kind: bytecode.UnwindCall, Start: uint32(start), End: uint32(end), Reg: r, Count: uint32(i)})
		}
	}
}

// splitAt records that the return whose code continues at pc frees the
// loop variables regs, innermost first (ZEND_FREE_ON_RETURN).
func (fc *fnCompiler) splitAt(pc int, regs []uint32) {
	for i, r := range regs {
		fc.unwinds = append(fc.unwinds, bytecode.Unwind{Kind: bytecode.UnwindSplit, Start: uint32(pc), Reg: r, Count: uint32(i)})
	}
}

// unwindTable is the function's unwind table with the prologue's plen
// instructions before the code, sorted by Start. A call's entries share
// its range; an outer call, which starts first, comes before the calls
// nested in it.
func (fc *fnCompiler) unwindTable(plen int) []bytecode.Unwind {
	if len(fc.unwinds) == 0 {
		return nil
	}
	out := append([]bytecode.Unwind(nil), fc.unwinds...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		// Ranges that start together: the longer one, the outer call,
		// first.
		return out[i].End > out[j].End
	})
	for i := range out {
		out[i].Start += uint32(plen)
		if out[i].End != 0 {
			out[i].End += uint32(plen)
		}
	}
	return out
}

// flushTail emits the exit stubs that belong after the function's code: the
// Returns of values carried out of every finally block.
func (fc *fnCompiler) flushTail() {
	if len(fc.tail) == 0 {
		return
	}
	stubs := fc.tail
	fc.tail = nil
	saved := fc.cur
	for _, s := range stubs {
		fc.bind(s.label)
		fc.cur = s.line
		fc.emitStub(s)
	}
	fc.cur = saved
}

// yieldExpr is yield (zend_compile_yield): the key is evaluated before the
// value. A generator declared to yield by reference yields a reference to
// a variable, an element or a property; anything else is yielded by value
// with PHP's notice, a call's result included, since a call here never
// returns a reference.
func (fc *fnCompiler) yieldExpr(n *ir.Node) operand {
	key, val := n.Kids[0], n.Kids[1]
	res := fc.alloc()
	var flags bytecode.YieldFlags
	var rk, rv uint32
	if key != nil {
		rk = fc.value(key)
		flags |= bytecode.YieldKey
	}
	if val != nil {
		flags |= bytecode.YieldValue
		switch {
		case !fc.f.ByRef:
			rv = fc.value(val)
		case val.Op == ir.OpVar || val.Op == ir.OpDim || val.Op == ir.OpProp || val.Op == ir.OpStaticProp:
			rv = fc.refArg(val, n.Line)
			flags |= bytecode.YieldRef
		default:
			rv = fc.value(val)
			flags |= bytecode.YieldNotice
		}
	}
	fc.emit(bytecode.OpYield, uint16(flags), res, rv, rk)
	fc.top = res - fc.base + 1
	return reg(res)
}

// yieldFrom is yield from (zend_compile_yield_from).
func (fc *fnCompiler) yieldFrom(n *ir.Node) operand {
	res := fc.alloc()
	r := fc.value(n.Kids[0])
	fc.emit(bytecode.OpYieldFrom, 0, res, r, 0)
	fc.top = res - fc.base + 1
	return reg(res)
}

// returnTypeWillChange reports #[\ReturnTypeWillChange] without arguments,
// the one method attribute the VM knows: it silences the deprecation of a
// return type that does not match a tentative one of an internal method.
func returnTypeWillChange(a ir.Attribute) bool {
	return a.Name != nil && phpname.Lower(strings.TrimPrefix(a.Name.Resolved, "\\")) == "returntypewillchange" && len(a.Arguments) == 0
}
