package phego

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/HazelnutParadise/phego/diag/codes"
	"github.com/HazelnutParadise/phego/internal/policy"
	"github.com/HazelnutParadise/phego/internal/runtime"
)

const (
	// eAll is PHP 8.5.12's E_ALL, the error_reporting of php -n.
	eAll = 30719
	// Limits of one WithGlobal value and of Result.Return; they equal the
	// runtime's snapshot limits, so a returned value can be passed back.
	maxGlobalDepth    = runtime.SnapshotMaxDepth
	maxGlobalElements = runtime.SnapshotMaxElements
)

// cleanupTimeout bounds request teardown after a run, including one whose
// context has ended, from the moment teardown starts. Past it the slot is
// released and the run's cleanup error says that teardown did not finish.
// It is a variable only so tests can shorten it.
var cleanupTimeout = 5 * time.Second

// iniSettings are the INI values one run uses.
type iniSettings struct {
	display, log bool
	reporting    int
}

// defaultINI is php -n on the CLI (PHP 8.5.12): display_errors "1",
// log_errors "0", html_errors "0" and error_reporting unset, which is E_ALL.
var defaultINI = iniSettings{display: true, reporting: eAll}

// WithINI sets one INI value for this run; when a name is given more than
// once, the last value wins. Without it a run uses php -n's CLI values:
// display_errors=1, log_errors=0, html_errors=0 and error_reporting=E_ALL
// (30719).
//
// display_errors and log_errors take "0", "1", "on", "off", "true", "false",
// "yes" or "no", in any letter case, as PHP's ini boolean parser reads them.
// html_errors takes the values that mean off; on is an UnsupportedError,
// because HTML error text is not implemented. error_reporting takes a
// decimal integer that fits 32 bits, such as "32767" or "-1"; constant
// expressions such as "E_ALL" are not accepted. Any other name or value is a
// ConfigError.
//
// As in PHP, a displayed diagnostic is written into the output stream
// (Result.Output or WithOutput's writer) at the point it is raised, and the
// log copy of every diagnostic goes to Result.Diagnostics. error_reporting
// filters both.
func WithINI(name, value string) RunOption {
	return func(c *runConfig) error {
		invalid := func(want string) error {
			return &ConfigError{Code: codes.RunINIValue, Field: "WithINI", Reason: fmt.Sprintf("%s=%q: %s", name, value, want)}
		}
		switch name {
		case "display_errors", "log_errors":
			on, ok := iniBool(value)
			if !ok {
				return invalid("expected 0, 1, on, off, true, false, yes or no")
			}
			if name == "display_errors" {
				c.ini.display = on
			} else {
				c.ini.log = on
			}
		case "html_errors":
			on, ok := iniBool(value)
			switch {
			case !ok:
				return invalid("expected 0, 1, on, off, true, false, yes or no")
			case on:
				return &UnsupportedError{Feature: "HTML error text (html_errors=" + value + ")", Option: "WithINI"}
			}
		case "error_reporting":
			n, ok := iniInt(value)
			if !ok {
				return invalid("expected a decimal integer that fits 32 bits, such as 32767 or -1")
			}
			c.ini.reporting = n
		default:
			return &ConfigError{Code: codes.RunININame, Field: "WithINI", Reason: fmt.Sprintf("%q is not a setting a run accepts", name)}
		}
		return nil
	}
}

// iniBool reads the spellings whose meaning PHP 8.5.12 gives both
// display_errors and log_errors the same way. Others ("stderr", "2", "abc")
// mean different things to the two settings and are refused.
func iniBool(v string) (on, ok bool) {
	switch strings.ToLower(v) {
	case "1", "on", "true", "yes":
		return true, true
	case "0", "off", "false", "no":
		return false, true
	}
	return false, false
}

// iniInt reads an optionally signed decimal integer in int32 range, which
// PHP's error_reporting reads as that same number.
func iniInt(v string) (int, bool) {
	digits := strings.TrimLeft(v, "+-")
	if len(v)-len(digits) > 1 || digits == "" || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 32)
	return int(n), err == nil
}

// execute runs program in a new request that owns release.
func (e *Engine) execute(ctx context.Context, program *Program, rc *runConfig, release func()) (*Result, error) {
	// The request's cleanup releases the slot; this covers runs that end
	// before the request starts. A second release is a no-op.
	defer release()
	ctx, done := e.observeClose(ctx)
	defer done()
	globals, err := e.globals(rc.inputs)
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	req, err := runtime.NewRequest(nil)
	if err == nil {
		err = req.Admit(func(context.Context) error { release(); return nil })
	}
	if err == nil {
		err = req.BeginInit()
	}
	if err == nil {
		err = req.Ready()
	}
	if err != nil {
		return &Result{Status: StatusFailed}, internalError("starting the request", err)
	}
	limits := e.policy.limits
	// Zero is no limit, as PHP has none. The interpreter keeps PHP frames
	// on its own stack, not the Go stack, so no depth overflows the
	// goroutine; until memory is metered only Steps, the context and the
	// process's memory stop runaway recursion.
	frames := int(min(limits[policy.Frames], int64(maxInt)))
	if frames == 0 {
		frames = maxInt
	}
	res := program.code.Run(runtime.RunConfig{
		Context: ctx,
		// Teardown runs after ctx may have ended, so it has its own
		// deadline, which starts when teardown starts.
		Cleanup:        context.WithoutCancel(ctx),
		CleanupTimeout: cleanupTimeout,
		Request:        req,
		IntWidth:       e.profile.IntWidth,
		Output:         rc.output,
		Depth:          frames,
		Callbacks:      maxInt, // calls are bounded by Steps and Frames
		Diagnostics:    int(min(limits[policy.Diagnostics], int64(maxInt))),
		OutputBytes:    int(min(limits[policy.Output], int64(maxInt))),
		Steps:          limits[policy.Steps],
		DisplayErrors:  rc.ini.display,
		LogErrors:      rc.ini.log,
		ErrorReporting: rc.ini.reporting,
		Globals:        globals,
		Prelude:        program.prelude,
	})
	return e.result(ctx, res)
}

// result maps the interpreter's outcome to the public Result and error. ctx
// is the run's context from observeClose.
func (e *Engine) result(ctx context.Context, res runtime.RunResult) (*Result, error) {
	r := &Result{Status: StatusFailed, ExitCode: res.ExitCode, Output: res.Output}
	for _, d := range res.Diagnostics {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Level: d.Level, Message: d.Message, File: d.File, Line: d.Line})
	}
	var err error
	switch res.End {
	case runtime.Completed:
		r.Status, r.Return = StatusCompleted, fromSnapshot(res.Return)
	case runtime.Exited:
		r.Status = StatusExited
	case runtime.UncaughtThrowable:
		if t := res.Throwable; t != nil {
			err = &PHPError{Kind: UncaughtThrowable, Message: t.Class + ": " + t.Message, File: t.File, Line: t.Line}
		} else {
			err = internalError("an uncaught throwable without a description", res.Err)
		}
	case runtime.Fatal:
		// PHPErrorKind has no kind for a run-time fatal error yet and stage
		// 1 raises none, so reaching one is an engine fault; it is never
		// reported as success.
		err = internalError("running the program (a PHP fatal error without a reportable kind)", res.Err)
	case runtime.Cancelled:
		r.Status, err = StatusCancelled, res.Err
		if errors.Is(context.Cause(ctx), errClosedDuringWork) {
			err = errClosedDuringWork
		}
	case runtime.LimitExceeded:
		r.Status, err = StatusPolicyDenied, e.limitError(res)
	case runtime.OutputFailed:
		err = res.Err
	case runtime.Unsupported:
		var u *runtime.UnsupportedError
		if errors.As(res.Err, &u) {
			err = &UnsupportedError{Feature: u.Feature, File: u.File, Line: u.Line}
		} else {
			err = &UnsupportedError{Feature: fmt.Sprint(res.Err)}
		}
	default:
		err = internalError("running the program", res.Err)
	}
	if res.CleanupErr != nil {
		// Status still says how the script ended; the error says both why
		// it did not complete and that teardown failed.
		err = errors.Join(err, internalError("cleaning up after the run", res.CleanupErr))
	}
	return r, err
}

// limitError names the budget that ended a run, as the policy meter does.
func (e *Engine) limitError(res runtime.RunResult) error {
	if errors.Is(res.Err, runtime.ErrStepLimit) {
		return &ResourceError{Resource: ResourceSteps, Limit: e.policy.limits[policy.Steps], Used: res.Steps, Requested: 1}
	}
	var le *runtime.LimitError
	if errors.As(res.Err, &le) {
		resource := map[string]Resource{"frames": ResourceFrames, "output": ResourceOutput, "diagnostics": ResourceDiagnostics,
			"return-depth": ResourceReturnDepth, "return-elements": ResourceReturnElements}[le.Resource]
		if resource != "" {
			return &ResourceError{Resource: resource, Limit: le.Limit, Used: le.Used, Requested: le.Requested}
		}
	}
	return internalError("an unexpected run limit", res.Err)
}

// internalError reports an engine fault. It is never a PHP result.
func internalError(during string, err error) error {
	if err == nil {
		err = errors.New("no cause recorded")
	}
	return &InternalError{Op: during, Err: err}
}

// globals converts WithGlobal values to the runtime's snapshot shape and
// checks them against the profile first, so a value PHP cannot hold is the
// host's ConfigError and not an engine fault.
func (e *Engine) globals(inputs map[string]Value) ([]runtime.Global, error) {
	out := make([]runtime.Global, 0, len(inputs))
	for name, v := range inputs {
		elements := maxGlobalElements
		snap, err := e.toSnapshot(v, maxGlobalDepth, &elements)
		if err != nil {
			return nil, &ConfigError{Code: codes.RunGlobalValue, Field: "WithGlobal", Reason: "$" + name + ": " + err.Error()}
		}
		out = append(out, runtime.Global{Name: name, Value: snap})
	}
	return out, nil
}

func (e *Engine) toSnapshot(v Value, depth int, elements *int) (any, error) {
	switch v.kind {
	case kindBool:
		return v.boolean, nil
	case kindInt:
		if e.profile.IntWidth == 32 && (v.integer < math.MinInt32 || v.integer > math.MaxInt32) {
			return nil, fmt.Errorf("integer %d does not fit a 32-bit PHP integer", v.integer)
		}
		return v.integer, nil
	case kindFloat:
		return v.floating, nil
	case kindBytes:
		return []byte(v.text), nil
	case kindArray:
		if depth == 0 {
			return nil, fmt.Errorf("arrays nest deeper than %d levels", maxGlobalDepth)
		}
		entries := make([]runtime.SnapshotEntry, 0, len(v.entries))
		for _, en := range v.entries {
			if *elements == 0 {
				return nil, fmt.Errorf("arrays hold more than %d elements", maxGlobalElements)
			}
			*elements--
			var key any = en.Key.text
			if !en.Key.isString {
				k, err := e.toSnapshot(Int64(en.Key.integer), depth, elements)
				if err != nil {
					return nil, err
				}
				key = k
			}
			child, err := e.toSnapshot(en.Value, depth-1, elements)
			if err != nil {
				return nil, err
			}
			entries = append(entries, runtime.SnapshotEntry{Key: key, Value: child})
		}
		return entries, nil
	}
	return nil, nil
}

// fromSnapshot converts a returned snapshot to a Value; arrays keep order.
func fromSnapshot(s any) Value {
	switch x := s.(type) {
	case bool:
		return Bool(x)
	case int64:
		return Int64(x)
	case float64:
		return Float64(x)
	case []byte:
		return Value{kind: kindBytes, text: string(x)}
	case []runtime.SnapshotEntry:
		var entries []Entry // nil when empty, as Array(nil) builds it
		if len(x) > 0 {
			entries = make([]Entry, len(x))
		}
		for i, en := range x {
			switch k := en.Key.(type) {
			case int64:
				entries[i].Key = IntKey(k)
			case string:
				entries[i].Key = StringKey(k)
			}
			entries[i].Value = fromSnapshot(en.Value)
		}
		return Value{kind: kindArray, entries: entries}
	}
	return Null()
}
