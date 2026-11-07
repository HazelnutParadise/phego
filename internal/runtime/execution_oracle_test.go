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
	if len(oracle.Cases) != 52 {
		t.Fatalf("want 52 cases, got %d", len(oracle.Cases))
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
			// Diagnostics join the event stream at the next event, so their
			// order relative to destructors, shutdown and flushes is compared.
			marked := 0
			markDiagnostics := func() {
				for ; marked < len(x.diagnostics); marked++ {
					events = append(events, "diagnostic:"+x.diagnostics[marked].level)
				}
			}
			event := func(s string) { markDiagnostics(); events = append(events, s) }
			// trigger_error(..., E_USER_ERROR) deprecates its level before the
			// fatal error; the fixtures use it to reach a fatal from PHP code.
			userError := func(x *execution, message string) completion {
				x.diagnostics = append(x.diagnostics, executionDiagnostic{"Deprecated", "Passing E_USER_ERROR to trigger_error() is deprecated since 8.4, throw an exception or call exit with a string message instead"})
				return completion{kind: endFatal, err: errors.New(message)}
			}
			result := x.run(ctx, func(x *execution) completion {
				// Handler names are PHP's callable names for the fixture closures.
				handler := func(file string, line int) string { return fmt.Sprintf("{closure:/fixtures/%s.php:%d}", file, line) }
				// solo_ fixtures do not require common.php.
				if strings.HasPrefix(tc.Case, "solo_") {
					return replaySoloOutputCase(t, x, tc.Case, event, handler(tc.Case, 6), handler(tc.Case, 7))
				}
				_, err := x.object("Tracked", func(x *execution) completion { event("destroy:global"); return x.write([]byte("D")) })
				must(err)
				must(x.registerShutdown(func(x *execution) completion { event("shutdown:1"); return x.write([]byte("S")) }))
				must(x.registerShutdown(func(x *execution) completion { event("shutdown:2"); return completion{} }))
				must(x.startBuffer(handler("common", 11), func(_ *execution, b []byte) ([]byte, completion) {
					event("flush:outer")
					return []byte("[" + string(b) + "]"), completion{}
				}))
				must(x.startBuffer(handler("common", 12), func(_ *execution, b []byte) ([]byte, completion) {
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
					must(x.startBuffer(handler("buffer_echo", 4), func(x *execution, b []byte) ([]byte, completion) {
						event("flush:echo")
						return b, x.write([]byte("discarded"))
					}))
				case "buffer_retained":
					must(x.startBuffer(handler("buffer_retained", 4), func(x *execution, b []byte) ([]byte, completion) {
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
					must(x.startBuffer(handler(tc.Case, 7), func(x *execution, b []byte) ([]byte, completion) {
						event("flush:object")
						v, err := x.object("BufferLocal", func(_ *execution) completion { event("destroy:buffer-local"); return completion{} })
						must(err)
						must(x.local(&v))
						return b, completion{}
					}))
				case "buffer_nested_start_shutdown", "buffer_nested_start_flush":
					must(x.startBuffer(handler(tc.Case, 5), func(x *execution, b []byte) ([]byte, completion) {
						event("flush:nested")
						if err := x.startBuffer("", nil); err != nil {
							var c completion
							if !errors.As(err, &c) {
								t.Fatalf("nested ob_start: %v", err)
							}
							return b, c
						}
						return b, completion{}
					}))
					if c := x.write([]byte("body")); c.kind != endNormal {
						return c
					}
					if tc.Case == "buffer_nested_start_flush" {
						if c := x.endBuffer("ob_end_flush"); c.kind != endNormal {
							return c
						}
						event("body:after")
					}
				case "flush_throw":
					must(x.startBuffer(handler("flush_throw", 4), func(_ *execution, _ []byte) ([]byte, completion) {
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
				case "finally_return_suppresses_throw", "array_finally_return_suppresses_throw", "finally_return_object_suppresses_throw":
					body := func(x *execution) completion {
						v, err := x.object("FinallyLocal", func(*execution) completion {
							event("destroy:local")
							return completion{kind: endThrow, err: errors.New("destructor")}
						})
						must(err)
						must(x.local(&v))
						return completion{kind: endThrow, err: errors.New("suppressed")}
					}
					finally := func(x *execution) completion {
						event("finally")
						if tc.Case == "finally_return_object_suppresses_throw" {
							v, err := x.object("Returned", func(*execution) completion {
								event("destroy:returned")
								return completion{kind: endThrow, err: errors.New("returned")}
							})
							must(err)
							return completion{kind: endReturn, value: v}
						}
						return completion{kind: endReturn, value: nullValue()}
					}
					var c completion
					if strings.HasPrefix(tc.Case, "array_") {
						c = x.call(func(x *execution) completion {
							a := x.heap.newArrayValue()
							must(x.local(&a))
							first, err := x.object("Earlier", func(*execution) completion {
								event("destroy:first")
								return completion{kind: endThrow, err: errors.New("first")}
							})
							must(err)
							second, err := x.object("FinallyReturn", func(x *execution) completion { return x.call(body, finally) })
							must(err)
							must(x.heap.setElement(&a, bytesValue([]byte("first")), first))
							must(x.heap.setElement(&a, bytesValue([]byte("second")), second))
							return completion{}
						}, nil)
					} else {
						c = x.call(body, finally)
					}
					if c.kind != endThrow {
						t.Fatalf("expected destructor exception: %+v", c)
					}
					event("caught:" + c.err.Error())
					for previous := c.previous; previous != nil; {
						var prior completion
						if !errors.As(previous, &prior) {
							t.Fatalf("non-completion previous: %v", previous)
						}
						event("previous:" + prior.err.Error())
						previous = prior.previous
					}
				case "return_replaced_destructor":
					c := x.call(func(x *execution) completion {
						v, err := x.object("ReturnTracked", func(x *execution) completion { event("destroy:returned"); return x.write([]byte("R")) })
						must(err)
						must(x.local(&v))
						local, err := x.object("UnwindFailure", func(*execution) completion {
							event("destroy:local")
							return completion{kind: endThrow, err: errors.New("local failure")}
						})
						must(err)
						must(x.local(&local))
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
				case "caught_then_throw", "caught_then_throw_helper", "caught_then_throw_destructor":
					c := x.call(func(x *execution) completion {
						handled := x.tryCatch(func(x *execution) completion {
							return x.call(func(*execution) completion { return completion{kind: endThrow, err: errors.New("first")} }, nil)
						}, func(completion) executionAction {
							return func(*execution) completion { event("caught:first"); return completion{} }
						})
						if handled.kind != endNormal {
							return handled
						}
						raise := func(*execution) completion { return completion{kind: endThrow, err: errors.New("second")} }
						if tc.Case == "caught_then_throw_helper" {
							return x.call(raise, nil)
						}
						if tc.Case == "caught_then_throw_destructor" {
							v, err := x.object("ThrowSecond", raise)
							must(err)
							must(x.local(&v))
							var result completion
							if err := x.heap.unset(&v); err != nil {
								if !errors.As(err, &result) {
									t.Fatal(err)
								}
							}
							return result
						}
						return raise(x)
					}, nil)
					if c.kind != endThrow {
						t.Fatalf("expected second throw: %+v", c)
					}
					event("caught:" + c.err.Error())
					if c.previous != nil {
						event("previous:" + c.previous.Error())
					}
				case "local_cow":
					show := func(v value) completion {
						snapshot, err := exportSnapshot(v)
						must(err)
						entries := snapshot.([]SnapshotEntry)
						if len(entries) != 1 {
							t.Fatalf("shared array lost: %#v", snapshot)
						}
						return x.write(entries[0].Value.([]byte))
					}
					c := x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(&a))
						must(x.heap.setElement(&a, bytesValue([]byte("key")), bytesValue([]byte("original"))))
						must(x.heap.assign(&x.globals, a))
						must(x.heap.setElement(&a, bytesValue([]byte("key")), bytesValue([]byte("changed"))))
						return show(a)
					}, nil)
					if c.kind != endNormal {
						return c
					}
					return show(x.globals)
				case "array_throw_chain", "array_throw_caught_chain":
					c := x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(&a))
						for i, name := range []string{"first", "second", "third"} {
							o, err := x.object("ChainThrow", func(x *execution) completion {
								if tc.Case == "array_throw_caught_chain" && name == "second" {
									c := x.call(func(*execution) completion { return completion{kind: endThrow, err: errors.New("middle")} }, nil)
									if c.kind != endThrow {
										return c
									}
									event("caught:middle")
									return completion{}
								}
								event("destroy:" + name)
								return completion{kind: endThrow, err: errors.New(name)}
							})
							must(err)
							key, err := intValue(int64(i), width)
							must(err)
							must(x.heap.setElement(&a, key, o))
						}
						return completion{}
					}, nil)
					if c.kind != endThrow {
						t.Fatalf("expected aggregate exception: %+v", c)
					}
					event("caught:" + c.err.Error())
					for previous := c.previous; previous != nil; {
						var prior completion
						if errors.As(previous, &prior) {
							event("previous:" + prior.err.Error())
							previous = prior.previous
						} else {
							event("previous:" + previous.Error())
							break
						}
					}
				case "array_throw_fatal", "array_throw_exit", "array_exit_throw", "array_exit_caught", "array_throw_exit_caught":
					c := x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(&a))
						kinds := []endKind{endThrow, endFatal, endNormal}
						if tc.Case == "array_throw_exit" || tc.Case == "array_throw_exit_caught" {
							kinds = []endKind{endThrow, endExit, endNormal}
						}
						if tc.Case == "array_exit_throw" {
							kinds = []endKind{endExit, endThrow, endNormal}
						}
						if tc.Case == "array_exit_caught" {
							kinds = []endKind{endExit, endNormal}
						}
						for i, kind := range kinds {
							o, err := x.object("Element", func(x *execution) completion {
								switch kind {
								case endThrow:
									event("destroy:throw")
									return completion{kind: endThrow, err: errors.New("earlier throw")}
								case endExit:
									event("destroy:exit")
									return completion{kind: endExit, code: 7}
								case endFatal:
									event("destroy:fatal")
									return userError(x, "fatal destructor")
								default:
									if strings.HasSuffix(tc.Case, "_caught") {
										return x.call(func(x *execution) completion {
											c := x.call(func(*execution) completion { return completion{kind: endThrow, err: errors.New("caught tail")} }, nil)
											if c.kind != endThrow {
												return c
											}
											event("caught:tail")
											return x.write([]byte("D"))
										}, func(*execution) completion { event("finally"); return completion{} })
									}
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
				case "frame_exit", "exit_cleanup_throw", "exit_cleanup_exit":
					return x.call(func(x *execution) completion {
						first, err := x.object("Local", func(x *execution) completion {
							switch tc.Case {
							case "exit_cleanup_throw":
								event("destroy:throw")
								return completion{kind: endThrow, err: errors.New("cleanup failure")}
							case "exit_cleanup_exit":
								event("destroy:exit")
								return completion{kind: endExit, code: 9}
							default:
								event("destroy:local")
								return x.write([]byte("D"))
							}
						})
						must(err)
						must(x.local(&first))
						if tc.Case != "frame_exit" {
							tail, err := x.object("Tracked", func(x *execution) completion { event("destroy:tail"); return x.write([]byte("D")) })
							must(err)
							must(x.local(&tail))
						}
						return completion{kind: endExit, code: 7}
					}, func(*execution) completion { event("finally"); return completion{} })
				case "array_fatal", "array_exit":
					return x.call(func(x *execution) completion {
						a := x.heap.newArrayValue()
						must(x.local(&a))
						first, err := x.object("ArrayFirst", func(x *execution) completion {
							event("destroy:array-a")
							if c := x.write([]byte("A")); c.kind != endNormal {
								return c
							}
							if tc.Case == "array_exit" {
								return completion{kind: endExit, code: 7}
							}
							return userError(x, "array failure")
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
					return userError(x, "body failure")
				case "exit":
					return completion{kind: endExit, code: 7}
				case "die":
					x.write([]byte("bye"))
					return completion{kind: endExit}
				case "frame_throw":
					c := x.call(func(x *execution) completion {
						v, err := x.object("Tracked", func(x *execution) completion { event("destroy:local"); return x.write([]byte("D")) })
						must(err)
						must(x.local(&v))
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
							must(x.local(&v))
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
			markDiagnostics()
			var want []string
			for _, line := range strings.Split(tc.Stderr, "\n") {
				if strings.HasPrefix(line, "shutdown:") || strings.HasPrefix(line, "destroy:") || strings.HasPrefix(line, "flush:") || line == "finally" || line == "caught" || strings.HasPrefix(line, "caught:") || strings.HasPrefix(line, "previous:") || line == "returned" || strings.HasPrefix(line, "body") {
					want = append(want, line)
				}
				// display_errors=stderr prints each diagnostic as "Level: message";
				// stack traces and "thrown in" lines continue the previous one.
				for _, level := range []string{"Fatal error", "Warning", "Deprecated", "Notice"} {
					if strings.HasPrefix(line, level+": ") {
						want = append(want, "diagnostic:"+level)
					}
				}
			}
			// Output handler deprecations name the handler and the flushing
			// context, and lock errors name the calling function; compare
			// their full text.
			handlerMessage := func(level, message string) bool {
				return level == "Deprecated" && strings.Contains(message, "user output handler") ||
					level == "Fatal error" && strings.Contains(message, "output buffering display handlers")
			}
			var wantHandler, gotHandler []string
			for _, line := range strings.Split(tc.Stderr, "\n") {
				for _, level := range []string{"Deprecated", "Fatal error"} {
					if message, ok := strings.CutPrefix(line, level+": "); ok && handlerMessage(level, message) {
						if at := strings.LastIndex(message, " in "); at >= 0 {
							message = message[:at]
						}
						wantHandler = append(wantHandler, message)
					}
				}
			}
			for _, d := range result.diagnostics {
				if handlerMessage(d.level, d.message) {
					gotHandler = append(gotHandler, d.message)
				}
			}
			if strings.Join(gotHandler, "\n") != strings.Join(wantHandler, "\n") {
				t.Errorf("handler messages=%q, oracle=%q", gotHandler, wantHandler)
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

// replaySoloOutputCase replays the solo_ fixtures, which set up their own
// shutdown function and buffers. The handler under test is declared after
// an optional shutdown function and outer buffer, one line each from line 6.
func replaySoloOutputCase(t *testing.T, x *execution, name string, event func(string), line6, line7 string) completion {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	outer, inner := line6, line7
	if name == "solo_end_exit" || name == "solo_end_lock_echo" {
		must(x.registerShutdown(func(x *execution) completion {
			event("shutdown:solo")
			if name == "solo_end_lock_echo" {
				return x.write([]byte("[S]"))
			}
			return x.write([]byte("S"))
		}))
		outer, inner = line7, strings.Replace(line7, ":7}", ":8}", 1)
	}
	switch name {
	case "solo_shutdown_throw":
		inner = outer // the only buffer, declared on line 6
	case "solo_end_lock_echo":
		inner = outer
	default:
		must(x.startBuffer(outer, func(_ *execution, b []byte) ([]byte, completion) {
			event("flush:outer")
			return []byte("[" + string(b) + "]"), completion{}
		}))
	}
	must(x.startBuffer(inner, func(x *execution, b []byte) ([]byte, completion) {
		switch name {
		case "solo_shutdown_throw":
			event("flush:throw")
		case "solo_end_lock_echo":
			event("flush:lock")
		default:
			event("flush:exit")
		}
		if c := x.write([]byte("x")); c.kind != endNormal {
			return nil, c
		}
		switch name {
		case "solo_shutdown_throw":
			return nil, completion{kind: endThrow, err: errors.New("Uncaught Exception: boom")}
		case "solo_end_lock_echo":
			if c := x.endBuffer("ob_end_flush"); c.kind != endNormal {
				return nil, c
			}
			return b, completion{}
		}
		return nil, completion{kind: endExit, code: 3}
	}))
	if c := x.write([]byte("body")); c.kind != endNormal {
		return c
	}
	if name == "solo_end_exit" || name == "solo_end_lock_echo" {
		if c := x.endBuffer("ob_end_flush"); c.kind != endNormal {
			return c
		}
		if name == "solo_end_exit" {
			return x.write([]byte("after"))
		}
	}
	return completion{}
}
