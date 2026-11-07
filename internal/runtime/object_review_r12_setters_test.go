package runtime

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func w13R12SetterNames() []string {
	names := make([]string, 0, 211)
	inputs := []string{"text", "numeric", "int", "float", "bool", "null", "array", "object"}
	for _, kind := range []string{"backed-wide", "virtual-wide", "implicit", "explicit"} {
		for _, input := range inputs {
			for _, strict := range []string{"weak", "strict"} {
				for _, mode := range []string{"normal", "ghost", "proxy"} {
					names = append(names, "r12-setter-"+kind+"-"+input+"-"+strict+"-"+mode)
				}
			}
		}
	}
	for _, kind := range []string{"inherited", "trait", "override"} {
		for _, input := range []string{"text", "array"} {
			for _, mode := range []string{"normal", "ghost", "proxy"} {
				names = append(names, "r12-setter-metadata-"+kind+"-"+input+"-weak-"+mode)
			}
		}
	}
	names = append(names, "r12-setter-held-receiver")
	return names
}

func w13R12SetterCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name == "r12-setter-held-receiver" {
		return w13R12HeldReceiverCase(t, h)
	}
	parts := strings.Split(strings.TrimPrefix(name, "r12-setter-"), "-")
	if len(parts) < 4 {
		t.Fatalf("invalid setter case %q", name)
	}
	mode, strictName, input := parts[len(parts)-1], parts[len(parts)-2], parts[len(parts)-3]
	kind := strings.Join(parts[:len(parts)-3], "-")
	if strings.HasPrefix(kind, "metadata-") {
		kind = strings.TrimPrefix(kind, "metadata-")
	}
	strict := strictName == "strict"
	if strictName != "strict" && strictName != "weak" || mode != "normal" && mode != "ghost" && mode != "proxy" {
		t.Fatalf("invalid setter case fields in %q", name)
	}
	events := []any{}
	var c, parent *class
	baseProperty := propertySpec{name: "x", typeName: "int", hasDefault: true, initial: int64(0)}
	stored := propertySpec{name: "stored", typeName: "int", hasDefault: true, initial: int64(0)}
	setter := func(paramName string, virtual bool) (propertySpec, map[string]hookParameter) {
		p := baseProperty
		params := map[string]hookParameter{"x": {name: paramName, typeName: "int|string"}}
		p.set = func(h *heap, this, v value) (value, error) {
			events = append(events, []any{"body", valueTypeName(v), w13Encode(v)})
			n := v.deref()
			if n.kind == valueString {
				n = w13Int(int64(len(n.text)))
			}
			if !virtual {
				if err := h.writeObject(this, "x", n, c, false); err != nil {
					return value{}, err
				}
			}
			if err := h.writeObject(this, "stored", n, c, false); err != nil {
				return value{}, err
			}
			return nullValue(), nil
		}
		return p, params
	}
	var props []propertySpec
	var setterParams map[string]hookParameter
	className := ""
	switch kind {
	case "inherited":
		className = "W13R12SetterInherited"
	case "backed-wide":
		className = "W13R12SetterBackedWide"
		p, params := setter("incoming", false)
		props, setterParams = []propertySpec{stored, p}, params
	case "virtual-wide":
		className = "W13R12SetterVirtualWide"
		p, params := setter("incoming", true)
		p.virtual, p.hasDefault, p.initial = true, false, nil
		props, setterParams = []propertySpec{stored, p}, params
	case "implicit":
		className = "W13R12SetterImplicit"
		p := baseProperty
		p.set = func(h *heap, this, v value) (value, error) {
			events = append(events, []any{"body", valueTypeName(v), w13Encode(v)})
			if err := h.writeObject(this, "x", v, c, false); err != nil {
				return value{}, err
			}
			if err := h.writeObject(this, "stored", v, c, false); err != nil {
				return value{}, err
			}
			return nullValue(), nil
		}
		props = []propertySpec{stored, p}
	case "explicit":
		className = "W13R12SetterExplicit"
		p := baseProperty
		setterParams = map[string]hookParameter{"x": {name: "incoming", typeName: "int"}}
		p.set = func(h *heap, this, v value) (value, error) {
			events = append(events, []any{"body", valueTypeName(v), w13Encode(v)})
			if err := h.writeObject(this, "x", v, c, false); err != nil {
				return value{}, err
			}
			if err := h.writeObject(this, "stored", v, c, false); err != nil {
				return value{}, err
			}
			return nullValue(), nil
		}
		props = []propertySpec{stored, p}
	case "trait":
		className = "W13R12SetterComposed"
		p, params := setter("traitInput", false)
		trait := w13Build(t, classSpec{name: "W13R12SetterTrait", isTrait: true, properties: []propertySpec{stored, p}, setterParameters: params})
		c = w13Build(t, classSpec{name: className, traits: []*class{trait}})
	case "override":
		className = "W13R12SetterOverride"
		p, params := setter("renamed", false)
		parentProp := baseProperty
		parentProp.set = func(h *heap, this, v value) (value, error) {
			return value{}, h.writeObject(this, "x", v, parent, false)
		}
		parent = w13Build(t, classSpec{name: "W13R12SetterBackedWide", properties: []propertySpec{stored, parentProp}, setterParameters: map[string]hookParameter{"x": {name: "incoming", typeName: "int|string"}}})
		props, setterParams = []propertySpec{p}, params
	default:
		t.Fatalf("unknown setter kind %q", kind)
	}
	if kind == "inherited" {
		p, params := setter("incoming", false)
		parent = w13Build(t, classSpec{name: "W13R12SetterBackedWide", properties: []propertySpec{stored, p}, setterParameters: params})
		c = w13Build(t, classSpec{name: "W13R12SetterInherited", parent: parent})
	} else if c == nil {
		c = w13Build(t, classSpec{name: className, parent: parent, properties: props, setterParameters: setterParams})
	}
	var v value
	switch input {
	case "text":
		v = w13Str("abc")
	case "numeric":
		v = w13Str("12")
	case "int":
		v = w13Int(2)
	case "float":
		v = floatValue(2.5)
	case "bool":
		v = boolValue(true)
	case "null":
		v = nullValue()
	case "array":
		v = h.newArrayValue()
	case "object":
		inputClass := w13Build(t, classSpec{name: "W13R12SetterText", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
			events = append(events, "convert")
			return w13Str("abc"), nil
		}}}})
		v = w13Instantiate(t, h, inputClass)
	default:
		t.Fatalf("unknown setter input %q", input)
	}
	var inputOwner value
	if v.kind == valueArray || v.kind == valueObject {
		must(t, h.assign(&inputOwner, v))
		v = inputOwner
	}
	var receiver value
	if mode == "normal" {
		receiver = w13Instantiate(t, h, c)
	} else if mode == "ghost" {
		var err error
		receiver, err = h.newLazyObject(c, false, func(*heap, value) (value, error) { events = append(events, "init"); return nullValue(), nil })
		must(t, err)
	} else {
		var err error
		receiver, err = h.newLazyObject(c, true, func(*heap, value) (value, error) {
			events = append(events, "factory")
			return w13Instantiate(t, h, c), nil
		})
		must(t, err)
	}
	var receiverOwner value
	must(t, h.assign(&receiverOwner, receiver))
	var attempt any
	file, line := "/w/review_r12_setters.php", 10
	if strict {
		file, line = "/w/review_r12_setter_strict.php", 3
	}
	if err := h.writeObjectAt(receiverOwner, "x", v, nil, strict, methodCallSite{file: file, line: line}); err != nil {
		attempt = w13Error(err)
	}
	afterAssign := h.isLazyObject(receiverOwner)
	assignEvents := append([]any{}, events...)
	storedValue, err := h.readObject(receiverOwner, "stored", nil)
	var observedStored any
	if err != nil {
		observedStored = w13Error(err)
	} else {
		observedStored = w13Encode(storedValue)
		must(t, h.discardMethodReturn(storedValue))
	}
	finalEvents := append([]any{}, events...)
	finalLazy := h.isLazyObject(receiverOwner)
	result := []any{attempt, afterAssign, assignEvents, observedStored, finalEvents, finalLazy}
	if inputOwner.kind != valueUndefined {
		must(t, h.unset(&inputOwner))
	}
	must(t, h.unset(&receiverOwner))
	var globals value
	must(t, h.shutdown(&globals))
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("setter fixture retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R12SetterParameters(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R12SetterNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R12SetterCase(t, h, name))
			})
		}
	}
}

func TestW13R12SetterMetadataIsolation(t *testing.T) {
	original := map[string]hookParameter{"x": {name: "incoming", typeName: "int|string"}}
	var c *class
	c, err := buildClass(classSpec{name: "W13R12SetterIsolation", properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(0), set: func(h *heap, this, v value) (value, error) {
		n := v.deref()
		if n.kind == valueString {
			n = w13Int(int64(len(n.text)))
		}
		return value{}, h.writeObject(this, "x", n, c, false)
	}}}, setterParameters: original})
	must(t, err)
	original["x"] = hookParameter{name: "changed", typeName: "array"}
	if got := c.schema.setterParameters["x"]; got.name != "incoming" || got.typeName != "int|string" {
		t.Fatalf("setter metadata changed after build: %#v", got)
	}
	const workers = 8
	var wg sync.WaitGroup
	for _, width := range []int{32, 64} {
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(width int) {
				defer wg.Done()
				h, e := referenceHeap(width)
				if e != nil {
					t.Error(e)
					return
				}
				o, e := h.instantiate(c)
				if e == nil {
					e = h.writeObjectAt(o, "x", w13Str("abc"), nil, false, methodCallSite{})
				}
				if e != nil {
					t.Error(e)
				}
				v, e := h.readObject(o, "x", nil)
				if e != nil || w13Encode(v) != int64(3) {
					t.Errorf("width %d stored %v, error %v", width, w13Encode(v), e)
				}
				if e := h.discardMethodReturn(v); e != nil {
					t.Error(e)
				}
				if e := h.discard(o); e != nil {
					t.Error(e)
				}
				var globals value
				if e := h.shutdown(&globals); e != nil {
					t.Error(e)
				}
				if h.liveObjects != 0 || len(h.typedSources) != 0 {
					t.Errorf("width %d retained values: live=%d typed=%d", width, h.liveObjects, len(h.typedSources))
				}
			}(width)
		}
	}
	wg.Wait()
}

func w13R12HeldReceiverCase(t *testing.T, h *heap) w13Case {
	t.Helper()
	events := []any{}
	var owner value
	var receiverClass *class
	receiverClass = w13Build(t, classSpec{
		name: "W13R12HeldReceiver",
		properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(0), set: func(h *heap, this, v value) (value, error) {
			events = append(events, "body")
			n := v.deref()
			if n.kind == valueString {
				n = w13Int(int64(len(n.text)))
			}
			if err := h.writeObject(this, "x", n, receiverClass, false); err != nil {
				return value{}, err
			}
			stored, err := h.readObject(this, "x", receiverClass)
			if err != nil {
				return value{}, err
			}
			events = append(events, []any{"stored", w13Encode(stored)})
			return nullValue(), h.discardMethodReturn(stored)
		}}},
		setterParameters: map[string]hookParameter{"x": {name: "v", typeName: "int|string"}},
		destructor:       func(*heap, value) error { events = append(events, "receiver"); return nil },
	})
	ownerObject := w13Instantiate(t, h, receiverClass)
	must(t, h.assign(&owner, ownerObject))
	inputClass := w13Build(t, classSpec{
		name: "W13R12HeldInput",
		methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
			events = append(events, "convert")
			var alias value
			if err := h.referenceObject(&alias, owner, "x", nil); err != nil {
				events = append(events, w13Error(err))
			} else {
				events = append(events, "reference-allowed")
				if err := h.unset(&owner); err != nil {
					return value{}, err
				}
				if err := h.unset(&alias); err != nil {
					return value{}, err
				}
			}
			if owner.kind != valueUndefined {
				if err := h.unset(&owner); err != nil {
					return value{}, err
				}
			}
			return w13Str("abc"), nil
		}}},
		destructor: func(*heap, value) error { events = append(events, "rhs"); return nil },
	})
	input := w13Instantiate(t, h, inputClass) // raw constructor temporary, with no owner
	borrowedReceiver := owner
	var attempt any
	if err := h.writeObjectAt(borrowedReceiver, "x", input, nil, false, methodCallSite{}); err != nil {
		attempt = w13Error(err)
	}
	if err := h.discard(input); err != nil {
		t.Fatal(err)
	}
	events = append(events, "after")
	if owner.kind != valueUndefined {
		t.Errorf("conversion did not unset receiver owner: %#v", owner)
	}
	eventSnapshot := append([]any{}, events...)
	var globals value
	if err := h.shutdown(&globals); err != nil {
		t.Fatal(err)
	}
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("held setter retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
	}
	return w13Case{Result: []any{attempt, eventSnapshot}, Diagnostics: w13Diagnostics(h)}
}
