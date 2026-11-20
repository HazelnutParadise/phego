package vm

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// W-17 stage 6: global, static variables, variadic parameters, argument
// unpacking and named arguments. REQ-001, REQ-016, REQ-025, REQ-026;
// T-009, T-010, T-029..T-032. Compiled as zend_compile_global_var,
// zend_compile_static_var, zend_compile_params and zend_compile_args do.

// globalStmt compiles one variable of a global statement, ZEND_BIND_GLOBAL:
// the local becomes a reference to the top-level variable of its name.
func (fc *fnCompiler) globalStmt(n *ir.Node) {
	if n.Var == nil {
		if len(n.Kids) > 0 {
			fc.unsupported("variable variables", n.Line)
		}
		fc.unsupported("superglobal", n.Line)
	}
	fc.emit(bytecode.OpBindGlobal, 0, fc.varLoc(n.Var), fc.c.stringConst(n.Var.Name), 0)
}

// staticStmt compiles one variable of a static statement. PHP binds a
// literal initializer with ZEND_BIND_STATIC alone and any other one with
// ZEND_BIND_INIT_STATIC_OR_JMP, the expression and ZEND_BIND_STATIC; both
// are the second form here, since a literal evaluated on the first run
// gives the same value:
//
//	      StaticInit L, done   ; bound and skipped once initialized
//	      <initializer -> r>   ; null without one
//	      BindStatic L, r      ; initializes unless a nested run did, binds
//	done:
//
// The initializer runs on every execution that finds the variable not yet
// initialized, so one that throws runs again on the next call, and a call
// made from inside it evaluates it again, as in PHP.
func (fc *fnCompiler) staticStmt(n *ir.Node) {
	if n.Var == nil {
		panic(internalError{"static statement without a variable"})
	}
	l := fc.varLoc(n.Var)
	done := fc.label()
	pc := fc.emit(bytecode.OpStaticInit, 0, l, 0, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 1, done})
	var r uint32
	if len(n.Kids) > 0 && n.Kids[0] != nil {
		r = fc.value(n.Kids[0])
	} else {
		r = fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	}
	fc.emit(bytecode.OpBindStatic, 0, l, r, 0)
	fc.bind(done)
}

// fixedParams returns the parameters that have a register, and the variadic
// parameter or nil.
func fixedParams(f *ir.Func) ([]*ir.Param, *ir.Param) {
	if n := len(f.Params); n > 0 && f.Params[n-1].Variadic {
		return f.Params[:n-1], f.Params[n-1]
	}
	return f.Params, nil
}

// argByRef reports whether the callee takes the argument by reference: the
// parameter of that name or position, or the variadic parameter for an
// argument beyond them. An unknown name is sent by value; the send throws.
func argByRef(callee *ir.Func, name string, position int) bool {
	fixed, variadic := fixedParams(callee)
	if name != "" {
		for _, p := range fixed {
			if p.Name == name {
				return p.ByRef
			}
		}
	} else if position < len(fixed) {
		return fixed[position].ByRef
	}
	return variadic != nil && variadic.ByRef
}

// calleeKnown reports whether PHP knows the callee while it compiles the
// call (zend_compile_call finds it in the function table, finalized): the
// name needs no run-time resolution in a namespace, and the function was
// declared, completely, before the code being compiled. Top-level code
// knows a function declared on an earlier line; a function body knows the
// functions declared before its own function.
func (fc *fnCompiler) calleeKnown(n *ir.Node, callee *ir.Func) bool {
	if n.Name.Fallback != "" {
		return false
	}
	if fc.f.Kind == ir.FuncMain {
		// A declaration on the call's own line is taken to precede it:
		// the HIR has no columns, and "function f() {} f(...)" is the
		// order such a line is written in.
		return callee.Line <= n.Line
	}
	self, ok := fc.c.declared[fc.f]
	return ok && fc.c.declared[callee] < self
}

// argListLine is the line of a call's argument list, that of its first
// argument. When PHP does not know the callee it gives a plain variable
// argument this line (zend_compile_args sets CG(zend_lineno) to the list's
// before it emits the SEND), so that is where its undefined-variable
// warning points.
//
// A Zend AST node takes the line of its first child, so the line is that of
// the leftmost operand of the first argument: the first element of an array
// literal, the left operand of an operator.
func argListLine(n *ir.Node) int {
	if len(n.Args) == 0 || n.Args[0].Value == nil {
		return n.Line
	}
	if a := n.Args[0]; a.Name != "" && a.Line > 0 {
		return a.Line // a named argument's node is on its name's line
	}
	first := n.Args[0].Value
	for {
		if first.FoldLine != 0 {
			// A concatenation of literals the parser folded is one string
			// on the line of the token after it.
			return first.FoldLine
		}
		var kid *ir.Node
		for _, k := range first.Kids {
			if k != nil {
				kid = k
				break
			}
		}
		if kid == nil {
			return positive(first.Line, n.Line)
		}
		first = kid
	}
}

// callPending compiles a call with a named or an unpacked argument. Unlike
// a positional call, whose arguments are all evaluated before the first
// Push, each argument is sent as soon as it is evaluated, as PHP's SEND
// opcodes run between the arguments: a send that throws (an unknown or
// repeated name, an operand that cannot be unpacked) does so before the
// next argument is evaluated.
//
//	InitCall f
//	<arg -> r> ; Push r [, name]     ; or Push PushLoad L, name for a variable
//	<arg -> r> ; SendUnpack r        ; or SendUnpack L for a variable
//	CallPending res
func (fc *fnCompiler) callPending(n *ir.Node, index uint32) operand {
	callee := fc.c.unit.Functions[index-1]
	res := fc.alloc()
	fc.emit(bytecode.OpInitCall, 0, 0, index, 0)
	entered := fc.pending
	fc.sent()
	// After an unpacked argument PHP no longer uses what it knows of the
	// callee.
	known, listLine := fc.calleeKnown(n, callee), argListLine(n)
	for i, a := range n.Args {
		mark := fc.top
		var name uint32
		if a.Name != "" {
			name = fc.c.stringConst(a.Name) + 1
		}
		switch {
		case a.Spread:
			// ZEND_SEND_UNPACK reads a plain variable itself: it warns
			// when undefined, and makes references in the variable's own
			// array for by-reference parameters.
			if a.Value.Op == ir.OpVar {
				fc.c.line = a.Value.Line
				fc.at(a.Value.Line)
				fc.emit(bytecode.OpSendUnpack, 0, fc.varLoc(a.Value.Var), 0, 0)
			} else {
				fc.emit(bytecode.OpSendUnpack, 0, fc.value(a.Value), 0, 0)
			}
			known = false
		case argByRef(callee, a.Name, i):
			r := fc.refArg(a.Value, n.Line)
			if !known && a.Value.Op == ir.OpVar {
				fc.cur = listLine
			}
			fc.emit(bytecode.OpPush, 0, r, name, 0)
		case a.Value.Op == ir.OpVar:
			// ZEND_SEND_VAR reads the variable itself, after it checked
			// the name.
			fc.c.line = a.Value.Line
			fc.at(a.Value.Line)
			if !known {
				fc.cur = listLine
			}
			fc.emit(bytecode.OpPush, uint16(bytecode.PushLoad), fc.varLoc(a.Value.Var), name, 0)
		default:
			fc.emit(bytecode.OpPush, 0, fc.value(a.Value), name, 0)
		}
		fc.sent()
		fc.top = mark
	}
	// ZEND_CHECK_UNDEF_ARGS carries the line reached after the last
	// argument and the call its own: a backtrace taken while the callee
	// fills omitted parameters shows the first, through this Checkpoint.
	fc.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	after := fc.cur
	fc.at(n.Line)
	fc.emit(bytecode.OpCallPending, 0, res, 0, 0)
	fc.cur = after
	fc.pending = entered
	fc.top = res - fc.base + 1
	return reg(res)
}

// sent counts one more entry on the argument stack of the calls being
// built.
func (fc *fnCompiler) sent() {
	fc.pending++
	fc.maxStack = max(fc.maxStack, fc.pending)
	if fc.pending > limits.MaxStack {
		fc.unsupported("too many call arguments", fc.cur)
	}
}
