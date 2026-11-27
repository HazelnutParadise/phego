package runtime

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. The
// class table of a program and the classes one run builds from it. A
// vmClassDecl is immutable program data. A vmClass, the W-13 class it wraps,
// its constants' values and its static properties belong to one run: they
// are built when the declaration executes (or before {main} for a class PHP
// binds early) and released with the run. Object semantics are W-13's; this
// file only resolves names, applies PHP's declaration rules and hands W-13
// the descriptions.

type vmMemberValue struct {
	// has says the member has a value; init is the index of the function
	// that computes it, or -1 when snapshot holds it (exportSnapshot's
	// shape, shared and read-only).
	has      bool
	init     int
	snapshot any
}

type vmConstDecl struct {
	name  string
	flags bytecode.MemberFlags
	value vmMemberValue
}

type vmPropDecl struct {
	name, typ string
	flags     bytecode.MemberFlags
	value     vmMemberValue
}

type vmMethodDecl struct {
	name, signature string
	fn              int
	flags           bytecode.MemberFlags
	line            int
}

type vmTraitRule struct {
	trait, method, alias string
	flags                bytecode.MemberFlags
	hasVisibility        bool
	insteadof            []string
	line                 int
}

type vmClassDecl struct {
	index              int
	name, parent       string
	flags              bytecode.ClassFlags
	line               int
	file               string
	interfaces, traits []string
	rules              []vmTraitRule
	consts             []vmConstDecl
	props              []vmPropDecl
	methods            []vmMethodDecl
}

// decodeClasses copies the class table out of the bytecode program. Every
// index was checked by the verifier; it is checked again here so that the
// interpreter never depends on that.
func (prog *Program) decodeClasses(p *bytecode.Program) error {
	n := p.ClassCount()
	if n == 0 {
		return nil
	}
	file := ""
	if p.SourceCount() > 0 {
		src, _ := p.Source(0)
		file = src.Name
	}
	str := func(i uint32) (string, error) {
		if int(i) >= len(prog.consts) || prog.consts[i].kind != valueString {
			return "", fmt.Errorf("NewProgram: class table names constant %d, which is not a string", i)
		}
		return prog.consts[i].text, nil
	}
	opt := func(i uint32) (string, error) {
		if i == 0 {
			return "", nil
		}
		return str(i - 1)
	}
	list := func(in []uint32) ([]string, error) {
		out := make([]string, len(in))
		for i, c := range in {
			var err error
			if out[i], err = str(c); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	member := func(value, init uint32) (vmMemberValue, error) {
		switch {
		case init != 0:
			if int(init-1) >= len(prog.funcs) || int(init-1) == prog.entry {
				return vmMemberValue{}, fmt.Errorf("NewProgram: class table names function %d out of range", init-1)
			}
			if i := int(init - 1); i >= 0 && i < len(prog.memberFunc) {
				prog.memberFunc[i] = true
			}
			return vmMemberValue{has: true, init: int(init - 1)}, nil
		case value != 0:
			if int(value-1) >= len(prog.consts) {
				return vmMemberValue{}, fmt.Errorf("NewProgram: class table names constant %d out of range", value-1)
			}
			snap, err := exportSnapshot(prog.consts[value-1])
			return vmMemberValue{has: true, init: -1, snapshot: snap}, err
		}
		return vmMemberValue{init: -1}, nil
	}
	prog.classes = make([]vmClassDecl, n)
	for i := range prog.classes {
		c, _ := p.Class(uint32(i))
		d := &prog.classes[i]
		d.index, d.flags, d.line, d.file = i, c.Flags, int(c.Line), file
		var err error
		if d.name, err = str(c.Name); err != nil {
			return err
		}
		if d.parent, err = opt(c.Parent); err != nil {
			return err
		}
		if d.interfaces, err = list(c.Interfaces); err != nil {
			return err
		}
		if d.traits, err = list(c.Traits); err != nil {
			return err
		}
		for _, r := range c.Rules {
			out := vmTraitRule{flags: r.Flags, hasVisibility: r.HasVisibility, line: int(r.Line)}
			if out.trait, err = opt(r.Trait); err != nil {
				return err
			}
			if out.method, err = str(r.Method); err != nil {
				return err
			}
			if out.alias, err = opt(r.Alias); err != nil {
				return err
			}
			if out.insteadof, err = list(r.Insteadof); err != nil {
				return err
			}
			d.rules = append(d.rules, out)
		}
		for _, k := range c.Constants {
			out := vmConstDecl{flags: k.Flags}
			if out.name, err = str(k.Name); err != nil {
				return err
			}
			if out.value, err = member(k.Value, k.Init); err != nil {
				return err
			}
			d.consts = append(d.consts, out)
		}
		for _, pr := range c.Properties {
			out := vmPropDecl{flags: pr.Flags}
			if out.name, err = str(pr.Name); err != nil {
				return err
			}
			if out.typ, err = opt(pr.Type); err != nil {
				return err
			}
			if out.value, err = member(pr.Value, pr.Init); err != nil {
				return err
			}
			d.props = append(d.props, out)
		}
		for _, m := range c.Methods {
			out := vmMethodDecl{flags: m.Flags, line: int(m.Line), fn: int(m.Function)}
			if out.fn >= 0 && out.fn < len(prog.memberFunc) {
				prog.memberFunc[out.fn] = true
			}
			if out.name, err = str(m.Name); err != nil {
				return err
			}
			if out.signature, err = opt(m.Signature); err != nil {
				return err
			}
			if out.fn >= len(prog.funcs) || out.fn == prog.entry {
				return fmt.Errorf("NewProgram: class table names function %d out of range", out.fn)
			}
			d.methods = append(d.methods, out)
		}
		if d.flags&bytecode.ClassHoisted != 0 {
			prog.hoisted = append(prog.hoisted, i)
		}
		if d.flags&bytecode.ClassTrait != 0 {
			if prog.fnTrait == nil {
				prog.fnTrait = make([]bool, len(prog.funcs))
			}
			for _, m := range d.methods {
				prog.fnTrait[m.fn] = true
			}
		}
	}
	return nil
}

// vmClass is a class of one run.
type vmClass struct {
	decl  *vmClassDecl
	class *class
	// consts holds the class's constants by name, inherited ones included;
	// an inherited constant shares its parent's record, so it is evaluated
	// once.
	consts map[string]*vmClassConst
	// ready says the default values of the properties were computed
	// (zend_update_class_constants), which PHP does when the class is first
	// instantiated or one of its static properties is first used.
	ready bool
	// defaults are the evaluated constant-expression defaults of the
	// properties the class declares, and methodOrder its methods' lower
	// case names in declaration order.
	defaults    map[string]any
	methodOrder []string
	// sigs holds, per lower-case method name, the declaration PHP prints
	// after "Class::" in inheritance errors.
	sigs map[string]string
}

type vmClassConst struct {
	owner *vmClass
	decl  *vmConstDecl
	value value
	state uint8 // 0 not evaluated, 1 being evaluated, 2 done
}

// vmFatal is a PHP fatal error that is not a Throwable (E_ERROR or
// E_COMPILE_ERROR raised while a class is declared).
type vmFatal struct {
	message string
	// line, when positive, is the line PHP reports instead of the
	// declaring instruction's: that of the member at fault.
	line int
}

func (e *vmFatal) Error() string { return e.message }

func accessOf(f bytecode.MemberFlags) access {
	switch f.Visibility() {
	case bytecode.MemberProtected:
		return protectedAccess
	case bytecode.MemberPrivate:
		return privateAccess
	}
	return publicAccess
}

// lookupClass finds a declared class by name (without a leading backslash).
func (s *vmState) lookupClass(name string) *class {
	if len(name) > 0 && name[0] == '\\' {
		name = name[1:]
	}
	lower := phpname.Lower(name)
	if c := s.h.classes[lower]; c != nil {
		return c
	}
	switch lower {
	case "stdclass":
		if s.h.registerClass(stdClassBuiltin) == nil {
			return stdClassBuiltin
		}
	case "closure":
		if s.h.registerClass(closureBuiltin) == nil {
			return closureBuiltin
		}
	case "stringable":
		if s.h.registerClass(stringableBuiltin) == nil {
			return stringableBuiltin
		}
	}
	if c := throwableBuiltins[lower]; c != nil {
		// A class is registered with its parents; the interfaces come
		// along by name.
		for _, i := range [2]*class{stringableBuiltin, throwableBuiltin} {
			if s.h.classes[phpname.Lower(i.name)] == nil && s.h.registerClass(i) != nil {
				return nil
			}
		}
		if s.h.registerClass(c) == nil {
			s.throwables = true
			return c
		}
	}
	return nil
}

// declareHoisted declares the classes PHP binds while compiling the file.
func (s *vmState) declareHoisted() error {
	for _, i := range s.prog.hoisted {
		if err := s.declareClass(i); err != nil {
			return err
		}
	}
	return nil
}

func classKind(c *class) string {
	switch s := c.schema; {
	case s == nil:
		return "class"
	case s.isInterface:
		return "interface"
	case s.isTrait:
		return "trait"
	case s.enumKind != enumNone:
		return "enum"
	}
	return "class"
}

func declKind(d *vmClassDecl) string {
	switch {
	case d.flags&bytecode.ClassInterface != 0:
		return "interface"
	case d.flags&bytecode.ClassTrait != 0:
		return "trait"
	}
	return "class"
}

// declareClass is ZEND_DECLARE_CLASS: it links class index of the program
// against the classes that exist now and registers it.
func (s *vmState) declareClass(index int) error {
	d := &s.prog.classes[index]
	h := s.h
	key := phpname.Lower(d.name)
	if s.lookupClass(d.name) != nil && h.classes[key].vm == nil {
		// A class the engine provides (stdClass, Stringable).
		return &vmFatal{message: "Cannot redeclare " + classKind(h.classes[key]) + " " + h.classes[key].name}
	}
	if old := h.classes[key]; old != nil {
		where := ""
		if old.vm != nil {
			where = " (previously declared in " + vmFileName(old.vm.decl.file) + ":" + strconv.Itoa(old.vm.decl.line) + ")"
		}
		return &vmFatal{message: "Cannot redeclare " + classKind(old) + " " + old.name + where}
	}
	vc := &vmClass{decl: d, consts: map[string]*vmClassConst{}}
	spec := classSpec{
		name:         d.name,
		final:        d.flags&bytecode.ClassFinal != 0,
		abstract:     d.flags&bytecode.ClassAbstract != 0,
		isInterface:  d.flags&bytecode.ClassInterface != 0,
		isTrait:      d.flags&bytecode.ClassTrait != 0,
		readonly:     d.flags&bytecode.ClassReadonly != 0,
		allowDynamic: d.flags&bytecode.ClassAllowDynamic != 0,
	}
	if d.parent != "" {
		parent := s.lookupClass(d.parent)
		if parent == nil {
			return &Throwable{"Error", `Class "` + d.parent + `" not found`}
		}
		switch ps := parent.schema; {
		case ps != nil && ps.isInterface:
			return &vmFatal{message: "Class " + d.name + " cannot extend interface " + parent.name}
		case ps != nil && ps.isTrait:
			return &vmFatal{message: "Class " + d.name + " cannot extend trait " + parent.name}
		case ps != nil && ps.enumKind != enumNone:
			return &vmFatal{message: "Class " + d.name + " cannot extend enum " + parent.name}
		case ps != nil && ps.final:
			return &vmFatal{message: "Class " + d.name + " cannot extend final class " + parent.name}
		}
		spec.parent = parent
	}
	for _, name := range d.interfaces {
		c := s.lookupClass(name)
		if c == nil {
			return &Throwable{"Error", `Interface "` + name + `" not found`}
		}
		if c.schema == nil || !c.schema.isInterface {
			if spec.isInterface {
				return &vmFatal{message: d.name + " cannot implement " + c.name + " - it is not an interface"}
			}
			return &vmFatal{message: d.name + " cannot implement " + c.name + " - it is not an interface"}
		}
		spec.interfaces = append(spec.interfaces, c)
	}
	for _, name := range d.traits {
		c := s.lookupClass(name)
		if c == nil {
			return &Throwable{"Error", `Trait "` + name + `" not found`}
		}
		if c.schema == nil || !c.schema.isTrait {
			return &Throwable{"Error", d.name + " cannot use " + c.name + " - it is not a trait"}
		}
		spec.traits = append(spec.traits, c)
	}
	for i := range d.props {
		p := &d.props[i]
		ps := propertySpec{
			name:       p.name,
			typeName:   p.typ,
			visibility: accessOf(p.flags),
			static:     p.flags&bytecode.MemberStatic != 0,
			readonly:   p.flags&bytecode.MemberReadonly != 0,
		}
		ps.setVisibility = ps.visibility
		if p.value.has && p.value.init < 0 {
			ps.hasDefault, ps.initial = true, p.value.snapshot
		}
		spec.properties = append(spec.properties, ps)
	}
	methods, err := s.classMethods(vc, d, &spec)
	if err != nil {
		return err
	}
	spec.methods = methods
	for i := range methods {
		vc.methodOrder = append(vc.methodOrder, phpname.Lower(methods[i].name))
	}
	for i := range methods {
		if phpname.Lower(methods[i].name) == "__destruct" && methods[i].call != nil {
			call := methods[i].call
			spec.destructor = func(h *heap, this value) error {
				v, err := call(h, this, nil)
				if err != nil {
					return err
				}
				return h.discard(v)
			}
		}
	}
	if err := s.linkThrowable(d, &spec); err != nil {
		return err
	}
	if err := s.linkCheck(vc, d, &spec); err != nil {
		return err
	}
	c, err := buildClass(spec)
	if err != nil {
		var t *Throwable
		if errors.As(err, &t) {
			// buildClass reports PHP's inheritance errors as throwables;
			// PHP raises them as fatal errors.
			return &vmFatal{message: t.Message}
		}
		const conflict = "incompatible trait property " // + Class::$name
		if msg := err.Error(); strings.HasPrefix(msg, conflict) {
			// zend_do_traits_property_binding.
			prop := msg[strings.LastIndex(msg, "$")+1:]
			for _, tr := range spec.traits {
				if tr.vm != nil && tr.vm.propDecl(prop) != nil {
					return &vmFatal{message: d.name + " and " + tr.name + " define the same property ($" + prop + ") in the composition of " + d.name + ". However, the definition differs and is considered incompatible. Class was composed"}
				}
			}
		}
		return s.internal("class %s: %v", d.name, err)
	}
	c.vm, vc.class = vc, c
	// Constants: the parent's, then the interfaces' and traits', then the
	// class's own.
	inherit := func(from *class) {
		if from == nil || from.vm == nil {
			return
		}
		for name, k := range from.vm.consts {
			if accessOf(k.decl.flags) == privateAccess {
				continue // a private constant is not inherited
			}
			if _, own := vc.consts[name]; !own {
				vc.consts[name] = k
			}
		}
	}
	for i := range d.consts {
		k := &d.consts[i]
		vc.consts[k.name] = &vmClassConst{owner: vc, decl: k}
	}
	inherit(spec.parent)
	for _, in := range spec.interfaces {
		inherit(in)
	}
	for _, t := range spec.traits {
		if t.vm == nil {
			continue
		}
		// A trait's constants become the using class's own.
		for name, k := range t.vm.consts {
			if _, own := vc.consts[name]; !own {
				vc.consts[name] = &vmClassConst{owner: vc, decl: k.decl}
			}
		}
	}
	if err := h.registerClass(c); err != nil {
		return err
	}
	s.declared = append(s.declared, vc)
	return nil
}

// methodCall is the callback W-13 uses to run a declared method from the
// runtime itself: a destructor, a magic method, __toString. fn is the
// method's function and vc the class that declares or uses it.
func methodCall(vc *vmClass, fn int, static bool) func(*heap, value, []value) (value, error) {
	return func(h *heap, this value, args []value) (value, error) {
		s := h.vm
		if s == nil {
			return value{}, errors.New("method called outside its run")
		}
		called := h.currentCalledScope
		if this.kind == valueObject && this.object != nil {
			called = this.object.class
		}
		if called == nil {
			called = vc.class
		}
		if static {
			this = value{}
		}
		// The one declared parameter is variadic (see classMethods): W-13
		// hands over the arguments as an array.
		var list []value
		if len(args) > 0 && args[0].kind == valueArray {
			for _, b := range args[0].array.buckets {
				if b != nil {
					list = append(list, b.val)
				}
			}
		}
		v, err := s.invoke(&s.prog.funcs[fn], this, vc.class, called, list)
		if err != nil {
			return value{}, err
		}
		unown(v)
		return v, nil
	}
}

// unown turns an owned value into the unowned temporary W-13's callbacks
// return: the owner is given up without releasing the value.
func unown(v value) {
	switch v.kind {
	case valueArray:
		v.array.refcount--
	case valueObject, valueResource:
		v.object.refcount--
	case valueReference:
		v.ref.refcount--
	}
}

// passThrough is the parameter list W-13 sees for every declared method:
// the VM binds and checks the real parameters itself when it enters the
// function (ZEND_RECV), so the runtime only forwards what it was given.
var passThrough = []methodParameter{{name: "args", variadic: true}}

func (s *vmState) methodSpecOf(vc *vmClass, m *vmMethodDecl, name string, flags bytecode.MemberFlags) methodSpec {
	static := flags&bytecode.MemberStatic != 0
	ms := methodSpec{
		name:       name,
		visibility: accessOf(flags),
		static:     static,
		final:      flags&bytecode.MemberFinal != 0,
		abstract:   flags&bytecode.MemberAbstract != 0,
		parameters: passThrough,
		fn:         int32(m.fn + 1),
	}
	if !ms.abstract {
		ms.call = methodCall(vc, m.fn, static)
	}
	return ms
}

// classMethods is the class's method list for W-13: its own methods, then
// the methods its traits contribute after the insteadof and alias rules
// (zend_do_bind_traits). A class method always wins over a trait's.
func (s *vmState) classMethods(vc *vmClass, d *vmClassDecl, spec *classSpec) ([]methodSpec, error) {
	out := make([]methodSpec, 0, len(d.methods))
	own := make(map[string]bool, len(d.methods))
	for i := range d.methods {
		m := &d.methods[i]
		own[phpname.Lower(m.name)] = true
		out = append(out, s.methodSpecOf(vc, m, m.name, m.flags))
	}
	if len(spec.traits) == 0 {
		return out, nil
	}
	traitOf := func(name string) *class {
		c := s.lookupClass(name)
		for _, t := range spec.traits {
			if t == c {
				return t
			}
		}
		return nil
	}
	// zend_traits_init_trait_structures: every alias rule is checked before
	// any method is copied.
	for i := range d.rules {
		r := &d.rules[i]
		if len(r.insteadof) != 0 {
			continue
		}
		lower := phpname.Lower(r.method)
		fail := func(msg string) error { return &vmFatal{message: msg, line: d.line} }
		if r.trait != "" {
			t := traitOf(r.trait)
			if t == nil {
				if s.lookupClass(r.trait) == nil {
					return nil, fail("Could not find trait " + r.trait)
				}
				return nil, fail("Required Trait " + r.trait + " wasn't added to " + d.name)
			}
			if t.schema.methods[lower] == nil {
				return nil, fail("An alias was defined for " + t.name + "::" + r.method + " but this method does not exist")
			}
			continue
		}
		var have []*class
		for _, t := range spec.traits {
			if t.schema.methods[lower] != nil {
				have = append(have, t)
			}
		}
		switch {
		case len(have) == 0 && r.alias != "":
			return nil, fail("An alias (" + r.alias + ") was defined for method " + r.method + "(), but this method does not exist")
		case len(have) == 0:
			return nil, fail("The modifiers of the trait method " + r.method + "() are changed, but this method does not exist. Error")
		case len(have) > 1 && r.alias != "":
			return nil, fail("An alias was defined for method " + r.method + "(), which exists in both " + have[0].name + " and " + have[1].name + ". Use " + have[0].name + "::" + r.method + " or " + have[1].name + "::" + r.method + " to resolve the ambiguity")
		}
	}
	// excluded[trait][method]: an insteadof rule removed the method.
	excluded := map[*class]map[string]bool{}
	for i := range d.rules {
		r := &d.rules[i]
		if len(r.insteadof) == 0 {
			continue
		}
		winner := traitOf(r.trait)
		if winner == nil {
			if c := s.lookupClass(r.trait); c == nil {
				return nil, &vmFatal{message: `Could not find trait ` + r.trait}
			}
			return nil, &vmFatal{message: "Required Trait " + r.trait + " wasn't added to " + d.name}
		}
		lower := phpname.Lower(r.method)
		if winner.schema.methods[lower] == nil {
			return nil, &vmFatal{message: "A precedence rule was defined for " + winner.name + "::" + r.method + " but this method does not exist"}
		}
		for _, name := range r.insteadof {
			loser := traitOf(name)
			if loser == nil {
				if c := s.lookupClass(name); c == nil {
					return nil, &vmFatal{message: `Could not find trait ` + name}
				}
				return nil, &vmFatal{message: "Required Trait " + name + " wasn't added to " + d.name}
			}
			if loser == winner {
				return nil, &vmFatal{message: "Inconsistent insteadof definition. The method " + r.method + " is to be used from " + winner.name + ", but " + winner.name + " is also on the exclude list"}
			}
			if excluded[loser] == nil {
				excluded[loser] = map[string]bool{}
			}
			excluded[loser][lower] = true
		}
	}
	// from[name] is the trait whose method the class already took.
	from := map[string]*class{}
	add := func(t *class, src *methodSpec, name string, flags *access) error {
		lower := phpname.Lower(name)
		if own[lower] {
			return nil
		}
		if other := from[lower]; other != nil {
			if other == t {
				return nil
			}
			return &vmFatal{message: "Trait method " + t.name + "::" + name + " has not been applied as " + d.name + "::" + name + ", because of collision with " + other.name + "::" + name}
		}
		from[lower] = t
		ms := *src
		ms.name = name
		if flags != nil {
			ms.visibility = *flags
		}
		if ms.fn > 0 && !ms.abstract {
			ms.call = methodCall(vc, int(ms.fn-1), ms.static)
		}
		out = append(out, ms)
		return nil
	}
	for _, t := range spec.traits {
		names := make([]string, 0, len(t.schema.methods))
		// The trait's own declaration order, which is the order PHP copies
		// methods in.
		if t.vm != nil {
			seen := map[string]bool{}
			for _, m := range t.vm.methodOrder {
				if !seen[m] {
					seen[m] = true
					names = append(names, m)
				}
			}
		}
		for _, lower := range names {
			src := t.schema.methods[lower]
			if src == nil {
				continue
			}
			// Aliases first (zend_add_trait_method is called for each alias
			// before the method under its own name).
			for i := range d.rules {
				r := &d.rules[i]
				if len(r.insteadof) != 0 || r.alias == "" || phpname.Lower(r.method) != lower {
					continue
				}
				if r.trait != "" && traitOf(r.trait) != t {
					continue
				}
				var vis *access
				if r.hasVisibility {
					v := accessOf(r.flags)
					vis = &v
				}
				if err := add(t, src, r.alias, vis); err != nil {
					return nil, err
				}
			}
			if excluded[t][lower] {
				continue
			}
			var vis *access
			for i := range d.rules {
				r := &d.rules[i]
				if len(r.insteadof) == 0 && r.alias == "" && r.hasVisibility && phpname.Lower(r.method) == lower && (r.trait == "" || traitOf(r.trait) == t) {
					v := accessOf(r.flags)
					vis = &v
				}
			}
			if err := add(t, src, src.name, vis); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// prepareClass computes the property defaults that are constant
// expressions, for c and its ancestors, the first time the class is
// instantiated or its static properties are used. The class belongs to this
// run, so writing the evaluated defaults into its description is not
// visible to any other.
func (s *vmState) prepareClass(c *class) error {
	vc := c.vm
	if vc == nil || vc.ready || c.schema == nil {
		return nil
	}
	if p := c.schema.parent; p != nil {
		if err := s.prepareClass(p); err != nil {
			return err
		}
	}
	for i := range c.schema.props {
		p := &c.schema.props[i]
		if p.hasDefault || p.owner == nil || p.owner.vm == nil {
			continue
		}
		owner := p.owner.vm
		decl := owner.propDecl(p.name)
		if decl == nil {
			// A trait's property: the declaration is the trait's.
			for _, t := range p.owner.schema.traits {
				if t.vm != nil {
					if decl = t.vm.propDecl(p.name); decl != nil {
						break
					}
				}
			}
		}
		if decl == nil || !decl.value.has || decl.value.init < 0 {
			continue
		}
		snap, ok := owner.defaults[p.name]
		if !ok {
			v, err := s.invoke(&s.prog.funcs[decl.value.init], value{}, p.owner, p.owner, nil)
			if err != nil {
				return err
			}
			snap, err = exportSnapshot(v)
			if rerr := s.h.release(v); err == nil {
				err = rerr
			}
			if err != nil {
				return err
			}
			if owner.defaults == nil {
				owner.defaults = map[string]any{}
			}
			owner.defaults[p.name] = snap
		}
		if integer, isInt := snap.(int64); isInt && methodTypeHas(p.typeName, "float") && !methodTypeHas(p.typeName, "int") {
			snap = float64(integer)
		}
		p.hasDefault, p.initial = true, snap
	}
	vc.ready = true
	return nil
}

// classConstant is ZEND_FETCH_CLASS_CONSTANT: the value of constant name of
// c, evaluated in its declaring class the first time.
func (s *vmState) classConstant(c *class, name string, scope *class) (value, error) {
	var k *vmClassConst
	if c.vm != nil {
		k = c.vm.consts[name]
	}
	if k == nil {
		return value{}, &Throwable{"Error", "Undefined constant " + c.name + "::" + name}
	}
	if vis := accessOf(k.decl.flags); !canAccess(scope, k.owner.class, vis) {
		return value{}, &Throwable{"Error", "Cannot access " + accessName(vis) + " constant " + c.name + "::" + name}
	}
	switch k.state {
	case 2:
		return k.value, nil
	case 1:
		return value{}, &Throwable{"Error", "Cannot declare self-referencing constant " + k.owner.class.name + "::" + name}
	}
	if k.decl.value.init < 0 {
		v, err := s.h.importSnapshotPHP8512(k.decl.value.snapshot, 1024, 1<<20)
		if err != nil {
			return value{}, err
		}
		retain(v)
		k.value, k.state = v, 2
		return v, nil
	}
	k.state = 1
	v, err := s.invoke(&s.prog.funcs[k.decl.value.init], value{}, k.owner.class, k.owner.class, nil)
	if err != nil {
		k.state = 0
		return value{}, err
	}
	k.value, k.state = v, 2
	return v, nil
}

func (vc *vmClass) propDecl(name string) *vmPropDecl {
	for i := range vc.decl.props {
		if vc.decl.props[i].name == name {
			return &vc.decl.props[i]
		}
	}
	return nil
}

// releaseClasses releases the values the run's classes hold.
func (s *vmState) releaseClasses() {
	for _, vc := range s.declared {
		for name, k := range vc.consts {
			if k.owner == vc && k.state == 2 {
				v := k.value
				k.value, k.state = value{}, 0
				_ = s.releaseValue(v)
			}
			delete(vc.consts, name)
		}
	}
	s.declared = nil
}
