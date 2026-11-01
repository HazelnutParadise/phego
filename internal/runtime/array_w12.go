package runtime

import (
	"fmt"
	"slices"
)

// arrayKey applies operation-specific PHP 8.5.12 diagnostics. Snapshot import
// retains its narrower int/string contract. REQ-017; T-033/034.
func (h *heap) arrayKey(k value, unset bool) (arrayKey, error) {
	k = k.deref()
	switch k.kind {
	case valueFloat:
		i := h.floatToIntSafe(k.floating)
		return intKey(i), nil
	case valueNull:
		if !unset {
			h.warn(levelDeprecated, "Using null as an array offset is deprecated, use an empty string instead")
		}
	case valueArray, valueObject:
		op := "access"
		if unset {
			op = "unset"
		}
		return arrayKey{}, &Throwable{"TypeError", "Cannot " + op + " offset of type " + typeName(k) + " on array"}
	}
	return normalizeKey(k, h.width)
}

// REQ-016/017; T-031/032/033/034. These operations are request-local.
func (h *heap) arrayPointer(slot *value, op string) (value, error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	v := slot.deref()
	if v.kind != valueArray {
		return value{}, &Throwable{"TypeError", op + "(): Argument #1 ($array) must be of type array, " + typeName(v) + " given"}
	}
	a := v.array
	if op != "current" && op != "key" {
		switch op {
		case "next", "prev", "reset", "end":
		default:
			return value{}, fmt.Errorf("unknown array pointer operation %q", op)
		}
		var err error
		a, err = h.separate(slot)
		if err != nil {
			return value{}, err
		}
	}
	switch op {
	case "reset":
		a.pointer = 0
	case "end":
		a.pointer = len(a.buckets) - 1
	case "next":
		if a.pointer < len(a.buckets) {
			a.pointer++
		}
	case "prev":
		// PHP cannot retreat from an already invalid internal pointer.
		if a.pointer >= len(a.buckets) || a.pointer < 0 {
			return boolValue(false), nil
		}
		a.pointer--
		for a.pointer >= 0 && a.buckets[a.pointer] == nil {
			a.pointer--
		}
	}
	if a.pointer < 0 {
		a.pointer = len(a.buckets)
	}
	for a.pointer < len(a.buckets) && a.buckets[a.pointer] == nil {
		a.pointer++
	}
	if a.pointer >= len(a.buckets) {
		if op == "key" {
			return nullValue(), nil
		}
		return boolValue(false), nil
	}
	b := a.buckets[a.pointer]
	if op == "key" {
		if b.key.isString {
			return bytesValue([]byte(b.key.text)), nil
		}
		return h.number(b.key.integer), nil
	}
	return b.val.deref(), nil
}

// foreachValue holds a value copy of the input for the duration of the loop.
func (h *heap) foreachValue(arr value, item *value, fn func(arrayKey) error) (err error) {
	if h.closed {
		return ErrHeapClosed
	}
	arr = arr.deref()
	if arr.kind != valueArray {
		h.warnForeach(arr)
		return nil
	}
	retain(arr)
	defer func() {
		if e := h.release(arr); err == nil {
			err = e
		}
	}()
	pos := 0
	for b, ok := arr.array.next(&pos); ok; b, ok = arr.array.next(&pos) {
		if err := h.assign(item, b.val); err != nil {
			return err
		}
		if err := fn(b.key); err != nil {
			return err
		}
	}
	return nil
}

// sortArray sorts a private copy and commits it even if a comparison throws.
// Callback mutations of the input are overwritten, as in PHP's usort/uasort.
// REQ-016/017; T-031/032/033/034. Comparator is trusted runtime code and must
// propagate cancellation/errors; no comparator is called after an error.
func (h *heap) sortArray(slot *value, preserveKeys bool, compare func(value, value) (int, error)) (err error) {
	if h.closed {
		return ErrHeapClosed
	}
	v := slot.deref()
	if v.kind != valueArray {
		return &Throwable{"TypeError", "array sort requires an array"}
	}
	a := h.duplicate(v.array)
	a.refcount = 1
	defer func() {
		if e := h.release(value{kind: valueArray, array: a}); err == nil {
			err = e
		}
	}()
	var failure error
	slices.SortStableFunc(a.buckets, func(x, y *bucket) int {
		if failure != nil {
			return 0
		}
		if h.closed {
			failure = ErrHeapClosed
			return 0
		}
		xv, yv := x.val.deref(), y.val.deref()
		retain(xv)
		retain(yv)
		n, e := compare(xv, yv)
		if rerr := h.release(yv); e == nil {
			e = rerr
		}
		if rerr := h.release(xv); e == nil {
			e = rerr
		}
		failure = e
		return n
	})
	a.pointer = 0
	if preserveKeys {
		a.toIndexed()
	} else {
		a.index = nil
		for i, b := range a.buckets {
			b.key = intKey(int64(i))
		}
		a.nextFree = int64(len(a.buckets))
	}
	if e := h.assign(slot, value{kind: valueArray, array: a}); failure == nil {
		failure = e
	}
	return failure
}
