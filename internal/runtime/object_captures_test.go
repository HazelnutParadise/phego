package runtime

import (
	"fmt"
	"testing"
)

func w13CaptureNames() []string {
	return []string{"lazy-captured-cycle", "lazy-capture-release", "lazy-capture-failure"}
}

func TestW13CapturedInitializers(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13CaptureNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13CaptureCase(t, h, name))
			})
		}
	}
}
func w13CaptureCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	c := w13Build(t, classSpec{name: "W13Captured", properties: []propertySpec{{name: "self", typeName: "mixed"}}, destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }})
	real := w13Instantiate(t, h, c)
	var owner value
	must(t, h.assign(&owner, real))
	weak, err := h.weakReferenceFor(real)
	must(t, err)
	// This opaque callable owner models the Closure object's capture edges.
	// PHP callable compilation and ReflectionFunction remain W-29 integration.
	initializer, err := h.newObject(&class{name: "Closure"})
	must(t, err)
	must(t, h.setProperty(initializer, "capture", real))
	var lazy value
	if name == "lazy-captured-cycle" {
		must(t, h.writeObject(real, "self", real, nil, true))
		lazy, err = h.newLazyObjectWithInitializer(c, true, initializer, func(*heap, value) (value, error) { return real, nil })
	} else {
		cl := w13Build(t, classSpec{name: "W13Lazy", properties: []propertySpec{{name: "i", typeName: "int"}}})
		lazy, err = h.newLazyObjectWithInitializer(cl, false, initializer, func(h *heap, o value) (value, error) {
			if name == "lazy-capture-failure" {
				return value{}, &Throwable{"Exception", "retry"}
			}
			return nullValue(), h.writeObject(o, "i", w13Int(1), nil, true)
		})
	}
	must(t, err)
	var lazyOwner value
	must(t, h.assign(&lazyOwner, lazy))
	must(t, h.unset(&owner))
	var result any
	switch name {
	case "lazy-captured-cycle":
		n, err := h.collectCycles()
		must(t, err)
		target, _ := weak.get()
		first := []any{int64(n), target.kind != valueNull}
		must(t, h.unset(&lazyOwner))
		n, err = h.collectCycles()
		must(t, err)
		target, _ = weak.get()
		result = []any{first, int64(n), target.kind == valueNull, events}
	case "lazy-capture-release":
		target, _ := weak.get()
		before := target.kind != valueNull
		v, err := h.readObject(lazy, "i", nil)
		must(t, err)
		target, _ = weak.get()
		result = []any{before, w13Encode(v), target.kind == valueNull, events}
	case "lazy-capture-failure":
		e := w13ObserveValue(func() (value, error) { return h.readObject(lazy, "i", nil) })
		target, _ := weak.get()
		alive := target.kind != valueNull
		must(t, h.unset(&lazyOwner))
		target, _ = weak.get()
		result = []any{e, alive, target.kind == valueNull, events}
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
