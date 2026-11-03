package runtime

import (
	"strings"
	"testing"
	"time"
)

func w13R3CallableReturnNames() []string {
	names := []string{}
	for _, shape := range []string{"array", "object", "invalid"} {
		for _, source := range []string{"typed", "mixed", "untyped"} {
			if shape == "invalid" && source != "typed" {
				continue
			}
			for _, frame := range []string{"", "-frame"} {
				names = append(names, "r3-callable-return-"+shape+"-"+source+frame)
			}
		}
	}
	return names
}

func w13R3CallableReturnCase(t *testing.T, h *heap, name string) w13Case {
	var result any
	parts := strings.Split(strings.TrimPrefix(name, "r3-callable-return-"), "-")
	shape, source := parts[0], parts[1]
	run := func(h *heap) {
		target := w13Build(t, classSpec{name: "W13R3CallableTarget", methods: []methodSpec{{name: "go", call: func(*heap, value, []value) (value, error) { return w13Str("go"), nil }}, {name: "__invoke", call: func(*heap, value, []value) (value, error) { return w13Str("go"), nil }}}})
		typ := source
		if source == "untyped" {
			typ = ""
		}
		if source == "typed" {
			typ = "array"
			if shape == "object" {
				typ = "object"
			}
		}
		var c *class
		c = w13Build(t, classSpec{name: "W13R3TypedCallable_" + shape + "_" + source, properties: []propertySpec{{name: "storage", typeName: typ}}, methods: []methodSpec{{name: "value", returnType: "callable", returnsReference: true, call: func(h *heap, this value, _ []value) (value, error) {
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
		}}}})
		o := w13Instantiate(t, h, c)
		v := w13Instantiate(t, h, target)
		if shape != "object" {
			a := h.newArrayValue()
			must(t, h.appendValue(&a, v))
			method := "go"
			if shape == "invalid" {
				method = "missing"
			}
			must(t, h.appendValue(&a, w13Str(method)))
			must(t, h.writeObject(o, "storage", a, c, true))
			must(t, h.unset(&a))
		} else {
			must(t, h.writeObject(o, "storage", v, c, true))
		}
		out, e := h.callMethod(o, "value", nil, nil, false)
		var observed any
		if e != nil {
			observed = w13Error(e)
		} else {
			valid := h.isObjectCallable(out, nil)
			var returned value
			if shape == "object" {
				returned, e = h.callMethod(out, "__invoke", nil, nil, false)
			} else {
				first, _, err := h.getElement(out, w13Int(0))
				must(t, err)
				returned, e = h.callMethod(first, "go", nil, nil, false)
			}
			must(t, e)
			stored := w13Read(t, h, o, "storage", nil)
			same, e := identical(out, stored)
			must(t, e)
			observed = []any{"ok", valid, w13Encode(returned), same}
			must(t, h.discardMethodReturn(out))
		}
		result = []any{observed, w13Encode(w13Read(t, h, o, "storage", nil))}
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
