package ssa

import (
	"context"
	"fmt"
	"math"

	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

type builder struct {
	ctx                 context.Context
	opts                Options
	p                   *Program
	used, constantBytes int
	active              map[*ir.Node]bool
	funcs               map[*ir.Func]bool
	classes             map[*ir.Class]bool
}

func build(ctx context.Context, u *ir.Unit, opts Options) (*Program, error) {
	if ctx == nil || u == nil || u.Main == nil {
		return nil, fmt.Errorf("ssa: context, unit and main are required")
	}
	if opts.MaxNodes <= 0 || uint64(opts.MaxNodes) > math.MaxUint32 || opts.MaxDepth <= 0 || opts.MaxDepth > 1024 || opts.MaxConstantBytes < 0 || opts.Passes & ^AllPasses != 0 {
		return nil, fmt.Errorf("ssa: invalid options")
	}
	if _, err := profile.Resolve(u.Profile.Release, u.Profile.IntWidth, u.Profile.ShortOpenTag); err != nil {
		return nil, err
	}
	b := builder{ctx: ctx, opts: opts, p: &Program{unit: u, profile: u.Profile, file: u.File}, active: map[*ir.Node]bool{}, funcs: map[*ir.Func]bool{}, classes: map[*ir.Class]bool{}}
	if err := b.function(u.Main, 0); err != nil {
		return nil, err
	}
	for _, f := range u.Functions {
		if err := b.charge(1, 0); err != nil {
			return nil, err
		}
		if err := b.function(f, 0); err != nil {
			return nil, err
		}
	}
	for _, c := range u.Classes {
		if err := b.charge(1, 0); err != nil {
			return nil, err
		}
		if err := b.class(c, 0); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.p, nil
}

func (b *builder) charge(n, depth int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if depth > b.opts.MaxDepth || n > b.opts.MaxNodes-b.used {
		return ErrLimit
	}
	b.used += n
	return nil
}

func (b *builder) function(f *ir.Func, depth int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if f == nil {
		return fmt.Errorf("ssa: nil function")
	}
	if b.funcs[f] {
		return nil
	}
	if err := b.charge(1, depth); err != nil {
		return err
	}
	b.funcs[f] = true
	index := len(b.p.functions)
	b.p.functions = append(b.p.functions, function{hir: f})
	if len(f.Body) > b.opts.MaxNodes-b.used || len(f.Params) > b.opts.MaxNodes-b.used-len(f.Body) {
		return ErrLimit
	}
	if err := b.charge(len(f.Body)+len(f.Params), depth); err != nil {
		return err
	}
	// No pointer into the growing functions/values slices survives recursion.
	for _, n := range f.Body {
		id, err := b.node(n, depth+1)
		if err != nil {
			return err
		}
		b.p.functions[index].roots = append(b.p.functions[index].roots, id)
	}
	for _, param := range f.Params {
		if param == nil {
			return fmt.Errorf("ssa: nil parameter")
		}
		id, err := b.node(param.Default, depth+1)
		if err != nil {
			return err
		}
		b.p.functions[index].defaults = append(b.p.functions[index].defaults, id)
	}
	return nil
}

func (b *builder) class(c *ir.Class, depth int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("ssa: nil class")
	}
	if b.classes[c] {
		return nil
	}
	if err := b.charge(1, depth); err != nil {
		return err
	}
	b.classes[c] = true
	for _, list := range [][]*ir.ClassConst{c.Consts, c.Cases} {
		if err := b.charge(len(list), depth); err != nil {
			return err
		}
		for _, k := range list {
			if k == nil {
				return fmt.Errorf("ssa: nil class constant")
			}
			if err := b.initializer(k.Value, depth+1); err != nil {
				return err
			}
		}
	}
	if err := b.charge(len(c.Props), depth); err != nil {
		return err
	}
	for _, p := range c.Props {
		if p == nil {
			return fmt.Errorf("ssa: nil property")
		}
		if err := b.initializer(p.Default, depth+1); err != nil {
			return err
		}
		if err := b.charge(len(p.Hooks), depth); err != nil {
			return err
		}
		for _, f := range p.Hooks {
			if err := b.function(f, depth+1); err != nil {
				return err
			}
		}
	}
	if err := b.charge(len(c.Methods), depth); err != nil {
		return err
	}
	for _, f := range c.Methods {
		if err := b.function(f, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) initializer(n *ir.Node, depth int) error {
	id, err := b.node(n, depth)
	if err == nil && id != None {
		b.p.initializers = append(b.p.initializers, id)
	}
	return err
}

func (b *builder) node(n *ir.Node, depth int) (ID, error) {
	if err := b.ctx.Err(); err != nil {
		return None, err
	}
	if n == nil {
		return None, nil
	}
	if err := b.charge(1, depth); err != nil {
		return None, err
	}
	if b.active[n] {
		return None, fmt.Errorf("ssa: cyclic HIR operands")
	}
	b.active[n] = true
	defer delete(b.active, n)
	if len(n.Kids) > b.opts.MaxNodes-b.used || len(n.Args) > b.opts.MaxNodes-b.used-len(n.Kids) {
		return None, ErrLimit
	}
	if err := b.charge(len(n.Kids)+len(n.Args), depth); err != nil {
		return None, err
	}
	var operands [2]ID
	large := len(n.Kids)+len(n.Args) > 2
	start := len(b.p.inputs)
	if large {
		b.p.inputs = append(b.p.inputs, make([]ID, len(n.Kids)+len(n.Args))...)
		operands[0] = ID(start)
	}
	declaredChildren := ir.Effect(0)
	effects := ir.Effect(0)
	sources, supplied := b.opts.Sources[n]
	for i, child := range n.Kids {
		id, err := b.node(child, depth+1)
		if err != nil {
			return None, err
		}
		if large {
			b.p.inputs[start+i] = id
		} else {
			operands[i] = id
		}
		if child != nil {
			declaredChildren |= child.Effects
		}
		if v, ok := b.p.Value(id); ok {
			effects |= v.Effects.PHP
			sources |= v.Sources
		}
	}
	for i, arg := range n.Args {
		if arg == nil {
			return None, fmt.Errorf("ssa: nil argument")
		}
		id, err := b.node(arg.Value, depth+1)
		if err != nil {
			return None, err
		}
		if large {
			b.p.inputs[start+len(n.Kids)+i] = id
		} else {
			operands[len(n.Kids)+i] = id
		}
		if arg.Value != nil {
			declaredChildren |= arg.Value.Effects
		}
		if v, ok := b.p.Value(id); ok {
			effects |= v.Effects.PHP
			sources |= v.Sources
		}
	}
	if n.Func != nil {
		if n.Op == ir.OpClosure {
			if err := b.charge(len(n.Func.Captures), depth); err != nil {
				return None, err
			}
			for _, c := range n.Func.Captures {
				if c == nil {
					return None, fmt.Errorf("ssa: nil capture")
				}
			}
		}
		if err := b.function(n.Func, depth+1); err != nil {
			return None, err
		}
	}
	if n.Class != nil {
		if err := b.class(n.Class, depth+1); err != nil {
			return None, err
		}
	}
	// Recompute children rather than trusting possibly stale parent summaries.
	// Preserve any additional annotations, including future safety dimensions.
	own := ir.OperationEffects(n)
	if !supplied && (sourceIntroducer(n) || own == ir.AllEffects) {
		sources |= UnknownSource
	}
	extra := n.Effects & ^(own | declaredChildren)
	effects |= own | extra
	if effects&ir.MayCallUser != 0 {
		effects |= ir.AllEffects
	}
	v := Value{HIR: n, Effects: Effects{PHP: effects, Ordered: true}, Sources: sources}
	d := definition{value: v, operands: operands}
	b.analyze(&d, extra)
	b.p.values = append(b.p.values, d)
	return ID(len(b.p.values)), nil
}

func sourceIntroducer(n *ir.Node) bool {
	switch n.Op {
	case ir.OpVar, ir.OpVarVar, ir.OpSuperGlobal, ir.OpThis, ir.OpDim, ir.OpProp, ir.OpNullsafe, ir.OpStaticProp, ir.OpClassConst, ir.OpConst,
		ir.OpCall, ir.OpCallValue, ir.OpMethodCall, ir.OpNullsafeMC, ir.OpStaticCall, ir.OpNew, ir.OpClone, ir.OpClosure, ir.OpArrowFn, ir.OpInclude, ir.OpEval, ir.OpShell, ir.OpYield, ir.OpYieldFrom:
		return true
	}
	return false
}
