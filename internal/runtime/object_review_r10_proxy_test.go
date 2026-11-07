package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R10ProxyNames() []string {
	names := make([]string, 0, 60)
	for _, magic := range []bool{false, true} {
		for _, warm := range []bool{false, true} {
			for _, scope := range []string{"global", "parent", "child"} {
				for _, op := range []string{"read", "write", "reference", "isset", "unset"} {
					kind, temperature := "plain", "cold"
					if magic {
						kind = "magic"
					}
					if warm {
						temperature = "warm"
					}
					names = append(names, fmt.Sprintf("r10-proxy-%s-%s-%s-%s", kind, temperature, scope, op))
				}
			}
		}
	}
	return names
}

func TestW13R10ProxyMetadata(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R10ProxyNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R10ProxyCase(t, h, name))
			})
		}
	}
}

func w13R10ProxyCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(name, "r10-proxy-"), "-")
	magic, warm := parts[0] == "magic", parts[1] == "warm"
	scope, op := parts[2], parts[3]
	base := "W13R10Proxy"
	if magic {
		base += "Magic"
	}
	parentName, childName := base+"Parent", base+"Child"
	events := []any{}
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
	parent := w13Build(t, classSpec{name: parentName, properties: []propertySpec{{name: "x", typeName: "int", visibility: protectedAccess, hasDefault: true, initial: int64(1)}, {name: "spare", typeName: "int", hasDefault: true, initial: int64(2)}}, methods: func() []methodSpec {
		if magic {
			return magicMethods("parent")
		}
		return nil
	}()})
	child := w13Build(t, classSpec{name: childName, parent: parent, properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(1)}}, methods: func() []methodSpec {
		if magic {
			return magicMethods("wrapper")
		}
		return nil
	}()})
	var backingOwner, proxyOwner, alias value
	backing, err := h.newObject(parent)
	must(t, err)
	must(t, h.assign(&backingOwner, backing))
	proxy, err := h.newLazyObject(child, true, func(*heap, value) (value, error) { events = append(events, "factory"); return backingOwner, nil })
	must(t, err)
	must(t, h.assign(&proxyOwner, proxy))
	defer func() {
		must(t, h.unset(&alias))
		must(t, h.unset(&proxyOwner))
		must(t, h.unset(&backingOwner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("proxy fixture retained request values", h.liveObjects, len(h.typedSources))
		}
	}()
	if warm {
		v, e := h.readObject(proxyOwner, "spare", nil)
		must(t, e)
		events = append(events, []any{"warm", w13Encode(v)})
	}
	var result any
	var opErr error
	scopeClass := (*class)(nil)
	if scope == "parent" {
		scopeClass = parent
	}
	if scope == "child" {
		scopeClass = child
	}
	switch op {
	case "read":
		v, e := h.readObject(proxyOwner, "x", scopeClass)
		opErr = e
		if e == nil {
			result = w13Encode(v)
		}
	case "write":
		opErr = h.writeObject(proxyOwner, "x", w13Int(3), scopeClass, false)
	case "reference":
		opErr = h.referenceObject(&alias, proxyOwner, "x", scopeClass)
		if opErr == nil {
			opErr = h.assign(&alias, w13Int(3))
			if opErr == nil {
				result = w13Encode(alias)
			}
		}
		must(t, h.unset(&alias))
	case "isset":
		var yes bool
		yes, opErr = h.issetObject(proxyOwner, "x", scopeClass)
		if opErr == nil {
			result = yes
		}
	case "unset":
		opErr = h.unsetObject(proxyOwner, "x", scopeClass)
	}
	if opErr != nil {
		result = w13Error(opErr)
	}
	peek := w13ObserveValue(func() (value, error) { return h.readObject(backingOwner, "x", parent) })
	out := w13Case{Result: []any{result, peek, append([]any{}, events...), h.isLazyObject(proxyOwner)}, Diagnostics: w13Diagnostics(h)}
	return out
}
