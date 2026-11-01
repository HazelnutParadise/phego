package runtime

import (
	"errors"
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
	err = h.sortArray(&a, true, func(x, y value) (int, error) {
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
