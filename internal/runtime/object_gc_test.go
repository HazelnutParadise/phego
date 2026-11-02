package runtime

import (
	"reflect"
	"testing"
)

// REQ-020/021; T-039-042. PHP oracle in tests/semantics/w13-objects/gc.php.
func w13RunGCCase(t *testing.T, h *heap, name string) w13Case {
	// matrix.php collects inside its live outer foreach before calling a case.
	// That loop table is consequently buffered before this case creates roots.
	var cases, item value
	for _, caseName := range w13CaseNames() {
		must(t, h.appendValue(&cases, str(caseName)))
	}
	var result w13Case
	must(t, h.foreachValue(cases, &item, func(arrayKey) error {
		if item.text == name {
			_, err := h.collectCycles()
			must(t, err)
			result = w13GCBody(t, h, name)
		}
		return nil
	}))
	must(t, h.unset(&item))
	must(t, h.unset(&cases))
	return result
}

func w13GCBody(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	var c *class
	c = &class{name: "W13GC", destructor: func(h *heap, this value) error {
		label, _ := this.object.props.lookup("name")
		events = append(events, label.text)
		if label.text == "nested" {
			n, err := h.collectCycles()
			if err != nil {
				return err
			}
			events = append(events, []any{"nested", int64(n)})
		}
		if label.text == "create" {
			b, err := h.newObject(c)
			if err != nil {
				return err
			}
			if err = h.setProperty(b, "name", str("created")); err != nil {
				return err
			}
			if err = h.setProperty(b, "self", b); err != nil {
				return err
			}
			h.possibleRoot(b.object)
		}
		return nil
	}}
	makeCycle := func(label string) value {
		o, err := h.newObject(c)
		must(t, err)
		must(t, h.setProperty(o, "name", str(label)))
		var v value
		must(t, h.assign(&v, o))
		must(t, h.setProperty(o, "self", o))
		return v
	}
	var result any
	switch name {
	case "lazy-uninitialized-destructor":
		o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { t.Fatal("unexpected initializer"); return nullValue(), nil })
		must(t, err)
		var owner value
		must(t, h.assign(&owner, o))
		must(t, h.unset(&owner))
		result = events
	case "gc-hole-order":
		a, b, c := makeCycle("a"), makeCycle("b"), makeCycle("c")
		wa, err := h.weakReferenceFor(a)
		must(t, err)
		must(t, h.unset(&a))
		must(t, h.unset(&b))
		must(t, h.unset(&c))
		target, ok := wa.get()
		if !ok {
			t.Fatal("early free")
		}
		retain(target)
		must(t, h.setProperty(target, "self", nullValue()))
		must(t, h.release(target))
		n, err := h.collectCycles()
		must(t, err)
		result = []any{int64(n), events}
	case "gc-nested":
		o := makeCycle("nested")
		must(t, h.unset(&o))
		n, err := h.collectCycles()
		must(t, err)
		result = []any{int64(n), events}
	case "gc-created":
		o := makeCycle("create")
		must(t, h.unset(&o))
		n, err := h.collectCycles()
		must(t, err)
		first := append([]any{}, events...)
		second, err := h.collectCycles()
		must(t, err)
		result = []any{int64(n), first, int64(second), events}
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13GCCreatedSecondPass(t *testing.T) {
	got := w13RunGCCase(t, testHeap(t), "gc-created").Result
	want := []any{int64(1), []any{"create", "created"}, int64(1), []any{"create", "created"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Go: %#v PHP: %#v", got, want)
	}
}

// Host callbacks are trusted, but legal heap stores must never create an
// owner that points to storage the collector subsequently marks freed.
func TestW13GCExternalRootDuringDestructor(t *testing.T) {
	h := testHeap(t)
	var saved, b value
	a, err := h.newObject(&class{name: "Rescue", destructor: func(h *heap, _ value) error { return h.assign(&saved, b) }})
	must(t, err)
	b, err = h.newObject(&class{name: "Target"})
	must(t, err)
	for _, o := range []value{a, b} {
		retain(o)
		must(t, h.setProperty(o, "self", o))
		must(t, h.release(o))
	}
	_, err = h.collectCycles()
	must(t, err)
	if _, err = h.checkedObject(saved); err != nil {
		t.Fatal("collector freed a newly owned object", err)
	}
	must(t, h.setProperty(saved, "self", nullValue()))
	must(t, h.unset(&saved))
}
