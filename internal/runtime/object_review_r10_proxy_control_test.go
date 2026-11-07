package runtime

import (
	"fmt"
	"strings"
	"testing"
)

// REQ-018/020; T-035/036/039/040. Preserve wrapper hooks and per-receiver notices.
func w13R10ProxyControlNames() []string {
	var names []string
	for _, mode := range []string{"hooks", "static", "staticmagic"} {
		for _, temperature := range []string{"cold", "warm"} {
			for _, op := range []string{"read", "write", "reference", "isset", "unset"} {
				if mode == "hooks" && op == "unset" {
					continue
				}
				names = append(names, "r10-proxy-"+mode+"-"+temperature+"-"+op)
			}
		}
	}
	return append(names, "r10-staticmagic-guarded-read", "r10-staticmagic-dynamic-read")
}

func w13R10ProxyControlCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if strings.HasPrefix(name, "r10-staticmagic-") {
		return w13R10StaticGuardCase(t, h, name)
	}
	parts := strings.Split(name, "-")
	hooks, warm, op := parts[2] == "hooks", parts[3] == "warm", parts[4]
	magic := parts[2] == "staticmagic"
	var parentCell, childCell value
	must(t, h.assign(&parentCell, w13Int(11)))
	must(t, h.assign(&childCell, w13Int(7)))
	events := []any{}
	makeProperties := func(label string, cell *value) []propertySpec {
		properties := []propertySpec{{name: "spare", typeName: "int", hasDefault: true, initial: int64(2)}}
		if !hooks {
			return append(properties, propertySpec{name: "x", static: true, typeName: "int", hasDefault: true, initial: int64(1)})
		}
		return append(properties,
			propertySpec{name: "x", typeName: "int", hasDefault: true, initial: int64(1), getReference: true, get: func(h *heap, _ value, _ value) (value, error) {
				events = append(events, label+":get")
				var raw value
				must(t, h.bind(&raw, cell))
				// The global cell retains the returned reference after this local owner ends.
				return raw, h.release(raw)
			}},
			propertySpec{name: "y", typeName: "int", virtual: true, set: func(_ *heap, _ value, rhs value) (value, error) {
				events = append(events, []any{label + ":set", w13Encode(rhs)})
				return nullValue(), nil
			}})
	}
	base := "W13R10Static"
	if hooks {
		base = "W13R10Hook"
	}
	magicMethods := func(label string) []methodSpec {
		return []methodSpec{
			{name: "__get", parameters: []methodParameter{{name: "name"}}, returnType: "mixed", call: func(*heap, value, []value) (value, error) {
				events = append(events, label+":get:x")
				return w13Int(9), nil
			}},
			{name: "__set", parameters: []methodParameter{{name: "name"}, {name: "value"}}, returnType: "void", call: func(*heap, value, []value) (value, error) {
				events = append(events, label+":set:x")
				return nullValue(), nil
			}},
			{name: "__isset", parameters: []methodParameter{{name: "name"}}, returnType: "bool", call: func(*heap, value, []value) (value, error) {
				events = append(events, label+":isset:x")
				return boolValue(true), nil
			}},
			{name: "__unset", parameters: []methodParameter{{name: "name"}}, returnType: "void", call: func(*heap, value, []value) (value, error) {
				events = append(events, label+":unset:x")
				return nullValue(), nil
			}},
		}
	}
	parentSpec := classSpec{name: base + "Parent", properties: makeProperties("parent", &parentCell)}
	if magic {
		ancestor := w13Build(t, parentSpec)
		base = "W13R10StaticMagic"
		parentSpec = classSpec{name: base + "Parent", parent: ancestor, methods: magicMethods("parent")}
	}
	parent := w13Build(t, parentSpec)
	var childProperties []propertySpec
	if hooks {
		childProperties = makeProperties("wrapper", &childCell)[1:]
	}
	childSpec := classSpec{name: base + "Child", parent: parent, properties: childProperties}
	if magic {
		childSpec.methods = magicMethods("wrapper")
	}
	child := w13Build(t, childSpec)
	backing := w13Instantiate(t, h, parent)
	var backingOwner, proxyOwner, alias value
	must(t, h.assign(&backingOwner, backing))
	proxy, err := h.newLazyObject(child, true, func(*heap, value) (value, error) { events = append(events, "factory"); return backingOwner, nil })
	must(t, err)
	must(t, h.assign(&proxyOwner, proxy))
	defer func() {
		must(t, h.unset(&alias))
		must(t, h.unset(&proxyOwner))
		must(t, h.unset(&backingOwner))
		must(t, h.unset(&childCell))
		must(t, h.unset(&parentCell))
		must(t, h.shutdown(&value{}))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("proxy control retained request values")
		}
	}()
	if warm {
		v, e := h.readObject(proxyOwner, "spare", nil)
		must(t, e)
		events = append(events, []any{"warm", w13Encode(v)})
	}
	var result any
	switch op {
	case "read":
		result = w13ObserveValue(func() (value, error) { return h.readObject(proxyOwner, "x", nil) })
	case "write":
		field := "x"
		if hooks {
			field = "y"
		}
		result = w13ObserveError(func() error { return h.writeObject(proxyOwner, field, w13Int(3), nil, false) })
	case "reference":
		err = h.referenceObject(&alias, proxyOwner, "x", nil)
		if err == nil {
			err = h.assign(&alias, w13Int(3))
		}
		if err != nil {
			result = w13Error(err)
		} else {
			result = w13Encode(alias)
		}
		must(t, h.unset(&alias))
	case "isset":
		v, e := h.issetObject(proxyOwner, "x", nil)
		if e != nil {
			result = w13Error(e)
		} else {
			result = v
		}
	case "unset":
		result = w13ObserveError(func() error { return h.unsetObject(proxyOwner, "x", nil) })
	}
	out := []any{result, append([]any{}, events...), h.isLazyObject(proxyOwner)}
	if hooks {
		out = []any{result, w13Encode(childCell), w13Encode(parentCell), append([]any{}, events...), h.isLazyObject(proxyOwner)}
	}
	return w13Case{Result: out, Diagnostics: w13Diagnostics(h)}
}

func TestW13R10ProxyControls(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R10ProxyControlNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R10ProxyControlCase(t, h, name))
			})
		}
	}
}

func w13R10StaticGuardCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parent := w13Build(t, classSpec{name: "W13R10StaticParent", properties: []propertySpec{{name: "spare", typeName: "int", hasDefault: true, initial: int64(2)}, {name: "x", typeName: "int", static: true, hasDefault: true, initial: int64(1)}}})
	events := []any{}
	c := w13Build(t, classSpec{name: "W13R10StaticGuard", parent: parent, methods: []methodSpec{
		{name: "__get", parameters: []methodParameter{{name: "name"}}, returnType: "mixed", call: func(h *heap, this value, _ []value) (value, error) {
			events = append(events, "get")
			return h.readObject(this, "x", nil)
		}},
		{name: "__set", parameters: []methodParameter{{name: "name"}, {name: "value"}}, returnType: "void", call: func(h *heap, this value, args []value) (value, error) {
			events = append(events, "set")
			return nullValue(), h.writeObject(this, "x", args[1], nil, false)
		}},
	}})
	var owner value
	must(t, h.assign(&owner, w13Instantiate(t, h, c)))
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("static guard control retained values")
		}
	}()
	if name == "r10-staticmagic-dynamic-read" {
		must(t, h.writeObject(owner, "x", w13Int(5), nil, false))
	}
	result := w13ObserveValue(func() (value, error) { return h.readObject(owner, "x", nil) })
	return w13Case{Result: []any{result, events}, Diagnostics: w13Diagnostics(h)}
}
