package runtime

import (
	"fmt"
	"testing"
)

func w13R6LazyNames() []string {
	return []string{
		"r6-lazy-virtual-unset", "r6-lazy-backed-unset", "r6-lazy-private-set-unset",
		"r6-lazy-protected-set-unset", "r6-lazy-readonly-uninit-unset", "r6-lazy-readonly-raw-unset",
		"r6-lazy-private-set-hook-unset", "r6-lazy-raw-plain-unset", "r6-lazy-readonly-init-unset",
		"r6-lazy-dynamic-magic", "r6-lazy-dynamic-plain", "r6-lazy-plain-unset", "r6-lazy-init-throw",
		"r6-lazy-magic-asym", "r6-lazy-recursive-unset", "r6-lazy-readonly-clone",
		"r6-lazy-readonly-cleared-magic",
		"r6-lazy-raw-unset-followup", "r6-lazy-raw-unset-magic",
		"r6-lazy-raw-unset-magic-throw", "r6-lazy-raw-unset-magic-reentry",
	}
}

func w13R6LazyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name == "r6-lazy-raw-unset-followup" || name == "r6-lazy-raw-unset-magic" || name == "r6-lazy-raw-unset-magic-throw" || name == "r6-lazy-raw-unset-magic-reentry" {
		return w13R6RawUnsetCase(t, h, name)
	}
	events := []any{}
	var result any
	owners := []*value{}
	defer func() {
		for _, owner := range owners {
			if owner.kind != valueUndefined {
				_ = h.unset(owner)
			}
		}
	}()
	track := func(v value) value {
		var owner value
		must(t, h.assign(&owner, v))
		owners = append(owners, &owner)
		return owner
	}
	newLazy := func(c *class, init func(*heap, value) (value, error)) value {
		o, err := h.newLazyObject(c, false, init)
		must(t, err)
		return track(o)
	}
	addMagic := func(name string) methodSpec {
		return methodSpec{name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(_ *heap, _ value, args []value) (value, error) {
			events = append(events, "magic:"+args[0].deref().text)
			return nullValue(), nil
		}}
	}
	switch name {
	case "r6-lazy-virtual-unset", "r6-lazy-backed-unset":
		virtual := name == "r6-lazy-virtual-unset"
		prop := propertySpec{name: "v", typeName: "int", virtual: virtual, get: func(_ *heap, _ value, _ value) (value, error) { events = append(events, "get"); return w13Int(1), nil }}
		className := "W13R6Virtual"
		prop.name = "v"
		if !virtual {
			className, prop.name, prop.virtual = "W13R6Backed", "b", false
			prop.hasDefault, prop.initial = true, int64(1)
			prop.set = func(_ *heap, _ value, _ value) (value, error) {
				events = append(events, "set")
				return nullValue(), nil
			}
		}
		c := w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "q", typeName: "int", hasDefault: true, initial: int64(0)}, prop}})
		calls := 0
		o := newLazy(c, func(_ *heap, _ value) (value, error) {
			calls++
			events = append(events, "init")
			return value{}, &Throwable{"Exception", "init"}
		})
		err := h.unsetObject(o, prop.name, nil)
		result = []any{virtual, w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-private-set-unset", "r6-lazy-protected-set-unset", "r6-lazy-readonly-uninit-unset":
		p := propertySpec{name: "p", typeName: "int", setVisibility: privateAccess}
		cname := "W13R6PrivSet"
		if name == "r6-lazy-protected-set-unset" {
			cname, p.setVisibility = "W13R6ProtSet", protectedAccess
		}
		if name == "r6-lazy-readonly-uninit-unset" {
			cname, p.name, p.readonly, p.setVisibility = "W13R6RoUninit", "ro", true, protectedAccess
		}
		c := w13Build(t, classSpec{name: cname, properties: []propertySpec{p, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) { calls++; return value{}, &Throwable{"Exception", "init"} })
		err := h.unsetObject(o, p.name, nil)
		result = []any{w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-readonly-raw-unset":
		c := w13Build(t, classSpec{name: "W13R6RoRaw", properties: []propertySpec{{name: "ro", typeName: "int", readonly: true}, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) { calls++; return nullValue(), nil })
		must(t, h.writeRawObjectProperty(o, "ro", w13Int(123), false))
		rawSlot, _ := o.object.props.lookup("ro")
		raw := w13Encode(*rawSlot)
		err := h.unsetObject(o, "ro", nil)
		result = []any{raw, w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-private-set-hook-unset":
		c := w13Build(t, classSpec{name: "W13R6PrivateHook", properties: []propertySpec{{name: "q", typeName: "int", hasDefault: true, initial: int64(0)}, {name: "p", typeName: "int", setVisibility: privateAccess, virtual: true, get: func(*heap, value, value) (value, error) { return w13Int(0), nil }, set: func(*heap, value, value) (value, error) { return nullValue(), nil }}}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) { calls++; return value{}, &Throwable{"Exception", "init"} })
		err := h.unsetObject(o, "p", nil)
		result = []any{w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-raw-plain-unset":
		c := w13Build(t, classSpec{name: "W13R6RawPlain", properties: []propertySpec{{name: "q", typeName: "int"}, {name: "spare", typeName: "int"}}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) { calls++; return value{}, &Throwable{"Exception", "init"} })
		must(t, h.writeRawObjectProperty(o, "q", w13Int(11), false))
		rawSlot, _ := o.object.props.lookup("q")
		raw := w13Encode(*rawSlot)
		err := h.unsetObject(o, "q", nil)
		result = []any{raw, w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-dynamic-magic", "r6-lazy-dynamic-plain":
		methods := []methodSpec{}
		if name == "r6-lazy-dynamic-magic" {
			methods = append(methods, methodSpec{name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(_ *heap, _ value, args []value) (value, error) {
				events = append(events, "magic:"+args[0].deref().text)
				return nullValue(), nil
			}})
		}
		className := "W13R6DynamicPlain"
		if name == "r6-lazy-dynamic-magic" {
			className = "W13R6DynamicMagic"
		}
		c := w13Build(t, classSpec{name: className, allowDynamic: true, properties: []propertySpec{{name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}, methods: methods})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) {
			calls++
			events = append(events, "init")
			return value{}, &Throwable{"Exception", "init"}
		})
		err := h.unsetObject(o, "missing", nil)
		result = []any{w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-readonly-init-unset":
		var c *class
		c = w13Build(t, classSpec{name: "W13R6ReadonlyInit", properties: []propertySpec{{name: "ro", typeName: "int", readonly: true, setVisibility: protectedAccess}, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}, methods: []methodSpec{{name: "initializeReadonly", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.writeObject(this, "ro", w13Int(123), c, true)
		}}, {name: "clearReadonly", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.unsetObject(this, "ro", c)
		}}}})
		calls := 0
		o := newLazy(c, func(h *heap, object value) (value, error) {
			calls++
			events = append(events, "init")
			_, err := h.callMethod(object, "initializeReadonly", nil, nil, false)
			return nullValue(), err
		})
		_, err := h.callMethod(o, "clearReadonly", nil, nil, false)
		result = []any{w13ObserveError(func() error { return err }), calls, h.isLazyObject(o)}
	case "r6-lazy-plain-unset":
		c := w13Build(t, classSpec{name: "W13R6Plain", properties: []propertySpec{{name: "q", typeName: "int"}}})
		calls := 0
		o := newLazy(c, func(h *heap, o value) (value, error) {
			calls++
			events = append(events, "init")
			return nullValue(), h.writeObject(o, "q", w13Int(11), c, true)
		})
		unsetErr := h.unsetObject(o, "q", nil)
		isset, err := h.issetObject(o, "q", nil)
		must(t, err)
		read := w13ObserveValue(func() (value, error) { return h.readObject(o, "q", nil) })
		result = []any{w13ObserveError(func() error { return unsetErr }), isset, read, calls, h.isLazyObject(o)}
	case "r6-lazy-init-throw":
		c := w13Build(t, classSpec{name: "W13R6ThrowPlain", properties: []propertySpec{{name: "q", typeName: "int"}}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) {
			calls++
			events = append(events, "init")
			return value{}, &Throwable{"Exception", "boom"}
		})
		first := h.unsetObject(o, "q", nil)
		a := []any{w13ObserveError(func() error { return first }), calls, h.isLazyObject(o)}
		read, readErr := h.readObject(o, "q", nil)
		result = append(a, w13ObserveValue(func() (value, error) { return read, readErr }), calls, h.isLazyObject(o))
	case "r6-lazy-magic-asym":
		var c *class
		c = w13Build(t, classSpec{name: "W13R6MagicAsym", properties: []propertySpec{{name: "p", typeName: "int", setVisibility: privateAccess}, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}, methods: []methodSpec{{name: "prepare", call: func(h *heap, this value, _ []value) (value, error) {
			if e := h.writeObject(this, "p", w13Int(7), c, true); e != nil {
				return value{}, e
			}
			return nullValue(), h.unsetObject(this, "p", c)
		}}, addMagic("__unset")}})
		o := track(w13Instantiate(t, h, c))
		_, err := h.callMethod(o, "prepare", nil, nil, false)
		must(t, err)
		result = []any{w13ObserveError(func() error { return h.unsetObject(o, "p", nil) }), events}
	case "r6-lazy-recursive-unset":
		var child *class
		var parent *class
		parent = w13Build(t, classSpec{name: "W13R6RecurParent", methods: []methodSpec{{name: "__unset", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, args []value) (value, error) {
			events = append(events, "magic:"+args[0].deref().text)
			return nullValue(), h.unsetObject(this, "p", parent)
		}}}})
		child = w13Build(t, classSpec{name: "W13R6RecurChild", parent: parent, properties: []propertySpec{{name: "p", typeName: "int", setVisibility: privateAccess}, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}})
		o := track(w13Instantiate(t, h, child))
		clearErr := h.unsetObject(o, "p", child)
		unsetErr := h.unsetObject(o, "p", nil)
		result = []any{w13ObserveError(func() error { return clearErr }), w13ObserveError(func() error { return unsetErr }), events}
	case "r6-lazy-readonly-clone":
		var c *class
		c = w13Build(t, classSpec{name: "W13R6RoClone", properties: []propertySpec{{name: "ro", typeName: "int", readonly: true, setVisibility: protectedAccess}}, methods: []methodSpec{{name: "__clone", call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, "clone")
			if e := h.unsetObject(this, "ro", c); e != nil {
				return value{}, e
			}
			return nullValue(), h.writeObject(this, "ro", w13Int(42), c, true)
		}}}})
		o := track(w13Instantiate(t, h, c))
		must(t, h.writeObject(o, "ro", w13Int(1), c, true))
		clone, err := h.cloneObject(o, nil)
		must(t, err)
		clone = track(clone)
		v, err := h.readObject(clone, "ro", nil)
		must(t, err)
		unsetErr := h.unsetObject(clone, "ro", nil)
		result = []any{w13Encode(v), events, w13ObserveError(func() error { return unsetErr }), events}
	case "r6-lazy-readonly-cleared-magic":
		var c *class
		c = w13Build(t, classSpec{name: "W13R6RoMagic", properties: []propertySpec{{name: "ro", typeName: "int", readonly: true, setVisibility: protectedAccess}, {name: "q", typeName: "int", hasDefault: true, initial: int64(0)}}, methods: []methodSpec{{name: "clear", call: func(h *heap, this value, _ []value) (value, error) { return nullValue(), h.unsetObject(this, "ro", c) }}, addMagic("__unset")}})
		calls := 0
		o := newLazy(c, func(*heap, value) (value, error) { calls++; events = append(events, "init"); return nullValue(), nil })
		_, clearErr := h.callMethod(o, "clear", nil, nil, false)
		a := []any{w13ObserveError(func() error { return clearErr }), calls, h.isLazyObject(o)}
		unsetErr := h.unsetObject(o, "ro", nil)
		result = append(a, w13ObserveError(func() error { return unsetErr }))
	default:
		t.Fatalf("unknown case %q", name)
	}
	switch name {
	case "r6-lazy-virtual-unset", "r6-lazy-backed-unset", "r6-lazy-private-set-unset", "r6-lazy-protected-set-unset", "r6-lazy-readonly-uninit-unset", "r6-lazy-readonly-raw-unset", "r6-lazy-private-set-hook-unset", "r6-lazy-raw-plain-unset", "r6-lazy-dynamic-magic", "r6-lazy-dynamic-plain", "r6-lazy-readonly-init-unset", "r6-lazy-plain-unset", "r6-lazy-init-throw", "r6-lazy-readonly-cleared-magic":
		result = []any{true, result, append([]any{}, events...)}
	}
	diagnostics := w13Diagnostics(h)
	for _, owner := range owners {
		must(t, h.unset(owner))
	}
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("lazy case retained objects=%d typed sources=%d", h.liveObjects, len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: diagnostics}
}

func TestW13R6LazyUnset(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R6LazyNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R6LazyCase(t, h, name))
			})
		}
	}
}
