// Package diagcodes is the single registry of Phego's diagnostic codes.
// docs/diagnostics/codes.md is generated from it (go generate) and a test
// keeps the two identical. Codes are never renumbered or reused; a retired
// code stays listed with Retired set.
//
// Ranges: PHG1xxx Engine configuration and run options, PHG2xxx bytecode
// loading and verification.
package diagcodes

//go:generate go run ./gen

import "github.com/HazelnutParadise/phego/diag"

// Entry documents one code. Note explains the cause and Help the fix; both
// are shown with every diagnostic carrying the code.
type Entry struct {
	Title   string
	Note    string
	Help    string
	Retired bool
}

const (
	ConfigOther         diag.Code = "PHG1000"
	ConfigNegative      diag.Code = "PHG1001"
	ConfigIntSize       diag.Code = "PHG1002"
	ConfigRelease       diag.Code = "PHG1003"
	HostFuncName        diag.Code = "PHG1004"
	HostFuncNil         diag.Code = "PHG1005"
	HostFuncArgs        diag.Code = "PHG1006"
	HostFuncCapability  diag.Code = "PHG1007"
	HostFuncDuplicate   diag.Code = "PHG1008"
	PolicyMode          diag.Code = "PHG1009"
	FilesRootConflict   diag.Code = "PHG1010"
	OutputLimitConflict diag.Code = "PHG1011"
	QueueWithoutActive  diag.Code = "PHG1012"
	RunOutputNil        diag.Code = "PHG1013"
	RunGlobalName       diag.Code = "PHG1014"
	ProgramForeign      diag.Code = "PHG1015"
	ProgramProfile      diag.Code = "PHG1016"
	RunOptionNil        diag.Code = "PHG1017"
	EngineZero          diag.Code = "PHG1018"
	MountPath           diag.Code = "PHG1019"
	MountSource         diag.Code = "PHG1020"
	MountRoot           diag.Code = "PHG1021"
	MountReadOnly       diag.Code = "PHG1022"
	MountTwice          diag.Code = "PHG1023"
	NetworkScheme       diag.Code = "PHG1024"
	NetworkHost         diag.Code = "PHG1025"
	NetworkPorts        diag.Code = "PHG1026"
	BytecodeInvalid     diag.Code = "PHG2001"
	BytecodeLimit       diag.Code = "PHG2002"
	BytecodeVersion     diag.Code = "PHG2003"
	BytecodeProfile     diag.Code = "PHG2004"
	BytecodeIntegrity   diag.Code = "PHG2005"
	BytecodeTrust       diag.Code = "PHG2006"
	BytecodeCapability  diag.Code = "PHG2007"
)

// Registry holds every code. Constant keys make a duplicate a compile error.
var Registry = map[diag.Code]Entry{
	ConfigOther: {Title: "configuration error without a specific code",
		Note: "the error was built by host code or predates its code",
		Help: "see the message for the field and reason"},
	ConfigNegative: {Title: "a size, count or limit is negative",
		Note: "limits and sizes count bytes or items and cannot be below zero",
		Help: "use 0 or a positive value; the field's documentation says what 0 means"},
	ConfigIntSize: {Title: "IntSize is not 32 or 64",
		Note: "PHP integers are 32 or 64 bits wide; 0 selects 64",
		Help: "set IntSize to 0, 32 or 64"},
	ConfigRelease: {Title: "the PHP release is not recorded",
		Note: "Release must name a PHP release recorded in the PHP release inventory",
		Help: "use phego.SupportedRelease, or leave Release empty for the default"},
	HostFuncName: {Title: "a host function name is not a PHP function name",
		Note: "host functions are called from PHP, so the name must be a PHP label that is not a reserved word such as echo or isset",
		Help: "use letters, digits and underscores, not starting with a digit, and avoid PHP reserved words"},
	HostFuncNil: {Title: "a host function has no Func",
		Note: "PHP calls reach the Go function in Func",
		Help: "set Func, or remove the entry"},
	HostFuncArgs: {Title: "a host function argument range is invalid",
		Note: "MinArgs must be at least 0 and MaxArgs at least MinArgs; -1 means no maximum",
		Help: "fix MinArgs and MaxArgs"},
	HostFuncCapability: {Title: "a host function capability is empty or repeated",
		Note: "each capability names one policy permission the function needs",
		Help: "remove the empty or repeated entry"},
	HostFuncDuplicate: {Title: "two host functions have the same name",
		Note: "PHP function names are case-insensitive, so the names collide",
		Help: "rename or remove one of them"},
	PolicyMode: {Title: "Policy.Mode is unknown",
		Note: "the mode decides how policy refusals are handled",
		Help: "use ModeCompat, ModeObserve or ModeEnforce"},
	FilesRootConflict: {Title: `Files and Policy.Files both mount "/"`,
		Note: "Files is mounted at \"/\", so a Policy.Files grant there would hide it",
		Help: `remove Files, or move the Policy.Files grant away from "/"`},
	OutputLimitConflict: {Title: "OutputLimit and Limits.Output disagree",
		Note: "both set the output budget; when both are non-zero they must match",
		Help: "set only Limits.Output"},
	QueueWithoutActive: {Title: "MaxQueued is set without MaxActive",
		Note: "the queue holds work waiting for one of MaxActive slots",
		Help: "set MaxActive to a positive value, or MaxQueued to 0"},
	RunOutputNil: {Title: "WithOutput was given a nil writer",
		Note: "output must go somewhere; without WithOutput it is captured in Result.Output",
		Help: "pass a writer, or drop WithOutput"},
	RunGlobalName: {Title: "WithGlobal was given an invalid variable name",
		Note: "the name is used as a PHP variable without the leading $; $this is reserved",
		Help: "use a PHP label such as \"input\""},
	ProgramForeign: {Title: "the program was not compiled by this engine",
		Note: "a Program carries the engine's profile and policy",
		Help: "compile the program with the engine that runs it"},
	ProgramProfile: {Title: "the program was compiled for another PHP profile",
		Note: "release, integer width and options must match between compile and run",
		Help: "recompile the program with this engine"},
	RunOptionNil: {Title: "a RunOption is nil",
		Note: "every option is called before the run starts",
		Help: "remove the nil option"},
	EngineZero: {Title: "the Engine was not created by NewEngine",
		Note: "a zero Engine has no profile, policy or limits",
		Help: "create engines with NewEngine"},
	MountPath: {Title: "a mount path is not a clean absolute PHP path",
		Note: "mounts are matched against absolute PHP paths",
		Help: `write the path starting with "/" and without "." or ".." segments`},
	MountSource: {Title: "a file mount needs exactly one of Root and FS",
		Note: "Root (called Dir in the policy layer's message) mounts a host directory, FS an fs.FS",
		Help: "set one of them"},
	MountRoot: {Title: "a mount Root is not an absolute host path",
		Note: "relative roots would depend on the process working directory",
		Help: "use an absolute directory path"},
	MountReadOnly: {Title: "an fs.FS mount cannot be writable",
		Note: "fs.FS has no write operations",
		Help: "use Root for a writable mount, or clear Write"},
	MountTwice: {Title: "a PHP path is mounted twice",
		Note: "each PHP path maps to one host location",
		Help: "remove or move one of the mounts"},
	NetworkScheme: {Title: "a network grant scheme is not a bare scheme",
		Note: `schemes are names such as "https", without "://"`,
		Help: "write the scheme name only"},
	NetworkHost: {Title: "a network grant host is invalid",
		Note: "hosts are names, addresses, or *.domain patterns; IPv6 zones must be visible ASCII",
		Help: "fix the host"},
	NetworkPorts: {Title: "a network grant has no valid ports",
		Note: "ports are 1 to 65535 and at least one is required",
		Help: "list the ports the grant allows"},
	BytecodeInvalid: {Title: "the bytecode is malformed or fails verification",
		Note: "all bytecode is untrusted and verified before it can run",
		Help: "regenerate the artifact with this Phego build"},
	BytecodeLimit: {Title: "the bytecode exceeds a verification limit",
		Note: "limits bound the time and memory spent verifying untrusted input",
		Help: "raise the bytecode verification limits, or split the program"},
	BytecodeVersion: {Title: "the bytecode version is not supported",
		Note: "each Phego build reads one bytecode format version",
		Help: "regenerate the artifact with this Phego build"},
	BytecodeProfile: {Title: "the bytecode identity is invalid or names another profile",
		Note: "release, integer width, options and host identity are part of the artifact identity",
		Help: "rebuild the artifact for this engine's profile"},
	BytecodeIntegrity: {Title: "the bytecode digest does not match",
		Note: "the artifact changed after it was produced",
		Help: "regenerate the artifact; do not edit it"},
	BytecodeTrust: {Title: "the bytecode artifact is not trusted",
		Note: "an artifact is accepted only when its SHA-256 matches a digest obtained from a trusted source",
		Help: "supply the artifact's SHA-256 from a trusted source when loading it"},
	BytecodeCapability: {Title: "the bytecode capabilities are unsupported or incomplete",
		Note: "the artifact needs an operation this build does not provide, or its declared capabilities do not cover its opcodes",
		Help: "rebuild the artifact for this Phego build"},
}
