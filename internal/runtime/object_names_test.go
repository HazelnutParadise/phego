package runtime

import "testing"

func w13NameCases() []string {
	return []string{"method-non-ascii", "class-non-ascii", "property-non-ascii-type"}
}
func w13NameCase(t *testing.T, h *heap, name string) w13Case {
	var observed any
	switch name {
	case "method-non-ascii":
		c := w13Build(t, classSpec{name: "W13NameMethods", methods: []methodSpec{{name: "Ö", call: func(*heap, value, []value) (value, error) { return str("upper"), nil }}, {name: "ö", call: func(*heap, value, []value) (value, error) { return str("lower"), nil }}}})
		o := w13Instantiate(t, h, c)
		a, err := h.callMethod(o, "Ö", nil, nil, false)
		must(t, err)
		b, err := h.callMethod(o, "ö", nil, nil, false)
		must(t, err)
		observed = []any{w13Encode(a), w13Encode(b)}
	case "class-non-ascii":
		a := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13Ö"}))
		c := w13Build(t, classSpec{name: "W13ö"})
		b, err := h.instantiate(c)
		if err != nil {
			observed = w13ObserveError(func() error { return err })
		} else {
			observed = []any{a.object.class.name, b.object.class.name}
		}
	case "property-non-ascii-type":
		o := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13NameTypes", properties: []propertySpec{{name: "o", typeName: "W13Ö"}}}))
		v := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13ö"}))
		observed = w13ObserveError(func() error { return h.writeObject(o, "o", v, nil, true) })
	default:
		t.Fatal(name)
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}
