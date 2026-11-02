package runtime

import "errors"

type activePropertyHook struct {
	receiver       *object
	prototypeOwner *class
	name           string
}

// A later cleanup exception replaces the active one while retaining its chain.
// Reuse W-14 completion identities and sticky termination precedence.
func (h *heap) mergeObjectErrors(previous, next error) error {
	if next == nil {
		return previous
	}
	if previous == nil {
		return next
	}
	convert := func(err error) completion {
		var c completion
		if errors.As(err, &c) {
			return c
		}
		var php *Throwable
		if errors.As(err, &php) {
			return completion{kind: endThrow, err: err}
		}
		return completion{kind: endInternal, err: err}
	}
	c := replaceCompletion(convert(previous), convert(next))
	if h.execution != nil {
		if stop := h.execution.check(); stop.kind != endNormal {
			return replaceCompletion(c, stop)
		}
	}
	return c
}

// REQ-018/020; T-035/036/039/040. Use the existing frame/termination
// boundary rather than creating a second callback and shutdown system.
func (h *heap) invokeObject(fn func() (value, error), byReference bool) (value, error) {
	if h.execution == nil {
		previous := h.currentHook
		h.currentHook = activePropertyHook{}
		defer func() { h.currentHook = previous }()
		v, err := fn()
		if err != nil {
			return value{}, err
		}
		if err := h.claimValues(v); err != nil {
			return value{}, err
		}
		if !byReference && v.kind == valueReference {
			return h.dereferenceMethodReturn(v)
		}
		return v, nil
	}
	x := h.execution
	c := x.call(func(*execution) completion {
		v, err := fn()
		if err == nil {
			err = h.claimValues(v)
			if err == nil && !byReference && v.kind == valueReference {
				v, err = h.dereferenceMethodReturn(v)
			}
		}
		if err != nil {
			var stop completion
			if errors.As(err, &stop) {
				return stop
			}
			var php *Throwable
			if errors.As(err, &php) {
				return completion{kind: endThrow, err: err}
			}
			return x.fail(endInternal, err)
		}
		return completion{kind: endReturn, value: v, returnByReference: byReference}
	}, nil)
	if c.kind != endReturn {
		return value{}, c
	}
	v := c.value
	// Transfer a VM result to the constructor-style temporary contract without
	// destroying it between return and the caller's subsequent store/discard.
	if c.returnOwned {
		switch v.kind {
		case valueArray:
			v.array.refcount--
		case valueObject, valueResource:
			v.object.refcount--
		case valueReference:
			v.ref.refcount--
		}
	}
	return v, nil
}
