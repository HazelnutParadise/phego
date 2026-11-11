package runtime

import (
	"strconv"
	"testing"
)

// REQ-018/020; T-035/036/039/040. The untouched slot still initializes;
// completed unset locations preserve their non-lazy state through rollback.
func w13R6RawUnsetCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	events := []any{}
	base := w13Build(t, classSpec{name: "W13R6RawFollowup", properties: []propertySpec{{name: "q", typeName: "int"}, {name: "spare", typeName: "int"}}})
	c := base
	magic := name != "r6-lazy-raw-unset-followup"
	if magic {
		c = w13Build(t, classSpec{name: "W13R6RawFollowupMagic", parent: base, methods: []methodSpec{
			{name: "__get", returnType: "mixed", parameters: []methodParameter{{name: "name"}}, call: func(_ *heap, _ value, args []value) (value, error) {
				events = append(events, "get:"+args[0].deref().text)
				return w13Int(23), nil
			}},
			{name: "__isset", returnType: "bool", parameters: []methodParameter{{name: "name"}}, call: func(_ *heap, _ value, args []value) (value, error) {
				events = append(events, "isset:"+args[0].deref().text)
				return boolValue(true), nil
			}},
			{name: "__set", returnType: "void", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(_ *heap, _ value, args []value) (value, error) {
				events = append(events, "set:"+args[0].deref().text+":"+strconv.FormatInt(args[1].deref().integer, 10))
				return nullValue(), nil
			}},
		}})
	}
	if name == "r6-lazy-raw-unset-magic-throw" || name == "r6-lazy-raw-unset-magic-reentry" {
		parent := c
		className := "W13R6RawFollowupMagicThrow"
		if name == "r6-lazy-raw-unset-magic-reentry" {
			className = "W13R6RawFollowupMagicReentry"
		}
		c = w13Build(t, classSpec{name: className, parent: parent, methods: []methodSpec{{name: "__set", returnType: "void", parameters: []methodParameter{{name: "name"}, {name: "value"}}, call: func(h *heap, this value, args []value) (value, error) {
			events = append(events, "set:"+args[0].deref().text+":"+strconv.FormatInt(args[1].deref().integer, 10))
			if name == "r6-lazy-raw-unset-magic-throw" {
				return value{}, &Throwable{"Exception", "set"}
			}
			return nullValue(), h.writeObject(this, args[0].deref().text, args[1], c, false)
		}}}})
	}
	calls := 0
	o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) {
		calls++
		events = append(events, "init")
		return value{}, &Throwable{"Exception", "init"}
	})
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("raw-unset fixture retained request values")
		}
	}()
	must(t, h.writeRawObjectProperty(o, "q", w13Int(11), false))
	read := func(field string) any {
		return w13ObserveValue(func() (value, error) { return h.readObject(o, field, nil) })
	}
	result := []any{w13ObserveError(func() error { return h.unsetObject(o, "q", nil) }), read("q"), w13ObserveValue(func() (value, error) { b, err := h.issetObject(o, "q", nil); return boolValue(b), err })}
	if !magic {
		result = append(result, w13ObserveError(func() error {
			var alias value
			err := h.referenceObject(&alias, o, "q", nil)
			return h.mergeObjectErrors(err, h.unset(&alias))
		}))
	}
	result = append(result, w13ObserveError(func() error { return h.writeObject(o, "q", w13Int(12), nil, false) }), read("q"), []any{calls, h.isLazyObject(o)})
	result = append(result, read("spare"), []any{calls, h.isLazyObject(o), read("q")})
	return w13Case{Result: []any{true, result, append([]any{}, events...)}, Diagnostics: w13Diagnostics(h)}
}
