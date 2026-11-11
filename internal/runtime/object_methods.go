package runtime

import (
	"fmt"
	"iter"
	"math"
	"strconv"
	"strings"
)

// REQ-012/018; T-023/024/035/036. An argument keeps its source location
// separate from its value, so by-reference binding can reject temporaries.
type methodParameter struct {
	name, typeName                    string
	byReference, variadic, hasDefault bool
	initial                           any
}
type methodArgument struct {
	name  string
	value value
	slot  *value
}
type methodCallSite struct {
	file string
	line int
}

// methodHold is one ownership hold acquired for a call. temporary marks a
// borrowed constructor-style value that must stay alive as an unowned
// temporary when this hold is the last owner.
type methodHold struct {
	value     value
	temporary bool
}

func (h *heap) callMethod(obj value, name string, args []methodArgument, scope *class, strict bool) (value, error) {
	return h.callMethodAt(obj, name, args, scope, strict, methodCallSite{})
}

// callMethodAt implements PHP's instance method dispatch: inherited methods,
// case-insensitive names, scope-aware visibility, private methods belonging to
// the calling scope, and __call for missing or inaccessible methods.
func (h *heap) callMethodAt(obj value, name string, args []methodArgument, scope *class, strict bool, site methodCallSite) (value, error) {
	if err := h.runtimeCheck(); err != nil {
		return value{}, err
	}
	if err := h.claimMethodValues(obj, args); err != nil {
		return value{}, err
	}
	obj = obj.deref()
	if obj.kind != valueObject || obj.object == nil {
		return value{}, fmt.Errorf("method call requires a live object owned by this request")
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return value{}, err
	}
	obj = value{kind: valueObject, object: o}
	// Methods keep the wrapper's class and $this; only backing access delegates.

	m, found := h.resolveMethod(o.class, name, scope)
	accessible := found && m != nil && canAccess(scope, m.owner, m.visibility)
	if accessible {
		return h.invokeMethod(obj, m, args, scope, strict, site)
	}

	if magic, ok := h.resolveMethod(o.class, "__call", nil); ok && magic != nil && !magic.static {
		return h.invokeMagicCall(obj, magic, name, args, scope, strict, site)
	}
	if found && m != nil {
		return value{}, &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + m.owner.name + "::" + m.name + "() from " + methodScopeLabel(scope)}
	}
	return value{}, &Throwable{"Error", "Call to undefined method " + o.class.name + "::" + name + "()"}
}

func methodScopeLabel(scope *class) string {
	if scope == nil {
		return "global scope"
	}
	return "scope " + scope.name
}

// resolveMethod returns the method PHP would consider for this call. A private
// method is bound to the scope that declares it, so a parent and a child can
// both declare private methods with the same name without overriding each
// other. Other methods come from the merged class table, which already
// contains inherited and trait methods.
func (h *heap) resolveMethod(c *class, name string, scope *class) (*methodSpec, bool) {
	if c == nil || c.schema == nil {
		return nil, false
	}
	key := phpNameLower(name)
	if scope != nil && classIs(c, scope) && scope.schema != nil {
		if candidate := scope.schema.methods[key]; candidate != nil &&
			candidate.visibility == privateAccess && candidate.owner == scope {
			return candidate, true
		}
	}
	if m, ok := c.schema.methods[key]; ok {
		return m, true
	}
	return nil, false
}

func (h *heap) invokeMagicCall(obj value, magic *methodSpec, methodName string, args []methodArgument, scope *class, strict bool, site methodCallSite) (res value, err error) {
	var packed value
	if _, err := h.separate(&packed); err != nil {
		return value{}, err
	}
	var result *methodHold
	defer func() {
		// The packed argument array is this wrapper's local owner. Release it
		// before the result protection so a __call result that aliases $args
		// survives as the caller's temporary.
		if rerr := h.release(packed); rerr != nil {
			err = h.mergeObjectErrors(err, rerr)
		}
		if result != nil {
			if err != nil {
				result.temporary = false
			}
			if rerr := h.releaseMethodHold(*result); rerr != nil {
				err = h.mergeObjectErrors(err, rerr)
			}
		}
		if err != nil {
			res = value{}
		}
	}()
	seenNamed := false
	for i := range args {
		a := &args[i]
		v := a.value.deref()
		var setErr error
		if a.name == "" {
			if seenNamed {
				return value{}, &Throwable{"Error", "Cannot use positional argument after named argument"}
			}
			setErr = h.appendValue(&packed, v)
		} else {
			seenNamed = true
			key := bytesValue([]byte(a.name))
			if _, found, err := h.getElement(packed, key); err != nil {
				return value{}, err
			} else if found {
				return value{}, &Throwable{"Error", "Named parameter $" + a.name + " overwrites previous argument"}
			}
			setErr = h.setElement(&packed, key, v)
		}
		if setErr != nil {
			return value{}, setErr
		}
	}
	// __call always receives ($name, $args); positional binding keeps working
	// when userland names those parameters differently.
	synthetic := []methodArgument{
		{name: "", value: bytesValue([]byte(methodName))},
		{name: "", value: packed},
	}
	out, callErr := h.invokeMethod(obj, magic, synthetic, scope, strict, site)
	if callErr != nil {
		return value{}, callErr
	}
	if hold, ok := h.holdMethodResult(out); ok {
		result = &hold
	}
	return out, nil
}

// invokeMethod binds and protects the callback arguments, invokes the method
// through the W-14 frame boundary, validates a typed return, and releases every
// owner acquired for the call even when the callback failed.
func (h *heap) invokeMethod(obj value, m *methodSpec, supplied []methodArgument, scope *class, strict bool, site methodCallSite) (res value, err error) {
	if err := h.claimMethodValues(obj, supplied); err != nil {
		return value{}, err
	}
	o, err := h.checkedObject(obj)
	if err != nil {
		return value{}, err
	}
	obj = value{kind: valueObject, object: o}
	var parameters, operands []methodHold
	var receiver methodHold
	var receiverOwned bool
	var result *methodHold
	// Capture every operand before any scalar conversion can call PHP.
	// Parameter owners then take over in declaration order.
	if count := methodRefcount(obj); count >= 0 {
		// The single-value form of holdMethodValue needs no slice allocation.
		retain(obj)
		receiver = methodHold{value: obj, temporary: count == 0}
		receiverOwned = true
	}

	defer func() {
		for _, hold := range operands {
			err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
		}
		for _, hold := range parameters {
			err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
		}
		if receiverOwned {
			err = h.mergeObjectErrors(err, h.releaseMethodHold(receiver))
		}
		if result != nil {
			if err != nil {
				result.temporary = false
			}
			err = h.mergeObjectErrors(err, h.releaseMethodHold(*result))
		}
		if err != nil {
			res = value{}
		}
	}()
	if m.call == nil {
		return value{}, &Throwable{"Error", "Cannot call abstract method " + m.owner.name + "::" + m.name + "()"}
	}
	args, err := h.captureMethodOperands(m, supplied, &operands)
	if err != nil {
		return value{}, err
	}
	callArgs, err := h.bindMethodArguments(obj, m, args, strict, site, &parameters)
	if err != nil {
		return value{}, err
	}
	// Converted source operands die here; unchanged operands survive through
	// their parameter owners. This also preserves named-argument teardown order.
	for _, hold := range operands {
		err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
	}
	operands = nil
	if err != nil && !objectPHPException(err) {
		return value{}, err
	}
	argumentCleanup := err
	this := obj
	if m.static {
		this = nullValue()
	}
	// Successful coercion remains successful when releasing its source throws.
	// PHP enters the body with that ordinary exception pending; sticky stops
	// above still prevent entry. Preserve the pending event across the call.
	out, callErr := h.invokeObjectInScope(func() (value, error) {
		raw, err := m.call(h, this, callArgs)
		if err != nil {
			return value{}, err
		}
		// Return verification belongs to this frame, before its locals unwind.
		return h.checkMethodReturn(raw, m, obj, m.returnStrict)
	}, m.returnsReference, obj.object.class)
	err = h.mergeObjectErrors(argumentCleanup, callErr)
	if err != nil {
		return value{}, h.mergeObjectErrors(err, h.discardMethodReturn(out))
	}
	if hold, ok := h.holdMethodResult(out); ok {
		result = &hold
	}
	return out, nil
}

func (h *heap) claimMethodValues(obj value, args []methodArgument) error {
	pending, err := h.pendingValues(obj)
	if err != nil {
		return err
	}
	for _, arg := range args {
		var next []gcNode
		if arg.slot != nil {
			next, err = h.pendingValues(arg.value, *arg.slot)
		} else {
			next, err = h.pendingValues(arg.value)
		}
		if err != nil {
			return err
		}
		pending = append(pending, next...)
	}
	h.commitValueOwnership(pending)
	return nil
}

func (h *heap) captureMethodOperands(m *methodSpec, supplied []methodArgument, holds *[]methodHold) ([]methodArgument, error) {
	args := append([]methodArgument(nil), supplied...)
	positional, variadic := 0, -1
	for i, p := range m.parameters {
		if p.variadic {
			variadic = i
		}
	}
	for i := range args {
		a := &args[i]
		index := -1
		if a.name != "" {
			for j, p := range m.parameters {
				if p.name == a.name {
					index = j
					break
				}
			}
			if index < 0 {
				index = variadic
			}
		} else {
			index = positional
			positional++
			if variadic >= 0 && index >= variadic {
				index = variadic
			}
		}
		if index >= 0 && index < len(m.parameters) && m.parameters[index].byReference && a.slot != nil {
			source := a.slot
			a.value = value{}
			if err := h.bind(&a.value, source); err != nil {
				return nil, err
			}
			a.slot = &a.value
			*holds = append(*holds, methodHold{value: a.value})
		} else {
			a.value = a.value.deref()
			if methodRefcount(a.value) >= 0 {
				retain(a.value)
				*holds = append(*holds, methodHold{value: a.value})
			}
		}
	}
	return args, nil
}

func (h *heap) bindMethodArguments(obj value, m *methodSpec, supplied []methodArgument, strict bool, site methodCallSite, cleanup *[]methodHold) ([]value, error) {
	params := m.parameters
	assigned := make([]*methodArgument, len(params))
	namedSeen := make(map[string]bool, len(supplied))
	hasNamed := false
	nextPositional := 0
	variadicIndex := -1
	for i := range params {
		if params[i].variadic {
			variadicIndex = i
		}
	}
	var variadic []methodArgument
	var extraPositionals []methodArgument

	for ai := range supplied {
		a := &supplied[ai]
		if a.name == "" {
			if hasNamed {
				return nil, &Throwable{"Error", "Cannot use positional argument after named argument"}
			}
			if variadicIndex >= 0 && nextPositional >= variadicIndex {
				variadic = append(variadic, *a)
				continue
			}
			if nextPositional >= len(params) {
				// PHP userland functions accept extra positional arguments.
				extraPositionals = append(extraPositionals, *a)
				continue
			}
			assigned[nextPositional] = a
			nextPositional++
			continue
		}

		hasNamed = true
		if namedSeen[a.name] {
			return nil, &Throwable{"Error", "Named parameter $" + a.name + " overwrites previous argument"}
		}
		namedSeen[a.name] = true
		index := -1
		for i := range params {
			if params[i].name == a.name {
				index = i
				break
			}
		}
		if index < 0 {
			if variadicIndex >= 0 {
				variadic = append(variadic, *a)
				continue
			}
			return nil, &Throwable{"Error", "Unknown named parameter $" + a.name}
		}
		if params[index].variadic {
			variadic = append(variadic, *a)
			continue
		}
		if assigned[index] != nil {
			return nil, &Throwable{"Error", "Named parameter $" + a.name + " overwrites previous argument"}
		}
		assigned[index] = a
	}

	firstMissing := -1
	for i := range params {
		if params[i].variadic || params[i].hasDefault {
			continue
		}
		if assigned[i] == nil {
			firstMissing = i
			break
		}
	}
	if firstMissing >= 0 {
		if hasNamed {
			return nil, &Throwable{"ArgumentCountError", fmt.Sprintf("%s::%s(): Argument #%d ($%s) not passed", m.owner.name, m.name, firstMissing+1, params[firstMissing].name)}
		}
		required, optional := 0, 0
		for i := range params {
			if params[i].variadic || params[i].hasDefault {
				optional++
			} else {
				required++
			}
		}
		expectation := "at least"
		if optional == 0 {
			expectation = "exactly"
		}
		return nil, &Throwable{"ArgumentCountError", fmt.Sprintf("Too few arguments to function %s::%s(), %d passed%s and %s %d expected", m.owner.name, m.name, len(supplied), methodCallLocation(site), expectation, required)}
	}

	callArgs := make([]value, len(params))
	for i := range params {
		p := &params[i]
		if p.variadic {
			arr, err := h.bindVariadicArgument(obj, m, p, variadic, strict, site, i, cleanup)
			if err != nil {
				return nil, err
			}
			callArgs[i] = arr
			continue
		}
		if assigned[i] == nil {
			dv, err := h.importSnapshotPHP8512(p.initial, 1024, 1<<20)
			if err != nil {
				return nil, err
			}
			*cleanup = append(*cleanup, methodHold{value: dv})
			callArgs[i] = dv
			continue
		}
		bv, err := h.bindMethodArgument(obj, m, p, assigned[i], i, strict, site, cleanup)
		if err != nil {
			return nil, err
		}
		callArgs[i] = bv
	}
	for _, a := range extraPositionals {
		v := a.value.deref()
		h.holdMethodValue(v, cleanup)
		callArgs = append(callArgs, v)
	}
	return callArgs, nil
}

func methodCallLocation(site methodCallSite) string {
	if site.file == "" && site.line == 0 {
		return ""
	}
	if site.line > 0 {
		return fmt.Sprintf(" in %s on line %d", site.file, site.line)
	}
	return " in " + site.file
}

func methodCalledLocation(site methodCallSite) string {
	if site.file == "" && site.line == 0 {
		return ""
	}
	if site.line > 0 {
		return fmt.Sprintf(", called in %s on line %d", site.file, site.line)
	}
	return ", called in " + site.file
}

func (h *heap) bindMethodArgument(obj value, m *methodSpec, p *methodParameter, a *methodArgument, index int, strict bool, site methodCallSite, cleanup *[]methodHold) (value, error) {
	v := a.value.deref()
	if p.byReference {
		if a.slot == nil {
			return value{}, &Throwable{"Error", fmt.Sprintf("%s::%s(): Argument #%d%s could not be passed by reference", m.owner.name, m.name, index+1, methodArgumentName(p))}
		}
		called := obj.object.class
		if p.typeName != "" && !methodAcceptsType(m, called, p.typeName, v) {
			// A typed property source constrains the existing reference.
			// Reject before coercion, which could execute Stringable code.
			if a.slot.kind == valueReference && len(h.typedSources[a.slot.ref]) != 0 {
				if !methodTypeHas(p.typeName, "callable") || !h.isCallable(v, callableContext{m.owner, called, !m.static && m.owner != nil}) {
					return value{}, h.methodArgumentTypeError(m, p, index, v, site)
				}
			} else {
				coerced, ok, err := h.coerceTypedValue(m, called, p.typeName, v, strict)
				if err != nil {
					return value{}, err
				}
				if !ok {
					return value{}, h.methodArgumentTypeError(m, p, index, v, site)
				}
				// Ordinary references retain weak coercion and source writeback.
				if err := h.assign(a.slot, coerced); err != nil {
					return value{}, err
				}
			}
		}
		var bound value
		if err := h.bind(&bound, a.slot); err != nil {
			return value{}, err
		}
		*cleanup = append(*cleanup, methodHold{value: bound})
		return bound, nil
	}

	called := obj.object.class
	if p.typeName != "" && !methodAcceptsType(m, called, p.typeName, v) {
		coerced, ok, err := h.coerceTypedValue(m, called, p.typeName, v, strict)
		if err != nil {
			return value{}, err
		}
		if !ok {
			return value{}, h.methodArgumentTypeError(m, p, index, v, site)
		}
		v = coerced
	}
	h.holdMethodValue(v, cleanup)
	return v, nil
}

func (h *heap) bindVariadicArgument(obj value, m *methodSpec, p *methodParameter, supplied []methodArgument, strict bool, site methodCallSite, index int, cleanup *[]methodHold) (value, error) {
	var arr value
	if _, err := h.separate(&arr); err != nil {
		return value{}, err
	}
	*cleanup = append(*cleanup, methodHold{value: arr})
	positionals := 0
	for ai := range supplied {
		a := &supplied[ai]
		bv, err := h.bindMethodArgument(obj, m, p, a, index+positionals, strict, site, cleanup)
		if err != nil {
			return value{}, err
		}
		if a.name == "" {
			positionals++
		}
		if p.byReference {
			var slot *value
			if a.name == "" {
				slot, err = h.appendElement(&arr)
			} else {
				slot, err = h.element(&arr, bytesValue([]byte(a.name)))
			}
			if err == nil {
				err = h.bind(slot, &bv)
			}
		} else if a.name == "" {
			err = h.appendValue(&arr, bv)
		} else {
			err = h.setElement(&arr, bytesValue([]byte(a.name)), bv)
		}
		if err != nil {
			return value{}, err
		}
	}
	return arr, nil
}

func (h *heap) methodArgumentTypeError(m *methodSpec, p *methodParameter, index int, v value, site methodCallSite) error {
	return &Throwable{"TypeError", fmt.Sprintf("%s::%s(): Argument #%d%s must be of type %s, %s given%s", m.owner.name, m.name, index+1, methodArgumentName(p), p.typeName, valueName(v), methodCalledLocation(site))}
}

func methodArgumentName(p *methodParameter) string {
	if p.variadic {
		return ""
	}
	return " ($" + p.name + ")"
}

func methodRefcount(v value) int {
	switch v.kind {
	case valueArray:
		if v.array != nil {
			return v.array.refcount
		}
	case valueObject, valueResource:
		if v.object != nil {
			return v.object.refcount
		}
	case valueReference:
		if v.ref != nil {
			return v.ref.refcount
		}
	}
	return -1
}

// holdMethodValue acquires an owner for an argument or receiver while user code
// runs. A value that was already owned must be released normally afterwards. An
// unowned constructor-style temporary is held only for the callback and then
// returned to its original unowned state, because releasing it would destroy a
// temporary its caller still holds.
func (h *heap) holdMethodValue(v value, cleanup *[]methodHold) {
	refcount := methodRefcount(v)
	if refcount < 0 {
		return
	}
	retain(v)
	*cleanup = append(*cleanup, methodHold{value: v, temporary: refcount == 0})
}

// holdMethodResult protects a callback result until the call's borrowed
// arguments and receiver have been dropped. If the protection is then the last
// owner, the result is left alive as the unowned temporary handed to the caller.
func (h *heap) holdMethodResult(v value) (methodHold, bool) {
	if methodRefcount(v) < 0 {
		return methodHold{}, false
	}
	retain(v)
	return methodHold{value: v, temporary: true}, true
}

func (h *heap) releaseMethodHold(hold methodHold) error {
	if hold.temporary && methodRefcount(hold.value) <= 1 {
		switch hold.value.kind {
		case valueArray:
			if hold.value.array != nil && hold.value.array.refcount > 0 {
				hold.value.array.refcount--
			}
		case valueObject, valueResource:
			if hold.value.object != nil && hold.value.object.refcount > 0 {
				hold.value.object.refcount--
			}
		case valueReference:
			if hold.value.ref != nil && hold.value.ref.refcount > 0 {
				hold.value.ref.refcount--
			}
		}
		return nil
	}
	return h.release(hold.value)
}

// Rejected or coerced-away raw results are consumed here. A coercion may run
// PHP that removes their last external owner, so keep a hold until it finishes.
func (h *heap) checkMethodReturn(v value, m *methodSpec, obj value, strict bool) (out value, err error) {
	typ := m.returnType
	if typ == "" || typ == "mixed" {
		return v, nil
	}
	called := (*class)(nil)
	if obj.kind == valueObject && obj.object != nil {
		called = obj.object.class
	}
	if typ != "void" && typ != "never" && (methodAcceptsType(m, called, typ, v) ||
		(methodTypeHas(typ, "callable") && h.isCallable(v, callableContext{m.owner, called, !m.static && m.owner != nil}))) {
		return v, nil
	}
	if err := h.claimValues(v); err != nil {
		return value{}, err
	}
	hold, protected := h.holdMethodResult(v)
	consume := true
	defer func() {
		if protected {
			if consume {
				// A converted composite can still share storage with the raw value.
				outHold, outProtected := h.holdMethodResult(out)
				hold.temporary = false
				err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
				if outProtected {
					if err != nil {
						outHold.temporary = false
					}
					err = h.mergeObjectErrors(err, h.releaseMethodHold(outHold))
				}
			} else {
				err = h.mergeObjectErrors(err, h.releaseMethodHold(hold))
			}
		}
		if err != nil {
			out = value{}
		}
	}()
	switch typ {
	case "void":
		return nullValue(), nil
	case "never":
		return value{}, &Throwable{"Error", m.owner.name + "::" + methodDisplayName(m) + "(): never-returning method must not return"}
	}
	if m.returnsReference && v.kind == valueReference && len(h.typedSources[v.ref]) != 0 {
		return value{}, methodReturnTypeError(m, v)
	}
	if coerced, ok, err := h.coerceTypedValueAfterCallable(m, called, typ, v, strict); err != nil {
		if objectPHPException(err) {
			return value{}, h.mergeObjectErrors(err, methodReturnTypeError(m, v))
		}
		return value{}, err
	} else if ok {
		if m.returnsReference && v.kind == valueReference {
			if err := h.assignPHP(&v, coerced, strict); err != nil {
				return value{}, err
			}
			// This branch updates the same wrapper and transfers its raw hold.
			consume = false
			return v, nil
		}
		return coerced, nil
	}
	return value{}, methodReturnTypeError(m, v)
}

func methodReturnTypeError(m *methodSpec, v value) *Throwable {
	return &Throwable{"TypeError", fmt.Sprintf("%s::%s(): Return value must be of type %s, %s returned", m.owner.name, methodDisplayName(m), m.returnType, valueName(v))}
}

func methodDisplayName(m *methodSpec) string {
	if m.propertyGetter {
		return "$" + m.name + "::get"
	}
	return m.name
}

// Invalid and coerced-away callback results obey the same temporary contract
// with or without an execution frame.
func (h *heap) discardMethodReturn(v value) error {
	return h.discard(v)
}

func methodAcceptsType(m *methodSpec, called *class, typ string, v value) bool {
	v = v.deref()
	typ = strings.TrimSpace(typ)
	if typ == "" || typ == "mixed" {
		return true
	}
	if strings.HasPrefix(typ, "?") {
		if v.kind == valueNull || v.kind == valueUndefined {
			return true
		}
		return methodAcceptsType(m, called, strings.TrimSpace(typ[1:]), v)
	}
	for part := range splitMethodType(typ, '|') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") && len(part) > 1 {
			part = strings.TrimSpace(part[1 : len(part)-1])
		}
		intersection := splitMethodType(part, '&')
		matches := true
		for name := range intersection {
			if !methodAcceptsPart(m, called, strings.TrimSpace(name), v) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// Yield depth-zero parts directly, without allocating temporary token slices.
func splitMethodType(typ string, sep byte) iter.Seq[string] {
	return func(yield func(string) bool) {
		start, depth := 0, 0
		for i := 0; i < len(typ); i++ {
			switch typ[i] {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case sep:
				if depth == 0 {
					if !yield(typ[start:i]) {
						return
					}
					start = i + 1
				}
			}
		}
		yield(typ[start:])
	}
}

func methodAcceptsPart(m *methodSpec, called *class, part string, v value) bool {
	part = strings.TrimPrefix(part, "\\")
	switch part {
	case "mixed":
		return true
	case "null":
		return v.kind == valueNull || v.kind == valueUndefined
	case "bool":
		return v.kind == valueBool
	case "true":
		return v.kind == valueBool && v.boolean
	case "false":
		return v.kind == valueBool && !v.boolean
	case "int":
		return v.kind == valueInt
	case "float":
		return v.kind == valueFloat
	case "string":
		return v.kind == valueString
	case "array":
		return v.kind == valueArray
	case "object":
		return v.kind == valueObject
	case "resource":
		return v.kind == valueResource
	case "iterable":
		if v.kind == valueArray {
			return true
		}
		return v.kind == valueObject && classMatchesAnyName(v.object.class, "Traversable", "Iterator", "IteratorAggregate")
	case "callable":
		// Callable validity requires request-local symbol and scope resolution.
		return false
	case "self":
		return v.kind == valueObject && classIs(v.object.class, m.owner)
	case "parent":
		if m.owner == nil || m.owner.schema == nil || m.owner.schema.parent == nil {
			return false
		}
		return v.kind == valueObject && classIs(v.object.class, m.owner.schema.parent)
	case "static":
		target := called
		if target == nil {
			target = m.owner
		}
		return v.kind == valueObject && classIs(v.object.class, target)
	case "void", "never":
		return false
	default:
		return v.kind == valueObject && classMatchesAnyName(v.object.class, part)
	}
}

func classMatchesAnyName(c *class, names ...string) bool {
	return len(names) != 0 && classReachable(c, nil, names)
}

func methodTypeHas(typ, want string) bool {
	typ = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(typ), "?"))
	if typ == "mixed" {
		return true
	}
	for part := range splitMethodType(typ, '|') {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") && len(part) > 1 {
			part = strings.TrimSpace(part[1 : len(part)-1])
		}
		for name := range splitMethodType(part, '&') {
			if strings.TrimSpace(name) == want {
				return true
			}
		}
	}
	return false
}

// coerceMethodValue follows PHP's weak scalar conversion order for parameters
// and return values. int -> float widening is allowed even under strict_types.
func (h *heap) coerceMethodValue(m *methodSpec, called *class, typ string, v value, strict bool) (value, bool) {
	v = v.deref()
	if methodAcceptsType(m, called, typ, v) {
		return v, true
	}
	if v.kind == valueInt && methodTypeHas(typ, "float") {
		return floatValue(float64(v.integer)), true
	}
	if strict {
		return value{}, false
	}
	if v.kind == valueNull || v.kind == valueUndefined {
		if methodTypeHas(typ, "null") || strings.HasPrefix(typ, "?") {
			return nullValue(), true
		}
		return value{}, false
	}
	switch v.kind {
	case valueArray, valueObject, valueResource, valueReference:
		return value{}, false
	}
	hasInt := methodTypeHas(typ, "int")
	hasFloat := methodTypeHas(typ, "float")
	hasString := methodTypeHas(typ, "string")
	hasBool := methodTypeHas(typ, "bool")

	if v.kind == valueString && (hasInt || hasFloat) {
		n := php8512NumericString(v.text, h.width, false)
		if n.kind != notNumeric {
			if hasFloat && (n.kind == numericFloat || !hasInt) {
				if n.kind == numericInt {
					return floatValue(float64(n.integer)), true
				}
				return floatValue(n.floating), true
			}
			if hasInt {
				if n.kind == numericInt {
					return h.number(n.integer), true
				}
				if !math.IsNaN(n.floating) && floatFitsInt(n.floating, h.width) {
					i := h.floatToIntCap(n.floating)
					if float64(i) != n.floating {
						h.warn(levelDeprecated, "Implicit conversion from float-string \""+cString(v.text)+"\" to int loses precision")
					}
					return h.number(i), true
				}
			}
		}
	}
	if hasInt {
		switch v.kind {
		case valueBool:
			if v.boolean {
				return h.number(1), true
			}
			return h.number(0), true
		case valueFloat:
			if !math.IsNaN(v.floating) && floatFitsInt(v.floating, h.width) {
				return h.number(h.floatToIntSafe(v.floating)), true
			}
		}
	}
	if hasFloat {
		switch v.kind {
		case valueBool:
			if v.boolean {
				return floatValue(1), true
			}
			return floatValue(0), true
		}
	}
	if hasString {
		switch v.kind {
		case valueBool:
			if v.boolean {
				return bytesValue([]byte("1")), true
			}
			return bytesValue(nil), true
		case valueInt:
			return bytesValue([]byte(strconv.FormatInt(v.integer, 10))), true
		case valueFloat:
			if math.IsNaN(v.floating) {
				h.warn(levelWarning, "unexpected NAN value was coerced to string")
			}
			return bytesValue([]byte(php8512FormatFloat(v.floating, 14))), true
		}
	}
	if hasBool {
		switch v.kind {
		case valueBool:
			return v, true
		case valueInt, valueFloat, valueString:
			return boolValue(h.toBool(v)), true
		}
	}
	return value{}, false
}

// Object coercion can execute PHP and throw, unlike scalar coercion.
func (h *heap) coerceTypedValue(m *methodSpec, called *class, typ string, v value, strict bool) (value, bool, error) {
	if err := h.claimValues(v); err != nil {
		return value{}, false, err
	}
	if methodAcceptsType(m, called, typ, v) ||
		(methodTypeHas(typ, "callable") && h.isCallable(v, callableContext{m.owner, called, !m.static && m.owner != nil})) {
		return v, true, nil
	}
	return h.coerceTypedValueAfterCallable(m, called, typ, v, strict)
}

// Both callers have claimed the value and performed callable validation once.
// Scalar/Stringable coercion must not repeat an observable deprecation.
func (h *heap) coerceTypedValueAfterCallable(m *methodSpec, called *class, typ string, v value, strict bool) (value, bool, error) {
	if out, ok := h.coerceMethodValue(m, called, typ, v, strict); ok {
		return out, true, nil
	}
	if !strict && v.deref().kind == valueObject && methodTypeHas(typ, "string") {
		if _, ok := h.resolveMethod(v.deref().object.class, "__toString", nil); ok {
			s, err := h.toString(v)
			return stringValue(s), err == nil, err
		}
	}
	return value{}, false, nil
}

type callableContext struct {
	scope, called *class
	hasThis       bool
}

func (h *heap) isObjectCallable(v value, scope *class) bool {
	return h.isCallable(v, callableContext{scope: scope, called: scope})
}

func (h *heap) isCallable(v value, context callableContext) bool {
	scope := context.scope
	v = v.deref()
	var target *class
	name := "__invoke"
	requireStatic := false
	strictClass, classArray := false, false
	className := func(name string) *class {
		strictClass = true
		var c *class
		relative := phpNameLower(name)
		switch relative {
		case "self":
			c = scope
			strictClass = false
		case "parent":
			if scope != nil && scope.schema != nil {
				c = scope.schema.parent
			}
		case "static":
			c = context.called
		default:
			c = h.classes[phpNameLower(strings.TrimPrefix(name, "\\"))]
			if c != nil && context.hasThis && classIs(context.called, scope) && classIs(scope, c) {
				requireStatic = false
			}
			return c
		}
		if c != nil {
			h.warn(levelDeprecated, "Use of \""+relative+"\" in callables is deprecated")
			if context.hasThis {
				requireStatic = false
			}
		}
		return c
	}
	switch v.kind {
	case valueObject:
		if v.object == nil || v.object.owner != h || v.object.freed {
			return false
		}
		// A bare object uses its invocation handler, independently of caller
		// visibility. Only explicit method callables can fall back to __call.
		if v.object.class == nil || v.object.class.schema == nil {
			return false
		}
		method := v.object.class.schema.methods["__invoke"]
		return method != nil && !method.abstract && method.call != nil
	case valueString:
		c, method, ok := strings.Cut(v.text, "::")
		if !ok {
			return false
		}
		requireStatic = true
		target = className(c)
		name = method
	case valueArray:
		if v.array == nil || v.array.count() != 2 {
			return false
		}
		first, ok := v.array.lookup(intKey(0))
		if !ok {
			return false
		}
		second, ok := v.array.lookup(intKey(1))
		if !ok || second.val.deref().kind != valueString {
			return false
		}
		name = second.val.deref().text
		switch obj := first.val.deref(); obj.kind {
		case valueObject:
			if obj.object == nil || obj.object.owner != h || obj.object.freed {
				return false
			}
			target = obj.object.class
		case valueString:
			requireStatic = true
			classArray = true
			target = className(obj.text)
		default:
			return false
		}
	default:
		return false
	}
	if target == nil {
		return false
	}
	lookupScope := scope
	if strictClass {
		lookupScope = nil
	}
	method, found := h.resolveMethod(target, name, lookupScope)
	if !found && classArray && !requireStatic {
		// PHP array class callables provide ce_org; compound strings do not.
		method, found = h.resolveMethod(context.called, name, scope)
		if found && strictClass && !classIs(target, method.owner) {
			found = false
		}
	}
	if found && canAccess(scope, method.owner, method.visibility) {
		return !method.abstract && method.call != nil && (!requireStatic || method.static)
	}
	magic := "__call"
	if requireStatic {
		magic = "__callStatic"
	}
	method, ok := h.resolveMethod(target, magic, nil)
	return ok && !method.abstract && method.call != nil
}

// Static property storage is request-local and keyed by the declaring class so
// inherited statics are shared while a redeclared child static is distinct.
func (h *heap) readStatic(c *class, name string, scope *class) (value, error) {
	if h.closed {
		return value{}, ErrHeapClosed
	}
	p, err := h.resolveStatic(c, name, scope)
	if err != nil {
		return value{}, err
	}
	tbl, err := h.staticTable(p.owner)
	if err != nil {
		return value{}, err
	}
	slot, _ := tbl.lookup(p.key)
	if slot == nil || slot.kind == valueUndefined {
		if p.typeName != "" {
			return value{}, &Throwable{"Error", "Typed static property " + p.label() + " must not be accessed before initialization"}
		}
		slot = tbl.slot(p.key)
		*slot = nullValue()
	}
	return slot.deref(), nil
}

func (h *heap) writeStatic(c *class, name string, v value, scope *class, strict bool) error {
	if h.closed {
		return ErrHeapClosed
	}
	p, err := h.resolveStatic(c, name, scope)
	if err != nil {
		return err
	}
	if !canAccess(scope, p.owner, p.setVisibility) {
		return &Throwable{"Error", "Cannot modify " + accessName(p.setVisibility) + "(set) property " + p.label() + " from " + h.calledScopeLabel(scope)}
	}
	tbl, err := h.staticTable(p.owner)
	if err != nil {
		return err
	}
	coerced, err := h.coerceProperty(p, v.deref(), strict, false)
	if err != nil {
		return err
	}
	slot, _ := tbl.lookup(p.key)
	if slot == nil {
		slot = tbl.slot(p.key)
	}
	return h.assignPHP(slot, coerced, strict)
}

func (h *heap) resolveStatic(c *class, name string, scope *class) (*propertyInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("class is required")
	}
	p, err := resolveProperty(c, name, scope)
	if err != nil {
		return nil, err
	}
	if p == nil || !p.static {
		return nil, &Throwable{"Error", "Access to undeclared static property " + c.name + "::$" + name}
	}
	return p, nil
}

// staticTable builds all static slots declared by owner on first use. Defaults
// are imported per request, and a failed import leaves no half-built table.
func (h *heap) staticTable(owner *class) (*propertyTable, error) {
	if h.classStatics == nil {
		h.classStatics = make(map[*class]*propertyTable)
	}
	if tbl := h.classStatics[owner]; tbl != nil {
		return tbl, nil
	}
	tbl := newPropertyTable(nil)
	for i := range owner.schema.props {
		p := &owner.schema.props[i]
		if !p.static || p.owner != owner {
			continue
		}
		slot := tbl.slot(p.key)
		switch {
		case p.hasDefault:
			v, err := h.importSnapshotPHP8512(p.initial, 1024, 1<<20)
			if err != nil {
				releaseStaticTable(h, tbl)
				return nil, err
			}
			*slot = v
		case p.typeName == "":
			*slot = nullValue()
		default:
			// A typed static without a default starts uninitialized. slot()
			// creates the location as null, so restore the undefined state.
			*slot = value{}
		}
	}
	h.classStatics[owner] = tbl
	return tbl, nil
}

func releaseStaticTable(h *heap, tbl *propertyTable) {
	tbl.each(func(_ string, slot *value) {
		if slot.kind != valueUndefined {
			h.detachPropertySource(slot)
			old := *slot
			*slot = value{}
			_ = h.release(old)
		}
	})
}
