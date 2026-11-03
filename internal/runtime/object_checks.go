package runtime

func (h *heap) runtimeCheck() error {
	if h.closed {
		return ErrHeapClosed
	}
	if h.execution != nil {
		if c := h.execution.check(); c.kind != endNormal {
			return c
		}
	}
	return nil
}
