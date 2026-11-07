package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// REQ-020, REQ-022, REQ-040; T-039, T-040, T-043, T-044, T-079, T-080.
func TestExecutionReturnSnapshotSurvivesShutdown(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{8, 16, 128})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got := x.run(ctx, func(x *execution) completion {
		v := x.heap.newArrayValue()
		if err := x.local(&v); err != nil {
			t.Fatal(err)
		}
		if err := x.heap.setElement(&v, bytesValue([]byte("key")), bytesValue([]byte("value"))); err != nil {
			t.Fatal(err)
		}
		return completion{kind: endReturn, value: v}
	})
	entries, ok := got.returned.([]SnapshotEntry)
	if !ok || len(entries) != 1 || entries[0].Key != "key" || string(entries[0].Value.([]byte)) != "value" || !x.heap.closed {
		t.Fatalf("invalid snapshot: %+v", got)
	}
	if got.end.value.kind != valueUndefined {
		t.Fatal("live PHP value escaped through completion")
	}
}

// REQ-009, REQ-020; T-017, T-018, T-039, T-040.
func TestExecutionFinallyCountsAgainstCallbackBudget(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{8, 2, 128})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	called := false
	got := x.run(ctx, func(x *execution) completion {
		return x.call(func(*execution) completion { return completion{} }, func(*execution) completion { called = true; return completion{} })
	})
	if got.end.kind != endLimit || !errors.Is(got.end.err, errExecutionLimit) || called {
		t.Fatalf("finally bypassed callback budget: %+v called=%v", got, called)
	}
}

// REQ-040; T-079, T-080. Expansion at a flush boundary must not bypass output limits.
func TestExecutionOutputFilterExpansionIsBounded(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{8, 16, 3})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got := x.run(ctx, func(x *execution) completion {
		if err := x.startBuffer("{closure:test}", func(_ *execution, b []byte) ([]byte, completion) { return append(b, b...), completion{} }); err != nil {
			t.Fatal(err)
		}
		return x.write([]byte("ab"))
	})
	if got.end.kind != endLimit || got.accepted != 0 || len(got.output) != 0 || !got.cleanupComplete {
		t.Fatalf("filter exceeded output limit: %+v", got)
	}
}
