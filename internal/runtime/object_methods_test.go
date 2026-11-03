package runtime

import (
	"encoding/json"
	"testing"
)

type w13MethodFixtures struct {
	methods, plain, iface *class
}

func w13BuildMethodFixtures(t *testing.T) *w13MethodFixtures {
	t.Helper()
	iface := w13Build(t, classSpec{
		name:        "W13Interface",
		isInterface: true,
		methods: []methodSpec{
			{name: "hello", returnType: "string", abstract: true},
		},
	})
	trait := w13Build(t, classSpec{
		name:    "W13Trait",
		isTrait: true,
		methods: []methodSpec{
			{
				name:       "hello",
				returnType: "string",
				call: func(*heap, value, []value) (value, error) {
					return w13Str("trait"), nil
				},
			},
		},
	})
	methods := w13Build(t, classSpec{
		name:       "W13Methods",
		interfaces: []*class{iface},
		traits:     []*class{trait},
		properties: []propertySpec{
			{name: "counter", typeName: "int", static: true, hasDefault: true, initial: int64(0)},
		},
		methods: []methodSpec{
			{
				name:       "sum",
				returnType: "int",
				parameters: []methodParameter{
					{name: "a", typeName: "int"},
					{name: "b", typeName: "int", hasDefault: true, initial: int64(2)},
				},
				call: func(h *heap, _ value, args []value) (value, error) {
					if len(args) != 2 {
						return value{}, &Throwable{"ArgumentCountError", "W13Methods::sum() expects two bound arguments"}
					}
					return h.arith("+", args[0], args[1])
				},
			},
			{
				name:       "bump",
				returnType: "int",
				parameters: []methodParameter{
					{name: "value", typeName: "int", byReference: true},
				},
				call: func(h *heap, _ value, args []value) (value, error) {
					if len(args) != 1 {
						return value{}, &Throwable{"ArgumentCountError", "W13Methods::bump() expects one bound argument"}
					}
					next, err := h.arith("+", args[0], w13Int(1))
					if err != nil {
						return value{}, err
					}
					if err := h.assign(&args[0], next); err != nil {
						return value{}, err
					}
					return next, nil
				},
			},
			{
				name:       "hidden",
				returnType: "string",
				visibility: privateAccess,
				call: func(*heap, value, []value) (value, error) {
					return w13Str("hidden"), nil
				},
			},
			{
				name:       "guarded",
				returnType: "string",
				visibility: protectedAccess,
				call: func(*heap, value, []value) (value, error) {
					return w13Str("guarded"), nil
				},
			},
			{
				name:       "__call",
				returnType: "string",
				parameters: []methodParameter{
					{name: "name", typeName: "string"},
					{name: "args", typeName: "array"},
				},
				call: func(_ *heap, _ value, args []value) (value, error) {
					if len(args) != 2 {
						return value{}, &Throwable{"Error", "W13Methods::__call() expects name and arguments"}
					}
					encoded, err := json.Marshal(w13Encode(args[1]))
					if err != nil {
						return value{}, err
					}
					return w13Str(args[0].deref().text + ":" + string(encoded)), nil
				},
			},
		},
	})
	plain := w13Build(t, classSpec{
		name: "W13PlainMethods",
		methods: []methodSpec{
			{
				name:       "hidden",
				returnType: "void",
				visibility: privateAccess,
				call: func(*heap, value, []value) (value, error) {
					return value{}, nil
				},
			},
		},
	})
	return &w13MethodFixtures{methods: methods, plain: plain, iface: iface}
}

func w13RunMethodCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	f := w13BuildMethodFixtures(t)
	var result any
	switch name {
	case "trait-interface":
		o := w13Instantiate(t, h, f.methods)
		hello, err := h.callMethod(o, "hello", nil, nil, true)
		w13Must(t, "callMethod(hello)", err)
		oo, err := h.checkedObject(o)
		w13Must(t, "checkedObject", err)
		result = []any{w13Encode(hello), classIs(oo.class, f.iface)}

	case "method-case":
		o := w13Instantiate(t, h, f.methods)
		v, err := h.callMethod(o, "HeLlO", nil, nil, true)
		w13Must(t, "callMethod(HeLlO)", err)
		result = w13Encode(v)

	case "method-named":
		o := w13Instantiate(t, h, f.methods)
		v, err := h.callMethod(o, "sum", []methodArgument{
			{name: "b", value: w13Int(4)},
			{name: "a", value: w13Int(3)},
		}, nil, true)
		w13Must(t, "callMethod(sum b:4,a:3)", err)
		result = w13Encode(v)

	case "method-default":
		o := w13Instantiate(t, h, f.methods)
		v, err := h.callMethod(o, "sum", []methodArgument{
			{name: "a", value: w13Int(3)},
		}, nil, true)
		w13Must(t, "callMethod(sum a:3)", err)
		result = w13Encode(v)

	case "method-missing":
		o := w13Instantiate(t, h, f.methods)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethodAt(o, "sum", nil, nil, true, methodCallSite{file: "/w/methods.php", line: 19})
		})

	case "method-type":
		o := w13Instantiate(t, h, f.methods)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethodAt(o, "sum", []methodArgument{
				{name: "a", value: w13Str("x")},
			}, nil, true, methodCallSite{file: "/w/methods.php", line: 20})
		})

	case "method-unknown-name":
		o := w13Instantiate(t, h, f.methods)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethod(o, "sum", []methodArgument{
				{name: "unknown", value: w13Int(1)},
			}, nil, true)
		})

	case "method-duplicate-name":
		o := w13Instantiate(t, h, f.methods)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethod(o, "sum", []methodArgument{
				{name: "", value: w13Int(1)},
				{name: "a", value: w13Int(2)},
			}, nil, true)
		})

	case "method-reference":
		o := w13Instantiate(t, h, f.methods)
		var v value
		w13Must(t, "assign v", h.assign(&v, w13Int(1)))
		got, err := h.callMethod(o, "bump", []methodArgument{
			{name: "value", value: v.deref(), slot: &v},
		}, nil, true)
		w13Must(t, "callMethod(bump slot)", err)
		result = []any{w13Encode(got), w13Encode(v)}

	case "method-reference-literal":
		o := w13Instantiate(t, h, f.methods)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethod(o, "bump", []methodArgument{
				{name: "value", value: w13Int(1)},
			}, nil, true)
		})

	case "method-magic":
		o := w13Instantiate(t, h, f.methods)
		missing, err := h.callMethod(o, "missing", []methodArgument{
			{name: "", value: w13Int(1)},
			{name: "", value: w13Int(2)},
		}, nil, true)
		w13Must(t, "callMethod(missing)", err)
		hidden, err := h.callMethod(o, "hidden", nil, nil, true)
		w13Must(t, "callMethod(hidden)", err)
		result = []any{w13Encode(missing), w13Encode(hidden)}

	case "method-private":
		o := w13Instantiate(t, h, f.plain)
		result = w13ObserveValue(func() (value, error) {
			return h.callMethod(o, "hidden", nil, nil, true)
		})

	case "static-state":
		w13Must(t, "writeStatic(counter,3)", h.writeStatic(f.methods, "counter", w13Int(3), nil, true))
		v, err := h.readStatic(f.methods, "counter", nil)
		w13Must(t, "readStatic(counter)", err)
		w13Must(t, "writeStatic(counter,0)", h.writeStatic(f.methods, "counter", w13Int(0), nil, true))
		result = w13Encode(v)

	default:
		t.Fatalf("unknown W-13 method case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13MethodCaseNames() []string {
	return []string{
		"trait-interface",
		"method-case",
		"method-named",
		"method-default",
		"method-missing",
		"method-type",
		"method-unknown-name",
		"method-duplicate-name",
		"method-reference",
		"method-reference-literal",
		"method-magic",
		"method-private",
		"static-state",
	}
}

func TestW13MethodsRequireDispatch(t *testing.T) {
	h, err := newHeap(64)
	w13Must(t, "newHeap", err)
	f := w13BuildMethodFixtures(t)
	o := w13Instantiate(t, h, f.methods)

	cases := []struct {
		name string
		call func() (value, error)
		want value
	}{
		{
			name: "sum named",
			want: w13Int(7),
			call: func() (value, error) {
				return h.callMethod(o, "sum", []methodArgument{
					{name: "b", value: w13Int(4)},
					{name: "a", value: w13Int(3)},
				}, nil, true)
			},
		},
		{
			name: "trait hello",
			want: w13Str("trait"),
			call: func() (value, error) {
				return h.callMethod(o, "hello", nil, nil, true)
			},
		},
		{
			name: "static write",
			call: func() (value, error) {
				return value{}, h.writeStatic(f.methods, "counter", w13Int(3), nil, true)
			},
		},
		{
			name: "static read",
			want: w13Int(3),
			call: func() (value, error) {
				return h.readStatic(f.methods, "counter", nil)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err != nil {
				t.Fatal(err)
			}
			if same, err := identical(got, tc.want); err != nil || !same {
				t.Fatalf("got %#v, want %#v: %v", got, tc.want, err)
			}
		})
	}
}
