package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func w13R5ReferenceNames() []string {
	return []string{
		"r5-ref-control", "r5-ref-unset-first", "r5-ref-unset-next", "r5-ref-unset-all",
		"r5-ref-add-source", "r5-ref-replace-source", "r5-ref-unset-alias", "r5-ref-rebind-alias",
		"r5-ref-drop-rhs", "r5-ref-string-mixed", "r5-ref-mixed-first", "r5-ref-string-objectunion",
		"r5-ref-string-union", "r5-ref-triple-unset-first", "r5-ref-triple-unset-next",
		"r5-ref-triple-unset-last", "r5-ref-triple-replace-last", "r5-ref-append-remove",
		"r5-ref-remove-append", "r5-ref-duplicate-clone", "r5-ref-rebind-typed",
	}
}

func w13R5ReferenceAttempt(fn func() (value, error)) any {
	v, err := fn()
	if err == nil {
		return w13Encode(v)
	}
	out := []any{}
	for err != nil {
		var c completion
		if errors.As(err, &c) {
			if c.err != nil {
				out = append(out, w13R5Throwable(c.err))
			}
			err = c.previous
			continue
		}
		out = append(out, w13R5Throwable(err))
		err = errors.Unwrap(err)
	}
	return out
}

func w13R5Throwable(err error) any {
	var php *Throwable
	if errors.As(err, &php) {
		return []any{php.Class, php.Message}
	}
	return []any{"Error", err.Error()}
}

func w13R5ReferenceCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	mode := name[len("r5-ref-"):]
	triple := mode == "triple-unset-first" || mode == "triple-unset-next" || mode == "triple-unset-last" || mode == "triple-replace-last"
	mutations := triple || mode == "append-remove" || mode == "remove-append" || mode == "duplicate-clone" || mode == "rebind-typed"
	className := "W13R5Reference"
	props := []propertySpec{
		{name: "a", typeName: "string", hasDefault: true, initial: []byte("old")},
		{name: "b", typeName: "string", hasDefault: true, initial: []byte("old")},
		{name: "c", typeName: "string", hasDefault: true, initial: []byte("old")},
	}
	if mutations {
		className = "W13R5Mutation"
		props = append(props, propertySpec{name: "m", typeName: "mixed", hasDefault: true, initial: []byte("old")})
	} else {
		props = append(props,
			propertySpec{name: "m", typeName: "mixed", hasDefault: true, initial: []byte("old")},
			propertySpec{name: "u", typeName: "int|string", hasDefault: true, initial: []byte("old")},
			propertySpec{name: "so", typeName: "object|string", hasDefault: true, initial: []byte("old")},
		)
	}
	c := w13Build(t, classSpec{name: className, properties: props})
	o := w13Instantiate(t, h, c)
	var objOwner value
	must(t, h.assign(&objOwner, o))

	// Every PHP variable has a stable Go cell. Unsetting removes its name and
	// clears that cell; an in-flight assignment retains its previous wrapper.
	globals := map[string]*value{}
	var aliasCell, otherCell, rhsCell, targetOwner, cloneOwner, stdOwner value
	must(t, h.assign(&aliasCell, w13Str("old")))
	must(t, h.assign(&otherCell, w13Str("other")))
	globals["r5_alias"], globals["r5_other"] = &aliasCell, &otherCell

	property := func(field string) (*value, *propertyInfo) {
		slot := o.object.props.slot(field)
		for i := range c.schema.props {
			if c.schema.props[i].name == field {
				return slot, &c.schema.props[i]
			}
		}
		t.Fatalf("missing property metadata for %s", field)
		return nil, nil
	}
	// Bind the initial PHP alias from a property. Additional aliases first bind
	// their slot to the existing variable cell, then use referenceObject to
	// register the typed property source through the production registration path.
	firstSource := func(field string, cell *value) { must(t, h.referenceObject(cell, o, field, nil)) }
	attach := func(field string, cell *value) {
		slot, _ := property(field)
		if slot.kind == valueUndefined {
			must(t, h.writeObject(o, field, w13Str("old"), nil, false))
		}
		if slot.kind == valueReference && cell.kind == valueReference && slot.ref == cell.ref {
			for _, source := range h.typedSources[slot.ref] {
				if source.slot == slot {
					return
				}
			}
		}
		must(t, h.bind(slot, cell))
		h.attachPropertySource(o.object, slot, field)
	}
	if mode == "mixed-first" {
		firstSource("m", globals["r5_alias"])
		attach("a", globals["r5_alias"])
	} else {
		firstSource("a", globals["r5_alias"])
		switch mode {
		case "string-mixed":
			attach("m", globals["r5_alias"])
		case "string-objectunion":
			attach("so", globals["r5_alias"])
		case "string-union":
			attach("u", globals["r5_alias"])
		case "triple-unset-first", "triple-unset-next", "triple-unset-last", "triple-replace-last":
			attach("b", globals["r5_alias"])
			attach("m", globals["r5_alias"])
		default:
			attach("b", globals["r5_alias"])
		}
	}
	if mode == "duplicate-clone" {
		must(t, h.unsetObject(o, "b", nil))
		clone, err := h.cloneObject(o, nil)
		must(t, err)
		must(t, h.assign(&cloneOwner, clone))
		attach("m", globals["r5_alias"])
		must(t, h.unsetObject(cloneOwner, "a", nil))
	}
	if mutations {
		targetClass := w13Build(t, classSpec{name: "W13R5IntTarget", properties: []propertySpec{{name: "i", typeName: "int", hasDefault: true, initial: int64(7)}}})
		must(t, h.assign(&targetOwner, w13Instantiate(t, h, targetClass)))
	}

	events := []any{}
	calls := 0
	var action func() error
	action = func() error {
		calls++
		if mutations && calls > 1 {
			return nil
		}
		switch mode {
		case "unset-first", "triple-unset-first":
			return h.unsetObject(o, "a", nil)
		case "unset-next", "triple-unset-next":
			return h.unsetObject(o, "b", nil)
		case "unset-all":
			if err := h.unsetObject(o, "a", nil); err != nil {
				return err
			}
			return h.unsetObject(o, "b", nil)
		case "triple-unset-last":
			return h.unsetObject(o, "m", nil)
		case "add-source":
			attach("c", globals["r5_alias"])
		case "replace-source":
			if err := h.unsetObject(o, "b", nil); err != nil {
				return err
			}
			attach("b", globals["r5_other"])
		case "triple-replace-last":
			if err := h.unsetObject(o, "m", nil); err != nil {
				return err
			}
			attach("c", globals["r5_alias"])
		case "append-remove":
			attach("m", globals["r5_alias"])
			return h.unsetObject(o, "b", nil)
		case "remove-append":
			if err := h.unsetObject(o, "b", nil); err != nil {
				return err
			}
			attach("m", globals["r5_alias"])
		case "unset-alias":
			cell := globals["r5_alias"]
			delete(globals, "r5_alias")
			if cell == nil {
				return nil
			}
			return h.unset(cell)
		case "rebind-alias":
			return h.bind(globals["r5_alias"], globals["r5_other"])
		case "drop-rhs":
			cell := globals["r5_rhs"]
			delete(globals, "r5_rhs")
			if cell == nil {
				return nil
			}
			return h.unset(cell)
		case "rebind-typed":
			return h.referenceObject(globals["r5_alias"], targetOwner, "i", nil)
		}
		return nil
	}
	stringSpec := classSpec{name: "W13R5Stringable", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) {
		events = append(events, "convert")
		if err := action(); err != nil {
			return value{}, err
		}
		return w13Str("new"), nil
	}}}}
	if mutations {
		stringSpec.name = "W13R5MutationStringable"
	} else {
		stringSpec.destructor = func(h *heap, _ value) error {
			events = append(events, "dtor:"+w13R5ReadA(t, h, o))
			return nil
		}
	}
	stringClass := w13Build(t, stringSpec)
	must(t, h.assign(&rhsCell, w13Instantiate(t, h, stringClass)))
	globals["r5_rhs"] = &rhsCell
	assignmentLocation := globals["r5_alias"]
	rhsArgument := *globals["r5_rhs"]
	assignment := w13R5ReferenceAttempt(func() (value, error) {
		if err := h.assignPHP(assignmentLocation, rhsArgument, false); err != nil {
			return value{}, err
		}
		return w13Str("ok"), nil
	})

	observedProps := map[string]any{}
	fields := []string{"a", "b", "c", "m", "u", "so"}
	if mutations {
		fields = []string{"a", "b", "c", "m"}
	}
	for _, field := range fields {
		isset, err := h.issetObject(o, field, nil)
		must(t, err)
		if !isset {
			observedProps[field] = "unset"
			continue
		}
		v, err := h.readObject(o, field, nil)
		must(t, err)
		observedProps[field] = w13Encode(v)
	}
	globalAlias := any("unset")
	if cell, ok := globals["r5_alias"]; ok {
		globalAlias = w13Encode(cell.deref())
	}
	otherValue := any(w13Encode(otherCell.deref()))
	if mutations {
		otherValue = w13Encode(targetOwner.object.props.slot("i").deref())
	}
	eventSnapshot := append([]any{}, events...)
	result := []any{assignment, observedProps, globalAlias, otherValue, eventSnapshot}
	if mode == "duplicate-clone" {
		must(t, h.unsetObject(o, "a", nil))
		stdClass := w13Build(t, classSpec{name: "stdClass"})
		must(t, h.assign(&stdOwner, w13Instantiate(t, h, stdClass)))
		result = append(result, w13R5ReferenceAttempt(func() (value, error) {
			if err := h.assignPHP(assignmentLocation, stdOwner, false); err != nil {
				return value{}, err
			}
			return w13Str("ok"), nil
		}), typeName(stdOwner))
	}

	// Snapshot all observable values before cleanup; cleanup order mirrors the
	// PHP fixture's release of globals followed by its retained object owner.
	for _, key := range []string{"r5_rhs", "r5_alias", "r5_other"} {
		if cell, ok := globals[key]; ok {
			must(t, h.unset(cell))
			delete(globals, key)
		}
	}
	must(t, h.unset(&aliasCell))
	must(t, h.unset(&otherCell))
	must(t, h.unset(&rhsCell))
	if cloneOwner.kind != valueUndefined {
		must(t, h.unset(&cloneOwner))
	}
	if stdOwner.kind != valueUndefined {
		must(t, h.unset(&stdOwner))
	}
	if targetOwner.kind != valueUndefined {
		must(t, h.unset(&targetOwner))
	}
	must(t, h.unset(&objOwner))
	if h.liveObjects != 0 || len(h.typedSources) != 0 {
		t.Fatalf("reference fixture retained objects=%d sources=%d", h.liveObjects, len(h.typedSources))
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13R5ReadA(t *testing.T, h *heap, obj value) string {
	t.Helper()
	isset, err := h.issetObject(obj, "a", nil)
	must(t, err)
	if !isset {
		return "unset"
	}
	v, err := h.readObject(obj, "a", nil)
	must(t, err)
	return v.text
}

func TestW13R5PHP8512Regression(t *testing.T) {
	dir := os.Getenv("PHEGO_W13_REFERENCE")
	if dir == "" {
		dir = "testdata"
	}
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("w13-ref-%d.json", width)))
			if err != nil {
				t.Fatalf("read reference: %v", err)
			}
			var ref struct {
				PHP     string             `json:"php"`
				IntSize int                `json:"int_size"`
				Cases   map[string]w13Case `json:"cases"`
			}
			if err = json.Unmarshal(data, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.PHP != "8.5.12" || ref.IntSize*8 != width {
				t.Fatalf("unexpected PHP profile: %+v", ref)
			}
			names := append(w13R5ReferenceNames(), w13R5OperationNames()...)
			names = append(names, w13R5LifetimeNames()...)
			for _, name := range names {
				name := name
				t.Run(name, func(t *testing.T) {
					want, ok := ref.Cases[name]
					if !ok {
						t.Fatalf("missing case %q in reference", name)
					}
					h, err := newHeap(width)
					if err != nil {
						t.Fatal(err)
					}
					got := w13RunCase(t, h, nil, name)
					gotJSON, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					wantJSON, err := json.Marshal(want)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(gotJSON, wantJSON) {
						t.Fatalf("Go: %s\nPHP: %s", gotJSON, wantJSON)
					}
				})
			}
		})
	}
}
