package runtime

import (
	"errors"
	"fmt"
	"testing"
)

func w13ReviewNames() []string {
	return []string{"compare-stringable", "compare-stringable-throw", "compare-lazy-different-classes", "compare-object-numeric-notice", "compare-lazy-operand-owners", "reference-readonly-dynamic", "reference-enum-dynamic", "magic-cleanup-chain", "method-invalid-return-owner", "property-error-chain", "weak-map-error-chain", "array-error-chain", "weak-map-lone-reference-clone", "weak-map-key-free-reentry", "lazy-rollback-write-declared", "lazy-rollback-write-dynamic"}
}

// This checks host safety, not compatibility with the consecutive PHP input
// that corrupts both pinned engines. Its independent observations are replayed
// separately by TestW13PHP8512Objects.
func TestW13LazyRollbackConsecutiveSafety(t *testing.T) {
	h := testHeap(t)
	run := w13RollbackProgram(t, h)
	for _, dynamic := range []bool{false, true} {
		run(dynamic)
	}
	globals := nullValue()
	must(t, h.shutdown(&globals))
	if h.liveObjects != 0 {
		t.Fatal("request retained live objects after shutdown")
	}
}

// Build immutable class descriptors once for all operations in one request.
func w13RollbackProgram(t *testing.T, h *heap) func(bool) any {
	events := []any{}
	c := w13Build(t, classSpec{name: "W13RollbackWrite", allowDynamic: true, properties: []propertySpec{{name: "i", typeName: "int", hasDefault: true, initial: int64(1)}, {name: "child", typeName: "mixed", hasDefault: true, initial: nil}}})
	nextClass := w13Build(t, classSpec{name: "W13RollbackNext", destructor: func(*heap, value) error { events = append(events, "next"); return nil }})
	writerClass := w13Build(t, classSpec{name: "W13RollbackWriter", properties: []propertySpec{{name: "owner"}, {name: "next"}, {name: "dynamic", typeName: "bool"}}, destructor: func(h *heap, this value) error {
		events = append(events, "writer")
		owner, err := h.readObject(this, "owner", nil)
		if err != nil {
			return err
		}
		next, err := h.readObject(this, "next", nil)
		if err != nil {
			return err
		}
		dynamic, err := h.readObject(this, "dynamic", nil)
		if err != nil {
			return err
		}
		property := "child"
		if dynamic.boolean {
			property = "added"
		}
		return h.writeObject(owner, property, next, nil, true)
	}})
	return func(dynamic bool) any {
		events = []any{}
		var next value
		must(t, h.assign(&next, w13Instantiate(t, h, nextClass)))
		calls := 0
		failure := &Throwable{"Exception", "rollback"}
		o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
			calls++
			if calls == 1 {
				writer := w13Instantiate(t, h, writerClass)
				must(t, h.writeObject(writer, "owner", this, nil, true))
				must(t, h.writeObject(writer, "next", next, nil, true))
				must(t, h.writeObject(writer, "dynamic", boolValue(dynamic), nil, true))
				property := "child"
				if dynamic {
					property = "temp"
				}
				if err := h.writeObject(this, property, writer, nil, true); err != nil {
					return value{}, err
				}
				return value{}, failure
			}
			return nullValue(), h.writeObject(this, "i", w13Int(9), nil, true)
		})
		must(t, err)
		first := w13ObserveValue(func() (value, error) { return h.readObject(o, "i", nil) })
		must(t, h.unset(&next))
		before := append([]any{}, events...)
		i, err := h.readObject(o, "i", nil)
		must(t, err)
		child, err := h.issetObject(o, "child", nil)
		must(t, err)
		added, err := h.issetObject(o, "added", nil)
		must(t, err)
		return []any{first, before, w13Encode(i), child, added, events}
	}
}

func w13ReviewCase(t *testing.T, h *heap, name string) w13Case {
	events := []any{}
	var result any
	switch name {
	case "lazy-rollback-write-declared", "lazy-rollback-write-dynamic":
		dynamic := name == "lazy-rollback-write-dynamic"
		result = w13RollbackProgram(t, h)(dynamic)
	case "property-error-chain", "weak-map-error-chain", "array-error-chain":
		errorsByName := map[string]*Throwable{"a": {"Exception", "a"}, "b": {"Exception", "b"}}
		c := w13Build(t, classSpec{name: "W13ReviewValue", properties: []propertySpec{{name: "name", typeName: "string"}}, destructor: func(h *heap, this value) error {
			n, err := h.readObject(this, "name", nil)
			if err != nil {
				return err
			}
			events = append(events, n.text)
			return errorsByName[n.text]
		}})
		child := func(label string) value {
			o := w13Instantiate(t, h, c)
			must(t, h.writeObject(o, "name", w13Str(label), nil, true))
			return o
		}
		var container value
		if name == "property-error-chain" {
			must(t, h.assign(&container, w13NewStdClass(t, h)))
			must(t, h.writeObject(container, "a", child("a"), nil, true))
			must(t, h.writeObject(container, "b", child("b"), nil, true))
		} else if name == "array-error-chain" {
			must(t, h.assign(&container, h.newArrayValue()))
			must(t, h.appendValue(&container, child("a")))
			must(t, h.appendValue(&container, child("b")))
		} else {
			m, err := h.newWeakMapObject()
			must(t, err)
			must(t, h.assign(&container, m))
			a, b := w13NewStdClass(t, h), w13NewStdClass(t, h)
			var ao, bo value
			must(t, h.assign(&ao, a))
			must(t, h.assign(&bo, b))
			must(t, h.writeWeakMap(m, a, child("a")))
			must(t, h.writeWeakMap(m, b, child("b")))
		}
		err := h.unset(&container)
		var php *Throwable
		if !errors.As(err, &php) {
			t.Fatal(err)
		}
		var previous any
		var end completion
		if errors.As(err, &end) && end.previous != nil {
			var prior *Throwable
			if errors.As(end.previous, &prior) {
				previous = prior.Message
			}
		}
		events = append(events, []any{"caught", php.Message, previous}, "after")
		result = events
	case "weak-map-lone-reference-clone":
		m, err := h.newWeakMapObject()
		must(t, err)
		key := w13NewStdClass(t, h)
		must(t, h.writeWeakMap(m, key, w13Int(1)))
		var alias value
		must(t, h.referenceWeakMap(&alias, m, key))
		must(t, h.unset(&alias))
		clone, err := h.cloneObject(m, nil)
		must(t, err)
		var a, b value
		must(t, h.referenceWeakMap(&a, m, key))
		must(t, h.referenceWeakMap(&b, clone, key))
		must(t, h.assign(&a, w13Int(2)))
		ma, err := h.readWeakMap(m, key)
		must(t, err)
		mb, err := h.readWeakMap(clone, key)
		must(t, err)
		result = []any{w13Encode(a), w13Encode(b), w13Encode(ma), w13Encode(mb)}
	case "weak-map-key-free-reentry":
		m1, err := h.newWeakMapObject()
		must(t, err)
		m2, err := h.newWeakMapObject()
		must(t, err)
		var map1, map2, keyOwner value
		must(t, h.assign(&map1, m1))
		must(t, h.assign(&map2, m2))
		key := w13NewStdClass(t, h)
		must(t, h.assign(&keyOwner, key))
		c := w13Build(t, classSpec{name: "W13WeakReentryValue", destructor: func(h *heap, _ value) error { events = append(events, "value"); return h.unset(&map2) }})
		must(t, h.writeWeakMap(m1, key, w13Instantiate(t, h, c)))
		must(t, h.writeWeakMap(m2, key, w13Int(1)))
		must(t, h.unset(&keyOwner))
		result = []any{events, len(h.weakMapObjects[m1.object].entries), map2.kind != valueUndefined}
	case "reference-readonly-dynamic", "reference-enum-dynamic":
		var o value
		if name == "reference-readonly-dynamic" {
			o = w13Instantiate(t, h, w13Build(t, classSpec{name: "W13ReferenceReadonly", readonly: true}))
		} else {
			c := w13Build(t, classSpec{name: "W13Enum", enumCases: []string{"A"}, enumValues: []any{[]byte("a")}})
			var err error
			o, err = h.enumCase(c, "A")
			must(t, err)
		}
		var alias value
		result = w13ObserveError(func() error { return h.referenceObject(&alias, o, "extra", nil) })
	case "compare-object-numeric-notice":
		c := w13Build(t, classSpec{name: "W13CompareNumeric"})
		o := w13Instantiate(t, h, c)
		values := []any{}
		for i, n := range []value{w13Int(1), w13Int(1), floatValue(1), floatValue(1)} {
			v, err := h.compare(o, n)
			must(t, err)
			if i%2 == 0 {
				values = append(values, v == 0)
			} else {
				values = append(values, v)
			}
		}
		result = values
	case "compare-lazy-operand-owners":
		c := w13Build(t, classSpec{name: "W13CompareHeld", properties: []propertySpec{{name: "i", typeName: "int"}, {name: "label", typeName: "string"}}, destructor: func(h *heap, this value) error {
			v, err := h.readObject(this, "label", nil)
			if err != nil {
				return err
			}
			events = append(events, "dtor:"+v.text)
			return nil
		}})
		var left, right value
		makeLazy := func(label string) value {
			o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
				events = append(events, "init:"+label)
				if label == "A" {
					if err := h.unset(&left); err != nil {
						return value{}, err
					}
					if err := h.unset(&right); err != nil {
						return value{}, err
					}
				}
				if err := h.writeObject(this, "i", w13Int(1), nil, true); err != nil {
					return value{}, err
				}
				return nullValue(), h.writeObject(this, "label", w13Str(label), nil, true)
			})
			must(t, err)
			return o
		}
		must(t, h.assign(&left, makeLazy("A")))
		must(t, h.assign(&right, makeLazy("B")))
		v, err := h.compare(left, right)
		if err != nil {
			result = []any{w13Error(err), events}
		} else {
			result = []any{v == 0, events}
		}
	case "compare-lazy-different-classes":
		makeLazy := func(name, label string) value {
			c := w13Build(t, classSpec{name: name, properties: []propertySpec{{name: "i", typeName: "int"}}})
			o, err := h.newLazyObject(c, false, func(h *heap, this value) (value, error) {
				events = append(events, label)
				return nullValue(), h.writeObject(this, "i", w13Int(1), nil, true)
			})
			must(t, err)
			return o
		}
		a, b := makeLazy("W13CompareLazyA", "A.init"), makeLazy("W13CompareLazyB", "B.init")
		first, err := h.compare(a, b)
		must(t, err)
		second, err := h.compare(a, b)
		must(t, err)
		result = []any{first == 0, second, events}
	case "compare-stringable":
		c := w13Build(t, classSpec{name: "W13CompareString", methods: []methodSpec{{name: "__toString", call: func(*heap, value, []value) (value, error) {
			events = append(events, "string")
			return w13Str("42"), nil
		}}}})
		o := w13Instantiate(t, h, c)
		var owner value
		must(t, h.assign(&owner, o))
		values := []any{}
		for i := range 4 {
			a, b := o, w13Str("42")
			if i%2 == 1 {
				a, b = b, a
			}
			v, err := h.compare(a, b)
			must(t, err)
			if i < 2 {
				values = append(values, v == 0)
			} else {
				values = append(values, v)
			}
		}
		result = []any{values, events}
	case "compare-stringable-throw":
		c := w13Build(t, classSpec{name: "W13CompareThrow", methods: []methodSpec{{name: "__toString", call: func(*heap, value, []value) (value, error) { return value{}, &Throwable{"Exception", "conversion"} }}}})
		result = w13ObserveError(func() error { _, err := h.compare(w13Instantiate(t, h, c), w13Str("x")); return err })
	case "magic-cleanup-chain":
		var argument value
		original := &Throwable{"Exception", "method"}
		c := w13Build(t, classSpec{name: "W13MagicCleanup", methods: []methodSpec{{name: "__call", parameters: []methodParameter{{name: "name"}, {name: "args"}}, call: func(h *heap, _ value, _ []value) (value, error) {
			if err := h.unset(&argument); err != nil {
				return value{}, err
			}
			return value{}, original
		}}}})
		a := w13Build(t, classSpec{name: "W13MagicCleanupArgument", destructor: func(*heap, value) error {
			events = append(events, "argument")
			return &Throwable{"Exception", "cleanup"}
		}})
		must(t, h.assign(&argument, w13Instantiate(t, h, a)))
		_, err := h.callMethod(w13Instantiate(t, h, c), "missing", []methodArgument{{value: argument}}, nil, false)
		var php *Throwable
		if !errors.As(err, &php) {
			t.Fatal("missing PHP exception", err)
		}
		var previous any
		var end completion
		if errors.As(err, &end) && end.previous != nil {
			var prior *Throwable
			if errors.As(end.previous, &prior) {
				previous = prior.Message
			}
		}
		result = []any{[]any{php.Message, previous}, events}
	case "method-invalid-return-owner":
		c := w13Build(t, classSpec{name: "W13InvalidReturn", methods: []methodSpec{{name: "bad", returnType: "int", call: func(h *heap, _ value, _ []value) (value, error) {
			return h.newObject(&class{name: "W13InvalidReturnObject", destructor: func(*heap, value) error { events = append(events, "result"); return nil }})
		}}}})
		e := w13ObserveValue(func() (value, error) { return h.callMethod(w13Instantiate(t, h, c), "bad", nil, nil, false) })
		result = []any{e, events}
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13ReviewRegressions(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13ReviewNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13ReviewCase(t, h, name))
			})
		}
	}
}
