package ir

import (
	"fmt"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// builtinOrder is the order zend_type_to_string prints built-in types in,
// after class names.
var builtinOrder = []string{"static", "callable", "object", "array", "string", "int", "float", "bool", "false", "true", "void", "never"}

// typeEntry is a class name or an intersection of class names in a type.
type typeEntry struct {
	name     string
	inter    []string
	iterable bool // the Traversable of iterable
}

// typeSet is a type being compiled: built-in members and class entries in
// declaration order.
type typeSet struct {
	mask    map[string]bool
	entries []typeEntry
	// unionNull is set when null is a union member, as in iterable|null,
	// which Reflection prints expanded rather than as ?iterable.
	unionNull bool
}

func (t *typeSet) has(b string) bool { return t.mask[b] }

// String prints the type as zend_type_to_string.
func (t *typeSet) String() string {
	var parts []string
	for _, e := range t.entries {
		if e.inter != nil {
			s := strings.Join(e.inter, "&")
			if len(t.entries) > 1 || len(t.mask) > 0 {
				s = "(" + s + ")"
			}
			parts = append(parts, s)
			continue
		}
		parts = append(parts, e.name)
	}
	if t.mask["mixed"] {
		return strings.Join(append(parts, "mixed"), "|")
	}
	for _, b := range builtinOrder {
		if !t.mask[b] {
			continue
		}
		if b == "bool" || (b == "false" || b == "true") && !t.mask["bool"] {
			parts = append(parts, b)
		}
		if b != "bool" && b != "false" && b != "true" {
			parts = append(parts, b)
		}
	}
	s := strings.Join(parts, "|")
	if t.mask["null"] {
		if s != "" && !strings.ContainsAny(s, "|&") {
			return "?" + s
		}
		if s == "" {
			return "null"
		}
		s += "|null"
	}
	return s
}

// typeOf compiles a declared type with the checks of
// zend_compile_typename_ex and returns it in ReflectionType's spelling.
func (l *lowerer) typeOf(n *frontend.Node, _ bool) *Type {
	marked := n.Kind == "nullable_type"
	if marked {
		n = n.Children[0]
	}
	t := &typeSet{mask: map[string]bool{}}
	switch n.Kind {
	case "union_type":
		l.cur = n.Line
		hasOnlyIterable := true
		for _, c := range n.Children {
			if c.Kind == "intersection_type" {
				hasOnlyIterable = false
				inter := l.intersection(c)
				entry := typeEntry{inter: inter}
				for _, e := range t.entries {
					if e.inter != nil {
						l.intersectionsRedundant(entry, e)
					} else {
						l.intersectionRedundantBy(entry, e)
					}
				}
				t.entries = append(t.entries, entry)
				continue
			}
			single := l.singleType(c)
			if single.mask["null"] {
				t.unionNull = true
			}
			if single.mask["mixed"] {
				l.fail(c.Line, "Type mixed can only be used as a standalone type")
			}
			if len(single.entries) > 0 && !single.entries[0].iterable {
				hasOnlyIterable = false
			}
			for b := range single.mask {
				if t.mask[b] {
					l.fail(c.Line, "Duplicate type %s is redundant", b)
				}
			}
			if t.mask["true"] && single.mask["false"] && len(single.mask) == 1 || t.mask["false"] && single.mask["true"] && len(single.mask) == 1 {
				l.fail(c.Line, "Type contains both true and false, bool must be used instead")
			}
			if t.mask["bool"] && (single.mask["false"] || single.mask["true"]) {
				l.fail(c.Line, "Duplicate type %s is redundant", (&typeSet{mask: single.mask}).String())
			}
			if (t.mask["false"] || t.mask["true"]) && single.mask["bool"] {
				l.fail(c.Line, "Duplicate type %s is redundant", map[bool]string{true: "false", false: "true"}[t.mask["false"]])
			}
			for b := range single.mask {
				t.mask[b] = true
			}
			for _, se := range single.entries {
				for _, e := range t.entries {
					if e.inter != nil {
						l.intersectionRedundantBy(e, se)
					} else if phpname.EqualFold(e.name, se.name) {
						l.fail(c.Line, "Duplicate type %s is redundant", se.name)
					}
				}
				t.entries = append(t.entries, se)
			}
		}
		if t.mask["object"] && (!hasOnlyIterable && len(t.entries) > 0 || t.mask["static"]) {
			l.fail(n.Line, "Type %s contains both object and a class type, which is redundant", t.String())
		}
	case "intersection_type":
		t.entries = []typeEntry{{inter: l.intersection(n)}}
	default:
		t = l.singleType(n)
	}
	if t.mask["mixed"] && marked {
		l.fail(n.Line, "Type mixed cannot be marked as nullable since mixed already includes null")
	}
	if t.mask["null"] && marked {
		l.fail(n.Line, "null cannot be marked as nullable")
	}
	if marked {
		t.mask["null"] = true
	}
	if t.mask["void"] && (len(t.mask) > 1 || len(t.entries) > 0) {
		l.fail(n.Line, "Void can only be used as a standalone type")
	}
	if t.mask["never"] && (len(t.mask) > 1 || len(t.entries) > 0) {
		l.fail(n.Line, "never can only be used as a standalone type")
	}
	return &Type{String: t.reflection(), zend: t.String()}
}

// reflection is the spelling ReflectionType::__toString uses, which keeps
// "iterable" for a type that is only iterable, as PHP does for
// compatibility.
func (t *typeSet) reflection() string {
	if len(t.entries) == 1 && t.entries[0].iterable && len(t.mask) == 1 ||
		len(t.entries) == 1 && t.entries[0].iterable && len(t.mask) == 2 && t.mask["null"] && !t.unionNull {
		if t.mask["null"] {
			return "?iterable"
		}
		return "iterable"
	}
	return t.String()
}

// intersection compiles an intersection type's class names.
func (l *lowerer) intersection(n *frontend.Node) []string {
	var names []string
	for _, c := range n.Children {
		single := l.singleType(c)
		if len(single.entries) == 1 && single.entries[0].iterable {
			l.fail(c.Line, "Type %s cannot be part of an intersection type", single.String())
		}
		if len(single.entries) == 0 {
			l.fail(c.Line, "Type %s cannot be part of an intersection type", single.String())
		}
		name := single.entries[0].name
		if phpname.EqualFold(name, "self") || phpname.EqualFold(name, "parent") {
			l.fail(c.Line, "Type %s cannot be part of an intersection type", c.Text)
		}
		for _, prev := range names {
			if phpname.EqualFold(prev, name) {
				l.fail(c.Line, "Duplicate type %s is redundant", name)
			}
		}
		names = append(names, name)
	}
	return names
}

func (l *lowerer) intersectionsRedundant(a, b typeEntry) {
	// a is the new intersection, b an earlier one, as
	// zend_are_intersection_types_redundant(single_type, earlier).
	small, large, flipped := a, b, false
	if len(b.inter) < len(a.inter) {
		small, large, flipped = b, a, true
	}
	sum := 0
	for _, o := range small.inter {
		for _, i := range large.inter {
			if phpname.EqualFold(o, i) {
				sum++
				break
			}
		}
	}
	if sum != len(small.inter) {
		return
	}
	smallStr, largeStr := strings.Join(small.inter, "&"), strings.Join(large.inter, "&")
	if flipped {
		smallStr, largeStr = strings.Join(b.inter, "&"), strings.Join(a.inter, "&")
	}
	if len(small.inter) == len(large.inter) {
		l.fail(l.cur, "Type %s is redundant with type %s", smallStr, largeStr)
	}
	l.fail(l.cur, "Type %s is redundant as it is more restrictive than type %s", largeStr, smallStr)
}

func (l *lowerer) intersectionRedundantBy(inter, single typeEntry) {
	for _, name := range inter.inter {
		if phpname.EqualFold(name, single.name) {
			l.fail(l.cur, "Type %s is redundant as it is more restrictive than type %s", strings.Join(inter.inter, "&"), single.name)
		}
	}
}

// singleType compiles one named type as zend_compile_single_typename.
func (l *lowerer) singleType(t *frontend.Node) *typeSet {
	set := &typeSet{mask: map[string]bool{}}
	name := t.Text
	lower := phpname.Lower(name)
	if t.Token == frontend.TStatic && l.class == nil && l.scopeKnown() {
		l.fail(t.Line, "Cannot use \"static\" when no class scope is active")
	}
	switch t.Token {
	case frontend.TArray, frontend.TCallable, frontend.TStatic:
		set.mask[lower] = true
		return set
	}
	form, rest := formOf(name)
	if (form == fullyQualified || form == relative) && builtinTypes[phpname.Lower(rest)] {
		l.fail(t.Line, "Type declaration '%s' must be unqualified", phpname.Lower(rest))
	}
	if form == unqualified {
		switch lower {
		case "iterable":
			set.entries = []typeEntry{{name: "Traversable", iterable: true}}
			set.mask["array"] = true
			return set
		case "null", "mixed", "bool", "false", "true", "float", "int", "string", "void", "never", "object":
			set.mask[lower] = true
			return set
		}
	}
	l.checkClassName(t)
	cls := l.typeClass(t)
	if cls.Special == "" && isReservedClassName(cls.Resolved) {
		l.fail(t.Line, "Cannot use \"%s\" as a type name as it is reserved", cls.Resolved)
	}
	if correct, confusable := confusableTypes[lower]; confusable && form == unqualified {
		if _, imported := l.ns.classes[lower]; !imported {
			extra := ""
			if l.ns.name != "" {
				extra = " or import the class with \"use\""
			}
			if correct != "" {
				l.diag(t.Line, "Warning", fmt.Sprintf("\"%s\" will be interpreted as a class name. Did you mean \"%s\"? Write \"\\%s\"%s to suppress this warning", name, correct, cls.Resolved, extra))
			} else {
				l.diag(t.Line, "Warning", fmt.Sprintf("\"%s\" is not a supported builtin type and will be interpreted as a class name. Write \"\\%s\"%s to suppress this warning", name, cls.Resolved, extra))
			}
		}
	}
	set.entries = []typeEntry{{name: cls.Resolved}}
	return set
}

// typeClass resolves a class name used in a type. self and parent keep
// their names; static is a built-in type handled by the caller.
func (l *lowerer) typeClass(t *frontend.Node) *Name {
	if s := specialClass(t.Text); s == "self" || s == "parent" {
		l.classRef(t)
		// With a known, named scope PHP substitutes the class itself.
		if l.scopeKnown() && !l.class.Anonymous {
			if s == "self" {
				return &Name{Kind: ClassName, Resolved: l.class.Name}
			}
			return &Name{Kind: ClassName, Resolved: l.class.Parent.Resolved}
		}
		return &Name{Kind: ClassName, Resolved: s, Special: s}
	}
	return l.ns.resolveClass(t.Text)
}

var classKinds = map[string]ClassKind{"class": KindClass, "interface": KindInterface, "trait": KindTrait, "enum": KindEnum}

// classDecl lowers a class-like declaration and checks its own members.
// bindClass checks inheritance when the dependencies are available.
func (l *lowerer) classDecl(s *frontend.Node, anonymous bool) *Class {
	c := s.Children
	if l.class != nil && !anonymous {
		l.fail(s.Line, "Class declarations may not be nested")
	}
	cl := &Class{Line: s.Line, Anonymous: anonymous}
	var extends, implements, members, mods *frontend.Node
	if anonymous {
		cl.Kind = KindClass
		cl.Name = "class@anonymous"
		mods, extends, implements, members = c[1], c[3], c[4], c[5]
	} else {
		cl.Kind = classKinds[s.Kind]
		short := c[2].Text
		kind := map[string]string{"class": "a class name", "interface": "an interface name", "trait": "a trait name", "enum": "an enum name"}[s.Kind]
		if isReservedClassName(short) {
			l.fail(s.Line, "Cannot use \"%s\" as %s as it is reserved", short, kind)
		}
		if short == "_" {
			l.diag(s.Line, "Deprecated", fmt.Sprintf("Using \"_\" as %s is deprecated since 8.4", kind))
		}
		cl.Name = l.ns.prefix(short)
		l.see("class", cl.Name)
		if imp, ok := l.ns.classes[phpname.Lower(short)]; ok && !phpname.EqualFold(imp, cl.Name) {
			l.fail(s.Line, "Cannot redeclare class %s (previously declared as local import)", cl.Name)
		}
		mods, extends, implements, members = c[1], c[4], c[5], c[6]
		if c[3] != nil {
			cl.Backing = l.typeOf(c[3], false)
		}
	}
	if mods != nil {
		for _, m := range mods.Children {
			cl.Modifiers = append(cl.Modifiers, phpname.Lower(m.Text))
		}
	}
	savedClass := l.class
	defer func() { l.class = savedClass }()
	if extends != nil {
		if extends.Kind == "names" {
			for _, n := range extends.Children {
				cl.Interfaces = append(cl.Interfaces, l.declName(n, "interface"))
			}
		} else {
			cl.Parent = l.declName(extends, "class")
		}
	}
	if implements != nil {
		for _, n := range implements.Children {
			cl.Interfaces = append(cl.Interfaces, l.declName(n, "interface"))
		}
	}
	l.class = cl
	cl.Attributes = l.checkAttributes(c[0], "class", s.Line)
	if !anonymous {
		l.unit.Classes = append(l.unit.Classes, cl)
	}
	for _, m := range members.Children {
		l.member(cl, m)
	}
	l.checkClass(cl, s)
	if hasClassMarker(cl.Attributes, "Attribute") {
		cl.IsAttribute = true
		cl.AttributeTargets = attributeClassTargets(c[0])
	}
	return cl
}

func hasClassMarker(attrs []Attribute, want string) bool {
	for _, a := range attrs {
		if a.Name != nil && globalAttributeName(a.Name.Resolved) == phpname.Lower(want) {
			return true
		}
	}
	return false
}

func (l *lowerer) declName(n *frontend.Node, kind string) *Name {
	name := l.ns.resolveClass(n.Text)
	if name.Special != "" {
		l.fail(n.Line, "Cannot use \"%s\" as %s name, as it is reserved", n.Text, kind)
	}
	return name
}

func modifierList(n *frontend.Node) []string {
	var out []string
	if n == nil {
		return nil
	}
	for _, m := range n.Children {
		out = append(out, phpname.Lower(m.Text))
	}
	return out
}

func hasModifier(mods []string, want string) bool {
	for _, m := range mods {
		if m == want {
			return true
		}
	}
	return false
}

func (l *lowerer) member(cl *Class, m *frontend.Node) {
	l.cur = m.Line
	c := m.Children
	switch m.Kind {
	case "trait_use":
		if cl.Kind == KindInterface {
			trait := c[0].Children[0].Text
			l.fail(m.Line, "Cannot use traits inside of interfaces. %s is used in %s", trait, cl.Name)
		}
		for _, n := range c[0].Children {
			cl.Traits = append(cl.Traits, l.declName(n, "trait"))
		}
		if len(c) > 1 && c[1] != nil {
			for _, a := range c[1].Children {
				ref := a.Children[0]
				rule := &TraitRule{Line: m.Line, Method: ref.Children[1].Text}
				if ref.Children[0] != nil {
					name := *ref.Children[0]
					name.Line = m.Line
					rule.Trait = l.declName(&name, "trait")
				}
				if a.Kind == "insteadof" {
					for _, n := range a.Children[1].Children {
						name := *n
						name.Line = m.Line
						rule.Insteadof = append(rule.Insteadof, l.declName(&name, "trait"))
					}
				} else {
					if a.Children[1] != nil {
						rule.Modifier = phpname.Lower(a.Children[1].Text)
					}
					if a.Children[2] != nil {
						rule.Alias = a.Children[2].Text
					}
				}
				cl.TraitRules = append(cl.TraitRules, rule)
			}
		}
	case "class_const":
		l.cur = c[3].Children[0].Line
		mods := modifierList(c[1])
		attrs := l.checkAttributes(c[0], "class constant", m.Line)
		var typ *Type
		if c[2] != nil {
			typ = l.typeOf(c[2], false)
		}
		for _, item := range c[3:] {
			cl.Consts = append(cl.Consts, &ClassConst{Name: item.Children[0].Text, Line: item.Line, Modifiers: mods, Type: typ, Value: l.constExpr(item.Children[1], false), Attributes: attrs, Class: cl})
		}
	case "enum_case":
		if cl.Kind != KindEnum {
			l.fail(m.Line, "Case can only be used in enums")
		}
		cc := &ClassConst{Name: c[1].Text, Line: m.Line, Attributes: l.checkAttributes(c[0], "class constant", m.Line), Class: cl}
		if c[2] != nil {
			cc.Value = l.constExpr(c[2], false)
		}
		cl.Cases = append(cl.Cases, cc)
	case "method":
		line := m.FunctionLine
		if line == 0 {
			line = m.Line
		}
		l.cur = line
		f := l.newFunc(FuncMethod, c[2].Text, line)
		f.Attributes = l.checkAttributes(c[0], "method", line)
		f.Modifiers = modifierList(c[1])
		f.Static = hasModifier(f.Modifiers, "static")
		f.ByRef = m.Text == "&"
		f.Class = cl
		cl.Methods = append(cl.Methods, f)
		l.functionBody(f, c[3], c[4], c[5], nil)
		for _, p := range f.Params {
			if len(p.Promoted) > 0 && c[5] == nil {
				l.fail(p.Line, "Cannot declare promoted property in an abstract constructor")
			}
		}
		l.promotedHooks(cl, c[3])
	case "property":
		l.cur = c[3].Children[0].Line
		if c[2] != nil {
			l.cur = l.zline(c[2])
		}
		mods := modifierList(c[1])
		attrs := l.checkAttributes(c[0], "property", c[3].Children[0].Line)
		var typ *Type
		if c[2] != nil {
			typ = l.typeOf(c[2], false)
		}
		var hooks *frontend.Node
		items := c[3:]
		if last := items[len(items)-1]; last.Kind == "hooks" {
			hooks, items = last, items[:len(items)-1]
		}
		for _, item := range items {
			p := &Prop{Name: item.Children[0].Text[1:], Line: m.Line, Modifiers: mods, Type: typ, HasHooks: hooks != nil, Backed: hooksBackProperty(hooks, item.Children[0].Text[1:]), Attributes: attrs, Class: cl}
			if d := item.Children[1]; d != nil {
				p.Default = l.constExpr(d, false)
			}
			if hooks != nil {
				p.Hooks = l.hooks(p.Name, hooks)
			}
			cl.Props = append(cl.Props, p)
		}
	}
}

func hooksBackProperty(hooks *frontend.Node, name string) bool {
	if hooks == nil {
		return false
	}
	var visit func(*frontend.Node) bool
	visit = func(n *frontend.Node) bool {
		if n == nil {
			return false
		}
		// A method call stores its member as a property node; calling
		// $this->x() does not fetch the property. Its object, name
		// expression and arguments are still searched.
		if n.Kind == "method_call" && len(n.Children) > 0 && n.Children[0] != nil &&
			(n.Children[0].Kind == "prop" || n.Children[0].Kind == "nullsafe_prop") {
			for _, child := range append(append([]*frontend.Node(nil), n.Children[0].Children...), n.Children[1:]...) {
				if child != nil && child.Kind != "identifier" && visit(child) {
					return true
				}
			}
			return false
		}
		if n.Kind == "prop" || n.Kind == "nullsafe_prop" {
			if len(n.Children) > 1 && hookUsesProperty(n.Children[0], n.Children[1], name) {
				return true
			}
		}
		switch n.Kind {
		case "closure", "arrow_function", "function", "class", "interface", "trait", "enum":
			return false
		case "new_class":
			if len(n.Children) > 2 {
				return visit(n.Children[2])
			}
			return false
		}
		for _, child := range n.Children {
			if visit(child) {
				return true
			}
		}
		return false
	}
	for _, h := range hooks.Children {
		// A short setter writes its expression into the property's storage.
		if phpname.EqualFold(h.Children[2].Text, "set") && h.Children[4] != nil && h.Children[4].Kind != "block" {
			return true
		}
		if len(h.Children) > 4 && visit(h.Children[4]) {
			return true
		}
	}
	return false
}

// promotedHooks compiles the hooks of promoted constructor properties.
func (l *lowerer) promotedHooks(cl *Class, params *frontend.Node) {
	method := cl.Methods[len(cl.Methods)-1]
	for i, p := range params.Children {
		mods, hooks := p.Children[1], p.Children[5]
		if (mods == nil || len(mods.Children) == 0) && hooks == nil {
			continue
		}
		name := p.Children[3].Text[1:]
		param := method.Params[i]
		prop := &Prop{Name: name, Line: p.Line, Modifiers: param.Promoted, Type: param.Type, Default: param.Default, HasHooks: hooks != nil, Backed: hooksBackProperty(hooks, name), Promoted: true, Attributes: param.Attributes, Class: cl}
		if hooks != nil {
			prop.Hooks = l.hooks(prop.Name, hooks)
		}
		cl.Props = append(cl.Props, prop)
	}
}

func (l *lowerer) hooks(prop string, list *frontend.Node) []*Func {
	var out []*Func
	for _, h := range list.Children {
		hc := h.Children
		name := phpname.Lower(hc[2].Text)
		f := l.newFunc(FuncHook, "$"+prop+"::"+name, hc[2].Line)
		f.Attributes = l.checkAttributes(hc[0], "property hook", hc[2].Line)
		f.property = prop
		f.ByRef = h.Text == "&"
		f.Modifiers = modifierList(hc[1])
		f.Class = l.class
		params := hc[3]
		if name == "get" && params != nil {
			l.fail(hc[2].Line, "get hook of property %s::$%s must not have a parameter list", l.class.Name, prop)
		}
		if name == "set" && params != nil && len(params.Children) != 1 {
			l.fail(hc[2].Line, "set hook of property %s::$%s must accept exactly one parameters", l.class.Name, prop)
		}
		if params == nil {
			params = &frontend.Node{Kind: "params", Line: h.Line}
		}
		l.functionBody(f, params, nil, hc[4], nil)
		out = append(out, f)
	}
	return out
}

// confusableTypes are class names PHP warns may be meant as built-in types.
var confusableTypes = map[string]string{"boolean": "bool", "integer": "int", "double": "float", "resource": ""}
