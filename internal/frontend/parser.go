package frontend

import (
	"fmt"
	"strings"
)

// MaxNesting bounds recursive syntax so hostile input cannot exhaust the Go
// stack; deeper input is rejected with a located error. It matches the
// default 10000-entry parser stack of the PHP 8.5.12 reference.
const MaxNesting = 10000

const tEOF Kind = -1

// Parse lexes and parses PHP 8.5.12 source into a syntax tree. A syntax
// error returns *SyntaxError with the line PHP reports for it.
func Parse(src []byte, opts LexOptions) (*Node, error) {
	toks, diags, err := Lex(src, opts)
	if err != nil {
		return nil, err
	}
	p := &parser{fatal: map[int]Diagnostic{}}
	eofLine := 1
	// kept[i] is the parser index of lexer token i, or of the next kept
	// token when i is trivia; a lexer error surfaces when PHP's parser
	// reads that token.
	kept := make([]int, len(toks)+1)
	for i, t := range toks {
		kept[i] = len(p.toks)
		eofLine = t.Line + countLines(t.Text)
		switch t.Kind {
		case TWhitespace, TComment, TDocComment, TOpenTag:
			continue
		}
		p.toks = append(p.toks, t)
	}
	kept[len(toks)] = len(p.toks)
	p.eof = Token{Kind: tEOF, Line: eofLine}
	for _, d := range diags {
		if !d.Fatal {
			continue
		}
		i := kept[min(max(d.Token, 0), len(toks))]
		if _, exists := p.fatal[i]; !exists {
			p.fatal[i] = d
		}
	}
	return p.parse()
}

type parser struct {
	toks  []Token
	pos   int
	depth int
	eof   Token
	fatal map[int]Diagnostic // token index -> lexer error raised when reached
	// inTarget is set while parsing a foreach target, where list() needs
	// no following assignment.
	inTarget bool
	// noPostfix stops member access after new without arguments.
	noPostfix bool
	// allowStatic is set while parsing a return type, the only type
	// position where static is valid.
	allowStatic bool
}

type parseFailure struct{ err *SyntaxError }

func (p *parser) parse() (root *Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(parseFailure)
			if !ok {
				panic(r)
			}
			root, err = nil, f.err
		}
	}()
	root = node("file", 1, p.statements(true, func() bool { return p.at(tEOF) })...)
	return root, nil
}

func (p *parser) fail(line int, format string, args ...any) {
	panic(parseFailure{&SyntaxError{Line: line, Message: fmt.Sprintf(format, args...)}})
}

func (p *parser) tok(i int) Token {
	if d, ok := p.fatal[p.pos+i]; ok && i <= 1 {
		p.fail(d.Line, "%s", d.Message)
	}
	if p.pos+i < len(p.toks) {
		return p.toks[p.pos+i]
	}
	return p.eof
}

func (p *parser) peek() Token    { return p.tok(0) }
func (p *parser) at(k Kind) bool { return p.peek().Kind == k }
func (p *parser) atAny(ks ...Kind) bool {
	k := p.peek().Kind
	for _, x := range ks {
		if k == x {
			return true
		}
	}
	return false
}

func (p *parser) next() Token {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *parser) accept(k Kind) bool {
	if p.at(k) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expect(k Kind) Token {
	if !p.at(k) {
		p.unexpected()
	}
	return p.next()
}

// endStatement accepts ';' or a close tag, which PHP treats as ';'.
// acceptSemi accepts ';' or a close tag, which PHP's parser reads as ';'.
func (p *parser) acceptSemi() bool { return p.accept(';') || p.accept(TCloseTag) }

func (p *parser) endStatement() {
	if !p.accept(';') && !p.accept(TCloseTag) {
		p.unexpected()
	}
}

// unexpected reports the current token. Like PHP, the line is where the
// token ends, which differs for tokens that span lines.
func (p *parser) unexpected() {
	t := p.peek()
	line := t.Line + countLines(t.Text)
	if t.Kind == TCloseTag {
		// PHP's parser sees a close tag as ';' on the line where it starts.
		line = t.Line
	}
	p.fail(line, "syntax error, unexpected %s", describe(t))
}

func describe(t Token) string {
	switch t.Kind {
	case tEOF:
		return "end of file"
	case TVariable:
		return fmt.Sprintf("variable %q", t.Text)
	case TString:
		return fmt.Sprintf("identifier %q", t.Text)
	case TNameQualified:
		return fmt.Sprintf("namespaced name %q", t.Text)
	case TNameFullyQualified:
		return fmt.Sprintf("fully qualified name %q", t.Text)
	case TNameRelative:
		return fmt.Sprintf("namespace-relative name %q", t.Text)
	case TLNumber:
		return fmt.Sprintf("integer %q", t.Text)
	case TDNumber:
		return fmt.Sprintf("floating-point number %q", t.Text)
	case TConstantEncapsedString:
		if strings.HasPrefix(t.Text, "'") || strings.HasPrefix(t.Text, "b'") || strings.HasPrefix(t.Text, "B'") {
			return fmt.Sprintf("single-quoted string %q", strings.Trim(t.Text, "bB'"))
		}
		return fmt.Sprintf("double-quoted string %q", strings.Trim(t.Text, "bB\""))
	case TCloseTag:
		return `token ";"`
	case TInlineHTML:
		return fmt.Sprintf("T_INLINE_HTML %q", t.Text)
	case TStartHeredoc:
		return fmt.Sprintf("heredoc start %q", strings.TrimRight(t.Text, "\r\n"))
	case TBadCharacter:
		return fmt.Sprintf("character 0x%02X", t.Text[0])
	}
	return fmt.Sprintf("token %q", t.Text)
}

func (p *parser) enter() {
	p.depth++
	if p.depth > MaxNesting {
		p.fail(p.peek().Line, "syntax nesting exceeds %d levels", MaxNesting)
	}
}

func (p *parser) leave() { p.depth-- }

func isKeyword(k Kind) bool { return k >= TAbstract && k <= THaltCompiler }

// isIdentifier reports tokens usable where PHP accepts semi-reserved words.
func isIdentifier(k Kind) bool { return k == TString || isKeyword(k) }

func isName(k Kind) bool {
	return k == TString || k == TNameQualified || k == TNameFullyQualified || k == TNameRelative
}

func (p *parser) identifier() *Node {
	t := p.peek()
	if !isIdentifier(t.Kind) {
		p.unexpected()
	}
	p.next()
	return leaf("identifier", t)
}

// name parses a class or namespace name. PHP's grammar also accepts static
// here and rejects it later when compiling.
func (p *parser) name() *Node {
	t := p.peek()
	if !isName(t.Kind) && t.Kind != TStatic {
		p.unexpected()
	}
	p.next()
	return leaf("name", t)
}

// --- statements ---

func (p *parser) statements(top bool, done func() bool) []*Node {
	var out []*Node
	for !done() {
		if p.at(tEOF) {
			p.unexpected()
		}
		if s := p.statement(top); s != nil {
			out = append(out, s)
		}
		if len(out) > 0 && out[len(out)-1].Kind == "halt_compiler" {
			break
		}
	}
	return out
}

func (p *parser) block() *Node {
	t := p.expect('{')
	body := p.statements(false, func() bool { return p.at('}') })
	p.expect('}')
	return node("block", t.Line, body...)
}

// altBody parses statements up to one of the end keywords for alternative syntax.
func (p *parser) altBody(ends ...Kind) *Node {
	line := p.peek().Line
	return node("block", line, p.statements(false, func() bool { return p.atAny(ends...) })...)
}

// singleStatement excludes declarations, which PHP permits in statement lists
// but not as an unbraced control-flow body.
func (p *parser) singleStatement() *Node { return p.statementContext(false, true) }

func (p *parser) statement(top bool) *Node { return p.statementContext(top, false) }

func (p *parser) statementContext(top, single bool) *Node {
	p.enter()
	defer p.leave()
	t := p.peek()
	switch t.Kind {
	case '{':
		return p.block()
	case ';', TCloseTag:
		p.next()
		return nil
	case TInlineHTML:
		p.next()
		return leaf("inline_html", t)
	case TVoidCast:
		// PHP 8.5 accepts (void) only as a whole expression statement.
		p.next()
		e := p.expr()
		p.endStatement()
		return node("void", t.Line, e)
	case TOpenTagWithEcho:
		p.next()
		n := node("echo", t.Line, p.exprList()...)
		p.endStatement()
		return n
	case TIf:
		return p.ifStatement()
	case TWhile:
		p.next()
		cond := p.parenExpr()
		if p.accept(':') {
			body := p.altBody(TEndwhile)
			p.expect(TEndwhile)
			p.endStatement()
			return node("while", t.Line, cond, body)
		}
		return node("while", t.Line, cond, p.singleStatement())
	case TDo:
		p.next()
		body := p.singleStatement()
		p.expect(TWhile)
		cond := p.parenExpr()
		p.endStatement()
		return node("do", t.Line, body, cond)
	case TFor:
		return p.forStatement()
	case TForeach:
		return p.foreachStatement()
	case TSwitch:
		return p.switchStatement()
	case TBreak, TContinue:
		p.next()
		var level *Node
		if !p.atAny(';', TCloseTag) {
			level = p.expr()
		}
		p.endStatement()
		return node(strings.ToLower(t.Text), t.Line, level)
	case TReturn:
		p.next()
		var v *Node
		if !p.atAny(';', TCloseTag) {
			v = p.expr()
		}
		p.endStatement()
		return node("return", t.Line, v)
	case TGlobal:
		p.next()
		n := node("global", t.Line)
		for {
			n.Children = append(n.Children, p.simpleVariable())
			if !p.accept(',') {
				break
			}
		}
		p.endStatement()
		return n
	case TStatic:
		if k := p.tok(1).Kind; k == TVariable {
			return p.staticVars()
		}
	case TEcho:
		p.next()
		n := node("echo", t.Line, p.exprList()...)
		p.endStatement()
		return n
	case TUnset:
		p.next()
		p.expect('(')
		if p.at(')') {
			p.unexpected()
		}
		n := node("unset", t.Line)
		for !p.at(')') {
			v := p.expr()
			if !isVariableTarget(v) {
				p.unexpected()
			}
			n.Children = append(n.Children, v)
			if !p.accept(',') {
				break
			}
		}
		p.expect(')')
		p.endStatement()
		return n
	case TDeclare:
		return p.declareStatement()
	case TTry:
		return p.tryStatement()
	case TGoto:
		p.next()
		label := p.identifierNotReserved()
		p.endStatement()
		return node("goto", t.Line, label)
	case TFunction:
		if k := p.tok(1).Kind; isIdentifier(k) || (isAmpersand(k) && isIdentifier(p.tok(2).Kind)) {
			if single {
				p.next()
				p.byRef()
				p.unexpected()
			}
			return p.functionDecl(nil)
		}
	case TAbstract, TFinal, TReadonly, TClass, TInterface, TTrait:
		if t.Kind == TClass && !isIdentifier(p.tok(1).Kind) && p.tok(1).Kind != '(' {
			// A class declaration needs a name; PHP reports the next token.
			p.next()
			p.unexpected()
		}
		if t.Kind != TReadonly || p.classModifiersAhead() {
			if t.Kind != TClass || isIdentifier(p.tok(1).Kind) {
				if single {
					p.unexpected()
				}
				return p.classDecl(nil)
			}
		}
	case TEnum:
		if single {
			p.unexpected()
		}
		return p.classDecl(nil)
	case TAttribute:
		attrs := p.attributes()
		switch p.peek().Kind {
		case TFunction:
			if k := p.tok(1).Kind; isIdentifier(k) || (isAmpersand(k) && isIdentifier(p.tok(2).Kind)) {
				if single {
					p.next()
					p.byRef()
					p.unexpected()
				}
				return p.functionDecl(attrs)
			}
		case TAbstract, TFinal, TReadonly, TClass, TInterface, TTrait, TEnum:
			if p.peek().Kind == TClass && !isIdentifier(p.tok(1).Kind) {
				p.next()
				p.unexpected()
			}
			if p.peek().Kind != TClass || isIdentifier(p.tok(1).Kind) {
				if single {
					p.unexpected()
				}
				return p.classDecl(attrs)
			}
		case TConst:
			if top {
				n := p.constStatement()
				n.Children = append([]*Node{attrs}, n.Children...)
				return n
			}
		}
		e := p.exprRest(p.exprWithAttributes(attrs), precLowest)
		p.endStatement()
		return node("expr", t.Line, e)
	case TString:
		if p.tok(1).Kind == ':' {
			p.next()
			p.next()
			return leaf("label", t)
		}
	case TNamespace:
		if top && p.tok(1).Kind != TNsSeparator {
			return p.namespaceStatement()
		}
	case TUse:
		if top {
			return p.useStatement()
		}
	case TConst:
		if top {
			return p.constStatement()
		}
	case THaltCompiler:
		if top {
			p.next()
			p.expect('(')
			p.expect(')')
			p.endStatement()
			return node("halt_compiler", t.Line)
		}
	}
	e := p.expr()
	p.endStatement()
	return node("expr", t.Line, e)
}

func isAmpersand(k Kind) bool {
	return k == TAmpersandFollowedByVarOrVararg || k == TAmpersandNotFollowedByVarOrVararg
}

// classModifiersAhead reports whether modifiers starting here lead to a class.
func (p *parser) classModifiersAhead() bool {
	for i := 0; ; i++ {
		switch p.tok(i).Kind {
		case TAbstract, TFinal, TReadonly:
		case TClass:
			return true
		default:
			return false
		}
	}
}

func (p *parser) parenExpr() *Node {
	p.expect('(')
	e := p.expr()
	p.expect(')')
	return e
}

func (p *parser) exprList() []*Node {
	out := []*Node{p.expr()}
	for p.accept(',') {
		out = append(out, p.expr())
	}
	return out
}

func (p *parser) ifStatement() *Node {
	t := p.next()
	cond := p.parenExpr()
	if p.accept(':') {
		n := node("if", t.Line, cond, p.altBody(TElseif, TElse, TEndif))
		for p.at(TElseif) {
			e := p.next()
			c := p.parenExpr()
			p.expect(':')
			n.Children = append(n.Children, node("elseif", e.Line, c, p.altBody(TElseif, TElse, TEndif)))
		}
		if p.at(TElse) {
			e := p.next()
			p.expect(':')
			n.Children = append(n.Children, node("else", e.Line, p.altBody(TEndif)))
		}
		p.expect(TEndif)
		p.endStatement()
		return n
	}
	n := node("if", t.Line, cond, p.singleStatement())
	for {
		switch {
		case p.at(TElseif):
			e := p.next()
			c := p.parenExpr()
			n.Children = append(n.Children, node("elseif", e.Line, c, p.singleStatement()))
			continue
		case p.at(TElse):
			e := p.next()
			n.Children = append(n.Children, node("else", e.Line, p.singleStatement()))
		}
		return n
	}
}

func (p *parser) forStatement() *Node {
	t := p.next()
	p.expect('(')
	parts := make([]*Node, 0, 4)
	for i, end := range []Kind{';', ';', ')'} {
		n := node("for_exprs", p.peek().Line)
		for !p.at(end) && !(end == ';' && p.at(TCloseTag)) {
			// (void) may discard any for expression except the final
			// condition, whose value decides the loop.
			e := p.peek()
			if e.Kind == TVoidCast {
				p.next()
				n.Children = append(n.Children, node("void", e.Line, p.expr()))
				if i == 1 && !p.at(',') {
					p.unexpected()
				}
			} else {
				n.Children = append(n.Children, p.expr())
			}
			if !p.accept(',') {
				break
			}
			if p.at(end) {
				p.unexpected()
			}
		}
		if end == ';' {
			if !p.acceptSemi() {
				p.unexpected()
			}
		} else {
			p.expect(end)
		}
		parts = append(parts, n)
	}
	if p.accept(':') {
		body := p.altBody(TEndfor)
		p.expect(TEndfor)
		p.endStatement()
		return node("for", t.Line, append(parts, body)...)
	}
	return node("for", t.Line, append(parts, p.singleStatement())...)
}

func (p *parser) foreachStatement() *Node {
	t := p.next()
	p.expect('(')
	subject := p.expr()
	p.expect(TAs)
	target := p.foreachTarget()
	var key *Node
	if p.accept(TDoubleArrow) {
		key, target = target, p.foreachTarget()
	}
	p.expect(')')
	if p.accept(':') {
		body := p.altBody(TEndforeach)
		p.expect(TEndforeach)
		p.endStatement()
		return node("foreach", t.Line, subject, key, target, body)
	}
	return node("foreach", t.Line, subject, key, target, p.singleStatement())
}

func (p *parser) foreachTarget() *Node {
	p.inTarget = true
	defer func() { p.inTarget = false }()
	ref := p.peek()
	byRef := isAmpersand(ref.Kind)
	if byRef {
		p.next()
	}
	start := p.peek()
	target := p.unary(precUnary)
	if start.Kind == TArray && target.Kind == "array" {
		p.fail(start.Line, "invalid foreach target: array() is not a destructuring target")
	}
	if !isAssignTarget(target) || (byRef && (target.Kind == "array" || target.Kind == "list")) {
		p.fail(start.Line+countLines(start.Text), "invalid foreach target: %s cannot be used here", describe(start))
	}
	if byRef {
		return node("by_ref", ref.Line, target)
	}
	return target
}

func (p *parser) switchStatement() *Node {
	t := p.next()
	subject := p.parenExpr()
	alt := false
	if p.accept(':') {
		alt = true
	} else {
		p.expect('{')
	}
	p.acceptSemi()
	n := node("switch", t.Line, subject)
	end := func() bool { return (!alt && p.at('}')) || (alt && p.at(TEndswitch)) }
	for !end() {
		c := p.peek()
		var label *Node
		switch {
		case p.accept(TCase):
			label = p.expr()
		case p.accept(TDefault):
		default:
			p.unexpected()
		}
		if !p.accept(':') && !p.acceptSemi() {
			p.unexpected()
		}
		body := p.statements(false, func() bool { return p.atAny(TCase, TDefault) || end() })
		n.Children = append(n.Children, node("case", c.Line, append([]*Node{label}, body...)...))
	}
	if alt {
		p.expect(TEndswitch)
		p.endStatement()
	} else {
		p.expect('}')
	}
	return n
}

func (p *parser) declareStatement() *Node {
	t := p.next()
	p.expect('(')
	n := node("declare", t.Line)
	for {
		name := p.identifier()
		p.expect('=')
		v := p.expr()
		if strings.EqualFold(name.Text, "encoding") && v.Kind != "literal" {
			p.fail(p.peek().Line, "Encoding must be a literal")
		}
		n.Children = append(n.Children, node("directive", name.Line, name, v))
		if !p.accept(',') {
			break
		}
	}
	p.expect(')')
	switch {
	case p.accept(':'):
		body := p.altBody(TEnddeclare)
		p.expect(TEnddeclare)
		p.endStatement()
		n.Children = append(n.Children, body)
	case p.atAny(';', TCloseTag):
		p.next()
	default:
		n.Children = append(n.Children, p.singleStatement())
	}
	return n
}

func (p *parser) tryStatement() *Node {
	t := p.next()
	n := node("try", t.Line, p.block())
	handled := false
	for p.at(TCatch) {
		c := p.next()
		p.expect('(')
		types := node("types", c.Line, p.name())
		for p.accept('|') {
			types.Children = append(types.Children, p.name())
		}
		var v *Node
		if p.at(TVariable) {
			v = leaf("variable", p.next())
		}
		p.expect(')')
		n.Children = append(n.Children, node("catch", c.Line, types, v, p.block()))
		handled = true
	}
	if p.at(TFinally) {
		f := p.next()
		n.Children = append(n.Children, node("finally", f.Line, p.block()))
		handled = true
	}
	// A try without catch or finally parses; PHP rejects it when compiling.
	_ = handled
	return n
}

func (p *parser) staticVars() *Node {
	t := p.next()
	n := node("static_vars", t.Line)
	for {
		v := leaf("variable", p.expect(TVariable))
		var init *Node
		if p.accept('=') {
			init = p.expr()
		}
		n.Children = append(n.Children, node("static_var", v.Line, v, init))
		if !p.accept(',') {
			break
		}
	}
	p.endStatement()
	return n
}

func (p *parser) namespaceStatement() *Node {
	t := p.next()
	var name *Node
	if k := p.peek().Kind; k == TString || k == TNameQualified || isKeyword(k) {
		// Namespace names may be reserved words.
		name = leaf("name", p.next())
	}
	if name != nil && p.atAny(';', TCloseTag) {
		p.next()
		return node("namespace", t.Line, name)
	}
	open := p.expect('{')
	body := p.statements(true, func() bool { return p.at('}') })
	if len(body) > 0 && body[len(body)-1].Kind == "halt_compiler" {
		// __halt_compiler() stops lexing, leaving this brace unclosed.
		p.fail(body[len(body)-1].Line, "Unclosed '{' on line %d", open.Line)
	}
	p.expect('}')
	return node("namespace", t.Line, append([]*Node{name}, body...)...)
}

func (p *parser) useKind() string {
	switch {
	case p.accept(TFunction):
		return "function"
	case p.accept(TConst):
		return "const"
	}
	return ""
}

// importName accepts the names PHP allows in use declarations: plain,
// qualified, or fully qualified, but not namespace-relative names or static.
func (p *parser) importName() *Node {
	if !p.atAny(TString, TNameQualified, TNameFullyQualified) {
		p.unexpected()
	}
	return leaf("name", p.next())
}

func (p *parser) useStatement() *Node {
	t := p.next()
	n := &Node{Kind: "use", Text: p.useKind(), Line: t.Line}
	for {
		name := p.importName()
		if p.accept(TNsSeparator) {
			if len(n.Children) != 0 {
				p.unexpected()
			}
			p.expect('{')
			group := node("group_use", name.Line, name)
			for {
				kind := ""
				if n.Text == "" {
					kind = p.useKind()
				}
				if !p.atAny(TString, TNameQualified) {
					p.unexpected()
				}
				item := &Node{Kind: "use_item", Text: kind, Line: p.peek().Line, Children: []*Node{leaf("name", p.next()), p.useAlias()}}
				group.Children = append(group.Children, item)
				if !p.accept(',') || p.at('}') {
					break
				}
			}
			p.expect('}')
			n.Children = append(n.Children, group)
			break
		} else {
			n.Children = append(n.Children, node("use_item", name.Line, name, p.useAlias()))
		}
		if !p.accept(',') {
			break
		}
	}
	p.endStatement()
	return n
}

func (p *parser) useAlias() *Node {
	if p.accept(TAs) {
		return p.identifierNotReserved()
	}
	return nil
}

// className accepts the names PHP allows for declared classes, interfaces,
// traits, and enums: plain identifiers plus the soft keywords enum and
// readonly.
func (p *parser) className() *Node {
	if !p.atAny(TString, TEnum, TReadonly) {
		p.unexpected()
	}
	return leaf("identifier", p.next())
}

func (p *parser) identifierNotReserved() *Node {
	t := p.expect(TString)
	return leaf("identifier", t)
}

func (p *parser) constStatement() *Node {
	t := p.next()
	n := node("const", t.Line)
	for {
		name := p.identifierNotReserved()
		p.expect('=')
		n.Children = append(n.Children, node("const_item", name.Line, name, p.expr()))
		if !p.accept(',') {
			break
		}
	}
	p.endStatement()
	return n
}

// --- declarations ---

func (p *parser) attributes() *Node {
	n := node("attributes", p.peek().Line)
	for p.at(TAttribute) {
		g := node("attribute_group", p.next().Line)
		if p.at(']') {
			p.unexpected()
		}
		for !p.at(']') {
			a := node("attribute", p.peek().Line, p.name())
			if p.at('(') {
				a.Children = append(a.Children, p.arguments())
			}
			g.Children = append(g.Children, a)
			if !p.accept(',') {
				break
			}
		}
		p.expect(']')
		n.Children = append(n.Children, g)
	}
	return n
}

func (p *parser) functionDecl(attrs *Node) *Node {
	t := p.expect(TFunction)
	byRef := p.byRef()
	if !p.atAny(TString, TReadonly) {
		p.unexpected()
	}
	name := leaf("identifier", p.next())
	params := p.parameters()
	ret := p.returnType()
	return &Node{Kind: "function", Text: byRef, Line: t.Line, Children: []*Node{attrs, name, params, ret, p.block()}}
}

func (p *parser) byRef() string {
	if isAmpersand(p.peek().Kind) {
		p.next()
		return "&"
	}
	return ""
}

func (p *parser) returnType() *Node {
	if p.accept(':') {
		p.allowStatic = true
		defer func() { p.allowStatic = false }()
		return p.typeExpr()
	}
	return nil
}

func (p *parser) parameters() *Node {
	t := p.expect('(')
	n := node("params", t.Line)
	for !p.at(')') {
		n.Children = append(n.Children, p.parameter())
		if !p.accept(',') {
			break
		}
	}
	p.expect(')')
	return n
}

func isModifier(k Kind) bool {
	switch k {
	case TPublic, TProtected, TPrivate, TStatic, TAbstract, TFinal, TReadonly, TVar,
		TPublicSet, TProtectedSet, TPrivateSet:
		return true
	}
	return false
}

func (p *parser) modifiers() *Node {
	n := node("modifiers", p.peek().Line)
	for isModifier(p.peek().Kind) {
		n.Children = append(n.Children, leaf("modifier", p.next()))
	}
	return n
}

func (p *parser) parameter() *Node {
	line := p.peek().Line
	var attrs *Node
	if p.at(TAttribute) {
		attrs = p.attributes()
	}
	mods := p.modifiers()
	p.checkModifiers(mods, targetParameter)
	var typ *Node
	if !p.at(TVariable) && !isAmpersand(p.peek().Kind) && !p.at(TEllipsis) {
		typ = p.typeExpr()
	}
	flags := p.byRef()
	if p.accept(TEllipsis) {
		flags += "..."
	}
	v := leaf("variable", p.expect(TVariable))
	var def *Node
	if p.accept('=') {
		def = p.expr()
	}
	var hooks *Node
	if p.at('{') {
		hooks = p.propertyHooks()
	}
	return &Node{Kind: "param", Text: flags, Line: line, Children: []*Node{attrs, mods, typ, v, def, hooks}}
}

func (p *parser) typeExpr() *Node {
	t := p.peek()
	if p.accept('?') {
		return node("nullable_type", t.Line, p.singleType())
	}
	first := p.typeTerm()
	switch {
	case p.at('|'):
		n := node("union_type", t.Line, first)
		for p.accept('|') {
			n.Children = append(n.Children, p.typeTerm())
		}
		return n
	case p.at(TAmpersandNotFollowedByVarOrVararg) && first.Kind != "intersection_type":
		n := node("intersection_type", t.Line, first)
		for p.accept(TAmpersandNotFollowedByVarOrVararg) {
			n.Children = append(n.Children, p.singleType())
		}
		return n
	}
	if first.Kind == "intersection_type" {
		p.unexpected()
	}
	return first
}

// typeTerm is a single type or a parenthesized intersection inside a union.
func (p *parser) typeTerm() *Node {
	if t := p.peek(); t.Kind == '(' {
		p.next()
		n := node("intersection_type", t.Line, p.singleType())
		for p.accept(TAmpersandNotFollowedByVarOrVararg) {
			n.Children = append(n.Children, p.singleType())
		}
		if len(n.Children) < 2 {
			p.unexpected()
		}
		p.expect(')')
		return n
	}
	return p.singleType()
}

func (p *parser) singleType() *Node {
	t := p.peek()
	switch {
	case t.Kind == TArray || t.Kind == TCallable || (t.Kind == TStatic && p.allowStatic):
		p.next()
		return leaf("type", t)
	case isName(t.Kind):
		p.next()
		return leaf("type", t)
	}
	p.unexpected()
	return nil
}

func (p *parser) classDecl(attrs *Node) *Node {
	line := p.peek().Line
	mods := node("modifiers", line)
	for p.atAny(TAbstract, TFinal, TReadonly) {
		mods.Children = append(mods.Children, leaf("modifier", p.next()))
	}
	p.checkClassModifiers(mods, false)
	t := p.next()
	n := &Node{Kind: strings.ToLower(t.Text), Line: line}
	name := p.className()
	var backing, extends, implements *Node
	if t.Kind == TEnum && p.accept(':') {
		backing = p.typeExpr()
	}
	if (t.Kind == TClass || t.Kind == TInterface) && p.accept(TExtends) {
		if t.Kind == TClass {
			extends = p.name()
		} else {
			extends = p.nameList()
		}
	}
	if (t.Kind == TClass || t.Kind == TEnum) && p.accept(TImplements) {
		implements = p.nameList()
	}
	n.Children = []*Node{attrs, mods, name, backing, extends, implements, p.classBody()}
	return n
}

func (p *parser) nameList() *Node {
	n := node("names", p.peek().Line, p.name())
	for p.accept(',') {
		n.Children = append(n.Children, p.name())
	}
	return n
}

func (p *parser) classBody() *Node {
	t := p.expect('{')
	n := node("members", t.Line)
	for !p.at('}') {
		if p.at(tEOF) {
			p.unexpected()
		}
		n.Children = append(n.Children, p.member())
	}
	p.expect('}')
	return n
}

func (p *parser) member() *Node {
	p.enter()
	defer p.leave()
	line := p.peek().Line
	if p.at(TUse) {
		return p.traitUse()
	}
	var attrs *Node
	if p.at(TAttribute) {
		attrs = p.attributes()
	}
	if p.at(TCase) {
		p.next()
		name := p.identifier()
		var v *Node
		if p.accept('=') {
			v = p.expr()
		}
		p.endStatement()
		return node("enum_case", line, attrs, name, v)
	}
	mods := p.modifiers()
	switch {
	case p.at(TConst):
		p.checkModifiers(mods, targetConstant)
		p.next()
		var typ *Node
		if !(isIdentifier(p.peek().Kind) && p.tok(1).Kind == '=') {
			p.allowStatic = true
			typ = p.typeExpr()
			p.allowStatic = false
		}
		n := node("class_const", line, attrs, mods, typ)
		for {
			name := p.identifier()
			p.expect('=')
			n.Children = append(n.Children, node("const_item", name.Line, name, p.expr()))
			if !p.accept(',') {
				break
			}
		}
		p.endStatement()
		return n
	case p.at(TFunction):
		p.checkModifiers(mods, targetMethod)
		p.next()
		byRef := p.byRef()
		name := p.identifier()
		params := p.parameters()
		ret := p.returnType()
		var body *Node
		if !p.acceptSemi() {
			body = p.block()
		}
		return &Node{Kind: "method", Text: byRef, Line: line, Children: []*Node{attrs, mods, name, params, ret, body}}
	}
	if len(mods.Children) == 0 {
		p.unexpected()
	}
	p.checkModifiers(mods, targetProperty)
	var typ *Node
	if !p.at(TVariable) {
		typ = p.typeExpr()
	}
	n := node("property", line, attrs, mods, typ)
	for {
		v := leaf("variable", p.expect(TVariable))
		var def *Node
		if p.accept('=') {
			def = p.expr()
		}
		n.Children = append(n.Children, node("property_item", v.Line, v, def))
		if !p.accept(',') {
			break
		}
	}
	if p.at('{') {
		if len(n.Children) != 4 { // attrs, modifiers, type, and one property
			p.unexpected()
		}
		n.Children = append(n.Children, p.propertyHooks())
		return n
	}
	p.endStatement()
	return n
}

func (p *parser) propertyHooks() *Node {
	p.enter()
	defer p.leave()
	t := p.expect('{')
	n := node("hooks", t.Line)
	for !p.at('}') {
		line := p.peek().Line
		var attrs *Node
		if p.at(TAttribute) {
			attrs = p.attributes()
		}
		mods := p.modifiers()
		p.checkModifiers(mods, targetPropertyHook)
		byRef := p.byRef()
		name := p.identifier()
		var params *Node
		if p.at('(') {
			params = p.parameters()
		}
		var body *Node
		switch {
		case p.accept(TDoubleArrow):
			body = p.expr()
			p.endStatement()
		case p.at('{'):
			body = p.block()
		default:
			p.endStatement()
		}
		n.Children = append(n.Children, &Node{Kind: "hook", Text: byRef, Line: line, Children: []*Node{attrs, mods, name, params, body}})
	}
	p.expect('}')
	return n
}

func (p *parser) traitUse() *Node {
	t := p.next()
	n := node("trait_use", t.Line, p.nameList())
	if p.accept('{') {
		rules := node("adaptations", t.Line)
		for !p.at('}') {
			line := p.peek().Line
			var method *Node
			if (isName(p.peek().Kind) || p.at(TStatic)) && p.tok(1).Kind == TDoubleColon {
				class := p.name()
				p.next()
				method = node("method_ref", line, class, p.identifier())
			} else {
				method = node("method_ref", line, nil, p.identifier())
			}
			switch {
			case p.accept(TInsteadof):
				rules.Children = append(rules.Children, node("insteadof", line, method, p.nameList()))
			case p.accept(TAs):
				var mod, alias *Node
				if isModifier(p.peek().Kind) {
					mod = leaf("modifier", p.next())
					p.checkModifiers(node("modifiers", mod.Line, mod), targetMethod)
				}
				if isIdentifier(p.peek().Kind) {
					alias = p.identifier()
				}
				if mod == nil && alias == nil {
					p.unexpected()
				}
				rules.Children = append(rules.Children, node("alias", line, method, mod, alias))
			default:
				p.unexpected()
			}
			p.endStatement()
		}
		p.expect('}')
		n.Children = append(n.Children, rules)
	} else {
		p.endStatement()
	}
	return n
}
