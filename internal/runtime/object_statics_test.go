package runtime

import (
	"fmt"
	"testing"
)

func w13StaticNames() []string {
	return []string{"static-instance-read", "static-instance-write", "static-instance-reference", "static-instance-unset", "static-instance-isset", "static-instance-protected", "static-reference-type", "static-reference-uninitialized", "static-reference-nullable", "static-unset"}
}

func w13StaticCase(t *testing.T, h *heap, name string) w13Case {
	c := w13Build(t, classSpec{name: "W13StaticCells", properties: []propertySpec{
		{name: "p", typeName: "int", static: true, hasDefault: true, initial: int64(1)},
		{name: "u", typeName: "int", static: true}, {name: "n", typeName: "?int", static: true},
		{name: "hidden", typeName: "int", static: true, visibility: protectedAccess, hasDefault: true, initial: int64(2)},
		{name: "instance", typeName: "int", hasDefault: true, initial: int64(3)},
	}})
	o := w13Instantiate(t, h, c)
	readStatic := func() any { return w13ObserveValue(func() (value, error) { return h.readStatic(c, "p", nil) }) }
	readInstance := func() any { return w13ObserveValue(func() (value, error) { return h.readObject(o, "p", nil) }) }
	var result any
	switch name {
	case "static-instance-read":
		result = []any{readInstance(), readStatic()}
	case "static-instance-write":
		must(t, h.writeObject(o, "p", w13Str("dynamic"), nil, false))
		result = []any{readInstance(), readStatic()}
	case "static-instance-reference":
		var alias value
		must(t, h.referenceObject(&alias, o, "p", nil))
		must(t, h.assignPHP(&alias, w13Str("alias"), false))
		result = []any{readInstance(), readStatic()}
	case "static-instance-unset":
		must(t, h.unsetObject(o, "p", nil))
		result = readStatic()
	case "static-instance-isset":
		v, err := h.issetObject(o, "p", nil)
		must(t, err)
		s, err := h.readStatic(c, "p", nil)
		must(t, err)
		result = []any{v, s.kind != valueNull}
	case "static-instance-protected":
		result = w13ObserveValue(func() (value, error) { return h.readObject(o, "hidden", nil) })
	case "static-reference-type":
		var alias value
		must(t, h.referenceStatic(&alias, c, "p", nil))
		err := w13ObserveError(func() error { return h.assignPHP(&alias, w13Str("bad"), false) })
		result = []any{err, w13Encode(alias), readStatic()}
	case "static-reference-uninitialized":
		var alias value
		result = w13ObserveError(func() error { return h.referenceStatic(&alias, c, "u", nil) })
	case "static-reference-nullable":
		var alias value
		must(t, h.referenceStatic(&alias, c, "n", nil))
		v, err := h.readStatic(c, "n", nil)
		must(t, err)
		result = []any{w13Encode(alias), w13Encode(v)}
	case "static-unset":
		results := []any{}
		for _, property := range []string{"p", "hidden", "missing", "instance"} {
			results = append(results, w13ObserveError(func() error { return h.unsetStatic(c, property) }))
		}
		result = results
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13StaticPropertyCells(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13StaticNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13StaticCase(t, h, name))
			})
		}
	}
}
