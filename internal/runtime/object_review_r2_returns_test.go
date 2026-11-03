package runtime

import (
	"strings"
	"testing"
	"time"
)

func w13R2ReturnNames() []string {
	return []string{"r2-return-magic", "r2-return-hook", "r2-return-magic-frame", "r2-return-hook-frame", "r2-return-coerced-hook", "r2-return-coerced-hook-frame", "r2-return-magic-reference", "r2-return-hook-reference", "r2-return-magic-reference-frame", "r2-return-hook-reference-frame", "r2-return-magic-value-object-reference", "r2-return-magic-value-scalar-reference", "r2-return-magic-readonly-reference"}
}

func w13R2ReturnCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var observation any
	run := func(h *heap) {
		child := w13Build(t, classSpec{name: "W13R2ReturnChild", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("child"), nil }}}, destructor: func(*heap, value) error { events = append(events, "child"); return nil }})
		getter := func(h *heap) (value, error) {
			v, err := h.instantiate(child)
			if err != nil {
				return value{}, err
			}
			retain(v)
			return value{kind: valueReference, ref: &reference{owner: h, target: v}}, nil
		}
		var box *class
		if strings.Contains(name, "magic") {
			className := "W13R2ReturnMagic"
			var properties []propertySpec
			if strings.Contains(name, "scalar") {
				className = "W13R2ReturnMagicScalar"
			} else if strings.Contains(name, "value-object") {
				className = "W13R2ReturnMagicObject"
			} else if strings.Contains(name, "readonly") {
				className = "W13R2ReturnReadonly"
				properties = []propertySpec{{name: "p", typeName: "int", readonly: true, setVisibility: protectedAccess}, {name: "storage", typeName: "int", hasDefault: true, initial: int64(1)}}
			}
			box = w13Build(t, classSpec{name: className, properties: properties, methods: []methodSpec{{name: "__get", returnsReference: !strings.Contains(name, "value-"), parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, _ []value) (value, error) {
				if strings.Contains(name, "scalar") {
					return w13Int(2), nil
				}
				if strings.Contains(name, "value-object") {
					return h.instantiate(child)
				}
				if strings.Contains(name, "readonly") {
					var local value
					if err := h.referenceObject(&local, this, "storage", nil); err != nil {
						return value{}, err
					}
					if err := h.release(local); err != nil {
						return value{}, err
					}
					return local, nil
				}
				return getter(h)
			}}}})
		} else if strings.Contains(name, "coerced") {
			box = w13Build(t, classSpec{name: "W13R2ReturnCoerced", properties: []propertySpec{{name: "p", typeName: "string", virtual: true, get: func(h *heap, _, _ value) (value, error) { return getter(h) }}}})
		} else {
			box = w13Build(t, classSpec{name: "W13R2ReturnHook", properties: []propertySpec{{name: "p", typeName: "W13R2ReturnChild", virtual: true, getReference: true, get: func(h *heap, _, _ value) (value, error) { return getter(h) }}}})
		}
		o := w13Instantiate(t, h, box)
		if strings.Contains(name, "readonly") {
			must(t, h.unsetObject(o, "p", box))
		}
		var result value
		if strings.Contains(name, "-reference") {
			err := h.referenceObject(&result, o, "p", nil)
			if err != nil && strings.Contains(name, "readonly") {
				observation = w13Error(err)
				return
			}
			must(t, err)
		} else {
			v, err := h.readObject(o, "p", nil)
			must(t, err)
			must(t, h.assign(&result, v))
		}
		if strings.Contains(name, "scalar") {
			observation = w13Encode(result)
		}
		if strings.Contains(name, "readonly") {
			must(t, h.assignPHP(&result, w13Int(2), false))
			v, err := h.readObject(o, "storage", nil)
			must(t, err)
			observation = w13Encode(v)
		}
		must(t, h.unset(&result))
		events = append(events, "after")
	}
	if strings.HasSuffix(name, "-frame") {
		x := newTestExecution(t, readyExecutionRequest(t), h.width, nil, executionLimits{16, 64, 1024})
		cleanup, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		result := x.run(cleanup, func(x *execution) completion { run(x.heap); return completion{} })
		if result.end.kind != endNormal || !result.cleanupComplete {
			t.Fatal(result)
		}
	} else {
		run(h)
	}
	if strings.Contains(name, "readonly") {
		return w13Case{Result: observation, Diagnostics: w13Diagnostics(h)}
	}
	if strings.Contains(name, "scalar") {
		return w13Case{Result: []any{observation, events}, Diagnostics: w13Diagnostics(h)}
	}
	return w13Case{Result: events, Diagnostics: w13Diagnostics(h)}
}
