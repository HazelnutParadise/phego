package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// W-14: REQ-009, REQ-020, REQ-022, REQ-030, REQ-040, REQ-044; T-017, T-018, T-039, T-040, T-043, T-044, T-059, T-060, T-079, T-080, T-087, T-088.
// This request-local core is not a PHP interpreter. The bytecode VM will supply
// actions. It uses the W-06 heap and W-05 Request without changing public SDK APIs.
type endKind uint8

const (
	endNormal endKind = iota
	endReturn
	endThrow
	endFatal
	endExit
	endCancel
	endOutput
	endInternal
	endLimit
)

type completion struct {
	serial   uint64 // Identifies a new throw/exit; forwarding preserves its order.
	kind     endKind
	code     int
	value    value
	err      error
	previous error
	// An owned return moves with its completion. Copying the Go struct does
	// not create another owner; the receiver must consume or forward it once.
	returnOwned       bool
	returnByReference bool
}

func (c completion) Error() string {
	if c.err != nil {
		return c.err.Error()
	}
	return "PHP execution stopped"
}

func (c completion) Unwrap() []error { return []error{c.err, c.previous} }

type executionAction func(*execution) completion

// outputFilter returns already-adapted output bytes. A successful PHP callback
// return must be converted to out by the VM adapter and report endNormal here;
// the completion channel carries outward termination, not the output string.
type outputFilter func(*execution, []byte) ([]byte, completion)
type executionLimits struct{ depth, callbacks, output int }
type executionFrame struct{ locals []*value }
type executionBuffer struct {
	data   []byte
	filter outputFilter
	name   string // PHP's callable name of filter, used in its diagnostics
}
type executionDiagnostic struct{ level, message string }

type executionResult struct {
	end             completion
	returned        any
	output          []byte
	accepted        int64
	diagnostics     []executionDiagnostic
	cleanupErr      error
	cleanupComplete bool
}

var errExecutionReentry = errors.New("execution already started")
var errExecutionLimit = errors.New("execution limit exceeded")

// execution has one logical owner. Context cancellation is the only concurrent
// input; actions and output writers must cooperate with cancellation. Cleanup
// uses a separate deadline-bearing context supplied by the host.
type execution struct {
	ctx                       context.Context
	cleanupContext            context.Context
	request                   *Request
	heap                      *heap
	globals                   value
	limits                    executionLimits
	writer                    io.Writer
	frames                    []*executionFrame
	shutdown                  []executionAction
	buffers                   []executionBuffer
	output                    []byte
	accepted                  int64
	diagnostics               []executionDiagnostic
	started, closed           bool
	filtering, handlerOutput  bool
	destructorStop            bool
	result                    executionResult
	stop                      completion
	phaseFatal                completion
	phaseCompletion, lastExit completion
	serial                    uint64
	calls                     int
}

func newExecution(ctx context.Context, request *Request, width int, writer io.Writer, limits executionLimits) (*execution, error) {
	if ctx == nil || request == nil || request.State() != Ready {
		return nil, errors.New("execution requires a context and a Ready request")
	}
	if limits.depth <= 0 || limits.callbacks <= 0 || limits.output <= 0 {
		return nil, errors.New("execution limits must be positive")
	}
	h, err := newHeap(width)
	if err != nil {
		return nil, err
	}
	x := &execution{ctx: ctx, request: request, heap: h, globals: nullValue(), writer: writer, limits: limits}
	h.execution = x
	return x, nil
}

// run starts a Ready request exactly once and always performs Runtime cleanup.
// A repeated or reentrant call is rejected without running any action.
func (x *execution) run(cleanup context.Context, body executionAction) executionResult {
	if x.started {
		return executionResult{end: completion{kind: endInternal, err: errExecutionReentry}}
	}
	if cleanup == nil || body == nil {
		return executionResult{end: completion{kind: endInternal, err: errors.New("body and cleanup context are required")}}
	}
	if _, ok := cleanup.Deadline(); !ok {
		return executionResult{end: completion{kind: endInternal, err: errors.New("cleanup requires a deadline")}}
	}
	if err := x.request.Start(); err != nil {
		return executionResult{end: completion{kind: endInternal, err: err}}
	}
	x.started = true
	x.cleanupContext = cleanup
	c := x.call(body, nil)
	var returned any
	if c.kind == endReturn {
		var err error
		returned, err = exportSnapshot(c.value)
		if err != nil {
			c = x.replaceOwnedCompletion(c, x.fail(endInternal, err))
		} else if released := x.releaseReturn(&c); released.kind != endNormal {
			c = x.replaceOwnedCompletion(c, released)
		}
	}
	if stop := x.check(); stop.kind != endNormal {
		c = x.replaceOwnedCompletion(c, stop)
	}
	x.recordEnd(c)
	if !stopsUserCode(c) {
		for i := 0; i < len(x.shutdown); i++ {
			next := x.call(x.shutdown[i], nil)
			if released := x.releaseReturn(&next); released.kind != endNormal {
				next = x.replaceOwnedCompletion(next, released)
			}
			if stop := x.check(); stop.kind != endNormal {
				next = x.replaceOwnedCompletion(next, stop)
			}
			if next.kind != endNormal && next.kind != endReturn {
				c = x.replaceOwnedCompletion(c, next)
				x.recordEnd(c)
				break
			}
		}
	}
	// Host stops suppress all PHP callbacks. Fatal errors mark only objects
	// already alive at the error; later objects may still destruct.
	if stopsUserCode(c) {
		x.destructorStop = true
	}
	if next := x.destruct(); next.kind != endNormal {
		c = x.replaceOwnedCompletion(c, next)
		x.recordEnd(c)
	}
	if !stopsUserCode(c) {
		// Output handlers may create and destroy new locals, but must not
		// restart a failed destructor pass on surviving old objects.
		for _, o := range x.heap.live {
			o.destructorCalled = true
		}
		x.destructorStop = false
		if next := x.flush(); next.kind != endNormal {
			c = x.replaceOwnedCompletion(c, next)
			x.recordEnd(c)
		}
	}
	// Output callbacks still need a live heap. Objects retained by those
	// callbacks get storage release, not a second shutdown destructor pass.
	x.destructorStop = true
	for _, o := range x.heap.live {
		o.destructorCalled = true
	}
	if err := x.heap.shutdown(&x.globals); err != nil {
		c = x.replaceOwnedCompletion(c, x.fail(endInternal, err))
	}
	x.buffers = nil
	x.shutdown = nil
	x.frames = nil
	x.closed = true
	if c.kind == endThrow || c.kind == endFatal {
		c.code = 255
	} else if x.lastExit.kind == endExit && (c.kind == endNormal || c.kind == endReturn || c.kind == endExit) {
		c.code = x.lastExit.code
	}
	// Never let the completion expose a PHP value after heap teardown.
	c.value = value{}
	var transitionErr error
	if c.kind == endNormal || c.kind == endReturn || c.kind == endExit {
		transitionErr = x.request.Finish()
	} else {
		transitionErr = x.request.Abort()
	}
	resourceErr := x.heap.closeResources(cleanup)
	cleanupErr := errors.Join(resourceErr, transitionErr, x.request.Close(cleanup))
	x.result = executionResult{end: c, returned: returned, output: append([]byte(nil), x.output...), accepted: x.accepted, diagnostics: append([]executionDiagnostic(nil), x.diagnostics...), cleanupErr: cleanupErr, cleanupComplete: x.request.State() == Closed}
	return x.result
}

// call owns local values until return/throw/exit unwinds them. Exit bypasses
// PHP finally; fatal and host stops retain live values for request teardown.
func (x *execution) call(body, finally executionAction) completion {
	return x.callInScope(body, finally, nil)
}

func (x *execution) callInScope(body, finally executionAction, called *class) completion {
	if body == nil || x.closed {
		return completion{kind: endInternal, err: errors.New("invalid execution action")}
	}
	if c := x.check(); c.kind != endNormal {
		return c
	}
	if len(x.frames) >= x.limits.depth || x.calls >= x.limits.callbacks {
		return x.fail(endLimit, errExecutionLimit)
	}
	x.calls++
	previousHook := x.heap.currentHook
	previousScope := x.heap.currentCalledScope
	x.heap.currentHook = activePropertyHook{}
	x.heap.currentCalledScope = called
	defer func() { x.heap.currentHook, x.heap.currentCalledScope = previousHook, previousScope }()
	f := &executionFrame{}
	x.frames = append(x.frames, f)
	enclosingPending := x.phaseCompletion
	c := ownReturn(x.invoke(body))
	if stop := x.check(); stop.kind != endNormal {
		c = x.replaceOwnedCompletion(c, stop)
	}
	if (c.kind == endNormal || c.kind == endReturn || c.kind == endThrow) && finally != nil {
		if x.calls >= x.limits.callbacks {
			c = x.replaceOwnedCompletion(c, x.fail(endLimit, errExecutionLimit))
		} else {
			x.calls++
			if next := x.invoke(finally); next.kind != endNormal {
				if c.kind == endThrow && next.kind == endReturn {
					// Finally suppressed this frame's throw. Restore only the
					// enclosing event before return replacement or local cleanup.
					x.phaseCompletion = enclosingPending
				}
				c = x.replaceOwnedCompletion(c, next)
			}
		}
		if stop := x.check(); stop.kind != endNormal {
			c = x.replaceOwnedCompletion(c, stop)
		}
	}
	if c.kind == endNormal || c.kind == endReturn || c.kind == endThrow || c.kind == endExit {
		for i := 0; i < len(f.locals); i++ {
			before := x.serial
			if err := x.heap.unset(f.locals[i]); err != nil {
				var next completion
				if errors.As(err, &next) {
					c = x.replaceOwnedCompletion(c, next)
				} else {
					c = x.replaceOwnedCompletion(c, x.fail(endInternal, err))
				}
			}
			c = x.orderedResult(c, before)
			if stop := x.check(); stop.kind != endNormal {
				c = x.replaceOwnedCompletion(c, stop)
			}
			if c.kind == endFatal || stopsUserCode(c) {
				break
			}
		}
	}
	// freeArray preserves its first PHP error; a later destructor may still
	// have set a phase fatal, cancellation, writer error, or resource stop.
	if stop := x.check(); stop.kind != endNormal {
		c = x.replaceOwnedCompletion(c, stop)
	}
	x.frames = x.frames[:len(x.frames)-1]
	return c
}

// tryCatch is a control-flow boundary within the current PHP frame. The adapter
// selects matching Throwable classes before returning a handler; nil rethrows.
// Handling restores the pending completion from before try, before user code runs.
func (x *execution) tryCatch(body executionAction, handler func(completion) executionAction) completion {
	if stop := x.check(); stop.kind != endNormal {
		return stop
	}
	pending := x.phaseCompletion
	c := x.invoke(body)
	if stop := x.check(); stop.kind != endNormal {
		return x.replaceOwnedCompletion(c, stop)
	}
	if c.kind != endThrow || handler == nil {
		return c
	}
	fn := handler(c)
	if fn == nil {
		return c
	}
	x.phaseCompletion = pending
	return x.invoke(fn)
}

func replaceCompletion(old, next completion) completion {
	if old.kind == endThrow && next.kind == endThrow &&
		(old.serial == 0 || old.serial != next.serial) && !containsCompletion(next.previous, old.serial) {
		next.previous = errors.Join(next.previous, old)
	}
	return next
}

// Identity is the completion serial, not error text or a user-defined Is method.
func containsCompletion(err error, serial uint64) bool {
	if err == nil || serial == 0 {
		return false
	}
	if c, ok := err.(completion); ok && c.serial == serial {
		return true
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range e.Unwrap() {
			if containsCompletion(child, serial) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		return containsCompletion(e.Unwrap(), serial)
	}
	return false
}

// REQ-020, REQ-022; T-039, T-040, T-043, T-044. A by-value return acquires
// its own logical owner before finally can unset or replace the source slot.
// Nested calls transfer an existing return owner without retaining it again.
func ownReturn(c completion) completion {
	if c.kind == endReturn && !c.returnOwned {
		if !c.returnByReference {
			c.value = c.value.deref()
		}
		retain(c.value)
		c.returnOwned = true
	}
	return c
}

func (x *execution) releaseReturn(c *completion) completion {
	if !c.returnOwned {
		return completion{}
	}
	v := c.value
	before := x.serial
	c.value, c.returnOwned = value{}, false
	if err := x.heap.release(v); err != nil {
		var next completion
		if errors.As(err, &next) {
			return x.orderedResult(next, before)
		}
		return x.fail(endInternal, err)
	}
	return x.orderedResult(completion{}, before)
}

// Acquire the replacement before releasing the old return, even when both
// point to the same value. A destructor raised by either release can replace
// the completion again; its discarded replacement must be consumed as well.
func (x *execution) replaceOwnedCompletion(old, next completion) completion {
	next = ownReturn(next)
	for {
		next = replaceCompletion(old, next)
		released := x.releaseReturn(&old)
		if released.kind == endNormal {
			return next
		}
		old, next = next, released
	}
}

// local establishes ownership for a previously unowned variable slot. Register
// before assign/bind/COW operations; afterwards mutate only through heap methods.
func (x *execution) local(slot *value) error {
	if slot == nil || len(x.frames) == 0 || x.closed {
		return errors.New("local requires a slot and an active frame")
	}
	for _, frame := range x.frames {
		for _, existing := range frame.locals {
			if existing == slot {
				return errors.New("local slot is already registered")
			}
		}
	}
	retain(*slot)
	f := x.frames[len(x.frames)-1]
	f.locals = append(f.locals, slot)
	return nil
}
func (x *execution) registerShutdown(fn executionAction) error {
	if fn == nil || x.closed {
		return errors.New("invalid shutdown callback")
	}
	if len(x.shutdown) >= x.limits.callbacks {
		x.fail(endLimit, errExecutionLimit)
		return errExecutionLimit
	}
	x.shutdown = append(x.shutdown, fn)
	return nil
}

// startBuffer is ob_start(). name is PHP's callable name for filter, such as
// {closure:FILE:LINE}; a buffer without a handler has no name.
func (x *execution) startBuffer(name string, filter outputFilter) error {
	if x.closed {
		return errors.New("execution is closed")
	}
	if filter != nil && name == "" {
		return errors.New("output handler requires its PHP callable name")
	}
	if x.filtering {
		// php_output_lock_error deactivates output first: every buffer is
		// discarded without running its handler, and later output is direct.
		x.buffers = nil
		return x.fail(endFatal, errors.New("Cannot use output buffering in output buffering display handlers"))
	}
	if len(x.buffers) >= x.limits.depth {
		x.fail(endLimit, errExecutionLimit)
		return errExecutionLimit
	}
	x.buffers = append(x.buffers, executionBuffer{filter: filter, name: name})
	return nil
}
func (x *execution) warning(message string) completion {
	if c := x.check(); c.kind != endNormal {
		return c
	}
	if len(x.diagnostics) >= x.limits.callbacks {
		return x.fail(endLimit, errExecutionLimit)
	}
	x.diagnostics = append(x.diagnostics, executionDiagnostic{"Warning", message})
	return completion{}
}

// object creates an object using the existing W-06 logical lifetime model.
// A destructor's terminating completion stops subsequent PHP destructors,
// while heap invalidation and Runtime resource cleanup still run.
func (x *execution) object(name string, destructor executionAction) (value, error) {
	return x.heap.newObject(&class{name: name, destructor: func(_ *heap, _ value) error {
		if x.destructorStop || destructor == nil {
			return nil
		}
		c := x.call(destructor, nil)
		if released := x.releaseReturn(&c); released.kind != endNormal {
			c = x.replaceOwnedCompletion(c, released)
		}
		if stop := x.check(); stop.kind != endNormal {
			c = x.replaceOwnedCompletion(c, stop)
		}
		if c.kind == endNormal || c.kind == endReturn {
			return nil
		}
		// A nested exception may be caught by the outer destructor. Only
		// destruct() decides whether an escaping error stops the whole pass.
		return c
	}})
}

func stopsUserCode(c completion) bool {
	return c.kind == endCancel || c.kind == endOutput || c.kind == endInternal || c.kind == endLimit
}

func (x *execution) fail(kind endKind, err error) completion {
	if kind == endFatal {
		return x.markFatal(completion{kind: kind, err: err})
	}
	if x.stop.kind == endNormal {
		x.stop = completion{kind: kind, err: err}
	}
	return x.stop
}

func (x *execution) check() completion {
	if x.stop.kind != endNormal {
		return x.stop
	}
	if err := x.ctx.Err(); err != nil {
		return x.fail(endCancel, err)
	}
	return x.phaseFatal
}

func (x *execution) invoke(fn executionAction) (c completion) {
	defer func() {
		if p := recover(); p != nil {
			c = x.fail(endInternal, fmt.Errorf("execution callback panicked: %v", p))
		}
	}()
	before := x.serial
	pending := x.phaseCompletion
	c = fn(x)
	if (c.kind == endNormal || c.kind == endReturn) && x.phaseCompletion.kind == endThrow && x.phaseCompletion.serial > before {
		// This action caught a newly raised exception. Restore the caller's
		// pending event instead of resurrecting the handled exception later.
		x.phaseCompletion = pending
	}
	if c.kind == endThrow || c.kind == endExit {
		if c.serial == 0 {
			x.serial++
			c.serial = x.serial
		}
		if c.kind == endExit && c.serial > x.lastExit.serial {
			x.lastExit = c
		}
		if c.serial > x.phaseCompletion.serial {
			c = replaceCompletion(x.phaseCompletion, c)
			x.phaseCompletion = c
		}
	}
	// Mark at the action boundary, before a containing array or property
	// release can continue into sibling destructors.
	if c.kind == endFatal {
		c = x.markFatal(c)
	}
	return x.orderedResult(c, before)
}

// An aggregate release may return an older throw/exit. Preserve the newer
// event and its chain without imposing it on cleanup callbacks entered later.
// A normal result after the latest throw means that callback caught it.
func (x *execution) orderedResult(c completion, since uint64) completion {
	for x.phaseCompletion.serial > since && x.phaseCompletion.serial > c.serial {
		if c.kind != endThrow && c.kind != endExit &&
			((c.kind != endNormal && c.kind != endReturn) || x.phaseCompletion.kind != endExit) {
			break
		}
		c = x.replaceOwnedCompletion(c, x.phaseCompletion)
	}
	return c
}

// Preserve a fatal across aggregate releases that return an earlier exception.
// Host stops remain independent and take precedence in check().
func (x *execution) markFatal(c completion) completion {
	if x.phaseFatal.kind == endNormal {
		x.phaseFatal = c
	}
	for _, o := range x.heap.live {
		o.destructorCalled = true
	}
	return x.phaseFatal
}

func (x *execution) recordEnd(c completion) {
	// The driver has consumed this phase's fatal. The next shutdown,
	// destructor, or output phase may execute new PHP callbacks.
	x.phaseFatal = completion{}
	x.phaseCompletion = completion{}
	if (c.kind == endThrow || c.kind == endFatal) && len(x.diagnostics) < x.limits.callbacks {
		x.diagnostics = append(x.diagnostics, executionDiagnostic{"Fatal error", c.Error()})
	}
}
