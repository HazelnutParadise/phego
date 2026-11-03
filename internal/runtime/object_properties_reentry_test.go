package runtime

import (
	"fmt"
	"testing"
)

func w13PropertyCaseNames() []string {
	return []string{"hook-receiver-owner", "clone-method", "magic-properties", "reference-bind-reentry", "property-leading-numeric", "lazy-ghost-return", "lazy-virtual-get", "lazy-rollback-reentry", "asymmetric-reference", "asymmetric-unset", "lazy-method-no-initialize", "lazy-comparison", "hook-reference", "hook-reference-type-source", "clone-source-owner"}
}

func w13PropertyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	var observed any
	switch name {
	case "clone-source-owner":
		var owner value
		c := w13Build(t, classSpec{name: "W13CloneLifetime", properties: []propertySpec{{name: "n", typeName: "int", hasDefault: true, initial: int64(1)}}, destructor: func(h *heap, this value) error {
			v, err := h.readObject(this, "n", nil)
			if err != nil {
				return err
			}
			events = append(events, fmt.Sprintf("dtor:%d", v.integer))
			return nil
		}, methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			if err := h.unset(&owner); err != nil {
				return value{}, err
			}
			if err := h.writeObject(this, "n", w13Int(2), nil, true); err != nil {
				return value{}, err
			}
			events = append(events, "clone-body")
			return nullValue(), nil
		}}}})
		must(t, h.assign(&owner, w13Instantiate(t, h, c)))
		_, err := h.cloneObject(owner, nil)
		must(t, err)
		events = append(events, "after")
		observed = events
	case "hook-reference", "hook-reference-type-source":
		var c *class
		c = w13Build(t, classSpec{name: "W13ReferenceHook", properties: []propertySpec{{name: "storage", visibility: privateAccess, hasDefault: true, initial: int64(1)}, {name: "p", typeName: "int", virtual: true, getReference: true, get: func(h *heap, this value, _ value) (value, error) {
			var r value
			if err := h.referenceObject(&r, this, "storage", c); err != nil {
				return value{}, err
			}
			return r, h.release(r)
		}}}})
		o := w13Instantiate(t, h, c)
		var r value
		err := h.referenceObject(&r, o, "p", nil)
		if err != nil {
			observed = w13ObserveError(func() error { return err })
			break
		}
		input := w13Int(2)
		if name == "hook-reference-type-source" {
			input = str("abc")
		}
		must(t, h.assignPHP(&r, input, false))
		observed = []any{w13Encode(r.deref()), w13ObserveValue(func() (value, error) { return h.readObject(o, "p", nil) })}
	case "asymmetric-reference", "asymmetric-unset":
		c := w13Build(t, classSpec{name: "W13Asymmetric", properties: []propertySpec{{name: "x", typeName: "int", setVisibility: privateAccess, hasDefault: true, initial: int64(1)}}})
		o := w13Instantiate(t, h, c)
		observed = w13ObserveError(func() error {
			if name == "asymmetric-unset" {
				return h.unsetObject(o, "x", nil)
			}
			var alias value
			return h.referenceObject(&alias, o, "x", nil)
		})
	case "lazy-method-no-initialize":
		calls := 0
		c := w13Build(t, classSpec{name: "W13LazyMethod", properties: []propertySpec{{name: "i", typeName: "int"}}, methods: []methodSpec{{name: "ping", call: func(*heap, value, []value) (value, error) { return w13Int(7), nil }}}})
		o, err := h.newLazyObject(c, false, func(h *heap, o value) (value, error) {
			calls++
			return nullValue(), h.writeObject(o, "i", w13Int(1), nil, true)
		})
		must(t, err)
		v, err := h.callMethod(o, "ping", nil, nil, false)
		must(t, err)
		observed = []any{w13Encode(v), int64(calls), h.isLazyObject(o)}
	case "lazy-comparison":
		calls := 0
		c := w13Build(t, classSpec{name: "W13Lazy", properties: []propertySpec{{name: "i", typeName: "int"}}})
		makeLazy := func(n int64) value {
			o, err := h.newLazyObject(c, false, func(h *heap, o value) (value, error) {
				calls++
				return nullValue(), h.writeObject(o, "i", w13Int(n), nil, true)
			})
			must(t, err)
			return o
		}
		a, b := makeLazy(1), makeLazy(2)
		same, err := h.looseEqual(a, b)
		must(t, err)
		observed = []any{same, int64(calls)}
	case "lazy-ghost-return":
		c := w13Build(t, classSpec{name: "W13Lazy", properties: []propertySpec{{name: "i", typeName: "int"}}})
		o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { return w13Int(1), nil })
		must(t, err)
		e := w13ObserveValue(func() (value, error) { return h.readObject(o, "i", nil) })
		observed = []any{e, h.isLazyObject(o)}
	case "lazy-virtual-get":
		calls := 0
		c := w13Build(t, classSpec{name: "W13LazyVirtual", properties: []propertySpec{{name: "v", typeName: "int", virtual: true, get: func(*heap, value, value) (value, error) { return w13Int(42), nil }}, {name: "i", typeName: "int"}}})
		o, err := h.newLazyObject(c, false, func(h *heap, o value) (value, error) {
			calls++
			return nullValue(), h.writeObject(o, "i", w13Int(1), nil, true)
		})
		must(t, err)
		v, err := h.readObject(o, "v", nil)
		must(t, err)
		observed = []any{w13Encode(v), int64(calls), h.isLazyObject(o)}
	case "lazy-rollback-reentry":
		calls := 0
		var lazy value
		c := w13Build(t, classSpec{name: "W13LazyRollback", properties: []propertySpec{{name: "i", typeName: "int"}, {name: "child", typeName: "mixed"}}})
		child := &class{name: "W13LazyRollbackChild", destructor: func(h *heap, _ value) error {
			events = append(events, w13ObserveValue(func() (value, error) { return h.readObject(lazy, "i", nil) }))
			return nil
		}}
		var err error
		lazy, err = h.newLazyObject(c, false, func(h *heap, o value) (value, error) {
			calls++
			if calls > 3 {
				t.Fatal("recursive lazy rollback")
			}
			must(t, h.writeObject(o, "i", w13Int(1), nil, true))
			v, err := h.newObject(child)
			if err != nil {
				return value{}, err
			}
			must(t, h.writeObject(o, "child", v, nil, true))
			return value{}, &Throwable{"Exception", "retry"}
		})
		must(t, err)
		e := w13ObserveValue(func() (value, error) { return h.readObject(lazy, "i", nil) })
		observed = []any{e, int64(calls), events}
	case "hook-receiver-owner":
		var owner value
		var weak *weakReference
		c := w13Build(t, classSpec{name: "W13HookOwner", destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }, properties: []propertySpec{{name: "value", typeName: "int", virtual: true, get: func(h *heap, _ value, _ value) (value, error) {
			events = append(events, "get")
			if err := h.unset(&owner); err != nil {
				return value{}, err
			}
			v, _ := weak.get()
			if v.kind == valueNull {
				events = append(events, "dead")
			} else {
				events = append(events, "live")
			}
			return w13Int(1), nil
		}}}})
		o := w13Instantiate(t, h, c)
		must(t, h.assign(&owner, o))
		var err error
		weak, err = h.weakReferenceFor(o)
		must(t, err)
		v, err := h.readObject(o, "value", nil)
		must(t, err)
		events = append(events, "after")
		observed = []any{w13Encode(v), events}
	case "clone-method":
		var c *class
		c = w13Build(t, classSpec{name: "W13Clone", properties: []propertySpec{{name: "i", typeName: "int", readonly: true}}, methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			if err := h.writeObject(this, "i", w13Int(2), c, true); err != nil {
				return value{}, err
			}
			events = append(events, "clone")
			return nullValue(), nil
		}}}})
		a := w13Instantiate(t, h, c)
		must(t, h.writeObject(a, "i", w13Int(1), c, true))
		b, err := h.cloneObject(a, nil)
		must(t, err)
		av, err := h.readObject(a, "i", nil)
		must(t, err)
		bv, err := h.readObject(b, "i", nil)
		must(t, err)
		observed = []any{w13Encode(av), w13Encode(bv), events}
	case "magic-properties":
		storage := map[string]value{}
		methods := []methodSpec{}
		for _, op := range []string{"get", "set", "isset", "unset"} {
			params := []methodParameter{{name: "name"}}
			if op == "set" {
				params = append(params, methodParameter{name: "value"})
			}
			methods = append(methods, methodSpec{name: "__" + op, parameters: params, call: func(_ *heap, _ value, args []value) (value, error) {
				key := args[0].text
				events = append(events, op+":"+key)
				switch op {
				case "get":
					if v, ok := storage[key]; ok {
						return v, nil
					}
					return nullValue(), nil
				case "set":
					storage[key] = args[1]
				case "isset":
					_, ok := storage[key]
					return boolValue(ok), nil
				case "unset":
					delete(storage, key)
				}
				return nullValue(), nil
			}})
		}
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Magic", methods: methods}))
		must(t, h.writeObject(o, "x", w13Int(3), nil, false))
		a := w13Isset(h, o, "x", nil)
		b, err := h.readObject(o, "x", nil)
		must(t, err)
		must(t, h.unsetObject(o, "x", nil))
		c := w13Isset(h, o, "x", nil)
		observed = []any{a, w13Encode(b), c, events}
	case "reference-bind-reentry":
		c := w13Build(t, classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}})
		o := w13Instantiate(t, h, c)
		must(t, h.writeObject(o, "i", w13Int(1), nil, true))
		var alias value
		old, err := h.newObject(&class{name: "W13ReferenceReentry", destructor: func(h *heap, _ value) error {
			events = append(events, w13ObserveError(func() error { return h.assign(&alias, str("x")) }))
			return nil
		}})
		must(t, err)
		must(t, h.assign(&alias, old))
		must(t, h.referenceObject(&alias, o, "i", nil))
		v, err := h.readObject(o, "i", nil)
		must(t, err)
		observed = []any{w13Encode(v), events}
	case "property-leading-numeric":
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}}))
		observed = w13ObserveError(func() error { return h.writeObject(o, "i", str("12x"), nil, false) })
	default:
		t.Fatal(name)
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}
