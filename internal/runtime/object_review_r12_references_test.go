package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R12ReferenceNames() []string {
	kinds := []string{"set-int", "set-uninit", "set-null", "set-array", "set-virtual", "set-object", "get-scalar", "get-object", "get-uninit", "get-reference", "inherited", "inherited-uninit", "own", "helper"}
	names := make([]string, 0, len(kinds)*3)
	for _, kind := range kinds {
		for _, mode := range []string{"normal", "ghost", "proxy"} {
			names = append(names, "r12-reference-"+kind+"-"+mode)
		}
	}
	return names
}

func w13R12ReferenceCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(name, "r12-reference-"), "-")
	if len(parts) < 2 {
		t.Fatalf("invalid reference case %q", name)
	}
	mode := parts[len(parts)-1]
	kind := strings.Join(parts[:len(parts)-1], "-")
	if mode != "normal" && mode != "ghost" && mode != "proxy" {
		t.Fatalf("invalid reference mode %q", mode)
	}
	classNames := map[string]string{
		"set-int": "W13R12RefSetInt", "set-uninit": "W13R12RefSetUninit",
		"set-null": "W13R12RefSetNull", "set-array": "W13R12RefSetArray",
		"set-virtual": "W13R12RefSetVirtual", "set-object": "W13R12RefSetObject", "get-scalar": "W13R12RefGetScalar",
		"get-object": "W13R12RefGetObject", "get-uninit": "W13R12RefGetUninit",
		"get-reference": "W13R12RefGetReference", "inherited": "W13R12RefInherited",
		"inherited-uninit": "W13R12RefInheritedUninit", "own": "W13R12RefOwn",
		"helper": "W13R12RefHelper",
	}
	className, ok := classNames[kind]
	if !ok {
		t.Fatalf("unknown reference kind %q", kind)
	}
	payload := w13Build(t, classSpec{name: "W13R12Payload"})
	events := []any{}
	property := propertySpec{name: "x", typeName: "int"}
	var parent *class
	var class *class
	switch kind {
	case "set-int":
		property.hasDefault, property.initial = true, int64(1)
		property.set = func(h *heap, this, v value) (value, error) { return value{}, h.writeObject(this, "x", v, class, false) }
	case "set-uninit":
		property.set = func(h *heap, this, v value) (value, error) { return value{}, h.writeObject(this, "x", v, class, false) }
	case "set-null":
		property.typeName, property.hasDefault, property.initial = "mixed", true, nil
		property.set = func(h *heap, this, v value) (value, error) { return value{}, h.writeObject(this, "x", v, class, false) }
	case "set-array":
		property.typeName, property.hasDefault, property.initial = "array", true, []SnapshotEntry{}
		property.set = func(h *heap, this, v value) (value, error) { return value{}, h.writeObject(this, "x", v, class, false) }
	case "set-virtual":
		property.virtual = true
		property.set = func(h *heap, this, v value) (value, error) {
			return value{}, h.writeObject(this, "anchor", v, class, false)
		}
	case "set-object":
		property.typeName = "object"
		property.set = func(h *heap, this, v value) (value, error) { return value{}, h.writeObject(this, "x", v, class, false) }
	case "get-scalar":
		property.virtual, property.hasDefault, property.initial = true, false, nil
		property.get = func(*heap, value, value) (value, error) { events = append(events, "get"); return w13Int(1), nil }
	case "get-object":
		property.typeName, property.virtual = "object", true
		property.get = func(h *heap, _, _ value) (value, error) {
			events = append(events, "get")
			return w13Instantiate(t, h, payload), nil
		}
	case "get-uninit":
		property.virtual = false
		property.get = func(h *heap, this, _ value) (value, error) {
			events = append(events, "get")
			return h.readObject(this, "x", class)
		}
	case "get-reference":
		property.hasDefault, property.initial, property.getReference = true, int64(1), true
		property.get = func(h *heap, this, _ value) (value, error) {
			events = append(events, "get")
			var ref value
			if err := h.referenceObject(&ref, this, "x", class); err != nil {
				return value{}, err
			}
			return ref, h.release(ref)
		}
	case "inherited", "inherited-uninit":
		parentName := "W13R12RefSetInt"
		if kind == "inherited-uninit" {
			parentName = "W13R12RefSetUninit"
		}
		property = propertySpec{name: "x", typeName: "int", hasDefault: kind == "inherited", initial: int64(1), set: func(h *heap, this, v value) (value, error) {
			return value{}, h.writeObject(this, "x", v, parent, false)
		}}
		parent = w13Build(t, classSpec{name: parentName, properties: []propertySpec{property}})
		className = "W13R12RefInherited"
		if kind == "inherited-uninit" {
			className += "Uninit"
		}
	case "own":
		property.hasDefault, property.initial = true, int64(1)
		property.set = func(h *heap, this, v value) (value, error) {
			events = append(events, "set")
			var alias value
			if err := h.referenceObject(&alias, this, "x", class); err != nil {
				return value{}, err
			}
			err := h.assignPHP(&alias, v, false)
			return value{}, h.mergeObjectErrors(err, h.release(alias))
		}
	case "helper":
		property.hasDefault, property.initial = true, int64(1)
		property.set = func(h *heap, this, v value) (value, error) {
			events = append(events, "set")
			if _, err := h.readObject(this, "x", class); err != nil {
				return value{}, err
			}
			_, err := h.callMethod(this, "helper", []methodArgument{{value: v}}, class, false)
			return value{}, err
		}
	default:
		t.Fatalf("unknown reference kind %q", kind)
	}
	methods := []methodSpec(nil)
	if kind == "helper" {
		methods = []methodSpec{{name: "helper", parameters: []methodParameter{{name: "value"}}, call: func(h *heap, this value, args []value) (value, error) {
			var alias value
			if err := h.referenceObject(&alias, this, "x", class); err != nil {
				return value{}, err
			}
			if err := h.assignPHP(&alias, args[0], false); err != nil {
				return value{}, h.mergeObjectErrors(err, h.release(alias))
			}
			return nullValue(), h.release(alias)
		}}}
	}
	if kind != "inherited" && kind != "inherited-uninit" {
		properties := []propertySpec{property}
		if kind == "get-scalar" || kind == "get-object" || kind == "set-virtual" {
			properties = []propertySpec{{name: "anchor", typeName: "int", hasDefault: true, initial: int64(0)}, property}
		}
		class = w13Build(t, classSpec{name: className, properties: properties, methods: methods})
	} else {
		class = w13Build(t, classSpec{name: className, parent: parent})
	}
	factory := func() value {
		o := w13Instantiate(t, h, class)
		if kind == "set-object" {
			must(t, h.writeObject(o, "x", w13Instantiate(t, h, payload), class, false))
		}
		return o
	}
	var object value
	if mode == "normal" {
		object = factory()
	} else if mode == "ghost" {
		var err error
		object, err = h.newLazyObject(class, false, func(h *heap, this value) (value, error) {
			events = append(events, "init")
			if kind == "set-object" {
				return nullValue(), h.writeObject(this, "x", w13Instantiate(t, h, payload), class, false)
			}
			return nullValue(), nil
		})
		must(t, err)
	} else {
		var err error
		object, err = h.newLazyObject(class, true, func(*heap, value) (value, error) { events = append(events, "factory"); return factory(), nil })
		must(t, err)
	}
	var owner value
	must(t, h.assign(&owner, object))
	before := h.isLazyObject(owner)
	var attempt any
	if kind == "own" || kind == "helper" {
		err := h.writeObject(owner, "x", w13Int(7), nil, false)
		if err != nil {
			attempt = w13Error(err)
		}
	} else {
		var alias value
		err := h.referenceObject(&alias, owner, "x", nil)
		if err == nil {
			events = append(events, "bound")
			err = h.assignPHP(&alias, w13Int(2), false)
		}
		if err != nil {
			attempt = w13Error(err)
		}
		must(t, h.unset(&alias))
	}
	var seen any
	var raw value
	raw, err := h.readObject(owner, "x", nil)
	if err != nil {
		seen = w13Error(err)
	} else {
		v := raw.deref()
		var scalar any
		if v.kind != valueObject {
			scalar = w13Encode(v)
		}
		seen = []any{valueTypeName(v), scalar}
		must(t, h.discardMethodReturn(raw))
	}
	eventSnapshot := append([]any{}, events...)
	finalLazy := h.isLazyObject(owner)
	result := []any{before, attempt, seen, eventSnapshot, finalLazy}
	diagnostics := w13Diagnostics(h)
	must(t, h.unset(&owner))
	must(t, h.shutdown(&value{}))
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatalf("reference case retained objects=%d sources=%d", len(h.live), len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: diagnostics}
}

func TestW13R12HookReferences(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R12ReferenceNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R12ReferenceCase(t, h, name))
			})
		}
	}
}
