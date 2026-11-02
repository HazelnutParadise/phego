package runtime

import (
	"errors"
	"fmt"
	"slices"
)

// ErrHeapClosed rejects use of a request heap after shutdown released it.
var ErrHeapClosed = errors.New("request heap is closed")

// class is the minimal class shape needed to model destruction.
type class struct {
	name            string
	destructor      func(h *heap, this value) error
	destructorOwner *class // immutable identity for prototype destructor callbacks
	schema          *classSchema
}

type object struct {
	owner            *heap
	handle           uint64
	class            *class
	refcount         int
	props            *propertyTable // exclusively owned; not a separate PHP value
	destructorCalled bool
	freed            bool
	weak             *weakReference
	weakMaps         []*weakMap
	lazy             *lazyObject
	hooks            map[string]bool
}

// weakReference observes an object without owning it. PHP returns the same
// WeakReference for the same live object.
type weakReference struct{ target *object }

// weakMap owns its values but not its object keys.
type weakMap struct {
	owner    *heap
	identity *object
	closed   bool
	keys     []*object
	entries  map[*object]value
}

// heap owns the mutable PHP values of one request. It is not safe for
// concurrent use; an Execution must not share it with another request.
type heap struct {
	width         int
	nextHandle    uint64
	freeHandles   []uint64
	noHandleReuse bool
	live          map[uint64]*object
	roots         map[gcNode]uint64 // possible cycle roots with buffering order
	rootSlots     []gcNode
	freeRootSlots []int
	closed        bool
	collecting    bool
	// diagnostics are the PHP warnings and deprecations raised by modeled
	// operations, in order.
	diagnostics []diagnostic
	// PHP 8.5.12 array user-sort deprecation state is request-local. A nested
	// user sort resets it; returning from that sort does not restore it.
	// REQ-017; T-033/T-034.
	sortCompareDeprecated bool
	enums                 map[*class]map[string]value
	comparingObjects      map[*object]bool
	resources             map[uint64]*phpResource
	nextResource          uint64
	typedSources          map[*reference][]propertySource
	execution             *execution
	classStatics          map[*class]*propertyTable
	gcTemporaries         []gcTemporary
	weakMapObjects        map[*object]*weakMap
	classes               map[string]*class
	currentHook           activePropertyHook
}

func newHeap(width int) (*heap, error) {
	if width != 32 && width != 64 {
		return nil, fmt.Errorf("invalid PHP integer width %d: expected 32 or 64 bits", width)
	}
	return &heap{width: width, live: make(map[uint64]*object), roots: make(map[gcNode]uint64), nextResource: 4}, nil
}

// Temporaries returned by constructors start with no owner. assign, bind, and
// property or element writes take a count; discard releases an unowned
// temporary that was never stored.

func (h *heap) newArrayValue() value {
	a := newArray()
	a.owner = h
	return value{kind: valueArray, array: a}
}

func (h *heap) newObject(c *class) (value, error) {
	v, err := h.newObjectShell(c)
	if err != nil {
		return value{}, err
	}
	o := v.object
	if c != nil && c.schema != nil {
		if err := h.initializeProperties(o); err != nil {
			o.destructorCalled = true
			_ = h.freeObject(o)
			return value{}, err
		}
	}
	return v, nil
}

// One safe Go allocation holds the identity and its private property table.
// Interior pointers preserve both addresses; resources still allocate only an
// identity and do not pay for an unused property table.
func (h *heap) newObjectShell(c *class) (value, error) {
	if err := h.runtimeCheck(); err != nil {
		return value{}, err
	}
	var handle uint64
	if !h.noHandleReuse && len(h.freeHandles) > 0 {
		last := len(h.freeHandles) - 1
		handle = h.freeHandles[last]
		h.freeHandles = h.freeHandles[:last]
	} else {
		h.nextHandle++
		handle = h.nextHandle
	}
	var layout *propertyLayout
	if c != nil && c.schema != nil {
		layout = c.schema.layout
	}
	storage := &struct {
		object     object
		properties propertyTable
	}{}
	o := &storage.object
	*o = object{owner: h, handle: handle, class: c, props: &storage.properties}
	o.props.initializeLayout(layout)
	h.live[handle] = o
	return value{kind: valueObject, object: o}, nil
}

func (h *heap) discard(v value) error {
	if h.closed && v.kind == valueObject && v.object != nil && v.object.owner == h &&
		v.object.freed && h.live[v.object.handle] != v.object {
		if v.object.refcount > 0 {
			return h.release(v)
		}
		return nil
	}
	if err := h.claimValues(v); err != nil {
		return err
	}
	switch v.kind {
	case valueReference:
		if v.ref.refcount == 0 {
			retain(v)
			return h.release(v)
		}
	case valueResource:
		if v.object.refcount == 0 {
			retain(v)
			return h.releaseResource(v)
		}
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
	case valueObject, valueResource:
		v.object.refcount++
	case valueReference:
		v.ref.refcount++
	}
}

// release drops one logical owner and destroys values that reach zero.
// Destructor errors are returned after cleanup of the released value finishes.
func (h *heap) release(v value) error {
	if h.closed && v.kind == valueObject && v.object != nil && v.object.owner == h &&
		v.object.freed && h.live[v.object.handle] != v.object {
		if v.object.refcount <= 0 {
			return fmt.Errorf("released object has no owned reference")
		}
		v.object.refcount--
		return nil
	}
	if err := h.claimValues(v); err != nil {
		return err
	}
	switch v.kind {
	case valueResource:
		return h.releaseResource(v)
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
		var index int
		if len(h.freeRootSlots) > 0 {
			last := len(h.freeRootSlots) - 1
			index = h.freeRootSlots[last]
			h.freeRootSlots = h.freeRootSlots[:last]
			h.rootSlots[index] = n
		} else {
			index = len(h.rootSlots)
			h.rootSlots = append(h.rootSlots, n)
		}
		h.roots[n] = uint64(index)
	}
}

func (h *heap) dropRoot(n gcNode) {
	if index, ok := h.roots[n]; ok {
		h.rootSlots[index] = nil
		h.freeRootSlots = append(h.freeRootSlots, int(index))
		delete(h.roots, n)
	}
}

// PHP 8.5.12 reuses holes LIFO and compacts by filling early holes from the
// tail. A stable compaction or a monotonically increasing timestamp changes
// observable destructor order. Slots also remove the old map-to-slice sort.
func (h *heap) bufferedRoots() []gcNode {
	last := len(h.rootSlots) - 1
	for first := 0; first <= last; first++ {
		if h.rootSlots[first] != nil {
			continue
		}
		for last > first && h.rootSlots[last] == nil {
			last--
		}
		if last <= first {
			break
		}
		n := h.rootSlots[last]
		h.rootSlots[first] = n
		h.rootSlots[last] = nil
		h.roots[n] = uint64(first)
		last--
	}
	for len(h.rootSlots) > 0 && h.rootSlots[len(h.rootSlots)-1] == nil {
		h.rootSlots = h.rootSlots[:len(h.rootSlots)-1]
	}
	h.freeRootSlots = nil
	return slices.Clone(h.rootSlots)
}

func (h *heap) freeArray(a *phpArray) error {
	if err := h.claimValues(value{kind: valueArray, array: a}); err != nil {
		return err
	}
	h.dropRoot(a)
	var first error
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		first = h.mergeObjectErrors(first, h.release(b.val))
	}
	for it := range a.cursors {
		delete(it.arrays, a)
	}
	a.buckets, a.index, a.live, a.cursors = nil, nil, 0, nil
	return first
}

// deleteObject follows the PHP 8.5.12 order: run the destructor once while the
// object is temporarily owned, then free it only if nothing resurrected it.
func (h *heap) deleteObject(o *object) error {
	if err := h.validateObject(o); err != nil {
		return err
	}
	var first error
	if !o.destructorCalled {
		o.destructorCalled = true
		if objectHasDestructor(o) {
			o.refcount++
			first = o.class.destructor(h, value{kind: valueObject, object: o})
			o.refcount--
		}
	}
	if o.refcount == 0 && !o.freed {
		first = h.mergeObjectErrors(first, h.freeObject(o))
	}
	return first
}

func (h *heap) freeObject(o *object) error {
	if err := h.validateObject(o); err != nil {
		return err
	}
	o.freed = true
	h.dropRoot(o)
	delete(h.live, o.handle)
	var first error
	if o.weak != nil {
		o.weak.target = nil
		o.weak = nil
	}
	first = h.mergeObjectErrors(first, h.freeWeakMap(o, nil))
	maps := o.weakMaps
	o.weakMaps = nil
	for _, m := range maps {
		if v, ok := m.entries[o]; ok {
			delete(m.entries, o)
			m.keys = slices.DeleteFunc(m.keys, func(k *object) bool { return k == o })
			first = h.mergeObjectErrors(first, h.release(v))
		}
	}
	first = h.mergeObjectErrors(first, h.freeLazyStorage(o, nil))
	first = h.mergeObjectErrors(first, h.freeProperties(o, nil))
	if !h.noHandleReuse {
		h.freeHandles = append(h.freeHandles, o.handle)
	}
	return first
}

// assign implements by-value assignment into a slot. A slot bound by
// reference is written through. The old value is released after the store,
// so its destructor observes the new value.
func (h *heap) assign(slot *value, v value) error {
	return h.assignPHP(slot, v, true)
}

// The source file controls scalar coercion for assignments through PHP aliases.
// Keep the existing host helper strict; VM adapters pass their actual mode.
func (h *heap) assignPHP(slot *value, v value, strict bool) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	if err := h.claimValues(*slot, v); err != nil {
		return err
	}
	v = v.deref()
	if v.kind == valueResource {
		if _, err := h.resourceRecord(v); err != nil {
			return err
		}
	}
	if slot.kind == valueReference {
		var err error
		v, err = h.checkReferenceWrite(slot.ref, v, strict)
		if err != nil {
			return err
		}
	}
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
	if err := h.claimValues(*dst, *src); err != nil {
		return err
	}
	if err := ownReferenceLocations(dst, src); err != nil {
		return err
	}
	if src.kind != valueReference {
		*src = value{kind: valueReference, ref: &reference{owner: h, refcount: 1, target: *src}}
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

// bindForeachItem preserves the direct-object destruction boundary of a
// foreach CV. Keep the old slot owned while its imminent destructor runs, so
// a reentrant write cannot release an already-consumed reference target.
// Capture the next reference first: that destructor may remove its bucket.
// General assignment/bind order and object-graph release remain unchanged.
// REQ-016; T-031/032/034.
func (h *heap) bindForeachItem(dst, src *value) error {
	if h.closed {
		return ErrHeapClosed
	}
	if err := h.claimValues(*dst, *src); err != nil {
		return err
	}
	if err := ownReferenceLocations(dst, src); err != nil {
		return err
	}
	if src.kind != valueReference {
		*src = value{kind: valueReference, ref: &reference{owner: h, refcount: 1, target: *src}}
	}
	if dst == src {
		return nil
	}
	next := *src
	retain(next)
	old := *dst
	var first error
	if old.kind != valueReference || old.ref.refcount == 1 {
		v := old.deref()
		if v.kind == valueObject && v.object.refcount == 1 && !v.object.destructorCalled && v.object.class != nil && v.object.class.destructor != nil {
			first = h.deleteObject(v.object)
		}
	}
	old = *dst
	*dst = next
	if err := h.release(old); first == nil {
		first = err
	}
	if first == nil && h.closed {
		first = ErrHeapClosed
	}
	return first
}

// A writable location takes ownership of a constructor-style temporary before
// its value is moved into a reference or overwritten. Validate both locations
// first, and count the raw wrapper rather than its dereferenced target.
func ownReferenceLocations(dst, src *value) error {
	sourceTemporary, destinationTemporary := methodRefcount(*src) == 0, methodRefcount(*dst) == 0
	for _, slot := range [2]*value{dst, src} {
		switch slot.kind {
		case valueArray, valueObject, valueResource, valueReference:
			if methodRefcount(*slot) < 0 {
				return fmt.Errorf("reference location has a negative owner count")
			}
		}
	}
	if sourceTemporary {
		retain(*src)
	}
	if dst != src && destinationTemporary {
		retain(*dst)
	}
	return nil
}

func (h *heap) unset(slot *value) error {
	if err := h.claimValues(*slot); err != nil {
		return err
	}
	old := *slot
	*slot = value{}
	return h.release(old)
}

// separate gives slot a writable array, creating one for null or undefined
// and duplicating a shared one.
func (h *heap) separate(slot *value) (*phpArray, error) {
	if err := h.runtimeCheck(); err != nil {
		return nil, err
	}
	if err := h.claimValues(*slot); err != nil {
		return nil, err
	}
	if slot.kind == valueReference {
		if slot.ref.target.kind == valueNull || slot.ref.target.kind == valueUndefined {
			for _, source := range h.typedSources[slot.ref] {
				if !acceptsObjectType(source.info.typeName, value{kind: valueArray}) {
					return nil, &Throwable{"TypeError", "Cannot auto-initialize an array inside a reference held by property " + source.info.label() + " of type " + source.info.typeName}
				}
			}
		}
		slot = &slot.ref.target
	}
	switch slot.kind {
	case valueUndefined, valueNull:
		a := newArray()
		a.owner = h
		a.refcount = 1
		*slot = value{kind: valueArray, array: a}
		return a, nil
	case valueArray:
		if slot.array.refcount < 0 {
			return nil, fmt.Errorf("array has a negative owner count")
		}
		if slot.array.refcount == 0 {
			slot.array.refcount = 1
			return slot.array, nil
		}
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
	// Callers validate the source first; copying only preserves same-heap edges.
	// REQ-017; T-033/T-034. An empty COW copy is uninitialized even if the
	// source was allocated. It keeps nextFree, but not capacity or iterators.
	if src.live == 0 {
		dup := newArray()
		dup.owner = h
		dup.nextFree = src.nextFree
		return dup
	}
	dup := &phpArray{owner: h, nextFree: src.nextFree, limit: src.limit, pointer: src.pointer}
	if !src.packed() {
		dup.index = make(map[arrayKey]int, src.live)
	} else {
		dup.buckets = make([]*bucket, len(src.buckets))
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
			if pos-1 == src.pointer {
				dup.pointer = len(dup.buckets)
			}
			dup.index[b.key] = len(dup.buckets)
			dup.buckets = append(dup.buckets, &bucket{key: b.key, val: v})
		} else {
			dup.buckets[pos-1] = &bucket{key: b.key, val: v}
		}
		dup.live++
	}
	// REQ-017; T-033/034: PHP resets an invalid internal pointer on COW.
	if src.pointer >= len(src.buckets) {
		dup.pointer = 0
	}
	for it, cursor := range src.cursors {
		copiedPos := cursor
		if !src.packed() && cursor < len(src.buckets) {
			copiedPos = 0
			for _, b := range src.buckets[:cursor] {
				if b != nil {
					copiedPos++
				}
			}
		}
		if dup.cursors == nil {
			dup.cursors = make(map[*arrayIterator]int)
		}
		dup.cursors[it] = copiedPos
		it.arrays[dup] = struct{}{}
	}
	return dup
}

// element returns a writable element slot, creating it as null when absent.
func (h *heap) element(slot *value, key value) (*value, error) {
	if err := h.claimValues(*slot, key); err != nil {
		return nil, err
	}
	a, err := h.separate(slot)
	if err != nil {
		return nil, err
	}
	k, err := h.arrayKey(key, false)
	if err != nil {
		return nil, err
	}
	if b, ok := a.lookup(k); ok {
		return &b.val, nil
	}
	return &a.insert(k, nullValue(), h.width).val, nil
}

func (h *heap) appendElement(slot *value) (*value, error) {
	if err := h.claimValues(*slot); err != nil {
		return nil, err
	}
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
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	if err := h.claimValues(*slot, key, v); err != nil {
		return err
	}
	if err := ownArrayLocation(slot); err != nil {
		return err
	}
	return h.storeElement(v, func() (*value, error) { return h.element(slot, key) })
}

func (h *heap) appendValue(slot *value, v value) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	if err := h.claimValues(*slot, v); err != nil {
		return err
	}
	if err := ownArrayLocation(slot); err != nil {
		return err
	}
	return h.storeElement(v, func() (*value, error) { return h.appendElement(slot) })
}

// Passing an unowned array as a writable location acquires its location owner
// before retaining the RHS. A copied Go value alone never acquires an owner.
func ownArrayLocation(slot *value) error {
	if slot.kind == valueReference {
		slot = &slot.ref.target
	}
	if slot.kind != valueArray {
		return nil
	}
	if slot.array.refcount < 0 {
		return fmt.Errorf("array has a negative owner count")
	}
	if slot.array.refcount == 0 {
		slot.array.refcount = 1
	}
	return nil
}

// storeElement holds the right-hand side while the destination element is
// created, so writing an array into itself separates it first: $a[] = $a
// yields [1, [1]] rather than a recursive array.
func (h *heap) storeElement(v value, element func() (*value, error)) error {
	if err := h.claimValues(v); err != nil {
		return err
	}
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
	if err := h.claimValues(container, key); err != nil {
		return value{}, false, err
	}
	container = container.deref()
	if container.kind != valueArray {
		return value{}, false, nil
	}
	k, err := h.arrayKey(key, false)
	if err != nil {
		return value{}, false, err
	}
	if b, ok := container.array.lookup(k); ok {
		return b.val.deref(), true, nil
	}
	return value{}, false, nil
}

func (h *heap) unsetElement(slot *value, key value) error {
	if err := h.claimValues(*slot, key); err != nil {
		return err
	}
	if slot.deref().kind != valueArray {
		return nil
	}
	a, err := h.separate(slot)
	if err != nil {
		return err
	}
	k, err := h.arrayKey(key, true)
	if err != nil {
		return err
	}
	if old, ok := a.remove(k); ok {
		return h.release(old)
	}
	return nil
}

// foreachRef iterates an array by reference. The loop owns the iterated
// location. It separates at initialization or an unrelated replacement, not
// on every fetch of a shared array. Each element is bound to item. When the
// body separates an array, positions are tracked in candidate copies; when
// the body assigns an unrelated array, iteration starts at its internal pointer.
// A tail deletion clamps the cursor, as PHP does, so an
// element appended into the freed slot is still visited. Copying a mixed
// array with holes can retain an exhausted cursor instead of visiting an
// append before that raw position, as in the fixed PHP 8.5.12 fixtures.
func (h *heap) foreachRef(arr, item *value, fn func(key arrayKey) error) (err error) {
	var loop value
	if err := h.bind(&loop, arr); err != nil {
		return err
	}
	defer func() {
		// Releasing the loop's hold can destroy the array if the body unset
		// the variable; destructor errors then surface from the loop.
		if rerr := h.release(loop); err == nil {
			err = rerr
		}
	}()
	if t := loop.ref.target; t.kind != valueArray {
		h.warnForeach(t)
		return nil
	}
	it := &arrayIterator{arrays: make(map[*phpArray]struct{})}
	defer func() {
		for a := range it.arrays {
			delete(a.cursors, it)
		}
	}()
	first := true
	var current *phpArray
	for {
		t := loop.ref.target
		if t.kind != valueArray {
			// PHP stops with a warning and keeps the value the body assigned.
			h.warnForeach(t)
			return nil
		}
		a := t.array
		pos, known := a.cursors[it]
		if first || !known {
			// FE_RESET separates once. A later fetch separates only for an
			// unrelated replacement, not just because the current array is
			// shared: binding the next item must release the previous item
			// before a subsequent body write decides which references to copy.
			for old := range it.arrays {
				delete(old.cursors, it)
				delete(it.arrays, old)
			}
			var err error
			a, err = h.separate(&loop)
			if err != nil {
				return err
			}
			if !first {
				pos = a.pointer
			} else {
				pos = 0
			}
		} else if a != current {
			// Selecting a tracked copy consumes the other candidates. A
			// later switch back is a replacement, not a stale saved cursor.
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
		current = a
		first = false
		b, ok := a.next(&pos)
		a.cursors[it] = pos
		if !ok {
			return nil
		}
		if err := h.bindForeachItem(item, &b.val); err != nil {
			return err
		}
		if err := fn(b.key); err != nil {
			return err
		}
	}
}

func (h *heap) warnForeach(v value) {
	h.warn(levelWarning, "foreach() argument must be of type array|object, "+typeName(v)+" given")
}

// typeName is PHP's get_debug_type() spelling for the modeled kinds.
func typeName(v value) string {
	switch v.deref().kind {
	case valueNull, valueUndefined:
		return "null"
	case valueBool:
		return "bool"
	case valueInt:
		return "int"
	case valueFloat:
		return "float"
	case valueString:
		return "string"
	case valueArray:
		return "array"
	case valueResource:
		if v.deref().object.freed {
			return "resource (closed)"
		}
		return "resource"
	}
	return v.deref().object.class.name
}

func (h *heap) property(obj value, name string) (*value, error) {
	if h.closed {
		return nil, ErrHeapClosed
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return nil, err
	}
	return o.props.slot(name), nil
}

func (h *heap) setProperty(obj value, name string, v value) error {
	p, err := h.property(obj, name)
	if err != nil {
		return err
	}
	return h.assign(p, v)
}

func (h *heap) weakReferenceFor(obj value) (*weakReference, error) {
	if err := h.claimValues(obj); err != nil {
		return nil, err
	}
	obj = obj.deref()
	if obj.kind != valueObject || obj.object == nil || obj.object.freed || obj.object.owner != h {
		return nil, fmt.Errorf("WeakReference requires a live object")
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return nil, err
	}
	if o.weak == nil {
		o.weak = &weakReference{target: o}
	}
	return o.weak, nil
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
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	if m == nil || (m.owner != nil && m.owner != h) || m.closed {
		return fmt.Errorf("WeakMap belongs to another or closed request")
	}
	if m.identity != nil {
		if err := h.validateObject(m.identity); err != nil {
			return err
		}
	}
	pending, err := h.pendingValues(key, v)
	if err != nil {
		return err
	}
	rawKey := key.deref()
	if rawKey.kind != valueObject || h.validateObject(rawKey.object) != nil {
		return fmt.Errorf("WeakMap key must be a live object")
	}
	o := rawKey.object
	old, exists := m.entries[o]
	if exists {
		next, err := h.pendingValues(old)
		if err != nil {
			return err
		}
		pending = append(pending, next...)
	}
	h.commitValueOwnership(pending)
	v = v.deref()
	retain(v)
	if m.entries == nil {
		m.entries = make(map[*object]value)
	}
	if m.owner == nil {
		m.owner = h
	}
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
		x.props.each(func(_ string, v *value) { cell(*v) })
		if x.lazy != nil {
			cell(x.lazy.initializer)
			cell(x.lazy.target)
		}
		// A WeakMap entry value is kept alive through its key, so a value
		// that refers back to its key forms a collectable cycle.
		for _, m := range x.weakMaps {
			if m.identity != nil {
				continue
			}
			if v, ok := m.entries[x]; ok {
				cell(v)
			}
		}
		return
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

// collectCycles implements gc_collect_cycles for the buffered roots. Garbage
// destructors run once; any node they resurrect survives, and the remainder is
// freed without running destructors again. It returns the number of freed
// arrays and objects.
func (h *heap) collectCycles() (int, error) {
	if err := h.runtimeCheck(); err != nil {
		return 0, err
	}
	if h.collecting {
		return 0, nil
	}
	h.collecting = true
	defer func() { h.collecting = false }()
	total, first, rerun := h.collectCyclePass()
	if rerun {
		n, err, _ := h.collectCyclePass()
		total += n
		if first == nil {
			first = err
		}
	}
	h.bufferedRoots()
	// PHP re-buffers live VM temporaries after collection. Visit current
	// frames first and each frame's live ranges in creation order.
	for end := len(h.gcTemporaries); end > 0; {
		start := end - 1
		depth := h.gcTemporaries[start].depth
		for start > 0 && h.gcTemporaries[start-1].depth == depth {
			start--
		}
		for _, temp := range h.gcTemporaries[start:end] {
			h.possibleRoot(temp.node)
		}
		end = start
	}
	return total, first
}

func (h *heap) collectCyclePass() (int, error, bool) {
	roots := h.bufferedRoots()
	white, err := garbageChecked(roots, h.runtimeCheck)
	if err != nil {
		return 0, err, false
	}
	// Remove live roots before the second compaction, as the target PHP
	// release does. This can change the order of the remaining white roots.
	whiteRoots := make(map[gcNode]bool, len(white))
	for _, n := range white {
		whiteRoots[n] = true
	}
	for _, n := range roots {
		if !whiteRoots[n] {
			h.dropRoot(n)
		}
	}
	// No PHP code ran between the scans, so reuse the proven white set.
	// Only rediscover its order after compaction, without redoing ownership
	// counting and liveness marking for the whole graph.
	stack := h.bufferedRoots()
	slices.Reverse(stack)
	white = white[:0]
	for len(stack) > 0 {
		last := len(stack) - 1
		n := stack[last]
		stack = stack[:last]
		if !whiteRoots[n] {
			continue
		}
		whiteRoots[n] = false
		white = append(white, n)
		start := len(stack)
		discoveryEdges(n, func(child gcNode) { stack = append(stack, child) })
		slices.Reverse(stack[start:])
	}
	var first error
	ranDestructor := false
	// A destructor may introduce external ownership without changing the
	// graph's counts. Defer its reachable garbage until the next tracing pass,
	// as PHP 8.5.12 does. There are at most two passes per explicit collection.
	whiteSet := make(map[gcNode]bool, len(white))
	for _, n := range white {
		whiteSet[n] = true
	}
	deferred := make(map[gcNode]bool)
	var deferTree func(gcNode)
	deferTree = func(n gcNode) {
		if deferred[n] || !whiteSet[n] {
			return
		}
		deferred[n] = true
		discoveryEdges(n, deferTree)
	}
	for _, n := range white {
		if o, ok := n.(*object); ok && !o.destructorCalled && objectHasDestructor(o) {
			deferTree(o)
		}
	}
	for _, n := range white {
		o, ok := n.(*object)
		if !ok || o.destructorCalled {
			continue
		}
		o.destructorCalled = true
		if objectHasDestructor(o) {
			ranDestructor = true
			o.refcount++
			if err := o.class.destructor(h, value{kind: valueObject, object: o}); err != nil && first == nil {
				first = err
			}
			o.refcount--
		}
	}
	if ranDestructor {
		// Destructors can create owners outside their original reachable graph.
		// Retest liveness before freeing any remaining candidate.
		current, err := garbageChecked(h.bufferedRoots(), h.runtimeCheck)
		if err != nil {
			return 0, h.mergeObjectErrors(first, err), true
		}
		clear(whiteSet)
		for _, n := range current {
			whiteSet[n] = true
		}
	}
	white = slices.DeleteFunc(white, func(n gcNode) bool {
		if ranDestructor && !whiteSet[n] {
			return true
		}
		if o, ok := n.(*object); ok && o.freed {
			return true
		}
		if deferred[n] {
			h.possibleRoot(n)
			return true
		}
		return false
	})
	dead := map[gcNode]bool{}
	for _, n := range white {
		dead[n] = true
	}
	freed := 0
	var freedHandles []uint64
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
			maps := x.weakMaps
			x.weakMaps = nil
			for _, m := range maps {
				if v, ok := m.entries[x]; ok {
					delete(m.entries, x)
					m.keys = slices.DeleteFunc(m.keys, func(k *object) bool { return k == x })
					if e := h.releaseOutside(v, dead); e != nil && err == nil {
						err = e
					}
				}
			}
			if e := h.freeWeakMap(x, dead); err == nil {
				err = e
			}
			if e := h.freeLazyStorage(x, dead); err == nil {
				err = e
			}
			if e := h.freeProperties(x, dead); e != nil && err == nil {
				err = e
			}
			if !h.noHandleReuse {
				freedHandles = append(freedHandles, x.handle)
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
	// Keep white object slots unavailable until every white node is cleaned.
	h.freeHandles = append(h.freeHandles, freedHandles...)
	return freed, first, ranDestructor
}

func (h *heap) freeGarbage(a *phpArray, dead map[gcNode]bool) error {
	if err := h.claimValues(value{kind: valueArray, array: a}); err != nil {
		return err
	}
	var first error
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		if err := h.releaseOutside(b.val, dead); err != nil && first == nil {
			first = err
		}
	}
	for it := range a.cursors {
		delete(it.arrays, a)
	}
	a.buckets, a.index, a.live, a.cursors = nil, nil, 0, nil
	return first
}

// releaseOutside drops an edge unless it points inside the garbage set, whose
// members are freed directly.
func (h *heap) releaseOutside(v value, dead map[gcNode]bool) error {
	var n gcNode
	switch v.kind {
	case valueResource:
		return h.release(v)
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
// qualify, remaining live objects get destructors in object-store-slot order,
// and then all storage is released without further destructor calls.
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
	// Objects created by these destructors get slots after the high-water mark.
	h.noHandleReuse = true
	// The object-store walk reads its growing top each step.
	for id := uint64(1); id <= h.nextHandle; id++ {
		o, ok := h.live[id]
		if !ok || o.destructorCalled {
			continue
		}
		o.destructorCalled = true
		if objectHasDestructor(o) {
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
	h.live, h.roots, h.freeHandles = nil, nil, nil
	h.rootSlots, h.freeRootSlots = nil, nil
	h.enums, h.typedSources = nil, nil
	h.classStatics = nil
	h.classes = nil
	h.gcTemporaries = nil
	h.weakMapObjects = nil
	*globals = value{}
	return first
}
