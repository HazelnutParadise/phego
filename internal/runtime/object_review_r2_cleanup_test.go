package runtime

import (
	"errors"
	"strings"
	"testing"
)

func w13R2CleanupNames() []string {
	return []string{"r2-cleanup-isset", "r2-cleanup-isset-throw", "r2-cleanup-clone", "r2-cleanup-clone-throw", "r2-cleanup-clone-self", "r2-cleanup-clone-array", "r2-cleanup-clone-double", "r2-cleanup-dynamic", "r2-cleanup-handler", "r2-cleanup-reentry"}
}

func w13R2CleanupCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	mode := strings.TrimPrefix(name, "r2-cleanup-")
	var observed any
	child := w13Build(t, classSpec{name: "W13R2CleanupChild", destructor: func(*heap, value) error {
		events = append(events, "child")
		if strings.HasSuffix(mode, "throw") {
			return &Throwable{"Exception", "child"}
		}
		return nil
	}})
	var o value
	var err error
	switch {
	case mode == "reentry":
		var c *class
		rc := w13Build(t, classSpec{name: "W13R2CleanupReentrantChild", properties: []propertySpec{{name: "owner", typeName: "W13R2CleanupReentrant"}}, destructor: func(h *heap, this value) error {
			events = append(events, "child")
			owner, e := h.readObject(this, "owner", nil)
			if e != nil {
				return e
			}
			_, e = h.callMethod(owner, "initialize", nil, nil, false)
			return e
		}})
		c = w13Build(t, classSpec{name: "W13R2CleanupReentrant", properties: []propertySpec{{name: "text", typeName: "string", readonly: true, setVisibility: protectedAccess}}, methods: []methodSpec{
			{name: "initialize", call: func(h *heap, this value, _ []value) (value, error) {
				return nullValue(), h.writeObject(this, "text", w13Str("after"), c, true)
			}},
			{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
				v, e := h.instantiate(rc)
				if e != nil {
					return value{}, e
				}
				return v, h.writeObject(v, "owner", this, nil, true)
			}},
		}})
		o = w13Instantiate(t, h, c)
		must(t, h.writeObject(o, "text", w13Str("before"), c, true))
		copy, e := h.cloneObject(o, nil)
		err = e
		if err == nil {
			observed = w13Encode(w13Read(t, h, copy, "text", nil))
			must(t, h.discardMethodReturn(copy))
		}
	case strings.HasPrefix(mode, "isset"):
		c := w13Build(t, classSpec{name: "W13R2CleanupHook", properties: []propertySpec{{name: "p", typeName: "object", virtual: true, get: func(h *heap, _, _ value) (value, error) { return h.instantiate(child) }}}})
		o = w13Instantiate(t, h, c)
		var set bool
		set, err = h.issetObject(o, "p", nil)
		if err == nil {
			observed = set
		}
	case mode == "dynamic":
		c := w13Build(t, classSpec{name: "W13R2CleanupDynamic", allowDynamic: true, properties: []propertySpec{{name: "storage", typeName: "int", hasDefault: true, initial: int64(7)}}, methods: []methodSpec{{name: "__get", returnsReference: true, parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, "get")
			var alias value
			e := h.referenceObject(&alias, this, "storage", nil)
			if e != nil {
				return value{}, e
			}
			return alias, h.release(alias)
		}}}})
		o = w13Instantiate(t, h, c)
		must(t, h.writeObject(o, "x", w13Int(1), nil, true))
		var alias value
		must(t, h.referenceObject(&alias, o, "x", nil))
		must(t, h.assign(&alias, w13Int(2)))
		observed = []any{w13Encode(w13Read(t, h, o, "x", nil)), w13Encode(w13Read(t, h, o, "storage", nil))}
		must(t, h.unset(&alias))
	case mode == "handler":
		var parent *class
		parent = w13Build(t, classSpec{name: "W13R2CleanupParent", methods: []methodSpec{{name: "__clone", visibility: privateAccess, call: func(*heap, value, []value) (value, error) { events = append(events, "parent"); return nullValue(), nil }}, {name: "duplicate", call: func(h *heap, this value, _ []value) (value, error) { return h.cloneObject(this, parent) }}}})
		sub := w13Build(t, classSpec{name: "W13R2CleanupSubclass", parent: parent, methods: []methodSpec{{name: "__clone", call: func(*heap, value, []value) (value, error) {
			events = append(events, "subclass")
			return nullValue(), nil
		}}}})
		o = w13Instantiate(t, h, sub)
		copy, e := h.callMethod(o, "duplicate", nil, nil, false)
		must(t, e)
		must(t, h.discardMethodReturn(copy))
	default:
		c := w13Build(t, classSpec{name: "W13R2CleanupClone", methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			switch mode {
			case "clone-self":
				return this, nil
			case "clone-array":
				a := h.newArrayValue()
				e := h.appendValue(&a, this)
				// appendValue owns its writable local; transfer it to a temporary.
				a.array.refcount--
				return a, e
			case "clone-double":
				return value{}, &Throwable{"Exception", "clone"}
			default:
				return h.instantiate(child)
			}
		}}}, destructor: func(*heap, value) error {
			events = append(events, "clone")
			if mode == "clone-double" {
				return &Throwable{"Exception", "destructor"}
			}
			return nil
		}})
		o = w13Instantiate(t, h, c)
		var owner value
		must(t, h.assign(&owner, o))
		copy, e := h.cloneObject(o, nil)
		err = e
		if err == nil {
			var result value
			must(t, h.assign(&result, copy))
			events = append(events, "after-clone")
			err = h.unset(&result)
		}
	}
	if err != nil {
		var php *Throwable
		if !errors.As(err, &php) {
			t.Fatal(err)
		}
		var prior any
		var c completion
		if errors.As(err, &c) && c.previous != nil {
			var p *Throwable
			if errors.As(c.previous, &p) {
				prior = p.Message
			}
		}
		observed = []any{php.Class, php.Message, prior}
	}
	events = append(events, "after")
	return w13Case{Result: []any{observed, events}, Diagnostics: w13Diagnostics(h)}
}
