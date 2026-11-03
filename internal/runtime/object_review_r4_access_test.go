package runtime

import (
	"testing"
)

// REQ-012/018; T-023/024/035/036.
func w13R4AccessNames() []string {
	return []string{
		"r4-access-virtual-writeonly",
		"r4-access-self-isset",
		"r4-access-backed-uninitialized",
		"r4-access-backed-initialized",
		"r4-access-backed-default",
		"r4-access-getter-null",
		"r4-access-getter-value",
		"r4-access-getter-throw",
		"r4-access-magic-control",
		"r4-access-lazy-ghost",
		"r4-access-lazy-proxy",
		"r4-callable-magic-bare",
		"r4-callable-magic-array",
		"r4-callable-weak-strict-args",
		"r4-callable-weak-strict-returns",
		"r4-callable-private-invoke",
		"r4-callable-inherited-invoke",
	}
}

func w13R4AccessCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	owners := []value{}
	own := func(v value) value {
		var owner value
		must(t, h.assign(&owner, v))
		owners = append(owners, owner)
		return owner
	}
	defer func() {
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
		if len(h.live) != 0 {
			t.Fatal("access fixture retained request objects", len(h.live))
		}
	}()
	switch name {
	case "r4-access-virtual-writeonly":
		c := w13Build(t, classSpec{name: "W13R4VirtualWriteOnly", properties: []propertySpec{{name: "x", typeName: "string", virtual: true, set: func(*heap, value, value) (value, error) { return nullValue(), nil }}}})
		o := own(w13Instantiate(t, h, c))
		set := any("set")
		if err := h.writeObject(o, "x", w13Str("value"), nil, false); err != nil {
			set = w13Error(err)
		}
		isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "x", nil); return boolValue(yes), err })
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "x", nil) })
		result = []any{set, isset, read}
	case "r4-access-self-isset":
		var c *class
		c = w13Build(t, classSpec{name: "W13R4SelfIsset", properties: []propertySpec{{name: "inside", typeName: "bool", hasDefault: true, initial: false}, {name: "sets", typeName: "int", hasDefault: true, initial: int64(0)}, {name: "x", typeName: "string", set: func(h *heap, this, v value) (value, error) {
			if err := h.writeObject(this, "sets", incrementProperty(t, h, this, "sets"), c, false); err != nil {
				return value{}, err
			}
			yes, err := h.issetObject(this, "x", c)
			if err != nil {
				return value{}, err
			}
			if err = h.writeObject(this, "inside", boolValue(yes), c, false); err != nil {
				return value{}, err
			}
			return nullValue(), nil
		}}}})
		o := own(w13Instantiate(t, h, c))
		must(t, h.writeObject(o, "x", w13Str("value"), nil, false))
		sets := w13ObserveValue(func() (value, error) { return h.readObject(o, "sets", c) })
		inside := w13ObserveValue(func() (value, error) { return h.readObject(o, "inside", c) })
		outside := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "x", nil); return boolValue(yes), err })
		result = []any{sets, inside, outside}
	case "r4-access-backed-uninitialized", "r4-access-backed-initialized", "r4-access-backed-default":
		hasDefault := name == "r4-access-backed-default"
		var initial any
		if hasDefault {
			initial = int64(5)
		}
		className := "W13R4BackedWriteOnly"
		if hasDefault {
			className = "W13R4BackedDefault"
		}
		var c *class
		c = w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "x", typeName: "?int", hasDefault: hasDefault, initial: initial, set: func(h *heap, this, v value) (value, error) {
			return nullValue(), h.writeObject(this, "x", v, c, true)
		}}}})
		o := own(w13Instantiate(t, h, c))
		if name == "r4-access-backed-initialized" {
			must(t, h.writeObject(o, "x", w13Int(7), nil, false))
		}
		isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "x", nil); return boolValue(yes), err })
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "x", nil) })
		result = []any{isset, read}
	case "r4-access-getter-null", "r4-access-getter-value", "r4-access-getter-throw":
		events := []any{}
		throwing := name == "r4-access-getter-throw"
		input := nullValue()
		typ := "?int"
		if name == "r4-access-getter-value" {
			input = w13Int(4)
		}
		if throwing {
			typ = "int"
		}
		c := w13Build(t, classSpec{name: map[bool]string{true: "W13R4GetterThrow", false: "W13R4Getter"}[throwing], properties: []propertySpec{{name: "x", typeName: typ, virtual: true, get: func(*heap, value, value) (value, error) {
			events = append(events, "get")
			if throwing {
				return value{}, &Throwable{"RuntimeException", "getter"}
			}
			return input, nil
		}}}})
		o := own(w13Instantiate(t, h, c))
		isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "x", nil); return boolValue(yes), err })
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "x", nil) })
		result = []any{isset, read, events}
	case "r4-access-magic-control":
		events := []any{}
		c := w13Build(t, classSpec{name: "W13R4MagicControl", methods: []methodSpec{
			{name: "__isset", parameters: []methodParameter{{name: "name", typeName: "string"}}, returnType: "bool", call: func(*heap, value, []value) (value, error) {
				events = append(events, "isset:"+"x")
				return boolValue(true), nil
			}},
			{name: "__get", parameters: []methodParameter{{name: "name", typeName: "string"}}, returnType: "mixed", call: func(*heap, value, []value) (value, error) { events = append(events, "get:"+"x"); return w13Int(9), nil }},
		}})
		o := own(w13Instantiate(t, h, c))
		isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "x", nil); return boolValue(yes), err })
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "x", nil) })
		result = []any{isset, read, events}
	case "r4-access-lazy-ghost", "r4-access-lazy-proxy":
		events := []any{}
		c := w13Build(t, classSpec{name: "W13R4Lazy", properties: []propertySpec{{name: "stored", typeName: "int", hasDefault: true, initial: int64(0)}, {name: "virtual", typeName: "int", virtual: true, get: func(h *heap, this, _ value) (value, error) {
			events = append(events, "get")
			return h.readObject(this, "stored", nil)
		}}}})
		var o value
		if name == "r4-access-lazy-ghost" {
			v, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
				events = append(events, "initialize")
				return nullValue(), h.writeObject(this, "stored", w13Int(8), nil, false)
			})
			must(t, err)
			o = own(v)
			before := h.isLazyObject(o)
			isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "virtual", nil); return boolValue(yes), err })
			result = []any{before, isset, h.isLazyObject(o), events}
		} else {
			target := own(w13Instantiate(t, h, c))
			must(t, h.writeObject(target, "stored", w13Int(6), nil, false))
			v, err := h.newLazyObject(c, true, func(*heap, value) (value, error) { events = append(events, "initialize"); return target, nil })
			must(t, err)
			o = own(v)
			before := h.isLazyObject(o)
			isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(o, "virtual", nil); return boolValue(yes), err })
			result = []any{before, isset, !h.isLazyObject(o), events}
		}
	case "r4-callable-magic-bare", "r4-callable-magic-array", "r4-callable-weak-strict-args", "r4-callable-weak-strict-returns":
		magic := w13Build(t, classSpec{name: "W13R4MagicCallable", methods: []methodSpec{{name: "__call", parameters: []methodParameter{{name: "name", typeName: "string"}, {name: "args", typeName: "array"}}, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("magic"), nil }}}})
		invoke := w13Build(t, classSpec{name: "W13R4Callable", methods: []methodSpec{{name: "__invoke", returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("invoke"), nil }}}})
		var callableReturn, badCallable value
		sinkBase := w13Build(t, classSpec{name: "W13R4CallableSink", methods: []methodSpec{
			{name: "acceptWeak", parameters: []methodParameter{{name: "value", typeName: "callable"}}, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("body"), nil }},
			{name: "acceptStrict", parameters: []methodParameter{{name: "value", typeName: "callable"}}, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("body"), nil }},
			{name: "returnWeak", returnType: "callable", call: func(_ *heap, _ value, _ []value) (value, error) { return callableReturn, nil }},
			{name: "badReturnWeak", returnType: "callable", call: func(_ *heap, _ value, _ []value) (value, error) { return badCallable, nil }},
		}})
		sink := w13Build(t, classSpec{name: "W13R4StrictCallableSink", parent: sinkBase, methods: []methodSpec{
			{name: "returnStrict", returnType: "callable", returnStrict: true, call: func(_ *heap, _ value, _ []value) (value, error) { return callableReturn, nil }},
			{name: "badReturnStrict", returnType: "callable", returnStrict: true, call: func(_ *heap, _ value, _ []value) (value, error) { return badCallable, nil }},
		}})
		sinkObj := own(w13Instantiate(t, h, sink))
		magicObj := own(w13Instantiate(t, h, magic))
		invokeObj := own(w13Instantiate(t, h, invoke))
		var arrayCallable value
		must(t, h.appendValue(&arrayCallable, magicObj))
		must(t, h.appendValue(&arrayCallable, w13Str("missing")))
		// appendValue already owns this writable location; transfer it to cleanup.
		owners = append(owners, arrayCallable)
		switch name {
		case "r4-callable-magic-bare":
			arg := w13ObserveValue(func() (value, error) {
				return h.callMethod(sinkObj, "acceptWeak", []methodArgument{{value: magicObj}}, nil, false)
			})
			result = []any{h.isObjectCallable(magicObj, nil), arg}
		case "r4-callable-magic-array":
			arg := w13ObserveValue(func() (value, error) {
				return h.callMethod(sinkObj, "acceptWeak", []methodArgument{{value: arrayCallable}}, nil, false)
			})
			callableReturn = arrayCallable
			out, err := h.callMethod(sinkObj, "returnWeak", nil, nil, false)
			valid := false
			if err == nil {
				valid = h.isObjectCallable(out, nil)
				must(t, h.discardMethodReturn(out))
			}
			result = []any{h.isObjectCallable(arrayCallable, nil), arg, valid}
		case "r4-callable-weak-strict-args":
			weakGood := w13ObserveValue(func() (value, error) {
				return h.callMethod(sinkObj, "acceptWeak", []methodArgument{{value: invokeObj}}, nil, false)
			})
			strictGood := w13ObserveValue(func() (value, error) {
				return h.callMethodAt(sinkObj, "acceptStrict", []methodArgument{{value: invokeObj}}, nil, true, methodCallSite{file: "/w/review_r4_strict.php", line: 5})
			})
			weakBad := w13ObserveValue(func() (value, error) {
				return h.callMethod(sinkObj, "acceptWeak", []methodArgument{{value: magicObj}}, nil, false)
			})
			strictBad := w13ObserveValue(func() (value, error) {
				return h.callMethodAt(sinkObj, "acceptStrict", []methodArgument{{value: magicObj}}, nil, true, methodCallSite{file: "/w/review_r4_strict.php", line: 5})
			})
			result = []any{weakGood, strictGood, weakBad, strictBad}
		case "r4-callable-weak-strict-returns":
			callableReturn = invokeObj
			badCallable = magicObj
			weak, err := h.callMethod(sinkObj, "returnWeak", nil, nil, false)
			weakValid := err == nil && h.isObjectCallable(weak, nil)
			if err == nil {
				must(t, h.discardMethodReturn(weak))
			}
			strict, err := h.callMethod(sinkObj, "returnStrict", nil, nil, false)
			strictValid := err == nil && h.isObjectCallable(strict, nil)
			if err == nil {
				must(t, h.discardMethodReturn(strict))
			}
			badWeak := w13ObserveValue(func() (value, error) { return h.callMethod(sinkObj, "badReturnWeak", nil, nil, false) })
			badStrict := w13ObserveValue(func() (value, error) { return h.callMethod(sinkObj, "badReturnStrict", nil, nil, false) })
			result = []any{weakValid, strictValid, badWeak, badStrict}
		}
	case "r4-callable-private-invoke", "r4-callable-inherited-invoke":
		if name == "r4-callable-private-invoke" {
			private := w13Build(t, classSpec{name: "W13R4PrivateInvoke", methods: []methodSpec{{name: "__invoke", visibility: privateAccess, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("private"), nil }}}})
			protected := w13Build(t, classSpec{name: "W13R4ProtectedInvoke", methods: []methodSpec{{name: "__invoke", visibility: protectedAccess, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("protected"), nil }}}})
			privateObj := own(w13Instantiate(t, h, private))
			protectedObj := own(w13Instantiate(t, h, protected))
			privateInvoke := w13ObserveValue(func() (value, error) {
				return h.invokeMethod(privateObj, private.schema.methods["__invoke"], nil, nil, false, methodCallSite{})
			})
			protectedInvoke := w13ObserveValue(func() (value, error) {
				return h.invokeMethod(protectedObj, protected.schema.methods["__invoke"], nil, nil, false, methodCallSite{})
			})
			result = []any{[]any{h.isObjectCallable(privateObj, nil), privateInvoke}, []any{h.isObjectCallable(protectedObj, nil), protectedInvoke}}
		} else {
			parent := w13Build(t, classSpec{name: "W13R4ParentInvoke", methods: []methodSpec{{name: "__invoke", visibility: privateAccess, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("parent"), nil }}}})
			child := w13Build(t, classSpec{name: "W13R4ChildInvoke", parent: parent, methods: []methodSpec{{name: "__invoke", visibility: publicAccess, returnType: "string", call: func(*heap, value, []value) (value, error) { return w13Str("child"), nil }}}})
			childObj := own(w13Instantiate(t, h, child))
			invoke := w13ObserveValue(func() (value, error) {
				return h.invokeMethod(childObj, child.schema.methods["__invoke"], nil, nil, false, methodCallSite{})
			})
			result = []any{h.isObjectCallable(childObj, nil), invoke}
		}
	default:
		t.Fatalf("unknown W13 R4 access case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func incrementProperty(t *testing.T, h *heap, obj value, name string) value {
	t.Helper()
	v, err := h.readObject(obj, name, nil)
	must(t, err)
	return w13Int(v.deref().integer + 1)
}
