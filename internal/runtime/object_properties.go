package runtime

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
	var first *propertyInfo
	coerced := false
	// Capture the original range, but read each metadata cell at its turn:
	// callbacks can remove/reorder sources in the same backing storage.
	for _, source := range h.typedSources[r] {
		p := source.info
		state := referenceTypeState(p, v, strict)
		if state == 0 {
			return value{}, referenceWriteError(p, v)
		}
		if first != nil && (state < 0) != coerced {
			return value{}, referenceCoercionError(first, p, v)
		}
		if state < 0 {
			candidate, ok, err := h.coerceTypedValue(&methodSpec{owner: p.owner}, p.owner, p.typeName, v, strict)
			if err != nil {
				if objectPHPException(err) {
					err = h.mergeObjectErrors(err, referenceWriteError(p, v))
				}
				return value{}, err
			}
			if !ok {
				return value{}, referenceWriteError(p, v)
			}
			if first != nil {
				same, err := identical(candidate, result)
				if err != nil {
					return value{}, err
				}
				if !same {
					return value{}, referenceCoercionError(first, p, v)
				}
			}
			result, coerced = candidate, true
		}
		if first == nil {
			first = p
		}
	}
	return result, nil
}

// Acceptance is distinct from possible scalar coercion. In particular, a
// previous exact type conflicts before a later Stringable conversion runs.
func referenceTypeState(p *propertyInfo, v value, strict bool) int {
	if methodAcceptsType(&methodSpec{owner: p.owner}, p.owner, p.typeName, v) {
		return 1
	}
	if strict {
		if v.kind == valueInt && methodTypeHas(p.typeName, "float") {
			return -1
		}
		return 0
	}
	if v.kind != valueNull && (methodTypeHas(p.typeName, "int") || methodTypeHas(p.typeName, "float") ||
		methodTypeHas(p.typeName, "string") || methodTypeHas(p.typeName, "bool")) {
		return -1
	}
	return 0
}

func referenceWriteError(p *propertyInfo, v value) error {
	return &Throwable{"TypeError", "Cannot assign " + typeName(v) + " to reference held by property " + p.label() + " of type " + p.typeName}
}

func referenceCoercionError(first, next *propertyInfo, v value) error {
	return &Throwable{"TypeError", "Cannot assign " + typeName(v) + " to reference held by property " + first.label() + " of type " + first.typeName +
		" and property " + next.label() + " of type " + next.typeName + ", as this would result in an inconsistent type conversion"}
}

// PHP's observable source-buffer order uses one inline source, then capacity
// four and doubling. Keep that capacity independently of Go's append policy.
func (h *heap) addPropertySource(r *reference, source propertySource) {
	if h.typedSources == nil {
		h.typedSources = make(map[*reference][]propertySource)
	}
	sources := h.typedSources[r]
	if len(sources) == cap(sources) {
		capacity := max(1, 2*cap(sources))
		if capacity == 2 {
			capacity = 4
		}
		grown := make([]propertySource, len(sources), capacity)
		copy(grown, sources)
		sources = grown
	}
	h.typedSources[r] = append(sources, source)
}

func (h *heap) detachPropertySource(slot *value) {
	if slot.kind != valueReference {
		return
	}
	sources := h.typedSources[slot.ref]
	for j, source := range sources {
		if source.slot != slot {
			continue
		}
		// PHP removes the first matching property-info identity, even when
		// clone added the same definition again. Remap its surviving Go slot.
		for i := range sources {
			info := sources[i].info
			// Inheritance copies Go descriptors without creating a new PHP
			// declaration. Redeclarations and trait consumers have new owners.
			if info != source.info && (source.info.owner == nil || info.owner != source.info.owner || info.key != source.info.key) {
				continue
			}
			if i != j {
				sources[j].slot = sources[i].slot
			}
			last := len(sources) - 1
			sources[i] = sources[last]
			// In-flight verifiers still observe the immutable tail metadata.
			// Drop only the mutable slot pointer so freed tables can be collected.
			sources[last].slot = nil
			sources = sources[:last]
			if last == 0 {
				delete(h.typedSources, slot.ref)
			} else {
				if last >= 4 && last*4 == cap(sources) {
					shrunk := make([]propertySource, last, last*2)
					copy(shrunk, sources)
					sources = shrunk
				}
				h.typedSources[slot.ref] = sources
			}
			return
		}
	}
}

func readonlyObjectPropertyWriteError(o *object, p *propertyInfo, slot *value) error {
	if p.readonly && slot != nil && slot.kind != valueUndefined &&
		(!o.hooks["\x00clone"] || o.hooks["\x00clone:"+p.key]) {
		return &Throwable{"Error", "Cannot modify readonly property " + p.label()}
	}
	return nil
}

func (h *heap) revalidatePropertyReceiver(o *object, p *propertyInfo) error {
	if o == nil || o.owner != h || o.freed || h.live[o.handle] != o {
		return &Throwable{"Error", "Object was released while assigning to property " + p.label()}
	}
	return nil
}

func (h *heap) readObject(obj value, name string, scope *class) (value, error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return value{}, err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false, "get")
	if err != nil {
		if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
			if callErr != nil {
				return value{}, callErr
			}
			return h.dereferenceMethodReturn(out)
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
	if p == nil && o.lazy != nil && !o.lazy.running {
		if slot, ok := o.props.lookup(name); !ok || slot.kind == valueUndefined {
			if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
				if callErr != nil {
					return value{}, callErr
				}
				return h.dereferenceMethodReturn(out)
			}
		}
	}
	previous := o
	pending := o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
	guarded := o.lazy != nil && !o.lazy.running && o.hooks[magicPropertyKey("get", name)]
	o, err = h.objectForPropertyRead(o, p, name)
	if err != nil {
		return value{}, err
	}
	key := name
	if p != nil {
		key = p.key
	}
	slot, exists := o.props.lookup(key)
	if (!exists || slot.kind == valueUndefined) && o != previous && guarded && (p == nil || (pending && previous.lazy != nil && previous.lazy.initialize == nil)) {
		guard := h.installPropertyGuard(o, "get", name)
		defer guard.restore()
	}
	if o != previous {
		return h.readObject(value{kind: valueObject, object: o}, name, scope)
	}
	if p == nil {
		if exists && slot.kind != valueUndefined {
			return slot.deref(), nil
		}
		if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
			if callErr != nil {
				return value{}, callErr
			}
			return h.dereferenceMethodReturn(out)
		}
		h.warn(levelWarning, "Undefined property: "+o.class.name+"::$"+name)
		return nullValue(), nil
	}
	if p.virtual {
		return value{}, &Throwable{"Error", "Property " + p.label() + " is write-only"}
	}
	if slot != nil && slot.isExplicitlyUnset() {
		if out, called, callErr := h.magicProperty(o, "get", name, nil); called {
			if callErr != nil {
				return value{}, callErr
			}
			return h.dereferenceMethodReturn(out)
		}
	}
	if slot.kind == valueUndefined {
		if p.typeName != "" {
			return value{}, &Throwable{"Error", "Typed property " + p.label() + " must not be accessed before initialization"}
		}
		h.warn(levelWarning, "Undefined property: "+o.class.name+"::$"+name)
		return nullValue(), nil
	}
	return slot.deref(), nil
}

type setterHookArgument struct {
	site   methodCallSite
	strict bool
}

func (h *heap) setPropertyHook(o *object, p *propertyInfo, v value, strict bool, site methodCallSite) (out value, err error) {
	out, err = h.runPropertyHookWithArgument(o, p, p.set, v, false, &setterHookArgument{site, strict})
	return out, h.mergeObjectErrors(err, h.discard(out))
}

func (h *heap) runPropertyHook(o *object, p *propertyInfo, hook objectHook, v value, byReference bool) (out value, err error) {
	return h.runPropertyHookWithArgument(o, p, hook, v, byReference, nil)
}

func (h *heap) runPropertyHookWithArgument(o *object, p *propertyInfo, hook objectHook, v value, byReference bool, argument *setterHookArgument) (out value, err error) {
	if err := h.claimValues(value{kind: valueObject, object: o}, v); err != nil {
		return value{}, err
	}
	receiver := methodHold{value: value{kind: valueObject, object: o}, temporary: o.refcount == 0}
	retain(receiver.value)
	// Capture the raw RHS after the receiver hold: they can be the same object.
	count := methodRefcount(v)
	rhs := methodHold{value: v, temporary: count == 0}
	if count >= 0 {
		retain(v)
	}
	out, err = h.invokeObjectInScope(func() (value, error) {
		if argument != nil {
			var err error
			v, err = h.prepareSetterArgument(o, p, v.deref(), argument.strict, argument.site)
			if err != nil {
				return value{}, err
			}
		}
		h.currentHook = activePropertyHook{receiver: o, prototypeOwner: p.prototypeOwner, name: p.name}
		return hook(h, value{kind: valueObject, object: o}, v)
	}, byReference, o.class)
	result, protected := h.holdMethodResult(out)
	if count >= 0 {
		if cleanup := h.releaseMethodHold(rhs); cleanup != nil {
			err = h.mergeObjectErrors(err, cleanup)
		}
	}
	if cleanup := h.releaseMethodHold(receiver); cleanup != nil {
		err = h.mergeObjectErrors(err, cleanup)
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
		if p.typeName == "" || p.typeName == "mixed" {
			return raw, nil
		}
		out, err := h.checkMethodReturn(raw, &methodSpec{owner: p.owner, name: p.name, propertyGetter: true, returnType: p.typeName, returnsReference: p.getReference}, this, p.hookStrict)
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
	return h.writeObjectAt(obj, name, v, scope, strict, methodCallSite{})
}

func (h *heap) writeObjectAt(obj value, name string, v value, scope *class, strict bool, site methodCallSite) error {
	if err := h.claimValues(obj, v); err != nil {
		return err
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	p, err := h.resolveInstanceProperty(o, name, scope, false, "set")
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
				return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + h.calledScopeLabel(scope)}
			}
			_, err = h.setPropertyHook(o, p, v, strict, site)
			return err
		}
		return &Throwable{"Error", "Property " + p.label() + " is read-only"}
	}
	if p != nil && !p.static && p.set != nil && !bypassHook {
		slot, _ := o.props.lookup(p.key)
		if err := readonlyObjectPropertyWriteError(o, p, slot); err != nil {
			return err
		}
		if !canAccess(scope, p.owner, p.setVisibility) {
			from := h.calledScopeLabel(scope)
			ro := ""
			if p.readonly && p.setVisibility == protectedAccess {
				ro = "readonly "
			}
			return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) " + ro + "property " + p.label() + " from " + from}
		}
		_, err = h.setPropertyHook(o, p, v, strict, site)
		return err
	}
	if o.lazy != nil && !o.lazy.running {
		key := name
		if p != nil {
			key = p.key
		}
		slot, exists := o.props.lookup(key)
		if (p == nil && (!exists || slot.kind == valueUndefined)) || (p != nil && o.lazy.initialize != nil && o.lazy.target.kind != valueObject && exists && slot.isExplicitlyUnset()) {
			// Magic assignment runs before initialization. Guarded reentry
			// falls through to the ordinary write/initialization path.
			if _, called, callErr := h.magicProperty(o, "set", name, []value{v}); called {
				return callErr
			}
		}
	}
	previous := o
	pending := o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
	guarded := o.lazy != nil && !o.lazy.running && o.hooks[magicPropertyKey("set", name)]
	o, err = h.objectForProperty(o, p, name)
	if err != nil {
		return err
	}
	if o != previous && guarded && (p == nil || (pending && previous.lazy != nil && previous.lazy.initialize == nil)) {
		guard := h.installPropertyGuard(o, "set", name)
		defer guard.restore()
	}
	if o != previous {
		return h.writeObjectAt(value{kind: valueObject, object: o}, name, v, scope, strict, site)
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
		return h.assignPHP(slot, v, strict)
	}
	if p.virtual && p.set == nil {
		return &Throwable{"Error", "Property " + p.label() + " is read-only"}
	}
	slot, _ := o.props.lookup(p.key)
	if slot.isExplicitlyUnset() {
		if _, called, callErr := h.magicProperty(o, "set", name, []value{v}); called {
			return callErr
		}
	}
	return h.writeObjectSlot(o, p, v, scope, strict)
}

func (h *heap) writeObjectSlot(o *object, p *propertyInfo, v value, scope *class, strict bool) error {
	slot, _ := o.props.lookup(p.key)
	if err := readonlyObjectPropertyWriteError(o, p, slot); err != nil {
		return err
	}
	if !canAccess(scope, p.owner, p.setVisibility) {
		from := h.calledScopeLabel(scope)
		ro := ""
		if p.readonly && p.setVisibility == protectedAccess {
			ro = "readonly "
		}
		return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) " + ro + "property " + p.label() + " from " + from}
	}
	v, err := h.coerceProperty(p, v, strict, false)
	if err != nil {
		return err
	}
	if err := h.revalidatePropertyReceiver(o, p); err != nil {
		return err
	}
	slot, _ = o.props.lookup(p.key)
	if slot == nil {
		return &Throwable{"Error", "Cannot write to virtual property " + p.label()}
	}
	if p.readonly && o.hooks["\x00clone"] {
		o.hooks["\x00clone:"+p.key] = true
	}
	return h.assignPHP(slot, v, strict)
}

func (h *heap) unsetObject(obj value, name string, scope *class) error {
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	for {
		// An initialized proxy's values and readonly state live on its target.
		if o.lazy != nil && o.lazy.target.kind == valueObject {
			info, lookupErr := resolvePropertyIn(o.class, name, scope, true)
			if lookupErr == nil && (info == nil || info.static) {
				if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
					return callErr
				}
				if info != nil {
					h.warnStaticInstanceProperty(o, info.name, "unset")
				}
				if o.hooks[magicPropertyKey("unset", name)] {
					next, forwardErr := h.initializeLazy(o)
					if forwardErr != nil {
						return forwardErr
					}
					guard := h.installPropertyGuard(next, "unset", name)
					if guard.target != nil {
						return h.unsetForwardedProperty(guard, name, scope)
					}
					o = next
					continue
				}
			}
			o, err = h.initializeLazy(o)
			if err != nil {
				return err
			}
		}
		p, err := h.resolveInstanceProperty(o, name, scope, false, "unset")
		if err != nil {
			if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
				return callErr
			}
			return err
		}
		key := name
		var slot *value
		if p == nil {
			if slot, ok := o.props.lookup(name); !ok || slot.kind == valueUndefined {
				if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
					return callErr
				}
			}
		}
		if p != nil {
			key = p.key
			if p.get != nil || p.set != nil {
				return &Throwable{"Error", "Cannot unset hooked property " + p.label()}
			}
			slot, _ = o.props.lookup(key)
			explicit := slot != nil && slot.isExplicitlyUnset()
			if explicit {
				if _, called, callErr := h.magicProperty(o, "unset", name, nil); called {
					return callErr
				}
			}
			if p.readonly && slot != nil && slot.kind != valueUndefined &&
				(!o.hooks["\x00clone"] || o.hooks["\x00clone:"+p.key]) {
				return &Throwable{"Error", "Cannot unset readonly property " + p.label()}
			}
			if !canAccess(scope, p.owner, p.setVisibility) {
				readonly := ""
				if p.readonly && p.setVisibility == protectedAccess {
					readonly = "readonly "
				}
				return &Throwable{"Error", "Cannot unset " + accessName(p.setVisibility) + "(set) " + readonly + "property " + p.label() + " from " + h.calledScopeLabel(scope)}
			}
			if explicit {
				return nil
			}
		}
		pending := o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
		guarded := pending && o.hooks[magicPropertyKey("unset", name)]
		next, err := h.objectForProperty(o, p, name)
		if err != nil {
			return err
		}
		if next != o || (pending && o.lazy.initialize == nil) {
			if next != o && guarded && o.lazy != nil && o.lazy.initialize == nil {
				guard := h.installPropertyGuard(next, "unset", name)
				if guard.target != nil {
					return h.unsetForwardedProperty(guard, name, scope)
				}
			}
			// Ghost initialization can fill a readonly slot in place. Resolve
			// and validate again, just as for a newly selected proxy target.
			o = next
			continue
		}
		if p != nil && slot != nil && !p.virtual {
			h.detachPropertySource(slot)
			old := *slot
			*slot = explicitlyUnsetValue()
			return h.release(old)
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
}

func (h *heap) unsetForwardedProperty(guard forwardedPropertyGuard, name string, scope *class) error {
	defer guard.restore()
	return h.unsetObject(value{kind: valueObject, object: guard.target}, name, scope)
}

func (h *heap) referenceObject(dst *value, obj value, name string, scope *class) error {
	if err := h.claimValues(*dst, obj); err != nil {
		return err
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return err
	}
	address, err := h.resolveObjectReference(o, name, scope)
	if err != nil {
		return err
	}
	// All forwarding guards end before old-destination cleanup can call PHP.
	if address.slot != nil {
		return h.bindPropertyReference(dst, address.slot, address.property)
	}
	return h.bindMethodReference(dst, address.raw)
}

type objectReferenceAddress struct {
	slot     *value
	property *propertyInfo
	raw      value
}

func (h *heap) resolveObjectReference(o *object, name string, scope *class) (objectReferenceAddress, error) {
	p, err := h.resolveInstanceProperty(o, name, scope, false, "get")
	if err != nil {
		if address, called, callErr := h.resolveMagicPropertyReference(o, name); called {
			return address, callErr
		}
		return objectReferenceAddress{}, err
	}
	key := name
	if p != nil {
		key = p.key
		if p.readonly {
			return objectReferenceAddress{}, &Throwable{"Error", "Cannot indirectly modify readonly property " + p.label()}
		}
		if !canAccess(scope, p.owner, p.setVisibility) {
			return objectReferenceAddress{}, &Throwable{"Error", "Cannot indirectly modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + h.calledScopeLabel(scope)}
		}
		bypassHook := h.propertyHookBypasses(o, p)
		if p.virtual && bypassHook {
			return objectReferenceAddress{}, &Throwable{"Error", "Must not read from virtual property " + p.label()}
		}
		if p.get != nil && !bypassHook {
			r, err := h.getPropertyHook(o, p, p.getReference)
			if err != nil {
				return objectReferenceAddress{}, err
			}
			if p.getReference || r.kind == valueObject {
				return objectReferenceAddress{raw: r}, nil
			}
			failure := indirectHookReferenceError(o, p)
			return objectReferenceAddress{}, h.mergeObjectErrors(failure, h.discardMethodReturn(r))
		}
		if p.virtual {
			return objectReferenceAddress{}, &Throwable{"Error", "Property " + o.class.name + "::$" + p.name + " is write-only"}
		}
	}
	if p == nil && o.lazy != nil && !o.lazy.running {
		if slot, ok := o.props.lookup(name); !ok || slot.kind == valueUndefined {
			if address, called, callErr := h.resolveMagicPropertyReference(o, name); called {
				return address, callErr
			}
		}
	}
	previous := o
	pending := o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
	guarded := o.lazy != nil && !o.lazy.running && o.hooks[magicPropertyKey("get", name)]
	o, err = h.objectForPropertyRead(o, p, name)
	if err != nil {
		return objectReferenceAddress{}, err
	}
	var forwardingGuard forwardedPropertyGuard
	if o != previous && guarded && (p == nil || (pending && previous.lazy != nil && previous.lazy.initialize == nil)) {
		forwardingGuard = h.installPropertyGuard(o, "get", name)
		defer forwardingGuard.restore()
	}
	if o != previous {
		return h.resolveObjectReference(o, name, scope)
	}
	var slot *value
	if p == nil {
		slot, _ = o.props.lookup(name)
		if slot == nil || slot.kind == valueUndefined {
			if address, called, callErr := h.resolveMagicPropertyReference(o, name); called {
				return address, callErr
			}
			slot, err = h.dynamicPropertySlot(o, name)
		}
	} else {
		slot = o.props.slot(key)
		if slot.isExplicitlyUnset() {
			if address, called, callErr := h.resolveMagicPropertyReference(o, name); called {
				return address, callErr
			}
		}
	}
	if err != nil {
		return objectReferenceAddress{}, err
	}
	if p != nil && p.set != nil && !h.propertyHookBypasses(o, p) {
		v := slot.deref()
		if v.kind == valueUndefined && p.typeName != "" {
			return objectReferenceAddress{}, &Throwable{"Error", "Typed property " + p.label() + " must not be accessed before initialization"}
		}
		if v.kind == valueObject {
			// A hooked by-value read exposes the object, not the backing location.
			return objectReferenceAddress{raw: v}, nil
		}
		return objectReferenceAddress{}, indirectHookReferenceError(o, p)
	}
	return objectReferenceAddress{slot: slot, property: p}, nil
}

func indirectHookReferenceError(o *object, p *propertyInfo) *Throwable {
	return &Throwable{"Error", "Indirect modification of " + o.class.name + "::$" + p.name + " is not allowed"}
}

func (h *heap) resolveMagicPropertyReference(o *object, name string) (objectReferenceAddress, bool, error) {
	raw, called, err := h.magicProperty(o, "get", name, nil)
	if !called || err != nil {
		return objectReferenceAddress{}, called, err
	}
	if raw.kind != valueReference && raw.deref().kind != valueObject {
		h.warn(levelNotice, "Indirect modification of overloaded property "+o.class.name+"::$"+cString(name)+" has no effect")
	}
	return objectReferenceAddress{raw: raw}, true, nil
}

// __get/__set/__isset/__unset recurse independently for each property.
func (h *heap) magicProperty(o *object, operation, name string, extra []value) (value, bool, error) {
	key := magicPropertyKey(operation, name)
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
	switch operation {
	case "isset":
		result := boolValue(h.toBool(out))
		return result, true, h.mergeObjectErrors(err, h.discardMethodReturn(out))
	case "set", "unset":
		return value{}, true, h.mergeObjectErrors(err, h.discardMethodReturn(out))
	default:
		return out, true, err
	}
}

func (h *heap) issetObject(obj value, name string, scope *class) (bool, error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return false, err
	}
	return h.issetObjectProperty(o, name, scope, false)
}

func (h *heap) issetObjectProperty(o *object, name string, scope *class, skipMagic bool) (bool, error) {
	p, accessErr := h.resolveInstanceProperty(o, name, scope, true)
	key := name
	if p != nil {
		key = p.key
	}
	if accessErr != nil || p == nil {
		if accessErr == nil {
			if !skipMagic && o.lazy != nil && !o.lazy.running {
				if slot, ok := o.props.lookup(key); !ok || slot.kind == valueUndefined {
					if v, called, callErr := h.magicProperty(o, "isset", name, nil); called {
						return h.toBool(v), callErr
					}
				}
			}
			previous := o
			next, err := h.objectForPropertyRead(o, p, name)
			if err != nil {
				return false, err
			}
			o = next
			if o != previous {
				return h.issetObjectProperty(o, name, scope, true)
			}
			if slot, ok := o.props.lookup(key); ok && slot.kind != valueUndefined {
				v := slot.deref()
				return v.kind != valueNull, nil
			}
		}
		if skipMagic {
			return false, nil
		}
		v, called, err := h.magicProperty(o, "isset", name, nil)
		if called {
			return h.toBool(v), err
		}
		return false, nil
	}
	if p.virtual || (p.get != nil && !h.propertyHookBypasses(o, p)) {
		v, err := h.readObject(value{kind: valueObject, object: o}, name, scope)
		set := err == nil && v.deref().kind != valueNull
		return set, h.mergeObjectErrors(err, h.discardMethodReturn(v))
	}
	previous := o
	pending := o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
	next, err := h.objectForPropertyRead(o, p, name)
	if err != nil {
		return false, err
	}
	o = next
	if o != previous {
		return h.issetObjectProperty(o, name, scope, true)
	}
	slot, ok := o.props.lookup(key)
	if ok && slot.isExplicitlyUnset() {
		if skipMagic || (pending && previous.lazy != nil && previous.lazy.initialize == nil) {
			return false, nil
		}
		v, called, err := h.magicProperty(o, "isset", name, nil)
		if called {
			return v.boolean, err
		}
		return false, nil
	}
	return ok && slot.kind != valueUndefined && slot.deref().kind != valueNull, nil
}
