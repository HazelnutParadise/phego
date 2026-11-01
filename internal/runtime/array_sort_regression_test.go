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
			must(t, h.sortArray(&a, false, func(x, y value) (int, error) {
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
