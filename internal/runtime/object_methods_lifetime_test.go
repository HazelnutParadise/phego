package runtime

import (
	"errors"
	"testing"
	"time"
)

func w13AdvancedMethodNames() []string {
	return []string{"method-return-cleanup", "method-variadic-reference", "method-return-declaration", "method-extra-arguments", "method-failed-result"}
}

func w13AdvancedMethodCase(t *testing.T, width int, name string) w13Case {
	t.Helper()
	x := newTestExecution(t, readyExecutionRequest(t), width, nil, executionLimits{16, 64, 1024})
	h := x.heap
	events := []any{}
	c := w13Build(t, classSpec{name: "W13MethodLifecycle", methods: []methodSpec{
		{name: "bad", returnType: "int", call: func(h *heap, _ value, _ []value) (value, error) {
			return h.newObject(&class{name: "W13HookDtor", destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }})
		}},
		{name: "variadic", returnType: "array", parameters: []methodParameter{{name: "args", variadic: true, byReference: true}}, call: func(h *heap, _ value, args []value) (value, error) {
			return args[0], h.setElement(&args[0], w13Int(0), w13Int(9))
		}},
		{name: "weakReturn", returnType: "int", call: func(*heap, value, []value) (value, error) { return str("7"), nil }},
		{name: "allArgs", returnType: "array", parameters: []methodParameter{{name: "a"}}, call: func(h *heap, _ value, args []value) (value, error) {
			var out value
			for _, v := range args {
				if err := h.appendValue(&out, v); err != nil {
					return value{}, err
				}
			}
			return out, nil
		}},
	}})
	var observed any
	cleanup, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	r := x.run(cleanup, func(*execution) completion {
		o := w13Instantiate(t, h, c)
		switch name {
		case "method-failed-result":
			var owner value
			failed := w13Build(t, classSpec{
				name: "W13MethodFail",
				destructor: func(*heap, value) error {
					events = append(events, "receiver")
					return &Throwable{"Exception", "cleanup"}
				},
				methods: []methodSpec{{name: "done", call: func(h *heap, _ value, _ []value) (value, error) {
					if err := h.unset(&owner); err != nil {
						return value{}, err
					}
					return h.newObject(&class{name: "W13MethodResult", destructor: func(*heap, value) error { events = append(events, "result"); return nil }})
				}}},
			})
			must(t, h.assign(&owner, w13Instantiate(t, h, failed)))
			stop := x.tryCatch(func(x *execution) completion {
				// PHP keeps the receiver operand alive until the statement has
				// discarded its unused result, independently of the callee frame.
				receiver := owner
				retain(receiver)
				out, err := h.callMethod(receiver, "done", nil, nil, false)
				if err == nil {
					err = h.discard(out)
				}
				if cleanup := h.release(receiver); cleanup != nil {
					err = cleanup
				}
				if err != nil {
					var c completion
					if errors.As(err, &c) {
						return c
					}
					return x.fail(endThrow, err)
				}
				return completion{}
			}, func(c completion) executionAction {
				return func(*execution) completion {
					var php *Throwable
					if !errors.As(c, &php) {
						t.Fatal(c)
					}
					events = append(events, "caught:"+php.Message)
					return completion{}
				}
			})
			if stop.kind != endNormal {
				return stop
			}
			events = append(events, "after")
			observed = append([]any{}, events...)
		case "method-return-cleanup":
			e := w13ObserveValue(func() (value, error) { return h.callMethod(o, "bad", nil, nil, true) })
			events = append(events, "after")
			observed = []any{e, append([]any{}, events...)}
		case "method-variadic-reference":
			v := w13Int(1)
			out, err := h.callMethod(o, "variadic", []methodArgument{{value: v, slot: &v}}, nil, false)
			must(t, err)
			observed = []any{w13Encode(out), w13Encode(v.deref())}
		case "method-return-declaration":
			out, err := h.callMethod(o, "weakReturn", nil, nil, true)
			if err != nil {
				observed = w13ObserveError(func() error { return err })
			} else {
				observed = w13Encode(out)
			}
		case "method-extra-arguments":
			out, err := h.callMethod(o, "allArgs", []methodArgument{{value: w13Int(1)}, {value: w13Int(2)}, {value: w13Int(3)}}, nil, false)
			must(t, err)
			observed = w13Encode(out)
		default:
			t.Fatal(name)
		}
		return completion{}
	})
	if r.end.kind != endNormal || !r.cleanupComplete {
		t.Fatal(r)
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}
