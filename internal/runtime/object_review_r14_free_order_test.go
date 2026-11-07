package runtime

import "testing"

// REQ-018/019; T-035-038. tests/semantics/w13-objects/review_r14_free_order.php.
func w13R14FreeNames() []string {
	return []string{"r14-free-declared", "r14-free-dynamic", "r14-free-subclass", "r14-free-materialized", "r14-free-unset-redeclare"}
}

func w13R14FreeCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	var order []any
	labels := map[*object]string{}
	probe := w13Build(t, classSpec{name: "W13R14Probe", destructor: func(_ *heap, this value) error {
		order = append(order, labels[this.deref().object])
		return nil
	}})
	holder := w13Build(t, classSpec{name: "W13R14Holder", allowDynamic: true, properties: []propertySpec{{name: "declared1", hasDefault: true}, {name: "declared2", hasDefault: true}}})
	child := w13Build(t, classSpec{name: "W13R14Child", parent: holder, properties: []propertySpec{{name: "declared3", hasDefault: true}}})
	newProbe := func(label string) value {
		o := w13Instantiate(t, h, probe)
		labels[o.object] = label
		return o
	}
	set := func(o value, scope *class, names ...string) {
		for _, n := range names {
			// writeObject takes ownership of the new probe.
			w13Write(t, h, o, n, newProbe(n), scope)
		}
	}
	// A new object is a temporary; a variable owns it until unset.
	var owner value
	own := func(c *class) value {
		o := w13Instantiate(t, h, c)
		w13Must(t, "own object", h.assign(&owner, o))
		return o
	}
	release := func(value) {
		w13Must(t, "unset object", h.unset(&owner))
	}
	var result any
	switch name {
	case "r14-free-declared":
		o := own(holder)
		set(o, holder, "declared1", "declared2")
		release(o)
		result = order
	case "r14-free-dynamic":
		o := own(holder)
		set(o, holder, "declared2", "declared1", "dynamic1", "dynamic2")
		release(o)
		result = order
	case "r14-free-subclass":
		o := own(child)
		set(o, child, "dynamic1", "declared3", "declared1", "dynamic2", "declared2")
		release(o)
		result = order
	case "r14-free-materialized":
		o := own(holder)
		set(o, holder, "dynamic1", "declared1", "dynamic2", "declared2")
		keys := w13ObjectKeys(o)
		release(o)
		result = []any{keys, order}
	case "r14-free-unset-redeclare":
		o := own(holder)
		set(o, holder, "declared1", "dynamic1")
		w13Must(t, "unsetObject(declared1)", h.unsetObject(o, "declared1", holder))
		w13Write(t, h, o, "declared1", newProbe("declared1b"), holder)
		set(o, holder, "dynamic2")
		release(o)
		result = order
	default:
		t.Fatalf("unknown case %s", name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}
