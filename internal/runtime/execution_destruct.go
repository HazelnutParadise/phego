package runtime

import "errors"

// REQ-020, REQ-022, REQ-040; T-039, T-040, T-043, T-044, T-079, T-080.
// W-06 supplies ownership and PHP 8.5.12 destruction order. Here the PHP
// destructor phase is separate from storage invalidation so output handlers
// can still use values. The existing W-06 prototype shutdown stays unchanged.
func (x *execution) destruct() completion {
	if x.destructorStop {
		return completion{}
	}
	convert := func(err error, since uint64) completion {
		if stop := x.check(); stop.kind != endNormal {
			x.destructorStop = true
			return stop
		}
		var c completion
		if err != nil && !errors.As(err, &c) {
			c = x.fail(endInternal, err)
		}
		c = x.orderedResult(c, since)
		if c.kind == endNormal {
			return c
		}
		x.destructorStop = true
		return c
	}
	if x.globals.kind == valueArray {
		a := x.globals.array
		for {
			before := a.count()
			for i := len(a.buckets) - 1; i >= 0; i-- {
				b := a.buckets[i]
				if b == nil || b.val.kind != valueObject || b.val.object.refcount != 1 {
					continue
				}
				old, _ := a.remove(b.key)
				before := x.serial
				if c := convert(x.heap.release(old), before); c.kind != endNormal {
					return c
				}
			}
			if a.count() == before {
				break
			}
		}
	}
	// The bound is checked again as destructors may create more objects.
	for id := uint64(1); id <= x.heap.nextHandle; id++ {
		o, ok := x.heap.live[id]
		if !ok || o.destructorCalled {
			continue
		}
		o.destructorCalled = true
		if o.class != nil && o.class.destructor != nil {
			o.refcount++
			before := x.serial
			err := o.class.destructor(x.heap, value{kind: valueObject, object: o})
			o.refcount--
			if c := convert(err, before); c.kind != endNormal {
				return c
			}
		}
	}
	return completion{}
}
