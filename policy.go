package phego

import (
	"context"
	"errors"
	"fmt"
	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/internal/diagcodes"
	"io/fs"
	"os"
	"strings"

	"github.com/HazelnutParadise/phego/internal/policy"
)

// Policy grants capabilities to every execution of an Engine. The zero value
// grants nothing. Policy modes change how rule events are handled; no mode
// widens the granted capabilities or lifts a limit. Use keyed fields: more
// capabilities are added as the features that need them arrive.
type Policy struct {
	Mode Mode
	// Files mounts host directories or file systems at PHP-visible paths.
	Files []FileGrant
	// Network lists the destinations PHP may connect to.
	Network []NetworkGrant
	// DynamicCode permits include, require and eval of code not compiled up
	// front. Included files still need a FileGrant.
	DynamicCode bool
}

// Mode is a policy mode.
type Mode uint8

const (
	// ModeCompat keeps PHP's normal behaviour inside the granted
	// capabilities and records known differences.
	ModeCompat Mode = iota
	// ModeObserve also records suspicious behaviour without blocking it.
	ModeObserve
	// ModeEnforce blocks operations that match an explicit rule.
	ModeEnforce
)

// FileGrant mounts a directory tree at Path, the absolute path PHP sees.
// Exactly one of Root and FS is set.
type FileGrant struct {
	Path string
	// Root is a host directory opened through os.Root. Absolute symbolic links
	// may target another granted Root; parent components in link targets may
	// not leave their selected mount. Regular files, directories and the host's
	// null, zero, full, random and urandom character devices are supported;
	// other special files are refused. The host must control the layout,
	// including existing hard links.
	Root string
	// FS is a trusted host implementation of a read-only source, for example
	// an embed.FS. It must confine reads to the authorized data and must not
	// perform unauthorized side effects. Phego cannot inspect its internals.
	// Use Root for host directories: os.DirFS follows escaping symlinks and
	// does not provide the required confinement.
	// Where a nested mount shadows this source, directories must support
	// fs.ReadDirFile and enumerate exact names consistent with Open. Unlisted
	// aliases and paths that cannot be verified within bounded reads are denied.
	// Symlinks under a source with nested mounts require fs.ReadLinkFS.
	// Recognizable OS files under such a source are refused; use Root so
	// physical hard-link aliases are checked.
	FS fs.FS
	// Write permits creating and modifying files. It requires Root.
	Write bool
}

// NetworkGrant grants connections to one destination pattern.
type NetworkGrant struct {
	// Scheme is the protocol, for example "https" or "tcp".
	Scheme string
	// Host is an ASCII name, an IP address without brackets or a port, or
	// "*.example.com" for every subdomain (not example.com itself).
	// Internationalized names must use punycode.
	Host string
	// Ports lists the allowed ports; at least one is required.
	Ports []int
	// AllowPrivate permits loopback, private and other non-public
	// addresses. Without it they are refused even when the name matches,
	// because the address actually connected to is checked.
	AllowPrivate bool
}

// Limits are per-execution budgets. They count what the engine charges,
// not process memory. A zero field selects the default in its comment.
type Limits struct {
	// Steps bounds VM instructions and loop iterations. Zero: unlimited.
	Steps int64
	// Memory bounds the logical size of PHP values. Zero: unlimited.
	Memory int64
	// Output bounds bytes written to the output. Zero: 8 MiB, or
	// Config.OutputLimit when that is set.
	Output int64
	// Diagnostics bounds recorded notices, warnings and errors. Zero: 10000.
	Diagnostics int
	// Frames bounds the call depth. Zero: unlimited.
	Frames int
	// OpenFiles bounds files open at once. Zero: unlimited.
	OpenFiles int
	// Connections bounds network and database connections open at once.
	// Zero: unlimited.
	Connections int
	// CompileBytes bounds source compiled at run time by include and eval,
	// summed over the execution. Each source is also bounded by
	// Config.MaxSourceBytes. Zero: unlimited.
	CompileBytes int64
}

// Capability names a kind of external operation.
type Capability string

const (
	CapabilityFileRead    Capability = "file-read"
	CapabilityFileWrite   Capability = "file-write"
	CapabilityNetwork     Capability = "network"
	CapabilityDynamicCode Capability = "dynamic-code"
	CapabilityCallback    Capability = "callback"
)

// Resource names a metered resource.
type Resource string

const (
	ResourceSteps        Resource = "steps"
	ResourceMemory       Resource = "memory"
	ResourceOutput       Resource = "output"
	ResourceDiagnostics  Resource = "diagnostics"
	ResourceFrames       Resource = "frames"
	ResourceOpenFiles    Resource = "open-files"
	ResourceConnections  Resource = "connections"
	ResourceCompileBytes Resource = "compile-bytes"
	ResourceExecutions   Resource = "executions"
)

// Common rule codes carried by PolicyError.
const (
	RuleNotGranted     = "not-granted"     // the capability is not granted
	RuleOutsideRoot    = "outside-root"    // the path is outside every mounted tree
	RuleNotAbsolute    = "not-absolute"    // the path is not absolute
	RulePrivateAddress = "private-address" // the connection reached a non-public address
)

const defaultDiagnostics = 10000

// compiledPolicy is the validated, immutable form of Config.Policy and
// Config.Limits that every run of an Engine shares.
type compiledPolicy struct {
	policy    *policy.Policy
	limits    policy.Limits
	admission *policy.Admission // nil when MaxActive is zero
}

// compilePolicy validates cfg.Policy, cfg.Limits, cfg.MaxActive and
// cfg.MaxQueued, folds the deprecated Files and OutputLimit fields into
// them, and returns the immutable form. Errors are *ConfigError with the
// public field path, for example "Policy.Files[0].Path". callbacks are the
// registered host function names, which registration alone authorizes.
func compilePolicy(cfg *Config, callbacks []string) (*compiledPolicy, error) {
	spec := policy.Spec{Callbacks: callbacks, DynamicCode: cfg.Policy.DynamicCode}
	switch cfg.Policy.Mode {
	case ModeCompat:
		spec.Mode = policy.Compat
	case ModeObserve:
		spec.Mode = policy.Observe
	case ModeEnforce:
		spec.Mode = policy.Enforce
	default:
		return nil, &ConfigError{Code: diagcodes.PolicyMode, Field: "Policy.Mode", Reason: fmt.Sprintf("unknown mode %d", cfg.Policy.Mode)}
	}
	for _, g := range cfg.Policy.Files {
		if cfg.Files != nil && g.Path == "/" {
			return nil, &ConfigError{Code: diagcodes.FilesRootConflict, Field: "Files", Reason: `Files and Policy.Files both mount "/"`}
		}
		spec.Files = append(spec.Files, policy.FileGrant{Path: g.Path, Dir: g.Root, FS: g.FS, Write: g.Write})
	}
	if cfg.Files != nil {
		spec.Files = append(spec.Files, policy.FileGrant{Path: "/", FS: cfg.Files})
	}
	for _, g := range cfg.Policy.Network {
		spec.Network = append(spec.Network, policy.NetworkGrant{Scheme: g.Scheme, Host: g.Host,
			Ports: append([]int(nil), g.Ports...), AllowPrivate: g.AllowPrivate})
	}
	pol, err := policy.New(spec)
	var se *policy.SpecError
	if errors.As(err, &se) {
		field := se.Field
		if strings.HasPrefix(field, "Files[") && strings.HasSuffix(field, ".Dir") {
			field = strings.TrimSuffix(field, ".Dir") + ".Root"
		}
		return nil, &ConfigError{Code: specCode(se.Code), Field: "Policy." + field, Reason: se.Reason}
	}
	if err != nil {
		return nil, err
	}

	l := cfg.Limits
	for _, f := range []struct {
		name string
		v    int64
	}{
		{"Steps", l.Steps}, {"Memory", l.Memory}, {"Output", l.Output}, {"Diagnostics", int64(l.Diagnostics)},
		{"Frames", int64(l.Frames)}, {"OpenFiles", int64(l.OpenFiles)}, {"Connections", int64(l.Connections)},
		{"CompileBytes", l.CompileBytes},
	} {
		if f.v < 0 {
			return nil, &ConfigError{Code: diagcodes.ConfigNegative, Field: "Limits." + f.name, Reason: "must not be negative"}
		}
	}
	output := l.Output
	switch {
	case cfg.OutputLimit != 0 && output != 0 && int64(cfg.OutputLimit) != output:
		return nil, &ConfigError{Code: diagcodes.OutputLimitConflict, Field: "OutputLimit", Reason: fmt.Sprintf("is %d but Limits.Output is %d", cfg.OutputLimit, output)}
	case output == 0 && cfg.OutputLimit != 0:
		output = int64(cfg.OutputLimit)
	case output == 0:
		output = defaultOutputLimit
	}
	cfg.OutputLimit = int(min(output, int64(maxInt)))
	diagnostics := int64(l.Diagnostics)
	if diagnostics == 0 {
		diagnostics = defaultDiagnostics
	}
	limits := policy.Limits{policy.Output: output, policy.Diagnostics: diagnostics}
	for r, n := range map[policy.Resource]int64{
		policy.Steps: l.Steps, policy.Memory: l.Memory, policy.Frames: int64(l.Frames),
		policy.OpenFiles: int64(l.OpenFiles), policy.Connections: int64(l.Connections), policy.CompileBytes: l.CompileBytes,
	} {
		if n > 0 {
			limits[r] = n
		}
	}

	c := &compiledPolicy{policy: pol, limits: limits}
	switch {
	case cfg.MaxActive < 0:
		return nil, &ConfigError{Code: diagcodes.ConfigNegative, Field: "MaxActive", Reason: "must not be negative"}
	case cfg.MaxQueued < 0:
		return nil, &ConfigError{Code: diagcodes.ConfigNegative, Field: "MaxQueued", Reason: "must not be negative"}
	case cfg.MaxActive == 0 && cfg.MaxQueued != 0:
		return nil, &ConfigError{Code: diagcodes.QueueWithoutActive, Field: "MaxQueued", Reason: "needs MaxActive to bound running work"}
	case cfg.MaxActive > 0:
		c.admission, err = policy.NewAdmission(cfg.MaxActive, cfg.MaxQueued)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

const maxInt = int(^uint(0) >> 1)

// admit takes an execution slot, or returns a *ResourceError when the
// Engine is full, ErrEngineClosed after Close, or ctx.Err() when ctx ends
// while waiting. The release function may be called more than once.
func (e *Engine) admit(ctx context.Context) (func(), error) {
	if e.policy.admission == nil {
		return func() {}, nil
	}
	release, err := e.policy.admission.Acquire(ctx)
	if err != nil {
		return nil, publicError(err)
	}
	return release, nil
}

// openFile opens a PHP path through the policy for reading.
func (e *Engine) openFile(path string) (fs.File, error) {
	f, err := e.policy.policy.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, publicError(err)
	}
	return f, nil
}

// publicError converts internal policy errors to the public types and
// passes other errors through unchanged.
func publicError(err error) error {
	var pe *policy.PolicyError
	var re *policy.ResourceError
	switch {
	case errors.Is(err, policy.ErrAdmissionClosed):
		return ErrEngineClosed
	case errors.As(err, &pe):
		return &PolicyError{Capability: Capability(pe.Kind.String()), Op: pe.Op, Target: pe.Target, Rule: pe.Rule}
	case errors.As(err, &re):
		return &ResourceError{Resource: Resource(re.Resource.String()), Limit: re.Limit, Used: re.Used, Requested: re.Requested}
	}
	return err
}

// specCode keeps the policy layer's code; Config validation rules out the
// policy errors that have none.
func specCode(c diag.Code) diag.Code {
	if c == "" {
		return diagcodes.ConfigOther
	}
	return c
}
