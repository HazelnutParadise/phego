package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// REQ-020/REQ-040; T-039/T-040/T-079/T-080.
func TestExecutionFinallyPreservesUnsuppressedThrow(t *testing.T) {
	for _, throws := range []bool{false, true} {
		x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		first, second := errors.New("body"), errors.New("finally")
		got := x.run(ctx, func(x *execution) completion {
			return x.call(func(*execution) completion { return completion{kind: endThrow, err: first} }, func(*execution) completion {
				if throws {
					return completion{kind: endThrow, err: second}
				}
				return completion{}
			})
		})
		cancel()
		if got.end.kind != endThrow {
			t.Fatalf("wrong completion: %+v", got.end)
		}
		if throws {
			if !errors.Is(got.end.err, second) || !errors.Is(got.end.previous, first) {
				t.Fatalf("lost previous: %+v", got.end)
			}
		} else if !errors.Is(got.end.err, first) || got.end.previous != nil {
			t.Fatalf("normal finally suppressed throw: %+v", got.end)
		}
	}
}
