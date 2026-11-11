package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// REQ-020/REQ-040; T-039/T-040/T-079/T-080.
func TestExecutionCatchPreservesPendingAndFrame(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pending, handled, next := errors.New("pending"), errors.New("handled"), errors.New("next")
	destroyed := 0
	got := x.run(ctx, func(x *execution) completion {
		ambient := x.invoke(func(*execution) completion { return completion{kind: endThrow, err: pending} })
		depth := len(x.frames)
		c := x.tryCatch(func(x *execution) completion {
			v, err := x.object("TryLocal", func(*execution) completion { destroyed++; return completion{} })
			if err != nil {
				t.Fatal(err)
			}
			if err = x.local(&v); err != nil {
				t.Fatal(err)
			}
			return x.call(func(*execution) completion { return completion{kind: endThrow, err: handled} }, nil)
		}, func(c completion) executionAction {
			if !errors.Is(c, handled) {
				t.Fatalf("wrong caught error: %v", c)
			}
			return func(x *execution) completion {
				if len(x.frames) != depth || destroyed != 0 {
					t.Fatal("catch introduced a frame or released try locals")
				}
				return x.call(func(*execution) completion { return completion{kind: endThrow, err: next} }, nil)
			}
		})
		if !errors.Is(c, next) || !errors.Is(c.previous, pending) || errors.Is(c.previous, handled) || !containsCompletion(c.previous, ambient.serial) {
			t.Fatalf("wrong chain: %+v / %v", c, c.previous)
		}
		return completion{}
	})
	if got.end.kind != endNormal || destroyed != 1 || !got.cleanupComplete {
		t.Fatalf("result=%+v destroyed=%d", got, destroyed)
	}
}

func TestExecutionCatchOnlyHandlesMatchingThrow(t *testing.T) {
	for _, kind := range []endKind{endThrow, endExit, endFatal, endCancel, endInternal} {
		t.Run(string(rune('A'+kind)), func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			selected := false
			got := x.run(ctx, func(x *execution) completion {
				return x.tryCatch(func(x *execution) completion {
					if stopsUserCode(completion{kind: kind}) {
						return x.fail(kind, errors.New("stop"))
					}
					return completion{kind: kind, code: 7, err: errors.New("unmatched")}
				}, func(completion) executionAction { selected = true; return nil })
			})
			if selected != (kind == endThrow) || got.end.kind != kind {
				t.Fatalf("selected=%v result=%+v", selected, got)
			}
		})
	}
}

func TestExecutionCatchHonorsEntryStop(t *testing.T) {
	for _, kind := range []endKind{endCancel, endOutput, endLimit, endFatal} {
		t.Run(string(rune('A'+kind)), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			x, err := newExecution(ctx, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			if err != nil {
				t.Fatal(err)
			}
			if kind == endCancel {
				cancel()
			} else {
				x.fail(kind, errors.New("prior stop"))
			}
			body, selector, handler := false, false, false
			c := x.tryCatch(func(*execution) completion { body = true; return completion{kind: endThrow} }, func(completion) executionAction {
				selector = true
				return func(*execution) completion { handler = true; return completion{} }
			})
			if c.kind != kind || body || selector || handler {
				t.Fatalf("kind=%v body=%v selector=%v handler=%v", c.kind, body, selector, handler)
			}
		})
	}
}
