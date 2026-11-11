package runtime

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func w13R3HookNames() []string {
	names := []string{}
	for _, mode := range []string{"normal", "conversion", "destructor"} {
		for _, frame := range []string{"", "-frame"} {
			names = append(names, "r3-hook-drop-"+mode+frame)
		}
	}
	return names
}

func w13R3HookCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var result any
	run := func(h *heap) {
		var owner value
		c := w13Build(t, classSpec{name: "W13R3OwnedStringable", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
			events = append(events, "convert")
			if e := h.unset(&owner); e != nil {
				return value{}, e
			}
			if strings.Contains(name, "conversion") {
				return value{}, &Throwable{"Exception", "conversion"}
			}
			return w13Str("converted"), nil
		}}}, destructor: func(*heap, value) error {
			events = append(events, "destructor")
			if strings.Contains(name, "destructor") {
				return &Throwable{"Exception", "destructor"}
			}
			return nil
		}})
		must(t, h.assign(&owner, w13Instantiate(t, h, c)))
		box := w13Build(t, classSpec{name: "W13R3DroppingGetter", properties: []propertySpec{{name: "value", typeName: "string", virtual: true, get: func(*heap, value, value) (value, error) { return owner, nil }}}})
		var returnErr error
		got := w13ObserveValue(func() (value, error) {
			v, e := h.readObject(w13Instantiate(t, h, box), "value", nil)
			returnErr = e
			return v, e
		})
		var previous any
		var end completion
		if errors.As(returnErr, &end) && end.previous != nil {
			var php *Throwable
			if errors.As(end.previous, &php) {
				previous = []any{php.Class, php.Message}
			}
		}
		result = []any{got, append([]any{}, events...), previous}
	}
	if strings.HasSuffix(name, "-frame") {
		x := newTestExecution(t, readyExecutionRequest(t), h.width, nil, executionLimits{16, 64, 1024})
		cleanup, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		out := x.run(cleanup, func(x *execution) completion { run(x.heap); return completion{} })
		if out.end.kind != endNormal || !out.cleanupComplete {
			t.Fatal(out)
		}
	} else {
		run(h)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
