package runtime

import (
	"strings"
	"testing"
)

func w13R6ProxyGuardNames() []string {
	return []string{"r6-proxy-guard-get", "r6-proxy-guard-reference", "r6-proxy-guard-isset", "r6-proxy-guard-set", "r6-proxy-guard-unset", "r6-proxy-guard-set-spare", "r6-proxy-guard-isset-fresh", "r6-proxy-guard-set-owned", "r6-proxy-guard-set-different", "r6-proxy-guard-get-dynamic", "r6-proxy-guard-reference-dynamic", "r6-proxy-guard-isset-dynamic", "r6-proxy-guard-set-dynamic", "r6-proxy-guard-unset-dynamic", "r6-proxy-guard-get-dynamic-direct", "r6-proxy-guard-reference-dynamic-direct", "r6-proxy-guard-isset-dynamic-direct", "r6-proxy-guard-unset-dynamic-direct"}
}

// REQ-018/020; T-035/036/039/040. Existing target guards survive forwarding.
func w13R6ProxyGuardCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	mode := strings.TrimPrefix(name, "r6-proxy-guard-")
	events := []any{}
	var c *class
	var target, proxy value
	readSpare := func(h *heap, this value) error { _, err := h.readObject(this, "spare", c); return err }
	c = w13Build(t, classSpec{name: "W13R6ProxyGuard", allowDynamic: true, properties: []propertySpec{{name: "q", typeName: "int"}, {name: "r", typeName: "int"}, {name: "spare", typeName: "int"}}, methods: []methodSpec{
		{name: "__get", returnType: "mixed", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, args []value) (value, error) {
			field := args[0].deref().text
			events = append(events, "get:"+field)
			if !strings.HasSuffix(mode, "-dynamic-direct") {
				if err := readSpare(h, this); err != nil {
					return value{}, err
				}
			}
			return h.readObject(this, field, c)
		}},
		{name: "__isset", returnType: "bool", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, args []value) (value, error) {
			field := args[0].deref().text
			events = append(events, "isset:"+field)
			if !strings.HasSuffix(mode, "-dynamic-direct") {
				if err := readSpare(h, this); err != nil {
					return value{}, err
				}
			}
			yes, err := h.issetObject(this, field, c)
			return boolValue(yes), err
		}},
		{name: "__unset", returnType: "void", parameters: []methodParameter{{name: "name"}}, call: func(h *heap, this value, args []value) (value, error) {
			field := args[0].deref().text
			events = append(events, "unset:"+field)
			if !strings.HasSuffix(mode, "-dynamic-direct") {
				if err := readSpare(h, this); err != nil {
					return value{}, err
				}
			}
			return nullValue(), h.unsetObject(this, field, c)
		}},
		{name: "__set", returnType: "void", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(h *heap, this value, args []value) (value, error) {
			field := args[0].deref().text
			event := "set:" + field
			if mode == "set-owned" {
				if this.object == target.object {
					event += ":target"
				} else {
					event += ":proxy"
				}
			}
			events = append(events, event)
			if len(events) > 12 {
				return value{}, &Throwable{"Exception", "bounded reentry"}
			}
			if mode == "set-owned" && this.object == target.object {
				if err := h.writeObject(proxy, "q", args[1], nil, false); err != nil {
					return value{}, err
				}
				if err := h.unsetObject(this, "q", c); err != nil {
					return value{}, err
				}
				return nullValue(), h.writeObject(this, "q", w13Int(17), c, false)
			}
			if mode == "set-different" && field == "q" {
				return nullValue(), h.writeObject(this, "r", args[1], c, false)
			}
			if mode == "set-spare" {
				if err := readSpare(h, this); err != nil {
					return value{}, err
				}
			}
			return nullValue(), h.writeObject(this, field, args[1], c, false)
		}},
	}})
	var targetOwner, proxyOwner value
	must(t, h.assign(&targetOwner, w13Instantiate(t, h, c)))
	target = targetOwner
	must(t, h.writeObject(target, "spare", w13Int(7), nil, false))
	must(t, h.unsetObject(target, "q", nil))
	must(t, h.unsetObject(target, "r", nil))
	p, err := h.newLazyObject(c, true, func(*heap, value) (value, error) { events = append(events, "init"); return target, nil })
	must(t, err)
	must(t, h.assign(&proxyOwner, p))
	proxy = proxyOwner
	defer func() {
		must(t, h.unset(&proxyOwner))
		must(t, h.unset(&targetOwner))
		if len(h.live) != 0 || len(h.typedSources) != 0 {
			t.Fatal("proxy guard fixture retained request values")
		}
	}()
	dynamic := strings.Contains(mode, "-dynamic")
	field := "q"
	if dynamic {
		field = "x"
	}
	if mode != "isset-fresh" && !dynamic {
		for _, field := range []string{"q", "r"} {
			must(t, h.writeRawObjectProperty(proxy, field, w13Int(11), false))
			must(t, h.unsetObject(proxy, field, nil))
		}
	}
	result := w13ObserveValue(func() (value, error) {
		switch mode {
		case "get", "get-dynamic", "get-dynamic-direct":
			return h.readObject(proxy, field, nil)
		case "reference", "reference-dynamic", "reference-dynamic-direct":
			var alias value
			err := h.referenceObject(&alias, proxy, field, nil)
			if err != nil {
				return value{}, err
			}
			out := alias.deref()
			return out, h.unset(&alias)
		case "isset", "isset-fresh", "isset-dynamic", "isset-dynamic-direct":
			yes, err := h.issetObject(proxy, field, nil)
			return boolValue(yes), err
		case "set", "set-spare", "set-different", "set-dynamic":
			if err := h.writeObject(proxy, field, w13Int(12), nil, false); err != nil {
				return value{}, err
			}
			field := field
			if mode == "set-different" {
				field = "r"
			}
			return h.readObject(proxy, field, nil)
		case "set-owned":
			if err := h.writeObject(target, "q", w13Int(12), nil, false); err != nil {
				return value{}, err
			}
			return h.readObject(target, "q", nil)
		case "unset", "unset-dynamic", "unset-dynamic-direct":
			return nullValue(), h.unsetObject(proxy, field, nil)
		default:
			t.Fatalf("unknown proxy guard mode %q", mode)
			return value{}, nil
		}
	})
	return w13Case{Result: []any{result, append([]any{}, events...), h.isLazyObject(proxy)}, Diagnostics: w13Diagnostics(h)}
}
