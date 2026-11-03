package runtime

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func w13R3FrameNames() []string {
	return []string{"r3-frame-return-instance", "r3-frame-return-static", "r3-frame-return-reference", "r3-frame-return-errors"}
}

func w13R3FrameCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var observed any
	x := newTestExecution(t, readyExecutionRequest(t), h.width, nil, executionLimits{16, 64, 1024})
	cleanup, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	out := x.run(cleanup, func(x *execution) completion {
		h := x.heap
		marker := w13Build(t, classSpec{name: "W13R3FrameMarker", destructor: func(*heap, value) error {
			events = append(events, "local")
			if strings.HasSuffix(name, "errors") {
				return &Throwable{"Exception", "local"}
			}
			return nil
		}})
		text := w13Build(t, classSpec{name: "W13R3FrameText", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
			events = append(events, "convert")
			if strings.HasSuffix(name, "errors") {
				return value{}, &Throwable{"Exception", "conversion"}
			}
			return w13Str("text"), nil
		}}}, destructor: func(*heap, value) error { events = append(events, "temporary"); return nil }})
		reference := strings.HasSuffix(name, "reference")
		producer := w13Build(t, classSpec{name: "W13R3FrameProducer", methods: []methodSpec{{name: "value", returnType: "string", returnsReference: reference, static: strings.HasSuffix(name, "static"), call: func(h *heap, _ value, _ []value) (value, error) {
			var local value
			if e := x.local(&local); e != nil {
				return value{}, e
			}
			if e := h.assign(&local, w13Instantiate(t, h, marker)); e != nil {
				return value{}, e
			}
			v := w13Instantiate(t, h, text)
			if !reference {
				return v, nil
			}
			var source, alias value
			if e := x.local(&source); e != nil {
				return value{}, e
			}
			if e := h.assign(&source, v); e != nil {
				return value{}, e
			}
			if e := x.local(&alias); e != nil {
				return value{}, e
			}
			if e := h.bind(&alias, &source); e != nil {
				return value{}, e
			}
			return alias, nil
		}}}})
		var v value
		var err error
		v, err = h.callMethod(w13Instantiate(t, h, producer), "value", nil, nil, false)
		if err == nil {
			observed = w13Encode(v)
			must(t, h.discardMethodReturn(v))
		} else {
			chain := []any{}
			for i := 0; err != nil && i < 16; i++ {
				var php *Throwable
				if !errors.As(err, &php) {
					t.Fatal(err)
				}
				chain = append(chain, []any{php.Class, php.Message})
				var end completion
				if !errors.As(err, &end) {
					break
				}
				err = end.previous
			}
			observed = chain
		}
		events = append(events, "after")
		return completion{}
	})
	if out.end.kind != endNormal || !out.cleanupComplete {
		t.Fatal(out)
	}
	return w13Case{Result: []any{observed, events}, Diagnostics: w13Diagnostics(h)}
}
