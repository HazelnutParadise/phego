package phego

import (
	"io/fs"

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
	// Root is a host directory, opened through os.Root so ".." and symbolic
	// links cannot leave it.
	Root string
	// FS is a read-only file system, for example an embed.FS.
	FS fs.FS
	// Write permits creating and modifying files. It requires Root.
	Write bool
}

// NetworkGrant grants connections to one destination pattern.
type NetworkGrant struct {
	// Scheme is the protocol, for example "https" or "tcp".
	Scheme string
	// Host is a host name, an IP address, or "*.example.com" for every
	// subdomain of example.com but not example.com itself.
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

// Rule codes carried by PolicyError.
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
