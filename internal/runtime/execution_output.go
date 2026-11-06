package runtime

import (
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
	// PHP 8.5.12 discards echo from a user output handler, including echo
	// from its local destructors, and emits a deprecation afterwards.
	if x.filtering {
		x.handlerOutput = x.handlerOutput || len(data) > 0
		return completion{}
	}
	remaining := int64(x.limits.output) - x.accepted
	for _, b := range x.buffers {
		remaining -= int64(len(b.data))
	}
	if int64(len(data)) > remaining {
		return x.fail(endLimit, errExecutionLimit)
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

// flush ends every buffer at request shutdown.
func (x *execution) flush() completion {
	for len(x.buffers) > 0 {
		if c := x.endBufferIn("PHP Request Shutdown"); c.kind != endNormal {
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
	if len(x.buffers) == 0 {
		return completion{}
	}
	i := len(x.buffers) - 1
	b := x.buffers[i]
	x.buffers = x.buffers[:i]
	if b.filter != nil {
		var c completion
		b.data, c = x.filter(b.filter, origin, b.name, b.data)
		if c.kind != endNormal {
			return c
		}
	}
	return x.write(b.data)
}

func (x *execution) filter(fn outputFilter, origin, name string, data []byte) (out []byte, c completion) {
	x.filtering = true
	x.handlerOutput = false
	defer func() { x.filtering = false; x.handlerOutput = false }()
	c = x.call(func(x *execution) completion { var next completion; out, next = fn(x, data); return next }, nil)
	if released := x.releaseReturn(&c); released.kind != endNormal {
		c = x.replaceOwnedCompletion(c, released)
	}
	if x.handlerOutput {
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
