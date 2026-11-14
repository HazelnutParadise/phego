package vm

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// W-17 stage 4: destructuring. REQ-001, REQ-016, REQ-025, REQ-026; T-009,
// T-010, T-029..T-032. "[$a, $b] = $x", list(), keyed and nested patterns,
// element targets, by-reference elements and foreach with a pattern, as
// zend_compile_assign (ZEND_AST_ARRAY), zend_compile_list_assign and
// zend_compile_foreach compile them in PHP 8.5.12.
//
// Without a by-reference element the right-hand side is a value: a plain
// variable is copied first (QM_ASSIGN, on the pattern's line) so that
// "[$a, $b] = $a" reads the array as it was, every other expression is
// evaluated once. Each element is read with FETCH_LIST_R (ArrayGet with
// ArrayGetList): a missing key warns and reads null, a container that is
// not an array reads null, with "Cannot use T as array" unless it is null,
// and a string is not a string container. The value of the assignment is
// the right-hand side.
//
// With a by-reference element (anywhere in the nesting) the right-hand side
// is a variable or an element made a reference (MAKE_REF), which the
// register rr holds for the whole assignment and releases at its end. An
// element that is bound, or holds a nested pattern with references, is
// fetched for writing from that reference (FETCH_LIST_W: FetchDim with
// FetchContainerValue) and made a reference itself; the other elements are
// read through the reference with FETCH_LIST_R.
//
// Lines follow zend_compile_list_assign: a fetch carries the line reached
// before it, which is the right-hand side's last line for the first one;
// the assignment of a plain variable moves the line to that variable, an
// element target or a by-reference target leaves it where the fetch was.

// patternHasRef reports whether a pattern binds any element by reference,
// zend_propagate_list_refs.
func patternHasRef(p *ir.Node) bool {
	for _, it := range p.Kids {
		if it == nil {
			continue
		}
		if it.Flags&ir.FlagRef != 0 {
			return true
		}
		if v := it.Kids[1]; v != nil && v.Op == ir.OpArray && patternHasRef(v) {
			return true
		}
	}
	return false
}

// patternLine is the line PHP's AST gives a pattern: that of its first
// element's target, down to the base variable of an element target or the
// first leaf of a nested pattern. A pattern of only skipped positions has
// the line of its own node.
func patternLine(p *ir.Node) int {
	for _, it := range p.Kids {
		if it == nil {
			continue
		}
		n := it.Kids[1]
		for n != nil {
			switch n.Op {
			case ir.OpArray:
				return positive(patternLine(n), it.Line)
			case ir.OpDim:
				n = n.Kids[0]
				continue
			}
			break
		}
		if n != nil && n.Line > 0 {
			return n.Line
		}
		return positive(it.Line, p.Line)
	}
	return p.Line
}

// patternKeyed reports whether the pattern's elements carry keys
// (list_is_keyed); the lowering already rejected a mix.
func patternKeyed(p *ir.Node) bool {
	for _, it := range p.Kids {
		if it != nil {
			return it.Kids[0] != nil
		}
	}
	return false
}

// destructure compiles "<pattern> = value" (OpDestructure). used asks for
// the value of the expression.
func (fc *fnCompiler) destructure(n *ir.Node, used bool) operand {
	pattern, value := n.Kids[0], n.Kids[1]
	if patternHasRef(pattern) {
		return fc.destructureRef(n, used)
	}
	var src uint32
	if value.Op == ir.OpVar {
		// QM_ASSIGN on the pattern's line: the array is copied before any
		// element is assigned, so "[$a, $b] = $a" reads the old $a.
		o := fc.expr(value)
		fc.at(patternLine(pattern))
		src = fc.materialize(o)
	} else {
		src = fc.value(value)
	}
	fc.list(pattern, src)
	return reg(src)
}

// list compiles the elements of a pattern against the Value register src,
// which holds the array, or a reference to it, that the elements are read
// from (FETCH_LIST_R).
func (fc *fnCompiler) list(pattern *ir.Node, src uint32) {
	keyed := patternKeyed(pattern)
	for i, it := range pattern.Kids {
		if it == nil {
			continue
		}
		mark := fc.top
		rk := fc.listKey(keyed, it, i)
		e := fc.alloc()
		fc.emit(bytecode.OpArrayGet, bytecode.ArrayGetList, e, src, rk)
		fc.assignElement(it.Kids[1], e)
		fc.top = mark
	}
}

// listKey evaluates the key of element i: the key expression of a keyed
// pattern (a plain variable is read right before the fetch, as the fetch
// itself reads a CV operand), or the position in an unkeyed one.
func (fc *fnCompiler) listKey(keyed bool, it *ir.Node, i int) uint32 {
	if keyed {
		return fc.value(it.Kids[0])
	}
	r := fc.alloc()
	fc.emit(bytecode.OpConst, 0, r, fc.c.literal(&ir.Literal{Kind: ir.LitInt, Int: int64(i)}), 0)
	return r
}

// assignElement compiles the assignment of a fetched element (in register
// e) to a pattern element's target, zend_emit_assign_znode: a variable is
// assigned on its own line, which the line then stays at; an element target
// has its keys evaluated after the fetch and leaves the line at the fetch;
// a nested pattern is destructured from e.
func (fc *fnCompiler) assignElement(target *ir.Node, e uint32) {
	switch target.Op {
	case ir.OpArray:
		fc.list(target, e)
	case ir.OpVar:
		fc.c.line = target.Line
		fc.at(target.Line)
		fc.emit(bytecode.OpStore, 0, fc.varLoc(target.Var), e, 0)
	case ir.OpDim:
		after := fc.cur
		fc.c.line = target.Line
		t := fc.dimTarget(target, target.Line)
		fc.writeDim(t, reg(e), false, false)
		fc.cur = after
	default:
		fc.unsupportedNode(target)
	}
}

// destructureRef compiles a destructuring assignment with by-reference
// elements. The right-hand side must be a variable or an element (the
// lowering rejects anything else as PHP does); it becomes a reference the
// register rr holds, and the elements are fetched from it.
func (fc *fnCompiler) destructureRef(n *ir.Node, used bool) operand {
	pattern, value := n.Kids[0], n.Kids[1]
	if used && value.Op != ir.OpVar {
		fc.unsupported("value of a destructuring assignment by reference from an element", n.Line)
	}
	mark := fc.top
	src := fc.refSource(value, n.Line)
	fc.top = mark
	rr := fc.alloc()
	fc.emit(bytecode.OpMakeRef, 0, rr, src, 0)
	fc.listRef(pattern, rr)
	// The reference the right-hand side became is released now, not when
	// the register is reused: PHP frees the VAR at the end of the
	// statement, and the reference count is visible when the array is
	// copied later. The value of the expression is the variable's value
	// after the assignment, read quietly like the result of "$a = &$b".
	fc.emit(bytecode.OpDrop, 0, rr, 0, 0)
	fc.top = mark
	r := fc.alloc()
	if used {
		fc.emit(bytecode.OpLoad, 1, r, src, 0)
	}
	return reg(r)
}

// listRef compiles the elements of a pattern with references against the
// Value register rr, which holds a reference to the array (MAKE_REF, or a
// by-reference IterNext). An element bound by reference, or holding a
// nested pattern with references, is fetched for writing (FETCH_LIST_W),
// made a reference and bound or destructured; any other element is read
// with FETCH_LIST_R through the reference.
func (fc *fnCompiler) listRef(pattern *ir.Node, rr uint32) {
	keyed := patternKeyed(pattern)
	for i, it := range pattern.Kids {
		if it == nil {
			continue
		}
		mark := fc.top
		target := it.Kids[1]
		byRef := it.Flags&ir.FlagRef != 0 || target.Op == ir.OpArray && patternHasRef(target)
		if !byRef {
			rk := fc.listKey(keyed, it, i)
			e := fc.alloc()
			fc.emit(bytecode.OpArrayGet, bytecode.ArrayGetList, e, rr, rk)
			fc.assignElement(target, e)
			fc.top = mark
			continue
		}
		flags := bytecode.FetchWrite | bytecode.FetchForRef | bytecode.FetchContainerValue
		var rk uint32
		switch {
		case !keyed:
			rk = fc.listKey(false, it, i)
		default:
			if o := fc.expr(it.Kids[0]); o.cv >= 0 && !o.quiet {
				// The fetch reads a plain variable key itself, after it
				// has checked the container.
				rk, flags = fc.loc(o.cv), flags|bytecode.FetchKeyLocation
			} else {
				rk = fc.materialize(o)
			}
		}
		fc.emit(bytecode.OpFetchDim, uint16(flags), elemReg, rr, rk)
		fc.top = mark
		r := fc.alloc()
		fc.emit(bytecode.OpMakeRef, 0, r, elemReg, 0)
		switch target.Op {
		case ir.OpArray:
			fc.listRef(target, r)
			fc.emit(bytecode.OpDrop, 0, r, 0, 0) // the nested VAR is freed
		case ir.OpVar:
			fc.c.line = target.Line
			fc.emit(bytecode.OpBind, uint16(bytecode.BindValue), fc.varLoc(target.Var), r, 0)
		case ir.OpDim:
			// zend_compile_assign_ref with a ZNODE source: the target's
			// keys, then its fetches by reference, then ASSIGN_REF; the
			// line stays at the element's fetch.
			after := fc.cur
			fc.c.line = target.Line
			t := fc.dimTarget(target, target.Line)
			dst := fc.fetchPath(t, len(t.keys), bytecode.FetchWrite, bytecode.FetchForRef)
			fc.cur = after
			fc.emit(bytecode.OpBind, uint16(bytecode.BindValue), dst, r, 0)
		default:
			fc.unsupportedNode(target)
		}
		fc.top = mark
	}
}

// foreachList compiles the pattern a foreach assigns each element to:
// FE_FETCH_R puts the element in a temporary that the pattern is read from;
// with references, FE_FETCH_RW puts a reference to the element there, the
// pattern binds through it and the reference is released at the end of the
// assignments, as PHP frees the VAR.
func (fc *fnCompiler) foreachList(it uint32, pattern *ir.Node, byRef bool, exit int) {
	tmp := fc.alloc()
	if byRef {
		pc := fc.emit(bytecode.OpIterNext, uint16(bytecode.IterByRef), it, tmp, 0)
		fc.fixups = append(fc.fixups, fixup{pc, 2, exit})
		fc.listRef(pattern, tmp)
		fc.emit(bytecode.OpDrop, 0, tmp, 0, 0)
		return
	}
	pc := fc.emit(bytecode.OpIterNext, 0, it, tmp, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 2, exit})
	fc.list(pattern, tmp)
}
