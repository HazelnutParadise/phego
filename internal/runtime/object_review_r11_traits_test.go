package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R11TraitNames() []string {
	families := []string{"private-int", "private-null", "private-uninitialized", "private-nullable", "private-union", "private-array", "private-empty-array", "private-string", "private-float", "private-self", "public-int", "protected-int", "ancestor-private", "class-own-private", "nested-private", "readonly-private"}
	var names []string
	for _, family := range families {
		for _, mode := range []string{"normal", "lazy"} {
			names = append(names, "r11-trait-"+family+"-"+mode)
		}
	}
	return names
}

func w13R11TraitCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(name, "-")
	family, mode := strings.Join(parts[2:len(parts)-1], "-"), parts[len(parts)-1]
	suffix := strings.ReplaceAll(family, "-", "_")
	className := "W13R11Traits_" + suffix
	aName, bName := "W13R11TraitA_"+suffix, "W13R11TraitB_"+suffix
	parentName := "W13R11TraitParent_" + suffix
	typA, typB, defaultA, defaultB := "int", "int", any(int64(1)), any(int64(1))
	hasA, hasB := true, true
	vis := privateAccess
	readonly := false
	switch family {
	case "private-null":
		typA, typB, defaultA, defaultB, hasA, hasB = "", "", nil, nil, false, true
	case "private-uninitialized":
		defaultA, defaultB, hasA, hasB = nil, nil, false, false
	case "private-nullable":
		typA, typB, defaultA, defaultB = "?int", "int|null", nil, nil
	case "private-union":
		typA, typB = "int|string", "string|int"
	case "private-array":
		typA, typB, defaultA, defaultB = "array", "array", []SnapshotEntry{{Key: "k", Value: int64(1)}, {Key: int64(2), Value: []byte("v")}}, []SnapshotEntry{{Key: "k", Value: int64(1)}, {Key: int64(2), Value: []byte("v")}}
	case "private-empty-array":
		typA, typB, defaultA, defaultB = "array", "array", []SnapshotEntry{}, []SnapshotEntry{}
	case "private-string":
		typA, typB, defaultA, defaultB = "string", "string", []byte{}, []byte{}
	case "private-float":
		typA, typB, defaultA, defaultB = "float", "float", int64(1), float64(1)
	case "private-self":
		typA, typB, defaultA, defaultB, hasA, hasB = "self", className, nil, nil, false, false
	case "public-int":
		vis = publicAccess
	case "protected-int":
		vis = protectedAccess
	case "readonly-private":
		readonly, defaultA, defaultB, hasA, hasB = true, nil, nil, false, false
	}
	prop := func(typ string, initial any, hasDefault bool) propertySpec {
		return propertySpec{name: "x", typeName: typ, visibility: vis, readonly: readonly, hasDefault: hasDefault, initial: initial}
	}
	ta := w13Build(t, classSpec{name: aName, isTrait: true, properties: []propertySpec{prop(typA, defaultA, hasA)}})
	var tb *class
	if family == "nested-private" {
		tb = w13Build(t, classSpec{name: bName, isTrait: true, traits: []*class{ta}})
	} else {
		tb = w13Build(t, classSpec{name: bName, isTrait: true, properties: []propertySpec{prop(typB, defaultB, hasB)}})
	}
	var parent *class
	if family == "ancestor-private" {
		parent = w13Build(t, classSpec{name: parentName, properties: []propertySpec{{name: "x", typeName: "int", visibility: privateAccess, hasDefault: true, initial: int64(9)}}, methods: []methodSpec{{name: "parentPeek", call: func(h *heap, this value, _ []value) (value, error) { return h.readObject(this, "x", parent) }}}})
	}
	var c *class
	methods := []methodSpec{{name: "peek", call: func(h *heap, this value, _ []value) (value, error) { return h.readObject(this, "x", c) }}}
	props := []propertySpec(nil)
	if family == "class-own-private" {
		props = append(props, propertySpec{name: "x", typeName: "int", visibility: privateAccess, hasDefault: true, initial: int64(1)})
	}
	c = w13Build(t, classSpec{name: className, parent: parent, traits: []*class{ta, tb}, properties: props, methods: methods})
	var calls int64
	var o value
	if mode == "normal" {
		o = w13Instantiate(t, h, c)
	} else {
		var err error
		o, err = h.newLazyObject(c, false, func(*heap, value) (value, error) { calls++; return nullValue(), nil })
		w13Must(t, "newLazyObject", err)
	}
	var owner value
	w13Must(t, "own object", h.assign(&owner, o))
	metadata := make([]any, 0, len(c.schema.props))
	for _, p := range c.schema.props {
		if p.owner == parent && p.visibility == privateAccess {
			continue
		}
		metadata = append(metadata, []any{p.name, p.owner.name})
	}
	peek := func() any {
		v, e := h.callMethod(o, "peek", nil, nil, false)
		if e != nil {
			return w13Error(e)
		}
		observed := []any{valueTypeName(v), w13Encode(v)}
		w13Must(t, "discard peek result", h.discardMethodReturn(v))
		return observed
	}
	var initial any
	if mode == "normal" {
		initial = peek()
	}
	before := []any{h.isLazyObject(o), int(calls)}
	rhs := w13Int(7)
	var write any
	importedArray := false
	switch family {
	case "private-array", "private-empty-array":
		var e error
		rhs, e = h.importSnapshotPHP8512([]SnapshotEntry{{Key: int64(0), Value: int64(2)}}, 1024, 1<<20)
		w13Must(t, "array value", e)
		importedArray = true
	case "private-string", "private-union":
		rhs = w13Str("new")
	case "private-float":
		rhs = value{kind: valueFloat, floating: 7.5}
	case "private-self":
		rhs = o
	}
	write = w13ObserveError(func() error { return h.writeRawObjectProperty(o, "x", rhs, false) })
	if importedArray {
		w13Must(t, "release imported RHS array", h.unset(&rhs))
	}
	afterRaw := []any{h.isLazyObject(o), int(calls)}
	var seen any
	if family == "private-self" {
		v, e := h.callMethod(o, "peek", nil, nil, false)
		seen = e == nil && v.object == o.object
		if e == nil {
			w13Must(t, "discard identity peek result", h.discardMethodReturn(v))
		}
	} else {
		seen = peek()
	}
	var parentSeen any
	if family == "ancestor-private" {
		v, e := h.callMethod(o, "parentPeek", nil, parent, false)
		if e != nil {
			parentSeen = w13Error(e)
		} else {
			parentSeen = w13Encode(v)
			w13Must(t, "discard parent peek result", h.discardMethodReturn(v))
		}
	}
	out := []any{metadata, initial, before, write, afterRaw, seen, parentSeen, []any{h.isLazyObject(o), int(calls)}}
	observed := w13Case{Result: out, Diagnostics: w13Diagnostics(h)}
	w13Must(t, "release object", h.unset(&owner))
	if e := h.shutdown(&value{}); e != nil {
		t.Fatalf("shutdown: %v", e)
	}
	if len(h.live) != 0 || len(h.typedSources) != 0 {
		t.Fatalf("retained objects/sources: %d/%d", len(h.live), len(h.typedSources))
	}
	return observed
}

func valueTypeName(v value) string {
	v = v.deref()
	switch v.kind {
	case valueUndefined, valueNull:
		return "null"
	case valueBool:
		return "bool"
	case valueInt:
		return "int"
	case valueFloat:
		return "float"
	case valueString:
		return "string"
	case valueArray:
		return "array"
	case valueObject:
		return v.object.class.name
	}
	return fmt.Sprint(v.kind)
}

func TestW13R11TraitProperties(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for _, name := range w13R11TraitNames() {
				t.Run(name, func(t *testing.T) {
					h, err := newHeap(width)
					w13Must(t, "newHeap", err)
					w13AssertOracleCase(t, width, name, w13R11TraitCase(t, h, name))
				})
			}
		})
	}
}
