package runtime

import (
	"errors"
	"testing"
)

// REQ-012/018/020/021; T-023/024/035/036/039/040/041/042.
func w13R5OperationNames() []string {
	return []string{
		"r5-op-missing-invalid-order", "r5-op-hidden-invalid-order",
		"r5-op-valid-positional-before-named", "r5-op-named-only",
		"r5-op-ordinary-positional", "r5-op-ordinary-invalid-order",
		"r5-op-getter-uninitialized", "r5-op-getter-null", "r5-op-getter-initialized",
		"r5-op-proxy-clone-throws", "r5-op-ordinary-clone-failure", "r5-op-proxy-clone-dtor-chain",
		"r5-op-clone-source-reference", "r5-op-clone-reference-detach", "r5-op-clone-dynamic-reference",
	}
}

func w13R5OperationCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	owners := []value{}
	own := func(v value) value { var x value; must(t, h.assign(&x, v)); owners = append(owners, x); return x }
	cleaned := false
	cleanup := func() {
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
		cleaned = true
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatalf("R5 case retained objects=%d sources=%d", h.liveObjects, len(h.typedSources))
		}
	}
	defer func() {
		if !cleaned {
			cleanup()
		}
	}()
	args := func(parts ...methodArgument) []methodArgument { return parts }
	methodClass := func() *class {
		return w13Build(t, classSpec{name: "W13R5Ops", methods: []methodSpec{
			{name: "__call", parameters: []methodParameter{{name: "name", typeName: "string"}, {name: "args", typeName: "array"}}, returnType: "array", call: func(h *heap, _ value, a []value) (value, error) {
				out := h.newArrayValue()
				retain(a[0])
				retain(a[1])
				out.array.insert(intKey(0), a[0], h.width)
				out.array.insert(intKey(1), a[1], h.width)
				return out, nil
			}},
			{name: "hidden", visibility: privateAccess, call: func(*heap, value, []value) (value, error) { return w13Str("hidden"), nil }},
			{name: "normal", parameters: []methodParameter{{name: "first", typeName: "mixed"}, {name: "second", typeName: "mixed", hasDefault: true, initial: nil}}, returnType: "array", call: func(h *heap, _ value, a []value) (value, error) {
				out := h.newArrayValue()
				for i, v := range a {
					retain(v)
					out.array.insert(intKey(int64(i)), v, h.width)
				}
				return out, nil
			}},
		}})
	}
	caseCall := func(obj value, method string, supplied []methodArgument) any {
		v, err := h.callMethodAt(obj, method, supplied, nil, false, methodCallSite{})
		if err != nil {
			return w13Error(err)
		}
		observed := w13Encode(v)
		must(t, h.discardMethodReturn(v))
		return observed
	}
	switch name {
	case "r5-op-missing-invalid-order", "r5-op-hidden-invalid-order", "r5-op-valid-positional-before-named", "r5-op-named-only", "r5-op-ordinary-positional", "r5-op-ordinary-invalid-order":
		events := []any{}
		// Magic-call events are observed only when argument binding succeeds.
		c := methodClass()
		magic := c.schema.methods["__call"]
		old := magic.call
		magic.call = func(h *heap, this value, a []value) (value, error) {
			events = append(events, "magic")
			return old(h, this, a)
		}
		o := own(w13Instantiate(t, h, c))
		invalid := args(methodArgument{name: "first", value: w13Int(1)}, methodArgument{value: w13Int(2)})
		switch name {
		case "r5-op-missing-invalid-order":
			result = []any{caseCall(o, "missing", invalid), events}
		case "r5-op-hidden-invalid-order":
			result = []any{caseCall(o, "hidden", invalid), events}
		case "r5-op-valid-positional-before-named":
			result = caseCall(o, "missing", args(methodArgument{value: w13Int(1)}, methodArgument{name: "second", value: w13Int(2)}))
		case "r5-op-named-only":
			result = caseCall(o, "missing", args(methodArgument{name: "first", value: w13Int(1)}))
		case "r5-op-ordinary-positional":
			result = caseCall(o, "normal", args(methodArgument{value: w13Int(1)}, methodArgument{value: w13Int(2)}))
		case "r5-op-ordinary-invalid-order":
			result = caseCall(o, "normal", invalid)
		}
	case "r5-op-getter-uninitialized", "r5-op-getter-null", "r5-op-getter-initialized":
		calls := 0
		typ := "?int"
		initial := any(nil)
		hasDefault := false
		if name == "r5-op-getter-null" {
			hasDefault = true
		}
		if name == "r5-op-getter-initialized" {
			typ = "int"
			hasDefault = true
			initial = int64(7)
		}
		className := "W13R5Getter"
		if name == "r5-op-getter-null" {
			className = "W13R5NullableGetter"
		}
		if name == "r5-op-getter-initialized" {
			className = "W13R5InitializedGetter"
		}
		if name == "r5-op-getter-uninitialized" {
			typ = "int"
		}
		c := w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "x", typeName: typ, hasDefault: hasDefault, initial: initial, get: func(h *heap, this, _ value) (value, error) {
			calls++
			yes, e := h.issetObject(this, "x", nil)
			if e != nil {
				return value{}, e
			}
			if yes {
				return h.readObject(this, "x", nil)
			}
			return w13Int(42), nil
		}}}})
		o := own(w13Instantiate(t, h, c))
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "x", nil) })
		isset := w13ObserveValue(func() (value, error) { v, e := h.issetObject(o, "x", nil); return boolValue(v), e })
		result = []any{read, isset, int64(calls)}
	case "r5-op-proxy-clone-throws", "r5-op-ordinary-clone-failure", "r5-op-proxy-clone-dtor-chain":
		events := []any{}
		throwDtor := name == "r5-op-proxy-clone-dtor-chain"
		dtorArmed := throwDtor
		c := w13Build(t, classSpec{name: "W13R5ProxyTarget", properties: []propertySpec{{name: "name", typeName: "string"}}, methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			v, e := h.readObject(this, "name", nil)
			if e != nil {
				return value{}, e
			}
			events = append(events, "clone:"+v.deref().text)
			if name == "r5-op-proxy-clone-dtor-chain" {
				return value{}, completion{kind: endThrow, err: &Throwable{"Exception", "clone failure"}, previous: &Throwable{"RuntimeException", "previous"}}
			}
			return value{}, &Throwable{"Exception", "clone"}
		}}}, destructor: func(h *heap, this value) error {
			v, e := h.readObject(this, "name", nil)
			if e == nil {
				events = append(events, "dtor:"+v.deref().text)
			}
			if dtorArmed && e == nil && v.deref().text == "source" {
				return &Throwable{"Exception", "destructor failure"}
			}
			return e
		}})
		target := own(w13Instantiate(t, h, c))
		must(t, h.writeObject(target, "name", w13Str("source"), nil, false))
		var proxy value
		if name != "r5-op-ordinary-clone-failure" {
			v, e := h.newLazyObject(c, true, func(*heap, value) (value, error) {
				events = append(events, "factory")
				return target, nil
			})
			must(t, e)
			proxy = own(v)
		} else {
			proxy = target
		}
		cloneErr := error(nil)
		cloned, err := h.cloneObject(proxy, nil)
		cloneErr = err
		var chain any
		var end completion
		if errors.As(cloneErr, &end) && end.previous != nil {
			chain = w13R5PreviousChain(end.previous)
		}
		obs := w13Error(cloneErr)
		if cloneErr == nil {
			cloned = own(cloned)
			obs = w13Encode(cloned)
		}
		snapshot := append([]any{}, events...)
		dtorArmed = false
		result = []any{obs, snapshot, chain}
		cleanup()
	case "r5-op-clone-source-reference", "r5-op-clone-reference-detach":
		c := w13Build(t, classSpec{name: "W13R5Typed", properties: []propertySpec{{name: "x", typeName: "int"}}})
		src := own(w13Instantiate(t, h, c))
		must(t, h.writeObject(src, "x", w13Int(1), nil, false))
		var alias value
		must(t, h.referenceObject(&alias, src, "x", nil))
		owners = append(owners, alias)
		clone := own(ownMustClone(t, h, src))
		if name == "r5-op-clone-source-reference" {
			must(t, h.unsetObject(src, "x", nil))
			bad := w13ObserveError(func() error { return h.assignPHP(&alias, w13Str("bad"), false) })
			result = []any{bad, w13Encode(alias)}
		} else {
			must(t, h.unsetObject(clone, "x", nil))
			bad := w13ObserveError(func() error { return h.assignPHP(&alias, w13Str("bad"), false) })
			must(t, h.unsetObject(src, "x", nil))
			write := w13ObserveError(func() error { return h.assignPHP(&alias, w13Str("free"), false) })
			if write == nil {
				write = "written"
			}
			result = []any{bad, write, w13Encode(alias)}
		}
	case "r5-op-clone-dynamic-reference":
		c := w13Build(t, classSpec{name: "W13R5DynamicRef", allowDynamic: true, properties: []propertySpec{{name: "s", static: true, typeName: "int", hasDefault: true, initial: int64(1)}}})
		src := own(w13Instantiate(t, h, c))
		must(t, h.writeObject(src, "s", w13Int(5), nil, false))
		var alias value
		must(t, h.referenceObject(&alias, src, "s", nil))
		owners = append(owners, alias)
		clone := own(ownMustClone(t, h, src))
		write := w13ObserveError(func() error { return h.assignPHP(&alias, w13Str("free"), false) })
		if write == nil {
			write = "written"
		}
		snap := w13ObjectAsMap(clone)
		result = []any{write, snap, w13Encode(alias)}
	default:
		t.Fatalf("unknown case %q", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func ownMustClone(t *testing.T, h *heap, src value) value {
	t.Helper()
	v, e := h.cloneObject(src, nil)
	must(t, e)
	return v
}

func w13R5PreviousChain(err error) []any {
	chain := []any{}
	for err != nil {
		var c completion
		if errors.As(err, &c) {
			if c.err != nil {
				var php *Throwable
				if errors.As(c.err, &php) {
					chain = append(chain, []any{php.Class, php.Message})
				}
			}
			err = c.previous
			continue
		}
		var php *Throwable
		if errors.As(err, &php) {
			chain = append(chain, []any{php.Class, php.Message})
		}
		break
	}
	return chain
}
