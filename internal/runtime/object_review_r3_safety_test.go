package runtime

import (
	"reflect"
	"testing"
)

// REQ-018/020; T-035/036/039/040. This is a host safety assertion, not a
// compatibility pass for the native conditional-body diagnostic.
func TestW13PendingArgumentDestructorSafety(t *testing.T) {
	for _, width := range []int{32, 64} {
		h, err := newHeap(width)
		must(t, err)
		events := []string{}
		var source, receiver value
		mode := "ok"
		c := w13Build(t, classSpec{name: "S", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
			events = append(events, "convert")
			return w13Str("converted"), h.unset(&source)
		}}}, destructor: func(*heap, value) error {
			events = append(events, "destructor")
			return &Throwable{"Exception", "destructor"}
		}})
		sink := w13Build(t, classSpec{name: "R", methods: []methodSpec{{name: "accept", parameters: []methodParameter{{name: "s", typeName: "string"}}, returnType: "string", call: func(_ *heap, _ value, args []value) (value, error) {
			events = append(events, "body")
			if mode == "throw" {
				return value{}, &Throwable{"Exception", "body"}
			}
			if mode == "bad" {
				return value{kind: valueArray, array: newArray()}, nil
			}
			return args[0], nil
		}}}})
		must(t, h.assign(&source, w13Instantiate(t, h, c)))
		must(t, h.assign(&receiver, w13Instantiate(t, h, sink)))
		_, err = h.callMethod(receiver, "accept", []methodArgument{{value: source}}, nil, false)
		if err == nil {
			t.Fatal("cleanup exception was lost")
		}
		if !reflect.DeepEqual(w13Error(err), []any{"Exception", "destructor"}) || !reflect.DeepEqual(events, []string{"convert", "destructor", "body"}) {
			t.Fatal(err, events)
		}
		must(t, h.unset(&receiver))
		globals := value{}
		must(t, h.shutdown(&globals))
		if h.liveObjects != 0 {
			t.Fatal("request retained objects", h.liveObjects)
		}
	}
}
