package runtime

import (
	"strconv"
	"strings"

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
	if f.this == nil {
		return &Throwable{"Error", "Using $this when not in object context"}
	}
	return s.setReg(f.regs, in.a, value{kind: valueObject, object: f.this})
}

// propGet is ZEND_FETCH_OBJ_R and, with MemberQuiet, ZEND_FETCH_OBJ_IS. The
// modes above MemberQuiet are the read half of a compound assignment and of
// ++/--, which throw for a container that is not an object.
func (s *vmState) propGet(f *vmFrame, in *vmInstr) error {
	if in.cache != 0 {
		// The fast path: a declared property this instruction resolved
		// before, holding a value.
		if obj := f.regs[in.b]; obj.kind == valueObject {
			if c := s.cacheOf(in); c.readable && c.class == obj.object.class && c.scope == f.scope && obj.object.lazy == nil {
				if v := obj.object.props.declared[c.slot]; v.kind != valueUndefined {
					if v.kind == valueReference {
						v = v.ref.target
					}
					if old := f.regs[in.a]; v.kind < valueArray && old.kind < valueArray {
						f.regs[in.a] = v
						return nil
					}
					return s.setReg(f.regs, in.a, v)
				}
			}
		}
	}
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
	c := obj.object.class
	switch mode {
	case bytecode.MemberQuiet:
		// zend_std_read_property(BP_VAR_IS): a class with __isset asks it
		// before __get; one with only __get reads through it; nothing
		// warns about a property that does not exist.
		_, hasIsset := s.h.resolveMethod(c, "__isset", nil)
		_, hasGet := s.h.resolveMethod(c, "__get", nil)
		if hasIsset || !hasGet {
			set, err := s.h.issetObject(obj, name, f.scope)
			if err != nil {
				return err
			}
			if !set {
				return s.setReg(f.regs, in.a, nullValue())
			}
		}
	case bytecode.MemberAssignOp, bytecode.MemberIncDec:
		// zend_std_get_property_ptr_ptr creates a property that does not
		// exist (with the dynamic property deprecation) before the read
		// half of the operation warns that it is undefined.
		if _, hasGet := s.h.resolveMethod(c, "__get", nil); !hasGet {
			if p, perr := resolvePropertyIn(c, name, f.scope, true); perr == nil && p == nil {
				if slot, ok := obj.object.props.lookup(name); !ok || slot.kind == valueUndefined {
					slot, err := s.h.dynamicPropertySlot(obj.object, name)
					if err != nil {
						return err
					}
					*slot = nullValue()
					s.h.warn(levelWarning, "Undefined property: "+c.name+"::$"+name)
					return s.setReg(f.regs, in.a, nullValue())
				}
			}
		}
	}
	v, err := s.h.readObject(obj, name, f.scope)
	if err != nil {
		return err
	}
	if in.cache != 0 {
		s.cacheProperty(s.cacheOf(in), obj.object, name, f.scope)
	}
	return s.setReg(f.regs, in.a, v)
}

// propSet is ZEND_ASSIGN_OBJ. With MemberResult the value register then
// holds the assignment's value: what a typed property stored.
func (s *vmState) propSet(f *vmFrame, in *vmInstr) error {
	if in.cache != 0 {
		// The fast path: a declared, initialized property that is not a
		// reference, and a value its type takes as it is.
		if obj := f.regs[in.a]; obj.kind == valueObject {
			if c := s.cacheOf(in); c.writable && c.class == obj.object.class && c.scope == f.scope && obj.object.lazy == nil {
				slot := &obj.object.props.declared[c.slot]
				v := f.regs[in.c]
				if v.kind == valueReference {
					v = v.ref.target
				}
				if slot.kind != valueUndefined && slot.kind != valueReference && v.kind != valueUndefined && (c.typ == "" || vmTypeAccepts(c.typ, v)) {
					old := *slot
					if v.kind >= valueArray {
						retain(v)
					}
					*slot = v
					if old.kind >= valueArray {
						return s.h.release(old)
					}
					return nil
				}
			}
		}
	}
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
	if in.cache != 0 {
		s.cacheProperty(s.cacheOf(in), obj.object, name, f.scope)
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
		// A readonly property that holds an object is fetched as that
		// object: what is written through it is the object, not the
		// property.
		if slot, ok := o.props.lookup(p.key); ok && slot.deref().kind == valueObject {
			return scratch(slot.deref())
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
	if c == nil && bytecode.AuxClassRef(in.aux) == bytecode.ClassNamed && v.kind == valueObject {
		// A class that is not declared: only an interface the engine adds
		// by itself (Stringable) can still match, by name.
		return s.setReg(f.regs, in.a, boolValue(classMatchesAnyName(v.object.class, strings.TrimPrefix(s.prog.consts[in.c].text, "\\"))))
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

// stdClassBuiltin is PHP's stdClass: no members, dynamic properties
// allowed. The description is immutable and shared.
var stdClassBuiltin = func() *class {
	c, err := buildClass(classSpec{name: "stdClass", allowDynamic: true})
	if err != nil {
		panic(err)
	}
	return c
}()

// propertyVisible is zend_check_property_access for a property table key:
// a mangled private name is visible in its class, a protected one in the
// object's class hierarchy. name is the property's plain name.
func propertyVisible(o *object, key string, scope *class) (name string, ok bool) {
	if len(key) == 0 || key[0] != 0 {
		return key, true
	}
	end := 1
	for end < len(key) && key[end] != 0 {
		end++
	}
	if end >= len(key) {
		return key, true
	}
	owner, name := key[1:end], key[end+1:]
	if scope == nil {
		return name, false
	}
	if owner == "*" {
		return name, classIs(scope, o.class) || classIs(o.class, scope)
	}
	return name, phpNameEqual(scope.name, owner)
}

// objectArray builds the array a foreach over o walks, or the result of an
// (array) cast. With visible, only the properties scope may see are taken,
// under their plain names; otherwise all of them under their table keys.
// With refs each element is a reference shared with the property.
func (s *vmState) objectArray(o *object, scope *class, visible, refs bool) (value, error) {
	h := s.h
	out := h.newArrayValue()
	retain(out)
	var first error
	o.props.each(func(key string, slot *value) {
		if first != nil || slot.kind == valueUndefined || slot.isExplicitlyUnset() {
			return
		}
		name := key
		if visible {
			var ok bool
			if name, ok = propertyVisible(o, key, scope); !ok {
				return
			}
		}
		k, err := s.readKey(stringValue(name))
		if err != nil {
			first = err
			return
		}
		if !refs {
			v := slot.deref()
			if v.kind >= valueArray {
				retain(v)
			}
			out.array.insert(k, v, s.prog.width)
			return
		}
		var info *propertyInfo
		if o.class.schema != nil {
			for i := range o.class.schema.props {
				if p := &o.class.schema.props[i]; p.key == key && !p.static {
					info = p
					break
				}
			}
		}
		if info != nil && info.readonly {
			first = &Throwable{"Error", "Cannot acquire reference to readonly property " + info.label()}
			return
		}
		var ref value
		if err := h.bindPropertyReference(&ref, slot, info); err != nil {
			first = err
			return
		}
		out.array.insert(k, ref, s.prog.width)
	})
	if first != nil {
		_ = h.release(out)
		return value{}, first
	}
	return out, nil
}

// castObject is the (object) cast.
func (s *vmState) castObject(v value) (value, error) {
	h := s.h
	if v.kind == valueObject {
		return v, nil
	}
	if err := h.registerClass(stdClassBuiltin); err != nil {
		return value{}, err
	}
	obj, err := h.newObject(stdClassBuiltin)
	if err != nil {
		return value{}, err
	}
	set := func(name string, val value) error {
		return h.assign(obj.object.props.slot(name), val)
	}
	switch v.kind {
	case valueNull, valueUndefined:
	case valueArray:
		for _, b := range v.array.buckets {
			if b == nil {
				continue
			}
			name := b.key.text
			if !b.key.isString {
				name = strconv.FormatInt(b.key.integer, 10)
			}
			if err := set(name, b.val.deref()); err != nil {
				return value{}, err
			}
		}
	default:
		if err := set("scalar", v); err != nil {
			return value{}, err
		}
	}
	return obj, nil
}

// callObject is ZEND_INIT_DYNAMIC_CALL for an object: its __invoke.
func (s *vmState) callObject(obj value, argc int, dest uint32) error {
	m, ok := s.h.resolveMethod(obj.object.class, "__invoke", nil)
	if !ok || m == nil || m.fn == 0 || m.call == nil {
		if err := s.dropArgs(argc); err != nil {
			return err
		}
		return &Throwable{"Error", "Object of type " + obj.object.class.name + " is not callable"}
	}
	fn := &s.prog.funcs[m.fn-1]
	if fn.paramByRef != nil {
		if argc > len(s.args) {
			return s.internal("call with %d arguments but %d pending", argc, len(s.args))
		}
		if err := s.derefArguments(fn, argc); err != nil {
			return err
		}
	}
	retain(obj)
	s.into = vmEnter{this: obj.object, scope: m.owner, called: obj.object.class}
	err := s.push(fn, argc, dest)
	if s.into.this != nil {
		_ = s.releaseObject(s.into.this)
	}
	s.into = vmEnter{}
	return err
}

// objectProperty is the property at position pos of o's table (declared
// slots, then dynamic ones in insertion order) with its key, or ok false
// past the end. A position that holds nothing has a nil slot.
func objectProperty(o *object, pos int) (key string, slot *value, ok bool) {
	t := o.props
	declared := 0
	if t.layout != nil {
		declared = len(t.layout.names)
		if pos < declared {
			if v := &t.declared[pos]; v.kind != valueUndefined {
				return t.layout.names[pos], v, true
			}
			return "", nil, true
		}
	}
	pos -= declared
	for b := t.head; b != nil; b = b.next {
		if pos >= b.used {
			pos -= b.used
			continue
		}
		e := &b.entries[pos]
		if cur, live := t.index[e.name]; live && cur == &e.value && e.value.kind != valueUndefined {
			return e.name, &e.value, true
		}
		return "", nil, true
	}
	return "", nil, false
}

// iterNextObject is ZEND_FE_FETCH_R on an object: the next property the
// calling scope may see, read from the object as it is now.
func (s *vmState) iterNextObject(f *vmFrame, in *vmInstr) (more bool, err error) {
	it := &f.regs[in.a]
	o := it.object
	for pos := int(it.integer); ; pos++ {
		if pos&0xfff == 0xfff && s.poll() {
			return false, nil
		}
		key, slot, ok := objectProperty(o, pos)
		if !ok {
			it.integer = int64(pos)
			return false, nil
		}
		if slot == nil || slot.isExplicitlyUnset() {
			continue
		}
		if _, visible := propertyVisible(o, key, f.scope); !visible {
			continue
		}
		it.integer = int64(pos + 1)
		if !it.boolean {
			return true, s.iterStore(f, in.aux, in.b, slot.deref())
		}
		// FE_FETCH_RW: the property becomes a reference the destination
		// shares.
		var info *propertyInfo
		if o.class.schema != nil {
			for i := range o.class.schema.props {
				if p := &o.class.schema.props[i]; p.key == key && !p.static {
					info = p
					break
				}
			}
		}
		if info != nil && info.readonly {
			return false, &Throwable{"Error", "Cannot acquire reference to readonly property " + info.label()}
		}
		var ref value
		if err := s.h.bindPropertyReference(&ref, slot, info); err != nil {
			return false, err
		}
		if bytecode.IterFlags(in.aux)&bytecode.IterLocation != 0 {
			dst, err := s.slot(f, in.b)
			if err == nil {
				err = s.h.bind(dst, &ref)
			}
			if rerr := s.releaseValue(ref); err == nil {
				err = rerr
			}
			return true, err
		}
		old := f.regs[in.b]
		f.regs[in.b] = ref
		return true, s.releaseValue(old)
	}
}

// iterKeyObject is the name of the property iterNextObject fetched last.
func (s *vmState) iterKeyObject(f *vmFrame, in *vmInstr) error {
	it := f.regs[in.b]
	key, slot, ok := objectProperty(it.object, int(it.integer)-1)
	if it.integer <= 0 || !ok || slot == nil {
		return s.unsupported("reading the key of an iterator that has no current element")
	}
	name, _ := propertyVisible(it.object, key, f.scope)
	return s.iterStore(f, in.aux, in.a, stringValue(name))
}

// vmCache is the inline cache of one member instruction in one run: what
// the instruction resolved the last time, for the class of the object it
// saw and the scope it ran in. It holds only descriptions of this run's
// classes, never a value. A hit skips the lookup, not a check: the cached
// facts are exactly the conditions under which the full operation would do
// the same thing.
type vmCache struct {
	class, scope *class
	// slot is the index of a declared property's slot. readable says a
	// read may use it directly; writable that a write may, for a value
	// typ accepts ("" for any).
	slot               int32
	readable, writable bool
	typ                string
	method             *methodSpec
}

func (s *vmState) cacheOf(in *vmInstr) *vmCache {
	if in.cache == 0 {
		return nil
	}
	if s.caches == nil {
		s.caches = make([]vmCache, s.prog.ncaches)
	}
	return &s.caches[in.cache-1]
}

// cacheProperty records how the instruction's property resolves for o's
// class in scope, when it is a plain declared property: no hook, not
// static or virtual, in an object that is not lazy.
func (s *vmState) cacheProperty(c *vmCache, o *object, name string, scope *class) {
	*c = vmCache{}
	if o.lazy != nil || o.class.schema == nil || o.props == nil || o.props.layout == nil {
		return
	}
	p, err := resolvePropertyIn(o.class, name, scope, true)
	if err != nil || p == nil || p.static || p.virtual || p.get != nil || p.set != nil {
		return
	}
	idx, ok := o.props.layout.index[p.key]
	if !ok {
		return
	}
	c.class, c.scope, c.slot = o.class, scope, int32(idx)
	c.readable = true
	c.writable = !p.readonly && canAccess(scope, p.owner, p.setVisibility)
	c.typ = p.typeName
}
