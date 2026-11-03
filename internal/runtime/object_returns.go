package runtime

import "errors"

// A by-value consumer must consume a temporary reference wrapper without
// destroying its target before the caller can store or discard that value.
// Borrowed wrappers remain owned by their original locations.
func (h *heap) dereferenceMethodReturn(raw value) (value, error) {
	if raw.kind != valueReference {
		return raw, nil
	}
	if err := h.claimValues(raw); err != nil {
		return value{}, err
	}
	out := raw.deref()
	hold, protected := h.holdMethodResult(out)
	err := h.discardMethodReturn(raw)
	if protected {
		if err != nil {
			hold.temporary = false
		}
		err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
	}
	if err != nil {
		return value{}, err
	}
	return out, nil
}

// Bind a callback result directly, without inventing an owned source location.
// A fresh wrapper owns its target; an existing wrapper gains only dst's owner.
func (h *heap) bindMethodReference(dst *value, raw value) error {
	for _, v := range [2]value{*dst, raw} {
		switch v.kind {
		case valueArray, valueObject, valueResource, valueReference:
			if methodRefcount(v) < 0 {
				return errors.New("callback reference has a negative owner count")
			}
		}
	}
	if err := h.claimValues(*dst, raw); err != nil {
		return err
	}
	if methodRefcount(*dst) == 0 {
		retain(*dst)
	}
	next := raw
	retain(raw)
	if raw.kind != valueReference {
		next = value{kind: valueReference, ref: &reference{owner: h, refcount: 1, target: raw}}
	}
	old := *dst
	*dst = next
	return h.release(old)
}
