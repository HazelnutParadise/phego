package runtime

import (
	"errors"
	"strconv"
	"strings"
)

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. Closures,
// first-class callables and the methods of the Closure class.
//
// A closure's body and its use list are immutable data of the Program. The
// Closure object and everything it holds (closureData) belong to the run
// that created it: the bound object and classes, the captured variables and
// the closure's own static variables. The object's owners are counted like
// any other object's, and the cycle collector follows what it holds, so
// destructors run when PHP runs them.

// closureData is what a Closure object holds (zend_closure).
type closureData struct {
	fn *vmFunction
	// this is the bound object (an owner) or nil; scope the class the code
	// runs in and called the class of late static binding.
	this          *object
	scope, called *class
	// captures holds the use variables, one per entry of fn.captures: a
	// value for a capture by value, a reference for one by reference, or
	// nothing for an implicit capture of an undefined variable. statics
	// holds the closure's static variables per local, allocated on first
	// use. A fake closure owns neither: it shares the static variables of
	// the function it was made from.
	captures []value
	statics  []value
	// fake marks a closure made from a function or method (f(...),
	// Closure::fromCallable), static one that never takes an object.
	fake, static bool
	// magic is the method name a fake closure of __call or __callStatic
	// passes along; fn is then the magic method's function.
	magic string
}

// The methods of the Closure class the VM implements (methodSpec.native).
const (
	closureBind uint8 = iota + 1
	closureBindTo
	closureCall
	closureFromCallable
	closureInvoke
	// closureMagic is not a method: it marks the frame of __call or
	// __callStatic entered through a closure of a magic method
	// (vmFrame.via), which a backtrace shows as an internal call.
	closureMagic
)

// closureBuiltin is PHP's Closure class: final, without dynamic properties,
// never instantiated by new. The description is immutable and shared.
var closureBuiltin = func() *class {
	native := func(*heap, value, []value) (value, error) {
		return value{}, errors.New("a method of Closure was called outside the VM")
	}
	m := func(name string, static bool, kind uint8) methodSpec {
		return methodSpec{name: name, visibility: publicAccess, static: static, native: kind, call: native}
	}
	c, err := buildClass(classSpec{name: "Closure", final: true, noDynamic: true, methods: []methodSpec{
		m("bind", true, closureBind), m("bindTo", false, closureBindTo), m("call", false, closureCall),
		m("fromCallable", true, closureFromCallable), m("__invoke", false, closureInvoke),
	}})
	if err != nil {
		panic(err)
	}
	return c
}()

// builtinClass reports a class the engine provides (ZEND_INTERNAL_CLASS).
func builtinClass(c *class) bool {
	return c == closureBuiltin || c == stdClassBuiltin || c == stringableBuiltin
}

// freeClosureStorage releases what a Closure object holds
// (zend_closure_free_storage): its variables, then the bound object.
func (h *heap) freeClosureStorage(o *object, dead map[gcNode]bool) error {
	cd := o.closure
	if cd == nil {
		return nil
	}
	o.closure = nil
	var first error
	for _, vars := range [2][]value{cd.captures, cd.statics} {
		for i, v := range vars {
			vars[i] = value{}
			first = h.mergeObjectErrors(first, h.releaseOutside(v, dead))
		}
	}
	if this := cd.this; this != nil {
		cd.this = nil
		first = h.mergeObjectErrors(first, h.releaseOutside(value{kind: valueObject, object: this}, dead))
	}
	return first
}

// newClosure makes the Closure object that holds cd, which it takes over:
// the object's own closureData (one allocation with the object, see
// newObjectShell) receives it. The result has no owner yet.
func (s *vmState) newClosure(cd closureData) (value, error) {
	if s.h.classes[closureKey] != closureBuiltin {
		if err := s.h.registerClass(closureBuiltin); err != nil {
			return value{}, err
		}
	}
	v, err := s.h.newObjectShell(closureBuiltin)
	if err != nil {
		return value{}, err
	}
	*v.object.closure = cd
	return v, nil
}

const closureKey = "closure"

// makeClosure is ZEND_DECLARE_LAMBDA_FUNCTION: the closure takes the
// frame's class scope, its called class and, unless the closure is static,
// its object.
func (s *vmState) makeClosure(f *vmFrame, in *vmInstr) error {
	fn := &s.prog.funcs[in.b]
	if !fn.closure {
		return s.internal("closure of function %d, which is not a closure function", in.b)
	}
	cd := closureData{fn: fn, scope: f.scope, called: f.called, static: fn.noThis}
	if f.this != nil {
		cd.called = f.this.class
		if !fn.noThis && cd.scope != nil {
			f.this.refcount++
			cd.this = f.this
		}
	}
	if n := len(fn.captures); n != 0 {
		cd.captures = make([]value, n)
	}
	v, err := s.newClosure(cd)
	if err != nil {
		if cd.this != nil {
			_ = s.releaseObject(cd.this)
		}
		return err
	}
	return s.setReg(f.regs, in.a, v)
}

// bindLexical is ZEND_BIND_LEXICAL: variable B becomes capture Aux of the
// closure in A. A capture by reference turns the variable into a reference
// the closure shares; a capture by value copies the variable's value, with
// PHP's warning for an undefined one unless the capture is implicit.
func (s *vmState) bindLexical(f *vmFrame, in *vmInstr) error {
	cl := f.regs[in.a]
	if cl.kind != valueObject || cl.object.closure == nil || cl.object.closure.fn != &s.prog.funcs[in.c] || int(in.aux) >= len(cl.object.closure.captures) {
		return s.internal("bind-lexical on a value that is not the closure of function %d", in.c)
	}
	cd := cl.object.closure
	c := cd.fn.captures[in.aux]
	src, err := s.slot(f, in.b)
	if err != nil {
		return err
	}
	var v value
	switch {
	case c.byRef:
		nullIfUndefined(src)
		if err := s.h.bind(&v, src); err != nil {
			return err
		}
	case src.deref().kind == valueUndefined:
		if !c.implicit {
			s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, in.b))
			if s.stopped() {
				return nil
			}
			v = nullValue()
		}
	default:
		v = src.deref()
		retain(v)
	}
	old := cd.captures[in.aux]
	cd.captures[in.aux] = v
	return s.releaseValue(old)
}

// bindCaptures gives a closure's frame its use variables (ZEND_BIND_STATIC
// without an initializer): a copy of a captured value, or the captured
// reference itself.
func (s *vmState) bindCaptures(fr *vmFrame, cd *closureData) {
	if cd.fake || len(cd.captures) != len(fr.fn.captures) {
		return
	}
	for i, c := range fr.fn.captures {
		v := cd.captures[i]
		if v.kind == valueUndefined {
			continue
		}
		if v.kind >= valueArray {
			retain(v)
		}
		fr.locals[c.local] = v
	}
}

// dropInto releases what a push that failed before it made its frame left
// behind, and clears the hand-over.
func (s *vmState) dropInto() {
	if s.into.this != nil {
		_ = s.releaseObject(s.into.this)
	}
	if s.into.closure != nil {
		_ = s.releaseObject(s.into.closure)
	}
	s.into = vmEnter{}
}

// dupVars is zend_array_dup on a closure's variables: every value gets one
// more owner, and a reference nothing else holds is replaced by its value.
func dupVars(src []value) []value {
	if src == nil {
		return nil
	}
	out := make([]value, len(src))
	for i, v := range src {
		if v.kind == valueReference && v.ref.refcount == 1 {
			v = v.ref.target
		}
		if v.kind >= valueArray {
			retain(v)
		}
		out[i] = v
	}
	return out
}

// dupClosure is zend_create_closure on an existing closure's function: a
// new closure with the given scope, called class and object, and its own
// copy of the source's variables. A bound object needs a scope; without
// one the Closure class stands in.
func (s *vmState) dupClosure(src *closureData, scope, called *class, this *object) (value, error) {
	cd := closureData{fn: src.fn, fake: src.fake, static: src.static, magic: src.magic, scope: scope, called: called}
	if scope == nil && this != nil {
		if err := s.h.registerClass(closureBuiltin); err != nil {
			return value{}, err
		}
		cd.scope = closureBuiltin
	}
	if cd.scope != nil && this != nil && !src.static {
		this.refcount++
		cd.this = this
	}
	if !src.fake {
		cd.captures, cd.statics = dupVars(src.captures), dupVars(src.statics)
	}
	v, err := s.newClosure(cd)
	if err != nil {
		_ = s.h.freeClosureStorage(&object{closure: &cd}, nil)
		return value{}, err
	}
	return v, nil
}

// --- callables ---

// vmCallTarget is a resolved callable: what a call of it enters. this and
// closure are borrowed.
type vmCallTarget struct {
	fn            int
	this          *object
	scope, called *class
	closure       *object
	magic         string
	magicFn       int
	native        uint8
	via           uint8
}

// startTarget records the call of t, as an INIT instruction leaves a call
// frame.
func (s *vmState) startTarget(t vmCallTarget) {
	rec := vmPendingCall{fn: t.fn, base: len(s.args), depth: len(s.frames), scope: t.scope, called: t.called, magic: t.magic, magicFn: t.magicFn, native: t.native, via: t.via}
	if t.this != nil {
		t.this.refcount++
		rec.this = t.this
	}
	if t.closure != nil {
		t.closure.refcount++
		rec.closure = t.closure
	}
	s.calls = append(s.calls, rec)
}

// closureTarget is the call of a Closure object: its function with its
// binding. The call holds the closure until it returns.
func (s *vmState) closureTarget(o *object) vmCallTarget {
	cd := o.closure
	t := vmCallTarget{fn: cd.fn.index, this: cd.this, scope: cd.scope, called: cd.called, closure: o}
	if cd.magic != "" {
		t.fn, t.magic, t.magicFn, t.via = s.prog.trampoline, cd.magic, cd.fn.index, closureMagic
	}
	return t
}

// startNative records the call of a method of the Closure class.
func (s *vmState) startNative(m *methodSpec, this value) error {
	if m.native == closureInvoke && this.kind == valueObject && this.object.closure != nil {
		// $closure->__invoke(...) is the closure's own call.
		s.startTarget(s.closureTarget(this.object))
		s.calls[len(s.calls)-1].via = closureInvoke
		return nil
	}
	t := vmCallTarget{fn: s.prog.trampoline, native: m.native}
	if this.kind == valueObject {
		t.this = this.object
	}
	call := m.native == closureCall && t.this != nil && t.this.closure != nil && t.this.closure.magic == ""
	if call {
		// Named arguments are matched against the closure's parameters,
		// which follow the object.
		t.fn = t.this.closure.fn.index
	}
	s.startTarget(t)
	if call {
		s.calls[len(s.calls)-1].shift = 1
	}
	return nil
}

// callClosureValue is CallValue on a Closure object: a positional call.
func (s *vmState) callClosureValue(o *object, cd *closureData, argc int, dest uint32) error {
	if cd.magic != "" {
		return s.unsupported("calling a closure of a magic method without a started call")
	}
	if argc > len(s.args) {
		return s.internal("call with %d arguments but %d pending", argc, len(s.args))
	}
	if cd.fn.paramByRef != nil {
		if err := s.derefArguments(cd.fn, argc); err != nil {
			return err
		}
	}
	o.refcount++
	s.into = vmEnter{scope: cd.scope, called: cd.called, closure: o}
	if cd.this != nil {
		cd.this.refcount++
		s.into.this = cd.this
	}
	err := s.push(cd.fn, argc, dest)
	s.dropInto()
	return err
}

// callableError is why a value is not callable, in zend_is_callable_ex's
// words.
type callableError struct{ reason string }

func (e *callableError) Error() string { return e.reason }

// resolveCallable finds what calling v enters. dynamic selects the errors
// of ZEND_INIT_DYNAMIC_CALL, thrown as Error; otherwise a value that is not
// callable gives a *callableError worded as zend_is_callable_ex does.
func (s *vmState) resolveCallable(f *vmFrame, v value, dynamic bool) (vmCallTarget, error) {
	v = v.deref()
	fail := func(dyn, reason string) (vmCallTarget, error) {
		if dynamic {
			return vmCallTarget{}, &Throwable{"Error", dyn}
		}
		return vmCallTarget{}, &callableError{reason}
	}
	switch v.kind {
	case valueObject:
		o := v.object
		if o.closure != nil {
			return s.closureTarget(o), nil
		}
		m, ok := s.h.resolveMethod(o.class, "__invoke", nil)
		if !ok || m == nil {
			return fail("Object of type "+o.class.name+" is not callable", "no array or string given")
		}
		if m.fn == 0 || m.call == nil {
			return vmCallTarget{}, s.unsupported("calling a method of a builtin class")
		}
		return vmCallTarget{fn: int(m.fn - 1), this: o, scope: m.owner, called: o.class}, nil
	case valueString:
		class, method, isMethod := strings.Cut(v.text, "::")
		if !isMethod {
			name := strings.TrimPrefix(v.text, "\\")
			idx, ok := s.prog.byName[phpNameLower(name)]
			if !ok {
				return fail("Call to undefined function "+name+"()", "function \""+v.text+"\" not found or invalid function name")
			}
			return vmCallTarget{fn: idx}, nil
		}
		c := s.lookupClass(class)
		if c == nil {
			return fail("Class \""+strings.TrimPrefix(class, "\\")+"\" not found", "class \""+strings.TrimPrefix(class, "\\")+"\" not found")
		}
		return s.staticTarget(f, c, method, nil, dynamic)
	case valueArray:
		a := v.array
		var first, second value
		ok := a.count() == 2
		if ok {
			b0, ok0 := a.lookup(intKey(0))
			b1, ok1 := a.lookup(intKey(1))
			if ok = ok0 && ok1; ok {
				first, second = b0.val.deref(), b1.val.deref()
			}
		}
		if !ok {
			return fail("Array callback must have exactly two elements", "array callback must have exactly two members")
		}
		if first.kind != valueObject && first.kind != valueString {
			return fail("First array member is not a valid class name or object", "first array member is not a valid class name or object")
		}
		if second.kind != valueString {
			return fail("Second array member is not a valid method", "second array member is not a valid method")
		}
		if first.kind == valueString {
			c := s.lookupClass(first.text)
			if c == nil {
				return fail("Class \""+strings.TrimPrefix(first.text, "\\")+"\" not found", "class \""+strings.TrimPrefix(first.text, "\\")+"\" not found")
			}
			return s.staticTarget(f, c, second.text, nil, dynamic)
		}
		return s.instanceTarget(f, first.object, second.text, dynamic)
	}
	if v.kind == valueUndefined {
		v = nullValue()
	}
	return fail("Value of type "+valueName(v)+" is not callable", "no array or string given")
}

// instanceTarget is the method name of object o as a callable sees it
// (the object's get_method handler): the method, or __call.
func (s *vmState) instanceTarget(f *vmFrame, o *object, name string, dynamic bool) (vmCallTarget, error) {
	c := o.class
	if o.closure != nil && phpNameEqual(name, "__invoke") {
		return s.closureTarget(o), nil
	}
	m, found := s.h.resolveMethod(c, name, f.scope)
	if found && m != nil && methodAccessible(f.scope, m) {
		if m.native != 0 {
			return vmCallTarget{}, s.unsupported("a callable that names a method of the Closure class")
		}
		if m.abstract || m.call == nil {
			return vmCallTarget{}, &Throwable{"Error", "Cannot call abstract method " + m.owner.name + "::" + m.name + "()"}
		}
		if m.fn == 0 {
			return vmCallTarget{}, s.unsupported("calling a method of a builtin class")
		}
		t := vmCallTarget{fn: int(m.fn - 1), scope: m.owner, called: c}
		if !m.static {
			t.this = o
		}
		return t, nil
	}
	if magic, ok := s.h.resolveMethod(c, "__call", nil); ok && magic != nil && !magic.static && magic.fn != 0 {
		return vmCallTarget{fn: s.prog.trampoline, this: o, scope: magic.owner, called: c, magic: name, magicFn: int(magic.fn - 1)}, nil
	}
	if found && m != nil {
		if dynamic {
			return vmCallTarget{}, &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + m.owner.name + "::" + m.name + "() from " + methodScopeLabel(f.scope)}
		}
		return vmCallTarget{}, &callableError{"cannot access " + accessName(m.visibility) + " method " + c.name + "::" + m.name + "()"}
	}
	if dynamic {
		return vmCallTarget{}, &Throwable{"Error", "Call to undefined method " + c.name + "::" + name + "()"}
	}
	return vmCallTarget{}, &callableError{"class " + c.name + " does not have a method \"" + name + "\""}
}

// staticTarget is method name of class c as a callable names it
// ("A::m", ["A", "m"]): a static method, or __callStatic. A non-static
// method is not callable this way.
func (s *vmState) staticTarget(f *vmFrame, c *class, name string, _ *object, dynamic bool) (vmCallTarget, error) {
	var m *methodSpec
	if c.schema != nil {
		m = c.schema.methods[phpNameLower(name)]
	}
	if m != nil && methodAccessible(f.scope, m) {
		if m.native != 0 {
			return vmCallTarget{}, s.unsupported("a callable that names a method of the Closure class")
		}
		if !m.static {
			if dynamic {
				return vmCallTarget{}, &Throwable{"Error", "Non-static method " + m.owner.name + "::" + m.name + "() cannot be called statically"}
			}
			return vmCallTarget{}, &callableError{"non-static method " + m.owner.name + "::" + m.name + "() cannot be called statically"}
		}
		if m.abstract || m.call == nil {
			return vmCallTarget{}, &Throwable{"Error", "Cannot call abstract method " + m.owner.name + "::" + m.name + "()"}
		}
		if m.fn == 0 {
			return vmCallTarget{}, s.unsupported("calling a method of a builtin class")
		}
		return vmCallTarget{fn: int(m.fn - 1), scope: m.owner, called: c}, nil
	}
	if magic, ok := s.h.resolveMethod(c, "__callStatic", nil); ok && magic != nil && magic.static && magic.fn != 0 {
		return vmCallTarget{fn: s.prog.trampoline, scope: magic.owner, called: c, magic: name, magicFn: int(magic.fn - 1)}, nil
	}
	if m != nil {
		if dynamic {
			return vmCallTarget{}, &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + c.name + "::" + m.name + "() from " + methodScopeLabel(f.scope)}
		}
		return vmCallTarget{}, &callableError{"cannot access " + accessName(m.visibility) + " method " + c.name + "::" + m.name + "()"}
	}
	if dynamic {
		return vmCallTarget{}, &Throwable{"Error", "Call to undefined method " + c.name + "::" + name + "()"}
	}
	return vmCallTarget{}, &callableError{"class " + c.name + " does not have a method \"" + name + "\""}
}

// fakeClosure is zend_create_fake_closure for a resolved callable: a
// closure of the function or method itself.
func (s *vmState) fakeClosure(t vmCallTarget) (value, error) {
	if t.closure != nil && t.magic == "" {
		return value{kind: valueObject, object: t.closure}, nil
	}
	if t.native != 0 {
		return value{}, s.unsupported("a closure of a method of the Closure class")
	}
	cd := closureData{fn: &s.prog.funcs[t.fn], fake: true, scope: t.scope, called: t.called, static: t.this == nil && t.scope != nil}
	if t.magic != "" {
		cd.fn, cd.magic = &s.prog.funcs[t.magicFn], t.magic
	}
	if t.this != nil && cd.scope != nil {
		t.this.refcount++
		cd.this = t.this
	}
	v, err := s.newClosure(cd)
	if err != nil && cd.this != nil {
		_ = s.releaseObject(cd.this)
	}
	return v, err
}

// popCall removes the innermost pending call and releases what it holds.
func (s *vmState) popCall() error {
	n := len(s.calls) - 1
	rec := s.calls[n]
	s.calls[n] = vmPendingCall{}
	s.calls = s.calls[:n]
	err := s.releaseValue(rec.named)
	if rec.this != nil {
		if rerr := s.releaseObject(rec.this); err == nil {
			err = rerr
		}
	}
	if rec.closure != nil {
		if rerr := s.releaseObject(rec.closure); err == nil {
			err = rerr
		}
	}
	return err
}

// callableConvert is ZEND_CALLABLE_CONVERT: the call the preceding INIT
// started becomes a Closure instead of being made. A started call of a
// closure gives back that closure.
func (s *vmState) callableConvert(f *vmFrame, in *vmInstr) error {
	rec, err := s.pendingCall()
	if err != nil {
		return err
	}
	if rec.pass || len(s.args) != rec.base {
		return s.internal("callable-convert on a call with arguments")
	}
	t := vmCallTarget{fn: rec.fn, this: rec.this, scope: rec.scope, called: rec.called, closure: rec.closure, magic: rec.magic, magicFn: rec.magicFn, native: rec.native}
	v, err := s.fakeClosure(t)
	if err == nil {
		err = s.setReg(f.regs, in.a, v)
	}
	if perr := s.popCall(); err == nil {
		err = perr
	}
	return err
}

// --- the methods of the Closure class ---

// nativeThrow is a throwable raised inside a method of the Closure class:
// the backtrace gets a frame for the method, "Closure::name" for a static
// one and "Closure->name" for a call on a closure.
func nativeThrow(name string, args []value, class, message string) error {
	frame := "Closure::" + name
	if name == "bindTo" || name == "call" {
		frame = "Closure->" + name
	}
	return &vmInternalFrameThrow{function: frame, args: append([]value(nil), args...), thrown: &Throwable{class, message}}
}

// callNative runs the method of the Closure class the pending call names.
// It reports enter when the call turned into a closure's call that
// callPending goes on to make (Closure::call); otherwise the result is in
// the destination register and the call is gone.
func (s *vmState) callNative(rec *vmPendingCall, in *vmInstr) (enter bool, err error) {
	f := s.frames[len(s.frames)-1]
	args := s.args[rec.base:]
	for i := range args {
		// The methods take their arguments by value.
		if args[i].kind == valueReference {
			t := args[i].ref.target
			retain(t)
			old := args[i]
			args[i] = t
			if err := s.releaseValue(old); err != nil {
				return false, err
			}
		}
	}
	name, min, max := "", 0, 0
	switch rec.native {
	case closureBind:
		name, min, max = "bind", 2, 3
	case closureBindTo:
		name, min, max = "bindTo", 1, 2
	case closureFromCallable:
		name, min, max = "fromCallable", 1, 1
	case closureCall:
		name, min, max = "call", 1, -1
	default:
		return false, s.internal("unknown native method %d", rec.native)
	}
	if rec.named.kind == valueArray && rec.native != closureCall {
		return false, s.unsupported("named arguments to Closure::" + name + "()")
	}
	if len(args) > 0 && args[0].kind == valueUndefined {
		return false, s.unsupported("Closure::" + name + "() without its first argument")
	}
	if len(args) < min || max >= 0 && len(args) > max {
		bound, n := "exactly", min
		switch {
		case min != max && len(args) < min:
			bound = "at least"
		case min != max:
			bound, n = "at most", max
		}
		plural := "s"
		if n == 1 {
			plural = ""
		}
		return false, nativeThrow(name, args, "ArgumentCountError", "Closure::"+name+"() expects "+bound+" "+strconv.Itoa(n)+" argument"+plural+", "+strconv.Itoa(len(args))+" given")
	}
	typeError := func(pos int, param, want string) error {
		return nativeThrow(name, args, "TypeError", "Closure::"+name+"(): Argument #"+strconv.Itoa(pos+1)+" ($"+param+") must be of type "+want+", "+valueName(args[pos])+" given")
	}
	var out value
	switch rec.native {
	case closureFromCallable:
		if out, err = s.fromCallable(f, args[0], args); err != nil {
			return false, err
		}
	case closureBind, closureBindTo:
		var cl *object
		rest := args
		if rec.native == closureBind {
			if args[0].kind != valueObject || args[0].object.closure == nil {
				return false, typeError(0, "closure", "Closure")
			}
			cl, rest = args[0].object, args[1:]
		} else {
			cl = rec.this
		}
		shift := len(args) - len(rest)
		if rest[0].kind != valueObject && rest[0].kind != valueNull {
			return false, typeError(shift, "newThis", "?object")
		}
		var newThis *object
		if rest[0].kind == valueObject {
			newThis = rest[0].object
		}
		scope, ok := cl.closure.scope, true
		if len(rest) > 1 {
			switch sv := rest[1]; sv.kind {
			case valueObject:
				scope = sv.object.class
			case valueNull:
				scope = nil
			case valueString:
				if sv.text != "static" {
					if scope = s.lookupClass(sv.text); scope == nil {
						s.h.warn(levelWarning, "Class \""+sv.text+"\" not found")
						ok = false
					}
				}
			default:
				return false, typeError(shift+1, "newScope", "object|string|null")
			}
		}
		out = nullValue()
		if ok && s.validBinding(cl.closure, newThis, scope) {
			called := scope
			if newThis != nil {
				called = newThis.class
			}
			if out, err = s.dupClosure(cl.closure, scope, called, newThis); err != nil {
				return false, err
			}
		}
	case closureCall:
		if args[0].kind != valueObject {
			return false, typeError(0, "newThis", "object")
		}
		cl, newThis := rec.this, args[0].object
		cd := cl.closure
		if !s.validBinding(cd, newThis, newThis.class) {
			out = nullValue()
			break
		}
		if cd.magic != "" {
			return false, s.unsupported("Closure::call() on a closure of a magic method")
		}
		// The closure's function runs with the object and its class as
		// scope, sharing the closure's own variables.
		copy(args, args[1:])
		args[len(args)-1] = value{}
		s.args = s.args[:len(s.args)-1]
		args = s.args[rec.base:]
		rec.native, rec.fn, rec.shift, rec.via = 0, cd.fn.index, 0, closureCall
		rec.this, rec.closure = newThis, cl // the first argument's owner and the method's object
		rec.scope, rec.called = newThis.class, newThis.class
		for i := range args {
			if i < len(cd.fn.paramByRef) && cd.fn.paramByRef[i] && args[i].kind != valueUndefined {
				pname := ""
				if i < len(cd.fn.paramNames) {
					pname = " ($" + cd.fn.paramNames[i] + ")"
				}
				s.h.warn(levelWarning, pendingName(cd.fn, rec.scope)+"(): Argument #"+strconv.Itoa(i+1)+pname+" must be passed by reference, value given")
				args[i] = value{kind: valueReference, ref: &reference{owner: s.h, refcount: 1, target: args[i]}}
			}
		}
		return true, nil
	}
	if s.stopped() {
		if out.kind >= valueArray {
			retain(out)
			_ = s.releaseValue(out)
		}
		return false, nil
	}
	if out.kind >= valueArray {
		retain(out) // held across the cleanup below
	}
	err = s.dropArgs(len(s.args) - rec.base)
	if perr := s.popCall(); err == nil {
		err = perr
	}
	if serr := s.setReg(f.regs, in.a, out); err == nil {
		err = serr
	}
	if out.kind >= valueArray {
		if rerr := s.releaseValue(out); err == nil {
			err = rerr
		}
	}
	return false, err
}

// validBinding is zend_valid_closure_binding: whether the closure may be
// bound to newThis and scope, with PHP's warning when it may not.
func (s *vmState) validBinding(cd *closureData, newThis *object, scope *class) bool {
	warn := func(msg string) bool {
		s.h.warn(levelWarning, msg+", this will be an error in PHP 9")
		return false
	}
	method := cd.fake && cd.scope != nil
	switch {
	case newThis != nil && cd.static:
		return warn("Cannot bind an instance to a static closure")
	case newThis != nil && method && !classIs(newThis.class, cd.scope):
		name := cd.fn.name
		if cd.magic != "" {
			name = cd.magic
		}
		return warn("Cannot bind method " + cd.scope.name + "::" + name + "() to object of class " + newThis.class.name)
	case newThis == nil && method && !cd.static:
		return warn("Cannot unbind $this of method")
	case newThis == nil && !cd.fake && cd.this != nil && cd.fn.usesThis:
		return warn("Cannot unbind $this of closure using $this")
	}
	if scope != nil && scope != cd.scope && builtinClass(scope) {
		return warn("Cannot bind closure to scope of internal class " + scope.name)
	}
	if cd.fake && scope != cd.scope {
		if cd.scope == nil {
			return warn("Cannot rebind scope of closure created from function")
		}
		return warn("Cannot rebind scope of closure created from method")
	}
	return true
}

// fromCallable is Closure::fromCallable: a closure is returned as it is,
// any other callable becomes a closure of what it names.
func (s *vmState) fromCallable(f *vmFrame, v value, args []value) (value, error) {
	if v.kind == valueObject && v.object.closure != nil {
		return v, nil
	}
	t, err := s.resolveCallable(f, v, false)
	if err != nil {
		var ce *callableError
		if errors.As(err, &ce) {
			return value{}, nativeThrow("fromCallable", args, "TypeError", "Failed to create closure from callable: "+ce.reason)
		}
		return value{}, err
	}
	return s.fakeClosure(t)
}

// compareClosures is zend_closure_compare for two different Closure
// objects: only closures made from the same function or method, with the
// same object and classes, are equal; every other pair is uncomparable.
func compareClosures(a, b *closureData) int {
	if a == nil || b == nil || !a.fake || !b.fake || a.this != b.this || a.called != b.called || a.scope != b.scope ||
		a.magic != b.magic || a.fn.name != b.fn.name {
		return 1
	}
	return 0
}

// functionExists reports whether name is a function the program declares
// (a closure's body has no name to call).
func (s *vmState) functionExists(name string) bool {
	_, ok := s.prog.byName[phpNameLower(strings.TrimPrefix(name, "\\"))]
	return ok
}
