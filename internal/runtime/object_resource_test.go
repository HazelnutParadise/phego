package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// REQ-019/020; T-037/038/039/040. Resource liveness is separate from the
// Go allocation and checked against the owning heap and identity generation.
func TestW13ResourceOwnershipAndClose(t *testing.T) {
	h := testHeap(t)
	other := testHeap(t)
	closed := 0
	v, err := h.newResource(context.Background(), "stream", func(context.Context) error { closed++; return nil })
	must(t, err)
	var a, b value
	must(t, h.assign(&a, v))
	must(t, h.assign(&b, v))
	if err := other.checkResource(v, "stream"); err == nil {
		t.Fatal("foreign resource accepted")
	}
	if err := h.checkResource(v, "database"); err == nil {
		t.Fatal("wrong resource kind accepted")
	}
	must(t, h.unset(&a))
	if closed != 0 {
		t.Fatal("closed before last owner")
	}
	must(t, h.unset(&b))
	if closed != 1 {
		t.Fatal(closed)
	}
	if err := h.checkResource(v, "stream"); err == nil {
		t.Fatal("closed resource accepted")
	}
	if err := h.closeResource(v, "stream", context.Background()); err == nil {
		t.Fatal("duplicate PHP close accepted")
	}
	if closed != 1 {
		t.Fatal("closed twice")
	}
}

func TestW13ResourceExecutionPaths(t *testing.T) {
	for _, end := range []endKind{endNormal, endThrow, endExit, endFatal, endCancel} {
		t.Run(string(rune('a'+end)), func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 32, 1024})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			closed := 0
			result := x.run(ctx, func(x *execution) completion {
				v, err := x.heap.newResource(x.ctx, "stream", func(context.Context) error { closed++; return nil })
				if err != nil {
					t.Fatal(err)
				}
				var slot value
				must(t, x.local(&slot))
				must(t, x.heap.assign(&slot, v))
				return completion{kind: end}
			})
			if closed != 1 || !result.cleanupComplete {
				t.Fatal(end, closed, result)
			}
		})
	}
}

func w13RunResourceCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	v, err := h.newResource(context.Background(), "stream", func(context.Context) error { events = append(events, "close"); return nil })
	must(t, err)
	var a value
	must(t, h.assign(&a, v))
	var result any
	switch name {
	case "resource-property-lifetime":
		o, err := h.newObject(&class{name: "stdClass"})
		must(t, err)
		var owner value
		must(t, h.assign(&owner, o))
		must(t, h.setProperty(o, "stream", v))
		must(t, h.unset(&a))
		must(t, h.unset(&owner))
		result = events
	case "resource-last-owner":
		var b value
		must(t, h.assign(&b, a))
		must(t, h.unset(&a))
		before := append([]any{}, events...)
		must(t, h.unset(&b))
		result = []any{before, events}
	case "resource-closed-kind":
		open, truth := h.checkResource(a, "stream") == nil, h.toBool(a)
		must(t, h.closeResource(a, "stream", context.Background()))
		result = []any{open, truth, typeName(a), h.checkResource(a, "stream") == nil, h.toBool(a), events}
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13ResourceTruth(t *testing.T) {
	h := testHeap(t)
	v, err := h.newResource(context.Background(), "stream", func(context.Context) error { return nil })
	must(t, err)
	if !h.toBool(v) {
		t.Fatal("open PHP resource must be true")
	}
	must(t, h.closeResource(v, "stream", context.Background()))
	if !h.toBool(v) {
		t.Fatal("closed PHP resource must remain true")
	}
}

func TestW13ResourcePropertyLastOwner(t *testing.T) {
	h := testHeap(t)
	closed := 0
	r, err := h.newResource(context.Background(), "stream", func(context.Context) error { closed++; return nil })
	must(t, err)
	o, err := h.newObject(&class{name: "stdClass"})
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	must(t, h.setProperty(o, "stream", r))
	must(t, h.unset(&owner))
	if closed != 1 {
		t.Fatal("property resource survived its last owner", closed)
	}
}

func TestW13ResourceCleanupReentry(t *testing.T) {
	h := testHeap(t)
	var second value
	calls := 0
	_, err := h.newResource(context.Background(), "first", func(context.Context) error { calls++; return h.unset(&second) })
	must(t, err)
	v, err := h.newResource(context.Background(), "second", func(context.Context) error { calls++; return nil })
	must(t, err)
	must(t, h.assign(&second, v))
	must(t, h.closeResources(context.Background()))
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestW13ResourceOperationCancellationCleanup(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "resource-context", true: "request-context"}[cancelRequest], func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 32, 1024})
			operation, cancelOperation := context.WithCancel(context.Background())
			defer cancelOperation()
			if cancelRequest {
				x.ctx = operation
			}
			cleanup, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
			defer cancelCleanup()
			attempts, closed := 0, 0
			var releaseErr error
			result := x.run(cleanup, func(x *execution) completion {
				v, err := x.heap.newResource(operation, "stream", func(ctx context.Context) error {
					attempts++
					if err := ctx.Err(); err != nil {
						return err
					}
					deadline, ok := ctx.Deadline()
					want, _ := cleanup.Deadline()
					if !ok || deadline != want {
						return errors.New("lost cleanup deadline")
					}
					closed++
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				var owner value
				must(t, x.local(&owner))
				must(t, x.heap.assign(&owner, v))
				cancelOperation()
				releaseErr = x.heap.unset(&owner)
				if releaseErr != nil {
					return x.fail(endInternal, releaseErr)
				}
				return completion{}
			})
			if releaseErr != nil || attempts != 1 || closed != 1 || !result.cleanupComplete {
				t.Fatal(releaseErr, attempts, closed, result)
			}
			want := endNormal
			if cancelRequest {
				want = endCancel
			}
			if result.end.kind != want {
				t.Fatal(result.end)
			}
		})
	}
}

func TestW13ResourceCleanupDeadlinePreserved(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 32, 1024})
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attempts := 0
	var releaseErr error
	x.run(cleanup, func(x *execution) completion {
		v, err := x.heap.newResource(x.ctx, "stream", func(ctx context.Context) error { attempts++; return ctx.Err() })
		must(t, err)
		var owner value
		must(t, x.local(&owner))
		must(t, x.heap.assign(&owner, v))
		cancel()
		releaseErr = x.heap.unset(&owner)
		return completion{}
	})
	if !errors.Is(releaseErr, context.Canceled) || attempts != 1 {
		t.Fatal("cleanup cancellation changed or retried", releaseErr, attempts)
	}
}
