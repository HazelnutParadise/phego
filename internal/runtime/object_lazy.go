package runtime

import "fmt"

func objectHasDestructor(o *object) bool {
	return o.class != nil && o.class.destructor != nil && (o.lazy == nil || (o.lazy.initialize == nil && o.lazy.target.kind != valueObject))
}

// REQ-018/020/021; T-035/036/039-042. The initializer and proxy target
// belong to this object, never its shared class description.
func (h *heap) newLazyObject(c *class, proxy bool, initialize func(*heap, value) (value, error)) (value, error) {
	if initialize == nil {
		return value{}, fmt.Errorf("lazy initializer is required")
	}
	if c == nil {
		return value{}, fmt.Errorf("class is required")
	}
	if c.schema != nil {
		if err := h.registerClass(c); err != nil {
			return value{}, err
		}
		s := c.schema
		kind := ""
		if s.isInterface {
			kind = "interface"
		} else if s.isTrait {
			kind = "trait"
		} else if s.abstract {
			kind = "abstract class"
		} else if s.enumKind != enumNone {
			kind = "enum"
		}
		if kind != "" {
			return value{}, &Throwable{"Error", "Cannot instantiate " + kind + " " + c.name}
		}
	}
	if c.schema != nil && (c.schema.layout == nil || len(c.schema.layout.names) == 0) {
		return h.newObject(c)
	}
	obj, err := h.newObjectShell(c)
	if err != nil {
		return value{}, err
	}
	obj.object.lazy = &lazyObject{initialize: initialize, proxy: proxy, remaining: len(obj.object.props.declared)}
	return obj, nil
}

// Host code and PHP callable ownership are explicit, separate inputs. Go closure
// reachability cannot stand in for the callable's logical PHP owners or edges.
func (h *heap) newLazyObjectWithInitializer(c *class, proxy bool, initializer value, initialize func(*heap, value) (value, error)) (value, error) {
	if err := h.claimValues(initializer); err != nil {
		return value{}, err
	}
	o, err := h.newLazyObject(c, proxy, initialize)
	if err != nil {
		return value{}, err
	}
	if o.object.lazy == nil {
		return o, nil
	}
	if err := h.assign(&o.object.lazy.initializer, initializer); err != nil {
		return value{}, h.mergeObjectErrors(err, h.discard(o))
	}
	return o, nil
}

func (h *heap) isLazyObject(obj value) bool {
	o, err := h.checkedObject(obj)
	return err == nil && o.lazy != nil && o.lazy.initialize != nil && !o.lazy.running
}

func (h *heap) isLazyProperty(obj value, name string) bool {
	o, err := h.checkedObject(obj)
	if err != nil || o.lazy == nil || o.lazy.initialize == nil ||
		(!o.lazy.proxy && o.lazy.running) || o.class.schema == nil {
		return false
	}
	p, err := h.resolveInstanceProperty(o, name, o.class, true)
	return err == nil && isLazyPropertyInfo(o, p)
}

func isLazyPropertyInfo(o *object, p *propertyInfo) bool {
	if p == nil || p.static || p.virtual || o.lazy == nil || o.lazy.initialize == nil ||
		(!o.lazy.proxy && o.lazy.running) {
		return false
	}
	_, nonLazy := o.lazy.nonLazy[p.key]
	return !nonLazy
}

// Resolve a backing location only when an actual access needs initialization.
func (h *heap) objectForProperty(o *object, p *propertyInfo, name string) (*object, error) {
	return h.resolvePropertyBacking(o, p, name, false)
}

func (h *heap) objectForPropertyRead(o *object, p *propertyInfo, name string) (*object, error) {
	return h.resolvePropertyBacking(o, p, name, true)
}

func (h *heap) resolvePropertyBacking(o *object, p *propertyInfo, name string, readUnset bool) (*object, error) {
	if o.lazy == nil {
		return o, nil
	}
	l := o.lazy
	if l.target.kind == valueObject {
		return h.initializeLazy(o)
	}
	if l.running {
		return o, nil
	}
	if l.initialize != nil {
		key := name
		if p != nil {
			key = p.key
		}
		if slot, ok := o.props.lookup(key); ok && (slot.kind != valueUndefined || (readUnset && slot.isExplicitlyUnset())) {
			return o, nil
		}
	}
	return h.initializeLazy(o)
}

// A forwarded guard owns only the bit it installed, for one target operation.
// Keep restoration on the Go stack; an existing target guard belongs to its caller.
type forwardedPropertyGuard struct {
	target *object
	key    string
}

func magicPropertyKey(operation, name string) string {
	return "\x00magic:" + operation + ":" + name
}

func (h *heap) installPropertyGuard(target *object, operation, name string) forwardedPropertyGuard {
	if _, ok := h.resolveMethod(target.class, "__"+operation, nil); !ok {
		return forwardedPropertyGuard{}
	}
	key := magicPropertyKey(operation, name)
	if target.hooks[key] {
		return forwardedPropertyGuard{}
	}
	if target.hooks == nil {
		target.hooks = make(map[string]bool)
	}
	target.hooks[key] = true
	return forwardedPropertyGuard{target, key}
}

func (g *forwardedPropertyGuard) restore() {
	target, key := g.target, g.key
	*g = forwardedPropertyGuard{}
	if target != nil {
		delete(target.hooks, key)
	}
}

func copyPropertyTable(src *propertyTable) *propertyTable {
	dst := newPropertyTable(src.layout)
	copyPropertyValues(dst, src)
	return dst
}

func copyPropertyValues(dst, src *propertyTable) {
	dst.materialized = src.materialized || src.index != nil
	if len(src.index) != 0 && dst.index == nil {
		dst.index = make(map[string]*value, len(src.index))
	}
	for i, v := range src.declared {
		if v.kind == valueUndefined {
			dst.declared[i] = v
		}
	}
	src.each(func(name string, v *value) { retain(*v); *dst.slot(name) = *v })
}

func (h *heap) initializeLazy(o *object) (out *object, err error) {
	l := o.lazy
	if l == nil {
		return o, nil
	}
	if l.target.kind == valueObject {
		return h.initializeLazy(l.target.object)
	}
	if l.initialize == nil || l.running {
		return o, nil
	}
	var before *propertyTable
	var hooksBefore map[string]bool
	var nonLazyBefore map[string]struct{}
	if !l.proxy {
		before = copyPropertyTable(o.props)
		// A running ghost has no lazy properties. Raw writes cannot change
		// this map until initialization returns, so the snapshot can share it.
		nonLazyBefore = l.nonLazy
		if o.hooks != nil {
			hooksBefore = make(map[string]bool, len(o.hooks))
			for key, active := range o.hooks {
				hooksBefore[key] = active
			}
		}
	}
	l.running = true
	held := methodHold{value: value{kind: valueObject, object: o}, temporary: o.refcount == 0}
	retain(held.value)
	defer func() {
		if cleanup := h.releaseMethodHold(held); cleanup != nil {
			err = h.mergeObjectErrors(err, cleanup)
		}
		l.running = false
		if err != nil {
			out = nil
		}
	}()
	if !l.proxy {
		err = h.initializeProperties(o)
	}
	var result value
	if err == nil {
		result, err = h.invokeObject(func() (value, error) {
			return l.initialize(h, value{kind: valueObject, object: o})
		}, false)
	}
	if err == nil && h.closed {
		err = ErrHeapClosed
	}
	if err == nil && !l.proxy && result.kind != valueNull && result.kind != valueUndefined {
		err = &Throwable{"TypeError", "Lazy object initializer must return NULL or no value"}
	}
	var target *object
	if err == nil && l.proxy {
		if result.deref().kind != valueObject {
			err = &Throwable{"TypeError", "Lazy proxy factory must return an instance of a class compatible with " + o.class.name + ", " + valueName(result) + " returned"}
		}
	}
	if err == nil && l.proxy {
		target, err = h.checkedObject(result)
		if err != nil {
			err = &Throwable{"TypeError", "Lazy proxy factory must return an instance of a class compatible with " + o.class.name + ", " + valueName(result) + " returned"}
		} else if !lazyProxyClassCompatible(h, o.class, target.class) {
			err = &Throwable{"TypeError", "The real instance class " + target.class.name + " is not compatible with the proxy class " + o.class.name + ". The proxy must be a instance of the same class as the real instance, or a sub-class with no additional properties, and no overrides of the __destructor or __clone methods."}
		} else if target == o {
			err = &Throwable{"Error", "Lazy proxy factory must return a non-lazy object"}
		} else if target.lazy != nil &&
			(target.lazy.initialize != nil || target.lazy.proxy) {
			err = &Throwable{"Error", "Lazy proxy factory must return a non-lazy object"}
		}
		if err == nil {
			result = result.deref()
			retain(result)
		}
	}
	if err != nil {
		err = h.mergeObjectErrors(err, h.discard(result))
		if !l.proxy {
			if cleanup := h.restoreLazyProperties(o, before); cleanup != nil {
				err = h.mergeObjectErrors(err, cleanup)
			}
			l.nonLazy = nonLazyBefore
			if hooksBefore == nil {
				o.hooks = nil
			} else {
				if o.hooks == nil {
					o.hooks = make(map[string]bool, len(hooksBefore))
				} else {
					clear(o.hooks)
				}
				for key, active := range hooksBefore {
					o.hooks[key] = active
				}
			}
		}
		return nil, err
	}

	var cleanup error
	if l.proxy {
		initializer := l.initializer
		l.initializer = value{}
		cleanup = h.mergeObjectErrors(cleanup, h.release(initializer))
		l.initialize = nil
		l.nonLazy = nil
		l.remaining = 0
		l.target = result
		result = value{}
		cleanup = h.mergeObjectErrors(cleanup, h.freeProperties(o, nil))
	} else {
		cleanup = h.mergeObjectErrors(cleanup, h.freePropertyTable(before, nil, false))
		initializer := l.initializer
		l.initializer = value{}
		cleanup = h.mergeObjectErrors(cleanup, h.release(initializer))
		l.initialize = nil
		l.nonLazy = nil
		l.remaining = 0
	}
	cleanup = h.mergeObjectErrors(cleanup, h.discard(result))
	if cleanup != nil {
		return nil, cleanup
	}
	if l.target.kind == valueObject {
		return h.initializeLazy(l.target.object)
	}
	return o, nil
}

func (h *heap) freeLazyStorage(o *object, dead map[gcNode]bool) error {
	if o.lazy == nil {
		return nil
	}
	initializer, target := o.lazy.initializer, o.lazy.target
	o.lazy.initializer, o.lazy.target = value{}, value{}
	o.lazy.nonLazy = nil
	o.lazy.remaining = 0
	err := h.releaseOutside(initializer, dead)
	return h.mergeObjectErrors(err, h.releaseOutside(target, dead))
}

// Restore declared locations in declaration order while initialization remains
// guarded. Snapshot values transfer their owners back to the original slots.
// PHP 8.5.12 overwrites values installed by reentrant destruction without
// releasing them. Preserve their logical counts: early release would change
// destructor timing. The request's object/resource stores still own final
// cleanup; the native consecutive-case corruption is diagnostic-only.
func (h *heap) restoreLazyProperties(o *object, before *propertyTable) error {
	var first error
	if o.class.schema != nil {
		// buildClass lays out instance slots in this filtered schema order.
		// Keep each source update beside its slot's release: callbacks can
		// observe which earlier declarations have already been restored.
		physical := 0
		for i := range o.class.schema.props {
			p := &o.class.schema.props[i]
			if p.static || p.virtual {
				continue
			}
			slot := &o.props.declared[physical]
			h.detachPropertySource(slot)
			old := *slot
			*slot = value{}
			if err := h.release(old); err != nil {
				first = h.mergeObjectErrors(first, err)
			}
			*slot = before.declared[physical]
			before.declared[physical] = value{}
			if slot.kind == valueReference && p.typeName != "" {
				h.addPropertySource(slot.ref, propertySource{slot, p})
			}
			physical++
		}
	}
	// The detached chain may still have an outer traversal. Cleanup must not
	// unlink it, even though this temporary wrapper has a different address.
	dynamic := &propertyTable{index: o.props.index, head: o.props.head, tail: o.props.tail,
		capacity: o.props.capacity, retired: o.props.retired, iterating: true}
	o.props.index = nil
	o.props.head = nil
	o.props.tail = nil
	o.props.capacity = 0
	o.props.retired = 0
	o.props.materialized = false
	if err := h.freePropertyTable(dynamic, nil, false); err != nil {
		first = h.mergeObjectErrors(first, err)
	}
	o.props.index = before.index
	o.props.head = before.head
	o.props.tail = before.tail
	o.props.capacity = before.capacity
	o.props.retired = before.retired
	o.props.materialized = before.materialized
	before.index = nil
	before.head = nil
	before.tail = nil
	before.capacity = 0
	before.retired = 0
	return first
}

func lazyProxyClassCompatible(h *heap, wrapper, target *class) bool {
	if wrapper == nil || target == nil {
		return false
	}
	if wrapper == target {
		return true
	}
	subclass := false
	for c := wrapper; c != nil; {
		if c == target {
			subclass = true
			break
		}
		if c.schema == nil {
			break
		}
		c = c.schema.parent
	}
	if !subclass ||
		wrapper.schema == nil || target.schema == nil ||
		len(wrapper.schema.props) != len(target.schema.props) ||
		wrapper.destructorOwner != target.destructorOwner {
		return false
	}
	wrapperClone, _ := h.resolveMethod(wrapper, "__clone", nil)
	targetClone, _ := h.resolveMethod(target, "__clone", nil)
	return wrapperClone == targetClone
}

func (h *heap) cloneObject(obj value, scope *class) (result value, err error) {
	o, err := h.checkedObject(obj)
	if err != nil {
		return value{}, err
	}
	count := methodRefcount(obj.deref())
	retain(obj.deref())
	source := methodHold{value: obj.deref(), temporary: count == 0}
	defer func() {
		hold, protected := h.holdMethodResult(result)
		err = h.mergeObjectErrors(err, h.releaseMethodHold(source))
		if protected {
			if err != nil {
				hold.temporary = false
			}
			err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
		}
		if err != nil {
			result = value{}
		}
	}()
	if m := h.weakMapObjects[o]; m != nil {
		return h.cloneWeakMap(m)
	}
	if o.class.schema != nil && o.class.schema.enumKind != enumNone {
		return value{}, &Throwable{"Error", "Trying to clone an uncloneable object of class " + o.class.name}
	}
	m, hasClone := h.resolveMethod(o.class, "__clone", nil)
	if hasClone && !canAccess(scope, m.owner, m.visibility) {
		return value{}, &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + m.owner.name + "::__clone() from " + methodScopeLabel(scope)}
	}
	if o.lazy != nil && o.lazy.proxy {
		wrapper := o
		if _, err := h.initializeLazy(wrapper); err != nil {
			return value{}, err
		}
		if wrapper.lazy.target.kind != valueObject {
			return value{}, &Throwable{"Error", "Cannot clone an uninitialized lazy proxy"}
		}
		clone, err := h.newObjectShell(wrapper.class)
		if err != nil {
			return value{}, err
		}
		cloneObjectProperties(h, clone.object, wrapper)
		backing, err := h.cloneObject(wrapper.lazy.target, scope)
		if err != nil {
			clone.object.destructorCalled = true
			return value{}, h.mergeObjectErrors(err, h.freeObject(clone.object))
		}
		clone.object.lazy = &lazyObject{proxy: true, target: backing}
		retain(backing)
		return clone, nil
	}
	o, err = h.initializeLazy(o)
	if err != nil {
		return value{}, err
	}
	clone, err := h.newObjectShell(o.class)
	if err != nil {
		return value{}, err
	}
	cloneObjectProperties(h, clone.object, o)
	if hasClone {
		if clone.object.hooks == nil {
			clone.object.hooks = make(map[string]bool)
		}
		clone.object.hooks["\x00clone"] = true
		// The handler may return the clone itself, directly or in a composite.
		// Keep the construction temporary alive while consuming that result.
		hold, _ := h.holdMethodResult(clone)
		out, callErr := h.invokeMethod(clone, m, nil, scope, false, methodCallSite{})
		err = h.mergeObjectErrors(callErr, h.discardMethodReturn(out))
		clear(clone.object.hooks)
		if err != nil {
			hold.temporary = false
			return value{}, h.mergeObjectErrors(err, h.releaseMethodHold(hold))
		}
		if err := h.releaseMethodHold(hold); err != nil {
			return value{}, err
		}
	}
	return clone, nil
}

func cloneObjectProperties(h *heap, dst, src *object) {
	copyPropertyValues(dst.props, src.props)
	dst.props.each(func(_ string, slot *value) {
		if slot.kind != valueReference {
			return
		}
		if slot.ref.refcount == 2 {
			// A reference without an external alias is separated by clone.
			old := *slot
			v := slot.deref()
			retain(v)
			*slot = v
			_ = h.release(old)
			return
		}
	})
	if schema := src.class.schema; schema != nil {
		for i := range schema.props {
			p := &schema.props[i]
			if p.static || p.virtual || p.typeName == "" {
				continue
			}
			if slot, ok := dst.props.lookup(p.key); ok && slot.kind == valueReference {
				h.addPropertySource(slot.ref, propertySource{slot, p})
			}
		}
	}
}

func (h *heap) enumCase(c *class, name string) (value, error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	if c == nil || c.schema == nil {
		return value{}, fmt.Errorf("enum class required")
	}
	if h.enums == nil {
		h.enums = make(map[*class]map[string]value)
	}
	cases := h.enums[c]
	if cases == nil {
		cases = make(map[string]value)
		h.enums[c] = cases
	}
	if v, ok := cases[name]; ok {
		return v, nil
	}
	for i, n := range c.schema.enumCases {
		if n != name {
			continue
		}
		v, err := h.newObject(c)
		if err != nil {
			return value{}, err
		}
		if err = h.writeObject(v, "name", value{kind: valueString, text: name}, c, true); err != nil {
			return value{}, err
		}
		if i < len(c.schema.enumValues) {
			backing, err := h.importSnapshotPHP8512(c.schema.enumValues[i], 1, 0)
			if err != nil {
				return value{}, err
			}
			err = h.writeObject(v, "value", backing, c, true)
			_ = h.release(backing)
			if err != nil {
				return value{}, err
			}
		}
		retain(v)
		cases[name] = v
		return v, nil
	}
	return value{}, &Throwable{"Error", "Undefined constant " + c.name + "::" + name}
}
