package runtime

import (
	"testing"
)

func w13R2PropertyNames() []string {
	return []string{"r2-property-unset-magic", "r2-property-stringable-receiver-release", "r2-property-readonly-unset", "r2-property-magic-result-destruction", "r2-property-readonly-coercion-reentry"}
}

func w13R2PropertyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	var result any
	switch name {
	case "r2-property-readonly-coercion-reentry":
		c := w13Build(t, classSpec{name: "W13R2PReadonlyCoercion", properties: []propertySpec{{name: "text", typeName: "string", readonly: true, setVisibility: protectedAccess}}})
		o := w13Instantiate(t, h, c)
		sourceClass := w13Build(t, classSpec{name: "W13R2PReadonlyString", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
			if err := h.writeObject(o, "text", w13Str("inner"), c, false); err != nil {
				return value{}, err
			}
			return w13Str("outer"), nil
		}}}})
		failure := w13ObserveError(func() error { return h.writeObject(o, "text", w13Instantiate(t, h, sourceClass), c, false) })
		v, err := h.readObject(o, "text", nil)
		must(t, err)
		result = []any{failure, w13Encode(v)}
	case "r2-property-unset-magic":
		var c *class
		c = w13Build(t, classSpec{name: "W13R2PUnset", properties: []propertySpec{{name: "p", typeName: "int", hasDefault: true, initial: int64(1)}, {name: "u", hasDefault: true, initial: []byte("initial")}, {name: "q", typeName: "int"}}, methods: []methodSpec{
			{name: "__get", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "get"); return w13Int(2), nil }},
			{name: "__set", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(h *heap, this value, args []value) (value, error) {
				events = append(events, "set")
				return nullValue(), nil
			}},
			{name: "__isset", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "isset"); return w13Int(1), nil }},
			{name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "unset"); return nullValue(), nil }},
			{name: "__clone", call: func(*heap, value, []value) (value, error) { events = append(events, "clone"); return nullValue(), nil }},
		}})
		o := w13Instantiate(t, h, c)
		must(t, h.unsetObject(o, "p", nil))
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "p", nil) })
		write := w13ObserveError(func() error { return h.writeObject(o, "p", w13Int(3), nil, false) })
		isset, err := h.issetObject(o, "p", nil)
		must(t, err)
		missing := w13ObserveValue(func() (value, error) { return h.readObject(o, "q", nil) })
		must(t, h.unsetObject(o, "p", nil))
		unsetAgain := append([]any{}, events...)
		must(t, h.unsetObject(o, "u", nil))
		untypedRead := w13ObserveValue(func() (value, error) { return h.readObject(o, "u", nil) })
		clone, err := h.cloneObject(o, nil)
		must(t, err)
		cloneRead := w13ObserveValue(func() (value, error) { return h.readObject(clone, "u", nil) })
		result = []any{read, write, isset, missing, unsetAgain, untypedRead, cloneRead, events}
	case "r2-property-stringable-receiver-release":
		var owner, source value
		c := w13Build(t, classSpec{name: "W13R2PReceiver", properties: []propertySpec{{name: "text", typeName: "string"}}, methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, "string")
			if err := h.unset(&owner); err != nil {
				return value{}, err
			}
			if err := h.unset(&source); err != nil {
				return value{}, err
			}
			return w13Str("x"), nil
		}}}, destructor: func(*heap, value) error { events = append(events, "destruct"); return nil }})
		must(t, h.assign(&owner, w13Instantiate(t, h, c)))
		must(t, h.assign(&source, w13Instantiate(t, h, c)))
		must(t, h.writeObject(owner, "text", w13Str(""), nil, false))
		must(t, h.writeObject(source, "text", w13Str("source"), nil, false))
		result = w13ObserveError(func() error { return h.writeObject(owner, "text", source, nil, false) })
		result = []any{result, events}
	case "r2-property-readonly-unset":
		var base, child *class
		base = w13Build(t, classSpec{name: "W13R2PReadonlyBase", properties: []propertySpec{{name: "p", typeName: "int", readonly: true, setVisibility: protectedAccess}}, methods: []methodSpec{{name: "clear", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.unsetObject(this, "p", base)
		}}, {name: "init", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.writeObject(this, "p", w13Int(7), base, false)
		}}}})
		child = w13Build(t, classSpec{name: "W13R2PReadonlyChild", parent: base, methods: []methodSpec{{name: "clearChild", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.unsetObject(this, "p", child)
		}}}})
		globalObject, declaringObject, childObject := w13Instantiate(t, h, child), w13Instantiate(t, h, child), w13Instantiate(t, h, child)
		global := w13ObserveError(func() error { return h.unsetObject(globalObject, "p", nil) })
		inDeclaring := w13ObserveValue(func() (value, error) { return h.callMethod(declaringObject, "clear", nil, nil, false) })
		inChild := w13ObserveValue(func() (value, error) { return h.callMethod(childObject, "clearChild", nil, nil, false) })
		init := w13ObserveValue(func() (value, error) { return h.callMethod(declaringObject, "init", nil, nil, false) })
		initializedUnset := w13ObserveError(func() error { return h.unsetObject(declaringObject, "p", base) })
		result = []any{global, inDeclaring, inChild, init, initializedUnset}
	case "r2-property-magic-result-destruction":
		var box *class
		var reentered value
		inDestructor := false
		resultClass := w13Build(t, classSpec{name: "W13R2PResult", destructor: func(h *heap, _ value) error {
			events = append(events, "result-dtor")
			if !inDestructor && reentered.kind != valueUndefined {
				inDestructor = true
				if err := h.unsetObject(reentered, "x", nil); err != nil {
					return err
				}
			}
			return &Throwable{"Exception", "dtor"}
		}})
		plainResultClass := w13Build(t, classSpec{name: "W13R2PPlainResult", destructor: func(*heap, value) error { events = append(events, "plain-dtor"); return nil }})
		newResult := func(h *heap, plain bool) (value, error) {
			if plain {
				return h.instantiate(plainResultClass)
			}
			return h.instantiate(resultClass)
		}
		box = w13Build(t, classSpec{name: "W13R2PMagic", methods: []methodSpec{
			{name: "__set", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(h *heap, _ value, _ []value) (value, error) {
				events = append(events, "set")
				return newResult(h, false)
			}},
			{name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, _ value, _ []value) (value, error) {
				events = append(events, "unset")
				return newResult(h, false)
			}},
			{name: "__isset", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, _ value, args []value) (value, error) {
				events = append(events, "isset")
				return newResult(h, len(args) > 0 && args[0].deref().text == "truthy")
			}},
		}})
		o := w13Instantiate(t, h, box)
		reentered = o
		a := w13ObserveError(func() error { return h.writeObject(o, "x", w13Int(1), nil, false) })
		b := w13ObserveError(func() error { return h.unsetObject(o, "x", nil) })
		c := w13ObserveError(func() error { _, err := h.issetObject(o, "x", nil); return err })
		d := w13ObserveValue(func() (value, error) {
			yes, err := h.issetObject(o, "truthy", nil)
			if err != nil {
				return value{}, err
			}
			return boolValue(yes), nil
		})
		result = []any{a, b, c, d, events}
	default:
		t.Fatalf("unknown case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
