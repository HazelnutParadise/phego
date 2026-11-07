package runtime

import (
	"strconv"
	"testing"
)

const w13R6GuardLifetimeName = "r6-proxy-guard-reference-lifetime"

// REQ-018/020; T-035/036/039/040. Old-destination cleanup follows slot resolution.
func w13R6GuardLifetimeCase(t *testing.T, h *heap) w13Case {
	t.Helper()
	events := []any{}
	var proxy, target, dst, out value
	answer := w13Int(23)
	borrowReference := func(cell *value) (value, error) {
		var result value
		if err := h.bind(&result, cell); err != nil {
			return value{}, err
		}
		return result, h.release(result)
	}
	c := w13Build(t, classSpec{name: "W13R6GuardLifetime", allowDynamic: true, properties: []propertySpec{{name: "spare", typeName: "int"}}, methods: []methodSpec{{name: "__get", parameters: []methodParameter{{name: "name"}}, returnType: "mixed", returnsReference: true, call: func(h *heap, this value, args []value) (value, error) {
		isProxy := this.object == proxy.object
		if isProxy {
			events = append(events, "get:proxy")
			if err := h.referenceObject(&dst, this, args[0].deref().text, nil); err != nil {
				return value{}, err
			}
			return borrowReference(&dst)
		}
		events = append(events, "get:target")
		return borrowReference(&answer)
	}}}})
	must(t, h.assign(&target, w13Instantiate(t, h, c)))
	p, err := h.newLazyObject(c, true, func(*heap, value) (value, error) { events = append(events, "init"); return target, nil })
	must(t, err)
	must(t, h.assign(&proxy, p))
	old := w13Build(t, classSpec{name: "W13R6GuardLifetimeOld", destructor: func(h *heap, _ value) error {
		events = append(events, "dtor")
		if err := h.unsetObject(target, "x", nil); err != nil {
			return err
		}
		v, err := h.readObject(target, "x", nil)
		if err != nil {
			return err
		}
		text := ""
		if v.deref().kind == valueInt {
			text = strconv.FormatInt(v.deref().integer, 10)
		}
		events = append(events, "value:"+text)
		return nil
	}})
	must(t, h.assign(&dst, w13Instantiate(t, h, old)))
	defer func() {
		for _, cell := range []*value{&out, &dst, &answer, &proxy, &target} {
			must(t, h.unset(cell))
		}
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("guard lifetime fixture retained request values")
		}
	}()
	must(t, h.referenceObject(&out, proxy, "x", nil))
	_, exists := target.object.props.lookup("x")
	return w13Case{Result: []any{append([]any{}, events...), w13Encode(out), w13Encode(dst), w13Encode(answer), exists}, Diagnostics: w13Diagnostics(h)}
}
