package runtime

import "testing"

func w13R3EnumNames() []string {
	return []string{"r3-enum-empty-unit", "r3-enum-empty-int", "r3-enum-empty-string"}
}

func w13R3EnumCase(t *testing.T, h *heap, name string) w13Case {
	kind, className := enumUnit, "W13R3EmptyUnit"
	if name == "r3-enum-empty-int" {
		kind, className = enumInt, "W13R3EmptyInt"
	}
	if name == "r3-enum-empty-string" {
		kind, className = enumString, "W13R3EmptyString"
	}
	c := w13Build(t, classSpec{name: className, enumKind: kind})
	properties := []any{}
	for _, p := range c.schema.props {
		properties = append(properties, []any{p.name, p.typeName, p.readonly})
	}
	result := []any{w13ObserveValue(func() (value, error) { return h.instantiate(c) }), properties}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13EnumDescriptorValidation(t *testing.T) {
	for _, spec := range []classSpec{
		{name: "Bad", enumKind: enumUnit, enumCases: []string{"A"}, enumValues: []any{int64(1)}},
		{name: "Bad", enumKind: enumInt, enumCases: []string{"A"}},
		{name: "Bad", enumKind: enumString, enumCases: []string{"A"}, enumValues: []any{int64(1)}},
		{name: "Bad", enumCases: []string{"A", "B"}, enumValues: []any{int64(1), []byte("b")}},
		{name: "Bad", enumKind: enumKind(9)},
		{name: "Bad", enumKind: enumUnit, properties: []propertySpec{{name: "name"}}},
	} {
		if _, err := buildClass(spec); err == nil {
			t.Errorf("accepted malformed enum descriptor: %+v", spec)
		}
	}
}
