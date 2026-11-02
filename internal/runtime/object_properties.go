package runtime

import (
	"slices"
)

// REQ-018; T-035/036. Every PHP write, including writes through aliases,
// checks the sources attached to the shared location before changing it.
func acceptsObjectType(typ string, v value) bool {
	return methodAcceptsType(&methodSpec{}, nil, typ, v)
}

func (h *heap) coerceProperty(p *propertyInfo, v value, strict, reference bool) (value, error) {
	if err := h.claimValues(v); err != nil {
		return value{}, err
	}
	v = v.deref()
	if coerced, ok, err := h.coerceTypedValue(&methodSpec{owner: p.owner}, p.owner, p.typeName, v, strict); err != nil {
		return value{}, err
	} else if ok {
		return coerced, nil
	}
	target := "property "
	if reference {
		target = "reference held by property "
	}
	return value{}, &Throwable{"TypeError", "Cannot assign " + typeName(v) + " to " + target + p.label() + " of type " + p.typeName}
}

func (h *heap) checkReferenceWrite(r *reference, v value, strict bool) (value, error) {
	if err := h.claimValues(value{kind: valueReference, ref: r}, v); err != nil {
		return value{}, err
	}
	result := v
	for i, source := range h.typedSources[r] {
		candidate, err := h.coerceProperty(source.info, v, strict, true)
		if err != nil {
			return value{}, err
		}
		if i > 0 {
			same, err := identical(candidate, result)
			if err != nil || !same {
				return value{}, &Throwable{"TypeError", "Cannot assign " + typeName(v) + " to reference held by property " + source.info.label() + " of type " + source.info.typeName}
			}
		}
		result = candidate
	}
	return result, nil
}

func (h *heap) detachPropertySource(slot *value) {
	if slot.kind == valueReference {
		sources := slices.DeleteFunc(h.typedSources[slot.ref], func(s propertySource) bool { return s.slot == slot })
		if len(sources) == 0 {
			delete(h.typedSources, slot.ref)
		} else {
			h.typedSources[slot.ref] = sources
		}
	}
}

func (h *heap) readObject(obj value, name string, scope *class) (value, error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return value{}, err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false)
	if err != nil {
		if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
			return out.deref(), callErr
		}
		return value{}, err
	}
	bypassHook := h.propertyHookBypasses(o, p)
	if p != nil && p.virtual {
		if bypassHook {
			return value{}, &Throwable{"Error", "Must not read from virtual property " + p.label()}
		}
		if p.get != nil {
			return h.readPropertyHook(o, p)
		}
		return value{}, &Throwable{"Error", "Property " + p.label() + " is write-only"}
	} else if p != nil && !p.static && p.get != nil && !bypassHook {
		return h.readPropertyHook(o, p)
	}
	o, err = h.objectForProperty(o, p, name)
	if err != nil {
		return value{}, err
	}
	if p == nil {
		if slot, ok := o.props.lookup(name); ok && slot.kind != valueUndefined {
			return slot.deref(), nil
		}
		if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
			return out.deref(), callErr
		}
		h.warn(levelWarning, "Undefined property: "+o.class.name+"::$"+name)
		return nullValue(), nil
	}
	if p.virtual {
		return value{}, &Throwable{"Error", "Property " + p.label() + " is write-only"}
	}
	slot, _ := o.props.lookup(p.key)
	if slot.kind == valueUndefined {
		if p.typeName != "" {
			return value{}, &Throwable{"Error", "Typed property " + p.label() + " must not be accessed before initialization"}
		}
		h.warn(levelWarning, "Undefined property: "+o.class.name+"::$"+name)
		return nullValue(), nil
	}
	return slot.deref(), nil
}

func (h *heap) propertyHook(o *object, p *propertyInfo, hook objectHook, v value) (out value, err error) {
	out, err = h.runPropertyHook(o, p, hook, v, false)
	return out, h.mergeObjectErrors(err, h.discard(out))
}

func (h *heap) runPropertyHook(o *object, p *propertyInfo, hook objectHook, v value, byReference bool) (out value, err error) {
	if err := h.claimValues(value{kind: valueObject, object: o}, v); err != nil {
		return value{}, err
	}
	var owners []methodHold
	h.holdMethodValue(value{kind: valueObject, object: o}, &owners)
	h.holdMethodValue(v, &owners)
	out, err = h.invokeObject(func() (value, error) {
		h.currentHook = activePropertyHook{receiver: o, prototypeOwner: p.prototypeOwner, name: p.name}
		return hook(h, value{kind: valueObject, object: o}, v)
	}, byReference)
	result, protected := h.holdMethodResult(out)
	for i := len(owners) - 1; i >= 0; i-- {
		if cleanup := h.releaseMethodHold(owners[i]); cleanup != nil {
			err = h.mergeObjectErrors(err, cleanup)
		}
	}
	if protected {
		if err != nil {
			result.temporary = false
		}
		if cleanup := h.releaseMethodHold(result); cleanup != nil {
			err = h.mergeObjectErrors(err, cleanup)
		}
	}
	if err != nil {
		return value{}, err
	}
	return out, nil
}

func (h *heap) readPropertyHook(o *object, p *propertyInfo) (value, error) {
	v, err := h.getPropertyHook(o, p, false)
	return v.deref(), err
}

func (h *heap) getPropertyHook(o *object, p *propertyInfo, byReference bool) (value, error) {
	return h.runPropertyHook(o, p, func(h *heap, this, valueArg value) (value, error) {
		raw, err := p.get(h, this, valueArg)
		if err != nil {
			return value{}, err
		}
		out, err := h.checkMethodReturn(raw, &methodSpec{owner: p.owner, name: "$" + p.name + "::get", returnType: p.typeName, returnsReference: p.getReference}, this, p.hookStrict)
		if err != nil {
			if cleanup := h.discard(raw); cleanup != nil {
				err = h.mergeObjectErrors(err, cleanup)
			}
		}
		return out, err
	}, nullValue(), byReference)
}

func (h *heap) propertyHookBypasses(o *object, p *propertyInfo) bool {
	active := h.currentHook
	if p == nil || active.receiver == nil || active.prototypeOwner != p.prototypeOwner || active.name != p.name {
		return false
	}
	if active.receiver == o {
		return true
	}
	lazy := active.receiver.lazy
	return lazy != nil && lazy.proxy && lazy.initialize == nil &&
		lazy.target.kind == valueObject && lazy.target.object == o
}

func (h *heap) writeObject(obj value, name string, v value, scope *class, strict bool) error {
	if err := h.claimValues(obj, v); err != nil {
		return err
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false)
	if err != nil {
		if _, called, callErr := h.magicProperty(o, "set", name, []value{v}); called {
			return callErr
		}
		return err
	}
	bypassHook := h.propertyHookBypasses(o, p)
	if p != nil && p.virtual {
		if bypassHook {
			return &Throwable{"Error", "Must not write to virtual property " + p.label()}
		}
		if p.set != nil {
			if !canAccess(scope, p.owner, p.setVisibility) {
				return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + methodScopeLabel(scope)}
			}
			v, err = h.coerceProperty(p, v, strict, false)
			if err != nil {
				return err
			}
			_, err = h.propertyHook(o, p, p.set, v)
			return err
		}
		return &Throwable{"Error", "Property " + p.label() + " is read-only"}
	}
	if p != nil && !p.static && p.set != nil && !bypassHook {
		slot, _ := o.props.lookup(p.key)
		if p.readonly && slot != nil && slot.kind != valueUndefined && (!o.hooks["\x00clone"] || o.hooks["\x00clone:"+p.key]) {
			return &Throwable{"Error", "Cannot modify readonly property " + p.label()}
		}
		if !canAccess(scope, p.owner, p.setVisibility) {
			from := "global scope"
			if scope != nil {
				from = "scope " + scope.name
			}
			ro := ""
			if p.readonly {
				ro = "readonly "
			}
			return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) " + ro + "property " + p.label() + " from " + from}
		}
		v, err = h.coerceProperty(p, v, strict, false)
		if err != nil {
			return err
		}
		_, err = h.propertyHook(o, p, p.set, v)
		return err
	}
	o, err = h.objectForProperty(o, p, name)
	if err != nil {
		return err
	}
	if p == nil {
		if slot, ok := o.props.lookup(name); !ok || slot.kind == valueUndefined {
			if _, called, callErr := h.magicProperty(o, "set", name, []value{v}); called {
				return callErr
			}
		}
		slot, err := h.dynamicPropertySlot(o, name)
		if err != nil {
			return err
		}
		return h.assign(slot, v)
	}
	if p.virtual && p.set == nil {
		return &Throwable{"Error", "Property " + p.label() + " is read-only"}
	}
	slot, _ := o.props.lookup(p.key)
	if p.readonly && slot != nil && slot.kind != valueUndefined && (!o.hooks["\x00clone"] || o.hooks["\x00clone:"+p.key]) {
		return &Throwable{"Error", "Cannot modify readonly property " + p.label()}
	}
	if !canAccess(scope, p.owner, p.setVisibility) {
		from := "global scope"
		if scope != nil {
			from = "scope " + scope.name
		}
		ro := ""
		if p.readonly {
			ro = "readonly "
		}
		return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) " + ro + "property " + p.label() + " from " + from}
	}
	v, err = h.coerceProperty(p, v, strict, false)
	if err != nil {
		return err
	}
	if slot == nil {
		return &Throwable{"Error", "Cannot write to virtual property " + p.label()}
	}
	if p.readonly && o.hooks["\x00clone"] {
		o.hooks["\x00clone:"+p.key] = true
	}
	return h.assign(slot, v)
}

func (h *heap) unsetObject(obj value, name string, scope *class) error {
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false)
	if err != nil {
		if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
			return callErr
		}
		return err
	}
	o, err = h.initializeLazy(o)
	if err != nil {
		return err
	}
	key := name
	if p == nil {
		if slot, ok := o.props.lookup(name); !ok || slot.kind == valueUndefined {
			if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
				return callErr
			}
		}
	}
	if p != nil {
		key = p.key
		if p.readonly {
			return &Throwable{"Error", "Cannot unset readonly property " + p.label()}
		}
		if !canAccess(scope, p.owner, p.setVisibility) {
			return &Throwable{"Error", "Cannot unset " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + methodScopeLabel(scope)}
		}
		if p.get != nil || p.set != nil {
			return &Throwable{"Error", "Cannot unset hooked property " + p.label()}
		}
	}
	if slot, ok := o.props.lookup(key); ok {
		h.detachPropertySource(slot)
	}
	old, ok := o.props.remove(key)
	if !ok {
		return nil
	}
	return h.release(old)
}

func (h *heap) referenceObject(dst *value, obj value, name string, scope *class) error {
	if err := h.claimValues(*dst, obj); err != nil {
		return err
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false)
	if err != nil {
		return err
	}
	key := name
	if p != nil {
		key = p.key
		if p.readonly {
			return &Throwable{"Error", "Cannot indirectly modify readonly property " + p.label()}
		}
		if !canAccess(scope, p.owner, p.setVisibility) {
			return &Throwable{"Error", "Cannot indirectly modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + methodScopeLabel(scope)}
		}
		bypassHook := h.propertyHookBypasses(o, p)
		if p.virtual && bypassHook {
			return &Throwable{"Error", "Must not read from virtual property " + p.label()}
		}
		if p.get != nil && !bypassHook && p.getReference {
			r, err := h.getPropertyHook(o, p, true)
			if err != nil {
				return err
			}
			return h.bind(dst, &r)
		}
		if (p.get != nil && !bypassHook) || p.virtual {
			return &Throwable{"Error", "Indirect modification of " + p.label() + " is not allowed"}
		}
	}
	o, err = h.objectForProperty(o, p, name)
	if err != nil {
		return err
	}
	var slot *value
	if p == nil {
		slot, err = h.dynamicPropertySlot(o, name)
	} else {
		slot = o.props.slot(key)
	}
	if err != nil {
		return err
	}
	return h.bindPropertyReference(dst, slot, p)
}

// __get/__set/__isset/__unset recurse independently for each property.
func (h *heap) magicProperty(o *object, operation, name string, extra []value) (value, bool, error) {
	key := "\x00magic:" + operation + ":" + name
	if o.hooks[key] {
		return value{}, false, nil
	}
	m, ok := h.resolveMethod(o.class, "__"+operation, nil)
	if !ok {
		return value{}, false, nil
	}
	if o.hooks == nil {
		o.hooks = make(map[string]bool)
	}
	o.hooks[key] = true
	defer delete(o.hooks, key)
	args := []methodArgument{{value: str(name)}}
	for _, v := range extra {
		args = append(args, methodArgument{value: v})
	}
	out, err := h.invokeMethod(value{kind: valueObject, object: o}, m, args, nil, false, methodCallSite{})
	return out, true, err
}

func (h *heap) issetObject(obj value, name string, scope *class) (bool, error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return false, err
	}
	p, accessErr := h.resolveInstanceProperty(o, name, scope, true)
	key := name
	if p != nil {
		key = p.key
	}
	if accessErr != nil || p == nil {
		if accessErr == nil {
			if slot, ok := o.props.lookup(key); ok && slot.kind != valueUndefined {
				v := slot.deref()
				return v.kind != valueNull, nil
			}
		}
		v, called, err := h.magicProperty(o, "isset", name, nil)
		if called {
			return h.toBool(v), err
		}
		return false, nil
	}
	if p.get != nil {
		v, err := h.readObject(obj, name, scope)
		return err == nil && v.deref().kind != valueNull, err
	}
	o, err = h.initializeLazy(o)
	if err != nil {
		return false, err
	}
	slot, ok := o.props.lookup(key)
	return ok && slot.kind != valueUndefined && slot.deref().kind != valueNull, nil
}
