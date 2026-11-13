package ir

import (
	"math"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/phpname"
	"github.com/HazelnutParadise/phego/internal/phpnum"
)

// Array elements and declaration constants are simplified before ordinary
// expression compilation. This pass never compiles dynamic calls or bodies.
// Selected preserves a branch even when its value is still dynamic.
type syntaxConstEntry struct {
	value    compileConst
	known    bool
	selected *frontend.Node
	height   int
	line     int // CG(zend_lineno) when the value was folded
}

func (l *lowerer) preEvaluateSyntax(n *frontend.Node) syntaxConstEntry {
	saved := l.foldLine
	l.foldLine = l.cur
	defer func() { l.foldLine = saved }()
	e := l.syntaxConstant(n, 0)
	if l.constants.exhausted {
		panic(compileFailure{&LimitError{Line: n.Line}})
	}
	return e
}

func (l *lowerer) syntaxConstant(n *frontend.Node, depth int) (entry syntaxConstEntry) {
	if n == nil {
		return
	}
	n = unparen(n)
	b := l.constants
	if !b.depth(depth) || !b.spendWork(1) {
		return
	}
	if old, ok := b.syntax[n]; ok {
		if !b.depth(depth + old.height) {
			return
		}
		if i := len(b.depths) - 1; i >= 0 {
			b.depths[i] = max(b.depths[i], depth+old.height)
		}
		return old
	}
	if b.syntax == nil {
		b.syntax = make(map[*frontend.Node]syntaxConstEntry)
	}
	i := len(b.depths)
	b.depths = append(b.depths, depth)
	completed := false
	defer func() {
		deepest := b.depths[i]
		b.depths = b.depths[:i]
		if i > 0 {
			b.depths[i-1] = max(b.depths[i-1], deepest)
		}
		entry.height = deepest - depth
		entry.line = l.foldLine
		if completed && !b.exhausted {
			b.syntax[n] = entry
		}
	}()
	entry = l.syntaxConstantValue(n, depth)
	completed = true
	return
}

func syntaxScalar(v *Literal) syntaxConstEntry {
	if v == nil {
		return syntaxConstEntry{}
	}
	return syntaxConstEntry{known: true, value: compileConst{value: v,
		className: v.Kind == LitString || v.Kind == LitInt || v.Kind == LitFloat}}
}

func (l *lowerer) syntaxConstantValue(n *frontend.Node, depth int) syntaxConstEntry {
	b := l.constants
	width := l.unit.Profile.IntWidth
	c := n.Children
	visit := func(child *frontend.Node) syntaxConstEntry { return l.syntaxConstant(child, depth+1) }
	result := func(v compileConst, known bool) syntaxConstEntry {
		return syntaxConstEntry{value: v, known: known}
	}
	switch n.Kind {
	case "literal":
		return syntaxScalar(l.literal(n))
	case "name":
		return syntaxScalar(lit(l.constant(n)))
	case "magic_const":
		return syntaxScalar(lit(l.magic(n)))
	case "interpolated", "heredoc":
		if n.Text == "`" {
			return syntaxConstEntry{}
		}
		for _, part := range c {
			if part.Kind != "string_part" && part.Kind != "heredoc_end" {
				return syntaxConstEntry{}
			}
		}
		return syntaxScalar(lit(l.interpolation(n)))
	case "array":
		if !b.spendWork(len(c)) {
			return syntaxConstEntry{}
		}
		all := true
		lastLine := l.cur
		for _, item := range c {
			if item == nil {
				l.fail(lastLine, "Cannot use empty array elements in arrays")
			}
			lastLine = l.zline(item)
			if item.Kind == "spread" {
				e := visit(item.Children[0])
				all = all && e.known
				continue
			}
			value := item.Children[1]
			v := visit(value)
			k := syntaxConstEntry{known: true}
			if item.Children[0] != nil {
				k = visit(item.Children[0])
			}
			all = all && v.known && k.known && value.Kind != "by_ref"
		}
		if !all {
			return syntaxConstEntry{}
		}
		entries := func(yield func(compileArrayElement) bool) {
			for _, item := range c {
				e := compileArrayElement{line: item.Line, spread: item.Kind == "spread"}
				if e.spread {
					e.value = b.syntax[unparen(item.Children[0])].value
				} else {
					e.value = b.syntax[unparen(item.Children[1])].value
					if key := item.Children[0]; key != nil {
						e.key = b.syntax[unparen(key)].value
						e.line = key.Line
					}
				}
				if !yield(e) {
					return
				}
			}
		}
		return result(l.assembleConstantArray(entries, len(c), width, b))
	case "unary":
		if syntaxChainOperand(n) == nil {
			return syntaxConstEntry{}
		}
		return l.syntaxChain(n, depth)
	case "binary":
		op := phpname.Lower(n.Text)
		if op == "|>" {
			return syntaxConstEntry{}
		}
		if op == "??" {
			return l.syntaxRightChain(n, depth)
		}
		if op == "**" && rightLink(n, "**") != nil {
			return l.syntaxRightChain(n, depth)
		}
		return l.syntaxChain(n, depth)
	case "ternary":
		return l.syntaxChain(n, depth)
	case "cast":
		kind := castNames[phpname.Lower(strings.Trim(n.Text, "() \t"))]
		if kind == "unset" {
			l.fail(n.Line, "The (unset) cast is no longer supported")
		}
		a := visit(c[0])
		if a.known {
			return result(compileConstantCast(kind, a.value, width, b))
		}
	case "dim":
		if c[1] == nil {
			l.fail(n.Line, "Cannot use [] for reading")
		}
		a, key := visit(c[0]), visit(c[1])
		if a.known && key.known {
			return result(compileConstantDim(a.value, key.value, width, b))
		}
	case "class_name":
		// zend_try_compile_const_expr_resolve_class_name, at CG(zend_lineno)
		// while the expression is evaluated.
		cls := unparen(c[0])
		if c[0].Kind == "name" {
			// A fully qualified \self names the same fetch as self.
			special := specialClass(strings.TrimPrefix(cls.Text, "\\"))
			if special != "" {
				l.ensureClassFetch(special, l.cur)
				if name, ok := l.compileTimeClassName(special); ok {
					return syntaxScalar(&Literal{Kind: LitString, Str: name})
				}
				return syntaxConstEntry{}
			}
			name := l.classRef(cls)
			return syntaxScalar(&Literal{Kind: LitString, Str: name.Resolved})
		}
		if v, isString, ok := parseTimeValue(cls); ok {
			if !isString {
				l.fail(l.cur, "Illegal class name")
			}
			special := specialClass(v)
			if special == "" {
				return syntaxScalar(&Literal{Kind: LitString, Str: l.fullyQualifiedClass(v)})
			}
			l.ensureClassFetch(special, l.cur)
			if name, ok := l.compileTimeClassName(special); ok {
				return syntaxScalar(&Literal{Kind: LitString, Str: name})
			}
		}
	case "class_const":
		// The class of a class constant is resolved as it is evaluated.
		if cls := unparen(c[0]); c[0].Kind != "name" {
			if v, isString, ok := parseTimeValue(cls); ok {
				if !isString {
					l.fail(l.cur, "Illegal class name")
				}
				l.fullyQualifiedClass(v)
			}
		}
		for _, child := range c {
			visit(child)
		}
	case "prop", "nullsafe_prop", "new":
		for _, child := range c {
			visit(child)
		}
	case "args":
		for _, child := range c {
			visit(child)
		}
	case "named_arg":
		visit(c[0])
	}
	return syntaxConstEntry{}
}

// syntaxChainOperand returns the operand a constant unary, binary
// (except ?? and |>), logical or ternary operation is built on, the link
// syntaxChain follows, or nil for any other node.
func syntaxChainOperand(n *frontend.Node) *frontend.Node {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case "unary":
		switch n.Text {
		case "!", "~", "+", "-":
			return n.Children[0]
		}
	case "binary":
		if op := phpname.Lower(n.Text); op != "??" && op != "|>" {
			return n.Children[0]
		}
	case "ternary":
		return n.Children[0]
	}
	return nil
}

// syntaxChain folds a chain of operations nested through their first
// operand iteratively (see compileChain), visiting operands in the order
// recursive evaluation does. Lowering folds the longest known prefix when a
// later operand is dynamic, so that prefix keeps its value, the unknown
// steps above it keep their entries, and a ternary keeps its selected
// branch.
func (l *lowerer) syntaxChain(n *frontend.Node, depth int) syntaxConstEntry {
	b := l.constants
	width := l.unit.Profile.IntWidth
	spine := []*frontend.Node{n}
	leaf, leafDepth := unparen(syntaxChainOperand(n)), depth+1
	for syntaxChainOperand(leaf) != nil {
		if _, ok := b.syntax[leaf]; ok {
			leafDepth = depth // a memoized link stays at this frame's depth
			break
		}
		if !b.spendWork(1) {
			return syntaxConstEntry{}
		}
		spine = append(spine, leaf)
		leaf = unparen(syntaxChainOperand(leaf))
	}
	first := l.syntaxConstant(leaf, leafDepth)
	known := first.known
	f := binaryFold{acc: first.value}
	var result syntaxConstEntry // the root entry when it is not known
	for i := len(spine) - 1; i >= 0; i-- {
		nd := spine[i]
		c := nd.Children
		op := phpname.Lower(nd.Text)
		entry := syntaxConstEntry{} // this step's entry when it fails
		ok := false
		switch nd.Kind {
		case "unary":
			if known && !b.exhausted {
				var v compileConst
				if v, ok = compileUnaryValue(nd.Text, f.value(), width, b); ok {
					f = binaryFold{acc: v}
				}
			}
		case "binary":
			right := l.syntaxConstant(c[1], depth+1)
			if !known {
				break
			}
			if op == "&&" || op == "and" || op == "||" || op == "or" || op == "xor" {
				// Unlike ordinary logical compilation, the constant syntax
				// pass visits both sides before testing the left operand. A
				// xor folds only when both sides are constant.
				if op == "xor" && !right.known {
					break
				}
				av, truthOK := l.compileTruth(f.value())
				if !truthOK {
					break
				}
				if op != "xor" && av == (op == "||" || op == "or") {
					f, ok = binaryFold{acc: syntaxScalar(&Literal{Kind: LitBool, Bool: av}).value}, true
					break
				}
				if right.known {
					if bv, truthOK := l.compileTruth(right.value); truthOK {
						if op == "xor" {
							bv = av != bv
						}
						f, ok = binaryFold{acc: syntaxScalar(&Literal{Kind: LitBool, Bool: bv}).value}, true
					}
				}
				break
			}
			joined, left := f.cat != nil, f.acc
			if right.known && !b.exhausted && f.step(op, right.value, width, depth, b) {
				ok = true
				// concat_function converts each constant operand once.
				if op == "." {
					if !joined {
						l.compileString(left)
					}
					l.compileString(right.value)
				}
			}
		case "ternary":
			if known {
				v := f.value()
				if truth, truthOK := l.compileTruth(v); truthOK {
					selected := c[2]
					if truth {
						selected = c[1]
						if selected == nil {
							selected = c[0]
						}
					}
					e := syntaxConstEntry{known: true, value: v}
					if selected != c[0] {
						e = l.syntaxConstant(selected, depth+1)
					}
					e.selected = selected
					if e.known {
						f, ok = binaryFold{acc: e.value}, true
					} else {
						entry = e
					}
					break
				}
			}
			if c[1] != nil {
				l.syntaxConstant(c[1], depth+1)
			}
			l.syntaxConstant(c[2], depth+1)
		}
		if ok {
			continue
		}
		if known && i < len(spine)-1 && !l.recordSyntax(spine[i+1], syntaxConstEntry{known: true, value: f.value()}, depth) {
			return syntaxConstEntry{}
		}
		known = false
		if i == 0 {
			result = entry
		} else if !l.recordSyntax(nd, entry, depth) {
			return syntaxConstEntry{}
		}
	}
	if !known {
		return result
	}
	return syntaxConstEntry{known: true, value: f.value()}
}

// rightLink returns n's right operand when it continues a right-associative
// chain of op (?? or **), or nil.
func rightLink(n *frontend.Node, op string) *frontend.Node {
	if n == nil || n.Kind != "binary" || phpname.Lower(n.Text) != op {
		return nil
	}
	if r := unparen(n.Children[1]); r != nil && r.Kind == "binary" && phpname.Lower(r.Text) == op {
		return r
	}
	return nil
}

// syntaxRightChain folds a right-nested chain of ?? or ** iteratively,
// visiting the left operands in order as recursive evaluation does. A ??
// stops at its first constant non-null operand and selects it; a null one
// selects the rest of the chain.
func (l *lowerer) syntaxRightChain(n *frontend.Node, depth int) syntaxConstEntry {
	b := l.constants
	op := phpname.Lower(n.Text)
	spine := []*frontend.Node{n}
	for r := rightLink(n, op); r != nil; r = rightLink(r, op) {
		if _, ok := b.syntax[r]; ok {
			break
		}
		if !b.spendWork(1) {
			return syntaxConstEntry{}
		}
		spine = append(spine, r)
	}
	isNull := func(e syntaxConstEntry) bool {
		return !e.value.array && e.value.value != nil && e.value.value.Kind == LitNull
	}
	lefts := make([]syntaxConstEntry, len(spine))
	cut := len(spine) // the ?? that selects its left operand, if any
	for i, nd := range spine {
		lefts[i] = l.syntaxConstant(nd.Children[0], depth+1)
		if op == "??" && lefts[i].known && !isNull(lefts[i]) {
			cut = i
			break
		}
	}
	var next syntaxConstEntry
	if cut == len(spine) {
		next = l.syntaxConstant(spine[len(spine)-1].Children[1], depth+1)
	}
	for i := min(cut, len(spine)-1); i >= 0; i-- {
		nd := spine[i]
		var e syntaxConstEntry
		switch {
		case op == "??" && i == cut:
			e = lefts[i]
			e.selected = nd.Children[0]
		case op == "??" && lefts[i].known:
			e = next
			e.selected = nd.Children[1]
		case op == "**" && lefts[i].known && next.known && !b.exhausted:
			v, ok := compileBinaryValues(op, lefts[i].value, next.value, l.unit.Profile.IntWidth, depth, b)
			e = syntaxConstEntry{value: v, known: ok}
		}
		if i != 0 && !l.recordSyntax(nd, e, depth) {
			return syntaxConstEntry{}
		}
		next = e
	}
	return next
}

// recordSyntax memoizes an intermediate entry of an iterative chain at the
// height its enclosing frame has reached, which never understates it. Its
// walk step already paid the entry's work.
func (l *lowerer) recordSyntax(n *frontend.Node, entry syntaxConstEntry, depth int) bool {
	b := l.constants
	if b.exhausted {
		return false
	}
	if i := len(b.depths) - 1; i >= 0 {
		entry.height = b.depths[i] - depth
	}
	entry.line = l.foldLine
	b.syntax[n] = entry
	return true
}

func compileConstantCast(kind string, a compileConst, width int, b *compileBudget) (compileConst, bool) {
	if a.value != nil && a.value.Kind == LitFloat && math.IsNaN(a.value.Float) {
		return compileConst{}, false
	}
	if kind == "object" {
		return compileConst{}, false
	}
	if kind == "array" {
		if a.array {
			return a, true
		}
		array := &compileArray{positions: make(map[compileKey]int)}
		if a.value.Kind != LitNull {
			array.put(compileKey{integer: true}, a, width)
		}
		return compileConst{array: true, elements: array}, true
	}
	if kind == "bool" {
		truth, ok := a.truth()
		return scalarConst(&Literal{Kind: LitBool, Bool: truth}), ok
	}
	if a.array {
		if kind != "int" && kind != "float" {
			return compileConst{}, false
		}
		truth, ok := a.truth()
		if !ok {
			return compileConst{}, false
		}
		a = scalarConst(&Literal{Kind: LitBool, Bool: truth})
	}
	if !b.scan(a.value) {
		return compileConst{}, false
	}
	v := a.value
	switch kind {
	case "string":
		if v.Kind == LitFloat {
			return compileConst{}, false
		}
		s, fresh, ok := constStringFresh(v)
		return scalarConst(&Literal{Kind: LitString, Str: s, Uninterned: fresh}), ok
	case "int", "float":
		if v.Kind == LitString {
			n := phpnum.Parse(v.Str, width, true)
			if n.Kind == phpnum.Integer {
				v = &Literal{Kind: LitInt, Int: n.Int}
			} else if n.Kind == phpnum.NotNumeric {
				v = &Literal{Kind: LitInt}
			} else {
				if kind == "int" {
					if math.IsInf(n.Float, 0) {
						return scalarConst(&Literal{Kind: LitInt}), true
					}
					limit := math.Ldexp(1, width-1)
					if n.Float >= limit {
						return scalarConst(&Literal{Kind: LitInt, Int: int64(uint64(1)<<(width-1) - 1)}), true
					}
					if n.Float < -limit {
						return scalarConst(&Literal{Kind: LitInt, Int: -int64(uint64(1) << (width - 1))}), true
					}
				}
				v = &Literal{Kind: LitFloat, Float: n.Float}
			}
		}
		n, ok := constNumber(v, width)
		if !ok {
			return compileConst{}, false
		}
		if kind == "float" {
			return scalarConst(&Literal{Kind: LitFloat, Float: n.f}), true
		}
		if n.integer {
			return scalarConst(&Literal{Kind: LitInt, Int: n.i}), true
		}
		limit := math.Ldexp(1, width-1)
		if n.f < -limit || n.f >= limit || math.IsNaN(n.f) {
			return compileConst{}, false
		}
		return scalarConst(&Literal{Kind: LitInt, Int: int64(n.f)}), true
	}
	return compileConst{}, false
}

func compileConstantDim(a, key compileConst, width int, b *compileBudget) (compileConst, bool) {
	if key.array || key.value == nil {
		return compileConst{}, false
	}
	k := key.value
	if k.Kind != LitInt && k.Kind != LitString {
		return compileConst{}, false
	}
	if a.array {
		if a.elements == nil {
			return compileConst{}, false
		}
		index, ok := compileArrayKey(key, width, b)
		if !ok {
			return compileConst{}, false
		}
		pos, found := a.elements.positions[index]
		if found {
			return a.elements.entries[pos].value, true
		}
		return compileConst{}, false
	}
	if a.value == nil || a.value.Kind != LitString || !b.scan(k) {
		return compileConst{}, false
	}
	i := k.Int
	if k.Kind == LitString {
		n := phpnum.Parse(k.Str, width, true)
		if n.Kind != phpnum.Integer {
			return compileConst{}, false
		}
		i = n.Int
	}
	if i < 0 || i >= int64(len(a.value.Str)) {
		return compileConst{}, false
	}
	return scalarConst(&Literal{Kind: LitString, Str: a.value.Str[i : i+1]}), true
}

func (l *lowerer) selectedSyntax(n *frontend.Node) *frontend.Node {
	n = unparen(n)
	for n != nil {
		e := l.constants.syntax[n]
		if e.selected == nil {
			break
		}
		n = unparen(e.selected)
	}
	return n
}

// Materialize an already evaluated value into ordinary Go HIR. The output
// retains allocation semantics; no mutable runtime array is shared in code.
func (l *lowerer) constantValueNode(value compileConst, line, depth int) *Node {
	b := l.constants
	if !b.depth(depth) || !b.spendWork(1) {
		panic(compileFailure{&LimitError{Line: line}})
	}
	if !value.array {
		return l.seal(&Node{Op: OpLiteral, Line: line, Value: value.value})
	}
	n := &Node{Op: OpArray, Line: line}
	for _, entry := range value.elements.entries {
		key := &Literal{Kind: LitString, Str: entry.key.text}
		if entry.key.integer {
			key = &Literal{Kind: LitInt, Int: entry.key.index}
		}
		k := l.seal(&Node{Op: OpLiteral, Line: line, Value: key})
		v := l.constantValueNode(entry.value, line, depth+1)
		n.Kids = append(n.Kids, l.node(OpItem, line, k, v))
	}
	return l.seal(n)
}
