package phego

import (
	"errors"
	"fmt"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/diag/codes"
)

// ErrEngineClosed is returned by work started after Engine.Close.
var ErrEngineClosed = errors.New("phego: engine is closed")

// PHPErrorKind classifies a PHP-level failure.
type PHPErrorKind uint8

const (
	// ParseError is a syntax error found before execution.
	ParseError PHPErrorKind = iota + 1
	// CompileError is an error PHP raises while compiling valid syntax.
	CompileError
	// UncaughtThrowable is a top-level exception or error PHP did not catch.
	UncaughtThrowable
)

func (k PHPErrorKind) String() string {
	switch k {
	case ParseError:
		return "ParseError"
	case CompileError:
		return "CompileError"
	case UncaughtThrowable:
		return "uncaught throwable"
	}
	return "unknown PHP error"
}

// PHPError reports a PHP failure with its source location. It never panics
// the host.
type PHPError struct {
	Kind    PHPErrorKind
	Message string
	File    string
	Line    int
}

func (e *PHPError) Error() string {
	return fmt.Sprintf("phego: PHP %s: %s in %s on line %d", e.Kind, e.Message, e.File, e.Line)
}

// UnsupportedError reports a request this build cannot honour, such as an
// unimplemented feature or PHP release. It is never reported as success.
type UnsupportedError struct {
	Feature string
}

func (e *UnsupportedError) Error() string { return "phego: unsupported: " + e.Feature }

// ConfigError reports an invalid Config, option, or host function. Code is
// the stable diagnostic code (docs/diagnostics/codes.md). Errors built by
// host code may leave it empty and then report PHG1000; a code that is not
// in the table is kept, with PHG1000's note and help.
type ConfigError struct {
	Field  string
	Reason string
	Code   diag.Code
}

func (e *ConfigError) Error() string { return "phego: invalid " + e.Field + ": " + e.Reason }

// Diagnostic describes the error in rustc style for diag.Render. The primary
// span names the Config field or run option, since Go configuration has no
// source file, so the field is not repeated: the message is the code's
// title and the reason labels the span. An error without a code of its own
// has no title that says what is wrong, so its reason is the message.
// Error() stays the one-line form with field and reason.
func (e *ConfigError) Diagnostic() diag.Diagnostic {
	code := e.Code
	entry, ok := codes.Lookup(code)
	message, label := entry.Title, e.Reason
	if !ok {
		entry, _ = codes.Lookup(codes.ConfigOther)
		if code == "" {
			code = codes.ConfigOther
		}
		message, label = e.Reason, ""
	}
	return diag.Diagnostic{
		Code:    code,
		Message: message,
		Primary: diag.Span{File: e.Field, Label: label},
		Notes:   []string{entry.Note},
		Help:    []string{entry.Help},
	}
}

// Diagnoser is implemented by Phego errors that can describe themselves as a
// structured diagnostic.
type Diagnoser interface {
	Diagnostic() diag.Diagnostic
}

// Diagnose returns the structured diagnostic of the first error in err's
// chain that implements Diagnoser. It reports false for nil and for errors
// without one, such as context cancellation. PHP-level failures keep PHP's
// own messages and are reported through PHPError, not here.
func Diagnose(err error) (diag.Diagnostic, bool) {
	var d Diagnoser
	if errors.As(err, &d) {
		return d.Diagnostic(), true
	}
	return diag.Diagnostic{}, false
}

// LimitError reports input rejected by a configured limit before it was
// processed.
type LimitError struct {
	Limit string
	Max   int
	Got   int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("phego: %s exceeds the limit of %d (got %d)", e.Limit, e.Max, e.Got)
}

// HostError wraps an error or panic from a host function. A panic is
// recovered at the boundary and never propagates into the engine.
type HostError struct {
	Function string
	Err      error
}

func (e *HostError) Error() string {
	return "phego: host function " + e.Function + " failed: " + e.Err.Error()
}

func (e *HostError) Unwrap() error { return e.Err }

// ReentrancyError reports a host function re-entering an execution that it
// was not registered to re-enter.
type ReentrancyError struct {
	Function string
}

func (e *ReentrancyError) Error() string {
	return "phego: host function " + e.Function + " does not allow re-entrant calls"
}

// PolicyError reports an operation the engine's Policy refused. Rule identifies
// the capability or safety refusal, or a heuristic rule matched in ModeEnforce.
type PolicyError struct {
	Capability Capability
	Op         string
	Target     string
	Rule       string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("phego: %s %s %q refused (%s)", e.Capability, e.Op, e.Target, e.Rule)
}

// ResourceError reports a budget that would be exceeded during a run, or
// an Engine with no free execution slot. LimitError is used instead for
// input rejected before any processing.
type ResourceError struct {
	Resource  Resource
	Limit     int64
	Used      int64
	Requested int64
}

func (e *ResourceError) Error() string {
	return fmt.Sprintf("phego: %s limit %d exceeded (used %d, requested %d)", e.Resource, e.Limit, e.Used, e.Requested)
}
