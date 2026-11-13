package ir

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/phpnum"
)

// compileConstMaxDepth bounds recursion of the evaluators. Left-nested
// operation chains are iterative and do not deepen it.
const compileConstMaxDepth = 2048

type compileConst struct {
	value     *Literal
	array     bool
	elements  *compileArray
	className bool
}

// compileClassKind identifies values PHP evaluates while compiling a class
// operand. It leaves throwing, diagnostic-producing or dynamic expressions
// for execution, and does not mutate HIR or depend on an optimization pass.
func (l *lowerer) compileClassKind(n *Node) string {
	return l.classKind(n, l.evaluateConstant)
}

// probeClassKind is compileClassKind for a run-time class operand check
// (see probeConstant).
func (l *lowerer) probeClassKind(n *Node) string {
	return l.classKind(n, l.probeConstant)
}

func (l *lowerer) classKind(n *Node, evaluate func(*Node) (compileConst, bool)) string {
	// Preserve arbitrarily large direct literal types without traversing or
	// copying their payload.
	if v := lit(n); v != nil {
		if v.Kind == LitString {
			return ""
		}
		if v.Kind == LitBool {
			if v.Bool {
				return "true"
			}
			return "false"
		}
		return typeName(v)
	}
	v, ok := evaluate(n)
	if !ok {
		return ""
	}
	if v.array {
		return "array"
	}
	if v.className {
		return ""
	}
	if v.value == nil {
		return ""
	}
	if v.value.Kind == LitBool {
		if v.value.Bool {
			return "true"
		}
		return "false"
	}
	return typeName(v.value)
}

// compileConstValue implements diagnostic-free constant operations. Throwing,
// diagnostic-producing and dynamic expressions remain unknown. Array-level
// compile diagnostics are emitted only after all elements are constant.
func (l *lowerer) compileConstValue(n *Node, depth int, budget *compileBudget) (compileConst, bool) {
	switch n.Op {
	case OpLiteral:
		if n.Value == nil || n.Value.Kind > LitString {
			return compileConst{}, false
		}
		return compileConst{value: n.Value, className: n.Value.Kind == LitString || n.Value.Kind == LitInt || n.Value.Kind == LitFloat}, true
	case OpArray:
		return l.compileConstArray(n, depth, budget)
	case OpUnary, OpLogical, OpBinary:
		if chainOperand(n) == nil {
			return compileConst{}, false
		}
		if powLink(n) != nil {
			return l.compilePowChain(n, depth, budget)
		}
		return l.compileChain(n, depth, budget)
	}
	// In particular, do not fold casts or ternaries: their PHP compile-time
	// behavior is not equivalent to evaluating a scalar expression here.
	return compileConst{}, false
}

// chainOperand returns the operand a unary, binary or logical operation is
// built on, the link compileChain follows, or nil for any other node.
func chainOperand(n *Node) *Node {
	if n == nil {
		return nil
	}
	switch {
	case n.Op == OpUnary && len(n.Kids) == 1, (n.Op == OpBinary || n.Op == OpLogical) && len(n.Kids) == 2:
		return n.Kids[0]
	}
	return nil
}

// compileChain folds a chain of operations nested through their first
// operand (a unary operand or a left operand) without recursing along it,
// so a long chain of any mix of these operations is not limited by
// evaluation depth. Consecutive concatenations share one builder: each
// operand is converted and charged once, so the chain costs its result
// length rather than the sum of every intermediate string. Intermediate
// results are not memoized; the walk stops at a memoized operand, which is
// evaluated at this frame's depth as it would be fresh, so repeated probes
// stay linear and reuse does not deepen the chain.
func (l *lowerer) compileChain(n *Node, depth int, budget *compileBudget) (compileConst, bool) {
	width := l.unit.Profile.IntWidth
	spine := []*Node{n}
	leaf, leafDepth := chainOperand(n), depth+1
	for chainOperand(leaf) != nil {
		if _, ok := budget.memo[leaf]; ok {
			leafDepth = depth
			break
		}
		if !budget.spendWork(1) {
			return compileConst{}, false
		}
		spine = append(spine, leaf)
		leaf = chainOperand(leaf)
	}
	a, ok := l.compileConst(leaf, leafDepth, budget)
	if !ok {
		return compileConst{}, false
	}
	f := binaryFold{acc: a}
	for i := len(spine) - 1; i >= 0; i-- {
		nd := spine[i]
		switch nd.Op {
		case OpUnary:
			v, ok := compileUnaryValue(nd.Text, f.value(), width, budget)
			if !ok {
				return compileConst{}, false
			}
			f = binaryFold{acc: v}
		case OpBinary:
			b, ok := l.compileConst(nd.Kids[1], depth+1, budget)
			if !ok || !f.step(nd.Text, b, width, depth, budget) {
				return compileConst{}, false
			}
		case OpLogical:
			av, ok := f.value().truth()
			if !ok {
				return compileConst{}, false
			}
			if nd.Text == "&&" && !av || nd.Text == "||" && av {
				f = binaryFold{acc: scalarConst(&Literal{Kind: LitBool, Bool: av})}
				continue
			}
			b, ok := l.compileConst(nd.Kids[1], depth+1, budget)
			if !ok {
				return compileConst{}, false
			}
			bv, ok := b.truth()
			if !ok {
				return compileConst{}, false
			}
			if nd.Text == "xor" {
				bv = av != bv
			}
			f = binaryFold{acc: scalarConst(&Literal{Kind: LitBool, Bool: bv})}
		}
	}
	return f.value(), true
}

// powLink returns n's right operand when it continues a right-associative
// ** chain, or nil.
func powLink(n *Node) *Node {
	if n == nil || n.Op != OpBinary || n.Text != "**" || len(n.Kids) != 2 {
		return nil
	}
	if r := n.Kids[1]; r != nil && r.Op == OpBinary && r.Text == "**" && len(r.Kids) == 2 {
		return r
	}
	return nil
}

// compilePowChain folds a right-nested ** chain iteratively: the left
// operands in order, then the innermost right operand, combined inward out.
func (l *lowerer) compilePowChain(n *Node, depth int, budget *compileBudget) (compileConst, bool) {
	spine := []*Node{n}
	for r := powLink(n); r != nil; r = powLink(r) {
		if _, ok := budget.memo[r]; ok {
			break
		}
		if !budget.spendWork(1) {
			return compileConst{}, false
		}
		spine = append(spine, r)
	}
	lefts := make([]compileConst, len(spine))
	for i, nd := range spine {
		v, ok := l.compileConst(nd.Kids[0], depth+1, budget)
		if !ok {
			return compileConst{}, false
		}
		lefts[i] = v
	}
	v, ok := l.compileConst(spine[len(spine)-1].Kids[1], depth+1, budget)
	if !ok {
		return compileConst{}, false
	}
	for i := len(spine) - 1; i >= 0; i-- {
		if v, ok = compileBinaryValues("**", lefts[i], v, l.unit.Profile.IntWidth, depth, budget); !ok {
			return compileConst{}, false
		}
	}
	return v, true
}

// binaryFold is the running value of a left-nested binary chain.
type binaryFold struct {
	acc compileConst
	cat *concatBuilder
	// owned marks acc's array as this fold's copy, so a run of unions adds
	// each operand's new keys in place and costs the operands' total size.
	owned bool
}

// step applies op to the running value and r. On failure the running value
// is still the chain's longest known prefix.
func (f *binaryFold) step(op string, r compileConst, width, depth int, budget *compileBudget) bool {
	if op == "." {
		if f.cat != nil {
			return f.cat.add(r, budget)
		}
		f.owned = false
		c := &concatBuilder{className: true}
		if !c.add(f.acc, budget) || !c.add(r, budget) {
			return false
		}
		f.cat = c
		return true
	}
	acc := f.value()
	f.acc, f.cat = acc, nil
	if op == "+" && acc.array && r.array && acc.elements != nil && r.elements != nil {
		return f.union(r.elements, budget)
	}
	f.owned = false
	v, ok := compileBinaryValues(op, acc, r, width, depth, budget)
	if ok {
		f.acc = v
	}
	return ok
}

// union is compileArrayUnion on the running array, copying it once per run.
func (f *binaryFold) union(b *compileArray, budget *compileBudget) bool {
	if !f.owned {
		a := f.acc.elements
		if !budget.spendWork(len(a.entries)) {
			return false
		}
		out := &compileArray{positions: make(map[compileKey]int, len(a.entries)), next: a.next, full: a.full}
		for _, e := range a.entries {
			if !budget.spendWork(len(e.key.text) / 64) {
				return false
			}
			if _, exists := out.positions[e.key]; !exists {
				out.positions[e.key] = len(out.entries)
				out.entries = append(out.entries, e)
			}
		}
		f.acc, f.owned = compileConst{array: true, elements: out}, true
	}
	out := f.acc.elements
	if !budget.spendWork(len(b.entries)) {
		return false
	}
	for _, e := range b.entries {
		if !budget.spendWork(len(e.key.text) / 64) {
			return false
		}
		if _, exists := out.positions[e.key]; !exists {
			out.positions[e.key] = len(out.entries)
			out.entries = append(out.entries, e)
		}
	}
	return true
}

func (f *binaryFold) value() compileConst {
	if f.cat != nil {
		return f.cat.result()
	}
	return f.acc
}

// concatBuilder accumulates the operands of a constant concatenation.
// fresh is Literal.Uninterned of the result so far.
type concatBuilder struct {
	text      strings.Builder
	className bool
	fresh     bool
}

func (c *concatBuilder) add(v compileConst, budget *compileBudget) bool {
	if v.array || v.value == nil || !budget.scan(v.value) {
		return false
	}
	s, fresh, ok := constStringFresh(v.value)
	if !ok || !budget.spendBytes(len(s)) {
		return false
	}
	c.fresh = concatFresh(c.text.Len() == 0, c.fresh, s == "", fresh)
	c.text.WriteString(s)
	c.className = c.className && v.className
	return true
}

func (c *concatBuilder) result() compileConst {
	return compileConst{value: &Literal{Kind: LitString, Str: c.text.String(), Uninterned: c.fresh}, className: c.className}
}

// concatFresh is Literal.Uninterned of x . y: concat_function returns the
// other operand's string when one side is empty and allocates the result
// otherwise.
func concatFresh(xEmpty, xFresh, yEmpty, yFresh bool) bool {
	switch {
	case xEmpty:
		return yFresh
	case yEmpty:
		return xFresh
	}
	return true
}

// constStringFresh is constString and whether PHP's conversion
// (zval_get_string) yields a new string: a string keeps its own mark, null,
// false, true and an integer from 0 to 9 become interned strings, and other
// integers and every float are converted into a new one.
func constStringFresh(v *Literal) (s string, fresh, ok bool) {
	s, ok = constString(v)
	switch v.Kind {
	case LitString:
		fresh = v.Uninterned
	case LitInt:
		fresh = v.Int < 0 || v.Int > 9
	case LitFloat:
		fresh = true
	}
	return s, fresh, ok
}

func compileUnary(op string, a *Literal, width int, budget *compileBudget) (compileConst, bool) {
	if a == nil {
		return compileConst{}, false
	}
	switch op {
	case "!":
		if a.Kind == LitFloat {
			return compileConst{}, false
		}
		b, ok := constBool(a)
		if ok {
			return scalarConst(&Literal{Kind: LitBool, Bool: !b}), true
		}
	case "~":
		if a.Kind == LitInt || a.Kind == LitFloat {
			if i, ok := constInteger(a, width); ok {
				return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(^i, width)}), true
			}
		}
		if a.Kind == LitString {
			if !budget.scan(a) || !budget.spendBytes(2*len(a.Str)) {
				budget.exhausted = true
				return compileConst{}, false
			}
			out := make([]byte, len(a.Str))
			for i := 0; i < len(a.Str); i++ {
				out[i] = ^a.Str[i]
			}
			// bitwise_not_function interns a one-byte result only.
			return scalarConst(&Literal{Kind: LitString, Str: string(out), Uninterned: len(out) != 1}), true
		}
	case "+", "-":
		if !budget.scan(a) {
			return compileConst{}, false
		}
		if n, ok := constNumber(a, width); ok {
			if a.Kind == LitString || a.Kind == LitBool || a.Kind == LitNull {
				if n.integer {
					if op == "+" {
						return scalarConst(&Literal{Kind: LitInt, Int: n.i}), true
					}
					if width == 32 && n.i == math.MinInt32 || width != 32 && n.i == math.MinInt64 {
						return scalarConst(&Literal{Kind: LitFloat, Float: -float64(n.i)}), true
					}
					return scalarConst(&Literal{Kind: LitInt, Int: -n.i}), true
				}
				if op == "+" {
					return scalarConst(&Literal{Kind: LitFloat, Float: n.f}), true
				}
				return scalarConst(&Literal{Kind: LitFloat, Float: -n.f}), true
			}
		}
		if a.Kind == LitInt {
			if op == "+" {
				return scalarConst(a), true
			}
			if width == 32 && a.Int == -1<<31 || width != 32 && a.Int == math.MinInt64 {
				return scalarConst(&Literal{Kind: LitFloat, Float: -float64(a.Int)}), true
			}
			return scalarConst(&Literal{Kind: LitInt, Int: -a.Int}), true
		}
		if a.Kind == LitFloat {
			if op == "+" {
				return scalarConst(a), true
			}
			return scalarConst(&Literal{Kind: LitFloat, Float: -a.Float}), true
		}
	}
	return compileConst{}, false
}

func compileBinary(op string, a, b *Literal, width int, budget *compileBudget) (compileConst, bool) {
	if a == nil || b == nil || !budget.scan(a, b) {
		return compileConst{}, false
	}
	if op == "." {
		x, xFresh, ok := constStringFresh(a)
		if !ok {
			return compileConst{}, false
		}
		y, yFresh, ok := constStringFresh(b)
		if !ok {
			return compileConst{}, false
		}
		if !budget.spendBytes(len(x)) || !budget.spendBytes(len(y)) {
			return compileConst{}, false
		}
		return scalarConst(&Literal{Kind: LitString, Str: x + y, Uninterned: concatFresh(x == "", xFresh, y == "", yFresh)}), true
	}
	if op == "===" || op == "!==" || op == "==" || op == "!=" || op == "<>" || op == "<" || op == "<=" || op == ">" || op == ">=" || op == "<=>" {
		// PHP compares greater-than by reversing less-than, including NaN.
		if op == ">" {
			a, b, op = b, a, "<"
		} else if op == ">=" {
			a, b, op = b, a, "<="
		}
		cmp, ok := constCompare(a, b, op == "===" || op == "!==", width)
		if !ok {
			return compileConst{}, false
		}
		if op == "<=>" {
			return scalarConst(&Literal{Kind: LitInt, Int: int64(cmp)}), true
		}
		v := false
		switch op {
		case "===", "==":
			v = cmp == 0
		case "!==", "!=", "<>":
			v = cmp != 0
		case "<":
			v = cmp < 0
		case "<=":
			v = cmp <= 0
		case ">":
			v = cmp > 0
		case ">=":
			v = cmp >= 0
		}
		return scalarConst(&Literal{Kind: LitBool, Bool: v}), true
	}
	if (op == "&" || op == "|" || op == "^") && a.Kind == LitString && b.Kind == LitString {
		length := len(a.Str)
		if op != "|" && len(b.Str) < length {
			length = len(b.Str)
		}
		if op == "|" && len(b.Str) > length {
			length = len(b.Str)
		}
		if length < 0 || !budget.spendBytes(2*length) {
			budget.exhausted = true
			return compileConst{}, false
		}
		out := make([]byte, length)
		for i := range out {
			var x, y byte
			if i < len(a.Str) {
				x = a.Str[i]
			}
			if i < len(b.Str) {
				y = b.Str[i]
			}
			switch op {
			case "&":
				out[i] = x & y
			case "|":
				out[i] = x | y
			case "^":
				out[i] = x ^ y
			}
		}
		// The string forms of bitwise_or_function and its siblings intern
		// the result of two one-byte operands only.
		fresh := len(a.Str) != 1 || len(b.Str) != 1
		return scalarConst(&Literal{Kind: LitString, Str: string(out), Uninterned: fresh}), true
	}
	if op == "&" || op == "|" || op == "^" || op == "<<" || op == ">>" || op == "%" {
		x, ok := constInteger(a, width)
		if !ok {
			return compileConst{}, false
		}
		y, ok := constInteger(b, width)
		if !ok {
			return compileConst{}, false
		}
		switch op {
		case "&":
			return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x&y, width)}), true
		case "|":
			return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x|y, width)}), true
		case "^":
			return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x^y, width)}), true
		case "%":
			if y == 0 {
				return compileConst{}, false
			}
			if a.Kind == LitFloat && math.Trunc(a.Float) != a.Float || b.Kind == LitFloat && math.Trunc(b.Float) != b.Float {
				return compileConst{}, false
			}
			if width == 32 {
				x, y = int64(int32(x)), int64(int32(y))
			}
			return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x%y, width)}), true
		case "<<", ">>":
			if y < 0 {
				return compileConst{}, false
			}
			if width != 32 {
				width = 64
			}
			if y >= int64(width) {
				if op == ">>" && x < 0 {
					return scalarConst(&Literal{Kind: LitInt, Int: -1}), true
				}
				return scalarConst(&Literal{Kind: LitInt}), true
			}
			if op == ">>" {
				return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x>>uint(y), width)}), true
			}
			return scalarConst(&Literal{Kind: LitInt, Int: boundedInt(x<<uint(y), width)}), true
		}
	}
	if op != "+" && op != "-" && op != "*" && op != "/" && op != "**" {
		return compileConst{}, false
	}
	x, ok := constNumber(a, width)
	if !ok {
		return compileConst{}, false
	}
	y, ok := constNumber(b, width)
	if !ok {
		return compileConst{}, false
	}
	if op == "/" && y.f == 0 {
		return compileConst{}, false
	}
	if x.integer && y.integer {
		switch op {
		case "+", "-", "*":
			var z int64
			valid := true
			if op == "+" {
				z = x.i + y.i
				valid = (x.i^z)&(y.i^z) >= 0
			} else if op == "-" {
				z = x.i - y.i
				valid = (x.i^y.i)&(x.i^z) >= 0
			} else {
				z, valid = checkedIntMul(x.i, y.i, width)
			}
			if valid && inWidth(z, width) {
				return scalarConst(&Literal{Kind: LitInt, Int: z}), true
			}
			f := float64(x.i) + float64(y.i)
			if op == "-" {
				f = float64(x.i) - float64(y.i)
			} else if op == "*" {
				f = float64(x.i) * float64(y.i)
			}
			return scalarConst(&Literal{Kind: LitFloat, Float: f}), true
		case "/":
			if x.i == math.MinInt64 && y.i == -1 {
				return scalarConst(&Literal{Kind: LitFloat, Float: -float64(math.MinInt64)}), true
			}
			if x.i%y.i == 0 {
				z := x.i / y.i
				if inWidth(z, width) {
					return scalarConst(&Literal{Kind: LitInt, Int: z}), true
				}
			}
		}
	}
	var f float64
	switch op {
	case "+":
		f = x.f + y.f
	case "-":
		f = x.f - y.f
	case "*":
		f = x.f * y.f
	case "/":
		f = x.f / y.f
	case "**":
		if x.f == 0 && y.f < 0 {
			return compileConst{}, false
		}
		if x.integer && y.integer {
			i, result, isInt := phpnum.IntPow(x.i, y.i, width)
			if isInt {
				return scalarConst(&Literal{Kind: LitInt, Int: i}), true
			}
			f = result
		} else {
			f = phpnum.Pow(x.f, y.f, width)
		}
	}
	return scalarConst(&Literal{Kind: LitFloat, Float: f}), true
}

type constNum struct {
	i       int64
	f       float64
	integer bool
}

func constNumber(v *Literal, width int) (constNum, bool) {
	switch v.Kind {
	case LitInt:
		return constNum{i: boundedInt(v.Int, width), f: float64(boundedInt(v.Int, width)), integer: true}, true
	case LitFloat:
		return constNum{f: v.Float}, true
	case LitString:
		x := phpnum.Parse(v.Str, width, false)
		switch x.Kind {
		case phpnum.Integer:
			return constNum{i: x.Int, f: float64(x.Int), integer: true}, true
		case phpnum.Floating:
			return constNum{f: x.Float}, true
		}
		return constNum{}, false
	case LitBool:
		if v.Bool {
			return constNum{i: 1, f: 1, integer: true}, true
		}
		return constNum{i: 0, f: 0, integer: true}, true
	case LitNull:
		return constNum{i: 0, f: 0, integer: true}, true
	}
	return constNum{}, false
}

func constInteger(v *Literal, width int) (int64, bool) {
	n, ok := constNumber(v, width)
	if !ok {
		return 0, false
	}
	if n.integer {
		return n.i, true
	}
	if math.IsInf(n.f, 0) || n.f >= math.Exp2(float64(width-1)) || n.f < -math.Exp2(float64(width-1)) || math.Trunc(n.f) != n.f {
		return 0, false
	}
	return int64(n.f), true
}

func constBool(v *Literal) (bool, bool) {
	switch v.Kind {
	case LitNull:
		return false, true
	case LitBool:
		return v.Bool, true
	case LitInt:
		return v.Int != 0, true
	case LitFloat:
		return v.Float != 0, true
	case LitString:
		return v.Str != "" && v.Str != "0", true
	}
	return false, false
}

func constString(v *Literal) (string, bool) {
	switch v.Kind {
	case LitNull:
		return "", true
	case LitBool:
		if v.Bool {
			return "1", true
		}
		return "", true
	case LitInt:
		return strconv.FormatInt(v.Int, 10), true
	case LitFloat:
		return phpnum.FormatFloat(v.Float, 14), true
	case LitString:
		return v.Str, true
	}
	return "", false
}

func constCompare(a, b *Literal, strict bool, width int) (int, bool) {
	if strict {
		if a.Kind != b.Kind {
			return 1, true
		}
		switch a.Kind {
		case LitNull:
			return 0, true
		case LitBool:
			if a.Bool == b.Bool {
				return 0, true
			}
			if !a.Bool {
				return -1, true
			}
			return 1, true
		case LitInt:
			return cmpInt(a.Int, b.Int), true
		case LitFloat:
			if math.IsNaN(a.Float) || math.IsNaN(b.Float) {
				return 1, true
			}
			return cmpFloat(a.Float, b.Float), true
		case LitString:
			return strings.Compare(a.Str, b.Str), true
		}
	}
	if a.Kind == LitBool || b.Kind == LitBool {
		x, ok := constBool(a)
		if !ok {
			return 0, false
		}
		y, ok := constBool(b)
		if !ok {
			return 0, false
		}
		if x == y {
			return 0, true
		}
		if !x {
			return -1, true
		}
		return 1, true
	}
	if a.Kind == LitNull || b.Kind == LitNull {
		if a.Kind == LitString || b.Kind == LitString {
			x, y := "", ""
			if a.Kind == LitString {
				x = a.Str
			}
			if b.Kind == LitString {
				y = b.Str
			}
			return strings.Compare(x, y), true
		}
		x, _ := constBool(a)
		y, _ := constBool(b)
		if x == y {
			return 0, true
		}
		if !x {
			return -1, true
		}
		return 1, true
	}
	if a.Kind == LitFloat && math.IsNaN(a.Float) || b.Kind == LitFloat && math.IsNaN(b.Float) {
		return 1, true
	}
	if a.Kind == LitString && b.Kind == LitString {
		return phpnum.CompareStrings(a.Str, b.Str, width), true
	}
	// PHP 8 compares a nonnumeric string and a number as strings. Numeric
	// strings still compare numerically.
	if a.Kind == LitString && b.Kind != LitString && phpnum.Parse(a.Str, width, false).Kind == phpnum.NotNumeric {
		x, _ := constString(a)
		y, ok := constString(b)
		if !ok {
			return 0, false
		}
		return strings.Compare(x, y), true
	}
	if b.Kind == LitString && a.Kind != LitString && phpnum.Parse(b.Str, width, false).Kind == phpnum.NotNumeric {
		x, ok := constString(a)
		if !ok {
			return 0, false
		}
		y, _ := constString(b)
		return strings.Compare(x, y), true
	}
	x, ok := constNumber(a, width)
	if !ok {
		return 0, false
	}
	y, ok := constNumber(b, width)
	if !ok {
		return 0, false
	}
	if x.integer && y.integer {
		return cmpInt(x.i, y.i), true
	}
	if math.IsNaN(x.f) || math.IsNaN(y.f) {
		return 1, true
	}
	return cmpFloat(x.f, y.f), true
}

func cmpInt(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func scalarConst(v *Literal) compileConst { return compileConst{value: v} }
func boundedInt(v int64, width int) int64 {
	if width == 32 {
		return int64(int32(v))
	}
	return v
}
func inWidth(v int64, width int) bool { return width != 32 || v >= math.MinInt32 && v <= math.MaxInt32 }

func checkedIntMul(a, b int64, width int) (int64, bool) { return phpnum.MulExact(a, b, width) }

func compileUnaryValue(op string, a compileConst, width int, budget *compileBudget) (compileConst, bool) {
	if a.array {
		if op == "!" {
			v, ok := a.truth()
			return scalarConst(&Literal{Kind: LitBool, Bool: !v}), ok
		}
		return compileConst{}, false
	}
	return compileUnary(op, a.value, width, budget)
}
func compileBinaryValues(op string, a, b compileConst, width, depth int, budget *compileBudget) (compileConst, bool) {
	if op == "+" && a.array && b.array {
		return compileArrayUnion(a, b, budget)
	}
	if a.array || b.array {
		return compileArrayBinary(op, a, b, width, depth, budget)
	}
	v, ok := compileBinary(op, a.value, b.value, width, budget)
	if ok && op == "." {
		v.className = a.className && b.className
	}
	return v, ok
}

// compileString reports the conversion zend_compile_binary_op and
// concat_function perform on a constant concatenation operand.
func (l *lowerer) compileString(v compileConst) {
	if v.value != nil && v.value.Kind == LitFloat && math.IsNaN(v.value.Float) {
		l.diag(l.cur, "Warning", "unexpected NAN value was coerced to string")
	}
}

// PHP emits a compile warning only where compilation actually converts a
// constant to bool. Memoized value inspection alone must not emit it again.
func (l *lowerer) compileTruth(v compileConst) (bool, bool) {
	if v.value != nil && v.value.Kind == LitFloat && math.IsNaN(v.value.Float) {
		l.diag(l.cur, "Warning", "unexpected NAN value was coerced to bool")
	}
	return v.truth()
}
