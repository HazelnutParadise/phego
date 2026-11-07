package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// REQ-016/017; T-031/032/034: interruption releases every loop hold and
// unregisters positions in all copies, without undoing completed writes.
func TestW12IterationErrorCleanup(t *testing.T) {
	for _, byRef := range []bool{false, true} {
		t.Run(map[bool]string{true: "reference", false: "value"}[byRef], func(t *testing.T) {
			h := testHeap(t)
			var a, b, c, item value
			must(t, h.appendValue(&a, num(t, 1)))
			must(t, h.appendValue(&a, num(t, 2)))
			stop := errors.New("interrupted")
			body := func(arrayKey) error {
				must(t, h.assign(&b, a))
				must(t, h.appendValue(&a, num(t, 3)))
				must(t, h.assign(&c, a))
				must(t, h.setElement(&a, str("tail"), num(t, 4)))
				return stop
			}
			var err error
			if byRef {
				err = h.foreachRef(&a, &item, body)
			} else {
				err = h.foreachValue(a, &item, body)
			}
			if !errors.Is(err, stop) {
				t.Fatalf("error %v", err)
			}
			for _, v := range []value{a, b, c} {
				if len(v.deref().array.cursors) != 0 {
					t.Fatal("iterator retained after error")
				}
			}
			if a.deref().array.count() != 4 {
				t.Fatal("completed body effects were lost")
			}
			must(t, h.unset(&item))
			must(t, h.unset(&a))
			must(t, h.unset(&b))
			must(t, h.unset(&c))
		})
	}
}

// REQ-007; T-013/014: a snapshot owns its bytes and order independently of
// caller mutation after import and request cleanup.
func TestW12SnapshotOwnershipAfterCleanup(t *testing.T) {
	h := testHeap(t)
	bytes := []byte{'a', 0, 'b'}
	input := []SnapshotEntry{{Key: "x", Value: bytes}, {Key: int64(0), Value: int64(7)}}
	v, err := h.importSnapshotPHP8512(input, 4, 16)
	must(t, err)
	out, err := exportSnapshot(v)
	must(t, err)
	done := make(chan struct{})
	go func() {
		for i := range bytes {
			bytes[i] = 'z'
		}
		input[0].Key = "changed"
		close(done)
	}()
	must(t, h.release(v))
	globals := value{}
	must(t, h.shutdown(&globals))
	<-done
	want := []SnapshotEntry{{Key: "x", Value: []byte{'a', 0, 'b'}}, {Key: int64(0), Value: int64(7)}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("snapshot %#v", out)
	}
}

// REQ-007/016/017; T-013/031/033: layout is Go metadata, not unsafe access.
func TestW12Layout(t *testing.T) {
	for _, v := range []any{value{}, reference{}, arrayKey{}, bucket{}, phpArray{}, arrayIterator{}} {
		t.Logf("%s: %d bytes", reflect.TypeOf(v).Name(), reflect.TypeOf(v).Size())
	}
}

// REQ-016; T-031/032/034: comparison arguments stay alive even when a
// shared element is replaced through another alias during the callback.
func TestW12SortOwnsCallbackArguments(t *testing.T) {
	h := testHeap(t)
	var a, external value
	o, err := h.newObject(&class{name: "C"})
	must(t, err)
	must(t, h.assign(&external, o))
	p, err := h.element(&a, num(t, 0))
	must(t, err)
	must(t, h.bind(p, &external))
	must(t, h.appendValue(&a, num(t, 1)))
	err = h.sortArray(&a, "uasort", true, func(x, y value) (int, error) {
		must(t, h.assign(&external, nullValue()))
		for _, v := range []value{x, y} {
			if v.kind == valueObject && v.object.freed {
				return 0, errors.New("comparison argument was destroyed during callback")
			}
		}
		return 0, nil
	})
	must(t, err)
	if !o.object.freed {
		t.Fatal("temporary comparator ownership leaked")
	}
}

// REQ-017; T-034: a logical position limit cannot wrap into a negative
// allocation length. Extreme integer keys use ordered storage, without gaps.
func TestW12PositionLimitBounds(t *testing.T) {
	a := newArray()
	a.limit = int(^uint(0) >> 1)
	before := a.limit
	if a.growLimit() || a.limit != before {
		t.Fatal("logical limit overflowed")
	}
	h := testHeap(t)
	var v value
	must(t, h.setElement(&v, num(t, 9223372036854775807), num(t, 1)))
	if v.array.packed() || len(v.array.buckets) != 1 {
		t.Fatal("extreme key allocated its gaps")
	}
}

// REQ-016; T-031/032/034. A direct previous object remains visible during
// the destructor caused by advancing foreach. Reentrant writes must not
// release an already-consumed owner or modify the newly selected element.
func TestW12ForeachObjectItemReleaseOrder(t *testing.T) {
	for _, rewrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "observe", true: "rewrite"}[rewrite], func(t *testing.T) {
			h := testHeap(t)
			var arr, item value
			seen := ""
			o, err := h.newObject(&class{name: "C", destructor: func(h *heap, _ value) error {
				seen = typeName(item)
				if rewrite {
					return h.assign(&item, num(t, 99))
				}
				return nil
			}})
			must(t, err)
			must(t, h.appendValue(&arr, o))
			must(t, h.appendValue(&arr, num(t, 42)))
			must(t, h.foreachRef(&arr, &item, func(arrayKey) error {
				if item.deref().kind == valueObject {
					return h.unsetElement(&arr, num(t, 0))
				}
				return nil
			}))
			if seen != "C" || item.deref().integer != 42 || o.object.refcount != 0 || !o.object.freed {
				t.Fatalf("seen %q item %+v object count=%d freed=%v", seen, item.deref(), o.object.refcount, o.object.freed)
			}
			must(t, h.unset(&arr))
			must(t, h.unset(&item))
		})
	}
}

// REQ-016; T-031/032/034. These are Go ownership/safety checks. Reentrant
// access to a reference being destroyed can corrupt the pinned PHP heap;
// those reference-process failures are recorded separately, never as passes.
func TestW12ForeachDirectObjectSafety(t *testing.T) {
	for _, referenceItem := range []bool{false, true} {
		for _, action := range []string{"throw", "unset", "alias", "delete-next", "resurrect"} {
			t.Run(fmt.Sprintf("reference=%t/%s", referenceItem, action), func(t *testing.T) {
				h := testHeap(t)
				var arr, item, alias, saved value
				calls, bodies := 0, 0
				stop := errors.New("stop")
				old, err := h.newObject(&class{name: "C", destructor: func(h *heap, this value) error {
					calls++
					switch action {
					case "throw":
						return stop
					case "unset":
						return h.unset(&item)
					case "alias":
						return h.bind(&alias, &item)
					case "delete-next":
						return h.unsetElement(&arr, num(t, 0))
					case "resurrect":
						return h.assign(&saved, this)
					}
					return nil
				}})
				must(t, err)
				must(t, h.assign(&item, old))
				if referenceItem {
					var temporary value
					must(t, h.bind(&temporary, &item))
					must(t, h.unset(&temporary))
				}
				must(t, h.appendValue(&arr, num(t, 42)))
				err = h.foreachRef(&arr, &item, func(arrayKey) error { bodies++; return nil })
				if action == "throw" {
					if !errors.Is(err, stop) || bodies != 0 {
						t.Fatalf("error=%v bodies=%d", err, bodies)
					}
				} else if err != nil || bodies != 1 {
					t.Fatalf("error=%v bodies=%d", err, bodies)
				}
				if item.deref().kind != valueInt || item.deref().integer != 42 || calls != 1 || len(arr.deref().array.cursors) != 0 {
					t.Fatalf("item=%+v calls=%d", item.deref(), calls)
				}
				retained := action == "alias" || action == "resurrect"
				if retained {
					if old.object.refcount != 1 || old.object.freed {
						t.Fatal("resurrected object lost its owner")
					}
				} else if old.object.refcount != 0 || !old.object.freed {
					t.Fatal("previous object ownership leaked")
				}
				for _, slot := range []*value{&arr, &item, &alias, &saved} {
					must(t, h.unset(slot))
				}
				if old.object.refcount != 0 || !old.object.freed || calls != 1 {
					t.Fatal("cleanup leaked or repeated destruction")
				}
			})
		}
	}
}

func BenchmarkW12OrderedLookup1K(b *testing.B) {
	h := benchHeap(b)
	var a value
	for i := int64(0); i < 1024; i++ {
		if err := h.setElement(&a, h.number(i*2), h.number(i)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		v, ok, err := h.getElement(a, h.number(1022))
		if err != nil || !ok || v.integer != 511 {
			b.Fatal("lookup", v, ok, err)
		}
	}
}
