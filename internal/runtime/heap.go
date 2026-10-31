package runtime

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// ErrHeapClosed rejects use of a request heap after shutdown released it.
var ErrHeapClosed = errors.New("request heap is closed")

// class is the minimal class shape needed to model destruction.
type class struct {
	name       string
	destructor func(h *heap, this value) error
}

type object struct {
	handle           uint64
	class            *class
	refcount         int
	props            *phpArray // exclusively owned; not a separate PHP value
	destructorCalled bool
	freed            bool
	weak             *weakReference
	weakMaps         []*weakMap
}

// weakReference observes an object without owning it. PHP returns the same
// WeakReference for the same live object.
type weakReference struct{ target *object }

// weakMap owns its values but not its object keys.
type weakMap struct {
	keys    []*object
	entries map[*object]value
}

// heap owns the mutable PHP values of one request. It is not safe for
// concurrent use; an Execution must not share it with another request.
type heap struct {
	width      int
	nextHandle uint64
	live       map[uint64]*object
	roots      map[gcNode]uint64 // possible cycle roots with buffering order
	rootSeq    uint64
	closed     bool
}

func newHeap(width int) (*heap, error) {
	if width != 32 && width != 64 {
		return nil, fmt.Errorf("invalid PHP integer width %d: expected 32 or 64 bits", width)
	}
	return &heap{width: width, live: make(map[uint64]*object), roots: make(map[gcNode]uint64)}, nil
}

// Temporaries returned by constructors start with no owner. assign, bind, and
// property or element writes take a count; discard releases an unowned
// temporary that was never stored.

func (h *heap) newArrayValue() value { return value{kind: valueArray, array: newArray()} }

func (h *heap) newObject(c *class) (value, error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	h.nextHandle++
	o := &object{handle: h.nextHandle, class: c, props: newArray()}
	o.props.refcount = 1
	h.live[o.handle] = o
	return value{kind: valueObject, object: o}, nil
}

func (h *heap) discard(v value) error {
	switch v.kind {
	case valueArray:
		if v.array.refcount == 0 {
			return h.freeArray(v.array)
		}
	case valueObject:
		if v.object.refcount == 0 && !v.object.freed {
			return h.deleteObject(v.object)
		}
	}
	return nil
}

func retain(v value) {
	switch v.kind {
	case valueArray:
		v.array.refcount++
	case valueObject:
		v.object.refcount++
	case valueReference:
		v.ref.refcount++
	}
}

// release drops one logical owner and destroys values that reach zero.
// Destructor errors are returned after cleanup of the released value finishes.
func (h *heap) release(v value) error {
	switch v.kind {
	case valueArray:
		v.array.refcount--
		if v.array.refcount == 0 {
			return h.freeArray(v.array)
		}
		h.possibleRoot(v.array)
	case valueObject:
		v.object.refcount--
		if v.object.refcount == 0 {
			return h.deleteObject(v.object)
		}
		h.possibleRoot(v.object)
	case valueReference:
		v.ref.refcount--
		if v.ref.refcount == 0 {
			return h.release(v.ref.target)
		}
		switch t := v.ref.target; t.kind {
		case valueArray:
			h.possibleRoot(t.array)
		case valueObject:
			h.possibleRoot(t.object)
		}
	}
	return nil
}

func (h *heap) possibleRoot(n gcNode) {
	if _, ok := h.roots[n]; !ok && !h.closed {
		h.rootSeq++
		h.roots[n] = h.rootSeq
	}
}

func (h *heap) dropRoot(n gcNode) { delete(h.roots, n) }

// bufferedRoots returns roots in the order they were buffered.
func (h *heap) bufferedRoots() []gcNode {
	out := make([]gcNode, 0, len(h.roots))
	for n := range h.roots {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b gcNode) int { return cmp.Compare(h.roots[a], h.roots[b]) })
	return out
}

func (h *heap) freeArray(a *phpArray) error {
	h.dropRoot(a)
	var first error
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		if err := h.release(b.val); err != nil && first == nil {
			first = err
		}
	}
	a.buckets, a.index, a.live = nil, nil, 0
	return first
}

// deleteObject follows the PHP 8.5.12 order: run the destructor once while the
// object is temporarily owned, then free it only if nothing resurrected it.
func (h *heap) deleteObject(o *object) error {
	var first error
	if !o.destructorCalled {
		o.destructorCalled = true
		if o.class != nil && o.class.destructor != nil {
			o.refcount++
			first = o.class.destructor(h, value{kind: valueObject, object: o})
			o.refcount--
		}
	}
	if o.refcount == 0 && !o.freed {
		if err := h.freeObject(o); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (h *heap) freeObject(o *object) error {
	o.freed = true
	h.dropRoot(o)
	delete(h.live, o.handle)
	var first error
	if o.weak != nil {
		o.weak.target = nil
		o.weak = nil
	}
	for _, m := range o.weakMaps {
		if v, ok := m.entries[o]; ok {
			delete(m.entries, o)
			m.keys = slices.DeleteFunc(m.keys, func(k *object) bool { return k == o })
			if err := h.release(v); err != nil && first == nil {
				first = err
			}
		}
	}
	o.weakMaps = nil
	if err := h.freeArray(o.props); err != nil && first == nil {
		first = err
	}
	return first
}

// assign implements by-value assignment into a slot. A slot bound by
// reference is written through. The old value is released after the store,
// so its destructor observes the new value.
func (h *heap) assign(slot *value, v value) error {
	if h.closed {
		return ErrHeapClosed
	}
	v = v.deref()
	retain(v)
	var old value
	if slot.kind == valueReference {
		old, slot.ref.target = slot.ref.target, v
	} else {
		old, *slot = *slot, v
	}
	return h.release(old)
}

// bind implements $dst =& $src, converting src into a shared location.
func (h *heap) bind(dst, src *value) error {
	if h.closed {
		return ErrHeapClosed
	}
	if src.kind != valueReference {
		*src = value{kind: valueReference, ref: &reference{refcount: 1, target: *src}}
	}
	if dst == src {
		return nil
	}
	ref := *src
	retain(ref)
	old := *dst
	*dst = ref
	return h.release(old)
}

func (h *heap) unset(slot *value) error {
	old := *slot
	*slot = value{}
	return h.release(old)
}

// separate gives slot a writable array, creating one for null or undefined
// and duplicating a shared one.
func (h *heap) separate(slot *value) (*phpArray, error) {
	if h.closed {
		return nil, ErrHeapClosed
	}
	if slot.kind == valueReference {
		slot = &slot.ref.target
	}
	switch slot.kind {
	case valueUndefined, valueNull:
		a := newArray()
		a.refcount = 1
		*slot = value{kind: valueArray, array: a}
		return a, nil
	case valueArray:
		if slot.array.refcount == 1 {
			return slot.array, nil
		}
		dup := h.duplicate(slot.array)
		dup.refcount = 1
		old := *slot
		*slot = value{kind: valueArray, array: dup}
		return dup, h.release(old)
	}
	return nil, fmt.Errorf("cannot use value kind %d as an array", slot.kind)
}

// duplicate copies an array. As in PHP 8.5.12, a reference element with no
// other owner is copied as its value, while shared references stay shared.
func (h *heap) duplicate(src *phpArray) *phpArray {
	dup := &phpArray{nextFree: src.nextFree}
	if !src.packed() {
		dup.index = make(map[arrayKey]int, src.live)
	}
	pos := 0
	for b, ok := src.next(&pos); ok; b, ok = src.next(&pos) {
		v := b.val
		if v.kind == valueReference && v.ref.refcount == 1 &&
			(v.ref.target.kind != valueArray || v.ref.target.array != src) {
			v = v.ref.target
		}
		retain(v)
		if !src.packed() {
			dup.index[b.key] = len(dup.buckets)
		}
		dup.buckets = append(dup.buckets, &bucket{key: b.key, val: v})
		dup.live++
	}
	return dup
}

// element returns a writable element slot, creating it as null when absent.
func (h *heap) element(slot *value, key value) (*value, error) {
	a, err := h.separate(slot)
	if err != nil {
		return nil, err
	}
	k, err := normalizeKey(key, h.width)
	if err != nil {
		return nil, err
	}
	if b, ok := a.lookup(k); ok {
		return &b.val, nil
	}
	return &a.insert(k, nullValue(), h.width).val, nil
}

func (h *heap) appendElement(slot *value) (*value, error) {
	a, err := h.separate(slot)
	if err != nil {
		return nil, err
	}
	k, err := a.appendKey()
	if err != nil {
		return nil, err
	}
	return &a.insert(k, nullValue(), h.width).val, nil
}

func (h *heap) setElement(slot *value, key, v value) error {
	return h.storeElement(v, func() (*value, error) { return h.element(slot, key) })
}

func (h *heap) appendValue(slot *value, v value) error {
	return h.storeElement(v, func() (*value, error) { return h.appendElement(slot) })
}

// storeElement holds the right-hand side while the destination element is
// created, so writing an array into itself separates it first: $a[] = $a
// yields [1, [1]] rather than a recursive array.
func (h *heap) storeElement(v value, element func() (*value, error)) error {
	v = v.deref()
	retain(v)
	e, err := element()
	if err == nil {
		err = h.assign(e, v)
	}
	if rerr := h.release(v); err == nil {
		err = rerr
	}
	return err
}

func (h *heap) getElement(container value, key value) (value, bool, error) {
	container = container.deref()
	if container.kind != valueArray {
		return value{}, false, nil
	}
	k, err := normalizeKey(key, h.width)
	if err != nil {
		return value{}, false, err
	}
	if b, ok := container.array.lookup(k); ok {
		return b.val.deref(), true, nil
	}
	return value{}, false, nil
}

func (h *heap) unsetElement(slot *value, key value) error {
	if slot.deref().kind != valueArray {
		return nil
	}
	a, err := h.separate(slot)
	if err != nil {
		return err
	}
	k, err := normalizeKey(key, h.width)
	if err != nil {
		return err
	}
	if old, ok := a.remove(k); ok {
		return h.release(old)
	}
	return nil
}

// foreachRef iterates an array by reference. As in PHP, the iterated variable
// becomes a shared location that the loop owns, so writes and appends in the
// body modify the iterated array rather than a separated copy. Each element is
// bound to item, and appended elements are visited. If the body assigns a
// different array to the variable, iteration continues from that array's
// start; if the loop itself has to separate a shared array, the cursor moves
// to the copy after the last visited key.
func (h *heap) foreachRef(arr, item *value, fn func(key arrayKey) error) error {
	var loop value
	if err := h.bind(&loop, arr); err != nil {
		return err
	}
	defer h.release(loop)
	var cur *phpArray
	var last arrayKey
	pos := 0
	for {
		if t := loop.ref.target; cur != nil && (t.kind != valueArray || t.array != cur) {
			cur, pos = nil, 0
		}
		a, err := h.separate(&loop)
		if err != nil {
			return err
		}
		if cur != nil && a != cur {
			pos = 0
			if i, ok := a.find(last); ok {
				pos = i + 1
			}
		}
		cur = a
		b, ok := a.next(&pos)
		if !ok {
			return nil
		}
		last = b.key
		if err := h.bind(item, &b.val); err != nil {
			return err
		}
		if err := fn(b.key); err != nil {
			return err
		}
	}
}

func (h *heap) property(obj value, name string) (*value, error) {
	obj = obj.deref()
	if obj.kind != valueObject || obj.object.freed {
		return nil, fmt.Errorf("property access requires a live object")
	}
	props := value{kind: valueArray, array: obj.object.props}
	return h.element(&props, value{kind: valueString, text: name})
}

func (h *heap) setProperty(obj value, name string, v value) error {
	p, err := h.property(obj, name)
	if err != nil {
		return err
	}
	return h.assign(p, v)
}

func (h *heap) weakReferenceFor(obj value) (*weakReference, error) {
	obj = obj.deref()
	if obj.kind != valueObject || obj.object.freed {
		return nil, fmt.Errorf("WeakReference requires a live object")
	}
	if obj.object.weak == nil {
		obj.object.weak = &weakReference{target: obj.object}
	}
	return obj.object.weak, nil
}

// get returns the target while its storage has not been freed. During its
// destructor the object is still reachable, matching PHP 8.5.12.
func (w *weakReference) get() (value, bool) {
	if w.target == nil {
		return nullValue(), false
	}
	return value{kind: valueObject, object: w.target}, true
}

func newWeakMap() *weakMap { return &weakMap{entries: make(map[*object]value)} }

func (h *heap) weakMapSet(m *weakMap, key, v value) error {
	key = key.deref()
	if key.kind != valueObject || key.object.freed {
		return fmt.Errorf("WeakMap key must be a live object")
	}
	o := key.object
	v = v.deref()
	retain(v)
	old, exists := m.entries[o]
	m.entries[o] = v
	if !exists {
		m.keys = append(m.keys, o)
		o.weakMaps = append(o.weakMaps, m)
		return nil
	}
	return h.release(old)
}

func (m *weakMap) count() int { return len(m.entries) }

// gcNode is a *phpArray, *object, or *reference visited by the collector.
type gcNode = any

// edges lists the composite nodes a node keeps alive. References are
// traversed but are not counted as freed garbage.
func edges(n gcNode, visit func(gcNode)) {
	var cell func(v value)
	cell = func(v value) {
		switch v.kind {
		case valueArray:
			visit(v.array)
		case valueObject:
			visit(v.object)
		case valueReference:
			visit(v.ref)
		}
	}
	var arr *phpArray
	switch x := n.(type) {
	case *phpArray:
		arr = x
	case *object:
		arr = x.props
	case *reference:
		cell(x.target)
		return
	}
	pos := 0
	for b, ok := arr.next(&pos); ok; b, ok = arr.next(&pos) {
		cell(b.val)
	}
}

func refcountOf(n gcNode) int {
	switch x := n.(type) {
	case *phpArray:
		return x.refcount
	case *object:
		return x.refcount
	case *reference:
		return x.refcount
	}
	return 0
}

// garbage returns, in discovery order, the nodes reachable from roots whose
// counts are explained entirely by edges inside that set.
func garbage(roots []gcNode) []gcNode {
	var order []gcNode
	seen := map[gcNode]bool{}
	internal := map[gcNode]int{}
	var walk func(n gcNode)
	walk = func(n gcNode) {
		if seen[n] {
			return
		}
		seen[n] = true
		order = append(order, n)
		edges(n, func(c gcNode) {
			internal[c]++
			walk(c)
		})
	}
	for _, r := range roots {
		walk(r)
	}
	alive := map[gcNode]bool{}
	var mark func(n gcNode)
	mark = func(n gcNode) {
		if alive[n] {
			return
		}
		alive[n] = true
		edges(n, mark)
	}
	for _, n := range order {
		if refcountOf(n) > internal[n] {
			mark(n)
		}
	}
	var out []gcNode
	for _, n := range order {
		if !alive[n] {
			out = append(out, n)
		}
	}
	return out
}

// collectCycles implements gc_collect_cycles for the buffered roots. Garbage
// destructors run once; any node they resurrect survives, and the remainder is
// freed without running destructors again. It returns the number of freed
// arrays and objects.
func (h *heap) collectCycles() (int, error) {
	if h.closed {
		return 0, ErrHeapClosed
	}
	roots := h.bufferedRoots()
	clear(h.roots)
	white := garbage(roots)
	var first error
	ranDestructor := false
	for _, n := range white {
		o, ok := n.(*object)
		if !ok || o.destructorCalled {
			continue
		}
		o.destructorCalled = true
		if o.class != nil && o.class.destructor != nil {
			ranDestructor = true
			o.refcount++
			if err := o.class.destructor(h, value{kind: valueObject, object: o}); err != nil && first == nil {
				first = err
			}
			o.refcount--
		}
	}
	if ranDestructor {
		// Destructors may break the cycle so that members are freed by
		// ordinary counting; those are no longer garbage and are not counted.
		// A destructed object whose count fell to zero is still freed here.
		white = garbage(slices.DeleteFunc(white, func(n gcNode) bool {
			if o, ok := n.(*object); ok {
				return o.freed
			}
			return refcountOf(n) == 0
		}))
	}
	dead := map[gcNode]bool{}
	for _, n := range white {
		dead[n] = true
	}
	freed := 0
	for _, n := range white {
		var err error
		switch x := n.(type) {
		case *phpArray:
			freed++
			err = h.freeGarbage(x, dead)
		case *object:
			freed++
			x.freed, x.destructorCalled = true, true
			delete(h.live, x.handle)
			if x.weak != nil {
				x.weak.target, x.weak = nil, nil
			}
			for _, m := range x.weakMaps {
				if v, ok := m.entries[x]; ok {
					delete(m.entries, x)
					m.keys = slices.DeleteFunc(m.keys, func(k *object) bool { return k == x })
					if e := h.releaseOutside(v, dead); e != nil && err == nil {
						err = e
					}
				}
			}
			x.weakMaps = nil
			if e := h.freeGarbage(x.props, dead); e != nil && err == nil {
				err = e
			}
		case *reference:
			if e := h.releaseOutside(x.target, dead); e != nil {
				err = e
			}
		}
		h.dropRoot(n)
		if err != nil && first == nil {
			first = err
		}
	}
	// Survivors stay buffered so a later collection can find them again.
	for _, r := range roots {
		if !dead[r] && refcountOf(r) > 0 {
			h.possibleRoot(r)
		}
	}
	return freed, first
}

func (h *heap) freeGarbage(a *phpArray, dead map[gcNode]bool) error {
	var first error
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		if err := h.releaseOutside(b.val, dead); err != nil && first == nil {
			first = err
		}
	}
	a.buckets, a.index, a.live = nil, nil, 0
	return first
}

// releaseOutside drops an edge unless it points inside the garbage set, whose
// members are freed directly.
func (h *heap) releaseOutside(v value, dead map[gcNode]bool) error {
	var n gcNode
	switch v.kind {
	case valueArray:
		n = v.array
	case valueObject:
		n = v.object
	case valueReference:
		n = v.ref
	default:
		return nil
	}
	if dead[n] {
		return nil
	}
	return h.release(v)
}

// shutdown follows PHP 8.5.12 request shutdown. Global objects owned only by
// their variable are destroyed in reverse symbol-table order until no more
// qualify, remaining live objects get destructors in creation order, and then
// all storage is released without further destructor calls.
func (h *heap) shutdown(globals *value) error {
	if h.closed {
		return ErrHeapClosed
	}
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if globals.kind == valueArray {
		a := globals.array
		for {
			before := a.count()
			for i := len(a.buckets) - 1; i >= 0; i-- {
				b := a.buckets[i]
				if b == nil || b.val.kind != valueObject || b.val.object.refcount != 1 {
					continue
				}
				old, _ := a.remove(b.key)
				keep(h.release(old))
			}
			if a.count() == before {
				break
			}
		}
	}
	// Objects created by these destructors get later handles and are visited
	// too, as PHP's object store walk reads its growing top each step.
	for id := uint64(1); id <= h.nextHandle; id++ {
		o, ok := h.live[id]
		if !ok || o.destructorCalled {
			continue
		}
		o.destructorCalled = true
		if o.class != nil && o.class.destructor != nil {
			o.refcount++
			keep(o.class.destructor(h, value{kind: valueObject, object: o}))
			o.refcount--
		}
	}
	h.closed = true
	for _, o := range h.live {
		o.freed = true
		if o.weak != nil {
			o.weak.target, o.weak = nil, nil
		}
	}
	h.live, h.roots = nil, nil
	*globals = value{}
	return first
}
