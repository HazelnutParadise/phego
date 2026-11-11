package runtime

// REQ-020/021; T-039-042. A VM temporary already has a logical owner;
// this ledger records its live range, without adding another owner.
type gcTemporary struct {
	node  gcNode
	depth int
}

func (h *heap) pushGCTemporary(v value) int {
	index := len(h.gcTemporaries)
	var node gcNode
	switch v = v.deref(); v.kind {
	case valueArray:
		node = v.array
	case valueObject:
		node = v.object
	default:
		return index
	}
	depth := 0
	if h.execution != nil {
		depth = len(h.execution.frames)
	}
	h.gcTemporaries = append(h.gcTemporaries, gcTemporary{node, depth})
	return index
}
func (h *heap) popGCTemporaries(index int) {
	if index <= len(h.gcTemporaries) {
		clear(h.gcTemporaries[index:])
		h.gcTemporaries = h.gcTemporaries[:index]
	}
}
