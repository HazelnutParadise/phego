package vm

import (
	"fmt"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// operand is an expression result. A plain PHP variable is not read when it
// is compiled: PHP reads a CV operand when the consuming operation runs, so
// $k + $k++ adds the incremented $k and the undefined-variable warning comes
// from the consumer. materialize emits that read right before the consumer.
type operand struct {
	reg   uint32
	cv    int // local index of a deferred variable read, or -1
	quiet bool
}

func reg(r uint32) operand { return operand{reg: r, cv: -1} }

func (fc *fnCompiler) materialize(o operand) uint32 {
	if o.cv < 0 {
		return o.reg
	}
	r := fc.alloc()
	fc.emit(bytecode.OpLoad, uint16(boolInt(o.quiet)), r, fc.loc(o.cv), 0)
	return r
}

// value compiles n and reads a variable operand right away.
func (fc *fnCompiler) value(n *ir.Node) uint32 { return fc.materialize(fc.expr(n)) }

// discard compiles an expression whose value is unused. A bare variable is
// still read, as PHP's CHECK_VAR warns about an undefined one.
func (fc *fnCompiler) discard(n *ir.Node) {
	mark := fc.top
	fc.value(n)
	fc.top = mark
}

var binaryOps = map[string]bytecode.BinaryOp{
	"+": bytecode.Add, "-": bytecode.Subtract, "*": bytecode.Multiply, "/": bytecode.Divide,
	".": bytecode.Concat, "%": bytecode.Modulo, "**": bytecode.Power,
	"<<": bytecode.ShiftLeft, ">>": bytecode.ShiftRight,
	"&": bytecode.BitAnd, "|": bytecode.BitOr, "^": bytecode.BitXor,
	"==": bytecode.Equal, "!=": bytecode.NotEqual, "<>": bytecode.NotEqual,
	"===": bytecode.Identical, "!==": bytecode.NotIdentical,
	"<": bytecode.Less, "<=": bytecode.LessOrEqual, "<=>": bytecode.Spaceship,
	// > and >= swap their operands, as PHP compiles them.
	">": bytecode.Less, ">=": bytecode.LessOrEqual,
}

var unaryOps = map[string]bytecode.UnaryOp{"!": bytecode.Not, "-": bytecode.Negate, "+": bytecode.Plus, "~": bytecode.BitNot}

var castKinds = map[string]bytecode.CastKind{
	"bool": bytecode.CastBool, "int": bytecode.CastInt, "float": bytecode.CastFloat,
	"string": bytecode.CastString, "array": bytecode.CastArray,
}

func (fc *fnCompiler) expr(n *ir.Node) operand {
	if n == nil {
		panic(internalError{"missing expression"})
	}
	fc.c.line = n.Line
	fc.at(n.Line)
	switch n.Op {
	case ir.OpLiteral:
		r := fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.literal(n.Value), 0)
		return reg(r)
	case ir.OpVar:
		i, ok := fc.vars[n.Var]
		if !ok {
			panic(internalError{"variable not in the function's table"})
		}
		return operand{cv: i, quiet: n.Flags&ir.FlagQuiet != 0}
	case ir.OpBinary:
		return fc.binary(n)
	case ir.OpLogical:
		return fc.logical(n)
	case ir.OpUnary:
		op, ok := unaryOps[n.Text]
		if !ok {
			panic(internalError{"unary operator " + n.Text})
		}
		return fc.unaryLike(n, bytecode.OpUnary, uint16(op))
	case ir.OpCast:
		kind, ok := castKinds[n.Text]
		if !ok {
			fc.unsupported(n.Text+" cast", n.Line)
		}
		return fc.unaryLike(n, bytecode.OpCast, uint16(kind))
	case ir.OpAssign:
		return fc.assign(n)
	case ir.OpAssignOp:
		return fc.assignOp(n)
	case ir.OpCoalesceSet:
		return fc.coalesceSet(n)
	case ir.OpPreInc, ir.OpPostInc:
		return fc.incDec(n)
	case ir.OpTernary:
		return fc.ternary(n)
	case ir.OpCoalesce:
		return fc.coalesce(n)
	case ir.OpIsset:
		return fc.isset(n)
	case ir.OpDim:
		return fc.dimRead(n)
	case ir.OpArray:
		return fc.array(n)
	case ir.OpInterp:
		return fc.interp(n)
	case ir.OpCall:
		return fc.call(n)
	case ir.OpPrint:
		v := fc.value(n.Kids[0])
		fc.emit(bytecode.OpOutput, 0, v, 0, 0)
		r := fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.literal(&ir.Literal{Kind: ir.LitInt, Int: 1}), 0)
		return reg(r)
	case ir.OpExit:
		return fc.exit(n)
	}
	fc.unsupportedNode(n)
	return operand{}
}

// unsupportedNode reports n's construct as unsupported.
func (fc *fnCompiler) unsupportedNode(n *ir.Node) {
	feature, ok := map[ir.Op]string{
		ir.OpForeach: "foreach", ir.OpClass: "class declaration", ir.OpNew: "new",
		ir.OpTry: "try", ir.OpThrow: "throw", ir.OpStatic: "static variable",
		ir.OpGlobal: "global statement", ir.OpConstDef: "const declaration",
		ir.OpConst: "constant lookup", ir.OpMagic: "magic constant", ir.OpEval: "eval",
		ir.OpMatch: "match", ir.OpDestructure: "list() destructuring",
		ir.OpVarVar: "variable variables", ir.OpSuperGlobal: "superglobal", ir.OpThis: "$this",
		ir.OpProp: "property access", ir.OpNullsafe: "nullsafe property access",
		ir.OpStaticProp: "static property access", ir.OpClassConst: "class constant",
		ir.OpClassName: "::class", ir.OpAssignRef: "reference assignment",
		ir.OpSilence: "@ error suppression", ir.OpEmpty: "empty()",
		ir.OpCallValue: "dynamic call", ir.OpMethodCall: "method call",
		ir.OpNullsafeMC: "nullsafe method call", ir.OpStaticCall: "static method call",
		ir.OpCallable: "first-class callable", ir.OpClone: "clone", ir.OpClosure: "closure",
		ir.OpArrowFn: "arrow function", ir.OpInstanceof: "instanceof",
		ir.OpShell: "shell execution", ir.OpYield: "yield", ir.OpYieldFrom: "yield",
		ir.OpSpread: "argument unpacking", ir.OpGoto: "goto", ir.OpDeclare: "declare",
		ir.OpFuncDecl: "conditional function declaration", ir.OpHalt: "__halt_compiler",
	}[n.Op]
	if n.Op == ir.OpInclude {
		feature, ok = n.Text, n.Text != ""
	}
	if !ok {
		feature = "construct " + string(n.Op)
	}
	fc.unsupported(feature, n.Line)
}

func (fc *fnCompiler) binary(n *ir.Node) operand {
	op, ok := binaryOps[n.Text]
	if !ok {
		if n.Text == "|>" {
			fc.unsupported("pipe operator", n.Line)
		}
		panic(internalError{"binary operator " + n.Text})
	}
	mark := fc.top
	a := fc.expr(n.Kids[0])
	b := fc.expr(n.Kids[1])
	if n.Text == ">" || n.Text == ">=" {
		a, b = b, a
	}
	ra := fc.materialize(a)
	rb := fc.materialize(b)
	r := fc.allocNot(mark, ra, rb)
	fc.emit(bytecode.OpBinary, uint16(op), r, ra, rb)
	return reg(r)
}

func (fc *fnCompiler) unaryLike(n *ir.Node, op bytecode.Opcode, aux uint16) operand {
	mark := fc.top
	v := fc.materialize(fc.expr(n.Kids[0]))
	r := fc.allocNot(mark, v)
	fc.emit(op, aux, r, v, 0)
	return reg(r)
}

// boolConst loads true or false into r.
func (fc *fnCompiler) boolConst(r uint32, v bool) {
	fc.emit(bytecode.OpConst, 0, r, fc.c.literal(&ir.Literal{Kind: ir.LitBool, Bool: v}), 0)
}

func (fc *fnCompiler) logical(n *ir.Node) operand {
	if n.Text == "xor" {
		mark := fc.top
		a := fc.expr(n.Kids[0])
		b := fc.expr(n.Kids[1])
		ra := fc.materialize(a)
		rb := fc.materialize(b)
		r := fc.allocNot(mark, ra, rb)
		fc.emit(bytecode.OpBinary, uint16(bytecode.BoolXor), r, ra, rb)
		return reg(r)
	}
	and := n.Text == "&&"
	if !and && n.Text != "||" {
		panic(internalError{"logical operator " + n.Text})
	}
	res := fc.alloc()
	mark := fc.top
	right, yes, no, end := fc.label(), fc.label(), fc.label(), fc.label()
	a := fc.value(n.Kids[0])
	if and {
		fc.branch(a, right, no)
	} else {
		fc.branch(a, yes, right)
	}
	fc.top = mark
	fc.bind(right)
	b := fc.value(n.Kids[1])
	fc.branch(b, yes, no)
	fc.top = mark
	fc.bind(yes)
	fc.boolConst(res, true)
	fc.jump(end)
	fc.bind(no)
	fc.boolConst(res, false)
	fc.bind(end)
	return reg(res)
}

func (fc *fnCompiler) ternary(n *ir.Node) operand {
	res := fc.alloc()
	mark := fc.top
	then, other, end := fc.label(), fc.label(), fc.label()
	c := fc.value(n.Kids[0])
	fc.branch(c, then, other)
	fc.bind(then)
	if n.Kids[1] == nil {
		fc.emit(bytecode.OpMove, 0, res, c, 0)
	} else {
		fc.top = mark
		fc.emit(bytecode.OpMove, 0, res, fc.value(n.Kids[1]), 0)
	}
	fc.jump(end)
	fc.top = mark
	fc.bind(other)
	fc.emit(bytecode.OpMove, 0, res, fc.value(n.Kids[2]), 0)
	fc.top = mark
	fc.bind(end)
	return reg(res)
}

// coalesce evaluates its left side quietly and keeps it unless it is null.
func (fc *fnCompiler) coalesce(n *ir.Node) operand {
	res := fc.alloc()
	mark := fc.top
	then, other, end := fc.label(), fc.label(), fc.label()
	a := fc.value(n.Kids[0])
	null := fc.alloc()
	fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	set := fc.alloc()
	fc.emit(bytecode.OpBinary, uint16(bytecode.NotIdentical), set, a, null)
	fc.branch(set, then, other)
	fc.bind(then)
	fc.emit(bytecode.OpMove, 0, res, a, 0)
	fc.jump(end)
	fc.top = mark
	fc.bind(other)
	fc.emit(bytecode.OpMove, 0, res, fc.value(n.Kids[1]), 0)
	fc.top = mark
	fc.bind(end)
	return reg(res)
}

// isset tests each operand in order; isset($a, $b) is an && chain.
func (fc *fnCompiler) isset(n *ir.Node) operand {
	res := fc.alloc()
	mark := fc.top
	no, end := fc.label(), fc.label()
	for i, k := range n.Kids {
		var r uint32
		switch k.Op {
		case ir.OpVar:
			fc.at(k.Line)
			r = fc.alloc()
			fc.emit(bytecode.OpIsset, 0, r, fc.varLoc(k.Var), 0)
		case ir.OpDim:
			if k.Kids[1] == nil {
				panic(internalError{"isset of []"})
			}
			base := fc.expr(k.Kids[0])
			key := fc.expr(k.Kids[1])
			rb := fc.materialize(base)
			rk := fc.materialize(key)
			r = fc.allocNot(mark, rb, rk)
			fc.emit(bytecode.OpArrayIsset, 0, r, rb, rk)
		default:
			fc.unsupportedNode(k)
		}
		if i == len(n.Kids)-1 {
			fc.emit(bytecode.OpMove, 0, res, r, 0)
			break
		}
		next := fc.label()
		fc.branch(r, next, no)
		fc.top = mark
		fc.bind(next)
	}
	fc.top = mark
	if len(n.Kids) > 1 {
		fc.jump(end)
		fc.bind(no)
		fc.boolConst(res, false)
		fc.bind(end)
	}
	return reg(res)
}

// plainBase returns the Location of the plain variable an element write
// goes through, or reports why the base is outside stage 1.
func (fc *fnCompiler) plainBase(base *ir.Node, line int) uint32 {
	if base == nil {
		panic(internalError{"element access without a base"})
	}
	switch base.Op {
	case ir.OpVar:
		fc.at(base.Line)
		return fc.varLoc(base.Var)
	case ir.OpDim:
		fc.unsupported("nested array write", line)
	case ir.OpCall, ir.OpMethodCall, ir.OpStaticCall, ir.OpNullsafeMC, ir.OpBinary:
		fc.unsupported("array write through a non-variable", line)
	}
	fc.unsupportedNode(base)
	return 0
}

func (fc *fnCompiler) dimRead(n *ir.Node) operand {
	base, key := n.Kids[0], n.Kids[1]
	if key == nil {
		panic(internalError{"read of []"})
	}
	if base.Op == ir.OpLiteral && base.Value.Kind == ir.LitString {
		fc.unsupported("string offset on a literal", n.Line)
	}
	mark := fc.top
	b := fc.expr(base)
	k := fc.expr(key)
	rb := fc.materialize(b)
	rk := fc.materialize(k)
	r := fc.allocNot(mark, rb, rk)
	fc.emit(bytecode.OpArrayGet, uint16(boolInt(n.Flags&ir.FlagQuiet != 0)), r, rb, rk)
	return reg(r)
}

func (fc *fnCompiler) assign(n *ir.Node) operand {
	target, value := n.Kids[0], n.Kids[1]
	switch target.Op {
	case ir.OpVar:
		// PHP gives ASSIGN its target's line, after compiling the value.
		l := fc.varLoc(target.Var)
		v := fc.expr(value)
		fc.at(target.Line)
		rv := fc.materialize(v)
		fc.emit(bytecode.OpStore, 0, l, rv, 0)
		return reg(rv)
	case ir.OpDim:
		// ASSIGN_DIM is created when its target is compiled, so it carries
		// the line reached after the key (or the base for []), and the value
		// read by its OP_DATA shares it.
		l := fc.plainBase(target.Kids[0], n.Line)
		var k operand
		if target.Kids[1] != nil {
			k = fc.expr(target.Kids[1])
		}
		opLine := fc.cur
		v := fc.expr(value)
		after := fc.cur
		fc.cur = opLine
		if target.Kids[1] == nil {
			rv := fc.materialize(v)
			fc.emit(bytecode.OpArrayAppend, 0, l, rv, 0)
			fc.cur = after
			return reg(rv)
		}
		rk := fc.materialize(k)
		rv := fc.materialize(v)
		fc.emit(bytecode.OpArraySet, 0, l, rk, rv)
		fc.cur = after
		return reg(rv)
	}
	fc.unsupportedNode(target)
	return operand{}
}

// assignOp compiles $x op= e: e first, then $x is read, as PHP's ASSIGN_OP.
func (fc *fnCompiler) assignOp(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar:
	case ir.OpDim:
		fc.unsupported("compound assignment to array element", n.Line)
	default:
		fc.unsupportedNode(target)
	}
	op, ok := binaryOps[n.Text]
	if !ok || n.Text == ">" || n.Text == ">=" {
		panic(internalError{"assignment operator " + n.Text + "="})
	}
	l := fc.varLoc(target.Var)
	mark := fc.top
	v := fc.value(n.Kids[1])
	old := fc.alloc()
	fc.emit(bytecode.OpLoad, 0, old, l, 0)
	r := fc.allocNot(mark, v, old)
	fc.emit(bytecode.OpBinary, uint16(op), r, old, v)
	fc.emit(bytecode.OpStore, 0, l, r, 0)
	return reg(r)
}

func (fc *fnCompiler) coalesceSet(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar:
	case ir.OpDim:
		fc.unsupported("??= on array element", n.Line)
	default:
		fc.unsupportedNode(target)
	}
	l := fc.varLoc(target.Var)
	fc.at(target.Line)
	res := fc.alloc()
	mark := fc.top
	keep, assign, end := fc.label(), fc.label(), fc.label()
	set := fc.alloc()
	fc.emit(bytecode.OpIsset, 0, set, l, 0)
	fc.branch(set, keep, assign)
	fc.bind(keep)
	fc.emit(bytecode.OpLoad, 1, res, l, 0)
	fc.jump(end)
	fc.top = mark
	fc.bind(assign)
	v := fc.value(n.Kids[1])
	fc.emit(bytecode.OpStore, 0, l, v, 0)
	fc.emit(bytecode.OpMove, 0, res, v, 0)
	fc.top = mark
	fc.bind(end)
	return reg(res)
}

func (fc *fnCompiler) incDec(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar:
	case ir.OpDim:
		fc.unsupported("increment of array element", n.Line)
	default:
		fc.unsupportedNode(target)
	}
	var op bytecode.IncDecOp
	switch {
	case n.Op == ir.OpPreInc && n.Text == "++":
		op = bytecode.PreInc
	case n.Op == ir.OpPreInc && n.Text == "--":
		op = bytecode.PreDec
	case n.Op == ir.OpPostInc && n.Text == "++":
		op = bytecode.PostInc
	case n.Op == ir.OpPostInc && n.Text == "--":
		op = bytecode.PostDec
	default:
		panic(internalError{"increment operator " + n.Text})
	}
	fc.at(target.Line) // PHP compiles the variable, then emits the operation
	r := fc.alloc()
	fc.emit(bytecode.OpIncDec, uint16(op), r, fc.varLoc(target.Var), 0)
	return reg(r)
}

// array builds a literal in a hidden local: ArraySet and ArrayAppend need a
// Location, and the hidden local is unset once the value is loaded.
func (fc *fnCompiler) array(n *ir.Node) operand {
	res := fc.alloc()
	fc.emit(bytecode.OpNewArray, 0, res, 0, 0)
	if len(n.Kids) == 0 {
		return reg(res)
	}
	if fc.hdepth >= fc.hidden {
		panic(internalError{"array nesting deeper than the prescan"})
	}
	l := fc.loc(fc.nvars + fc.hdepth)
	fc.hdepth++
	fc.emit(bytecode.OpStore, 0, l, res, 0)
	mark := fc.top
	for _, it := range n.Kids {
		if it == nil {
			panic(internalError{"empty array item"})
		}
		switch {
		case it.Op == ir.OpSpread:
			fc.unsupported("array spread", it.Line)
		case it.Op != ir.OpItem:
			panic(internalError{"array element " + string(it.Op)})
		case it.Flags&ir.FlagRef != 0:
			fc.unsupported("by-reference array element", it.Line)
		}
		if key := it.Kids[0]; key == nil {
			v := fc.value(it.Kids[1])
			fc.emit(bytecode.OpArrayAppend, 0, l, v, 0)
		} else {
			k := fc.expr(key)
			v := fc.expr(it.Kids[1])
			// ADD_ARRAY_ELEMENT reads its value operand before its key.
			rv := fc.materialize(v)
			rk := fc.materialize(k)
			fc.emit(bytecode.OpArraySet, 0, l, rk, rv)
		}
		fc.top = mark
	}
	fc.emit(bytecode.OpLoad, 0, res, l, 0)
	fc.emit(bytecode.OpUnset, 0, l, 0, 0)
	fc.hdepth--
	return reg(res)
}

// interp joins the parts left to right; a lone part is cast to string.
func (fc *fnCompiler) interp(n *ir.Node) operand {
	if len(n.Kids) == 0 {
		panic(internalError{"empty interpolation"})
	}
	mark := fc.top
	first := n.Kids[0]
	if len(n.Kids) == 1 {
		v := fc.value(first)
		r := fc.allocNot(mark, v)
		fc.emit(bytecode.OpCast, uint16(bytecode.CastString), r, v, 0)
		return reg(r)
	}
	acc := fc.expr(first)
	for _, p := range n.Kids[1:] {
		part := fc.expr(p)
		ra := fc.materialize(acc)
		rp := fc.materialize(part)
		r := fc.allocNot(mark, ra, rp)
		fc.emit(bytecode.OpBinary, uint16(bytecode.Concat), r, ra, rp)
		acc = reg(r)
	}
	return acc
}

// call evaluates every argument into a register before the first Push, so
// short-circuit arguments never leave pushed arguments at a join.
func (fc *fnCompiler) call(n *ir.Node) operand {
	if n.Name == nil {
		panic(internalError{"call without a name"})
	}
	name, index, known := fc.resolve(n.Name)
	if !known {
		return fc.undefinedCall(n)
	}
	for _, a := range n.Args {
		switch {
		case a.Spread:
			fc.unsupported("argument unpacking", positive(a.Value.Line, n.Line))
		case a.Name != "":
			fc.unsupported("named arguments", positive(a.Value.Line, n.Line))
		}
	}
	argc := uint32(len(n.Args))
	if argc > limits.MaxStack {
		fc.unsupported("too many call arguments", n.Line)
	}
	// A unit function called with the wrong number of arguments still gets
	// them all evaluated; the run time then raises ArgumentCountError for
	// too few and ignores extra ones.
	direct := uint32(len(fc.c.unit.Functions[index-1].Params)) == argc
	res := fc.alloc()
	var callee uint32
	if !direct {
		callee = fc.alloc()
		fc.emit(bytecode.OpConst, 0, callee, fc.c.stringConst(name), 0)
	}
	args := make([]uint32, len(n.Args))
	for i, a := range n.Args {
		args[i] = fc.value(a.Value)
	}
	for _, r := range args {
		fc.emit(bytecode.OpPush, 0, r, 0, 0)
	}
	fc.maxStack = max(fc.maxStack, argc)
	// The call itself carries the call's line, as zend_compile_call_common
	// sets it on DO_FCALL; the current line stays at the last argument.
	after := fc.cur
	fc.at(n.Line)
	if direct {
		fc.emit(bytecode.OpCall, 0, res, index, argc)
	} else {
		fc.emit(bytecode.OpCallValue, 0, res, callee, argc)
	}
	fc.cur = after
	fc.top = res - fc.base + 1
	return reg(res)
}

// undefinedCall compiles a call to a name that is neither a unit function
// nor a builtin. PHP's INIT_FCALL_BY_NAME throws "Call to undefined function"
// before any argument is evaluated, so the arguments are not compiled at
// all: CallValue with no arguments reports the name at the call's line.
// Stage 1 declares every function before the file runs (no conditional
// declarations, include or eval), so the name cannot become defined later.
func (fc *fnCompiler) undefinedCall(n *ir.Node) operand {
	for _, name := range []string{n.Name.Resolved, n.Name.Fallback} {
		if name != "" && isBuiltin(name) {
			fc.unsupported("builtin function "+name+"()", n.Line)
		}
	}
	res := fc.alloc()
	callee := fc.alloc()
	fc.emit(bytecode.OpConst, 0, callee, fc.c.stringConst(n.Name.Resolved), 0)
	fc.emit(bytecode.OpCallValue, 0, res, callee, 0)
	fc.top = res - fc.base + 1
	return reg(res)
}

// resolve finds a hoisted function of the unit by the resolved name, then
// by the global fallback, case-insensitively. name is the spelling a
// CallValue passes to the run time.
func (fc *fnCompiler) resolve(n *ir.Name) (name string, index uint32, ok bool) {
	if i, found := fc.c.funcIndex[strings.ToLower(n.Resolved)]; found {
		return n.Resolved, i, true
	}
	if n.Fallback != "" {
		if i, found := fc.c.funcIndex[strings.ToLower(n.Fallback)]; found {
			return n.Fallback, i, true
		}
	}
	return n.Resolved, 0, false
}

func (fc *fnCompiler) exit(n *ir.Node) operand {
	switch len(n.Args) {
	case 0:
		fc.emit(bytecode.OpExit, 0, 0, 0, 0)
	case 1:
		a := n.Args[0]
		switch {
		case a.Spread:
			fc.unsupported("argument unpacking", n.Line)
		case a.Name != "":
			fc.unsupported("named arguments", n.Line)
		}
		v := fc.value(a.Value)
		fc.emit(bytecode.OpExit, 1, v, 0, 0)
	default:
		fc.unsupported(fmt.Sprintf("exit with %d arguments", len(n.Args)), n.Line)
	}
	// The value of an exit expression is never produced; code reading this
	// register is unreachable.
	return reg(fc.alloc())
}
