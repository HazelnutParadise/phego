package bytecode

import (
	"errors"
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

func verifyImage(b *budget, image Image, cloneCost bool) (CapabilitySet, error) {
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
			if p.Source == 0 || uint64(p.Source) > uint64(len(image.Sources)) || p.Line == 0 {
				return 0, functionError(invalid("invalid source position"), fi, pc)
			}
			s := image.Sources[p.Source-1]
			if p.Start > p.End || p.End > s.Bytes || p.Line > s.Lines {
				return 0, functionError(invalid("source position outside source"), fi, pc)
			}
		}
		caps, err := verifyFunction(b, image, uint32(fi), f)
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

type argShape struct {
	kind  RegisterKind
	named bool
	name  string
}
type flowState struct {
	defs  []uint64
	stack []argShape
}

func verifyFunction(b *budget, image Image, index uint32, f *Function) (CapabilitySet, error) {
	n := len(f.Code)
	if err := verifyHandlers(b, f); err != nil {
		return 0, functionError(err, int(index), -1)
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
		if err := verifyInstruction(image, f, uint32(index), uint32(pc), in); err != nil {
			return 0, functionError(err, int(index), pc)
		}
		for ei, ec := 0, successorCount(in, uint32(pc), uint32(n)); ei < ec; ei++ {
			e := successor(in, uint32(pc), uint32(n), ei)
			if err := checkEdge(b, f, uint32(pc), e.pc, e.leave); err != nil {
				return 0, functionError(err, int(index), pc)
			}
		}
		if in.Op == OpEndFinally {
			kind, width := HandlerKind(0), uint64(n)+1
			for _, h := range f.Handlers {
				if err := b.chargeWork(1); err != nil {
					return 0, err
				}
				if uint32(pc) >= h.Target && uint32(pc) < h.BodyEnd && uint64(h.BodyEnd-h.Target) < width {
					kind, width = h.Kind, uint64(h.BodyEnd-h.Target)
				}
			}
			if kind != Finally {
				return 0, functionError(invalid("endfinally outside innermost finally body"), int(index), pc)
			}
		}
	}
	if err := verifyFlow(b, image, f, uint32(index)); err != nil {
		return 0, functionError(err, int(index), -1)
	}
	return caps, nil
}

func verifyInstruction(image Image, f *Function, fi, pc uint32, in Instruction) error {
	reg := func(n uint32, k RegisterKind) bool {
		return uint64(n) < uint64(len(f.Registers)) && f.Registers[n] == k
	}
	zero := in.B == 0 && in.C == 0
	if in.Op != OpBinary && in.Op != OpInclude && in.Aux != 0 {
		return invalid("unused aux field")
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
		if !reg(in.A, Value) || !reg(in.B, Location) || in.C != 0 {
			return invalid("invalid load operands")
		}
	case OpStore:
		if !reg(in.A, Location) || !reg(in.B, Value) || in.C != 0 {
			return invalid("invalid store operands")
		}
	case OpBinary:
		if in.Aux < uint16(Add) || in.Aux > uint16(BitXor) || !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
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
		if !reg(in.A, Value) || !reg(in.B, Value) || !reg(in.C, Value) {
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
	default:
		return invalid("unknown opcode")
	}
	_ = fi
	_ = pc
	return nil
}

type edge struct {
	pc    uint32
	leave bool
}

func successorCount(in Instruction, pc, n uint32) int {
	switch in.Op {
	case OpReturn, OpReturnRef, OpThrow, OpEndFinally:
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
	for i, h := range f.Handlers {
		if err := b.chargeWork(1); err != nil {
			return err
		}
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
		for j := 0; j < i; j++ {
			if err := b.chargeWork(1); err != nil {
				return err
			}
			p := f.Handlers[j]
			// Two handlers are siblings with disjoint regions, or the whole inner
			// handler belongs to one region of its outer handler. This permits try /
			// catch inside finally while excluding cross-scope exception transfers.
			contains := func(a, z, b, y uint32) bool { return a <= b && y <= z }
			nested := func(outer, inner Handler) bool {
				return (contains(outer.Start, outer.End, inner.Start, inner.End) && contains(outer.Start, outer.End, inner.Target, inner.BodyEnd)) ||
					(contains(outer.Target, outer.BodyEnd, inner.Start, inner.End) && contains(outer.Target, outer.BodyEnd, inner.Target, inner.BodyEnd))
			}
			disjoint := func(a, z, b, y uint32) bool { return z <= b || y <= a }
			siblings := disjoint(h.Start, h.End, p.Start, p.End) && disjoint(h.Start, h.End, p.Target, p.BodyEnd) &&
				disjoint(h.Target, h.BodyEnd, p.Start, p.End) && disjoint(h.Target, h.BodyEnd, p.Target, p.BodyEnd)
			if !siblings && !nested(p, h) && !nested(h, p) {
				return invalid("crossing handler regions")
			}
		}
	}
	return nil
}

func checkEdge(b *budget, f *Function, from, to uint32, leave bool) error {
	if to >= uint32(len(f.Code)) {
		return invalid("control flow falls off function")
	}
	for _, h := range f.Handlers {
		if err := b.chargeWork(1); err != nil {
			return err
		}
		fromBody, toBody := from >= h.Target && from < h.BodyEnd, to >= h.Target && to < h.BodyEnd
		if toBody && !fromBody {
			return invalid("normal control enters handler body")
		}
		if fromBody && !toBody && !leave {
			return invalid("normal control leaves handler body")
		}
		fromProtected, toProtected := from >= h.Start && from < h.End, to >= h.Start && to < h.End
		if !fromProtected && toProtected && to != h.Start {
			return invalid("control enters protected range after start")
		}
		if fromProtected && !toProtected && h.Kind == Finally && !leave {
			return invalid("control leaves finally range without leave")
		}
	}
	return nil
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
		case OpLeave, OpEndFinally:
			if len(stack) != 0 {
				return invalidPC("unwind instruction leaves pending arguments")
			}
		}
		if in.Op == OpJump || in.Op == OpBranch || in.Op == OpLeave || (in.Op != OpReturn && in.Op != OpReturnRef && in.Op != OpThrow && in.Op != OpEndFinally) {
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
