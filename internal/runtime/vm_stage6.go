package runtime

import (
	"errors"
	"fmt"
	"strconv"
)

// W-17 stage 6: REQ-001, REQ-016, REQ-025, REQ-026; T-009, T-010,
// T-029..T-032. The format 1.6 instructions: global and static variables,
// variadic parameters, argument unpacking and named arguments. Each follows
// the Zend handler named in its comment; the expectations are the PHP
// 8.5.12 outputs of tests/vm/w17/cases/{global,static,variadic,unpack,named}_*.

// Decoded aux values of Push: how the operand A is sent.
const (
	vmPushRef  uint16 = 1 // a Location sent by reference (ZEND_SEND_REF)
	vmPushLoad uint16 = 2 // a Location whose value is sent (ZEND_SEND_VAR)
	// Format 1.7, for a callee found at run time.
	vmPushAuto    uint16 = 3 // a Location sent as the callee declares (ZEND_SEND_VAR_EX)
	vmPushTemp    uint16 = 4 // a Value no variable holds (ZEND_SEND_VAL_EX)
	vmPushResult  uint16 = 5 // a call's result (ZEND_SEND_VAR_NO_REF_EX)
	vmPushElement uint16 = 6 // the value of an element or a property
)

// vmPendingCall is a call InitCall started and CallPending has not made
// yet, PHP's call frame between INIT_FCALL and DO_FCALL. Its arguments are
// the pending arguments from base up; a hole among them is an undefined
// value. It belongs to the frame that was on top when it started.
type vmPendingCall struct {
	fn    int
	base  int
	depth int
	// named holds the named arguments no parameter takes, which the
	// variadic parameter collects (extra_named_params): an array the call
	// owns, or the zero value.
	named value
	// Format 1.7: the object (an owner), the lexical class and the called
	// class of a method call. magic is the name of a method that __call or
	// __callStatic receives, whose function is magicFn.
	this          *object
	scope, called *class
	magic         string
	magicFn       int
	// pass marks the call a class without a constructor gets for the
	// arguments of new: they are evaluated and discarded.
	pass bool
}

// vmCallInfo is what a frame entered by CallPending knows beyond a plain
// call's frame.
type vmCallInfo struct {
	named value
	// omitted marks the parameters below nargs that no argument filled:
	// their defaults are evaluated as for an argument that was not passed
	// (zend_handle_undef_args).
	omitted []uint64
	// holePC is the prologue instruction that receives the last omitted
	// parameter, or -1. Up to it the frame is where PHP runs
	// ZEND_CHECK_UNDEF_ARGS, whose line a backtrace shows for the call.
	holePC int32
	// missing marks a frame entered only to report, in it, a parameter
	// without a default that no argument filled.
	missing bool
}

func (c *vmCallInfo) isOmitted(p uint32) bool {
	return int(p/64) < len(c.omitted) && c.omitted[p/64]&(1<<(p%64)) != 0
}

// passed reports whether the call gave parameter p an argument.
func (f *vmFrame) passed(p uint32) bool {
	return int(p) < f.nargs && (f.call == nil || !f.call.isOmitted(p))
}

// errNoFrame is callPending's report that the call needed no frame.
var errNoFrame = errors.New("call made without a frame")

// vmTraitStatic names the static variables of a trait method as used by
// one class.
type vmTraitStatic struct {
	fn    int
	scope *class
}

// --- global ---

// bindGlobal is ZEND_BIND_GLOBAL: the variable A becomes a reference to
// the global variable named B, which is created holding null when it does
// not exist. The globals are {main}'s variables, and beside them the names
// {main} never mentions.
func (s *vmState) bindGlobal(f *vmFrame, in *vmInstr) error {
	dst, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	name := s.prog.consts[in.b].text
	var src *value
	if i, ok := s.prog.mainLocal[name]; ok {
		src = &s.frames[0].locals[i]
	} else if src, err = s.globalVar(name); err != nil {
		return err
	}
	nullIfUndefined(src)
	if src == dst {
		// "global $x" at the top level: the variable now exists.
		return nil
	}
	return s.h.bind(dst, src)
}

// globalVar is the global variable that is not one of {main}'s: created on
// first use, from the host's value of that name when it gave one.
func (s *vmState) globalVar(name string) (*value, error) {
	if p, ok := s.globalVars[name]; ok {
		return p, nil
	}
	p := new(value)
	for _, g := range s.globals {
		if g.Name != name {
			continue
		}
		v, err := s.h.importSnapshotPHP8512(g.Value, vmGlobalDepth, vmGlobalElements)
		if err != nil {
			return nil, fmt.Errorf("global $%s: %w", g.Name, err)
		}
		err = s.h.assign(p, v)
		if rerr := s.h.release(v); err == nil {
			err = rerr
		}
		if err != nil {
			return nil, err
		}
	}
	if s.globalVars == nil {
		s.globalVars = make(map[string]*value)
	}
	s.globalVars[name] = p
	return p, nil
}

// --- static variables ---

// staticSlot is the run's static variable of the function's local that the
// Location register r names. The slots live with the run, never in the
// shared Program.
func (s *vmState) staticSlot(f *vmFrame, r uint32) (*value, error) {
	l := f.locs[r]
	if l <= 0 || int(l) > len(f.locals) {
		return nil, s.unsupported("a static variable that is not a local")
	}
	fi := f.fn.index
	if s.prog.fnTrait != nil && s.prog.fnTrait[fi] && f.scope != nil {
		key := vmTraitStatic{fi, f.scope}
		slots := s.traitStatics[key]
		if slots == nil {
			if s.traitStatics == nil {
				s.traitStatics = map[vmTraitStatic][]value{}
			}
			slots = make([]value, f.fn.locals)
			s.traitStatics[key] = slots
		}
		return &slots[l-1], nil
	}
	if s.statics == nil {
		s.statics = make([][]value, len(s.prog.funcs))
	}
	if s.statics[fi] == nil {
		s.statics[fi] = make([]value, f.fn.locals)
	}
	return &s.statics[fi][l-1], nil
}

// staticInit is ZEND_BIND_INIT_STATIC_OR_JMP: when the static variable is
// initialized, A is bound to it and the initializer is skipped.
func (s *vmState) staticInit(f *vmFrame, in *vmInstr) (bound bool, err error) {
	st, err := s.staticSlot(f, in.a)
	if err != nil || st.kind != valueReference {
		return false, err
	}
	dst, err := s.slot(f, in.a)
	if err != nil {
		return false, err
	}
	return true, s.h.bind(dst, st)
}

// bindStatic is ZEND_BIND_STATIC with an initializer: the value of B
// becomes the static variable unless a call made from the initializer
// already initialized it, and A is bound to the variable. B is emptied.
func (s *vmState) bindStatic(f *vmFrame, in *vmInstr) error {
	v := take(f.regs, in.b)
	if v.kind == valueReference {
		t := v.ref.target
		retain(t)
		if err := s.releaseValue(v); err != nil {
			return err
		}
		v = t
	}
	st, err := s.staticSlot(f, in.a)
	if err != nil {
		_ = s.releaseValue(v)
		return err
	}
	if st.kind != valueReference {
		*st = value{kind: valueReference, ref: &reference{owner: s.h, refcount: 1, target: v}}
	} else if err := s.releaseValue(v); err != nil {
		return err
	}
	dst, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	return s.h.bind(dst, st)
}

// --- variadic parameters ---

// recvVariadic is ZEND_RECV_VARIADIC: the arguments beyond the declared
// parameters, in order from key 0, then the extra named arguments under
// their names. An argument sent by reference stays one in the array.
func (s *vmState) recvVariadic(f *vmFrame, in *vmInstr) error {
	arr := s.h.newArrayValue()
	a := arr.array
	// A by-value variadic parameter holds values: a reference that
	// hand-written bytecode sent is read here, as a by-value parameter's
	// Store reads its register.
	byRef := f.fn.variadicByRef
	typ := ""
	if len(f.fn.paramTypes) > f.fn.params {
		typ = f.fn.paramTypes[f.fn.params]
	}
	strict := false
	if n := len(s.frames); n >= 2 && !f.boundary {
		strict = s.frames[n-2].fn.strict
	}
	ms := methodSpec{owner: f.scope, name: f.fn.name, static: f.this == nil}
	for i, v := range f.extra {
		if !byRef {
			v = v.deref()
		}
		if typ != "" && !vmTypeAccepts(typ, v.deref()) {
			out, ok, err := s.h.coerceTypedValue(&ms, f.called, typ, v.deref(), strict)
			if err != nil {
				return err
			}
			if !ok {
				f.pc, f.unreceived = 0, true
				return s.argumentTypeError(f, f.fn.params+i, typ, v.deref())
			}
			v = out
		}
		if v.kind >= valueArray {
			retain(v)
		}
		a.insert(intKey(int64(i)), v, s.prog.width)
	}
	if f.call != nil && f.call.named.kind == valueArray {
		for _, b := range f.call.named.array.buckets {
			if b == nil {
				continue
			}
			v := b.val
			if !byRef {
				v = v.deref()
			}
			if v.kind >= valueArray {
				retain(v)
			}
			a.insert(b.key, v, s.prog.width)
		}
	}
	return s.setReg(f.regs, in.a, arr)
}

// --- calls with named or unpacked arguments ---

// pendingCall is the innermost call the current frame started.
func (s *vmState) pendingCall() (*vmPendingCall, error) {
	if n := len(s.calls); n > 0 && s.calls[n-1].depth == len(s.frames) {
		return &s.calls[n-1], nil
	}
	return nil, s.unsupported("a named or unpacked argument outside a started call")
}

// dropCalls releases the pending calls of the frames at depth and above.
func (s *vmState) dropCalls(depth int) error {
	var first error
	for n := len(s.calls); n > 0 && s.calls[n-1].depth >= depth; n = len(s.calls) {
		named, this := s.calls[n-1].named, s.calls[n-1].this
		s.calls[n-1] = vmPendingCall{}
		s.calls = s.calls[:n-1]
		if err := s.releaseValue(named); err != nil && first == nil {
			first = err
		}
		if this != nil {
			if err := s.releaseObject(this); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// pushValue takes the operand of a Push as an owned argument.
func (s *vmState) pushValue(f *vmFrame, in *vmInstr) (value, error) {
	switch in.aux {
	case vmPushRef:
		src, err := s.slot(f, in.a)
		if err != nil {
			return value{}, err
		}
		nullIfUndefined(src)
		var arg value
		err = s.h.bind(&arg, src)
		return arg, err
	case vmPushLoad:
		v, _, err := s.readVar(f, in.a)
		if err != nil {
			return value{}, err
		}
		if v.kind >= valueArray {
			retain(v)
		}
		return v, nil
	}
	v := f.regs[in.a]
	switch {
	case v.kind == valueUndefined:
		v = nullValue()
	case v.kind == valueReference:
		f.regs[in.a] = value{} // the reference moves to the argument
	case v.kind >= valueArray:
		retain(v)
	}
	return v, nil
}

// namedTarget is zend_handle_named_arg: where the argument named name of
// the pending call goes. index is its parameter, whose pending slot is
// made if needed (leaving holes before it), or -1 for a name the variadic
// parameter collects. A name no parameter has, and one already given,
// throw before the argument is read.
func (s *vmState) namedTarget(rec *vmPendingCall, name string) (index int, err error) {
	fn := &s.prog.funcs[rec.fn]
	index = -1
	for i, p := range fn.paramNames[:min(fn.params, len(fn.paramNames))] {
		if p == name {
			index = i
			break
		}
	}
	if index < 0 {
		if !fn.variadic {
			return 0, &Throwable{"Error", "Unknown named parameter $" + name}
		}
		if rec.named.kind == valueArray {
			if _, dup := rec.named.array.lookup(arrayKey{isString: true, text: name}); dup {
				return 0, &Throwable{"Error", "Named parameter $" + name + " overwrites previous argument"}
			}
		}
		return -1, nil
	}
	have := len(s.args) - rec.base
	if have < 0 {
		return 0, s.internal("pending call with %d arguments below its base", -have)
	}
	if index < have {
		if s.args[rec.base+index].kind != valueUndefined {
			return 0, &Throwable{"Error", "Named parameter $" + name + " overwrites previous argument"}
		}
		return index, nil
	}
	for ; have <= index; have++ {
		s.args = append(s.args, value{})
	}
	return index, nil
}

// storeNamed gives the pending call its named argument v (owned).
func (s *vmState) storeNamed(rec *vmPendingCall, index int, name string, v value) {
	if index >= 0 {
		s.args[rec.base+index] = v
		return
	}
	if rec.named.kind != valueArray {
		rec.named = s.h.newArrayValue()
		retain(rec.named)
	}
	rec.named.array.insert(arrayKey{isString: true, text: name}, v, s.prog.width)
}

// sendNamed is a SEND opcode with a named argument: the name is resolved
// against the callee, then the operand is read.
func (s *vmState) sendNamed(f *vmFrame, in *vmInstr) error {
	rec, err := s.pendingCall()
	if err != nil {
		return err
	}
	name := s.prog.consts[in.b-1].text
	index, err := s.namedTarget(rec, name)
	if err != nil {
		return err
	}
	var v value
	if in.aux >= vmPushAuto {
		v, err = s.sendByCallee(f, in, rec, index)
	} else {
		v, err = s.pushValue(f, in)
	}
	if err != nil {
		return err
	}
	if v.kind == valueUndefined {
		v = nullValue()
	}
	s.storeNamed(rec, index, name, v)
	return nil
}

// byRefAt reports whether the function takes argument position i (from 0)
// by reference; positions beyond the parameters belong to the variadic one.
func (fn *vmFunction) byRefAt(i int) bool {
	if i < len(fn.paramByRef) {
		return fn.paramByRef[i]
	}
	return fn.variadicByRef
}

// vmUnpackPoll is how many unpacked elements pass between two cancellation
// polls.
const vmUnpackPoll = 1 << 16

// sendUnpack is ZEND_SEND_UNPACK for an array: each element becomes the
// next positional argument, or the named argument of its string key. For a
// by-reference parameter the element itself becomes a reference when the
// operand is a variable, whose array is separated first; an element of a
// temporary array is passed as a new reference.
func (s *vmState) sendUnpack(f *vmFrame, in *vmInstr) error {
	rec, err := s.pendingCall()
	if err != nil {
		return err
	}
	fn := &s.prog.funcs[rec.fn]
	var slot *value
	variable := in.aux == vmPushRef
	if variable {
		if slot, err = s.slot(f, in.a); err != nil {
			return err
		}
		if slot.deref().kind == valueUndefined {
			if name, ok := s.variableName(f, in.a); ok {
				s.h.warn(levelWarning, "Undefined variable $"+name)
				if s.stopped() {
					return nil
				}
			}
		}
	} else {
		slot = &f.regs[in.a]
	}
	t := slot.deref()
	if t.kind == valueObject {
		return s.unsupported("unpacking an object")
	}
	if t.kind != valueArray {
		if t.kind == valueUndefined {
			t = nullValue()
		}
		return &Throwable{"TypeError", "Only arrays and Traversables can be unpacked, " + valueName(t) + " given"}
	}
	if len(s.args) < rec.base {
		return s.internal("pending call with %d arguments below its base", rec.base-len(s.args))
	}
	a := t.array
	if variable && fn.anyByRef && a.refcount > 1 {
		// The variable's array is separated when an element is going to
		// be made a reference in it.
		pos, separate := len(s.args)-rec.base, false
		for _, b := range a.buckets {
			if b == nil {
				continue
			}
			p := pos
			if b.key.isString {
				p = fn.paramIndex(b.key.text)
			} else {
				pos++
			}
			if p >= 0 && fn.byRefAt(p) {
				separate = true
				break
			}
		}
		if separate {
			if a, err = s.h.separate(slot); err != nil {
				return err
			}
		}
	}
	named := false
	for i, b := range a.buckets {
		if i%vmUnpackPoll == vmUnpackPoll-1 && s.poll() {
			return nil
		}
		if b == nil {
			continue
		}
		index, pos := -1, len(s.args)-rec.base
		if b.key.isString {
			named = true
			if index, err = s.namedTarget(rec, b.key.text); err != nil {
				return err
			}
			pos = index
			if index < 0 {
				pos = fn.params
			}
		} else if named {
			return &Throwable{"Error", "Cannot use positional argument after named argument during unpacking"}
		}
		var v value
		switch {
		case !fn.byRefAt(pos):
			if v = b.val.deref(); v.kind >= valueArray {
				retain(v)
			}
		case variable || b.val.kind == valueReference:
			if err := s.h.bind(&v, &b.val); err != nil {
				return err
			}
		default:
			if b.val.kind >= valueArray {
				retain(b.val)
			}
			v = value{kind: valueReference, ref: &reference{owner: s.h, refcount: 1, target: b.val}}
		}
		if b.key.isString {
			s.storeNamed(rec, index, b.key.text, v)
		} else {
			s.args = append(s.args, v)
		}
	}
	return nil
}

// paramIndex is the parameter named name, the variadic position for a name
// it would collect, or -1.
func (fn *vmFunction) paramIndex(name string) int {
	for i, p := range fn.paramNames[:min(fn.params, len(fn.paramNames))] {
		if p == name {
			return i
		}
	}
	if fn.variadic {
		return fn.params
	}
	return -1
}

// callPending is ZEND_CHECK_UNDEF_ARGS and the call: the pending call's
// arguments, holes included, become the callee's frame. A hole in a
// parameter without a default throws in that frame, at the instruction
// that receives the parameter; the other holes are filled by the callee's
// prologue, which evaluates their defaults as for arguments not passed.
func (s *vmState) callPending(in *vmInstr) (err error) {
	if in.aux != 0 {
		return s.unsupported("calls returning a location")
	}
	rec, err := s.pendingCall()
	if err != nil {
		return err
	}
	if rec.pass {
		f := s.frames[len(s.frames)-1]
		err = s.dropArgs(len(s.args) - rec.base)
		named := rec.named
		*rec = vmPendingCall{}
		s.calls = s.calls[:len(s.calls)-1]
		if rerr := s.releaseValue(named); err == nil {
			err = rerr
		}
		if rerr := s.setReg(f.regs, in.a, nullValue()); err == nil {
			err = rerr
		}
		if err == nil {
			err = errNoFrame
		}
		return err
	}
	if rec.magic != "" {
		if err := s.packMagic(rec); err != nil {
			return err
		}
	}
	fn := &s.prog.funcs[rec.fn]
	argc := len(s.args) - rec.base
	if argc < 0 {
		return s.internal("pending call with %d arguments below its base", -argc)
	}
	// The call's extra knowledge stays on the stack unless the frame needs
	// it: only omitted parameters or collected names make it escape.
	var local vmCallInfo
	local.named, local.holePC = rec.named, -1
	into := vmEnter{this: rec.this, scope: rec.scope, called: rec.called}
	rec.this = nil
	first, last := -1, -1
	args := s.args[rec.base:]
	for i := 0; i < argc && i < fn.params; i++ {
		switch a := &args[i]; {
		case a.kind == valueUndefined:
			if local.omitted == nil {
				local.omitted = make([]uint64, (fn.params+63)/64)
			}
			local.omitted[i/64] |= 1 << (i % 64)
			if first < 0 {
				first = i
			}
			last = i
		case a.kind == valueReference && i < len(fn.paramByRef) && !fn.paramByRef[i]:
			// A by-value parameter receives the value of a reference.
			t := a.ref.target
			retain(t)
			old := *a
			*a = t
			if err := s.releaseValue(old); err != nil {
				return err
			}
		}
	}
	rec.named = value{}
	s.calls = s.calls[:len(s.calls)-1]
	missing := -1
	if first >= 0 {
		if first < fn.required {
			missing, last = first, first
		}
		if last < len(fn.paramPC) {
			local.holePC = fn.paramPC[last]
		}
	}
	var info *vmCallInfo
	if local.omitted != nil || local.named.kind == valueArray {
		kept := local
		info = &kept
	}
	if missing >= 0 {
		info.missing = true
	}
	s.entering = info
	s.into = into
	err = s.push(fn, argc, in.a)
	s.entering = nil
	if s.into.this != nil {
		// push failed before it made the frame.
		_ = s.releaseObject(s.into.this)
	}
	s.into = vmEnter{}
	if err != nil {
		return err
	}
	if missing < 0 {
		return nil
	}
	callee := s.frames[len(s.frames)-1]
	callee.unreceived = true
	if pc := fn.paramPC[missing]; pc >= 0 {
		callee.pc = int(pc)
	}
	name := ""
	if missing < len(fn.paramNames) {
		name = fn.paramNames[missing]
	}
	return &Throwable{"ArgumentCountError", s.funcName(callee) + "(): Argument #" + strconv.Itoa(missing+1) + " ($" + name + ") not passed"}
}

// derefArguments gives the by-value parameters of fn the values of the
// references among the top argc pending arguments. Compiled code never
// sends one; it keeps a reference out of a by-value parameter's register
// whatever bytecode calls the function by name.
func (s *vmState) derefArguments(fn *vmFunction, argc int) error {
	args := s.args[len(s.args)-argc:]
	for i := 0; i < argc && i < len(fn.paramByRef); i++ {
		if a := &args[i]; a.kind == valueReference && !fn.paramByRef[i] {
			t := a.ref.target
			retain(t)
			old := *a
			*a = t
			if err := s.releaseValue(old); err != nil {
				return err
			}
		}
	}
	return nil
}
