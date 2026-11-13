package runtime

import (
	"math"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 3: references. REQ-001, REQ-016, REQ-017, REQ-025, REQ-026;
// T-009, T-010, T-031..T-034. The format 1.3 instructions Bind and MakeRef,
// by-reference arguments (Push of a Location) and foreach by reference.
// Each follows the Zend handler named in its comment; the expectations are
// the PHP 8.5.12 outputs of tests/vm/w17/cases/{ref,foreach_ref,param_ref}_*.

// vmIterSubjectLocation is the decoded aux bit of a by-reference IterInit
// whose subject is a Location rather than a temporary in a Value register.
const vmIterSubjectLocation uint16 = 4

// vmRefIter is the state of one foreach by reference, FE_RESET_RW's
// iterator. The iterator register holds the loop's own reference to the
// subject; its integer field names this state (vmRefIterID). A copy of the
// register that hand-written bytecode kept after the loop ended names no
// state: the state checks the reference and a generation that every reuse
// of its slot increments.
type vmRefIter struct {
	it      *arrayIterator
	loop    *reference
	current *phpArray
	first   bool
	key     arrayKey
	hasKey  bool
	live    bool
	gen     uint32
}

// vmRefIterID is the integer field of an iterator register: the slot plus
// one in the low 32 bits, the slot's generation in the high 32 bits.
func vmRefIterID(index int, gen uint32) int64 { return int64(gen)<<32 | int64(index+1) }

// nullIfUndefined is what ASSIGN_REF, MAKE_REF and SEND_REF do to a
// variable or an element that holds nothing: the reference they create
// holds null, and no warning is raised.
func nullIfUndefined(slot *value) {
	if slot.kind == valueUndefined {
		*slot = nullValue()
	}
}

// bind is ZEND_ASSIGN_REF: A becomes another name of the location B. With
// BindValue, B is a Value register the instruction empties; a reference in
// it (MAKE_REF, FE_FETCH_RW, SEND_REF) is bound, any other value (a
// by-reference parameter's default, RECV_INIT) is assigned.
func (s *vmState) bind(f *vmFrame, in *vmInstr) error {
	dst, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	if in.c != 0 {
		// The Bind that receives a by-reference parameter with a default.
		s.cacheDefault(f, in)
	}
	if bytecode.BindFlags(in.aux) != bytecode.BindValue {
		src, err := s.slot(f, in.b)
		if err != nil {
			return err
		}
		nullIfUndefined(src)
		return s.h.bind(dst, src)
	}
	if f.regs[in.b].kind != valueReference {
		v := take(f.regs, in.b)
		err := s.h.assign(dst, v)
		if rerr := s.releaseValue(v); err == nil {
			err = rerr
		}
		return err
	}
	err = s.h.bind(dst, &f.regs[in.b])
	if rerr := s.releaseValue(take(f.regs, in.b)); err == nil {
		err = rerr
	}
	return err
}

// makeRef is ZEND_MAKE_REF: the location B becomes a reference, which the
// Value register A then holds, so that fetching the other side of an
// assignment by reference cannot leave a pointer into an array behind.
func (s *vmState) makeRef(f *vmFrame, in *vmInstr) error {
	src, err := s.slot(f, in.b)
	if err != nil {
		return err
	}
	nullIfUndefined(src)
	// The register's old value goes through releaseValue, which ends an
	// iteration it may name.
	if err := s.clearReg(f, in.a); err != nil {
		return err
	}
	return s.h.bind(&f.regs[in.a], src)
}

// clearReg empties the Value register r.
func (s *vmState) clearReg(f *vmFrame, r uint32) error {
	old := f.regs[r]
	f.regs[r] = value{}
	return s.releaseValue(old)
}

// pushRef is ZEND_SEND_REF: the variable or element becomes a reference
// and the argument stack holds it, which the callee's by-reference
// parameter binds (Bind with BindValue).
func (s *vmState) pushRef(f *vmFrame, in *vmInstr) error {
	src, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	nullIfUndefined(src)
	var arg value
	if err := s.h.bind(&arg, src); err != nil {
		return err
	}
	s.args = append(s.args, arg)
	return nil
}

// --- foreach by reference ---

// refIter is the by-reference iteration an iterator register names, or nil
// for a by-value iterator.
func (s *vmState) refIter(v value) *vmRefIter {
	if v.kind != valueReference || uint32(v.integer) == 0 {
		return nil
	}
	slot := int64(uint32(v.integer)) - 1
	if slot < 0 || slot >= int64(len(s.refIters)) {
		return nil
	}
	if it := &s.refIters[slot]; it.live && it.loop == v.ref && it.gen == uint32(v.integer>>32) {
		return it
	}
	return nil
}

// endRefIter forgets the positions of a by-reference iteration whose
// iterator register is being released (FE_FREE), so that the arrays it
// visited no longer track it.
func (s *vmState) endRefIter(v value) {
	it := s.refIter(v)
	if it == nil {
		return
	}
	for a := range it.it.arrays {
		delete(a.cursors, it.it)
	}
	*it = vmRefIter{gen: it.gen}
	if it.gen != math.MaxUint32 {
		// A slot whose generation would wrap is retired instead, so no
		// old copy of an iterator register can ever name a new iteration.
		s.freeIters = append(s.freeIters, int(uint32(v.integer))-1)
	}
}

// iterInitRef is ZEND_FE_RESET_RW: the loop takes its own reference to the
// subject array, held by a variable, an element or a temporary, as
// heap.foreachRef does. A subject that is not an array warns and is not
// iterated.
func (s *vmState) iterInitRef(f *vmFrame, in *vmInstr) (iterable bool, err error) {
	var subject *value
	if in.aux&vmIterSubjectLocation != 0 {
		if subject, err = s.slot(f, in.b); err != nil {
			return false, err
		}
	} else {
		subject = &f.regs[in.b]
	}
	// The subject is read (BP_VAR_R): an undefined variable warns and is
	// not created, and only an array becomes a reference.
	if t := subject.deref(); t.kind != valueArray {
		if t.kind == valueObject {
			return false, s.unsupported("iterating an object")
		}
		if t.kind == valueUndefined && in.aux&vmIterSubjectLocation != 0 {
			if name, ok := s.variableName(f, in.b); ok {
				s.h.warn(levelWarning, "Undefined variable $"+name)
				if s.stopped() {
					return false, nil
				}
			}
		}
		old := f.regs[in.a]
		f.regs[in.a] = value{}
		if err := s.releaseValue(old); err != nil {
			return false, err
		}
		s.h.warn(levelWarning, "foreach() argument must be of type array|object, "+valueName(t)+" given")
		return false, nil
	}
	var loop value
	if err := s.h.bind(&loop, subject); err != nil {
		return false, err
	}
	var index int
	if n := len(s.freeIters); n > 0 {
		index, s.freeIters = s.freeIters[n-1], s.freeIters[:n-1]
	} else {
		index = len(s.refIters)
		s.refIters = append(s.refIters, vmRefIter{})
	}
	gen := s.refIters[index].gen + 1
	s.refIters[index] = vmRefIter{it: &arrayIterator{arrays: make(map[*phpArray]struct{})}, loop: loop.ref, first: true, live: true, gen: gen}
	loop.integer = vmRefIterID(index, gen)
	old := f.regs[in.a]
	f.regs[in.a] = loop // the loop's reference moves into the register
	return true, s.releaseValue(old)
}

// iterNextRef is ZEND_FE_FETCH_RW, one step of heap.foreachRef: the
// iteration follows the array the subject holds now, at the position the
// iterator has in it, and binds the next element to the destination. It
// reports false when the iteration is over, the subject stopped being an
// array (with PHP's warning) or the run stopped.
func (s *vmState) iterNextRef(f *vmFrame, in *vmInstr, ri *vmRefIter) (more bool, err error) {
	h := s.h
	loop := &f.regs[in.a]
	t := loop.ref.target
	if t.kind != valueArray {
		h.warn(levelWarning, "foreach() argument must be of type array|object, "+valueName(t)+" given")
		return false, nil
	}
	it := ri.it
	a := t.array
	pos, known := a.cursors[it]
	if ri.first || !known {
		// FE_RESET separates once; a later fetch separates only for an
		// array that replaced the one the loop was following.
		for old := range it.arrays {
			delete(old.cursors, it)
			delete(it.arrays, old)
		}
		// separate keeps the integer field: the slot is the reference's
		// target, not the register.
		hold := value{kind: valueReference, ref: loop.ref}
		if a, err = h.separate(&hold); err != nil {
			return false, err
		}
		if ri.first {
			pos = 0
		} else {
			pos = a.pointer
		}
	} else if a != ri.current {
		for old := range it.arrays {
			if old != a {
				delete(old.cursors, it)
				delete(it.arrays, old)
			}
		}
	}
	if a.cursors == nil {
		a.cursors = make(map[*arrayIterator]int)
	}
	it.arrays[a] = struct{}{}
	ri.current, ri.first = a, false
	pos, b, stopped := nextLive(a, pos, s.poll)
	a.cursors[it] = pos
	if stopped || b == nil {
		return false, nil
	}
	// FE_FETCH_RW stores the key before the reference assignment, whose
	// release may run a destructor that removes this bucket.
	ri.key, ri.hasKey = b.key, true
	if bytecode.IterFlags(in.aux)&bytecode.IterLocation == 0 {
		if in.b == in.a {
			return false, s.unsupported("fetching an element into its own iterator")
		}
		if err := s.clearReg(f, in.b); err != nil {
			return false, err
		}
		return true, h.bind(&f.regs[in.b], &b.val)
	}
	dst, err := s.slot(f, in.b)
	if err != nil {
		return false, err
	}
	return true, h.bindForeachItem(dst, &b.val)
}

// iterKeyRef is the key of the element the by-reference iteration fetched
// last.
func (s *vmState) iterKeyRef(f *vmFrame, in *vmInstr, ri *vmRefIter) error {
	if !ri.hasKey {
		return s.unsupported("reading the key of an iterator that has no current element")
	}
	if ri.key.isString {
		return s.iterStore(f, in.aux, in.a, stringValue(ri.key.text))
	}
	return s.iterStore(f, in.aux, in.a, s.h.number(ri.key.integer))
}
