package runtime

import (
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 7A: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. How the
// VM enters methods. A call written in PHP runs in a frame of the same
// loop, like a function call, so PHP recursion never grows the Go stack.
// Only code the runtime itself calls in the middle of an instruction (a
// destructor, a magic method, __toString, a constant's initializer) runs in
// a nested loop (invoke).

// vmEnter is what the next push gives the frame it creates beyond a plain
// function's.
type vmEnter struct {
	this          *object // an owner, which the frame takes over
	scope, called *class
	// internal marks a call the runtime makes: its arguments are converted
	// in weak mode whatever the caller's strict_types.
	internal bool
	// ctor marks a constructor's call (see vmPendingCall.ctor).
	ctor bool
}

// vmMaxNesting bounds the nested loops of one run. PHP's own bound is the C
// stack (zend.max_allowed_stack_size); the error it raises there is the one
// reported here.
const vmMaxNesting = 12000

// invoke runs fn to completion from inside an instruction and returns its
// result as an owned value. args are borrowed. An error is the completion
// that ended the nested run.
func (s *vmState) invoke(fn *vmFunction, this value, scope, called *class, args []value) (value, error) {
	if s.nesting >= vmMaxNesting {
		return value{}, &Throwable{"Error", "Maximum call stack size of 8339456 bytes (zend.max_allowed_stack_size - zend.reserved_stack_size) reached. Infinite recursion?"}
	}
	// The nesting is counted until this call's frames are gone: releasing
	// their values can run destructors, which nest in turn.
	s.nesting++
	defer func() { s.nesting-- }()
	for _, a := range args {
		a = a.deref()
		if a.kind >= valueArray {
			retain(a)
		}
		s.args = append(s.args, a)
	}
	ended, end, floor := s.ended, s.end, s.floor
	s.ended, s.end = false, completion{}
	depth := len(s.frames)
	var receiver *object
	if this.kind == valueObject {
		retain(this)
		receiver = this.object
	}
	s.into = vmEnter{this: receiver, scope: scope, called: called, internal: true}
	err := s.push(fn, len(args), 0)
	if s.into.this != nil {
		// push failed before it made the frame.
		_ = s.releaseObject(s.into.this)
	}
	s.into = vmEnter{}
	if err != nil {
		s.stop(err)
	} else {
		s.frames[len(s.frames)-1].boundary = true
		s.floor = len(s.frames)
		s.loop()
	}
	out := s.end
	release := out.kind == endNormal || out.kind == endReturn || out.kind == endThrow || out.kind == endExit
	for len(s.frames) > depth {
		if perr := s.pop(release); perr != nil && release {
			// A destructor of the frame's values failed: that is how the
			// call ends.
			if out.kind == endReturn && out.returnOwned {
				_ = s.releaseValue(out.value)
			}
			s.ended, s.end = false, completion{}
			s.stop(perr)
			out = s.end
			release = false
		}
	}
	s.ended, s.end, s.floor = ended, end, floor
	if out.kind == endReturn {
		return out.value, nil
	}
	if out.kind == endNormal {
		return nullValue(), nil
	}
	return value{}, out
}

// funcName is the name PHP gives the frame's function in messages.
func (s *vmState) funcName(f *vmFrame) string {
	if f.scope != nil {
		return f.scope.name + "::" + f.fn.name
	}
	return f.fn.name
}

func pendingName(fn *vmFunction, scope *class) string {
	if scope != nil {
		return scope.name + "::" + fn.name
	}
	return fn.name
}

// vmTypeAccepts is the fast path of a type check: the declared type is one
// plain name and the value already has it. Everything else goes through
// W-13's coercion.
func vmTypeAccepts(typ string, v value) bool {
	switch typ {
	case "mixed":
		return true
	case "int":
		return v.kind == valueInt
	case "string":
		return v.kind == valueString
	case "bool":
		return v.kind == valueBool
	case "float":
		return v.kind == valueFloat
	case "array":
		return v.kind == valueArray
	}
	return false
}

// checkArguments is the type check of ZEND_RECV for the arguments the call
// passed, in order, in the callee's frame. A value that does not have the
// declared type is converted as PHP's coercion rules allow, or the call
// throws a TypeError located at the function.
func (s *vmState) checkArguments(f *vmFrame, argc int) error {
	for i := 0; i < argc && i < f.fn.params; i++ {
		if f.fn.paramTypes[i] != "" {
			if pc := f.fn.paramPC[i]; pc >= 0 {
				f.pc = int(pc)
			}
			if err := s.checkParameter(f, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkParameter is the type check of ZEND_RECV for parameter i, run by
// the instruction that receives it.
func (s *vmState) checkParameter(f *vmFrame, i int) error {
	fn := f.fn
	if i >= len(fn.paramTypes) || fn.paramTypes[i] == "" || !f.passed(uint32(i)) {
		return nil
	}
	typ := fn.paramTypes[i]
	strict := false
	if n := len(s.frames); n >= 2 && !f.weak {
		strict = s.frames[n-2].fn.strict
	}
	for {
		raw := f.regs[i]
		if raw.kind == valueUndefined {
			return nil // omitted: the default is used
		}
		v := raw.deref()
		if vmTypeAccepts(typ, v) {
			return nil
		}
		ms := methodSpec{owner: f.scope, name: fn.name, static: f.this == nil}
		out, ok, err := s.h.coerceTypedValue(&ms, f.called, typ, v, strict)
		if err != nil {
			return err
		}
		if !ok {
			return s.argumentTypeError(f, i, typ, v)
		}
		if raw.kind == valueReference {
			return s.h.assignPHP(&f.regs[i], out, strict)
		}
		return s.setReg(f.regs, uint32(i), out)
	}
}

func (s *vmState) argumentTypeError(f *vmFrame, i int, typ string, v value) error {
	name := ""
	if i < len(f.fn.paramNames) && f.fn.paramNames[i] != "" && i < f.fn.params {
		name = " ($" + f.fn.paramNames[i] + ")"
	}
	msg := s.funcName(f) + "(): Argument #" + strconv.Itoa(i+1) + name + " must be of type " + resolvedType(typ, f) + ", " + valueName(v) + " given"
	if n := len(s.frames); n >= 2 && !f.boundary {
		caller := s.frames[n-2]
		msg += ", called in " + vmFileName(caller.fn.files[caller.pc]) + " on line " + strconv.Itoa(int(caller.fn.lines[caller.pc]))
	}
	return &Throwable{"TypeError", msg}
}

// resolvedType is zend_type_to_string_resolved: the declared type with
// self, parent and static replaced by the classes they name in frame f.
func resolvedType(typ string, f *vmFrame) string {
	if f.scope == nil || !(strings.Contains(typ, "self") || strings.Contains(typ, "static") || strings.Contains(typ, "parent")) {
		return typ
	}
	var b strings.Builder
	for i := 0; i < len(typ); {
		j := i
		for j < len(typ) && (typ[j] == '_' || typ[j] == '\\' || typ[j] >= '0' && typ[j] <= '9' || typ[j] >= 'A' && typ[j] <= 'Z' || typ[j] >= 'a' && typ[j] <= 'z' || typ[j] >= 0x80) {
			j++
		}
		if j == i {
			b.WriteByte(typ[i])
			i++
			continue
		}
		switch word := typ[i:j]; {
		case word == "self":
			b.WriteString(f.scope.name)
		case word == "static" && f.called != nil:
			b.WriteString(f.called.name)
		case word == "parent" && f.scope.schema != nil && f.scope.schema.parent != nil:
			b.WriteString(f.scope.schema.parent.name)
		default:
			b.WriteString(word)
		}
		i = j
	}
	return b.String()
}

// verifyReturn is ZEND_VERIFY_RETURN_TYPE.
func (s *vmState) verifyReturn(f *vmFrame, in *vmInstr) error {
	fn := f.fn
	typ := fn.returnType
	switch bytecode.ReturnCheck(in.aux) {
	case bytecode.ReturnNone:
		return &Throwable{"TypeError", s.funcName(f) + "(): Return value must be of type " + typ + ", none returned"}
	case bytecode.ReturnNever:
		kind := "function"
		if f.scope != nil {
			kind = "method"
		}
		return &Throwable{"TypeError", s.funcName(f) + "(): never-returning " + kind + " must not implicitly return"}
	}
	v := f.regs[in.a]
	if v.kind == valueUndefined {
		v = nullValue()
	}
	v = v.deref()
	if typ == "" || vmTypeAccepts(typ, v) {
		return nil
	}
	ms := methodSpec{owner: f.scope, name: fn.name, static: f.this == nil}
	out, ok, err := s.h.coerceTypedValue(&ms, f.called, typ, v, fn.strict)
	if err != nil {
		return err
	}
	if !ok {
		return &Throwable{"TypeError", s.funcName(f) + "(): Return value must be of type " + resolvedType(typ, f) + ", " + valueName(v) + " returned"}
	}
	return s.setReg(f.regs, in.a, out)
}

// --- starting calls ---

// startCall records the call of method m, as ZEND_INIT_METHOD_CALL leaves a
// call frame: the arguments that follow are sent to it and CallPending
// enters it.
func (s *vmState) startCall(m *methodSpec, this value, called *class) error {
	if m.abstract || m.call == nil {
		return &Throwable{"Error", "Cannot call abstract method " + m.owner.name + "::" + m.name + "()"}
	}
	if m.fn == 0 {
		return s.unsupported("calling a method of a builtin class")
	}
	rec := vmPendingCall{fn: int(m.fn - 1), base: len(s.args), depth: len(s.frames), scope: m.owner, called: called}
	if !m.static && this.kind == valueObject {
		retain(this)
		rec.this = this.object
	}
	s.calls = append(s.calls, rec)
	return nil
}

// startMagic records a call that __call or __callStatic receives: the
// arguments are collected into an array when the call is made.
func (s *vmState) startMagic(magic *methodSpec, this value, called *class, name string) error {
	if magic.fn == 0 || magic.call == nil {
		return s.unsupported("calling a method of a builtin class")
	}
	rec := vmPendingCall{fn: s.prog.trampoline, base: len(s.args), depth: len(s.frames), scope: magic.owner, called: called, magic: name, magicFn: int(magic.fn - 1)}
	if !magic.static && this.kind == valueObject {
		retain(this)
		rec.this = this.object
	}
	s.calls = append(s.calls, rec)
	return nil
}

func (s *vmState) methodNameOperand(f *vmFrame, in *vmInstr, operand uint32) (string, error) {
	if in.aux&bytecode.MemberNameReg == 0 {
		return s.prog.consts[operand].text, nil
	}
	v := f.regs[operand].deref()
	if v.kind != valueString {
		return "", &Throwable{"Error", "Method name must be a string"}
	}
	return v.text, nil
}

// methodAccessible is zend_check_protected for a method: a protected one is
// reachable from every class related to the class that first declared it
// (zend_get_function_root_class).
func methodAccessible(scope *class, m *methodSpec) bool {
	switch m.visibility {
	case publicAccess:
		return true
	case privateAccess:
		return scope == m.owner
	}
	if scope == nil {
		return false
	}
	root := m.owner
	lower := phpNameLower(m.name)
	for c := root; c != nil && c.schema != nil; c = c.schema.parent {
		if pm := c.schema.methods[lower]; pm != nil && pm.visibility != privateAccess {
			root = pm.owner
		}
	}
	return classIs(scope, root) || classIs(root, scope)
}

// initMethod is ZEND_INIT_METHOD_CALL.
func (s *vmState) initMethod(f *vmFrame, in *vmInstr) error {
	if in.aux == bytecode.InitCallable {
		return s.initCallable(f, in)
	}
	name, err := s.methodNameOperand(f, in, in.b)
	if err != nil {
		return err
	}
	obj := f.regs[in.a].deref()
	if obj.kind != valueObject {
		if obj.kind == valueUndefined {
			obj = nullValue()
		}
		return &Throwable{"Error", "Call to a member function " + name + "() on " + valueName(obj)}
	}
	h := s.h
	c := obj.object.class
	cache := s.cacheOf(in)
	if cache != nil && cache.method != nil && cache.class == c && cache.scope == f.scope {
		return s.startCall(cache.method, obj, c)
	}
	m, found := h.resolveMethod(c, name, f.scope)
	if found && m != nil && methodAccessible(f.scope, m) {
		if cache != nil {
			*cache = vmCache{class: c, scope: f.scope, method: m}
		}
		return s.startCall(m, obj, c)
	}
	if magic, ok := h.resolveMethod(c, "__call", nil); ok && magic != nil && !magic.static {
		return s.startMagic(magic, obj, c, name)
	}
	if found && m != nil {
		if phpNameEqual(m.name, "__construct") {
			return &Throwable{"Error", "Cannot call " + accessName(m.visibility) + " " + c.name + "::__construct()"}
		}
		return &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + m.owner.name + "::" + m.name + "() from " + methodScopeLabel(f.scope)}
	}
	return &Throwable{"Error", "Call to undefined method " + c.name + "::" + name + "()"}
}

// initStatic is ZEND_INIT_STATIC_METHOD_CALL (zend_std_get_static_method).
func (s *vmState) initStatic(f *vmFrame, in *vmInstr) error {
	c, err := s.classOperand(f, in, in.a)
	if err != nil {
		return err
	}
	name, err := s.methodNameOperand(f, in, in.b)
	if err != nil {
		return err
	}
	h := s.h
	ref := bytecode.AuxClassRef(in.aux)
	// The object of the calling method goes along when it is an instance
	// of the named class (parent::m(), self::m(), A::m() inside A).
	var this value
	if f.this != nil && classIs(f.this.class, c) {
		this = value{kind: valueObject, object: f.this}
	}
	called := c
	if ref == bytecode.ClassSelf || ref == bytecode.ClassParent {
		// self:: and parent:: forward the called class.
		if f.this != nil {
			called = f.this.class
		} else if f.called != nil {
			called = f.called
		}
	}
	// zend_std_get_static_method looks in the named class only: the private
	// method of the calling scope that an instance call prefers plays no
	// part here.
	var m *methodSpec
	if c.schema != nil {
		m = c.schema.methods[phpNameLower(name)]
	}
	found := m != nil
	if !found && in.aux&bytecode.MemberNameReg == 0 && phpNameEqual(name, "__construct") {
		// PHP decides this when it compiles a literal constructor name,
		// for any class reference and before __call or __callStatic.
		return &Throwable{"Error", "Cannot call constructor"}
	}
	if found && methodAccessible(f.scope, m) {
		if m.static {
			return s.startCall(m, value{}, called)
		}
		if this.kind != valueObject {
			if m.abstract {
				return &Throwable{"Error", "Cannot call abstract method " + m.owner.name + "::" + m.name + "()"}
			}
			return &Throwable{"Error", "Non-static method " + m.owner.name + "::" + m.name + "() cannot be called statically"}
		}
		return s.startCall(m, this, this.object.class)
	}
	if this.kind == valueObject {
		if magic, ok := h.resolveMethod(c, "__call", nil); ok && magic != nil && !magic.static {
			return s.startMagic(magic, this, this.object.class, name)
		}
	}
	if magic, ok := h.resolveMethod(c, "__callStatic", nil); ok && magic != nil && magic.static {
		return s.startMagic(magic, value{}, called, name)
	}
	if found && m != nil {
		if phpNameEqual(m.name, "__construct") {
			return &Throwable{"Error", "Cannot call " + accessName(m.visibility) + " " + c.name + "::__construct()"}
		}
		return &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + c.name + "::" + m.name + "() from " + methodScopeLabel(f.scope)}
	}
	return &Throwable{"Error", "Call to undefined method " + c.name + "::" + name + "()"}
}

// newObject is ZEND_NEW: the object is created and stored, then its
// constructor's call is started. Without a constructor the instruction
// jumps past the arguments, which PHP does not evaluate then.
func (s *vmState) newObject(f *vmFrame, in *vmInstr) (jump bool, err error) {
	c, err := s.classOperand(f, in, in.b)
	if err != nil {
		return false, err
	}
	if err := s.prepareClass(c); err != nil {
		return false, err
	}
	obj, err := s.h.instantiate(c)
	if err != nil {
		return false, err
	}
	if err := s.setReg(f.regs, in.a, obj); err != nil {
		return false, err
	}
	ctor, ok := s.h.resolveMethod(c, "__construct", f.scope)
	if !ok || ctor == nil {
		if in.aux&bytecode.NewArgs == 0 {
			return true, nil
		}
		// The call owns the object while the arguments run: when one of
		// them fails, the object counts as not constructed and gets no
		// destructor (zend_object_store_ctor_failed).
		retain(obj)
		s.calls = append(s.calls, vmPendingCall{fn: s.prog.trampoline, base: len(s.args), depth: len(s.frames), pass: true, ctor: true, this: obj.object})
		return false, nil
	}
	if !canAccess(f.scope, ctor.owner, ctor.visibility) {
		return false, &Throwable{"Error", "Call to " + accessName(ctor.visibility) + " " + c.name + "::__construct() from " + methodScopeLabel(f.scope)}
	}
	if err := s.startCall(ctor, f.regs[in.a], c); err != nil {
		return false, err
	}
	s.calls[len(s.calls)-1].ctor = true
	return false, nil
}

// packMagic turns the pending call's arguments into the two arguments of
// __call or __callStatic: the method name and the array of arguments.
func (s *vmState) packMagic(rec *vmPendingCall) error {
	arr := s.h.newArrayValue()
	retain(arr)
	args := s.args[rec.base:]
	var first error
	for i, a := range args {
		v := a.deref()
		if v.kind == valueUndefined {
			v = nullValue()
		}
		if err := s.h.appendValue(&arr, v); err != nil && first == nil {
			first = err
		}
		args[i] = value{}
		if err := s.releaseValue(a); err != nil && first == nil {
			first = err
		}
	}
	s.args = s.args[:rec.base]
	if rec.named.kind == valueArray {
		for _, b := range rec.named.array.buckets {
			if b != nil {
				if err := s.h.setElement(&arr, stringValue(b.key.text), b.val.deref()); err != nil && first == nil {
					first = err
				}
			}
		}
		named := rec.named
		rec.named = value{}
		if err := s.releaseValue(named); err != nil && first == nil {
			first = err
		}
	}
	s.args = append(s.args, stringValue(rec.magic), arr)
	rec.fn, rec.magic = rec.magicFn, ""
	return first
}

// sendByCallee is a Push whose mode the callee decides (format 1.7): pos is
// the argument's position, or -1 for a named argument a variadic parameter
// collects.
func (s *vmState) sendByCallee(f *vmFrame, in *vmInstr, rec *vmPendingCall, pos int) (value, error) {
	fn := &s.prog.funcs[rec.fn]
	byRef := fn.variadicByRef
	if pos >= 0 {
		byRef = fn.byRefAt(pos)
	}
	switch in.aux {
	case vmPushAuto:
		if byRef {
			src, err := s.slot(f, in.a)
			if err != nil {
				return value{}, err
			}
			nullIfUndefined(src)
			var arg value
			err = s.h.bind(&arg, src)
			return arg, err
		}
		v, _, err := s.readVar(f, in.a)
		if err != nil {
			return value{}, err
		}
		if v.kind >= valueArray {
			retain(v)
		}
		return v, nil
	case vmPushTemp:
		if byRef {
			name := ""
			if pos >= 0 && pos < fn.params && pos < len(fn.paramNames) {
				name = " ($" + fn.paramNames[pos] + ")"
			}
			return value{}, &Throwable{"Error", pendingName(fn, rec.scope) + "(): Argument #" + strconv.Itoa(pos+1) + name + " could not be passed by reference"}
		}
	case vmPushElement:
		if byRef {
			return value{}, s.unsupported("passing an element or a property to a by-reference parameter of a callee found at run time")
		}
	case vmPushResult:
		if byRef {
			s.h.warn(levelNotice, "Only variables should be passed by reference")
		}
	}
	v := f.regs[in.a]
	switch {
	case v.kind == valueUndefined:
		v = nullValue()
	case v.kind == valueReference:
		t := v.ref.target
		if t.kind >= valueArray {
			retain(t)
		}
		v = t
	case v.kind >= valueArray:
		retain(v)
	}
	return v, nil
}

// initCallable is ZEND_INIT_DYNAMIC_CALL: an object is called through
// __invoke, a string names a function of the program.
func (s *vmState) initCallable(f *vmFrame, in *vmInstr) error {
	callable := f.regs[in.a].deref()
	switch callable.kind {
	case valueObject:
		c := callable.object.class
		m, ok := s.h.resolveMethod(c, "__invoke", nil)
		if !ok || m == nil {
			return &Throwable{"Error", "Object of type " + c.name + " is not callable"}
		}
		return s.startCall(m, callable, c)
	case valueString:
		name := strings.TrimPrefix(callable.text, "\\")
		idx, ok := s.prog.byName[phpNameLower(name)]
		if !ok {
			if strings.Contains(name, "::") {
				return s.unsupported("calling a method through a string")
			}
			return &Throwable{"Error", "Call to undefined function " + name + "()"}
		}
		s.calls = append(s.calls, vmPendingCall{fn: idx, base: len(s.args), depth: len(s.frames)})
		return nil
	case valueArray:
		return s.unsupported("calling an array callable")
	}
	return &Throwable{"Error", "Value not callable"}
}

// releaseObject gives up one owner of o.
func (s *vmState) releaseObject(o *object) error {
	return s.h.release(value{kind: valueObject, object: o})
}
