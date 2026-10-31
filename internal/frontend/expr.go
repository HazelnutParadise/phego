package frontend

// Binding powers follow PHP 8.5.12's operator precedence, lowest first.
const (
	precLowest     = 1
	precOr         = 1
	precXor        = 2
	precAnd        = 3
	precPrint      = 4
	precYield      = 5
	precYieldFrom  = 7
	precAssign     = 8
	precTernary    = 9
	precCoalesce   = 10
	precBoolOr     = 11
	precBoolAnd    = 12
	precBitOr      = 13
	precBitXor     = 14
	precBitAnd     = 15
	precEquality   = 16
	precCompare    = 17
	precPipe       = 18
	precConcat     = 19
	precShift      = 20
	precAdditive   = 21
	precMultiply   = 22
	precNot        = 23
	precInstanceof = 24
	precUnary      = 25
	precPow        = 26
)

type binaryOp struct {
	prec  int
	right bool
	none  bool // non-associative
}

var binaryOps = map[Kind]binaryOp{
	TLogicalOr: {prec: precOr}, TLogicalXor: {prec: precXor}, TLogicalAnd: {prec: precAnd},
	TCoalesce: {prec: precCoalesce, right: true}, TBooleanOr: {prec: precBoolOr},
	TBooleanAnd: {prec: precBoolAnd}, '|': {prec: precBitOr}, '^': {prec: precBitXor},
	TAmpersandFollowedByVarOrVararg: {prec: precBitAnd}, TAmpersandNotFollowedByVarOrVararg: {prec: precBitAnd},
	TIsEqual: {prec: precEquality, none: true}, TIsNotEqual: {prec: precEquality, none: true},
	TIsIdentical: {prec: precEquality, none: true}, TIsNotIdentical: {prec: precEquality, none: true},
	TSpaceship: {prec: precEquality, none: true},
	'<':        {prec: precCompare, none: true}, '>': {prec: precCompare, none: true},
	TIsSmallerOrEqual: {prec: precCompare, none: true}, TIsGreaterOrEqual: {prec: precCompare, none: true},
	TPipe: {prec: precPipe}, '.': {prec: precConcat}, TSl: {prec: precShift}, TSr: {prec: precShift},
	'+': {prec: precAdditive}, '-': {prec: precAdditive},
	'*': {prec: precMultiply}, '/': {prec: precMultiply}, '%': {prec: precMultiply},
	TPow: {prec: precPow, right: true},
}

var assignOps = map[Kind]bool{
	'=': true, TPlusEqual: true, TMinusEqual: true, TMulEqual: true, TDivEqual: true,
	TConcatEqual: true, TModEqual: true, TAndEqual: true, TOrEqual: true, TXorEqual: true,
	TSlEqual: true, TSrEqual: true, TPowEqual: true, TCoalesceEqual: true,
}

func (p *parser) expr() *Node { return p.exprPrec(precLowest) }

// exprPrec parses an expression whose binary operators bind at least as
// tightly as min.
func (p *parser) exprPrec(min int) *Node {
	p.enter()
	defer p.leave()
	return p.exprRest(p.unary(min), min)
}

// exprRest continues operator parsing after the first operand.
func (p *parser) exprRest(left *Node, min int) *Node {
	for {
		t := p.peek()
		if t.Kind == '?' && precTernary >= min {
			p.next()
			var then *Node
			if !p.at(':') {
				then = p.exprPrec(precLowest)
			}
			p.expect(':')
			left = node("ternary", t.Line, left, then, p.exprPrec(precTernary+1))
			continue
		}
		if t.Kind == TInstanceof && precInstanceof >= min {
			p.next()
			left = node("instanceof", t.Line, left, p.classReference())
			continue
		}
		op, ok := binaryOps[t.Kind]
		if !ok || op.prec < min {
			return left
		}
		p.next()
		next := op.prec + 1
		if op.right {
			next = op.prec
		}
		right := p.exprPrec(next)
		left = &Node{Kind: "binary", Text: t.Text, Line: t.Line, Children: []*Node{left, right}}
		if op.none {
			if n, ok := binaryOps[p.peek().Kind]; ok && n.none && n.prec == op.prec {
				p.unexpected()
			}
		}
	}
}

func isAssignable(n *Node) bool {
	switch n.Kind {
	case "variable", "variable_variable", "dim", "prop", "nullsafe_prop", "static_prop", "list", "array":
		return true
	}
	return false
}

// isAssignTarget also admits calls, which PHP parses on the left of an
// assignment and rejects when compiling.
func isAssignTarget(n *Node) bool {
	switch n.Kind {
	case "call", "method_call", "static_call":
		return true
	}
	return isAssignable(n)
}

func isVariableTarget(n *Node) bool {
	return isAssignTarget(n) && n.Kind != "array" && n.Kind != "list"
}

// referenceTarget parses a PHP variable or call usable after & or prefix ++/--.
func (p *parser) referenceTarget() *Node {
	start := p.peek()
	target := p.postfix(p.primary())
	if !isVariableTarget(target) {
		p.fail(start.Line+countLines(start.Text), "invalid variable target: %s cannot be used here", describe(start))
	}
	return target
}

// canStartExpr reports whether k can begin an expression; yield without a
// value is followed by anything else.
func canStartExpr(k Kind) bool {
	switch k {
	case ';', ')', ',', ']', '}', ':', TCloseTag, TDoubleArrow, tEOF, TAs,
		'*', '/', '%', '.', '|', '^', '<', '>', '=', '?', TPow, TSl, TSr, TPipe,
		TIsEqual, TIsNotEqual, TIsIdentical, TIsNotIdentical, TSpaceship,
		TIsSmallerOrEqual, TIsGreaterOrEqual, TBooleanAnd, TBooleanOr, TCoalesce,
		TLogicalAnd, TLogicalOr, TLogicalXor, TInstanceof:
		return false
	}
	return !assignOps[k]
}

// assignment binds to a variable on its left regardless of the surrounding
// precedence, as in PHP's grammar (`!$a = f()` is `!($a = f())`).
func (p *parser) assignment(left *Node) *Node {
	for {
		t := p.peek()
		if !assignOps[t.Kind] || !isAssignTarget(left) {
			return left
		}
		if left.Kind == "array" || left.Kind == "list" {
			if t.Kind != '=' || left.Kind == "array" && left.Text == "array" || isAmpersand(p.tok(1).Kind) {
				return left
			}
		}
		p.next()
		if t.Kind == '=' && isAmpersand(p.peek().Kind) {
			p.next()
			left = node("assign_ref", t.Line, left, p.referenceTarget())
			continue
		}
		left = &Node{Kind: "assign", Text: t.Text, Line: t.Line, Children: []*Node{left, p.exprPrec(precAssign)}}
	}
}

var castKinds = map[Kind]bool{
	TIntCast: true, TDoubleCast: true, TStringCast: true, TArrayCast: true, TObjectCast: true,
	TBoolCast: true, TUnsetCast: true, TVoidCast: true,
}

func (p *parser) unary(min int) *Node {
	t := p.peek()
	switch {
	case t.Kind == '!':
		p.next()
		return &Node{Kind: "unary", Text: "!", Line: t.Line, Children: []*Node{p.exprPrec(precNot)}}
	case t.Kind == '-' || t.Kind == '+' || t.Kind == '~' || t.Kind == '@':
		p.next()
		return &Node{Kind: "unary", Text: t.Text, Line: t.Line, Children: []*Node{p.exprPrec(precUnary)}}
	case t.Kind == TVoidCast:
		p.unexpected()
	case castKinds[t.Kind]:
		p.next()
		return &Node{Kind: "cast", Text: t.Text, Line: t.Line, Children: []*Node{p.exprPrec(precUnary)}}
	case t.Kind == TInc || t.Kind == TDec:
		p.next()
		return &Node{Kind: "pre_inc", Text: t.Text, Line: t.Line, Children: []*Node{p.referenceTarget()}}
	case t.Kind == TPrint:
		p.next()
		return node("print", t.Line, p.exprPrec(precPrint))
	case t.Kind == TYield:
		p.next()
		if !canStartExpr(p.peek().Kind) {
			return node("yield", t.Line, nil, nil)
		}
		v := p.exprPrec(precYield)
		if p.accept(TDoubleArrow) {
			return node("yield", t.Line, v, p.exprPrec(precYield))
		}
		return node("yield", t.Line, nil, v)
	case t.Kind == TYieldFrom:
		p.next()
		return node("yield_from", t.Line, p.exprPrec(precYieldFrom))
	case t.Kind == TInclude || t.Kind == TIncludeOnce || t.Kind == TRequire || t.Kind == TRequireOnce:
		p.next()
		return &Node{Kind: "include", Text: t.Text, Line: t.Line, Children: []*Node{p.exprPrec(precLowest)}}
	case t.Kind == TThrow:
		p.next()
		return node("throw", t.Line, p.exprPrec(precLowest))
	case t.Kind == TClone:
		p.next()
		if p.at('(') {
			// clone(...) takes arguments in PHP 8.5, but a single
			// parenthesized operand may continue with member access.
			args := p.arguments()
			if len(args.Children) == 1 && args.Children[0].Kind != "spread" && args.Children[0].Kind != "named_arg" &&
				p.atAny(TObjectOperator, TNullsafeObjectOperator, TDoubleColon, '[') {
				return node("clone", t.Line, p.postfix(node("paren", args.Line, args.Children[0])))
			}
			return node("clone", t.Line, args)
		}
		return node("clone", t.Line, p.exprPrec(precUnary+2))
	case t.Kind == TAttribute:
		return p.exprWithAttributes(p.attributes())
	}
	return p.assignment(p.postfix(p.primary()))
}

// exprWithAttributes parses the closure or arrow function the attributes
// belong to.
func (p *parser) exprWithAttributes(attrs *Node) *Node {
	t := p.peek()
	switch t.Kind {
	case TFunction, TFn:
		return p.closure(attrs, nil)
	case TStatic:
		if k := p.tok(1).Kind; k == TFunction || k == TFn {
			p.next()
			return p.closure(attrs, leaf("modifier", t))
		}
	}
	p.unexpected()
	return nil
}

func (p *parser) primary() *Node {
	p.enter()
	defer p.leave()
	t := p.peek()
	switch t.Kind {
	case TVariable:
		p.next()
		return leaf("variable", t)
	case '$':
		return p.simpleVariable()
	case TLNumber, TDNumber, TConstantEncapsedString:
		p.next()
		return leaf("literal", t)
	case TLine, TFile, TDir, TClassC, TTraitC, TMethodC, TFuncC, TNsC, TPropertyC:
		p.next()
		return leaf("magic_const", t)
	case '"', '`':
		return p.interpolated(t.Kind)
	case TStartHeredoc:
		return p.heredoc()
	case '(':
		p.next()
		e := p.expr()
		p.expect(')')
		return node("paren", t.Line, e)
	case '[':
		return p.arrayLiteral('[', ']', "array")
	case TArray:
		if p.tok(1).Kind == '(' {
			p.next()
			n := p.arrayLiteral('(', ')', "array")
			n.Text = "array"
			return n
		}
	case TList:
		p.next()
		n := p.arrayLiteral('(', ')', "list")
		if !p.inTarget && !p.at('=') {
			p.unexpected()
		}
		return n
	case TIsset:
		p.next()
		p.expect('(')
		n := node("isset", t.Line)
		for !p.at(')') {
			n.Children = append(n.Children, p.expr())
			if !p.accept(',') {
				break
			}
		}
		if len(n.Children) == 0 {
			p.unexpected()
		}
		p.expect(')')
		return n
	case TEmpty, TEval:
		p.next()
		p.expect('(')
		e := p.expr()
		p.expect(')')
		return &Node{Kind: "builtin", Text: t.Text, Line: t.Line, Children: []*Node{e}}
	case TExit:
		p.next()
		n := &Node{Kind: "exit", Text: t.Text, Line: t.Line}
		if p.at('(') {
			n.Children = []*Node{p.arguments()}
		}
		return n
	case TNew:
		n := p.newExpr(nil)
		if n.Children[1] == nil {
			p.noPostfix = true
		}
		return n
	case TFunction, TFn:
		return p.closure(nil, nil)
	case TStatic:
		switch p.tok(1).Kind {
		case TFunction, TFn:
			p.next()
			return p.closure(nil, leaf("modifier", t))
		case TDoubleColon:
			p.next()
			return leaf("name", t)
		}
	case TMatch:
		if p.tok(1).Kind == '(' {
			return p.match()
		}
	case TReadonly:
		if p.tok(1).Kind == '(' {
			p.next()
			return leaf("name", t)
		}
	}
	if isName(t.Kind) || (t.Kind == TNamespace && p.tok(1).Kind == TNsSeparator) {
		p.next()
		return leaf("name", t)
	}
	if isIdentifier(t.Kind) && p.tok(1).Kind == TDoubleColon {
		// parent::, self:: and similar are plain identifiers.
		p.next()
		return leaf("name", t)
	}
	p.unexpected()
	return nil
}

func (p *parser) simpleVariable() *Node {
	p.enter()
	defer p.leave()
	t := p.peek()
	switch t.Kind {
	case TVariable:
		p.next()
		return leaf("variable", t)
	case '$':
		p.next()
		if p.accept('{') {
			e := p.expr()
			p.expect('}')
			return node("variable_variable", t.Line, e)
		}
		return node("variable_variable", t.Line, p.simpleVariable())
	}
	p.unexpected()
	return nil
}

func (p *parser) postfix(n *Node) *Node {
	if p.noPostfix {
		p.noPostfix = false
		return n
	}
	// PHP requires parentheses before postfix access on these expressions.
	if n.Kind == "match" || n.Kind == "closure" || n.Kind == "heredoc" || n.Kind == "list" {
		return n
	}
	for {
		t := p.peek()
		if n.Kind == "literal" && !callable(n) && (t.Kind == '[' || t.Kind == TObjectOperator || t.Kind == TNullsafeObjectOperator || t.Kind == TDoubleColon) {
			return n
		}
		if n.Kind == "magic_const" && t.Kind == TDoubleColon {
			return n
		}
		switch t.Kind {
		case '[':
			p.next()
			var idx *Node
			if !p.at(']') {
				idx = p.expr()
			}
			p.expect(']')
			n = node("dim", t.Line, n, idx)
		case TObjectOperator, TNullsafeObjectOperator:
			p.next()
			kind := "prop"
			if t.Kind == TNullsafeObjectOperator {
				kind = "nullsafe_prop"
			}
			n = node(kind, t.Line, n, p.memberName())
			if p.at('(') {
				n = node("method_call", t.Line, n, p.arguments())
			}
		case TDoubleColon:
			p.next()
			m := p.peek()
			switch {
			case m.Kind == TVariable || m.Kind == '$':
				n = node("static_prop", t.Line, n, p.simpleVariable())
			case m.Kind == TClass:
				p.next()
				n = node("class_name", t.Line, n)
			case m.Kind == '{':
				p.next()
				e := p.expr()
				p.expect('}')
				n = node("class_const", t.Line, n, e)
			default:
				n = node("class_const", t.Line, n, p.identifier())
			}
			if p.at('(') {
				n = node("static_call", t.Line, n, p.arguments())
			}
		case '(':
			if !callable(n) {
				return n
			}
			n = node("call", t.Line, n, p.arguments())
		case TInc, TDec:
			if !isVariableTarget(n) {
				return n
			}
			p.next()
			return &Node{Kind: "post_inc", Text: t.Text, Line: t.Line, Children: []*Node{n}}
		default:
			return n
		}
	}
}

func callable(n *Node) bool {
	switch n.Kind {
	case "literal":
		// A binary prefix (b or B) does not change callability.
		text := n.Text
		if text != "" && (text[0] == 'b' || text[0] == 'B') {
			text = text[1:]
		}
		return text != "" && (text[0] == '\'' || text[0] == '"')
	case "magic_const", "isset", "builtin", "exit", "print", "yield", "post_inc", "class_name":
		return false
	}
	return true
}

func (p *parser) memberName() *Node {
	t := p.peek()
	switch {
	case t.Kind == TVariable || t.Kind == '$':
		return p.simpleVariable()
	case t.Kind == '{':
		p.next()
		e := p.expr()
		p.expect('}')
		return e
	case isIdentifier(t.Kind):
		p.next()
		return leaf("identifier", t)
	}
	p.unexpected()
	return nil
}

func (p *parser) arguments() *Node {
	t := p.expect('(')
	n := node("args", t.Line)
	if p.at(TEllipsis) && p.tok(1).Kind == ')' {
		p.next()
		p.next()
		return node("callable_args", t.Line)
	}
	for !p.at(')') {
		a := p.peek()
		switch {
		case p.accept(TEllipsis):
			n.Children = append(n.Children, node("spread", a.Line, p.expr()))
		case isIdentifier(a.Kind) && p.tok(1).Kind == ':' && p.tok(2).Kind != ':':
			p.next()
			p.next()
			n.Children = append(n.Children, &Node{Kind: "named_arg", Text: a.Text, Line: a.Line, Children: []*Node{p.expr()}})
		default:
			n.Children = append(n.Children, p.expr())
		}
		if !p.accept(',') {
			break
		}
	}
	p.expect(')')
	return n
}

func (p *parser) arrayLiteral(open, close Kind, kind string) *Node {
	t := p.expect(open)
	n := node(kind, t.Line)
	// Nested list() is valid inside any array or list literal.
	saved := p.inTarget
	p.inTarget = true
	defer func() { p.inTarget = saved }()
	for !p.at(close) {
		if p.at(',') {
			p.next()
			n.Children = append(n.Children, nil)
			continue
		}
		n.Children = append(n.Children, p.arrayItem())
		if !p.accept(',') {
			break
		}
	}
	p.expect(close)
	return n
}

func (p *parser) arrayItem() *Node {
	t := p.peek()
	if p.accept(TEllipsis) {
		return node("spread", t.Line, p.expr())
	}
	if isAmpersand(t.Kind) {
		p.next()
		return node("item", t.Line, nil, node("by_ref", t.Line, p.referenceTarget()))
	}
	v := p.expr()
	if p.accept(TDoubleArrow) {
		if r := p.peek(); isAmpersand(r.Kind) {
			p.next()
			return node("item", t.Line, v, node("by_ref", r.Line, p.referenceTarget()))
		}
		return node("item", t.Line, v, p.expr())
	}
	return node("item", t.Line, nil, v)
}

// classReference parses the right side of instanceof and the class of new.
func (p *parser) classReference() *Node {
	t := p.peek()
	switch {
	case t.Kind == TStatic || isName(t.Kind):
		p.next()
		n := leaf("name", t)
		if !p.at(TDoubleColon) {
			return n
		}
		return p.classReferenceSuffix(n)
	case t.Kind == '(':
		p.next()
		e := p.expr()
		p.expect(')')
		return node("paren", t.Line, e)
	case t.Kind == TVariable || t.Kind == '$':
		return p.classReferenceSuffix(p.simpleVariable())
	}
	p.unexpected()
	return nil
}

func (p *parser) classReferenceSuffix(n *Node) *Node {
	for {
		s := p.peek()
		switch s.Kind {
		case '[':
			p.next()
			var idx *Node
			if !p.at(']') {
				idx = p.expr()
			}
			p.expect(']')
			n = node("dim", s.Line, n, idx)
		case TObjectOperator, TNullsafeObjectOperator:
			p.next()
			kind := "prop"
			if s.Kind == TNullsafeObjectOperator {
				kind = "nullsafe_prop"
			}
			n = node(kind, s.Line, n, p.memberName())
		case TDoubleColon:
			p.next()
			n = node("static_prop", s.Line, n, p.simpleVariable())
		default:
			return n
		}
	}
}

func (p *parser) newExpr(attrs *Node) *Node {
	t := p.expect(TNew)
	if p.at(TAttribute) {
		attrs = p.attributes()
	}
	mods := node("modifiers", p.peek().Line)
	for p.atAny(TReadonly, TFinal, TAbstract) && p.tok(1).Kind != '(' {
		mods.Children = append(mods.Children, leaf("modifier", p.next()))
	}
	if len(mods.Children) > 0 && !p.at(TClass) {
		p.unexpected()
	}
	p.checkClassModifiers(mods, true)
	if p.at(TClass) {
		c := p.next()
		var args *Node
		if p.at('(') {
			args = p.arguments()
		}
		var extends, implements *Node
		if p.accept(TExtends) {
			extends = p.name()
		}
		if p.accept(TImplements) {
			implements = p.nameList()
		}
		return node("new_class", c.Line, attrs, mods, args, extends, implements, p.classBody())
	}
	class := p.classReference()
	var args *Node
	if p.at('(') {
		args = p.arguments()
	}
	// PHP 8.4 allows member access directly on new only with arguments.
	return node("new", t.Line, class, args)
}

func (p *parser) closure(attrs, static *Node) *Node {
	t := p.next()
	byRef := p.byRef()
	params := p.parameters()
	if t.Kind == TFn {
		ret := p.returnType()
		p.expect(TDoubleArrow)
		return &Node{Kind: "arrow_function", Text: byRef, Line: t.Line, Children: []*Node{attrs, static, params, ret, p.exprPrec(precLowest)}}
	}
	var uses *Node
	if p.at(TUse) {
		u := p.next()
		uses = node("uses", u.Line)
		p.expect('(')
		if p.at(')') {
			p.unexpected()
		}
		for !p.at(')') {
			ref := p.byRef()
			v := p.expect(TVariable)
			uses.Children = append(uses.Children, &Node{Kind: "use_var", Text: ref, Line: v.Line, Children: []*Node{leaf("variable", v)}})
			if !p.accept(',') {
				break
			}
		}
		p.expect(')')
	}
	ret := p.returnType()
	return &Node{Kind: "closure", Text: byRef, Line: t.Line, Children: []*Node{attrs, static, params, uses, ret, p.block()}}
}

func (p *parser) match() *Node {
	t := p.next()
	subject := p.parenExpr()
	p.expect('{')
	n := node("match", t.Line, subject)
	for !p.at('}') {
		arm := node("match_arm", p.peek().Line)
		if p.accept(TDefault) {
			arm.Children = append(arm.Children, nil)
			p.accept(',')
		} else {
			conds := node("conditions", p.peek().Line)
			for !p.at(TDoubleArrow) {
				conds.Children = append(conds.Children, p.expr())
				if !p.accept(',') {
					break
				}
			}
			arm.Children = append(arm.Children, conds)
		}
		p.expect(TDoubleArrow)
		arm.Children = append(arm.Children, p.expr())
		n.Children = append(n.Children, arm)
		if !p.accept(',') {
			break
		}
	}
	p.expect('}')
	return n
}

// --- strings ---

func (p *parser) interpolated(end Kind) *Node {
	t := p.next()
	n := &Node{Kind: "interpolated", Text: t.Text, Line: t.Line, Children: p.encapsList(end)}
	p.expect(end)
	return n
}

func (p *parser) heredoc() *Node {
	t := p.next()
	n := &Node{Kind: "heredoc", Text: t.Text, Line: t.Line, Children: p.encapsList(TEndHeredoc)}
	// The end token includes the indentation that determines the string value.
	n.Children = append(n.Children, leaf("heredoc_end", p.expect(TEndHeredoc)))
	return n
}

func (p *parser) encapsList(end Kind) []*Node {
	var out []*Node
	for !p.at(end) {
		t := p.peek()
		switch t.Kind {
		case TEncapsedAndWhitespace:
			p.next()
			out = append(out, leaf("string_part", t))
		case TVariable:
			p.next()
			v := leaf("variable", t)
			switch p.peek().Kind {
			case '[':
				p.next()
				off := p.peek()
				var idx *Node
				switch off.Kind {
				case TNumString, TString:
					p.next()
					idx = leaf("literal", off)
				case '-':
					p.next()
					idx = &Node{Kind: "literal", Text: "-" + p.expect(TNumString).Text, Line: off.Line}
				case TVariable:
					p.next()
					idx = leaf("variable", off)
				default:
					p.unexpected()
				}
				p.expect(']')
				v = node("dim", t.Line, v, idx)
			case TObjectOperator, TNullsafeObjectOperator:
				op := p.next()
				kind := "prop"
				if op.Kind == TNullsafeObjectOperator {
					kind = "nullsafe_prop"
				}
				v = node(kind, op.Line, v, leaf("identifier", p.expect(TString)))
			}
			out = append(out, v)
		case TCurlyOpen:
			p.next()
			e := p.referenceTarget()
			p.expect('}')
			out = append(out, e)
		case TDollarOpenCurlyBraces:
			p.next()
			var e *Node
			if p.at(TStringVarname) {
				name := p.next()
				e = leaf("variable_name", name)
				if p.accept('[') {
					idx := p.expr()
					p.expect(']')
					e = node("dim", name.Line, e, idx)
				}
			} else {
				e = node("variable_variable", t.Line, p.expr())
			}
			p.expect('}')
			out = append(out, e)
		default:
			p.unexpected()
		}
	}
	return out
}
