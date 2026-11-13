package bytecode

// W-17 stage 2 review fixes: REQ-025, REQ-026; T-049, T-050. An element
// location (the destination of FetchDim) names a bucket of an array, like
// Zend's INDIRECT zval, so it is valid only until the array changes in any
// other way. The rule below lets it be used only within the straight-line
// sequence the compiler emits after it, where nothing else can reach the
// array; any other use is invalid bytecode.

// elementWindow is the state of the sequence a FetchDim opened.
type elementWindow struct {
	reg    uint32 // the element register; valid only while open
	base   uint32 // the Location the first FetchDim of the chain fetched from
	open   bool
	loaded bool // a Load of the element ran: Binary and the Store follow
}

// verifyElementLocations checks every use of an element register, in
// reachable and unreachable code alike. A register is an element register
// when some FetchDim defines it. After a FetchDim of the register from a
// Location that is not an element register, a sequence follows in which
// only these instructions may appear, in this order:
//
//   - FetchDim of the register from itself: the next level of the chain;
//   - Const, and Load of a variable other than the chain's base variable:
//     the key and value operands read between the levels;
//   - one of IncDec, Store, ArraySet, ArrayAppend or ArrayUnset through
//     the register, which ends the sequence; a value it reads itself
//     (ValueLocation) is a variable other than the base;
//   - or Load of the register, then Const and Binary, then Store to the
//     register, which ends the sequence (an assign-op);
//   - or, from format 1.3, one of these, which ends the sequence: Bind of
//     the register to or from a variable, or to a reference in a Value
//     register; MakeRef or a by-reference IterInit of the register. A
//     reference does not point into the array, so nothing stale is left
//     behind.
//
// A jump target, a handler target or any other instruction ends the
// sequence, and the register may not appear in any instruction outside a
// sequence: not in Local, Move, Drop, Push, Call, Isset, Unset, the
// iterator instructions or as a key or value Location. A variable is told
// apart from the base by the local each Location is bound to; a Location
// bound by more than one Local, or defined other than by Local, is never
// known to differ from the base.
func verifyElementLocations(b *budget, f *Function) error {
	n, nr := len(f.Code), len(f.Registers)
	hasFetch := false
	for _, in := range f.Code {
		if in.Op == OpFetchDim {
			hasFetch = true
			break
		}
	}
	if !hasFetch {
		return nil
	}
	if err := b.reserve(uint64(nr)*5 + uint64(n) + 64); err != nil {
		return err
	}
	if err := b.chargeWork(2 * uint64(n)); err != nil {
		return err
	}
	elem := make([]bool, nr)
	local := make([]int32, nr) // -1: no Local yet; -2: not known to be one variable
	for i := range local {
		local[i] = -1
	}
	target := make([]bool, n)
	for _, h := range f.Handlers {
		target[h.Target] = true
	}
	for pc, in := range f.Code {
		switch in.Op {
		case OpFetchDim:
			elem[in.A] = true
		case OpLocal:
			if local[in.A] == -1 {
				local[in.A] = int32(in.B)
			} else if local[in.A] != int32(in.B) {
				local[in.A] = -2
			}
		case OpMove, OpCall:
			if f.Registers[in.A] == Location {
				local[in.A] = -2
			}
		}
		for ei, ec := 0, successorCount(in, uint32(pc), uint32(n)); ei < ec; ei++ {
			if e := successor(in, uint32(pc), uint32(n), ei); e.pc != uint32(pc)+1 && e.pc < uint32(n) {
				target[e.pc] = true
			}
		}
	}
	other := func(r, base uint32) bool {
		return !elem[r] && local[r] >= 0 && local[base] >= 0 && local[r] != local[base]
	}
	var w elementWindow
	for pc, in := range f.Code {
		if target[pc] {
			w.open = false
		}
		ok, err := w.step(in, elem, other)
		if err != nil {
			return functionError(err, -1, pc)
		}
		if ok {
			continue
		}
		w.open = false
		locs, k := locationOperands(in, f)
		for _, r := range locs[:k] {
			if elem[r] {
				return functionError(invalid("element location used outside the instruction sequence of the FetchDim that defined it"), -1, pc)
			}
		}
	}
	return nil
}

// step applies in to an open sequence and reports whether in belongs to it;
// a FetchDim from a variable opens one. It returns an error for an
// instruction that uses the element register in a way the sequence does
// not allow.
func (w *elementWindow) step(in Instruction, elem []bool, other func(r, base uint32) bool) (bool, error) {
	bad := invalid("element location used outside the instruction sequence of the FetchDim that defined it")
	if in.Op == OpFetchDim && elem[in.A] {
		flags := FetchFlags(in.Aux)
		if flags&FetchKeyLocation != 0 && elem[in.C] {
			return false, bad
		}
		if elem[in.B] {
			// The next level of an open chain, in place.
			if !w.open || w.loaded || in.A != w.reg || in.B != w.reg {
				return false, bad
			}
			return true, nil
		}
		*w = elementWindow{reg: in.A, base: in.B, open: true}
		return true, nil
	}
	if !w.open {
		return false, nil
	}
	e := w.reg
	switch in.Op {
	case OpConst:
		return true, nil
	case OpLoad:
		if in.B == e && !w.loaded {
			w.loaded = true
			return true, nil
		}
		if in.B != e && !w.loaded && other(in.B, w.base) {
			return true, nil
		}
	case OpBinary:
		return w.loaded, nil
	case OpStore:
		if in.A == e {
			w.open = false
			return true, nil
		}
	case OpIncDec:
		if in.B == e && !w.loaded {
			w.open = false
			return true, nil
		}
	case OpBind:
		// $x =& <element>, <element> =& $x, or <element> =& a reference
		// in a register (MakeRef, a by-reference IterNext).
		value := BindFlags(in.Aux) == BindValue
		if !w.loaded && (in.A == e && (value || !elem[in.B]) || in.B == e && !value && !elem[in.A]) {
			w.open = false
			return true, nil
		}
	case OpMakeRef, OpIterInit:
		// MakeRef replaces the element by a reference before anything else
		// can reach the array; a by-reference IterInit binds the loop to it.
		if in.B == e && !w.loaded && (in.Op == OpMakeRef || IterFlags(in.Aux) == IterByRef) {
			w.open = false
			return true, nil
		}
	case OpArraySet, OpArrayAppend, OpArrayUnset:
		if in.A != e || w.loaded {
			break
		}
		flags := ArrayWriteFlags(in.Aux)
		switch {
		case in.Op == OpArraySet && flags&KeyLocation != 0 && elem[in.B],
			in.Op == OpArrayUnset && flags == KeyLocation && elem[in.B]:
			return false, bad
		case in.Op == OpArraySet && flags&ValueLocation != 0 && !other(in.C, w.base),
			in.Op == OpArrayAppend && flags&ValueLocation != 0 && !other(in.B, w.base):
			// A value read through the base would store the array, or a
			// copy sharing its buckets, into its own element.
			return false, bad
		}
		w.open = false
		return true, nil
	}
	return false, nil
}

// locationOperands lists the Location registers in reads and writes.
func locationOperands(in Instruction, f *Function) (regs [3]uint32, n int) {
	add := func(r uint32) { regs[n] = r; n++ }
	isLoc := func(r uint32) bool { return uint64(r) < uint64(len(f.Registers)) && f.Registers[r] == Location }
	switch in.Op {
	case OpLocal, OpStore, OpReturnRef, OpUnset:
		add(in.A)
	case OpLoad, OpIsset, OpIncDec:
		add(in.B)
	case OpArraySet:
		add(in.A)
		if ArrayWriteFlags(in.Aux)&KeyLocation != 0 {
			add(in.B)
		}
		if ArrayWriteFlags(in.Aux)&ValueLocation != 0 {
			add(in.C)
		}
	case OpArrayAppend:
		add(in.A)
		if ArrayWriteFlags(in.Aux)&ValueLocation != 0 {
			add(in.B)
		}
	case OpArrayUnset:
		add(in.A)
		if ArrayWriteFlags(in.Aux) == KeyLocation {
			add(in.B)
		}
	case OpArrayAdd:
		if ArrayAddFlags(in.Aux)&AddKeyLocation != 0 {
			add(in.C)
		}
	case OpFetchDim:
		add(in.A)
		add(in.B)
		if FetchFlags(in.Aux)&FetchKeyLocation != 0 {
			add(in.C)
		}
	case OpIterNext:
		if IterFlags(in.Aux)&IterLocation != 0 {
			add(in.B)
		}
	case OpIterInit, OpBind:
		if in.Op == OpBind {
			add(in.A)
		}
		if isLoc(in.B) {
			add(in.B)
		}
	case OpMakeRef:
		add(in.B)
	case OpIterKey:
		if IterFlags(in.Aux) == IterLocation {
			add(in.A)
		}
	case OpMove:
		if isLoc(in.A) {
			add(in.A)
			add(in.B)
		}
	case OpDrop, OpPush, OpCall:
		if isLoc(in.A) {
			add(in.A)
		}
	}
	return regs, n
}
