package ir

import (
	"fmt"
	"strconv"
	"strings"
)

// Dump renders a unit as deterministic text for tests and debugging.
func Dump(u *Unit) string {
	var b strings.Builder
	d := dumper{b: &b}
	fmt.Fprintf(&b, "unit %s strict=%t\n", u.Profile.Key(), u.Strict)
	for _, f := range u.Functions {
		d.fn(f, 0)
	}
	for _, c := range u.Classes {
		d.class(c, 0)
	}
	d.fn(u.Main, 0)
	for _, diag := range u.Diagnostics {
		fmt.Fprintf(&b, "%s: %s on line %d\n", diag.Level, diag.Message, diag.Line)
	}
	return b.String()
}

type dumper struct{ b *strings.Builder }

func (d dumper) line(depth int, format string, args ...any) {
	d.b.WriteString(strings.Repeat("  ", depth))
	fmt.Fprintf(d.b, format, args...)
	d.b.WriteByte('\n')
}

func (d dumper) fn(f *Func, depth int) {
	var flags []string
	if f.ByRef {
		flags = append(flags, "byref")
	}
	if f.Static {
		flags = append(flags, "static")
	}
	if f.Generator {
		flags = append(flags, "generator")
	}
	if f.Hoisted {
		flags = append(flags, "hoisted")
	}
	if f.DynamicScope {
		flags = append(flags, "dynscope")
	}
	if f.UsesArgs {
		flags = append(flags, "args")
	}
	ret := ""
	if f.Return != nil {
		ret = ": " + f.Return.String
	}
	d.line(depth, "func %s(%s)%s [%s] L%d", f.Name, d.params(f.Params), ret, strings.Join(flags, ","), f.Line)
	for _, c := range f.Captures {
		mode := "value"
		if c.ByRef {
			mode = "ref"
		} else if c.Implicit {
			mode = "implicit"
		}
		d.line(depth+1, "capture $%s %s", c.Name, mode)
	}
	for _, v := range f.Vars {
		d.line(depth+1, "var $%s %s", v.Name, varFlags(v.Flags))
	}
	for _, n := range f.Body {
		d.node(n, depth+1)
	}
}

func (d dumper) params(ps []*Param) string {
	var parts []string
	for _, p := range ps {
		s := ""
		if p.Type != nil {
			s = p.Type.String + " "
		}
		if p.ByRef {
			s += "&"
		}
		if p.Variadic {
			s += "..."
		}
		s += "$" + p.Name
		if p.Default != nil {
			s += " = " + d.inline(p.Default)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func (d dumper) class(c *Class, depth int) {
	d.line(depth, "class %s kind=%d L%d", c.Name, c.Kind, c.Line)
	for _, m := range c.Methods {
		d.fn(m, depth+1)
	}
	for _, p := range c.Props {
		for _, h := range p.Hooks {
			d.fn(h, depth+1)
		}
	}
}

var varFlagNames = []string{"param", "global", "static", "captured", "captured_ref", "ref", "may_ref", "written", "read"}

func varFlags(f VarFlag) string {
	var parts []string
	for i, n := range varFlagNames {
		if f&(1<<i) != 0 {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, ",")
}

func (d dumper) inline(n *Node) string {
	if n == nil {
		return "_"
	}
	if n.Op == OpLiteral {
		return literalText(n.Value)
	}
	return string(n.Op)
}

func literalText(v *Literal) string {
	switch v.Kind {
	case LitNull:
		return "null"
	case LitBool:
		return strconv.FormatBool(v.Bool)
	case LitInt:
		return strconv.FormatInt(v.Int, 10)
	case LitFloat:
		return strconv.FormatFloat(v.Float, 'g', 17, 64)
	}
	return strconv.Quote(v.Str)
}

func (d dumper) node(n *Node, depth int) {
	if n == nil {
		d.line(depth, "_")
		return
	}
	s := string(n.Op)
	if n.Text != "" {
		s += " " + strconv.Quote(n.Text)
	}
	if n.Value != nil {
		s += " " + literalText(n.Value)
	}
	if n.Name != nil {
		s += " name=" + n.Name.Resolved
		if n.Name.Fallback != "" {
			s += "|" + n.Name.Fallback
		}
	}
	if n.Var != nil {
		s += " $" + n.Var.Name
	}
	s += " {" + n.Effects.String() + "}"
	d.line(depth, "%s L%d", s, n.Line)
	if n.Func != nil && (n.Op == OpClosure || n.Op == OpArrowFn || n.Op == OpFuncDecl) {
		d.fn(n.Func, depth+1)
	}
	if n.Class != nil {
		d.class(n.Class, depth+1)
	}
	for _, a := range n.Args {
		prefix := "arg"
		if a.Name != "" {
			prefix += " " + a.Name + ":"
		}
		if a.Spread {
			prefix += " ..."
		}
		if a.MayRef {
			prefix += " mayref"
		}
		d.line(depth+1, "%s", prefix)
		d.node(a.Value, depth+2)
	}
	for _, k := range n.Kids {
		d.node(k, depth+1)
	}
}
