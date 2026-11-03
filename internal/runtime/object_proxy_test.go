package runtime

import (
	"strconv"
	"testing"
)

func w13ProxyBaseProperties() []propertySpec {
	return []propertySpec{
		{name: "value", typeName: "int"},
		{name: "label", typeName: "string", hasDefault: true, initial: []byte("base")},
	}
}

func w13ProxyBase(t *testing.T) *class {
	t.Helper()
	return w13Build(t, classSpec{name: "W13ProxyBase", properties: w13ProxyBaseProperties()})
}

func w13ProxyArray(h *heap, values ...value) (value, error) {
	array := h.newArrayValue()
	for _, v := range values {
		if err := h.appendValue(&array, v); err != nil {
			return value{}, err
		}
	}
	return array, nil
}

func w13ProxyPropertyInitialized(obj value, name string) bool {
	slot, ok := obj.deref().object.props.lookup(name)
	return ok && slot.kind != valueUndefined
}

func w13ProxyCaseNames() []string {
	return []string{
		"proxy-scalar-factory",
		"proxy-incompatible-class",
		"proxy-incompatible-lazy-class-priority",
		"proxy-real-subclass-rejected",
		"proxy-parent-target-compatible",
		"proxy-extra-property-rejected",
		"proxy-clone-override-rejected",
		"proxy-destructor-override-rejected",
		"proxy-self-target-rejected",
		"proxy-uninitialized-ghost-target-rejected",
		"proxy-initialized-ghost-target-accepted",
		"proxy-initialized-proxy-target-rejected",
		"proxy-wrapper-method-this",
		"proxy-wrapper-clone-backing",
		"proxy-factory-failure-property-write",
		"proxy-factory-reads-defaults",
	}
}

func w13RunProxyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var result any
	switch name {
	case "proxy-scalar-factory":
		base := w13ProxyBase(t)
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return w13Int(7), nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-incompatible-class":
		base := w13ProxyBase(t)
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return w13NewStdClass(t, h), nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-incompatible-lazy-class-priority":
		base := w13ProxyBase(t)
		unrelated := w13Build(t, classSpec{name: "W13ProxyUnrelated"})
		ghost, err := h.newLazyObject(unrelated, false, func(*heap, value) (value, error) {
			return nullValue(), nil
		})
		w13Must(t, "newLazyObject(unrelated ghost)", err)
		var targetOwner value
		w13Must(t, "own unrelated ghost", h.assign(&targetOwner, ghost))
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-real-subclass-rejected":
		base := w13ProxyBase(t)
		child := w13Build(t, classSpec{name: "W13ProxyChild", parent: base})
		var targetOwner value
		w13Must(t, "own child target", h.assign(&targetOwner, w13Instantiate(t, h, child)))
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-parent-target-compatible":
		base := w13ProxyBase(t)
		wrapper := w13Build(t, classSpec{name: "W13ProxyWrapper", parent: base})
		real := w13Instantiate(t, h, base)
		w13Must(t, "initialize parent target", h.writeObject(real, "value", w13Int(9), nil, true))
		var targetOwner value
		w13Must(t, "own parent target", h.assign(&targetOwner, real))
		proxy, err := h.newLazyObject(wrapper, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = []any{
			proxy.object.class.name,
			w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) }),
			targetOwner.object.class.name,
		}

	case "proxy-extra-property-rejected":
		base := w13ProxyBase(t)
		extra := w13Build(t, classSpec{name: "W13ProxyExtra", parent: base, properties: []propertySpec{{name: "extra", typeName: "int"}}})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own parent target", h.assign(&targetOwner, real))
		proxy, err := h.newLazyObject(extra, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-clone-override-rejected":
		var base, wrapper *class
		base = w13Build(t, classSpec{name: "W13ProxyCloneBase", properties: []propertySpec{{name: "value", typeName: "int"}}, methods: []methodSpec{{name: "__clone", call: func(*heap, value, []value) (value, error) { return nullValue(), nil }}}})
		wrapper = w13Build(t, classSpec{name: "W13ProxyCloneOverride", parent: base, methods: []methodSpec{{name: "__clone", call: func(*heap, value, []value) (value, error) { return nullValue(), nil }}}})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own clone target", h.assign(&targetOwner, real))
		proxy, err := h.newLazyObject(wrapper, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-destructor-override-rejected":
		base := w13Build(t, classSpec{name: "W13ProxyDestructorBase", properties: []propertySpec{{name: "value", typeName: "int"}}, destructor: func(*heap, value) error { return nil }})
		wrapper := w13Build(t, classSpec{name: "W13ProxyDestructorOverride", parent: base, destructor: func(*heap, value) error { return nil }})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own destructor target", h.assign(&targetOwner, real))
		proxy, err := h.newLazyObject(wrapper, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-self-target-rejected":
		var proxy value
		var err error
		base := w13ProxyBase(t)
		proxy, err = h.newLazyObject(base, true, func(*heap, value) (value, error) { return proxy, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-uninitialized-ghost-target-rejected":
		base := w13ProxyBase(t)
		ghost, err := h.newLazyObject(base, false, func(h *heap, obj value) (value, error) {
			return nullValue(), h.writeObject(obj, "value", w13Int(1), nil, true)
		})
		w13Must(t, "newLazyObject(ghost)", err)
		var targetOwner value
		w13Must(t, "own ghost target", h.assign(&targetOwner, ghost))
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-initialized-ghost-target-accepted":
		base := w13ProxyBase(t)
		ghost, err := h.newLazyObject(base, false, func(h *heap, obj value) (value, error) {
			if err := h.writeObject(obj, "value", w13Int(12), nil, true); err != nil {
				return value{}, err
			}
			return nullValue(), nil
		})
		w13Must(t, "newLazyObject(ghost)", err)
		_, err = h.readObject(ghost, "value", nil)
		w13Must(t, "initialize ghost target", err)
		var targetOwner value
		w13Must(t, "own ghost target", h.assign(&targetOwner, ghost))
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = []any{
			w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) }),
			proxy.object.class.name,
			targetOwner.object.class.name,
		}

	case "proxy-initialized-proxy-target-rejected":
		base := w13ProxyBase(t)
		real := w13Instantiate(t, h, base)
		w13Must(t, "initialize real target", h.writeObject(real, "value", w13Int(13), nil, true))
		var realOwner value
		w13Must(t, "own real target", h.assign(&realOwner, real))
		target, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return realOwner, nil })
		w13Must(t, "newLazyObject(target proxy)", err)
		if _, err := h.readObject(target, "value", nil); err != nil {
			w13Must(t, "initialize target proxy", err)
		}
		var targetOwner value
		w13Must(t, "own target proxy", h.assign(&targetOwner, target))
		proxy, err := h.newLazyObject(base, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })

	case "proxy-wrapper-method-this":
		base := w13Build(t, classSpec{name: "W13ProxyBase", properties: w13ProxyBaseProperties(), methods: []methodSpec{{name: "who", call: func(h *heap, this value, _ []value) (value, error) {
			className := this.object.class.name
			return w13ProxyArray(h, w13Str(className), w13Str(className))
		}}}})
		wrapper := w13Build(t, classSpec{name: "W13ProxyWrapper", parent: base, methods: []methodSpec{{name: "who", call: func(h *heap, this value, _ []value) (value, error) {
			className := this.object.class.name
			return w13ProxyArray(h, w13Str("wrapper"), w13Str(className), w13Str("W13ProxyWrapper"))
		}}}})
		real := w13Instantiate(t, h, base)
		w13Must(t, "initialize parent target", h.writeObject(real, "value", w13Int(14), nil, true))
		var targetOwner value
		w13Must(t, "own parent target", h.assign(&targetOwner, real))
		calls := int64(0)
		var proxy value
		var err error
		proxy, err = h.newLazyObject(wrapper, true, func(*heap, value) (value, error) {
			calls++
			return targetOwner, nil
		})
		w13Must(t, "newLazyObject(proxy)", err)
		before := []any{h.isLazyProperty(proxy, "value"), calls}
		first := w13ObserveValue(func() (value, error) { return h.callMethod(proxy, "who", nil, nil, false) })
		afterMethod := []any{h.isLazyProperty(proxy, "value"), calls, proxy.object.class.name}
		propertyValue := w13ObserveValue(func() (value, error) { return h.readObject(proxy, "value", nil) })
		second := w13ObserveValue(func() (value, error) { return h.callMethod(proxy, "who", nil, nil, false) })
		result = []any{before, first, afterMethod, propertyValue, second, calls, proxy.object.class.name}

	case "proxy-wrapper-clone-backing":
		events := []any{}
		var base *class
		base = w13Build(t, classSpec{name: "W13ProxyCloneBase", properties: []propertySpec{{name: "value", typeName: "int", hasDefault: true, initial: int64(1)}}, methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, []any{"clone", this.object.class.name})
			v, err := h.readObject(this, "value", base)
			if err != nil {
				return value{}, err
			}
			return nullValue(), h.writeObject(this, "value", w13Int(v.integer+1), base, true)
		}}}})
		wrapper := w13Build(t, classSpec{name: "W13ProxyCloneWrapper", parent: base})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own clone target", h.assign(&targetOwner, real))
		proxy, err := h.newLazyObject(wrapper, true, func(*heap, value) (value, error) { return targetOwner, nil })
		w13Must(t, "newLazyObject(proxy)", err)
		clone, err := h.cloneObject(proxy, nil)
		if err != nil {
			result = w13ObserveError(func() error { return err })
			break
		}
		result = []any{
			proxy.object.class.name,
			clone.object.class.name,
			w13ObserveValue(func() (value, error) { return h.readObject(clone, "value", nil) }),
			events,
		}

	case "proxy-factory-failure-property-write":
		base := w13Build(t, classSpec{name: "W13ProxyDefaults", properties: []propertySpec{
			{name: "typed", typeName: "int", hasDefault: true, initial: int64(7)},
			{name: "untyped", hasDefault: true, initial: []byte("plain-default")},
		}})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own default target", h.assign(&targetOwner, real))
		calls := int64(0)
		var proxy value
		var err error
		proxy, err = h.newLazyObject(base, true, func(h *heap, obj value) (value, error) {
			calls++
			if err := h.writeObject(obj, "typed", w13Int(41), nil, true); err != nil {
				return value{}, err
			}
			if err := h.writeObject(obj, "untyped", w13Str("factory-write"), nil, true); err != nil {
				return value{}, err
			}
			if calls == 1 {
				return value{}, &Throwable{"Exception", "factory failed"}
			}
			return targetOwner, nil
		})
		w13Must(t, "newLazyObject(proxy)", err)
		before := []any{h.isLazyProperty(proxy, "typed"), h.isLazyProperty(proxy, "untyped")}
		first := w13ObserveValue(func() (value, error) { return h.readObject(proxy, "typed", nil) })
		afterFailure := []any{h.isLazyProperty(proxy, "typed"), h.isLazyProperty(proxy, "untyped")}
		written := []any{
			w13ObserveValue(func() (value, error) { return h.readObject(proxy, "typed", nil) }),
			w13ObserveValue(func() (value, error) { return h.readObject(proxy, "untyped", nil) }),
		}
		result = []any{before, first, afterFailure, written, calls}

	case "proxy-factory-reads-defaults":
		base := w13Build(t, classSpec{name: "W13ProxyDefaults", properties: []propertySpec{
			{name: "typed", typeName: "int", hasDefault: true, initial: int64(7)},
			{name: "untyped", hasDefault: true, initial: []byte("plain-default")},
		}})
		real := w13Instantiate(t, h, base)
		var targetOwner value
		w13Must(t, "own default target", h.assign(&targetOwner, real))
		var proxy value
		reads := []any{}
		var err error
		proxy, err = h.newLazyObject(base, true, func(h *heap, obj value) (value, error) {
			reads = []any{
				[]any{h.isLazyProperty(obj, "typed"), w13ProxyPropertyInitialized(obj, "typed"), w13ObserveValue(func() (value, error) { return h.readObject(obj, "typed", nil) })},
				[]any{h.isLazyProperty(obj, "untyped"), w13ProxyPropertyInitialized(obj, "untyped"), w13ObserveValue(func() (value, error) { return h.readObject(obj, "untyped", nil) })},
			}
			return targetOwner, nil
		})
		w13Must(t, "newLazyObject(proxy)", err)
		typed := w13ObserveValue(func() (value, error) { return h.readObject(proxy, "typed", nil) })
		untyped := w13ObserveValue(func() (value, error) { return h.readObject(proxy, "untyped", nil) })
		result = []any{reads, typed, untyped}

	default:
		t.Fatalf("unknown W-13 proxy case %q", name)
	}

	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13ProxyObservations(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for _, name := range w13ProxyCaseNames() {
				t.Run(name, func(t *testing.T) {
					h, err := newHeap(width)
					w13Must(t, "newHeap", err)
					w13AssertOracleCase(t, width, name, w13RunProxyCase(t, h, name))
				})
			}
		})
	}
}
