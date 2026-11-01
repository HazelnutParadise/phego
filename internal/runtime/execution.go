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
	kind     endKind
	code     int
	value    value
	err      error
	previous error
}

func (c completion) Error() string {
	if c.err != nil {
		return c.err.Error()
	}
	return "PHP execution stopped"
}

func (c completion) Unwrap() []error { return []error{c.err, c.previous} }

type executionAction func(*execution) completion
type outputFilter func(*execution, []byte) ([]byte, completion)
type executionLimits struct{ depth, callbacks, output int }
type executionFrame struct{ locals []value }
type executionBuffer struct {
	data   []byte
	filter outputFilter
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
	ctx                      context.Context
	request                  *Request
	heap                     *heap
	globals                  value
	limits                   executionLimits
	writer                   io.Writer
	frames                   []*executionFrame
	shutdown                 []executionAction
	buffers                  []executionBuffer
	output                   []byte
	accepted                 int64
	diagnostics              []executionDiagnostic
	started, closed          bool
	destructing              bool
	filtering, handlerOutput bool
	destructorStop           bool
	result                   executionResult
	stop                     completion
	calls                    int
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
	return &execution{ctx: ctx, request: request, heap: h, globals: nullValue(), writer: writer, limits: limits}, nil
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
	c := x.call(body, nil)
	var returned any
	if c.kind == endReturn {
		var err error
		returned, err = exportSnapshot(c.value)
		if err != nil {
			c = completion{kind: endInternal, err: err}
		}
	}
	x.recordEnd(c)
	if !stopsUserCode(c) {
		for i := 0; i < len(x.shutdown); i++ {
			next := x.call(x.shutdown[i], nil)
			if next.kind != endNormal && next.kind != endReturn {
				c = next
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
		c = replaceCompletion(c, next)
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
			c = next
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
		c = x.fail(endInternal, err)
	}
	x.buffers = nil
	x.shutdown = nil
	x.frames = nil
	x.closed = true
	if c.kind == endThrow || c.kind == endFatal {
		c.code = 255
	}
	// Never let the completion expose a PHP value after heap teardown.
	c.value = value{}
	var transitionErr error
	if c.kind == endNormal || c.kind == endReturn || c.kind == endExit {
		transitionErr = x.request.Finish()
	} else {
		transitionErr = x.request.Abort()
	}
	cleanupErr := errors.Join(transitionErr, x.request.Close(cleanup))
	x.result = executionResult{end: c, returned: returned, output: append([]byte(nil), x.output...), accepted: x.accepted, diagnostics: append([]executionDiagnostic(nil), x.diagnostics...), cleanupErr: cleanupErr, cleanupComplete: x.request.State() == Closed}
	return x.result
}

// call owns local values until return/throw unwinds them. exit/fatal/cancel
// bypass PHP finally; their live values are retained for request teardown.
func (x *execution) call(body, finally executionAction) completion {
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
	f := &executionFrame{}
	x.frames = append(x.frames, f)
	c := x.invoke(body)
	if stop := x.check(); stop.kind != endNormal {
		c = stop
	}
	if (c.kind == endNormal || c.kind == endReturn || c.kind == endThrow) && finally != nil {
		if x.calls >= x.limits.callbacks {
			c = x.fail(endLimit, errExecutionLimit)
		} else {
			x.calls++
			if next := x.invoke(finally); next.kind != endNormal {
				c = replaceCompletion(c, next)
			}
		}
		if stop := x.check(); stop.kind != endNormal {
			c = stop
		}
	}
	if c.kind == endReturn {
		retain(c.value)
	}
	if c.kind == endNormal || c.kind == endReturn || c.kind == endThrow {
		for i := 0; i < len(f.locals); i++ {
			if err := x.heap.release(f.locals[i]); err != nil {
				var next completion
				if errors.As(err, &next) {
					c = replaceCompletion(c, next)
				} else {
					c = x.fail(endInternal, err)
				}
				if c.kind != endThrow {
					break
				}
			}
		}
	}
	x.frames = x.frames[:len(x.frames)-1]
	return c
}

func replaceCompletion(old, next completion) completion {
	if old.kind == endThrow && next.kind == endThrow {
		next.previous = errors.Join(next.previous, old.err, old.previous)
	}
	return next
}

func (x *execution) local(v value) error {
	if len(x.frames) == 0 || x.closed {
		return errors.New("local requires an active frame")
	}
	retain(v)
	f := x.frames[len(x.frames)-1]
	f.locals = append(f.locals, v)
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
func (x *execution) startBuffer(filter outputFilter) error {
	if x.closed {
		return errors.New("execution is closed")
	}
	if x.filtering {
		return x.fail(endFatal, errors.New("Cannot use output buffering in output buffering display handlers"))
	}
	if len(x.buffers) >= x.limits.depth {
		x.fail(endLimit, errExecutionLimit)
		return errExecutionLimit
	}
	x.buffers = append(x.buffers, executionBuffer{filter: filter})
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
		if c.kind == endNormal || c.kind == endReturn {
			return nil
		}
		// During request teardown any terminating destructor stops the rest.
		// During normal frame unwinding a PHP exception may still be caught.
		if x.destructing {
			x.destructorStop = true
		}
		return c
	}})
}

func stopsUserCode(c completion) bool {
	return c.kind == endCancel || c.kind == endOutput || c.kind == endInternal || c.kind == endLimit
}

func (x *execution) fail(kind endKind, err error) completion {
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
	return completion{}
}

func (x *execution) invoke(fn executionAction) (c completion) {
	defer func() {
		if p := recover(); p != nil {
			c = x.fail(endInternal, fmt.Errorf("execution callback panicked: %v", p))
		}
	}()
	c = fn(x)
	// Mark at the action boundary, before a containing array or property
	// release can continue into sibling destructors.
	if c.kind == endFatal {
		for _, o := range x.heap.live {
			o.destructorCalled = true
		}
	}
	return c
}

func (x *execution) recordEnd(c completion) {
	if c.kind == endFatal {
		for _, o := range x.heap.live {
			o.destructorCalled = true
		}
	}
	if (c.kind == endThrow || c.kind == endFatal) && len(x.diagnostics) < x.limits.callbacks {
		x.diagnostics = append(x.diagnostics, executionDiagnostic{"Fatal error", c.Error()})
	}
}
