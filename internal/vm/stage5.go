package vm

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpconst"
)

// W-17 stage 5: constants, match, empty() and @. REQ-001, REQ-016,
// REQ-025, REQ-026; T-009, T-010, T-029..T-032. Compiled as
// zend_compile_const, zend_compile_const_decl, zend_compile_match,
// zend_compile_isset_or_empty and zend_compile_silence do.

// foldable reports whether PHP folds the constant name at compile time
// (zend_try_ct_eval_const): the name needs no run-time fallback, it is a
// predefined constant and not a deprecated one. A resource or a
// platform-dependent constant is refused with its location instead of
// being guessed.
func (fc *fnCompiler) foldable(name *ir.Name, line int) (phpconst.Constant, bool) {
	if name.Fallback != "" {
		// An unqualified name in a namespace resolves at run time, but
		// the unit is the whole program (no include, eval or define()):
		// when it declares no constant of the namespaced name, the
		// fallback is certain, and a resource or platform-dependent
		// constant behind it is refused here rather than at run time.
		if !fc.c.declaredConsts[phpconst.Key(name.Resolved)] {
			if c, ok := phpconst.Lookup(fc.c.unit.Profile.IntWidth, name.Fallback); ok {
				fc.unsupportedConstant(c, name.Fallback, line)
			}
		}
		return phpconst.Constant{}, false
	}
	c, ok := phpconst.Lookup(fc.c.unit.Profile.IntWidth, phpconst.Key(name.Resolved))
	if !ok {
		return phpconst.Constant{}, false
	}
	fc.unsupportedConstant(c, name.Resolved, line)
	return c, c.Deprecated == ""
}

// unsupportedConstant refuses a predefined constant the VM cannot give a
// value: a resource or a platform-dependent one.
func (fc *fnCompiler) unsupportedConstant(c phpconst.Constant, written string, line int) {
	switch c.Kind {
	case phpconst.Resource:
		fc.unsupported("the constant "+written+" (a resource)", line)
	case phpconst.Platform:
		fc.unsupported("the platform-dependent constant "+written, line)
	}
}

// declaredConstants collects the registered names of every constant the
// unit declares with const: the statements of {main}, since PHP accepts
// const only at the top level (blocks of OpConstDef).
func declaredConstants(unit *ir.Unit) map[string]bool {
	out := map[string]bool{}
	stack := []*ir.Node{}
	for _, s := range unit.Main.Body {
		stack = append(stack, s)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		if n.Op == ir.OpConstDef && n.Name != nil {
			out[phpconst.Key(n.Name.Resolved)] = true
			continue
		}
		if n.Op == ir.OpBlock {
			stack = append(stack, n.Kids...)
		}
	}
	return out
}

// predefined converts a predefined constant to a pool constant.
func (c *compiler) predefined(k phpconst.Constant) uint32 {
	switch k.Kind {
	case phpconst.Null:
		return c.literal(&ir.Literal{Kind: ir.LitNull})
	case phpconst.Bool:
		return c.literal(&ir.Literal{Kind: ir.LitBool, Bool: k.Bool})
	case phpconst.Int:
		return c.literal(&ir.Literal{Kind: ir.LitInt, Int: k.Int})
	case phpconst.Float:
		return c.literal(&ir.Literal{Kind: ir.LitFloat, Float: k.Float})
	case phpconst.String:
		return c.literal(&ir.Literal{Kind: ir.LitString, Str: k.Str})
	}
	panic(internalError{"predefined constant " + k.Name + " has no value"})
}

// constant compiles a constant lookup. A predefined constant becomes its
// value, as PHP substitutes it while compiling; any other name is fetched
// when the instruction runs, with the fallback to the global name for an
// unqualified name in a namespace (IS_CONSTANT_UNQUALIFIED_IN_NAMESPACE).
func (fc *fnCompiler) constant(n *ir.Node) operand {
	if n.Name == nil {
		panic(internalError{"constant lookup without a name"})
	}
	r := fc.alloc()
	if c, ok := fc.foldable(n.Name, n.Line); ok {
		fc.emit(bytecode.OpConst, 0, r, fc.c.predefined(c), 0)
		return reg(r)
	}
	var aux uint16
	if n.Name.Fallback != "" {
		aux = bytecode.FetchConstUnqualified
	}
	fc.emit(bytecode.OpFetchConst, aux, r, fc.c.stringConst(phpconst.Key(n.Name.Resolved)), fc.c.stringConst(n.Name.Resolved))
	return reg(r)
}

// constDecl compiles a const statement: one DeclareConst per constant, each
// value evaluated when the declaration runs (ZEND_DECLARE_CONST evaluates a
// constant expression with zval_update_constant_ex). Every diagnostic of a
// value carries the declaration's line, which is CG(zend_lineno) when PHP
// emits the opline: the statement's line (that of the first constant's
// name), moved to the line of each constant reference the compiler could
// not fold (zend_compile_const_expr_const), in source order, and never
// moved back between the constants of one statement.
func (fc *fnCompiler) constDecl(n *ir.Node) {
	line := positive(n.Kids[0].Line, n.Line)
	for _, item := range n.Kids {
		if item.Op != ir.OpConstDef || item.Name == nil || len(item.Kids) != 1 || item.Kids[0] == nil {
			panic(internalError{"malformed const declaration"})
		}
		line = fc.unfoldedConstantLine(item.Kids[0], line)
		fc.cur = line
		fc.c.line = line
		mark := fc.top
		fc.pinned = true
		v := fc.value(item.Kids[0])
		fc.emit(bytecode.OpDeclareConst, 0, v, fc.c.stringConst(phpconst.Key(item.Name.Resolved)), 0)
		fc.pinned = false
		fc.top = mark
	}
}

// unfoldedConstantLine returns the line of the last constant reference in n
// that is not folded at compile time, in source order, or line when there
// is none. The walk keeps its own stack.
func (fc *fnCompiler) unfoldedConstantLine(n *ir.Node, line int) int {
	stack := []*ir.Node{n}
	for len(stack) > 0 {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if k == nil {
			continue
		}
		if k.Op == ir.OpConst && k.Name != nil {
			if _, ok := fc.foldable(k.Name, k.Line); !ok {
				line = positive(k.Line, line)
			}
		}
		for i := len(k.Kids) - 1; i >= 0; i-- {
			stack = append(stack, k.Kids[i])
		}
		for i := len(k.Args) - 1; i >= 0; i-- {
			stack = append(stack, k.Args[i].Value)
		}
	}
	return line
}

// jumptableCondition reports whether a match condition is an int or string
// PHP knows while compiling (can_match_use_jumptable). The lowering already
// folded every condition PHP folds (ir.evalMatchCondition), so such a
// condition is a literal here.
func (fc *fnCompiler) jumptableCondition(n *ir.Node) bool {
	return n.Op == ir.OpLiteral && n.Value != nil && (n.Value.Kind == ir.LitInt || n.Value.Kind == ir.LitString)
}

// matchExpr compiles a match expression. PHP compiles two forms. With at
// least two conditions that are all int or string literals, ZEND_MATCH
// looks the subject up in a jump table: a variable subject is read once,
// with its undefined warning, before any condition. Otherwise each
// condition is compiled in order and compared with ZEND_CASE_STRICT or
// ZEND_IS_IDENTICAL, which reads a variable subject again at each
// comparison, after the condition was evaluated. Both forms are expressed
// with Identical and Branch; the difference is when the subject variable is
// read. Without a default arm the fall-through is ZEND_MATCH_ERROR, which
// reads an undefined subject variable without a warning and carries the
// subject's line in the jump-table form and the last condition's otherwise.
// The arm bodies follow, each moving its value to the result register.
func (fc *fnCompiler) matchExpr(n *ir.Node) operand {
	res := fc.alloc()
	mark := fc.top
	subject := fc.expr(n.Kids[0])
	arms := n.Kids[1:]
	conds, jumptable := 0, true
	for _, arm := range arms {
		if arm.Op != ir.OpArm || len(arm.Kids) == 0 {
			panic(internalError{"malformed match arm"})
		}
		for _, c := range arm.Kids[1:] {
			conds++
			if !fc.jumptableCondition(c) {
				jumptable = false
			}
		}
	}
	jumptable = jumptable && conds >= 2
	var rs uint32
	if jumptable {
		rs = fc.materialize(subject)
	}
	subjectLine := fc.cur
	base := fc.top
	end := fc.label()
	bodies := make([]int, len(arms))
	def := -1
	for i, arm := range arms {
		bodies[i] = fc.label()
		if len(arm.Kids) == 1 {
			if def >= 0 {
				panic(internalError{"two default arms"})
			}
			def = i
			continue
		}
		for _, c := range arm.Kids[1:] {
			cond := fc.expr(c)
			sub := rs
			if !jumptable {
				sub = fc.materialize(subject)
			}
			rc := fc.materialize(cond)
			t := fc.allocNot(base, sub, rc)
			fc.emit(bytecode.OpBinary, uint16(bytecode.Identical), t, sub, rc)
			next := fc.label()
			fc.branch(t, bodies[i], next)
			fc.bind(next)
			fc.top = base
		}
	}
	if def >= 0 {
		fc.jump(bodies[def])
	} else {
		sub := rs
		if !jumptable {
			quiet := subject
			quiet.quiet = true
			sub = fc.materialize(quiet)
		} else {
			fc.cur = subjectLine
		}
		fc.emit(bytecode.OpMatchError, 0, sub, 0, 0)
		fc.top = base
	}
	for i, arm := range arms {
		fc.bind(bodies[i])
		v := fc.value(arm.Kids[0])
		fc.emit(bytecode.OpMove, 0, res, v, 0)
		fc.jump(end)
		fc.top = base
	}
	fc.bind(end)
	fc.top = mark
	return reg(res)
}

// empty compiles empty(). A variable is ZEND_ISSET_ISEMPTY_CV: read quietly
// and tested as a bool. An element is the quiet fetch of every level but
// the last, then ZEND_ISSET_ISEMPTY_DIM_OBJ with ZEND_ISEMPTY. Anything
// else is compiled as !expr, as zend_compile_isset_or_empty rewrites it.
func (fc *fnCompiler) empty(n *ir.Node) operand {
	k := n.Kids[0]
	if k == nil {
		panic(internalError{"empty() without an operand"})
	}
	mark := fc.top
	switch k.Op {
	case ir.OpVar:
		fc.at(k.Line)
		v := fc.alloc()
		fc.emit(bytecode.OpLoad, 1, v, fc.varLoc(k.Var), 0)
		r := fc.allocNot(mark, v)
		fc.emit(bytecode.OpUnary, uint16(bytecode.Not), r, v, 0)
		return reg(r)
	case ir.OpDim:
		if k.Kids[1] == nil {
			panic(internalError{"empty of []"})
		}
		rb, rk := fc.readChain(k)
		r := fc.allocNot(mark, rb, rk)
		fc.emit(bytecode.OpArrayIsset, bytecode.ArrayIssetEmpty, r, rb, rk)
		return reg(r)
	}
	v := fc.value(k)
	r := fc.allocNot(mark, v)
	fc.emit(bytecode.OpUnary, uint16(bytecode.Not), r, v, 0)
	return reg(r)
}

// silence compiles @expr: BeginSilence saves error_reporting in a register
// and keeps only the fatal levels, the operand is compiled inside (a
// variable is read inside too, as zend_compile_silence forces a fetch for
// it), and EndSilence restores the saved mask. used is false when the
// value is discarded.
func (fc *fnCompiler) silence(n *ir.Node, used bool) operand {
	saved := fc.alloc()
	fc.emit(bytecode.OpBeginSilence, 0, saved, 0, 0)
	var v uint32
	if used {
		v = fc.value(n.Kids[0])
	} else {
		fc.discard(n.Kids[0])
	}
	fc.emit(bytecode.OpEndSilence, 0, saved, 0, 0)
	return reg(v)
}
