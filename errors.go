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
	// File and Line locate a PHP construct this build cannot run yet, with
	// the file name given to Compile and PHP's line number. They are empty
	// and zero for a request without a source location, such as a release.
	// Line is zero when only the file is known.
	File string
	Line int
	// Option names the Config field or run option that asked for the
	// feature, such as "Limits.Memory" or "WithINI", when the request came
	// from one. It is empty otherwise.
	Option string
	// lineLength is the byte length of line Line of File when the engine
	// knew the source; zero marks the span to the end of the line.
	lineLength int
}

func (e *UnsupportedError) Error() string {
	switch {
	case e.Option != "":
		return "phego: unsupported: " + e.Feature + " (" + e.Option + ")"
	case e.File == "" && e.Line == 0:
		return "phego: unsupported: " + e.Feature
	case e.Line == 0:
		return fmt.Sprintf("phego: unsupported: %s in %s", e.Feature, e.File)
	}
	return fmt.Sprintf("phego: unsupported: %s in %s on line %d", e.Feature, e.File, e.Line)
}

// Diagnostic describes the error in rustc style for diag.Render. A located
// construct reports PHG3001 with its file and line as the primary span; the
// compiler records lines but not columns, so the span covers the whole line
// from column 1. Pass the source through diag.Options.Source to show it.
// Anything else reports PHG3000: a request from a Config field or run option
// has that option as the span, as ConfigError has its field, and one without
// any location, such as a PHP release, has the feature.
func (e *UnsupportedError) Diagnostic() diag.Diagnostic {
	code, primary := codes.Unsupported, diag.Span{File: e.Feature}
	message := e.Feature + " is not supported by this build"
	switch {
	case e.Option != "":
		primary = diag.Span{File: e.Option}
	case e.File != "" || e.Line != 0:
		code = codes.UnsupportedConstruct
		primary = diag.Span{File: e.File, Line: e.Line, Label: "this build's VM cannot run this yet"}
		if e.Line > 0 {
			primary.Column = 1
			primary.Length = e.lineLength
		}
		message = "unsupported PHP construct: " + e.Feature
	}
	entry, _ := codes.Lookup(code)
	return diag.Diagnostic{Code: code, Message: message, Primary: primary, Notes: []string{entry.Note}, Help: []string{entry.Help}}
}

// InternalError reports a fault inside Phego itself: a broken invariant of
// the compiler, verifier or interpreter. It is never a PHP result and never
// the host's mistake; report it with the program that caused it. Op says
// what the engine was doing and Err is the cause.
type InternalError struct {
	Op  string
	Err error
}

func (e *InternalError) Error() string {
	return "phego: internal error while " + e.Op + ": " + e.Err.Error()
}

func (e *InternalError) Unwrap() error { return e.Err }

// Diagnostic describes the error in rustc style for diag.Render, with code
// PHG9000. An internal fault has no source location, so the span names
// what the engine was doing and its label is the cause.
func (e *InternalError) Diagnostic() diag.Diagnostic {
	entry, _ := codes.Lookup(codes.Internal)
	return diag.Diagnostic{
		Code:    codes.Internal,
		Message: entry.Title,
		Primary: diag.Span{File: e.Op, Label: e.Err.Error()},
		Notes:   []string{entry.Note},
		Help:    []string{entry.Help},
	}
}

// ConfigError reports an invalid Config, option, or host function. Code is
// the stable diagnostic code (engineering/diagnostics/codes.md). Errors built by
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

// located records the byte length of u's line in source, so the diagnostic
// marks that line exactly. Lines end at \n, \r\n or \r, as diag counts them.
func located(u *UnsupportedError, source string) *UnsupportedError {
	line, start := 1, 0
	for i := 0; i <= len(source) && line <= u.Line; i++ {
		end := i == len(source) || source[i] == '\n' || source[i] == '\r'
		if !end {
			continue
		}
		if line == u.Line {
			u.lineLength = i - start
			return u
		}
		if i+1 < len(source) && source[i] == '\r' && source[i+1] == '\n' {
			i++
		}
		line, start = line+1, i+1
	}
	return u
}
