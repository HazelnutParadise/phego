package vm

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026. Closures, arrow
// functions and first-class callables.

// closureIndex is the function index of a closure's body, assigned the
// first time code that creates the closure is compiled.
func (c *compiler) closureIndex(f *ir.Func) uint32 {
	if i, ok := c.closures[f]; ok {
		return i
	}
	if c.closures == nil {
		c.closures = map[*ir.Func]uint32{}
	}
	i := uint32(len(c.funcs))
	c.funcs = append(c.funcs, f)
	c.closures[f] = i
	return i
}

// closure compiles function () use (...) {} and fn () => ...
// (ZEND_DECLARE_LAMBDA_FUNCTION, then one ZEND_BIND_LEXICAL per captured
// variable, each on the line of its use). PHP leaves its line counter at
// the end of the declaration.
func (fc *fnCompiler) closure(n *ir.Node) operand {
	f := n.Func
	if len(f.Captures) > 0xffff+1 {
		fc.unsupported("too many captured variables", n.Line)
	}
	index := fc.c.closureIndex(f)
	fc.at(n.Line)
	res := fc.alloc()
	fc.emit(bytecode.OpClosure, 0, res, index, 0)
	for i, cp := range captureList(f) {
		v := fc.f.Var(cp.Name)
		if v == nil {
			panic(internalError{"captured variable not in the defining function's table"})
		}
		if !cp.Implicit {
			fc.at(cp.Line)
		}
		fc.emit(bytecode.OpBindLexical, uint16(i), res, fc.varLoc(v), index)
	}
	// zend_compile_func_decl leaves the line counter at the declaration's
	// last line: the closing brace, or the end of an arrow function's
	// expression.
	end := f.EndLine
	if end == 0 {
		end = lastLine(f.Body, n.Line)
	}
	fc.at(end)
	return reg(res)
}

// callable compiles the first-class callable syntax f(...): the call is
// started as usual and ZEND_CALLABLE_CONVERT turns it into a Closure.
func (fc *fnCompiler) callable(n *ir.Node) operand {
	call := n.Kids[0]
	switch call.Op {
	case ir.OpCall:
		if call.Name == nil {
			panic(internalError{"call without a name"})
		}
		res := fc.alloc()
		fc.at(call.Line)
		if _, index, known := fc.resolve(call.Name); known {
			fc.emit(bytecode.OpInitCall, 0, 0, index, 0)
		} else {
			for _, name := range []string{call.Name.Resolved, call.Name.Fallback} {
				if name != "" && fc.c.host != nil && fc.c.host(phpname.Lower(name)) {
					fc.unsupported("a closure of host function "+name+"()", n.Line)
				}
				if name != "" && isBuiltin(name) {
					fc.unsupported("a closure of builtin function "+name+"()", n.Line)
				}
			}
			// The name is looked up when the instruction runs, which
			// throws for a function that does not exist.
			callee := fc.alloc()
			fc.emit(bytecode.OpConst, 0, callee, fc.c.stringConst(call.Name.Resolved), 0)
			fc.emit(bytecode.OpInitMethod, bytecode.InitCallable, callee, 0, 0)
		}
		fc.sent() // the call's mark, which the conversion takes
		fc.pending--
		fc.emit(bytecode.OpCallableConvert, 0, res, 0, 0)
		fc.top = res - fc.base + 1
		return reg(res)
	case ir.OpCallValue, ir.OpMethodCall, ir.OpStaticCall:
		fc.converting = call
		return fc.expr(call)
	}
	fc.unsupportedNode(n)
	return operand{}
}

// usesThis reports whether the code uses $this itself (ZEND_ACC_USES_THIS),
// not counting the closures it creates.
func usesThis(nodes []*ir.Node) bool {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Op == ir.OpThis || usesThis(n.Kids) {
			return true
		}
		for _, a := range n.Args {
			if a != nil && a.Value != nil && usesThis([]*ir.Node{a.Value}) {
				return true
			}
		}
	}
	return false
}

// captureList is the use list of closure f: its captures that have a
// variable in the closure. An arrow function also lists the names only a
// function nested in it uses (PHP binds those too, to no effect).
func captureList(f *ir.Func) []*ir.Capture {
	out := make([]*ir.Capture, 0, len(f.Captures))
	for _, cp := range f.Captures {
		if f.Var(cp.Name) != nil {
			out = append(out, cp)
		}
	}
	return out
}

// lastLine is the greatest line of the nodes, at least floor.
func lastLine(nodes []*ir.Node, floor int) int {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		floor = max(floor, n.Line, lastLine(n.Kids, floor))
		for _, a := range n.Args {
			if a != nil && a.Value != nil {
				floor = lastLine([]*ir.Node{a.Value}, floor)
			}
		}
		if n.Func != nil && (n.Op == ir.OpClosure || n.Op == ir.OpArrowFn) {
			floor = max(floor, n.Func.EndLine, lastLine(n.Func.Body, floor))
		}
	}
	return floor
}

// createsClosure reports a closure, an arrow function or a first-class
// callable among the nodes.
func createsClosure(nodes []*ir.Node) bool {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Op == ir.OpClosure || n.Op == ir.OpArrowFn || n.Op == ir.OpCallable || createsClosure(n.Kids) {
			return true
		}
		for _, a := range n.Args {
			if a != nil && a.Value != nil && createsClosure([]*ir.Node{a.Value}) {
				return true
			}
		}
	}
	return false
}

// appendsElement reports an element access with [] at some level of n.
func appendsElement(n *ir.Node) bool {
	for ; n != nil && n.Op == ir.OpDim; n = n.Kids[0] {
		if n.Kids[1] == nil {
			return true
		}
	}
	return false
}
