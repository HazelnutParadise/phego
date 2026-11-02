package runtime

import (
	"math"
	"strings"
	"testing"
)

func w13TypeCaseNames() []string {
	names := []string{"getter-return-type", "getter-return-coerce", "reference-weak-coerce", "method-stringable", "callable-missing", "callable-empty", "callable-plain", "callable-invoke", "callable-method", "callable-static"}
	for _, n := range []string{"nan", "inf", "huge", "negative-huge", "string-huge", "fraction", "string-fraction", "numeric"} {
		names = append(names, "property-coerce-"+n)
	}
	return names
}

func w13TypeCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var observed any
	switch {
	case strings.HasPrefix(name, "property-coerce-"):
		inputs := map[string]value{"nan": floatValue(math.NaN()), "inf": floatValue(math.Inf(1)), "huge": floatValue(1e30), "negative-huge": floatValue(-1e30), "string-huge": str("1e30"), "fraction": floatValue(1.5), "string-fraction": str("1.5"), "numeric": str("7")}
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}}))
		observed = w13ObserveValue(func() (value, error) {
			if err := h.writeObject(o, "i", inputs[strings.TrimPrefix(name, "property-coerce-")], nil, false); err != nil {
				return value{}, err
			}
			return h.readObject(o, "i", nil)
		})
	case name == "getter-return-type" || name == "getter-return-coerce":
		className, input := "W13BadGetter", "abc"
		if name == "getter-return-coerce" {
			className, input = "W13NumericGetter", "7"
		}
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "i", typeName: "int", virtual: true, get: func(*heap, value, value) (value, error) { return str(input), nil }}}}))
		observed = w13ObserveValue(func() (value, error) { return h.readObject(o, "i", nil) })
	case name == "reference-weak-coerce":
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Base", properties: []propertySpec{{name: "i", typeName: "int"}}}))
		must(t, h.writeObject(o, "i", w13Int(1), nil, true))
		var r value
		must(t, h.referenceObject(&r, o, "i", nil))
		err := h.assignPHP(&r, str("7"), false)
		if err != nil {
			observed = w13ObserveError(func() error { return err })
		} else {
			v, err := h.readObject(o, "i", nil)
			must(t, err)
			observed = []any{w13Encode(r.deref()), w13Encode(v)}
		}
	default:
		var input value
		method, typ := "callback", "callable"
		switch name {
		case "callable-invoke", "callable-method", "callable-static":
			callable := w13Build(t, classSpec{name: "W13Callable", methods: []methodSpec{{name: "__invoke", call: func(*heap, value, []value) (value, error) { return nullValue(), nil }}, {name: "instance", call: func(*heap, value, []value) (value, error) { return nullValue(), nil }}, {name: "stat", static: true, call: func(*heap, value, []value) (value, error) { return nullValue(), nil }}}})
			obj := w13Instantiate(t, h, callable)
			input = obj
			if name == "callable-static" {
				input = str("W13Callable::stat")
			} else if name == "callable-method" {
				input = value{}
				must(t, h.appendValue(&input, obj))
				must(t, h.appendValue(&input, str("instance")))
			}
		case "callable-missing":
			input = str("does_not_exist")
		case "callable-empty":
			input = h.newArrayValue()
		case "callable-plain":
			var err error
			input, err = h.newObject(&class{name: "stdClass"})
			must(t, err)
		case "method-stringable":
			method, typ = "string", "string"
			input = w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Stringable", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { return str("abc"), nil }}}}))
		default:
			t.Fatal(name)
		}
		c := w13Build(t, classSpec{name: "W13TypedMethods", methods: []methodSpec{{name: method, returnType: "string", parameters: []methodParameter{{name: map[bool]string{true: "s", false: "c"}[typ == "string"], typeName: typ}}, call: func(_ *heap, _ value, args []value) (value, error) {
			if typ == "string" {
				return args[0], nil
			}
			return str("called"), nil
		}}}})
		o := w13Instantiate(t, h, c)
		observed = w13ObserveValue(func() (value, error) { return h.callMethod(o, method, []methodArgument{{value: input}}, nil, false) })
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}
