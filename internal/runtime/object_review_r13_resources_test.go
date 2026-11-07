package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type w13R13ClosePanic func() string

func (p w13R13ClosePanic) String() string { return p() }

// These host-side cases exercise resource ownership while a trusted close
// callback reenters the same heap. They do not represent PHP acceptance cases.
func TestW13R13ResourceCloseReentry(t *testing.T) {
	type closeMode string
	const (
		lastRelease   closeMode = "last-release"
		explicitOwned closeMode = "explicit-owned"
		explicitTemp  closeMode = "explicit-temporary"
	)
	type ownerAction string
	const (
		keepAlias     ownerAction = "keep-alias"
		dropReacquire ownerAction = "drop-reacquire"
		dropOnly      ownerAction = "drop-only"
	)
	for _, width := range []int{32, 64} {
		for _, mode := range []closeMode{lastRelease, explicitOwned, explicitTemp} {
			for _, action := range []ownerAction{keepAlias, dropReacquire, dropOnly} {
				for _, outcome := range []string{"success", "error", "panic", "panic-reentry"} {
					t.Run(fmt.Sprintf("%d/%s/%s/%s", width, mode, action, outcome), func(t *testing.T) {
						h, err := referenceHeap(width)
						must(t, err)
						ctx := context.Background()
						hostFailure := errors.New("host close failure")
						var original, alias value
						var resource value
						calls := 0
						formatCalls := 0
						var callbackAssignErr, callbackUnsetErr, recursiveErr error
						callbackWasClosed := false
						resource, err = h.newResource(ctx, "stream", func(context.Context) error {
							calls++
							record, recordErr := h.resourceRecord(resource)
							callbackWasClosed = recordErr == nil && record.closed && resource.object.freed
							recursiveErr = h.closeResource(resource, "stream", ctx)
							if mode == explicitOwned {
								callbackUnsetErr = h.unset(&original)
							}
							if action == keepAlias || action == dropReacquire {
								callbackAssignErr = h.assign(&alias, resource)
							}
							if action == dropReacquire {
								callbackUnsetErr = errors.Join(callbackUnsetErr, h.unset(&alias))
								callbackAssignErr = errors.Join(callbackAssignErr, h.assign(&alias, resource))
							}
							if action == dropOnly && mode != explicitOwned {
								callbackAssignErr = h.assign(&alias, resource)
								callbackUnsetErr = h.unset(&alias)
							}
							switch outcome {
							case "error":
								return hostFailure
							case "panic":
								panic("close sentinel")
							case "panic-reentry":
								panic(w13R13ClosePanic(func() string {
									formatCalls++
									callbackAssignErr = errors.Join(callbackAssignErr, h.assign(&alias, resource))
									callbackUnsetErr = errors.Join(callbackUnsetErr, h.unset(&alias))
									callbackAssignErr = errors.Join(callbackAssignErr, h.assign(&alias, resource))
									return "close sentinel"
								}))
							default:
								return nil
							}
						})
						must(t, err)
						if mode != explicitTemp {
							must(t, h.assign(&original, resource))
						}
						var closeErr error
						if mode == lastRelease {
							closeErr = h.unset(&original)
						} else {
							closeErr = h.closeResource(resource, "stream", ctx)
						}
						if calls != 1 || !callbackWasClosed {
							t.Errorf("callback calls=%d closed-at-entry=%t", calls, callbackWasClosed)
						}
						if recursiveErr == nil || !strings.Contains(recursiveErr.Error(), "expected an open stream resource") {
							t.Errorf("recursive close: %v", recursiveErr)
						}
						if callbackAssignErr != nil || callbackUnsetErr != nil {
							t.Errorf("callback assign=%v unset=%v", callbackAssignErr, callbackUnsetErr)
						}
						if outcome == "panic-reentry" && formatCalls != 1 {
							t.Errorf("panic formatting calls=%d", formatCalls)
						}
						switch outcome {
						case "error":
							if !errors.Is(closeErr, hostFailure) {
								t.Errorf("close error=%v", closeErr)
							}
						case "panic", "panic-reentry":
							if closeErr == nil || !strings.Contains(closeErr.Error(), "resource close panicked: close sentinel") {
								t.Errorf("close panic=%v", closeErr)
							}
						default:
							if closeErr != nil {
								t.Errorf("close result=%v", closeErr)
							}
						}

						postOwners := alias.kind != valueUndefined
						if record, recordErr := h.resourceRecord(resource); postOwners {
							if recordErr != nil || !record.closed {
								t.Errorf("owned closed record=%v, %v", record, recordErr)
							}
							if h.checkResource(resource, "stream") == nil || typeName(resource) != "resource (closed)" || !h.toBool(resource) {
								t.Error("closed resource operations/truth/type mismatch")
							}
						} else if mode == explicitTemp {
							if recordErr != nil || !record.closed {
								t.Errorf("raw temporary record should remain until discard: %v", recordErr)
							}
						} else if recordErr == nil {
							t.Errorf("ownerless closed record retained: %v", record)
						}
						if postOwners {
							var closedCopy value
							if err := h.assign(&closedCopy, resource); err != nil {
								t.Errorf("same-heap closed resource assignment: %v", err)
							} else {
								must(t, h.unset(&closedCopy))
							}
						}
						if mode == explicitTemp {
							must(t, h.discard(resource))
						}
						if alias.kind != valueUndefined {
							must(t, h.unset(&alias))
						}
						if original.kind != valueUndefined {
							must(t, h.unset(&original))
						}
						if len(h.resources) != 0 || calls != 1 {
							t.Errorf("cleanup resources=%d callback calls=%d", len(h.resources), calls)
						}
						if _, err := h.resourceRecord(resource); err == nil || !strings.Contains(err.Error(), "generation is no longer valid") {
							t.Errorf("released resource generation: %v", err)
						}
						if err := h.shutdown(&value{}); err != nil {
							t.Errorf("shutdown: %v", err)
						}
					})
				}
			}
		}
	}
}
