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
//	[P+L, P+L+T)    Value temporaries, allocated as a stack per statement
//	[P+L+T]         one Location register for elements, only in a function
//	                that writes through an element (FetchDim)
//
// L counts the PHP variables, in Func.Vars order.
type fnCompiler struct {
	c *compiler
	f *ir.Func

	vars    map[*ir.Var]int
	nvars   int
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
	// pinned keeps cur where it is: the code of a parameter default carries
	// the parameter's line whatever lines its expression spans.
	pinned bool

	loops []loop
	// elemPCs are the instructions that name the element register, by the
	// placeholder elemReg until finish knows where the temporaries end.
	elemPCs []int
}

// elemReg stands for the function's element Location register in emitted
// instructions; finish replaces it. No register, constant or jump target
// has this value.
const elemReg = ^uint32(0)

type fixup struct {
	pc    int
	field int // 0 = A, 1 = B, 2 = C
	label int
}

// loop is a break/continue target: a loop or a switch, as HIR counts them.
// A foreach also has its iterator register, which a break or continue to an
// outer loop drops on the way out, as PHP emits FE_FREE.
type loop struct {
	isSwitch bool
	brk      int
	cont     int
	iter     uint32
	hasIter  bool
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
			if ce, ok := r.(*CompileError); ok {
				err = ce
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
	locals := fc.nvars
	if uint64(locals) > uint64(limits.MaxLocals) {
		fc.unsupported("too many variables", f.Line)
	}
	fc.base = uint32(fc.params + locals)
	fc.cur = positive(f.Line, 1)
	fc.receive()
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
	if len(fc.elemPCs) > 0 && uint64(fc.base)+uint64(fc.maxTop)+1 > uint64(limits.MaxRegisters) {
		fc.unsupported("expression too large", f.Line)
	}
	return fc.finish(locals), nil
}

// receive emits the part of the prologue that receives the parameters, PHP's
// RECV and RECV_INIT, each on its parameter's line: "Store Lk, r_i", and for
// a parameter with a default "ParamDefault r_i, <the Store>" and the code
// that puts the default in r_i before it. The default is evaluated in the
// callee, on every call that omits the argument, as RECV_INIT evaluates a
// constant expression.
func (fc *fnCompiler) receive() {
	for i, p := range fc.f.Params {
		fc.cur = positive(p.Line, fc.f.Line)
		if p.Default != nil {
			passed := fc.label()
			pc := fc.emit(bytecode.OpParamDefault, 0, uint32(i), 0, 0)
			fc.fixups = append(fc.fixups, fixup{pc, 1, passed})
			mark := fc.top
			fc.pinned = true
			if d := p.Default; d.Op == ir.OpLiteral {
				fc.c.line = d.Line
				fc.emit(bytecode.OpConst, 0, uint32(i), fc.c.literal(d.Value), 0)
			} else {
				fc.emit(bytecode.OpMove, 0, uint32(i), fc.value(d), 0)
				if !constArray(d) && fc.defaultMayBeInterned(d) {
					fc.unsupported("parameter default whose string value PHP may intern", fc.cur)
				}
			}
			fc.pinned = false
			fc.top = mark
			fc.bind(passed)
		}
		fc.emit(bytecode.OpStore, 0, fc.varLoc(p.Var), uint32(i), 0)
	}
}

// defaultMayBeInterned reports whether a default PHP evaluates at run time
// (one the lowering did not fold to a literal) may yield a string whose
// interning the VM cannot tell. ZEND_RECV_INIT keeps an evaluated default
// for later calls only when it is not refcounted, so a default that warns
// does so again on each call, or only on the first. The VM keeps a string
// of at most one byte (vmDefaultCacheable), which is what PHP interns for a
// string offset, a digit converted from an integer, a concatenation with an
// empty side and a one-byte literal written without an escape. It cannot
// tell three cases, which are refused instead:
//
//   - a string converted from a float ("1" is not interned) or from an
//     array ("Array" is): a concatenation, interpolation or string cast
//     with an operand that may be a float or an array;
//   - a one-byte literal written with an escape ("\n" is not interned):
//     a one-byte string literal that may be the value, when the lines of
//     the default contain a backslash.
//
// Keys of an element read or an array item, and the operands of
// arithmetic, are never the value. The walk keeps its own stack.
func (fc *fnCompiler) defaultMayBeInterned(d *ir.Node) bool {
	const (
		value   = iota // the node may be (part of) the default's value
		operand        // converted to a number or bool, or a key
		holder         // the array an element is read from
	)
	type item struct {
		n   *ir.Node
		pos int
	}
	conv, floatOrArray, oneByte := false, false, false
	first, last := d.Line, d.Line
	for stack := []item{{d, value}}; len(stack) > 0; {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := it.n
		if n == nil {
			continue
		}
		if n.Line > 0 {
			first, last = min(first, n.Line), max(last, n.Line)
		}
		kid := value
		switch n.Op {
		case ir.OpLiteral:
			if it.pos == value && n.Value != nil {
				switch {
				case n.Value.Kind == ir.LitFloat:
					floatOrArray = true
				case n.Value.Kind == ir.LitString && len(n.Value.Str) == 1:
					oneByte = true
				}
			}
		case ir.OpArray:
			if it.pos == value {
				floatOrArray = true
			}
			if it.pos == operand {
				kid = operand
			}
		case ir.OpItem:
			if it.pos == operand {
				kid = operand
			}
			stack = append(stack, item{n.Kids[0], operand}, item{n.Kids[1], kid})
			continue
		case ir.OpDim:
			if it.pos == operand {
				stack = append(stack, item{n.Kids[0], operand}, item{n.Kids[1], operand})
			} else {
				stack = append(stack, item{n.Kids[0], holder}, item{n.Kids[1], operand})
			}
			continue
		case ir.OpInterp:
			conv = true
		case ir.OpCast:
			if n.Text == "string" {
				conv = true
			} else {
				kid = operand
				if it.pos == value {
					floatOrArray = true
				}
			}
		case ir.OpBinary:
			if n.Text == "." {
				conv = true
			} else {
				kid = operand
				if it.pos == value {
					floatOrArray = true
				}
			}
		case ir.OpUnary:
			kid = operand
			if it.pos == value {
				floatOrArray = true
			}
		default:
			if it.pos == operand {
				kid = operand
			}
		}
		for _, k := range n.Kids {
			stack = append(stack, item{k, kid})
		}
	}
	if conv && floatOrArray {
		return true
	}
	return oneByte && fc.linesHaveBackslash(first, last)
}

// linesHaveBackslash reports whether source lines first..last contain a
// backslash.
func (fc *fnCompiler) linesHaveBackslash(first, last int) bool {
	line := 1
	for _, b := range fc.c.source {
		if line > last {
			break
		}
		if b == '\\' && line >= first {
			return true
		}
		if b == '\n' {
			line++
		}
	}
	return false
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
		// A hoisted declaration is a CompileError (compiler.compile). A
		// conditional one fails in PHP when it runs, which stage 1 cannot
		// compile anyway.
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
	if line > 0 && !fc.pinned {
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
	if a == elemReg || b == elemReg || cc == elemReg {
		fc.elemPCs = append(fc.elemPCs, len(fc.code)-1)
	}
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

// finish prepends the Locals of the prologue, resolves labels and the
// element register and builds the frame.
func (fc *fnCompiler) finish(locals int) bytecode.Function {
	c, f := fc.c, fc.f
	declLine := uint32(positive(f.Line, 1))
	var code []bytecode.Instruction
	for i := 0; i < locals; i++ {
		code = append(code, bytecode.Instruction{Op: bytecode.OpLocal, A: fc.loc(i), B: uint32(i)})
	}
	plen := len(code)
	elem := fc.base + fc.maxTop
	for _, pc := range fc.elemPCs {
		in := &fc.code[pc]
		for _, field := range []*uint32{&in.A, &in.B, &in.C} {
			if *field == elemReg {
				*field = elem
			}
		}
	}
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
	if len(fc.elemPCs) > 0 {
		regs = append(regs, bytecode.Location)
	}
	names := make([]uint32, locals)
	for i := range names {
		names[i] = c.stringConst(f.Vars[i].Name)
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
	case ir.OpForeach:
		fc.foreachStmt(n)
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
		// The iterators of the foreach loops left on the way are dropped.
		for _, inner := range fc.loops[len(fc.loops)-depth+1:] {
			if inner.hasIter {
				fc.emit(bytecode.OpDrop, 0, inner.iter, 0, 0)
			}
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
		// FETCH_DIM_UNSET for every level but the last, then UNSET_DIM,
		// which reads a plain variable key itself after it reports an
		// undefined container.
		t := fc.dimTarget(k, k.Line)
		last := t.keys[len(t.keys)-1]
		if last.kind == keyNone {
			panic(internalError{"unset of []"})
		}
		container := fc.fetchPath(t, len(t.keys)-1, bytecode.FetchUnset, 0)
		fc.cur = last.line
		if rk, isLoc := fc.keyOperand(last); isLoc {
			fc.emit(bytecode.OpArrayUnset, uint16(bytecode.KeyLocation), container, rk, 0)
		} else {
			fc.emit(bytecode.OpArrayUnset, 0, container, rk, 0)
		}
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

// foreachStmt compiles a by-value foreach: FE_RESET_R on the subject's line,
// then per element FE_FETCH_R and the assignments of the value and then the
// key. The iterator register holds the array for the whole loop, above the
// temporaries of the body's statements; leaving the loop drops it.
//
//	      <subject -> rs>
//	      IterInit it, rs, end
//	next: IterNext it, <value>, brk
//	      [IterKey <key>, it]
//	      <body>              ; continue jumps to next
//	      Jump next
//	brk:  Drop it             ; break jumps here
//	end:
func (fc *fnCompiler) foreachStmt(n *ir.Node) {
	if n.Flags&ir.FlagRef != 0 {
		fc.unsupported("foreach by reference", n.Line)
	}
	subject, key, val, body := n.Kids[0], n.Kids[1], n.Kids[2], n.Kids[3]
	it := fc.alloc()
	inner := fc.top
	rs := fc.value(subject)
	next, brk, end := fc.label(), fc.label(), fc.label()
	pc := fc.emit(bytecode.OpIterInit, 0, it, rs, 0)
	fc.fixups = append(fc.fixups, fixup{pc, 2, end})
	fc.top = inner
	fc.bind(next)
	fc.iterTarget(val, func(dest uint32, flags bytecode.IterFlags) {
		pc := fc.emit(bytecode.OpIterNext, uint16(flags), it, dest, 0)
		fc.fixups = append(fc.fixups, fixup{pc, 2, brk})
	})
	fc.top = inner
	if key != nil {
		fc.iterTarget(key, func(dest uint32, flags bytecode.IterFlags) {
			fc.emit(bytecode.OpIterKey, uint16(flags), dest, it, 0)
		})
		fc.top = inner
	}
	fc.loops = append(fc.loops, loop{brk: brk, cont: next, iter: it, hasIter: true})
	fc.stmt(body)
	fc.loops = fc.loops[:len(fc.loops)-1]
	fc.jump(next)
	fc.bind(brk)
	fc.emit(bytecode.OpDrop, 0, it, 0, 0)
	fc.bind(end)
}

// iterTarget compiles the write of a fetched element or key to its target.
// A plain variable is assigned by the iterator instruction itself, as
// FE_FETCH_R assigns a CV; an element target receives a temporary through
// the same write as "$target = <temporary>".
func (fc *fnCompiler) iterTarget(target *ir.Node, fetch func(dest uint32, flags bytecode.IterFlags)) {
	if target == nil {
		panic(internalError{"foreach without a target"})
	}
	fc.c.line = target.Line
	switch target.Op {
	case ir.OpVar:
		fetch(fc.varLoc(target.Var), bytecode.IterLocation)
	case ir.OpDim:
		tmp := fc.alloc()
		fetch(tmp, 0)
		t := fc.dimTarget(target, target.Line)
		fc.writeDim(t, reg(tmp), false, false)
	case ir.OpDestructure, ir.OpArray:
		fc.unsupported("list() destructuring", target.Line)
	default:
		fc.unsupportedNode(target)
	}
}
