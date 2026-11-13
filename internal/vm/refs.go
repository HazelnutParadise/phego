package vm

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// W-17 stage 3: references. REQ-001, REQ-016, REQ-017, REQ-025; T-009,
// T-010, T-031..T-034. Assignment by reference, by-reference arguments and
// parameters, and foreach by reference, compiled as zend_compile_assign_ref,
// zend_compile_args (SEND_REF) and zend_compile_foreach do.

// refSource compiles the variable or element a reference is taken of, as
// zend_compile_var(BP_VAR_W, by_ref) does, and returns the Location that
// names it: a variable is not fetched, an element is fetched for writing
// level by level, the last level with ZEND_FETCH_DIM_REF.
func (fc *fnCompiler) refSource(n *ir.Node, line int) uint32 {
	switch n.Op {
	case ir.OpVar:
		fc.at(n.Line)
		return fc.varLoc(n.Var)
	case ir.OpDim:
		t := fc.dimTarget(n, line)
		return fc.fetchPath(t, len(t.keys), bytecode.FetchWrite, bytecode.FetchForRef)
	case ir.OpCall, ir.OpMethodCall, ir.OpStaticCall, ir.OpNullsafeMC, ir.OpCallValue, ir.OpNew:
		fc.unsupported("reference to the result of a call", n.Line)
	case ir.OpProp, ir.OpStaticProp, ir.OpNullsafe, ir.OpVarVar, ir.OpSuperGlobal, ir.OpThis:
		fc.unsupportedNode(n)
	default:
		fc.unsupported("reference to a value that is not a variable", n.Line)
	}
	return 0
}

// assignRef compiles "$target =& $source". The target's keys are compiled
// first and its fetches delayed (zend_delayed_compile_var), then the source
// is fetched; when both sides are more than a plain variable, MAKE_REF
// turns the source into a reference before the target's fetches can move
// the array it points into (PHP bug #71539). used asks for the value of the
// expression, which is the bound variable's value.
func (fc *fnCompiler) assignRef(n *ir.Node, used bool) operand {
	target, source := n.Kids[0], n.Kids[1]
	switch target.Op {
	case ir.OpVar:
		l := fc.varLoc(target.Var)
		src := fc.refSource(source, n.Line)
		fc.emit(bytecode.OpBind, 0, l, src, 0)
		return fc.refResult(l, used)
	case ir.OpDim:
		t := fc.dimTarget(target, n.Line)
		mark := fc.top
		var src, flags uint32
		if source.Op == ir.OpVar {
			src = fc.refSource(source, n.Line)
		} else {
			e := fc.refSource(source, n.Line)
			src, flags = fc.alloc(), uint32(bytecode.BindValue)
			fc.emit(bytecode.OpMakeRef, 0, src, e, 0)
			if used {
				fc.unsupported("value of an assignment by reference between two elements", n.Line)
			}
		}
		dst := fc.fetchPath(t, len(t.keys), bytecode.FetchWrite, bytecode.FetchForRef)
		fc.emit(bytecode.OpBind, uint16(flags), dst, src, 0)
		fc.top = mark
		if flags == 0 {
			return fc.refResult(src, used)
		}
		return reg(fc.alloc())
	}
	fc.unsupportedNode(target)
	return operand{}
}

// refResult reads the variable l quietly for the value of an assignment by
// reference; it cannot be undefined after the binding. A discarded value
// gets a register no instruction writes, which fails verification if it
// were ever read.
func (fc *fnCompiler) refResult(l uint32, used bool) operand {
	r := fc.alloc()
	if used {
		fc.emit(bytecode.OpLoad, 1, r, l, 0)
	}
	return reg(r)
}

// refArg compiles an argument for a by-reference parameter, SEND_REF: the
// variable or element becomes a reference at the argument's position, and
// the register that holds it is pushed with the other arguments.
func (fc *fnCompiler) refArg(n *ir.Node, line int) uint32 {
	if n.Op != ir.OpVar && n.Op != ir.OpDim {
		fc.unsupported("passing a value that is not a variable by reference", positive(n.Line, line))
	}
	mark := fc.top
	src := fc.refSource(n, line)
	fc.top = mark
	r := fc.alloc()
	fc.emit(bytecode.OpMakeRef, 0, r, src, 0)
	return r
}

// foreachRefInit compiles the subject of a foreach by reference
// (FE_RESET_RW): a variable or an element is iterated in place, anything
// else as a temporary the loop owns.
func (fc *fnCompiler) foreachRefInit(it uint32, subject *ir.Node) int {
	var src uint32
	switch subject.Op {
	case ir.OpVar, ir.OpDim:
		src = fc.refSource(subject, subject.Line)
	default:
		src = fc.value(subject)
	}
	return fc.emit(bytecode.OpIterInit, uint16(bytecode.IterByRef), it, src, 0)
}

// iterRefTarget compiles the binding of a fetched element to the value
// target of a foreach by reference. A plain variable is bound by IterNext
// itself; an element target receives the reference through a register and
// is fetched after it, as zend_emit_assign_ref_znode compiles it.
func (fc *fnCompiler) iterRefTarget(it uint32, target *ir.Node, exit int) {
	fc.c.line = target.Line
	switch target.Op {
	case ir.OpVar:
		pc := fc.emit(bytecode.OpIterNext, uint16(bytecode.IterLocation|bytecode.IterByRef), it, fc.varLoc(target.Var), 0)
		fc.fixups = append(fc.fixups, fixup{pc, 2, exit})
	case ir.OpDim:
		tmp := fc.alloc()
		pc := fc.emit(bytecode.OpIterNext, uint16(bytecode.IterByRef), it, tmp, 0)
		fc.fixups = append(fc.fixups, fixup{pc, 2, exit})
		t := fc.dimTarget(target, target.Line)
		dst := fc.fetchPath(t, len(t.keys), bytecode.FetchWrite, bytecode.FetchForRef)
		fc.emit(bytecode.OpBind, uint16(bytecode.BindValue), dst, tmp, 0)
	case ir.OpDestructure, ir.OpArray:
		fc.unsupported("list() destructuring", target.Line)
	default:
		fc.unsupportedNode(target)
	}
}
