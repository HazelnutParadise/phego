package runtime

import (
	"fmt"
	"testing"
	"time"
)

func w13ShutdownCaseNames() []string {
	return []string{"shutdown-handle-reuse", "shutdown-late-hole"}
}

func w13RunShutdownCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name != "shutdown-handle-reuse" && name != "shutdown-late-hole" {
		t.Fatalf("unknown shutdown case %q", name)
	}

	x := newTestExecution(t, readyExecutionRequest(t), h.width, nil, executionLimits{16, 64, 1024})
	cleanup, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	result := x.run(cleanup, func(x *execution) completion {
		fail := func(err error) completion { return x.fail(endInternal, err) }
		global := func(name string, v value) error {
			return x.heap.setElement(&x.globals, bytesValue([]byte(name)), v)
		}
		newTracked := func(label string) (value, error) {
			return x.object(label, func(x *execution) completion {
				if c := x.write([]byte(label + "\n")); c.kind != endNormal {
					return c
				}
				if label != "maker" {
					return completion{}
				}
				created, err := x.object("new", func(x *execution) completion {
					return x.write([]byte("new\n"))
				})
				if err != nil {
					return fail(err)
				}
				if err := global("new1", created); err != nil {
					return fail(err)
				}
				if err := global("new2", created); err != nil {
					return fail(err)
				}
				return completion{}
			})
		}
		if err := x.heap.assign(&x.globals, x.heap.newArrayValue()); err != nil {
			return fail(err)
		}
		hole, err := newTracked("hole")
		if err != nil {
			return fail(err)
		}
		if err := global("hole", hole); err != nil {
			return fail(err)
		}
		old, err := newTracked("old")
		if err != nil {
			return fail(err)
		}
		if err := global("old", old); err != nil {
			return fail(err)
		}
		if err := global("old2", old); err != nil {
			return fail(err)
		}
		if name == "shutdown-handle-reuse" {
			if err := x.heap.unsetElement(&x.globals, bytesValue([]byte("hole"))); err != nil {
				return fail(err)
			}
		}
		maker, err := newTracked("maker")
		if err != nil {
			return fail(err)
		}
		if err := global("maker", maker); err != nil {
			return fail(err)
		}
		if err := global("maker2", maker); err != nil {
			return fail(err)
		}
		if name == "shutdown-late-hole" {
			if err := x.heap.unsetElement(&x.globals, bytesValue([]byte("hole"))); err != nil {
				return fail(err)
			}
		}
		return x.write([]byte("body-end\n"))
	})
	if result.end.kind != endNormal || result.cleanupErr != nil || !result.cleanupComplete {
		t.Fatalf("shutdown execution failed: %+v", result)
	}
	if len(result.diagnostics) != 0 {
		t.Fatalf("shutdown execution diagnostics: %+v", result.diagnostics)
	}
	return w13Case{Result: string(result.output), Diagnostics: []any{}}
}

func w13RunShutdownHeapCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name != "shutdown-handle-reuse" && name != "shutdown-late-hole" {
		t.Fatalf("unknown shutdown case %q", name)
	}

	var globals value
	must(t, h.assign(&globals, h.newArrayValue()))
	output := ""
	global := func(name string, v value) error {
		return h.setElement(&globals, bytesValue([]byte(name)), v)
	}
	var newTracked func(string) (value, error)
	newTracked = func(label string) (value, error) {
		return h.newObject(&class{name: label, destructor: func(h *heap, _ value) error {
			output += label + "\n"
			if label != "maker" {
				return nil
			}
			created, err := newTracked("new")
			if err != nil {
				return err
			}
			if err := global("new1", created); err != nil {
				return err
			}
			return global("new2", created)
		}})
	}
	hole, err := newTracked("hole")
	must(t, err)
	must(t, global("hole", hole))
	old, err := newTracked("old")
	must(t, err)
	must(t, global("old", old))
	must(t, global("old2", old))
	if name == "shutdown-handle-reuse" {
		must(t, h.unsetElement(&globals, bytesValue([]byte("hole"))))
	}
	maker, err := newTracked("maker")
	must(t, err)
	must(t, global("maker", maker))
	must(t, global("maker2", maker))
	if name == "shutdown-late-hole" {
		must(t, h.unsetElement(&globals, bytesValue([]byte("hole"))))
	}
	output += "body-end\n"
	must(t, h.shutdown(&globals))
	return w13Case{Result: output, Diagnostics: w13Diagnostics(h)}
}

func TestW13ShutdownHandleReuse(t *testing.T) {
	expected := map[string]string{
		"shutdown-handle-reuse": "hole\nbody-end\nmaker\nold\nnew\n",
		"shutdown-late-hole":    "hole\nbody-end\nold\nmaker\nnew\n",
	}
	for _, width := range []int{32, 64} {
		for _, name := range w13ShutdownCaseNames() {
			t.Run(fmt.Sprintf("%d/%s/execution", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				got := w13RunShutdownCase(t, h, name)
				if got.Result != expected[name] {
					t.Fatalf("execution output=%q, want %q", got.Result, expected[name])
				}
				w13AssertOracleCase(t, width, name, got)
			})
			t.Run(fmt.Sprintf("%d/%s/heap-shutdown", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				got := w13RunShutdownHeapCase(t, h, name)
				if got.Result != expected[name] {
					t.Fatalf("heap shutdown output=%q, want %q", got.Result, expected[name])
				}
				w13AssertOracleCase(t, width, name, got)
			})
		}
	}
}
