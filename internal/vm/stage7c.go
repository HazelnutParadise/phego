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
//	         Leave after
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
// continues, so a break, continue or return needs no code of its own here
// beyond the Leave.

// tryScope is a try statement being compiled: loops is how many loops
// enclosed it, finally whether it has a finally block.
type tryScope struct {
	loops   int
	finally bool
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

// exitLoop is a break or continue to label target of the loop at index li:
// a plain jump, or a Leave when it leaves a try statement. The iterators of
// the foreach loops left on the way are dropped here, unless a finally
// block runs first: PHP frees them in nesting order around the finally
// blocks, which the VM does when the Leave completes.
func (fc *fnCompiler) exitLoop(li, target int) {
	tries, finally := false, false
	for _, t := range fc.tries {
		if t.loops > li {
			tries = true
			finally = finally || t.finally
		}
	}
	if !finally {
		for _, inner := range fc.loops[li+1:] {
			if inner.hasIter {
				fc.emit(bytecode.OpDrop, 0, inner.iter, 0, 0)
			}
		}
	}
	if tries {
		fc.leaveTo(target)
		return
	}
	fc.jump(target)
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
	fc.leaveTo(after)
	end := len(fc.code)
	if len(catches) > 0 {
		fc.top = mark
		r := fc.alloc()
		target := len(fc.code)
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
	fc.tries = fc.tries[:len(fc.tries)-1]
	if finally != nil {
		fc.top = mark
		target := len(fc.code)
		fc.at(finally.Line)
		fc.stmt(finally.Kids[0])
		fc.emit(bytecode.OpEndFinally, 0, 0, 0, 0)
		fc.handlers = append(fc.handlers, pendingHandler{start: start, end: target, target: target, bodyEnd: len(fc.code), kind: bytecode.Finally})
	}
	fc.bind(after)
	fc.top = mark
}

// throwExpr is throw as a statement or an expression: the value is thrown
// and control never continues, so the result register is never written.
func (fc *fnCompiler) throwExpr(n *ir.Node) operand {
	r := fc.value(n.Kids[0])
	fc.at(n.Line)
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
