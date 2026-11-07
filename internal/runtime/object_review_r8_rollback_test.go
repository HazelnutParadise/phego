package runtime

import (
	"fmt"
	"testing"
)

func w13R8Names() []string {
	return []string{"r8-rollback-sources-typed", "r8-rollback-sources-untyped"}
}

// REQ-018; T-035/036. Rollback callbacks observe the live typed-source order.
func w13R8Case(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	typed := name == "r8-rollback-sources-typed"
	if !typed && name != "r8-rollback-sources-untyped" {
		t.Fatalf("unknown R8 case %q", name)
	}

	baseName, className, propertyType := "W13R8BaseUntyped", "W13R8Untyped", ""
	if typed {
		baseName, className, propertyType = "W13R8BaseTyped", "W13R8Typed", "int"
	}
	base := w13Build(t, classSpec{name: baseName, properties: []propertySpec{
		{name: "hidden", typeName: "int", visibility: privateAccess, hasDefault: true, initial: int64(9)},
		{name: "a", typeName: propertyType},
	}})
	child := w13Build(t, classSpec{name: className, parent: base, properties: []propertySpec{
		{name: "a", typeName: propertyType, hasDefault: true, initial: int64(7)},
		{name: "s", typeName: "int", static: true, hasDefault: true, initial: int64(7)},
		{name: "v", typeName: "int", virtual: true, get: func(*heap, value, value) (value, error) { return w13Int(99), nil }},
		{name: "victim", hasDefault: true, initial: nil},
		{name: "b", typeName: "int", hasDefault: true, initial: int64(2)},
		{name: "trigger", typeName: "int", hasDefault: true, initial: int64(1)},
	}})

	events := []any{}
	var globalAlias value
	tokenClass := w13Build(t, classSpec{name: "W13R8Token", destructor: func(h *heap, _ value) error {
		assignment := w13ObserveError(func() error { return h.assign(&globalAlias, w13Str("bad")) })
		events = append(events, []any{"globalAlias", assignment})
		return nil
	}})
	calls := 0
	o, err := h.newLazyObject(child, false, func(h *heap, this value) (value, error) {
		calls++
		events = append(events, []any{"init", int64(calls)})
		token, err := h.instantiate(tokenClass)
		if err != nil {
			return value{}, err
		}
		if err := h.writeObject(this, "victim", token, nil, false); err != nil {
			return value{}, h.mergeObjectErrors(err, h.discard(token))
		}
		return value{}, &Throwable{"Exception", "init"}
	})
	must(t, err)
	var owner value
	must(t, h.assign(&owner, o))
	var alias, temp value
	defer func() {
		must(t, h.unset(&globalAlias))
		must(t, h.unset(&temp))
		must(t, h.unset(&alias))
		must(t, h.unset(&owner))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatalf("R8 rollback fixture retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
		}
	}()

	for _, property := range []string{"a", "b"} {
		must(t, h.writeRawObjectProperty(o, property, w13Int(1), false))
	}
	must(t, h.referenceObject(&alias, o, "a", nil))
	b, err := resolvePropertyIn(child, "b", child, true)
	must(t, err)
	bSlot, ok := o.object.props.lookup(b.key)
	if !ok {
		t.Fatal("b has no physical slot")
	}
	must(t, h.bind(bSlot, &alias))
	must(t, h.referenceObject(&temp, o, "b", nil))
	must(t, h.unset(&temp))
	must(t, h.bind(&globalAlias, &alias))

	attemptResults := []any{}
	for range 2 {
		attemptResults = append(attemptResults, w13ObserveValue(func() (value, error) {
			return h.readObject(o, "trigger", nil)
		}))
		attemptResults = append(attemptResults, w13ObserveError(func() error { return h.assign(&alias, w13Str("bad")) }))
	}

	pA, err := resolvePropertyIn(child, "a", child, true)
	must(t, err)
	aSlot, ok := o.object.props.lookup(pA.key)
	if !ok {
		t.Fatal("a has no physical slot")
	}
	hidden, err := resolvePropertyIn(base, "hidden", base, true)
	must(t, err)
	hiddenSlot, ok := o.object.props.lookup(hidden.key)
	if !ok {
		t.Fatal("inherited private property has no physical slot")
	}
	if hiddenSlot.kind != valueUndefined {
		t.Fatal("rollback initialized an untouched inherited private slot")
	}
	values := []any{w13Encode(*aSlot), w13Encode(*bSlot), w13Encode(alias)}
	result := []any{attemptResults, events, values, h.isLazyObject(o), isLazyPropertyInfo(o.object, hidden), calls}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R8PHP8512Regression(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R8Names() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R8Case(t, h, name))
			})
		}
	}
}
