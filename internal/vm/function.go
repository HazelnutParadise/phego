package vm

import (
	"fmt"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
)

// Frame layout of every compiled function:
//
//	[0, P)          parameter Value registers (bytecode requires them first)
//	[P, P+L)        one Location register per local, bound by the prologue
//	[P+L, ...)      Value temporaries, allocated as a stack per statement
//
// L counts the PHP variables (Func.Vars order) followed by the hidden
// locals that hold array literals under construction, one per nesting level.
type fnCompiler struct {
	c *compiler
	f *ir.Func

	vars    map[*ir.Var]int
	nvars   int
	hidden  int // hidden locals reserved by the prescan
	hdepth  int // hidden locals in use
	params  int
	base    uint32 // first temporary register
	top     uint32 // temporaries in use
	maxTop  uint32
	overReg int // line where the register limit was first exceeded

	code     []bytecode.Instruction
	lines    []uint32
	labels   []int
	fixups   []fixup
	maxStack uint32
	maxLine  int
	// cur is the line the next instruction gets, PHP's CG(zend_lineno): it
	// moves to each statement and expression as compilation reaches it, so
	// an operation carries the line of the last operand compiled for it.
	cur int

	loops []loop
}

type fixup struct {
	pc    int
	field int // 0 = A, 1 = B, 2 = C
	label int
}

// loop is a break/continue target: a loop or a switch, as HIR counts them.
type loop struct {
	isSwitch bool
	brk      int
	cont     int
}

// function compiles one HIR function, returning an UnsupportedError for the
// first unsupported construct in its source order.
func (c *compiler) function(f *ir.Func) (fn bytecode.Function, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ue, ok := r.(*UnsupportedError); ok {
				err = ue
				return
			}
			panic(r)
		}
	}()
	fc := &fnCompiler{c: c, f: f, vars: make(map[*ir.Var]int, len(f.Vars)), nvars: len(f.Vars), params: len(f.Params)}
	for i, v := range f.Vars {
		fc.vars[v] = i
	}
	fc.signature()
	fc.hidden = arrayDepth(f.Body)
	locals := fc.nvars + fc.hidden
	if uint64(locals) > uint64(limits.MaxLocals) {
		fc.unsupported("too many variables", f.Line)
	}
	fc.base = uint32(fc.params + locals)
	fc.cur = positive(f.Line, 1)
	for _, s := range f.Body {
		fc.stmt(s)
	}
	end := int(c.lines)
	if f.Kind != ir.FuncMain {
		end = max(fc.maxLine, f.Line)
	}
	fc.cur = end
	r := fc.alloc()
	fc.emit(bytecode.OpConst, 0, r, c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
	fc.emit(bytecode.OpReturn, 0, r, 0, 0)
	if fc.overReg != 0 {
		fc.unsupported("expression too large", fc.overReg)
	}
	return fc.finish(locals), nil
}

// signature rejects the declaration forms stage 1 does not support, in
// source order: attributes, by-reference return, then each parameter.
func (fc *fnCompiler) signature() {
	f := fc.f
	if f.Kind == ir.FuncMain {
		return
	}
	if len(f.Attributes) > 0 {
		fc.unsupported("function attribute", positive(f.Attributes[0].Line, f.Line))
	}
	if isBuiltin(f.Name) {
		// PHP fails with "Cannot redeclare function name()" at compile time;
		// ir.Lower does not report it, and calling the user function instead
		// would be wrong.
		fc.unsupported("declaration of builtin function "+f.Name+"()", f.Line)
	}
	if f.ByRef {
		fc.unsupported("return by reference", f.Line)
	}
	for _, p := range f.Params {
		line := positive(p.Line, f.Line)
		switch {
		case len(p.Attributes) > 0:
			fc.unsupported("parameter attribute", positive(p.Attributes[0].Line, line))
		case p.Type != nil:
			fc.unsupported("typed parameter", line)
		case p.ByRef:
			fc.unsupported("by-reference parameter", line)
		case p.Variadic:
			fc.unsupported("variadic parameter", line)
		case p.Default != nil:
			fc.unsupported("parameter default value", line)
		case len(p.Promoted) > 0:
			fc.unsupported("promoted parameter", line)
		case p.Var == nil:
			panic(internalError{"parameter without a variable"})
		}
	}
	if f.Return != nil {
		fc.unsupported("return type", f.Line)
	}
}

func positive(line, fallback int) int {
	if line > 0 {
		return line
	}
	return fallback
}

// arrayDepth is the deepest nesting of non-empty array literals, which
// bounds the hidden locals alive at the same time.
func arrayDepth(nodes []*ir.Node) int {
	d := 0
	for _, n := range nodes {
		d = max(d, nodeArrayDepth(n))
	}
	return d
}

func nodeArrayDepth(n *ir.Node) int {
	if n == nil {
		return 0
	}
	d := arrayDepth(n.Kids)
	for _, a := range n.Args {
		d = max(d, nodeArrayDepth(a.Value))
	}
	if n.Op == ir.OpArray && len(n.Kids) > 0 && n.Flags&ir.FlagWrite == 0 {
		d++
	}
	return d
}

func (fc *fnCompiler) unsupported(feature string, line int) {
	panic(&UnsupportedError{Feature: feature, File: fc.c.unit.File, Line: positive(line, positive(fc.c.line, 1))})
}

// local returns the Location register bound to local index i.
func (fc *fnCompiler) loc(i int) uint32 { return uint32(fc.params + i) }

func (fc *fnCompiler) varLoc(v *ir.Var) uint32 {
	i, ok := fc.vars[v]
	if !ok {
		panic(internalError{"variable not in the function's table"})
	}
	return fc.loc(i)
}

// at moves the current line to a node's line, as compiling it does in PHP.
func (fc *fnCompiler) at(line int) {
	if line > 0 {
		fc.cur = line
	}
}

// alloc returns a fresh temporary register.
func (fc *fnCompiler) alloc() uint32 {
	r := fc.base + fc.top
	fc.top++
	if fc.top > fc.maxTop {
		fc.maxTop = fc.top
		if uint64(fc.base)+uint64(fc.maxTop) > uint64(limits.MaxRegisters) && fc.overReg == 0 {
			fc.overReg = fc.cur
		}
	}
	return r
}

// allocNot releases the temporaries above mark and allocates a destination
// distinct from the given source registers, so no instruction both reads
// and overwrites the same register.
func (fc *fnCompiler) allocNot(mark uint32, srcs ...uint32) uint32 {
	fc.top = mark
	for {
		r := fc.alloc()
		clash := false
		for _, s := range srcs {
			if s == r {
				clash = true
			}
		}
		if !clash {
			return r
		}
	}
}

func (fc *fnCompiler) label() int {
	fc.labels = append(fc.labels, -1)
	return len(fc.labels) - 1
}

func (fc *fnCompiler) bind(l int) { fc.labels[l] = len(fc.code) }

// emit appends an instruction on the current line.
func (fc *fnCompiler) emit(op bytecode.Opcode, aux uint16, a, b, cc uint32) int {
	line := fc.cur
	fc.maxLine = max(fc.maxLine, line)
	fc.code = append(fc.code, bytecode.Instruction{Op: op, Aux: aux, A: a, B: b, C: cc})
	fc.lines = append(fc.lines, uint32(line))
	return len(fc.code) - 1
}

func (fc *fnCompiler) jump(l int) {
	pc := fc.emit(bytecode.OpJump, 0, 0, 0, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 0, l})
}

func (fc *fnCompiler) branch(cond uint32, yes, no int) {
	pc := fc.emit(bytecode.OpBranch, 0, cond, 0, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 1, yes}, fixup{pc, 2, no})
}

// finish prepends the prologue, resolves labels and builds the frame.
func (fc *fnCompiler) finish(locals int) bytecode.Function {
	c, f := fc.c, fc.f
	declLine := uint32(positive(f.Line, 1))
	var code []bytecode.Instruction
	for i := 0; i < locals; i++ {
		code = append(code, bytecode.Instruction{Op: bytecode.OpLocal, A: fc.loc(i), B: uint32(i)})
	}
	for i, p := range f.Params {
		code = append(code, bytecode.Instruction{Op: bytecode.OpStore, A: fc.varLoc(p.Var), B: uint32(i)})
	}
	plen := len(code)
	positions := make([]bytecode.Position, 0, plen+len(fc.code))
	for range code {
		positions = append(positions, bytecode.Position{Source: 1, Line: declLine})
	}
	for _, fx := range fc.fixups {
		target := fc.labels[fx.label]
		if target < 0 {
			panic(internalError{"unbound label"})
		}
		t := uint32(target + plen)
		switch fx.field {
		case 0:
			fc.code[fx.pc].A = t
		case 1:
			fc.code[fx.pc].B = t
		default:
			fc.code[fx.pc].C = t
		}
	}
	code = append(code, fc.code...)
	for _, l := range fc.lines {
		positions = append(positions, bytecode.Position{Source: 1, Line: l})
	}
	regs := make([]bytecode.RegisterKind, int(fc.base)+int(fc.maxTop))
	for i := range regs {
		regs[i] = bytecode.Value
	}
	for i := 0; i < locals; i++ {
		regs[fc.loc(i)] = bytecode.Location
	}
	names := make([]uint32, locals)
	for i := range names {
		if i < fc.nvars {
			names[i] = c.stringConst(f.Vars[i].Name)
		} else {
			names[i] = c.stringConst("")
		}
	}
	name := f.Name
	if f.Kind == ir.FuncMain {
		name = "{main}"
	}
	return bytecode.Function{
		Name:       name,
		Params:     uint32(fc.params),
		Locals:     uint32(locals),
		MaxStack:   fc.maxStack,
		ReturnKind: bytecode.Value,
		Registers:  regs,
		Code:       code,
		Positions:  positions,
		LocalNames: names,
	}
}

// --- statements ---

func (fc *fnCompiler) stmt(n *ir.Node) {
	if n == nil {
		return
	}
	if err := fc.c.ctx.Err(); err != nil {
		panic(ctxError{err})
	}
	fc.c.line = n.Line
	fc.at(n.Line)
	mark := fc.top
	defer func() { fc.top = mark }()
	switch n.Op {
	case ir.OpBlock:
		for _, k := range n.Kids {
			fc.stmt(k)
		}
	case ir.OpExpr, ir.OpVoid:
		fc.discard(n.Kids[0])
	case ir.OpEcho:
		for _, k := range n.Kids {
			r := fc.value(k)
			fc.emit(bytecode.OpOutput, 0, r, 0, 0)
			fc.top = mark
		}
	case ir.OpHTML:
		r := fc.alloc()
		fc.emit(bytecode.OpConst, 0, r, fc.c.literal(n.Value), 0)
		fc.emit(bytecode.OpOutput, 0, r, 0, 0)
	case ir.OpIf:
		fc.ifStmt(n)
	case ir.OpWhile:
		fc.whileStmt(n)
	case ir.OpDo:
		fc.doStmt(n)
	case ir.OpFor:
		fc.forStmt(n)
	case ir.OpSwitch:
		fc.switchStmt(n)
	case ir.OpBreak, ir.OpContinue:
		depth := 1
		if n.Value != nil && n.Value.Kind == ir.LitInt {
			depth = int(n.Value.Int)
		}
		if depth < 1 || depth > len(fc.loops) {
			panic(internalError{fmt.Sprintf("%s %d outside %d loops", n.Op, depth, len(fc.loops))})
		}
		l := fc.loops[len(fc.loops)-depth]
		if n.Op == ir.OpContinue && !l.isSwitch {
			fc.jump(l.cont)
		} else {
			fc.jump(l.brk)
		}
	case ir.OpReturn:
		if n.Flags&ir.FlagRef != 0 {
			fc.unsupported("return by reference", n.Line)
		}
		var r uint32
		if len(n.Kids) > 0 && n.Kids[0] != nil {
			r = fc.value(n.Kids[0])
		} else {
			r = fc.alloc()
			fc.emit(bytecode.OpConst, 0, r, fc.c.literal(&ir.Literal{Kind: ir.LitNull}), 0)
		}
		fc.emit(bytecode.OpReturn, 0, r, 0, 0)
	case ir.OpUnset:
		for _, k := range n.Kids {
			fc.unset(k)
			fc.top = mark
		}
	case ir.OpLabel:
		// A label without goto has no run-time effect.
	default:
		fc.unsupportedNode(n)
	}
}

func (fc *fnCompiler) unset(k *ir.Node) {
	switch k.Op {
	case ir.OpVar:
		fc.at(k.Line)
		fc.emit(bytecode.OpUnset, 0, fc.varLoc(k.Var), 0, 0)
	case ir.OpDim:
		base := k.Kids[0]
		if base != nil && base.Op == ir.OpDim {
			fc.unsupported("nested array unset", k.Line)
		}
		l := fc.plainBase(base, k.Line)
		if k.Kids[1] == nil {
			panic(internalError{"unset of []"})
		}
		r := fc.value(k.Kids[1])
		fc.emit(bytecode.OpArrayUnset, 0, l, r, 0)
	default:
		fc.unsupportedNode(k)
	}
}

func (fc *fnCompiler) ifStmt(n *ir.Node) {
	end := fc.label()
	kids := n.Kids
	i := 0
	for ; i+1 < len(kids); i += 2 {
		cond, body := kids[i], kids[i+1]
		then, next := fc.label(), fc.label()
		mark := fc.top
		r := fc.value(cond)
		fc.branch(r, then, next)
		fc.top = mark
		fc.bind(then)
		fc.stmt(body)
		fc.jump(end)
		fc.bind(next)
	}
	if i < len(kids) {
		fc.stmt(kids[i])
	}
	fc.bind(end)
}

func (fc *fnCompiler) whileStmt(n *ir.Node) {
	cond, body, end := fc.label(), fc.label(), fc.label()
	fc.bind(cond)
	fc.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	mark := fc.top
	r := fc.value(n.Kids[0])
	fc.branch(r, body, end)
	fc.top = mark
	fc.bind(body)
	fc.loops = append(fc.loops, loop{brk: end, cont: cond})
	fc.stmt(n.Kids[1])
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.jump(cond)
	fc.bind(end)
}

func (fc *fnCompiler) doStmt(n *ir.Node) {
	body, cont, end := fc.label(), fc.label(), fc.label()
	fc.bind(body)
	fc.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	fc.loops = append(fc.loops, loop{brk: end, cont: cont})
	fc.stmt(n.Kids[0])
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.bind(cont)
	mark := fc.top
	r := fc.value(n.Kids[1])
	fc.branch(r, body, end)
	fc.top = mark
	fc.bind(end)
}

// forExprs returns the expressions of a for-loop part.
func forExprs(part *ir.Node) []*ir.Node {
	if part == nil {
		return nil
	}
	out := make([]*ir.Node, 0, len(part.Kids))
	for _, e := range part.Kids {
		if e != nil && e.Op == ir.OpVoid {
			e = e.Kids[0]
		}
		out = append(out, e)
	}
	return out
}

func (fc *fnCompiler) forStmt(n *ir.Node) {
	cond, body, step, end := fc.label(), fc.label(), fc.label(), fc.label()
	mark := fc.top
	for _, e := range forExprs(n.Kids[0]) {
		fc.discard(e)
		fc.top = mark
	}
	fc.bind(cond)
	fc.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	if conds := forExprs(n.Kids[1]); len(conds) > 0 {
		for _, e := range conds[:len(conds)-1] {
			fc.discard(e)
			fc.top = mark
		}
		last := conds[len(conds)-1]
		r := fc.value(last)
		fc.branch(r, body, end)
		fc.top = mark
	}
	fc.bind(body)
	fc.loops = append(fc.loops, loop{brk: end, cont: step})
	fc.stmt(n.Kids[3])
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.bind(step)
	for _, e := range forExprs(n.Kids[2]) {
		fc.discard(e)
		fc.top = mark
	}
	fc.jump(cond)
	fc.bind(end)
}

// switchStmt compares the subject with each case label in order using loose
// equality. A subject that is a plain variable is read at each comparison,
// as PHP's IS_EQUAL on a CV does; any other subject is evaluated once.
func (fc *fnCompiler) switchStmt(n *ir.Node) {
	end := fc.label()
	subject := fc.expr(n.Kids[0])
	cases := n.Kids[1:]
	bodies := make([]int, len(cases))
	def := -1
	mark := fc.top
	for i, cs := range cases {
		bodies[i] = fc.label()
		label := cs.Kids[0]
		if label == nil {
			def = i
			continue
		}
		lo := fc.expr(label)
		rs := fc.materialize(subject)
		rl := fc.materialize(lo)
		t := fc.allocNot(mark, rs, rl)
		fc.emit(bytecode.OpBinary, uint16(bytecode.Equal), t, rs, rl)
		next := fc.label()
		fc.branch(t, bodies[i], next)
		fc.bind(next)
		fc.top = mark
	}
	if def >= 0 {
		fc.jump(bodies[def])
	} else {
		fc.jump(end)
	}
	fc.loops = append(fc.loops, loop{isSwitch: true, brk: end})
	for i, cs := range cases {
		fc.bind(bodies[i])
		fc.stmt(cs.Kids[1])
	}
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.bind(end)
}
