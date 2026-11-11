package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// REQ-019/020; T-037/038/039/040. Explicit cleanup uses its supplied
// context while ordinary operations remain subject to operation cancellation.
func TestW13ExplicitResourceCloseAfterCancellation(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, mode := range []string{"valid", "canceled-cleanup", "foreign", "kind", "generation", "closed", "heap-closed", "nil", "panic", "callback-error", "reentry"} {
			t.Run(fmt.Sprintf("%d/%s", width, mode), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				operation, cancelOperation := context.WithCancel(context.Background())
				defer cancelOperation()
				cleanup, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
				defer cancelCleanup()
				calls := 0
				var received context.Context
				var owner value
				var reentry error
				hostFailure := errors.New("host-close-failure")
				v, err := h.newResource(operation, "stream", func(ctx context.Context) error {
					calls++
					received = ctx
					if calls > 1 {
						return errors.New("duplicate close callback")
					}
					if mode == "reentry" {
						reentry = h.closeResource(owner, "stream", ctx)
					}
					if mode == "panic" {
						panic("close")
					}
					if mode == "callback-error" {
						return hostFailure
					}
					return ctx.Err()
				})
				must(t, err)
				must(t, h.assign(&owner, v))
				defer func() {
					h.closed = false
					if record, err := h.resourceRecord(owner); err == nil && !record.closed {
						_ = closeResourceRecord(record, context.Background())
					}
					must(t, h.unset(&owner))
					if len(h.resources) != 0 {
						t.Fatal("retained resource records", len(h.resources))
					}
				}()
				if mode == "closed" {
					must(t, h.closeResource(owner, "stream", cleanup))
				}
				cancelOperation()
				if !errors.Is(h.checkResource(owner, "stream"), context.Canceled) && mode != "closed" {
					t.Fatal("ordinary operation lost cancellation")
				}
				if mode == "canceled-cleanup" {
					cancelCleanup()
				}
				target, kind, closeContext := owner, "stream", context.Context(cleanup)
				receiver := h
				switch mode {
				case "foreign":
					receiver, err = referenceHeap(width)
					must(t, err)
				case "kind":
					kind = "database"
				case "generation":
					identity := *owner.object
					target.object = &identity
				case "nil":
					closeContext = nil
				case "heap-closed":
					h.closed = true
				}
				err = receiver.closeResource(target, kind, closeContext)
				record, recordErr := h.resourceRecord(owner)
				must(t, recordErr)
				rejected := mode == "foreign" || mode == "kind" || mode == "generation" || mode == "nil" || mode == "closed" || mode == "heap-closed"
				if rejected {
					wantCalls := 0
					if mode == "closed" {
						wantCalls = 1
					}
					if err == nil || calls != wantCalls || record.closed != (mode == "closed") {
						t.Fatalf("err=%v calls=%d closed=%t", err, calls, record.closed)
					}
					switch mode {
					case "foreign":
						if !strings.Contains(err.Error(), "does not belong to this request") {
							t.Fatal(err)
						}
					case "kind", "closed":
						if !strings.Contains(err.Error(), "expected an open ") {
							t.Fatal(err)
						}
					case "generation":
						if !strings.Contains(err.Error(), "generation is no longer valid") {
							t.Fatal(err)
						}
					case "nil":
						if err.Error() != "resource cleanup context is required" {
							t.Fatal(err)
						}
					case "heap-closed":
						if !errors.Is(err, ErrHeapClosed) {
							t.Fatal(err)
						}
					}
					return
				}
				if calls != 1 || received != cleanup || !record.closed || !owner.object.freed {
					t.Errorf("err=%v calls=%d supplied context retained=%t closed=%t freed=%t", err, calls, received == cleanup, record.closed, owner.object.freed)
				}
				switch mode {
				case "canceled-cleanup":
					if !errors.Is(err, context.Canceled) {
						t.Error(err)
					}
				case "panic":
					if err == nil || err.Error() != "resource close panicked: close" {
						t.Error(err)
					}
				case "callback-error":
					if !errors.Is(err, hostFailure) {
						t.Error(err)
					}
				default:
					if err != nil {
						t.Error(err)
					}
				}
				if mode == "reentry" && (reentry == nil || !strings.Contains(reentry.Error(), "expected an open stream resource")) {
					t.Errorf("reentry: %v", reentry)
				}
			})
		}
	}
}
