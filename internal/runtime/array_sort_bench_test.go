package runtime

import (
	"fmt"
	"testing"
)

// REQ-017; T-033/T-034: record time, allocations and actual PHP comparator
// calls. Comparison-count conformance is separately checked by native traces.
func BenchmarkW12Sorting(b *testing.B) {
	for _, size := range []int{32, 128, 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			h := benchHeap(b)
			var source value
			seed := int64(73)
			for i := 0; i < size; i++ {
				seed = (seed*73 + 19) % 9973
				if e := h.appendValue(&source, h.number(seed)); e != nil {
					b.Fatal(e)
				}
			}
			defer h.unset(&source)
			calls := 0
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var a value
				if e := h.assign(&a, source); e != nil {
					b.Fatal(e)
				}
				if e := h.sortArray(&a, "usort", false, func(x, y value) (int, error) { calls++; return h.compare(x, y) }); e != nil {
					b.Fatal(e)
				}
				if e := h.unset(&a); e != nil {
					b.Fatal(e)
				}
			}
			b.ReportMetric(float64(calls)/float64(b.N), "comparisons/op")
		})
	}
}

// BenchmarkW12SortingEqual sorts values that all compare equal, so every
// comparison falls back to the original positions.
func BenchmarkW12SortingEqual(b *testing.B) {
	h := benchHeap(b)
	var source value
	for i := 0; i < 1024; i++ {
		if e := h.appendValue(&source, h.number(7)); e != nil {
			b.Fatal(e)
		}
	}
	defer h.unset(&source)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var a value
		if e := h.assign(&a, source); e != nil {
			b.Fatal(e)
		}
		if e := h.sortArray(&a, "sort", false, func(x, y value) (int, error) { return 0, nil }); e != nil {
			b.Fatal(e)
		}
		if e := h.unset(&a); e != nil {
			b.Fatal(e)
		}
	}
}
