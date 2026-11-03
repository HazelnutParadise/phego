package runtime

import (
	"fmt"
	"testing"
)

// REQ-012/018; T-023/024/035/036. Keep dispatch lookup independent of the
// number of unrelated methods without introducing another index.
func BenchmarkW13ScopedMethodResolution(b *testing.B) {
	for _, size := range []int{1, 64, 1024} {
		spec := classSpec{name: "W13R4Lookup"}
		for i := range size {
			spec.methods = append(spec.methods, methodSpec{name: fmt.Sprintf("method%d", i)})
		}
		spec.methods = append(spec.methods, methodSpec{name: "target"})
		c, err := buildClass(spec)
		if err != nil {
			b.Fatal(err)
		}
		h, err := newHeap(64)
		if err != nil {
			b.Fatal(err)
		}
		expected := c.schema.methods["target"]
		for _, scoped := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/scoped=%t", size, scoped), func(b *testing.B) {
				var scope *class
				if scoped {
					scope = c
				}
				b.ReportAllocs()
				for b.Loop() {
					m, ok := h.resolveMethod(c, "target", scope)
					if !ok || m != expected {
						b.Fatal("lookup changed")
					}
				}
			})
		}
	}
}
