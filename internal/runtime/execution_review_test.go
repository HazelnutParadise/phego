package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// REQ-020, REQ-022; T-039, T-040, T-043, T-044.
// PR #74: a pending return must survive removal of its last variable in finally.
func TestExecutionReviewReturnSurvivesFinally(t *testing.T) {
	for _, kind := range []string{"array", "object"} {
		t.Run(kind, func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			destroyed := 0
			got := x.run(ctx, func(x *execution) completion {
				c := x.call(func(x *execution) completion {
					v := x.heap.newArrayValue()
					if kind == "object" {
						var err error
						v, err = x.object("Returned", func(*execution) completion { destroyed++; return completion{} })
						if err != nil {
							t.Fatal(err)
						}
					}
					if err := x.heap.assign(&x.globals, v); err != nil {
						t.Fatal(err)
					}
					if kind == "array" {
						if err := x.heap.setElement(&x.globals, bytesValue([]byte("key")), bytesValue([]byte("kept"))); err != nil {
							t.Fatal(err)
						}
					}
					return completion{kind: endReturn, value: x.globals}
				}, func(x *execution) completion {
					if err := x.heap.unset(&x.globals); err != nil {
						return completion{kind: endInternal, err: err}
					}
					return completion{}
				})
				if c.kind != endReturn {
					t.Errorf("return replaced: %+v", c)
					return c
				}
				if kind == "array" {
					snap, err := exportSnapshot(c.value)
					if err != nil {
						t.Fatal(err)
					}
					want := []SnapshotEntry{{Key: "key", Value: []byte("kept")}}
					if !reflect.DeepEqual(snap, want) {
						t.Errorf("pending array lost: got %#v, want %#v", snap, want)
					}
				} else if destroyed != 0 || c.value.object.freed {
					t.Errorf("return object destroyed in finally: count=%d freed=%v", destroyed, c.value.object.freed)
				}
				if err := x.heap.release(c.value); err != nil {
					return completion{kind: endInternal, err: err}
				}
				return completion{}
			})
			if got.end.kind != endNormal || !got.cleanupComplete {
				t.Fatalf("run: %+v", got)
			}
			if kind == "object" && destroyed != 1 {
				t.Errorf("returned object destruction count=%d", destroyed)
			}
		})
	}
}

// REQ-020; T-039, T-040. Dropping a replaced return must occur before caller catch.
func TestExecutionReviewUnwindReleasesReplacedReturn(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var events []string
	failure := errors.New("local failure")
	got := x.run(ctx, func(x *execution) completion {
		c := x.call(func(x *execution) completion {
			returned, err := x.object("Returned", func(*execution) completion { events = append(events, "returned"); return completion{} })
			if err != nil {
				t.Fatal(err)
			}
			if err := x.local(returned); err != nil {
				t.Fatal(err)
			}
			local, err := x.object("Broken", func(*execution) completion {
				events = append(events, "local")
				return completion{kind: endThrow, err: failure}
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := x.local(local); err != nil {
				t.Fatal(err)
			}
			return completion{kind: endReturn, value: returned}
		}, nil)
		if c.kind != endThrow || !errors.Is(c.err, failure) {
			t.Errorf("wrong unwind result: %+v", c)
		}
		events = append(events, "caught")
		return completion{}
	})
	if got.end.kind != endNormal {
		t.Fatalf("run: %+v", got)
	}
	if !reflect.DeepEqual(events, []string{"local", "returned", "caught"}) {
		t.Fatalf("unwind order=%v", events)
	}
}

// REQ-009, REQ-020, REQ-040; T-017, T-018, T-039, T-040, T-079, T-080.
// A first PHP error from array release must not hide a later sticky host stop.
func TestExecutionReviewHostStopOverridesEarlierDestructorThrow(t *testing.T) {
	for _, kind := range []string{"cancel", "writer", "limit"} {
		t.Run(kind, func(t *testing.T) {
			request := readyExecutionRequest(t)
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			limits := executionLimits{16, 64, 1024}
			var writer executionWriter
			writeErr := errors.New("write failed")
			want := endCancel
			if kind == "writer" {
				writer = func([]byte) (int, error) { return 1, writeErr }
				want = endOutput
			}
			if kind == "limit" {
				limits.output = 1
				want = endLimit
			}
			x, err := newExecution(ctx, request, 64, nil, limits)
			if err != nil {
				t.Fatal(err)
			}
			if writer != nil {
				x.writer = writer
			}
			cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got := x.run(cleanup, func(x *execution) completion {
				a := x.heap.newArrayValue()
				if err := x.local(a); err != nil {
					t.Fatal(err)
				}
				first, err := x.object("First", func(*execution) completion { return completion{kind: endThrow, err: errors.New("first PHP failure")} })
				if err != nil {
					t.Fatal(err)
				}
				second, err := x.object("Second", func(x *execution) completion {
					if kind == "cancel" {
						stop()
						return completion{}
					}
					return x.write([]byte("ab"))
				})
				if err != nil {
					t.Fatal(err)
				}
				zero, _ := intValue(0, 64)
				one, _ := intValue(1, 64)
				if err := x.heap.setElement(&a, zero, first); err != nil {
					t.Fatal(err)
				}
				if err := x.heap.setElement(&a, one, second); err != nil {
					t.Fatal(err)
				}
				return completion{}
			})
			if got.end.kind != want || !got.cleanupComplete {
				t.Fatalf("host stop masked: got=%+v want kind=%d", got, want)
			}
			if kind == "writer" && (got.accepted != 1 || !errors.Is(got.end.err, writeErr)) {
				t.Errorf("writer prefix/error lost: %+v", got)
			}
		})
	}
}

// REQ-020; T-039, T-040. Forwarding a nested call moves exactly one owner.
func TestExecutionReviewNestedReturnTransfersOneOwner(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	destroyed := 0
	got := x.run(ctx, func(x *execution) completion {
		c := x.call(func(x *execution) completion {
			return x.call(func(x *execution) completion {
				v, err := x.object("Returned", func(*execution) completion { destroyed++; return completion{} })
				if err != nil {
					t.Fatal(err)
				}
				return completion{kind: endReturn, value: v}
			}, nil)
		}, nil)
		if c.kind != endReturn || destroyed != 0 {
			t.Errorf("invalid nested return: %+v destroyed=%d", c, destroyed)
		}
		if released := x.releaseReturn(&c); released.kind != endNormal {
			return released
		}
		if destroyed != 1 {
			t.Errorf("nested return retained too many owners: destroyed=%d", destroyed)
		}
		if released := x.releaseReturn(&c); released.kind != endNormal || destroyed != 1 {
			t.Errorf("return consumed twice: %+v count=%d", released, destroyed)
		}
		return completion{}
	})
	if got.end.kind != endNormal || destroyed != 1 {
		t.Fatalf("result=%+v destroyed=%d", got, destroyed)
	}
}

// REQ-020; T-039, T-040. Replacement destruction can itself replace a finally return.
func TestExecutionReviewFinallyReplacementOwnsAndReleasesBothReturns(t *testing.T) {
	for _, mode := range []string{"new", "same", "throwing-destructors"} {
		t.Run(mode, func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var events []string
			oldErr, newErr := errors.New("old destructor"), errors.New("new destructor")
			got := x.run(ctx, func(x *execution) completion {
				var old value
				c := x.call(func(x *execution) completion {
					var err error
					old, err = x.object("Old", func(*execution) completion {
						events = append(events, "old")
						if mode == "throwing-destructors" {
							return completion{kind: endThrow, err: oldErr}
						}
						return completion{}
					})
					if err != nil {
						t.Fatal(err)
					}
					return completion{kind: endReturn, value: old}
				}, func(x *execution) completion {
					if mode == "same" {
						return completion{kind: endReturn, value: old}
					}
					next, err := x.object("New", func(*execution) completion {
						events = append(events, "new")
						if mode == "throwing-destructors" {
							return completion{kind: endThrow, err: newErr}
						}
						return completion{}
					})
					if err != nil {
						t.Fatal(err)
					}
					return completion{kind: endReturn, value: next}
				})
				if mode == "throwing-destructors" {
					if c.kind != endThrow || c.err != newErr || !errors.Is(c.previous, oldErr) {
						t.Errorf("wrong destructor exception chain: %+v", c)
					}
				} else {
					if c.kind != endReturn {
						t.Errorf("lost finally return: %+v", c)
					}
					if mode == "same" && len(events) != 0 {
						t.Errorf("same-value replacement destroyed pending return: %v", events)
					}
					if released := x.releaseReturn(&c); released.kind != endNormal {
						return released
					}
				}
				events = append(events, "caller")
				return completion{}
			})
			want := []string{"old", "new", "caller"}
			if mode == "same" {
				want = []string{"old", "caller"}
			}
			if got.end.kind != endNormal || !reflect.DeepEqual(events, want) {
				t.Fatalf("result=%+v events=%v want=%v", got, events, want)
			}
		})
	}
}

// REQ-020, REQ-040; T-039, T-040, T-079, T-080. Callback results are not retained until shutdown.
func TestExecutionReviewShutdownConsumesIgnoredReturn(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var events []string
	got := x.run(ctx, func(x *execution) completion {
		if err := x.registerShutdown(func(x *execution) completion {
			v, err := x.object("Returned", func(*execution) completion { events = append(events, "destroy"); return completion{} })
			if err != nil {
				t.Fatal(err)
			}
			return completion{kind: endReturn, value: v}
		}); err != nil {
			t.Fatal(err)
		}
		if err := x.registerShutdown(func(*execution) completion { events = append(events, "next"); return completion{} }); err != nil {
			t.Fatal(err)
		}
		return completion{}
	})
	if got.end.kind != endNormal || !reflect.DeepEqual(events, []string{"destroy", "next"}) {
		t.Fatalf("result=%+v events=%v", got, events)
	}
}
