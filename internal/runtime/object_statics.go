package runtime

func (h *heap) referenceStatic(dst *value, c *class, name string, scope *class) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	p, err := h.resolveStatic(c, name, scope)
	if err != nil {
		return err
	}
	if !canAccess(scope, p.owner, p.setVisibility) {
		return &Throwable{"Error", "Cannot indirectly modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + h.calledScopeLabel(scope)}
	}
	table, err := h.staticTable(p.owner)
	if err != nil {
		return err
	}
	return h.bindPropertyReference(dst, table.slot(p.key), p)
}

func (h *heap) unsetStatic(c *class, name string) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	return &Throwable{"Error", "Attempt to unset static property " + c.name + "::$" + name}
}

// An accessible static declaration accessed through an object is a separate
// dynamic instance property. Visibility is checked before raising the notice.
func (h *heap) resolveInstanceProperty(o *object, name string, scope *class, silent bool) (*propertyInfo, error) {
	if len(name) != 0 && name[0] == 0 {
		return nil, &Throwable{"Error", "Cannot access property starting with \"\\0\""}
	}
	p, err := resolvePropertyIn(o.class, name, scope, true)
	if err != nil || p == nil || !p.static {
		return p, err
	}
	if !silent {
		h.warn(levelNotice, "Accessing static property "+p.label()+" as non static")
	}
	return nil, nil
}

// Install the constraint before bind can release the previous alias and invoke
// PHP destructors. Both static and instance cells use this ordering.
func (h *heap) bindPropertyReference(dst, slot *value, p *propertyInfo) error {
	if err := h.claimValues(*dst, *slot); err != nil {
		return err
	}
	if slot.kind == valueUndefined {
		if p != nil && !acceptsObjectType(p.typeName, nullValue()) {
			return &Throwable{"Error", "Cannot access uninitialized non-nullable property " + p.label() + " by reference"}
		}
		*slot = nullValue()
	}
	if p != nil && p.typeName != "" {
		if slot.kind != valueReference {
			*slot = value{kind: valueReference, ref: &reference{owner: h, refcount: 1, target: *slot}}
		}
		for _, source := range h.typedSources[slot.ref] {
			if source.slot == slot {
				return h.bind(dst, slot)
			}
		}
		h.addPropertySource(slot.ref, propertySource{slot, p})
	}
	return h.bind(dst, slot)
}

func (h *heap) dynamicPropertySlot(o *object, name string) (*value, error) {
	if schema := o.class.schema; schema != nil {
		if schema.noDynamic || schema.readonly {
			return nil, &Throwable{"Error", "Cannot create dynamic property " + o.class.name + "::$" + name}
		}
		if _, exists := o.props.lookup(name); !exists && !schema.allowDynamic {
			h.warn(levelDeprecated, "Creation of dynamic property "+o.class.name+"::$"+name+" is deprecated")
		}
	}
	return o.props.slot(name), nil
}
