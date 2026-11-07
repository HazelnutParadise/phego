# Phego diagnostic codes

Generated from `internal/diagcodes/codes.go` by `go generate ./internal/diagcodes`; do not edit by hand.

These codes cover Phego's own diagnostics only. PHP warnings, fatal errors and exception messages keep PHP 8.5.12's exact text and carry no PHG code.

A published code is never renumbered or reused. `PHG1xxx` covers Engine configuration and run options, `PHG2xxx` bytecode loading and verification.

## PHG1000: configuration error without a specific code

- note: the error was built by host code or predates its code
- help: see the message for the field and reason

## PHG1001: a size, count or limit is negative

- note: limits and sizes count bytes or items and cannot be below zero
- help: use 0 or a positive value; the field's documentation says what 0 means

## PHG1002: IntSize is not 32 or 64

- note: PHP integers are 32 or 64 bits wide; 0 selects 64
- help: set IntSize to 0, 32 or 64

## PHG1003: the PHP release is not recorded

- note: Release must name a PHP release recorded in the PHP release inventory
- help: use phego.SupportedRelease, or leave Release empty for the default

## PHG1004: a host function name is not a PHP function name

- note: host functions are called from PHP, so the name must be a PHP label that is not a reserved word such as echo or isset
- help: use letters, digits and underscores, not starting with a digit, and avoid PHP reserved words

## PHG1005: a host function has no Func

- note: PHP calls reach the Go function in Func
- help: set Func, or remove the entry

## PHG1006: a host function argument range is invalid

- note: MinArgs must be at least 0 and MaxArgs at least MinArgs; -1 means no maximum
- help: fix MinArgs and MaxArgs

## PHG1007: a host function capability is empty or repeated

- note: each capability names one policy permission the function needs
- help: remove the empty or repeated entry

## PHG1008: two host functions have the same name

- note: PHP function names are case-insensitive, so the names collide
- help: rename or remove one of them

## PHG1009: Policy.Mode is unknown

- note: the mode decides how policy refusals are handled
- help: use ModeCompat, ModeObserve or ModeEnforce

## PHG1010: Files and Policy.Files both mount "/"

- note: Files is mounted at "/", so a Policy.Files grant there would hide it
- help: remove Files, or move the Policy.Files grant away from "/"

## PHG1011: OutputLimit and Limits.Output disagree

- note: both set the output budget; when both are non-zero they must match
- help: set only Limits.Output

## PHG1012: MaxQueued is set without MaxActive

- note: the queue holds work waiting for one of MaxActive slots
- help: set MaxActive to a positive value, or MaxQueued to 0

## PHG1013: WithOutput was given a nil writer

- note: output must go somewhere; without WithOutput it is captured in Result.Output
- help: pass a writer, or drop WithOutput

## PHG1014: WithGlobal was given an invalid variable name

- note: the name is used as a PHP variable without the leading $; $this is reserved
- help: use a PHP label such as "input"

## PHG1015: the program was not compiled by this engine

- note: a Program carries the engine's profile and policy
- help: compile the program with the engine that runs it

## PHG1016: the program was compiled for another PHP profile

- note: release, integer width and options must match between compile and run
- help: recompile the program with this engine

## PHG1017: a RunOption is nil

- note: every option is called before the run starts
- help: remove the nil option

## PHG1018: the Engine was not created by NewEngine

- note: a zero Engine has no profile, policy or limits
- help: create engines with NewEngine

## PHG1019: a mount path is not a clean absolute PHP path

- note: mounts are matched against absolute PHP paths
- help: write the path starting with "/" and without "." or ".." segments

## PHG1020: a file mount needs exactly one of Root and FS

- note: Root (called Dir in the policy layer's message) mounts a host directory, FS an fs.FS
- help: set one of them

## PHG1021: a mount Root is not an absolute host path

- note: relative roots would depend on the process working directory
- help: use an absolute directory path

## PHG1022: an fs.FS mount cannot be writable

- note: fs.FS has no write operations
- help: use Root for a writable mount, or clear Write

## PHG1023: a PHP path is mounted twice

- note: each PHP path maps to one host location
- help: remove or move one of the mounts

## PHG1024: a network grant scheme is not a bare scheme

- note: schemes are names such as "https", without "://"
- help: write the scheme name only

## PHG1025: a network grant host is invalid

- note: hosts are names, addresses, or *.domain patterns; IPv6 zones must be visible ASCII
- help: fix the host

## PHG1026: a network grant has no valid ports

- note: ports are 1 to 65535 and at least one is required
- help: list the ports the grant allows

## PHG2001: the bytecode is malformed or fails verification

- note: all bytecode is untrusted and verified before it can run
- help: regenerate the artifact with this Phego build

## PHG2002: the bytecode exceeds a verification limit

- note: limits bound the time and memory spent verifying untrusted input
- help: raise the bytecode verification limits, or split the program

## PHG2003: the bytecode version is not supported

- note: each Phego build reads one bytecode format version
- help: regenerate the artifact with this Phego build

## PHG2004: the bytecode identity is invalid or names another profile

- note: release, integer width, options and host identity are part of the artifact identity
- help: rebuild the artifact for this engine's profile

## PHG2005: the bytecode digest does not match

- note: the artifact changed after it was produced
- help: regenerate the artifact; do not edit it

## PHG2006: the bytecode artifact is not trusted

- note: an artifact is accepted only when its SHA-256 matches a digest obtained from a trusted source
- help: supply the artifact's SHA-256 from a trusted source when loading it

## PHG2007: the bytecode capabilities are unsupported or incomplete

- note: the artifact needs an operation this build does not provide, or its declared capabilities do not cover its opcodes
- help: rebuild the artifact for this Phego build
