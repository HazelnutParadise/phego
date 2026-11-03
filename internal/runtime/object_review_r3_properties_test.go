package runtime

import "testing"

func w13R3PropertyNames() []string {
	return []string{"r3-property-private-inherited", "r3-property-private-static", "r3-property-lazy-ghost", "r3-property-enum-readonly", "r3-property-leading-nul", "r3-property-mangled-nul"}
}

func w13R3PropertyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	switch name {
	case "r3-property-private-inherited":
		owners := []value{}
		own := func(v value) value {
			var owner value
			must(t, h.assign(&owner, v))
			owners = append(owners, owner)
			return owner
		}
		parent := w13Build(t, classSpec{name: "W13R3PrivateParent", properties: []propertySpec{{name: "x", visibility: privateAccess, hasDefault: true, initial: int64(1)}}})
		var child *class
		child = w13Build(t, classSpec{name: "W13R3PrivateChild", parent: parent, allowDynamic: true, methods: []methodSpec{{name: "readParent", call: func(h *heap, this value, _ []value) (value, error) { return h.readObject(this, "x", parent) }}, {name: "readChild", call: func(h *heap, this value, _ []value) (value, error) { return h.readObject(this, "x", child) }}, {name: "writeParent", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.writeObject(this, "x", w13Int(4), parent, false)
		}}, {name: "refParent", call: func(h *heap, this value, _ []value) (value, error) {
			var ref value
			if err := h.referenceObject(&ref, this, "x", parent); err != nil {
				return value{}, err
			}
			if err := h.assignPHP(&ref, w13Int(6), false); err != nil {
				_ = h.release(ref)
				return value{}, err
			}
			if err := h.release(ref); err != nil {
				return value{}, err
			}
			return h.readObject(this, "x", parent)
		}}}})
		makeObj := func() value { return own(w13Instantiate(t, h, child)) }
		global := makeObj()
		read := w13ObserveValue(func() (value, error) { return h.readObject(global, "x", nil) })
		write := w13ObserveError(func() error { return h.writeObject(global, "x", w13Int(2), nil, false) })
		ref := w13ObserveValue(func() (value, error) {
			var v value
			err := h.referenceObject(&v, global, "x", nil)
			out := v.deref()
			return out, h.mergeObjectErrors(err, h.release(v))
		})
		isset := w13ObserveValue(func() (value, error) { yes, err := h.issetObject(global, "x", nil); return boolValue(yes), err })
		unset := w13ObserveError(func() error { return h.unsetObject(global, "x", nil) })
		parentObj := makeObj()
		parentRead := w13ObserveValue(func() (value, error) { return h.callMethod(parentObj, "readParent", nil, nil, false) })
		parentWrite := w13ObserveValue(func() (value, error) { return h.callMethod(parentObj, "writeParent", nil, nil, false) })
		parentAfter := w13ObserveValue(func() (value, error) { return h.readObject(parentObj, "x", parent) })
		childObj := makeObj()
		childScopeRead := w13ObserveValue(func() (value, error) { return h.callMethod(childObj, "readChild", nil, nil, false) })
		childWrite := w13ObserveError(func() error { return h.writeObject(childObj, "x", w13Int(9), nil, false) })
		parentStill := w13ObserveValue(func() (value, error) { return h.readObject(childObj, "x", parent) })
		dynamic := w13ObserveValue(func() (value, error) { return h.readObject(childObj, "x", child) })
		parentReference := w13ObserveValue(func() (value, error) { return h.callMethod(parentObj, "refParent", nil, nil, false) })
		result = []any{read, write, ref, isset, unset, parentRead, parentWrite, parentAfter, parentReference, childScopeRead, childWrite, parentStill, dynamic}
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
	case "r3-property-private-static":
		parent := w13Build(t, classSpec{name: "W13R3StaticParent", properties: []propertySpec{{name: "x", static: true, visibility: privateAccess, hasDefault: true, initial: int64(1)}}})
		child := w13Build(t, classSpec{name: "W13R3StaticChild", parent: parent})
		result = w13ObserveValue(func() (value, error) {
			p, err := h.resolveStatic(child, "x", nil)
			if err != nil {
				return value{}, err
			}
			if !canAccess(nil, p.owner, p.visibility) {
				return value{}, &Throwable{"Error", "Cannot access private property W13R3StaticChild::$x"}
			}
			return value{}, nil
		})
	case "r3-property-lazy-ghost":
		owners := []value{}
		c := w13Build(t, classSpec{name: "W13R3Lazy", allowDynamic: true, properties: []propertySpec{{name: "value", typeName: "?int"}}})
		calls := 0
		makeGhost := func(mode string) value {
			o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
				calls++
				switch mode {
				case "value":
					if err := h.writeObject(this, "dynamic", w13Int(7), nil, false); err != nil {
						return value{}, err
					}
					return nullValue(), h.writeObject(this, "value", w13Int(7), nil, false)
				case "null":
					return nullValue(), h.writeObject(this, "value", nullValue(), nil, false)
				case "throw":
					return value{}, &Throwable{"Exception", "initializer"}
				default:
					return nullValue(), nil
				}
			})
			must(t, err)
			var owner value
			must(t, h.assign(&owner, o))
			owners = append(owners, owner)
			return owner
		}
		isLazy := func(v value) bool { return v.object.lazy != nil && v.object.lazy.initialize != nil }
		undeclared := makeGhost("value")
		u, err := h.issetObject(undeclared, "dynamic", nil)
		must(t, err)
		before := isLazy(undeclared)
		vals := []any{u, before}
		v := w13ObserveValue(func() (value, error) { return h.readObject(undeclared, "value", nil) })
		vals = append(vals, v, isLazy(undeclared))
		for _, mode := range []string{"null", "missing", "throw"} {
			o := makeGhost(mode)
			v := w13ObserveValue(func() (value, error) { return h.readObject(o, "value", nil) })
			vals = append(vals, v, isLazy(o))
		}
		proxyTarget := w13Instantiate(t, h, c)
		var proxyTargetOwner value
		must(t, h.assign(&proxyTargetOwner, proxyTarget))
		owners = append(owners, proxyTargetOwner)
		must(t, h.writeObject(proxyTarget, "dynamic", w13Int(11), nil, false))
		proxy, err := h.newLazyObject(c, true, func(*heap, value) (value, error) { return proxyTarget, nil })
		must(t, err)
		var proxyOwner value
		must(t, h.assign(&proxyOwner, proxy))
		owners = append(owners, proxyOwner)
		proxyValue := w13ObserveValue(func() (value, error) { return h.readObject(proxy, "dynamic", nil) })
		set, err := h.issetObject(proxy, "dynamic", nil)
		must(t, err)
		result = append(vals, proxyValue, set, isLazy(proxy), calls)
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
	case "r3-property-enum-readonly":
		pure := w13Build(t, classSpec{name: "W13R3PureEnum", enumCases: []string{"A", "B"}})
		backed := w13Build(t, classSpec{name: "W13R3BackedEnum", enumCases: []string{"A", "B"}, enumValues: []any{[]byte("a"), []byte("b")}})
		inspect := func(c *class, caseName string, properties []string) []any {
			v, err := h.enumCase(c, caseName)
			must(t, err)
			items := []any{w13ObjectAsMap(v)}
			for _, p := range properties {
				items = append(items, w13ObserveError(func() error { return h.writeObject(v, p, w13Str("changed"), nil, false) }), w13ObserveError(func() error { return h.unsetObject(v, p, nil) }), func() any {
					var ref value
					e := w13ObserveError(func() error { return h.referenceObject(&ref, v, p, nil) })
					_ = h.release(ref)
					return e
				}())
			}
			items = append(items, w13ObjectAsMap(v))
			items = append(items, w13ObserveError(func() error { return h.writeObject(v, "extra", w13Int(1), nil, false) }), w13Isset(h, v, "extra", nil))
			return items
		}
		boxClass := w13Build(t, classSpec{name: "W13R3EnumBox", properties: []propertySpec{{name: "unit", typeName: "UnitEnum"}, {name: "backed", typeName: "BackedEnum"}}})
		box := w13Instantiate(t, h, boxClass)
		must(t, h.writeObject(box, "unit", mustEnumCase(t, h, pure, "A"), nil, true))
		must(t, h.writeObject(box, "backed", mustEnumCase(t, h, backed, "A"), nil, true))
		bad := w13ObserveError(func() error { return h.writeObject(box, "backed", mustEnumCase(t, h, pure, "A"), nil, true) })
		result = []any{inspect(pure, "A", []string{"name"}), inspect(backed, "A", []string{"name", "value"}), w13ObjectAsMap(mustEnumCase(t, h, backed, "A")), w13Encode(w13Read(t, h, box, "unit", nil)), w13Encode(w13Read(t, h, box, "backed", nil)), bad}
	case "r3-property-leading-nul":
		events := []any{}
		c := w13Build(t, classSpec{name: "W13R3Nul", allowDynamic: true, properties: []propertySpec{{name: "x", visibility: privateAccess, hasDefault: true, initial: int64(1)}, {name: "y", visibility: protectedAccess, hasDefault: true, initial: int64(2)}}, methods: []methodSpec{{name: "__get", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) {
			events = append(events, "get")
			return w13Str("magic"), nil
		}}, {name: "__set", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "set"); return nullValue(), nil }}, {name: "__isset", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "isset"); return w13Int(1), nil }}, {name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(*heap, value, []value) (value, error) { events = append(events, "unset"); return nullValue(), nil }}}})
		o := w13Instantiate(t, h, c)
		observe := func(key string) []any {
			return []any{w13ObserveValue(func() (value, error) { return h.readObject(o, key, nil) }), w13ObserveError(func() error { return h.writeObject(o, key, w13Int(8), nil, false) }), func() any {
				b, e := h.issetObject(o, key, nil)
				if e != nil {
					return w13Error(e)
				}
				return b
			}(), w13ObserveError(func() error { return h.unsetObject(o, key, nil) }), func() any {
				return w13ObserveValue(func() (value, error) {
					var ref value
					err := h.referenceObject(&ref, o, key, nil)
					out := ref.deref()
					return out, h.mergeObjectErrors(err, h.release(ref))
				})
			}()}
		}
		result = []any{observe("\x00x"), observe("\x00*y"), observe("x"), events, w13ObjectAsMap(o)}
	case "r3-property-mangled-nul":
		c := w13Build(t, classSpec{name: "W13R3NulPlain", allowDynamic: true, properties: []propertySpec{{name: "secret", typeName: "int", visibility: privateAccess, hasDefault: true, initial: int64(1)}, {name: "p", typeName: "int", visibility: protectedAccess, hasDefault: true, initial: int64(2)}}})
		out := []any{}
		for _, key := range []string{"\x00W13R3NulPlain\x00secret", "\x00*\x00p"} {
			o := w13Instantiate(t, h, c)
			out = append(out, []any{w13ObserveValue(func() (value, error) { return h.readObject(o, key, nil) }), w13ObserveError(func() error { return h.writeObject(o, key, w13Str("bad"), nil, false) }), w13ObserveValue(func() (value, error) { v, e := h.issetObject(o, key, nil); return boolValue(v), e }), w13ObserveError(func() error { return h.unsetObject(o, key, nil) }), w13ObserveValue(func() (value, error) {
				var r value
				e := h.referenceObject(&r, o, key, nil)
				v := r.deref()
				return v, h.mergeObjectErrors(e, h.release(r))
			}), w13ObserveValue(func() (value, error) { return h.readObject(o, "secret", c) }), w13ObserveValue(func() (value, error) { return h.readObject(o, "p", c) })})
		}
		o := w13Instantiate(t, h, c)
		for _, key := range []string{"", "a\x00b"} {
			must(t, h.writeObject(o, key, w13Str("ok"), nil, false))
			out = append(out, w13Encode(w13Read(t, h, o, key, nil)))
		}
		result = out
	default:
		t.Fatalf("unknown case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func mustEnumCase(t *testing.T, h *heap, c *class, name string) value {
	t.Helper()
	v, err := h.enumCase(c, name)
	must(t, err)
	return v
}
