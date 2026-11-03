package runtime

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func w13R3ValueNames() []string {
	var names []string
	for _, family := range []string{"plain", "inherited", "trait"} {
		for _, path := range []string{"property", "argument", "return"} {
			for _, mode := range []string{"weak", "strict"} {
				names = append(names, "r3-value-stringable-"+family+"-"+path+"-"+mode)
			}
		}
	}
	for _, path := range []string{"property", "argument", "return"} {
		names = append(names, "r3-value-stringable-negative-"+path)
	}
	for _, path := range []string{"property", "argument", "return"} {
		for _, input := range []string{"precision", "exponent", "negative-zero", "boundary-fixed", "boundary-exponent"} {
			names = append(names, "r3-value-float-string-"+path+"-"+input)
		}
	}
	for _, path := range []string{"property", "argument", "return"} {
		for _, input := range []string{"huge", "negative-huge", "nan", "inf"} {
			names = append(names, "r3-value-intstring-overflow-"+path+"-"+input)
		}
	}
	for _, frame := range []string{"", "-frame"} {
		names = append(names, "r3-value-return-owned-stringable"+frame)
		for _, failure := range []string{"", "-conversion-throw", "-destructor-throw"} {
			if failure != "" {
				names = append(names, "r3-value-return-owned-stringable"+failure+frame)
			}
			names = append(names, "r3-value-return-argument-stringable"+failure+frame)
		}
	}
	names = append(names, "r3-value-hook-borrowed-stringable", "r3-value-hook-borrowed-string-conversion")
	return names
}

func w13R3ValueCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	events := []any{}

	makeStringable := func(family string) value {
		textMethod := methodSpec{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("text"), nil }}
		switch family {
		case "plain":
			return w13Instantiate(t, h, w13Build(t, classSpec{name: "W13R3ValuePlain", methods: []methodSpec{textMethod}}))
		case "inherited":
			parent := w13Build(t, classSpec{name: "W13R3ValueStringBase", methods: []methodSpec{textMethod}})
			child := w13Build(t, classSpec{name: "W13R3ValueInherited", parent: parent})
			return w13Instantiate(t, h, child)
		case "trait":
			trait := w13Build(t, classSpec{name: "W13R3ValueStringTrait", isTrait: true, methods: []methodSpec{textMethod}})
			class := w13Build(t, classSpec{name: "W13R3ValueTraitUser", traits: []*class{trait}})
			return w13Instantiate(t, h, class)
		default:
			t.Fatalf("unknown Stringable family %q", family)
			return value{}
		}
	}

	observeTypedStringable := func(path, family string, strict bool) any {
		var owner value
		must(t, h.assign(&owner, makeStringable(family)))
		typ := "Stringable"
		switch path {
		case "property":
			c := w13Build(t, classSpec{name: "W13R3ValueStringBox", properties: []propertySpec{{name: "value", typeName: typ}}})
			box := w13Instantiate(t, h, c)
			got := w13ObserveValue(func() (value, error) {
				if err := h.writeObject(box, "value", owner, nil, strict); err != nil {
					return value{}, err
				}
				stored, err := h.readObject(box, "value", nil)
				if err != nil {
					return value{}, err
				}
				return boolValue(stored.kind == valueObject), nil
			})
			must(t, h.unset(&owner))
			return got
		case "argument":
			c := w13Build(t, classSpec{name: "W13R3ValueStringArgument", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "value", typeName: typ}}, call: func(_ *heap, _ value, args []value) (value, error) {
				return boolValue(args[0].kind == valueObject), nil
			}}}})
			got := w13ObserveValue(func() (value, error) {
				return h.callMethod(w13Instantiate(t, h, c), "accept", []methodArgument{{value: owner}}, nil, strict)
			})
			must(t, h.unset(&owner))
			return got
		case "return":
			c := w13Build(t, classSpec{name: "W13R3ValueStringReturn", methods: []methodSpec{{name: "produce", returnType: typ, returnStrict: strict, call: func(*heap, value, []value) (value, error) { return owner, nil }}}})
			got := w13ObserveValue(func() (value, error) {
				out, err := h.callMethod(w13Instantiate(t, h, c), "produce", nil, nil, false)
				if err != nil {
					return value{}, err
				}
				ok, identityErr := identical(out, owner)
				if identityErr != nil {
					return value{}, identityErr
				}
				if err := h.discardMethodReturn(out); err != nil {
					return value{}, err
				}
				return boolValue(ok), nil
			})
			must(t, h.unset(&owner))
			return got
		default:
			t.Fatalf("unknown Stringable path %q", path)
			return nil
		}
	}

	if strings.HasPrefix(name, "r3-value-stringable-") {
		parts := strings.Split(strings.TrimPrefix(name, "r3-value-stringable-"), "-")
		if len(parts) == 3 {
			result = observeTypedStringable(parts[1], parts[0], parts[2] == "strict")
		} else if len(parts) == 2 && parts[0] == "negative" {
			cNoString := w13Build(t, classSpec{name: "W13R3ValueNotStringable"})
			var owner value
			must(t, h.assign(&owner, w13Instantiate(t, h, cNoString)))
			switch parts[1] {
			case "property":
				box := w13Instantiate(t, h, w13Build(t, classSpec{name: "W13R3ValueNegativeBox", properties: []propertySpec{{name: "value", typeName: "Stringable"}}}))
				result = w13ObserveError(func() error { return h.writeObject(box, "value", owner, nil, false) })
			case "argument":
				c := w13Build(t, classSpec{name: "W13R3ValueNegativeArgument", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "value", typeName: "Stringable"}}, call: func(*heap, value, []value) (value, error) { return boolValue(true), nil }}}})
				result = w13ObserveError(func() error {
					out, err := h.callMethod(w13Instantiate(t, h, c), "accept", []methodArgument{{value: owner}}, nil, false)
					if err != nil {
						return err
					}
					return h.discardMethodReturn(out)
				})
			case "return":
				c := w13Build(t, classSpec{name: "W13R3ValueNegativeReturn", methods: []methodSpec{{name: "produce", returnType: "Stringable", call: func(*heap, value, []value) (value, error) { return owner, nil }}}})
				result = w13ObserveError(func() error {
					out, err := h.callMethod(w13Instantiate(t, h, c), "produce", nil, nil, false)
					if err != nil {
						return err
					}
					return h.discardMethodReturn(out)
				})
			default:
				t.Fatalf("unknown negative Stringable path %q", parts[1])
			}
			must(t, h.unset(&owner))
		} else {
			t.Fatalf("unknown case %q", name)
		}
	} else if strings.HasPrefix(name, "r3-value-float-string-") {
		parts := strings.SplitN(strings.TrimPrefix(name, "r3-value-float-string-"), "-", 2)
		if len(parts) != 2 {
			t.Fatalf("unknown case %q", name)
		}
		inputs := map[string]float64{"precision": 1.2345678901234567, "exponent": 1e20, "negative-zero": math.Copysign(0, -1), "boundary-fixed": 99999999999999.0, "boundary-exponent": 1e14}
		input, ok := inputs[parts[1]]
		if !ok {
			t.Fatalf("unknown float input %q", parts[1])
		}
		f := floatValue(input)
		switch parts[0] {
		case "property":
			c := w13Build(t, classSpec{name: "W13R3FloatProperty", properties: []propertySpec{{name: "value", typeName: "string"}}})
			o := w13Instantiate(t, h, c)
			result = w13ObserveValue(func() (value, error) {
				if err := h.writeObject(o, "value", f, nil, false); err != nil {
					return value{}, err
				}
				return h.readObject(o, "value", nil)
			})
		case "argument":
			c := w13Build(t, classSpec{name: "W13R3FloatArgument", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "value", typeName: "string"}}, call: func(_ *heap, _ value, args []value) (value, error) { return args[0], nil }}}})
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(w13Instantiate(t, h, c), "accept", []methodArgument{{value: f}}, nil, false)
			})
		case "return":
			c := w13Build(t, classSpec{name: "W13R3FloatReturn", methods: []methodSpec{{name: "produce", parameters: []methodParameter{{name: "value", typeName: "mixed"}}, returnType: "string", call: func(*heap, value, []value) (value, error) { return f, nil }}}})
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(w13Instantiate(t, h, c), "produce", []methodArgument{{value: f}}, nil, false)
			})
		default:
			t.Fatalf("unknown float conversion path %q", parts[0])
		}
	} else if strings.HasPrefix(name, "r3-value-intstring-overflow-") {
		parts := strings.SplitN(strings.TrimPrefix(name, "r3-value-intstring-overflow-"), "-", 2)
		inputs := map[string]value{"huge": floatValue(1e30), "negative-huge": floatValue(-1e30), "nan": floatValue(math.NaN()), "inf": floatValue(math.Inf(1))}
		input, ok := inputs[parts[1]]
		if !ok || len(parts) != 2 {
			t.Fatalf("unknown int|string input in %q", name)
		}
		typ := "int|string"
		switch parts[0] {
		case "property":
			c := w13Build(t, classSpec{name: "W13R3IntStringProperty", properties: []propertySpec{{name: "value", typeName: typ}}})
			o := w13Instantiate(t, h, c)
			result = w13ObserveValue(func() (value, error) {
				if err := h.writeObject(o, "value", input, nil, false); err != nil {
					return value{}, err
				}
				return h.readObject(o, "value", nil)
			})
		case "argument":
			c := w13Build(t, classSpec{name: "W13R3IntStringArgument", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "value", typeName: typ}}, call: func(_ *heap, _ value, args []value) (value, error) { return args[0], nil }}}})
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(w13Instantiate(t, h, c), "accept", []methodArgument{{value: input}}, nil, false)
			})
		case "return":
			c := w13Build(t, classSpec{name: "W13R3IntStringReturn", methods: []methodSpec{{name: "produce", parameters: []methodParameter{{name: "value", typeName: "mixed"}}, returnType: typ, call: func(*heap, value, []value) (value, error) { return input, nil }}}})
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(w13Instantiate(t, h, c), "produce", []methodArgument{{value: input}}, nil, false)
			})
		default:
			t.Fatalf("unknown int|string path %q", parts[0])
		}
	} else if strings.HasPrefix(name, "r3-value-return-owned-stringable") || strings.HasPrefix(name, "r3-value-return-argument-stringable") {
		frame := strings.HasSuffix(name, "-frame")
		direct := !strings.Contains(name, "-argument-stringable")
		failure := ""
		for _, candidate := range []string{"-conversion-throw", "-destructor-throw"} {
			if strings.Contains(name, candidate) {
				failure = candidate
			}
		}
		run := func(h *heap) {
			var globalOwner value
			stringer := w13Build(t, classSpec{name: "W13R3OwnedStringable", destructor: func(*heap, value) error {
				events = append(events, "destructor")
				if failure == "-destructor-throw" {
					return &Throwable{"Exception", "destructor"}
				}
				return nil
			}, methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
				events = append(events, "convert")
				if err := h.unset(&globalOwner); err != nil {
					return value{}, err
				}
				if failure == "-conversion-throw" {
					return value{}, &Throwable{"Exception", "conversion"}
				}
				return w13Str("converted"), nil
			}}}})
			must(t, h.assign(&globalOwner, w13Instantiate(t, h, stringer)))
			if direct {
				producer := w13Build(t, classSpec{name: "W13R3OwnedStringDirectReturn", methods: []methodSpec{{name: "produce", returnType: "string", call: func(*heap, value, []value) (value, error) { return globalOwner, nil }}}})
				var returnErr error
				got := w13ObserveValue(func() (value, error) {
					v, e := h.callMethod(w13Instantiate(t, h, producer), "produce", nil, nil, false)
					returnErr = e
					return v, e
				})
				alive := globalOwner.kind == valueObject && !globalOwner.object.freed
				before := append([]any{}, events...)
				if alive {
					must(t, h.unset(&globalOwner))
				}
				var previous any
				var end completion
				if errors.As(returnErr, &end) && end.previous != nil {
					var php *Throwable
					if errors.As(end.previous, &php) {
						previous = []any{php.Class, php.Message}
					}
				}
				result = []any{got, alive, before, events, previous}
			} else {
				consumer := w13Build(t, classSpec{name: "W13R3OwnedStringSink", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "value", typeName: "string"}}, returnType: "string", call: func(_ *heap, _ value, args []value) (value, error) {
					events = append(events, "body")
					return args[0], nil
				}}}})
				got := w13ObserveValue(func() (value, error) {
					return h.callMethod(w13Instantiate(t, h, consumer), "accept", []methodArgument{{value: globalOwner}}, nil, false)
				})
				result = []any{got, append([]any{}, events...)}
			}
		}
		if frame {
			x := newTestExecution(t, readyExecutionRequest(t), h.width, nil, executionLimits{16, 64, 1024})
			cleanup, cancel := cleanupDeadline(t, time.Second)
			defer cancel()
			out := x.run(cleanup, func(x *execution) completion { run(x.heap); return completion{} })
			if out.end.kind != endNormal || !out.cleanupComplete {
				t.Fatalf("execution: %#v", out)
			}
		} else {
			run(h)
		}
	} else if name == "r3-value-hook-borrowed-stringable" || name == "r3-value-hook-borrowed-string-conversion" {
		var globalOwner value
		stringer := w13Build(t, classSpec{name: "W13R3BorrowedStringable", destructor: func(*heap, value) error { events = append(events, "destructor"); return nil }, methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
			events = append(events, "convert")
			return w13Str("hook-text"), nil
		}}}})
		must(t, h.assign(&globalOwner, w13Instantiate(t, h, stringer)))
		propertyType := "Stringable"
		if name == "r3-value-hook-borrowed-string-conversion" {
			propertyType = "string"
		}
		getter := w13Build(t, classSpec{name: "W13R3BorrowedGetter", properties: []propertySpec{{name: "value", typeName: propertyType, virtual: true, get: func(*heap, value, value) (value, error) { return globalOwner, nil }}}})
		o := w13Instantiate(t, h, getter)
		var localOwner value
		got := w13ObserveValue(func() (value, error) {
			v, err := h.readObject(o, "value", nil)
			if err != nil {
				return value{}, err
			}
			if v.kind == valueObject {
				if err := h.assign(&localOwner, v); err != nil {
					return value{}, err
				}
				return boolValue(true), nil
			}
			return v, nil
		})
		before := append([]any{}, events...)
		must(t, h.unset(&globalOwner))
		afterGlobal := append([]any{}, events...)
		must(t, h.unset(&localOwner))
		result = []any{got, before, afterGlobal, events}
	} else {
		t.Fatalf("unknown case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
