package ir

import (
	"fmt"
	"path"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// Options configures lowering.
type Options struct {
	// File is the path PHP reports for __FILE__, __DIR__ and closure names.
	File string
}

// Lower compiles a syntax tree produced by frontend.Parse for the same
// profile. A fatal compile error is returned as *CompileError with the line
// PHP reports; compile warnings are recorded in Unit.Diagnostics.
func Lower(tree *frontend.Node, prof profile.Profile, opts Options) (unit *Unit, err error) {
	if tree == nil || tree.Kind != "file" {
		return nil, fmt.Errorf("ir: Lower needs a file syntax tree")
	}
	l := &lowerer{
		unit: &Unit{File: opts.File, Profile: prof},
		ns:   newNamespace(""),
		top:  tree.Children,
	}
	// PHP scans the whole file before compiling it, so warnings raised while
	// decoding literals precede compile diagnostics.
	var scanned []Diagnostic
	l.dec = stringDecoder{warn: func(line int, msg string) {
		scanned = append(scanned, Diagnostic{Line: line, Level: "Warning", Message: msg})
	}}
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(compileFailure)
			if !ok {
				panic(r)
			}
			unit, err = nil, f.err
		}
	}()
	main := l.newFunc(FuncMain, "", 1)
	l.unit.Main = main
	l.fn = main
	l.hoisted = map[string]int{}
	for _, s := range tree.Children {
		main.Body = appendNode(main.Body, l.topStatement(s))
	}
	l.finishFunc(main)
	main.Strict = l.unit.Strict
	l.unit.Diagnostics = append(scanned, l.unit.Diagnostics...)
	return l.unit, nil
}

type compileFailure struct{ err *CompileError }

func (l *lowerer) fail(line int, format string, args ...any) {
	panic(compileFailure{&CompileError{Line: line, Message: fmt.Sprintf(format, args...)}})
}

func (l *lowerer) diag(line int, level, msg string) {
	l.unit.Diagnostics = append(l.unit.Diagnostics, Diagnostic{Line: line, Level: level, Message: msg})
}

type region struct {
	kind  byte // 'l' loop, 's' switch, 'f' finally
	id    int
	start int // line, for diagnostics
}

type jump struct {
	kind    string // break, continue, goto
	line    int
	label   string
	regions []region
	target  int // index into regions of the targeted loop for break/continue
}

type label struct {
	line    int
	regions []region
}

// lowerer holds compile state. Per-function state is saved and restored
// around nested functions.
type lowerer struct {
	unit *Unit
	dec  stringDecoder
	top  []*frontend.Node

	ns *namespace
	// unbracketed is set after an unbracketed namespace declaration, while
	// PHP's current namespace is set; bracketed after a bracketed one.
	unbracketed bool
	bracketed   bool
	inNamespace bool

	fn      *Func
	class   *Class
	regions []region
	nextID  int
	jumps   []jump
	labels  map[string]label
	// hoisted records top-level functions by lower-case name.
	hoisted map[string]int
	// inConstExpr is set while compiling a constant expression.
	inConstExpr bool
	// seen records the symbols declared so far in the namespace, which a
	// later import may not shadow, as PHP's seen_symbols.
	seen   map[string]bool
	zlines map[*frontend.Node]int
	// ticks is set inside declare(ticks=N), where a tick function may run
	// user code after any statement.
	ticks bool
	// cur is the line of the expression compiled last, which PHP reports
	// for errors raised while compiling a construct's later parts.
	cur int
}

func (l *lowerer) newFunc(kind FuncKind, name string, line int) *Func {
	return &Func{Kind: kind, Name: name, Line: line, Strict: l.unit.Strict, vars: map[string]*Var{}}
}

// enterFunc starts compiling a nested function body and returns a function
// restoring the outer state.
func (l *lowerer) enterFunc(f *Func) func() {
	saved := struct {
		fn      *Func
		regions []region
		jumps   []jump
		labels  map[string]label
	}{l.fn, l.regions, l.jumps, l.labels}
	l.fn, l.regions, l.jumps, l.labels = f, nil, nil, nil
	return func() { l.fn, l.regions, l.jumps, l.labels = saved.fn, saved.regions, saved.jumps, saved.labels }
}

// finishFunc resolves the jumps of the current function, as PHP's pass_two
// does when a function's compilation ends.
func (l *lowerer) finishFunc(f *Func) {
	for _, j := range l.jumps {
		var dst []region
		if j.kind == "goto" {
			lab, ok := l.labels[j.label]
			if !ok {
				l.fail(j.line, "'goto' to undefined label '%s'", j.label)
			}
			if !loopPrefix(lab.regions, j.regions) {
				l.fail(j.line, "'goto' into loop or switch statement is disallowed")
			}
			dst = lab.regions
		} else {
			dst = j.regions[:j.target]
		}
		for _, r := range j.regions {
			if r.kind == 'f' && !hasRegion(dst, r.id) {
				l.fail(j.line, "jump out of a finally block is disallowed")
			}
		}
		for _, r := range dst {
			if r.kind == 'f' && !hasRegion(j.regions, r.id) {
				l.fail(j.line, "jump into a finally block is disallowed")
			}
		}
	}
	_ = f
}

func hasRegion(rs []region, id int) bool {
	for _, r := range rs {
		if r.id == id {
			return true
		}
	}
	return false
}

// loopPrefix reports whether the loop and switch regions of a label enclose
// the goto, so the jump does not enter a loop.
func loopPrefix(label, from []region) bool {
	for _, r := range label {
		if r.kind != 'f' && !hasRegion(from, r.id) {
			return false
		}
	}
	return true
}

func (l *lowerer) push(kind byte, line int) {
	l.nextID++
	l.regions = append(l.regions, region{kind: kind, id: l.nextID, start: line})
}

func (l *lowerer) pop() { l.regions = l.regions[:len(l.regions)-1] }

func appendNode(list []*Node, n *Node) []*Node {
	if n == nil {
		return list
	}
	if n.Op == OpBlock && n.Kids == nil && n.Line == 0 {
		return list
	}
	return append(list, n)
}

// --- top level ---

func (l *lowerer) topStatement(s *frontend.Node) *Node {
	switch s.Kind {
	case "namespace":
		return l.namespaceDecl(s)
	case "use":
		l.verifyNamespace(s)
		l.useDecl(s)
		return nil
	case "halt_compiler":
		if l.bracketed && l.inNamespace {
			l.fail(s.Line, "__HALT_COMPILER() can only be used from the outermost scope")
		}
		return &Node{Op: OpHalt, Line: s.Line}
	case "block":
		// A top-level block is a statement list: its functions are still
		// declared before the file runs.
		b := &Node{Op: OpBlock, Line: s.Line}
		for _, c := range s.Children {
			b.Kids = appendNode(b.Kids, l.statement(c, true))
		}
		l.verifyNamespace(s)
		return l.seal(b)
	}
	n := l.statement(s, true)
	l.verifyNamespace(s)
	return n
}

func (l *lowerer) verifyNamespace(s *frontend.Node) {
	if l.bracketed && !l.inNamespace {
		l.fail(s.Line, "No code may exist outside of namespace {}")
	}
}

// isFirstStatement mirrors zend_is_first_statement with nop statements
// allowed: only declare statements may precede s.
func (l *lowerer) isFirstStatement(s *frontend.Node) bool {
	for _, t := range l.top {
		if t == s {
			return true
		}
		if t.Kind != "declare" {
			return false
		}
	}
	return false
}

func (l *lowerer) namespaceDecl(s *frontend.Node) *Node {
	name := s.Children[0]
	bracketed := s.Text == "{"
	if !l.bracketed {
		if l.unbracketed && bracketed {
			l.fail(s.Line, "Cannot mix bracketed namespace declarations with unbracketed namespace declarations")
		}
	} else if !bracketed {
		l.fail(s.Line, "Cannot mix bracketed namespace declarations with unbracketed namespace declarations")
	} else if l.inNamespace {
		l.fail(s.Line, "Namespace declarations cannot be nested")
	}
	first := (!bracketed && !l.unbracketed) || (bracketed && !l.bracketed)
	if first && !l.isFirstStatement(s) {
		l.fail(s.Line, "Namespace declaration statement has to be the very first statement or after any declare call in the script")
	}
	nsName := ""
	if name != nil {
		nsName = name.Text
		if strings.EqualFold(nsName, "namespace") {
			l.fail(s.Line, "Cannot use '%s' as namespace name", nsName)
		}
	}
	l.ns = newNamespace(nsName)
	l.seen = nil // PHP clears seen symbols with the import tables
	l.inNamespace = true
	if !bracketed {
		l.unbracketed = true
		return nil
	}
	l.bracketed = true
	block := &Node{Op: OpBlock, Line: s.Line}
	for _, c := range s.Children[1:] {
		block.Kids = appendNode(block.Kids, l.topStatement(c))
	}
	l.inNamespace = false
	l.ns = newNamespace("")
	l.seen = nil
	return l.seal(block)
}

func (l *lowerer) useDecl(s *frontend.Node) {
	kind := s.Text
	for _, item := range s.Children {
		if item.Kind == "group_use" {
			prefix := item.Children[0].Text
			for _, g := range item.Children[1:] {
				k := kind
				if g.Text != "" {
					k = g.Text
				}
				l.importName(k, prefix+"\\"+g.Children[0].Text, g.Children[1], g.Line)
			}
			continue
		}
		l.importName(kind, item.Children[0].Text, item.Children[1], item.Line)
	}
}

// see records a declared symbol of the file.
func (l *lowerer) see(kind, full string) {
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	if kind != "const" {
		full = strings.ToLower(full)
	}
	l.seen[kind+":"+full] = true
}

// importName adds one use declaration, checking conflicts as
// zend_compile_use.
func (l *lowerer) importName(kind, name string, alias *frontend.Node, line int) {
	name = strings.TrimPrefix(name, "\\")
	short := name
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		short = name[i+1:]
	} else if alias == nil && l.ns.name == "" {
		l.diag(line, "Warning", fmt.Sprintf("The use statement with non-compound name '%s' has no effect", name))
	}
	if alias != nil {
		short = alias.Text
	}
	label := map[string]string{"function": " function", "const": " const"}[kind]
	inUse := func() {
		l.fail(line, "Cannot use%s %s as %s because the name is already in use", label, name, short)
	}
	ns := l.ns
	key := strings.ToLower(short)
	if kind == "const" {
		key = short
	}
	if kind == "" && isReservedClassName(short) {
		l.fail(line, "Cannot use %s as %s because '%s' is a special class name", name, short, short)
	}
	symbol := kind
	if symbol == "" {
		symbol = "class"
	}
	check := key
	if ns.name != "" {
		check = strings.ToLower(ns.name) + "\\" + key
	}
	if l.seen[symbol+":"+check] && !strings.EqualFold(strings.TrimPrefix(name, "\\"), check) {
		inUse()
	}
	table := ns.classes
	switch kind {
	case "function":
		table = ns.functions
	case "const":
		table = ns.constants
	}
	if _, dup := table[key]; dup {
		inUse()
	}
	table[key] = strings.TrimPrefix(name, "\\")
}

// --- statements ---

func (l *lowerer) block(n *frontend.Node) *Node {
	b := &Node{Op: OpBlock, Line: lineOf(n)}
	if n == nil {
		return b
	}
	if n.Kind != "block" {
		b.Kids = appendNode(nil, l.statement(n, false))
		return l.seal(b)
	}
	for _, s := range n.Children {
		b.Kids = appendNode(b.Kids, l.statement(s, false))
	}
	return l.seal(b)
}

func lineOf(n *frontend.Node) int {
	if n == nil {
		return 0
	}
	return n.Line
}

func (l *lowerer) statement(s *frontend.Node, top bool) *Node {
	n := l.stmt(s, top)
	if n != nil {
		l.seal(n)
		if l.ticks {
			n.Effects |= MayCallUser
		}
	}
	return n
}

// seal computes the effects of a statement from its own effects and its
// direct children; expressions are sealed as they are built.
func (l *lowerer) seal(n *Node) *Node {
	e := ownEffects(n)
	for _, k := range n.Kids {
		if k != nil {
			e |= k.Effects
		}
	}
	for _, a := range n.Args {
		e |= a.Value.Effects
		if a.MayRef {
			e |= MayAlias | MayWrite
		}
	}
	n.Effects = e
	return n
}

func (l *lowerer) stmt(s *frontend.Node, top bool) *Node {
	c := s.Children
	switch s.Kind {
	case "expr":
		return &Node{Op: OpExpr, Line: s.Line, Kids: []*Node{l.expr(c[0])}}
	case "void":
		return &Node{Op: OpVoid, Line: s.Line, Kids: []*Node{l.expr(c[0])}}
	case "echo":
		n := &Node{Op: OpEcho, Line: s.Line}
		for _, e := range c {
			n.Kids = append(n.Kids, l.expr(e))
		}
		return n
	case "inline_html":
		return &Node{Op: OpHTML, Line: s.Line, Value: &Literal{Kind: LitString, Str: s.Text}}
	case "block":
		return l.block(s)
	case "if":
		n := &Node{Op: OpIf, Line: s.Line, Kids: []*Node{l.expr(c[0]), l.block(c[1])}}
		for _, e := range c[2:] {
			if e.Kind == "elseif" {
				n.Kids = append(n.Kids, l.expr(e.Children[0]), l.block(e.Children[1]))
			} else {
				n.Kids = append(n.Kids, l.block(e.Children[0]))
			}
		}
		return n
	case "while":
		cond := l.expr(c[0])
		l.push('l', s.Line)
		body := l.block(c[1])
		l.pop()
		return &Node{Op: OpWhile, Line: s.Line, Kids: []*Node{cond, body}}
	case "do":
		l.push('l', s.Line)
		body := l.block(c[0])
		l.pop()
		return &Node{Op: OpDo, Line: s.Line, Kids: []*Node{body, l.expr(c[1])}}
	case "for":
		n := &Node{Op: OpFor, Line: s.Line}
		for _, part := range c[:3] {
			b := &Node{Op: OpBlock, Line: part.Line}
			for _, e := range part.Children {
				if e.Kind == "void" {
					b.Kids = append(b.Kids, l.seal(&Node{Op: OpVoid, Line: e.Line, Kids: []*Node{l.expr(e.Children[0])}}))
				} else {
					b.Kids = append(b.Kids, l.expr(e))
				}
			}
			n.Kids = append(n.Kids, l.seal(b))
		}
		l.push('l', s.Line)
		n.Kids = append(n.Kids, l.block(c[3]))
		l.pop()
		return n
	case "foreach":
		return l.foreach(s)
	case "switch":
		return l.switchStmt(s)
	case "break", "continue":
		return l.breakContinue(s)
	case "return":
		return l.returnStmt(s)
	case "global":
		b := &Node{Op: OpBlock, Line: s.Line}
		for _, v := range c {
			b.Kids = append(b.Kids, l.seal(l.globalVar(v)))
		}
		return b
	case "static_vars":
		b := &Node{Op: OpBlock, Line: s.Line}
		for _, sv := range c {
			b.Kids = append(b.Kids, l.seal(l.staticVar(sv)))
		}
		return b
	case "unset":
		n := &Node{Op: OpUnset, Line: s.Line}
		for _, v := range c {
			n.Kids = append(n.Kids, l.unsetTarget(v))
		}
		return n
	case "declare":
		return l.declare(s)
	case "try":
		return l.try(s)
	case "goto":
		l.jumps = append(l.jumps, jump{kind: "goto", line: s.Line, label: c[0].Text, regions: append([]region(nil), l.regions...)})
		return &Node{Op: OpGoto, Line: s.Line, Text: c[0].Text}
	case "label":
		if l.labels == nil {
			l.labels = map[string]label{}
		}
		if _, dup := l.labels[s.Text]; dup {
			l.fail(s.Line, "Label '%s' already defined", s.Text)
		}
		l.labels[s.Text] = label{line: s.Line, regions: append([]region(nil), l.regions...)}
		return &Node{Op: OpLabel, Line: s.Line, Text: s.Text}
	case "function":
		f := l.function(s, top)
		if f.Hoisted {
			return nil
		}
		return &Node{Op: OpFuncDecl, Line: s.Line, Func: f}
	case "class", "interface", "trait", "enum":
		cl := l.classDecl(s, false)
		return &Node{Op: OpClass, Line: s.Line, Class: cl}
	case "const":
		return l.constDecl(s)
	case "halt_compiler":
		return &Node{Op: OpHalt, Line: s.Line}
	}
	panic(fmt.Sprintf("ir: unexpected statement %q", s.Kind))
}

func (l *lowerer) foreach(s *frontend.Node) *Node {
	c := s.Children
	subject := l.expr(c[0])
	n := &Node{Op: OpForeach, Line: s.Line, Kids: []*Node{subject, nil, nil, nil}}
	if k := c[1]; k != nil {
		if k.Kind == "by_ref" {
			l.fail(s.Line, "Key element cannot be a reference")
		}
		if k.Kind == "list" || k.Kind == "array" {
			l.fail(s.Line, "Cannot use list as key element")
		}
		n.Kids[1] = l.target(k, s.Line)
	}
	value := c[2]
	if value.Kind == "by_ref" {
		n.Flags |= FlagRef
		value = value.Children[0]
		if isThis(value) {
			l.fail(s.Line, "Cannot re-assign $this")
		}
		l.ensureWritable(unparen(value), s.Line)
		n.Kids[2] = l.refTarget(value)
	} else {
		if isThis(value) {
			l.fail(s.Line, "Cannot re-assign $this")
		}
		if value.Kind == "list" || value.Kind == "array" {
			n.Kids[2] = l.destructurePattern(value, s.Line)
		} else {
			n.Kids[2] = l.target(value, s.Line)
		}
	}
	l.push('l', s.Line)
	n.Kids[3] = l.block(c[3])
	l.pop()
	return n
}

func (l *lowerer) switchStmt(s *frontend.Node) *Node {
	c := s.Children
	n := &Node{Op: OpSwitch, Line: s.Line, Kids: []*Node{l.expr(c[0])}}
	l.push('s', s.Line)
	seenDefault := false
	for _, cs := range c[1:] {
		if cs.Text == ";" {
			l.diag(cs.Line, "Deprecated", "Case statements followed by a semicolon (;) are deprecated, use a colon (:) instead")
		}
		var label *Node
		if cs.Children[0] == nil {
			if seenDefault {
				l.fail(cs.Line, "Switch statements may only contain one default clause")
			}
			seenDefault = true
		} else {
			label = l.expr(cs.Children[0])
		}
		body := &Node{Op: OpBlock, Line: cs.Line}
		for _, st := range cs.Children[1:] {
			body.Kids = appendNode(body.Kids, l.statement(st, false))
		}
		n.Kids = append(n.Kids, l.seal(&Node{Op: OpCase, Line: cs.Line, Kids: []*Node{label, l.seal(body)}}))
	}
	l.pop()
	return n
}

func (l *lowerer) breakContinue(s *frontend.Node) *Node {
	depth := int64(1)
	line := s.Line
	if d := s.Children[0]; d != nil {
		line = zline(d) // PHP reports the line of the depth operand
		if d.Kind != "literal" {
			l.fail(line, "'%s' operator with non-integer operand is no longer supported", s.Kind)
		}
		v := l.literal(d)
		if v.Kind != LitInt || v.Int < 1 {
			l.fail(line, "'%s' operator accepts only positive integers", s.Kind)
		}
		depth = v.Int
	}
	var loops []int
	for i := len(l.regions) - 1; i >= 0; i-- {
		if l.regions[i].kind != 'f' {
			loops = append(loops, i)
		}
	}
	if len(loops) == 0 {
		l.fail(line, "'%s' not in the 'loop' or 'switch' context", s.Kind)
	}
	if depth > int64(len(loops)) {
		plural := "s"
		if depth == 1 {
			plural = ""
		}
		l.fail(line, "Cannot '%s' %d level%s", s.Kind, depth, plural)
	}
	target := loops[depth-1]
	if s.Kind == "continue" && l.regions[target].kind == 's' {
		outer := int(depth) < len(loops)
		switch {
		case depth == 1 && !outer:
			l.diag(s.Line, "Warning", `"continue" targeting switch is equivalent to "break"`)
		case depth == 1:
			l.diag(s.Line, "Warning", fmt.Sprintf(`"continue" targeting switch is equivalent to "break". Did you mean to use "continue %d"?`, depth+1))
		case !outer:
			l.diag(s.Line, "Warning", fmt.Sprintf(`"continue %d" targeting switch is equivalent to "break %d"`, depth, depth))
		default:
			l.diag(s.Line, "Warning", fmt.Sprintf(`"continue %d" targeting switch is equivalent to "break %d". Did you mean to use "continue %d"?`, depth, depth, depth+1))
		}
	}
	l.jumps = append(l.jumps, jump{kind: s.Kind, line: s.Line, regions: append([]region(nil), l.regions...), target: target})
	op := OpBreak
	if s.Kind == "continue" {
		op = OpContinue
	}
	return &Node{Op: op, Line: s.Line, Value: &Literal{Kind: LitInt, Int: depth}}
}

func (l *lowerer) returnStmt(s *frontend.Node) *Node {
	n := &Node{Op: OpReturn, Line: s.Line}
	v := s.Children[0]
	f := l.fn
	if f.Return != nil && !f.Generator {
		kind := "function"
		if l.class != nil {
			kind = "method"
		}
		switch f.Return.String {
		case "void":
			if v != nil {
				hint := ""
				if isNullLiteral(v) {
					hint = " (did you mean \"return;\" instead of \"return null;\"?)"
				}
				l.fail(s.Line, "A void %s must not return a value%s", kind, hint)
			}
		case "never":
			l.fail(s.Line, "A never-returning %s must not return", kind)
		default:
			if v == nil && f.Kind != FuncMain {
				hint := ""
				if typeAllowsNull(f.Return.String) || strings.HasPrefix(f.Return.String, "?") {
					hint = " (did you mean \"return null;\" instead of \"return;\"?)"
				}
				l.fail(s.Line, "A %s with return type must return a value%s", kind, hint)
			}
		}
	}
	if v != nil {
		if f.ByRef && (isVariableLike(v) || shortCircuited(unparen(v))) {
			n.Kids = []*Node{l.refTarget(v)}
		} else {
			n.Kids = []*Node{l.expr(v)}
		}
		if f.ByRef {
			// A by-reference return binds the result; returning a value
			// that is not a variable raises a notice.
			n.Flags |= FlagRef
		}
		if f.Return != nil && f.Return.String != "mixed" && !f.Generator {
			n.Flags |= FlagCoerce
		}
	}
	return n
}

func isNullLiteral(n *frontend.Node) bool {
	return n != nil && n.Kind == "name" && strings.EqualFold(n.Text, "null")
}

func (l *lowerer) globalVar(v *frontend.Node) *Node {
	if v.Kind == "variable" {
		name := v.Text[1:]
		if name == "this" {
			l.fail(v.Line, "Cannot use $this as global variable")
		}
		if isSuperGlobal(name) {
			return &Node{Op: OpGlobal, Line: v.Line, Text: name}
		}
		vr := l.variable(name, VarGlobal|VarRef|VarWritten)
		return &Node{Op: OpGlobal, Line: v.Line, Var: vr, Text: name}
	}
	l.fn.DynamicScope = true
	return &Node{Op: OpGlobal, Line: v.Line, Kids: []*Node{l.expr(v.Children[0])}}
}

func (l *lowerer) staticVar(sv *frontend.Node) *Node {
	name := sv.Children[0].Text[1:]
	if name == "this" {
		l.fail(sv.Line, "Cannot use $this as static variable")
	}
	if l.fn.statics == nil {
		l.fn.statics = map[string]bool{}
	}
	if l.fn.statics[name] {
		l.fail(sv.Line, "Duplicate declaration of static variable $%s", name)
	}
	l.fn.statics[name] = true
	n := &Node{Op: OpStatic, Line: sv.Line, Text: name, Var: l.variable(name, VarStatic|VarRef|VarWritten)}
	if init := sv.Children[1]; init != nil {
		n.Kids = []*Node{l.expr(init)}
	}
	return n
}

func (l *lowerer) declare(s *frontend.Node) *Node {
	directives := s.Children
	var body *frontend.Node
	if last := directives[len(directives)-1]; last.Kind != "directive" {
		body = last
		directives = directives[:len(directives)-1]
	}
	n := &Node{Op: OpDeclare, Line: s.Line}
	for _, d := range directives {
		name, value := d.Children[0].Text, d.Children[1]
		if value.Kind != "literal" && !isConstantLiteral(value) {
			l.fail(s.Line, "declare(%s) value must be a literal", name)
		}
		switch strings.ToLower(name) {
		case "ticks":
		case "encoding":
			if !l.isFirstStatement(s) {
				l.fail(s.Line, "Encoding declaration pragma must be the very first statement in the script")
			}
			l.diag(s.Line, "Warning", "declare(encoding=...) ignored because Zend multibyte feature is turned off by settings")
		case "strict_types":
			if !l.isFirstStatement(s) {
				l.fail(s.Line, "strict_types declaration must be the very first statement in the script")
			}
			if body != nil {
				l.fail(s.Line, "strict_types declaration must not use block mode")
			}
			v := l.constantValue(value)
			if v == nil || v.Kind != LitInt || (v.Int != 0 && v.Int != 1) {
				l.fail(s.Line, "strict_types declaration must have 0 or 1 as its value")
			}
			if v.Int == 1 {
				l.unit.Strict = true
				l.fn.Strict = true
			}
		default:
			l.diag(s.Line, "Warning", fmt.Sprintf("Unsupported declare '%s'", name))
		}
		n.Text = strings.ToLower(name)
		n.Value = l.constantValue(value)
	}
	ticks := false
	for _, d := range directives {
		if v := l.constantValue(d.Children[1]); strings.EqualFold(d.Children[0].Text, "ticks") && v != nil && v.Kind == LitInt && v.Int > 0 {
			ticks = true
		}
	}
	if body != nil {
		saved := l.ticks
		l.ticks = l.ticks || ticks
		n.Kids = []*Node{l.block(body)}
		l.ticks = saved
	} else if ticks {
		l.ticks = true // the rest of the file ticks
	}
	return n
}

// isConstantLiteral reports a name PHP's parser folds into a literal, such
// as true or null.
func isConstantLiteral(n *frontend.Node) bool {
	return n.Kind == "name" && isBuiltinConstant(n.Text)
}

func (l *lowerer) try(s *frontend.Node) *Node {
	c := s.Children
	if len(c) == 1 {
		l.fail(s.Line, "Cannot use try without catch or finally")
	}
	n := &Node{Op: OpTry, Line: s.Line, Kids: []*Node{l.block(c[0])}}
	for _, h := range c[1:] {
		if h.Kind == "catch" {
			cn := &Node{Op: OpCatch, Line: h.Line}
			for _, t := range h.Children[0].Children {
				if specialClass(t.Text) != "" {
					l.fail(h.Line, "Bad class name in the catch statement")
				}
				name := l.classRef(t)
				cn.Kids = append(cn.Kids, l.seal(&Node{Op: OpClassName, Line: t.Line, Name: name}))
			}
			if v := h.Children[1]; v != nil {
				if v.Text == "$this" {
					l.fail(h.Line, "Cannot re-assign $this")
				}
				cn.Var = l.variable(v.Text[1:], VarWritten)
			}
			cn.Kids = append(cn.Kids, l.block(h.Children[2]))
			n.Kids = append(n.Kids, l.seal(cn))
			continue
		}
		l.push('f', h.Line)
		body := l.block(h.Children[0])
		l.pop()
		n.Kids = append(n.Kids, l.seal(&Node{Op: OpFinally, Line: h.Line, Kids: []*Node{body}}))
	}
	return n
}

func (l *lowerer) constDecl(s *frontend.Node) *Node {
	b := &Node{Op: OpBlock, Line: s.Line}
	for _, item := range s.Children {
		if item == nil || item.Kind != "const_item" {
			continue // attributes
		}
		short := item.Children[0].Text
		if isBuiltinConstant(short) {
			l.fail(item.Line, "Cannot redeclare constant '%s'", short)
		}
		full := l.ns.prefix(short)
		if imp, ok := l.ns.constants[short]; ok && imp != full {
			l.fail(item.Line, "Cannot declare const %s because the name is already in use", full)
		}
		l.see("const", full)
		value := l.constExpr(item.Children[1], true)
		b.Kids = append(b.Kids, l.seal(&Node{Op: OpConstDef, Line: item.Line, Name: &Name{Kind: ConstantName, Resolved: full}, Kids: []*Node{value}}))
	}
	return b
}

// --- functions ---

func (l *lowerer) function(s *frontend.Node, top bool) *Func {
	c := s.Children
	short := c[1].Text
	full := l.ns.prefix(short)
	f := l.newFunc(FuncNamed, full, s.Line)
	f.ByRef = s.Text == "&"
	lower := strings.ToLower(short)
	switch lower {
	case "__autoload":
		l.fail(s.Line, "__autoload() is no longer supported, use spl_autoload_register() instead")
	case "assert":
		if l.ns.name != "" {
			l.fail(s.Line, "Defining a custom assert() function is not allowed, as the function has special semantics")
		}
	}
	if imp, ok := l.ns.functions[lower]; ok && !strings.EqualFold(imp, full) {
		l.fail(s.Line, "Cannot redeclare function %s() (previously declared as local import)", full)
	}
	l.see("function", full)
	f.Hoisted = top && l.fn.Kind == FuncMain
	l.unit.Functions = append(l.unit.Functions, f)
	l.functionBody(f, c[2], c[3], c[4], nil)
	if f.Hoisted {
		key := strings.ToLower(full)
		if prev, dup := l.hoisted[key]; dup {
			l.fail(s.Line, "Cannot redeclare function %s() (previously declared in %s:%d)", full, l.unit.File, prev)
		}
		l.hoisted[key] = s.Line
	}
	return f
}

// functionBody compiles parameters, return type and body of f.
func (l *lowerer) functionBody(f *Func, params, ret, body *frontend.Node, uses *frontend.Node) {
	restore := l.enterFunc(f)
	defer restore()
	savedClass := l.class
	if f.Kind == FuncNamed {
		l.class = nil
	}
	defer func() { l.class = savedClass }()
	f.Class = l.class
	if ret != nil {
		f.Return = l.typeOf(ret, false)
		if f.Return.String == "void" && f.ByRef {
			l.diag(ret.Line, "Deprecated", fmt.Sprintf("%s(): Returning by reference from a void function is deprecated", l.funcDisplayName(f)))
		}
	}
	// PHP's parser marks generators, so the return type is checked before
	// the body is compiled.
	if containsYield(body) {
		f.Generator = true
		if f.Return != nil && !generatorCompatible(f.Return.zend) {
			l.fail(ret.Line, "Generator return type must be a supertype of Generator, %s given", f.Return.zend)
		}
	}
	l.params(f, params)
	if uses != nil {
		l.closureUses(f, uses)
	}
	switch {
	case body == nil:
	case f.Kind == FuncArrow:
		f.Body = []*Node{l.seal(&Node{Op: OpReturn, Line: body.Line, Kids: []*Node{l.expr(body)}})}
	case body.Kind == "block":
		for _, s := range body.Children {
			f.Body = appendNode(f.Body, l.statement(s, false))
		}
	default:
		f.Body = []*Node{l.seal(&Node{Op: OpReturn, Line: body.Line, Kids: []*Node{l.expr(body)}})}
	}
	l.finishFunc(f)
}

// containsYield reports a yield in a function body, not counting nested
// functions and classes.
func containsYield(n *frontend.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case "yield", "yield_from":
		return true
	case "function", "closure", "arrow_function", "class", "interface", "trait", "enum", "new_class":
		return false
	}
	for _, c := range n.Children {
		if containsYield(c) {
			return true
		}
	}
	return false
}

func generatorCompatible(t string) bool {
	for _, part := range strings.Split(strings.TrimPrefix(t, "?"), "|") {
		switch strings.ToLower(strings.Trim(part, "()")) {
		case "mixed", "object", "iterable", "traversable", "iterator", "generator":
			return true
		}
		if strings.Contains(part, "Traversable") {
			return true
		}
	}
	return false
}

func (l *lowerer) params(f *Func, list *frontend.Node) {
	seen := map[string]bool{}
	lastRequired := -1
	for i, p := range list.Children {
		if p.Children[4] == nil && !strings.Contains(p.Text, "...") {
			lastRequired = i
		}
	}
	for i, p := range list.Children {
		attrs, mods, typ, v, def := p.Children[0], p.Children[1], p.Children[2], p.Children[3], p.Children[4]
		_ = attrs
		name := v.Text[1:]
		param := &Param{Name: name, Line: p.Line, ByRef: strings.Contains(p.Text, "&"), Variadic: strings.Contains(p.Text, "...")}
		if isSuperGlobal(name) {
			l.fail(p.Line, "Cannot re-assign auto-global variable %s", name)
		}
		if seen[name] {
			l.fail(p.Line, "Redefinition of parameter $%s", name)
		}
		seen[name] = true
		if name == "this" {
			l.fail(p.Line, "Cannot use $this as parameter")
		}
		if f.Variadic {
			l.fail(p.Line, "Only the last parameter can be variadic")
		}
		if param.Variadic {
			f.Variadic = true
			if def != nil {
				l.fail(p.Line, "Variadic parameter cannot have a default value")
			}
		}
		if mods != nil {
			for _, m := range mods.Children {
				param.Promoted = append(param.Promoted, strings.ToLower(m.Text))
			}
		}
		var defKind string // PHP type of a constant default, "" for an expression
		forced := false
		if def != nil {
			param.Default = l.constExpr(def, true)
			defKind = constKind(param.Default)
		}
		if typ != nil {
			param.Type = l.typeOf(typ, false)
			forceNull := defKind == "null" && len(param.Promoted) == 0
			if forceNull && !strings.HasPrefix(param.Type.String, "?") && !typeAllowsNull(param.Type.String) {
				forced = true
				l.diag(p.Line, "Deprecated", fmt.Sprintf("%s(): Implicitly marking parameter $%s as nullable is deprecated, the explicit nullable type must be used instead", l.funcDisplayName(f), name))
				param.Type = &Type{String: nullableType(param.Type.String), Implicit: true, zend: nullableType(param.Type.zend)}
			}
			switch param.Type.String {
			case "void":
				l.fail(p.Line, "void cannot be used as a parameter type")
			case "never":
				l.fail(p.Line, "never cannot be used as a parameter type")
			}
			if defKind != "" && !forceNull && !validDefault(param.Type.zend, param.Default.Value, defKind) {
				l.fail(p.Line, "Cannot use %s as default value for parameter $%s of type %s", defKind, name, param.Type.zend)
			}
		}
		if def != nil && i < lastRequired && !forced {
			required := list.Children[lastRequired].Children[3].Text[1:]
			l.diag(p.Line, "Deprecated", fmt.Sprintf("%s(): Optional parameter $%s declared before required parameter $%s is implicitly treated as a required parameter", l.funcDisplayName(f), name, required))
			// PHP compiles the parameter as required and drops the default.
			param.Default = nil
		}
		param.Var = l.variable(name, VarParam|VarWritten)
		if param.ByRef {
			param.Var.Flags |= VarRef
		}
		if len(param.Promoted) > 0 {
			if f.Kind != FuncMethod || !strings.EqualFold(f.Name, "__construct") {
				l.fail(p.Line, "Cannot declare promoted property outside a constructor")
			}
			if param.Variadic {
				l.fail(p.Line, "Cannot declare variadic promoted property")
			}
		}
		f.Params = append(f.Params, param)
	}
}

func (l *lowerer) funcDisplayName(f *Func) string {
	if f.Kind == FuncMethod && l.class != nil {
		return l.class.Name + "::" + f.Name
	}
	return f.Name
}

func typeAllowsNull(t string) bool {
	if t == "mixed" || t == "null" {
		return true
	}
	for _, part := range strings.Split(t, "|") {
		if part == "null" {
			return true
		}
	}
	return false
}

// nullableType adds null to a type string as zend_type_to_string prints it.
func nullableType(t string) string {
	if !strings.ContainsAny(t, "|&") {
		return "?" + t
	}
	if strings.Contains(t, "&") && !strings.Contains(t, "|") {
		return "(" + t + ")|null"
	}
	return t + "|null"
}

func (l *lowerer) closureUses(f *Func, uses *frontend.Node) {
	// PHP checks each use before moving its current line to it, so errors
	// carry the line of the previous use, or of the use list for the first.
	line := uses.Line
	for _, u := range uses.Children {
		prev := line
		line = u.Line
		name := u.Children[0].Text[1:]
		if name == "this" {
			l.fail(prev, "Cannot use $this as lexical variable")
		}
		if isSuperGlobal(name) {
			l.fail(prev, "Cannot use auto-global as lexical variable")
		}
		for _, c := range f.Captures {
			if c.Name == name {
				l.fail(prev, "Cannot use variable $%s twice", name)
			}
		}
		if v := f.vars[name]; v != nil && v.Flags&VarParam != 0 {
			l.fail(prev, "Cannot use lexical variable $%s as a parameter name", name)
		}
		byRef := u.Text == "&"
		if f.statics == nil {
			f.statics = map[string]bool{}
		}
		f.statics[name] = true
		f.Captures = append(f.Captures, &Capture{Name: name, ByRef: byRef, Line: u.Line})
		flags := VarCaptured | VarWritten
		if byRef {
			flags |= VarCapturedRef
		}
		l.variable(name, flags)
	}
}

// closureName is the name PHP 8.4+ gives a closure: its defining function
// or file and the line.
func (l *lowerer) closureName(line int) string {
	scope := l.unit.File
	for f := l.fn; f != nil; f = f.Parent {
		if f.Kind == FuncNamed || f.Kind == FuncMethod {
			if f.Kind == FuncMethod && f.Class != nil {
				scope = f.Class.Name + "::" + f.Name + "()"
			} else {
				scope = f.Name + "()"
			}
			break
		}
		if f.Kind == FuncMain {
			break
		}
		if f.Kind == FuncClosure || f.Kind == FuncArrow {
			continue
		}
	}
	return fmt.Sprintf("{closure:%s:%d}", scope, line)
}

// --- variables ---

// variable returns the named variable of the current function, creating it
// on first use, and records how it is used.
func (l *lowerer) variable(name string, flags VarFlag) *Var {
	f := l.fn
	v := f.vars[name]
	if v == nil {
		v = &Var{Name: name}
		f.vars[name] = v
		f.Vars = append(f.Vars, v)
	}
	v.Flags |= flags
	return v
}

// implicitBinds lists the variables an arrow function captures, as PHP's
// find_implicit_binds: every variable named in the body, the use lists of
// nested closures and the bodies of nested arrow functions, except $this,
// superglobals and the function's own parameters.
func implicitBinds(params, body *frontend.Node) []string {
	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if name == "this" || isSuperGlobal(name) || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	var walk func(n *frontend.Node)
	walk = func(n *frontend.Node) {
		if n == nil {
			return
		}
		switch n.Kind {
		case "variable":
			add(n.Text[1:])
			return
		case "variable_name":
			add(n.Text)
			return
		case "closure":
			if uses := n.Children[3]; uses != nil {
				for _, u := range uses.Children {
					add(u.Children[0].Text[1:])
				}
			}
			return
		case "arrow_function":
			walk(n.Children[4])
			return
		case "new_class":
			walk(n.Children[2])
			return
		case "class", "interface", "trait", "enum", "function":
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(body)
	var out []string
	for _, name := range names {
		isParam := false
		for _, p := range params.Children {
			if p.Children[3].Text[1:] == name {
				isParam = true
			}
		}
		if !isParam {
			out = append(out, name)
		}
	}
	return out
}

var superGlobals = map[string]bool{
	"GLOBALS": true, "_SERVER": true, "_GET": true, "_POST": true, "_FILES": true,
	"_COOKIE": true, "_SESSION": true, "_REQUEST": true, "_ENV": true,
}

func isSuperGlobal(name string) bool { return superGlobals[name] }

func isThis(n *frontend.Node) bool { return n != nil && n.Kind == "variable" && n.Text == "$this" }

// fileDir is __DIR__ for a file path.
func fileDir(file string) string {
	if file == "" {
		return ""
	}
	return path.Dir(file)
}
