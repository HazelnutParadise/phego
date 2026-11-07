package runtime

import (
	"fmt"
	"testing"
)

// REQ-018/020; T-035/036/039/040. Go-private constructor ownership must
// survive comparison regardless of which operand already has an owner.
func TestW13R10ComparisonHoldOwners(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, ownedLeft := range []bool{false, true} {
			for _, ownedRight := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%t/%t", width, ownedLeft, ownedRight), func(t *testing.T) {
					h, err := newHeap(width)
					must(t, err)
					c := w13Build(t, classSpec{name: "W13R10Owners", properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(1)}}})
					left, right := w13Instantiate(t, h, c), w13Instantiate(t, h, c)
					var leftOwner, rightOwner value
					if ownedLeft {
						must(t, h.assign(&leftOwner, left))
					}
					if ownedRight {
						must(t, h.assign(&rightOwner, right))
					}
					leftCount, rightCount := left.object.refcount, right.object.refcount
					got, err := h.compareObjectValues(left, right, nil)
					must(t, err)
					if got != 0 || h.liveObjects != 2 || left.object.refcount != leftCount || right.object.refcount != rightCount {
						t.Fatalf("comparison=%d live=%d counts=%d/%d want=%d/%d", got, h.liveObjects, left.object.refcount, right.object.refcount, leftCount, rightCount)
					}
					if ownedLeft {
						must(t, h.unset(&leftOwner))
					} else {
						must(t, h.discard(left))
					}
					if ownedRight {
						must(t, h.unset(&rightOwner))
					} else {
						must(t, h.discard(right))
					}
					if h.liveObjects != 0 || len(h.typedSources) != 0 {
						t.Fatal("comparison retained request values")
					}
				})
			}
		}
	}
}

func BenchmarkW13ObjectComparison(b *testing.B) {
	for _, size := range []int{0, 1, 32} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			spec := classSpec{name: "W13R10Bench"}
			for i := range size {
				spec.properties = append(spec.properties, propertySpec{name: fmt.Sprintf("p%d", i), typeName: "int", hasDefault: true, initial: int64(1)})
			}
			c, err := buildClass(spec)
			if err != nil {
				b.Fatal(err)
			}
			h, err := newHeap(64)
			if err != nil {
				b.Fatal(err)
			}
			var owners [2]value
			for i := range owners {
				v, err := h.instantiate(c)
				if err != nil {
					b.Fatal(err)
				}
				if err := h.assign(&owners[i], v); err != nil {
					b.Fatal(err)
				}
			}
			if _, err := h.compareObjectValues(owners[0], owners[1], nil); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if got, err := h.compareObjectValues(owners[0], owners[1], nil); err != nil || got != 0 {
					b.Fatalf("comparison=%d error=%v", got, err)
				}
			}
			b.StopTimer()
			for i := range owners {
				if err := h.unset(&owners[i]); err != nil {
					b.Fatal(err)
				}
			}
			if h.liveObjects != 0 {
				b.Fatal("comparison retained objects")
			}
		})
	}
}
