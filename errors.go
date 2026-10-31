package phego

import (
	"errors"
	"fmt"
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

// ConfigError reports an invalid Config, option, or host function.
type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string { return "phego: invalid " + e.Field + ": " + e.Reason }

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
