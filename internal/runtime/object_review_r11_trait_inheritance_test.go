package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func w13R11TraitInheritanceNames() []string {
	var names []string
	for _, access := range []string{"public", "protected"} {
		for _, kind := range []string{"instance", "static"} {
			for _, count := range []string{"one", "two"} {
				names = append(names, "r11-trait-inherit-"+access+"_"+kind+"_"+count)
			}
		}
	}
	names = append(names, "r11-trait-iterable-normal", "r11-trait-iterable-lazy")
	return append(names, "r11-trait-setter-protected-private", "r11-trait-setter-private-protected", "r11-trait-setter-public-protected")
}

// REQ-018; T-035/036. Composition and inheritance have different collision rules.
func w13R11TraitInheritanceCase(t *testing.T, h *heap, name string) w13Case {
	var result any
	var owners []value
	defer func() {
		for i := range owners {
			must(t, h.unset(&owners[i]))
		}
		must(t, h.shutdown(&value{}))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("trait inheritance retained request state")
		}
	}()
	own := func(v value) value {
		var owner value
		must(t, h.assign(&owner, v))
		owners = append(owners, owner)
		return v
	}
	if suffix, ok := strings.CutPrefix(name, "r11-trait-inherit-"); ok {
		parts := strings.Split(suffix, "_")
		property := propertySpec{name: "x", typeName: "int", static: parts[1] == "static", hasDefault: true, initial: int64(9)}
		if parts[0] == "protected" {
			property.visibility = protectedAccess
		}
		p := w13Build(t, classSpec{name: "W13R11Parent_" + suffix, properties: []propertySpec{property}})
		property.initial = int64(2)
		a := w13Build(t, classSpec{name: "W13R11ImportA_" + suffix, isTrait: true, properties: []propertySpec{property}})
		b := w13Build(t, classSpec{name: "W13R11ImportB_" + suffix, isTrait: true, properties: []propertySpec{property}})
		traits := []*class{a}
		if parts[2] == "two" {
			traits = append(traits, b)
		}
		c := w13Build(t, classSpec{name: "W13R11Child_" + suffix, parent: p, traits: traits})
		d := w13Build(t, classSpec{name: "W13R11Plain_" + suffix, parent: p})
		classes := []*class{p, c, d}
		var objects []value
		if !property.static {
			for _, class := range classes {
				objects = append(objects, own(w13Instantiate(t, h, class)))
			}
		}
		read := func(i int) any {
			var v value
			var err error
			if property.static {
				v, err = h.readStatic(classes[i], "x", classes[i])
			} else {
				v, err = h.readObject(objects[i], "x", classes[i])
			}
			must(t, err)
			return w13Encode(v)
		}
		before := []any{read(0), read(1), read(2)}
		for i, n := range []int64{10, 20} {
			if property.static {
				must(t, h.writeStatic(classes[i], "x", w13Int(n), classes[i], true))
			} else {
				must(t, h.writeObject(objects[i], "x", w13Int(n), classes[i], true))
			}
		}
		result = []any{[]any{c.schema.props[0].owner.name, d.schema.props[0].owner.name}, before, []any{read(0), read(1), read(2)}}
	} else if strings.HasPrefix(name, "r11-trait-iterable-") {
		a := w13Build(t, classSpec{name: "W13R11IterableA", isTrait: true, properties: []propertySpec{{name: "x", typeName: "iterable", hasDefault: true, initial: []SnapshotEntry{}}}})
		b := w13Build(t, classSpec{name: "W13R11IterableB", isTrait: true, properties: []propertySpec{{name: "x", typeName: "Traversable|array", hasDefault: true, initial: []SnapshotEntry{}}}})
		c := w13Build(t, classSpec{name: "W13R11Iterable", traits: []*class{a, b}})
		calls := 0
		var o value
		if strings.HasSuffix(name, "-lazy") {
			var err error
			o, err = h.newLazyObject(c, false, func(*heap, value) (value, error) { calls++; return nullValue(), nil })
			must(t, err)
		} else {
			o = w13Instantiate(t, h, c)
		}
		own(o)
		before := []any{h.isLazyObject(o), calls}
		rhs, err := h.importSnapshotPHP8512([]SnapshotEntry{{Key: int64(0), Value: int64(2)}}, 2, 1)
		must(t, err)
		must(t, h.writeRawObjectProperty(o, "x", rhs, false))
		must(t, h.unset(&rhs))
		v, err := h.readObject(o, "x", nil)
		must(t, err)
		result = []any{before, []any{h.isLazyObject(o), calls}, w13Encode(v)}
	} else {
		suffix := strings.TrimPrefix(name, "r11-trait-setter-")
		parts := strings.Split(suffix, "-")
		className := "W13R11Setter_" + strings.ReplaceAll(suffix, "-", "_")
		accesses := map[string]access{"public": publicAccess, "protected": protectedAccess, "private": privateAccess}
		property := propertySpec{name: "x", typeName: "int", hasDefault: true, initial: int64(1), setVisibility: accesses[parts[0]]}
		a := w13Build(t, classSpec{name: className + "A", isTrait: true, properties: []propertySpec{property}})
		property.setVisibility = accesses[parts[1]]
		b := w13Build(t, classSpec{name: className + "B", isTrait: true, properties: []propertySpec{property}})
		c := w13Build(t, classSpec{name: className, traits: []*class{a, b}})
		o := own(w13Instantiate(t, h, c))
		setter := c.schema.props[0].setVisibility
		err := h.writeObject(o, "x", w13Int(2), nil, true)
		var failure any
		if err != nil {
			failure = w13Error(err)
		}
		result = []any{setter == privateAccess, setter == protectedAccess, failure, w13Encode(w13Read(t, h, o, "x", nil))}
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func TestW13R11TraitInheritance(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R11TraitInheritanceNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R11TraitInheritanceCase(t, h, name))
			})
		}
	}
}
