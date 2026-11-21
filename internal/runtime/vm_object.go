package runtime

import (
	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. The
// format 1.7 member instructions. Each one resolves its operands and calls
// the W-13 operation that owns the PHP semantics; the comments name the
// Zend handler whose order of checks the instruction follows.

// classOperand resolves the class operand of an instruction
// (zend_fetch_class and the CONST operand's cache slot).
func (s *vmState) classOperand(f *vmFrame, in *vmInstr, operand uint32) (*class, error) {
	switch bytecode.AuxClassRef(in.aux) {
	case bytecode.ClassNamed:
		if s.classCache == nil {
			s.classCache = make([]*class, len(s.prog.consts))
		}
		if c := s.classCache[operand]; c != nil {
			return c, nil
		}
		name := s.prog.consts[operand].text
		c := s.lookupClass(name)
		if c == nil {
			return nil, &Throwable{"Error", `Class "` + name + `" not found`}
		}
		s.classCache[operand] = c
		return c, nil
	case bytecode.ClassSelf:
		if f.scope == nil {
			return nil, &Throwable{"Error", `Cannot use "self" when no class scope is active`}
		}
		return f.scope, nil
	case bytecode.ClassParent:
		if f.scope == nil {
			return nil, &Throwable{"Error", `Cannot use "parent" when no class scope is active`}
		}
		if f.scope.schema == nil || f.scope.schema.parent == nil {
			return nil, &Throwable{"Error", `Cannot use "parent" when current class scope has no parent`}
		}
		return f.scope.schema.parent, nil
	case bytecode.ClassStatic:
		if f.called == nil {
			return nil, &Throwable{"Error", `Cannot use "static" when no class scope is active`}
		}
		return f.called, nil
	}
	switch v := f.regs[operand].deref(); v.kind {
	case valueObject:
		return v.object.class, nil
	case valueString:
		c := s.lookupClass(v.text)
		if c == nil {
			return nil, &Throwable{"Error", `Class "` + v.text + `" not found`}
		}
		return c, nil
	}
	return nil, &Throwable{"Error", "Class name must be a valid object or a string"}
}

// memberName is the property name operand: a constant, or a value
// converted to string as ZEND_FETCH_OBJ_* does for a non-constant name.
func (s *vmState) memberName(f *vmFrame, in *vmInstr, operand uint32) (string, error) {
	if in.aux&bytecode.MemberNameReg == 0 {
		return s.prog.consts[operand].text, nil
	}
	v := f.regs[operand].deref()
	if v.kind == valueString {
		return v.text, nil
	}
	return s.h.toString(v)
}

// objectOperand is the object operand: a Value register, or the element a
// FetchDim or FetchProp left in a Location (MemberObjLocation).
func (s *vmState) objectOperand(f *vmFrame, in *vmInstr, operand uint32) (value, error) {
	if in.aux&bytecode.MemberObjLocation == 0 {
		v := f.regs[operand].deref()
		if v.kind == valueUndefined {
			v = nullValue()
		}
		return v, nil
	}
	slot, err := s.elementSlot(f, operand)
	if err != nil {
		return value{}, err
	}
	v := slot.deref()
	if v.kind == valueUndefined {
		v = nullValue()
	}
	return v, nil
}

// this is ZEND_FETCH_THIS.
func (s *vmState) this(f *vmFrame, in *vmInstr) error {
	if f.this.kind != valueObject {
		return &Throwable{"Error", "Using $this when not in object context"}
	}
	return s.setReg(f.regs, in.a, f.this)
}

// propGet is ZEND_FETCH_OBJ_R and, with MemberQuiet, ZEND_FETCH_OBJ_IS. The
// modes above MemberQuiet are the read half of a compound assignment and of
// ++/--, which throw for a container that is not an object.
func (s *vmState) propGet(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	mode := bytecode.AuxMemberMode(in.aux)
	if obj.kind != valueObject {
		switch mode {
		case bytecode.MemberRead:
			s.h.warn(levelWarning, `Attempt to read property "`+name+`" on `+valueName(obj))
		case bytecode.MemberAssignOp:
			return &Throwable{"Error", `Attempt to assign property "` + name + `" on ` + valueName(obj)}
		case bytecode.MemberIncDec:
			return &Throwable{"Error", `Attempt to increment/decrement property "` + name + `" on ` + valueName(obj)}
		}
		return s.setReg(f.regs, in.a, nullValue())
	}
	if mode == bytecode.MemberQuiet {
		// read_property(BP_VAR_IS): __isset decides whether __get runs,
		// and nothing warns.
		set, err := s.h.issetObject(obj, name, f.scope)
		if err != nil {
			return err
		}
		if !set {
			return s.setReg(f.regs, in.a, nullValue())
		}
	}
	v, err := s.h.readObject(obj, name, f.scope)
	if err != nil {
		return err
	}
	return s.setReg(f.regs, in.a, v)
}

// propSet is ZEND_ASSIGN_OBJ. With MemberResult the value register then
// holds the assignment's value: what a typed property stored.
func (s *vmState) propSet(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.a)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.b)
	if err != nil {
		return err
	}
	v := f.regs[in.c].deref()
	if v.kind == valueUndefined {
		v = nullValue()
	}
	if obj.kind != valueObject {
		return &Throwable{"Error", `Attempt to assign property "` + name + `" on ` + valueName(obj)}
	}
	file, line := s.where()
	if err := s.h.writeObjectAt(obj, name, v, f.scope, f.fn.strict, methodCallSite{file: file, line: line}); err != nil {
		return err
	}
	if in.aux&bytecode.MemberResult == 0 {
		return nil
	}
	// The stored value, when the property is a declared one with a type.
	if p, perr := resolvePropertyIn(obj.object.class, name, f.scope, true); perr == nil && p != nil && p.typeName != "" && !p.static && !p.virtual && p.set == nil {
		if slot, ok := obj.object.props.lookup(p.key); ok && slot.kind != valueUndefined {
			return s.setReg(f.regs, in.c, slot.deref())
		}
	}
	return nil
}

// fetchProp is ZEND_FETCH_OBJ_W, _RW and _UNSET: A names the property
// itself, for the element or property operation that follows.
func (s *vmState) fetchProp(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	mode := bytecode.AuxMemberMode(in.aux)
	scratch := func(v value) error {
		old := s.fetchScratch
		if v.kind >= valueArray {
			retain(v)
		}
		s.fetchScratch = v
		if err := s.releaseValue(old); err != nil {
			return err
		}
		return s.setElementLocation(f, in.a, &s.fetchScratch)
	}
	if obj.kind != valueObject {
		if mode == bytecode.MemberUnset {
			return scratch(nullValue())
		}
		return &Throwable{"Error", `Attempt to modify property "` + name + `" on ` + valueName(obj)}
	}
	o := obj.object
	if mode == bytecode.MemberUnset {
		// Nothing is created for an unset below a missing property.
		p, perr := resolvePropertyIn(o.class, name, f.scope, true)
		key := name
		if perr == nil && p != nil {
			key = p.key
		}
		if slot, ok := o.props.lookup(key); perr != nil || !ok || slot.kind == valueUndefined {
			return scratch(nullValue())
		}
	}
	if p, perr := resolvePropertyIn(o.class, name, f.scope, true); perr == nil && p != nil && p.readonly && !p.static {
		if slot, ok := o.props.lookup(p.key); ok && slot.kind != valueUndefined && slot.deref().kind != valueObject {
			return &Throwable{"Error", "Cannot modify readonly property " + p.label()}
		}
	}
	address, err := s.h.resolveObjectReference(o, name, f.scope)
	if err != nil {
		return err
	}
	if address.slot != nil {
		if mode == bytecode.MemberReadWrite && address.slot.kind == valueUndefined {
			s.h.warn(levelWarning, "Undefined property: "+o.class.name+"::$"+name)
		}
		return s.setElementLocation(f, in.a, address.slot)
	}
	return scratch(address.raw.deref())
}

// propRef is the property side of ZEND_ASSIGN_OBJ_REF and of every other
// place a reference to a property is taken: A receives the reference.
func (s *vmState) propRef(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	if obj.kind != valueObject {
		return &Throwable{"Error", `Attempt to modify property "` + name + `" on ` + valueName(obj)}
	}
	// The object is held across the register's release: the register may
	// be its last owner's operand.
	retain(obj)
	err = s.clearReg(f, in.a)
	if err == nil {
		err = s.h.referenceObject(&f.regs[in.a], obj, name, f.scope)
	}
	if rerr := s.releaseValue(obj); err == nil {
		err = rerr
	}
	return err
}

// propIsset is ZEND_ISSET_ISEMPTY_PROP_OBJ.
func (s *vmState) propIsset(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	empty := bytecode.AuxMemberMode(in.aux) == bytecode.MemberEmpty
	if obj.kind != valueObject {
		return s.setReg(f.regs, in.a, boolValue(empty))
	}
	set, err := s.h.issetObject(obj, name, f.scope)
	if err != nil {
		return err
	}
	if !empty || !set {
		return s.setReg(f.regs, in.a, boolValue(set != empty))
	}
	v, err := s.h.readObject(obj, name, f.scope)
	if err != nil {
		return err
	}
	truthy := s.h.toBool(v)
	if derr := s.h.discard(v); derr != nil {
		return derr
	}
	return s.setReg(f.regs, in.a, boolValue(!truthy))
}

// propUnset is ZEND_UNSET_OBJ.
func (s *vmState) propUnset(f *vmFrame, in *vmInstr) error {
	obj, err := s.objectOperand(f, in, in.a)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.b)
	if err != nil {
		return err
	}
	if obj.kind != valueObject {
		return nil
	}
	return s.h.unsetObject(obj, name, f.scope)
}

// staticClass resolves the class of a static property access and computes
// its default values on first use.
func (s *vmState) staticClass(f *vmFrame, in *vmInstr, operand uint32) (*class, error) {
	c, err := s.classOperand(f, in, operand)
	if err != nil {
		return nil, err
	}
	return c, s.prepareClass(c)
}

// staticGet is ZEND_FETCH_STATIC_PROP_R and _IS, and
// ZEND_ISSET_ISEMPTY_STATIC_PROP for the bool modes.
func (s *vmState) staticGet(f *vmFrame, in *vmInstr) error {
	mode := bytecode.AuxMemberMode(in.aux)
	c, err := s.staticClass(f, in, in.b)
	if err != nil {
		if mode == bytecode.MemberRead {
			return err
		}
		// isset(Missing::$x) is false; so is the property of a class whose
		// defaults cannot be computed.
		if _, php := err.(*Throwable); !php {
			return err
		}
		if mode == bytecode.MemberQuiet {
			return err
		}
		return s.setReg(f.regs, in.a, boolValue(mode == bytecode.MemberEmpty))
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	if mode == bytecode.MemberRead {
		v, err := s.h.readStatic(c, name, f.scope)
		if err != nil {
			return err
		}
		return s.setReg(f.regs, in.a, v)
	}
	v := nullValue()
	if p, perr := s.h.resolveStatic(c, name, f.scope); perr == nil {
		tbl, err := s.h.staticTable(p.owner)
		if err != nil {
			return err
		}
		if slot, ok := tbl.lookup(p.key); ok && slot.kind != valueUndefined {
			v = slot.deref()
		}
	} else if mode == bytecode.MemberQuiet {
		return perr
	}
	switch mode {
	case bytecode.MemberIsset:
		v = boolValue(v.kind != valueNull)
	case bytecode.MemberEmpty:
		v = boolValue(!s.h.toBool(v))
	}
	return s.setReg(f.regs, in.a, v)
}

// staticSet is ZEND_ASSIGN_STATIC_PROP.
func (s *vmState) staticSet(f *vmFrame, in *vmInstr) error {
	c, err := s.staticClass(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	v := f.regs[in.a].deref()
	if v.kind == valueUndefined {
		v = nullValue()
	}
	if err := s.h.writeStatic(c, name, v, f.scope, f.fn.strict); err != nil {
		return err
	}
	if in.aux&bytecode.MemberResult == 0 {
		return nil
	}
	out, err := s.h.readStatic(c, name, f.scope)
	if err != nil {
		return err
	}
	return s.setReg(f.regs, in.a, out)
}

// fetchStatic is ZEND_FETCH_STATIC_PROP_W and _RW: A names the property.
func (s *vmState) fetchStatic(f *vmFrame, in *vmInstr) error {
	c, err := s.staticClass(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	h := s.h
	p, err := h.resolveStatic(c, name, f.scope)
	if err != nil {
		return err
	}
	if !canAccess(f.scope, p.owner, p.setVisibility) {
		return &Throwable{"Error", "Cannot indirectly modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + h.calledScopeLabel(f.scope)}
	}
	tbl, err := h.staticTable(p.owner)
	if err != nil {
		return err
	}
	slot, ok := tbl.lookup(p.key)
	if !ok {
		slot = tbl.slot(p.key)
	}
	if slot.kind == valueUndefined && p.typeName != "" {
		return &Throwable{"Error", "Typed static property " + p.label() + " must not be accessed before initialization"}
	}
	return s.setElementLocation(f, in.a, slot)
}

// staticRef is the static property side of ZEND_ASSIGN_STATIC_PROP_REF.
func (s *vmState) staticRef(f *vmFrame, in *vmInstr) error {
	c, err := s.staticClass(f, in, in.b)
	if err != nil {
		return err
	}
	name, err := s.memberName(f, in, in.c)
	if err != nil {
		return err
	}
	if err := s.clearReg(f, in.a); err != nil {
		return err
	}
	return s.h.referenceStatic(&f.regs[in.a], c, name, f.scope)
}

// classConst is ZEND_FETCH_CLASS_CONSTANT.
func (s *vmState) classConst(f *vmFrame, in *vmInstr) error {
	c, err := s.classOperand(f, in, in.b)
	if err != nil {
		return err
	}
	v, err := s.classConstant(c, s.prog.consts[in.c].text, f.scope)
	if err != nil {
		return err
	}
	return s.setReg(f.regs, in.a, v)
}

// className is ZEND_FETCH_CLASS_NAME: static::class, self::class in a
// trait, and $object::class.
func (s *vmState) className(f *vmFrame, in *vmInstr) error {
	if bytecode.AuxClassRef(in.aux) == bytecode.ClassValue {
		v := f.regs[in.b].deref()
		if v.kind != valueObject {
			if v.kind == valueUndefined {
				v = nullValue()
			}
			return &Throwable{"TypeError", `Cannot use "::class" on ` + valueName(v)}
		}
		return s.setReg(f.regs, in.a, stringValue(v.object.class.name))
	}
	c, err := s.classOperand(f, in, in.b)
	if err != nil {
		return err
	}
	return s.setReg(f.regs, in.a, stringValue(c.name))
}

// instanceOf is ZEND_INSTANCEOF: a class that does not exist is no error.
func (s *vmState) instanceOf(f *vmFrame, in *vmInstr) error {
	v := f.regs[in.b].deref()
	var c *class
	switch bytecode.AuxClassRef(in.aux) {
	case bytecode.ClassNamed:
		if s.classCache != nil {
			c = s.classCache[in.c]
		}
		if c == nil {
			c = s.lookupClass(s.prog.consts[in.c].text)
		}
	case bytecode.ClassValue:
		switch cv := f.regs[in.c].deref(); cv.kind {
		case valueObject:
			c = cv.object.class
		case valueString:
			c = s.lookupClass(cv.text)
		default:
			return &Throwable{"Error", "Class name must be a valid object or a string"}
		}
	default:
		var err error
		if c, err = s.classOperand(f, in, in.c); err != nil {
			return err
		}
	}
	return s.setReg(f.regs, in.a, boolValue(c != nil && v.kind == valueObject && classIs(v.object.class, c)))
}

// clone is ZEND_CLONE.
func (s *vmState) clone(f *vmFrame, in *vmInstr) error {
	v := f.regs[in.b].deref()
	if v.kind != valueObject {
		if v.kind == valueUndefined {
			v = nullValue()
		}
		return &Throwable{"TypeError", "clone(): Argument #1 ($object) must be of type object, " + valueName(v) + " given"}
	}
	out, err := s.h.cloneObject(v, f.scope)
	if err != nil {
		return err
	}
	return s.setReg(f.regs, in.a, out)
}
