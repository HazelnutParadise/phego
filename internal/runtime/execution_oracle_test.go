package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// REQ-020, REQ-040; T-039, T-040, T-079, T-080.
// These are local action replays, not execution of PHP source by Phego.
func TestExecutionOracleReplay(t *testing.T) {
	for _, width := range []int{64, 32} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			replayExecutionOracle(t, width)
		})
	}
}

func replayExecutionOracle(t *testing.T, width int) {
	name := "reference.json"
	if width == 32 {
		name = "reference-32.json"
	}
	data, err := os.ReadFile("../../tests/lifecycle/w14/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		IntSize    int    `json:"int_size"`
		PHPVersion string `json:"php_version"`
		OS         string `json:"os"`
		Cases      []struct {
			Case           string
			ExitCode       int `json:"exit_code"`
			Stdout, Stderr string
		}
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.IntSize*8 != width || oracle.PHPVersion != "8.5.12" || oracle.OS != "Linux" {
		t.Fatalf("unexpected reference profile: %s/%s/%d", oracle.PHPVersion, oracle.OS, oracle.IntSize)
	}
	if len(oracle.Cases) != 30 {
		t.Fatalf("want 30 cases, got %d", len(oracle.Cases))
	}
	for _, tc := range oracle.Cases {
		t.Run(tc.Case, func(t *testing.T) {
			r, _ := NewRequest(nil)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(r.Admit(func(context.Context) error { return nil }))
			must(r.BeginInit())
			must(r.Ready())
			x, err := newExecution(context.Background(), r, width, nil, executionLimits{32, 64, 4096})
			must(err)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var events []string
			event := func(s string) { events = append(events, s) }
			result := x.run(ctx, func(x *execution) completion {
				_, err := x.object("Tracked", func(x *execution) completion { event("destroy:global"); return x.write([]byte("D")) })
				must(err)
				must(x.registerShutdown(func(x *execution) completion { event("shutdown:1"); return x.write([]byte("S")) }))
				must(x.registerShutdown(func(x *execution) completion { event("shutdown:2"); return completion{} }))
				must(x.startBuffer(func(_ *execution, b []byte) ([]byte, completion) {
					event("flush:outer")
					return []byte("[" + string(b) + "]"), completion{}
				}))
				must(x.startBuffer(func(_ *execution, b []byte) ([]byte, completion) {
					event("flush:inner")
					return []byte("(" + string(b) + ")"), completion{}
				}))
				switch tc.Case {
				case "shutdown_nested_catch":
					_, err := x.object("Outer", func(x *execution) completion {
						child, err := x.object("NestedChild", func(*execution) completion {
							event("destroy:child")
							return completion{kind: endThrow, err: errors.New("child failure")}
						})
						must(err)
						var slot value
						must(x.heap.assign(&slot, child))
						var caught completion
						if err := x.heap.unset(&slot); !errors.As(err, &caught) || caught.kind != endThrow {
							t.Fatalf("expected child exception: %v", err)
						}
						event("caught:child")
						return completion{}
					})
					must(err)
					_, err = x.object("Tracked", func(x *execution) completion { event("destroy:tail"); return x.write([]byte("D")) })
					must(err)
				case "fatal_shutdown_create":
					// Store the pre-fatal object so the callback can explicitly unset it.
					old := value{kind: valueObject, object: x.heap.live[1]}
					must(x.heap.assign(&x.globals, x.heap.newArrayValue()))
					must(x.heap.setElement(&x.globals, bytesValue([]byte("a")), old))
					must(x.registerShutdown(func(x *execution) completion {
						event("shutdown:create")
						must(x.heap.unsetElement(&x.globals, bytesValue([]byte("a"))))
						v, err := x.object("Tracked", func(x *execution) completion { event("destroy:new-local"); return x.write([]byte("D")) })
						must(err)
						var local value
						must(x.heap.assign(&local, v))
						must(x.heap.unset(&local))
						v, err = x.object("Tracked", func(x *execution) completion { event("destroy:new-retained"); return x.write([]byte("D")) })
						must(err)
						must(x.heap.setElement(&x.globals, bytesValue([]byte("new")), v))
						return completion{}
					}))
				case "buffer_echo":
					must(x.startBuffer(func(x *execution, b []byte) ([]byte, completion) {
						event("flush:echo")
						return b, x.write([]byte("discarded"))
					}))
				case "buffer_retained":
					must(x.startBuffer(func(x *execution, b []byte) ([]byte, completion) {
						event("flush:retained")
						v, err := x.object("Tracked", func(x *execution) completion { event("destroy:unexpected-late"); return x.write([]byte("D")) })
						must(err)
						must(x.heap.assign(&x.globals, x.heap.newArrayValue()))
						must(x.heap.setElement(&x.globals, bytesValue([]byte("late")), v))
						return b, completion{}
					}))
				case "destructor_create":
					_, err := x.object("Creator", func(x *execution) completion {
						event("destroy:creator")
						v, err := x.object("Tracked", func(x *execution) completion { event("destroy:late"); return x.write([]byte("D")) })
						must(err)
						must(x.heap.assign(&x.globals, x.heap.newArrayValue()))
						must(x.heap.setElement(&x.globals, bytesValue([]byte("late")), v))
						return x.write([]byte("C"))
					})
					must(err)
				case "buffer_object", "fatal_buffer_object":
					must(x.startBuffer(func(x *execution, b []byte) ([]byte, completion) {
						event("flush:object")
						v, err := x.object("BufferLocal", func(_ *execution) completion { event("destroy:buffer-local"); return completion{} })
						must(err)
						must(x.local(v))
						return b, completion{}
					}))
				case "flush_throw":
					must(x.startBuffer(func(_ *execution, _ []byte) ([]byte, completion) {
						event("flush:throw")
						return nil, completion{kind: endThrow, err: errors.New("flush failure")}
					}))
				case "shutdown_throw", "shutdown_exit", "fatal_shutdown_exit", "uncaught_shutdown_exit":
					must(x.registerShutdown(func(_ *execution) completion {
						if tc.Case != "shutdown_throw" {
							event("shutdown:exit")
							return completion{kind: endExit, code: 9}
						}
						event("shutdown:throw")
						return completion{kind: endThrow, err: errors.New("shutdown failure")}
					}))
					must(x.registerShutdown(func(_ *execution) completion { event("shutdown:unexpected"); return completion{} }))
				case "shutdown_register":
					must(x.registerShutdown(func(x *execution) completion {
						event("shutdown:register")
						must(x.registerShutdown(func(_ *execution) completion { event("shutdown:late"); return completion{} }))
						return completion{}
					}))
				case "destructor_throw":
					_, err := x.object("Broken", func(_ *execution) completion {
						event("destroy:throw")
						return completion{kind: endThrow, err: errors.New("destructor failure")}
					})
					must(err)
				}
				if c := x.write([]byte("B")); c.kind != endNormal {
					return c
				}
				switch tc.Case {
				case "normal":
					event("body")
				case "return_finally_array", "return_finally_reference":
					var alias value
					c := x.call(func(x *execution) completion {
						must(x.heap.assign(&x.globals, x.heap.newArrayValue()))
						must(x.heap.setElement(&x.globals, bytesValue([]byte("key")), bytesValue([]byte("kept"))))
						if tc.Case == "return_finally_reference" {
							must(x.heap.bind(&alias, &x.globals))
						}
						return completion{kind: endReturn, value: x.globals}
					}, func(x *execution) completion {
						if tc.Case == "return_finally_reference" {
							v := x.heap.newArrayValue()
							must(x.heap.setElement(&v, bytesValue([]byte("key")), bytesValue([]byte("changed"))))
							must(x.heap.assign(&alias, v))
							must(x.heap.unset(&alias))
						}
						must(x.heap.unset(&x.globals))
						event("finally")
						return completion{}
					})
					if c.kind != endReturn {
						t.Fatalf("return failed: %+v", c)
					}
					snapshot, err := exportSnapshot(c.value)
					must(err)
					entries := snapshot.([]SnapshotEntry)
					if len(entries) != 1 {
						t.Fatalf("pending return lost: %#v", snapshot)
					}
					if next := x.write(entries[0].Value.([]byte)); next.kind != endNormal {
						return next
					}
					if next := x.releaseReturn(&c); next.kind != endNormal {
						return next
					}
					event("returned")
				case "return_replaced_destructor":
					c := x.call(func(x *execution) completion {
						v, err := x.object("ReturnTracked", func(x *execution) completion { event("destroy:returned"); return x.write([]byte("R")) })
						must(err)
						must(x.local(v))
						local, err := x.object("UnwindFailure", func(*execution) completion {
							event("destroy:local")
							return completion{kind: endThrow, err: errors.New("local failure")}
						})
						must(err)
						must(x.local(local))
						return completion{kind: endReturn, value: v}
					}, nil)
					if c.kind != endThrow {
						t.Fatalf("destructor failure lost: %+v", c)
					}
					event("caught")
				case "return_finally_replace":
					newReturn := func(name, output string) completion {
						v, err := x.object("Replacement", func(x *execution) completion {
							event("destroy:" + name)
							if next := x.write([]byte(output)); next.kind != endNormal {
								return next
							}
							return completion{kind: endThrow, err: errors.New(name)}
						})
						must(err)
						return completion{kind: endReturn, value: v}
					}
					c := x.call(func(*execution) completion { return newReturn("old", "O") }, func(*execution) completion { return newReturn("new", "N") })
					if c.kind != endThrow || c.previous == nil {
						t.Fatalf("replacement exception chain lost: %+v", c)
					}
					event("caught:" + c.err.Error())
					event("previous:" + c.previous.Error())
				case "array_throw_fatal":
					c := x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(a))
						for i, kind := range []endKind{endThrow, endFatal, endNormal} {
							o, err := x.object("Element", func(x *execution) completion {
								switch kind {
								case endThrow:
									event("destroy:throw")
									return completion{kind: endThrow, err: errors.New("earlier throw")}
								case endFatal:
									event("destroy:fatal")
									return completion{kind: endFatal, err: errors.New("fatal destructor")}
								default:
									event("destroy:tail")
									return x.write([]byte("D"))
								}
							})
							must(err)
							key, err := intValue(int64(i), width)
							must(err)
							must(x.heap.setElement(&a, key, o))
						}
						return completion{}
					}, nil)
					if c.kind == endThrow {
						event("caught")
						return x.write([]byte("unexpected"))
					}
					return c
				case "array_fatal", "array_exit":
					return x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(a))
						first, err := x.object("ArrayFirst", func(x *execution) completion {
							event("destroy:array-a")
							if c := x.write([]byte("A")); c.kind != endNormal {
								return c
							}
							if tc.Case == "array_exit" {
								return completion{kind: endExit, code: 7}
							}
							return completion{kind: endFatal, err: errors.New("array failure")}
						})
						must(err)
						second, err := x.object("Tracked", func(x *execution) completion { event("destroy:array-b"); return x.write([]byte("D")) })
						must(err)
						zero, err := intValue(0, width)
						must(err)
						one, err := intValue(1, width)
						must(err)
						must(x.heap.setElement(&a, zero, first))
						must(x.heap.setElement(&a, one, second))
						return completion{}
					}, nil)
				case "warning":
					x.warning("first warning")
					x.warning("second warning")
					event("body:continued")
				case "uncaught", "uncaught_shutdown_exit":
					return completion{kind: endThrow, err: errors.New("body failure")}
				case "fatal", "fatal_shutdown_exit", "fatal_shutdown_create", "fatal_buffer_object":
					return completion{kind: endFatal, err: errors.New("body failure")}
				case "exit":
					return completion{kind: endExit, code: 7}
				case "die":
					x.write([]byte("bye"))
					return completion{kind: endExit}
				case "frame_throw":
					c := x.call(func(x *execution) completion {
						v, err := x.object("Tracked", func(x *execution) completion { event("destroy:local"); return x.write([]byte("D")) })
						must(err)
						must(x.local(v))
						return completion{kind: endThrow, err: errors.New("caught")}
					}, func(_ *execution) completion { event("finally"); return completion{} })
					if c.kind != endThrow {
						t.Errorf("throw lost: %+v", c)
					}
					event("caught")
				case "frame_locals", "frame_destructor_throw":
					c := x.call(func(x *execution) completion {
						for _, name := range []string{"local-a", "local-b"} {
							v, err := x.object("Tracked", func(x *execution) completion {
								event("destroy:" + name)
								if c := x.write([]byte("D")); c.kind != endNormal {
									return c
								}
								if tc.Case == "frame_destructor_throw" && name == "local-a" {
									return completion{kind: endThrow, err: errors.New("local failure")}
								}
								return completion{}
							})
							must(err)
							must(x.local(v))
						}
						return completion{}
					}, nil)
					if tc.Case == "frame_destructor_throw" {
						if c.kind != endThrow {
							t.Errorf("throw lost: %+v", c)
						}
						event("caught")
					} else {
						if c.kind != endNormal {
							t.Errorf("unexpected completion: %+v", c)
						}
						event("returned")
					}
				}
				return completion{}
			})
			if string(result.output) != tc.Stdout || result.end.code != tc.ExitCode {
				t.Errorf("output=%q code=%d, oracle=%q/%d", result.output, result.end.code, tc.Stdout, tc.ExitCode)
			}
			var want []string
			for _, line := range strings.Split(tc.Stderr, "\n") {
				if strings.HasPrefix(line, "shutdown:") || strings.HasPrefix(line, "destroy:") || strings.HasPrefix(line, "flush:") || line == "finally" || line == "caught" || strings.HasPrefix(line, "caught:") || strings.HasPrefix(line, "previous:") || line == "returned" || strings.HasPrefix(line, "body") {
					want = append(want, line)
				}
			}
			if strings.Join(events, "\n") != strings.Join(want, "\n") {
				t.Errorf("events=%v, oracle=%v", events, want)
			}
			if !result.cleanupComplete || result.cleanupErr != nil {
				t.Errorf("cleanup: %+v", result)
			}
		})
	}
}
