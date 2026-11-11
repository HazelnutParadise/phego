package runtime

import (
	"errors"
	"fmt"
	"io"
)

// REQ-009, REQ-040; T-017, T-079, T-080. Output is committed once, including
// a writer's accepted prefix on failure. PHP and host diagnostics stay separate.
// write is echo: it honours a pending stop or cancellation first and then
// writes into the innermost buffer.
func (x *execution) write(data []byte) completion {
	return x.outputOp(len(x.buffers)-1, data, true)
}

// writeTo writes into buffer top, or straight to the writer when top < 0.
// report uses it for displayed diagnostics, which are written even while
// the execution is stopping.
func (x *execution) writeTo(top int, data []byte) completion {
	return x.outputOp(top, data, false)
}

// outputOp is php_output_op: a write while a handler runs marks the buffer
// that receives it as having produced output, which is how a displayed
// diagnostic that skipped a disabled handler flags the next one. write and
// writeTo are its two entries; they stay one-line so that they inline and
// every echo is a single call (a completion is too large to return through
// a second frame for free: benchstat measured about 30% on amd64).
func (x *execution) outputOp(top int, data []byte, checked bool) completion {
	if x.closed {
		return completion{kind: endInternal, err: fmt.Errorf("execution is closed")}
	}
	if checked {
		if c := x.check(); c.kind != endNormal {
			return c
		}
	}
	if (x.filtering || x.outputRunning) && top >= 0 {
		x.buffers[top].produced = true
	}
	// A running handler's buffer stays on the stack, so its input and its
	// echo both count here.
	remaining := int64(x.limits.output) - x.accepted
	for _, b := range x.buffers {
		remaining -= int64(len(b.data))
	}
	if int64(len(data)) > remaining {
		return x.fail(endLimit, errExecutionLimit)
	}
	if top >= 0 {
		b := &x.buffers[top]
		b.data = append(b.data, data...)
		return completion{}
	}
	return x.writeOut(data)
}

// writeOut commits data to the captured output or the host writer.
func (x *execution) writeOut(data []byte) (c completion) {
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
	if x.filtering || x.outputRunning {
		return x.outputLockError(origin)
	}
	if len(x.buffers) == 0 {
		return completion{}
	}
	i := len(x.buffers) - 1
	if x.buffers[i].filter == nil {
		data := x.buffers[i].data
		x.buffers = x.buffers[:i]
		return x.write(data)
	}
	out, c := x.filter(origin)
	if len(x.buffers) <= i || x.outputRunning {
		// The lock error discarded every buffer, or a fatal error left this
		// one running on the stack.
		return c
	}
	x.buffers = x.buffers[:i]
	if c.kind == endThrow || c.kind == endExit {
		if origin == requestShutdown {
			// Without a PHP frame the throw or exit bails out of
			// php_output_end_all, and php_output_deactivate discards every
			// buffer: nothing is printed and outer handlers never run. The
			// buffers stay until teardown discards them, so the uncaught
			// error displayed now lands in the running handler's buffer and
			// is discarded with it, as in PHP.
			x.buffers = x.buffers[:i+1]
			return c
		}
		// From PHP code the handler's result reaches the next level before
		// the throw or exit propagates.
		if w := x.write(out); w.kind != endNormal {
			return x.replaceOwnedCompletion(c, w)
		}
		return c
	}
	if c.kind != endNormal {
		return c
	}
	return x.write(out)
}

// outputLockError is php_output_lock_error for an output buffer operation
// inside a handler. Output is deactivated first: every buffer is discarded
// without running its handler, and later output is direct.
func (x *execution) outputLockError(origin string) completion {
	x.buffers = nil
	return x.fail(endFatal, errors.New(origin+": Cannot use output buffering in output buffering display handlers"))
}

// filter runs the innermost buffer's handler on the buffer's data. As in
// PHP, the buffer stays on the stack while its handler runs: ob_get_level()
// counts it, and the handler's echo, including from its locals' destructors,
// is appended to it and counts against the output limit. It returns what
// reaches the next level:
//   - the handler's returned string, when it returned, even if a local's
//     destructor then throws or exits (main/output.c keeps retval);
//   - the buffer with the handler's echo, when the handler itself throws or
//     exits and so leaves no return value.
//
// The handler-output deprecation needs a returned value; a fatal error or a
// host stop never returns, and at request shutdown a destructor's throw or
// exit bails out first.
func (x *execution) filter(origin string) (out []byte, c completion) {
	i := len(x.buffers) - 1
	fn, name := x.buffers[i].filter, x.buffers[i].name
	n := len(x.buffers[i].data)
	data := x.buffers[i].data[:n:n] // the handler's appends cannot reach its echo
	x.filtering = true
	defer func() { x.filtering = false }()
	returned := false
	c = x.call(func(x *execution) completion {
		var next completion
		out, next = fn(x, data)
		returned = next.kind == endNormal || next.kind == endReturn
		return next
	}, nil)
	if released := x.releaseReturn(&c); released.kind != endNormal {
		c = x.replaceOwnedCompletion(c, released)
	}
	if len(x.buffers) <= i {
		return nil, c
	}
	if c.kind == endFatal {
		// The fatal error bails out of php_output_handler_op, so the buffer
		// stays on the stack with OG(running) set: shutdown functions still
		// count it, and the next buffer operation, including the
		// request-shutdown flush, is the output lock error.
		x.outputRunning = true
		return nil, c
	}
	held := x.buffers[i].data
	if !returned && (c.kind == endThrow || c.kind == endExit) {
		return held, c
	}
	// At request shutdown a destructor's throw or exit bails out inside
	// zend_call_function, before main/output.c reaches the deprecation.
	switch c.kind {
	case endThrow, endExit:
		if origin == requestShutdown {
			break
		}
		fallthrough
	case endNormal, endReturn:
		if returned && x.buffers[i].produced {
			if r := x.report("Deprecated", origin+": Producing output from user output handler "+name+" is deprecated", true); r.kind != endNormal {
				c = x.replaceOwnedCompletion(c, r)
			}
		}
	}
	if stop := x.check(); stop.kind != endNormal {
		c = x.replaceOwnedCompletion(c, stop)
	}
	return out, c
}
