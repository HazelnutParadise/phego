package runtime

// User comparison adapter derived from PHP 8.5.12 ext/standard/array.c.
// PHP License 3.01; see third_party/php-sort/PHP-LICENSE and UPSTREAM.json.
// REQ-017; T-033/T-034. W-17's PHP call dispatcher supplies call's values.
func (h *heap) sortArrayUser(slot *value, preserveKeys bool, call func(value, value) (value, error)) error {
	return h.sortArrayUserWithDiagnostic(slot, preserveKeys, call, func(level int, message string) error { h.warn(level, message); return nil })
}

// W-17 supplies PHP error-handler dispatch through diagnose. A thrown handler
// may preserve a valid true comparison but suppress a false reverse retry.
func (h *heap) sortArrayUserWithDiagnostic(slot *value, preserveKeys bool, call func(value, value) (value, error), diagnose func(int, string) error) error {
	h.sortCompareDeprecated = false
	return h.sortArrayWithPolicy(slot, preserveKeys, func(a, b *value) (n int, err error) {
		result, err := h.sortUserCall(a, b, call)
		defer func(held value) {
			if e := h.release(held); err == nil {
				err = e
			}
		}(result)
		if err != nil {
			return h.sortComparisonResult(result, err, diagnose)
		}
		if h.closed {
			return 0, ErrHeapClosed
		}
		// PHP inspects the return slot before dereferencing it. A by-reference
		// bool return is coerced directly, without bool deprecation or retry.
		v := result
		if v.kind == valueBool {
			if !h.sortCompareDeprecated {
				name := "usort"
				if preserveKeys {
					name = "uasort"
				}
				diagnosticErr := diagnose(levelDeprecated, name+"(): Returning bool from comparison function is deprecated, return an integer less than, equal to, or greater than zero")
				h.sortCompareDeprecated = true
				if diagnosticErr == nil && h.closed {
					diagnosticErr = ErrHeapClosed
				}
				if diagnosticErr != nil {
					if v.boolean {
						return 1, diagnosticErr
					}
					return 0, diagnosticErr
				}
			}
			if !v.boolean {
				// The retry result is converted once, never recursively retried.
				retry, callErr := h.sortUserCall(b, a, call)
				defer func(held value) {
					if e := h.release(held); err == nil {
						err = e
					}
				}(retry)
				n, err = h.sortComparisonResult(retry, callErr, diagnose)
				return -n, err
			}
		}
		return h.sortComparisonResult(result, nil, diagnose)
	}, true)
}

// REQ-017; T-033/T-034. Return an owned raw result, preserving by-reference
// return kind. Protect it before releasing this invocation's argument holds,
// which can run destructors. The caller must release it even after shutdown.
func (h *heap) sortUserCall(a, b *value, call func(value, value) (value, error)) (result value, err error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	x, y := a.deref(), b.deref()
	retain(x)
	retain(y)
	result, err = call(x, y)
	retain(result)
	if e := h.release(y); err == nil {
		err = e
	}
	if e := h.release(x); err == nil {
		err = e
	}
	if err == nil && h.closed {
		err = ErrHeapClosed
	}
	return result, err
}

// Results follow the constructor/borrowed-value ownership convention. Hold a
// temporary result while coercing it and release it even when the call threw.
func (h *heap) sortComparisonResult(v value, callErr error, diagnose func(int, string) error) (n int, err error) {
	v = v.deref()
	retain(v)
	defer func() {
		if e := h.release(v); err == nil {
			err = e
		}
	}()
	if callErr != nil {
		return 0, callErr
	}
	if h.closed {
		return 0, ErrHeapClosed
	}
	if v.kind == valueObject {
		err = diagnose(levelWarning, "Object of class "+typeName(v)+" could not be converted to int")
		if err == nil && h.closed {
			err = ErrHeapClosed
		}
		return 1, err
	}
	// Existing scalar conversion records diagnostics. Route only the newly
	// emitted suffix through this call's PHP handler, once, before releasing
	// the result; handlers may reenter and append their own diagnostics.
	start := len(h.diagnostics)
	converted := h.toInt(v)
	emitted := append([]diagnostic(nil), h.diagnostics[start:]...)
	h.diagnostics = h.diagnostics[:start]
	for _, d := range emitted {
		err = diagnose(d.level, d.message)
		if err == nil && h.closed {
			err = ErrHeapClosed
		}
		if err != nil {
			break
		}
	}
	return compareInts(converted, 0), err
}
