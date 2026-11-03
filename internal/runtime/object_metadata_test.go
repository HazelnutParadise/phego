package runtime

import "testing"

// REQ-018; T-035/036. Shared descriptions must be immutable snapshots.
func TestW13EnumDefaultSnapshot(t *testing.T) {
	input := []byte("a")
	c := w13Build(t, classSpec{name: "W13Enum", enumCases: []string{"A"}, enumValues: []any{input}})
	input[0] = 'b'
	h := testHeap(t)
	o, err := h.enumCase(c, "A")
	must(t, err)
	v, err := h.readObject(o, "value", nil)
	must(t, err)
	if v.text != "a" {
		t.Fatal("shared enum descriptor changed", v.text)
	}
}

func TestW13FinalTraitOverride(t *testing.T) {
	trait := w13Build(t, classSpec{name: "FinalTrait", isTrait: true, methods: []methodSpec{{name: "m", final: true}}})
	_, err := buildClass(classSpec{name: "Consumer", traits: []*class{trait}, methods: []methodSpec{{name: "m"}}})
	// A class's own method wins over a trait method, including final ones.
	must(t, err)
}

func TestW13ImmutableMethodIdentity(t *testing.T) {
	parent := w13Build(t, classSpec{name: "Parent", methods: []methodSpec{{name: "m"}}})
	child := w13Build(t, classSpec{name: "Child", parent: parent})
	h := testHeap(t)
	a, _ := h.resolveMethod(parent, "m", nil)
	b, _ := h.resolveMethod(parent, "M", nil)
	inherited, _ := h.resolveMethod(child, "m", nil)
	if a != b || a != inherited {
		t.Fatal("lookup copied an immutable method descriptor")
	}
}

// REQ-012; T-024. A failed parent registration cannot publish its child.
func TestW13ClassRegistrationAtomic(t *testing.T) {
	h := testHeap(t)
	original := w13Build(t, classSpec{name: "RegisteredParent"})
	must(t, h.registerClass(original))
	conflict := w13Build(t, classSpec{name: "RegisteredParent"})
	child := w13Build(t, classSpec{name: "RegisteredChild", parent: conflict})
	for range 2 {
		if _, err := h.instantiate(child); err == nil {
			t.Fatal("parent conflict bypassed on retry")
		}
		if h.classes[phpNameLower(child.name)] != nil {
			t.Fatal("failed declaration published a child")
		}
	}
	if h.classes[phpNameLower(original.name)] != original {
		t.Fatal("conflict replaced the existing class")
	}
}
