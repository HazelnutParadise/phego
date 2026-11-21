package runtime

import (
	"errors"
	"testing"
)

// W-17 stage 7A fixes in the W-13 runtime: REQ-018; T-035, T-036. The
// expectations are PHP 8.5.12's (tests/vm/w17/cases
// fatal_magic_method_get_typed_incompatible, prop_typed_uninit_unset_magic
// and object_bool_arith_context in the pinned images).

// __get answers for a typed property that was unset: the value must be
// assignable to the property's type, and is converted to it in weak mode.
func TestW13MagicGetOfUnsetTypedPropertyIsChecked(t *testing.T) {
	for _, tc := range []struct {
		name    string
		get     value
		want    any
		message string
	}{
		{"compatible", value{kind: valueInt, integer: 7}, int64(7), ""},
		{"numeric string converts", stringValue("12"), int64(12), ""},
		{"incompatible", stringValue("not an int"), nil, "Value of type string returned from W13S7Typed::__get() must be compatible with unset property W13S7Typed::$lazy of type int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := newHeap(64)
			must(t, err)
			c := w13Build(t, classSpec{
				name:       "W13S7Typed",
				properties: []propertySpec{{name: "lazy", typeName: "int"}},
				methods: []methodSpec{{name: "__get", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) {
					return tc.get, nil
				}}},
			})
			var o value
			must(t, h.assign(&o, w13Instantiate(t, h, c)))
			defer func() { must(t, h.unset(&o)) }()
			must(t, h.unsetObject(o, "lazy", nil))
			v, err := h.readObject(o, "lazy", nil)
			if tc.message != "" {
				var php *Throwable
				if !errors.As(err, &php) || php.Class != "TypeError" || php.Message != tc.message {
					t.Fatalf("read = %v, %v; want TypeError %q", v, err, tc.message)
				}
				return
			}
			if err != nil || v.kind != valueInt || v.integer != tc.want.(int64) {
				t.Fatalf("read = %+v, %v; want int %v", v, err, tc.want)
			}
		})
	}
}

// (int) and (float) of an object warn and give 1.
func TestW13ObjectToNumberWarns(t *testing.T) {
	h, err := newHeap(64)
	must(t, err)
	c := w13Build(t, classSpec{name: "W13S7Plain"})
	var o value
	must(t, h.assign(&o, w13Instantiate(t, h, c)))
	defer func() { must(t, h.unset(&o)) }()
	if n := h.toInt(o); n != 1 {
		t.Errorf("toInt = %d, want 1", n)
	}
	if f := h.toFloat(o); f != 1 {
		t.Errorf("toFloat = %v, want 1", f)
	}
	want := []string{"Object of class W13S7Plain could not be converted to int", "Object of class W13S7Plain could not be converted to float"}
	if len(h.diagnostics) != 2 || h.diagnostics[0].message != want[0] || h.diagnostics[1].message != want[1] || h.diagnostics[0].level != levelWarning {
		t.Errorf("diagnostics = %+v, want warnings %q", h.diagnostics, want)
	}
}
