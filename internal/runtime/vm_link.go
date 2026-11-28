package runtime

import (
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. The
// inheritance rules PHP applies when it links a class at run time
// (zend_do_link_class, do_inheritance_check_on_method,
// zend_verify_abstract_class), against the classes that exist then. The
// lowering applies the same rules at compile time to the classes PHP binds
// early; every other class reaches these checks through OpDeclareClass, so
// bytecode that did not come from the compiler is checked as well. Each
// failure is PHP's fatal error, with its message.

// methodSignatureText is the declaration PHP prints for method lower of
// class c in an inheritance error.
func methodSignatureText(c *class, m *methodSpec) string {
	lower := phpname.Lower(m.name)
	for owner := c; owner != nil && owner.vm != nil; {
		if tail, ok := owner.vm.sigs[lower]; ok {
			return m.owner.name + "::" + tail
		}
		if owner.schema == nil {
			break
		}
		owner = owner.schema.parent
	}
	return m.owner.name + "::" + m.name + "()"
}

// typeAtoms splits a declared type into its alternatives, lower-cased
// where PHP compares case-insensitively; "?T" is "T|null" and iterable is
// "Traversable|array".
func typeAtoms(typ string) []string {
	typ = strings.TrimSpace(typ)
	if strings.HasPrefix(typ, "?") {
		typ = typ[1:] + "|null"
	}
	var out []string
	for part := range splitMethodType(typ, '|') {
		part = strings.TrimPrefix(strings.TrimSpace(part), "\\")
		if part == "iterable" {
			out = append(out, "traversable", "array")
			continue
		}
		out = append(out, phpname.Lower(part))
	}
	return out
}

// subtype reports whether every value of type a is a value of type b
// (zend_perform_covariant_type_check), for types declared in classes ca and
// cb. A class name neither side has loaded is compared by name.
func (s *vmState) subtype(a string, ca *class, b string, cb *class) bool {
	if b == "" || b == "mixed" {
		return true
	}
	if a == "" {
		return false
	}
	resolve := func(atom string, c *class) (string, *class) {
		switch atom {
		case "self", "static":
			if c != nil {
				return phpname.Lower(c.name), c
			}
		case "parent":
			if c != nil && c.schema != nil && c.schema.parent != nil {
				return phpname.Lower(c.schema.parent.name), c.schema.parent
			}
		}
		if l := s.linking; l != nil && atom == phpname.Lower(l.name) {
			return atom, l
		}
		return atom, s.h.classes[atom]
	}
	want := typeAtoms(b)
	for _, x := range typeAtoms(a) {
		if x == "mixed" {
			return false
		}
		xn, xc := resolve(x, ca)
		ok := false
		for _, y := range want {
			yn, yc := resolve(y, cb)
			switch {
			case xn == yn, y == "mixed":
				ok = true
			case x == "static" && y == "static":
				ok = true
			case y == "static":
				// Only static is a subtype of static.
			case yn == "object" && (xc != nil || !isBuiltinTypeName(xn)):
				ok = true
			case xn == "true" && yn == "bool", xn == "false" && yn == "bool":
				ok = true
			case xc != nil && yc != nil && classIs(xc, yc):
				ok = true
			}
			if ok {
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func isBuiltinTypeName(n string) bool {
	switch n {
	case "int", "float", "string", "bool", "array", "null", "void", "never", "mixed", "object", "callable", "true", "false", "iterable":
		return true
	}
	return false
}

// signatureCompatible is zend_do_perform_implementation_check: whether the
// method child of class cc may replace parent of class pc.
func (s *vmState) signatureCompatible(child *methodSpec, cc *class, parent *methodSpec, pc *class) bool {
	if child.fn == 0 || parent.fn == 0 {
		return true
	}
	cf, pf := &s.prog.funcs[child.fn-1], &s.prog.funcs[parent.fn-1]
	if cf.required > pf.required {
		return false
	}
	if !cf.variadic && cf.params < pf.params {
		return false
	}
	if pf.variadic && !cf.variadic {
		return false
	}
	ptype := func(f *vmFunction, i int) string {
		if i < len(f.paramTypes) {
			return f.paramTypes[i]
		}
		return ""
	}
	n := max(cf.params, pf.params)
	for i := 0; i < n; i++ {
		ci, pi := i, i
		if i >= cf.params {
			if !cf.variadic {
				break
			}
			ci = cf.params // the variadic parameter takes the rest
		}
		if i >= pf.params {
			if !pf.variadic {
				break
			}
			pi = pf.params
		}
		if cf.byRefAt(ci) != pf.byRefAt(pi) {
			return false
		}
		// Parameters are contravariant: the child accepts all the parent does.
		if ct := ptype(cf, ci); ct != "" && !s.subtype(ptype(pf, pi), pc, ct, cc) {
			return false
		}
	}
	if pf.variadic && cf.variadic {
		if cf.variadicByRef != pf.variadicByRef {
			return false
		}
		if ct := ptype(cf, cf.params); ct != "" && !s.subtype(ptype(pf, pf.params), pc, ct, cc) {
			return false
		}
	}
	// The return type is covariant.
	if pf.returnType != "" {
		if cf.returnType == "" || !s.subtype(cf.returnType, cc, pf.returnType, pc) {
			return false
		}
	}
	return true
}

// linkCheck applies the rules to the class d is about to become; spec
// holds its resolved parent, interfaces and methods.
func (s *vmState) linkCheck(vc *vmClass, d *vmClassDecl, spec *classSpec) error {
	vc.sigs = make(map[string]string, len(d.methods))
	lines := make(map[string]int, len(d.methods))
	decls := make(map[string]bytecode.MemberFlags, len(d.methods))
	for i := range d.methods {
		m := &d.methods[i]
		decls[phpname.Lower(m.name)] = m.flags
		tail := m.signature
		if j := strings.Index(tail, "::"); j >= 0 {
			tail = tail[j+2:]
		}
		vc.sigs[phpname.Lower(m.name)] = tail
		lines[phpname.Lower(m.name)] = m.line
	}
	// self stands for the class while it is linked: its name and ancestors
	// are what the type checks need.
	self := &class{name: d.name, vm: vc, schema: &classSchema{classSpec: classSpec{name: d.name, parent: spec.parent, interfaces: spec.interfaces}}}
	s.linking = self
	defer func() { s.linking = nil }()
	own := make(map[string]*methodSpec, len(spec.methods))
	for i := range spec.methods {
		m := &spec.methods[i]
		m.owner = self // for messages; buildClass sets the real owner
		own[phpname.Lower(m.name)] = m
	}
	signature := func(m *methodSpec) string {
		if m.owner == self {
			if tail, ok := vc.sigs[phpname.Lower(m.name)]; ok {
				return d.name + "::" + tail
			}
			// A trait's method keeps the trait's declaration.
			for _, t := range spec.traits {
				if t.vm != nil {
					if tail, ok := t.vm.sigs[phpname.Lower(m.name)]; ok {
						return d.name + "::" + tail
					}
				}
			}
			return d.name + "::" + m.name + "()"
		}
		return methodSignatureText(m.owner, m)
	}
	check := func(child, parent *methodSpec, parentClass *class) error {
		lower := phpname.Lower(child.name)
		fail := func(msg string) error { return &vmFatal{message: msg, line: lines[lower]} }
		if parent.visibility == privateAccess && !parent.abstract {
			return nil
		}
		ctor := lower == "__construct"
		if parent.final {
			return fail("Cannot override final method " + parent.owner.name + "::" + parent.name + "()")
		}
		if parent.static != child.static {
			if parent.static {
				return fail("Cannot make static method " + parent.owner.name + "::" + parent.name + "() non static in class " + d.name)
			}
			return fail("Cannot make non static method " + parent.owner.name + "::" + parent.name + "() static in class " + d.name)
		}
		if child.abstract && !parent.abstract {
			return fail("Cannot make non abstract method " + parent.owner.name + "::" + parent.name + "() abstract in class " + d.name)
		}
		if child.visibility > parent.visibility {
			weaker := " or weaker"
			if parent.visibility == publicAccess {
				weaker = ""
			}
			return fail("Access level to " + d.name + "::" + child.name + "() must be " + accessName(parent.visibility) + " (as in class " + parent.owner.name + ")" + weaker)
		}
		// A constructor is only bound by an abstract or interface declaration.
		if ctor && !parent.abstract {
			return nil
		}
		if parent.fn == 0 && parent.tentative && child.fn > 0 {
			// An internal method with a tentative return type
			// (Iterator, IteratorAggregate): it takes no parameter, and a
			// return type that is missing or not compatible is deprecated.
			cf := &s.prog.funcs[child.fn-1]
			proto := parent.owner.name + "::" + parent.name + "(): " + parent.returnType
			if cf.required > 0 {
				return fail("Declaration of " + signature(child) + " must be compatible with " + proto)
			}
			if cf.returnType == "" || !s.subtype(cf.returnType, self, parent.returnType, parentClass) {
				if decls[lower]&bytecode.MemberReturnTypeWillChange == 0 {
					s.warnAt(levelDeprecated, "Return type of "+signature(child)+" should either be compatible with "+proto+", or the #[\\ReturnTypeWillChange] attribute should be used to temporarily suppress the notice", lines[lower])
					if s.stopped() {
						return nil
					}
				}
			}
			return nil
		}
		if !s.signatureCompatible(child, self, parent, parentClass) {
			return fail("Declaration of " + signature(child) + " must be compatible with " + signature(parent))
		}
		return nil
	}
	// inherited collects the methods the class gets from its parent and
	// interfaces, in PHP's order, to find the abstract ones left.
	type inheritedMethod struct {
		m    *methodSpec
		from *class
	}
	var inherited []inheritedMethod
	seen := map[string]bool{}
	visit := func(c *class) error {
		if c == nil || c.schema == nil {
			return nil
		}
		names := methodNames(c)
		for _, lower := range names {
			pm := c.schema.methods[lower]
			if pm == nil {
				continue
			}
			if child := own[lower]; child != nil {
				if err := check(child, pm, c); err != nil {
					return err
				}
				continue
			}
			if !seen[lower] {
				seen[lower] = true
				inherited = append(inherited, inheritedMethod{pm, c})
			}
		}
		return nil
	}
	if err := visit(spec.parent); err != nil {
		return err
	}
	for _, in := range spec.interfaces {
		if err := visit(in); err != nil {
			return err
		}
	}
	// A method the class declares must match the abstract method of the same
	// name in a trait it uses (zend_add_trait_method).
	for _, tr := range spec.traits {
		if tr.schema == nil {
			continue
		}
		for _, lower := range methodNames(tr) {
			tm := tr.schema.methods[lower]
			child := own[lower]
			if tm == nil || !tm.abstract || child == nil {
				continue
			}
			if _, declared := vc.sigs[lower]; !declared {
				continue // the method comes from a trait itself
			}
			if tm.static != child.static {
				if tm.static {
					return &vmFatal{message: "Cannot make static method " + tr.name + "::" + tm.name + "() non static in class " + self.name, line: lines[lower]}
				}
				return &vmFatal{message: "Cannot make non static method " + tr.name + "::" + tm.name + "() static in class " + self.name, line: lines[lower]}
			}
			// self and parent in the trait's signature mean the using class
			// (fixup_trait_scope).
			if !s.signatureCompatible(child, self, tm, self) {
				return &vmFatal{message: "Declaration of " + signature(child) + " must be compatible with " + methodSignatureText(tr, tm), line: lines[lower]}
			}
		}
	}
	// An inherited concrete method must satisfy the interfaces too.
	if spec.parent != nil && spec.parent.schema != nil {
		for _, in := range spec.interfaces {
			if in.schema == nil {
				continue
			}
			for _, lower := range methodNames(in) {
				im := in.schema.methods[lower]
				pm := spec.parent.schema.methods[lower]
				if im == nil || pm == nil || own[lower] != nil || pm.abstract || pm == im {
					continue
				}
				if pm.static != im.static || !s.signatureCompatible(pm, spec.parent, im, in) {
					return &vmFatal{message: "Declaration of " + methodSignatureText(pm.owner, pm) + " must be compatible with " + methodSignatureText(im.owner, im)}
				}
			}
		}
	}
	if !spec.isInterface && !spec.isTrait {
		// zend_implement_traversable, zend_implement_aggregate and
		// zend_implement_iterator.
		it, agg := classIs(self, iteratorBuiltin), classIs(self, iteratorAggregateBuiltin)
		if it && agg {
			return &vmFatal{message: "Class " + d.name + " cannot implement both Iterator and IteratorAggregate at the same time"}
		}
		if !spec.abstract && !it && !agg && classIs(self, traversableBuiltin) {
			return &vmFatal{message: "Class " + d.name + " must implement interface Traversable as part of either Iterator or IteratorAggregate", unknown: true}
		}
	}
	if !spec.abstract && !spec.isInterface && !spec.isTrait {
		var left []string
		for i := range spec.methods {
			if m := &spec.methods[i]; m.abstract {
				left = append(left, d.name+"::"+m.name)
			}
		}
		for _, im := range inherited {
			if !im.m.abstract {
				continue
			}
			lower := phpname.Lower(im.m.name)
			// A concrete method inherited from the parent implements it.
			if spec.parent != nil && spec.parent.schema != nil {
				if pm := spec.parent.schema.methods[lower]; pm != nil && !pm.abstract {
					continue
				}
			}
			left = append(left, im.m.owner.name+"::"+im.m.name)
		}
		if n := len(left); n > 0 {
			plural := ""
			if n > 1 {
				plural = "s"
			}
			shown := left
			more := ""
			if n > 3 {
				shown, more = left[:3], ", ..."
			}
			return &vmFatal{message: "Class " + d.name + " contains " + strconv.Itoa(n) + " abstract method" + plural + " and must therefore be declared abstract or implement the remaining method" + plural + " (" + strings.Join(shown, ", ") + more + ")"}
		}
	}
	return s.linkConstants(d, spec)
}

// methodNames lists the lower-case method names of a declared class or of
// an internal class whose order PHP shows in declaration order, and of any
// other class in sorted order.
func methodNames(c *class) []string {
	if order, ok := builtinMethodOrder[c]; ok {
		return append([]string(nil), order...)
	}
	if c.vm != nil && len(c.vm.methodOrder) > 0 {
		names := append([]string(nil), c.vm.methodOrder...)
		have := make(map[string]bool, len(names))
		for _, n := range names {
			have[n] = true
		}
		var rest []string
		for n := range c.schema.methods {
			if !have[n] {
				rest = append(rest, n)
			}
		}
		sortStrings(rest)
		return append(rest, names...)
	}
	names := make([]string, 0, len(c.schema.methods))
	for n := range c.schema.methods {
		names = append(names, n)
	}
	sortStrings(names)
	return names
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// linkConstants checks the class's constants against the ones it inherits
// (do_inherit_constant_check, do_inherit_iface_constant).
func (s *vmState) linkConstants(d *vmClassDecl, spec *classSpec) error {
	own := make(map[string]*vmConstDecl, len(d.consts))
	for i := range d.consts {
		own[d.consts[i].name] = &d.consts[i]
	}
	sources := make([]*class, 0, 1+len(spec.interfaces))
	if spec.parent != nil {
		sources = append(sources, spec.parent)
	}
	sources = append(sources, spec.interfaces...)
	from := map[string]*vmClassConst{}
	for _, src := range sources {
		if src.vm == nil {
			continue
		}
		for name, k := range src.vm.consts {
			if mine := own[name]; mine != nil {
				if k.decl.flags&bytecode.MemberFinal != 0 {
					return &vmFatal{message: d.name + "::" + name + " cannot override final constant " + k.owner.class.name + "::" + name}
				}
				if pv, cv := accessOf(k.decl.flags), accessOf(mine.flags); cv > pv && pv != privateAccess {
					weaker := " or weaker"
					if pv == publicAccess {
						weaker = ""
					}
					return &vmFatal{message: "Access level to " + d.name + "::" + name + " must be " + accessName(pv) + " (as in class " + k.owner.class.name + ")" + weaker}
				}
				continue
			}
			if other := from[name]; other != nil && other != k && other.owner != k.owner &&
				accessOf(other.decl.flags) != privateAccess && accessOf(k.decl.flags) != privateAccess {
				// A private constant of the parent is not inherited, so it
				// cannot clash with an interface's.
				kind := "Class"
				if spec.isInterface {
					kind = "Interface"
				}
				return &vmFatal{message: kind + " " + d.name + " inherits both " + other.owner.class.name + "::" + name + " and " + k.owner.class.name + "::" + name + ", which is ambiguous"}
			}
			from[name] = k
		}
	}
	return nil
}
