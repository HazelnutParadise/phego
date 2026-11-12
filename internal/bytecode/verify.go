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
// use any format 1.1 opcode, Aux value or binary operator.
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

type argShape struct {
	kind  RegisterKind
	named bool
	name  string
}
type flowState struct {
	defs  []uint64
	stack []argShape
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
	if err := verifyFlow(b, image, f, uint32(index)); err != nil {
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

func verifyInstruction(image Image, f *Function, minor uint16, in Instruction) error {
	if err := checkOperands(image, f, in); err != nil {
		return err
	}
	if minor == 0 && format11(in) {
		return invalid("instruction needs bytecode format 1.1, but the file declares format 1.0")
	}
	return nil
}

func checkOperands(image Image, f *Function, in Instruction) error {
	reg := func(n uint32, k RegisterKind) bool {
		return uint64(n) < uint64(len(f.Registers)) && f.Registers[n] == k
	}
	zero := in.B == 0 && in.C == 0
	switch in.Op {
	case OpBinary, OpInclude, OpLoad, OpArrayGet, OpUnary, OpCast, OpIncDec, OpExit, OpArraySet, OpArrayAppend, OpArrayUnset, OpArrayAdd:
	default:
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
	case OpJump, OpLeave:
		if !zero || uint64(in.A) >= uint64(len(f.Code)) {
			return invalid("invalid control target")
		}
	case OpBranch:
		if !reg(in.A, Value) || uint64(in.B) >= uint64(len(f.Code)) || uint64(in.C) >= uint64(len(f.Code)) {
			return invalid("invalid branch operands")
		}
	case OpPush:
		if uint64(in.A) >= uint64(len(f.Registers)) || in.C != 0 {
			return invalid("invalid push operands")
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
		if !reg(in.A, Value) || f.ReturnKind != Value || !zero {
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
		if in.Aux > 1 || !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
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
		if in.Aux < uint16(Not) || in.Aux > uint16(BitNot) || !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid unary operands: A and B must be Value registers, C zero, Aux Not to BitNot")
		}
	case OpCast:
		if in.Aux < uint16(CastBool) || in.Aux > uint16(CastArray) || !reg(in.A, Value) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid cast operands: A and B must be Value registers, C zero, Aux CastBool to CastArray")
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
		if in.Aux > uint16(ValueLocation|KeyLocation) || !reg(in.A, Location) || !reg(in.B, key) || !reg(in.C, val) {
			return invalid("invalid array-set operands: A must be a Location, B and C Value registers, or Locations where Aux has KeyLocation or ValueLocation")
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
		if !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
			return invalid("invalid array-isset operands: A, B and C must be Value registers")
		}
	case OpUnset:
		if !reg(in.A, Location) || !zero {
			return invalid("invalid unset operands: A must be a Location, B and C zero")
		}
	case OpExit:
		if !zero || in.Aux > 1 || (in.Aux == 0 && in.A != 0) || (in.Aux == 1 && !reg(in.A, Value)) {
			return invalid("invalid exit operands: Aux 0 needs A, B and C zero, Aux 1 a status Value in A and B, C zero")
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
	case OpReturn, OpReturnRef, OpThrow, OpEndFinally, OpExit:
		return 0
	case OpBranch:
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
	// the innermost Finally handler whose protected range contains pc.
	body, enter, finally []int32
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
	arrays := uint64(2)
	if finallies > 0 {
		arrays++
	}
	if err := b.reserve(128); err != nil {
		return nil, err
	}
	if err := b.reserveMul(uint64(n), arrays*4); err != nil {
		return nil, err
	}
	if err := b.reserveMul(uint64(2*h+finallies), 16+4); err != nil {
		return nil, err
	}
	if err := b.chargeWork(3*uint64(n) + 3*uint64(h)); err != nil {
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

func verifyFlow(b *budget, image Image, f *Function, fi uint32) error {
	n, nr := len(f.Code), len(f.Registers)
	words := (nr + 63) / 64
	if err := b.reserve(uint64(n)*17 + uint64(words)*16 + uint64(f.MaxStack)*24 + 256); err != nil {
		return err
	}
	states := make([]*flowState, n)
	queue := make([]int, n)
	inQ := make([]bool, n)
	qHead, qTail, qCount := 0, 0, 0
	var scratchDefs []uint64
	var scratchStack []argShape
	enqueue := func(pc int) {
		if !inQ[pc] {
			queue[qTail] = pc
			qTail = (qTail + 1) % n
			qCount++
			inQ[pc] = true
		}
	}
	merge := func(pc int, defs []uint64, stack []argShape) (bool, error) {
		if pc < 0 || pc >= n {
			return false, invalid("control flow falls off function")
		}
		if err := b.chargeWork(uint64(words) + uint64(len(stack))); err != nil {
			return false, err
		}
		old := states[pc]
		if old == nil {
			if err := b.reserve(uint64(words)*8 + uint64(len(stack))*32 + 64); err != nil {
				return false, err
			}
			old = &flowState{defs: make([]uint64, words), stack: append([]argShape(nil), stack...)}
			copy(old.defs, defs)
			states[pc] = old
			enqueue(pc)
			return true, nil
		}
		if len(old.stack) != len(stack) {
			return false, invalid("argument stack shape differs at control-flow join")
		}
		for i := range stack {
			if old.stack[i].kind != stack[i].kind || old.stack[i].named != stack[i].named || len(old.stack[i].name) != len(stack[i].name) {
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
		}
		changed := false
		for i := range old.defs {
			next := old.defs[i] & defs[i]
			if next != old.defs[i] {
				old.defs[i] = next
				changed = true
			}
		}
		if changed {
			enqueue(pc)
		}
		return changed, nil
	}
	seed := make([]uint64, words)
	for i := uint32(0); i < f.Params; i++ {
		seed[i/64] |= uint64(1) << (i % 64)
	}
	if _, err := merge(0, seed, nil); err != nil {
		return err
	}
	for _, h := range f.Handlers {
		if err := b.chargeWork(uint64(words) + 1); err != nil {
			return err
		}
		clear(seed)
		if h.Kind == Catch {
			seed[h.Register/64] |= uint64(1) << (h.Register % 64)
		}
		if _, err := merge(int(h.Target), seed, nil); err != nil {
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
		}
		copy(scratchDefs, st.defs)
		depth := copy(scratchStack, st.stack)
		stack := scratchStack[:depth]
		in := f.Code[pc]
		read := func(r uint32) error {
			if scratchDefs[r/64]&(uint64(1)<<(r%64)) == 0 {
				return invalid("register read before definition")
			}
			return nil
		}
		invalidPC := func(detail string) error {
			return &Error{Kind: ErrInvalid, Offset: -1, Function: int(fi), PC: pc, Detail: detail}
		}
		write := func(r uint32) { scratchDefs[r/64] |= uint64(1) << (r % 64) }
		clear := func(r uint32) { scratchDefs[r/64] &^= uint64(1) << (r % 64) }
		if err := b.chargeWork(uint64(len(stack)) + uint64(words) + 1); err != nil {
			return err
		}
		switch in.Op {
		case OpConst, OpLocal, OpNewArray:
			write(in.A)
		case OpMove:
			if err := read(in.B); err != nil {
				return flowError(err, fi, pc)
			}
			write(in.A)
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
			if len(stack) >= int(f.MaxStack) {
				return invalidPC("argument stack exceeds frame limit")
			}
			a := argShape{kind: f.Registers[in.A]}
			if in.B != 0 {
				a.named = true
				a.name = image.Constants[in.B-1].Bytes
			}
			stack = stack[:len(stack)+1]
			stack[len(stack)-1] = a
		case OpCall:
			if int(in.C) > len(stack) {
				return invalidPC("direct call argument stack underflow")
			}
			for i := len(stack) - int(in.C); i < len(stack); i++ {
				if stack[i].kind != Value || stack[i].named {
					return invalidPC("direct call requires positional value arguments")
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
			}
			stack = stack[:len(stack)-int(in.C)]
			write(in.A)
		case OpReturn, OpReturnRef, OpThrow, OpOutput:
			if err := read(in.A); err != nil {
				return flowError(err, fi, pc)
			}
			if in.Op != OpOutput && len(stack) != 0 {
				return invalidPC("terminal instruction leaves pending arguments")
			}
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
		}
		if in.Op == OpJump || in.Op == OpBranch || in.Op == OpLeave || (in.Op != OpReturn && in.Op != OpReturnRef && in.Op != OpThrow && in.Op != OpEndFinally && in.Op != OpExit) {
			for ei, ec := 0, successorCount(in, uint32(pc), uint32(n)); ei < ec; ei++ {
				e := successor(in, uint32(pc), uint32(n), ei)
				d := scratchDefs
				if e.leave {
					for i := range scratchDefs {
						scratchDefs[i] = 0
					}
				}
				if _, err := merge(int(e.pc), d, stack); err != nil {
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
