// Package policy decides which external operations one PHP execution may
// perform and meters the resources it uses.
//
// A Policy is an immutable capability set built once from a Spec and shared
// by every execution of a site. Each operation that leaves the engine (a file
// read or write, a network connection, a database, a process, dynamic code
// or a host callback) asks the same Policy, whatever backend runs the PHP
// code. Policy modes change how heuristic rule events are handled; they never
// widen the granted capabilities or lift resource limits.
package policy

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Mode is the site's policy mode.
type Mode uint8

const (
	// Compat keeps PHP's normal behaviour inside the granted capabilities
	// and records known differences.
	Compat Mode = iota
	// Observe also records suspicious behaviour without blocking it.
	Observe
	// Enforce blocks operations that match an explicit rule.
	Enforce
)

func (m Mode) String() string {
	switch m {
	case Compat:
		return "compat"
	case Observe:
		return "observe"
	case Enforce:
		return "enforce"
	}
	return fmt.Sprintf("Mode(%d)", uint8(m))
}

// Kind is a capability kind.
type Kind uint8

const (
	FileRead Kind = iota + 1
	FileWrite
	Network
	Database
	// Process starts an executable with an argument vector.
	Process
	// Shell runs a command line through a shell. It is a separate
	// capability: granting Process does not grant Shell.
	Shell
	// DynamicCode compiles code at run time: include, require and eval.
	DynamicCode
	Callback
	Service
)

var kindNames = map[Kind]string{
	FileRead: "file-read", FileWrite: "file-write", Network: "network", Database: "database",
	Process: "process", Shell: "shell", DynamicCode: "dynamic-code", Callback: "callback", Service: "service",
}

func (k Kind) String() string {
	if s, ok := kindNames[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Spec describes the capabilities granted to a site. The zero value grants
// nothing.
type Spec struct {
	Mode Mode
	// Files mounts directory trees at PHP-visible paths. PHP may read them,
	// and write when Write is set.
	Files []FileGrant
	// Network lists the destinations PHP may connect to.
	Network []NetworkGrant
	// Databases names the configured connections PHP may open.
	Databases []string
	// Processes lists absolute paths of executables PHP may start with an
	// argument vector.
	Processes []string
	// Shell permits command lines run through a shell.
	Shell bool
	// DynamicCode permits include, require and eval of code PHP did not
	// receive at compile time. Includes still need file read access.
	DynamicCode bool
	// Callbacks names the host callbacks PHP may call. Names are compared
	// in ASCII lower case, as PHP compares function names.
	Callbacks []string
	// Services names shared services PHP may use.
	Services []string
}

// FileGrant mounts a tree at Path, the absolute path PHP sees. Exactly one
// of Dir and FS is set; Write requires Dir.
type FileGrant struct {
	Path string
	// Dir is an absolute host directory, opened through os.Root.
	Dir string
	// FS is a read-only file system.
	FS    fs.FS
	Write bool
}

// NetworkGrant grants connections to one destination pattern.
type NetworkGrant struct {
	// Scheme is the protocol, for example "https" or "tcp".
	Scheme string
	// Host is a host name, an IP address, or "*.example.com" for every
	// subdomain of example.com (not example.com itself).
	Host string
	// Ports are the allowed ports; empty allows none.
	Ports []int
	// AllowPrivate permits loopback, private, link-local and other
	// non-public addresses. Without it such addresses are refused even when
	// the host name matches, so DNS cannot redirect a granted name inward.
	AllowPrivate bool
}

// Policy is an immutable capability set.
type Policy struct {
	mode        Mode
	files       []FileGrant
	network     []NetworkGrant
	databases   map[string]bool
	processes   map[string]bool
	shell       bool
	dynamicCode bool
	callbacks   map[string]bool
	services    map[string]bool
}

// New validates spec and returns its policy. Invalid grants are rejected
// rather than ignored, so a typo cannot silently widen or narrow access.
func New(spec Spec) (*Policy, error) {
	if spec.Mode > Enforce {
		return nil, &SpecError{Field: "Mode", Reason: fmt.Sprintf("unknown mode %d", spec.Mode)}
	}
	p := &Policy{
		mode: spec.Mode, shell: spec.Shell, dynamicCode: spec.DynamicCode,
		databases: map[string]bool{}, processes: map[string]bool{}, callbacks: map[string]bool{}, services: map[string]bool{},
	}
	mounted := map[string]bool{}
	for i, g := range spec.Files {
		field := fmt.Sprintf("Files[%d]", i)
		switch {
		case !path.IsAbs(g.Path) || path.Clean(g.Path) != g.Path:
			return nil, &SpecError{Field: field + ".Path", Reason: fmt.Sprintf("%q is not a clean absolute path", g.Path)}
		case (g.Dir == "") == (g.FS == nil):
			return nil, &SpecError{Field: field, Reason: "exactly one of Dir and FS must be set"}
		case g.Dir != "" && !filepath.IsAbs(g.Dir):
			return nil, &SpecError{Field: field + ".Dir", Reason: fmt.Sprintf("%q is not an absolute path", g.Dir)}
		case g.Write && g.FS != nil:
			return nil, &SpecError{Field: field + ".Write", Reason: "an fs.FS mount is read-only"}
		case mounted[g.Path]:
			return nil, &SpecError{Field: field + ".Path", Reason: fmt.Sprintf("%q is mounted twice", g.Path)}
		}
		mounted[g.Path] = true
		if g.Dir != "" {
			g.Dir = filepath.Clean(g.Dir)
		}
		p.files = append(p.files, g)
	}
	for i, g := range spec.Network {
		field := fmt.Sprintf("Network[%d]", i)
		if g.Scheme == "" || strings.ContainsAny(g.Scheme, ":/") {
			return nil, &SpecError{Field: field, Reason: fmt.Sprintf("scheme %q is not a bare scheme", g.Scheme)}
		}
		if g.Host == "" || strings.ContainsAny(g.Host, "/:@") && !isIPv6Literal(g.Host) || strings.Count(g.Host, "*") > 1 ||
			strings.Contains(g.Host, "*") && !strings.HasPrefix(g.Host, "*.") {
			return nil, &SpecError{Field: field, Reason: fmt.Sprintf("host %q is not a host name, address or *.domain pattern", g.Host)}
		}
		if len(g.Ports) == 0 {
			return nil, &SpecError{Field: field, Reason: "no ports are granted"}
		}
		for _, port := range g.Ports {
			if port < 1 || port > 65535 {
				return nil, &SpecError{Field: field, Reason: fmt.Sprintf("port %d is out of range", port)}
			}
		}
		g.Scheme, g.Host = strings.ToLower(g.Scheme), strings.ToLower(strings.TrimSuffix(g.Host, "."))
		g.Ports = append([]int(nil), g.Ports...)
		sort.Ints(g.Ports)
		p.network = append(p.network, g)
	}
	for _, set := range []struct {
		field string
		names []string
		into  map[string]bool
		fold  bool
	}{
		{"Databases", spec.Databases, p.databases, false},
		{"Callbacks", spec.Callbacks, p.callbacks, true},
		{"Services", spec.Services, p.services, false},
	} {
		for i, name := range set.names {
			if name == "" {
				return nil, &SpecError{Field: fmt.Sprintf("%s[%d]", set.field, i), Reason: "name is empty"}
			}
			if set.fold {
				name = asciiLower(name)
			}
			set.into[name] = true
		}
	}
	for i, path := range spec.Processes {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, &SpecError{Field: fmt.Sprintf("Processes[%d]", i), Reason: fmt.Sprintf("%q is not a clean absolute path", path)}
		}
		p.processes[path] = true
	}
	return p, nil
}

// Mode returns the policy mode.
func (p *Policy) Mode() Mode { return p.mode }

// Allow checks an operation that is not a file or network access, which
// have their own checks. It returns a *PolicyError when the capability is
// not granted, in every mode.
func (p *Policy) Allow(kind Kind, op, target string) error {
	allowed := false
	switch kind {
	case Database:
		allowed = p.databases[target]
	case Process:
		allowed = p.processes[target]
	case Shell:
		allowed = p.shell
	case DynamicCode:
		allowed = p.dynamicCode
	case Callback:
		allowed = p.callbacks[asciiLower(target)]
	case Service:
		allowed = p.services[target]
	case FileRead, FileWrite, Network:
		return &PolicyError{Kind: kind, Op: op, Target: target, Rule: "use-dedicated-check"}
	default:
		return &PolicyError{Kind: kind, Op: op, Target: target, Rule: "unknown-capability"}
	}
	if !allowed {
		return &PolicyError{Kind: kind, Op: op, Target: target, Rule: "not-granted"}
	}
	return nil
}

// RuleMatched reports that a heuristic rule matched an operation, for
// example a tainted value reaching an SQL structure. In enforce mode the
// operation is refused; in compat and observe modes it proceeds and the
// caller records the event. Capability checks are not rules and are
// enforced in every mode.
func (p *Policy) RuleMatched(kind Kind, op, target, rule string) error {
	if p.mode == Enforce {
		return &PolicyError{Kind: kind, Op: op, Target: target, Rule: rule}
	}
	return nil
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func isIPv6Literal(h string) bool { return strings.Contains(h, ":") && !strings.ContainsAny(h, "/@") }
