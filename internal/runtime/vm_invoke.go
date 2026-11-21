package runtime

import (
	"strconv"

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
	this          value // owned by the frame once pushed
	scope, called *class
	// internal marks a call the runtime makes: its arguments are converted
	// in weak mode whatever the caller's strict_types.
	internal bool
}

// vmMaxNesting bounds the nested loops of one run. PHP's own bound is the C
// stack (zend.max_allowed_stack_size); the error it raises there is the one
// reported here.
const vmMaxNesting = 4096

// invoke runs fn to completion from inside an instruction and returns its
// result as an owned value. args are borrowed. An error is the completion
// that ended the nested run.
func (s *vmState) invoke(fn *vmFunction, this value, scope, called *class, args []value) (value, error) {
	if s.nesting >= vmMaxNesting {
		return value{}, &Throwable{"Error", "Maximum call stack size of 8339456 bytes (zend.max_allowed_stack_size - zend.reserved_stack_size) reached. Infinite recursion?"}
	}
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
	if this.kind == valueObject {
		retain(this)
	} else {
		this = value{}
	}
	s.into = vmEnter{this: this, scope: scope, called: called, internal: true}
	err := s.push(fn, len(args), 0)
	if s.into.this.kind == valueObject {
		// push failed before it made the frame.
		_ = s.releaseValue(s.into.this)
	}
	s.into = vmEnter{}
	if err != nil {
		s.stop(err)
	} else {
		s.frames[len(s.frames)-1].boundary = true
		s.floor = len(s.frames)
		s.nesting++
		s.loop()
		s.nesting--
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
func (s *vmState) checkArguments(f *vmFrame, argc int, internal bool) error {
	fn := f.fn
	strict := false
	if n := len(s.frames); n >= 2 && !internal {
		strict = s.frames[n-2].fn.strict
	}
	ms := methodSpec{owner: f.scope, name: fn.name, static: f.this.kind != valueObject}
	for i := 0; i < argc && i < fn.params; i++ {
		typ := fn.paramTypes[i]
		if typ == "" {
			continue
		}
		raw := f.regs[i]
		if raw.kind == valueUndefined {
			continue // omitted: the default is used
		}
		v := raw.deref()
		if vmTypeAccepts(typ, v) {
			continue
		}
		if pc := fn.paramPC[i]; pc >= 0 {
			f.pc = int(pc)
		}
		out, ok, err := s.h.coerceTypedValue(&ms, f.called, typ, v, strict)
		if err != nil {
			return err
		}
		if !ok {
			return s.argumentTypeError(f, i, typ, v)
		}
		if raw.kind == valueReference {
			if err := s.h.assign(&raw.ref.target, out); err != nil {
				return err
			}
			continue
		}
		if err := s.setReg(f.regs, uint32(i), out); err != nil {
			return err
		}
	}
	return nil
}

func (s *vmState) argumentTypeError(f *vmFrame, i int, typ string, v value) error {
	name := ""
	if i < len(f.fn.paramNames) && f.fn.paramNames[i] != "" && i < f.fn.params {
		name = " ($" + f.fn.paramNames[i] + ")"
	}
	msg := s.funcName(f) + "(): Argument #" + strconv.Itoa(i+1) + name + " must be of type " + typ + ", " + valueName(v) + " given"
	if n := len(s.frames); n >= 2 && !f.boundary {
		caller := s.frames[n-2]
		msg += ", called in " + vmFileName(caller.fn.files[caller.pc]) + " on line " + strconv.Itoa(int(caller.fn.lines[caller.pc]))
	}
	return &Throwable{"TypeError", msg}
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
	ms := methodSpec{owner: f.scope, name: fn.name, static: f.this.kind != valueObject}
	out, ok, err := s.h.coerceTypedValue(&ms, f.called, typ, v, fn.strict)
	if err != nil {
		return err
	}
	if !ok {
		return &Throwable{"TypeError", s.funcName(f) + "(): Return value must be of type " + typ + ", " + valueName(v) + " returned"}
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
		rec.this = this
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
		rec.this = this
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

// initMethod is ZEND_INIT_METHOD_CALL.
func (s *vmState) initMethod(f *vmFrame, in *vmInstr) error {
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
	m, found := h.resolveMethod(c, name, f.scope)
	if found && m != nil && canAccess(f.scope, m.owner, m.visibility) {
		return s.startCall(m, obj, c)
	}
	if magic, ok := h.resolveMethod(c, "__call", nil); ok && magic != nil && !magic.static {
		return s.startMagic(magic, obj, c, name)
	}
	if found && m != nil {
		return &Throwable{"Error", "Call to " + accessName(m.visibility) + " method " + c.name + "::" + m.name + "() from " + methodScopeLabel(f.scope)}
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
	if f.this.kind == valueObject && classIs(f.this.object.class, c) {
		this = f.this
	}
	called := c
	if ref == bytecode.ClassSelf || ref == bytecode.ClassParent {
		// self:: and parent:: forward the called class.
		if f.this.kind == valueObject {
			called = f.this.object.class
		} else if f.called != nil {
			called = f.called
		}
	}
	m, found := h.resolveMethod(c, name, f.scope)
	if found && m != nil && canAccess(f.scope, m.owner, m.visibility) {
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
		return true, nil
	}
	if !canAccess(f.scope, ctor.owner, ctor.visibility) {
		return false, &Throwable{"Error", "Call to " + accessName(ctor.visibility) + " " + c.name + "::__construct() from " + methodScopeLabel(f.scope)}
	}
	return false, s.startCall(ctor, f.regs[in.a], c)
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
