package vm

import (
	"fmt"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpname"
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

// discard compiles an expression whose value is unused. A bare variable
// emits nothing: zend_do_free drops an unused CV without reading it, so
// "$u;" and the discarded parts of a for loop never warn.
func (fc *fnCompiler) discard(n *ir.Node) {
	mark := fc.top
	switch {
	case n != nil && n.Op == ir.OpVar:
		fc.c.line = n.Line
		fc.at(n.Line)
		if _, ok := fc.vars[n.Var]; !ok {
			panic(internalError{"variable not in the function's table"})
		}
	case n != nil && n.Op == ir.OpAssign:
		fc.assign(n, false)
	default:
		fc.value(n)
	}
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
		return fc.assign(n, true)
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
	if n.Text == "." {
		// zend_compile_binary_op gives CONCAT a constant array operand
		// through a CAST emitted after both operands, left first: its
		// "Array to string conversion" comes before CONCAT reads a plain
		// variable operand.
		for i, o := range []*operand{&a, &b} {
			if constArray(n.Kids[i]) {
				r := fc.materialize(*o)
				fc.emit(bytecode.OpCast, uint16(bytecode.CastString), r, r, 0)
				*o = reg(r)
			}
		}
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
			// FETCH_DIM_IS for every level but the last, then
			// ISSET_ISEMPTY_DIM_OBJ, in the order of any element read.
			rb, rk := fc.readChain(k)
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
// goes through, or reports why the base is outside this stage.
func (fc *fnCompiler) plainBase(base *ir.Node, line int) uint32 {
	if base == nil {
		panic(internalError{"element access without a base"})
	}
	switch base.Op {
	case ir.OpVar:
		fc.at(base.Line)
		return fc.varLoc(base.Var)
	case ir.OpCall, ir.OpMethodCall, ir.OpStaticCall, ir.OpNullsafeMC, ir.OpBinary:
		fc.unsupported("array write through a non-variable", line)
	}
	fc.unsupportedNode(base)
	return 0
}

// keyKind says how an element target's key reaches its instruction.
type keyKind uint8

const (
	keyNone  keyKind = iota // $a[]
	keyConst                // a literal, loaded right before its instruction
	keyVar                  // a plain variable, read by the instruction itself
	keyReg                  // any other expression, evaluated into a register
)

// dimKey is one level of an element target with its key evaluated. line is
// the line its instruction carries: PHP creates the opline of each level
// when the level is compiled, on the line reached after its key.
type dimKey struct {
	kind keyKind
	lit  *ir.Literal
	cv   int
	reg  uint32
	line int
	dim  int // line of the level's own node, for a compile error
}

// dimTarget is an element target $base[k1]...[kn], outermost level first.
type dimTarget struct {
	base *ir.Var
	loc  uint32
	keys []dimKey
}

// dimTarget compiles the keys of an element target, as
// zend_delayed_compile_dim does: the key expressions are evaluated in source
// order, each once, before any level is fetched. A literal is not loaded and
// a plain variable is not read here: the instruction of its level does that,
// as PHP's FETCH_DIM and ASSIGN_DIM read CONST and CV operands when they run.
// The levels are walked with a loop, so the nesting depth costs no Go stack.
func (fc *fnCompiler) dimTarget(n *ir.Node, line int) dimTarget {
	var levels []*ir.Node
	base := n
	for base != nil && base.Op == ir.OpDim {
		levels = append(levels, base)
		base = base.Kids[0]
	}
	t := dimTarget{loc: fc.plainBase(base, line), base: base.Var, keys: make([]dimKey, 0, len(levels))}
	for i := len(levels) - 1; i >= 0; i-- {
		d := levels[i]
		k := dimKey{dim: d.Line}
		switch key := d.Kids[1]; {
		case key == nil:
		case key.Op == ir.OpLiteral:
			fc.c.line = key.Line
			fc.at(key.Line)
			k.kind, k.lit = keyConst, key.Value
		default:
			if o := fc.expr(key); o.cv >= 0 && !o.quiet {
				k.kind, k.cv = keyVar, o.cv
			} else {
				k.kind, k.reg = keyReg, fc.materialize(o)
			}
		}
		k.line = fc.cur
		t.keys = append(t.keys, k)
	}
	return t
}

// keyOperand returns the operand of a key: a Location for a plain variable,
// otherwise a Value register, which for a literal is loaded here.
func (fc *fnCompiler) keyOperand(k dimKey) (r uint32, isLocation bool) {
	switch k.kind {
	case keyVar:
		return fc.loc(k.cv), true
	case keyConst:
		r = fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.literal(k.lit), 0)
		return r, false
	case keyReg:
		return k.reg, false
	}
	panic(internalError{"element target without a key"})
}

// fetchPath emits a FetchDim for each of the first n levels of t and returns
// the Location the next level is reached through: the base variable when n
// is zero, else the element register, which every level reuses. use is the
// FetchFlags use of the last of them; the others are containers of the next.
func (fc *fnCompiler) fetchPath(t dimTarget, n int, mode, use bytecode.FetchFlags) uint32 {
	container := t.loc
	for i, k := range t.keys[:n] {
		flags := mode
		if i == n-1 {
			flags |= use
		}
		fc.cur = k.line
		mark := fc.top
		if k.kind == keyNone {
			fc.emit(bytecode.OpFetchDim, uint16(flags|bytecode.FetchAppend), elemReg, container, 0)
		} else if rk, isLoc := fc.keyOperand(k); isLoc {
			fc.emit(bytecode.OpFetchDim, uint16(flags|bytecode.FetchKeyLocation), elemReg, container, rk)
		} else {
			fc.emit(bytecode.OpFetchDim, uint16(flags), elemReg, container, rk)
		}
		if k.kind == keyConst {
			fc.top = mark // the literal's register is free again
		}
		container = elemReg
	}
	return container
}

// writeDim emits "$target = v": FETCH_DIM_W for every level but the last,
// then ASSIGN_DIM, which carries the line of the last level. A value that is
// a plain variable is passed as its Location, since ASSIGN_DIM's OP_DATA
// reads such a CV itself, after the key is converted; self marks the one
// exception, a value that is the base variable ($a[k] = $a, and the same
// with ??= and op=), which PHP copies (QM_ASSIGN) before the levels are
// fetched. used asks for the value of the assignment expression: an
// element write names a result register, because the result of a write to a
// string offset is the byte written, not the value.
func (fc *fnCompiler) writeDim(t dimTarget, v operand, self, used bool) operand {
	last := t.keys[len(t.keys)-1]
	fc.cur = last.line
	var flags bytecode.ArrayWriteFlags
	var rv uint32
	if v.cv >= 0 && !v.quiet && !self {
		flags |= bytecode.ValueLocation
		rv = fc.loc(v.cv)
	} else {
		rv = fc.materialize(v)
	}
	container := fc.fetchPath(t, len(t.keys)-1, bytecode.FetchWrite, 0)
	fc.cur = last.line
	if last.kind == keyNone {
		fc.emit(bytecode.OpArrayAppend, uint16(flags), container, rv, 0)
		if flags&bytecode.ValueLocation == 0 {
			return reg(rv)
		}
		// The appended value, read back quietly: the write cannot change a
		// variable other than the container, and an undefined one is null.
		// A discarded value gets a register no instruction writes, which
		// fails verification if it were ever read.
		r := fc.alloc()
		if used {
			fc.emit(bytecode.OpLoad, 1, r, rv, 0)
		}
		return reg(r)
	}
	rk, isLoc := fc.keyOperand(last)
	if isLoc {
		flags |= bytecode.KeyLocation
	}
	if !used {
		fc.emit(bytecode.OpArraySet, uint16(flags), container, rk, rv)
		if flags&bytecode.ValueLocation != 0 {
			return reg(fc.alloc())
		}
		return reg(rv)
	}
	res := rv
	if flags&bytecode.ValueLocation != 0 {
		res = fc.alloc()
	}
	aux, ok := bytecode.ArraySetAux(flags, res)
	if !ok {
		fc.unsupported("expression too large", last.line)
	}
	fc.emit(bytecode.OpArraySet, aux, container, rk, rv)
	return reg(res)
}

// readChain compiles the base and the keys of the element read n, and the
// fetch of every level but the last; it returns the register that holds the
// last container and the one that holds the last key. PHP delays the fetches
// of a chain (zend_delayed_compile_dim): a base that is not a plain variable
// is evaluated first, then the key expressions in source order, and only then
// is the base variable read and each level fetched, a plain variable key
// being read by its own level. Each fetch carries the line reached after its
// key. The levels are walked with a loop, so depth costs no Go stack.
func (fc *fnCompiler) readChain(n *ir.Node) (container, key uint32) {
	var levels []*ir.Node
	base := n
	for base != nil && base.Op == ir.OpDim {
		if base.Kids[1] == nil {
			panic(internalError{"read of []"})
		}
		levels = append(levels, base)
		base = base.Kids[0]
	}
	b := fc.expr(base)
	type level struct {
		key   operand
		lit   *ir.Literal
		line  int
		quiet uint16
	}
	keys := make([]level, len(levels))
	for i := len(levels) - 1; i >= 0; i-- {
		d := levels[i]
		k := &keys[len(levels)-1-i]
		k.quiet = uint16(boolInt(d.Flags&ir.FlagQuiet != 0))
		if kn := d.Kids[1]; kn.Op == ir.OpLiteral && len(levels) > 1 {
			// A literal of a chain is loaded right before its level, so a
			// long chain needs no register per level.
			fc.c.line = kn.Line
			fc.at(kn.Line)
			k.lit = kn.Value
		} else if k.key = fc.expr(kn); k.key.cv < 0 {
			k.key = reg(fc.materialize(k.key))
		}
		k.line = fc.cur
	}
	fc.cur = keys[0].line
	container = fc.materialize(b)
	if len(keys) == 1 {
		return container, fc.materialize(keys[0].key)
	}
	// Two registers take turns holding the element of a level.
	t := [2]uint32{fc.alloc(), fc.alloc()}
	for i, k := range keys {
		fc.cur = k.line
		inner := fc.top
		if k.lit != nil {
			key = fc.alloc()
			fc.emit(bytecode.OpConst, 0, key, fc.c.literal(k.lit), 0)
		} else {
			key = fc.materialize(k.key)
		}
		if i == len(keys)-1 {
			return container, key
		}
		dest := t[i%2]
		fc.emit(bytecode.OpArrayGet, k.quiet, dest, container, key)
		container = dest
		fc.top = inner
	}
	panic(internalError{"element read without a level"})
}

func (fc *fnCompiler) dimRead(n *ir.Node) operand {
	mark := fc.top
	rb, rk := fc.readChain(n)
	r := fc.allocNot(mark, rb, rk)
	fc.emit(bytecode.OpArrayGet, uint16(boolInt(n.Flags&ir.FlagQuiet != 0)), r, rb, rk)
	return reg(r)
}

// assign compiles "=". used is false when the value of the assignment is
// discarded, which spares the read-back of an element write's value.
func (fc *fnCompiler) assign(n *ir.Node, used bool) operand {
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
		return fc.assignDim(n, used)
	}
	fc.unsupportedNode(target)
	return operand{}
}

// assignDim compiles an element assignment of any depth: the keys in source
// order, then the value, then the write (writeDim). The current line ends
// where the value left it.
func (fc *fnCompiler) assignDim(n *ir.Node, used bool) operand {
	target, value := n.Kids[0], n.Kids[1]
	t := fc.dimTarget(target, n.Line)
	v := fc.expr(value)
	after := fc.cur
	r := fc.writeDim(t, v, value.Op == ir.OpVar && value.Var == t.base, used)
	fc.cur = after
	return r
}

// assignOp compiles $x op= e: e first, then $x is read, as PHP's ASSIGN_OP.
func (fc *fnCompiler) assignOp(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar, ir.OpDim:
	default:
		fc.unsupportedNode(target)
	}
	op, ok := binaryOps[n.Text]
	if !ok || n.Text == ">" || n.Text == ">=" {
		panic(internalError{"assignment operator " + n.Text + "="})
	}
	if target.Op == ir.OpDim {
		return fc.assignOpDim(n, op)
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

// assignOpDim compiles $a[k]...[kn] op= e, PHP's ASSIGN_DIM_OP: the keys,
// then e, then every level is fetched for read and write (a missing key
// warns and becomes null), then a plain variable e is read, then the
// element is combined and stored. The fetch of the last level is marked as
// an assign-op's, which decides what PHP says about a string or scalar
// container.
func (fc *fnCompiler) assignOpDim(n *ir.Node, op bytecode.BinaryOp) operand {
	mark := fc.top
	t := fc.dimTarget(n.Kids[0], n.Line)
	v := fc.expr(n.Kids[1])
	after := fc.cur
	if value := n.Kids[1]; value.Op == ir.OpVar && value.Var == t.base {
		// $a[k] op= $a: PHP copies the base variable before the levels are
		// fetched, as it does for "=".
		fc.cur = t.keys[len(t.keys)-1].line
		v = reg(fc.materialize(v))
	}
	e := fc.fetchPath(t, len(t.keys), bytecode.FetchReadWrite, bytecode.FetchForAssignOp)
	rv := fc.materialize(v)
	old := fc.alloc()
	fc.emit(bytecode.OpLoad, 1, old, e, 0)
	r := fc.allocNot(mark, rv, old)
	fc.emit(bytecode.OpBinary, uint16(op), r, old, rv)
	fc.emit(bytecode.OpStore, 0, e, r, 0)
	fc.cur = after
	return reg(r)
}

// coalesceSetDim compiles $a[k]...[kn] ??= e as zend_compile_assign_coalesce
// does: the target is read quietly (FETCH_DIM_IS at each level), and only
// when that is null is e evaluated and the target written (FETCH_DIM_W and
// ASSIGN_DIM). The key expressions are evaluated once, for both passes;
// a key that is a plain variable is read by each pass, as PHP memoizes
// compiled expressions but reads a CV wherever it is used.
func (fc *fnCompiler) coalesceSetDim(n *ir.Node) operand {
	target, value := n.Kids[0], n.Kids[1]
	res := fc.alloc()
	mark := fc.top
	t := fc.dimTarget(target, n.Line)
	for _, k := range t.keys {
		if k.kind == keyNone {
			// PHP rejects the file when it compiles the read.
			panic(&CompileError{Message: "Cannot use [] for reading", File: fc.c.unit.File, Line: positive(k.dim, n.Line)})
		}
	}
	keys := fc.top
	keep, assign, end := fc.label(), fc.label(), fc.label()
	// Two registers take turns holding the container and the element.
	cur, other := fc.alloc(), fc.alloc()
	fc.cur = t.keys[0].line
	fc.emit(bytecode.OpLoad, 1, cur, t.loc, 0)
	for _, k := range t.keys {
		fc.cur = k.line
		inner := fc.top
		rk, isLoc := fc.keyOperand(k)
		if isLoc {
			r := fc.alloc()
			fc.emit(bytecode.OpLoad, 0, r, rk, 0)
			rk = r
		}
		fc.emit(bytecode.OpArrayGet, 1, other, cur, rk)
		cur, other = other, cur
		fc.top = inner
	}
	null := fc.alloc()
	fc.emit(bytecode.OpConst, 0, null, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	set := fc.alloc()
	fc.emit(bytecode.OpBinary, uint16(bytecode.NotIdentical), set, cur, null)
	fc.branch(set, keep, assign)
	fc.bind(keep)
	fc.emit(bytecode.OpMove, 0, res, cur, 0)
	fc.jump(end)
	fc.top = keys
	fc.bind(assign)
	v := fc.expr(value)
	after := fc.cur
	fc.emit(bytecode.OpMove, 0, res, fc.materialize(fc.writeDim(t, v, value.Op == ir.OpVar && value.Var == t.base, true)), 0)
	fc.cur = after
	fc.top = mark
	fc.bind(end)
	return reg(res)
}

func (fc *fnCompiler) coalesceSet(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar:
	case ir.OpDim:
		return fc.coalesceSetDim(n)
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
	// As for "=", the assignment and a plain variable it reads carry the
	// target's line.
	ve := fc.expr(n.Kids[1])
	fc.at(target.Line)
	v := fc.materialize(ve)
	fc.emit(bytecode.OpStore, 0, l, v, 0)
	fc.emit(bytecode.OpMove, 0, res, v, 0)
	fc.top = mark
	fc.bind(end)
	return reg(res)
}

func (fc *fnCompiler) incDec(n *ir.Node) operand {
	target := n.Kids[0]
	switch target.Op {
	case ir.OpVar, ir.OpDim:
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
	if target.Op == ir.OpDim {
		// FETCH_DIM_RW for every level, the last one marked as an
		// increment's, then the operation on the element, on the line of
		// the last key.
		mark := fc.top
		t := fc.dimTarget(target, n.Line)
		e := fc.fetchPath(t, len(t.keys), bytecode.FetchReadWrite, bytecode.FetchForIncDec)
		fc.top = mark
		r := fc.alloc()
		fc.emit(bytecode.OpIncDec, uint16(op), r, e, 0)
		return reg(r)
	}
	fc.at(target.Line) // PHP compiles the variable, then emits the operation
	r := fc.alloc()
	fc.emit(bytecode.OpIncDec, uint16(op), r, fc.varLoc(target.Var), 0)
	return reg(r)
}

// constArray reports whether n is an array literal PHP evaluates at compile
// time (zend_try_ct_eval_array): every key and value a literal or such an
// array, no spread and no reference. The walk keeps its own stack, so the
// nesting depth costs no Go stack.
func constArray(n *ir.Node) bool {
	if n == nil || n.Op != ir.OpArray {
		return false
	}
	for stack := []*ir.Node{n}; len(stack) > 0; {
		a := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, it := range a.Kids {
			if it == nil || it.Op != ir.OpItem || it.Flags&ir.FlagRef != 0 {
				return false
			}
			if k := it.Kids[0]; k != nil && k.Op != ir.OpLiteral {
				return false
			}
			switch v := it.Kids[1]; {
			case v == nil:
				return false
			case v.Op == ir.OpArray:
				stack = append(stack, v)
			case v.Op != ir.OpLiteral:
				return false
			}
		}
	}
	return true
}

// array builds a literal in its result register: NewArray, then one
// ArrayAdd per element (INIT_ARRAY / ADD_ARRAY_ELEMENT), each value read
// before its key. A key that is a plain variable is passed as its Location
// so that an undefined one is the key "" with only its warning, as
// ADD_ARRAY_ELEMENT treats an undefined CV.
func (fc *fnCompiler) array(n *ir.Node) operand {
	res := fc.alloc()
	fc.emit(bytecode.OpNewArray, 0, res, 0, 0)
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
			fc.emit(bytecode.OpArrayAdd, 0, res, v, 0)
		} else {
			k := fc.expr(key)
			v := fc.expr(it.Kids[1])
			rv := fc.materialize(v)
			if k.cv >= 0 && !k.quiet {
				fc.emit(bytecode.OpArrayAdd, uint16(bytecode.AddKey|bytecode.AddKeyLocation), res, rv, fc.loc(k.cv))
			} else {
				fc.emit(bytecode.OpArrayAdd, uint16(bytecode.AddKey), res, rv, fc.materialize(k))
			}
		}
		fc.top = mark
	}
	return reg(res)
}

// interpPart is one part of an interpolated string after adjacent literals
// are merged and empty ones dropped: n is nil for a literal.
type interpPart struct {
	n    *ir.Node
	text string
	line int
}

func interpParts(kids []*ir.Node) []interpPart {
	var out []interpPart
	for _, k := range kids {
		if k == nil {
			panic(internalError{"empty interpolation part"})
		}
		if k.Op != ir.OpLiteral || k.Value == nil || k.Value.Kind != ir.LitString {
			out = append(out, interpPart{n: k, line: k.Line})
			continue
		}
		if last := len(out) - 1; last >= 0 && out[last].n == nil {
			out[last].text += k.Value.Str
			out[last].line = positive(k.Line, out[last].line)
		} else if k.Value.Str != "" {
			out = append(out, interpPart{text: k.Value.Str, line: k.Line})
		}
	}
	return out
}

// interp joins the parts of an interpolated string. One part is cast to
// string. Two parts are PHP's FAST_CONCAT, which reads both operands when it
// runs, so a variable part is read after the other part is evaluated. Three
// or more are a rope (ROPE_INIT, ROPE_ADD, ROPE_END) that converts each part
// as soon as it is compiled: the first part is converted before the second
// is evaluated, and each later part by the Concat right after it.
func (fc *fnCompiler) interp(n *ir.Node) operand {
	parts := interpParts(n.Kids)
	mark := fc.top
	part := func(p interpPart) operand {
		if p.n != nil {
			return fc.expr(p.n)
		}
		fc.at(p.line)
		r := fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.stringConst(p.text), 0)
		return reg(r)
	}
	switch len(parts) {
	case 0:
		panic(internalError{"empty interpolation"})
	case 1:
		v := fc.materialize(part(parts[0]))
		r := fc.allocNot(mark, v)
		fc.emit(bytecode.OpCast, uint16(bytecode.CastString), r, v, 0)
		return reg(r)
	}
	acc := part(parts[0])
	if len(parts) > 2 && parts[0].n != nil {
		v := fc.materialize(acc)
		r := fc.allocNot(mark, v)
		fc.emit(bytecode.OpCast, uint16(bytecode.CastString), r, v, 0)
		acc = reg(r)
	}
	for _, p := range parts[1:] {
		next := part(p)
		ra := fc.materialize(acc)
		rp := fc.materialize(next)
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
		if name != "" && fc.c.host != nil && fc.c.host(phpname.Lower(name)) {
			fc.unsupported("calling host function "+name+"()", n.Line)
		}
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
	if i, found := fc.c.funcIndex[phpname.Lower(n.Resolved)]; found {
		return n.Resolved, i, true
	}
	if n.Fallback != "" {
		if i, found := fc.c.funcIndex[phpname.Lower(n.Fallback)]; found {
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
