package runtime

import (
	"fmt"
	"testing"
)

func w13R9Names() []string {
	return []string{"r9-dynamic-churn-detached", "r9-dynamic-churn-shared"}
}

func w13R9Case(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	detached := name == "r9-dynamic-churn-detached"
	if !detached && name != "r9-dynamic-churn-shared" {
		t.Fatalf("unknown R9 case %q", name)
	}
	c := w13Build(t, classSpec{name: "W13R9Dynamic", allowDynamic: true})
	o, err := h.newObject(c)
	w13Must(t, "newObject", err)
	var owner, alias value
	w13Must(t, "own object", h.assign(&owner, o))
	defer func() {
		w13Must(t, "unset alias", h.unset(&alias))
		w13Must(t, "unset owner", h.unset(&owner))
		if len(h.live) != 0 || len(h.typedSources) != 0 {
			t.Fatalf("R9 fixture retained values: live=%d typed=%d", len(h.live), len(h.typedSources))
		}
	}()
	w13Must(t, "set anchor", h.setProperty(o, "anchor", w13Int(1)))
	w13Must(t, "reference anchor", h.referenceObject(&alias, o, "anchor", nil))
	for i := range 128 {
		w13Must(t, "set x", h.setProperty(o, "x", w13Int(int64(i))))
		w13Must(t, "unset x", h.unsetObject(o, "x", nil))
		tmp := fmt.Sprintf("tmp%d", i)
		w13Must(t, "set tmp", h.setProperty(o, tmp, w13Int(int64(i))))
		w13Must(t, "unset tmp", h.unsetObject(o, tmp, nil))
	}
	if detached {
		w13Must(t, "unset anchor", h.unsetObject(o, "anchor", nil))
		w13Must(t, "reinsert anchor", h.setProperty(o, "anchor", w13Int(3)))
	}
	w13Must(t, "assign alias", h.assign(&alias, w13Int(7)))
	w13Must(t, "set tail", h.setProperty(o, "tail", w13Int(2)))
	clone, err := h.cloneObject(o, nil)
	w13Must(t, "clone object", err)
	var cloneOwner value
	w13Must(t, "own clone", h.assign(&cloneOwner, clone))
	defer func() { w13Must(t, "unset clone", h.unset(&cloneOwner)) }()
	w13Must(t, "set clone tail", h.setProperty(clone, "tail", w13Int(9)))
	return w13Case{Result: []any{
		w13ObjectKeys(o), w13Encode(w13Read(t, h, o, "anchor", nil)), w13Encode(w13Read(t, h, o, "tail", nil)),
		w13ObjectKeys(clone), w13Encode(w13Read(t, h, clone, "anchor", nil)), w13Encode(w13Read(t, h, clone, "tail", nil)), w13Encode(alias),
	}, Diagnostics: w13Diagnostics(h)}
}

func TestW13R9PHP8512Regression(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R9Names() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := newHeap(width)
				w13Must(t, "newHeap", err)
				w13AssertOracleCase(t, width, name, w13R9Case(t, h, name))
			})
		}
	}
}
