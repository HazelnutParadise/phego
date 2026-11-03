package runtime

import (
	"fmt"
	"slices"
)

// This shared descriptor contains no request-owned PHP values.
var weakMapBuiltin = &class{name: "WeakMap", schema: &classSchema{classSpec: classSpec{name: "WeakMap", final: true, noDynamic: true}}}

func (h *heap) newWeakMapObject() (value, error) {
	v, err := h.newObject(weakMapBuiltin)
	if err != nil {
		return value{}, err
	}
	m := newWeakMap()
	m.owner = h
	m.identity = v.object
	if h.weakMapObjects == nil {
		h.weakMapObjects = make(map[*object]*weakMap)
	}
	h.weakMapObjects[v.object] = m
	return v, nil
}
func (h *heap) checkedWeakMap(v value) (*weakMap, error) {
	o, err := h.checkedObject(v)
	if err != nil {
		return nil, err
	}
	m := h.weakMapObjects[o]
	if m == nil || m.closed || m.owner != h {
		return nil, fmt.Errorf("operation requires a live WeakMap owned by this request")
	}
	return m, nil
}
func (h *heap) checkedWeakKey(key value) (*object, error) {
	if err := h.claimValues(key); err != nil {
		return nil, err
	}
	key = key.deref()
	if key.kind != valueObject {
		return nil, &Throwable{"TypeError", "WeakMap key must be an object"}
	}
	return h.checkedObject(key)
}
func (h *heap) writeWeakMap(mapValue, key, v value) error {
	if err := h.claimValues(mapValue, key, v); err != nil {
		return err
	}
	m, err := h.checkedWeakMap(mapValue)
	if err != nil {
		return err
	}
	if _, err := h.checkedWeakKey(key); err != nil {
		return err
	}
	return h.weakMapSet(m, key, v)
}
func (h *heap) readWeakMap(mapValue, key value) (value, error) {
	m, err := h.checkedWeakMap(mapValue)
	if err != nil {
		return value{}, err
	}
	k, err := h.checkedWeakKey(key)
	if err != nil {
		return value{}, err
	}
	if v, ok := m.entries[k]; ok {
		return v.deref(), nil
	}
	return value{}, &Throwable{"Error", "Object not contained in WeakMap"}
}
func takeWeakEntry(m *weakMap, key *object) (value, bool) {
	v, ok := m.entries[key]
	if !ok {
		return value{}, false
	}
	delete(m.entries, key)
	m.keys = slices.DeleteFunc(m.keys, func(o *object) bool { return o == key })
	key.weakMaps = slices.DeleteFunc(key.weakMaps, func(other *weakMap) bool { return other == m })
	return v, true
}
func (h *heap) deleteWeakMap(mapValue, key value) error {
	m, err := h.checkedWeakMap(mapValue)
	if err != nil {
		return err
	}
	k, err := h.checkedWeakKey(key)
	if err != nil {
		return err
	}
	if v, ok := takeWeakEntry(m, k); ok {
		return h.release(v)
	}
	return nil
}
func (h *heap) countWeakMap(mapValue value) (int, error) {
	m, err := h.checkedWeakMap(mapValue)
	if err != nil {
		return 0, err
	}
	return m.count(), nil
}
func (h *heap) referenceWeakMap(dst *value, mapValue, key value) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	pending, err := h.pendingValues(*dst, mapValue, key)
	if err != nil {
		return err
	}
	mapObject := mapValue.deref()
	if mapObject.kind != valueObject || mapObject.object == nil {
		return fmt.Errorf("property access requires a live object owned by this request")
	}
	if err := h.validateObject(mapObject.object); err != nil {
		return err
	}
	m := h.weakMapObjects[mapObject.object]
	if m == nil || m.closed || m.owner != h {
		return fmt.Errorf("operation requires a live WeakMap owned by this request")
	}
	keyObject := key.deref()
	if keyObject.kind != valueObject {
		return &Throwable{"TypeError", "WeakMap key must be an object"}
	}
	if err := h.validateObject(keyObject.object); err != nil {
		return err
	}
	k := keyObject.object
	v, ok := m.entries[k]
	if !ok {
		return &Throwable{"Error", "Object not contained in WeakMap"}
	}
	next, err := h.pendingValues(v)
	if err != nil {
		return err
	}
	h.commitValueOwnership(append(pending, next...))
	newReference := v.kind != valueReference
	if newReference {
		v = value{kind: valueReference, ref: &reference{owner: h, refcount: 1, target: v}}
	}
	retain(v)
	if newReference {
		m.entries[k] = v
	}
	old := *dst
	*dst = v
	return h.release(old)
}

func (h *heap) freeWeakMap(o *object, dead map[gcNode]bool) error {
	m := h.weakMapObjects[o]
	if m == nil || m.closed {
		return nil
	}
	m.closed = true
	delete(h.weakMapObjects, o)
	keys, entries := m.keys, m.entries
	m.keys = nil
	m.entries = nil
	// Unregister every backlink before any value destructor can run.
	for _, key := range keys {
		key.weakMaps = slices.DeleteFunc(key.weakMaps, func(other *weakMap) bool { return other == m })
	}
	var first error
	for _, key := range keys {
		v := entries[key]
		delete(entries, key)
		first = h.mergeObjectErrors(first, h.releaseOutside(v, dead))
	}
	return first
}

func (h *heap) cloneWeakMap(src *weakMap) (value, error) {
	v, err := h.newWeakMapObject()
	if err != nil {
		return value{}, err
	}
	dst := h.weakMapObjects[v.object]
	for _, key := range src.keys {
		raw, ok := src.entries[key]
		if !ok {
			continue
		}
		if raw.kind == valueReference && raw.ref.refcount == 1 {
			raw = raw.deref()
		}
		retain(raw)
		dst.keys = append(dst.keys, key)
		dst.entries[key] = raw
		key.weakMaps = append(key.weakMaps, dst)
	}
	return v, nil
}
