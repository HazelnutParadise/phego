package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// RunConfig configures one execution of a Program.
type RunConfig struct {
	Context        context.Context // cancellation of the run
	Cleanup        context.Context // must have a deadline; bounds teardown (see execution.run)
	Request        *Request        // in state Ready
	IntWidth       int             // 32 or 64; must equal the program's profile
	Output         io.Writer       // nil captures output
	Depth          int             // call depth limit (>0); {main} counts as one
	Callbacks      int             // call and diagnostic budget (>0), as executionLimits.callbacks
	Diagnostics    int             // diagnostic budget apart from calls; 0 shares Callbacks
	OutputBytes    int             // output limit (>0)
	Steps          int64           // instruction step budget; 0 means unlimited
	DisplayErrors  bool
	LogErrors      bool
	ErrorReporting int      // PHP error_reporting bitmask; diagnostics outside it are neither displayed nor logged
	Globals        []Global // host-supplied top-level variables, by name
	// Prelude holds the diagnostics PHP raised while compiling the file. They
	// are reported, in order and at their own File and Line, before {main}
	// starts, so display_errors, log_errors and ErrorReporting apply to them.
	Prelude []RunDiagnostic

	// variant selects a benchmark candidate layout; the zero value is the
	// production path. Only tests in this package set it.
	variant vmVariant
}

// Global is a host-supplied top-level variable. Value has exportSnapshot's
// shape: nil, bool, int64, float64, []byte or []SnapshotEntry.
type Global struct {
	Name  string
	Value any
}

// RunEnd says how a run ended.
type RunEnd uint8

const (
	Completed         RunEnd = iota + 1 // the script ended or returned
	Exited                              // exit or die
	UncaughtThrowable                   // a PHP Error reached the top level
	Fatal                               // a PHP fatal error
	Cancelled                           // the context was cancelled
	LimitExceeded                       // a step, depth, call, diagnostic or output limit
	OutputFailed                        // the output writer failed
	Unsupported                         // a construct the stage-1 VM cannot run
	Internal                            // an engine or host-input fault
)

func (e RunEnd) String() string {
	switch e {
	case Completed:
		return "Completed"
	case Exited:
		return "Exited"
	case UncaughtThrowable:
		return "UncaughtThrowable"
	case Fatal:
		return "Fatal"
	case Cancelled:
		return "Cancelled"
	case LimitExceeded:
		return "LimitExceeded"
	case OutputFailed:
		return "OutputFailed"
	case Unsupported:
		return "Unsupported"
	case Internal:
		return "Internal"
	}
	return fmt.Sprintf("RunEnd(%d)", uint8(e))
}

// RunResult is the outcome of Program.Run.
type RunResult struct {
	End         RunEnd
	ExitCode    int             // the process exit status PHP would have (0-255)
	Return      any             // exportSnapshot shape; nil when absent
	Output      []byte          // captured output when Output was nil
	Diagnostics []RunDiagnostic // the log channel, in order
	// Err is the host-level cause for Cancelled, LimitExceeded, OutputFailed,
	// Unsupported (an *UnsupportedError) and Internal; for UncaughtThrowable
	// it is the *Throwable.
	Err        error
	Throwable  *ThrownError // class, message, file, line of an uncaught throwable
	CleanupErr error
	// Steps is the number of instructions charged against RunConfig.Steps.
	Steps int64
}

// RunDiagnostic is one logged PHP diagnostic. Level is PHP's spelling, such
// as "Warning", "Deprecated" or "Fatal error".
type RunDiagnostic struct {
	Level, Message, File string
	Line                 int
}

// ThrownError describes an uncaught PHP throwable.
type ThrownError struct {
	Class, Message, File string
	Line                 int
}

// ErrStepLimit ends a run that used its instruction budget.
var ErrStepLimit = errors.New("instruction step limit exceeded")

// LimitError is the cause of a LimitExceeded run for every budget except
// the step budget (ErrStepLimit). Resource is "frames", "calls", "output" or
// "diagnostics"; Used is what the run had used before Requested more.
type LimitError struct {
	Resource               string
	Limit, Used, Requested int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s limit %d exceeded (used %d, requested %d)", e.Resource, e.Limit, e.Used, e.Requested)
}

// Unwrap keeps errors.Is(err, errExecutionLimit) true for every limit.
func (e *LimitError) Unwrap() error { return errExecutionLimit }

func limitHit(resource string, limit, used, requested int) error {
	return &LimitError{Resource: resource, Limit: int64(limit), Used: int64(used), Requested: int64(requested)}
}

// Run executes the program once. All mutable state belongs to the run.
func (p *Program) Run(cfg RunConfig) RunResult {
	if err := p.checkConfig(cfg); err != nil {
		return RunResult{End: Internal, ExitCode: 255, Err: err}
	}
	x, err := newExecution(cfg.Context, cfg.Request, cfg.IntWidth, cfg.Output, executionLimits{depth: cfg.Depth, callbacks: cfg.Callbacks, output: cfg.OutputBytes})
	if err != nil {
		return RunResult{End: Internal, ExitCode: 255, Err: err}
	}
	x.diagnosticCap = cfg.Diagnostics
	s := newVMState(p, x, cfg)
	res := x.run(cfg.Cleanup, s.body)
	return s.result(res)
}

func (p *Program) checkConfig(cfg RunConfig) error {
	switch {
	case p == nil:
		return errors.New("Run: nil program")
	case cfg.Context == nil || cfg.Cleanup == nil || cfg.Request == nil:
		return errors.New("Run: Context, Cleanup and Request are required")
	case cfg.IntWidth != p.width:
		return fmt.Errorf("Run: the program was compiled for %d-bit integers, not %d", p.width, cfg.IntWidth)
	case cfg.Depth <= 0 || cfg.Callbacks <= 0 || cfg.OutputBytes <= 0:
		return errors.New("Run: Depth, Callbacks and OutputBytes must be positive")
	case cfg.Diagnostics < 0:
		return errors.New("Run: Diagnostics must not be negative")
	case cfg.Steps < 0:
		return errors.New("Run: Steps must not be negative")
	}
	return nil
}

func (s *vmState) result(res executionResult) RunResult {
	out := RunResult{Output: res.output, CleanupErr: res.cleanupErr, ExitCode: 255, Steps: s.steps}
	for _, d := range res.diagnostics {
		out.Diagnostics = append(out.Diagnostics, RunDiagnostic{Level: d.level, Message: d.message, File: d.file, Line: d.line})
	}
	c := res.end
	switch c.kind {
	case endNormal, endReturn:
		out.End, out.ExitCode = Completed, 0
		if c.kind == endReturn {
			out.Return = res.returned
		}
		if s.x.lastExit.kind == endExit {
			out.End, out.ExitCode = Exited, c.code&0xff
		}
	case endExit:
		out.End, out.ExitCode = Exited, c.code&0xff
	case endThrow:
		out.End = UncaughtThrowable
		var u *vmUncaught
		if errors.As(c.err, &u) {
			out.Err = u.thrown
			out.Throwable = &ThrownError{Class: u.thrown.Class, Message: u.thrown.Message, File: u.file, Line: u.line}
		} else {
			out.Err = c.err
		}
	case endFatal:
		out.End, out.Err = Fatal, c.err
	case endCancel:
		out.End, out.Err = Cancelled, c.err
	case endLimit:
		out.End, out.Err = LimitExceeded, c.err
	case endOutput:
		out.End, out.Err = OutputFailed, c.err
	default:
		out.End, out.Err = Internal, c.err
		var u *UnsupportedError
		if errors.As(c.err, &u) {
			out.End = Unsupported
		}
	}
	if out.End == Internal && out.Err == nil {
		out.Err = errors.New("execution ended without a cause")
	}
	return out
}

// vmUncaught is the completion error of an uncaught throwable. Error() is the
// message PHP reports as the fatal error ("Uncaught ...\n  thrown").
type vmUncaught struct {
	thrown *Throwable
	file   string
	line   int
	trace  string
	text   string
}

func (u *vmUncaught) Error() string { return u.text }
func (u *vmUncaught) Unwrap() error { return u.thrown }
