package runtime

import (
	"context"
	"testing"
	"time"
)

// REQ-020, REQ-022, REQ-040; T-039, T-040, T-043, T-044, T-079, T-080.
func TestExecutionLocalSlotReassignmentAndRegistration(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	oldCount, newCount := 0, 0
	var slot value
	got := x.run(ctx, func(x *execution) completion {
		var err error
		slot, err = x.object("Old", func(*execution) completion { oldCount++; return completion{} })
		if err != nil {
			t.Fatal(err)
		}
		if err = x.local(&slot); err != nil {
			t.Fatal(err)
		}
		if err = x.local(nil); err == nil {
			t.Fatal("nil slot accepted")
		}
		if err = x.local(&slot); err == nil {
			t.Fatal("duplicate slot accepted")
		}
		c := x.call(func(x *execution) completion {
			if err := x.local(&slot); err == nil {
				t.Fatal("outer slot registered twice")
			}
			return completion{}
		}, nil)
		if c.kind != endNormal {
			return c
		}
		if slot.object.refcount != 1 {
			t.Fatalf("duplicate registration changed ownership: %d", slot.object.refcount)
		}
		if err = x.heap.unset(&slot); err != nil {
			t.Fatal(err)
		}
		replacement, err := x.object("New", func(*execution) completion { newCount++; return completion{} })
		if err != nil {
			t.Fatal(err)
		}
		if err = x.heap.assign(&slot, replacement); err != nil {
			t.Fatal(err)
		}
		return completion{}
	})
	if got.end.kind != endNormal || !got.cleanupComplete || oldCount != 1 || newCount != 1 || slot.kind != valueUndefined {
		t.Fatalf("result=%+v old=%d new=%d slot=%v", got, oldCount, newCount, slot.kind)
	}
}

func TestExecutionLocalReferenceSlotsPreserveGlobalOwner(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got := x.run(ctx, func(x *execution) completion {
		if err := x.heap.assign(&x.globals, x.heap.newArrayValue()); err != nil {
			t.Fatal(err)
		}
		c := x.call(func(x *execution) completion {
			first, second := nullValue(), nullValue()
			if err := x.local(&first); err != nil {
				t.Fatal(err)
			}
			if err := x.local(&second); err != nil {
				t.Fatal(err)
			}
			if err := x.heap.bind(&first, &x.globals); err != nil {
				t.Fatal(err)
			}
			if err := x.heap.bind(&second, &first); err != nil {
				t.Fatal(err)
			}
			if err := x.heap.setElement(&second, bytesValue([]byte("key")), bytesValue([]byte("kept"))); err != nil {
				t.Fatal(err)
			}
			if err := x.heap.unset(&first); err != nil {
				t.Fatal(err)
			}
			return completion{}
		}, nil)
		if c.kind != endNormal {
			return c
		}
		if x.globals.kind != valueReference || x.globals.ref.refcount != 1 {
			t.Fatalf("global ownership lost: %+v", x.globals)
		}
		snapshot, err := exportSnapshot(x.globals)
		if err != nil {
			t.Fatal(err)
		}
		entries := snapshot.([]SnapshotEntry)
		if len(entries) != 1 || string(entries[0].Value.([]byte)) != "kept" {
			t.Fatalf("global changed: %#v", snapshot)
		}
		return completion{}
	})
	if got.end.kind != endNormal || !got.cleanupComplete {
		t.Fatalf("result=%+v", got)
	}
}
