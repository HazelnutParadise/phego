package ir

import (
	"math"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
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
}

func (l *lowerer) preEvaluateSyntax(n *frontend.Node) syntaxConstEntry {
	e := l.syntaxConstant(n, 0)
	if l.constants.exhausted {
		panic(compileFailure{&constantLimitError{line: n.Line}})
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
	if len(b.syntax)+len(b.memo)+len(b.depths) >= compileConstMaxTreeNodes {
		b.exhausted = true
		return
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
		if n.Text != "!" && n.Text != "~" && n.Text != "+" && n.Text != "-" {
			return syntaxConstEntry{}
		}
		return l.syntaxUnary(n, depth)
	case "binary":
		op := strings.ToLower(n.Text)
		if op == "|>" {
			return syntaxConstEntry{}
		}
		a := visit(c[0])
		if op == "??" {
			if a.known {
				selected := c[0]
				if !a.value.array && a.value.value.Kind == LitNull {
					selected = c[1]
					a = visit(selected)
				}
				a.selected = selected
				return a
			}
			visit(c[1])
			return syntaxConstEntry{}
		}
		// Unlike ordinary logical compilation, the constant syntax pass
		// visits both sides before testing the left operand's truth value.
		right := visit(c[1])
		if !a.known {
			return syntaxConstEntry{}
		}
		if op == "&&" || op == "and" || op == "||" || op == "or" || op == "xor" {
			av, ok := l.compileTruth(a.value)
			or := op == "||" || op == "or"
			if !ok {
				return syntaxConstEntry{}
			}
			if op != "xor" && av == or {
				return syntaxScalar(&Literal{Kind: LitBool, Bool: av})
			}
			if right.known {
				bv, ok := l.compileTruth(right.value)
				if ok {
					if op == "xor" {
						bv = av != bv
					}
					return syntaxScalar(&Literal{Kind: LitBool, Bool: bv})
				}
			}
			return syntaxConstEntry{}
		}
		if right.known {
			return result(compileBinaryValues(op, a.value, right.value, width, depth, b))
		}
	case "ternary":
		a := visit(c[0])
		if a.known {
			if truth, ok := l.compileTruth(a.value); ok {
				selected := c[2]
				if truth {
					selected = c[1]
					if selected == nil {
						selected = c[0]
					}
				}
				e := visit(selected)
				e.selected = selected
				return e
			}
		}
		visit(c[1])
		visit(c[2])
	case "cast":
		kind := castNames[strings.ToLower(strings.Trim(n.Text, "() \t"))]
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
		if cls := unparen(c[0]); cls.Kind == "name" && specialClass(cls.Text) == "" {
			name := l.classRef(cls)
			return syntaxScalar(&Literal{Kind: LitString, Str: name.Resolved})
		}
	case "class_const", "prop", "nullsafe_prop", "new":
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

// A unary chain is a linear sequence of work, not recursive nesting. Pending
// entries reserve the same bounded frame slots as recursive evaluation, so a
// large leaf cannot consume the space needed by the rest of the chain.
func (l *lowerer) syntaxUnary(n *frontend.Node, depth int) syntaxConstEntry {
	b := l.constants
	chain := []*frontend.Node{n}
	child := unparen(n.Children[0])
	var value syntaxConstEntry
	for {
		if child == nil || child.Kind != "unary" || (child.Text != "!" && child.Text != "~" && child.Text != "+" && child.Text != "-") {
			value = l.syntaxConstant(child, depth+1)
			break
		}
		if !b.spendWork(1) {
			return syntaxConstEntry{}
		}
		if old, ok := b.syntax[child]; ok {
			// A cached unary occupies the same iterative frame depth as a
			// fresh one. Charging depth+1 here would make memo hits stricter.
			if !b.depth(depth + old.height) {
				return syntaxConstEntry{}
			}
			i := len(b.depths) - 1
			b.depths[i] = max(b.depths[i], depth+old.height)
			value = old
			break
		}
		if len(b.syntax)+len(b.memo)+len(b.depths) >= compileConstMaxTreeNodes {
			b.exhausted = true
			return syntaxConstEntry{}
		}
		chain = append(chain, child)
		b.depths = append(b.depths, depth)
		child = unparen(child.Children[0])
	}
	for i := len(chain) - 1; i >= 0; i-- {
		entry := syntaxConstEntry{} // selected belongs to the operand, never this unary.
		if value.known && !b.exhausted {
			entry.value, entry.known = compileUnaryValue(chain[i].Text, value.value, l.unit.Profile.IntWidth, b)
		}
		last := len(b.depths) - 1
		deepest := b.depths[last]
		entry.height = deepest - depth
		if i != 0 {
			b.depths = b.depths[:last]
			b.depths[last-1] = max(b.depths[last-1], deepest)
			if !b.exhausted {
				b.syntax[chain[i]] = entry
			}
		}
		value = entry
	}
	return value
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
		s, ok := constString(v)
		return scalarConst(&Literal{Kind: LitString, Str: s}), ok
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
		panic(compileFailure{&constantLimitError{line: line}})
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
