package runtime

import (
	"reflect"
	"testing"
)

// REQ-017; T-033/T-034. PHP 8.5.12 counterexamples: observable comparison
// direction/order cannot be substituted with Go's stable-sort contract.
func TestW12SortObservableRegression(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, want []int64
		constant    int
		calls       [][2]int64
	}{
		{name: "always-positive", input: []int64{3, 1, 2}, want: []int64{1, 2, 3}, constant: 1},
		{name: "always-negative", input: []int64{3, 1, 2, 5, 4}, want: []int64{3, 1, 2, 5, 4}, constant: -1},
		{name: "argument-order", input: []int64{5, 3, 8, 1, 9, 2}, want: []int64{1, 2, 3, 5, 8, 9}, calls: [][2]int64{{5, 3}, {5, 8}, {8, 1}, {5, 1}, {3, 1}, {8, 9}, {9, 2}, {8, 2}, {5, 2}, {3, 2}, {1, 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHeap(t)
			var a value
			for _, v := range tc.input {
				must(t, h.appendValue(&a, h.number(v)))
			}
			calls := [][2]int64{}
			must(t, h.sortArray(&a, "usort", false, func(x, y value) (int, error) {
				calls = append(calls, [2]int64{x.integer, y.integer})
				if tc.constant != 0 {
					return tc.constant, nil
				}
				return h.compare(x, y)
			}))
			got := []int64{}
			for _, b := range a.deref().array.buckets {
				got = append(got, b.val.deref().integer)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("values %v, PHP %v", got, tc.want)
			}
			if tc.calls != nil && !reflect.DeepEqual(calls, tc.calls) {
				t.Errorf("calls %v, PHP %v", calls, tc.calls)
			}
			must(t, h.unset(&a))
		})
	}
}

// REQ-017; T-033. PHP 8.5.12 sort() keeps elements reachable by iteration
// during comparisons but not by key: the arm64, i386 and amd64 images report
// "Undefined array key 1" for $a[1] read from __toString inside sort($a), and
// count($a) stays 3. usort() compares a private copy, where $a[1] still reads.
func TestW12SortKeyLookupDuringComparisons(t *testing.T) {
	for _, tc := range []struct {
		name  string
		user  bool
		found bool
	}{
		{name: "native", user: false, found: false},
		{name: "user", user: true, found: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHeap(t)
			var a value
			var seen []bool
			var counts []int
			c := w13Build(t, classSpec{name: "W12ReadDuringSort", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
				_, ok, err := h.getElement(a.deref(), h.number(1))
				seen = append(seen, ok)
				counts = append(counts, a.deref().array.count())
				return w13Str("b"), err
			}}}})
			o, err := h.newObject(c)
			must(t, err)
			must(t, h.appendValue(&a, o))
			must(t, h.appendValue(&a, w13Str("a")))
			must(t, h.appendValue(&a, w13Str("c")))
			if tc.user {
				must(t, h.sortArray(&a, "usort", false, h.compare))
			} else {
				must(t, h.sortArrayRegular(&a, "sort", false))
			}
			if len(seen) == 0 {
				t.Fatal("__toString was not called")
			}
			for i := range seen {
				if seen[i] != tc.found || counts[i] != 3 {
					t.Errorf("comparison %d: key 1 found=%t count=%d; PHP found=%t count=3", i, seen[i], counts[i], tc.found)
				}
			}
			if _, ok, _ := h.getElement(a.deref(), h.number(1)); !ok {
				t.Error("key 1 missing after sort")
			}
			must(t, h.unset(&a))
		})
	}
}
