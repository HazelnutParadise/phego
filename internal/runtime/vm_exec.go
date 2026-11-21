package runtime

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// vmPollInterval bounds the straight-line instructions between two
// cancellation polls. Back-edges, calls, returns and Checkpoint always poll.
const vmPollInterval = 1024

// vmFrame is one PHP call. regs, locals and locs are windows of the
// execution's slabs.
// A location register holds local index + 1 for a variable, -(index + 1)
// for an element in elems, and zero for "not defined".
type vmFrame struct {
	fn     *vmFunction
	code   []vmInstr
	pc     int
	regs   []value
	locals []value
	locs   []int32
	// elems holds the elements FetchDim found for the frame's element
	// locations; a location register names one as -(index + 1).
	elems []*value
	// freshV, freshL and freshE say the frame's window opened a new chunk
	// of the value, location and element stacks (vmStack).
	freshV, freshL, freshE bool
	// boundary marks the frame a nested run (vmState.invoke) entered: its
	// Return ends that run instead of resuming a caller in the same loop.
	boundary bool
	// ctor marks a constructor's frame until it returns: a frame popped
	// with it still set did not complete, and its object is then never
	// destructed.
	ctor bool
	// weak marks a frame the runtime entered itself: its arguments are
	// converted in weak mode whatever the caller's strict_types.
	weak  bool
	dest  uint32
	nargs int
	// this is the object of a method call (an owner), scope the class the
	// code was declared in or used by (the lexical scope of self and of
	// visibility checks) and called the class of late static binding. All
	// are zero for a plain function.
	this   *object
	scope  *class
	called *class
	// closure is the Closure object a closure's frame runs for (an owner):
	// it holds the frame's static variables and outlives the call even
	// when the last variable naming it is overwritten meanwhile.
	closure *object
	// via is the method of the Closure class the frame was entered
	// through (closureCall, closureInvoke) or zero: a backtrace shows that
	// method's frame below this one.
	via uint8
	// unreceived marks a frame whose call passed too few arguments: its pc
	// names the parameter that is missing, but no prologue Store has run.
	unreceived bool
	extra      []value // arguments beyond the declared parameters, for backtraces
	// call is set for a frame CallPending entered with named arguments the
	// variadic parameter collects, or with omitted parameters.
	call *vmCallInfo
}

type vmState struct {
	prog *Program
	x    *execution
	h    *heap
	// frames are the live frames, innermost last. Each is the frame object
	// of its depth in frameObjs, so a *vmFrame stays valid while PHP code
	// that an instruction calls (a destructor, a magic method) pushes and
	// pops frames above it. The three stacks hand out windows that never
	// move for the same reason.
	frames    []*vmFrame
	frameObjs []*vmFrame
	vals      vmStack[value]
	locs      vmStack[int32]
	elems     vmStack[*value]
	// defaults is RECV_INIT's run-time cache: per function index, per
	// parameter, the default value an earlier call evaluated and PHP keeps
	// (vmDefaultCacheable). Allocated on first use; it lives with the run,
	// never in the shared Program.
	defaults [][]vmCachedDefault
	// refIters are the run's foreach-by-reference iterations; an iterator
	// register names one (vmRefIter). freeIters are reusable indexes.
	refIters  []vmRefIter
	freeIters []int
	args      []value
	// calls are the calls InitCall started and CallPending has not made
	// yet, innermost last. entering hands CallPending's extra knowledge of
	// a call to push.
	calls    []vmPendingCall
	entering *vmCallInfo
	// statics holds the run's static variables: per function index, per
	// local, a reference once the variable is initialized. globalVars are
	// the global variables {main} has no local for. Both live with the
	// run, never in the shared Program.
	statics      [][]value
	traitStatics map[vmTraitStatic][]value
	globalVars   map[string]*value
	// reporting is the run's error_reporting, EG(error_reporting): the
	// INI value, lowered to the fatal levels inside @ (BeginSilence) and
	// restored after it (EndSilence). x.errors.excluded mirrors it.
	reporting int
	// consts holds the constants the run declared with const, by registered
	// name (the namespace part lower-cased). They never enter the shared
	// Program; the predefined ones are in internal/phpconst.
	consts map[string]value
	// constCache is ZEND_FETCH_CONSTANT\'s run-time cache slot per
	// instruction (function index and pc): the first resolution of a
	// non-deprecated name is kept for the rest of the run, so a namespaced
	// constant declared later does not take over an unqualified name that
	// already fell back to the global one.
	constCache map[uint64]value
	// Format 1.7. declared are the classes the run declared and classCache
	// the classes its instructions resolved by name, per constant index.
	// into is what the next push gives its frame. floor is the frame depth
	// at which the current loop returns to Go (1 for {main}) and nesting
	// the number of loops running. fetchScratch holds the value a FetchProp
	// names when the property has no location of its own.
	declared     []*vmClass
	classCache   []*class
	into         vmEnter
	floor        int
	nesting      int
	fetchScratch value
	// caches are the run's inline caches (vmCache), allocated on first use.
	caches []vmCache
	// typedHole stands for a typed property that holds null or nothing and
	// whose type takes no array, while a FetchProp or FetchStatic names
	// it: writing an element through it throws.
	typedHole     value
	typedHoleProp *propertyInfo
	// traceHeld are the objects the backtrace of the uncaught error holds
	// (an owner each) until the error is reported.
	traceHeld []value
	// globalsKept says {main}'s variables were handed to the execution as
	// its symbol table (keepGlobals).
	globalsKept bool
	// linking is the class whose declaration is being checked.
	linking *class
	steps   int64
	limit   int64
	next    int64 // steps at which the next slow check runs
	done    <-chan struct{}
	scratch []byte
	globals []Global
	prelude []RunDiagnostic
	end     completion
	ended   bool

	// offsetBuf holds the bytes of offsetLast, the string the last string
	// offset write built, when offsetValid is set, so that a write
	// extending it appends instead of copying (offsetString).
	offsetBuf   strings.Builder
	offsetLast  string
	offsetValid bool

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
	s.reporting = cfg.ErrorReporting
	x.errors = errorChannels{display: cfg.DisplayErrors, log: cfg.LogErrors, where: s.where, excluded: ^cfg.ErrorReporting}
	x.heap.sink = s.warn
	x.heap.vm = s
	s.floor = 1
	return s
}

// where is the current PHP location for diagnostics.
func (s *vmState) where() (string, int) {
	if len(s.frames) == 0 {
		return s.lastFile, s.lastLine
	}
	f := s.frames[len(s.frames)-1]
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
	f := s.frames[len(s.frames)-1]
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
	if err := s.declareHoisted(); err != nil {
		s.stop(err)
		return false
	}
	return true
}

// bindGlobals imports host values into the {main} locals of the same name.
func (s *vmState) bindGlobals(main *vmFunction) error {
	f := s.frames[0]
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
	var fatal *vmFatal
	var located *vmUncaught
	switch {
	case errors.As(err, &c) && c.kind == endThrow && !errors.As(c.err, &located) && errors.As(c.err, &t):
		// A throwable the runtime wrapped on its way out of a callback: it
		// is reported like any other, from where the instruction is.
		s.end = s.uncaught(t)
	case errors.As(err, &c):
		s.end = c
	case errors.As(err, &fatal):
		_, _, s.lastFile, s.lastLine = s.location()
		if fatal.line > 0 {
			s.lastLine = fatal.line
		}
		s.x.errors.trailer = "Stack trace:\n" + s.backtrace("", nil) + "\n"
		s.end = s.x.fail(endFatal, errors.New(fatal.message))
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
		return s.releaseValue(old)
	}
	return nil
}

func (s *vmState) releaseValue(v value) error {
	if v.kind == valueReference && v.integer != 0 {
		s.endRefIter(v)
	}
	if v.kind >= valueArray {
		return s.h.release(v)
	}
	return nil
}

func (s *vmState) kill(f *vmFrame, in *vmInstr) error {
	var first error
	for _, r := range f.fn.kills[in.kill : in.kill+uint32(in.nkill)] {
		v := f.regs[r]
		if v.kind < valueArray {
			continue // a scalar holds nothing; the register is rewritten before it is read
		}
		f.regs[r] = value{}
		if err := s.releaseValue(v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// slot is the variable or the element a location register names, or an
// internal error for a register no Local or FetchDim defined on this path.
func (s *vmState) slot(f *vmFrame, r uint32) (*value, error) {
	l := f.locs[r]
	if l <= 0 || int(l) > len(f.locals) {
		return s.elementSlot(f, r)
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
	fr := s.newFrame()
	into := s.into
	if into != (vmEnter{}) {
		s.into = vmEnter{}
	}
	// Field by field: the frame object lives on the heap, and assigning
	// the whole struct goes through the garbage collector's bulk write
	// barrier on every call.
	fr.fn, fr.code, fr.pc, fr.dest, fr.nargs = fn, fn.code, 0, dest, argc
	if fr.this != into.this || fr.scope != into.scope || fr.called != into.called {
		fr.this, fr.scope, fr.called = into.this, into.scope, into.called
	}
	fr.boundary, fr.unreceived = false, false
	if fr.closure != into.closure {
		fr.closure = into.closure
	}
	if fr.via != into.via {
		fr.via = into.via
	}
	if s.h.currentCalledScope != into.called {
		s.h.currentCalledScope = into.called
	}
	// The windows, with the common case (room in the current chunk) written
	// out: a generic method call here costs a recursive PHP function
	// several percent.
	var win []value
	if top := s.vals.top + fn.nvals + fn.locals; top <= len(s.vals.cur) {
		win = s.vals.cur[s.vals.top:top:top]
		s.vals.top = top
	} else {
		win, fr.freshV = s.vals.grow(fn.nvals+fn.locals), true
	}
	fr.regs, fr.locals = win[:fn.nvals:fn.nvals], win[fn.nvals:]
	if top := s.locs.top + fn.nlocs; top <= len(s.locs.cur) {
		fr.locs = s.locs.cur[s.locs.top:top:top]
		s.locs.top = top
	} else {
		fr.locs, fr.freshL = s.locs.grow(fn.nlocs), true
	}
	if top := s.elems.top + fn.nelems; top <= len(s.elems.cur) {
		fr.elems = s.elems.cur[s.elems.top:top:top]
		s.elems.top = top
	} else {
		fr.elems, fr.freshE = s.elems.grow(fn.nelems), true
	}
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
	if len(fn.captures) != 0 && into.closure != nil {
		s.bindCaptures(fr, into.closure.closure)
	}
	if e := s.entering; e != nil {
		s.entering = nil
		fr.call = e
		if e.missing {
			// CallPending reports the missing parameter in this frame.
			s.frames = append(s.frames, fr)
			return nil
		}
	}
	s.frames = append(s.frames, fr)
	fr.weak, fr.ctor = into.internal || into.via != 0, into.ctor
	if argc < fn.required {
		// ZEND_RECV checks each parameter in order: the type of an
		// argument before a missing one is reported first.
		if fn.typed {
			if err := s.checkArguments(fr, argc); err != nil {
				return err
			}
		}
		return s.tooFewArguments(argc)
	}
	return nil
}

// newFrame is the frame object of the next depth. The objects are allocated
// in blocks and reused, so entering a function allocates nothing.
func (s *vmState) newFrame() *vmFrame {
	d := len(s.frames)
	if d == len(s.frameObjs) {
		block := make([]vmFrame, max(16, min(d, 1024)))
		for i := range block {
			s.frameObjs = append(s.frameObjs, &block[i])
		}
	}
	return s.frameObjs[d]
}

// vmStack hands out consecutive windows of fixed chunks. A window never
// moves: when the current chunk is full a larger one is started, and the
// frame that opened it gives it back.
type vmStack[T any] struct {
	cur   []T
	top   int
	prev  []vmStackMark[T]
	spare []T
}

type vmStackMark[T any] struct {
	cur []T
	top int
}

// grow starts a chunk that holds at least n elements.
//
//go:noinline
func (k *vmStack[T]) grow(n int) []T {
	k.prev = append(k.prev, vmStackMark[T]{k.cur, k.top})
	if size := max(2*len(k.cur), n, 16); len(k.spare) >= max(n, len(k.cur)+1) {
		k.cur, k.spare = k.spare, nil
	} else {
		k.cur = make([]T, size)
	}
	k.top = n
	return k.cur[:n:n]
}

// shrink goes back to the chunk before the newest one, after the frame
// that opened it gave its window back.
//
//go:noinline
func (k *vmStack[T]) shrink() {
	m := k.prev[len(k.prev)-1]
	k.prev[len(k.prev)-1] = vmStackMark[T]{}
	k.prev = k.prev[:len(k.prev)-1]
	k.spare = k.cur
	k.cur, k.top = m.cur, m.top
}

// pop releases the top frame's values and removes it. The values are
// released for normal ends, returns, throws and exits, as callInScope does;
// fatal errors and host stops keep them for request teardown.
func (s *vmState) pop(release bool) error {
	f := s.frames[len(s.frames)-1]
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
	if f.call != nil && release {
		keep(s.releaseValue(f.call.named))
	}
	if len(s.calls) > 0 {
		// Calls the frame started and never made: an error or exit ended
		// it between their arguments.
		err := s.dropCalls(len(s.frames))
		if release {
			keep(err)
		}
	}
	this := f.this
	if this != nil {
		f.this = nil
		if f.ctor {
			this.destructorCalled = true
		}
	}
	clear(f.locals)
	clear(f.locs)
	clear(f.elems)
	s.vals.top -= len(f.regs) + len(f.locals)
	s.locs.top -= len(f.locs)
	s.elems.top -= len(f.elems)
	if f.freshV {
		s.vals.shrink()
	}
	if f.freshL {
		s.locs.shrink()
	}
	if f.freshE {
		s.elems.shrink()
	}
	// The frame object is reused. push writes every field it reads; only
	// what could keep a value alive is cleared here, and only when set, so
	// a plain call writes no pointer on its way out.
	if f.extra != nil {
		f.extra = nil
	}
	if f.call != nil {
		f.call = nil
	}
	f.freshV, f.freshL, f.freshE = false, false, false
	s.frames = s.frames[:len(s.frames)-1]
	var outer *class
	if n := len(s.frames); n > 0 {
		outer = s.frames[n-1].called
	}
	if s.h.currentCalledScope != outer {
		s.h.currentCalledScope = outer
	}
	// $this is released after the frame's variables (i_free_compiled_variables
	// runs before OBJ_RELEASE(ZEND_THIS) in zend_leave_helper).
	if release && this != nil {
		keep(s.releaseObject(this))
	}
	// The closure goes last (zend_leave_helper: OBJ_RELEASE of the closure
	// after the variables).
	if cl := f.closure; cl != nil {
		f.closure = nil
		if release {
			keep(s.releaseObject(cl))
		}
	}
	return first
}

// unwind pops every frame after the loop ended.
func (s *vmState) unwind() {
	release := s.end.kind == endNormal || s.end.kind == endReturn || s.end.kind == endThrow || s.end.kind == endExit
	if release && len(s.frames) > 0 {
		s.keepGlobals()
	}
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
	// What else the run holds (constants, static variables, the classes'
	// constants and static properties) is storage PHP frees after every
	// destructor ran: once the run declared a class it is left to the
	// request's teardown, which releases it without calling PHP code.
	if release && len(s.declared) == 0 {
		for name, v := range s.consts {
			delete(s.consts, name)
			_ = s.releaseValue(v)
		}
		for slot, v := range s.constCache {
			delete(s.constCache, slot)
			_ = s.releaseValue(v)
		}
		for name, p := range s.globalVars {
			delete(s.globalVars, name)
			_ = s.releaseValue(*p)
		}
		for _, slots := range s.statics {
			for i, v := range slots {
				slots[i] = value{}
				_ = s.releaseValue(v)
			}
		}
	}
	s.globalVars = nil
	s.statics = nil
	s.consts = nil
	s.constCache = nil
}

// codeKind maps a Binary aux to its operator spelling for the helpers.
var binaryOps = [...]string{
	bytecode.Add: "+", bytecode.Subtract: "-", bytecode.Multiply: "*", bytecode.Divide: "/", bytecode.Power: "**",
	bytecode.BitAnd: "&", bytecode.BitOr: "|", bytecode.BitXor: "^", bytecode.ShiftLeft: "<<", bytecode.ShiftRight: ">>",
}

// vmCopyChunk is how many bytes of a large string are copied between two
// cancellation polls.
const vmCopyChunk = 4 << 20

// join is a . b for two strings. A result larger than vmCopyChunk is built
// in chunks with a cancellation poll between them, so a run that copies
// very large strings still ends promptly; when the run was stopped the
// result is unused.
func (s *vmState) join(a, b string) value {
	if len(a)+len(b) <= vmCopyChunk {
		return stringValue(a + b)
	}
	if s.poll() {
		return nullValue()
	}
	var out strings.Builder
	out.Grow(len(a) + len(b))
	for _, part := range [2]string{a, b} {
		for len(part) > 0 {
			n := min(len(part), vmCopyChunk)
			out.WriteString(part[:n])
			part = part[n:]
			if len(part) > 0 && s.poll() {
				return nullValue()
			}
		}
	}
	return stringValue(out.String())
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
			return s.join(a.text, b.text), nil
		}
		// As heap.concat, with the copy made by join so that it can be
		// cancelled whatever the operand types are.
		if err := h.claimValues(a, b); err != nil {
			return value{}, err
		}
		x, err := h.toString(a)
		if err != nil {
			return value{}, err
		}
		y, err := h.toString(b)
		if err != nil {
			return value{}, err
		}
		return s.join(x, y), nil
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
	callable = callable.deref()
	if callable.kind == valueObject {
		return s.callObject(callable, argc, dest)
	}
	if callable.kind != valueString {
		return s.unsupported("calling a non-string callable")
	}
	name := strings.TrimPrefix(callable.text, "\\")
	idx, ok := s.prog.byName[phpname.Lower(name)]
	if !ok {
		// PHP throws when the name is resolved, before any argument is
		// sent; the pending arguments are discarded.
		if err := s.dropArgs(argc); err != nil {
			return err
		}
		return &Throwable{"Error", "Call to undefined function " + name + "()"}
	}
	fn := &s.prog.funcs[idx]
	if fn.paramByRef != nil {
		if argc > len(s.args) {
			return s.internal("call with %d arguments but %d pending", argc, len(s.args))
		}
		if err := s.derefArguments(fn, argc); err != nil {
			return err
		}
	}
	return s.push(fn, argc, dest)
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

// tooFewArguments is zend_missing_arg_error, raised in the callee frame by
// the RECV of the first missing parameter: the frame stands at the prologue
// instruction that receives it, which carries the parameter's line. A
// function with optional parameters expects "at least" its required ones.
func (s *vmState) tooFewArguments(argc int) error {
	callee := s.frames[len(s.frames)-1]
	if len(s.frames) < 2 {
		return &Throwable{"ArgumentCountError", "Too few arguments to function " + s.funcName(callee) + "(), " + strconv.Itoa(argc) + " passed and at least " + strconv.Itoa(callee.fn.required) + " expected"}
	}
	if callee.via != 0 {
		// Called by an internal function (a method of Closure): there is
		// no PHP call site to name.
		callee.unreceived = true
		if pc := callee.fn.paramPC[argc]; pc >= 0 {
			callee.pc = int(pc)
		}
		bound := "exactly"
		if callee.fn.required < callee.fn.params {
			bound = "at least"
		}
		return &Throwable{"ArgumentCountError", fmt.Sprintf("Too few arguments to function %s(), %d passed and %s %d expected", s.funcName(callee), argc, bound, callee.fn.required)}
	}
	caller := s.frames[len(s.frames)-2]
	callee.unreceived = true
	if pc := callee.fn.paramPC[argc]; pc >= 0 {
		callee.pc = int(pc)
	}
	bound := "exactly"
	if callee.fn.required < callee.fn.params {
		bound = "at least"
	}
	return &Throwable{"ArgumentCountError", fmt.Sprintf("Too few arguments to function %s(), %d passed in %s on line %d and %s %d expected",
		s.funcName(callee), argc, vmFileName(caller.fn.files[caller.pc]), caller.fn.lines[caller.pc], bound, callee.fn.required)}
}

// ret returns v (owned) from the top frame to its caller.
func (s *vmState) ret(v value) error {
	f := s.frames[len(s.frames)-1]
	dest := f.dest
	f.ctor = false // the constructor completed
	err := s.pop(true)
	caller := s.frames[len(s.frames)-1]
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

// keepGlobals hands the variables of {main} to the execution as PHP's
// global symbol table, in the order they were created, when the run used
// objects: request shutdown destroys the objects only a global variable
// holds first, in reverse order, and then calls every other destructor
// (zend_call_destructors). Without a class nothing can observe the order,
// and the variables are released with the frame.
func (s *vmState) keepGlobals() {
	if s.h.liveObjects == 0 || s.frames[0].fn != &s.prog.funcs[s.prog.entry] {
		return
	}
	f := s.frames[0]
	globals := s.h.newArrayValue()
	retain(globals)
	for i := range f.locals {
		v := f.locals[i]
		if v.kind == valueUndefined || f.fn.localNames[i] == "" {
			continue
		}
		f.locals[i] = value{}
		globals.array.insert(arrayKey{isString: true, text: f.fn.localNames[i]}, v, s.prog.width)
	}
	for name, p := range s.globalVars {
		if p.kind != valueUndefined {
			globals.array.insert(arrayKey{isString: true, text: name}, *p, s.prog.width)
			*p = value{}
		}
	}
	s.x.globals = globals
	s.globalsKept = true
}
