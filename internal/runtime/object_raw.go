package runtime

import "errors"

// REQ-018; T-035/036. This private adapter selects a physical declaration
// by name. Public Reflection and explicit ancestor-private selection are separate.
func (h *heap) writeRawObjectProperty(obj value, name string, v value, strict bool) (err error) {
	if err := h.claimValues(obj, v); err != nil {
		return err
	}
	receiver := obj.deref()
	v = v.deref()
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	if o.lazy != nil && o.lazy.target.kind == valueObject {
		o, err = h.initializeLazy(o)
		if err != nil {
			return err
		}
	}
	p, err := h.resolveInstanceProperty(o, name, o.class, true)
	if err != nil {
		return err
	}
	if p == nil || p.static || p.virtual {
		return errors.New("raw access requires a declared instance property")
	}
	receiverCount := methodRefcount(receiver)
	receiverHold, _ := h.holdMethodResult(receiver)
	receiverHold.temporary = receiverCount == 0
	valueCount := methodRefcount(v)
	valueHold, valueProtected := h.holdMethodResult(v)
	valueHold.temporary = valueCount == 0
	defer func() {
		if valueProtected {
			err = h.mergeObjectErrors(err, h.releaseMethodHold(valueHold))
		}
		err = h.mergeObjectErrors(err, h.releaseMethodHold(receiverHold))
	}()
	l := o.lazy
	wasLazy := isLazyPropertyInfo(o, p)
	if wasLazy {
		if l.nonLazy == nil {
			l.nonLazy = make(map[string]struct{})
		}
		l.nonLazy[p.key] = struct{}{}
	}
	err = h.writeObjectSlot(o, p, v, p.owner, strict)
	if err != nil && wasLazy && o.lazy == l && l.initialize != nil {
		if slot, ok := o.props.lookup(p.key); ok && slot.kind == valueUndefined {
			delete(l.nonLazy, p.key)
		}
	}
	if wasLazy && o.lazy == l && !l.running && l.initialize != nil && !o.freed {
		if _, nonLazy := l.nonLazy[p.key]; !nonLazy {
			return err
		}
		l.remaining--
		if l.remaining != 0 {
			return err
		}
		// Publish realization before releasing the callable's last owner:
		// its destructor can observe or reenter the receiver.
		initializer := l.initializer
		l.initializer = value{}
		l.initialize = nil
		l.proxy = false
		l.nonLazy = nil
		err = h.mergeObjectErrors(err, h.release(initializer))
	}
	return err
}
