package runtime

import (
	"fmt"
	"testing"
)

func w13R11NestedTrait(tb testing.TB, count int) *class {
	tb.Helper()
	entries := make([]SnapshotEntry, count)
	for i := range entries {
		entries[i] = SnapshotEntry{Key: int64(i), Value: []SnapshotEntry{{Key: "text", Value: []byte("original")}}}
	}
	c, err := buildClass(classSpec{name: "W13R11NestedTrait", isTrait: true, properties: []propertySpec{{name: "x", typeName: "array", hasDefault: true, initial: entries}}})
	if err != nil {
		tb.Fatal(err)
	}
	// The first class build owns a copy, including nested bytes.
	entries[0].Value.([]SnapshotEntry)[0].Value.([]byte)[0] = 'X'
	return c
}

// REQ-018/021; T-035/036/042. Shared immutable trait snapshots never share PHP cells.
func TestW13R11TraitDefaultIsolation(t *testing.T) {
	trait := w13R11NestedTrait(t, 2)
	a := w13Build(t, classSpec{name: "W13R11NestedConsumerA", traits: []*class{trait}})
	b := w13Build(t, classSpec{name: "W13R11NestedConsumerB", traits: []*class{trait}})
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			t.Parallel()
			h, err := referenceHeap(width)
			must(t, err)
			x, y := w13Instantiate(t, h, a), w13Instantiate(t, h, b)
			left, right := x.object.props.slot("x"), y.object.props.slot("x")
			leftEntry, ok := left.array.lookup(intKey(0))
			if !ok {
				t.Fatal("missing left entry")
			}
			must(t, h.setElement(&leftEntry.val, w13Str("text"), w13Str("changed")))
			rightEntry, ok := right.array.lookup(intKey(0))
			if !ok {
				t.Fatal("missing right entry")
			}
			text, ok, err := h.getElement(rightEntry.val, w13Str("text"))
			must(t, err)
			if !ok || text.text != "original" {
				t.Fatal("trait input or another consumer changed the shared default")
			}
			must(t, h.discard(x))
			must(t, h.discard(y))
		})
	}
}

func BenchmarkW13TraitSnapshotBuild(b *testing.B) {
	trait := w13R11NestedTrait(b, 256)
	b.ReportAllocs()
	for b.Loop() {
		c, err := buildClass(classSpec{name: "W13R11NestedConsumer", traits: []*class{trait}})
		if err != nil || len(c.schema.props) != 1 {
			b.Fatalf("build: %v", err)
		}
	}
}
