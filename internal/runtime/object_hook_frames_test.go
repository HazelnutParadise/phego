package runtime

import (
	"fmt"
	"testing"
)

func w13HookFrameNames() []string {
	return []string{"hook-lazy-backed-frame", "hook-helper-frame", "hook-other-receiver", "hook-parent-prototype", "hook-proxy-backing-receiver"}
}
func w13HookFrameCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var observed any
	switch name {
	case "hook-lazy-backed-frame":
		var c *class
		c = w13Build(t, classSpec{name: "W13LazyBackedHook", properties: []propertySpec{{name: "p", typeName: "int", hasDefault: true, initial: int64(1), get: func(h *heap, this value, _ value) (value, error) {
			events = append(events, "get")
			return h.readObject(this, "p", c)
		}}}})
		o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
			events = append(events, "init")
			return nullValue(), h.writeObject(this, "p", w13Int(9), c, true)
		})
		must(t, err)
		v, err := h.readObject(o, "p", nil)
		must(t, err)
		observed = []any{w13Encode(v), events}
	case "hook-helper-frame":
		var c *class
		c = w13Build(t, classSpec{name: "W13HelperHook", properties: []propertySpec{{name: "count", typeName: "int", hasDefault: true, initial: int64(0)}, {name: "p", typeName: "int", hasDefault: true, initial: int64(7), get: func(h *heap, this value, _ value) (value, error) {
			n, err := h.readObject(this, "count", c)
			if err != nil {
				return value{}, err
			}
			n.integer++
			if err = h.writeObject(this, "count", n, c, true); err != nil {
				return value{}, err
			}
			events = append(events, fmt.Sprintf("get:%d", n.integer))
			if n.integer == 1 {
				return h.callMethod(this, "helper", nil, c, false)
			}
			return h.readObject(this, "p", c)
		}}}, methods: []methodSpec{{name: "helper", call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, "helper")
			return h.readObject(this, "p", c)
		}}}})
		o := w13Instantiate(t, h, c)
		v, err := h.readObject(o, "p", nil)
		must(t, err)
		count, err := h.readObject(o, "count", nil)
		must(t, err)
		observed = []any{w13Encode(v), w13Encode(count), events}
	case "hook-other-receiver":
		var c *class
		c = w13Build(t, classSpec{name: "W13OtherHook", properties: []propertySpec{{name: "other", typeName: "?W13OtherHook", hasDefault: true, initial: nil}, {name: "p", typeName: "int", hasDefault: true, initial: int64(7), get: func(h *heap, this value, _ value) (value, error) {
			events = append(events, "get")
			other, err := h.readObject(this, "other", c)
			if err != nil {
				return value{}, err
			}
			if other.kind == valueObject {
				return h.readObject(other, "p", c)
			}
			return h.readObject(this, "p", c)
		}}}})
		a, b := w13Instantiate(t, h, c), w13Instantiate(t, h, c)
		must(t, h.writeObject(a, "other", b, nil, true))
		v, err := h.readObject(a, "p", nil)
		must(t, err)
		observed = []any{w13Encode(v), events}
	case "hook-parent-prototype":
		var parent *class
		parent = w13Build(t, classSpec{name: "W13HookParent", properties: []propertySpec{{name: "p", typeName: "int", visibility: protectedAccess, hasDefault: true, initial: int64(7), get: func(h *heap, this value, _ value) (value, error) {
			events = append(events, "parent")
			return h.readObject(this, "p", parent)
		}}}})
		child := w13Build(t, classSpec{name: "W13HookChild", parent: parent, properties: []propertySpec{{name: "p", typeName: "int", hasDefault: true, initial: int64(7), get: func(h *heap, this value, _ value) (value, error) {
			events = append(events, "child")
			p, err := resolveProperty(parent, "p", parent)
			if err != nil {
				return value{}, err
			}
			return h.readPropertyHook(this.object, p)
		}}}, methods: []methodSpec{{name: "readParent", call: func(h *heap, this value, _ []value) (value, error) {
			return h.readObject(this, "p", nil)
		}}}})
		o := w13Instantiate(t, h, child)
		v, err := h.callMethod(o, "readParent", nil, nil, false)
		must(t, err)
		observed = []any{w13Encode(v), events}
	case "hook-proxy-backing-receiver":
		base := w13Build(t, classSpec{name: "W13ProxyHookBase", properties: []propertySpec{{name: "q", typeName: "int", hasDefault: true, initial: int64(1)}, {name: "p", typeName: "int", virtual: true, get: func(*heap, value, value) (value, error) { return w13Int(7), nil }}}})
		child := w13Build(t, classSpec{name: "W13ProxyHookChild", parent: base, properties: []propertySpec{{name: "p", typeName: "int", virtual: true, get: func(h *heap, this value, _ value) (value, error) {
			target, err := h.initializeLazy(this.object)
			if err != nil {
				return value{}, err
			}
			return h.readObject(value{kind: valueObject, object: target}, "p", nil)
		}}}})
		real := w13Instantiate(t, h, base)
		var owner value
		must(t, h.assign(&owner, real))
		o, err := h.newLazyObject(child, true, func(*heap, value) (value, error) { return real, nil })
		must(t, err)
		if _, err = h.readObject(o, "q", nil); err != nil {
			observed = w13ObserveError(func() error { return err })
			break
		}
		observed = w13ObserveValue(func() (value, error) { return h.readObject(o, "p", nil) })
	default:
		t.Fatal(name)
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}

func TestW13HookFrames(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13HookFrameNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13HookFrameCase(t, h, name))
			})
		}
	}
}
