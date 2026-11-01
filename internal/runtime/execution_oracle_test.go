package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// REQ-020, REQ-040; T-039, T-040, T-079, T-080.
// These are local action replays, not execution of PHP source by Phego.
func TestExecutionOracleReplay(t *testing.T) {
	data, err := os.ReadFile("../../tests/lifecycle/w14/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Case           string
			ExitCode       int `json:"exit_code"`
			Stdout, Stderr string
		}
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
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
			x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 4096})
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
						zero, err := intValue(0, 64)
						must(err)
						one, err := intValue(1, 64)
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
				if strings.HasPrefix(line, "shutdown:") || strings.HasPrefix(line, "destroy:") || strings.HasPrefix(line, "flush:") || line == "finally" || line == "caught" || line == "returned" || strings.HasPrefix(line, "body") {
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
