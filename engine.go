package phego

import (
	"context"
	"errors"
	"fmt"
	"github.com/HazelnutParadise/phego/diag/codes"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/policy"
	"github.com/HazelnutParadise/phego/internal/profile"
	"github.com/HazelnutParadise/phego/internal/runtime"
	"github.com/HazelnutParadise/phego/internal/vm"
)

// SupportedRelease is the PHP release whose syntax this build implements.
const SupportedRelease = "8.5.12"

const (
	defaultMaxSourceBytes = 8 << 20
	defaultOutputLimit    = 8 << 20
)

// Config selects the PHP profile, limits, and host functions of an Engine.
// The zero value selects PHP 8.5.12 with 64-bit integers.
type Config struct {
	// Release is the exact PHP release, for example "8.5.12". Empty selects
	// SupportedRelease. A known release this build does not implement yet is
	// an UnsupportedError and a version not recorded in the release
	// inventory is a ConfigError; neither is approximated by another release.
	Release string
	// IntSize is the guest integer width in bits: 32 or 64. Zero selects 64.
	IntSize int
	// ShortOpenTag mirrors the short_open_tag INI setting.
	ShortOpenTag bool
	// MaxSourceBytes limits each compiled source. Zero selects 8 MiB.
	MaxSourceBytes int
	// OutputLimit limits captured output per run. Zero selects 8 MiB.
	//
	// Deprecated: use Limits.Output. Setting both to different values is a
	// ConfigError.
	OutputLimit int
	// Files is a trusted host implementation, as described by FileGrant.FS.
	// It mounts a read-only file system at "/" for RunFile, which then
	// accepts paths relative to it. Nil grants no file access.
	// Used alone, it preserves direct Open path and error behavior. With
	// Policy.Files, it is a parent mount and must meet FileGrant.FS directory
	// enumeration and symlink-verification requirements.
	//
	// Deprecated: use Policy.Files. Mounting both at "/" is a ConfigError.
	Files fs.FS
	// Policy grants the capabilities PHP code may use. The zero value
	// grants nothing.
	Policy Policy
	// Limits bounds what each run may use.
	Limits Limits
	// MaxActive bounds runs executing at once and MaxQueued the runs
	// waiting for a slot. Work beyond both is refused before it starts with
	// a ResourceError. Zero MaxActive sets no bound; MaxQueued then must be
	// zero.
	MaxActive, MaxQueued int
	// HostFunctions are the Go functions PHP code may call. They are
	// validated and snapshotted by NewEngine.
	HostFunctions []HostFunction
}

// HostFunction is a Go function exposed to PHP. Host functions are trusted
// host code: Phego cannot intercept file or network access they perform.
type HostFunction struct {
	Name string
	// MinArgs and MaxArgs bound the argument count; MaxArgs -1 is variadic.
	MinArgs, MaxArgs int
	// Reentrant permits the function to run while another call of the same
	// execution is in progress; otherwise such a call is a ReentrancyError.
	Reentrant bool
	// Capabilities names the host capabilities the function needs.
	Capabilities []string
	Func         func(HostContext, []Value) (Value, error)
}

// HostContext is the restricted view a host function receives for one call.
type HostContext interface {
	// Context is cancelled when the calling execution is cancelled.
	Context() context.Context
	// Function is the registered name being called.
	Function() string
}

// isPHPLabel reports whether s is a PHP label: an ASCII letter, underscore,
// or any byte 0x80-0xff, followed by those or digits. PHP works on bytes, so
// UTF-8 is not required.
func isPHPLabel(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c >= 0x80 || (c|0x20 >= 'a' && c|0x20 <= 'z') || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// isPHPFunctionName reports whether PHP 8.5.12 lexes name as a plain
// function name, which excludes reserved words such as echo or isset.
func isPHPFunctionName(name string) bool {
	if !isPHPLabel(name) {
		return false
	}
	toks, _, err := frontend.Lex([]byte("<?php "+name), frontend.LexOptions{IntWidth: 64, MaxBytes: len(name) + 6})
	return err == nil && len(toks) == 2 && (toks[1].Kind == frontend.TString || toks[1].Kind == frontend.TReadonly)
}

// asciiLower folds only ASCII letters, as PHP 8 does for function names.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// Engine compiles and runs PHP programs. It is safe for concurrent use;
// every run gets an isolated execution.
type Engine struct {
	cfg     Config
	profile profile.Profile
	host    map[string]HostFunction // lower-cased name -> snapshot
	policy  *compiledPolicy
	mu      sync.Mutex
	runs    sync.WaitGroup
	shut    bool
	drain   chan struct{}
	// closing ends when Close is called; every compile and run observes
	// it besides its own context.
	closing      context.Context
	closeRunning context.CancelCauseFunc
	ready        bool // set by NewEngine; a zero Engine is unusable
}

// errZeroEngine rejects use of an Engine not created by NewEngine.
var errZeroEngine = &ConfigError{Code: codes.EngineZero, Field: "Engine", Reason: "was not created by NewEngine"}

// NewEngine validates cfg and returns an engine whose settings cannot change.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.Release == "" {
		cfg.Release = SupportedRelease
	}
	if cfg.IntSize == 0 {
		cfg.IntSize = 64
	}
	prof, err := profile.Resolve(cfg.Release, cfg.IntSize, cfg.ShortOpenTag)
	var pe *profile.Error
	if errors.As(err, &pe) {
		switch {
		case pe.Status == profile.NotImplemented:
			return nil, &UnsupportedError{Feature: "PHP release " + cfg.Release}
		case pe.Status == profile.Unrecorded:
			return nil, &ConfigError{Code: codes.ConfigRelease, Field: "Release", Reason: pe.Reason}
		}
		return nil, &ConfigError{Code: codes.ConfigIntSize, Field: "IntSize", Reason: fmt.Sprintf("%d is not 32 or 64", cfg.IntSize)}
	}
	if err != nil {
		return nil, err
	}
	for name, v := range map[string]*int{"MaxSourceBytes": &cfg.MaxSourceBytes, "OutputLimit": &cfg.OutputLimit} {
		if *v < 0 {
			return nil, &ConfigError{Code: codes.ConfigNegative, Field: name, Reason: "must not be negative"}
		}
	}
	if cfg.MaxSourceBytes == 0 {
		cfg.MaxSourceBytes = defaultMaxSourceBytes
	}
	host := make(map[string]HostFunction, len(cfg.HostFunctions))
	for i, f := range cfg.HostFunctions {
		field := fmt.Sprintf("HostFunctions[%d]", i)
		switch {
		case !isPHPFunctionName(f.Name):
			return nil, &ConfigError{Code: codes.HostFuncName, Field: field, Reason: fmt.Sprintf("name %q is not a PHP function name", f.Name)}
		case f.Func == nil:
			return nil, &ConfigError{Code: codes.HostFuncNil, Field: field, Reason: "Func is nil"}
		case f.MinArgs < 0 || (f.MaxArgs != -1 && f.MaxArgs < f.MinArgs):
			return nil, &ConfigError{Code: codes.HostFuncArgs, Field: field, Reason: fmt.Sprintf("invalid argument range %d..%d", f.MinArgs, f.MaxArgs)}
		}
		seenCap := map[string]bool{}
		for _, c := range f.Capabilities {
			if c == "" || seenCap[c] {
				return nil, &ConfigError{Code: codes.HostFuncCapability, Field: field, Reason: fmt.Sprintf("capability %q is empty or repeated", c)}
			}
			seenCap[c] = true
		}
		key := asciiLower(f.Name)
		if _, dup := host[key]; dup {
			return nil, &ConfigError{Code: codes.HostFuncDuplicate, Field: field, Reason: fmt.Sprintf("duplicate function name %q (PHP names are case-insensitive)", f.Name)}
		}
		f.Capabilities = append([]string(nil), f.Capabilities...)
		host[key] = f
	}
	names := make([]string, 0, len(host))
	for _, f := range host {
		names = append(names, f.Name)
	}
	pol, err := compilePolicy(&cfg, names)
	if err != nil {
		return nil, err
	}
	cfg.HostFunctions = nil
	closing, closeRunning := context.WithCancelCause(context.Background())
	return &Engine{cfg: cfg, profile: prof, host: host, policy: pol, drain: make(chan struct{}),
		closing: closing, closeRunning: closeRunning, ready: true}, nil
}

// Program is an immutable compiled program bound to the engine that made it.
// It may be run concurrently.
type Program struct {
	engine *Engine
	// profile is the profile the program was compiled for. A program never
	// runs under a different profile, even through a shared cache.
	profile profile.Profile
	name    string
	// code is the verified bytecode prepared for the interpreter. It holds
	// no request state, so every run shares it.
	code *runtime.Program
	// prelude holds the warnings and deprecations PHP raises while
	// compiling the file; every run reports them before the script starts.
	prelude []runtime.RunDiagnostic
}

// Name returns the source name used in diagnostics.
func (p *Program) Name() string { return p.name }

// errClosedDuringWork is the cause of compiles and runs that Close ended. It
// matches both ErrEngineClosed and context.Canceled.
var errClosedDuringWork = fmt.Errorf("%w: %w", ErrEngineClosed, context.Canceled)

// observeClose returns a context that also ends, with errClosedDuringWork
// as its cause, when Close is called. stop releases it.
func (e *Engine) observeClose(ctx context.Context) (context.Context, func()) {
	work, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(e.closing, func() { cancel(errClosedDuringWork) })
	return work, func() { stop(); cancel(nil) }
}

// workErr is the error of work that ctx, from observeClose, ended: the
// caller's own context error, or errClosedDuringWork when Close ended it.
func workErr(ctx context.Context) error {
	if cause := context.Cause(ctx); errors.Is(cause, errClosedDuringWork) {
		return cause
	}
	return ctx.Err()
}

// begin registers a run or compile so Close can wait for it.
func (e *Engine) begin() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return errZeroEngine
	}
	if e.shut {
		return ErrEngineClosed
	}
	e.runs.Add(1)
	return nil
}

// Compile parses, compiles and verifies source under the engine's profile.
// name identifies the source in diagnostics. The size limit applies before
// parsing starts. A syntax error is a PHPError of kind ParseError and a fatal
// error PHP raises while compiling is one of kind CompileError. A construct
// this build cannot run yet, or source beyond this build's compiler limits,
// is an UnsupportedError located at its file and line; no partial program
// is returned. Compiling observes ctx and Close throughout: it returns ctx's
// error, or an error matching ErrEngineClosed and context.Canceled when
// Close ended it.
func (e *Engine) Compile(ctx context.Context, name, source string) (*Program, error) {
	if err := e.begin(); err != nil {
		return nil, err
	}
	defer e.runs.Done()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(source) > e.cfg.MaxSourceBytes {
		return nil, &LimitError{Limit: "source size", Max: e.cfg.MaxSourceBytes, Got: len(source)}
	}
	work, done := e.observeClose(ctx)
	defer done()
	p, err := e.compile(work, name, source)
	if work.Err() != nil {
		// Also when the last, uninterruptible step finished after ctx
		// ended: a cancelled compile does not hand out a program.
		return nil, workErr(work)
	}
	return p, err
}

// compile parses source and prepares it for the interpreter.
func (e *Engine) compile(ctx context.Context, name, source string) (*Program, error) {
	tree, err := frontend.ParseContext(ctx, []byte(source), frontend.LexOptions{
		ShortOpenTag: e.cfg.ShortOpenTag, IntWidth: e.cfg.IntSize, MaxBytes: e.cfg.MaxSourceBytes,
	})
	var se *frontend.SyntaxError
	switch {
	case errors.As(err, &se):
		kind := ParseError
		if se.Compile {
			kind = CompileError
		}
		return nil, &PHPError{Kind: kind, Message: se.Message, File: name, Line: se.Line}
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		// The sizes are checked before parsing, so what remains is the
		// parser's own work and nesting bound, which PHP does not have.
		return nil, &UnsupportedError{Feature: "source beyond this build's parser limits (" + err.Error() + ")", File: name}
	}
	return e.compileTree(ctx, name, source, tree)
}

// compileTree lowers a parsed file and prepares it for the interpreter.
func (e *Engine) compileTree(ctx context.Context, name, source string, tree *frontend.Node) (*Program, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unit, err := ir.LowerContext(ctx, tree, e.profile, ir.Options{File: name})
	var ce *ir.CompileError
	var le *ir.LimitError
	switch {
	case errors.As(err, &ce):
		return nil, &PHPError{Kind: CompileError, Message: ce.Message, File: name, Line: ce.Line}
	case errors.As(err, &le):
		// The constant evaluation budget is this build's, as the bytecode
		// limits below are.
		return nil, located(&UnsupportedError{Feature: "a constant expression beyond this build's evaluation limits", File: name, Line: le.Line}, source)
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, internalError("lowering "+name, err)
	}
	image, err := vm.Compile(ctx, unit, []byte(source))
	var vu *vm.UnsupportedError
	var vc *vm.CompileError
	switch {
	case errors.As(err, &vc):
		return nil, &PHPError{Kind: CompileError, Message: vc.Message, File: vc.File, Line: vc.Line}
	case errors.As(err, &vu):
		return nil, located(&UnsupportedError{Feature: vu.Feature, File: vu.File, Line: vu.Line}, source)
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, internalError("compiling "+name, err)
	}
	built, err := bytecode.Build(ctx, image, bytecode.Options{Limits: bytecode.DefaultLimits()})
	var be *bytecode.Error
	switch {
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, bytecode.ErrLimit):
		// The verification limits are this build's, not the host's: a
		// program beyond them is one this build cannot run.
		u := &UnsupportedError{Feature: "a program beyond the bytecode limits", File: name, Line: 1}
		if errors.As(err, &be) {
			if be.Detail != "" {
				u.Feature += " (" + be.Detail + ")"
			}
			if be.Pos.Line > 0 {
				u.Line = int(be.Pos.Line)
			}
		}
		return nil, located(u, source)
	case err != nil:
		return nil, internalError("verifying "+name, err)
	}
	code, err := runtime.NewProgram(built)
	if err != nil {
		return nil, internalError("preparing "+name, err)
	}
	p := &Program{engine: e, profile: e.profile, name: name, code: code}
	for _, d := range unit.Diagnostics {
		p.prelude = append(p.prelude, runtime.RunDiagnostic{Level: d.Level, Message: d.Message, File: name, Line: d.Line})
	}
	return p, nil
}

// Status is how a run ended.
type Status uint8

const (
	// StatusFailed means the run did not complete; the error says why.
	StatusFailed Status = iota
	// StatusCompleted means the script finished normally.
	StatusCompleted
	// StatusExited means the script called exit or die.
	StatusExited
	// StatusCancelled means the context ended the run.
	StatusCancelled
	// StatusPolicyDenied means a capability or limit policy stopped the run.
	StatusPolicyDenied
)

// Result is a Go-owned snapshot of one run. It stays valid after the run's
// execution state is released and never holds live PHP objects.
type Result struct {
	Status Status
	// Return is the top-level return value; PHP null when absent.
	Return Value
	// ExitCode is the PHP exit status: 0 for a completed script, exit's
	// status, and 255 after an uncaught throwable or another failure.
	ExitCode int
	// Output is the captured output when no writer was given. Output
	// written before a failure is kept. With display_errors on (the
	// default, see WithINI) displayed PHP diagnostics are part of it, at
	// the point PHP printed them.
	Output []byte
	// Diagnostics are PHP's log_errors channel: notices, warnings and
	// errors in order, each with the file and line PHP reports. It is empty
	// by default, because log_errors is off by default as with php -n;
	// turn it on with WithINI("log_errors", "1"). It holds at most
	// Limits.Diagnostics entries: later diagnostics are not recorded, so a
	// record of exactly that many may have been cut short.
	Diagnostics []Diagnostic
}

// Diagnostic is one PHP diagnostic message with its location.
type Diagnostic struct {
	Level   string
	Message string
	File    string
	Line    int
}

// RunOption configures one run.
type RunOption func(*runConfig) error

type runConfig struct {
	output io.Writer
	inputs map[string]Value
	ini    iniSettings
}

// WithOutput streams output to w instead of capturing it. A write error
// ends the run and is reported; output is never silently dropped.
func WithOutput(w io.Writer) RunOption {
	return func(c *runConfig) error {
		if w == nil {
			return &ConfigError{Code: codes.RunOutputNil, Field: "WithOutput", Reason: "writer is nil"}
		}
		c.output = w
		return nil
	}
}

// WithGlobal defines a global variable before the script starts. The value
// is already a Go-owned copy, so later changes by the caller cannot reach it.
func WithGlobal(name string, v Value) RunOption {
	return func(c *runConfig) error {
		if !isPHPLabel(name) || name == "this" {
			return &ConfigError{Code: codes.RunGlobalName, Field: "WithGlobal", Reason: fmt.Sprintf("%q is not a PHP variable name", name)}
		}
		if c.inputs == nil {
			c.inputs = map[string]Value{}
		}
		c.inputs[name] = v
		return nil
	}
}

// Run executes program in a new isolated execution. Output written before a
// failure is kept in Result.Output (or was written to WithOutput's writer).
// The error says why a run did not complete: a PHPError for an uncaught
// throwable (ExitCode 255, as PHP exits), a ResourceError for a limit, the
// context's error when ctx ends the run (one matching ErrEngineClosed and
// context.Canceled when Close ended it), the writer's error, an
// UnsupportedError for a construct this build cannot run or for a
// Limits.Memory it cannot enforce, or an InternalError for a fault inside
// Phego. A failed request cleanup after the script ended is also an error,
// joined to any other; Status still says how the script ended.
func (e *Engine) Run(ctx context.Context, program *Program, opts ...RunOption) (*Result, error) {
	return e.run(ctx, program, nil, opts...)
}

// run reuses the slot RunFile already holds when release is not nil.
func (e *Engine) run(ctx context.Context, program *Program, release func(), opts ...RunOption) (*Result, error) {
	if err := e.begin(); err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer e.runs.Done()
	if program == nil || program.engine != e {
		return &Result{Status: StatusFailed}, &ConfigError{Code: codes.ProgramForeign, Field: "program", Reason: "was not compiled by this engine"}
	}
	if program.profile != e.profile {
		return &Result{Status: StatusFailed}, &ConfigError{Code: codes.ProgramProfile, Field: "program",
			Reason: "was compiled for " + program.profile.String() + ", not " + e.profile.String()}
	}
	if e.policy.limits[policy.Memory] > 0 {
		// The interpreter does not account memory yet, so a run that the
		// host bounded by memory is refused instead of running unbounded.
		return &Result{Status: StatusFailed}, &UnsupportedError{Feature: "enforcing a memory limit", Option: "Limits.Memory"}
	}
	rc := runConfig{ini: defaultINI}
	for _, opt := range opts {
		if opt == nil {
			return &Result{Status: StatusFailed}, &ConfigError{Code: codes.RunOptionNil, Field: "RunOption", Reason: "is nil"}
		}
		if err := opt(&rc); err != nil {
			return &Result{Status: StatusFailed}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return &Result{Status: StatusCancelled}, err
	}
	if release == nil {
		var err error
		release, err = e.admit(ctx)
		if err != nil {
			var re *ResourceError
			switch {
			case errors.As(err, &re):
				return &Result{Status: StatusPolicyDenied}, err
			case errors.Is(err, ErrEngineClosed):
				// Close ended the wait for a slot.
				return &Result{Status: StatusCancelled}, errClosedDuringWork
			case ctx.Err() != nil:
				return &Result{Status: StatusCancelled}, err
			}
			return &Result{Status: StatusFailed}, err
		}
	}
	return e.execute(ctx, program, &rc, release)
}

// RunFile reads path through Policy.Files, or directly through legacy
// Config.Files when used alone, then compiles and runs it, as php path does:
// a device or FIFO the backend opens is read as the script, a directory runs
// as an empty script, and a socket cannot be opened. On a host directory a
// FIFO waits for a writer, and the system's open error is returned for what
// cannot be opened, such as a terminal without a controlling tty. Reading
// stops at MaxSourceBytes, and when ctx ends while the file is opened or
// read, RunFile reports StatusCancelled. Opening a file through an fs.FS or
// Files is the host's code and is not interrupted; it may wait for a FIFO
// writer.
func (e *Engine) RunFile(ctx context.Context, path string, opts ...RunOption) (*Result, error) {
	if err := e.begin(); err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer e.runs.Done()
	if err := ctx.Err(); err != nil {
		return &Result{Status: StatusCancelled}, err
	}
	if e.policy.limits[policy.Memory] > 0 {
		// Refused before anything is opened: see run.
		return &Result{Status: StatusFailed}, &UnsupportedError{Feature: "enforcing a memory limit", Option: "Limits.Memory"}
	}
	// Close also ends the open, the read and the compile.
	ctx, done := e.observeClose(ctx)
	defer done()
	release, err := e.admit(ctx)
	if err != nil {
		status := StatusFailed
		var re *ResourceError
		switch {
		case errors.As(err, &re):
			status = StatusPolicyDenied
		case errors.Is(err, ErrEngineClosed):
			return &Result{Status: StatusCancelled}, errClosedDuringWork
		case ctx.Err() != nil:
			status = StatusCancelled
		}
		return &Result{Status: status}, err
	}
	// The run hands the slot to its request; releasing again is a no-op.
	defer release()
	var f fs.File
	if e.cfg.Files != nil && len(e.cfg.Policy.Files) == 0 {
		// Keep the legacy FS's path interpretation and errors unchanged.
		f, err = e.cfg.Files.Open(path)
	} else {
		phpPath := path
		if !strings.HasPrefix(phpPath, "/") {
			phpPath = "/" + phpPath
		}
		f, err = e.openFile(ctx, phpPath)
	}
	var pe *PolicyError
	if errors.As(err, &pe) {
		if len(e.cfg.Policy.Files) == 0 && e.cfg.Files == nil {
			pe.Rule = RuleNotGranted // nothing is mounted at all
		}
		return &Result{Status: StatusPolicyDenied}, pe
	}
	if err != nil {
		status := StatusFailed
		if ctx.Err() != nil {
			// The policy ends its waits, such as a FIFO's wait for a
			// writer, when ctx ends.
			status = StatusCancelled
		}
		return &Result{Status: status}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	// PHP 8.5.12 CLI: a directory opens but reads nothing, so it runs as an
	// empty script; open(2) fails on a socket ("Could not open input file").
	var data []byte
	switch mode := info.Mode(); {
	case mode&fs.ModeSocket != 0:
		return &Result{Status: StatusFailed}, &fs.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	case mode.IsDir():
	default:
		data, err = e.readSource(ctx, f, info, path)
		if ctx.Err() != nil && err != nil {
			// As for the open: the read's error carries context.Cause(ctx).
			return &Result{Status: StatusCancelled}, err
		}
		if err != nil {
			return &Result{Status: StatusFailed}, err
		}
	}
	if len(data) > e.cfg.MaxSourceBytes {
		return &Result{Status: StatusFailed}, &LimitError{Limit: "source size", Max: e.cfg.MaxSourceBytes, Got: len(data)}
	}
	program, err := e.Compile(ctx, path, string(data))
	if err != nil {
		if ctx.Err() != nil {
			return &Result{Status: StatusCancelled}, workErr(ctx)
		}
		return &Result{Status: StatusFailed}, err
	}
	return e.run(ctx, program, release, opts...)
}

// Close stops admitting work, cancels the compiles and runs in progress and
// waits until they have finished, or until ctx ends, which returns ctx's
// error. Cancelled work ends as when its own context ends: a run reports
// StatusCancelled with an error matching both ErrEngineClosed and
// context.Canceled, and its request cleanup, bounded in time, releases its
// execution slot. Later work returns ErrEngineClosed. Close is safe to call
// more than once; later calls wait the same way.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.ready {
		e.mu.Unlock()
		return errZeroEngine
	}
	if !e.shut {
		e.shut = true
		if e.policy.admission != nil {
			e.policy.admission.Close()
		}
		e.closeRunning(ErrEngineClosed)
		go func() {
			e.runs.Wait()
			close(e.drain)
		}()
	}
	e.mu.Unlock()
	select {
	case <-e.drain:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run compiles and executes source once with a default PHP 8.5.12 engine.
func Run(ctx context.Context, source string, opts ...RunOption) (*Result, error) {
	e, err := NewEngine(Config{})
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer e.Close(context.Background())
	program, err := e.Compile(ctx, "Standard input code", source)
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	return e.Run(ctx, program, opts...)
}

// readSource reads one byte past MaxSourceBytes, so a larger source is
// detected. A FIFO or device can block, as php path waits for a FIFO's
// writer. A FIFO or device from a host directory mount ends its own waits
// when ctx ends, and one that an fs.FS or Files opened as an *os.File is read
// with the same waits (policy.ReadHostFile). Any other fs.File is the host's
// trusted code and is read as it is. path names the file in errors.
func (e *Engine) readSource(ctx context.Context, f fs.File, info fs.FileInfo, path string) ([]byte, error) {
	limit := int64(e.cfg.MaxSourceBytes)
	if limit < math.MaxInt64 {
		limit++
	}
	if hf, ok := f.(*os.File); ok && !info.Mode().IsRegular() {
		return policy.ReadHostFile(ctx, hf, path, limit)
	}
	return io.ReadAll(io.LimitReader(f, limit))
}
