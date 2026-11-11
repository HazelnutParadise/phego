package runtime

import (
	"context"
	"errors"
	"testing"
)

// REQ-020/021; T-040/042. A sticky host cancellation cannot be caught by
// PHP or converted into an ordinary successful collection.
func TestW13GCHostCancellation(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithCancel(context.Background())
	x.ctx = ctx
	o, err := x.heap.newObject(&class{name: "Cancel"})
	must(t, err)
	var owner value
	must(t, x.heap.assign(&owner, o))
	must(t, x.heap.setProperty(o, "self", o))
	must(t, x.heap.unset(&owner))
	cancel()
	count, err := x.heap.collectCycles()
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("count=%d error=%v", count, err)
	}
}

func TestW13LargeGraphCancellation(t *testing.T) {
	h := testHeap(t)
	var owners []value
	for range 4096 {
		o, err := h.newObject(&class{name: "Node"})
		must(t, err)
		retain(o)
		owners = append(owners, o)
	}
	for i, o := range owners {
		must(t, h.setProperty(o, "next", owners[(i+1)%len(owners)]))
	}
	root := owners[0].object
	for _, o := range owners {
		must(t, h.release(o))
	}
	checks := 0
	_, err := garbageChecked([]gcNode{root}, func() error {
		checks++
		if checks == 3 {
			return context.Canceled
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || checks != 3 {
		t.Fatal(checks, err)
	}
	if h.liveObjects != len(owners) {
		t.Fatal("cancelled tracing freed partial graph")
	}
	n, err := h.collectCycles()
	must(t, err)
	if n != len(owners) || h.liveObjects != 0 {
		t.Fatal(n, h.liveObjects)
	}
}
