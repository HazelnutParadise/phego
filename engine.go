package phego

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
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
	OutputLimit int
	// Files grants RunFile read access. Nil grants no file access.
	Files fs.FS
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
	mu      sync.Mutex
	runs    sync.WaitGroup
	shut    bool
	drain   chan struct{}
	ready   bool // set by NewEngine; a zero Engine is unusable
}

// errZeroEngine rejects use of an Engine not created by NewEngine.
var errZeroEngine = &ConfigError{Field: "Engine", Reason: "was not created by NewEngine"}

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
			return nil, &ConfigError{Field: "Release", Reason: pe.Reason}
		}
		return nil, &ConfigError{Field: "IntSize", Reason: fmt.Sprintf("%d is not 32 or 64", cfg.IntSize)}
	}
	if err != nil {
		return nil, err
	}
	for name, v := range map[string]*int{"MaxSourceBytes": &cfg.MaxSourceBytes, "OutputLimit": &cfg.OutputLimit} {
		if *v < 0 {
			return nil, &ConfigError{Field: name, Reason: "must not be negative"}
		}
	}
	if cfg.MaxSourceBytes == 0 {
		cfg.MaxSourceBytes = defaultMaxSourceBytes
	}
	if cfg.OutputLimit == 0 {
		cfg.OutputLimit = defaultOutputLimit
	}
	host := make(map[string]HostFunction, len(cfg.HostFunctions))
	for i, f := range cfg.HostFunctions {
		field := fmt.Sprintf("HostFunctions[%d]", i)
		switch {
		case !isPHPFunctionName(f.Name):
			return nil, &ConfigError{Field: field, Reason: fmt.Sprintf("name %q is not a PHP function name", f.Name)}
		case f.Func == nil:
			return nil, &ConfigError{Field: field, Reason: "Func is nil"}
		case f.MinArgs < 0 || (f.MaxArgs != -1 && f.MaxArgs < f.MinArgs):
			return nil, &ConfigError{Field: field, Reason: fmt.Sprintf("invalid argument range %d..%d", f.MinArgs, f.MaxArgs)}
		}
		seenCap := map[string]bool{}
		for _, c := range f.Capabilities {
			if c == "" || seenCap[c] {
				return nil, &ConfigError{Field: field, Reason: fmt.Sprintf("capability %q is empty or repeated", c)}
			}
			seenCap[c] = true
		}
		key := asciiLower(f.Name)
		if _, dup := host[key]; dup {
			return nil, &ConfigError{Field: field, Reason: fmt.Sprintf("duplicate function name %q (PHP names are case-insensitive)", f.Name)}
		}
		f.Capabilities = append([]string(nil), f.Capabilities...)
		host[key] = f
	}
	cfg.HostFunctions = nil
	return &Engine{cfg: cfg, profile: prof, host: host, drain: make(chan struct{}), ready: true}, nil
}

// Program is an immutable compiled program bound to the engine that made it.
// It may be run concurrently.
type Program struct {
	engine *Engine
	// profile is the profile the program was compiled for. A program never
	// runs under a different profile, even through a shared cache.
	profile profile.Profile
	name    string
	tree    *frontend.Node
}

// Name returns the source name used in diagnostics.
func (p *Program) Name() string { return p.name }

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

// Compile parses source under the engine's profile. name identifies the
// source in diagnostics. The size limit applies before parsing starts.
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
	tree, err := frontend.Parse([]byte(source), frontend.LexOptions{
		ShortOpenTag: e.cfg.ShortOpenTag, IntWidth: e.cfg.IntSize, MaxBytes: e.cfg.MaxSourceBytes,
	})
	var se *frontend.SyntaxError
	if errors.As(err, &se) {
		kind := ParseError
		if se.Compile {
			kind = CompileError
		}
		return nil, &PHPError{Kind: kind, Message: se.Message, File: name, Line: se.Line}
	}
	if err != nil {
		return nil, err
	}
	return &Program{engine: e, profile: e.profile, name: name, tree: tree}, nil
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
	// ExitCode is the PHP exit status.
	ExitCode int
	// Output is the captured output when no writer was given. Output
	// written before a failure is kept.
	Output []byte
	// Diagnostics are PHP notices, warnings, and errors in order.
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
}

// WithOutput streams output to w instead of capturing it. A write error
// ends the run and is reported; output is never silently dropped.
func WithOutput(w io.Writer) RunOption {
	return func(c *runConfig) error {
		if w == nil {
			return &ConfigError{Field: "WithOutput", Reason: "writer is nil"}
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
			return &ConfigError{Field: "WithGlobal", Reason: fmt.Sprintf("%q is not a PHP variable name", name)}
		}
		if c.inputs == nil {
			c.inputs = map[string]Value{}
		}
		c.inputs[name] = v
		return nil
	}
}

// Run executes program in a new isolated execution.
func (e *Engine) Run(ctx context.Context, program *Program, opts ...RunOption) (*Result, error) {
	if err := e.begin(); err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer e.runs.Done()
	if program == nil || program.engine != e {
		return &Result{Status: StatusFailed}, &ConfigError{Field: "program", Reason: "was not compiled by this engine"}
	}
	if program.profile != e.profile {
		return &Result{Status: StatusFailed}, &ConfigError{Field: "program",
			Reason: "was compiled for " + program.profile.String() + ", not " + e.profile.String()}
	}
	var rc runConfig
	for _, opt := range opts {
		if opt == nil {
			return &Result{Status: StatusFailed}, &ConfigError{Field: "RunOption", Reason: "is nil"}
		}
		if err := opt(&rc); err != nil {
			return &Result{Status: StatusFailed}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return &Result{Status: StatusCancelled}, err
	}
	// Execution arrives with the VM; until then a run never claims success.
	return &Result{Status: StatusFailed}, &UnsupportedError{Feature: "PHP execution"}
}

// RunFile reads path through Config.Files, then compiles and runs it.
func (e *Engine) RunFile(ctx context.Context, path string, opts ...RunOption) (*Result, error) {
	if err := e.begin(); err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer e.runs.Done()
	if err := ctx.Err(); err != nil {
		return &Result{Status: StatusCancelled}, err
	}
	if e.cfg.Files == nil {
		return &Result{Status: StatusPolicyDenied}, &ConfigError{Field: "Files", Reason: "file access is not granted"}
	}
	f, err := e.cfg.Files.Open(path)
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(e.cfg.MaxSourceBytes)+1))
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	if len(data) > e.cfg.MaxSourceBytes {
		return &Result{Status: StatusFailed}, &LimitError{Limit: "source size", Max: e.cfg.MaxSourceBytes, Got: len(data)}
	}
	program, err := e.Compile(ctx, path, string(data))
	if err != nil {
		return &Result{Status: StatusFailed}, err
	}
	return e.Run(ctx, program, opts...)
}

// Close stops new work and waits for running work until ctx ends. It does
// not yet cancel work in progress; that arrives with execution. It is safe to
// call more than once; later calls wait the same way.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.ready {
		e.mu.Unlock()
		return errZeroEngine
	}
	if !e.shut {
		e.shut = true
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
