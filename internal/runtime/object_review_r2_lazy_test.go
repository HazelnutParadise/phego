package runtime

import (
	"strings"
	"testing"
)

func w13R2LazyNames() []string {
	return []string{"r2-lazy-zero-proxy", "r2-lazy-zero-ghost", "r2-lazy-capture-proxy", "r2-lazy-capture-ghost", "r2-lazy-capture-throw-proxy", "r2-lazy-capture-throw-ghost", "r2-lazy-comparison-state"}
}

func w13R2LazyCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	if name == "r2-lazy-comparison-state" {
		c := w13Build(t, classSpec{name: "W13R2CompareLazy", properties: []propertySpec{{name: "a", typeName: "int"}, {name: "b", typeName: "int"}}})
		left, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
			return nullValue(), h.writeObject(this, "a", w13Int(3), nil, true)
		})
		must(t, err)
		right := w13Instantiate(t, h, c)
		must(t, h.writeObject(right, "a", w13Int(3), nil, true))
		must(t, h.writeObject(right, "b", w13Int(2), nil, true))
		first, err := h.compare(left, right)
		must(t, err)
		second, err := h.compare(left, right)
		must(t, err)
		return w13Case{Result: []any{first, second}, Diagnostics: []any{}}
	}
	c := w13Build(t, classSpec{name: "W13R2ZeroProperties", destructor: func(*heap, value) error { events = append(events, "destructor"); return nil }})
	proxy := strings.HasSuffix(name, "proxy")
	initialize := func(h *heap, _ value) (value, error) {
		events = append(events, "initializer")
		if proxy {
			return h.instantiate(c)
		}
		return nullValue(), nil
	}
	if strings.Contains(name, "capture") {
		child := w13Build(t, classSpec{name: "W13R2Captured", destructor: func(*heap, value) error {
			events = append(events, "capture")
			if strings.Contains(name, "throw") {
				return &Throwable{"Exception", "capture"}
			}
			return nil
		}})
		closure, err := h.newObject(&class{name: "Closure"})
		must(t, err)
		must(t, h.setProperty(closure, "captured", w13Instantiate(t, h, child)))
		var initializer value
		must(t, h.assign(&initializer, closure))
		o, err := h.newLazyObjectWithInitializer(c, proxy, initializer, initialize)
		var owner value
		if err == nil {
			must(t, h.assign(&owner, o))
		}
		err = h.mergeObjectErrors(err, h.unset(&initializer))
		var flags any
		if err != nil {
			err = h.mergeObjectErrors(err, h.unset(&owner))
			flags = w13Error(err)
		} else {
			pending := o.object.lazy != nil && o.object.lazy.initialize != nil
			flags = []any{pending, !pending}
			events = append(events, "after-constructor")
			must(t, h.unset(&owner))
		}
		return w13Case{Result: []any{flags, events}, Diagnostics: []any{}}
	}
	o, err := h.newLazyObject(c, proxy, initialize)
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	pending := o.object.lazy != nil && o.object.lazy.initialize != nil
	before := []any{pending, !pending}
	must(t, h.unset(&owner))
	return w13Case{Result: []any{before, events}, Diagnostics: []any{}}
}

func TestW13ZeroLazyInitializerOwnership(t *testing.T) {
	local, foreign := testHeap(t), testHeap(t)
	initializer, err := foreign.newObject(&class{name: "Closure"})
	must(t, err)
	c := w13Build(t, classSpec{name: "EmptyLazy"})
	_, err = local.newLazyObjectWithInitializer(c, false, initializer, func(*heap, value) (value, error) { return nullValue(), nil })
	if err == nil {
		t.Fatal("zero-property optimization accepted a foreign initializer")
	}
	if len(local.live) != 0 || initializer.object.owner != foreign || initializer.object.freed {
		t.Fatal("rejected initializer changed request state")
	}
}
