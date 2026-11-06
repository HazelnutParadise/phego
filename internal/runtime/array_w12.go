package runtime

import "fmt"

// arrayKey applies operation-specific PHP 8.5.12 diagnostics. Snapshot import
// retains its narrower int/string contract. REQ-017; T-033/034.
func (h *heap) arrayKey(k value, unset bool) (arrayKey, error) {
	if err := h.claimValues(k); err != nil {
		return arrayKey{}, err
	}
	k = k.deref()
	switch k.kind {
	case valueResource:
		h.warn(levelWarning, fmt.Sprintf("Resource ID#%d used as offset, casting to integer (%d)", k.object.handle, k.object.handle))
		return intKey(int64(k.object.handle)), nil
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
	if err := h.claimValues(*slot); err != nil {
		return value{}, err
	}
	v := slot.deref()
	if v.kind != valueArray {
		return value{}, &Throwable{"TypeError", op + "(): Argument #1 ($array) must be of type array, " + argumentTypeName(v) + " given"}
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
	pos := a.pointer
	for pos < len(a.buckets) && a.buckets[pos] == nil {
		pos++
	}
	switch op {
	case "reset":
		pos = 0
	case "end":
		pos = len(a.buckets) - 1
	case "next":
		if pos >= len(a.buckets) {
			return boolValue(false), nil
		}
		pos++
	case "prev":
		// PHP cannot retreat from an already invalid internal pointer.
		if pos >= len(a.buckets) || pos < 0 {
			return boolValue(false), nil
		}
		pos--
		for pos >= 0 && a.buckets[pos] == nil {
			pos--
		}
	}
	if pos < 0 {
		pos = len(a.buckets)
	}
	for pos < len(a.buckets) && a.buckets[pos] == nil {
		pos++
	}
	if op != "current" && op != "key" {
		a.pointer = pos
	}
	if pos >= len(a.buckets) {
		if op == "key" {
			return nullValue(), nil
		}
		return boolValue(false), nil
	}
	b := a.buckets[pos]
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
	if err := h.claimValues(arr, *item); err != nil {
		return err
	}
	arr = arr.deref()
	if arr.kind != valueArray {
		h.warnForeach(arr)
		return nil
	}
	retain(arr)
	temporaries := h.pushGCTemporary(arr)
	defer func() {
		h.popGCTemporaries(temporaries)
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
func (h *heap) sortArray(slot *value, op string, preserveKeys bool, compare func(value, value) (int, error)) (err error) {
	return h.sortArrayWithPolicy(slot, op, preserveKeys, func(a, b *value) (int, error) {
		return h.sortCompareValues(a, b, compare)
	}, true)
}

// SORT_REGULAR shares the PHP schedule, but native comparisons are not user
// calls suppressed by a pending callback exception. W-13 owns object handlers.
func (h *heap) sortArrayRegular(slot *value, op string, preserveKeys bool) error {
	return h.sortArrayWithPolicy(slot, op, preserveKeys, func(a, b *value) (int, error) {
		return h.sortCompareValues(a, b, h.compare)
	}, false)
}

// REQ-017; T-033/T-034. Retain snapshots only for this invocation. The user
// adapter receives live locations separately so a bool retry can reload them.
func (h *heap) sortCompareValues(a, b *value, compare func(value, value) (int, error)) (n int, err error) {
	if err := h.claimValues(*a, *b); err != nil {
		return 0, err
	}
	x, y := a.deref(), b.deref()
	retain(x)
	retain(y)
	n, err = compare(x, y)
	if e := h.release(y); err == nil {
		err = e
	}
	if e := h.release(x); err == nil {
		err = e
	}
	return n, err
}

// op is the PHP function name used in its diagnostics.
func (h *heap) sortArrayWithPolicy(slot *value, op string, preserveKeys bool, compare func(*value, *value) (int, error), stopUserCalls bool) (err error) {
	if h.closed {
		return ErrHeapClosed
	}
	if err := h.claimValues(*slot); err != nil {
		return err
	}
	v := slot.deref()
	if v.kind != valueArray {
		return &Throwable{"TypeError", op + "(): Argument #1 ($array) must be of type array, " + argumentTypeName(v) + " given"}
	}
	// Sorting an empty array preserves its next append index. Retain the
	// argument location for nonempty input, as a by-reference call does: the
	// callback can unset or rebind the caller's variable without changing it.
	if stopUserCalls && v.array.count() == 0 {
		return nil
	}
	var argument value
	if err := h.bind(&argument, slot); err != nil {
		return err
	}
	defer func() {
		if e := h.unset(&argument); err == nil {
			err = e
		}
	}()
	var a *phpArray
	if stopUserCalls {
		a = h.duplicate(v.array)
		a.toIndexed()
		a.refcount = 1
	} else {
		// Native SORT_REGULAR separates the argument, unpacks before its
		// temporary hold, then mutates that array without a final reassignment.
		a, err = h.separate(&argument)
		if err != nil {
			return err
		}
		if a.packed() && a.limit != 0 {
			a.toIndexed()
		}
		retain(value{kind: valueArray, array: a})
	}
	defer func() {
		if e := h.release(value{kind: valueArray, array: a}); err == nil {
			err = e
		}
	}()
	if a.count() == 0 || (preserveKeys && a.count() == 1) {
		if stopUserCalls {
			return h.assign(&argument, value{kind: valueArray, array: a})
		}
		return nil
	}
	// Already ordered native arrays remove holes without remapping foreach
	// positions. Clear lookup chains during comparisons, as PHP does for
	// recursive-array comparisons; rebuild them only after sorting.
	write := 0
	for _, b := range a.buckets {
		if b != nil {
			a.buckets[write] = b
			write++
		}
	}
	clear(a.buckets[write:])
	a.buckets = a.buckets[:write]
	clear(a.index)
	var failure error
	// REQ-017; T-033/T-034. PHP's stable fallback follows original live
	// positions, even while an exception suppresses further user calls.
	original := make(map[*bucket]int, len(a.buckets))
	for i, b := range a.buckets {
		original[b] = i
	}
	php8512Sort(a.buckets, func(x, y *bucket) int {
		n := 0
		if failure == nil && h.closed {
			failure = ErrHeapClosed
		}
		if !h.closed && (failure == nil || !stopUserCalls) {
			var e error
			n, e = compare(&x.val, &y.val)
			if failure == nil {
				failure = e
			}
		}
		if n != 0 {
			return n
		}
		return compareInts(int64(original[x]), int64(original[y]))
	})
	a.pointer = 0
	if preserveKeys {
		for i, b := range a.buckets {
			a.index[b.key] = i
		}
	} else {
		a.index = nil
		for i, b := range a.buckets {
			b.key = intKey(int64(i))
		}
		a.nextFree = int64(len(a.buckets))
	}
	if stopUserCalls {
		if e := h.assign(&argument, value{kind: valueArray, array: a}); failure == nil {
			failure = e
		}
	}
	return failure
}
