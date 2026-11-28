package vm

import (
	"sort"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026. try, catch, finally
// and throw over the exception table bytecode 1.0 defines
// (zend_compile_try, zend_compile_throw, zend_handle_loops_and_finally).
//
//	start:   the try block
//	         Jump after                 (Leave after when there is a finally block)
//	catch:   r = the exception          (Catch handler of [start, catch))
//	         t = r instanceof A; Branch t, body, next
//	body:    $e = r; the catch block; Leave after
//	next:    ... ; Throw r              (no clause matched)
//	finally: the finally block          (Finally handler of [start, finally))
//	         EndFinally
//	after:
//
// The VM runs the finally block for every Leave, Return and throw that
// leaves its range, and frees the temporaries that are dead where control
// continues. A break, continue or return that leaves foreach loops goes
// through an exit stub after the loop, which frees their iterators first
// (exitStub, exitLoop, returnValue).

// tryScope is a try statement being compiled: loops is how many loops
// enclosed it, finally whether it has a finally block that is still to
// run, and inCatch whether one of its handler bodies (a catch block, or
// the finally block itself) is being compiled.
type tryScope struct {
	loops   int
	finally bool
	inCatch bool
}

// pendingHandler is a handler whose positions are instruction indexes of
// fc.code, before the prologue is prepended.
type pendingHandler struct {
	start, end, target, bodyEnd int
	register                    uint32
	kind                        bytecode.HandlerKind
}

// leaveTo emits a Leave to label l.
func (fc *fnCompiler) leaveTo(l int) {
	pc := fc.emit(bytecode.OpLeave, 0, 0, 0, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 0, l})
}

// exitStub is the code a break, continue or return jumps to when it leaves
// foreach loops: it frees their iterators, innermost first, and goes on to
// the target (PHP's FE_FREE before the jump or the RETURN). It is emitted
// right after the outermost loop it frees, which puts it outside every try
// statement inside that loop: a Drop inside a protected range would leave
// the iterator undefined for the handler, and PHP attributes an exception
// raised by freeing a loop variable to the end of the loop, not to the try
// around the break.
type exitStub struct {
	label, target int
	drops         []uint32
	line          int
	leave, ret    bool
	notice        uint16
	reg           uint32
	// carry makes the stub's Leave carry the return value in reg
	// (bytecode.LeaveCarry) to the next stub of the same return.
	carry bool
}

// popLoop ends the innermost loop and hands its stubs to stmt.
func (fc *fnCompiler) popLoop() {
	l := fc.loops[len(fc.loops)-1]
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.stubs = append(fc.stubs, l.stubs...)
}

// flushStubs emits the stubs of the loop statement just compiled; control
// that leaves the loop normally jumps over them.
func (fc *fnCompiler) flushStubs() {
	if len(fc.stubs) == 0 {
		return
	}
	stubs := fc.stubs
	fc.stubs = nil
	skip := fc.label()
	fc.jump(skip)
	saved := fc.cur
	for _, s := range stubs {
		fc.bind(s.label)
		fc.cur = s.line
		fc.emitStub(s)
	}
	fc.cur = saved
	fc.bind(skip)
}

// emitStub emits the code of stub s at the current position.
func (fc *fnCompiler) emitStub(s exitStub) {
	for _, r := range s.drops {
		fc.emit(bytecode.OpDrop, 0, r, 0, 0)
	}
	switch {
	case s.ret:
		fc.emit(bytecode.OpReturn, s.notice, s.reg, 0, 0)
	case s.carry:
		fc.leaveCarry(s.target, s.reg)
	case s.leave:
		fc.leaveTo(s.target)
	default:
		fc.jump(s.target)
	}
}

// leaveCarry emits a Leave to label l that carries register r.
func (fc *fnCompiler) leaveCarry(l int, r uint32) {
	pc := fc.emit(bytecode.OpLeave, bytecode.LeaveCarry, 0, 0, r)
	fc.fixups = append(fc.fixups, fixup{pc, 0, l})
}

// leaves reports whether control that goes from the code compiled now to a
// place inside the loops[:outer+1] and outside the loops above inner needs
// a Leave: it leaves a finally range or a catch block of a try statement
// that lies between the two.
func (fc *fnCompiler) leaves(outer, inner int) bool {
	for _, t := range fc.tries {
		if t.loops > outer && t.loops <= inner && (t.finally || t.inCatch) {
			return true
		}
	}
	return false
}

// iterators are the iterator registers of the foreach loops above index li,
// innermost first.
func (fc *fnCompiler) iterators(li int) []uint32 {
	var out []uint32
	for i := len(fc.loops) - 1; i > li; i-- {
		if fc.loops[i].hasIter {
			out = append(out, fc.loops[i].iter)
		}
	}
	return out
}

// goTo emits a jump to label l, as a Leave when leave is set.
func (fc *fnCompiler) goTo(l int, leave bool) {
	if leave {
		fc.leaveTo(l)
		return
	}
	fc.jump(l)
}

// exitLoop is a break or continue to label target of the loop at index li.
// With no iterator to free it is a jump, or a Leave when it leaves a
// finally range or a catch block (leaving the try block of a try without
// finally is the plain jump PHP emits). Otherwise it goes to a stub after
// the outermost loop it leaves, which frees the iterators and goes on.
func (fc *fnCompiler) exitLoop(li, target int) {
	top := len(fc.loops) - 1
	drops := fc.iterators(li)
	if len(drops) == 0 {
		fc.goTo(target, fc.leaves(li, top+1))
		return
	}
	j := li + 1
	stub := exitStub{label: fc.label(), target: target, drops: drops, line: fc.cur, leave: fc.leaves(li, j)}
	fc.loops[j].stubs = append(fc.loops[j].stubs, stub)
	fc.goTo(stub.label, fc.leaves(j, top+1))
}

// returnValue is the Return of register r. PHP frees the loop variables
// a return leaves and runs the finally blocks around it in nesting order,
// innermost first, with the value kept in a temporary meanwhile
// (zend_handle_loops_and_finally). The loop variables between two finally
// blocks are freed by a stub after the outermost of those loops, which lies
// inside the outer try, so an exception a destructor throws there reaches
// that try's catch and its finally block; the value goes from stub to stub
// with Leaves that carry it across the finally blocks (LeaveCarry). The
// last stub returns it. Without a finally block to cross, one stub frees
// every loop variable and returns, and with no loop variable to free the
// Return stays in place, where the VM runs the finally blocks itself.
func (fc *fnCompiler) returnValue(notice uint16, r uint32) {
	all := fc.iterators(-1)
	if len(all) == 0 {
		fc.emit(bytecode.OpReturn, notice, r, 0, 0)
		return
	}
	// The finally blocks still to run, innermost first, by how many loops
	// enclose each try statement.
	var finallies []int
	for i := len(fc.tries) - 1; i >= 0; i-- {
		if fc.tries[i].finally {
			finallies = append(finallies, fc.tries[i].loops)
		}
	}
	// groups[i] are the loops (indexes into fc.loops) freed after the
	// i-th finally block runs and before the next one: group 0 is inside
	// the innermost finally's try, the last group outside every one.
	groups := make([][]int, len(finallies)+1)
	for li := len(fc.loops) - 1; li >= 0; li-- {
		if !fc.loops[li].hasIter {
			continue
		}
		g := 0
		for g < len(finallies) && li < finallies[g] {
			g++
		}
		groups[g] = append(groups[g], li)
	}
	chain := false
	for _, g := range groups[1:] {
		chain = chain || len(g) > 0
	}
	if !chain {
		// Every loop to free is inside the innermost finally's try: one
		// stub after the outermost of them frees them all and returns,
		// and the VM runs the finally blocks around that Return.
		fc.returnStub(notice, r, all)
		return
	}
	if r < fc.base {
		// The value leaves its register while finally blocks run: a
		// variable's own register must stay as it is.
		t := fc.alloc()
		fc.emit(bytecode.OpMove, 0, t, r, 0)
		r = t
	}
	// The stubs of the chain, from the return outwards. A group without
	// loops to free needs none: the next Leave crosses its finally block
	// too.
	type hop struct {
		label int
		loop  int // the outermost loop of the group, or -1 for the tail
		drops []uint32
	}
	var hops []hop
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		var drops []uint32
		for _, li := range g {
			drops = append(drops, fc.loops[li].iter)
		}
		hops = append(hops, hop{label: fc.label(), loop: g[len(g)-1], drops: drops})
	}
	if len(groups[len(groups)-1]) == 0 {
		// No loop outside the outermost finally's try: the Return goes
		// after the function's code, outside every try statement.
		hops = append(hops, hop{label: fc.label(), loop: -1})
	}
	first := hops[0]
	if len(groups[0]) > 0 {
		// The first stub is inside the innermost finally's try: a jump,
		// or a Leave out of a catch block, where the value stays in r.
		fc.goTo(first.label, fc.leaves(first.loop, len(fc.loops)))
	} else {
		fc.leaveCarry(first.label, r)
	}
	fc.splitAt(len(fc.code)-1, all)
	for i, h := range hops {
		stub := exitStub{label: h.label, drops: h.drops, line: fc.cur, notice: notice, reg: r}
		if i == len(hops)-1 {
			stub.ret = true
		} else {
			stub.target, stub.carry = hops[i+1].label, true
		}
		if h.loop < 0 {
			fc.tail = append(fc.tail, stub)
		} else {
			fc.loops[h.loop].stubs = append(fc.loops[h.loop].stubs, stub)
		}
	}
}

// returnStub is a return that frees the loop variables drops (innermost
// first) in a stub after the outermost loop with one and returns there.
func (fc *fnCompiler) returnStub(notice uint16, r uint32, drops []uint32) {
	m := 0
	for !fc.loops[m].hasIter {
		m++
	}
	stub := exitStub{label: fc.label(), drops: drops, line: fc.cur, ret: true, notice: notice, reg: r}
	fc.loops[m].stubs = append(fc.loops[m].stubs, stub)
	fc.goTo(stub.label, fc.leaves(m, len(fc.loops)))
	fc.splitAt(len(fc.code)-1, drops)
}

func (fc *fnCompiler) tryStmt(n *ir.Node) {
	var catches []*ir.Node
	var finally *ir.Node
	for _, k := range n.Kids[1:] {
		switch k.Op {
		case ir.OpCatch:
			catches = append(catches, k)
		case ir.OpFinally:
			finally = k
		}
	}
	mark := fc.top
	after := fc.label()
	fc.tries = append(fc.tries, tryScope{loops: len(fc.loops), finally: finally != nil})
	start := len(fc.code)
	fc.stmt(n.Kids[0])
	// The end of the try block: through the finally block when there is
	// one, else a jump over the catch blocks (ZEND_JMP).
	if finally != nil {
		fc.leaveTo(after)
	} else {
		fc.jump(after)
	}
	end := len(fc.code)
	if len(catches) > 0 {
		fc.top = mark
		r := fc.alloc()
		target := len(fc.code)
		fc.tries[len(fc.tries)-1].inCatch = true
		for _, c := range catches {
			fc.at(c.Line)
			body, next := fc.label(), fc.label()
			classes := c.Kids[:len(c.Kids)-1]
			for i, class := range classes {
				// ZEND_CATCH looks the class up without loading or
				// reporting it: an unknown class matches nothing.
				t := fc.alloc()
				fc.emit(bytecode.OpInstanceof, uint16(bytecode.ClassNamed), t, r, fc.c.stringConst(class.Name.Resolved))
				other := next
				if i < len(classes)-1 {
					other = fc.label()
				}
				fc.branch(t, body, other)
				if other != next {
					fc.bind(other)
				}
				fc.top = mark + 1
			}
			fc.bind(body)
			if c.Var != nil {
				fc.emit(bytecode.OpStore, 0, fc.varLoc(c.Var), r, 0)
			}
			fc.emit(bytecode.OpDrop, 0, r, 0, 0)
			fc.stmt(c.Kids[len(c.Kids)-1])
			fc.leaveTo(after)
			fc.bind(next)
		}
		fc.emit(bytecode.OpThrow, 0, r, 0, 0)
		fc.handlers = append(fc.handlers, pendingHandler{start: start, end: end, target: target, bodyEnd: len(fc.code), register: r, kind: bytecode.Catch})
	}
	if finally != nil {
		// The finally block is a handler body too: an exit from it (a
		// return to a stub that frees iterators) is a Leave, and it no
		// longer runs this try's own finally block.
		fc.tries[len(fc.tries)-1] = tryScope{loops: len(fc.loops), inCatch: true}
		fc.top = mark
		target := len(fc.code)
		fc.at(finally.Line)
		fc.stmt(finally.Kids[0])
		fc.emit(bytecode.OpEndFinally, 0, 0, 0, 0)
		fc.handlers = append(fc.handlers, pendingHandler{start: start, end: target, target: target, bodyEnd: len(fc.code), kind: bytecode.Finally})
	}
	fc.tries = fc.tries[:len(fc.tries)-1]
	fc.bind(after)
	fc.top = mark
}

// throwExpr is throw as a statement or an expression: the value is thrown
// and control never continues, so the result register is never written.
func (fc *fnCompiler) throwExpr(n *ir.Node) operand {
	// zend_compile_throw emits ZEND_THROW on the line compilation has
	// reached after the operand: where "Can only throw objects" is
	// reported.
	r := fc.value(n.Kids[0])
	fc.emit(bytecode.OpThrow, 0, r, 0, 0)
	// The code after a throw is unreachable, but an expression still
	// names a result: a register the verifier sees defined.
	res := fc.alloc()
	fc.emit(bytecode.OpConst, 0, res, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	return reg(res)
}

// handlerTable is the function's exception table with the prologue's
// plen instructions before the code: by start, the longer range first.
func (fc *fnCompiler) handlerTable(plen int) []bytecode.Handler {
	if len(fc.handlers) == 0 {
		return nil
	}
	hs := append([]pendingHandler(nil), fc.handlers...)
	sort.SliceStable(hs, func(i, j int) bool {
		if hs[i].start != hs[j].start {
			return hs[i].start < hs[j].start
		}
		return hs[i].end > hs[j].end
	})
	out := make([]bytecode.Handler, len(hs))
	for i, h := range hs {
		out[i] = bytecode.Handler{Start: uint32(h.start + plen), End: uint32(h.end + plen), Target: uint32(h.target + plen), BodyEnd: uint32(h.bodyEnd + plen), Kind: h.kind}
		if h.kind == bytecode.Catch {
			out[i].Register = h.register
		}
	}
	return out
}
