package runtime

import (
	"fmt"
	"reflect"
	"testing"
)

// REQ-018/020; T-035/036/039/040. Host safety only: pinned PHP crashes
// when conversion unsets its alias and then clones the source object.
func TestW13PendingReferenceCloneSafety(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			h, err := referenceHeap(width)
			must(t, err)
			var owner, alias, clone, rhs value
			c := w13Build(t, classSpec{name: "H5", properties: []propertySpec{{name: "a", typeName: "string", hasDefault: true, initial: []byte("old")}}})
			must(t, h.assign(&owner, w13Instantiate(t, h, c)))
			must(t, h.referenceObject(&alias, owner, "a", nil))
			text := w13Build(t, classSpec{name: "HS5", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
				if err := h.unset(&alias); err != nil {
					return value{}, err
				}
				copied, err := h.cloneObject(owner, nil)
				if err != nil {
					return value{}, err
				}
				if err := h.assign(&clone, copied); err != nil {
					return value{}, err
				}
				return w13Str("new"), nil
			}}}})
			must(t, h.assign(&rhs, w13Instantiate(t, h, text)))
			must(t, h.assignPHP(&alias, rhs, false))
			original, err := h.readObject(owner, "a", nil)
			must(t, err)
			copied, err := h.readObject(clone, "a", nil)
			must(t, err)
			observed := []any{w13Encode(original), w13Encode(copied), alias.kind != valueUndefined}
			if !reflect.DeepEqual(observed, []any{"new", "new", false}) {
				t.Fatal(observed)
			}
			must(t, h.unset(&rhs))
			must(t, h.unset(&owner))
			must(t, h.unset(&clone))
			globals := value{}
			must(t, h.shutdown(&globals))
			if h.liveObjects != 0 || len(h.typedSources) != 0 {
				t.Fatal("retained objects/sources", h.liveObjects, len(h.typedSources))
			}
		})
	}
}
