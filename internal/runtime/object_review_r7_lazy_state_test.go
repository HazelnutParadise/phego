package runtime

import (
	"encoding/json"
	"fmt"
	"testing"
)

func w13R7LazyStateNames() []string {
	return []string{"r7-lazy-state-ghost", "r7-lazy-state-proxy"}
}

func w13R7RawControlNames() []string {
	return []string{"r7-raw-last-slot", "r7-raw-type-error", "r7-raw-proxy-failure", "r7-raw-running-proxy-failure", "r7-raw-receiver-reference-rebind", "r7-raw-shadow-public", "r7-raw-shadow-private"}
}

func TestW13R7RawControls(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R7RawControlNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R7RawControlCase(t, h, name))
			})
		}
	}
}

func w13R7RawControlCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	if name == "r7-raw-receiver-reference-rebind" {
		return w13R7RawReceiverRebindCase(t, h)
	}
	if name == "r7-raw-shadow-public" || name == "r7-raw-shadow-private" {
		return w13R7RawShadowCase(t, h, name)
	}
	events := []any{}
	var owner value
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("raw control retained request values")
		}
	}()
	if name == "r7-raw-last-slot" {
		c := w13Build(t, classSpec{name: "W13R7RawSingle", properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(1)}}, destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }})
		tokenClass := w13Build(t, classSpec{name: "W13R7RawInitializerOwner", destructor: func(h *heap, _ value) error {
			x, err := h.readObject(owner, "x", nil)
			if err == nil {
				events = append(events, []any{"release", h.isLazyObject(owner), w13Encode(x)})
			}
			return err
		}})
		token := w13Instantiate(t, h, tokenClass)
		o, err := h.newLazyObjectWithInitializer(c, false, token, func(*heap, value) (value, error) { events = append(events, "init"); return nullValue(), nil })
		must(t, err)
		must(t, h.assign(&owner, o))
		before := h.isLazyObject(o)
		failure := w13ObserveError(func() error { return h.writeRawObjectProperty(o, "x", w13Int(42), false) })
		x, err := h.readObject(o, "x", nil)
		must(t, err)
		result := []any{before, failure, h.isLazyObject(o), h.isLazyProperty(o, "x"), w13Encode(x), append([]any{}, events...)}
		must(t, h.unset(&owner))
		return w13Case{Result: []any{result, append([]any{}, events...)}, Diagnostics: w13Diagnostics(h)}
	}
	c := w13Build(t, classSpec{name: "W13R7LazyState", properties: []propertySpec{{name: "x", typeName: "int", hasDefault: true, initial: int64(1)}, {name: "y", typeName: "int", hasDefault: true, initial: int64(2)}}})
	calls := 0
	proxy := name == "r7-raw-proxy-failure" || name == "r7-raw-running-proxy-failure"
	o, err := h.newLazyObject(c, proxy, func(h *heap, this value) (value, error) {
		calls++
		if proxy {
			var err error
			if name == "r7-raw-running-proxy-failure" {
				err = h.writeRawObjectProperty(this, "x", w13Int(41), false)
			} else {
				err = h.writeObject(this, "x", w13Int(41), nil, false)
			}
			if err != nil {
				return value{}, err
			}
			return value{}, &Throwable{"Exception", "factory"}
		}
		return nullValue(), nil
	})
	must(t, err)
	must(t, h.assign(&owner, o))
	state := func() []any { return []any{h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y")} }
	if !proxy {
		array := h.newArrayValue()
		failure := w13ObserveError(func() error { return h.writeRawObjectProperty(o, "x", array, false) })
		must(t, h.discard(array))
		return w13Case{Result: []any{failure, h.isLazyObject(o), h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y"), calls}, Diagnostics: w13Diagnostics(h)}
	}
	failure := w13ObserveValue(func() (value, error) { return h.readObject(o, "y", nil) })
	if name == "r7-raw-running-proxy-failure" {
		result := []any{failure, []any{h.isLazyObject(o), h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y"), calls}}
		must(t, h.writeRawObjectProperty(o, "y", w13Int(42), false))
		x, err := h.readObject(o, "x", nil)
		must(t, err)
		y, err := h.readObject(o, "y", nil)
		must(t, err)
		result = append(result, []any{h.isLazyObject(o), h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y"), w13Encode(x), w13Encode(y), calls})
		return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
	}
	result := []any{failure, state()}
	must(t, h.writeRawObjectProperty(o, "x", w13Int(42), false))
	result = append(result, state())
	must(t, h.writeRawObjectProperty(o, "y", w13Int(2), false))
	x, err := h.readObject(o, "x", nil)
	must(t, err)
	y, err := h.readObject(o, "y", nil)
	must(t, err)
	result = append(result, []any{h.isLazyObject(o), h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y"), w13Encode(x), w13Encode(y), calls})
	copy, cloneErr := h.cloneObject(o, nil)
	var cloneObservation any = w13ObserveError(func() error { return cloneErr })
	if cloneErr == nil {
		cx, err := h.readObject(copy, "x", nil)
		must(t, err)
		cy, err := h.readObject(copy, "y", nil)
		must(t, err)
		cloneObservation = []any{w13Encode(cx), w13Encode(cy), copy.object != o.object, h.isLazyObject(copy)}
		must(t, h.discard(copy))
	}
	result = append(result, cloneObservation)
	outer, err := h.newLazyObject(c, true, func(*heap, value) (value, error) { return o, nil })
	must(t, err)
	var outerOwner value
	must(t, h.assign(&outerOwner, outer))
	result = append(result, []any{w13ObserveValue(func() (value, error) { return h.readObject(outer, "y", nil) }), h.isLazyObject(outer)})
	must(t, h.unset(&outerOwner))
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13R7RawReceiverRebindCase(t *testing.T, h *heap) w13Case {
	t.Helper()
	events := []any{}
	c := w13Build(t, classSpec{name: "W13R7RawString", properties: []propertySpec{{name: "x", typeName: "string", hasDefault: true, initial: []byte("old")}}, destructor: func(h *heap, this value) error {
		x, err := h.readObject(this, "x", nil)
		if err == nil {
			events = append(events, "dtor:"+x.text)
		}
		return err
	}})
	o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { return nullValue(), nil })
	must(t, err)
	var owner, alias, sourceOwner value
	must(t, h.assign(&owner, o))
	must(t, h.bind(&alias, &owner))
	weak, err := h.weakReferenceFor(o)
	must(t, err)
	sourceClass := w13Build(t, classSpec{name: "W13R7RawRebindSource", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
		if err := h.assign(&alias, nullValue()); err != nil {
			return value{}, err
		}
		_, live := weak.get()
		events = append(events, []any{"convert", live})
		return w13Str("raw"), nil
	}}}})
	must(t, h.assign(&sourceOwner, w13Instantiate(t, h, sourceClass)))
	defer func() {
		must(t, h.unset(&sourceOwner))
		must(t, h.unset(&alias))
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("receiver-rebind fixture retained values")
		}
	}()
	failure := w13ObserveError(func() error { return h.writeRawObjectProperty(owner, "x", sourceOwner, false) })
	_, live := weak.get()
	return w13Case{Result: []any{failure, w13Encode(owner), w13Encode(alias), events, live}, Diagnostics: w13Diagnostics(h)}
}

func w13R7RawShadowCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var parent *class
	parent = w13Build(t, classSpec{name: "W13R7RawParent", properties: []propertySpec{{name: "x", typeName: "int", visibility: privateAccess, hasDefault: true, initial: int64(11)}}, methods: []methodSpec{{name: "parentX", returnType: "int", call: func(h *heap, this value, _ []value) (value, error) {
		return h.readObject(this, "x", parent)
	}}}})
	visibility, className := publicAccess, "W13R7RawChildPublic"
	if name == "r7-raw-shadow-private" {
		visibility, className = privateAccess, "W13R7RawChildPrivate"
	}
	c := w13Build(t, classSpec{name: className, parent: parent, properties: []propertySpec{{name: "x", typeName: "int", visibility: visibility, hasDefault: true, initial: int64(22)}}})
	calls := 0
	o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { calls++; return nullValue(), nil })
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("shadow raw fixture retained values")
		}
	}()
	result := []any{}
	p, err := resolvePropertyIn(c, "x", c, true)
	must(t, err)
	for _, v := range []int64{41, 42} {
		must(t, h.writeRawObjectProperty(o, "x", w13Int(v), false))
		slot, _ := o.object.props.lookup(p.key)
		result = append(result, []any{h.isLazyObject(o), h.isLazyProperty(o, "x"), w13Encode(*slot), calls})
	}
	result = append(result, w13ObserveValue(func() (value, error) { return h.callMethod(o, "parentX", nil, nil, false) }), calls)
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R7LazyPropertyState(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R7LazyStateNames() {
			h, err := referenceHeap(width)
			must(t, err)
			got, err := json.Marshal(w13R7LazyStateCase(t, h, name).Result)
			must(t, err)
			want := `[[true,true],[false,true],[false,true],2,[false,false],[false,false]]`
			if name == "r7-lazy-state-proxy" {
				want = `[[true,true],[false,true],[false,true],2,[false,true],[false,false]]`
			}
			if string(got) != want {
				t.Errorf("%d/%s: Go %s; pinned PHP %s", width, name, got, want)
			}
		}
	}
}

// REQ-018; T-035/036. Raw/unset slots and untouched slots differ while pending.
func w13R7LazyStateCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	proxy := name == "r7-lazy-state-proxy"
	c := w13Build(t, classSpec{name: "W13R7LazyState", properties: []propertySpec{
		{name: "x", typeName: "int", hasDefault: true, initial: int64(1)},
		{name: "y", typeName: "int", hasDefault: true, initial: int64(2)},
	}})
	var running []any
	o, err := h.newLazyObject(c, proxy, func(h *heap, this value) (value, error) {
		running = []any{h.isLazyProperty(this, "x"), h.isLazyProperty(this, "y")}
		if proxy {
			return h.newObject(c)
		}
		return nullValue(), nil
	})
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("lazy state fixture retained request values")
		}
	}()
	state := func() []any { return []any{h.isLazyProperty(o, "x"), h.isLazyProperty(o, "y")} }
	result := []any{state()}
	must(t, h.writeRawObjectProperty(o, "x", w13Int(42), false))
	result = append(result, state())
	must(t, h.unsetObject(o, "x", nil))
	result = append(result, state())
	y, err := h.readObject(o, "y", nil)
	must(t, err)
	result = append(result, w13Encode(y), running, state())
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
