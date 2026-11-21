package bytecode

import (
	"encoding/binary"
	"fmt"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.7 adds the
// class table and the instructions that use objects. The table is immutable
// compiler data like the rest of an image: names, flags and the indices of
// the functions that hold method bodies and constant-expression values. No
// object, static value or evaluated constant is ever part of a Program.

// FunctionFlags describes a function (format 1.7).
type FunctionFlags uint8

const (
	// FuncStrict marks a function compiled under declare(strict_types=1):
	// the calls it makes and its own return value are checked strictly.
	FuncStrict FunctionFlags = 1 << iota
	// FuncNoThis marks code that never has $this: a static method or a
	// plain function. OpThis in it throws.
	FuncNoThis

	functionFlagsKnown = FuncStrict | FuncNoThis
)

// ClassRef says how an instruction names a class. It is the low three bits
// of the Aux of every instruction with a class operand.
type ClassRef uint16

const (
	ClassNamed  ClassRef = iota // the operand is a String constant: the resolved name
	ClassSelf                   // the operand is zero: the class the code is declared in
	ClassParent                 // the operand is zero: its parent
	ClassStatic                 // the operand is zero: the called class (late static binding)
	ClassValue                  // the operand is a Value register: a class name or an object

	classRefMask uint16 = 7
)

// The remaining Aux bits of the format 1.7 member instructions.
const (
	// MemberNameReg makes the name operand a Value register instead of a
	// String constant.
	MemberNameReg uint16 = 1 << 3
	// MemberObjLocation makes the object operand of OpPropGet, OpPropSet,
	// OpPropUnset and OpFetchProp the element Location a FetchDim or
	// FetchProp defined: the object is read from that element.
	MemberObjLocation uint16 = 1 << 6
	// MemberResult makes OpPropSet and OpStaticSet leave the assignment's
	// value in the value register: what the property holds after the
	// conversion to its declared type.
	MemberResult uint16 = 1 << 7

	memberModeShift        = 4
	memberModeMask  uint16 = 3 << memberModeShift
)

// MemberMode is bits 4 and 5 of a member instruction's Aux.
type MemberMode uint16

const (
	// OpPropGet, OpStaticGet.
	MemberRead  MemberMode = 0
	MemberQuiet MemberMode = 1 // no diagnostics for a missing property (isset, ??)
	// OpPropIsset; OpStaticGet's bool forms.
	MemberIsset MemberMode = 2
	MemberEmpty MemberMode = 3
	// OpPropGet as the read half of a compound assignment or of ++/--: a
	// container that is not an object throws instead of warning.
	MemberAssignOp MemberMode = 2
	MemberIncDec   MemberMode = 3
	// OpPropSet, OpStaticSet: C (A for StaticSet) holds a reference the
	// property is bound to (ZEND_ASSIGN_OBJ_REF), or the value ++ or --
	// computed, which a property of type int rejects when it left the
	// integer range.
	MemberBindRef   MemberMode = 1
	MemberIncrement MemberMode = 2
	MemberDecrement MemberMode = 3
	// OpFetchProp, OpFetchStatic.
	MemberWrite     MemberMode = 0
	MemberReadWrite MemberMode = 1
	MemberUnset     MemberMode = 2
)

// MemberAux builds the Aux of a member instruction.
func MemberAux(ref ClassRef, nameReg bool, mode MemberMode, objLocation bool) uint16 {
	aux := uint16(ref) | uint16(mode)<<memberModeShift
	if nameReg {
		aux |= MemberNameReg
	}
	if objLocation {
		aux |= MemberObjLocation
	}
	return aux
}

// AuxClassRef and AuxMemberMode decode a member instruction's Aux.
func AuxClassRef(aux uint16) ClassRef     { return ClassRef(aux & classRefMask) }
func AuxMemberMode(aux uint16) MemberMode { return MemberMode(aux & memberModeMask >> memberModeShift) }

// ReturnCheck is the Aux of OpVerifyReturn.
type ReturnCheck uint16

const (
	ReturnValue ReturnCheck = iota // convert the value to the declared type or throw
	ReturnNone                     // a typed function ended without a value: always throws
	ReturnNever                    // a never-returning function returned: always throws
)

// The Aux values of OpPush that format 1.7 adds, for calls whose callee is
// only known when the call is made. PushAuto sends the Location A by
// reference when the callee's parameter takes one, else its value
// (ZEND_SEND_VAR_EX). PushTemp sends the Value A, which no variable holds:
// a by-reference parameter throws (ZEND_SEND_VAL_EX). PushResult sends the
// Value A, the result of a call: a by-reference parameter gets it with
// PHP's "Only variables should be passed by reference" notice
// (ZEND_SEND_VAR_NO_REF_EX).
//
// PushElement sends the Value A, read from an element or a property. PHP
// fetches such an argument for writing when the parameter is by reference
// (ZEND_FETCH_DIM_FUNC_ARG, ZEND_FETCH_OBJ_FUNC_ARG); this format cannot
// yet, so a by-reference parameter ends the run as unsupported instead of
// receiving a copy.
const (
	PushAuto    PushFlags = 2
	PushTemp    PushFlags = 3
	PushResult  PushFlags = 4
	PushElement PushFlags = 5
)

// NewArgs, in the Aux of OpNew, says arguments follow: a class without a
// constructor then starts a call that evaluates and discards them, as
// ZEND_NEW calls zend_pass_function, and C is not taken.
const NewArgs uint16 = 1 << 3

// InitCallable, in the Aux of OpInitMethod, makes A a callable value
// instead of an object whose method B names: an object is called through
// __invoke and a string names a function (ZEND_INIT_DYNAMIC_CALL). B is
// zero.
const InitCallable uint16 = 1 << 4

// UnknownCallee is the callee of a call OpNew, OpInitMethod or OpInitStatic
// started: it is found when the instruction runs.
const UnknownCallee = ^uint32(0)

// Role is what one operand of a format 1.7 instruction is.
type Role uint8

const (
	RoleNone   Role = iota // the operand is zero
	RoleUse                // a Value register the instruction reads
	RoleDef                // a Value register the instruction writes
	RoleUseDef             // a Value register read and then replaced
	RoleLocDef             // a Location register the instruction defines (an element location)
	RoleLocUse             // the element Location the object is read from
	RoleString             // a String constant
	RoleClass              // an index into the class table
	RoleTarget             // an instruction index
)

// Roles17 gives the roles of the operands of a format 1.7 instruction. ok
// is false for any other opcode and for an Aux the opcode does not define.
func Roles17(in Instruction) (roles [3]Role, ok bool) {
	aux := in.Aux
	class := func() Role {
		switch AuxClassRef(aux) {
		case ClassNamed:
			return RoleString
		case ClassValue:
			return RoleUse
		}
		return RoleNone
	}
	name := RoleString
	if aux&MemberNameReg != 0 {
		name = RoleUse
	}
	obj := RoleUse
	if aux&MemberObjLocation != 0 {
		obj = RoleLocUse
	}
	mode := AuxMemberMode(aux)
	// only reports whether aux uses nothing but the bits in allowed.
	only := func(allowed uint16) bool { return aux&^allowed == 0 }
	validRef := AuxClassRef(aux) <= ClassValue
	switch in.Op {
	case OpDeclareClass:
		return [3]Role{RoleNone, RoleClass, RoleNone}, aux == 0
	case OpNew:
		return [3]Role{RoleDef, class(), RoleTarget}, only(classRefMask|NewArgs) && validRef
	case OpInitMethod:
		if aux == InitCallable {
			return [3]Role{RoleUse, RoleNone, RoleNone}, true
		}
		return [3]Role{RoleUse, name, RoleNone}, only(MemberNameReg)
	case OpInitStatic:
		return [3]Role{class(), name, RoleNone}, only(classRefMask|MemberNameReg) && validRef
	case OpThis:
		// Aux 1 asks whether there is a $this (isset($this)): A is a bool.
		return [3]Role{RoleDef, RoleNone, RoleNone}, aux <= 1
	case OpPropGet:
		return [3]Role{RoleDef, obj, name}, only(MemberNameReg | memberModeMask | MemberObjLocation)
	case OpPropSet:
		return [3]Role{obj, name, RoleUseDef}, only(MemberNameReg | memberModeMask | MemberObjLocation | MemberResult)
	case OpFetchProp:
		return [3]Role{RoleLocDef, obj, name}, only(MemberNameReg|memberModeMask|MemberObjLocation) && mode <= MemberUnset
	case OpPropRef:
		return [3]Role{RoleDef, RoleUse, name}, only(MemberNameReg)
	case OpPropIsset:
		return [3]Role{RoleDef, RoleUse, name}, only(MemberNameReg|memberModeMask) && mode >= MemberIsset
	case OpPropUnset:
		return [3]Role{obj, name, RoleNone}, only(MemberNameReg | MemberObjLocation)
	case OpStaticGet:
		return [3]Role{RoleDef, class(), name}, only(classRefMask|MemberNameReg|memberModeMask) && validRef
	case OpStaticSet:
		return [3]Role{RoleUseDef, class(), name}, only(classRefMask|MemberNameReg|memberModeMask|MemberResult) && validRef
	case OpFetchStatic:
		return [3]Role{RoleLocDef, class(), name}, only(classRefMask|MemberNameReg|memberModeMask) && validRef && mode <= MemberReadWrite
	case OpStaticRef:
		return [3]Role{RoleDef, class(), name}, only(classRefMask|MemberNameReg) && validRef
	case OpClassConst:
		return [3]Role{RoleDef, class(), RoleString}, only(classRefMask) && validRef
	case OpClassName:
		return [3]Role{RoleDef, class(), RoleNone}, only(classRefMask) && validRef
	case OpInstanceof:
		return [3]Role{RoleDef, RoleUse, class()}, only(classRefMask) && validRef
	case OpClone:
		return [3]Role{RoleDef, RoleUse, RoleNone}, aux == 0
	case OpVerifyReturn:
		return [3]Role{RoleUseDef, RoleNone, RoleNone}, aux <= uint16(ReturnNever)
	}
	return roles, false
}

// format17 reports whether a known instruction needs format 1.7.
func format17(in Instruction) bool {
	switch {
	case in.Op >= OpDeclareClass && in.Op <= OpVerifyReturn:
		return true
	case in.Op == OpPush:
		return in.Aux > uint16(PushLoad)
	case in.Op == OpCast:
		return CastKind(in.Aux) == CastObject
	case in.Op == OpUnary:
		return UnaryOp(in.Aux) > BitNot
	}
	return false
}

// checkOperands17 checks a format 1.7 instruction's operands against its
// roles.
func checkOperands17(image Image, f *Function, in Instruction) error {
	roles, ok := Roles17(in)
	if !ok {
		return invalid("invalid aux for a format 1.7 instruction")
	}
	for i, v := range [3]uint32{in.A, in.B, in.C} {
		good := false
		switch roles[i] {
		case RoleNone:
			good = v == 0
		case RoleUse, RoleDef, RoleUseDef:
			good = uint64(v) < uint64(len(f.Registers)) && f.Registers[v] == Value
		case RoleLocDef, RoleLocUse:
			good = uint64(v) < uint64(len(f.Registers)) && f.Registers[v] == Location
		case RoleString:
			good = uint64(v) < uint64(len(image.Constants)) && image.Constants[v].Kind == String
		case RoleClass:
			good = uint64(v) < uint64(len(image.Classes))
		case RoleTarget:
			good = uint64(v) < uint64(len(f.Code))
		}
		if !good {
			return invalid(fmt.Sprintf("invalid operand %c of a format 1.7 instruction", 'A'+i))
		}
	}
	if in.Op == OpFetchProp && in.Aux&MemberObjLocation != 0 && in.A != in.B {
		return invalid("a property fetched from an element location must replace that location")
	}
	return nil
}

// ClassFlags describes a class-like declaration.
type ClassFlags uint16

const (
	ClassInterface ClassFlags = 1 << iota
	ClassTrait
	ClassAbstract
	ClassFinal
	ClassReadonly
	// ClassHoisted marks a declaration PHP binds while compiling the file
	// (early binding): it exists before the first instruction runs. Every
	// other class is declared by its OpDeclareClass.
	ClassHoisted
	// ClassAllowDynamic is #[AllowDynamicProperties]: creating a dynamic
	// property on an object of the class is not deprecated.
	ClassAllowDynamic

	classFlagsKnown = ClassInterface | ClassTrait | ClassAbstract | ClassFinal | ClassReadonly | ClassHoisted | ClassAllowDynamic
)

// MemberFlags describes a constant, property or method of a class. The low
// two bits are the visibility.
type MemberFlags uint16

const (
	MemberPublic    MemberFlags = 0
	MemberProtected MemberFlags = 1
	MemberPrivate   MemberFlags = 2
	memberVisMask   MemberFlags = 3

	MemberStatic   MemberFlags = 1 << 2
	MemberFinal    MemberFlags = 1 << 3
	MemberAbstract MemberFlags = 1 << 4
	MemberReadonly MemberFlags = 1 << 5
	// MemberByRef marks a method that returns by reference.
	MemberByRef MemberFlags = 1 << 6

	memberFlagsKnown = memberVisMask | MemberStatic | MemberFinal | MemberAbstract | MemberReadonly | MemberByRef
)

// Visibility is the member's visibility bits.
func (f MemberFlags) Visibility() MemberFlags { return f & memberVisMask }

// ClassConstant is a class constant. Exactly one of Value and Init is not
// zero: Value is the index plus one of the constant's scalar value in the
// pool, Init the index plus one of a function without parameters that
// computes it (in the declaring class's scope) the first time it is read.
type ClassConstant struct {
	Name        uint32 // String constant
	Flags       MemberFlags
	Value, Init uint32
}

// Property is a declared property. Type is zero or the index plus one of a
// String constant. Value and Init are as in ClassConstant, both zero for a
// property without a default.
type Property struct {
	Name        uint32 // String constant
	Flags       MemberFlags
	Type        uint32
	Value, Init uint32
}

// Method names the function that holds a method's body. An abstract method
// has one too: its parameter table describes the signature, and its code is
// never run. Signature is the index plus one of a String constant holding
// the declaration as PHP prints it in inheritance errors, or zero.
type Method struct {
	Name      uint32 // String constant
	Function  uint32
	Flags     MemberFlags
	Signature uint32
	Line      uint32
}

// TraitRule is one adaptation of a class's trait uses. Trait is zero or the
// index plus one of the String constant naming the trait before "::".
// With Insteadof, the method of Trait wins over those traits. Otherwise the
// rule is an alias: Alias is zero or the index plus one of the new name,
// and Flags is the new visibility when HasVisibility is set.
type TraitRule struct {
	Trait, Method uint32
	Alias         uint32
	Flags         MemberFlags
	HasVisibility bool
	Insteadof     []uint32
	Line          uint32
}

// Class is one class, interface or trait declaration. Parent is zero or the
// index plus one of a String constant; names are resolved when the class is
// declared.
type Class struct {
	Name       uint32 // String constant
	Flags      ClassFlags
	Parent     uint32
	Line       uint32
	Interfaces []uint32
	Traits     []uint32
	Rules      []TraitRule
	Constants  []ClassConstant
	Properties []Property
	Methods    []Method
}

// ClassCount is the size of the class table.
func (p *Program) ClassCount() int { return len(p.image.Classes) }

// Class returns a copy of entry index of the class table.
func (p *Program) Class(index uint32) (Class, bool) {
	if uint64(index) >= uint64(len(p.image.Classes)) {
		return Class{}, false
	}
	return copyClass(p.image.Classes[index]), true
}

func copyClass(c Class) Class {
	c.Interfaces = append([]uint32(nil), c.Interfaces...)
	c.Traits = append([]uint32(nil), c.Traits...)
	c.Constants = append([]ClassConstant(nil), c.Constants...)
	c.Properties = append([]Property(nil), c.Properties...)
	c.Methods = append([]Method(nil), c.Methods...)
	rules := make([]TraitRule, len(c.Rules))
	for i, r := range c.Rules {
		r.Insteadof = append([]uint32(nil), r.Insteadof...)
		rules[i] = r
	}
	c.Rules = rules
	return c
}

// ReturnType is the declared return type, or "" (format 1.7).
func (f FunctionView) ReturnType() string {
	if f.function == nil || f.function.ReturnType == 0 || uint64(f.function.ReturnType-1) >= uint64(len(f.constants)) {
		return ""
	}
	return f.constants[f.function.ReturnType-1].Bytes
}

// Flags is the function's flags (format 1.7).
func (f FunctionView) Flags() FunctionFlags {
	if f.function == nil {
		return 0
	}
	return f.function.Flags
}

// ParameterType is the declared type of parameter index, or "".
func (f FunctionView) ParameterType(index uint32) string {
	if f.function == nil || uint64(index) >= uint64(len(f.function.Parameters)) {
		return ""
	}
	t := f.function.Parameters[index].Type
	if t == 0 || uint64(t-1) >= uint64(len(f.constants)) {
		return ""
	}
	return f.constants[t-1].Bytes
}

// classRecords is the number of table records of a class, which bounds the
// work and memory of handling it.
func classRecords(c *Class) uint64 {
	n := uint64(len(c.Interfaces)) + uint64(len(c.Traits)) + uint64(len(c.Rules)) + uint64(len(c.Constants)) + uint64(len(c.Properties)) + uint64(len(c.Methods)) + 1
	for i := range c.Rules {
		n += uint64(len(c.Rules[i].Insteadof))
	}
	return n
}

// verifyClasses checks the class table: every index names a constant of the
// right kind or a function, every member name is unique in its class, and
// each function is the body of at most one member, so the interpreter can
// trust the table's shape. Whether the declarations are valid PHP (the
// inheritance rules) is checked when a class is declared, against the
// classes that exist then.
func verifyClasses(b *budget, image Image, minor uint16, cloneCost bool) error {
	if len(image.Classes) == 0 {
		return nil
	}
	if minor < 7 {
		return invalid(fmt.Sprintf("a class table needs bytecode format 1.7, but the file declares format 1.%d", minor))
	}
	if uint64(len(image.Classes)) > uint64(b.lim.MaxFunctions) {
		return ErrLimit
	}
	str := func(i uint32) bool {
		return uint64(i) < uint64(len(image.Constants)) && image.Constants[i].Kind == String
	}
	name := func(i uint32) bool { return str(i) && image.Constants[i].Bytes != "" }
	optName := func(i uint32) bool { return i == 0 || name(i-1) }
	// used marks the functions that are a member's body or initializer.
	if err := b.reserve(uint64(len(image.Functions)) + 64); err != nil {
		return err
	}
	used := make([]bool, len(image.Functions))
	body := func(i uint32, initializer bool) error {
		if uint64(i) >= uint64(len(image.Functions)) || i == image.Entry {
			return invalid("class member names a function that is not in the image or is the entry")
		}
		if used[i] {
			return invalid("a function is the body of more than one class member")
		}
		used[i] = true
		f := &image.Functions[i]
		if f.ReturnKind != Value {
			return invalid("a class member's function must return a value")
		}
		if initializer && (f.Params != 0 || len(f.Parameters) != 0) {
			return invalid("an initializer function takes no parameters")
		}
		return nil
	}
	valueOrInit := func(value, init uint32, required bool) error {
		switch {
		case value != 0 && init != 0:
			return invalid("a class member has both a constant value and an initializer")
		case value != 0:
			if uint64(value-1) >= uint64(len(image.Constants)) {
				return invalid("a class member's value is not in the constant pool")
			}
		case init != 0:
			return body(init-1, true)
		case required:
			return invalid("a class constant has no value")
		}
		return nil
	}
	var total uint64
	for ci := range image.Classes {
		c := &image.Classes[ci]
		records := classRecords(c)
		total += records
		if total > uint64(b.lim.MaxConstants)+uint64(b.lim.MaxFunctions) {
			return ErrLimit
		}
		if err := b.chargeWork(records); err != nil {
			return err
		}
		if cloneCost {
			if err := b.reserveMul(records, 40); err != nil {
				return err
			}
		}
		fail := func(detail string) error {
			return invalid(fmt.Sprintf("class %d: %s", ci, detail))
		}
		if !name(c.Name) || !optName(c.Parent) {
			return fail("the class or parent name is not a non-empty string constant")
		}
		if c.Flags&^classFlagsKnown != 0 {
			return fail("unknown class flags")
		}
		if c.Flags&ClassInterface != 0 && c.Flags&(ClassTrait|ClassFinal|ClassReadonly) != 0 || c.Flags&ClassTrait != 0 && c.Flags&(ClassFinal|ClassReadonly|ClassAbstract) != 0 || c.Flags&ClassAbstract != 0 && c.Flags&ClassFinal != 0 {
			return fail("contradictory class flags")
		}
		if c.Flags&(ClassInterface|ClassTrait) != 0 && c.Parent != 0 {
			return fail("an interface or trait has no parent class")
		}
		for _, lists := range [2][]uint32{c.Interfaces, c.Traits} {
			for _, n := range lists {
				if !name(n) {
					return fail("an interface or trait name is not a non-empty string constant")
				}
			}
		}
		if c.Flags&ClassInterface != 0 && (len(c.Traits) != 0 || len(c.Properties) != 0) {
			return fail("an interface has no traits or properties")
		}
		for _, r := range c.Rules {
			if !optName(r.Trait) || !name(r.Method) || !optName(r.Alias) || r.Flags&^memberVisMask != 0 || r.Flags.Visibility() > MemberPrivate {
				return fail("invalid trait rule")
			}
			if !r.HasVisibility && r.Flags != 0 {
				return fail("invalid trait rule: visibility bits without HasVisibility")
			}
			if len(r.Insteadof) != 0 && (r.Trait == 0 || r.Alias != 0 || r.HasVisibility) {
				return fail("an insteadof trait rule names its trait and has no alias")
			}
			if len(r.Insteadof) == 0 && r.Alias == 0 && !r.HasVisibility {
				return fail("a trait alias rule changes nothing")
			}
			for _, n := range r.Insteadof {
				if !name(n) {
					return fail("invalid trait rule")
				}
			}
		}
		// Names are compared as the pool's strings; the sizes are bounded
		// by MaxConstants, and each comparison is charged above.
		if err := b.reserveMul(records, 48); err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(c.Constants)+len(c.Properties)+len(c.Methods))
		unique := func(kind byte, n string) bool {
			key := string(kind) + n
			if _, dup := seen[key]; dup {
				return false
			}
			seen[key] = struct{}{}
			return true
		}
		for _, k := range c.Constants {
			if !name(k.Name) || k.Flags&^(memberVisMask|MemberFinal) != 0 || k.Flags.Visibility() > MemberPrivate {
				return fail("invalid class constant")
			}
			if !unique('c', image.Constants[k.Name].Bytes) {
				return fail("duplicate class constant name")
			}
			if err := valueOrInit(k.Value, k.Init, true); err != nil {
				return err
			}
		}
		for _, p := range c.Properties {
			if !name(p.Name) || !optName(p.Type) || p.Flags&^(memberVisMask|MemberStatic|MemberReadonly|MemberFinal) != 0 || p.Flags.Visibility() > MemberPrivate {
				return fail("invalid property")
			}
			if p.Type != 0 {
				if typ := image.Constants[p.Type-1].Bytes; !validType(typ) || typ == "void" || typ == "never" || typ == "callable" {
					return fail("a property's type does not spell a property type")
				}
			}
			if !unique('p', image.Constants[p.Name].Bytes) {
				return fail("duplicate property name")
			}
			if err := valueOrInit(p.Value, p.Init, false); err != nil {
				return err
			}
		}
		for _, m := range c.Methods {
			if !name(m.Name) || !optName(m.Signature) || m.Flags&^(memberVisMask|MemberStatic|MemberFinal|MemberAbstract|MemberByRef) != 0 || m.Flags.Visibility() > MemberPrivate {
				return fail("invalid method")
			}
			if !unique('m', lowerASCII(image.Constants[m.Name].Bytes)) {
				return fail("duplicate method name")
			}
			if err := body(m.Function, false); err != nil {
				return err
			}
			if m.Flags&MemberAbstract != 0 && m.Flags&MemberFinal != 0 {
				return fail("a method is both abstract and final")
			}
			if c.Flags&ClassInterface != 0 && (m.Flags&MemberAbstract == 0 || m.Flags.Visibility() != MemberPublic) {
				return fail("an interface method is public and abstract")
			}
			if static := image.Functions[m.Function].Flags&FuncNoThis != 0; static != (m.Flags&MemberStatic != 0) {
				return fail("a method's static flag differs from its function's")
			}
		}
	}
	return nil
}

// lowerASCII folds the ASCII letters of a PHP identifier, as PHP does for
// method and class names.
func lowerASCII(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// verifyFunction17 checks the format 1.7 fields of a function.
func verifyFunction17(image Image, minor uint16, f *Function) error {
	if minor < 7 {
		if f.Flags != 0 || f.ReturnType != 0 {
			return invalid(fmt.Sprintf("function flags and return types need bytecode format 1.7, but the file declares format 1.%d", minor))
		}
		for i, p := range f.Parameters {
			if p.Type != 0 || p.Flags&^paramFlagsKnown16 != 0 {
				return invalid(fmt.Sprintf("parameter %d uses a type or flag of bytecode format 1.7, but the file declares format 1.%d", i, minor))
			}
		}
		return nil
	}
	if f.Flags&^functionFlagsKnown != 0 {
		return invalid("unknown function flags")
	}
	str := func(i uint32) bool {
		return i == 0 || uint64(i-1) < uint64(len(image.Constants)) && image.Constants[i-1].Kind == String && image.Constants[i-1].Bytes != "" && validType(image.Constants[i-1].Bytes)
	}
	if !str(f.ReturnType) {
		return invalid("the return type is not a non-empty string constant that spells a type")
	}
	for i, p := range f.Parameters {
		if !str(p.Type) || p.Type != 0 && (image.Constants[p.Type-1].Bytes == "void" || image.Constants[p.Type-1].Bytes == "never") {
			return invalid(fmt.Sprintf("the type of parameter %d is not a non-empty string constant that spells a parameter type", i))
		}
	}
	return nil
}

// --- wire ---

// Encoded sizes of the fixed parts of the class table records.
const (
	classWire     = 16 // Name, Flags:u16 + pad:u16, Parent, Line
	constantWire  = 14 // Name, Flags:u16, Value, Init
	propertyWire  = 18 // Name, Flags:u16, Type, Value, Init
	methodWire    = 18 // Name, Function, Flags:u16, Signature, Line
	traitRuleWire = 19 // Trait, Method, Alias, Flags:u16, HasVisibility:u8, Line
)

func classesSize(b *budget, classes []Class, add func(uint64) bool) error {
	if uint64(len(classes)) > uint64(^uint32(0)) || !add(4) {
		return ErrLimit
	}
	for i := range classes {
		c := &classes[i]
		if err := b.chargeWork(classRecords(c)); err != nil {
			return err
		}
		n := uint64(classWire) + 6*4 + uint64(len(c.Interfaces)+len(c.Traits))*4 + uint64(len(c.Constants))*constantWire + uint64(len(c.Properties))*propertyWire + uint64(len(c.Methods))*methodWire
		for j := range c.Rules {
			n += traitRuleWire + 4 + uint64(len(c.Rules[j].Insteadof))*4
		}
		if !add(n) {
			return ErrLimit
		}
	}
	return nil
}

func (w *writer) classes(classes []Class) error {
	u32s := func(vs ...uint32) error {
		for _, v := range vs {
			if err := w.u32(v); err != nil {
				return err
			}
		}
		return nil
	}
	list := func(vs []uint32) error {
		if err := w.u32(uint32(len(vs))); err != nil {
			return err
		}
		return u32s(vs...)
	}
	if err := w.u32(uint32(len(classes))); err != nil {
		return err
	}
	for i := range classes {
		c := &classes[i]
		if err := w.u32(c.Name); err != nil {
			return err
		}
		if err := w.u16(uint16(c.Flags)); err != nil {
			return err
		}
		if err := w.u16(0); err != nil {
			return err
		}
		if err := u32s(c.Parent, c.Line); err != nil {
			return err
		}
		if err := list(c.Interfaces); err != nil {
			return err
		}
		if err := list(c.Traits); err != nil {
			return err
		}
		if err := w.u32(uint32(len(c.Rules))); err != nil {
			return err
		}
		for _, r := range c.Rules {
			if err := u32s(r.Trait, r.Method, r.Alias); err != nil {
				return err
			}
			if err := w.u16(uint16(r.Flags)); err != nil {
				return err
			}
			if err := w.u8(boolByte(r.HasVisibility)); err != nil {
				return err
			}
			if err := w.u32(r.Line); err != nil {
				return err
			}
			if err := list(r.Insteadof); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(c.Constants))); err != nil {
			return err
		}
		for _, k := range c.Constants {
			if err := w.u32(k.Name); err != nil {
				return err
			}
			if err := w.u16(uint16(k.Flags)); err != nil {
				return err
			}
			if err := u32s(k.Value, k.Init); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(c.Properties))); err != nil {
			return err
		}
		for _, p := range c.Properties {
			if err := w.u32(p.Name); err != nil {
				return err
			}
			if err := w.u16(uint16(p.Flags)); err != nil {
				return err
			}
			if err := u32s(p.Type, p.Value, p.Init); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(c.Methods))); err != nil {
			return err
		}
		for _, m := range c.Methods {
			if err := u32s(m.Name, m.Function); err != nil {
				return err
			}
			if err := w.u16(uint16(m.Flags)); err != nil {
				return err
			}
			if err := u32s(m.Signature, m.Line); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *reader) classes(b *budget) ([]Class, error) {
	le := binary.LittleEndian
	nc, err := r.count(classWire+6*4, b.lim.MaxFunctions)
	if err != nil || nc == 0 {
		return nil, err
	}
	if err = b.reserveMul(uint64(nc), 160); err != nil {
		return nil, err
	}
	// Every list is bounded by the payload it still has to fit in (count)
	// and by the pool and function limits.
	maxRecords := b.lim.MaxConstants
	list := func() ([]uint32, error) {
		n, e := r.count(4, maxRecords)
		if e != nil || n == 0 {
			return nil, e
		}
		if e = b.reserveMul(uint64(n), 4); e != nil {
			return nil, e
		}
		out := make([]uint32, int(n))
		for i := range out {
			if out[i], e = r.u32(); e != nil {
				return nil, e
			}
		}
		return out, nil
	}
	classes := make([]Class, int(nc))
	for i := range classes {
		c := &classes[i]
		p, e := r.take(classWire)
		if e != nil {
			return nil, e
		}
		if le.Uint16(p[6:]) != 0 {
			return nil, r.invalid("nonzero class padding")
		}
		c.Name, c.Flags, c.Parent, c.Line = le.Uint32(p), ClassFlags(le.Uint16(p[4:])), le.Uint32(p[8:]), le.Uint32(p[12:])
		if c.Interfaces, e = list(); e != nil {
			return nil, e
		}
		if c.Traits, e = list(); e != nil {
			return nil, e
		}
		n, e := r.count(traitRuleWire+4, maxRecords)
		if e != nil {
			return nil, e
		}
		if n > 0 {
			if e = b.reserveMul(uint64(n), 56); e != nil {
				return nil, e
			}
			c.Rules = make([]TraitRule, int(n))
			for j := range c.Rules {
				if p, e = r.take(traitRuleWire); e != nil {
					return nil, e
				}
				if p[14] > 1 {
					return nil, r.invalid("invalid trait rule flag")
				}
				c.Rules[j] = TraitRule{Trait: le.Uint32(p), Method: le.Uint32(p[4:]), Alias: le.Uint32(p[8:]), Flags: MemberFlags(le.Uint16(p[12:])), HasVisibility: p[14] == 1, Line: le.Uint32(p[15:])}
				if c.Rules[j].Insteadof, e = list(); e != nil {
					return nil, e
				}
			}
		}
		if n, e = r.count(constantWire, maxRecords); e != nil {
			return nil, e
		}
		if n > 0 {
			if e = b.reserveMul(uint64(n), 16); e != nil {
				return nil, e
			}
			c.Constants = make([]ClassConstant, int(n))
			for j := range c.Constants {
				if p, e = r.take(constantWire); e != nil {
					return nil, e
				}
				c.Constants[j] = ClassConstant{Name: le.Uint32(p), Flags: MemberFlags(le.Uint16(p[4:])), Value: le.Uint32(p[6:]), Init: le.Uint32(p[10:])}
			}
		}
		if n, e = r.count(propertyWire, maxRecords); e != nil {
			return nil, e
		}
		if n > 0 {
			if e = b.reserveMul(uint64(n), 20); e != nil {
				return nil, e
			}
			c.Properties = make([]Property, int(n))
			for j := range c.Properties {
				if p, e = r.take(propertyWire); e != nil {
					return nil, e
				}
				c.Properties[j] = Property{Name: le.Uint32(p), Flags: MemberFlags(le.Uint16(p[4:])), Type: le.Uint32(p[6:]), Value: le.Uint32(p[10:]), Init: le.Uint32(p[14:])}
			}
		}
		if n, e = r.count(methodWire, b.lim.MaxFunctions); e != nil {
			return nil, e
		}
		if n > 0 {
			if e = b.reserveMul(uint64(n), 20); e != nil {
				return nil, e
			}
			c.Methods = make([]Method, int(n))
			for j := range c.Methods {
				if p, e = r.take(methodWire); e != nil {
					return nil, e
				}
				c.Methods[j] = Method{Name: le.Uint32(p), Function: le.Uint32(p[4:]), Flags: MemberFlags(le.Uint16(p[8:])), Signature: le.Uint32(p[10:]), Line: le.Uint32(p[14:])}
			}
		}
	}
	return classes, nil
}

func cloneClasses(b *budget, src []Class) ([]Class, error) {
	if len(src) == 0 {
		return nil, nil
	}
	dst := make([]Class, len(src))
	for i := range src {
		if err := b.chargeWork(classRecords(&src[i])); err != nil {
			return nil, err
		}
		dst[i] = copyClass(src[i])
	}
	return dst, nil
}

// validType reports whether s has the shape of a PHP type declaration: an
// optional "?" before one name, or names joined by "|", where a member may
// be names joined by "&" (in parentheses inside a union). The runtime
// treats the names as it does for compiled PHP; what the check keeps out is
// a string the type machinery was never written for.
func validType(s string) bool {
	if len(s) > 1 && s[0] == '?' {
		s = s[1:]
		for i := 0; i < len(s); i++ {
			if s[i] == '|' || s[i] == '&' || s[i] == '(' {
				return false
			}
		}
	}
	name := func(n string) bool {
		if n == "" {
			return false
		}
		for i := 0; i < len(n); i++ {
			c := n[i]
			switch {
			case c == '_' || c == '\\' || c >= 0x80 || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			case c >= '0' && c <= '9' && i > 0:
			default:
				return false
			}
		}
		return true
	}
	intersection := func(m string) bool {
		for {
			i := 0
			for i < len(m) && m[i] != '&' {
				i++
			}
			if !name(m[:i]) {
				return false
			}
			if i == len(m) {
				return true
			}
			m = m[i+1:]
		}
	}
	for {
		i := 0
		for i < len(s) && s[i] != '|' {
			i++
		}
		member := s[:i]
		if len(member) > 2 && member[0] == '(' && member[len(member)-1] == ')' {
			member = member[1 : len(member)-1]
		}
		if !intersection(member) {
			return false
		}
		if i == len(s) {
			return true
		}
		s = s[i+1:]
	}
}
