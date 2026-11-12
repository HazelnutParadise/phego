package runtime

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// vmPollInterval bounds the straight-line instructions between two
// cancellation polls. Back-edges, calls, returns and Checkpoint always poll.
const vmPollInterval = 1024

// vmFrame is one PHP call. regs, locals and locs are windows of the
// execution's slabs.
// A location register holds local index + 1; zero is "not defined".
type vmFrame struct {
	fn     *vmFunction
	code   []vmInstr
	pc     int
	regs   []value
	locals []value
	locs   []int32
	base   int // offset of regs in the value slab
	lbase  int // offset of locs in the location slab
	dest   uint32
	nargs  int
	extra  []value // arguments beyond the declared parameters, for backtraces
}

type vmState struct {
	prog    *Program
	x       *execution
	h       *heap
	frames  []vmFrame
	slab    []value
	top     int
	lslab   []int32
	ltop    int
	args    []value
	steps   int64
	limit   int64
	next    int64 // steps at which the next slow check runs
	done    <-chan struct{}
	scratch []byte
	globals []Global
	prelude []RunDiagnostic
	end     completion
	ended   bool
	// lastFile and lastLine locate diagnostics raised when no frame is
	// active: the uncaught throwable reported after unwinding.
	lastFile string
	lastLine int
}

func newVMState(p *Program, x *execution, cfg RunConfig) *vmState {
	s := &vmState{prog: p, x: x, h: x.heap, globals: cfg.Globals, prelude: cfg.Prelude, done: cfg.Context.Done()}
	// Zero is unlimited. Every limit stays below MaxInt64 because slow
	// computes limit+1.
	s.limit = math.MaxInt64 - 1
	if cfg.Steps > 0 {
		s.limit = min(cfg.Steps, s.limit)
	}
	s.next = min(s.limit, vmPollInterval)
	excluded := ^cfg.ErrorReporting
	x.errors = errorChannels{display: cfg.DisplayErrors, log: cfg.LogErrors, where: s.where, excluded: excluded}
	x.heap.sink = s.warn
	return s
}

// where is the current PHP location for diagnostics.
func (s *vmState) where() (string, int) {
	if len(s.frames) == 0 {
		return s.lastFile, s.lastLine
	}
	f := &s.frames[len(s.frames)-1]
	return f.fn.files[f.pc], int(f.fn.lines[f.pc])
}

// warn is the heap's diagnostic sink. A failed display write or the
// diagnostic cap records the execution's stop, which the loop observes.
func (s *vmState) warn(level int, message string) {
	s.x.report(diagnosticLevelName(level), message, false)
}

func (s *vmState) location() (fn string, pc int, file string, line int) {
	if len(s.frames) == 0 {
		return "{main}", -1, s.lastFile, s.lastLine
	}
	f := &s.frames[len(s.frames)-1]
	return f.fn.name, f.pc, f.fn.files[f.pc], int(f.fn.lines[f.pc])
}

func (s *vmState) unsupported(feature string) error {
	fn, pc, file, line := s.location()
	return &UnsupportedError{Feature: feature, Function: fn, PC: pc, File: file, Line: line}
}

func (s *vmState) internal(format string, args ...any) error {
	fn, pc, file, line := s.location()
	return &InternalError{Detail: fmt.Sprintf(format, args...), Function: fn, PC: pc, File: file, Line: line}
}

// body is the execution action of the request: it runs {main} to
// completion with loop, the instruction loop.
func (s *vmState) body(loop func(*vmState)) completion {
	if s.enter() {
		loop(s)
	}
	s.unwind()
	return s.end
}

// enter reports the prelude and pushes {main} with its globals. It reports
// false when the run ended before {main}'s first instruction.
func (s *vmState) enter() bool {
	// Compile-time diagnostics come first, located where PHP compiled them.
	for _, d := range s.prelude {
		s.lastFile, s.lastLine = d.File, d.Line
		if c := s.x.report(d.Level, d.Message, false); c.kind != endNormal {
			s.ended, s.end = true, c
			return false
		}
	}
	s.lastFile, s.lastLine = "", 0
	main := &s.prog.funcs[s.prog.entry]
	if err := s.push(main, 0, 0); err != nil {
		s.stop(err)
		return false
	}
	if err := s.bindGlobals(main); err != nil {
		s.stop(err)
		return false
	}
	return true
}

// bindGlobals imports host values into the {main} locals of the same name.
func (s *vmState) bindGlobals(main *vmFunction) error {
	f := &s.frames[0]
	for _, g := range s.globals {
		for i, name := range main.localNames {
			if name == "" || name != g.Name {
				continue
			}
			v, err := s.h.importSnapshotPHP8512(g.Value, vmGlobalDepth, vmGlobalElements)
			if err != nil {
				return fmt.Errorf("global $%s: %w", g.Name, err)
			}
			err = s.h.assign(&f.locals[i], v)
			if rerr := s.h.release(v); err == nil {
				err = rerr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Limits of one imported global value.
const (
	vmGlobalDepth    = 256
	vmGlobalElements = 1 << 20
)

// stop ends the loop with err classified as PHP-visible or host-level.
func (s *vmState) stop(err error) {
	if s.ended {
		return
	}
	s.ended = true
	var t *Throwable
	var c completion
	var internal *vmInternalFrameThrow
	switch {
	case errors.As(err, &c):
		s.end = c
	case errors.As(err, &internal):
		s.end = s.uncaughtFrom(internal.thrown, internal.function, internal.args)
	case errors.As(err, &t):
		s.end = s.uncaught(t)
	case errors.Is(err, ErrNextElementOccupied):
		s.end = s.uncaught(&Throwable{"Error", err.Error()})
	default:
		var u *UnsupportedError
		var ie *InternalError
		if !errors.As(err, &u) && !errors.As(err, &ie) && !errors.Is(err, errExecutionLimit) && !errors.Is(err, ErrStepLimit) {
			err = s.internal("%v", err)
		}
		kind := endInternal
		if errors.Is(err, errExecutionLimit) || errors.Is(err, ErrStepLimit) {
			kind = endLimit
		}
		s.end = s.x.fail(kind, err)
	}
}

// stopped reports a stop recorded by the execution, such as a failed write
// of a displayed diagnostic, and ends the loop with it.
func (s *vmState) stopped() bool {
	if s.x.stop.kind != endNormal {
		if !s.ended {
			s.ended = true
			s.end = s.x.stop
		}
		return true
	}
	return s.ended
}

// slow runs when the step counter reaches s.next: it enforces the step
// budget before the instruction's effects and polls cancellation.
func (s *vmState) slow() bool {
	if s.steps > s.limit {
		s.steps = s.limit
		s.stop(ErrStepLimit)
		return true
	}
	if s.poll() {
		return true
	}
	s.next = min(s.limit+1, s.steps+vmPollInterval)
	return false
}

// poll observes cancellation and stops recorded by the execution.
func (s *vmState) poll() bool {
	if s.done != nil {
		select {
		case <-s.done:
			s.stop(s.x.fail(endCancel, s.x.ctx.Err()))
			return true
		default:
		}
	}
	return s.stopped()
}

// setReg stores an owned copy of v in register r, releasing the old value.
func (s *vmState) setReg(regs []value, r uint32, v value) error {
	old := regs[r]
	if v.kind >= valueArray {
		retain(v)
	}
	regs[r] = v
	if old.kind >= valueArray {
		return s.h.release(old)
	}
	return nil
}

func (s *vmState) releaseValue(v value) error {
	if v.kind >= valueArray {
		return s.h.release(v)
	}
	return nil
}

func (s *vmState) kill(f *vmFrame, in *vmInstr) error {
	var first error
	for _, r := range f.fn.kills[in.kill : in.kill+uint32(in.nkill)] {
		v := f.regs[r]
		f.regs[r] = value{}
		if err := s.releaseValue(v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// slot is the variable a location register names, or an internal error for
// a register no Local defined on this path.
func (s *vmState) slot(f *vmFrame, r uint32) (*value, error) {
	l := f.locs[r]
	if l <= 0 || int(l) > len(f.locals) {
		return nil, s.internal("location register %d read before Local", r)
	}
	return &f.locals[l-1], nil
}

func (s *vmState) localName(f *vmFrame, r uint32) string {
	if l := f.locs[r]; l > 0 && int(l) <= len(f.fn.localNames) {
		return f.fn.localNames[l-1]
	}
	return ""
}

// push enters fn with the top argc pending arguments. dest is the caller's
// destination register for the return value.
func (s *vmState) push(fn *vmFunction, argc int, dest uint32) error {
	if fn.unsupported != "" {
		return s.unsupported(fn.unsupported)
	}
	x := s.x
	// {main} runs inside the execution's own frame, which counts as one.
	if depth := len(x.frames) - 1 + len(s.frames); depth+1 > x.limits.depth {
		return x.fail(endLimit, limitHit("frames", x.limits.depth, depth, 1))
	}
	if len(s.frames) > 0 {
		if x.calls >= x.limits.callbacks {
			return x.fail(endLimit, limitHit("calls", x.limits.callbacks, x.calls, 1))
		}
		x.calls++
	}
	if argc > len(s.args) {
		return s.internal("call with %d arguments but %d pending", argc, len(s.args))
	}
	n := fn.nvals + fn.locals
	fr := vmFrame{fn: fn, code: fn.code, dest: dest, nargs: argc}
	if s.top+n > len(s.slab) {
		s.growSlab(n)
	}
	fr.base = s.top
	win := s.slab[s.top : s.top+n : s.top+n]
	s.top += n
	fr.regs, fr.locals = win[:fn.nvals:fn.nvals], win[fn.nvals:]
	if s.ltop+fn.nlocs > len(s.lslab) {
		s.growLocations(fn.nlocs)
	}
	fr.lbase = s.ltop
	fr.locs = s.lslab[s.ltop : s.ltop+fn.nlocs : s.ltop+fn.nlocs]
	s.ltop += fn.nlocs
	args := s.args[len(s.args)-argc:]
	for i, a := range args {
		if i < fn.params {
			fr.regs[i] = a // ownership moves from the argument stack
		} else {
			fr.extra = append(fr.extra, a)
		}
	}
	clear(args)
	s.args = s.args[:len(s.args)-argc]
	s.frames = append(s.frames, fr)
	if argc < fn.params {
		return s.tooFewArguments(argc)
	}
	return nil
}

// growSlab reallocates the value slab geometrically and re-slices every
// live frame's window into it. The slab never shrinks during a run.
func (s *vmState) growSlab(n int) {
	size := max(2*len(s.slab), s.top+n, 16)
	slab := make([]value, size)
	copy(slab, s.slab[:s.top])
	s.slab = slab
	for i := range s.frames {
		f := &s.frames[i]
		nv := f.fn.nvals
		win := slab[f.base : f.base+nv+f.fn.locals : f.base+nv+f.fn.locals]
		f.regs, f.locals = win[:nv:nv], win[nv:]
	}
}

func (s *vmState) growLocations(n int) {
	size := max(2*len(s.lslab), s.ltop+n, 16)
	slab := make([]int32, size)
	copy(slab, s.lslab[:s.ltop])
	s.lslab = slab
	for i := range s.frames {
		f := &s.frames[i]
		f.locs = slab[f.lbase : f.lbase+f.fn.nlocs : f.lbase+f.fn.nlocs]
	}
}

// pop releases the top frame's values and removes it. The values are
// released for normal ends, returns, throws and exits, as callInScope does;
// fatal errors and host stops keep them for request teardown.
func (s *vmState) pop(release bool) error {
	f := &s.frames[len(s.frames)-1]
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if release {
		for i := range f.locals {
			v := f.locals[i]
			f.locals[i] = value{}
			keep(s.releaseValue(v))
		}
	}
	for i := range f.regs {
		v := f.regs[i]
		f.regs[i] = value{}
		if release {
			keep(s.releaseValue(v))
		}
	}
	for _, v := range f.extra {
		if release {
			keep(s.releaseValue(v))
		}
	}
	clear(f.locals)
	clear(f.locs)
	s.top = f.base
	s.ltop = f.lbase
	s.frames[len(s.frames)-1] = vmFrame{}
	s.frames = s.frames[:len(s.frames)-1]
	return first
}

// unwind pops every frame after the loop ended.
func (s *vmState) unwind() {
	release := s.end.kind == endNormal || s.end.kind == endReturn || s.end.kind == endThrow || s.end.kind == endExit
	for len(s.frames) > 0 {
		if err := s.pop(release); err != nil && release {
			release = false
			s.stop(err)
		}
	}
	for _, v := range s.args {
		_ = s.releaseValue(v)
	}
	s.args = nil
}

// codeKind maps a Binary aux to its operator spelling for the helpers.
var binaryOps = [...]string{
	bytecode.Add: "+", bytecode.Subtract: "-", bytecode.Multiply: "*", bytecode.Divide: "/", bytecode.Power: "**",
	bytecode.BitAnd: "&", bytecode.BitOr: "|", bytecode.BitXor: "^", bytecode.ShiftLeft: "<<", bytecode.ShiftRight: ">>",
}

// binary is PHP's binary operator op on a and b.
func (s *vmState) binary(op bytecode.BinaryOp, a, b value) (value, error) {
	h := s.h
	switch op {
	case bytecode.Add, bytecode.Subtract, bytecode.Multiply, bytecode.Divide, bytecode.Power:
		return h.arith(binaryOps[op], a, b)
	case bytecode.Modulo:
		return h.mod(a, b)
	case bytecode.Concat:
		if a.kind == valueString && b.kind == valueString {
			return stringValue(a.text + b.text), nil
		}
		return h.concat(a, b)
	case bytecode.Equal, bytecode.NotEqual:
		eq, err := h.looseEqual(a, b)
		return boolValue(eq == (op == bytecode.Equal)), err
	case bytecode.Identical, bytecode.NotIdentical:
		same, err := s.identical(a, b)
		return boolValue(same == (op == bytecode.Identical)), err
	case bytecode.Less, bytecode.LessOrEqual, bytecode.Spaceship:
		c, err := s.compare(a, b)
		if err != nil {
			return value{}, err
		}
		switch op {
		case bytecode.Less:
			return boolValue(c < 0), nil
		case bytecode.LessOrEqual:
			return boolValue(c <= 0), nil
		}
		return h.number(int64(c)), nil
	case bytecode.BitAnd, bytecode.BitOr, bytecode.BitXor:
		return h.bitwise(binaryOps[op], a, b)
	case bytecode.ShiftLeft, bytecode.ShiftRight:
		return h.shift(binaryOps[op], a, b)
	case bytecode.BoolXor:
		// boolean_xor_function converts the left operand first.
		x := h.toBool(a)
		y := h.toBool(b)
		return boolValue(x != y), nil
	}
	return value{}, s.internal("unknown binary operator %d", op)
}

// compare is zend_compare without the per-call recursion guard allocation
// when neither operand can recurse.
func (s *vmState) compare(a, b value) (int, error) {
	if a.kind < valueArray && b.kind < valueArray {
		return s.h.compareIn(a, b, nil)
	}
	return s.h.compare(a, b)
}

func (s *vmState) identical(a, b value) (bool, error) {
	if a.kind != valueArray || b.kind != valueArray {
		if a.kind == valueUndefined {
			a = nullValue()
		}
		if b.kind == valueUndefined {
			b = nullValue()
		}
		return identicalIn(a, b, nil)
	}
	return identical(a, b)
}

// fastBinary computes int and float operations that need no conversion,
// diagnostics or overflow to float; ok is false when the general path is
// needed. It is identical to the helpers for these operand kinds.
func (s *vmState) fastBinary(op bytecode.BinaryOp, a, b value) (value, bool) {
	if a.kind == valueInt && b.kind == valueInt {
		i, j := a.integer, b.integer
		switch op {
		case bytecode.Add:
			r := i + j
			if (r > i) == (j > 0) && s.h.inRange(r) {
				return value{kind: valueInt, integer: r}, true
			}
		case bytecode.Subtract:
			r := i - j
			if (r < i) == (j > 0) && s.h.inRange(r) {
				return value{kind: valueInt, integer: r}, true
			}
		case bytecode.Multiply:
			if r, ok := s.h.mulExact(i, j); ok && s.h.inRange(r) {
				return value{kind: valueInt, integer: r}, true
			}
		case bytecode.Less:
			return boolValue(i < j), true
		case bytecode.LessOrEqual:
			return boolValue(i <= j), true
		case bytecode.Equal, bytecode.Identical:
			return boolValue(i == j), true
		case bytecode.NotEqual, bytecode.NotIdentical:
			return boolValue(i != j), true
		case bytecode.Spaceship:
			return value{kind: valueInt, integer: int64(compareInts(i, j))}, true
		case bytecode.BitAnd:
			return value{kind: valueInt, integer: i & j}, true
		case bytecode.BitOr:
			return value{kind: valueInt, integer: i | j}, true
		case bytecode.BitXor:
			return value{kind: valueInt, integer: i ^ j}, true
		case bytecode.Modulo:
			if j != 0 && j != -1 {
				return value{kind: valueInt, integer: i % j}, true
			}
		}
		return value{}, false
	}
	if a.kind == valueFloat && b.kind == valueFloat {
		f, g := a.floating, b.floating
		switch op {
		case bytecode.Add:
			return floatValue(f + g), true
		case bytecode.Subtract:
			return floatValue(f - g), true
		case bytecode.Multiply:
			return floatValue(f * g), true
		}
	}
	return value{}, false
}

// output writes the string form of v.
func (s *vmState) output(v value) completion {
	switch v.kind {
	case valueString:
		s.scratch = append(s.scratch[:0], v.text...)
	case valueInt:
		s.scratch = strconv.AppendInt(s.scratch[:0], v.integer, 10)
	default:
		str, err := s.h.toString(v)
		if err != nil {
			s.stop(err)
			return s.end
		}
		if s.stopped() {
			return s.end
		}
		s.scratch = append(s.scratch[:0], str...)
	}
	c := s.x.write(s.scratch)
	if cap(s.scratch) > 1<<16 {
		s.scratch = nil
	}
	return c
}

// callValue resolves a string callable naming a program function.
func (s *vmState) callValue(callable value, argc int, dest uint32) error {
	if callable.kind != valueString {
		return s.unsupported("calling a non-string callable")
	}
	name := strings.TrimPrefix(callable.text, "\\")
	idx, ok := s.prog.byName[strings.ToLower(name)]
	if !ok {
		// PHP throws when the name is resolved, before any argument is
		// sent; the pending arguments are discarded.
		if err := s.dropArgs(argc); err != nil {
			return err
		}
		return &Throwable{"Error", "Call to undefined function " + name + "()"}
	}
	return s.push(&s.prog.funcs[idx], argc, dest)
}

func (s *vmState) dropArgs(argc int) error {
	if argc > len(s.args) {
		return s.internal("call with %d arguments but %d pending", argc, len(s.args))
	}
	var first error
	for _, v := range s.args[len(s.args)-argc:] {
		if err := s.releaseValue(v); err != nil && first == nil {
			first = err
		}
	}
	clear(s.args[len(s.args)-argc:])
	s.args = s.args[:len(s.args)-argc]
	return first
}

// tooFewArguments is zend_missing_arg_error, raised in the callee frame at
// its first instruction (the declaration line), as PHP's RECV does.
func (s *vmState) tooFewArguments(argc int) error {
	callee := &s.frames[len(s.frames)-1]
	caller := &s.frames[len(s.frames)-2]
	return &Throwable{"ArgumentCountError", fmt.Sprintf("Too few arguments to function %s(), %d passed in %s on line %d and exactly %d expected",
		callee.fn.name, argc, vmFileName(caller.fn.files[caller.pc]), caller.fn.lines[caller.pc], callee.fn.params)}
}

// ret returns v (owned) from the top frame to its caller.
func (s *vmState) ret(v value) error {
	f := &s.frames[len(s.frames)-1]
	dest := f.dest
	err := s.pop(true)
	caller := &s.frames[len(s.frames)-1]
	old := caller.regs[dest]
	caller.regs[dest] = v
	if rerr := s.releaseValue(old); err == nil {
		err = rerr
	}
	return err
}

// take moves the value out of register r, defined or not, as an owned value;
// an undefined register yields null, as PHP's return of an unset value.
func take(regs []value, r uint32) value {
	v := regs[r]
	regs[r] = value{}
	if v.kind == valueUndefined {
		return nullValue()
	}
	return v
}
