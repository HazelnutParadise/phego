package runtime

import (
	"fmt"
	"strings"
	"testing"
)

// REQ-018; T-035/036. Reference aliases must retain the declaring property's
// type source while caller strictness controls ordinary PHP writes.
func w13R10AliasNames() []string {
	names := make([]string, 0, 20)
	for _, storage := range []string{"declared", "dynamic", "static", "raw", "raw-lazy"} {
		for _, strict := range []string{"weak", "strict"} {
			for _, input := range []string{"numeric", "bad"} {
				names = append(names, fmt.Sprintf("r10-alias-%s-%s-%s", storage, strict, input))
			}
		}
	}
	return names
}

func TestW13R10PropertyAliasStrictness(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R10AliasNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				w13AssertOracleCase(t, width, name, w13R10AliasCase(t, h, name))
			})
		}
	}
}

func w13R10AliasCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(name, "r10-alias-"), "-")
	if len(parts) < 3 {
		t.Fatalf("unknown R10 alias case %q", name)
	}
	inputName, strictName := parts[len(parts)-1], parts[len(parts)-2]
	storage := strings.Join(parts[:len(parts)-2], "-")
	if (storage != "declared" && storage != "dynamic" && storage != "static" && storage != "raw" && storage != "raw-lazy") || (strictName != "weak" && strictName != "strict") || (inputName != "numeric" && inputName != "bad") {
		t.Fatalf("unknown R10 alias case %q", name)
	}
	strict, input := strictName == "strict", inputName
	c := w13Build(t, classSpec{name: "W13R10Alias", allowDynamic: true, properties: []propertySpec{
		{name: "a", hasDefault: true, initial: int64(1)},
		{name: "b", typeName: "int"},
		{name: "spare", typeName: "int", hasDefault: true, initial: int64(42)},
		{name: "s", static: true, hasDefault: true, initial: int64(1)},
		{name: "t", typeName: "int", static: true},
	}})

	var owner, alias, temp value
	var calls int
	if storage == "raw-lazy" {
		obj, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { calls++; return value{}, nil })
		must(t, err)
		must(t, h.assign(&owner, obj))
	} else {
		obj := w13Instantiate(t, h, c)
		must(t, h.assign(&owner, obj))
	}
	defer func() {
		must(t, h.unset(&temp))
		must(t, h.unset(&alias))
		must(t, h.unset(&owner))
		must(t, h.shutdown(&value{}))
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatalf("R10 alias fixture retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
		}
	}()

	field := "a"
	if storage == "dynamic" {
		field = "d"
		must(t, h.writeObject(owner, field, w13Int(1), nil, false))
	}
	if storage == "raw-lazy" {
		must(t, h.writeRawObjectProperty(owner, "a", w13Int(1), false))
		must(t, h.writeRawObjectProperty(owner, "b", w13Int(1), false))
	}
	var operation func(value) error
	if storage == "static" {
		var staticAlias value
		must(t, h.referenceStatic(&staticAlias, c, "s", nil))
		table, err := h.staticTable(c)
		must(t, err)
		tSlot, ok := table.lookup("t")
		if !ok || tSlot == nil {
			t.Fatal("static t has no physical slot")
		}
		must(t, h.bind(tSlot, &staticAlias))
		must(t, h.referenceStatic(&temp, c, "t", nil)) // install t's typed source
		must(t, h.unset(&temp))
		operation = func(v value) error { return h.writeStatic(c, "s", v, nil, strict) }
		must(t, h.assign(&alias, staticAlias))
		must(t, h.unset(&staticAlias))
	} else {
		must(t, h.referenceObject(&alias, owner, field, nil))
		b, err := resolvePropertyIn(c, "b", c, true)
		must(t, err)
		bSlot, ok := owner.object.props.lookup(b.key)
		if !ok || bSlot == nil {
			t.Fatal("instance b has no physical slot")
		}
		must(t, h.bind(bSlot, &alias))
		must(t, h.referenceObject(&temp, owner, "b", nil)) // install b's typed source
		must(t, h.unset(&temp))
		operation = func(v value) error {
			if storage == "raw" || storage == "raw-lazy" {
				// Native Reflection raw writes always use weak property coercion.
				return h.writeRawObjectProperty(owner, field, v, false)
			}
			return h.writeObject(owner, field, v, nil, strict)
		}
	}

	rhs := w13Str("bad")
	if input == "numeric" {
		rhs = w13Str("2")
	}
	result := w13ObserveError(func() error { return operation(rhs) })
	var values []any
	if storage == "static" {
		values = []any{w13ObserveValue(func() (value, error) { return h.readStatic(c, "s", nil) }), w13ObserveValue(func() (value, error) { return h.readStatic(c, "t", nil) })}
	} else {
		values = []any{w13ObserveValue(func() (value, error) { return h.readObject(owner, field, nil) }), w13ObserveValue(func() (value, error) { return h.readObject(owner, "b", nil) })}
	}
	out := []any{result, values[0], values[1]}
	if storage == "raw" || storage == "raw-lazy" {
		out = append(out, int64(calls), h.isLazyObject(owner))
	}
	return w13Case{Result: out, Diagnostics: w13Diagnostics(h)}
}
