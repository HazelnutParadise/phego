package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
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

// A large concatenation stops when the run has been cancelled, whatever
// the operand types are, and gives the same bytes as heap.concat otherwise.
func TestVMConcatChecksCancellationForEveryOperandType(t *testing.T) {
	a := newAsm(t, 64)
	a.fn("{main}", 0, 2).retNull()
	big := stringValue(strings.Repeat("x", 2*vmCopyChunk))
	for name, pair := range map[string][2]value{
		"string . string": {big, big},
		"string . int":    {big, intOf(t, 1)},
		"int . string":    {intOf(t, 1), big},
		"string . null":   {big, nullValue()},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		cfg := vmConfig(t, 64)
		cfg.Context = ctx
		x, err := newExecution(ctx, cfg.Request, 64, nil, executionLimits{depth: 4, callbacks: 16, output: 1 << 10})
		if err != nil {
			t.Fatal(err)
		}
		s := newVMState(a.program(), x, cfg)
		got, err := s.binary(bytecode.Concat, pair[0], pair[1])
		want, werr := x.heap.concat(pair[0], pair[1])
		if err != nil || werr != nil || got.kind != valueString || got.text != want.text || s.stopped() {
			t.Fatalf("%s: uncancelled join differs or stopped: %v %v", name, err, werr)
		}
		cancel()
		got, err = s.binary(bytecode.Concat, pair[0], pair[1])
		if err != nil || !s.stopped() || got.kind == valueString {
			t.Errorf("%s: the copy was made after the context ended (stopped %v, kind %v, err %v)", name, s.stopped(), got.kind, err)
		}
	}
}

func intOf(t *testing.T, i int64) value {
	t.Helper()
	v, err := intValue(i, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
