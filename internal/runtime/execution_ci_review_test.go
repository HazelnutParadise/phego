package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// REQ-020, REQ-040; T-039, T-040, T-079, T-080.
func TestExecutionReviewCaughtNestedShutdownThrow(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var events []string
	got := x.run(ctx, func(x *execution) completion {
		_, err := x.object("Outer", func(x *execution) completion {
			child, err := x.object("Child", func(*execution) completion {
				events = append(events, "child")
				return completion{kind: endThrow, err: errors.New("child failure")}
			})
			if err != nil {
				t.Fatal(err)
			}
			var slot value
			if err := x.heap.assign(&slot, child); err != nil {
				t.Fatal(err)
			}
			var caught completion
			if err := x.heap.unset(&slot); !errors.As(err, &caught) || caught.kind != endThrow {
				t.Fatalf("expected child exception: %v", err)
			}
			events = append(events, "caught")
			return completion{}
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = x.object("Tail", func(*execution) completion { events = append(events, "tail"); return completion{} })
		if err != nil {
			t.Fatal(err)
		}
		return completion{}
	})
	if got.end.kind != endNormal || !got.cleanupComplete || !reflect.DeepEqual(events, []string{"child", "caught", "tail"}) {
		t.Fatalf("end=%+v events=%v", got, events)
	}
}

func TestExecutionReviewFatalOverridesAggregateThrow(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fatal := errors.New("fatal destructor")
	var events []string
	got := x.run(ctx, func(x *execution) completion {
		if err := x.registerShutdown(func(x *execution) completion { events = append(events, "shutdown"); return x.write([]byte("S")) }); err != nil {
			t.Fatal(err)
		}
		c := x.call(func(x *execution) completion {
			a := x.heap.newArrayValue()
			if err := x.local(&a); err != nil {
				t.Fatal(err)
			}
			for i, kind := range []endKind{endThrow, endFatal} {
				kind := kind
				o, err := x.object("Element", func(*execution) completion {
					if kind == endFatal {
						events = append(events, "fatal")
						return completion{kind: kind, err: fatal}
					}
					events = append(events, "throw")
					return completion{kind: kind, err: errors.New("earlier throw")}
				})
				if err != nil {
					t.Fatal(err)
				}
				key, _ := intValue(int64(i), 64)
				if err := x.heap.setElement(&a, key, o); err != nil {
					t.Fatal(err)
				}
			}
			return completion{}
		}, nil)
		if c.kind == endThrow {
			events = append(events, "caught")
			return x.write([]byte("unexpected"))
		}
		return c
	})
	if got.end.kind != endFatal || !errors.Is(got.end.err, fatal) || got.end.code != 255 || string(got.output) != "S" || !got.cleanupComplete || !reflect.DeepEqual(events, []string{"throw", "fatal", "shutdown"}) {
		t.Fatalf("end=%+v events=%v", got, events)
	}
}

// A PHP fatal terminates only its current phase; host cancellation is permanent.
func TestExecutionReviewFatalPhaseBoundaries(t *testing.T) {
	for _, phase := range []string{"body", "shutdown", "destructor", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			runCtx, stop := context.WithCancel(context.Background())
			defer stop()
			x, err := newExecution(runCtx, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var events []string
			fail := func(x *execution) completion {
				events = append(events, "fatal")
				if phase == "cancel" {
					stop()
				}
				return completion{kind: endFatal, err: errors.New("phase fatal")}
			}
			got := x.run(ctx, func(x *execution) completion {
				if err := x.startBuffer(func(x *execution, b []byte) ([]byte, completion) {
					events = append(events, "flush")
					v, err := x.object("NewInOutput", func(*execution) completion { events = append(events, "new"); return completion{} })
					if err != nil {
						t.Fatal(err)
					}
					if err := x.local(&v); err != nil {
						t.Fatal(err)
					}
					return b, completion{}
				}); err != nil {
					t.Fatal(err)
				}
				if err := x.registerShutdown(func(x *execution) completion {
					events = append(events, "shutdown")
					if phase == "shutdown" {
						return fail(x)
					}
					return completion{}
				}); err != nil {
					t.Fatal(err)
				}
				if phase == "destructor" {
					if _, err := x.object("Fatal", fail); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "body" || phase == "cancel" {
					return fail(x)
				}
				return completion{}
			})
			want := []string{"fatal", "shutdown", "flush", "new"}
			kind := endFatal
			if phase == "shutdown" || phase == "destructor" {
				want = []string{"shutdown", "fatal", "flush", "new"}
			}
			if phase == "cancel" {
				want = []string{"fatal"}
				kind = endCancel
			}
			if got.end.kind != kind || !got.cleanupComplete || !reflect.DeepEqual(events, want) {
				t.Fatalf("result=%+v events=%v want=%v", got, events, want)
			}
		})
	}
}

func TestExecutionReviewFatalOverridesPropertyThrow(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{16, 64, 1024})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var events []string
			got := x.run(ctx, func(x *execution) completion {
				parent, err := x.object("Parent", nil)
				if err != nil {
					t.Fatal(err)
				}
				if shutdown {
					if err := x.heap.assign(&x.globals, x.heap.newArrayValue()); err != nil {
						t.Fatal(err)
					}
					if err := x.heap.setElement(&x.globals, bytesValue([]byte("parent")), parent); err != nil {
						t.Fatal(err)
					}
				} else if err := x.local(&parent); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"throw", "fatal", "tail"} {
					child, err := x.object("Child", func(*execution) completion {
						events = append(events, name)
						if name == "throw" {
							return completion{kind: endThrow, err: errors.New(name)}
						}
						if name == "fatal" {
							return completion{kind: endFatal, err: errors.New(name)}
						}
						return completion{}
					})
					if err != nil {
						t.Fatal(err)
					}
					if err := x.heap.setProperty(parent, name, child); err != nil {
						t.Fatal(err)
					}
				}
				return completion{}
			})
			if got.end.kind != endFatal || !got.cleanupComplete || !reflect.DeepEqual(events, []string{"throw", "fatal"}) {
				t.Fatalf("result=%+v events=%v", got, events)
			}
		})
	}
}
