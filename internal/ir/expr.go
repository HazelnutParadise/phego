package ir

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// context is how an expression's value is used.
type context uint8

const (
	ctxRead  context = iota
	ctxWrite         // assignment target
	ctxQuiet         // isset, empty and the left side of ??
	ctxRef           // bound by reference
	ctxUnset
	ctxArg // argument the callee may take by reference: [] is allowed
)

func (l *lowerer) expr(n *frontend.Node) *Node { return l.exprCtx(n, ctxRead) }

// target lowers an assignment target.
func (l *lowerer) target(n *frontend.Node, line int) *Node {
	n = unparen(n)
	if isThis(n) {
		l.fail(line, "Cannot re-assign $this")
	}
	l.ensureWritable(n, line)
	return l.exprCtx(n, ctxWrite)
}

// refTarget lowers the source or target of a reference binding.
func (l *lowerer) refTarget(n *frontend.Node) *Node {
	n = unparen(n)
	if shortCircuited(n) {
		l.fail(n.Line, "Cannot take reference of a nullsafe chain")
	}
	switch n.Kind {
	case "call", "method_call", "static_call", "new":
		return l.exprCtx(n, ctxRead)
	}
	return l.exprCtx(n, ctxRef)
}

func (l *lowerer) unsetTarget(n *frontend.Node) *Node {
	n = unparen(n)
	if isThis(n) {
		l.fail(n.Line, "Cannot unset $this")
	}
	l.ensureWritable(n, n.Line)
	return l.exprCtx(n, ctxUnset)
}

// markUnsetBase records, before baseOf lowers n as the container of a level
// of an unset target, that n is itself such a level: PHP compiles the whole
// dim and property chain of unset() with BP_VAR_UNSET, so "[]" fails at any
// level, not only the outermost.
func (l *lowerer) markUnsetBase(n *frontend.Node, unset bool) {
	switch unparen(n).Kind {
	case "dim", "prop":
		l.unsetBase = unset
	}
}

// takeUnsetBase reports whether the dim or property being lowered belongs to
// an unset target, and clears the mark for the expressions below it.
func (l *lowerer) takeUnsetBase(ctx context) bool {
	unset := ctx == ctxUnset || l.unsetBase
	l.unsetBase = false
	return unset
}

// ensureWritable mirrors zend_ensure_writable_variable.
func (l *lowerer) ensureWritable(n *frontend.Node, line int) {
	switch n.Kind {
	case "call":
		l.fail(line, "Can't use function return value in write context")
	case "binary":
		if n.Text == "|>" {
			l.fail(line, "Can't use function return value in write context")
		}
	case "method_call", "static_call":
		l.fail(line, "Can't use method return value in write context")
	}
	if shortCircuited(n) {
		l.fail(line, "Can't use nullsafe operator in write context")
	}
	if n.Kind == "variable" && n.Text == "$GLOBALS" {
		l.fail(line, "$GLOBALS can only be modified using the $GLOBALS[$name] = $value syntax")
	}
}

// shortCircuited reports a member chain containing ?->.
func shortCircuited(n *frontend.Node) bool {
	for n != nil {
		switch n.Kind {
		case "nullsafe_prop":
			return true
		case "dim", "prop", "method_call", "static_prop":
			n = n.Children[0]
		case "static_call", "class_const":
			n = n.Children[0]
		default:
			return false
		}
	}
	return false
}

func unparen(n *frontend.Node) *frontend.Node {
	for n != nil && n.Kind == "paren" {
		n = n.Children[0]
	}
	return n
}

func isVariableLike(n *frontend.Node) bool {
	switch unparen(n).Kind {
	case "variable", "variable_variable", "dim", "prop", "static_prop", "variable_name":
		return true
	}
	return false
}

// canWrite reports what zend_can_write_to_variable accepts.
func canWrite(n *frontend.Node) bool {
	n = unparen(n)
	for n.Kind == "dim" || n.Kind == "prop" {
		n = unparen(n.Children[0])
	}
	if shortCircuited(n) {
		return false
	}
	switch n.Kind {
	case "variable", "variable_variable", "static_prop", "variable_name":
		return true
	case "call", "method_call", "static_call":
		return true
	case "binary":
		return n.Text == "|>"
	}
	return false
}

// listHasRef reports a destructuring pattern with a by-reference entry.
func listHasRef(n *frontend.Node) bool {
	for _, item := range n.Children {
		if item == nil || item.Kind != "item" {
			continue
		}
		v := item.Children[1]
		if v.Kind == "by_ref" {
			return true
		}
		if v = unparen(v); (v.Kind == "array" || v.Kind == "list") && listHasRef(v) {
			return true
		}
	}
	return false
}

func (l *lowerer) node(op Op, line int, kids ...*Node) *Node {
	return l.seal(&Node{Op: op, Line: line, Kids: kids})
}

func (l *lowerer) exprCtx(n *frontend.Node, ctx context) *Node {
	if l.constants == nil {
		l.constants = l.syntaxBudget(n)
		defer func() { l.constants = nil }()
	}
	l.nest()
	defer func() { l.nesting-- }()
	n, folded := l.exprHead(n, ctx)
	if folded != nil {
		return folded
	}
	return l.exprBody(n, ctx)
}

// exprHead is the part of exprCtx every expression passes before its kind
// is lowered: it follows a branch the syntax pass selected, sets the line
// and returns the folded value when the syntax pass knows it.
func (l *lowerer) exprHead(n *frontend.Node, ctx context) (*frontend.Node, *Node) {
	n = l.selectedSyntax(n)
	l.cur = l.zline(n)
	if entry := l.constants.syntax[n]; entry.known && (ctx == ctxRead || ctx == ctxQuiet) && n.Kind != "array" {
		// A folded value carries the line where PHP folded it.
		if entry.line != 0 {
			l.cur = entry.line
		}
		// The node carries the line PHP's line counter rests at after the
		// folded expression: the last operand's, or for a concatenation the
		// parser folded, the token after it.
		return n, l.constantValueNode(entry.value, foldedLine(n), 0)
	}
	return n, nil
}

// foldedLine is the line CG(zend_lineno) rests at after PHP compiles the
// constant expression n: each zend_compile_expr sets it to its node's line,
// so a binary operation leaves it at its right operand's last line and
// parentheses at their content's; a concatenation of literals is already
// one zval when it is compiled, created by the parser on the line of the
// token after it (frontend.Node.FoldLine); every other node leaves its own
// line.
func foldedLine(n *frontend.Node) int {
	for n != nil {
		switch {
		case n.Kind == "binary" && n.FoldLine != 0:
			return n.FoldLine
		case n.Kind == "binary":
			n = n.Children[1]
		case n.Kind == "paren":
			n = n.Children[0]
		default:
			return n.Line
		}
	}
	return 0
}

// exprBody lowers n by kind once exprHead has passed it.
func (l *lowerer) exprBody(n *frontend.Node, ctx context) *Node {
	c := n.Children
	switch n.Kind {
	case "literal":
		return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: l.literal(n)})
	case "variable", "variable_name":
		return l.varNode(n, ctx)
	case "variable_variable":
		l.fn.DynamicScope = true
		nd := &Node{Op: OpVarVar, Line: n.Line, Kids: []*Node{l.expr(c[0])}}
		// zend_compile_simple_var_no_cv converts a constant name to a string.
		l.convertName(nd, 0, c[0])
		l.flag(nd, ctx)
		return l.seal(nd)
	case "dim":
		if c[1] == nil && (ctx == ctxRead || ctx == ctxQuiet) {
			l.fail(n.Line, "Cannot use [] for reading")
		}
		if b := unparen(c[0]); c[1] == nil && b.Kind == "variable" && b.Text == "$GLOBALS" {
			l.fail(n.Line, "Cannot append to $GLOBALS")
		}
		unset := l.takeUnsetBase(ctx)
		l.markUnsetBase(c[0], unset)
		base := l.baseOf(c[0], ctx, n.Line)
		if unset && c[1] == nil {
			// zend_delayed_compile_dim checks each level of an unset
			// target, innermost first, after compiling its container: the
			// line is where that left off.
			l.fail(l.cur, "Cannot use [] for unsetting")
		}
		var idx *Node
		if c[1] != nil {
			idx = l.dimIndex(c[1])
		}
		nd := &Node{Op: OpDim, Line: n.Line, Kids: []*Node{base, idx}}
		l.flag(nd, ctx)
		return l.seal(nd)
	case "prop", "nullsafe_prop":
		op := OpProp
		if n.Kind == "nullsafe_prop" {
			op = OpNullsafe
		}
		obj := c[0]
		unset := l.takeUnsetBase(ctx)
		var base *Node
		if isThis(unparen(obj)) {
			base = l.thisNode(obj.Line)
		} else if ctx == ctxWrite || ctx == ctxRef || ctx == ctxUnset {
			l.markUnsetBase(obj, unset)
			base = l.baseOf(obj, ctxWrite, n.Line)
		} else {
			base = l.exprCtx(obj, ctx)
		}
		nd := &Node{Op: op, Line: n.Line, Kids: []*Node{base, nil}}
		l.memberName(nd, c[1])
		if unset {
			// zend_delayed_compile_prop compiles the name as an expression,
			// which moves the line an outer "[]" is reported on.
			l.cur = l.zline(c[1])
		}
		l.flag(nd, ctx)
		return l.seal(nd)
	case "static_prop":
		nd := &Node{Op: OpStaticProp, Line: n.Line, Kids: []*Node{nil, nil}}
		l.classOperand(nd, c[0])
		if v := c[1]; v.Kind == "variable" {
			nd.Text = v.Text[1:]
		} else {
			nd.Kids[1] = l.expr(v.Children[0])
			l.convertName(nd, 1, v.Children[0])
		}
		l.flag(nd, ctx)
		return l.seal(nd)
	case "class_const":
		nd := &Node{Op: OpClassConst, Line: n.Line, Kids: []*Node{nil, nil}}
		l.classOperand(nd, c[0])
		if c[1].Kind == "identifier" {
			nd.Text = c[1].Text
		} else {
			nd.Kids[1] = l.expr(c[1])
		}
		return l.seal(nd)
	case "class_name":
		nd := &Node{Op: OpClassName, Line: n.Line, Kids: []*Node{nil}}
		if cls := c[0]; cls.Kind == "name" {
			name := l.classRef(cls)
			if name.Special == "" {
				return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: name.Resolved}})
			}
			if folded, ok := l.compileTimeClassName(name.Special); ok {
				return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: folded}})
			}
			nd.Name = name
		} else if v, isString, ok := parseTimeValue(cls); ok && isString {
			// zend_try_compile_const_expr_resolve_class_name: a string the
			// parser holds names self, parent or static, which must be valid
			// here, or is a fully qualified class name.
			special := specialClass(v)
			if special == "" {
				return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: l.fullyQualifiedClass(v)}})
			}
			l.ensureClassFetch(special, l.cur)
			if folded, ok := l.compileTimeClassName(special); ok {
				return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: folded}})
			}
			nd.Name = l.ns.resolveClass(special)
		} else {
			nd.Kids[0] = l.expr(cls)
			if k := l.compileClassKind(nd.Kids[0]); k != "" {
				if unparen(cls).Kind == "literal" && k != "string" {
					l.fail(l.cur, "Illegal class name")
				}
				l.fail(l.cur, "Cannot use \"::class\" on %s", k)
			}
		}
		return l.seal(nd)
	case "name":
		return l.constant(n)
	case "magic_const":
		return l.magic(n)
	case "interpolated", "heredoc":
		return l.interpolation(n)
	case "array":
		if ctx == ctxWrite {
			return l.destructurePattern(n, n.Line)
		}
		return l.arrayLiteral(n)
	case "list":
		return l.destructurePattern(n, n.Line)
	case "assign":
		return l.assign(n)
	case "assign_ref":
		left, right := unparen(c[0]), c[1]
		if isThis(left) {
			l.fail(n.Line, "Cannot re-assign $this")
		}
		l.ensureWritable(left, zline(n))
		if unparen(right).Kind == "variable" && unparen(right).Text == "$GLOBALS" {
			l.fail(n.Line, "Cannot acquire reference to $GLOBALS")
		}
		if left.Kind == "array" || left.Kind == "list" {
			l.fail(n.Line, "Cannot assign reference to non referenceable value")
		}
		src := l.refTarget(right)
		dst := l.exprCtx(left, ctxRef)
		return l.node(OpAssignRef, n.Line, dst, src)
	case "pre_inc", "post_inc":
		target := unparen(c[0])
		l.ensureWritable(target, n.Line)
		op := OpPreInc
		if n.Kind == "post_inc" {
			op = OpPostInc
		}
		nd := &Node{Op: op, Line: n.Line, Text: n.Text, Kids: []*Node{l.exprCtx(target, ctxWrite)}}
		l.markRead(nd.Kids[0])
		return l.seal(nd)
	case "unary":
		if n.Text == "@" {
			return l.node(OpSilence, n.Line, l.exprCtx(c[0], ctx))
		}
		return l.seal(&Node{Op: OpUnary, Line: n.Line, Text: n.Text, Kids: []*Node{l.expr(c[0])}})
	case "cast":
		return l.cast(n)
	case "binary":
		return l.binary(n)
	case "ternary":
		if inner := c[0]; inner.Kind == "ternary" {
			switch {
			case inner.Children[1] != nil && c[1] != nil:
				l.fail(zline(n), "Unparenthesized `a ? b : c ? d : e` is not supported. Use either `(a ? b : c) ? d : e` or `a ? b : (c ? d : e)`")
			case inner.Children[1] != nil:
				l.fail(zline(n), "Unparenthesized `a ? b : c ?: d` is not supported. Use either `(a ? b : c) ?: d` or `a ? b : (c ?: d)`")
			case c[1] != nil:
				l.fail(zline(n), "Unparenthesized `a ?: b ? c : d` is not supported. Use either `(a ?: b) ? c : d` or `a ?: (b ? c : d)`")
			}
		}
		cond := l.expr(c[0])
		var then *Node
		if c[1] != nil {
			then = l.expr(c[1])
		}
		return l.node(OpTernary, n.Line, cond, then, l.expr(c[2]))
	case "isset":
		nd := &Node{Op: OpIsset, Line: n.Line}
		for _, e := range c {
			e = unparen(e)
			if !isVariableLike(e) && e.Kind != "nullsafe_prop" && e.Kind != "class_const" {
				l.fail(zline(e), "Cannot use isset() on the result of an expression (you can use \"null !== expression\" instead)")
			}
			nd.Kids = append(nd.Kids, l.exprCtx(e, ctxQuiet))
		}
		return l.seal(nd)
	case "builtin":
		if phpname.EqualFold(n.Text, "empty") {
			return l.node(OpEmpty, n.Line, l.exprCtx(c[0], ctxQuiet))
		}
		l.fn.DynamicScope = true
		return l.node(OpEval, n.Line, l.expr(c[0]))
	case "include":
		l.fn.DynamicScope = true
		return l.seal(&Node{Op: OpInclude, Line: n.Line, Text: phpname.Lower(n.Text), Kids: []*Node{l.expr(c[0])}})
	case "exit":
		nd := &Node{Op: OpExit, Line: n.Line}
		if len(c) > 0 {
			nd.Args = l.args(c[0], n.Line)
		}
		return l.seal(nd)
	case "print":
		return l.node(OpPrint, n.Line, l.expr(c[0]))
	case "clone":
		if c[0].Kind == "callable_args" {
			fn := l.seal(&Node{Op: OpCall, Line: n.Line, Name: &Name{Kind: FunctionName, Resolved: "clone"}})
			return l.node(OpCallable, n.Line, fn)
		}
		if c[0].Kind == "args" {
			nd := &Node{Op: OpClone, Line: n.Line, Args: l.args(c[0], n.Line)}
			return l.seal(nd)
		}
		return l.node(OpClone, n.Line, l.expr(c[0]))
	case "instanceof":
		nd := &Node{Op: OpInstanceof, Line: n.Line, Kids: []*Node{l.expr(c[0]), nil}}
		// zend_compile_instanceof folds a constant object operand to false
		// without compiling the class. self::class and parent::class are
		// constants where the scope is known, even when Phego cannot spell
		// an anonymous class's name.
		if _, known := l.evaluateConstant(nd.Kids[0]); known || l.constantClassName(nd.Kids[0]) {
			return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitBool}})
		}
		l.classOperandAt(nd, 1, c[1])
		return l.seal(nd)
	case "call":
		return l.call(n)
	case "method_call":
		return l.methodCall(n)
	case "static_call":
		return l.staticCall(n)
	case "new":
		return l.newExpr(n)
	case "new_class":
		if c[2] != nil && c[2].Kind == "callable_args" {
			l.fail(n.Line, "Cannot create Closure for new expression")
		}
		cl := l.classDecl(n, true)
		nd := &Node{Op: OpNew, Line: n.Line, Class: cl}
		if c[2] != nil {
			nd.Args = l.args(c[2], n.Line)
		}
		return l.seal(nd)
	case "closure", "arrow_function":
		if nd, ok := l.constClosures[n]; ok {
			delete(l.constClosures, n)
			return nd
		}
		return l.closure(n)
	case "match":
		return l.match(n)
	case "throw":
		return l.node(OpThrow, n.Line, l.expr(c[0]))
	case "yield":
		l.markGenerator(n.Line)
		var key, value *Node
		if c[0] != nil {
			key = l.expr(c[0])
		}
		if c[1] != nil {
			if l.fn.ByRef && (isVariableLike(c[1]) || shortCircuited(unparen(c[1]))) {
				value = l.refTarget(c[1])
			} else {
				value = l.expr(c[1])
			}
		}
		return l.node(OpYield, n.Line, key, value)
	case "yield_from":
		l.markGenerator(n.Line)
		if l.fn.ByRef {
			l.fail(n.Line, "Cannot use \"yield from\" inside a by-reference generator")
		}
		return l.node(OpYieldFrom, n.Line, l.expr(c[0]))
	case "spread":
		return l.node(OpSpread, n.Line, l.expr(c[0]))
	case "num_string":
		return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: numString(n.Text)})
	case "string_offset":
		return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: n.Text}})
	}
	l.fail(n.Line, "Cannot use temporary expression in write context")
	return nil
}

// baseOf lowers the container of a written dim or property.
func (l *lowerer) baseOf(n *frontend.Node, ctx context, line int) *Node {
	n = unparen(n)
	if ctx == ctxRead || ctx == ctxQuiet || ctx == ctxArg {
		return l.exprCtx(n, ctx)
	}
	if isThis(n) {
		return l.thisNode(n.Line)
	}
	switch n.Kind {
	case "variable", "variable_name", "variable_variable", "dim", "prop", "static_prop":
		if n.Kind == "variable" && n.Text == "$GLOBALS" {
			return l.exprCtx(n, ctxRead)
		}
		return l.exprCtx(n, ctxWrite)
	case "call", "method_call", "static_call", "nullsafe_prop", "binary":
		if n.Kind == "binary" && n.Text != "|>" {
			break
		}
		return l.exprCtx(n, ctxRead)
	}
	l.fail(line, "Cannot use temporary expression in write context")
	return nil
}

func (l *lowerer) dimIndex(n *frontend.Node) *Node {
	return l.expr(n)
}

func (l *lowerer) flag(nd *Node, ctx context) {
	switch ctx {
	case ctxWrite:
		nd.Flags |= FlagWrite
	case ctxQuiet:
		nd.Flags |= FlagQuiet
	case ctxRef:
		nd.Flags |= FlagRef | FlagWrite
	case ctxUnset:
		nd.Flags |= FlagUnset
	}
}

// markRead records that a written target is also read, as for ++ and op=.
func (l *lowerer) markRead(n *Node) {
	if n.Op == OpVar {
		n.Var.Flags |= VarRead
	}
}

func (l *lowerer) varNode(n *frontend.Node, ctx context) *Node {
	name := n.Text
	if n.Kind == "variable" {
		name = name[1:]
	}
	if name == "this" {
		return l.thisNode(n.Line)
	}
	if isSuperGlobal(name) {
		nd := &Node{Op: OpSuperGlobal, Line: n.Line, Text: name}
		l.flag(nd, ctx)
		return l.seal(nd)
	}
	var flags VarFlag
	switch ctx {
	case ctxRead, ctxQuiet, ctxArg:
		flags = VarRead
	case ctxWrite, ctxUnset:
		flags = VarWritten
	case ctxRef:
		flags = VarRef | VarWritten
	}
	nd := &Node{Op: OpVar, Line: n.Line, Var: l.variable(name, flags)}
	l.flag(nd, ctx)
	return l.seal(nd)
}

func (l *lowerer) thisNode(line int) *Node {
	n := l.seal(&Node{Op: OpThis, Line: line})
	if l.fn.Kind == FuncMethod && !l.fn.Static {
		n.Effects = 0 // $this is always bound in an instance method
	}
	return n
}

func (l *lowerer) memberName(nd *Node, name *frontend.Node) {
	if name.Kind == "identifier" {
		nd.Text = name.Text
		return
	}
	nd.Kids[1] = l.expr(name)
	l.convertName(nd, 1, name)
}

// convertName applies the convert_to_string that zend_delayed_compile_prop,
// zend_compile_static_prop, zend_compile_simple_var_no_cv and
// zend_compile_global_var perform on kid i of nd, the name compiled from
// source, when it is a constant. Only conversions that warn are folded
// here; a name PHP leaves for execution keeps its expression. A property
// or static property keeps the result as its literal Text; a variable
// variable or global keeps a string literal operand.
func (l *lowerer) convertName(nd *Node, i int, source *frontend.Node) {
	v, known := l.probeConstant(nd.Kids[i])
	if !known {
		return
	}
	var text, warning string
	switch {
	case v.array:
		text, warning = "Array", "Array to string conversion"
	case v.value != nil && v.value.Kind == LitFloat && math.IsNaN(v.value.Float):
		text, _ = constString(v.value)
		warning = "unexpected NAN value was coerced to string"
	default:
		return
	}
	l.diag(l.cur, "Warning", warning)
	if nd.Op == OpVarVar || nd.Op == OpGlobal {
		nd.Kids[i] = l.seal(&Node{Op: OpLiteral, Line: nd.Kids[i].Line, Value: &Literal{Kind: LitString, Str: text}})
	} else {
		nd.Text, nd.Kids[i] = text, nil
	}
	if l.nameWarnings == nil {
		l.nameWarnings = make(map[*Node]nameWarning)
	}
	// A second compilation starts at the name's own line
	// (zend_compile_expr_inner sets CG(zend_lineno) to it).
	l.nameWarnings[nd] = nameWarning{l.zline(source), warning}
}

type nameWarning struct {
	line    int
	message string
}

// repeatNameConversions follows zend_compile_assign_coalesce, which compiles
// its target again after the default value. Memoized operands, including a
// static property's class, are reused, but every fetch in the target
// converts its constant member name again.
func (l *lowerer) repeatNameConversions(n *Node) {
	var chain []*Node
walk:
	for n != nil {
		switch n.Op {
		case OpProp, OpNullsafe, OpDim:
			chain = append(chain, n)
			n = n.Kids[0]
		case OpStaticProp, OpVarVar:
			chain = append(chain, n)
			break walk
		default:
			break walk
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if w, ok := l.nameWarnings[chain[i]]; ok {
			l.diag(w.line, "Warning", w.message)
		}
	}
}

// classOperand fills the class of a static access: a resolved name or an
// expression evaluated at run time.
func (l *lowerer) classOperand(nd *Node, cls *frontend.Node) {
	l.classOperandAt(nd, 0, cls)
}

// classOperandAt lowers the class of a static access, new or instanceof into
// nd.Name or nd.Kids[i], as zend_compile_class_ref does.
func (l *lowerer) classOperandAt(nd *Node, i int, cls *frontend.Node) {
	// Parentheses make a name an expression: (A)::f() reads constant A.
	if cls.Kind == "name" {
		nd.Name = l.classRef(cls)
		return
	}
	parsed := l.checkStringClassName(cls)
	nd.Kids[i] = l.expr(cls)
	// zend_compile_class_ref rejects a class expression compiled to a
	// constant other than a string. A constant string names self, parent or
	// static, which must be valid here, or is a fully qualified name.
	if k := l.probeClassKind(nd.Kids[i]); k != "" && k != "string" {
		l.fail(l.cur, "Illegal class name")
	}
	if parsed {
		return
	}
	if v, known := l.evaluateConstant(nd.Kids[i]); known && v.value != nil && v.value.Kind == LitString {
		if special := specialClass(v.value.Str); special != "" {
			l.ensureClassFetch(special, l.cur)
			nd.Name, nd.Kids[i] = l.ns.resolveClass(special), nil
		} else {
			l.fullyQualifiedClass(v.value.Str)
		}
	}
}

// checkStringClassName applies zend_resolve_class_name to a string the
// parser already holds, reporting whether cls is one.
func (l *lowerer) checkStringClassName(cls *frontend.Node) bool {
	v, isString, ok := parseTimeValue(cls)
	if ok && isString {
		l.fullyQualifiedClass(v)
	}
	return ok && isString
}

// fullyQualifiedClass is zend_resolve_class_name for a string compiled as a
// fully qualified class name: one leading backslash is removed, and the
// name cannot be self, parent or static.
func (l *lowerer) fullyQualifiedClass(v string) string {
	if specialClass(v) == "" && strings.HasPrefix(v, "\\") {
		v = v[1:]
	}
	if specialClass(v) != "" {
		l.fail(l.cur, "'\\%s' is an invalid class name", v)
	}
	return v
}

// compileTimeClassName is the name zend_try_compile_const_expr_resolve_class_name
// folds self or parent to where the scope is known. An anonymous class's
// real name is only known at run time.
func (l *lowerer) compileTimeClassName(special string) (string, bool) {
	if l.class == nil || !l.scopeKnown() {
		return "", false
	}
	switch {
	case special == "self" && !l.class.Anonymous:
		return l.class.Name, true
	case special == "parent" && l.class.Parent != nil:
		return l.class.Parent.Resolved, true
	}
	return "", false
}

// constantClassName reports self::class or parent::class where PHP folds it
// to a constant (see compileTimeClassName).
func (l *lowerer) constantClassName(n *Node) bool {
	if n == nil || n.Op != OpClassName || n.Name == nil || l.class == nil || !l.scopeKnown() {
		return false
	}
	return n.Name.Special == "self" || n.Name.Special == "parent" && l.class.Parent != nil
}

// ensureClassFetch is zend_ensure_valid_class_fetch_type for self, parent or
// static where the scope is known at compile time.
func (l *lowerer) ensureClassFetch(special string, line int) {
	if !l.scopeKnown() {
		return
	}
	if l.class == nil {
		l.fail(line, "Cannot use \"%s\" when no class scope is active", special)
	}
	if special == "parent" && l.class.Parent == nil {
		l.fail(line, "Cannot use \"parent\" when current class scope has no parent")
	}
}

// classRef resolves a class name and checks self, parent and static.
func (l *lowerer) classRef(n *frontend.Node) *Name {
	l.checkClassName(n)
	name := l.ns.resolveClass(n.Text)
	if name.Special != "" && l.scopeKnown() && !l.inConstExpr {
		if l.class == nil {
			l.fail(n.Line, "Cannot use \"%s\" when no class scope is active", name.Special)
		}
		if name.Special == "parent" && l.class.Parent == nil {
			l.fail(n.Line, "Cannot use \"parent\" when current class scope has no parent")
		}
	}
	return name
}

// scopeKnown mirrors zend_is_scope_known: closures can be rebound, file
// code inherits its includer's scope, and traits stand for their user.
func (l *lowerer) scopeKnown() bool {
	if l.fn == nil || l.fn.Kind == FuncClosure || l.fn.Kind == FuncArrow {
		return false
	}
	if l.class == nil {
		return l.fn.Kind != FuncMain
	}
	return l.class.Kind != KindTrait
}

// constant lowers a bare name: true, false and null fold to literals.
func (l *lowerer) constant(n *frontend.Node) *Node {
	name := l.ns.resolveConstant(n.Text)
	if name.Fallback == "" {
		switch phpname.Lower(name.Resolved) {
		case "true":
			return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitBool, Bool: true}})
		case "false":
			return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitBool}})
		case "null":
			return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitNull}})
		}
	}
	return l.seal(&Node{Op: OpConst, Line: n.Line, Name: name})
}

func (l *lowerer) magic(n *frontend.Node) *Node {
	str := func(s string) *Node {
		return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: s}})
	}
	f := l.fn
	switch strings.ToUpper(n.Text) {
	case "__LINE__":
		return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitInt, Int: int64(n.Line)}})
	case "__FILE__":
		return str(l.unit.File)
	case "__DIR__":
		return str(fileDir(l.unit.File))
	case "__NAMESPACE__":
		return str(l.ns.name)
	case "__FUNCTION__":
		switch f.Kind {
		case FuncMain:
			return str("")
		case FuncHook:
			return str(f.Name)
		}
		return str(f.Name)
	case "__METHOD__":
		switch f.Kind {
		case FuncMain:
			return str("")
		case FuncMethod, FuncHook:
			if l.class != nil {
				return str(l.class.Name + "::" + f.Name)
			}
		}
		return str(f.Name)
	case "__CLASS__":
		if l.class == nil {
			return str("")
		}
		if l.class.Kind == KindTrait {
			return l.seal(&Node{Op: OpMagic, Line: n.Line, Text: "__CLASS__"})
		}
		if l.class.Anonymous {
			return l.seal(&Node{Op: OpMagic, Line: n.Line, Text: "__CLASS__"})
		}
		return str(l.class.Name)
	case "__TRAIT__":
		if l.class != nil && l.class.Kind == KindTrait {
			return str(l.class.Name)
		}
		return str("")
	case "__PROPERTY__":
		for g := f; g != nil; g = g.Parent {
			if g.Kind == FuncHook {
				return str(g.property)
			}
		}
		return str("")
	}
	return l.seal(&Node{Op: OpMagic, Line: n.Line, Text: n.Text})
}

func (l *lowerer) markGenerator(line int) {
	f := l.fn
	if f.Kind == FuncMain {
		l.fail(line, "The \"yield\" expression can only be used inside a function")
	}
	f.Generator = true
}

func numString(text string) *Literal {
	if v, err := strconv.ParseInt(text, 10, 64); err == nil && strconv.FormatInt(v, 10) == text {
		return &Literal{Kind: LitInt, Int: v}
	}
	return &Literal{Kind: LitString, Str: text}
}

// literal decodes a literal token.
func (l *lowerer) literal(n *frontend.Node) *Literal {
	switch n.Token {
	case frontend.TLNumber, frontend.TDNumber:
		return numberLiteral(n.Text, n.Token)
	}
	return &Literal{Kind: LitString, Str: l.decodeScanned(n, func(d stringDecoder) string { return d.constant(n.Text, n.Line) }), Uninterned: constantBodyLen(n.Text) > 1}
}

// constantBodyLen is the length of a T_CONSTANT_ENCAPSED_STRING between its
// quotes. zend_scan_escape_string and the single-quoted scanner intern a
// body of at most one byte and allocate any longer one, even when escape
// sequences decode it to a single byte.
func constantBodyLen(text string) int {
	if text != "" && (text[0] == 'b' || text[0] == 'B') {
		text = text[1:]
	}
	return len(text) - 2
}

// constantValue returns the literal value of a declare directive.
func (l *lowerer) constantValue(n *frontend.Node) *Literal {
	n = unparen(n)
	switch n.Kind {
	case "literal":
		return l.literal(n)
	case "name":
		switch phpname.Lower(n.Text) {
		case "true":
			return &Literal{Kind: LitBool, Bool: true}
		case "false":
			return &Literal{Kind: LitBool}
		case "null":
			return &Literal{Kind: LitNull}
		}
	}
	return nil
}

// interpolation lowers a double-quoted, shell or heredoc string. A string
// without embedded expressions folds to one literal.
func (l *lowerer) interpolation(n *frontend.Node) *Node {
	parts := n.Children
	quote := byte('"')
	heredoc := n.Kind == "heredoc"
	nowdoc := heredoc && strings.Contains(n.Text, "'")
	indent := 0
	if heredoc {
		quote = 0
		end := parts[len(parts)-1]
		parts = parts[:len(parts)-1]
		indent = heredocIndent(end.Text)
	} else if n.Text == "`" {
		quote = '`'
	}
	nd := &Node{Op: OpInterp, Line: n.Line}
	if n.Text == "`" {
		nd.Op = OpShell
		l.diag(n.Line, "Deprecated", "The backtick (`) operator is deprecated, use shell_exec() instead")
	}
	for i, p := range parts {
		if p.Kind != "string_part" {
			// PHP reports the line of the part compiled before this one.
			line := l.cur
			if i > 0 {
				line = zline(parts[i-1])
			}
			switch {
			case p.Kind == "variable_name", p.Kind == "dim" && p.Children[0].Kind == "variable_name":
				l.diag(line, "Deprecated", "Using ${var} in strings is deprecated, use {$var} instead")
			case p.Kind == "variable_variable" && p.Text == "${":
				l.diag(line, "Deprecated", "Using ${expr} (variable variables) in strings is deprecated, use {${expr}} instead")
			}
			nd.Kids = append(nd.Kids, l.expr(p))
			continue
		}
		text := p.Text
		if heredoc {
			last := i == len(parts)-1
			if last {
				text = trimFinalNewline(text)
			}
			text = stripIndentation(text, indent, i == 0, last)
		}
		// The scanner interns a heredoc or quoted part of at most one byte
		// (after the indentation is removed); a nowdoc is always copied.
		fresh := len(text) > 1 || nowdoc && text != ""
		if !nowdoc {
			raw := text
			text = l.decodeScanned(p, func(d stringDecoder) string { return d.escapes(raw, quote, p.Line) })
		}
		nd.Kids = append(nd.Kids, l.seal(&Node{Op: OpLiteral, Line: p.Line, Value: &Literal{Kind: LitString, Str: text, Uninterned: fresh}}))
	}
	if nd.Op == OpInterp {
		folded := true
		var sb strings.Builder
		for _, k := range nd.Kids {
			if k.Op != OpLiteral {
				folded = false
				break
			}
			sb.WriteString(k.Value.Str)
		}
		if folded {
			// One part is the scanner's string itself.
			fresh := len(nd.Kids) > 1 || len(nd.Kids) == 1 && nd.Kids[0].Value.Uninterned
			return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitString, Str: sb.String(), Uninterned: fresh}})
		}
	}
	return l.seal(nd)
}

func (l *lowerer) arrayLiteral(n *frontend.Node) *Node {
	entry := l.preEvaluateSyntax(n)
	nd := &Node{Op: OpArray, Line: n.Line}
	if len(n.Children) == 0 && n.CloseLine != 0 {
		// zend_ast_create_list with no child takes the current line, which
		// is the closing bracket's: "[\n][0]" and "[$a] = [\n]" report
		// that line.
		nd.Line = n.CloseLine
	}
	for _, item := range n.Children {
		if item == nil {
			l.fail(l.cur, "Cannot use empty array elements in arrays")
		}
		if item.Kind == "spread" {
			v := l.expr(item.Children[0])
			nd.Kids = append(nd.Kids, l.node(OpSpread, item.Line, v))
			continue
		}
		if v := item.Children[1]; v.Kind == "list" {
			l.fail(v.Line, "Cannot use list() as standalone expression")
		}
		var key *Node
		if item.Children[0] != nil {
			key = l.expr(item.Children[0])
		}
		it := &Node{Op: OpItem, Line: item.Line}
		if v := item.Children[1]; v.Kind == "by_ref" {
			it.Flags |= FlagRef
			it.Kids = []*Node{key, l.refTarget(v.Children[0])}
		} else {
			it.Kids = []*Node{key, l.expr(v)}
		}
		nd.Kids = append(nd.Kids, l.seal(it))
	}
	if entry.known {
		if l.constants.memo == nil {
			l.constants.memo = make(map[*Node]compileMemoEntry)
		}
		l.constants.memo[nd] = compileMemoEntry{value: entry.value, known: true, height: entry.height}
	} else {
		_, entry.known = l.evaluateConstant(nd)
	}
	if entry.known {
		// zend_try_ct_eval_array folds a constant array without compiling
		// its elements, so the line stays at the array.
		l.cur = l.zline(n)
	}
	return l.seal(nd)
}

// destructurePattern lowers a list() or [] assignment target, with the
// checks of zend_compile_list_assign.
func (l *lowerer) destructurePattern(n *frontend.Node, line int) *Node {
	style := n.Kind
	if n.Kind == "array" && n.Text == "array" {
		l.fail(line, "Cannot assign to array(), use [] instead")
	}
	nd := &Node{Op: OpArray, Line: n.Line, Flags: FlagWrite}
	keyed := false
	for _, item := range n.Children {
		if item != nil {
			keyed = item.Kind == "item" && item.Children[0] != nil
			break
		}
	}
	hasElems := false
	for _, item := range n.Children {
		if item == nil {
			if keyed {
				l.fail(l.cur, "Cannot use empty array entries in keyed array assignment")
			}
			nd.Kids = append(nd.Kids, nil)
			continue
		}
		if item.Kind == "spread" {
			l.fail(l.cur, "Spread operator is not supported in assignments")
		}
		hasElems = true
		if keyed != (item.Children[0] != nil) {
			l.fail(l.cur, "Cannot mix keyed and unkeyed array entries in assignments")
		}
		var key *Node
		if item.Children[0] != nil {
			key = l.expr(item.Children[0])
		}
		it := &Node{Op: OpItem, Line: item.Line}
		v := item.Children[1]
		if v.Kind == "by_ref" {
			it.Flags |= FlagRef
			it.Kids = []*Node{key, l.refTarget(v.Children[0])}
		} else {
			v = unparen(v)
			switch {
			case v.Kind == "array" || v.Kind == "list":
				if v.Kind == "array" && v.Text == "array" {
					l.fail(line, "Cannot assign to array(), use [] instead")
				}
				if (v.Kind == "list") != (style == "list") {
					l.fail(l.cur, "Cannot mix [] and list()")
				}
				it.Kids = []*Node{key, l.destructurePattern(v, line)}
			case !canWrite(v):
				l.fail(zline(v), "Assignments can only happen to writable values")
			default:
				it.Kids = []*Node{key, l.target(v, line)}
			}
		}
		nd.Kids = append(nd.Kids, l.seal(it))
	}
	if !hasElems {
		l.fail(l.cur, "Cannot use empty list")
	}
	return l.seal(nd)
}

func (l *lowerer) assign(n *frontend.Node) *Node {
	left, right := unparen(n.Children[0]), n.Children[1]
	if isThis(left) {
		l.fail(n.Line, "Cannot re-assign $this")
	}
	if n.Text == "=" && (left.Kind == "array" || left.Kind == "list") {
		var value *Node
		if listHasRef(left) {
			r := unparen(right)
			switch r.Kind {
			case "variable", "variable_variable", "variable_name", "dim", "prop", "nullsafe_prop", "static_prop", "call", "method_call", "static_call":
			default:
				l.fail(n.Line, "Cannot assign reference to non referenceable value")
			}
			value = l.refTarget(r)
		} else {
			value = l.expr(right)
		}
		pattern := l.destructurePattern(left, n.Line)
		return l.node(OpDestructure, n.Line, pattern, value)
	}
	l.ensureWritable(left, zline(n))
	if n.Text == "=" {
		dst := l.exprCtx(left, ctxWrite)
		return l.node(OpAssign, n.Line, dst, l.expr(right))
	}
	dst := l.exprCtx(left, ctxWrite)
	l.markRead(dst)
	if n.Text == "??=" {
		value := l.expr(right)
		l.repeatNameConversions(dst)
		return l.seal(&Node{Op: OpCoalesceSet, Line: n.Line, Kids: []*Node{dst, value}})
	}
	return l.seal(&Node{Op: OpAssignOp, Line: n.Line, Text: strings.TrimSuffix(n.Text, "="), Kids: []*Node{dst, l.expr(right)}})
}

var castNames = map[string]string{
	"int": "int", "integer": "int", "bool": "bool", "boolean": "bool", "float": "float",
	"double": "float", "real": "float", "string": "string", "binary": "string",
	"array": "array", "object": "object", "unset": "unset", "void": "void",
}

func (l *lowerer) cast(n *frontend.Node) *Node {
	inner := phpname.Lower(strings.Trim(n.Text, "() \t"))
	target := castNames[inner]
	switch inner {
	case "unset":
		l.fail(n.Line, "The (unset) cast is no longer supported")
	}
	return l.seal(&Node{Op: OpCast, Line: n.Line, Text: target, Kids: []*Node{l.expr(n.Children[0])}})
}

// binary lowers a chain of binary operations nested through their left
// operand, such as 1+1+…+1, with a loop instead of one Go frame per
// operator, so a long chain cannot overflow the goroutine stack. Every link
// passes exprHead and its own checks in the order recursive lowering takes
// them: the operator's checks and the left operand first, then, from the
// innermost link out, each right operand.
func (l *lowerer) binary(n *frontend.Node) *Node {
	var spine []*frontend.Node
	var left *Node
	base := l.nesting
	defer func() { l.nesting = base }() // also when a link fails
	for {
		if phpname.Lower(n.Text) == "|>" && n.Children[1].Kind == "arrow_function" {
			l.fail(zline(n), "Arrow functions on the right hand side of |> must be parenthesized")
		}
		if len(spine) > 0 {
			l.nest() // the link n is the left operand of the one above it
		}
		spine = append(spine, n)
		if phpname.Lower(n.Text) == "??" {
			left = l.exprCtx(n.Children[0], ctxQuiet)
			break
		}
		next, folded := l.exprHead(n.Children[0], ctxRead)
		if folded != nil {
			left = folded
			break
		}
		if next.Kind != "binary" {
			left = l.exprBody(next, ctxRead)
			break
		}
		n = next
	}
	for i := len(spine) - 1; i >= 0; i-- {
		left = l.binaryLink(spine[i], left)
		if i > 0 {
			l.nesting--
		}
	}
	return left
}

// binaryLink lowers one binary operation whose left operand is lowered.
func (l *lowerer) binaryLink(n *frontend.Node, left *Node) *Node {
	c := n.Children
	op := phpname.Lower(n.Text)
	switch op {
	case "??":
		return l.node(OpCoalesce, n.Line, left, l.expr(c[1]))
	case "&&", "||", "and", "or", "xor":
		switch op {
		case "and":
			op = "&&"
		case "or":
			op = "||"
		}
		v, known := l.evaluateConstant(left)
		if known && op != "xor" {
			if value, ok := l.compileTruth(v); ok && value == (op == "||") {
				l.compileTruth(v) // PHP performs a second conversion for the result.
				return l.seal(&Node{Op: OpLiteral, Line: n.Line, Value: &Literal{Kind: LitBool, Bool: value}})
			}
		}
		right := l.expr(c[1])
		if known {
			if rv, rknown := l.evaluateConstant(right); rknown {
				if op == "xor" {
					l.compileTruth(v)
				}
				l.compileTruth(rv)
			}
		}
		return l.seal(&Node{Op: OpLogical, Line: n.Line, Text: op, Kids: []*Node{left, right}})
	case "<>":
		op = "!="
	}
	right := l.expr(c[1])
	if op == "." && !l.inConstExpr {
		// zend_compile_binary_op converts each constant operand to a string
		// at compile time, whether or not the whole expression folds.
		for _, operand := range []*Node{left, right} {
			if v, known := l.probeConstant(operand); known {
				l.compileString(v)
			}
		}
	}
	// PHP's parser folds a concatenation of two literals into one string
	// (zend_ast_create_concat_op) on the line of the token after it; the
	// node keeps that line so the code after the operation reports it.
	return l.seal(&Node{Op: OpBinary, Line: n.Line, FoldLine: n.FoldLine, Text: op, Kids: []*Node{left, right}})
}

// --- calls ---

// args lowers an argument list, checking the ordering rules of
// zend_compile_args. A nil result with callable set marks f(...).
func (l *lowerer) args(n *frontend.Node, line int) []*Arg {
	return l.argsFor(n, line, nil)
}

func (l *lowerer) argsFor(n *frontend.Node, line int, callee *Func) []*Arg {
	var out []*Arg
	named, unpacked := false, false
	for _, a := range n.Children {
		arg := &Arg{}
		v := a
		switch a.Kind {
		case "spread":
			if named {
				l.fail(l.cur, "Cannot use argument unpacking after named arguments")
			}
			unpacked = true
			arg.Spread = true
			v = a.Children[0]
		case "named_arg":
			named = true
			arg.Name = a.Text
			v = a.Children[0]
		default:
			if unpacked {
				l.fail(l.cur, "Cannot use positional argument after argument unpacking")
			}
			if named {
				l.fail(l.cur, "Cannot use positional argument after named argument")
			}
		}
		byValue := !unpacked && l.knownValueArgument(callee, arg.Name, len(out))
		if !arg.Spread && !byValue && isVariableLike(v) && !isThis(unparen(v)) {
			arg.MayRef = true
			arg.Value = l.sendVar(unparen(v))
		} else {
			arg.Value = l.expr(v)
		}
		out = append(out, arg)
	}
	_ = line
	return out
}

// Only completed, unconditional declarations can determine argument modes.
// Unknown names and arguments following unpacking remain runtime decisions.
func (l *lowerer) knownValueArgument(f *Func, name string, position int) bool {
	if f == nil {
		return false
	}
	if name != "" {
		if l.namedParamRefs == nil {
			l.namedParamRefs = make(map[*Func]map[string]bool)
		}
		m := l.namedParamRefs[f]
		if m == nil {
			m = make(map[string]bool, len(f.Params))
			for _, p := range f.Params {
				m[p.Name] = p.ByRef
			}
			l.namedParamRefs[f] = m
		}
		byRef, known := m[name]
		return known && !byRef
	}
	if position < len(f.Params) {
		return !f.Params[position].ByRef
	}
	if len(f.Params) != 0 {
		last := f.Params[len(f.Params)-1]
		if last.Variadic {
			return !last.ByRef
		}
	}
	return true
}

// sendVar lowers a variable argument whose passing mode depends on the
// callee: it is read normally, but the variable may become a reference.
func (l *lowerer) sendVar(v *frontend.Node) *Node {
	nd := l.exprCtx(v, ctxArg)
	root := nd
	for root.Op == OpDim || root.Op == OpProp {
		root = root.Kids[0]
	}
	if root.Op == OpVar {
		root.Var.Flags |= VarMayRef
	}
	return nd
}

// scopeFunctions are functions that read or write the caller's local
// variables or arguments by name.
var scopeFunctions = map[string]string{
	"extract": "scope", "compact": "scope", "get_defined_vars": "scope", "parse_str": "scope",
	"func_get_args": "args", "func_get_arg": "args", "func_num_args": "args",
}

func (l *lowerer) call(n *frontend.Node) *Node {
	// (foo)() calls the value of the constant foo, so only a bare name is a
	// direct function call.
	callee, argList := n.Children[0], n.Children[1]
	nd := &Node{Line: n.Line}
	if callee.Kind == "name" {
		nd.Op = OpCall
		nd.Name = l.ns.resolveFunction(callee.Text)
		base := phpname.Lower(nd.Name.Resolved)
		if nd.Name.Fallback != "" {
			base = phpname.Lower(nd.Name.Fallback)
		}
		switch scopeFunctions[base] {
		case "scope":
			l.fn.DynamicScope = true
		case "args":
			l.fn.UsesArgs = true
		}
	} else {
		nd.Op = OpCallValue
		nd.Kids = []*Node{l.expr(callee)}
	}
	if argList.Kind == "callable_args" {
		return l.node(OpCallable, n.Line, l.seal(nd))
	}
	var known *Func
	if nd.Name != nil {
		known = l.hoisted[phpname.Lower(nd.Name.Resolved)]
	}
	nd.Args = l.argsFor(argList, n.Line, known)
	return l.seal(nd)
}

func (l *lowerer) methodCall(n *frontend.Node) *Node {
	member, argList := n.Children[0], n.Children[1]
	nd := &Node{Op: OpMethodCall, Line: n.Line, Kids: []*Node{nil, nil}}
	if member.Kind == "nullsafe_prop" {
		nd.Op = OpNullsafeMC
	}
	if argList.Kind == "callable_args" && shortCircuited(member) {
		l.fail(n.Line, "Cannot combine nullsafe operator with Closure creation")
	}
	obj := unparen(member.Children[0])
	if isThis(obj) {
		nd.Kids[0] = l.thisNode(obj.Line)
	} else {
		nd.Kids[0] = l.expr(obj)
	}
	l.methodName(nd, member.Children[1], n.Line)
	if argList.Kind == "callable_args" {
		return l.node(OpCallable, n.Line, l.seal(nd))
	}
	nd.Args = l.args(argList, n.Line)
	return l.seal(nd)
}

func (l *lowerer) methodName(nd *Node, name *frontend.Node, _ int) {
	if name.Kind == "identifier" {
		nd.Text = name.Text
		return
	}
	nd.Kids[1] = l.expr(name)
	// zend_compile_method_call and zend_compile_static_call reject a name
	// compiled to a constant other than a string.
	if v, known := l.probeConstant(nd.Kids[1]); known && (v.array || v.value != nil && v.value.Kind != LitString) {
		l.fail(l.cur, "Method name must be a string")
	}
}

func (l *lowerer) staticCall(n *frontend.Node) *Node {
	ref, argList := n.Children[0], n.Children[1]
	if hook := l.parentHookCall(n); hook != nil {
		return hook
	}
	nd := &Node{Op: OpStaticCall, Line: n.Line, Kids: []*Node{nil, nil}}
	l.classOperand(nd, ref.Children[0])
	switch ref.Kind {
	case "class_name":
		nd.Text = "class" // A::class() calls a method named class
	case "static_prop":
		// A::$m() takes the method name from a variable.
		nd.Kids[1] = l.expr(ref.Children[1])
	default:
		l.methodName(nd, ref.Children[1], n.Line)
	}
	if argList.Kind == "callable_args" {
		return l.node(OpCallable, n.Line, l.seal(nd))
	}
	nd.Args = l.args(argList, n.Line)
	return l.seal(nd)
}

func (l *lowerer) newExpr(n *frontend.Node) *Node {
	nd := &Node{Op: OpNew, Line: n.Line, Kids: []*Node{nil}}
	l.classOperand(nd, n.Children[0])
	if a := n.Children[1]; a != nil {
		if a.Kind == "callable_args" {
			l.fail(n.Line, "Cannot create Closure for new expression")
		}
		nd.Args = l.args(a, n.Line)
	}
	return l.seal(nd)
}

func (l *lowerer) closure(n *frontend.Node) *Node {
	c := n.Children
	kind, op := FuncClosure, OpClosure
	params, ret := c[2], c[3]
	var uses, body *frontend.Node
	if n.Kind == "arrow_function" {
		kind, op = FuncArrow, OpArrowFn
		body = c[4]
	} else {
		uses, ret, body = c[3], c[4], c[5]
	}
	f := l.newFunc(kind, l.closureName(n.Line), n.Line)
	f.Parent = l.fn
	f.ByRef = n.Text == "&"
	f.Static = c[1] != nil
	if uses != nil {
		for _, u := range uses.Children {
			name := u.Children[0].Text[1:]
			if name == "this" || isSuperGlobal(name) {
				continue
			}
			if u.Text == "&" {
				l.variable(name, VarRef|VarWritten)
			} else {
				l.variable(name, VarRead)
			}
		}
	}
	if kind == FuncArrow {
		for _, name := range implicitBinds(params, body) {
			f.Captures = append(f.Captures, &Capture{Name: name, Implicit: true, Line: n.Line})
			l.variable(name, VarRead)
		}
	}
	l.functionBody(f, params, ret, body, uses)
	for _, cp := range f.Captures {
		if v := f.vars[cp.Name]; v != nil {
			v.Flags |= VarCaptured
		}
	}
	return l.seal(&Node{Op: op, Line: n.Line, Func: f})
}

func (l *lowerer) match(n *frontend.Node) *Node {
	nd := &Node{Op: OpMatch, Line: n.Line, Kids: []*Node{l.expr(n.Children[0])}}
	seenDefault := false
	for _, arm := range n.Children[1:] {
		a := &Node{Op: OpArm, Line: arm.Line}
		if arm.Children[0] == nil {
			if seenDefault {
				l.fail(arm.Line, "Match expressions may only contain one default arm")
			}
			seenDefault = true
		}
		a.Kids = []*Node{nil}
		if conds := arm.Children[0]; conds != nil {
			for _, e := range conds.Children {
				a.Kids = append(a.Kids, l.expr(e))
			}
		}
		a.Kids[0] = l.expr(arm.Children[1])
		nd.Kids = append(nd.Kids, l.seal(a))
	}
	return l.seal(nd)
}

// constExpr lowers a default value or constant initializer after checking
// it is a constant expression. dynamic allows new and object casts, as in
// parameter defaults and global constants.
func (l *lowerer) constExpr(n *frontend.Node, dynamic bool) *Node {
	if l.constants == nil {
		l.constants = l.syntaxBudget(n)
		defer func() { l.constants = nil }()
	}
	l.preEvaluateSyntax(n)
	if l.constClosures == nil {
		l.constClosures = make(map[*frontend.Node]*Node)
		defer func() { l.constClosures = nil }()
	}
	l.checkConstExpr(n, dynamic)
	saved := l.inConstExpr
	line := l.cur
	l.inConstExpr = true
	// Lowering the value builds HIR, while PHP keeps the enclosing declaration
	// location when it evaluates successive constant initializers.
	defer func() { l.inConstExpr, l.cur = saved, line }()
	return l.expr(n)
}

// truthy converts a literal to bool as PHP does.
func truthy(v *Literal) bool {
	switch v.Kind {
	case LitBool:
		return v.Bool
	case LitInt:
		return v.Int != 0
	case LitFloat:
		return v.Float != 0
	case LitString:
		return v.Str != "" && v.Str != "0"
	}
	return false
}

// parentHookCall recognizes parent::$prop::get() and parent::$prop::set(),
// which call the parent class's property hook, as
// zend_compile_parent_property_hook_call.
func (l *lowerer) parentHookCall(n *frontend.Node) *Node {
	ref := n.Children[0]
	if ref.Kind != "class_const" || ref.Children[1].Kind != "identifier" {
		return nil
	}
	sp := ref.Children[0]
	if sp.Kind != "static_prop" || sp.Children[0].Kind != "name" || specialClass(sp.Children[0].Text) != "parent" {
		return nil
	}
	var prop string
	if p := sp.Children[1]; p.Kind == "variable" {
		prop = p.Text[1:]
	} else {
		if len(p.Children) != 1 || unparen(p.Children[0]).Kind != "literal" {
			return nil
		}
		v := l.expr(p.Children[0])
		var ok bool
		if v.Value == nil {
			return nil
		}
		prop, ok = constString(v.Value)
		if !ok {
			return nil
		}
	}
	method := phpname.Lower(ref.Children[1].Text)
	if method != "get" && method != "set" {
		return nil
	}
	if l.class == nil {
		l.fail(n.Line, "Cannot use \"parent\" when no class scope is active")
	}
	if n.Children[1].Kind == "callable_args" {
		l.fail(n.Line, "Cannot create Closure for parent property hook call")
	}
	hook := ref.Children[1].Text
	f := l.fn
	if f.Kind != FuncHook {
		l.fail(n.Line, "Must not use parent::$%s::%s() outside a property hook", prop, hook)
	}
	if prop != f.property {
		l.fail(n.Line, "Must not use parent::$%s::%s() in a different property ($%s)", prop, hook, f.property)
	}
	if active := f.Name[strings.Index(f.Name, "::")+2:]; active != method {
		l.fail(n.Line, "Must not use parent::$%s::%s() in a different property hook (%s)", prop, hook, active)
	}
	nd := &Node{Op: OpStaticCall, Line: n.Line, Name: &Name{Kind: ClassName, Resolved: "parent", Special: "parent"},
		Text: "$" + prop + "::" + method, Kids: []*Node{nil, nil}}
	nd.Args = l.args(n.Children[1], n.Line)
	return l.seal(nd)
}

// constExprKinds are the syntax kinds zend_is_allowed_in_const_expr accepts.
var constExprKinds = map[string]bool{
	"literal": true, "binary": true, "unary": true, "cast": true, "ternary": true, "dim": true,
	"array": true, "item": true, "spread": true, "name": true, "class_const": true, "class_name": true,
	"magic_const": true, "new": true, "args": true, "named_arg": true, "prop": true, "nullsafe_prop": true,
	"closure": true, "call": true, "static_call": true, "callable_args": true, "identifier": true,
}

// checkConstExpr validates a constant expression as zend_compile_const_expr.
func (l *lowerer) checkConstExpr(n *frontend.Node, dynamic bool) {
	if n == nil {
		return
	}
	if l.constants == nil {
		l.constants = l.syntaxBudget(n)
		defer func() { l.constants = nil }()
	}
	if l.preEvaluateSyntax(n).known {
		return
	}
	n = l.selectedSyntax(n)
	switch n.Kind {
	case "interpolated", "heredoc":
		for _, p := range n.Children {
			if p.Kind != "string_part" && p.Kind != "heredoc_end" {
				l.fail(n.Line, "Constant expression contains invalid operations")
			}
		}
		if n.Text == "`" {
			l.fail(n.Line, "Constant expression contains invalid operations")
		}
		return
	}
	if n.Kind == "new_class" {
		if !dynamic {
			l.fail(n.Line, "New expressions are not supported in this context")
		}
		l.fail(n.Line, "Cannot use anonymous class in constant expression")
	}
	if !constExprKinds[n.Kind] || n.Kind == "binary" && n.Text == "|>" || n.Kind == "unary" && n.Text == "@" {
		l.fail(n.Line, "Constant expression contains invalid operations")
	}
	c := n.Children
	switch n.Kind {
	case "array":
		for _, item := range c {
			if item == nil {
				l.fail(n.Line, "Cannot use empty array elements in arrays")
			}
		}
	case "class_const":
		cls := unparen(c[0])
		if cls.Kind == "array" || cls.Kind == "literal" && cls.Token != frontend.TConstantEncapsedString {
			l.fail(n.Line, "Class name must be a valid object or a string")
		}
		// The class must be a zval: a name, or a string the parser folded.
		// Parentheses around a name make it a constant fetch.
		if _, isString, ok := parseTimeValue(cls); c[0].Kind != "name" && !(ok && isString) {
			l.fail(n.Line, "Dynamic class names are not allowed in compile-time class constant references")
		}
		if c[0].Kind != "name" {
			l.checkStringClassName(cls)
		}
		if specialClass(cls.Text) == "static" {
			l.fail(n.Line, "\"static::\" is not allowed in compile-time constants")
		}
		if c[1].Kind != "identifier" {
			l.checkConstExpr(c[1], dynamic)
		}
		return
	case "class_name":
		cls := unparen(c[0])
		v, isString, ok := parseTimeValue(cls)
		if c[0].Kind != "name" && !(ok && isString) {
			l.fail(n.Line, "(expression)::class cannot be used in constant expressions")
		}
		special := specialClass(cls.Text)
		if c[0].Kind != "name" {
			// zend_try_compile_const_expr_resolve_class_name checks a
			// parser-held string before zend_compile_const_expr_class_name.
			if special = specialClass(v); special == "" {
				l.fullyQualifiedClass(v)
			} else {
				l.ensureClassFetch(special, n.Line)
			}
		}
		if special == "static" {
			l.fail(n.Line, "static::class cannot be used for compile-time class name resolution")
		}
		if special == "parent" && l.class != nil && l.class.Kind != KindTrait && l.class.Parent == nil {
			l.fail(n.Line, "Cannot use \"parent\" when current class scope has no parent")
		}
		return
	case "cast":
		if castNames[phpname.Lower(strings.Trim(n.Text, "() \t"))] == "object" && !dynamic {
			l.fail(n.Line, "Object casts are not supported in this context")
		}
	case "new":
		if !dynamic {
			l.fail(n.Line, "New expressions are not supported in this context")
		}
		l.constClassRef(c[0], n.Line)
		if c[1] != nil {
			if c[1].Kind == "callable_args" {
				l.fail(n.Line, "Cannot create Closure for new expression")
			}
			l.constArgs(c[1], dynamic)
		}
		return
	case "closure":
		if c[1] == nil {
			l.fail(n.Line, "Closures in constant expressions must be static")
		}
		if c[3] != nil {
			l.fail(n.Line, "Cannot use(...) variables in constant expression")
		}
		if l.constClosures != nil {
			// zend_compile_const_expr compiles a closure where it reaches
			// it, so its body reports before later operands.
			saved, line := l.inConstExpr, l.cur
			l.inConstExpr = true
			l.constClosures[n] = l.closure(n)
			l.inConstExpr, l.cur = saved, line
		}
		return
	case "call", "static_call":
		if c[1].Kind != "callable_args" {
			l.fail(n.Line, "Constant expression contains invalid operations")
		}
		if n.Kind == "call" {
			callee := unparen(c[0])
			if c[0].Kind == "paren" && callee.Kind == "name" {
				l.fail(n.Line, "Cannot use dynamic function name in constant expression")
			}
			if callee.Kind == "literal" && callee.Token != frontend.TConstantEncapsedString {
				l.fail(n.Line, "Illegal function name")
			}
			if callee.Kind != "name" {
				l.fail(n.Line, "Cannot use dynamic function name in constant expression")
			}
		} else {
			ref := c[0]
			l.constClassRef(ref.Children[0], n.Line)
			if ref.Children[1].Kind != "identifier" {
				l.fail(n.Line, "Method name must be a string")
			}
		}
		return
	case "args":
		l.constArgs(n, dynamic)
		return
	}
	for _, k := range c {
		if k != nil {
			l.checkConstExpr(k, dynamic)
		}
	}
}

// constClassRef checks the class of new or a static call in a constant
// expression.
func (l *lowerer) constClassRef(cls *frontend.Node, line int) {
	cls = unparen(cls)
	if cls.Kind != "name" {
		l.fail(line, "Cannot use dynamic class name in constant expression")
	}
	if specialClass(cls.Text) == "static" {
		l.fail(line, "\"static\" is not allowed in compile-time constants")
	}
}

func (l *lowerer) constArgs(args *frontend.Node, dynamic bool) {
	named := false
	for _, a := range args.Children {
		switch a.Kind {
		case "spread":
			l.fail(a.Line, "Argument unpacking in constant expressions is not supported")
		case "named_arg":
			named = true
			l.checkConstExpr(a.Children[0], dynamic)
			continue
		}
		if named {
			l.fail(a.Line, "Cannot use positional argument after named argument")
		}
		l.checkConstExpr(a, dynamic)
	}
}

// zline is the line PHP's AST gives a node: a node with operands takes the
// line of its first operand, a declaration or leaf its own line.
// zline caches the result for every node on the operand spine it walks,
// so lowering a long left-nested chain such as 1 + 1 + ... stays linear.
func (l *lowerer) zline(n *frontend.Node) int {
	if l.zlines == nil {
		l.zlines = map[*frontend.Node]int{}
	}
	var spine []*frontend.Node
	line := 0
	for cur := n; ; {
		if v, ok := l.zlines[cur]; ok {
			line = v
			break
		}
		spine = append(spine, cur)
		plain := unparen(cur)
		if plain.LeadingEmptyLine != 0 {
			line = plain.LeadingEmptyLine
			break
		}
		next := zlineStep(plain)
		if next == nil {
			line = unparen(cur).Line
			break
		}
		cur = next
	}
	for _, s := range spine {
		l.zlines[s] = line
	}
	return line
}

// zlineStep returns the operand whose line a node takes, or nil when the
// node's own line is used.
func zlineStep(n *frontend.Node) *frontend.Node {
	switch n.Kind {
	case "closure", "arrow_function", "function", "class", "interface", "trait", "enum", "new_class",
		"literal", "variable", "name", "magic_const", "interpolated", "heredoc", "match":
		return nil
	case "array", "list":
		for _, c := range n.Children {
			if c != nil {
				return c
			}
		}
		return nil
	case "yield":
		return n.Children[1]
	case "item":
		return n.Children[1]
	}
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[0]
}

func zline(n *frontend.Node) int {
	for {
		n = unparen(n)
		if n.LeadingEmptyLine != 0 {
			return n.LeadingEmptyLine
		}
		switch n.Kind {
		case "closure", "arrow_function", "function", "class", "interface", "trait", "enum", "new_class",
			"literal", "variable", "name", "magic_const", "interpolated", "heredoc", "match":
			return n.Line
		case "array", "list":
			for _, c := range n.Children {
				if c != nil {
					return zline(c)
				}
			}
			return n.Line
		case "yield":
			if n.Children[1] != nil {
				n = n.Children[1]
				continue
			}
			return n.Line
		case "item":
			n = n.Children[1]
			continue
		}
		if len(n.Children) == 0 || n.Children[0] == nil {
			return n.Line
		}
		n = n.Children[0]
	}
}

// checkClassName rejects \self and namespace\self forms, as
// zend_resolve_class_name.
func (l *lowerer) checkClassName(n *frontend.Node) {
	switch form, rest := formOf(n.Text); form {
	case fullyQualified:
		if specialClass(rest) != "" {
			l.fail(n.Line, "'\\%s' is an invalid class name", rest)
		}
	case relative:
		if specialClass(rest) != "" {
			l.fail(n.Line, "'namespace\\%s' is an invalid class name", rest)
		}
	}
}

// typeName is zend_zval_type_name for a literal.
func typeName(v *Literal) string {
	return [...]string{LitNull: "null", LitBool: "bool", LitInt: "int", LitFloat: "float", LitString: "string"}[v.Kind]
}

// constKind is the PHP type name of a default value PHP folds to a
// constant, or "" when the value is computed at run time.
func constKind(n *Node) string {
	switch n.Op {
	case OpLiteral:
		if n.Value.Kind == LitBool {
			return "bool"
		}
		return typeName(n.Value)
	case OpUnary:
		if (n.Text == "-" || n.Text == "+") && lit(n.Kids[0]) != nil && numeric(lit(n.Kids[0])) {
			return typeName(lit(n.Kids[0]))
		}
	case OpArray:
		for _, k := range n.Kids {
			if k == nil || k.Op != OpItem || k.Flags&FlagRef != 0 {
				return ""
			}
			for _, part := range k.Kids {
				if part != nil && constKind(part) == "" {
					return ""
				}
			}
		}
		return "array"
	}
	return ""
}

// validDefault mirrors zend_is_valid_default_value for a constant default.
func validDefault(typ string, v *Literal, kind string) bool {
	parts := map[string]bool{}
	for _, p := range strings.Split(strings.TrimPrefix(typ, "?"), "|") {
		parts[phpname.Lower(strings.Trim(p, "()"))] = true
	}
	if strings.HasPrefix(typ, "?") {
		parts["null"] = true
	}
	if parts["mixed"] {
		return true
	}
	switch kind {
	case "bool":
		return parts["bool"] || v != nil && v.Bool && parts["true"] || v != nil && !v.Bool && parts["false"]
	case "int":
		return parts["int"] || parts["float"]
	}
	return parts[kind]
}
