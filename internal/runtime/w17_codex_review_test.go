package runtime

import (
	"context"
	"testing"
)

// Separating a large shared array checks for a host stop as it copies: a
// cancelled run does not finish the copy, the source is untouched and the
// partial copy is released.
func TestSeparateStopsCopyingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	x, err := newExecution(ctx, readyRequest(t), 64, nil, executionLimits{depth: 4, callbacks: 16, output: 1 << 10})
	if err != nil {
		t.Fatal(err)
	}
	h := x.heap
	slot := h.newArrayValue()
	const n = 3 * copyCheckInterval
	for i := 0; i < n; i++ {
		if err := h.appendValue(&slot, h.number(int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	shared := slot
	retain(shared) // a second holder, so a write must copy
	dup, err := h.duplicateChecked(slot.array)
	if err != nil || dup.count() != n {
		t.Fatalf("uncancelled copy: %v", err)
	}
	if err := h.discard(value{kind: valueArray, array: dup}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := h.duplicateChecked(slot.array); err == nil {
		t.Fatal("the copy finished after the context ended")
	}
	if slot.array.count() != n || slot.array.refcount != 2 {
		t.Fatalf("source changed: %d elements, %d holders", slot.array.count(), slot.array.refcount)
	}
}
