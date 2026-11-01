package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// REQ-017; T-033/T-034: every PHP callback invocation reloads live bucket
// references, including a retry after a callback or diagnostic handler write.
func TestW12SortUserRetryLiveReference(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		for _, inDiagnostic := range []bool{false, true} {
			t.Run(fmt.Sprint(preserve, "/", inDiagnostic), func(t *testing.T) {
				h := testHeap(t)
				var a, alias value
				must(t, h.appendValue(&a, h.number(1)))
				must(t, h.appendValue(&a, h.number(2)))
				element, err := h.element(&a, h.number(0))
				must(t, err)
				must(t, h.bind(&alias, element))
				calls := [][2]int64{}
				must(t, h.sortArrayUserWithDiagnostic(&a, preserve, func(x, y value) (value, error) {
					calls = append(calls, [2]int64{x.integer, y.integer})
					if len(calls) == 1 {
						if !inDiagnostic {
							must(t, h.assign(&alias, h.number(10)))
						}
						return boolValue(false), nil
					}
					return h.number(x.integer - y.integer), nil
				}, func(int, string) error {
					if inDiagnostic {
						return h.assign(&alias, h.number(10))
					}
					return nil
				}))
				if want := [][2]int64{{1, 2}, {2, 10}}; !reflect.DeepEqual(calls, want) {
					t.Fatalf("calls=%v, PHP=%v", calls, want)
				}
				for i, want := range []int64{2, 10} {
					if got := a.deref().array.buckets[i].val.deref().integer; got != want {
						t.Fatalf("item%d=%d, PHP=%d", i, got, want)
					}
				}
				must(t, h.unset(&alias))
				must(t, h.unset(&a))
			})
		}
	}
}

// REQ-017; T-034: retry shutdown must prevent further host diagnostics,
// preserve the callback error, and release its temporary return object.
func TestW12SortUserRetryClosedHeap(t *testing.T) {
	for _, callbackError := range []error{nil, context.Canceled} {
		t.Run(fmt.Sprint(callbackError), func(t *testing.T) {
			h := testHeap(t)
			var a value
			must(t, h.appendValue(&a, h.number(1)))
			must(t, h.appendValue(&a, h.number(2)))
			calls, afterClose := 0, 0
			var returned *object
			err := h.sortArrayUserWithDiagnostic(&a, false, func(x, y value) (value, error) {
				calls++
				if calls == 1 {
					return boolValue(false), nil
				}
				v, e := h.newObject(&class{name: "stdClass"})
				must(t, e)
				returned = v.object
				must(t, h.shutdown(&a))
				return v, callbackError
			}, func(int, string) error {
				if h.closed {
					afterClose++
				}
				return nil
			})
			want := callbackError
			if want == nil {
				want = ErrHeapClosed
			}
			if !errors.Is(err, want) || calls != 2 || afterClose != 0 {
				t.Fatalf("error=%v calls=%d diagnostics-after-close=%d", err, calls, afterClose)
			}
			if returned == nil || !returned.freed || returned.refcount != 0 {
				t.Fatalf("return object not cleaned: %+v", returned)
			}
		})
	}
}

// REQ-017; T-034: callback return ownership outlives its argument holds.
func TestW12SortUserBorrowedArgumentResult(t *testing.T) {
	h := testHeap(t)
	var a, alias value
	destroyed := 0
	v, err := h.newObject(&class{name: "SortResult", destructor: func(*heap, value) error { destroyed++; return nil }})
	must(t, err)
	must(t, h.appendValue(&a, v))
	must(t, h.appendValue(&a, h.number(2)))
	element, err := h.element(&a, h.number(0))
	must(t, err)
	must(t, h.bind(&alias, element))
	warnings := 0
	must(t, h.sortArrayUserWithDiagnostic(&a, false, func(x, y value) (value, error) {
		must(t, h.assign(&alias, h.number(10)))
		return x, nil
	}, func(int, string) error {
		warnings++
		if v.object.freed || destroyed != 0 {
			t.Fatal("borrowed result freed before coercion completed")
		}
		return nil
	}))
	if warnings != 1 || !v.object.freed || v.object.refcount != 0 || destroyed != 1 {
		t.Fatalf("warnings=%d destroyed=%d object=%+v", warnings, destroyed, v.object)
	}
	must(t, h.unset(&alias))
	must(t, h.unset(&a))
}

// REQ-017; T-034: a reference-valued callback result is coerced directly;
// PHP's bool-return deprecation/retry tests the actual return kind first.
func TestW12SortUserReferenceBoolean(t *testing.T) {
	h := testHeap(t)
	var a, ret, alias value
	for _, i := range []int64{1, 3, 2} {
		must(t, h.appendValue(&a, h.number(i)))
	}
	must(t, h.assign(&ret, boolValue(false)))
	must(t, h.bind(&alias, &ret))
	calls := [][2]int64{}
	must(t, h.sortArrayUser(&a, false, func(x, y value) (value, error) {
		calls = append(calls, [2]int64{x.integer, y.integer})
		if e := h.assign(&ret, boolValue(x.integer > y.integer)); e != nil {
			return value{}, e
		}
		return alias, nil
	}))
	if want := [][2]int64{{1, 3}, {3, 2}, {1, 2}}; !reflect.DeepEqual(calls, want) || len(h.diagnostics) != 0 {
		t.Fatalf("calls=%v diagnostics=%+v", calls, h.diagnostics)
	}
	must(t, h.unset(&a))
	must(t, h.unset(&alias))
	must(t, h.unset(&ret))
}

func TestW12SortDiagnosticFailure(t *testing.T) {
	for _, truth := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[truth], func(t *testing.T) {
			h := testHeap(t)
			var a value
			for _, i := range []int64{3, 1, 2} {
				must(t, h.appendValue(&a, h.number(i)))
			}
			stop := errors.New("comparison warning failed")
			calls, warnings := 0, 0
			err := h.sortArrayUserWithDiagnostic(&a, false, func(x, y value) (value, error) { calls++; return boolValue(truth), nil }, func(level int, message string) error { warnings++; return stop })
			if !errors.Is(err, stop) || calls != 1 || warnings != 1 {
				t.Fatalf("error=%v calls=%d warnings=%d", err, calls, warnings)
			}
			want := []int64{3, 1, 2}
			if truth {
				want = []int64{1, 3, 2}
			}
			for i, b := range a.deref().array.buckets {
				if b.val.integer != want[i] {
					t.Fatalf("partial permutation differs from PHP: %+v", a.deref())
				}
			}
			must(t, h.unset(&a))
		})
	}
}

func TestW12SortUserClosedHeap(t *testing.T) {
	h := testHeap(t)
	var a value
	must(t, h.appendValue(&a, h.number(1)))
	must(t, h.appendValue(&a, h.number(2)))
	calls, warnings := 0, 0
	err := h.sortArrayUserWithDiagnostic(&a, false, func(x, y value) (value, error) {
		calls++
		if e := h.shutdown(&a); e != nil {
			return value{}, e
		}
		return boolValue(false), nil
	}, func(int, string) error { warnings++; return nil })
	if !errors.Is(err, ErrHeapClosed) || calls != 1 || warnings != 0 {
		t.Fatalf("error=%v calls=%d warnings=%d", err, calls, warnings)
	}
}

// REQ-017; T-034. Conversion warnings must use the same PHP handler hook as
// bool deprecations, retain the effective result and release temporary values.
func TestW12SortObjectCoercionDiagnostic(t *testing.T) {
	h := testHeap(t)
	var a value
	for _, i := range []int64{3, 1, 2} {
		must(t, h.appendValue(&a, h.number(i)))
	}
	defer h.unset(&a)
	calls, warnings := 0, 0
	stop := errors.New("Object of class stdClass could not be converted to int")
	objects := []*object{}
	err := h.sortArrayUserWithDiagnostic(&a, false, func(x, y value) (value, error) {
		calls++
		v, e := h.newObject(&class{name: "stdClass"})
		objects = append(objects, v.object)
		return v, e
	}, func(level int, message string) error {
		warnings++
		if level != levelWarning || message != stop.Error() {
			t.Fatalf("diagnostic %d %q", level, message)
		}
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 || warnings != 1 {
		t.Fatalf("error=%v calls=%d warnings=%d", err, calls, warnings)
	}
	for i, want := range []int64{1, 3, 2} {
		if a.deref().array.buckets[i].val.integer != want {
			t.Fatal("partial permutation differs from PHP")
		}
	}
	for _, o := range objects {
		if !o.freed || o.refcount != 0 {
			t.Fatal("temporary comparator result retained")
		}
	}
}

// REQ-017; T-034. A nested user sort resets PHP's shared warning flag.
func TestW12SortUserNestedDeprecation(t *testing.T) {
	h := testHeap(t)
	var a, inner value
	for _, i := range []int64{1, 3, 2} {
		must(t, h.appendValue(&a, h.number(i)))
	}
	must(t, h.appendValue(&inner, h.number(2)))
	must(t, h.appendValue(&inner, h.number(1)))
	calls := 0
	must(t, h.sortArrayUser(&a, false, func(x, y value) (value, error) {
		calls++
		if calls == 2 {
			must(t, h.sortArrayUser(&inner, false, func(x, y value) (value, error) { n, e := h.compare(x, y); return h.number(int64(n)), e }))
		}
		return boolValue(x.integer > y.integer), nil
	}))
	if len(h.diagnostics) != 2 {
		t.Fatalf("deprecations=%d, want 2", len(h.diagnostics))
	}
	must(t, h.unset(&a))
	must(t, h.unset(&inner))
}

// REQ-017; T-034: after cancellation/closed-heap failure, finish only the
// internal schedule, never reenter host code or discard the original error.
func TestW12SortStopsHostCalls(t *testing.T) {
	for _, closeHeap := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "close"}[closeHeap], func(t *testing.T) {
			h := testHeap(t)
			var a value
			for _, i := range []int64{5, 3, 8, 1, 9, 2} {
				must(t, h.appendValue(&a, h.number(i)))
			}
			calls := 0
			err := h.sortArray(&a, false, func(x, y value) (int, error) {
				calls++
				if closeHeap {
					return 0, h.shutdown(&a)
				}
				return 0, context.Canceled
			})
			want := context.Canceled
			if closeHeap {
				want = ErrHeapClosed
			}
			if !errors.Is(err, want) || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if !closeHeap {
				must(t, h.unset(&a))
			}
		})
	}
}

// REQ-017; T-033/T-034: PHP's array user-comparison adapter retries false
// with swapped arguments, once, and emits a request-local deprecation.
func TestW12SortUserBooleanAdapter(t *testing.T) {
	h := testHeap(t)
	var a value
	for _, i := range []int64{1, 3, 2} {
		must(t, h.appendValue(&a, h.number(i)))
	}
	calls := [][2]int64{}
	must(t, h.sortArrayUser(&a, false, func(x, y value) (value, error) {
		calls = append(calls, [2]int64{x.integer, y.integer})
		return boolValue(x.integer > y.integer), nil
	}))
	want := [][2]int64{{1, 3}, {3, 1}, {3, 2}, {1, 2}, {2, 1}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v, PHP %v", calls, want)
	}
	if len(h.diagnostics) != 1 || h.diagnostics[0].level != levelDeprecated || h.diagnostics[0].message != "usort(): Returning bool from comparison function is deprecated, return an integer less than, equal to, or greater than zero" {
		t.Fatalf("diagnostics %+v", h.diagnostics)
	}
	for i, b := range a.deref().array.buckets {
		if b.val.deref().integer != int64(i+1) {
			t.Fatalf("result %+v", a.deref())
		}
	}
	must(t, h.unset(&a))
}

func TestW12SortUserReverseFailure(t *testing.T) {
	h := testHeap(t)
	var a value
	must(t, h.appendValue(&a, h.number(1)))
	must(t, h.appendValue(&a, h.number(2)))
	stop := errors.New("reverse comparison failed")
	calls := 0
	err := h.sortArrayUser(&a, true, func(x, y value) (value, error) {
		calls++
		if calls == 2 {
			return value{}, stop
		}
		return boolValue(false), nil
	})
	if !errors.Is(err, stop) || calls != 2 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	if a.deref().array.buckets[0].val.integer != 1 || a.deref().array.buckets[1].val.integer != 2 {
		t.Fatal("reverse failure lost stable tie")
	}
	must(t, h.unset(&a))
}
