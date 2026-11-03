package runtime

import (
	"math"
	"strings"
	"testing"
)

func w13R2ValueNames() []string {
	names := []string{
		"r2-values-string-reference-concat",
		"r2-values-typed-compare-primary",
		"r2-values-typed-compare-reverse",
		"r2-values-typed-compare-prefix",
		"r2-values-typed-compare-uninitialized",
		"r2-values-typed-materialized-table",
		"r2-values-typed-materialized-prefix",
		"r2-values-materialized-clone",
	}
	for _, where := range []string{"property", "argument", "return"} {
		for _, typ := range []string{"intstring", "intbool"} {
			for _, input := range []string{"huge", "nan", "inf", "numeric-string"} {
				names = append(names, "r2-values-union-"+where+"-"+typ+"-"+input)
			}
		}
	}
	names = append(names,
		"r2-values-magic-duplicate-missing",
		"r2-values-magic-duplicate-inaccessible",
	)
	for _, kind := range []string{"ordinary", "ghost", "proxy"} {
		for _, visibility := range []string{"private", "protected"} {
			names = append(names, "r2-values-clone-"+kind+"-"+visibility)
		}
	}
	return append(names, "r2-values-clone-in-scope")
}

func w13R2ValueCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	var result any

	switch {
	case name == "r2-values-materialized-clone":
		c := w13Build(t, classSpec{name: "W13R2VTypedMaterialized", allowDynamic: true, properties: []propertySpec{{name: "a", typeName: "int"}, {name: "b", typeName: "int"}}})
		left, right := w13Instantiate(t, h, c), w13Instantiate(t, h, c)
		must(t, h.writeObject(left, "a", w13Int(3), nil, true))
		must(t, h.writeObject(right, "a", w13Int(3), nil, true))
		must(t, h.writeObject(right, "b", w13Int(2), nil, true))
		_, err := h.materializeObjectProperties(left)
		must(t, err)
		copy, err := h.cloneObject(left, nil)
		must(t, err)
		first, err := h.compare(copy, right)
		must(t, err)
		must(t, h.discardMethodReturn(copy))
		must(t, h.writeObject(left, "extra", w13Int(7), nil, true))
		must(t, h.unsetObject(left, "extra", nil))
		copy, err = h.cloneObject(left, nil)
		must(t, err)
		second, err := h.compare(copy, right)
		must(t, err)
		must(t, h.discardMethodReturn(copy))
		result = []any{first, second}
	case name == "r2-values-string-reference-concat":
		var c *class
		c = w13Build(t, classSpec{
			name:       "W13R2VStringReference",
			properties: []propertySpec{{name: "text", typeName: "string", hasDefault: true, initial: []byte("value")}},
			methods: []methodSpec{{
				name: "__toString", returnType: "string", returnsReference: true,
				call: func(h *heap, this value, _ []value) (value, error) {
					var text value
					if err := h.referenceObject(&text, this, "text", c); err != nil {
						return value{}, err
					}
					returned := text
					if err := h.release(text); err != nil {
						return value{}, err
					}
					return returned, nil
				},
			}},
			destructor: func(*heap, value) error {
				events = append(events, "wrapper")
				return nil
			},
		})
		var owner value
		must(t, h.assign(&owner, w13Instantiate(t, h, c)))
		converted := w13ObserveValue(func() (value, error) { return h.concat(w13Str("prefix:"), owner) })
		before := append([]any{}, events...)
		must(t, h.unset(&owner))
		result = []any{converted, before, events}

	case strings.HasPrefix(name, "r2-values-typed-compare-") || name == "r2-values-typed-materialized-table" || name == "r2-values-typed-materialized-prefix":
		className := "W13R2VTypedCompare"
		allowDynamic := false
		if name == "r2-values-typed-materialized-table" || name == "r2-values-typed-materialized-prefix" {
			className = "W13R2VTypedMaterialized"
			allowDynamic = true
		}
		c := w13Build(t, classSpec{
			name:         className,
			allowDynamic: allowDynamic,
			properties:   []propertySpec{{name: "a", typeName: "int"}, {name: "b", typeName: "int"}},
		})
		makeObject := func(a int64, hasB bool, b int64) value {
			o := w13Instantiate(t, h, c)
			must(t, h.writeObject(o, "a", w13Int(a), nil, true))
			if hasB {
				must(t, h.writeObject(o, "b", w13Int(b), nil, true))
			}
			return o
		}
		compare := func(left, right value) any {
			return w13ObserveValue(func() (value, error) {
				n, err := h.compare(left, right)
				if err != nil {
					return value{}, err
				}
				return w13Int(int64(n)), nil
			})
		}
		switch name {
		case "r2-values-typed-compare-primary", "r2-values-typed-compare-reverse":
			left := makeObject(1, false, 0)
			right := makeObject(0, true, 0)
			var leftOwner, rightOwner value
			must(t, h.assign(&leftOwner, left))
			must(t, h.assign(&rightOwner, right))
			if name == "r2-values-typed-compare-reverse" {
				leftOwner, rightOwner = rightOwner, leftOwner
			}
			result = compare(leftOwner, rightOwner)
			must(t, h.unset(&leftOwner))
			must(t, h.unset(&rightOwner))

		case "r2-values-typed-compare-prefix":
			left, right := makeObject(3, true, 1), makeObject(3, true, 2)
			result = compare(left, right)

		case "r2-values-typed-compare-uninitialized":
			left, right := makeObject(3, false, 0), makeObject(3, true, 2)
			result = []any{
				compare(left, right),
				compare(right, left),
				w13ObjectAsMap(left),
				w13ObjectAsMap(left),
				w13ObjectAsMap(right),
				w13ObjectAsMap(right),
			}

		case "r2-values-typed-materialized-table":
			left, right := makeObject(1, false, 0), makeObject(0, true, 0)
			before := compare(left, right)
			leftVars := w13ObjectAsMap(left)
			_, err := h.materializeObjectProperties(left)
			must(t, err)
			leftCast := w13ObjectAsMap(left)
			afterMaterialize := compare(left, right)
			must(t, h.writeObject(left, "extra", w13Int(7), nil, false))
			afterAdd := compare(left, right)
			must(t, h.unsetObject(left, "extra", nil))
			afterRemove := compare(left, right)
			result = []any{
				before,
				leftVars,
				leftCast,
				afterMaterialize,
				afterAdd,
				afterRemove,
				w13ObjectAsMap(left),
				w13ObjectAsMap(left),
			}

		case "r2-values-typed-materialized-prefix":
			left, right := makeObject(3, false, 0), makeObject(3, true, 2)
			before := compare(left, right)
			_, err := h.materializeObjectProperties(left)
			must(t, err)
			afterGetVars := compare(left, right)
			must(t, h.writeObject(left, "extra", w13Int(7), nil, false))
			must(t, h.writeObject(right, "extra", w13Int(7), nil, false))
			afterAdd := compare(left, right)
			must(t, h.unsetObject(left, "extra", nil))
			must(t, h.unsetObject(right, "extra", nil))
			afterRemove := compare(left, right)
			result = []any{before, afterGetVars, afterAdd, afterRemove}
		}

	case strings.HasPrefix(name, "r2-values-union-"):
		fields := strings.SplitN(strings.TrimPrefix(name, "r2-values-union-"), "-", 3)
		if len(fields) != 3 {
			t.Fatal(name)
		}
		where, typeName, inputName := fields[0], fields[1], fields[2]
		types := map[string]string{"intstring": "int|string", "intbool": "int|bool"}
		typ, ok := types[typeName]
		if !ok {
			t.Fatal(name)
		}
		inputs := map[string]value{
			"huge":           floatValue(1e30),
			"nan":            floatValue(math.NaN()),
			"inf":            floatValue(math.Inf(1)),
			"numeric-string": w13Str("1e30"),
		}
		input, ok := inputs[inputName]
		if !ok {
			t.Fatal(name)
		}
		classNames := map[string]string{
			"intstring": "W13R2VUnionIntString",
			"intbool":   "W13R2VUnionIntBool",
		}
		className, ok := classNames[typeName]
		if !ok {
			t.Fatal(name)
		}
		switch where {
		case "property":
			c := w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "value", typeName: typ}}})
			o := w13Instantiate(t, h, c)
			result = w13ObserveValue(func() (value, error) {
				if err := h.writeObject(o, "value", input, nil, false); err != nil {
					return value{}, err
				}
				return h.readObject(o, "value", nil)
			})
		case "argument":
			c := w13Build(t, classSpec{name: className, methods: []methodSpec{{
				name: "accept", parameters: []methodParameter{{name: "value", typeName: typ}},
				call: func(_ *heap, _ value, args []value) (value, error) { return args[0], nil },
			}}})
			o := w13Instantiate(t, h, c)
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(o, "accept", []methodArgument{{value: input}}, nil, false)
			})
		case "return":
			c := w13Build(t, classSpec{name: className, methods: []methodSpec{{
				name: "produce", returnType: typ,
				call: func(_ *heap, _ value, _ []value) (value, error) { return input, nil },
			}}})
			o := w13Instantiate(t, h, c)
			result = w13ObserveValue(func() (value, error) {
				return h.callMethod(o, "produce", nil, nil, false)
			})
		default:
			t.Fatal(name)
		}

	case name == "r2-values-magic-duplicate-missing" || name == "r2-values-magic-duplicate-inaccessible":
		methods := []methodSpec{{
			name: "__call", parameters: []methodParameter{{name: "name"}, {name: "arguments"}},
			call: func(*heap, value, []value) (value, error) {
				events = append(events, "magic")
				return nullValue(), nil
			},
		}}
		methodName := "missing"
		if name == "r2-values-magic-duplicate-inaccessible" {
			methods = append(methods, methodSpec{name: "hidden", visibility: privateAccess, call: func(*heap, value, []value) (value, error) {
				events = append(events, "hidden")
				return nullValue(), nil
			}})
			methodName = "hidden"
		}
		c := w13Build(t, classSpec{name: "W13R2VMagicDuplicate", methods: methods})
		o := w13Instantiate(t, h, c)
		got := w13ObserveValue(func() (value, error) {
			return h.callMethod(o, methodName, []methodArgument{
				{name: "duplicate", value: w13Int(1)},
				{name: "duplicate", value: w13Int(2)},
			}, nil, false)
		})
		result = []any{got, events}

	case strings.HasPrefix(name, "r2-values-clone-"):
		parts := strings.Split(name, "-")
		if len(parts) != 5 {
			t.Fatal(name)
		}
		kind, visibilityName := parts[3], parts[4]
		if kind == "in" {
			visibilityName = "private"
		}
		visibility := map[string]access{"private": privateAccess, "protected": protectedAccess}[visibilityName]
		if visibility == publicAccess {
			t.Fatal(name)
		}
		cloneClassName := "W13R2VClone" + map[string]string{"private": "Private", "protected": "Protected"}[visibilityName]
		var c *class
		methods := []methodSpec{{
			name: "__clone", visibility: visibility,
			call: func(*heap, value, []value) (value, error) {
				events = append(events, "clone")
				return nullValue(), nil
			},
		}}
		if kind == "in" {
			methods = append(methods, methodSpec{
				name: "duplicate",
				call: func(h *heap, this value, _ []value) (value, error) {
					return h.cloneObject(this, c)
				},
			})
		}
		c = w13Build(t, classSpec{
			name: cloneClassName, methods: methods,
			destructor: func(*heap, value) error {
				events = append(events, "destructor")
				return nil
			},
		})

		var source value
		if kind == "ghost" {
			var err error
			source, err = h.newLazyObject(c, false, func(*heap, value) (value, error) {
				events = append(events, "initializer")
				return nullValue(), nil
			})
			must(t, err)
		} else if kind == "proxy" {
			var err error
			source, err = h.newLazyObject(c, true, func(h *heap, _ value) (value, error) {
				events = append(events, "initializer")
				return w13Instantiate(t, h, c), nil
			})
			must(t, err)
		} else {
			source = w13Instantiate(t, h, c)
		}
		var sourceOwner, cloneOwner value
		must(t, h.assign(&sourceOwner, source))
		var cloneValue any
		if kind == "in" {
			clone, err := h.callMethod(sourceOwner, "duplicate", nil, nil, false)
			must(t, err)
			must(t, h.assign(&cloneOwner, clone))
		} else {
			cloneValue = w13ObserveValue(func() (value, error) {
				clone, err := h.cloneObject(sourceOwner, nil)
				if err != nil {
					return value{}, err
				}
				if err := h.assign(&cloneOwner, clone); err != nil {
					return value{}, err
				}
				return clone, nil
			})
		}
		afterClone := append([]any{}, events...)
		if cloneOwner.kind != valueUndefined {
			must(t, h.unset(&cloneOwner))
		}
		must(t, h.unset(&sourceOwner))
		if kind == "in" {
			result = []any{afterClone, events}
		} else {
			result = []any{cloneValue, afterClone, events}
		}

	default:
		t.Fatal(name)
	}

	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
