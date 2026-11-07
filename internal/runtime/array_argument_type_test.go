package runtime

import (
	"context"
	"errors"
	"testing"
)

// REQ-016/017; T-031-T-034. PHP 8.5.12 (pinned arm64 image) names the
// function and spells the given type with zend_zval_value_name: booleans by
// value and every resource, open or closed, as "resource".
func TestW12ArgumentTypeErrorsNameFunctionAndValue(t *testing.T) {
	h := testHeap(t)
	noop := func(context.Context) error { return nil }
	open, err := h.newResource(context.Background(), "stream", noop)
	must(t, err)
	closed, err := h.newResource(context.Background(), "stream", noop)
	must(t, err)
	must(t, h.closeResource(closed, "stream", context.Background()))
	object, err := h.newObject(&class{name: "stdClass"})
	must(t, err)
	anonymous, err := h.newObject(&class{name: "class@anonymous"})
	must(t, err)
	values := []struct {
		v    value
		name string
	}{
		{h.number(1), "int"}, {bytesValue([]byte("x")), "string"}, {value{kind: valueNull}, "null"},
		{value{kind: valueFloat, floating: 1.5}, "float"}, {boolValue(true), "true"}, {boolValue(false), "false"},
		{object, "stdClass"}, {anonymous, "class@anonymous"}, {open, "resource"}, {closed, "resource"},
	}
	compare := func(x, y value) (int, error) { return h.compare(x, y) }
	call := func(x, y value) (value, error) { return h.number(0), nil }
	for _, fn := range []struct {
		op  string
		run func(slot *value) error
	}{
		{"sort", func(s *value) error { return h.sortArrayRegular(s, "sort", false) }},
		{"asort", func(s *value) error { return h.sortArrayRegular(s, "asort", true) }},
		{"usort", func(s *value) error { return h.sortArray(s, "usort", false, compare) }},
		{"uasort", func(s *value) error { return h.sortArrayUser(s, "uasort", true, call) }},
		{"current", func(s *value) error { _, err := h.arrayPointer(s, "current"); return err }},
		{"reset", func(s *value) error { _, err := h.arrayPointer(s, "reset"); return err }},
	} {
		for _, tc := range values {
			// PHP deprecates current()/reset() on objects and still reads
			// their properties; that path is not modeled here.
			if tc.v.kind == valueObject && (fn.op == "current" || fn.op == "reset") {
				continue
			}
			slot := tc.v
			err := fn.run(&slot)
			want := fn.op + "(): Argument #1 ($array) must be of type array, " + tc.name + " given"
			var th *Throwable
			if !errors.As(err, &th) || th.Class != "TypeError" || th.Message != want {
				t.Errorf("%s(%s): got %v, want TypeError %q", fn.op, tc.name, err, want)
			}
		}
	}
}

// uksort shares the user comparison path; its deprecation names uksort.
func TestW12BoolComparisonDeprecationNamesFunction(t *testing.T) {
	for _, op := range []string{"usort", "uasort", "uksort"} {
		h := testHeap(t)
		var a value
		must(t, h.appendValue(&a, h.number(2)))
		must(t, h.appendValue(&a, h.number(1)))
		var got []string
		must(t, h.sortArrayUserWithDiagnostic(&a, op, op != "usort", func(x, y value) (value, error) {
			return boolValue(x.integer > y.integer), nil
		}, func(_ int, message string) error { got = append(got, message); return nil }))
		want := op + "(): Returning bool from comparison function is deprecated, return an integer less than, equal to, or greater than zero"
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: %q, want %q", op, got, want)
		}
	}
}
