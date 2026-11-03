package runtime

import "testing"

func w13R5LifetimeNames() []string {
	return []string{"r5-life-throw-1", "r5-life-throw-2", "r5-life-cleanup-order", "r5-life-self", "r5-life-parent", "r5-life-invalid-array", "r5-life-mixed-array", "r5-life-source-inherited", "r5-life-source-redeclared", "r5-life-source-private", "r5-life-source-trait"}
}

// REQ-018/020; T-035/036/039/040. RHS remains alive through verification/store.
func w13R5LifetimeCase(t *testing.T, h *heap, name string) w13Case {
	switch name {
	case "r5-life-source-inherited", "r5-life-source-redeclared", "r5-life-source-private", "r5-life-source-trait":
		return w13R5SourceIdentityCase(t, h, name)
	}
	if name == "r5-life-self" || name == "r5-life-parent" {
		return w13R5RelativeTypeCase(t, h, name)
	}
	if name == "r5-life-invalid-array" || name == "r5-life-mixed-array" {
		return w13R5UncoercibleCase(t, h, name)
	}
	events := []any{}
	order := name == "r5-life-cleanup-order"
	className, typ, initial := "W13R5Life", "string", any([]byte("old"))
	if order {
		className, typ, initial = "W13R5Array", "array|string", nil
	}
	c := w13Build(t, classSpec{name: className, properties: []propertySpec{{name: "a", typeName: typ, hasDefault: !order, initial: initial}, {name: "b", typeName: typ, hasDefault: !order, initial: initial}}})
	var owner, alias, rhs value
	must(t, h.assign(&owner, w13Instantiate(t, h, c)))
	if order {
		oldClass := w13Build(t, classSpec{name: "W13R5Old", destructor: func(*heap, value) error { events = append(events, "old"); return nil }})
		var arr value
		must(t, h.appendValue(&arr, w13Instantiate(t, h, oldClass)))
		must(t, h.writeObject(owner, "a", arr, nil, false))
		must(t, h.unset(&arr))
	}
	must(t, h.referenceObject(&alias, owner, "a", nil))
	if name != "r5-life-throw-1" {
		slot := owner.object.props.slot("b")
		must(t, h.bind(slot, &alias))
		h.attachPropertySource(owner.object, slot, "b")
	}
	spec := classSpec{name: "W13R5Throw", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
		events = append(events, "convert")
		if !order {
			return value{}, &Throwable{"Exception", "convert"}
		}
		if err := h.unset(&rhs); err != nil {
			return value{}, err
		}
		return w13Str("new"), nil
	}}}}
	if order {
		spec.name = "W13R5LifeString"
		spec.destructor = func(*heap, value) error { events = append(events, "rhs"); return nil }
	}
	must(t, h.assign(&rhs, w13Instantiate(t, h, w13Build(t, spec))))
	result := w13R5ReferenceAttempt(func() (value, error) {
		if err := h.assignPHP(&alias, rhs, false); err != nil {
			return value{}, err
		}
		return w13Str("ok"), nil
	})
	observation := []any{result, w13Encode(alias.deref()), append([]any{}, events...)}
	must(t, h.unset(&rhs))
	must(t, h.unset(&alias))
	must(t, h.unset(&owner))
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatal("live objects", len(h.live), "typed sources", len(h.typedSources))
	}
	return w13Case{Result: observation, Diagnostics: w13Diagnostics(h)}
}

func w13R5UncoercibleCase(t *testing.T, h *heap, name string) w13Case {
	c := w13Build(t, classSpec{name: "W13R5MixedScalar", properties: []propertySpec{{name: "m", typeName: "mixed", hasDefault: true, initial: int64(1)}, {name: "i", typeName: "int", hasDefault: true, initial: int64(1)}}})
	var owner, alias, input value
	must(t, h.assign(&owner, w13Instantiate(t, h, c)))
	if name == "r5-life-mixed-array" {
		must(t, h.referenceObject(&alias, owner, "m", nil))
		slot := owner.object.props.slot("i")
		must(t, h.bind(slot, &alias))
		h.attachPropertySource(owner.object, slot, "i")
	} else {
		must(t, h.referenceObject(&alias, owner, "i", nil))
	}
	must(t, h.assign(&input, h.newArrayValue()))
	result := w13R5ReferenceAttempt(func() (value, error) { return nullValue(), h.assignPHP(&alias, input, false) })
	must(t, h.unset(&input))
	must(t, h.unset(&alias))
	must(t, h.unset(&owner))
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatal("live objects", len(h.live), "typed sources", len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13R5RelativeTypeCase(t *testing.T, h *heap, name string) w13Case {
	var owner, alias, target value
	spec := classSpec{name: "W13R5Self", properties: []propertySpec{{name: "p", typeName: "?self", hasDefault: true}}}
	if name == "r5-life-parent" {
		base := w13Build(t, classSpec{name: "W13R5ParentBase"})
		spec.name, spec.parent, spec.properties[0].typeName = "W13R5Parent", base, "?parent"
		must(t, h.assign(&target, w13Instantiate(t, h, base)))
	}
	must(t, h.assign(&owner, w13Instantiate(t, h, w13Build(t, spec))))
	if name == "r5-life-self" {
		must(t, h.assign(&target, owner))
	}
	must(t, h.referenceObject(&alias, owner, "p", nil))
	result := w13R5ReferenceAttempt(func() (value, error) {
		if err := h.assignPHP(&alias, target, true); err != nil {
			return value{}, err
		}
		return w13Str("ok"), nil
	})
	same, err := identical(alias.deref(), target)
	must(t, err)
	observed := []any{result, same}
	must(t, h.assignPHP(&alias, nullValue(), true))
	must(t, h.unset(&alias))
	must(t, h.unset(&target))
	must(t, h.unset(&owner))
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatal("live objects", len(h.live), "typed sources", len(h.typedSources))
	}
	return w13Case{Result: observed, Diagnostics: w13Diagnostics(h)}
}

// An inherited property definition retains its identity across descriptor copies;
// redeclarations and separate trait consumers have distinct declaring owners.
func w13R5SourceIdentityCase(t *testing.T, h *heap, name string) w13Case {
	props := []propertySpec{{name: "a", typeName: "string", hasDefault: true, initial: []byte("old")}, {name: "m", typeName: "mixed", hasDefault: true, initial: []byte("old")}}
	baseSpec := classSpec{name: "W13R5InfoBase", properties: props}
	var base, child, sourceScope, destinationScope *class
	switch name {
	case "r5-life-source-private":
		baseSpec.name = "W13R5PrivateBase"
		baseSpec.properties[0].visibility = privateAccess
		base = w13Build(t, baseSpec)
		child = w13Build(t, classSpec{name: "W13R5PrivateChild", parent: base, properties: []propertySpec{{name: "a", typeName: "string", visibility: privateAccess, hasDefault: true, initial: []byte("old")}}})
		sourceScope, destinationScope = base, child
	case "r5-life-source-trait":
		trait := w13Build(t, classSpec{name: "W13R5InfoTrait", isTrait: true, properties: props})
		base = w13Build(t, classSpec{name: "W13R5TraitA", traits: []*class{trait}})
		child = w13Build(t, classSpec{name: "W13R5TraitB", traits: []*class{trait}})
	default:
		base = w13Build(t, baseSpec)
		childSpec := classSpec{name: "W13R5InfoChild", parent: base}
		if name == "r5-life-source-redeclared" {
			childSpec.name = "W13R5InfoRedeclared"
			childSpec.properties = []propertySpec{props[0]}
		}
		child = w13Build(t, childSpec)
	}
	var source, destination, alias, rhs, std value
	must(t, h.assign(&source, w13Instantiate(t, h, base)))
	must(t, h.assign(&destination, w13Instantiate(t, h, child)))
	must(t, h.referenceObject(&alias, source, "a", sourceScope))
	p, err := h.resolveInstanceProperty(destination.object, "a", destinationScope, false)
	must(t, err)
	slot := destination.object.props.slot(p.key)
	must(t, h.bind(slot, &alias))
	h.attachPropertySource(destination.object, slot, p.key)
	slot = source.object.props.slot("m")
	must(t, h.bind(slot, &alias))
	h.attachPropertySource(source.object, slot, "m")
	must(t, h.unsetObject(destination, "a", destinationScope))
	events := []any{}
	text := w13Build(t, classSpec{name: "W13R5InfoStringable", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
		events = append(events, "convert")
		return w13Str("new"), nil
	}}}})
	must(t, h.assign(&rhs, w13Instantiate(t, h, text)))
	assignment := w13R5ReferenceAttempt(func() (value, error) { return nullValue(), h.assignPHP(&alias, rhs, false) })
	result := []any{assignment, append([]any{}, events...)}
	must(t, h.unsetObject(source, "a", sourceScope))
	must(t, h.assign(&std, w13Instantiate(t, h, w13Build(t, classSpec{name: "stdClass"}))))
	result = append(result, w13R5ReferenceAttempt(func() (value, error) { return nullValue(), h.assignPHP(&alias, std, false) }), typeName(alias.deref()))
	for _, owner := range []*value{&alias, &rhs, &std, &source, &destination} {
		must(t, h.unset(owner))
	}
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatal("retained objects/sources", len(h.live), len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
