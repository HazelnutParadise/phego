package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// REQ-012/018/020/021; T-023/024/035-042.
type w13Case struct {
	Result      any   `json:"result"`
	Diagnostics []any `json:"diagnostics"`
}

type w13Method func(*heap, value, []value) (value, error)

type w13Fixtures struct {
	base, child, dynamic, hooks, lazy, enum *class
	initialize, baseSecret, childSecret     w13Method
	events                                  *[]string
}

func w13Int(i int64) value { return value{kind: valueInt, integer: i} }

func w13Str(s string) value { return bytesValue([]byte(s)) }

func w13Must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func w13Build(t *testing.T, spec classSpec) *class {
	t.Helper()
	c, err := buildClass(spec)
	w13Must(t, "buildClass("+spec.name+")", err)
	return c
}

func w13BuildFixtures(t *testing.T, events *[]string) *w13Fixtures {
	t.Helper()
	f := &w13Fixtures{events: events}
	var base, child, hooks, lazy, classEnum *class

	baseSpec := classSpec{
		name: "W13Base",
		properties: []propertySpec{
			{name: "i", typeName: "int"},
			{name: "nullable", typeName: "?int", hasDefault: true, initial: nil},
			{name: "ro", typeName: "int", readonly: true, setVisibility: protectedAccess},
			{name: "secret", typeName: "string", visibility: privateAccess, hasDefault: true, initial: []byte("base")},
			{name: "protected", typeName: "string", visibility: protectedAccess, hasDefault: true, initial: []byte("protected")},
		},
		methods: []methodSpec{
			{name: "initialize", call: func(h *heap, this value, args []value) (value, error) {
				return value{}, h.writeObject(this, "ro", args[0], base, true)
			}},
			{name: "baseSecret", call: func(h *heap, this value, _ []value) (value, error) {
				return h.readObject(this, "secret", base)
			}},
		},
	}
	base = w13Build(t, baseSpec)
	f.base = base
	f.initialize = baseSpec.methods[0].call
	f.baseSecret = baseSpec.methods[1].call

	childSpec := classSpec{
		name:   "W13Child",
		parent: base,
		properties: []propertySpec{
			{name: "secret", typeName: "string", visibility: privateAccess, hasDefault: true, initial: []byte("child")},
		},
		methods: []methodSpec{
			{name: "childSecret", call: func(h *heap, this value, _ []value) (value, error) {
				return h.readObject(this, "secret", child)
			}},
		},
	}
	child = w13Build(t, childSpec)
	f.child = child
	f.childSecret = childSpec.methods[0].call

	dynamicSpec := classSpec{name: "W13Dynamic", allowDynamic: true}
	f.dynamic = w13Build(t, dynamicSpec)

	hooksSpec := classSpec{
		name: "W13Hooks",
		properties: []propertySpec{
			{
				name: "backed", typeName: "int", hasDefault: true, initial: int64(1),
				get: func(h *heap, this value, _ value) (value, error) {
					*events = append(*events, "get")
					return h.readObject(this, "backed", hooks)
				},
				set: func(h *heap, this value, v value) (value, error) {
					*events = append(*events, "set")
					doubled, err := h.arith("*", v, w13Int(2))
					if err != nil {
						return value{}, err
					}
					return value{}, h.writeObject(this, "backed", doubled, hooks, true)
				},
			},
			{
				name: "virtual", typeName: "int", virtual: true,
				get: func(h *heap, this value, _ value) (value, error) {
					backed, err := h.readObject(this, "backed", hooks)
					if err != nil {
						return value{}, err
					}
					return h.arith("+", backed, w13Int(10))
				},
			},
		},
	}
	hooks = w13Build(t, hooksSpec)
	f.hooks = hooks

	lazySpec := classSpec{
		name: "W13Lazy",
		properties: []propertySpec{
			{name: "i", typeName: "int"},
			{name: "s", typeName: "string", hasDefault: true, initial: []byte("default")},
		},
	}
	lazy = w13Build(t, lazySpec)
	f.lazy = lazy

	enumSpec := classSpec{
		name:       "W13Enum",
		enumCases:  []string{"A", "B"},
		enumValues: []any{[]byte("a"), []byte("b")},
	}
	classEnum = w13Build(t, enumSpec)
	f.enum = classEnum
	return f
}

func w13Instantiate(t *testing.T, h *heap, c *class) value {
	t.Helper()
	v, err := h.instantiate(c)
	w13Must(t, "instantiate("+c.name+")", err)
	return v
}

func w13NewStdClass(t *testing.T, h *heap) value {
	t.Helper()
	v, err := h.newObject(&class{name: "stdClass"})
	w13Must(t, "newObject(stdClass)", err)
	return v
}

func w13Read(t *testing.T, h *heap, obj value, name string, scope *class) value {
	t.Helper()
	v, err := h.readObject(obj, name, scope)
	w13Must(t, "readObject("+name+")", err)
	return v
}

func w13Write(t *testing.T, h *heap, obj value, name string, v value, scope *class) {
	t.Helper()
	w13Must(t, "writeObject("+name+")", h.writeObject(obj, name, v, scope, true))
}

func w13Call(t *testing.T, what string, fn w13Method, h *heap, this value, args ...value) value {
	t.Helper()
	v, err := fn(h, this, args)
	w13Must(t, what, err)
	return v
}

func w13Error(err error) any {
	var throwable *Throwable
	if errors.As(err, &throwable) {
		return []any{throwable.Class, throwable.Message}
	}
	return []any{"Error", err.Error()}
}

func w13ObserveValue(fn func() (value, error)) any {
	v, err := fn()
	if err != nil {
		return w13Error(err)
	}
	return w13Encode(v)
}

func w13ObserveError(fn func() error) any {
	if err := fn(); err != nil {
		return w13Error(err)
	}
	return nil
}

func w13Diagnostics(h *heap) []any {
	out := make([]any, 0, len(h.diagnostics))
	for _, d := range h.diagnostics {
		out = append(out, []any{int64(d.level), d.message})
	}
	return out
}

func w13Encode(v value) any {
	v = v.deref()
	switch v.kind {
	case valueUndefined, valueNull:
		return nil
	case valueBool:
		return v.boolean
	case valueInt:
		return v.integer
	case valueFloat:
		return v.floating
	case valueString:
		return v.text
	case valueArray:
		return w13EncodeArray(v.array)
	case valueObject:
		return w13ObjectAsMap(v)
	}
	return nil
}

func w13EncodeArray(a *phpArray) any {
	type entry struct {
		key arrayKey
		val value
	}
	var entries []entry
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		entries = append(entries, entry{key: b.key, val: b.val})
	}
	list := true
	for i, e := range entries {
		if e.key.isString || e.key.integer != int64(i) {
			list = false
			break
		}
	}
	if list {
		out := make([]any, len(entries))
		for i, e := range entries {
			out[i] = w13Encode(e.val)
		}
		return out
	}
	out := make(map[string]any, len(entries))
	for _, e := range entries {
		key := e.key.text
		if !e.key.isString {
			key = strconv.FormatInt(e.key.integer, 10)
		}
		out[key] = w13Encode(e.val)
	}
	return out
}

func w13ObjectKeys(obj value) []any {
	obj = obj.deref()
	out := []any{}
	obj.object.props.each(func(name string, _ *value) {
		out = append(out, name)
	})
	return out
}

func w13ObjectAsMap(obj value) map[string]any {
	obj = obj.deref()
	out := map[string]any{}
	obj.object.props.each(func(name string, slot *value) {
		out[name] = w13Encode(*slot)
	})
	return out
}

func w13Isset(h *heap, obj value, name string, scope *class) bool {
	isset, err := h.issetObject(obj, name, scope)
	return err == nil && isset
}

// attachPropertySource seeds typed-source metadata for fixtures that bind slots
// directly. Production rollback uses its already-known schema descriptor.
func (h *heap) attachPropertySource(o *object, slot *value, key string) {
	if slot.kind != valueReference || o.class.schema == nil {
		return
	}
	for i := range o.class.schema.props {
		p := &o.class.schema.props[i]
		if p.key == key && !p.static && !p.virtual && p.typeName != "" {
			h.addPropertySource(slot.ref, propertySource{slot, p})
			return
		}
	}
}

func w13RunCase(t *testing.T, h *heap, f *w13Fixtures, name string) w13Case {
	for _, review := range w13R11TraitInheritanceNames() {
		if name == review {
			return w13R11TraitInheritanceCase(t, h, name)
		}
	}
	for _, review := range w13R11TraitNames() {
		if name == review {
			return w13R11TraitCase(t, h, name)
		}
	}
	for _, review := range w13R11CallableNames() {
		if name == review {
			return w13R11CallableCase(t, h, name)
		}
	}
	for _, review := range w13R11EnumNames() {
		if name == review {
			return w13R11EnumCase(t, h, name)
		}
	}
	for _, review := range w13R10ProxyControlNames() {
		if name == review {
			return w13R10ProxyControlCase(t, h, name)
		}
	}
	for _, review := range w13R10ProxyNames() {
		if name == review {
			return w13R10ProxyCase(t, h, name)
		}
	}
	for _, review := range w13R10LifeNames() {
		if name == review {
			return w13R10LifeCase(t, h, name)
		}
	}
	for _, review := range w13R10AliasNames() {
		if name == review {
			return w13R10AliasCase(t, h, name)
		}
	}
	for _, review := range w13R9Names() {
		if name == review {
			return w13R9Case(t, h, name)
		}
	}
	for _, review := range w13R8Names() {
		if name == review {
			return w13R8Case(t, h, name)
		}
	}
	for _, review := range w13R7RawControlNames() {
		if name == review {
			return w13R7RawControlCase(t, h, name)
		}
	}
	for _, review := range w13R7Names() {
		if name == review {
			return w13R7Case(t, h, name)
		}
	}
	for _, review := range w13R7LazyStateNames() {
		if name == review {
			return w13R7LazyStateCase(t, h, name)
		}
	}
	if name == w13R6GuardLifetimeName {
		return w13R6GuardLifetimeCase(t, h)
	}
	for _, review := range w13R6ProxyGuardNames() {
		if name == review {
			return w13R6ProxyGuardCase(t, h, name)
		}
	}
	for _, review := range w13R6ScopeNames() {
		if name == review {
			return w13R6ScopeCase(t, h, name)
		}
	}
	for _, review := range w13R6LazyNames() {
		if name == review {
			return w13R6LazyCase(t, h, name)
		}
	}
	for _, review := range w13R6ReferenceNames() {
		if name == review {
			return w13R6ReferenceCase(t, h, name)
		}
	}
	for _, review := range w13R5LifetimeNames() {
		if name == review {
			return w13R5LifetimeCase(t, h, name)
		}
	}
	for _, review := range w13R5ReferenceNames() {
		if name == review {
			return w13R5ReferenceCase(t, h, name)
		}
	}
	for _, review := range w13R5OperationNames() {
		if name == review {
			return w13R5OperationCase(t, h, name)
		}
	}
	for _, review := range w13R4AccessNames() {
		if name == review {
			return w13R4AccessCase(t, h, name)
		}
	}
	for _, review := range w13R4WeakMapNames() {
		if name == review {
			return w13R4WeakMapCase(t, h, name)
		}
	}
	for _, review := range w13R3CallableReturnNames() {
		if name == review {
			return w13R3CallableReturnCase(t, h, name)
		}
	}
	for _, review := range w13R3TypedReturnNames() {
		if name == review {
			return w13R3TypedReturnCase(t, h, name)
		}
	}
	for _, review := range w13R3FrameNames() {
		if name == review {
			return w13R3FrameCase(t, h, name)
		}
	}
	for _, review := range w13R3HookNames() {
		if name == review {
			return w13R3HookCase(t, h, name)
		}
	}
	for _, review := range w13R3ValueNames() {
		if name == review {
			return w13R3ValueCase(t, h, name)
		}
	}
	for _, review := range w13R3PropertyNames() {
		if name == review {
			return w13R3PropertyCase(t, h, name)
		}
	}
	for _, review := range w13R3EnumNames() {
		if name == review {
			return w13R3EnumCase(t, h, name)
		}
	}
	for _, review := range w13R2CleanupNames() {
		if name == review {
			return w13R2CleanupCase(t, h, name)
		}
	}
	for _, review := range w13R2LazyNames() {
		if name == review {
			return w13R2LazyCase(t, h, name)
		}
	}
	for _, review := range w13R2ValueNames() {
		if name == review {
			return w13R2ValueCase(t, h, name)
		}
	}
	for _, review := range w13R2PropertyNames() {
		if name == review {
			return w13R2PropertyCase(t, h, name)
		}
	}
	for _, review := range w13R2ReturnNames() {
		if name == review {
			return w13R2ReturnCase(t, h, name)
		}
	}
	for _, review := range w13ReviewNames() {
		if name == review {
			return w13ReviewCase(t, h, name)
		}
	}
	for _, static := range w13StaticNames() {
		if name == static {
			return w13StaticCase(t, h, name)
		}
	}
	t.Helper()
	var result any
	for _, hookFrame := range w13HookFrameNames() {
		if name == hookFrame {
			return w13HookFrameCase(t, h, name)
		}
	}
	for _, captureCase := range w13CaptureNames() {
		if name == captureCase {
			return w13CaptureCase(t, h, name)
		}
	}
	for _, shutdownCase := range w13ShutdownCaseNames() {
		if name == shutdownCase {
			return w13RunShutdownCase(t, h, name)
		}
	}
	for _, proxyCase := range w13ProxyCaseNames() {
		if name == proxyCase {
			return w13RunProxyCase(t, h, name)
		}
	}
	for _, caseName := range w13NameCases() {
		if name == caseName {
			return w13NameCase(t, h, name)
		}
	}
	for _, ownership := range w13OwnershipNames() {
		if name == ownership {
			return w13OwnershipCase(t, h.width, name)
		}
	}
	for _, typeCase := range w13TypeCaseNames() {
		if name == typeCase {
			return w13TypeCase(t, h, name)
		}
	}
	for _, propertyCase := range w13PropertyCaseNames() {
		if name == propertyCase {
			return w13PropertyCase(t, h, name)
		}
	}
	for _, advanced := range w13AdvancedMethodNames() {
		if name == advanced {
			return w13AdvancedMethodCase(t, h.width, name)
		}
	}
	for _, weakCase := range w13WeakMapCaseNames() {
		if name == weakCase {
			return w13RunWeakMapCase(t, h, name)
		}
	}
	if strings.HasPrefix(name, "gc-sequence-") {
		return w13RunGCCorpusCase(t, h, name)
	}
	if name == "resource-last-owner" || name == "resource-closed-kind" || name == "resource-property-lifetime" {
		return w13RunResourceCase(t, h, name)
	}
	for _, methodCase := range w13MethodCaseNames() {
		if name == methodCase {
			return w13RunMethodCase(t, h, name)
		}
	}
	if name == "hook-frame-lifetime" {
		return w13Case{Result: w13RunHookFrame(t, h.width), Diagnostics: []any{}}
	}
	if name == "gc-hole-order" || name == "gc-nested" || name == "gc-created" || name == "lazy-uninitialized-destructor" {
		return w13RunGCCase(t, h, name)
	}

	switch name {
	case "uninitialized":
		o := w13Instantiate(t, h, f.base)
		result = w13ObserveValue(func() (value, error) { return h.readObject(o, "i", f.base) })

	case "typed-write":
		o := w13Instantiate(t, h, f.base)
		result = w13ObserveError(func() error { return h.writeObject(o, "i", w13Str("x"), f.base, true) })

	case "typed-null":
		o := w13Instantiate(t, h, f.base)
		result = w13ObserveError(func() error { return h.writeObject(o, "i", nullValue(), f.base, true) })

	case "typed-reference":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "i", w13Int(1), f.base)
		var r value
		w13Must(t, "referenceObject(i)", h.referenceObject(&r, o, "i", f.base))
		e := w13ObserveError(func() error { return h.assign(&r, w13Str("x")) })
		result = []any{e, w13Encode(r.deref()), w13Encode(w13Read(t, h, o, "i", f.base))}

	case "reference-detach":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "i", w13Int(1), f.base)
		var r value
		w13Must(t, "referenceObject(i)", h.referenceObject(&r, o, "i", f.base))
		w13Must(t, "unsetObject(i)", h.unsetObject(o, "i", f.base))
		w13Must(t, "assign detached reference", h.assign(&r, w13Str("free")))
		result = []any{w13Encode(r.deref()), w13Isset(h, o, "i", f.base)}

	case "reference-destroy":
		o := w13Instantiate(t, h, f.base)
		var owner value
		w13Must(t, "own object", h.assign(&owner, o))
		w13Write(t, h, o, "i", w13Int(1), f.base)
		var r value
		w13Must(t, "referenceObject(i)", h.referenceObject(&r, o, "i", f.base))
		w13Must(t, "unset object", h.unset(&owner))
		w13Must(t, "assign detached reference", h.assign(&r, w13Str("free")))
		result = w13Encode(r.deref())

	case "reference-array-promotion":
		o := w13Instantiate(t, h, f.base)
		var r value
		w13Must(t, "referenceObject(nullable)", h.referenceObject(&r, o, "nullable", f.base))
		result = w13ObserveError(func() error { return h.appendValue(&r, w13Int(1)) })

	case "readonly-init":
		o := w13Instantiate(t, h, f.base)
		result = w13ObserveError(func() error { return h.writeObject(o, "ro", w13Int(1), nil, true) })

	case "readonly-rewrite":
		o := w13Instantiate(t, h, f.base)
		w13Call(t, "initialize(1)", f.initialize, h, o, w13Int(1))
		result = w13ObserveError(func() error { return h.writeObject(o, "ro", w13Int(2), nil, true) })

	case "readonly-reference":
		o := w13Instantiate(t, h, f.base)
		w13Call(t, "initialize(1)", f.initialize, h, o, w13Int(1))
		result = w13ObserveError(func() error {
			var r value
			return h.referenceObject(&r, o, "ro", nil)
		})

	case "readonly-unset":
		o := w13Instantiate(t, h, f.base)
		w13Call(t, "initialize(1)", f.initialize, h, o, w13Int(1))
		result = w13ObserveError(func() error { return h.unsetObject(o, "ro", nil) })

	case "private-access":
		o := w13Instantiate(t, h, f.child)
		result = w13ObserveValue(func() (value, error) { return h.readObject(o, "secret", nil) })

	case "protected-access":
		o := w13Instantiate(t, h, f.child)
		result = w13ObserveValue(func() (value, error) { return h.readObject(o, "protected", nil) })

	case "private-inheritance":
		o := w13Instantiate(t, h, f.child)
		baseSecret := w13Call(t, "baseSecret", f.baseSecret, h, o)
		childSecret := w13Call(t, "childSecret", f.childSecret, h, o)
		result = []any{w13Encode(baseSecret), w13Encode(childSecret), w13ObjectKeys(o)}

	case "dynamic-order":
		o := w13Instantiate(t, h, f.dynamic)
		w13Write(t, h, o, "a", w13Int(1), f.dynamic)
		w13Write(t, h, o, "b", w13Int(2), f.dynamic)
		w13Must(t, "unsetObject(a)", h.unsetObject(o, "a", f.dynamic))
		w13Write(t, h, o, "a", w13Int(3), f.dynamic)
		result = w13ObjectAsMap(o)

	case "declared-order":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "i", w13Int(1), f.base)
		w13Must(t, "unsetObject(nullable)", h.unsetObject(o, "nullable", f.base))
		w13Write(t, h, o, "nullable", w13Int(2), f.base)
		result = w13ObjectKeys(o)

	case "dynamic-deprecation":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "extra", w13Int(1), f.base)
		w13Write(t, h, o, "extra", w13Int(2), f.base)
		result = w13Encode(w13Read(t, h, o, "extra", f.base))

	case "clone-reference":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "i", w13Int(1), f.base)
		var r value
		w13Must(t, "referenceObject(i)", h.referenceObject(&r, o, "i", f.base))
		c, err := h.cloneObject(o, nil)
		w13Must(t, "cloneObject", err)
		w13Must(t, "assign shared reference", h.assign(&r, w13Int(2)))
		same, err := identical(o, c)
		w13Must(t, "identical", err)
		equal, err := h.looseEqual(o, c)
		w13Must(t, "looseEqual", err)
		result = []any{
			w13Encode(w13Read(t, h, o, "i", nil)),
			w13Encode(w13Read(t, h, c, "i", nil)),
			same,
			equal,
		}

	case "clone-separate":
		o := w13Instantiate(t, h, f.base)
		w13Write(t, h, o, "i", w13Int(1), f.base)
		c, err := h.cloneObject(o, nil)
		w13Must(t, "cloneObject", err)
		w13Write(t, h, c, "i", w13Int(2), nil)
		equal, err := h.looseEqual(o, c)
		w13Must(t, "looseEqual", err)
		result = []any{
			w13Encode(w13Read(t, h, o, "i", nil)),
			w13Encode(w13Read(t, h, c, "i", nil)),
			equal,
		}

	case "hooks":
		*f.events = (*f.events)[:0]
		o := w13Instantiate(t, h, f.hooks)
		w13Write(t, h, o, "backed", w13Int(3), f.hooks)
		backed := w13Read(t, h, o, "backed", f.hooks)
		virtual := w13Read(t, h, o, "virtual", f.hooks)
		events := make([]any, len(*f.events))
		for i, event := range *f.events {
			events[i] = event
		}
		result = []any{w13Encode(backed), w13Encode(virtual), events}

	case "virtual-write":
		o := w13Instantiate(t, h, f.hooks)
		result = w13ObserveError(func() error { return h.writeObject(o, "virtual", w13Int(1), nil, true) })

	case "virtual-reference":
		o := w13Instantiate(t, h, f.hooks)
		result = w13ObserveError(func() error {
			var r value
			return h.referenceObject(&r, o, "virtual", nil)
		})

	case "hook-reflection":
		result = h.reflectObjectProperties(f.hooks)

	case "lazy-ghost":
		calls := 0
		o, err := h.newLazyObject(f.lazy, false, func(h *heap, obj value) (value, error) {
			calls++
			return value{}, h.writeObject(obj, "i", w13Int(7), f.lazy, true)
		})
		w13Must(t, "newLazyObject(ghost)", err)
		before := []any{h.isLazyObject(o), int64(calls)}
		i := w13Read(t, h, o, "i", f.lazy)
		s := w13Read(t, h, o, "s", f.lazy)
		result = []any{before, w13Encode(i), w13Encode(s), int64(calls), h.isLazyObject(o)}

	case "lazy-failure-retry":
		calls := 0
		o, err := h.newLazyObject(f.lazy, false, func(h *heap, obj value) (value, error) {
			calls++
			if err := h.writeObject(obj, "i", w13Int(int64(calls)), f.lazy, true); err != nil {
				return value{}, err
			}
			if calls == 1 {
				return value{}, &Throwable{"Exception", "retry"}
			}
			return value{}, nil
		})
		w13Must(t, "newLazyObject(ghost)", err)
		first := w13ObserveValue(func() (value, error) { return h.readObject(o, "i", f.lazy) })
		state := h.isLazyObject(o)
		second := w13Read(t, h, o, "i", f.lazy)
		result = []any{first, state, w13Encode(second), int64(calls)}

	case "lazy-proxy":
		real := w13Instantiate(t, h, f.lazy)
		w13Write(t, h, real, "i", w13Int(9), f.lazy)
		calls := 0
		o, err := h.newLazyObject(f.lazy, true, func(*heap, value) (value, error) {
			calls++
			return real, nil
		})
		w13Must(t, "newLazyObject(proxy)", err)
		same, err := identical(o, real)
		w13Must(t, "identical", err)
		i := w13Read(t, h, o, "i", f.lazy)
		result = []any{same, w13Encode(i), int64(calls), !h.isLazyObject(o)}

	case "enum-identity":
		a, err := h.enumCase(f.enum, "A")
		w13Must(t, "enumCase(A)", err)
		b, err := h.enumCase(f.enum, "B")
		w13Must(t, "enumCase(B)", err)
		sameAA, err := identical(a, a)
		w13Must(t, "identical(A,A)", err)
		sameAB, err := identical(a, b)
		w13Must(t, "identical(A,B)", err)
		equal, err := h.looseEqual(a, b)
		w13Must(t, "looseEqual(A,B)", err)
		order, err := h.compare(a, b)
		w13Must(t, "compare(A,B)", err)
		result = []any{
			sameAA,
			sameAB,
			w13Encode(w13Read(t, h, a, "name", f.enum)),
			w13Encode(w13Read(t, h, a, "value", f.enum)),
			equal,
			int64(order),
		}

	case "enum-clone":
		a, err := h.enumCase(f.enum, "A")
		w13Must(t, "enumCase(A)", err)
		result = w13ObserveValue(func() (value, error) { return h.cloneObject(a, nil) })

	case "weak-reference":
		o := w13NewStdClass(t, h)
		var slot value
		w13Must(t, "assign stdClass", h.assign(&slot, o))
		w, err := h.weakReferenceFor(o)
		w13Must(t, "weakReferenceFor", err)
		again, err := h.weakReferenceFor(o)
		w13Must(t, "weakReferenceFor again", err)
		w13Must(t, "unset stdClass", h.unset(&slot))
		v, _ := w.get()
		result = []any{w == again, w13Encode(v)}

	case "weak-map-backedge":
		m := newWeakMap()
		o := w13NewStdClass(t, h)
		var slot value
		w13Must(t, "assign stdClass", h.assign(&slot, o))
		w, err := h.weakReferenceFor(o)
		w13Must(t, "weakReferenceFor", err)
		w13Must(t, "weakMapSet", h.weakMapSet(m, o, o))
		w13Must(t, "unset stdClass", h.unset(&slot))
		freed, err := h.collectCycles()
		w13Must(t, "collectCycles", err)
		v, _ := w.get()
		result = []any{int64(freed), int64(m.count()), v.deref().kind == valueNull || v.deref().kind == valueUndefined}

	case "cycle":
		o := w13NewStdClass(t, h)
		var slot value
		w13Must(t, "assign stdClass", h.assign(&slot, o))
		w13Must(t, "setProperty(self)", h.setProperty(o, "self", o))
		w, err := h.weakReferenceFor(o)
		w13Must(t, "weakReferenceFor", err)
		w13Must(t, "unset stdClass", h.unset(&slot))
		freed, err := h.collectCycles()
		w13Must(t, "collectCycles", err)
		v, _ := w.get()
		result = []any{int64(freed), v.deref().kind == valueNull || v.deref().kind == valueUndefined}

	default:
		t.Fatalf("unknown W-13 case %q", name)
	}

	return w13Case{Result: result, Diagnostics: w13Diagnostics(h)}
}

func w13CaseNames() []string {
	names := append([]string{}, w13MethodCaseNames()...)
	names = append(names, w13R7LazyStateNames()...)
	names = append(names, w13R7Names()...)
	names = append(names, w13R7RawControlNames()...)
	names = append(names, w13R8Names()...)
	names = append(names, w13R9Names()...)
	names = append(names, w13R10ProxyNames()...)
	names = append(names, w13R11EnumNames()...)
	names = append(names, w13R11TraitNames()...)
	names = append(names, w13R11TraitInheritanceNames()...)
	names = append(names, w13R11CallableNames()...)
	names = append(names, w13R10ProxyControlNames()...)
	names = append(names, w13R10LifeNames()...)
	names = append(names, w13R10AliasNames()...)
	names = append(names, w13R6ProxyGuardNames()...)
	names = append(names, w13R6GuardLifetimeName)
	names = append(names, w13R6LazyNames()...)
	names = append(names, w13R6ScopeNames()...)
	names = append(names, w13R6ReferenceNames()...)
	names = append(names, w13AdvancedMethodNames()...)
	names = append(names, w13PropertyCaseNames()...)
	names = append(names, w13TypeCaseNames()...)
	names = append(names, w13OwnershipNames()...)
	names = append(names, w13NameCases()...)
	names = append(names, w13ProxyCaseNames()...)
	names = append(names, w13ShutdownCaseNames()...)
	names = append(names, w13CaptureNames()...)
	names = append(names, w13HookFrameNames()...)
	names = append(names, w13StaticNames()...)
	names = append(names, w13ReviewNames()...)
	names = append(names, w13R2ReturnNames()...)
	names = append(names, w13R2ValueNames()...)
	names = append(names, w13R2PropertyNames()...)
	names = append(names, w13R2LazyNames()...)
	names = append(names, w13R2CleanupNames()...)
	names = append(names, w13R4AccessNames()...)
	names = append(names, w13R5ReferenceNames()...)
	names = append(names, w13R5LifetimeNames()...)
	names = append(names, w13R5OperationNames()...)
	names = append(names, w13R4WeakMapNames()...)
	names = append(names, w13R3EnumNames()...)
	names = append(names, w13R3PropertyNames()...)
	names = append(names, w13R3ValueNames()...)
	names = append(names, w13R3HookNames()...)
	names = append(names, w13R3FrameNames()...)
	names = append(names, w13R3TypedReturnNames()...)
	names = append(names, w13R3CallableReturnNames()...)
	names = append(names, w13WeakMapCaseNames()...)
	for i := 1; i <= 64; i++ {
		names = append(names, fmt.Sprintf("gc-sequence-%d", i))
	}
	return append(names, []string{
		"resource-last-owner", "resource-closed-kind", "resource-property-lifetime",
		"hook-frame-lifetime",
		"gc-hole-order", "gc-nested", "gc-created", "lazy-uninitialized-destructor",
		"uninitialized",
		"typed-write",
		"typed-null",
		"typed-reference",
		"reference-detach",
		"reference-destroy",
		"reference-array-promotion",
		"readonly-init",
		"readonly-rewrite",
		"readonly-reference",
		"readonly-unset",
		"private-access",
		"protected-access",
		"private-inheritance",
		"dynamic-order",
		"declared-order",
		"dynamic-deprecation",
		"clone-reference",
		"clone-separate",
		"hooks",
		"virtual-write",
		"virtual-reference",
		"hook-reflection",
		"lazy-ghost",
		"lazy-failure-retry",
		"lazy-proxy",
		"enum-identity",
		"enum-clone",
		"weak-reference",
		"weak-map-backedge",
		"cycle",
	}...)
}

func TestW13PHP8512Objects(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			events := []string{}
			fixtures := w13BuildFixtures(t, &events)
			observed := map[string]w13Case{}
			allRan := true
			for _, name := range w13CaseNames() {
				name := name
				if !t.Run(name, func(t *testing.T) {
					h, err := newHeap(width)
					w13Must(t, "newHeap", err)
					observed[name] = w13RunCase(t, h, fixtures, name)
				}) {
					allRan = false
				}
			}
			if !allRan {
				return
			}

			dir := os.Getenv("PHEGO_W13_REFERENCE")
			if dir == "" {
				dir = "testdata"
			}
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("w13-ref-%d.json", width)))
			w13Must(t, "read reference", err)
			var reference struct {
				PHP     string             `json:"php"`
				IntSize int                `json:"int_size"`
				Cases   map[string]w13Case `json:"cases"`
			}
			w13Must(t, "decode reference", json.Unmarshal(data, &reference))
			if reference.PHP != "8.5.12" || reference.IntSize*8 != width {
				t.Fatalf("unexpected reference profile: php=%s int_size=%d width=%d", reference.PHP, reference.IntSize, width)
			}
			if len(reference.Cases) != len(w13CaseNames()) {
				t.Fatalf("reference has %d cases, want %d", len(reference.Cases), len(w13CaseNames()))
			}
			if len(observed) != len(w13CaseNames()) {
				t.Fatalf("observed %d cases, want %d", len(observed), len(w13CaseNames()))
			}

			raw, err := json.Marshal(observed)
			w13Must(t, "marshal observed", err)
			var normalized map[string]w13Case
			w13Must(t, "normalize observed", json.Unmarshal(raw, &normalized))

			for _, name := range w13CaseNames() {
				want, ok := reference.Cases[name]
				if !ok {
					t.Errorf("%s: missing from reference", name)
					continue
				}
				got := normalized[name]
				if !reflect.DeepEqual(got, want) {
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(want)
					t.Errorf("%s\nGo:  %s\nPHP: %s", name, gotJSON, wantJSON)
				}
			}
		})
	}
}
