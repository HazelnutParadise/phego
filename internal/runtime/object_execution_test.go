package runtime

import (
	"reflect"
	"testing"
	"time"
)

// REQ-018/020; T-035/036/039/040. Property hooks are PHP call frames, so
// local destruction must occur before the caller resumes.
func w13RunHookFrame(t *testing.T, width int) []any {
	t.Helper()
	x := newTestExecution(t, readyExecutionRequest(t), width, nil, executionLimits{16, 64, 1024})
	events := []any{}
	c, err := buildClass(classSpec{name: "W13HookFrame", properties: []propertySpec{{name: "value", typeName: "int", virtual: true, get: func(h *heap, _ value, _ value) (value, error) {
		var local value
		if err := x.local(&local); err != nil {
			return value{}, err
		}
		o, err := h.newObject(&class{name: "W13HookDtor", destructor: func(*heap, value) error { events = append(events, "dtor"); return nil }})
		if err != nil {
			return value{}, err
		}
		if err = h.assign(&local, o); err != nil {
			return value{}, err
		}
		return w12Int(1), nil
	}}}})
	must(t, err)
	cleanup, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	var number int64
	result := x.run(cleanup, func(x *execution) completion {
		o, err := x.heap.instantiate(c)
		if err != nil {
			t.Fatal(err)
		}
		v, err := x.heap.readObject(o, "value", nil)
		if err != nil {
			t.Fatal(err)
		}
		number = v.integer
		events = append(events, "after")
		return completion{}
	})
	if result.end.kind != endNormal || !result.cleanupComplete {
		t.Fatal(result)
	}
	return []any{number, events}
}

func TestW13HookFrameLifetime(t *testing.T) {
	got := w13RunHookFrame(t, 64)
	want := []any{int64(1), []any{"dtor", "after"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Go: %#v PHP: %#v", got, want)
	}
}
