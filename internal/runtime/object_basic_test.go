package runtime

import (
	"errors"
	"testing"
)

// REQ-018/020/021; T-035/036/039-042. Expected PHP messages come from
// tests/semantics/w13-objects/matrix.php on both pinned PHP 8.5.12 widths.
func TestW13TypedReferenceSources(t *testing.T) {
	for _, width := range []int{32, 64} {
		h, _ := referenceHeap(width)
		c, err := buildClass(classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}})
		if err != nil {
			t.Fatal(err)
		}
		o, err := h.instantiate(c)
		if err != nil {
			t.Fatal(err)
		}
		var owner, alias value
		must(t, h.assign(&owner, o))
		must(t, h.writeObject(o, "i", w12Int(1), nil, true))
		must(t, h.referenceObject(&alias, o, "i", nil))
		err = h.assign(&alias, str("x"))
		if err == nil || err.Error() != "TypeError: Cannot assign string to reference held by property W13Base::$i of type int" {
			t.Fatal(err)
		}
		if alias.deref().integer != 1 {
			t.Fatal("failed write changed reference")
		}
		must(t, h.unsetObject(o, "i", nil))
		must(t, h.assign(&alias, str("free")))
		if alias.deref().text != "free" {
			t.Fatal("source was not detached")
		}
		must(t, h.unset(&owner))
		must(t, h.unset(&alias))
	}
}

func TestW13DefaultsAreRequestLocal(t *testing.T) {
	c, err := buildClass(classSpec{name: "Defaults", properties: []propertySpec{{name: "a", typeName: "array", hasDefault: true, initial: []SnapshotEntry{{Key: int64(0), Value: int64(1)}}}}})
	if err != nil {
		t.Fatal(err)
	}
	h, _ := newHeap(64)
	other, _ := newHeap(64)
	a, err := h.instantiate(c)
	must(t, err)
	b, err := other.instantiate(c)
	must(t, err)
	av, err := h.readObject(a, "a", nil)
	must(t, err)
	bv, err := other.readObject(b, "a", nil)
	must(t, err)
	if av.array == bv.array {
		t.Fatal("shared mutable class default")
	}
	if _, err := other.readObject(a, "a", nil); err == nil {
		t.Fatal("accepted foreign object")
	}
}

func TestW13HookAndLazyRetry(t *testing.T) {
	h, _ := newHeap(64)
	c, err := buildClass(classSpec{name: "W13Lazy", properties: []propertySpec{{name: "i", typeName: "int"}, {name: "s", typeName: "string", hasDefault: true, initial: []byte("default")}}})
	must(t, err)
	calls := 0
	o, err := h.newLazyObject(c, false, func(h *heap, o value) (value, error) {
		calls++
		if err := h.writeObject(o, "i", w12Int(int64(calls)), c, true); err != nil {
			return value{}, err
		}
		if calls == 1 {
			return value{}, &Throwable{"Exception", "retry"}
		}
		return nullValue(), nil
	})
	must(t, err)
	if _, err = h.readObject(o, "i", nil); err == nil || err.Error() != "Exception: retry" {
		t.Fatal(err)
	}
	if !h.isLazyObject(o) {
		t.Fatal("failed initializer committed")
	}
	i, err := h.readObject(o, "i", nil)
	must(t, err)
	if i.integer != 2 || calls != 2 || h.isLazyObject(o) {
		t.Fatal(i, calls)
	}
}

func TestW13ReadonlyAndPrivate(t *testing.T) {
	h, _ := newHeap(64)
	c, err := buildClass(classSpec{name: "W13Base", properties: []propertySpec{{name: "ro", typeName: "int", readonly: true}, {name: "secret", typeName: "string", visibility: privateAccess, hasDefault: true, initial: []byte("base")}}})
	must(t, err)
	o, err := h.instantiate(c)
	must(t, err)
	err = h.writeObject(o, "ro", w12Int(1), nil, true)
	if err == nil || err.Error() != "Error: Cannot modify protected(set) readonly property W13Base::$ro from global scope" {
		t.Fatal(err)
	}
	must(t, h.writeObject(o, "ro", w12Int(1), c, true))
	err = h.writeObject(o, "ro", w12Int(2), c, true)
	if err == nil || err.Error() != "Error: Cannot modify readonly property W13Base::$ro" {
		t.Fatal(err)
	}
	_, err = h.readObject(o, "secret", nil)
	var php *Throwable
	if !errors.As(err, &php) || php.Message != "Cannot access private property W13Base::$secret" {
		t.Fatal(err)
	}
}

func TestW13ObjectComparison(t *testing.T) {
	h := testHeap(t)
	c, err := buildClass(classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}})
	must(t, err)
	a, err := h.instantiate(c)
	must(t, err)
	must(t, h.writeObject(a, "i", w12Int(1), nil, true))
	b, err := h.cloneObject(a, nil)
	must(t, err)
	if equal, err := h.looseEqual(a, b); err != nil || !equal {
		t.Fatal(equal, err)
	}
	must(t, h.writeObject(b, "i", w12Int(2), nil, true))
	if order, err := h.compare(a, b); err != nil || order != -1 {
		t.Fatal(order, err)
	}
	e, err := buildClass(classSpec{name: "W13Enum", enumCases: []string{"A", "B"}, enumValues: []any{[]byte("a"), []byte("b")}})
	must(t, err)
	ea, err := h.enumCase(e, "A")
	must(t, err)
	eb, err := h.enumCase(e, "B")
	must(t, err)
	if order, err := h.compare(ea, eb); err != nil || order != 1 {
		t.Fatal(order, err)
	}
}

func TestW13UninitializedLazyDoesNotDestruct(t *testing.T) {
	h := testHeap(t)
	calls := 0
	// Pinned PHP immediately makes zero-property classes ordinary objects.
	// Keep this fixture genuinely lazy; zero-property behavior has its own oracle.
	c, err := buildClass(classSpec{name: "LazyDtor", properties: []propertySpec{{name: "p", typeName: "int"}}, destructor: func(*heap, value) error { calls++; return nil }})
	must(t, err)
	o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { t.Fatal("unexpected initialization"); return value{}, nil })
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	must(t, h.unset(&owner))
	if calls != 0 {
		t.Fatal("uninitialized lazy destructor ran")
	}
}
