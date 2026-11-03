package runtime

import (
	"fmt"
	"testing"
)

// REQ-018/019/021; T-035/036/038/042. Shared descriptors carry no mutable
// instance defaults, static cells, class-name table or enum instances.
func TestW13ParallelRequestIsolation(t *testing.T) {
	c := w13Build(t, classSpec{name: "SharedClass", properties: []propertySpec{{name: "items", typeName: "array", hasDefault: true, initial: []SnapshotEntry{{Key: int64(0), Value: int64(1)}}}, {name: "counter", typeName: "int", static: true, hasDefault: true, initial: int64(0)}}})
	e := w13Build(t, classSpec{name: "SharedEnum", enumCases: []string{"A"}, enumValues: []any{[]byte("a")}})
	type identities struct {
		array *phpArray
		enum  *object
	}
	results := make(chan identities, 8)
	t.Run("requests", func(t *testing.T) {
		for i := 1; i <= 8; i++ {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				h, err := newHeap(32 + 32*(i%2))
				must(t, err)
				o := w13Instantiate(t, h, c)
				slot := o.object.props.slot("items")
				initial, ok, err := h.getElement(*slot, w13Int(0))
				must(t, err)
				if !ok || initial.integer != 1 {
					t.Fatal("default contaminated")
				}
				must(t, h.appendValue(slot, w13Int(int64(i))))
				itemArray := slot.array
				for range 64 {
					must(t, h.writeStatic(c, "counter", w13Int(int64(i)), nil, true))
					v, err := h.readStatic(c, "counter", nil)
					must(t, err)
					if v.integer != int64(i) {
						t.Fatal("shared static cell", v)
					}
				}
				enum, err := h.enumCase(e, "A")
				must(t, err)
				same, err := h.enumCase(e, "A")
				must(t, err)
				if enum.object != same.object {
					t.Fatal("enum identity changed within request")
				}
				results <- identities{itemArray, enum.object}
				globals := nullValue()
				must(t, h.shutdown(&globals))
			})
		}
	})
	close(results)
	arrays := map[*phpArray]bool{}
	enums := map[*object]bool{}
	for result := range results {
		if arrays[result.array] || enums[result.enum] {
			t.Fatal("request values crossed heaps")
		}
		arrays[result.array] = true
		enums[result.enum] = true
	}
	if len(arrays) != 8 || len(enums) != 8 {
		t.Fatal("incomplete isolation exercise")
	}
}
