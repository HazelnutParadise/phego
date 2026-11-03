package runtime

import "testing"

// These are private descriptor checks, not PHP declaration diagnostics.
func TestW13R12SetterDescriptorValidation(t *testing.T) {
	setter := func(*heap, value, value) (value, error) { return nullValue(), nil }
	for _, test := range []struct {
		name       string
		properties []propertySpec
		parameters map[string]hookParameter
	}{
		{"unknown", nil, map[string]hookParameter{"x": {name: "v", typeName: "int"}}},
		{"case-sensitive", []propertySpec{{name: "X", set: setter}}, map[string]hookParameter{"x": {name: "v"}}},
		{"no-setter", []propertySpec{{name: "x"}}, map[string]hookParameter{"x": {name: "v"}}},
		{"unnamed", []propertySpec{{name: "x", set: setter}}, map[string]hookParameter{"x": {typeName: "int"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := buildClass(classSpec{name: "W13R12InvalidMetadata", properties: test.properties, setterParameters: test.parameters}); err == nil {
				t.Fatal("invalid private setter metadata was accepted")
			}
		})
	}
	c := w13Build(t, classSpec{name: "W13R12NoSetterMetadata", setterParameters: map[string]hookParameter{}})
	if c.schema.setterParameters != nil {
		t.Fatal("empty metadata should retain no map")
	}
}
