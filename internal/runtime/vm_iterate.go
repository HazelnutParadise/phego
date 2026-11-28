package runtime

import (
	"math"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032.
// Iterating Traversable objects (Zend/zend_interfaces.c and the iterator of
// Zend/zend_generators.c): foreach, yield from and argument unpacking walk
// a Generator through its own functions and any other Iterator through its
// methods, after IteratorAggregate::getIterator has given one.
//
// An iterator register of a foreach over such an object holds the Iterator
// (an owner) and names a slot of vmState.objIters with a negative integer
// field; a plain object's iteration keeps its property position there.

// vmObjIter is the state of one foreach over a Traversable object: the
// position zend_object_iterator.index counts and the key the fetch read,
// which the IterKey after it takes.
type vmObjIter struct {
	live   bool
	gen    uint32
	index  int64
	key    value
	hasKey bool
}

// objIterOf returns the state an iterator register names, or nil.
func (s *vmState) objIterOf(v value) *vmObjIter {
	if v.kind != valueObject || v.integer >= 0 {
		return nil
	}
	id := -v.integer
	slot := int(uint32(id)) - 1
	if slot < 0 || slot >= len(s.objIters) {
		return nil
	}
	if it := &s.objIters[slot]; it.live && it.gen == uint32(id>>32) {
		return it
	}
	return nil
}

// newObjIter starts the state of a foreach over iterator object o and
// returns the register value that holds o (owned by the caller) and names
// the state.
func (s *vmState) newObjIter(o *object) value {
	var slot int
	if n := len(s.freeObjIters); n > 0 {
		slot = s.freeObjIters[n-1]
		s.freeObjIters = s.freeObjIters[:n-1]
	} else {
		slot = len(s.objIters)
		s.objIters = append(s.objIters, vmObjIter{})
	}
	gen := s.objIters[slot].gen + 1
	s.objIters[slot] = vmObjIter{live: true, gen: gen}
	return value{kind: valueObject, object: o, integer: -(int64(gen)<<32 | int64(slot+1))}
}

// endObjIter releases the state an iterator register names.
func (s *vmState) endObjIter(v value) {
	it := s.objIterOf(v)
	if it == nil {
		return
	}
	key := it.key
	*it = vmObjIter{gen: it.gen}
	if it.gen != math.MaxUint32 {
		s.freeObjIters = append(s.freeObjIters, int(uint32(-v.integer))-1)
	}
	_ = s.releaseValue(key)
}

// isTraversable reports an object whose class implements Traversable.
func (s *vmState) isTraversable(o *object) bool {
	return o != nil && (o.gen != nil || classIs(o.class, traversableBuiltin))
}

// callIterMethod calls the method name of o, a user class's Iterator or
// IteratorAggregate method, without arguments; the result is owned.
func (s *vmState) callIterMethod(o *object, name string) (value, error) {
	m, ok := s.h.resolveMethod(o.class, name, o.class)
	if !ok || m == nil || m.fn <= 0 {
		return value{}, s.internal("%s::%s() is not a method the program declares", o.class.name, name)
	}
	return s.invoke(&s.prog.funcs[m.fn-1], value{kind: valueObject, object: o}, m.owner, o.class, nil)
}

// objectIterator is get_iterator for Traversable object o: a Generator
// iterates itself (after the checks of zend_generator_get_iterator), an
// IteratorAggregate gives its iterator through getIterator, as many times
// as it takes to reach an Iterator, and an Iterator iterates itself
// (zend_user_it_get_new_iterator). The result holds the Iterator, owned.
func (s *vmState) objectIterator(o *object, byRef bool) (value, error) {
	cur := o
	cur.refcount++
	for {
		if g := cur.gen; g != nil {
			if g.frame == nil {
				_ = s.releaseObject(cur)
				return value{}, &Throwable{"Exception", "Cannot traverse an already closed generator"}
			}
			if byRef && !g.fn.byRef {
				_ = s.releaseObject(cur)
				return value{}, &Throwable{"Exception", "You can only iterate a generator by-reference if it declared that it yields by-reference"}
			}
			return value{kind: valueObject, object: cur}, nil
		}
		if !classIs(cur.class, iteratorAggregateBuiltin) {
			if byRef {
				_ = s.releaseObject(cur)
				return value{}, &Throwable{"Error", "An iterator cannot be used with foreach by reference"}
			}
			return value{kind: valueObject, object: cur}, nil
		}
		v, err := s.callIterMethod(cur, "getIterator")
		name := cur.class.name
		if rerr := s.releaseObject(cur); err == nil {
			err = rerr
		}
		if err != nil {
			_ = s.releaseValue(v)
			return value{}, err
		}
		if v.kind != valueObject || !s.isTraversable(v.object) {
			_ = s.releaseValue(v)
			return value{}, &Throwable{"Exception", "Objects returned by " + name + "::getIterator() must be traversable or implement interface Iterator"}
		}
		cur = v.object
	}
}

// iterRewind, iterValid, iterCurrent, iterKey and iterForward are the
// functions of zend_object_iterator for Iterator o.
func (s *vmState) iterRewind(o *object) error {
	if o.gen != nil {
		return s.genRewind(o)
	}
	v, err := s.callIterMethod(o, "rewind")
	if err != nil {
		return err
	}
	return s.releaseValue(v)
}

func (s *vmState) iterValid(o *object) (bool, error) {
	if g := o.gen; g != nil {
		if err := s.genEnsureInit(o); err != nil {
			return false, err
		}
		s.genCurrent(g)
		return g.frame != nil, nil
	}
	v, err := s.callIterMethod(o, "valid")
	if err != nil {
		return false, err
	}
	ok := s.h.toBool(v)
	return ok, s.releaseValue(v)
}

// iterCurrent is the current element, owned; for a generator that yields
// by reference it is the reference.
func (s *vmState) iterCurrent(o *object, byRef bool) (value, error) {
	if g := o.gen; g != nil {
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		v := s.genCurrent(g).value
		if !byRef {
			v = v.deref()
		}
		if v.kind == valueUndefined {
			return nullValue(), nil
		}
		retain(v)
		return v, nil
	}
	v, err := s.callIterMethod(o, "current")
	if err != nil {
		return value{}, err
	}
	return v, nil
}

func (s *vmState) iterKeyOf(o *object) (value, error) {
	if g := o.gen; g != nil {
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		k := s.genCurrent(g).key.deref()
		if k.kind == valueUndefined {
			return nullValue(), nil
		}
		retain(k)
		return k, nil
	}
	k, err := s.callIterMethod(o, "key")
	if err != nil {
		return value{}, err
	}
	if k.kind == valueReference {
		t := k.ref.target
		retain(t)
		_ = s.releaseValue(k)
		k = t
	}
	return k, nil
}

func (s *vmState) iterForward(o *object) error {
	if o.gen != nil {
		if err := s.genEnsureInit(o); err != nil {
			return err
		}
		return s.genResume(o)
	}
	v, err := s.callIterMethod(o, "next")
	if err != nil {
		return err
	}
	return s.releaseValue(v)
}

// objectIterStep fetches the next element of the Iterator in *it, whose
// position is *index (zero right after the rewind): every fetch but the
// first moves the iterator forward first and checks that it is still
// valid; validFirst checks the first one too (a yield from, whose rewind
// did not). It reports false at the end; the element and, with withKey,
// the key are owned.
func (s *vmState) objectIterStep(it *value, index *int64, withKey, validFirst bool) (more bool, v, k value, err error) {
	o := it.object
	first := *index == 0
	*index++
	if !first {
		if err := s.iterForward(o); err != nil {
			return false, value{}, value{}, err
		}
	}
	if !first || validFirst {
		ok, err := s.iterValid(o)
		if err != nil || !ok {
			return false, value{}, value{}, err
		}
	}
	if v, err = s.iterCurrent(o, false); err != nil {
		return false, value{}, value{}, err
	}
	if withKey {
		if k, err = s.iterKeyOf(o); err != nil {
			_ = s.releaseValue(v)
			return false, value{}, value{}, err
		}
	}
	return true, v, k, nil
}

// iterInitTraversable is ZEND_FE_RESET_R and FE_RESET_RW on Traversable
// object o (zend_fe_reset_iterator): the iterator is rewound and checked;
// an empty one is released at once, where PHP's jump reaches its FE_FREE.
func (s *vmState) iterInitTraversable(f *vmFrame, in *vmInstr, o *object, byRef bool) (bool, error) {
	old := f.regs[in.a]
	f.regs[in.a] = value{}
	if err := s.releaseValue(old); err != nil {
		return false, err
	}
	it, err := s.objectIterator(o, byRef)
	if err != nil {
		return false, err
	}
	if err := s.iterRewind(it.object); err != nil {
		_ = s.releaseValue(it)
		return false, err
	}
	ok, err := s.iterValid(it.object)
	if err != nil || !ok {
		if rerr := s.releaseValue(it); err == nil {
			err = rerr
		}
		return false, err
	}
	r := s.newObjIter(it.object)
	r.boolean = byRef
	f.regs[in.a] = r
	return true, nil
}

// iterNextTraversable is ZEND_FE_FETCH_R and FE_FETCH_RW on the Iterator in
// iterator register in.a: it moves forward (but for the first element),
// checks it is still valid, reads the current element and, when the next
// instruction takes the key, the key, before the element is assigned.
func (s *vmState) iterNextTraversable(f *vmFrame, in *vmInstr) (bool, error) {
	reg := f.regs[in.a]
	st := s.objIterOf(reg)
	if st == nil {
		return false, s.unsupported("advancing an iterator that IterInit did not create")
	}
	pc := f.pc
	withKey := pc+1 < len(f.code) && f.code[pc+1].op == bytecode.OpIterKey && f.code[pc+1].b == in.a
	o := reg.object
	o.refcount++
	defer func() { _ = s.releaseObject(o) }()
	index := st.index
	first := index == 0
	index++
	st.index = index
	if !first {
		if err := s.iterForward(o); err != nil {
			return false, err
		}
		ok, err := s.iterValid(o)
		if err != nil || !ok {
			return false, err
		}
	}
	v, err := s.iterCurrent(o, reg.boolean)
	if err != nil {
		return false, err
	}
	var k value
	if withKey {
		if k, err = s.iterKeyOf(o); err != nil {
			_ = s.releaseValue(v)
			return false, err
		}
	}
	// The register's state may have moved while user code ran.
	if st = s.objIterOf(f.regs[in.a]); st != nil {
		old := st.key
		st.key, st.hasKey = k, withKey
		if err := s.releaseValue(old); err != nil {
			_ = s.releaseValue(v)
			return false, err
		}
	} else {
		_ = s.releaseValue(k)
	}
	if reg.boolean {
		// FE_FETCH_RW of a generator that yields by reference: the
		// destination shares the reference.
		if v.kind != valueReference {
			ref := value{}
			if err := s.h.bind(&ref, &v); err != nil {
				return false, err
			}
			_ = s.releaseValue(v)
			v = ref
		}
		if bytecode.IterFlags(in.aux)&bytecode.IterLocation != 0 {
			dst, err := s.slot(f, in.b)
			if err == nil {
				err = s.h.bind(dst, &v)
			}
			if rerr := s.releaseValue(v); err == nil {
				err = rerr
			}
			return true, err
		}
		old := f.regs[in.b]
		f.regs[in.b] = v
		return true, s.releaseValue(old)
	}
	err = s.iterStore(f, in.aux, in.b, v)
	if rerr := s.releaseValue(v); err == nil {
		err = rerr
	}
	return true, err
}

// iterKeyTraversable is the key the fetch before it read, which goes to
// the key's destination and is released there.
func (s *vmState) iterKeyTraversable(f *vmFrame, in *vmInstr, st *vmObjIter) error {
	if !st.hasKey {
		return s.unsupported("reading the key of an iterator that has no current element")
	}
	k := st.key
	st.key, st.hasKey = value{}, false
	err := s.iterStore(f, in.aux, in.a, k)
	if rerr := s.releaseValue(k); err == nil {
		err = rerr
	}
	return err
}
