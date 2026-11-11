package ssa

import (
	"math"

	"github.com/HazelnutParadise/phego/internal/ir"
)

func literalType(k ir.LiteralKind) Type {
	switch k {
	case ir.LitNull:
		return Null
	case ir.LitBool:
		return Bool
	case ir.LitInt:
		return Int
	case ir.LitFloat:
		return Float
	case ir.LitString:
		return String
	}
	return Unknown
}

func (b *builder) operand(d *definition, i int) Value {
	if i >= len(d.value.HIR.Kids) {
		return Value{}
	}
	v, _ := b.p.Value(d.edge(b.p, i))
	return v
}

func (b *builder) analyze(d *definition, extra ir.Effect) {
	n, v := d.value.HIR, &d.value
	kids, args := len(n.Kids), len(n.Args)
	a, c := b.operand(d, 0), b.operand(d, 1)
	switch n.Op {
	case ir.OpLiteral:
		// An int the profile cannot represent is neither typed nor folded.
		if n.Value != nil && literalType(n.Value.Kind) != Unknown && (n.Value.Kind != ir.LitInt || b.inRange(n.Value.Int)) {
			v.Type = literalType(n.Value.Kind)
			v.Constant, v.IsConstant = *n.Value, n.Flags == 0 && kids == 0 && args == 0 && v.Effects.PHP == 0
		}
	case ir.OpIsset, ir.OpEmpty, ir.OpLogical, ir.OpInstanceof:
		v.Type = Bool
	case ir.OpUnary:
		if n.Text == "!" {
			v.Type = Bool
		}
	case ir.OpCast:
		switch n.Text {
		case "int":
			v.Type = Int
		case "bool":
			v.Type = Bool
		case "string":
			v.Type = String
		case "float":
			v.Type = Float
		}
	case ir.OpBinary:
		switch n.Text {
		case "===", "!==", "==", "!=", "<>", "<", "<=", ">", ">=":
			v.Type = Bool
		case "<=>":
			v.Type = Int
		case ".":
			v.Type = String
		case "&", "|", "^":
			if a.Type == Int && c.Type == Int {
				v.Type = Int
			}
		}
	}
	if n.Flags == 0 && args == 0 && n.Func == nil && n.Class == nil {
		if b.opts.Passes&SpecializeTypes != 0 {
			switch {
			case n.Op == ir.OpUnary && n.Text == "!" && kids == 1 && a.Type == Bool:
				v.Specialization = BoolNot
			case n.Op == ir.OpBinary && kids == 2 && a.Type == Int && c.Type == Int && (n.Text == "&" || n.Text == "|" || n.Text == "^"):
				v.Specialization = IntBitwise
			}
		}
		if b.opts.Passes&FoldConstants != 0 && extra == 0 {
			if result, ok := b.fold(d, a, c); ok {
				v.Constant, v.IsConstant, v.Type = result, true, literalType(result.Kind)
				v.Effects.PHP = 0 // the whitelist proves no diagnostics/callbacks
			}
		}
	}
	// Only literal values or proven pure computations can lose the barrier.
	// All locations/control/allocations/checks remain explicit ordered operations.
	if v.IsConstant && v.Effects.PHP == 0 {
		v.Effects.Ordered = false
	}
}

func (b *builder) inRange(n int64) bool {
	return b.p.profile.IntWidth == 64 || n >= math.MinInt32 && n <= math.MaxInt32
}

func (b *builder) fold(d *definition, a, c Value) (ir.Literal, bool) {
	n := d.value.HIR
	if n.Op == ir.OpLiteral {
		return d.value.Constant, d.value.IsConstant
	}
	if !a.IsConstant || a.Effects.Ordered {
		return ir.Literal{}, false
	}
	if n.Op == ir.OpUnary && len(n.Kids) == 1 {
		if n.Text == "!" && a.Type == Bool {
			return ir.Literal{Kind: ir.LitBool, Bool: !a.Constant.Bool}, true
		}
		if a.Type != Int {
			return ir.Literal{}, false
		}
		i := a.Constant.Int
		switch n.Text {
		case "+":
			return a.Constant, true
		case "-":
			if i != math.MinInt64 && b.inRange(-i) {
				return ir.Literal{Kind: ir.LitInt, Int: -i}, true
			}
		case "~":
			return ir.Literal{Kind: ir.LitInt, Int: ^i}, true
		}
	}
	if n.Op != ir.OpBinary || len(n.Kids) != 2 || !c.IsConstant || c.Effects.Ordered {
		return ir.Literal{}, false
	}
	if n.Text == "." && a.Type == String && c.Type == String {
		x, y := a.Constant.Str, c.Constant.Str
		remaining := b.opts.MaxConstantBytes - b.constantBytes
		if len(x) <= remaining && len(y) <= remaining-len(x) {
			b.constantBytes += len(x) + len(y)
			return ir.Literal{Kind: ir.LitString, Str: x + y}, true
		}
	}
	if n.Text == "===" || n.Text == "!==" {
		// Float NaN and signed-zero equality use PHP's floating rules, not Go
		// struct equality. Leave all floats to the shared runtime in this pass.
		if a.Type == Float || c.Type == Float {
			return ir.Literal{}, false
		}
		equal := false
		if a.Type == c.Type {
			switch a.Type {
			case Null:
				equal = true
			case Bool:
				equal = a.Constant.Bool == c.Constant.Bool
			case Int:
				equal = a.Constant.Int == c.Constant.Int
			case String:
				equal = a.Constant.Str == c.Constant.Str
			}
		}
		if n.Text == "!==" {
			equal = !equal
		}
		return ir.Literal{Kind: ir.LitBool, Bool: equal}, true
	}
	if a.Type != Int || c.Type != Int {
		return ir.Literal{}, false
	}
	i, j := a.Constant.Int, c.Constant.Int
	var result int64
	switch n.Text {
	case "+":
		result = i + j
		if (j > 0 && result < i) || (j < 0 && result > i) {
			return ir.Literal{}, false
		}
	case "-":
		result = i - j
		if (j < 0 && result < i) || (j > 0 && result > i) {
			return ir.Literal{}, false
		}
	case "*":
		result = i * j
		if (i == -1 && j == math.MinInt64) || (j == -1 && i == math.MinInt64) || (i != 0 && result/i != j) {
			return ir.Literal{}, false
		}
	case "&":
		result = i & j
	case "|":
		result = i | j
	case "^":
		result = i ^ j
	default:
		return ir.Literal{}, false
	}
	if !b.inRange(result) {
		return ir.Literal{}, false
	}
	return ir.Literal{Kind: ir.LitInt, Int: result}, true
}
