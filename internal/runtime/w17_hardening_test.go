package runtime

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// A step budget of MaxInt64 keeps the poll interval: limit+1 must not wrap
// and send every instruction down the slow path.
func TestVMStepBudgetDoesNotWrap(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.retNull()
	for _, steps := range []int64{0, 1, math.MaxInt64 - 1, math.MaxInt64} {
		cfg := vmConfig(t, 64)
		cfg.Steps = steps
		x, err := newExecution(cfg.Context, cfg.Request, 64, nil, executionLimits{depth: 1, callbacks: 1, output: 1})
		if err != nil {
			t.Fatal(err)
		}
		s := newVMState(a.program(), x, cfg)
		s.steps = min(vmPollInterval, s.limit)
		if s.slow() {
			continue // a budget of one ends at its first check
		}
		if want := min(s.limit+1, 2*vmPollInterval); s.next != want || s.next <= s.steps {
			t.Errorf("Steps %d: next check at %d, want %d", steps, s.next, want)
		}
	}
}

// Once the context has ended, a diagnostic is neither displayed nor
// recorded, and the writer is not called.
func TestReportStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	x, err := newExecution(ctx, readyRequest(t), 64, &out, executionLimits{depth: 4, callbacks: 16, output: 1 << 10})
	if err != nil {
		t.Fatal(err)
	}
	x.errors = errorChannels{display: true, log: true}
	x.diagnosticCap = 10
	cancel()
	if c := x.report("Warning", "late", false); c.kind != endCancel || out.Len() != 0 || len(x.diagnostics) != 0 {
		t.Fatalf("%v, wrote %q, recorded %d", c.kind, out.String(), len(x.diagnostics))
	}
}

// CleanupTimeout starts when teardown starts, so a run longer than the
// timeout still has the whole timeout for its teardown.
func TestVMCleanupTimeoutStartsAtTeardown(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "i")
	top := m.pc()
	m.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	m.jump(top)
	m.retNull()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	cfg := vmConfig(t, 64)
	cfg.Context = ctx
	cfg.Cleanup = context.WithoutCancel(ctx)
	cfg.CleanupTimeout = 100 * time.Millisecond
	res := a.program().Run(cfg)
	if res.End != Cancelled || res.CleanupErr != nil {
		t.Fatalf("%v %v cleanup %v", res.End, res.Err, res.CleanupErr)
	}
	// Without a deadline or a timeout, cleanup is refused as before.
	cfg = vmConfig(t, 64)
	cfg.Cleanup = context.Background()
	if res := a.program().Run(cfg); res.End != Internal || res.Err == nil || !strings.Contains(res.Err.Error(), "deadline") {
		t.Fatalf("%v %v", res.End, res.Err)
	}
}
