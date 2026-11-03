package runtime

import (
	"math"
	"testing"
)

// REQ-018; T-035/036. These validate private descriptors, not PHP compile diagnostics.
func TestW13R11TraitDescriptorCollisions(t *testing.T) {
	base := propertySpec{name: "x", visibility: privateAccess, typeName: "mixed", hasDefault: true, initial: int64(1)}
	for _, name := range []string{"default-kind", "default-value", "visibility", "static", "readonly", "type", "get-hook", "set-hook", "array-order", "nan"} {
		t.Run(name, func(t *testing.T) {
			a, b := base, base
			switch name {
			case "default-kind":
				b.initial = float64(1)
			case "default-value":
				b.initial = int64(2)
			case "visibility":
				b.visibility = publicAccess
			case "static":
				b.static = true
			case "readonly":
				b.readonly = true
			case "type":
				b.typeName = "int"
			case "get-hook":
				b.get = func(*heap, value, value) (value, error) { return nullValue(), nil }
			case "set-hook":
				b.set = func(*heap, value, value) (value, error) { return nullValue(), nil }
			case "array-order":
				a.initial = []SnapshotEntry{{Key: "a", Value: nil}, {Key: "b", Value: nil}}
				b.initial = []SnapshotEntry{{Key: "b", Value: nil}, {Key: "a", Value: nil}}
			case "nan":
				a.initial, b.initial = math.NaN(), math.NaN()
			}
			for _, visibility := range []access{privateAccess, protectedAccess, publicAccess} {
				a.visibility = visibility
				if name != "visibility" {
					b.visibility = visibility
				} else {
					b.visibility = (visibility + 1) % 3
				}
				ta := w13Build(t, classSpec{name: "W13R11InvalidA", isTrait: true, properties: []propertySpec{a}})
				tb := w13Build(t, classSpec{name: "W13R11InvalidB", isTrait: true, properties: []propertySpec{b}})
				if _, err := buildClass(classSpec{name: "W13R11InvalidConsumer", traits: []*class{ta, tb}}); err == nil {
					t.Fatalf("accepted incompatible %s visibility=%d", name, visibility)
				}
			}
		})
	}
}

func TestW13R11TraitIgnoredUntypedInitial(t *testing.T) {
	a := w13Build(t, classSpec{name: "W13R11IgnoredInitialA", isTrait: true, properties: []propertySpec{{name: "x", initial: int64(9)}}})
	b := w13Build(t, classSpec{name: "W13R11IgnoredInitialB", isTrait: true, properties: []propertySpec{{name: "x", hasDefault: true}}})
	c := w13Build(t, classSpec{name: "W13R11IgnoredInitialConsumer", traits: []*class{a, b}})
	for _, width := range []int{32, 64} {
		h, err := newHeap(width)
		must(t, err)
		obj := w13Instantiate(t, h, c)
		got, err := h.readObject(obj, "x", nil)
		must(t, err)
		if got.kind != valueNull {
			t.Fatal("omitted untyped default did not remain null")
		}
		must(t, h.discard(obj))
	}
}

func TestW13R11TraitSnapshotAndTypeEquivalence(t *testing.T) {
	for _, tc := range []struct {
		name, typA, typB string
		a, b             any
	}{
		{"duplicate-normalized-key", "array", "array", []SnapshotEntry{{Key: "1", Value: int64(4)}, {Key: int64(1), Value: int64(5)}}, []SnapshotEntry{{Key: int64(1), Value: int64(5)}}},
		{"nested-keys", "array", "array", []SnapshotEntry{{Key: "k", Value: []SnapshotEntry{{Key: "2", Value: []byte(nil)}}}}, []SnapshotEntry{{Key: "k", Value: []SnapshotEntry{{Key: int64(2), Value: []byte{}}}}}},
		{"empty-array", "array", "array", []SnapshotEntry(nil), []SnapshotEntry{}},
		{"empty-string", "string", "string", []byte(nil), []byte{}},
		{"union", "int|string", "string|int", int64(1), int64(1)},
		{"nullable", "?int", "null|int", nil, nil},
		{"intersection", "(W13A&W13B)|null", "null|(w13b&w13a)", nil, nil},
		{"float-widen", "float", "float", int64(1), float64(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := propertySpec{name: "x", visibility: privateAccess, typeName: tc.typA, hasDefault: true, initial: tc.a}
			b := propertySpec{name: "x", visibility: privateAccess, typeName: tc.typB, hasDefault: true, initial: tc.b}
			ta := w13Build(t, classSpec{name: "W13R11CompatibleA", isTrait: true, properties: []propertySpec{a}})
			tb := w13Build(t, classSpec{name: "W13R11CompatibleB", isTrait: true, properties: []propertySpec{b}})
			c := w13Build(t, classSpec{name: "W13R11CompatibleConsumer", traits: []*class{ta, tb}})
			if len(c.schema.props) != 1 || len(c.schema.layout.names) != 1 {
				t.Fatalf("compatible traits produced %d descriptors/%d slots", len(c.schema.props), len(c.schema.layout.names))
			}
			if c.schema.props[0].typeName != tc.typA {
				t.Fatal("replaced first descriptor instead of preserving it")
			}
		})
	}
}
