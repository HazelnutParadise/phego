package runtime

import (
	"fmt"
	"reflect"
	"testing"
)

func w13R7Names() []string {
	return []string{"r7-default-int", "r7-default-null", "r7-default-unset-typed", "r7-default-unset-untyped", "r7-default-array", "r7-default-reference", "r7-default-object", "r7-default-retry", "r7-default-retry-raw"}
}

// REQ-018/020; T-035/036. Materialized slots survive unrelated ghost defaults.
func w13R7Case(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	className := "W13R7DefaultsMixed"
	props := []propertySpec{{name: "x", typeName: "mixed", hasDefault: true, initial: int64(1)}, {name: "y", typeName: "int", hasDefault: true, initial: int64(2)}}
	if name == "r7-default-unset-untyped" {
		className = "W13R7DefaultsUntyped"
		props[0] = propertySpec{name: "x", hasDefault: true, initial: int64(1)}
	}
	if name == "r7-default-int" || name == "r7-default-unset-typed" {
		className = "W13R7DefaultsInt"
		props[0] = propertySpec{name: "x", typeName: "int", hasDefault: true, initial: int64(1)}
	}
	if name == "r7-default-array" {
		className = "W13R7DefaultsArray"
		props[0] = propertySpec{name: "x", typeName: "array", hasDefault: true, initial: []SnapshotEntry{{Key: int64(0), Value: int64(1)}}}
	}
	if name == "r7-default-object" {
		className = "W13R7DefaultsObject"
		props[0] = propertySpec{name: "x", typeName: "mixed", hasDefault: true, initial: nil}
	}
	props = append(props, propertySpec{name: "z", typeName: "int", hasDefault: true, initial: int64(3)})
	if name == "r7-default-null" {
		className = "W13R7DefaultsNull"
	}
	var c *class
	c = w13Build(t, classSpec{name: className, properties: props})
	var objectValue value
	var payloadOwner value
	if name == "r7-default-object" {
		oc := w13Build(t, classSpec{name: "W13R7Payload", destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }})
		must(t, h.assign(&payloadOwner, w13Instantiate(t, h, oc)))
		objectValue = payloadOwner
	}
	calls := 0
	o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
		calls++
		events = append(events, []any{"init", calls})
		if name == "r7-default-retry-raw" && calls == 1 {
			if err := h.writeRawObjectProperty(this, "z", w13Int(99), false); err != nil {
				return value{}, err
			}
			events = append(events, []any{"raw", h.isLazyProperty(this, "z")})
		}
		if (name == "r7-default-retry" || name == "r7-default-retry-raw") && calls == 1 {
			events = append(events, "throw")
			return value{}, &Throwable{"Exception", "retry"}
		}
		return nullValue(), nil
	})
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	var alias value
	defer func() {
		must(t, h.unset(&alias))
		must(t, h.unset(&payloadOwner))
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatalf("defaults case retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
		}
	}()
	initial := w13Int(42)
	if name == "r7-default-null" {
		initial = nullValue()
	}
	if name == "r7-default-array" {
		initial = h.newArrayValue()
		must(t, h.setElement(&initial, w13Int(0), w13Int(42)))
	}
	if name == "r7-default-object" {
		initial = objectValue
	}
	must(t, h.writeRawObjectProperty(o, "x", initial, false))
	if name == "r7-default-unset-typed" || name == "r7-default-unset-untyped" {
		must(t, h.unsetObject(o, "x", nil))
	}
	if name == "r7-default-reference" {
		must(t, h.referenceObject(&alias, o, "x", nil))
	}
	if name == "r7-default-retry" {
		must(t, h.writeRawObjectProperty(o, "z", w13Int(3), false))
		must(t, h.unsetObject(o, "z", nil))
	}
	read := func(field string) any {
		return w13ObserveValue(func() (value, error) { return h.readObject(o, field, nil) })
	}
	result := []any{h.isLazyObject(o)}
	if name == "r7-default-retry" || name == "r7-default-retry-raw" {
		result = append(result, read("y"), h.isLazyObject(o))
		if name == "r7-default-retry-raw" {
			result = append(result, h.isLazyProperty(o, "z"))
		}
		result = append(result, read("y"), h.isLazyObject(o), read("x"), read("z"))
	} else if name == "r7-default-reference" {
		result = append(result, w13ObserveValue(func() (value, error) { return h.readObject(o, "y", nil) }), h.isLazyObject(o))
		must(t, h.assign(&alias, w13Int(43)))
		result = append(result, w13Encode(alias), read("x"))
	} else if name == "r7-default-object" {
		result = append(result, read("y"), h.isLazyObject(o), o.object.props.slot("x").deref().object == objectValue.object, read("z"))
	} else {
		result = append(result, w13ObserveValue(func() (value, error) { return h.readObject(o, "y", nil) }), h.isLazyObject(o), read("x"), read("z"))
	}
	if name == "r7-default-object" {
		result = append(result, append([]any{}, events...))
		must(t, h.unset(&payloadOwner))
		result = append(result, append([]any{}, events...))
		must(t, h.unset(&owner))
		owner = value{}
		result = append(result, append([]any{}, events...))
	}
	return w13Case{Result: []any{true, result, append([]any{}, events...)}, Diagnostics: w13Diagnostics(h)}
}

func TestW13R7PHP8512Regression(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R7Names() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R7Case(t, h, name))
			})
		}
	}
}

func TestW13R7FreshDefaults(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			h, err := newHeap(width)
			must(t, err)
			c := w13Build(t, classSpec{name: "W13R7Fresh", properties: []propertySpec{{name: "i", typeName: "int", hasDefault: true, initial: int64(2)}, {name: "n", typeName: "?int", hasDefault: true, initial: nil}, {name: "a", typeName: "array", hasDefault: true, initial: []SnapshotEntry{{Key: int64(0), Value: int64(1)}}}, {name: "u", typeName: "int"}}})
			o := w13Instantiate(t, h, c)
			var owner value
			must(t, h.assign(&owner, o))
			defer func() {
				must(t, h.unset(&owner))
				if h.liveObjects != 0 || len(h.typedSources) != 0 {
					t.Fatal("fresh defaults retained values")
				}
			}()
			got := []any{w13ObserveValue(func() (value, error) { return h.readObject(o, "i", nil) }), w13ObserveValue(func() (value, error) { return h.readObject(o, "n", nil) }), w13ObserveValue(func() (value, error) { return h.readObject(o, "a", nil) }), w13ObserveValue(func() (value, error) { return h.readObject(o, "u", nil) })}
			want := []any{int64(2), nil, []any{int64(1)}, []any{"Error", "Typed property W13R7Fresh::$u must not be accessed before initialization"}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("fresh defaults: got %#v want %#v", got, want)
			}
		})
	}
}
