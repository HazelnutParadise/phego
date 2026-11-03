package runtime

// materializeObjectProperties is the property-view boundary used by PHP casts
// and reflection adapters. Tests establish its observable comparison state.
func (h *heap) materializeObjectProperties(obj value) (*propertyTable, error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return nil, err
	}
	o, err = h.initializeLazy(o)
	if err != nil {
		return nil, err
	}
	o.props.materialize()
	return o.props, nil
}

func (p *propertyTable) materialize() {
	p.materialized = true
}
