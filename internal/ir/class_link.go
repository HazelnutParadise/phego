package ir

import (
	"fmt"
	"strings"

	"github.com/HazelnutParadise/phego/internal/phpnum"
)

// bindClass records only declarations whose dependencies can be linked now.
// Merely seeing a declaration does not make it available to a later class.
func (l *lowerer) bindClass(cl *Class, top bool) {
	saved := l.class
	l.class = cl
	defer func() { l.class = saved }()
	if hasStringableMethod(cl) || len(cl.Interfaces) != 0 || len(cl.Traits) != 0 || cl.Kind == KindEnum || l.unresolvedHookVariance(cl) {
		return
	}
	if cl.Parent != nil {
		if !top || cl.Anonymous {
			return
		}
		parent := l.boundClass(cl.Parent.Resolved)
		if parent == nil || l.classBindingStatus(cl, parent) == bindingUnknown {
			return
		}
		if parent.Kind != KindClass {
			kind := map[ClassKind]string{KindTrait: "trait", KindInterface: "interface", KindEnum: "enum"}[parent.Kind]
			l.fail(cl.Line, "Class %s cannot extend %s %s", cl.Name, kind, parent.Name)
		}
		if hasModifier(parent.Modifiers, "final") {
			l.fail(cl.Line, "Class %s cannot extend final class %s", cl.Name, parent.Name)
		}
		if hasModifier(cl.Modifiers, "readonly") != hasModifier(parent.Modifiers, "readonly") {
			if hasModifier(cl.Modifiers, "readonly") {
				l.fail(cl.Line, "Readonly class %s cannot extend non-readonly class %s", cl.Name, parent.Name)
			}
			l.fail(cl.Line, "Non-readonly class %s cannot extend readonly class %s", cl.Name, parent.Name)
		}
		l.bindClassMembers(cl, parent)
	}
	l.checkBoundOverrides(cl)
	if top && !cl.Anonymous && cl.Name != "" {
		key := strings.ToLower(strings.TrimPrefix(cl.Name, "\\"))
		if l.boundClasses[key] == nil {
			l.boundClasses[key] = cl
		}
	}
}

func (l *lowerer) bindClassMembers(cl, parent *Class) {
	parentMethods := make(map[string]*Func)
	parentProps := make(map[string]*Prop)
	for current := parent; current != nil; current = l.boundClass(parentName(current)) {
		for _, f := range current.Methods {
			key := strings.ToLower(f.Name)
			if parentMethods[key] == nil {
				parentMethods[key] = f
			}
		}
		for _, p := range current.Props {
			if parentProps[p.Name] == nil {
				parentProps[p.Name] = p
			}
		}
	}
	for _, f := range cl.Methods {
		base := parentMethods[strings.ToLower(f.Name)]
		if base == nil || (hasModifier(base.Modifiers, "private") && !hasModifier(base.Modifiers, "abstract") && !strings.EqualFold(base.Name, "__construct")) {
			continue
		}
		baseClass := parent.Name
		if base.Class != nil {
			baseClass = base.Class.Name
		}
		if hasModifier(base.Modifiers, "final") {
			l.fail(f.Line, "Cannot override final method %s::%s()", baseClass, base.Name)
		}
		if f.Static != hasModifier(base.Modifiers, "static") {
			if f.Static {
				l.fail(f.Line, "Cannot make non static method %s::%s() static in class %s", baseClass, base.Name, cl.Name)
			}
			l.fail(f.Line, "Cannot make static method %s::%s() non static in class %s", baseClass, base.Name, cl.Name)
		}
		if hasModifier(f.Modifiers, "abstract") && !hasModifier(base.Modifiers, "abstract") {
			l.fail(f.Line, "Cannot make non abstract method %s::%s() abstract in class %s", baseClass, base.Name, cl.Name)
		}
		concreteConstructor := false
		if strings.EqualFold(f.Name, "__construct") {
			if proto := l.constructorPrototype(parent); proto != nil {
				base = proto
				if base.Class != nil {
					baseClass = base.Class.Name
				}
			} else {
				concreteConstructor = true
			}
		}
		if !concreteConstructor && !methodVisibilityCompatible(f, base) {
			if strings.EqualFold(f.Name, "__construct") || strings.HasPrefix(strings.ToLower(f.Name), "__") {
				l.fail(f.Line, "Access level to %s::%s() must be %s (as in class %s)", cl.Name, f.Name, visibilityName(base.Modifiers), baseClass)
			}
			l.fail(f.Line, "Access level to %s::%s() must be %s (as in class %s) or weaker", cl.Name, f.Name, visibilityName(base.Modifiers), baseClass)
		}
		compatible := concreteConstructor || l.methodCompatible(cl, parent, f, base)
		if !compatible {
			originClass, originMethod := l.methodOrigin(parent, base.Name)
			l.fail(f.Line, "Declaration of %s must be compatible with %s", methodSignature(cl.Name, f), methodSignature(originClass.Name, originMethod))
		}
	}
	for _, p := range cl.Props {
		base := parentProps[p.Name]
		if base == nil {
			l.verifyHookedProperty(cl, p, p.Backed, cl.Line)
			continue
		}
		baseClass := parent.Name
		if base.Class != nil {
			baseClass = base.Class.Name
		}
		if hasModifier(base.Modifiers, "final") || hasModifier(base.Modifiers, "private(set)") {
			l.fail(cl.Line, "Cannot override final property %s::$%s", baseClass, p.Name)
		}
		if hasModifier(base.Modifiers, "private") {
			continue
		}
		if hasModifier(base.Modifiers, "static") != hasModifier(p.Modifiers, "static") {
			if hasModifier(p.Modifiers, "static") {
				l.fail(cl.Line, "Cannot redeclare non static %s::$%s as static %s::$%s", baseClass, p.Name, cl.Name, p.Name)
			}
			l.fail(cl.Line, "Cannot redeclare static %s::$%s as non static %s::$%s", baseClass, p.Name, cl.Name, p.Name)
		}
		if effectivePropertyReadonly(base) != effectivePropertyReadonly(p) && !hasModifier(base.Modifiers, "abstract") {
			if effectivePropertyReadonly(p) {
				l.fail(cl.Line, "Cannot redeclare non-readonly property %s::$%s as readonly %s::$%s", parent.Name, p.Name, cl.Name, p.Name)
			}
			l.fail(cl.Line, "Cannot redeclare readonly property %s::$%s as non-readonly %s::$%s", parent.Name, p.Name, cl.Name, p.Name)
		}
		if !propertySetVisibilityCompatible(p, base) {
			baseClass := parent.Name
			if base.Class != nil {
				baseClass = base.Class.Name
			}
			required, suffix := "omitted", ""
			if setVisibility := asymmetricVisibility(base.Modifiers); setVisibility != "" {
				required, suffix = setVisibility, " or weaker"
			}
			l.fail(cl.Line, "Set access level of %s::$%s must be %s (as in class %s)%s", cl.Name, p.Name, required, baseClass, suffix)
		}
		if visibilityRank(p.Modifiers) < visibilityRank(base.Modifiers) {
			suffix := " or weaker"
			if visibilityRank(base.Modifiers) == 2 {
				suffix = ""
			}
			l.fail(cl.Line, "Access level to %s::$%s must be %s (as in class %s)%s", cl.Name, p.Name, visibilityName(base.Modifiers), baseClass, suffix)
		}
		for _, h := range p.Hooks {
			for _, ph := range base.Hooks {
				if hookName(h) != hookName(ph) {
					continue
				}
				if hasModifier(ph.Modifiers, "final") {
					l.fail(cl.Line, "Cannot override final property hook %s::$%s::%s()", baseClass, p.Name, hookName(ph))
				}
				childSig, parentSig := effectiveHookSignature(p, h), effectiveHookSignature(base, ph)
				if l.bindingMethod(&childSig, &parentSig) == bindingIncompatible {
					l.fail(h.Line, "Declaration of %s must be compatible with %s", methodSignature(cl.Name, &childSig), methodSignature(baseClass, &parentSig))
				}
			}
		}
		if base.Type == nil && p.Type != nil {
			l.fail(cl.Line, "Type of %s::$%s must be omitted to match the parent definition in class %s", cl.Name, p.Name, baseClass)
		}
		if status, variance := l.propertyTypeStatus(p, base); status == bindingIncompatible {
			l.fail(cl.Line, "Type of %s::$%s must be %s%s (as in class %s)", cl.Name, p.Name, variance, base.Type.zend, baseClass)
		}
		l.verifyHookedProperty(cl, p, l.propertyBacked(p), cl.Line)
	}
	for _, c := range cl.Consts {
		for class := parent; class != nil; class = l.boundClass(parentName(class)) {
			var base *ClassConst
			for _, candidate := range class.Consts {
				if candidate.Name == c.Name {
					// Private constants are not inherited and do not constrain a
					// child declaration with the same name.
					if hasModifier(candidate.Modifiers, "private") {
						continue
					}
					base = candidate
					break
				}
			}
			if base == nil {
				continue
			}
			if hasModifier(base.Modifiers, "final") {
				l.fail(cl.Line, "%s::%s cannot override final constant %s::%s", cl.Name, c.Name, class.Name, base.Name)
			}
			if visibilityRank(c.Modifiers) < visibilityRank(base.Modifiers) {
				suffix := " or weaker"
				if visibilityRank(base.Modifiers) == 2 {
					suffix = ""
				}
				l.fail(cl.Line, "Access level to %s::%s must be %s (as in class %s)%s", cl.Name, c.Name, visibilityName(base.Modifiers), class.Name, suffix)
			}
			if base.Type != nil && (c.Type == nil || l.bindingType(c.Type.zend, base.Type.zend) == bindingIncompatible) {
				l.fail(cl.Line, "Type of %s::%s must be compatible with %s::%s of type %s", cl.Name, c.Name, class.Name, base.Name, base.Type.zend)
			}
			break
		}
	}
	if cl.Kind == KindClass && !hasModifier(cl.Modifiers, "abstract") {
		l.checkAbstractImplementations(cl, parent)
	}
}

func parentName(cl *Class) string {
	if cl == nil || cl.Parent == nil {
		return ""
	}
	return cl.Parent.Resolved
}

func (l *lowerer) hasAbstractParentConstructor(parent *Class) bool {
	for current := parent; current != nil; current = l.boundClass(parentName(current)) {
		for _, method := range current.Methods {
			if strings.EqualFold(method.Name, "__construct") && hasModifier(method.Modifiers, "abstract") {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) hasUnresolvedInterface(cl *Class) bool {
	for current := cl; current != nil; current = l.boundClass(parentName(current)) {
		for _, name := range current.Interfaces {
			iface := l.boundClass(name.Resolved)
			if iface == nil || iface.Kind != KindInterface {
				return true
			}
		}
	}
	return false
}

func hookName(f *Func) string {
	return strings.ToLower(f.Name[strings.LastIndex(f.Name, "::")+2:])
}

func visibilityName(mods []string) string {
	if hasModifier(mods, "private") {
		return "private"
	}
	if hasModifier(mods, "protected") {
		return "protected"
	}
	return "public"
}

func visibilityRank(mods []string) int {
	switch visibilityName(mods) {
	case "public":
		return 2
	case "protected":
		return 1
	default:
		return 0
	}
}

func methodVisibilityCompatible(child, parent *Func) bool {
	return visibilityRank(child.Modifiers) >= visibilityRank(parent.Modifiers)
}

func propertySetVisibilityCompatible(child, parent *Prop) bool {
	// A read-only virtual property grants no parent write operation. Its
	// implementation may add a setter with independently chosen visibility.
	if parent.HasHooks && !hasPropertyHook(parent, "set") {
		return true
	}
	if !hasAsymmetricVisibility(child.Modifiers) && !(effectivePropertyReadonly(child) && visibilityRank(child.Modifiers) == 2) {
		return true
	}
	return propertySetVisibilityRank(child) >= propertySetVisibilityRank(parent)
}

func propertySetVisibilityRank(p *Prop) int {
	if rank := setVisibilityRank(p.Modifiers); hasAsymmetricVisibility(p.Modifiers) {
		return rank
	}
	if effectivePropertyReadonly(p) && visibilityRank(p.Modifiers) == 2 {
		// PHP gives public readonly properties protected(set) by default.
		return 1
	}
	return visibilityRank(p.Modifiers)
}

func asymmetricVisibility(mods []string) string {
	for _, mod := range mods {
		switch mod {
		case "public(set)", "protected(set)", "private(set)":
			return mod
		}
	}
	return ""
}

func setVisibilityRank(mods []string) int {
	for _, m := range mods {
		switch m {
		case "private(set)":
			return 0
		case "protected(set)":
			return 1
		case "public(set)":
			return 2
		}
	}
	return visibilityRank(mods)
}

func (l *lowerer) checkAbstractImplementations(cl, parent *Class) {
	if len(cl.Traits) != 0 {
		return // trait methods may satisfy the inherited declarations during linking
	}
	var missing []string
	seenMethods := map[string]bool{}
	for current := cl; current != nil; current = l.boundClass(parentName(current)) {
		for _, method := range current.Methods {
			key := strings.ToLower(method.Name)
			if seenMethods[key] {
				continue
			}
			seenMethods[key] = true
			if hasModifier(method.Modifiers, "abstract") {
				owner := current.Name
				if method.Class != nil {
					owner = method.Class.Name
				}
				missing = append(missing, owner+"::"+method.Name)
			}
		}
	}
	seenProps := map[string]bool{}
	for current := cl; current != nil; current = l.boundClass(parentName(current)) {
		for _, p := range current.Props {
			if seenProps[p.Name] {
				continue
			}
			seenProps[p.Name] = true
			for _, slot := range []string{"get", "set"} {
				hook := l.effectivePropertyHook(cl, p, slot)
				if hook == nil || hook.HasBody && !hasModifier(hook.Modifiers, "abstract") {
					continue
				}
				owner := current.Name
				if hook.Class != nil {
					owner = hook.Class.Name
				}
				missing = append(missing, owner+"::$"+p.Name+"::"+slot)
			}
		}
	}
	n := len(missing)
	if n == 0 {
		return
	}
	shown := missing
	if len(shown) > 3 {
		shown = shown[:3]
		shown = append(shown, "...")
	}
	plural := ""
	if n != 1 {
		plural = "s"
	}
	l.fail(cl.Line, "Class %s contains %d abstract method%s and must therefore be declared abstract or implement the remaining method%s (%s)", cl.Name, n, plural, plural, strings.Join(shown, ", "))
}

// effectivePropertyHook applies PHP's per-slot inheritance rules for an
// overriding property. Inherited properties keep the hook set materialized by
// their declaring class, which preserves PHP's deferred abstract-hook behavior.
func (l *lowerer) effectivePropertyHook(cl *Class, prop *Prop, slot string) *Func {
	if prop.Class != cl {
		return propertyHook(prop, slot)
	}
	var chain []*Class
	for current := cl; current != nil; current = l.boundClass(parentName(current)) {
		chain = append(chain, current)
	}
	var effective *Func
	for i := len(chain) - 1; i >= 0; i-- {
		current := chain[i]
		var currentProp *Prop
		for _, candidate := range current.Props {
			if candidate.Name == prop.Name {
				currentProp = candidate
				break
			}
		}
		if currentProp == nil {
			continue
		}
		if own := propertyHook(currentProp, slot); own != nil {
			effective = own
			continue
		}
		if effective != nil && abstractHook(effective) && propertyHasOperation(currentProp, slot) {
			effective = nil
		}
	}
	return effective
}

func propertyHook(prop *Prop, slot string) *Func {
	for _, hook := range prop.Hooks {
		if hookName(hook) == slot {
			return hook
		}
	}
	return nil
}

func abstractHook(hook *Func) bool {
	return hook != nil && (!hook.HasBody || hasModifier(hook.Modifiers, "abstract"))
}

func propertyHasOperation(prop *Prop, slot string) bool {
	if propertyHook(prop, slot) != nil {
		return true
	}
	if prop.HasHooks && !prop.Backed {
		return false
	}
	if slot == "get" {
		return true
	}
	return !effectivePropertyReadonly(prop)
}

func sameType(a, b string) bool {
	left, right := typeDNF(a), typeDNF(b)
	if len(left) != len(right) {
		return false
	}
	used := make([]bool, len(right))
	for _, clause := range left {
		found := false
		for j, other := range right {
			if !used[j] && sameTypeClause(clause, other) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sameTypeClause(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, atom := range a {
		found := false
		for i, other := range b {
			if !used[i] && strings.EqualFold(atom, other) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (l *lowerer) methodOrigin(parent *Class, name string) (*Class, *Func) {
	originClass, origin := parent, (*Func)(nil)
	seen := map[string]bool{}
	for class := parent; class != nil; {
		key := strings.ToLower(class.Name)
		if seen[key] {
			break
		}
		seen[key] = true
		for _, f := range class.Methods {
			if strings.EqualFold(f.Name, name) {
				if origin == nil {
					originClass, origin = class, f
				} else if !sameFuncSignature(f, origin) {
					return originClass, origin
				} else {
					originClass, origin = class, f
				}
				break
			}
		}
		if class.Parent == nil {
			break
		}
		class = l.boundClass(class.Parent.Resolved)
	}
	return originClass, origin
}

func sameFuncSignature(a, b *Func) bool {
	if a.ByRef != b.ByRef || len(a.Params) != len(b.Params) {
		return false
	}
	if a.Return == nil != (b.Return == nil) {
		return false
	}
	if a.Return != nil && !sameType(a.Return.zend, b.Return.zend) {
		return false
	}
	for i := range a.Params {
		x, y := a.Params[i], b.Params[i]
		if x.ByRef != y.ByRef || x.Variadic != y.Variadic || x.Type == nil != (y.Type == nil) {
			return false
		}
		if x.Type != nil && !sameType(x.Type.zend, y.Type.zend) {
			return false
		}
		if (x.Default == nil) != (y.Default == nil) {
			return false
		}
	}
	return true
}

func (l *lowerer) methodCompatible(childClass, parentClass *Class, child, parent *Func) bool {
	if child.Static != parent.Static {
		return false
	}
	if parent.ByRef && !child.ByRef {
		return false
	}
	childVariadic := len(child.Params) != 0 && child.Params[len(child.Params)-1].Variadic
	if (len(child.Params) < len(parent.Params) && !childVariadic) || requiredParams(child) > requiredParams(parent) {
		return false
	}
	if !l.returnTypeCovariant(child.Return, parent.Return, childClass, parentClass) {
		return false
	}
	for i, p := range parent.Params {
		childIndex := i
		if childIndex >= len(child.Params) {
			if !childVariadic {
				return false
			}
			childIndex = len(child.Params) - 1
		}
		c := child.Params[childIndex]
		if p.ByRef != c.ByRef {
			return false
		}
		if p.Type != nil && c.Type != nil && !l.parameterTypeCompatible(c.Type.zend, p.Type.zend) {
			return false
		}
		if p.Type == nil && c.Type != nil && c.Type.zend != "mixed" {
			return false
		}
	}
	return true
}

func requiredParams(f *Func) int {
	n := 0
	for _, p := range f.Params {
		if p.Default == nil && !p.Variadic {
			n++
		}
	}
	return n
}

func (l *lowerer) returnTypeCovariant(child, parent *Type, childClass, parentClass *Class) bool {
	if parent == nil {
		return true
	}
	if child == nil {
		return false
	}
	return l.returnTypeCompatible(child.zend, parent.zend, childClass, parentClass)
}

func (l *lowerer) parameterTypeCompatible(child, parent string) bool {
	return l.bindingType(parent, child) != bindingIncompatible
}

func (l *lowerer) returnTypeCompatible(child, parent string, childClass, parentClass *Class) bool {
	if strings.HasPrefix(parent, "?") {
		parent = strings.TrimPrefix(parent, "?") + "|null"
	}
	if strings.HasPrefix(child, "?") {
		child = strings.TrimPrefix(child, "?") + "|null"
	}
	if strings.EqualFold(child, parent) || child == "never" {
		return true
	}
	if strings.EqualFold(parent, "static") {
		return childClass != nil && hasModifier(childClass.Modifiers, "final") && strings.EqualFold(child, childClass.Name)
	}
	if parent == "mixed" {
		return child != "void"
	}
	if child == "static" && (childClass != nil && strings.EqualFold(parent, childClass.Name) || parentClass != nil && strings.EqualFold(parent, parentClass.Name)) {
		return true
	}
	if !l.typesKnown(child, parent) {
		return true
	}
	return l.typeIsSubtype(child, parent)
}

func (l *lowerer) typeIsSubtype(child, parent string) bool {
	if strings.EqualFold(child, parent) || strings.EqualFold(parent, "mixed") || strings.EqualFold(child, "never") {
		return true
	}
	childParts, parentParts := typeDNF(child), typeDNF(parent)
	for _, childClause := range childParts {
		covered := false
		for _, parentClause := range parentParts {
			if l.typeClauseSubtype(childClause, parentClause) || len(childClause) > 1 && len(childClause) == len(parentClause) && l.typeClauseSubtype(parentClause, childClause) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// typeDNF splits canonical PHP type spellings into unions of intersections.
// Parentheses only group intersections inside unions in the compiler output.
func typeDNF(t string) [][]string {
	if strings.HasPrefix(t, "?") {
		t = strings.TrimPrefix(t, "?") + "|null"
	}
	var out [][]string
	for _, union := range splitTypeTopLevel(t, '|') {
		union = stripTypeParens(strings.TrimSpace(union))
		var clause []string
		for _, atom := range splitTypeTopLevel(union, '&') {
			atom = strings.ToLower(strings.TrimSpace(stripTypeParens(strings.TrimSpace(atom))))
			if atom != "" {
				clause = append(clause, atom)
			}
		}
		if len(clause) != 0 {
			out = append(out, clause)
		}
	}
	return out
}

func splitTypeTopLevel(t string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if t[i] == sep && depth == 0 {
				parts = append(parts, t[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, t[start:])
}

func stripTypeParens(t string) string {
	for len(t) >= 2 && t[0] == '(' && t[len(t)-1] == ')' {
		depth, wrapsAll := 0, true
		for i := 0; i < len(t)-1; i++ {
			switch t[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					wrapsAll = false
				}
			}
			if depth < 0 {
				wrapsAll = false
				break
			}
		}
		if !wrapsAll {
			break
		}
		t = strings.TrimSpace(t[1 : len(t)-1])
	}
	return t
}

func (l *lowerer) typeClauseSubtype(child, parent []string) bool {
	for _, want := range parent {
		covered := false
		for _, got := range child {
			if l.atomSubtype(got, want) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func (l *lowerer) atomSubtype(child, parent string) bool {
	child, parent = strings.TrimPrefix(child, "\\"), strings.TrimPrefix(parent, "\\")
	if child == parent || parent == "mixed" || child == "never" {
		return true
	}
	if parent == "object" {
		return !map[string]bool{"int": true, "float": true, "string": true, "bool": true, "true": true, "false": true, "array": true, "null": true, "void": true, "never": true, "callable": true, "iterable": true, "mixed": true, "resource": true}[child]
	}
	if parent == "bool" && (child == "true" || child == "false") || parent == "iterable" && child == "generator" {
		return true
	}
	if parent == "iterable" && (child == "array" || l.classSubtype(child, "traversable")) {
		return true
	}
	if parent == "callable" && child == "closure" {
		return true
	}
	return l.classSubtype(child, parent)
}

func (l *lowerer) classSubtype(child, parent string) bool {
	child = strings.TrimPrefix(child, "\\")
	parent = strings.TrimPrefix(parent, "\\")
	if strings.EqualFold(child, parent) {
		return true
	}
	builtins := map[string][]string{"arrayobject": {"traversable", "iteratoraggregate"}, "iterator": {"traversable"}, "generator": {"iterator", "traversable"}, "closure": {"callable"}}
	for _, p := range builtins[strings.ToLower(child)] {
		if strings.EqualFold(p, parent) {
			return true
		}
	}
	seen := map[string]bool{}
	queue := []string{child}
	for len(queue) > 0 && len(seen) <= len(l.boundClasses)+4 {
		name := queue[0]
		queue = queue[1:]
		name = strings.TrimPrefix(name, "\\")
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		if strings.EqualFold(name, parent) {
			return true
		}
		c := l.boundClass(key)
		if c == nil && l.class != nil && strings.EqualFold(key, l.class.Name) {
			c = l.class
		}
		if c == nil {
			continue
		}
		if c.Parent != nil {
			queue = append(queue, c.Parent.Resolved)
		}
		for _, i := range c.Interfaces {
			queue = append(queue, i.Resolved)
		}
	}
	return false
}

func (l *lowerer) boundClass(name string) *Class {
	if cl := builtinCoreClass(l.unit.Profile, name); cl != nil {
		return cl
	}
	return l.boundClasses[strings.ToLower(strings.TrimPrefix(name, "\\"))]
}

func (l *lowerer) typesKnown(a, b string) bool {
	knownBuiltin := map[string]bool{"mixed": true, "null": true, "bool": true, "true": true, "false": true, "int": true, "float": true, "string": true, "array": true, "object": true, "void": true, "never": true, "callable": true, "iterable": true, "static": true, "self": true, "parent": true, "closure": true, "traversable": true, "iterator": true, "iteratoraggregate": true, "arrayobject": true, "stdclass": true, "stringable": true, "countable": true, "throwable": true}
	for _, t := range []string{a, b} {
		for _, clause := range typeDNF(t) {
			for _, atom := range clause {
				atom = strings.TrimPrefix(atom, "\\")
				if knownBuiltin[atom] || l.boundClass(atom) != nil || l.class != nil && strings.EqualFold(atom, l.class.Name) {
					continue
				}
				return false
			}
		}
	}
	return true
}

func methodSignature(class string, f *Func) string {
	var params []string
	for _, p := range f.Params {
		s := ""
		if p.Type != nil {
			s += p.Type.zend + " "
		}
		if p.ByRef {
			s += "&"
		}
		if p.Variadic {
			s += "..."
		}
		s += "$" + p.Name
		if p.Default != nil && (p.Type == nil || !p.Type.Implicit) {
			s += " = " + literalPreview(p.Default)
		}
		params = append(params, s)
	}
	prefix := ""
	if f.ByRef {
		prefix = "& "
	}
	s := prefix + class + "::" + f.Name + "(" + strings.Join(params, ", ") + ")"
	if f.Return != nil {
		s += ": " + f.Return.zend
	}
	return s
}

func literalPreview(n *Node) string {
	if n == nil {
		return "..."
	}
	if n.Op == OpArray {
		if len(n.Kids) == 0 {
			return "[]"
		}
		return "[...]"
	}
	if n.Op == OpConst && n.Name != nil {
		return n.Name.Resolved
	}
	if n.Op == OpClassConst && n.Name != nil && n.Text != "" {
		return n.Name.Resolved + "::" + n.Text
	}
	if n.Op != OpLiteral || n.Value == nil {
		return "<expression>"
	}
	switch n.Value.Kind {
	case LitNull:
		return "null"
	case LitBool:
		if n.Value.Bool {
			return "true"
		}
		return "false"
	case LitInt:
		return fmt.Sprint(n.Value.Int)
	case LitFloat:
		return phpnum.FormatFloat(n.Value.Float, 14)
	case LitString:
		if len(n.Value.Str) > 16 {
			return fmt.Sprintf("'%s...'%s", n.Value.Str[:10], "")
		}
		return fmt.Sprintf("'%s'", n.Value.Str)
	default:
		return "..."
	}
}

func arrayPreview(n *Node) string {
	parts := make([]string, 0, len(n.Kids))
	for _, item := range n.Kids {
		if item == nil || item.Op != OpItem || len(item.Kids) < 2 {
			return "[...]"
		}
		if item.Kids[0] != nil {
			parts = append(parts, literalPreview(item.Kids[0])+" => "+literalPreview(item.Kids[1]))
		} else {
			parts = append(parts, literalPreview(item.Kids[1]))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func hasStringableMethod(cl *Class) bool {
	for _, f := range cl.Methods {
		if strings.EqualFold(f.Name, "__tostring") {
			return true
		}
	}
	return false
}

func (l *lowerer) constructorPrototype(parent *Class) *Func {
	var proto *Func
	for current := parent; current != nil; current = l.boundClass(parentName(current)) {
		for _, f := range current.Methods {
			if strings.EqualFold(f.Name, "__construct") && hasModifier(f.Modifiers, "abstract") {
				proto = f
			}
		}
	}
	return proto
}

func (l *lowerer) propertyBacked(p *Prop) bool {
	for p != nil {
		if !p.HasHooks || p.Backed {
			return true
		}
		p = l.boundParentProperty(p.Class, p.Name)
	}
	return false
}

// effectiveHookSignature supplies implicit PHP signature components on a
// shallow copy. The declaration and its compiled function stay unchanged.
func effectiveHookSignature(p *Prop, h *Func) Func {
	f := *h
	if hookName(h) == "get" {
		f.Return = p.Type
	} else {
		f.Return = &Type{String: "void", zend: "void"}
		if len(f.Params) == 0 {
			f.Params = []*Param{{Name: "value", Type: p.Type}}
		}
	}
	return f
}

func (l *lowerer) propertyTypeStatus(child, parent *Prop) (bindingResult, string) {
	variance := ""
	if !l.propertyBacked(parent) {
		if !hasPropertyHook(parent, "set") {
			variance = "subtype of "
		} else if !hasPropertyHook(parent, "get") {
			variance = "supertype of "
		}
	}
	if parent.Type == nil {
		if child.Type == nil {
			return bindingCompatible, variance
		}
		return bindingIncompatible, variance
	}
	if child.Type == nil {
		return bindingIncompatible, variance
	}
	a, b := bindingCompatible, bindingCompatible
	if variance != "supertype of " {
		a = l.bindingType(child.Type.zend, parent.Type.zend)
	}
	if variance != "subtype of " {
		b = l.bindingType(parent.Type.zend, child.Type.zend)
	}
	if a == bindingIncompatible || b == bindingIncompatible {
		return bindingIncompatible, variance
	}
	if a == bindingUnknown || b == bindingUnknown {
		return bindingUnknown, variance
	}
	return bindingCompatible, variance
}
