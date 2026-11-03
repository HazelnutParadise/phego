package runtime

import (
	"strings"
	"testing"
	"time"
)

func w13R3TypedReturnNames() []string {
	names := []string{}
	for _, target := range []string{"string", "float", "text"} {
		for _, source := range []string{"int", "union", "mixed", "untyped"} {
			if target == "text" && (source == "int" || source == "union") {
				continue
			}
			for _, path := range []string{"method", "hook"} {
				for _, frame := range []string{"", "-frame"} {
					names = append(names, "r3-typed-return-"+target+"-"+source+"-"+path+frame)
				}
			}
		}
	}
	return names
}

func w13R3TypedReturnCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var result any
	parts := strings.Split(strings.TrimPrefix(name, "r3-typed-return-"), "-")
	target, source, path := parts[0], parts[1], parts[2]
	run := func(h *heap) {
		typ := source
		if typ == "untyped" {
			typ = ""
		}
		if typ == "union" {
			typ = "int|string"
			if target == "float" {
				typ = "int|float"
			}
		}
		returnType := target
		if target == "text" {
			returnType = "string"
		}
		text := w13Build(t, classSpec{name: "W13R3TypedReturnText", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
			events = append(events, "convert")
			return w13Str("text"), nil
		}}}})
		className := "W13R3TypedReturn_" + target + "_" + source + "_" + path
		var c *class
		getter := func(h *heap, this value) (value, error) {
			var r value
			if h.execution != nil {
				if e := h.execution.local(&r); e != nil {
					return value{}, e
				}
			}
			if e := h.referenceObject(&r, this, "storage", c); e != nil {
				return value{}, e
			}
			raw := r
			if h.execution == nil {
				if e := h.release(r); e != nil {
					return value{}, e
				}
			}
			return raw, nil
		}
		spec := classSpec{name: className, properties: []propertySpec{{name: "storage", typeName: typ}}}
		if path == "method" {
			spec.methods = []methodSpec{{name: "value", returnType: returnType, returnsReference: true, call: func(h *heap, this value, _ []value) (value, error) { return getter(h, this) }}}
		} else {
			spec.properties = append(spec.properties, propertySpec{name: "value", typeName: returnType, virtual: true, getReference: true, get: func(h *heap, this, _ value) (value, error) { return getter(h, this) }})
		}
		c = w13Build(t, spec)
		o := w13Instantiate(t, h, c)
		initial := w13Int(7)
		if target == "text" {
			initial = w13Instantiate(t, h, text)
		}
		must(t, h.writeObject(o, "storage", initial, c, true))
		observed := w13ObserveValue(func() (value, error) {
			if path == "method" {
				return h.callMethod(o, "value", nil, nil, false)
			}
			return h.readObject(o, "value", nil)
		})
		stored, e := h.readObject(o, "storage", nil)
		must(t, e)
		result = []any{observed, typeName(stored), w13Encode(stored), append([]any{}, events...)}
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
