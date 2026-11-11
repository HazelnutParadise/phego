package runtime

import (
	"fmt"
	"testing"
	"time"
)

func w13R6ScopeNames() []string {
	return []string{"r6-scope-write", "r6-scope-reference", "r6-scope-static", "r6-scope-target", "r6-scope-nested", "r6-scope-global", "r6-scope-conversion", "r6-scope-hook", "r6-scope-readonly-private", "r6-scope-readonly-private-unset"}
}

func w13R6ScopeCase(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	owners := []*value{}
	own := func(v value) value {
		cell := new(value)
		must(t, h.assign(cell, v))
		owners = append(owners, cell)
		return *cell
	}
	defer func() {
		for i := len(owners) - 1; i >= 0; i-- {
			must(t, h.unset(owners[i]))
		}
		if h.liveObjects != 0 || len(h.typedSources) != 0 {
			t.Fatal("scope fixture retained request values")
		}
	}()
	targetSpec := classSpec{name: "W13R6ScopeTarget", properties: []propertySpec{{name: "p", typeName: "int", setVisibility: privateAccess, hasDefault: true, initial: int64(1)}}}
	if name == "r6-scope-readonly-private" || name == "r6-scope-readonly-private-unset" {
		targetSpec.name = "W13R6ScopeTargetReadonly"
		targetSpec.properties[0].readonly = true
		targetSpec.properties[0].hasDefault = false
	}
	targetClass := w13Build(t, targetSpec)
	target := own(w13Instantiate(t, h, targetClass))
	var parent, other *class
	other = w13Build(t, classSpec{name: "W13R6ScopeOther", methods: []methodSpec{{name: "fail", call: func(h *heap, _ value, _ []value) (value, error) {
		return nullValue(), h.writeObject(target, "p", w13Int(2), other, false)
	}}}})
	otherObject := own(w13Instantiate(t, h, other))
	stringClass := w13Build(t, classSpec{name: "W13R6ScopeString", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(*heap, value, []value) (value, error) { return value{}, &Throwable{"Exception", "convert"} }}}})
	stringObject := own(w13Instantiate(t, h, stringClass))
	call := func(o value, method string) any {
		return w13R6Attempt(func() (value, error) { return h.callMethod(o, method, nil, nil, false) })
	}
	var captured any
	parent = w13Build(t, classSpec{name: "W13R6ScopeParent", properties: []propertySpec{{name: "v", typeName: "int", virtual: true, set: func(h *heap, this, v value) (value, error) {
		return nullValue(), h.writeObject(this, "p", v, parent, false)
	}}}, methods: []methodSpec{
		{name: "fail", call: func(h *heap, this value, _ []value) (value, error) {
			return nullValue(), h.writeObject(this, "p", w13Int(2), parent, false)
		}},
		{name: "reference", call: func(h *heap, this value, _ []value) (value, error) {
			var alias value
			err := h.referenceObject(&alias, this, "p", parent)
			return nullValue(), h.mergeObjectErrors(err, h.unset(&alias))
		}},
		{name: "staticFail", static: true, call: func(h *heap, _ value, _ []value) (value, error) {
			return nullValue(), h.writeObject(target, "p", w13Int(2), parent, false)
		}},
		{name: "other", call: func(h *heap, _ value, _ []value) (value, error) {
			return nullValue(), h.writeObject(target, "p", w13Int(2), parent, false)
		}},
		{name: "otherUnset", call: func(h *heap, _ value, _ []value) (value, error) {
			return nullValue(), h.unsetObject(target, "p", parent)
		}},
		{name: "nested", call: func(h *heap, this value, _ []value) (value, error) {
			captured = []any{call(otherObject, "fail"), call(this, "fail")}
			return nullValue(), nil
		}},
		{name: "globalCall", call: func(h *heap, this value, _ []value) (value, error) {
			first := w13R6Attempt(func() (value, error) {
				return h.invokeObject(func() (value, error) { return nullValue(), h.writeObject(target, "p", w13Int(2), nil, false) }, false)
			})
			captured = []any{first, call(this, "fail")}
			return nullValue(), nil
		}},
		{name: "conversion", call: func(h *heap, this value, _ []value) (value, error) {
			first := w13R6Attempt(func() (value, error) { s, err := h.toString(stringObject); return str(s), err })
			captured = []any{first, call(this, "fail")}
			return nullValue(), nil
		}},
	}})
	child := w13Build(t, classSpec{name: "W13R6ScopeChild", parent: parent, properties: []propertySpec{{name: "p", typeName: "int", setVisibility: privateAccess, hasDefault: true, initial: int64(1)}}})
	o := own(w13Instantiate(t, h, child))
	var result any
	switch name {
	case "r6-scope-write":
		result = call(o, "fail")
	case "r6-scope-reference":
		result = call(o, "reference")
	case "r6-scope-static":
		result = call(o, "staticFail")
	case "r6-scope-target":
		result = call(o, "other")
	case "r6-scope-readonly-private":
		result = call(o, "other")
	case "r6-scope-readonly-private-unset":
		result = call(o, "otherUnset")
	case "r6-scope-nested":
		mustCall := call(o, "nested")
		if mustCall != nil {
			t.Fatal(mustCall)
		}
		result = captured
	case "r6-scope-global":
		mustCall := call(o, "globalCall")
		if mustCall != nil {
			t.Fatal(mustCall)
		}
		result = captured
	case "r6-scope-conversion":
		mustCall := call(o, "conversion")
		if mustCall != nil {
			t.Fatal(mustCall)
		}
		result = captured
	case "r6-scope-hook":
		result = w13R6Attempt(func() (value, error) { return nullValue(), h.writeObject(o, "v", w13Int(2), nil, false) })
	default:
		t.Fatal(name)
	}
	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

// Both callback implementations must preserve and restore the same called scope.
func TestW13CalledScopeExecutionFrames(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, name := range w13R6ScopeNames() {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				x := newTestExecution(t, readyExecutionRequest(t), width, nil, executionLimits{32, 64, 1 << 20})
				var got w13Case
				cleanup, cancel := cleanupDeadline(t, time.Second)
				defer cancel()
				r := x.run(cleanup, func(x *execution) completion {
					got = w13R6ScopeCase(t, x.heap, name)
					return completion{kind: endNormal}
				})
				if !r.cleanupComplete {
					t.Fatal("execution cleanup incomplete", r)
				}
				w13AssertOracleCase(t, width, name, got)
			})
		}
	}
}
