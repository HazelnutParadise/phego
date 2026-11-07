package runtime

import (
	"errors"
	"fmt"
	"io"
)

// REQ-009, REQ-040; T-017, T-079, T-080. Output is committed once, including
// a writer's accepted prefix on failure. PHP and host diagnostics stay separate.
func (x *execution) write(data []byte) (c completion) {
	if x.closed {
		return completion{kind: endInternal, err: fmt.Errorf("execution is closed")}
	}
	if c := x.check(); c.kind != endNormal {
		return c
	}
	// A running handler's input buffer is still held while it echoes.
	remaining := int64(x.limits.output) - x.accepted - x.filterHeld - int64(len(x.handlerEcho))
	for _, b := range x.buffers {
		remaining -= int64(len(b.data))
	}
	if int64(len(data)) > remaining {
		return x.fail(endLimit, errExecutionLimit)
	}
	// Echo from a user output handler, including its local destructors, is
	// kept aside: PHP 8.5.12 drops it when the handler returns and passes it
	// on with the buffer when the handler throws (see filter).
	if x.filtering {
		x.handlerEcho = append(x.handlerEcho, data...)
		return completion{}
	}
	if len(x.buffers) > 0 {
		b := &x.buffers[len(x.buffers)-1]
		b.data = append(b.data, data...)
		return completion{}
	}
	if x.writer == nil {
		x.output = append(x.output, data...)
		x.accepted += int64(len(data))
		return completion{}
	}
	defer func() {
		if p := recover(); p != nil {
			c = x.fail(endInternal, fmt.Errorf("output writer panicked: %v", p))
		}
	}()
	n, err := x.writer.Write(data)
	if n < 0 || n > len(data) {
		return x.fail(endInternal, fmt.Errorf("output writer returned invalid byte count %d", n))
	}
	x.accepted += int64(n)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return x.fail(endOutput, err)
	}
	return x.check()
}

// requestShutdown is the origin of the flush at request shutdown, which runs
// handlers without a PHP frame.
const requestShutdown = "PHP Request Shutdown"

// flush ends every buffer at request shutdown.
func (x *execution) flush() completion {
	for len(x.buffers) > 0 {
		if c := x.endBufferIn(requestShutdown); c.kind != endNormal {
			return c
		}
	}
	return completion{}
}

// endBuffer is ob_end_flush() and its relatives: the innermost buffer passes
// through its handler into the next buffer or the writer. op names the PHP
// function whose call flushed it, which handler diagnostics report.
func (x *execution) endBuffer(op string) completion {
	return x.endBufferIn(op + "()")
}

func (x *execution) endBufferIn(origin string) completion {
	if c := x.check(); c.kind != endNormal {
		return c
	}
	if x.filtering {
		return x.outputLockError(origin)
	}
	if len(x.buffers) == 0 {
		return completion{}
	}
	i := len(x.buffers) - 1
	b := x.buffers[i]
	x.buffers = x.buffers[:i]
	if b.filter != nil {
		var c completion
		b.data, c = x.filter(b.filter, origin, b.name, b.data)
		if c.kind == endThrow || c.kind == endExit {
			if origin == requestShutdown {
				// Without a PHP frame the throw or exit bails out of
				// php_output_end_all, and php_output_deactivate discards
				// every buffer: nothing is printed and outer handlers
				// never run.
				x.buffers = nil
				return c
			}
			// From PHP code the buffer, with the handler's echo, reaches
			// the next level before the throw or exit propagates.
			if w := x.write(b.data); w.kind != endNormal {
				return x.replaceOwnedCompletion(c, w)
			}
			return c
		}
		if c.kind != endNormal {
			return c
		}
	}
	return x.write(b.data)
}

// outputLockError is php_output_lock_error for an output buffer operation
// inside a handler. Output is deactivated first: every buffer is discarded
// without running its handler, and later output is direct.
func (x *execution) outputLockError(origin string) completion {
	x.buffers = nil
	return x.fail(endFatal, errors.New(origin+": Cannot use output buffering in output buffering display handlers"))
}

func (x *execution) filter(fn outputFilter, origin, name string, data []byte) (out []byte, c completion) {
	x.filtering = true
	x.filterHeld = int64(len(data))
	x.handlerEcho = nil
	defer func() { x.filtering = false; x.filterHeld = 0; x.handlerEcho = nil }()
	c = x.call(func(x *execution) completion { var next completion; out, next = fn(x, data); return next }, nil)
	if released := x.releaseReturn(&c); released.kind != endNormal {
		c = x.replaceOwnedCompletion(c, released)
	}
	if c.kind == endThrow || c.kind == endExit {
		// php_output_handler_op: a handler that throws or exits leaves no
		// return value, so it passes its buffer, with its own echo,
		// without the deprecation.
		return append(append([]byte(nil), data...), x.handlerEcho...), c
	}
	// The deprecation needs a returned value; a fatal error or a host stop
	// inside the handler never returns.
	if (c.kind == endNormal || c.kind == endReturn) && len(x.handlerEcho) > 0 {
		if len(x.diagnostics) >= x.limits.callbacks {
			c = x.replaceOwnedCompletion(c, x.fail(endLimit, errExecutionLimit))
		} else {
			x.diagnostics = append(x.diagnostics, executionDiagnostic{"Deprecated", origin + ": Producing output from user output handler " + name + " is deprecated"})
		}
	}
	if stop := x.check(); stop.kind != endNormal {
		c = x.replaceOwnedCompletion(c, stop)
	}
	return out, c
}
