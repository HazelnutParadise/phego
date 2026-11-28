package bytecode

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/HazelnutParadise/phego/internal/profile"
)

func validateIdentity(id Identity) error {
	if id.Profile.Release == "" || profile.ReleaseStatus(id.Profile.Release) == profile.Unrecorded {
		return ErrProfile
	}
	resolved, err := profile.Resolve(id.Profile.Release, id.Profile.IntWidth, id.Profile.ShortOpenTag)
	if err != nil || resolved != id.Profile || id.GuestPlatform == "" || id.Target == "" ||
		id.Source == ([32]byte{}) || id.Compiler == ([32]byte{}) || id.Modules == ([32]byte{}) ||
		id.RuntimeABI == ([32]byte{}) || id.Policy == ([32]byte{}) || id.Scope == ([32]byte{}) {
		return ErrProfile
	}
	return nil
}

// verifyImage checks image as format version 1.minor. Minor 0 files may not
// use any format 1.1 opcode, Aux value or binary operator, files below minor
// 2 may not use any format 1.2 opcode or Aux value, and so on up to 1.6
// (format13 to format16).
func verifyImage(b *budget, image Image, minor uint16, cloneCost bool) (CapabilitySet, error) {
	if err := b.poll(); err != nil {
		return 0, err
	}
	if err := validateIdentity(image.Identity); err != nil {
		return 0, fail(ErrProfile, "invalid host identity")
	}
	if image.Required & ^AllCapabilities != 0 {
		return 0, invalid("unknown required capability")
	}
	if len(image.Functions) == 0 || uint64(image.Entry) >= uint64(len(image.Functions)) {
		return 0, invalid("image table count or entry is invalid")
	}
	if uint64(len(image.Functions)) > uint64(b.lim.MaxFunctions) || uint64(len(image.Constants)) > uint64(b.lim.MaxConstants) || uint64(len(image.Sources)) > uint64(b.lim.MaxSources) {
		return 0, ErrLimit
	}
	if cloneCost {
		if err := b.reserveMul(uint64(len(image.Functions)), 160); err != nil {
			return 0, err
		}
		if err := b.reserveMul(uint64(len(image.Constants)), 32); err != nil {
			return 0, err
		}
		if err := b.reserveMul(uint64(len(image.Sources)), 80); err != nil {
			return 0, err
		}
	}
	if cloneCost {
		if err := b.reserve(uint64(len(image.Identity.Profile.Release)) + uint64(len(image.Identity.GuestPlatform)) + uint64(len(image.Identity.Target)) + 512); err != nil {
			return 0, err
		}
	}
	for _, c := range image.Constants {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		switch c.Kind {
		case Null:
			if c.Bits != 0 || c.Bytes != "" {
				return 0, invalid("null constant has unused data")
			}
		case Bool:
			if c.Bits > 1 || c.Bytes != "" {
				return 0, invalid("invalid bool constant")
			}
		case Int:
			if c.Bytes != "" || (image.Identity.Profile.IntWidth == 32 && int64(int32(uint32(c.Bits))) != int64(c.Bits)) {
				return 0, invalid("integer constant is outside guest range")
			}
		case Float:
			if c.Bytes != "" {
				return 0, invalid("float constant has unused bytes")
			}
		case String:
			if c.Bits != 0 {
				return 0, invalid("string constant has unused bits")
			}
			if cloneCost {
				if err := b.reserve(uint64(len(c.Bytes)) + 24); err != nil {
					return 0, err
				}
			}
		default:
			return 0, invalid("unknown constant kind")
		}
	}
	for _, s := range image.Sources {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		if s.Digest == ([32]byte{}) {
			return 0, invalid("source digest is zero")
		}
		if cloneCost {
			if err := b.reserve(uint64(len(s.Name)) + 24); err != nil {
				return 0, err
			}
		}
	}
	var instructionTotal, handlerTotal uint64
	var inferred CapabilitySet
	for fi := range image.Functions {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		f := &image.Functions[fi]
		if f.Name == "" || len(f.Code) == 0 || len(f.Positions) != len(f.Code) ||
			len(f.Registers) == 0 || f.Params > uint32(len(f.Registers)) ||
			(f.ReturnKind != Value && f.ReturnKind != Location) {
			return 0, invalid("invalid function frame or metadata")
		}
		if f.MaxStack > b.lim.MaxStack || f.Locals > b.lim.MaxLocals {
			return 0, ErrLimit
		}
		if uint64(len(f.Registers)) > uint64(b.lim.MaxRegisters) {
			return 0, ErrLimit
		}
		if uint64(len(f.Code)) > uint64(b.lim.MaxInstructions) || instructionTotal > uint64(b.lim.MaxInstructions)-uint64(len(f.Code)) {
			return 0, ErrLimit
		}
		instructionTotal += uint64(len(f.Code))
		if uint64(len(f.Handlers)) > uint64(b.lim.MaxHandlers) || handlerTotal > uint64(b.lim.MaxHandlers)-uint64(len(f.Handlers)) {
			return 0, ErrLimit
		}
		handlerTotal += uint64(len(f.Handlers))
		if cloneCost {
			if err := b.reserve(uint64(len(f.Name)) + 64); err != nil {
				return 0, err
			}
			if err := b.reserveMul(uint64(len(f.Registers)), 8); err != nil {
				return 0, err
			}
			if err := b.reserveMul(uint64(len(f.Code)), 32); err != nil {
				return 0, err
			}
			if err := b.reserveMul(uint64(len(f.Positions)), 24); err != nil {
				return 0, err
			}
			if err := b.reserveMul(uint64(len(f.Handlers)), 40); err != nil {
				return 0, err
			}
		}
		if err := verifyLocalNames(b, image, f, cloneCost); err != nil {
			return 0, functionError(err, fi, -1)
		}
		if err := verifyParameters(b, image, f, cloneCost); err != nil {
			return 0, functionError(err, fi, -1)
		}
		if err := verifyFunction17(image, minor, f); err != nil {
			return 0, functionError(err, fi, -1)
		}
		for _, k := range f.Registers {
			if err := b.chargeWork(1); err != nil {
				return 0, err
			}
			if k != Value && k != Location {
				return 0, invalid("unknown register kind")
			}
		}
		if f.Params > 0 {
			for i := uint32(0); i < f.Params; i++ {
				if f.Registers[i] != Value {
					return 0, invalid("parameter register is not a value")
				}
			}
		}
		for pc, p := range f.Positions {
			if err := b.chargeWork(1); err != nil {
				return 0, err
			}
			if p == (Position{}) {
				continue
			}
			switch positionProblem(image.Sources, p) {
			case positionInvalid:
				return 0, functionError(invalid("invalid source position"), fi, pc)
			case positionOutside:
				return 0, functionError(invalid("source position outside source"), fi, pc)
			}
		}
		caps, err := verifyFunction(b, image, minor, uint32(fi), f)
		if err != nil {
			return 0, err
		}
		inferred |= caps
	}
	if err := verifyClasses(b, image, minor, cloneCost); err != nil {
		return 0, err
	}
	if image.Functions[image.Entry].Flags&FuncClosure != 0 {
		return 0, invalid("the entry function is a closure function")
	}
	if inferred&^image.Required != 0 {
		return 0, fail(ErrCapability, "required capability omits an opcode requirement")
	}
	if image.Required&^b.supported != 0 {
		return 0, fail(ErrCapability, "host does not support required capabilities")
	}
	return image.Required, nil
}

// verifyLocalNames checks the count first, so the loop is bounded by
// Locals and MaxLocals before any entry is read.
func verifyLocalNames(b *budget, image Image, f *Function, cloneCost bool) error {
	if len(f.LocalNames) == 0 {
		return nil
	}
	if uint64(len(f.LocalNames)) != uint64(f.Locals) {
		return invalid(fmt.Sprintf("local name count %d must be 0 or the function's Locals (%d)", len(f.LocalNames), f.Locals))
	}
	if cloneCost {
		if err := b.reserveMul(uint64(len(f.LocalNames)), 4); err != nil {
			return err
		}
	}
	if err := b.chargeWork(uint64(len(f.LocalNames))); err != nil {
		return err
	}
	for i, c := range f.LocalNames {
		if uint64(c) >= uint64(len(image.Constants)) {
			return invalid(fmt.Sprintf("local name index %d for local %d is outside the %d constants", c, i, len(image.Constants)))
		}
		if image.Constants[c].Kind != String {
			return invalid(fmt.Sprintf("local name constant %d for local %d is not a string", c, i))
		}
	}
	return nil
}

// verifyParameters checks a function's parameter table (format 1.6): it is
// empty, or has one entry per parameter register and, for a variadic
// function, one more that alone carries ParamVariadic. The count is checked
// first, so the loop is bounded by the registers before any entry is read.
func verifyParameters(b *budget, image Image, f *Function, cloneCost bool) error {
	n := uint64(len(f.Parameters))
	if n == 0 {
		return nil
	}
	if n != uint64(f.Params) && n != uint64(f.Params)+1 {
		return invalid(fmt.Sprintf("parameter table has %d entries; it must have none, the function's Params (%d), or one more for a variadic parameter", n, f.Params))
	}
	if cloneCost {
		if err := b.reserveMul(n, 8); err != nil {
			return err
		}
	}
	if err := b.chargeWork(n); err != nil {
		return err
	}
	for i, p := range f.Parameters {
		if uint64(p.Name) >= uint64(len(image.Constants)) || image.Constants[p.Name].Kind != String {
			return invalid(fmt.Sprintf("name of parameter %d is not a string constant", i))
		}
		if p.Flags&^paramFlagsKnown != 0 {
			return invalid(fmt.Sprintf("parameter %d has unknown flags", i))
		}
		if variadic := p.Flags&ParamVariadic != 0; variadic != (uint64(i) == uint64(f.Params)) {
			return invalid(fmt.Sprintf("parameter %d: ParamVariadic marks exactly the entry after the %d parameter registers", i, f.Params))
		}
	}
	return nil
}

// variadic reports whether the function's parameter table ends with a
// variadic parameter.
func (f *Function) variadic() bool {
	return uint64(len(f.Parameters)) == uint64(f.Params)+1
}

// The kinds of the argument stack entries that are not arguments (format
// 1.6): the mark OpInitCall leaves for the call it starts, and the unknown
// number of arguments an OpSendUnpack sent to that call.
const (
	shapeMark   RegisterKind = Location + 1
	shapeUnpack RegisterKind = Location + 2
)

// argShape is one entry of the argument stack. ref says the argument may be
// a reference: a Location sent by reference, or a Value register that may
// hold one. fn is the callee of a shapeMark.
type argShape struct {
	kind  RegisterKind
	named bool
	ref   bool
	// late marks an argument sent as the callee declares (PushAuto,
	// PushTemp, PushResult): only the call it was sent to may take it.
	late bool
	fn   uint32
	name string
}

// flowState is what is known where an instruction starts: the registers
// defined on every path (defs), those that may hold a reference on some
// path (refs, nil when the function is not tracked) and the argument stack.
type flowState struct {
	defs  []uint64
	refs  []uint64
	stack []argShape
}

// tracksReferences reports whether a format 1.6 function can hold a
// reference in a Value register: it creates one (OpMakeRef, a by-reference
// iterator or its element), or a parameter register can arrive holding one.
// Only such functions pay for the second bit set of the flow analysis.
func tracksReferences(f *Function) bool {
	if f.Params > 0 {
		if len(f.Parameters) == 0 {
			return true
		}
		for _, p := range f.Parameters[:f.Params] {
			if p.Flags&ParamByRef != 0 {
				return true
			}
		}
	}
	for _, in := range f.Code {
		switch in.Op {
		case OpMakeRef, OpPropRef, OpStaticRef:
			return true
		case OpIterInit, OpIterNext:
			if IterFlags(in.Aux)&IterByRef != 0 {
				return true
			}
		}
	}
	return false
}

func verifyFunction(b *budget, image Image, minor uint16, index uint32, f *Function) (CapabilitySet, error) {
	n := len(f.Code)
	if err := verifyHandlers(b, f); err != nil {
		return 0, functionError(err, int(index), -1)
	}
	handlers, err := newHandlerIndex(b, f)
	if err != nil {
		return 0, err
	}
	var caps CapabilitySet
	for pc, in := range f.Code {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		safety, ok := in.Op.Safety()
		if !ok || safety.Steps == 0 || !safety.CheckCancel {
			return 0, functionError(invalid("unknown opcode"), int(index), pc)
		}
		caps |= safety.Capabilities
		if err := verifyInstruction(image, f, minor, in); err != nil {
			return 0, functionError(err, int(index), pc)
		}
		for ei, ec := 0, successorCount(in, uint32(pc), uint32(n)); ei < ec; ei++ {
			e := successor(in, uint32(pc), uint32(n), ei)
			if err := handlers.checkEdge(uint32(pc), e.pc, e.leave); err != nil {
				return 0, functionError(err, int(index), pc)
			}
		}
		if in.Op == OpEndFinally {
			if kind, ok := handlers.innermostBody(pc); !ok || kind != Finally {
				return 0, functionError(invalid("endfinally outside innermost finally body"), int(index), pc)
			}
		}
	}
	if err := verifyElementLocations(b, f); err != nil {
		return 0, functionError(err, int(index), -1)
	}
	if err := verifyFlow(b, image, minor, f, uint32(index), handlers, minor >= 9); err != nil {
		// A 1.0 to 1.8 file whose only fault is a register read after an
		// exception handler or a finally block took over needs format 1.9.
		if minor < 9 && len(f.Handlers) > 0 && errors.Is(err, ErrInvalid) && verifyFlow(b, image, minor, f, uint32(index), handlers, true) == nil {
			var fe *Error
			pc := -1
			if errors.As(err, &fe) {
				pc = fe.PC
			}
			return 0, functionError(invalid(fmt.Sprintf("register kept across an exception handler needs bytecode format 1.9, but the file declares format 1.%d", minor)), int(index), pc)
		}
		return 0, functionError(err, int(index), -1)
	}
	return caps, nil
}

// format11 reports whether a known instruction needs format 1.1: a 1.1
// opcode, a 1.1 binary operator, or the quiet Aux of Load or ArrayGet.
func format11(in Instruction) bool {
	switch in.Op {
	case OpUnary, OpCast, OpIncDec, OpArraySet, OpArrayAppend, OpArrayUnset, OpIsset, OpArrayIsset, OpUnset, OpExit, OpArrayAdd:
		return true
	case OpBinary:
		return in.Aux >= uint16(Modulo)
	case OpLoad, OpArrayGet:
		return in.Aux != 0
	}
	return false
}

// format12 reports whether a known instruction needs format 1.2: a 1.2
// opcode, or an ArraySet whose Aux names a result register.
func format12(in Instruction) bool {
	switch in.Op {
	case OpFetchDim, OpIterInit, OpIterNext, OpIterKey, OpParamDefault:
		return true
	case OpArraySet:
		return in.Aux > uint16(ValueLocation|KeyLocation)
	}
	return false
}

// format13 reports whether a known instruction needs format 1.3: a 1.3
// opcode, or a by-reference iterator instruction.
func format13(in Instruction) bool {
	switch in.Op {
	case OpBind, OpMakeRef:
		return true
	case OpIterInit, OpIterNext:
		return IterFlags(in.Aux)&IterByRef != 0
	case OpFetchDim:
		return FetchFlags(in.Aux).Use() == FetchForRef
	}
	return false
}

// format14 reports whether a known instruction needs format 1.4: a list
// read (ArrayGet with ArrayGetList) or a FetchDim from a Value container.
func format14(in Instruction) bool {
	switch in.Op {
	case OpArrayGet:
		return in.Aux == ArrayGetList
	case OpFetchDim:
		return FetchFlags(in.Aux)&FetchContainerValue != 0
	}
	return false
}

// format15 reports whether a known instruction needs format 1.5: a 1.5
// opcode, or an ArrayIsset that tests for empty.
func format15(in Instruction) bool {
	switch in.Op {
	case OpFetchConst, OpDeclareConst, OpBeginSilence, OpEndSilence, OpMatchError:
		return true
	case OpArrayIsset:
		return in.Aux == ArrayIssetEmpty
	}
	return false
}

// format16 reports whether a known instruction needs format 1.6: a 1.6
// opcode, or a Push that reads a variable itself.
func format16(in Instruction) bool {
	switch in.Op {
	case OpBindGlobal, OpStaticInit, OpBindStatic, OpRecvVariadic, OpInitCall, OpSendUnpack, OpCallPending:
		return true
	case OpPush:
		return in.Aux != 0
	}
	return false
}

// is17 reports whether op is a format 1.7 opcode.
func is17(op Opcode) bool { return op >= OpDeclareClass && op <= OpArgByRef }

// is18 reports whether op is a format 1.8 opcode. The 1.8 opcodes share
// the operand roles (Roles17) and the data-flow rules of the 1.7 ones.
func is18(op Opcode) bool { return op >= OpClosure && op <= OpArgByRef }

func verifyInstruction(image Image, f *Function, minor uint16, in Instruction) error {
	// A format 1.7 opcode in an older file is reported as that: such a
	// file has no class table, so its operands could never be valid.
	if minor < 8 && is18(in.Op) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.8, but the file declares format 1.%d", minor))
	}
	if minor < 7 && is17(in.Op) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.7, but the file declares format 1.%d", minor))
	}
	if err := checkOperands(image, f, in); err != nil {
		return err
	}
	if minor >= 8 && (in.Op == OpCall || in.Op == OpInitCall) && image.Functions[in.B].Flags&FuncClosure != 0 {
		return invalid("a closure function is entered only through its Closure object, not by a direct call")
	}
	if minor < 10 && in.Op == OpLeave && in.Aux != 0 {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.10, but the file declares format 1.%d", minor))
	}
	if minor < 8 && in.Op == OpReturn && in.Aux != 0 {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.8, but the file declares format 1.%d", minor))
	}
	if minor < 7 && format17(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.7, but the file declares format 1.%d", minor))
	}
	if minor < 6 && format16(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.6, but the file declares format 1.%d", minor))
	}
	if minor < 5 && format15(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.5, but the file declares format 1.%d", minor))
	}
	if minor < 4 && format14(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.4, but the file declares format 1.%d", minor))
	}
	if minor < 3 && format13(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.3, but the file declares format 1.%d", minor))
	}
	if minor < 2 && format12(in) {
		return invalid(fmt.Sprintf("instruction needs bytecode format 1.2, but the file declares format 1.%d", minor))
	}
	if minor == 0 && format11(in) {
		return invalid("instruction needs bytecode format 1.1, but the file declares format 1.0")
	}
	// Checked after the format: only a format 1.6 file has a parameter table.
	if in.Op == OpRecvVariadic && !f.variadic() {
		return invalid("recv-variadic in a function whose parameter table does not end with a variadic parameter")
	}
	return nil
}

func checkOperands(image Image, f *Function, in Instruction) error {
	reg := func(n uint32, k RegisterKind) bool {
		return uint64(n) < uint64(len(f.Registers)) && f.Registers[n] == k
	}
	zero := in.B == 0 && in.C == 0
	switch in.Op {
	case OpBinary, OpInclude, OpLoad, OpArrayGet, OpUnary, OpCast, OpIncDec, OpExit, OpArraySet, OpArrayAppend, OpArrayUnset, OpArrayAdd,
		OpFetchDim, OpIterInit, OpIterNext, OpIterKey, OpBind, OpArrayIsset, OpFetchConst, OpPush, OpReturn, OpLeave:
	default:
		if is17(in.Op) {
			return checkOperands17(image, f, in)
		}
		if in.Aux != 0 {
			return invalid("unused aux field")
		}
	}
	switch in.Op {
	case OpCheckpoint, OpEndFinally:
		if in.Aux != 0 || in.A != 0 || in.B != 0 || in.C != 0 {
			return invalid("unused operands")
		}
	case OpConst:
		if !reg(in.A, Value) || in.C != 0 || uint64(in.B) >= uint64(len(image.Constants)) {
			return invalid("invalid const operands")
		}
	case OpMove:
		if in.C != 0 || uint64(in.A) >= uint64(len(f.Registers)) || uint64(in.B) >= uint64(len(f.Registers)) || f.Registers[in.A] != f.Registers[in.B] {
			return invalid("invalid move operands")
		}
	case OpLocal:
		if !reg(in.A, Location) || in.C != 0 || in.B >= f.Locals {
			return invalid("invalid local operands")
		}
	case OpLoad:
		if in.Aux > 1 || !reg(in.A, Value) || !reg(in.B, Location) || in.C != 0 {
			return invalid("invalid load operands")
		}
	case OpStore:
		if !reg(in.A, Location) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid store operands")
		}
	case OpBinary:
		if in.Aux < uint16(Add) || in.Aux > uint16(BoolXor) || !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
			return invalid("invalid binary operands")
		}
	case OpJump:
		if !zero || uint64(in.A) >= uint64(len(f.Code)) {
			return invalid("invalid control target")
		}
	case OpLeave:
		switch {
		case uint64(in.A) >= uint64(len(f.Code)):
			return invalid("invalid control target")
		case in.Aux == 0 && !zero, in.Aux == LeaveCarry && (in.B != 0 || !reg(in.C, Value)), in.Aux > LeaveCarry:
			return invalid("invalid leave operands")
		}
	case OpBranch:
		if !reg(in.A, Value) || uint64(in.B) >= uint64(len(f.Code)) || uint64(in.C) >= uint64(len(f.Code)) {
			return invalid("invalid branch operands")
		}
	case OpPush:
		if uint64(in.A) >= uint64(len(f.Registers)) || in.C != 0 {
			return invalid("invalid push operands")
		}
		wantsLocation := in.Aux == uint16(PushLoad) || in.Aux == uint16(PushAuto)
		wantsValue := in.Aux == uint16(PushTemp) || in.Aux == uint16(PushResult) || in.Aux == uint16(PushElement)
		if in.Aux > uint16(PushElement) || (wantsLocation && f.Registers[in.A] != Location) || (wantsValue && f.Registers[in.A] != Value) {
			return invalid("invalid push operands: Aux is 0, PushLoad or PushAuto, which need a Location in A, or PushTemp, PushResult or PushElement, which need a Value")
		}
		if in.B != 0 && (uint64(in.B-1) >= uint64(len(image.Constants)) || image.Constants[in.B-1].Kind != String) {
			return invalid("invalid named argument")
		}
	case OpCall:
		if in.B >= uint32(len(image.Functions)) || !reg(in.A, image.Functions[in.B].ReturnKind) || in.C != image.Functions[in.B].Params || in.C > f.MaxStack {
			return invalid("invalid direct call operands")
		}
	case OpCallValue:
		if !reg(in.A, Value) || !reg(in.B, Value) || in.C > f.MaxStack {
			return invalid("invalid dynamic call operands")
		}
	case OpHostCall:
		if !reg(in.A, Value) || in.C > f.MaxStack || uint64(in.B) >= uint64(len(image.Constants)) || image.Constants[in.B].Kind != String {
			return invalid("invalid host call operands")
		}
	case OpReturn:
		if in.Aux > ReturnNotice || !reg(in.A, Value) || f.ReturnKind != Value || !zero {
			return invalid("invalid return operands")
		}
	case OpReturnRef:
		if !reg(in.A, Location) || f.ReturnKind != Location || !zero {
			return invalid("invalid reference return operands")
		}
	case OpThrow, OpOutput:
		if !reg(in.A, Value) || !zero {
			return invalid("invalid value operation operands")
		}
	case OpNewArray:
		if !reg(in.A, Value) || !zero {
			return invalid("invalid new-array operands")
		}
	case OpArrayGet:
		if in.Aux > ArrayGetList || !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
			return invalid("invalid array-get operands")
		}
	case OpInclude:
		if in.Aux > 3 || !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid include operands")
		}
	case OpEval, OpShell:
		if !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid dynamic operation operands")
		}
	case OpDrop:
		if uint64(in.A) >= uint64(len(f.Registers)) || !zero {
			return invalid("invalid drop operands")
		}
	case OpUnary:
		if in.Aux < uint16(Not) || in.Aux > uint16(Decrement) || !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid unary operands: A and B must be Value registers, C zero, Aux Not to Decrement")
		}
	case OpCast:
		if in.Aux < uint16(CastBool) || in.Aux > uint16(CastObject) || !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid cast operands: A and B must be Value registers, C zero, Aux CastBool to CastObject")
		}
	case OpIncDec:
		if in.Aux < uint16(PreInc) || in.Aux > uint16(PostDec) || !reg(in.A, Value) || !reg(in.B, Location) || in.C != 0 {
			return invalid("invalid increment operands: A must be a Value, B a Location, C zero, Aux PreInc to PostDec")
		}
	case OpArraySet:
		key, val := Value, Value
		if ArrayWriteFlags(in.Aux)&KeyLocation != 0 {
			key = Location
		}
		if ArrayWriteFlags(in.Aux)&ValueLocation != 0 {
			val = Location
		}
		if in.Aux&(1<<2) != 0 || !reg(in.A, Location) || !reg(in.B, key) || !reg(in.C, val) {
			return invalid("invalid array-set operands: A must be a Location, B and C Value registers, or Locations where Aux has KeyLocation or ValueLocation; Aux bit 2 is zero")
		}
		if r, ok := ArraySetResult(in.Aux); ok && !reg(r, Value) {
			return invalid("invalid array-set result: Aux bits 3 to 15 must be zero or the index of a Value register plus one")
		}
	case OpArrayAppend:
		val := Value
		if ArrayWriteFlags(in.Aux)&ValueLocation != 0 {
			val = Location
		}
		if in.Aux > uint16(ValueLocation) || !reg(in.A, Location) || !reg(in.B, val) || in.C != 0 {
			return invalid("invalid array-append operands: A must be a Location, B a Value (a Location where Aux is ValueLocation), C zero")
		}
	case OpArrayUnset:
		key := Value
		if ArrayWriteFlags(in.Aux) == KeyLocation {
			key = Location
		}
		if (in.Aux != 0 && ArrayWriteFlags(in.Aux) != KeyLocation) || !reg(in.A, Location) || !reg(in.B, key) || in.C != 0 {
			return invalid("invalid array-unset operands: A must be a Location, B a Value (a Location where Aux is KeyLocation), C zero")
		}
	case OpArrayAdd:
		flags := ArrayAddFlags(in.Aux)
		key := Value
		if flags&AddKeyLocation != 0 {
			key = Location
		}
		switch {
		case in.Aux > uint16(AddKey|AddKeyLocation) || flags == AddKeyLocation || !reg(in.A, Value) || !reg(in.B, Value):
		case flags&AddKey == 0 && in.C == 0, flags&AddKey != 0 && reg(in.C, key):
			return nil
		}
		return invalid("invalid array-add operands: A and B must be Value registers, C a key Value (a Location where Aux has AddKeyLocation) when Aux has AddKey, otherwise zero")
	case OpIsset:
		if !reg(in.A, Value) || !reg(in.B, Location) || in.C != 0 {
			return invalid("invalid isset operands: A must be a Value, B a Location, C zero")
		}
	case OpArrayIsset:
		if in.Aux > ArrayIssetEmpty || !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
			return invalid("invalid array-isset operands: A, B and C must be Value registers, Aux 0 or ArrayIssetEmpty")
		}
	case OpUnset:
		if !reg(in.A, Location) || !zero {
			return invalid("invalid unset operands: A must be a Location, B and C zero")
		}
	case OpExit:
		if !zero || in.Aux > 1 || (in.Aux == 0 && in.A != 0) || (in.Aux == 1 && !reg(in.A, Value)) {
			return invalid("invalid exit operands: Aux 0 needs A, B and C zero, Aux 1 a status Value in A and B, C zero")
		}
	case OpFetchDim:
		flags := FetchFlags(in.Aux)
		key := Value
		if flags&FetchKeyLocation != 0 {
			key = Location
		}
		container := Location
		if flags&FetchContainerValue != 0 {
			container = Value
		}
		switch {
		case flags&^fetchKnown != 0 || flags.Mode() > FetchUnset:
		case flags.Use() == FetchForRef && flags.Mode() != FetchWrite:
		case flags&FetchContainerValue != 0 && flags.Use() != FetchForRef:
		case flags.Mode() == FetchUnset && (flags.Use() != 0 || flags&FetchAppend != 0):
		case !reg(in.A, Location) || !reg(in.B, container):
		case flags&FetchAppend != 0 && flags&FetchKeyLocation == 0 && in.C == 0, flags&FetchAppend == 0 && reg(in.C, key):
			return nil
		}
		return invalid("invalid fetch-dim operands: A and B must be Locations (B a Value where Aux has FetchContainerValue, which needs FetchForRef), C a key Value (a Location where Aux has FetchKeyLocation, zero with FetchAppend); Aux is a mode, FetchKeyLocation, FetchAppend and a use, and FetchUnset takes neither FetchAppend nor a use")
	case OpIterInit:
		subject := reg(in.B, Value)
		if in.Aux == uint16(IterByRef) {
			subject = subject || reg(in.B, Location)
		}
		if (in.Aux != 0 && in.Aux != uint16(IterByRef)) || !reg(in.A, Value) || !subject || uint64(in.C) >= uint64(len(f.Code)) {
			return invalid("invalid iter-init operands: A and B must be Value registers (B may be a Location where Aux is IterByRef), C an instruction index")
		}
	case OpIterNext:
		dest := Value
		if IterFlags(in.Aux)&IterLocation != 0 {
			dest = Location
		}
		if in.Aux > uint16(IterLocation|IterByRef) || !reg(in.A, Value) || !reg(in.B, dest) || uint64(in.C) >= uint64(len(f.Code)) {
			return invalid("invalid iter-next operands: A must be a Value, B a Value (a Location where Aux has IterLocation), C an instruction index; Aux is IterLocation and IterByRef")
		}
	case OpIterKey:
		dest := Value
		if IterFlags(in.Aux) == IterLocation {
			dest = Location
		}
		if in.Aux > uint16(IterLocation) || !reg(in.A, dest) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid iter-key operands: A must be a Value (a Location where Aux is IterLocation), B a Value, C zero")
		}
	case OpParamDefault:
		if in.A >= f.Params || uint64(in.B) >= uint64(len(f.Code)) || in.C != 0 {
			return invalid("invalid param-default operands: A must be a parameter register, B an instruction index, C zero")
		}
	case OpBind:
		source := Location
		if in.Aux == uint16(BindValue) {
			source = Value
		}
		if in.Aux > uint16(BindValue) || !reg(in.A, Location) || !reg(in.B, source) || in.C != 0 {
			return invalid("invalid bind operands: A must be a Location, B a Location (a Value where Aux is BindValue), C zero")
		}
	case OpMakeRef:
		if !reg(in.A, Value) || !reg(in.B, Location) || in.C != 0 {
			return invalid("invalid make-ref operands: A must be a Value, B a Location, C zero")
		}
	case OpFetchConst:
		str := func(n uint32) bool {
			return uint64(n) < uint64(len(image.Constants)) && image.Constants[n].Kind == String
		}
		flags := FetchConstFlags(in.Aux)
		switch {
		case flags&^fetchConstKnown != 0, !reg(in.A, Value), !str(in.B), !str(in.C):
		case flags&FetchConstDefault != 0 && (flags&FetchConstNoCache == 0 || in.A >= f.Params):
		default:
			return nil
		}
		return invalid("invalid fetch-constant operands: A must be a Value (a parameter register with FetchConstDefault, which needs FetchConstNoCache), B and C string constants; Aux is FetchConstFlags")
	case OpDeclareConst:
		if !reg(in.A, Value) || in.C != 0 || uint64(in.B) >= uint64(len(image.Constants)) || image.Constants[in.B].Kind != String {
			return invalid("invalid declare-constant operands: A must be a Value, B a string constant, C zero")
		}
	case OpBeginSilence, OpEndSilence, OpMatchError:
		if !reg(in.A, Value) || !zero {
			return invalid("invalid value operation operands")
		}
	case OpBindGlobal:
		if !reg(in.A, Location) || in.C != 0 || uint64(in.B) >= uint64(len(image.Constants)) || image.Constants[in.B].Kind != String {
			return invalid("invalid bind-global operands: A must be a Location, B a string constant, C zero")
		}
	case OpStaticInit:
		if !reg(in.A, Location) || uint64(in.B) >= uint64(len(f.Code)) || in.C != 0 {
			return invalid("invalid static-init operands: A must be a Location, B an instruction index, C zero")
		}
	case OpBindStatic:
		if !reg(in.A, Location) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid bind-static operands: A must be a Location, B a Value, C zero")
		}
	case OpRecvVariadic:
		if !reg(in.A, Value) || !zero {
			return invalid("invalid recv-variadic operands: A must be a Value, B and C zero")
		}
	case OpInitCall:
		if in.A != 0 || in.C != 0 || uint64(in.B) >= uint64(len(image.Functions)) {
			return invalid("invalid init-call operands: B must be a function index, A and C zero")
		}
	case OpSendUnpack:
		if uint64(in.A) >= uint64(len(f.Registers)) || !zero {
			return invalid("invalid send-unpack operands: A must be a register, B and C zero")
		}
	case OpCallPending:
		if uint64(in.A) >= uint64(len(f.Registers)) || !zero {
			return invalid("invalid call-pending operands: A must be a register, B and C zero")
		}
	default:
		return invalid("unknown opcode")
	}
	return nil
}

type edge struct {
	pc    uint32
	leave bool
}

func successorCount(in Instruction, pc, n uint32) int {
	switch in.Op {
	case OpReturn, OpReturnRef, OpThrow, OpEndFinally, OpExit, OpMatchError:
		return 0
	case OpBranch, OpIterInit, OpIterNext, OpParamDefault, OpStaticInit, OpNew:
		return 2
	default:
		return 1
	}
}
func successor(in Instruction, pc, n uint32, which int) edge {
	switch in.Op {
	case OpJump:
		return edge{pc: in.A}
	case OpLeave:
		return edge{pc: in.A, leave: true}
	case OpBranch:
		if which == 0 {
			return edge{pc: in.B}
		}
		return edge{pc: in.C}
	case OpIterInit, OpIterNext, OpNew:
		// Edge 0 falls through; edge 1 is taken for a subject that cannot
		// be iterated, when the iteration is over, or for a class without
		// a constructor.
		if which == 1 {
			return edge{pc: in.C}
		}
		return edge{pc: pc + 1}
	case OpParamDefault, OpStaticInit:
		if which == 1 {
			return edge{pc: in.B}
		}
		return edge{pc: pc + 1}
	default:
		return edge{pc: pc + 1}
	}
}

func verifyHandlers(b *budget, f *Function) error {
	// Handlers are checked in table order, each on its own and then against
	// the earlier ones: the first handler that fails its own checks bounds
	// the nesting check, and a crossing among the earlier handlers wins.
	bad, badErr := len(f.Handlers), error(nil)
	for i, h := range f.Handlers {
		if err := b.chargeWork(1); err != nil {
			return err
		}
		if badErr = checkHandler(f, i, h); badErr != nil {
			bad = i
			break
		}
	}
	earlier := *f
	earlier.Handlers = f.Handlers[:bad]
	if err := verifyHandlerNesting(b, &earlier); err != nil {
		return err
	}
	return badErr
}

// checkHandler validates handler i alone and its order after handler i-1.
func checkHandler(f *Function, i int, h Handler) error {
	if h.Start >= h.End || h.End > uint32(len(f.Code)) || h.Target == 0 || h.Target >= h.BodyEnd || h.BodyEnd > uint32(len(f.Code)) || h.StackDepth != 0 ||
		(h.Target < h.End && h.BodyEnd > h.Start) {
		return invalid("invalid handler ranges")
	}
	if h.Kind == Catch {
		if uint64(h.Register) >= uint64(len(f.Registers)) || f.Registers[h.Register] != Value {
			return invalid("invalid catch register")
		}
	} else if h.Kind == Finally {
		if h.Register != 0 {
			return invalid("finally has register")
		}
	} else {
		return invalid("unknown handler kind")
	}
	if i > 0 {
		p := f.Handlers[i-1]
		if h.Start < p.Start || (h.Start == p.Start && h.End >= p.End) {
			return invalid("noncanonical handler order")
		}
	}
	return nil
}

// handlerSpan is a protected range or a body of handler h.
type handlerSpan struct {
	start, end uint32
	h          int32
	body       bool
}

func sortSpans(spans []handlerSpan) {
	slices.SortFunc(spans, func(x, y handlerSpan) int {
		return cmp.Or(cmp.Compare(x.start, y.start), cmp.Compare(y.end, x.end))
	})
}

// verifyHandlerNesting requires two handlers to be siblings with disjoint
// regions, or the whole inner handler to belong to one region of its outer
// handler. This permits try / catch inside finally while excluding
// cross-scope exception transfers. Equivalently, all protected ranges and
// bodies form a laminar family, and a handler's range and body have the same
// innermost enclosing region. A sorted sweep checks this in O(h log h).
func verifyHandlerNesting(b *budget, f *Function) error {
	h := len(f.Handlers)
	if h == 0 {
		return nil
	}
	// 2h spans of 16 bytes, and 2h int32 parents and open entries.
	if err := b.reserveMul(uint64(h), 2*16+2*4+2*4); err != nil {
		return err
	}
	if err := b.chargeWork(uint64(2 * h)); err != nil {
		return err
	}
	spans := make([]handlerSpan, 0, 2*h)
	for i, x := range f.Handlers {
		spans = append(spans, handlerSpan{x.Start, x.End, int32(i), false}, handlerSpan{x.Target, x.BodyEnd, int32(i), true})
	}
	sortSpans(spans)
	parent := make([]int32, 2*h) // enclosing span index per range (2h) and body (2h+1)
	open := make([]int32, 0, 2*h)
	for i, s := range spans {
		for len(open) > 0 && spans[open[len(open)-1]].end <= s.start {
			open = open[:len(open)-1]
		}
		enclosing := int32(-1)
		if len(open) > 0 {
			top := spans[open[len(open)-1]]
			if s.end > top.end || s.start == top.start && s.end == top.end {
				return invalid("crossing handler regions")
			}
			enclosing = open[len(open)-1]
		}
		slot := 2 * int(s.h)
		if s.body {
			slot++
		}
		parent[slot] = enclosing
		open = append(open, int32(i))
	}
	for i := 0; i < h; i++ {
		if parent[2*i] != parent[2*i+1] {
			return invalid("crossing handler regions")
		}
	}
	return nil
}

// handlerIndex answers the handler questions about one pc in constant time.
// verifyHandlerNesting has proved the regions laminar, so the regions that
// contain a pc form a chain and its innermost member decides each question.
type handlerIndex struct {
	f *Function
	// body is the innermost handler whose body contains pc; enter is the
	// innermost whose protected range contains both pc-1 and pc; finally is
	// the innermost Finally handler whose protected range contains pc; rng
	// is the innermost handler of either kind whose protected range
	// contains pc.
	body, enter, finally, rng []int32
}

func newHandlerIndex(b *budget, f *Function) (*handlerIndex, error) {
	n, h := len(f.Code), len(f.Handlers)
	x := &handlerIndex{f: f}
	if h == 0 {
		return x, nil
	}
	finallies := 0
	for _, k := range f.Handlers {
		if k.Kind == Finally {
			finallies++
		}
	}
	// The index itself (a pointer and three slice headers, within 128
	// bytes), an int32 array per pc for body and enter, and for finally
	// only when there is one, and per span 16 bytes plus a 4-byte open
	// entry.
	arrays := uint64(3)
	if finallies > 0 {
		arrays++
	}
	if err := b.reserve(128); err != nil {
		return nil, err
	}
	if err := b.reserveMul(uint64(n), arrays*4); err != nil {
		return nil, err
	}
	if err := b.reserveMul(uint64(3*h+finallies), 16+4); err != nil {
		return nil, err
	}
	if err := b.chargeWork(4*uint64(n) + 4*uint64(h)); err != nil {
		return nil, err
	}
	bodies := make([]handlerSpan, 0, h)
	ranges := make([]handlerSpan, 0, h)
	finals := make([]handlerSpan, 0, finallies)
	for i, k := range f.Handlers {
		bodies = append(bodies, handlerSpan{k.Target, k.BodyEnd, int32(i), true})
		ranges = append(ranges, handlerSpan{k.Start, k.End, int32(i), false})
		if k.Kind == Finally {
			finals = append(finals, handlerSpan{k.Start, k.End, int32(i), false})
		}
	}
	x.body = innermostSpans(bodies, n, false)
	x.enter = innermostSpans(ranges, n, true)
	x.rng = innermostSpans(append([]handlerSpan(nil), ranges...), n, false)
	if finallies > 0 {
		x.finally = innermostSpans(finals, n, false)
	}
	return x, nil
}

// innermostSpans sweeps laminar spans once. out[pc] is the handler of the
// innermost span containing pc, or, with before, of the innermost span
// containing pc that starts before pc; -1 when there is none.
func innermostSpans(spans []handlerSpan, n int, before bool) []int32 {
	sortSpans(spans)
	out := make([]int32, n)
	open := make([]int32, 0, len(spans)) // indexes into spans
	innermost := func() int32 {
		if len(open) == 0 {
			return -1
		}
		return spans[open[len(open)-1]].h
	}
	next := 0
	for pc := 0; pc < n; pc++ {
		for len(open) > 0 && spans[open[len(open)-1]].end <= uint32(pc) {
			open = open[:len(open)-1]
		}
		if before {
			out[pc] = innermost()
		}
		for next < len(spans) && spans[next].start == uint32(pc) {
			open = append(open, int32(next))
			next++
		}
		if !before {
			out[pc] = innermost()
		}
	}
	return out
}

func (x *handlerIndex) checkEdge(from, to uint32, leave bool) error {
	f := x.f
	if to >= uint32(len(f.Code)) {
		return invalid("control flow falls off function")
	}
	if len(f.Handlers) == 0 {
		return nil
	}
	inBody := func(h int32, pc uint32) bool { return pc >= f.Handlers[h].Target && pc < f.Handlers[h].BodyEnd }
	inRange := func(h int32, pc uint32) bool { return pc >= f.Handlers[h].Start && pc < f.Handlers[h].End }
	if h := x.body[to]; h >= 0 && !inBody(h, from) {
		return invalid("normal control enters handler body")
	}
	if h := x.body[from]; h >= 0 && !leave && !inBody(h, to) {
		return invalid("normal control leaves handler body")
	}
	if h := x.enter[to]; h >= 0 && !inRange(h, from) {
		return invalid("control enters protected range after start")
	}
	if x.finally == nil {
		return nil // no finally range to leave
	}
	if h := x.finally[from]; h >= 0 && !leave && !inRange(h, to) {
		return invalid("control leaves finally range without leave")
	}
	return nil
}

// innermostBody is the kind of the innermost handler whose body contains pc.
func (x *handlerIndex) innermostBody(pc int) (HandlerKind, bool) {
	if len(x.f.Handlers) == 0 || x.body[pc] < 0 {
		return 0, false
	}
	return x.f.Handlers[x.body[pc]].Kind, true
}

// leaveFirst reports the Finally handler a Leave at pc to target runs
// first: the innermost one whose range holds pc and not target. It is -1
// when the transfer leaves no finally range and is a plain jump.
func (x *handlerIndex) leaveFirst(pc, target uint32) int32 {
	if x.finally == nil {
		return -1
	}
	h := x.finally[pc]
	if h < 0 || target >= x.f.Handlers[h].Start && target < x.f.Handlers[h].End {
		return -1
	}
	return h
}

// leaveTargets works out, for every EndFinally, the targets of the Leaves
// it completes. A Leave runs the finally block leaveFirst names; that
// block's EndFinally goes on like a Leave from its own pc, as the VM does:
// through the next finally range around it that the target is outside of,
// or to the target.
func (x *handlerIndex) leaveTargets(b *budget) (map[uint32][]uint32, error) {
	f := x.f
	if x.finally == nil {
		return nil, nil
	}
	if err := b.chargeWork(2 * uint64(len(f.Code))); err != nil {
		return nil, err
	}
	ends := map[int32][]uint32{} // the EndFinally instructions of each handler body
	type pending struct {
		h int32
		t uint32
	}
	var work []pending
	seen := map[pending]bool{}
	add := func(h int32, t uint32) error {
		p := pending{h, t}
		if seen[p] {
			return nil
		}
		if err := b.reserve(48); err != nil {
			return err
		}
		seen[p] = true
		work = append(work, p)
		return nil
	}
	for pc, in := range f.Code {
		switch in.Op {
		case OpEndFinally:
			if h := x.body[pc]; h >= 0 {
				if err := b.reserve(8); err != nil {
					return nil, err
				}
				ends[h] = append(ends[h], uint32(pc))
			}
		case OpLeave:
			if h := x.leaveFirst(uint32(pc), in.A); h >= 0 {
				if err := add(h, in.A); err != nil {
					return nil, err
				}
			}
		}
	}
	out := map[uint32][]uint32{}
	for len(work) > 0 {
		p := work[len(work)-1]
		work = work[:len(work)-1]
		for _, e := range ends[p.h] {
			if err := b.chargeWork(1); err != nil {
				return nil, err
			}
			if next := x.leaveFirst(e, p.t); next >= 0 {
				if err := add(next, p.t); err != nil {
					return nil, err
				}
				continue
			}
			if err := b.reserve(8); err != nil {
				return nil, err
			}
			out[e] = append(out[e], p.t)
		}
	}
	return out, nil
}

// verifyFlow checks the registers and the argument stack along every path.
// With keep (format 1.9) a handler's entry has the registers that are
// defined before every instruction of its protected range, and a Leave
// carries the registers that are still defined when the last finally block
// it runs ends; before 1.9 a handler's entry and a Leave's target have none.
func verifyFlow(b *budget, image Image, minor uint16, f *Function, fi uint32, handlers *handlerIndex, keep bool) error {
	n, nr := len(f.Code), len(f.Registers)
	keep = keep && len(f.Handlers) > 0
	words := (nr + 63) / 64
	// Format 1.6 keeps references out of the by-value parameters of a
	// direct call; only a function that can hold one tracks them. A call's
	// result is never a reference: Return yields the value of one.
	track := minor >= 6 && tracksReferences(f)
	stateWords := words
	if track {
		stateWords = 2 * words
	}
	if err := b.reserve(uint64(n)*17 + uint64(stateWords)*16 + uint64(f.MaxStack)*32 + 256); err != nil {
		return err
	}
	states := make([]*flowState, n)
	queue := make([]int, n)
	inQ := make([]bool, n)
	qHead, qTail, qCount := 0, 0, 0
	var scratchDefs, scratchRefs []uint64
	var scratchStack []argShape
	enqueue := func(pc int) {
		if !inQ[pc] {
			queue[qTail] = pc
			qTail = (qTail + 1) % n
			qCount++
			inQ[pc] = true
		}
	}
	merge := func(pc int, defs, refs []uint64, stack []argShape) (bool, error) {
		if pc < 0 || pc >= n {
			return false, invalid("control flow falls off function")
		}
		if err := b.chargeWork(uint64(stateWords) + uint64(len(stack))); err != nil {
			return false, err
		}
		old := states[pc]
		if old == nil {
			if err := b.reserve(uint64(stateWords)*8 + uint64(len(stack))*32 + 64); err != nil {
				return false, err
			}
			old = &flowState{defs: make([]uint64, words), stack: append([]argShape(nil), stack...)}
			copy(old.defs, defs)
			if track {
				old.refs = append([]uint64(nil), refs...)
			}
			states[pc] = old
			enqueue(pc)
			return true, nil
		}
		if len(old.stack) != len(stack) {
			return false, invalid("argument stack shape differs at control-flow join")
		}
		changed := false
		for i := range stack {
			if old.stack[i].kind != stack[i].kind || old.stack[i].named != stack[i].named || old.stack[i].late != stack[i].late || old.stack[i].fn != stack[i].fn || len(old.stack[i].name) != len(stack[i].name) {
				return false, invalid("argument stack shape differs at control-flow join")
			}
			for off := 0; off < len(stack[i].name); {
				end := chunkEnd(off, len(stack[i].name), 32768)
				if err := b.chargeWork(uint64(end - off)); err != nil {
					return false, err
				}
				if old.stack[i].name[off:end] != stack[i].name[off:end] {
					return false, invalid("argument stack shape differs at control-flow join")
				}
				off = end
			}
			if stack[i].ref && !old.stack[i].ref {
				old.stack[i].ref = true
				changed = true
			}
		}
		for i := range old.defs {
			next := old.defs[i] & defs[i]
			if next != old.defs[i] {
				old.defs[i] = next
				changed = true
			}
		}
		for i := range old.refs {
			next := old.refs[i] | refs[i]
			if next != old.refs[i] {
				old.refs[i] = next
				changed = true
			}
		}
		if changed {
			enqueue(pc)
		}
		return changed, nil
	}
	seed := make([]uint64, words)
	var seedRefs []uint64
	if track {
		seedRefs = make([]uint64, words)
	}
	for i := uint32(0); i < f.Params; i++ {
		seed[i/64] |= uint64(1) << (i % 64)
		// A parameter register may arrive holding a reference when the
		// parameter is declared by reference, or nothing is declared.
		if track && (len(f.Parameters) == 0 || f.Parameters[i].Flags&ParamByRef != 0) {
			seedRefs[i/64] |= uint64(1) << (i % 64)
		}
	}
	if _, err := merge(0, seed, seedRefs, nil); err != nil {
		return err
	}
	if track {
		clear(seedRefs)
	}
	// conts are, per EndFinally, the targets of the Leaves it completes.
	var conts map[uint32][]uint32
	if keep {
		var err error
		if conts, err = handlers.leaveTargets(b); err != nil {
			return err
		}
	}
	// carries are the targets of the Leaves that carry a value (format
	// 1.10), with the register they write there. Every Leave to one target
	// carries the same register or none, so the continuation of a finally
	// block knows which register it defines.
	var carries map[uint32]uint32
	for pc := range f.Code {
		if in := f.Code[pc]; in.Op == OpLeave && in.Aux == LeaveCarry {
			if carries == nil {
				carries = map[uint32]uint32{}
			}
			carries[in.A] = in.C
		}
	}
	if carries != nil {
		if err := b.chargeWork(uint64(n)); err != nil {
			return err
		}
		for pc := range f.Code {
			in := f.Code[pc]
			if in.Op != OpLeave {
				continue
			}
			if c, ok := carries[in.A]; ok && (in.Aux != LeaveCarry || in.C != c) {
				return &Error{Kind: ErrInvalid, Offset: -1, Function: int(fi), PC: pc, Detail: "leaves to one target carry different registers"}
			}
		}
	}
	carryDefs, carryRefs := make([]uint64, words), make([]uint64, len(seedRefs))
	for _, h := range f.Handlers {
		if keep {
			// The entry state comes from the instructions of the range.
			break
		}
		if err := b.chargeWork(uint64(words) + 1); err != nil {
			return err
		}
		clear(seed)
		if h.Kind == Catch {
			seed[h.Register/64] |= uint64(1) << (h.Register % 64)
		}
		if _, err := merge(int(h.Target), seed, seedRefs, nil); err != nil {
			return err
		}
	}
	for qCount > 0 {
		if err := b.chargeWork(1); err != nil {
			return err
		}
		pc := queue[qHead]
		qHead = (qHead + 1) % n
		qCount--
		inQ[pc] = false
		st := states[pc]
		if scratchDefs == nil {
			scratchDefs = make([]uint64, words)
			scratchStack = make([]argShape, int(f.MaxStack))
			if track {
				scratchRefs = make([]uint64, words)
			}
		}
		copy(scratchDefs, st.defs)
		copy(scratchRefs, st.refs)
		depth := copy(scratchStack, st.stack)
		stack := scratchStack[:depth]
		in := f.Code[pc]
		if keep {
			// An instruction of a protected range can pass control to the
			// innermost handler (a throw) and to the innermost finally
			// block (a Return or a Leave), before it wrote anything.
			hr := handlers.rng[pc]
			hf := int32(-1)
			if handlers.finally != nil {
				hf = handlers.finally[pc]
			}
			targets := [2]int32{hr, hf}
			if hf == hr {
				targets[1] = -1
			}
			for _, hi := range targets {
				if hi < 0 {
					continue
				}
				h := f.Handlers[hi]
				var had uint64
				if h.Kind == Catch {
					had = scratchDefs[h.Register/64] & (uint64(1) << (h.Register % 64))
					scratchDefs[h.Register/64] |= uint64(1) << (h.Register % 64)
				}
				_, err := merge(int(h.Target), scratchDefs, scratchRefs, nil)
				if h.Kind == Catch && had == 0 {
					scratchDefs[h.Register/64] &^= uint64(1) << (h.Register % 64)
				}
				if err != nil {
					return flowError(err, fi, pc)
				}
			}
		}
		read := func(r uint32) error {
			if scratchDefs[r/64]&(uint64(1)<<(r%64)) == 0 {
				return invalid("register read before definition")
			}
			return nil
		}
		invalidPC := func(detail string) error {
			return &Error{Kind: ErrInvalid, Offset: -1, Function: int(fi), PC: pc, Detail: detail}
		}
		isRef := func(r uint32) bool { return track && scratchRefs[r/64]&(uint64(1)<<(r%64)) != 0 }
		setRef := func(r uint32) {
			if track {
				scratchRefs[r/64] |= uint64(1) << (r % 64)
			}
		}
		dropRef := func(r uint32) {
			if track {
				scratchRefs[r/64] &^= uint64(1) << (r % 64)
			}
		}
		// write defines r with a value that is not a reference.
		write := func(r uint32) {
			scratchDefs[r/64] |= uint64(1) << (r % 64)
			dropRef(r)
		}
		clear := func(r uint32) {
			scratchDefs[r/64] &^= uint64(1) << (r % 64)
			dropRef(r)
		}
		// push adds an entry to the argument stack.
		push := func(a argShape) error {
			if len(stack) >= int(f.MaxStack) {
				return invalidPC("argument stack exceeds frame limit")
			}
			stack = stack[:len(stack)+1]
			stack[len(stack)-1] = a
			return nil
		}
		// mark is the index of the innermost started call, or -1.
		mark := func() int {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].kind == shapeMark {
					return i
				}
			}
			return -1
		}
		// fallDef is a register the instruction defines on its fall-through
		// edge (edge 0) only; fallRef says it may then hold a reference.
		var fallDef uint32
		hasFallDef, fallRef, fallMark := false, false, false
		if err := b.chargeWork(uint64(len(stack)) + uint64(stateWords) + 1); err != nil {
			return err
		}
		switch in.Op {
		case OpConst, OpLocal, OpNewArray:
			write(in.A)
		case OpMove:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			ref := isRef(in.B)
			write(in.A)
			if ref {
				setRef(in.A)
			}
		case OpLoad:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpStore:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
		case OpBinary:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.C); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpBranch:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
		case OpPush:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			a := argShape{kind: f.Registers[in.A]}
			switch {
			case in.Aux == uint16(PushLoad):
				// The variable's value is sent, not the variable.
				a.kind = Value
			case in.Aux == uint16(PushAuto):
				// Sent as the callee declares: only a started call whose
				// callee is found at run time takes it.
				if m := mark(); m < 0 || stack[m].fn != UnknownCallee {
					return invalidPC("push by the callee's declaration outside a call whose callee is resolved at run time")
				}
				a.kind, a.late = Value, true
			case in.Aux == uint16(PushTemp) || in.Aux == uint16(PushResult) || in.Aux == uint16(PushElement):
				a.late = true
				if m := mark(); m < 0 || stack[m].fn != UnknownCallee {
					return invalidPC("push by the callee's declaration outside a call whose callee is resolved at run time")
				}
			case a.kind == Location:
				a.ref = true
			case isRef(in.A):
				// The instruction moves the reference to the stack.
				a.ref = true
				dropRef(in.A)
			}
			if in.B != 0 {
				a.named = true
				a.name = image.Constants[in.B-1].Bytes
			}
			if err := push(a); err != nil {
				return err
			}
		case OpCall:
			if int(in.C) > len(stack) {
				return invalidPC("direct call argument stack underflow")
			}
			callee := &image.Functions[in.B]
			for i := len(stack) - int(in.C); i < len(stack); i++ {
				if stack[i].kind != Value || stack[i].named || stack[i].late {
					return invalidPC("direct call requires positional value arguments")
				}
				// A callee that records its parameters receives a reference
				// only where it declares one (format 1.6).
				if p := i - (len(stack) - int(in.C)); minor >= 6 && stack[i].ref && p < len(callee.Parameters) && callee.Parameters[p].Flags&ParamByRef == 0 {
					return invalidPC(fmt.Sprintf("direct call passes a reference to by-value parameter %d", p))
				}
			}
			stack = stack[:len(stack)-int(in.C)]
			write(in.A)
		case OpCallValue:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if int(in.C) > len(stack) {
				return invalidPC("dynamic call argument stack underflow")
			}
			for i := len(stack) - int(in.C); i < len(stack); i++ {
				if stack[i].kind != Value && stack[i].kind != Location {
					return invalidPC("dynamic call takes arguments of a started call")
				}
				// A named argument belongs to a started call and may leave
				// no pending argument behind (a variadic parameter collects
				// it), so only CallPending takes it (format 1.6).
				if minor >= 6 && (stack[i].named || stack[i].late) {
					return invalidPC("dynamic call takes a named argument, which only a started call receives")
				}
			}
			stack = stack[:len(stack)-int(in.C)]
			write(in.A)
		case OpHostCall:
			if int(in.C) > len(stack) {
				return invalidPC("host call argument stack underflow")
			}
			for i := len(stack) - int(in.C); i < len(stack); i++ {
				if stack[i].kind != Value {
					return invalidPC("host call arguments must be values")
				}
				if minor >= 6 && (stack[i].named || stack[i].late) {
					return invalidPC("host call takes a named argument, which only a started call receives")
				}
			}
			stack = stack[:len(stack)-int(in.C)]
			write(in.A)
		case OpInitCall:
			if err := push(argShape{kind: shapeMark, fn: in.B}); err != nil {
				return err
			}
		case OpSendUnpack:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if mark() < 0 {
				return invalidPC("send-unpack outside a started call")
			}
			if err := push(argShape{kind: shapeUnpack}); err != nil {
				return err
			}
		case OpCallPending:
			m := mark()
			if m < 0 {
				return invalidPC("call-pending without a started call")
			}
			if want := Value; stack[m].fn != UnknownCallee && f.Registers[in.A] != image.Functions[stack[m].fn].ReturnKind || stack[m].fn == UnknownCallee && f.Registers[in.A] != want {
				return invalidPC("call-pending destination does not match the callee's return kind")
			}
			stack = stack[:m]
			write(in.A)
		case OpReturn, OpReturnRef, OpThrow, OpOutput, OpMatchError:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if in.Op != OpOutput && len(stack) != 0 {
				return invalidPC("terminal instruction leaves pending arguments")
			}
		case OpFetchConst, OpBeginSilence, OpRecvVariadic:
			write(in.A)
		case OpDeclareConst, OpEndSilence, OpBindGlobal, OpStaticInit:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
		case OpBindStatic:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			clear(in.B) // the instruction empties it
		case OpArrayGet:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.C); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpInclude, OpEval, OpShell:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpDrop:
			clear(in.A)
		case OpUnary, OpCast, OpIncDec, OpIsset:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpArrayIsset:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.C); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
		case OpArraySet:
			for _, r := range [...]uint32{in.A, in.B, in.C} {
				if err := read(r); err != nil {
					return flowError(err, fi, pc)
				}
			}
			if r, ok := ArraySetResult(in.Aux); ok {
				write(r)
			}
		case OpFetchDim:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if FetchFlags(in.Aux)&FetchAppend == 0 {
				if err := read(in.C); err != nil {
					return flowError(err, fi, pc)
				}
			}
			write(in.A)
		case OpIterInit:
			// The iterator is written on the fall-through edge only; the
			// jump for a subject that cannot be iterated leaves it cleared.
			// A by-reference iterator is the loop's reference to the subject.
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			clear(in.A)
			fallDef, hasFallDef, fallRef = in.A, true, IterFlags(in.Aux)&IterByRef != 0
		case OpIterNext:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if IterFlags(in.Aux)&IterLocation != 0 {
				if err := read(in.B); err != nil {
					return flowError(err, fi, pc)
				}
			} else {
				// The element is written only when there is one; by
				// reference it is a reference.
				fallDef, hasFallDef, fallRef = in.B, true, IterFlags(in.Aux)&IterByRef != 0
			}
		case OpIterKey:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if IterFlags(in.Aux) == IterLocation {
				if err := read(in.A); err != nil {
					return flowError(err, fi, pc)
				}
			} else {
				write(in.A)
			}
		case OpParamDefault:
			// Reads no register: it tests the frame's argument count.
		case OpBind:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if BindFlags(in.Aux) == BindValue {
				clear(in.B) // the instruction empties it
			}
		case OpMakeRef:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
			setRef(in.A)
		case OpArrayAppend, OpArrayUnset:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
		case OpArrayAdd:
			// A is read and stays defined: the element is added in place.
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			if ArrayAddFlags(in.Aux)&AddKey != 0 {
				if err := read(in.C); err != nil {
					return flowError(err, fi, pc)
				}
			}
		case OpUnset:
			// The Location stays defined; only the PHP variable is unset.
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
		case OpExit:
			// Exit may run while call arguments are pending, so unlike
			// Return it does not require an empty argument stack.
			if in.Aux == 1 {
				if err := read(in.A); err != nil {
					return flowError(err, fi, pc)
				}
			}
		case OpLeave, OpEndFinally:
			if len(stack) != 0 {
				return invalidPC("unwind instruction leaves pending arguments")
			}
			if in.Op == OpLeave && in.Aux == LeaveCarry {
				if err := read(in.C); err != nil {
					return flowError(err, fi, pc)
				}
				if isRef(in.C) {
					return invalidPC("leave carries a reference")
				}
			}
			if keep && in.Op == OpEndFinally {
				for _, t := range conts[uint32(pc)] {
					defs, refs := scratchDefs, scratchRefs
					if c, ok := carries[t]; ok {
						// The finally block ends with the carried value,
						// never a reference, written back to its register.
						copy(carryDefs, scratchDefs)
						carryDefs[c/64] |= uint64(1) << (c % 64)
						defs = carryDefs
						if track {
							copy(carryRefs, scratchRefs)
							carryRefs[c/64] &^= uint64(1) << (c % 64)
							refs = carryRefs
						}
					}
					if _, err := merge(int(t), defs, refs, stack); err != nil {
						return flowError(err, fi, pc)
					}
				}
			}
		default:
			if !is17(in.Op) {
				break
			}
			roles, _ := Roles17(in)
			ops := [3]uint32{in.A, in.B, in.C}
			for i, role := range roles {
				if role == RoleUse || role == RoleUseDef || role == RoleLocUse {
					if err := read(ops[i]); err != nil {
						return flowError(err, fi, pc)
					}
				}
			}
			for i, role := range roles {
				if role == RoleDef || role == RoleUseDef || role == RoleLocDef {
					write(ops[i])
				}
			}
			switch in.Op {
			case OpPropRef, OpStaticRef:
				setRef(in.A)
			case OpInitMethod, OpInitStatic:
				if err := push(argShape{kind: shapeMark, fn: UnknownCallee}); err != nil {
					return err
				}
			case OpArgByRef:
				if mark() < 0 {
					return invalidPC("arg-by-ref outside a started call")
				}
			case OpCallableConvert:
				// ZEND_CALLABLE_CONVERT replaces the call frame the
				// preceding INIT built, before any argument is sent.
				if m := mark(); m < 0 || m != len(stack)-1 {
					return invalidPC("callable-convert needs a started call without arguments")
				}
				stack = stack[:len(stack)-1]
			case OpNew:
				// The class has no constructor on the jump edge: no call is
				// started there.
				fallMark = true
			}
		}
		if in.Op == OpJump || in.Op == OpBranch || in.Op == OpLeave || (in.Op != OpReturn && in.Op != OpReturnRef && in.Op != OpThrow && in.Op != OpEndFinally && in.Op != OpExit && in.Op != OpMatchError) {
			if fallMark {
				if in.Aux&NewArgs == 0 {
					if _, err := merge(int(in.C), scratchDefs, scratchRefs, stack); err != nil {
						return flowError(err, fi, pc)
					}
				}
				if err := push(argShape{kind: shapeMark, fn: UnknownCallee}); err != nil {
					return err
				}
				if _, err := merge(pc+1, scratchDefs, scratchRefs, stack); err != nil {
					return flowError(err, fi, pc)
				}
				continue
			}
			if hasFallDef {
				// The jump edge is merged before the fall-through
				// definition is added.
				if _, err := merge(int(successor(in, uint32(pc), uint32(n), 1).pc), scratchDefs, scratchRefs, stack); err != nil {
					return flowError(err, fi, pc)
				}
				write(fallDef)
				if fallRef {
					setRef(fallDef)
				}
				if _, err := merge(pc+1, scratchDefs, scratchRefs, stack); err != nil {
					return flowError(err, fi, pc)
				}
				continue
			}
			for ei, ec := 0, successorCount(in, uint32(pc), uint32(n)); ei < ec; ei++ {
				e := successor(in, uint32(pc), uint32(n), ei)
				if e.leave && keep {
					if handlers.leaveFirst(uint32(pc), e.pc) >= 0 {
						continue // the last finally block's EndFinally reaches it
					}
				} else if e.leave {
					for i := range scratchDefs {
						scratchDefs[i] = 0
					}
					for i := range scratchRefs {
						scratchRefs[i] = 0
					}
				}
				if _, err := merge(int(e.pc), scratchDefs, scratchRefs, stack); err != nil {
					return flowError(err, fi, pc)
				}
			}
		}
	}
	return nil
}

func flowError(err error, fi uint32, pc int) error {
	var e *Error
	if errors.As(err, &e) {
		cp := *e
		cp.Function, cp.PC = int(fi), pc
		return &cp
	}
	kind := err
	if kind == nil {
		kind = ErrInvalid
	}
	return &Error{Kind: kind, Offset: -1, Function: int(fi), PC: pc, Detail: "invalid register or control flow"}
}

func functionError(err error, fi, pc int) error {
	var e *Error
	if errors.As(err, &e) {
		cp := *e
		if cp.Function < 0 {
			cp.Function = fi
		}
		if pc >= 0 && cp.PC < 0 {
			cp.PC = pc
		}
		return &cp
	}
	return &Error{Kind: err, Offset: -1, Function: fi, PC: pc, Detail: "invalid function"}
}

const (
	positionOK = iota
	positionInvalid
	positionOutside
)

// positionProblem checks a non-zero position against its source record. Both
// verification and diagnostics use it, so a diagnostic never shows a
// position verification would reject.
func positionProblem(sources []Source, p Position) int {
	if p.Source == 0 || uint64(p.Source) > uint64(len(sources)) || p.Line == 0 {
		return positionInvalid
	}
	s := sources[p.Source-1]
	if p.Start > p.End || p.End > s.Bytes || p.Line > s.Lines {
		return positionOutside
	}
	return positionOK
}
