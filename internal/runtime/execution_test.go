package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func readyExecutionRequest(t *testing.T) *Request {
	t.Helper()
	r, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestExecution(t *testing.T, r *Request, width int, w io.Writer, limits executionLimits) *execution {
	t.Helper()
	x, err := newExecution(context.Background(), r, width, w, limits)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func cleanupDeadline(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}

// REQ-020, REQ-022; T-017/T-018/T-059.
func TestExecutionConstructorRejectsInvalidInputs(t *testing.T) {
	ready := readyExecutionRequest(t)
	notReady, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		ctx    context.Context
		r      *Request
		width  int
		limits executionLimits
	}{
		{"nil context", nil, ready, 64, executionLimits{1, 1, 1}},
		{"nil request", context.Background(), nil, 64, executionLimits{1, 1, 1}},
		{"request not ready", context.Background(), notReady, 64, executionLimits{1, 1, 1}},
		{"width zero", context.Background(), ready, 0, executionLimits{1, 1, 1}},
		{"width invalid", context.Background(), ready, 128, executionLimits{1, 1, 1}},
		{"zero depth", context.Background(), ready, 32, executionLimits{0, 1, 1}},
		{"zero callbacks", context.Background(), ready, 64, executionLimits{1, 0, 1}},
		{"zero output", context.Background(), ready, 64, executionLimits{1, 1, 0}},
		{"negative limit", context.Background(), ready, 64, executionLimits{1, -1, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newExecution(tc.ctx, tc.r, tc.width, nil, tc.limits); err == nil {
				t.Fatal("newExecution accepted invalid input")
			}
		})
	}
	for _, width := range []int{32, 64} {
		t.Run(map[int]string{32: "width 32", 64: "width 64"}[width], func(t *testing.T) {
			x := newTestExecution(t, readyExecutionRequest(t), width, nil, executionLimits{1, 1, 1})
			if x.heap.width != width {
				t.Fatalf("heap width=%d, want %d", x.heap.width, width)
			}
		})
	}
}

// REQ-020, REQ-022; T-039/T-040/T-043/T-044.
func TestExecutionRunOnceReentryAndCleanupLedger(t *testing.T) {
	r, err := NewRequest([]InitStep{{Name: "resource", Resource: true}})
	if err != nil {
		t.Fatal(err)
	}
	var released, closed int
	if err := r.Admit(func(context.Context) error { released++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("resource", func(context.Context) error { closed++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x := newTestExecution(t, r, 64, nil, executionLimits{4, 4, 32})
	var bodyCalls int
	var nested executionResult
	ctx, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	got := x.run(ctx, func(x *execution) completion {
		bodyCalls++
		nested = x.run(ctx, func(*execution) completion { bodyCalls++; return completion{} })
		return completion{}
	})
	if got.end.kind != endNormal || bodyCalls != 1 {
		t.Fatalf("outer run=%+v, body calls=%d", got.end, bodyCalls)
	}
	if nested.end.kind != endInternal || !errors.Is(nested.end.err, errExecutionReentry) {
		t.Fatalf("nested run=%+v, want reentry error", nested.end)
	}
	again := x.run(ctx, func(*execution) completion { bodyCalls++; return completion{} })
	if again.end.kind != endInternal || !errors.Is(again.end.err, errExecutionReentry) || bodyCalls != 1 {
		t.Fatalf("second run=%+v calls=%d", again.end, bodyCalls)
	}
	if closed != 1 || released != 1 || !got.cleanupComplete {
		t.Fatalf("ledger close=%d release=%d result=%+v", closed, released, got)
	}
}

// REQ-009, REQ-020, REQ-022, REQ-030, REQ-040; T-017/T-039/T-040/T-043/T-044/T-079/T-080.
func TestExecutionCancellationPreservesAcceptedOutputAndStopsPHPCallbacks(t *testing.T) {
	r, err := NewRequest([]InitStep{{Name: "resource", Resource: true}})
	if err != nil {
		t.Fatal(err)
	}
	var resources int
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("resource", func(context.Context) error { resources++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	x, err := newExecution(ctx, r, 64, nil, executionLimits{4, 8, 32})
	if err != nil {
		t.Fatal(err)
	}
	var shutdown, destructor, filter int
	if err := x.registerShutdown(func(*execution) completion { shutdown++; return completion{} }); err != nil {
		t.Fatal(err)
	}
	result, err := func() (executionResult, error) {
		cleanup, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		var res executionResult
		res = x.run(cleanup, func(x *execution) completion {
			v, err := x.object("tracked", func(*execution) completion { destructor++; return completion{} })
			if err != nil {
				return completion{kind: endInternal, err: err}
			}
			if err := x.heap.assign(&x.globals, v); err != nil {
				return completion{kind: endInternal, err: err}
			}
			if c := x.write([]byte("accepted")); c.kind != endNormal {
				return c
			}
			if err := x.startBuffer("{closure:test}", func(*execution, []byte) ([]byte, completion) { filter++; return nil, completion{} }); err != nil {
				return completion{kind: endInternal, err: err}
			}
			if c := x.write([]byte("buffered")); c.kind != endNormal {
				return c
			}
			stop()
			return x.write([]byte("rejected"))
		})
		return res, nil
	}()
	if err != nil {
		t.Fatal(err)
	}
	if result.end.kind != endCancel || string(result.output) != "accepted" || result.accepted != int64(len("accepted")) {
		t.Fatalf("cancellation result=%+v", result)
	}
	if shutdown != 0 || destructor != 0 || filter != 0 || resources != 1 || !result.cleanupComplete {
		t.Fatalf("callbacks shutdown=%d destructor=%d filter=%d resources=%d result=%+v", shutdown, destructor, filter, resources, result)
	}
}

type executionWriter func([]byte) (int, error)

func (w executionWriter) Write(p []byte) (int, error) { return w(p) }

// REQ-009, REQ-040; T-017/T-079/T-080.
func TestExecutionWriterAcceptanceAndFaults(t *testing.T) {
	writeFailure := errors.New("disk full")
	tests := []struct {
		name         string
		write        executionWriter
		wantAccepted int64
		wantErr      error
		wantEnd      endKind
	}{
		{"accepted prefix with error", func([]byte) (int, error) { return 2, writeFailure }, 2, writeFailure, endOutput},
		{"short write", func([]byte) (int, error) { return 1, nil }, 1, io.ErrShortWrite, endOutput},
		{"invalid negative count", func([]byte) (int, error) { return -1, nil }, 0, nil, endInternal},
		{"invalid oversized count", func(p []byte) (int, error) { return len(p) + 1, nil }, 0, nil, endInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := readyExecutionRequest(t)
			calls := 0
			x := newTestExecution(t, r, 64, executionWriter(func(p []byte) (int, error) { calls++; return tc.write(p) }), executionLimits{4, 4, 32})
			ctx, cancel := cleanupDeadline(t, time.Second)
			defer cancel()
			result := x.run(ctx, func(x *execution) completion { return x.write([]byte("abc")) })
			if result.accepted != tc.wantAccepted || result.end.kind != tc.wantEnd {
				t.Fatalf("result=%+v", result)
			}
			if tc.wantErr != nil && !errors.Is(result.end.err, tc.wantErr) {
				t.Fatalf("end error=%v, want %v", result.end.err, tc.wantErr)
			}
			if calls != 1 || !result.cleanupComplete {
				t.Fatalf("writer calls=%d result=%+v", calls, result)
			}
		})
	}
	t.Run("writer panic is contained and cleanup still runs", func(t *testing.T) {
		r, err := NewRequest([]InitStep{{Name: "resource", Resource: true}})
		if err != nil {
			t.Fatal(err)
		}
		var closed int
		if err := r.Admit(func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := r.BeginInit(); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CommitResource("resource", func(context.Context) error { closed++; return nil }); err != nil {
			t.Fatal(err)
		}
		if err := r.Ready(); err != nil {
			t.Fatal(err)
		}
		writer := executionWriter(func([]byte) (int, error) { panic("writer panic") })
		x := newTestExecution(t, r, 32, writer, executionLimits{4, 4, 32})
		ctx, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		result := x.run(ctx, func(x *execution) completion { return x.write([]byte("x")) })
		if result.end.kind != endInternal || !strings.Contains(result.end.err.Error(), "writer panic") || result.cleanupComplete != true {
			t.Fatalf("result=%+v", result)
		}
		if closed != 1 {
			t.Fatalf("close count=%d, want 1", closed)
		}
	})
}

// REQ-020, REQ-022; T-043/T-044/T-059.
func TestExecutionCleanupErrorDoesNotReplacePHPExit(t *testing.T) {
	r, err := NewRequest([]InitStep{{Name: "resource", Resource: true}})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("close failed")
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("resource", func(context.Context) error { return failure }); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x := newTestExecution(t, r, 32, nil, executionLimits{2, 2, 16})
	ctx, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	result := x.run(ctx, func(*execution) completion { return completion{kind: endExit, code: 7} })
	if result.end.kind != endExit || result.end.code != 7 || !errors.Is(result.cleanupErr, failure) {
		t.Fatalf("result=%+v", result)
	}
}

// REQ-020, REQ-022; T-043/T-044/T-059.
func TestExecutionCleanupTimeoutReturnsStableIncompleteSnapshot(t *testing.T) {
	r, err := NewRequest([]InitStep{{Name: "resource", Resource: true}})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("resource", func(context.Context) error { close(entered); <-release; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x := newTestExecution(t, r, 64, nil, executionLimits{2, 2, 16})
	ctx, cancel := cleanupDeadline(t, 20*time.Millisecond)
	result := x.run(ctx, func(x *execution) completion { return x.write([]byte("fixed")) })
	cancel()
	<-entered
	if result.cleanupComplete {
		t.Fatalf("timeout falsely reported clean: %+v", result)
	}
	beforeOutput := append([]byte(nil), result.output...)
	beforeKind, beforeCode, beforeError, beforeAccepted := result.end.kind, result.end.code, result.end.err, result.accepted
	close(release)
	deadline := time.Now().Add(time.Second)
	for r.State() != Closed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if r.State() != Closed {
		t.Fatal("resource cleanup did not finish after callback was released")
	}
	if result.cleanupComplete || !bytes.Equal(result.output, beforeOutput) || result.accepted != beforeAccepted || result.end.kind != beforeKind || result.end.code != beforeCode || !errors.Is(result.end.err, beforeError) {
		t.Fatalf("returned snapshot changed after cleanup: %+v", result)
	}
}

// REQ-009, REQ-020, REQ-040; T-017/T-059/T-079/T-080.
func TestExecutionBinaryCaptureWarningsAndSnapshotOwnership(t *testing.T) {
	x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{4, 8, 32})
	input := []byte{0, 0xff, 'x'}
	ctx, cancel := cleanupDeadline(t, time.Second)
	defer cancel()
	result := x.run(ctx, func(x *execution) completion {
		if c := x.write(input); c.kind != endNormal {
			return c
		}
		input[0] = 99
		x.warning("first")
		x.warning("second")
		return completion{}
	})
	if !bytes.Equal(result.output, []byte{0, 0xff, 'x'}) || result.accepted != 3 {
		t.Fatalf("output=%v accepted=%d", result.output, result.accepted)
	}
	if len(result.diagnostics) != 2 || result.diagnostics[0].message != "first" || result.diagnostics[1].message != "second" {
		t.Fatalf("diagnostics=%+v", result.diagnostics)
	}
	result.output[0] = 7
	result.diagnostics[0].message = "changed"
	if x.output[0] != 0 || x.diagnostics[0].message != "first" {
		t.Fatalf("result snapshot aliases execution state: output=%v diagnostics=%+v", x.output, x.diagnostics)
	}
}

// REQ-020, REQ-022; T-017/T-059.
func TestExecutionInstancesOwnSeparateHeapsAndState(t *testing.T) {
	x1 := newTestExecution(t, readyExecutionRequest(t), 32, nil, executionLimits{2, 2, 8})
	x2 := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{2, 2, 8})
	v1, err := x1.object("one", nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := x2.object("two", nil)
	if err != nil {
		t.Fatal(err)
	}
	if x1.heap == x2.heap || v1.object == v2.object {
		t.Fatal("execution instances share mutable heap state")
	}
	ctx1, cancel1 := cleanupDeadline(t, time.Second)
	defer cancel1()
	ctx2, cancel2 := cleanupDeadline(t, time.Second)
	defer cancel2()
	r1 := x1.run(ctx1, func(x *execution) completion { return x.write([]byte("one")) })
	r2 := x2.run(ctx2, func(x *execution) completion { return x.write([]byte("two")) })
	if string(r1.output) != "one" || string(r2.output) != "two" || !x1.heap.closed || !x2.heap.closed {
		t.Fatalf("results=%q/%q heap closed=%t/%t", r1.output, r2.output, x1.heap.closed, x2.heap.closed)
	}
}

// REQ-009, REQ-020, REQ-022, REQ-040; T-017/T-018/T-079/T-080.
func TestExecutionLimitsCannotBeSwallowed(t *testing.T) {
	t.Run("frame depth", func(t *testing.T) {
		x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{1, 8, 32})
		ctx, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		result := x.run(ctx, func(x *execution) completion {
			_ = x.call(func(x *execution) completion {
				_ = x.call(func(*execution) completion { return completion{} }, nil)
				return completion{}
			}, nil)
			return completion{}
		})
		if result.end.kind != endLimit || !errors.Is(result.end.err, errExecutionLimit) {
			t.Fatalf("swallowed depth limit: %+v", result.end)
		}
	})
	t.Run("callback count", func(t *testing.T) {
		x := newTestExecution(t, readyExecutionRequest(t), 32, nil, executionLimits{4, 1, 32})
		ctx, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		result := x.run(ctx, func(x *execution) completion {
			_ = x.call(func(*execution) completion { return completion{} }, nil)
			return completion{}
		})
		if result.end.kind != endLimit || !errors.Is(result.end.err, errExecutionLimit) {
			t.Fatalf("swallowed callback limit: %+v", result.end)
		}
	})
	t.Run("output bytes", func(t *testing.T) {
		x := newTestExecution(t, readyExecutionRequest(t), 64, nil, executionLimits{4, 4, 2})
		ctx, cancel := cleanupDeadline(t, time.Second)
		defer cancel()
		result := x.run(ctx, func(x *execution) completion {
			_ = x.write([]byte("abc"))
			return completion{}
		})
		if result.end.kind != endLimit || !errors.Is(result.end.err, errExecutionLimit) || result.accepted != 0 {
			t.Fatalf("swallowed output limit: %+v", result)
		}
	})
}

// REQ-040; T-079/T-080. PHP names the echoing handler and the flushing
// context: the calling function, or "PHP Request Shutdown" at shutdown.
func TestOutputHandlerDeprecationNamesHandlerAndContext(t *testing.T) {
	r, _ := NewRequest(nil)
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 4096})
	if err != nil {
		t.Fatal(err)
	}
	echo := func(x *execution, b []byte) ([]byte, completion) { return b, x.write([]byte("discarded")) }
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := x.run(cleanup, func(x *execution) completion {
		if err := x.startBuffer("named", echo); err != nil {
			t.Fatal(err)
		}
		if c := x.endBuffer("ob_end_flush"); c.kind != endNormal {
			return c
		}
		if err := x.startBuffer("", echo); err == nil {
			t.Error("a handler without its PHP name was accepted")
		}
		if err := x.startBuffer("", nil); err != nil {
			t.Error(err)
		}
		if err := x.startBuffer("{closure:/app/a.php:4}", echo); err != nil {
			t.Fatal(err)
		}
		return completion{}
	})
	var got []string
	for _, d := range result.diagnostics {
		got = append(got, d.level+": "+d.message)
	}
	want := []string{
		"Deprecated: ob_end_flush(): Producing output from user output handler named is deprecated",
		"Deprecated: PHP Request Shutdown: Producing output from user output handler {closure:/app/a.php:4} is deprecated",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics=%q, want %q", got, want)
	}
}

// PHP 8.5.12 (arm64 pinned image): ob_end_flush() inside a handler is a lock
// error like ob_start(). Output is deactivated first, so every buffer is
// discarded and the outer handler never runs; a shutdown function then sees
// level 0. The fatal names the calling function.
func TestOutputLockErrorFromHandlerEndDiscardsBuffers(t *testing.T) {
	for _, op := range []string{"ob_start", "ob_end_flush"} {
		t.Run(op, func(t *testing.T) {
			r, _ := NewRequest(nil)
			if err := r.Admit(func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if err := r.BeginInit(); err != nil {
				t.Fatal(err)
			}
			if err := r.Ready(); err != nil {
				t.Fatal(err)
			}
			x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 4096})
			if err != nil {
				t.Fatal(err)
			}
			outerRan := false
			outer := func(x *execution, b []byte) ([]byte, completion) {
				outerRan = true
				return append(append([]byte("<outer:"), b...), '>'), completion{}
			}
			inner := func(x *execution, b []byte) ([]byte, completion) {
				if op == "ob_start" {
					if err := x.startBuffer("", nil); err != nil {
						return b, x.check()
					}
					return b, completion{}
				}
				return b, x.endBuffer(op)
			}
			level := -1
			cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result := x.run(cleanup, func(x *execution) completion {
				if err := x.registerShutdown(func(x *execution) completion {
					level = len(x.buffers)
					return x.write([]byte("[shutdown]"))
				}); err != nil {
					t.Fatal(err)
				}
				if err := x.startBuffer("{closure:outer}", outer); err != nil {
					t.Fatal(err)
				}
				if c := x.write([]byte("A")); c.kind != endNormal {
					return c
				}
				if err := x.startBuffer("{closure:inner}", inner); err != nil {
					t.Fatal(err)
				}
				if c := x.write([]byte("B")); c.kind != endNormal {
					return c
				}
				return x.endBuffer("ob_end_flush")
			})
			want := op + "(): Cannot use output buffering in output buffering display handlers"
			var fatal string
			for _, d := range result.diagnostics {
				if d.level == "Fatal error" {
					fatal = d.message
				}
			}
			if string(result.output) != "[shutdown]" || outerRan || level != 0 || fatal != want {
				t.Errorf("output=%q outerRan=%v level=%d fatal=%q, want %q", result.output, outerRan, level, fatal, want)
			}
		})
	}
}

// PHP 8.5.12 (arm64 pinned image): a handler that echoes and then throws
// leaves its buffer and its own echo to the next level, without the
// deprecation, and the exception reaches the ob_end_flush() caller. The
// probe prints "bodyxcaught boom".
func TestOutputHandlerThrowPassesBufferAndEcho(t *testing.T) {
	r, _ := NewRequest(nil)
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 4096})
	if err != nil {
		t.Fatal(err)
	}
	throws := func(x *execution, b []byte) ([]byte, completion) {
		if c := x.write([]byte("x")); c.kind != endNormal {
			return nil, c
		}
		return nil, completion{kind: endThrow, err: errors.New("boom")}
	}
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := x.run(cleanup, func(x *execution) completion {
		if err := x.startBuffer("{closure:/w/t.php:2}", throws); err != nil {
			t.Fatal(err)
		}
		if c := x.write([]byte("body")); c.kind != endNormal {
			return c
		}
		if c := x.endBuffer("ob_end_flush"); c.kind != endThrow {
			t.Errorf("throw lost: %+v", c)
		}
		return x.write([]byte("caught boom"))
	})
	if string(result.output) != "bodyxcaught boom" || len(result.diagnostics) != 0 {
		t.Errorf("output=%q diagnostics=%+v", result.output, result.diagnostics)
	}
}

// While a handler runs, the buffer it filters is still held, so the
// handler's own echo counts against the output limit together with it. A
// handler stopped by the limit never returns, so it gets no handler-output
// deprecation (PHP emits it only for a returned value).
func TestOutputHandlerEchoCountsFilteredBuffer(t *testing.T) {
	r, _ := NewRequest(nil)
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 10})
	if err != nil {
		t.Fatal(err)
	}
	// The first echo fits beside the held 9-byte buffer; the second does not.
	echoes := func(x *execution, b []byte) ([]byte, completion) {
		for _, s := range []string{"1", "23456789"} {
			if c := x.write([]byte(s)); c.kind != endNormal {
				return nil, c
			}
		}
		return []byte("ok"), completion{}
	}
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := x.run(cleanup, func(x *execution) completion {
		if err := x.startBuffer("{closure:/w/t.php:2}", echoes); err != nil {
			t.Fatal(err)
		}
		if c := x.write([]byte("abcdefghi")); c.kind != endNormal {
			return c
		}
		return x.endBuffer("ob_end_flush")
	})
	if result.end.kind != endLimit || len(result.output) != 0 {
		t.Errorf("end=%+v output=%q, want the output limit with nothing written", result.end, result.output)
	}
	for _, d := range result.diagnostics {
		if d.level == "Deprecated" {
			t.Errorf("unexpected deprecation for a handler stopped by the limit: %+v", d)
		}
	}
}

// PHP 8.5.12 reports ob_get_level() == 1 inside the only buffer's handler:
// the running buffer stays on the stack until its handler returns.
func TestOutputHandlerRunsOnItsOwnLevel(t *testing.T) {
	r, _ := NewRequest(nil)
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err != nil {
		t.Fatal(err)
	}
	x, err := newExecution(context.Background(), r, 64, nil, executionLimits{32, 64, 4096})
	if err != nil {
		t.Fatal(err)
	}
	level := -1
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := x.run(cleanup, func(x *execution) completion {
		if err := x.startBuffer("{closure:/w/t.php:2}", func(x *execution, b []byte) ([]byte, completion) {
			level = len(x.buffers)
			return b, completion{}
		}); err != nil {
			t.Fatal(err)
		}
		if c := x.write([]byte("body")); c.kind != endNormal {
			return c
		}
		return x.endBuffer("ob_end_flush")
	})
	if level != 1 || string(result.output) != "body" {
		t.Errorf("level in handler=%d output=%q, want 1 and body", level, result.output)
	}
}
