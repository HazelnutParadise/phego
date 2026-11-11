package ir

import (
	"slices"
	"strings"
)

// An unresolved subtype obligation defers the declaration as a whole. It is
// neither a successful check nor an incompatible declaration.
type bindingResult uint8

const (
	bindingCompatible bindingResult = iota
	bindingIncompatible
	bindingUnknown
)

func (l *lowerer) unresolvedHookVariance(cl *Class) bool {
	for _, p := range cl.Props {
		for _, h := range p.Hooks {
			if hookName(h) == "set" && len(h.Params) == 1 && p.Type != nil && h.Params[0].Type != nil {
				if l.bindingType(p.Type.zend, h.Params[0].Type.zend) == bindingUnknown {
					return true
				}
			}
		}
	}
	return false
}

func (l *lowerer) classBindingStatus(cl, parent *Class) bindingResult {
	// Preserve parent declaration order, including the position of overridden
	// members inherited from earlier ancestors.
	var chain []*Class
	for p := parent; p != nil; p = l.boundClass(parentName(p)) {
		chain = append(chain, p)
	}
	var methods []*Func
	var props []*Prop
	var constants []*ClassConst
	mi, pi, ci := map[string]int{}, map[string]int{}, map[string]int{}
	for i := len(chain) - 1; i >= 0; i-- {
		for _, f := range chain[i].Methods {
			key := strings.ToLower(f.Name)
			if j, ok := mi[key]; ok {
				methods[j] = f
			} else {
				mi[key] = len(methods)
				methods = append(methods, f)
			}
		}
		for _, p := range chain[i].Props {
			if j, ok := pi[p.Name]; ok {
				props[j] = p
			} else {
				pi[p.Name] = len(props)
				props = append(props, p)
			}
		}
		for _, c := range chain[i].Consts {
			if j, ok := ci[c.Name]; ok {
				constants[j] = c
			} else {
				ci[c.Name] = len(constants)
				constants = append(constants, c)
			}
		}
	}
	children := map[string]*Func{}
	for _, f := range cl.Methods {
		children[strings.ToLower(f.Name)] = f
	}
	for _, base := range methods {
		f := children[strings.ToLower(base.Name)]
		if f == nil {
			continue
		}
		if hasModifier(base.Modifiers, "private") && !hasModifier(base.Modifiers, "abstract") && !strings.EqualFold(base.Name, "__construct") {
			continue
		}
		if hasModifier(base.Modifiers, "final") || f.Static != base.Static || hasModifier(f.Modifiers, "abstract") && !hasModifier(base.Modifiers, "abstract") {
			return bindingIncompatible
		}
		if strings.EqualFold(base.Name, "__construct") {
			if proto := l.constructorPrototype(parent); proto != nil {
				base = proto
			} else {
				continue
			}
		}
		if !methodVisibilityCompatible(f, base) {
			return bindingIncompatible
		}
		if result := l.bindingMethod(f, base); result != bindingCompatible {
			return result
		}
	}
	childProps := map[string]*Prop{}
	for _, p := range cl.Props {
		childProps[p.Name] = p
	}
	for _, base := range props {
		p := childProps[base.Name]
		if p == nil || hasModifier(base.Modifiers, "private") {
			continue
		}
		for _, h := range p.Hooks {
			for _, ph := range base.Hooks {
				if hookName(h) != hookName(ph) {
					continue
				}
				x, y := effectiveHookSignature(p, h), effectiveHookSignature(base, ph)
				if result := l.bindingMethod(&x, &y); result != bindingCompatible {
					return result
				}
			}
		}
		if result, _ := l.propertyTypeStatus(p, base); result != bindingCompatible {
			return result
		}
	}
	childConsts := map[string]*ClassConst{}
	for _, c := range cl.Consts {
		childConsts[c.Name] = c
	}
	for _, base := range constants {
		c := childConsts[base.Name]
		if c == nil || base.Type == nil || c.Type == nil || hasModifier(base.Modifiers, "private") {
			continue
		}
		if result := l.bindingType(c.Type.zend, base.Type.zend); result != bindingCompatible {
			return result
		}
	}
	return bindingCompatible
}

func effectivePropertyReadonly(p *Prop) bool {
	return p != nil && (hasModifier(p.Modifiers, "readonly") || p.Class != nil && hasModifier(p.Class.Modifiers, "readonly"))
}

func (l *lowerer) bindingMethod(child, parent *Func) bindingResult {
	if requiredParams(child) > requiredParams(parent) || parent.ByRef && !child.ByRef {
		return bindingIncompatible
	}
	cv := len(child.Params) > 0 && child.Params[len(child.Params)-1].Variadic
	pv := len(parent.Params) > 0 && parent.Params[len(parent.Params)-1].Variadic
	if pv && !cv {
		return bindingIncompatible
	}
	result := bindingCompatible
	for i := 0; i < max(len(child.Params), len(parent.Params)); i++ {
		var c, p *Param
		if i < len(child.Params) {
			c = child.Params[i]
		} else if cv {
			c = child.Params[len(child.Params)-1]
		}
		if i < len(parent.Params) {
			p = parent.Params[i]
		} else if pv {
			p = parent.Params[len(parent.Params)-1]
		}
		if p == nil {
			continue
		}
		if c == nil {
			return bindingIncompatible
		}
		if c.Type != nil && c.Type.zend != "mixed" {
			if p.Type == nil {
				return bindingIncompatible
			}
			status := l.bindingType(p.Type.zend, c.Type.zend)
			if status == bindingIncompatible {
				return status
			}
			if status == bindingUnknown {
				result = status
			}
		}
		if c.ByRef != p.ByRef {
			return bindingIncompatible
		}
	}
	if parent.Return != nil {
		if child.Return == nil {
			return bindingIncompatible
		}
		status := l.bindingType(child.Return.zend, parent.Return.zend)
		if status != bindingCompatible {
			return status
		}
	}
	return result
}

func (l *lowerer) bindingType(child, parent string) bindingResult {
	if sameType(child, parent) || child == "never" {
		return bindingCompatible
	}
	if parent == "mixed" {
		if child == "void" {
			return bindingIncompatible
		}
		return bindingCompatible
	}
	result := bindingCompatible
	parentDNF := typeDNF(parent)
	for _, c := range typeDNF(child) {
		if len(c) == 1 && c[0] == "static" {
			// zend_perform_covariant_type_check: static may replace a type
			// that already accepts the class. That check never loads classes.
			if !slices.ContainsFunc(parentDNF, func(p []string) bool { return len(p) == 1 && p[0] == "static" }) && !l.permitsSelf(parentDNF) {
				return bindingIncompatible
			}
			continue
		}
		union := bindingIncompatible
		for _, p := range parentDNF {
			clause := bindingCompatible
			for _, want := range p {
				atom := bindingIncompatible
				for _, got := range c {
					candidate := l.bindingAtom(got, want)
					if candidate == bindingCompatible {
						atom = candidate
						break
					}
					if candidate == bindingUnknown {
						atom = candidate
					}
				}
				if atom == bindingIncompatible {
					clause = atom
					break
				}
				if atom == bindingUnknown {
					clause = atom
				}
			}
			if clause == bindingCompatible {
				union = clause
				break
			}
			if clause == bindingUnknown {
				union = clause
			}
		}
		if union == bindingIncompatible {
			return union
		}
		if union == bindingUnknown {
			result = union
		}
	}
	return result
}

// permitsSelf is zend_type_permits_self for the class being bound. Its
// ZEND_TYPE_FOREACH visits the members of a pure intersection but skips an
// intersection nested in a union.
func (l *lowerer) permitsSelf(parent [][]string) bool {
	if l.class == nil {
		return false
	}
	names := parent
	if len(parent) == 1 {
		names = nil
		for _, atom := range parent[0] {
			names = append(names, []string{atom})
		}
	}
	for _, clause := range names {
		if len(clause) != 1 {
			continue
		}
		atom := strings.TrimPrefix(clause[0], "\\")
		if atom == "object" || !scalarTypeAtom(atom) && l.classSubtype(l.class.Name, atom) {
			return true
		}
	}
	return false
}

func scalarTypeAtom(s string) bool {
	switch s {
	case "mixed", "null", "bool", "true", "false", "int", "float", "string", "array", "object", "void", "never", "callable", "iterable", "static", "self", "parent":
		return true
	}
	return false
}

func (l *lowerer) bindingAtom(child, parent string) bindingResult {
	child, parent = strings.TrimPrefix(child, "\\"), strings.TrimPrefix(parent, "\\")
	if child == parent || child == "never" {
		return bindingCompatible
	}
	if parent == "mixed" && child != "void" {
		return bindingCompatible
	}
	if !scalarTypeAtom(child) && parent == "static" {
		// zend_is_class_subtype_of_type: only a final class may name itself
		// in place of static, and an unloaded name leaves that unresolved.
		if l.class == nil || !hasModifier(l.class.Modifiers, "final") {
			return bindingIncompatible
		}
		if strings.EqualFold(child, l.class.Name) {
			return bindingCompatible
		}
		if !l.typesKnown(child, child) {
			return bindingUnknown
		}
		return bindingIncompatible
	}
	if !scalarTypeAtom(child) && (parent == "object" || parent == "callable" || !scalarTypeAtom(parent)) {
		if !l.typesKnown(child, parent) {
			return bindingUnknown
		}
	}
	if l.atomSubtype(child, parent) {
		return bindingCompatible
	}
	return bindingIncompatible
}

func (l *lowerer) checkBoundOverrides(cl *Class) {
	if cl.Kind == KindTrait {
		return
	}
	for _, f := range cl.Methods {
		if hasClassMarker(f.Attributes, "Override") && !l.boundParentMethod(cl, f.Name) {
			l.fail(f.Line, "%s::%s() has #[\\Override] attribute, but no matching parent method exists", cl.Name, f.Name)
		}
	}
	for _, p := range cl.Props {
		base := l.boundParentProperty(cl, p.Name)
		if hasClassMarker(p.Attributes, "Override") && base == nil {
			l.fail(cl.Line, "%s::$%s has #[\\Override] attribute, but no matching parent property exists", cl.Name, p.Name)
		}
		for _, h := range p.Hooks {
			if !hasClassMarker(h.Attributes, "Override") {
				continue
			}
			found := base != nil && (!base.HasHooks || base.Backed || hasPropertyHook(base, hookName(h)))
			if !found {
				l.fail(h.Line, "%s::$%s::%s() has #[\\Override] attribute, but no matching parent method exists", cl.Name, p.Name, hookName(h))
			}
		}
	}
}

func (l *lowerer) boundParentMethod(cl *Class, name string) bool {
	if strings.EqualFold(name, "__construct") {
		return l.hasAbstractParentConstructor(l.boundClass(parentName(cl)))
	}
	for p := l.boundClass(parentName(cl)); p != nil; p = l.boundClass(parentName(p)) {
		for _, f := range p.Methods {
			if strings.EqualFold(f.Name, name) && !hasModifier(f.Modifiers, "private") {
				return true
			}
		}
	}
	return false
}
func (l *lowerer) boundParentProperty(cl *Class, name string) *Prop {
	for p := l.boundClass(parentName(cl)); p != nil; p = l.boundClass(parentName(p)) {
		for _, prop := range p.Props {
			if prop.Name == name && !hasModifier(prop.Modifiers, "private") {
				return prop
			}
		}
	}
	return nil
}
