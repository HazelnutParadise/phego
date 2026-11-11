package runtime

import "testing"

func w13WeakMapCaseNames() []string {
	return []string{"weak-map-destroy", "weak-map-delete", "weak-map-clone", "weak-map-value-map-cycle", "weak-map-key-map-cycle", "weak-map-live-key", "weak-map-reference-clone"}
}

func w13RunWeakMapCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name == "weak-map-reference-clone" {
		return w13WeakMapReferenceCase(t, h)
	}
	events := []any{}
	m, err := h.newWeakMapObject()
	must(t, err)
	var mapOwner, keyOwner, valueOwner value
	must(t, h.assign(&mapOwner, m))
	key, err := h.newObject(&class{name: "stdClass"})
	must(t, err)
	must(t, h.assign(&keyOwner, key))
	c := &class{name: "stdClass"}
	if name == "weak-map-destroy" || name == "weak-map-delete" || name == "weak-map-clone" {
		c = &class{name: "W13GC", destructor: func(*heap, value) error { events = append(events, "value"); return nil }}
	}
	v, err := h.newObject(c)
	must(t, err)
	must(t, h.assign(&valueOwner, v))
	must(t, h.writeWeakMap(m, key, v))
	var result any
	switch name {
	case "weak-map-destroy":
		w, err := h.weakReferenceFor(v)
		must(t, err)
		must(t, h.unset(&valueOwner))
		must(t, h.unset(&mapOwner))
		target, _ := w.get()
		result = []any{target.kind == valueNull, events}
	case "weak-map-delete":
		must(t, h.unset(&valueOwner))
		must(t, h.deleteWeakMap(m, key))
		n, err := h.countWeakMap(m)
		must(t, err)
		result = []any{int64(n), events}
	case "weak-map-clone":
		must(t, h.unset(&valueOwner))
		clone, err := h.cloneObject(m, nil)
		must(t, err)
		var cloneOwner value
		must(t, h.assign(&cloneOwner, clone))
		must(t, h.unset(&mapOwner))
		before := append([]any{}, events...)
		must(t, h.unset(&keyOwner))
		n, err := h.countWeakMap(clone)
		must(t, err)
		result = []any{before, int64(n), events}
	case "weak-map-value-map-cycle":
		must(t, h.setProperty(v, "map", m))
		w, err := h.weakReferenceFor(m)
		must(t, err)
		must(t, h.unset(&valueOwner))
		must(t, h.unset(&mapOwner))
		n, err := h.collectCycles()
		must(t, err)
		target, _ := w.get()
		result = []any{int64(n), target.kind == valueNull}
	case "weak-map-key-map-cycle":
		must(t, h.setProperty(key, "map", m))
		must(t, h.setProperty(v, "key", key))
		w, err := h.weakReferenceFor(m)
		must(t, err)
		must(t, h.unset(&keyOwner))
		must(t, h.unset(&valueOwner))
		must(t, h.unset(&mapOwner))
		n, err := h.collectCycles()
		must(t, err)
		target, _ := w.get()
		result = []any{int64(n), target.kind == valueNull}
	case "weak-map-live-key":
		must(t, h.setProperty(v, "key", key))
		w, err := h.weakReferenceFor(v)
		must(t, err)
		must(t, h.unset(&valueOwner))
		n, err := h.collectCycles()
		must(t, err)
		count, err := h.countWeakMap(m)
		must(t, err)
		target, _ := w.get()
		result = []any{int64(n), int64(count), target.kind != valueNull}
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13WeakMapReferenceCase(t *testing.T, h *heap) w13Case {
	m, err := h.newWeakMapObject()
	must(t, err)
	key, err := h.newObject(&class{name: "stdClass"})
	must(t, err)
	var mapOwner, keyOwner, r, cloneOwner value
	must(t, h.assign(&mapOwner, m))
	must(t, h.assign(&keyOwner, key))
	must(t, h.writeWeakMap(m, key, w13Int(1)))
	must(t, h.referenceWeakMap(&r, m, key))
	clone, err := h.cloneObject(m, nil)
	must(t, err)
	must(t, h.assign(&cloneOwner, clone))
	must(t, h.writeWeakMap(m, key, w13Int(2)))
	v, err := h.readWeakMap(clone, key)
	must(t, err)
	before := []any{w13Encode(r.deref()), w13Encode(v)}
	must(t, h.writeWeakMap(clone, key, w13Int(3)))
	v, err = h.readWeakMap(m, key)
	must(t, err)
	after := []any{w13Encode(r.deref()), w13Encode(v)}
	must(t, h.deleteWeakMap(m, key))
	must(t, h.assign(&r, w13Int(4)))
	v, err = h.readWeakMap(clone, key)
	must(t, err)
	count, err := h.countWeakMap(m)
	must(t, err)
	return w13Case{Result: []any{before, after, w13Encode(v), int64(count)}, Diagnostics: w13Diagnostics(h)}
}

func TestW13WeakMapReferenceClone(t *testing.T) {
	for _, width := range []int{32, 64} {
		h, err := referenceHeap(width)
		must(t, err)
		w13AssertOracleCase(t, width, "weak-map-reference-clone", w13WeakMapReferenceCase(t, h))
	}
}

// REQ-020/021; T-039-042. Entry values are released with the map, and a
// value is strongly reachable through an entry only while both owners live.
func TestW13WeakMapObjectDestroy(t *testing.T) {
	h := testHeap(t)
	calls := 0
	m, err := h.newWeakMapObject()
	must(t, err)
	key, err := h.newObject(&class{name: "stdClass"})
	must(t, err)
	v, err := h.newObject(&class{name: "Value", destructor: func(*heap, value) error { calls++; return nil }})
	must(t, err)
	var owner, keyOwner value
	must(t, h.assign(&owner, m))
	must(t, h.assign(&keyOwner, key))
	must(t, h.writeWeakMap(m, key, v))
	must(t, h.unset(&owner))
	if calls != 1 {
		t.Fatal("map did not release its value", calls)
	}
	must(t, h.unset(&keyOwner))
}
