package runtime

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func w13OwnershipNames() []string {
	return []string{"method-parameter-order", "method-named-parameter-order", "method-coerce-receiver", "method-coerce-second", "method-coerce-source", "method-failed-parameter"}
}

func w13OwnershipCase(t *testing.T, width int, name string) w13Case {
	x := newTestExecution(t, readyExecutionRequest(t), width, nil, executionLimits{16, 64, 1024})
	h := x.heap
	events := []any{}
	var observed []any
	var a, b, receiver, source value
	parameter := func(label string, fail bool) value {
		o, err := h.newObject(&class{name: "W13OwnedParameter", destructor: func(*heap, value) error {
			events = append(events, label)
			if fail {
				return &Throwable{"Exception", "cleanup"}
			}
			return nil
		}})
		must(t, err)
		return o
	}
	var methods []methodSpec
	methods = append(methods, methodSpec{name: "order", parameters: []methodParameter{{name: "a"}, {name: "b"}}, call: func(h *heap, _ value, _ []value) (value, error) {
		must(t, h.unset(&a))
		must(t, h.unset(&b))
		events = append(events, "body")
		return w13Int(7), nil
	}})
	methods = append(methods, methodSpec{name: "text", parameters: []methodParameter{{name: "s", typeName: "string"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "body"); return nullValue(), nil }})
	methods = append(methods, methodSpec{name: "second", parameters: []methodParameter{{name: "s", typeName: "string"}, {name: "p"}}, call: func(h *heap, _ value, args []value) (value, error) {
		_, err := h.checkedObject(args[1])
		if err != nil {
			events = append(events, "dead")
		} else {
			events = append(events, "body")
		}
		return nullValue(), nil
	}})
	methods = append(methods, methodSpec{name: "fail", parameters: []methodParameter{{name: "p"}}, call: func(h *heap, _ value, _ []value) (value, error) {
		must(t, h.unset(&a))
		return h.newObject(&class{name: "W13MethodResult", destructor: func(*heap, value) error { events = append(events, "result"); return nil }})
	}})
	c := w13Build(t, classSpec{name: "W13OwnershipMethods", methods: methods, destructor: func(*heap, value) error { events = append(events, "receiver"); return nil }})
	cleanup, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	r := x.run(cleanup, func(*execution) completion {
		m := w13Instantiate(t, h, c)
		must(t, h.assign(&receiver, m))
		method := "text"
		var args []methodArgument
		if strings.HasSuffix(name, "parameter-order") {
			must(t, h.assign(&a, parameter("a", false)))
			must(t, h.assign(&b, parameter("b", false)))
			method = "order"
			args = []methodArgument{{value: a}, {value: b}}
			if name == "method-named-parameter-order" {
				args = []methodArgument{{name: "b", value: b}, {name: "a", value: a}}
			}
		} else if name == "method-failed-parameter" {
			must(t, h.assign(&a, parameter("parameter", true)))
			method = "fail"
			args = []methodArgument{{value: a}}
		} else {
			s := w13Build(t, classSpec{name: "W13OwnedString", destructor: func(*heap, value) error { events = append(events, "string-dtor"); return nil }, methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
				events = append(events, "string")
				switch name {
				case "method-coerce-receiver":
					must(t, h.unset(&receiver))
				case "method-coerce-second":
					must(t, h.unset(&a))
				case "method-coerce-source":
					must(t, h.unset(&source))
				}
				return str("abc"), nil
			}}}})
			must(t, h.assign(&source, w13Instantiate(t, h, s)))
			args = []methodArgument{{value: source}}
			if name == "method-coerce-second" {
				method = "second"
				must(t, h.assign(&a, parameter("arg2", false)))
				args = append(args, methodArgument{value: a})
			}
		}
		stop := x.tryCatch(func(*execution) completion {
			out, err := h.callMethod(m, method, args, nil, false)
			if err == nil {
				err = h.discard(out)
			}
			if err != nil {
				var c completion
				if errors.As(err, &c) {
					return c
				}
				return completion{kind: endThrow, err: err}
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
		return completion{}
	})
	if r.end.kind != endNormal || !r.cleanupComplete {
		t.Fatal(r)
	}
	return w13Case{Result: observed, Diagnostics: []any{}}
}
