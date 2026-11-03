// Package bytecode defines Phego's versioned register bytecode and bounded
// verifier. It does not execute PHP. Only a successfully built or loaded Program
// may be handed to a backend. See docs/decisions/W-16-bytecode.md.
//
// REQ-025, REQ-061, REQ-064; T-049, T-050, T-121, T-122, T-127, T-128.
package bytecode

import (
	"context"
	"errors"
	"fmt"

	"github.com/HazelnutParadise/phego/internal/profile"
)

const FormatVersion uint16 = 1

// Identity is compared in full against a trusted host expectation. Digests are
// of canonical manifests, not filenames or mtimes. Empty manifests still have
// a nonzero digest. GuestPlatform identifies OS/path/numeric semantic rules.
// Target identifies the build target, including any CPU-dependent assumptions;
// use "portable-go" only when the compiler makes none.
type Identity struct {
	Profile                                              profile.Profile
	GuestPlatform, Target                                string
	Source, Compiler, Modules, RuntimeABI, Policy, Scope [32]byte
}

// Limits are independent of execution quotas. Every limit must be positive.
// Memory bounds conservative accounted allocations, not process RSS. Work
// bounds decoding and data-flow operations; Context also permits cancellation.
type Limits struct {
	MaxBytes, MaxMemory, MaxWork                                                                            uint64
	MaxFunctions, MaxInstructions, MaxConstants, MaxRegisters, MaxLocals, MaxStack, MaxHandlers, MaxSources uint32
}

func DefaultLimits() Limits {
	return Limits{64 << 20, 128 << 20, 32 << 20, 4096, 1 << 20, 1 << 18, 4096, 65536, 4096, 65536, 65536}
}

type RegisterKind uint8

const (
	Value RegisterKind = iota + 1
	Location
)

type ConstantKind uint8

const (
	Null ConstantKind = iota + 1
	Bool
	Int
	Float
	String
)

// Constant contains only immutable scalar data. Bits is bool (0/1), signed
// two's-complement integer or IEEE-754 float bits. Bytes preserves binary PHP
// strings. Unused fields must be zero; there are no live heap values here.
type Constant struct {
	Kind  ConstantKind
	Bits  uint64
	Bytes string
}

type CapabilitySet uint16

const (
	HostCallbacks CapabilitySet = 1 << iota
	DynamicCode
	FileRead
	Shell
	AllCapabilities = HostCallbacks | DynamicCode | FileRead | Shell
)

type Opcode uint16

const (
	OpCheckpoint Opcode = iota + 1
	OpConst
	OpMove
	OpLocal
	OpLoad
	OpStore
	OpBinary
	OpJump
	OpBranch
	OpPush
	OpCall
	OpCallValue
	OpHostCall
	OpReturn
	OpReturnRef
	OpThrow
	OpOutput
	OpNewArray
	OpArrayGet
	OpInclude
	OpEval
	OpShell
	OpDrop
	OpLeave
	OpEndFinally
)

type BinaryOp uint16

const (
	Add BinaryOp = iota + 1
	Subtract
	Multiply
	Divide
	Concat
	Equal
	Identical
	Less
	BitAnd
	BitOr
	BitXor
)

// Instruction is a pointer-free 16-byte record in memory and on disk. Jump
// targets are local instruction indices. Aux and unused operands are checked
// per opcode, never silently ignored.
type Instruction struct {
	Op      Opcode
	Aux     uint16
	A, B, C uint32
}

type HandlerKind uint32

const (
	Catch HandlerKind = iota + 1
	Finally
)

// Handler protects [Start,End). Target is outside that range. StackDepth is
// zero in version 1: failed argument construction is discarded during unwind.
// Catch writes a PHP exception to Register; Finally uses pending completion
// state owned by the execution and has Register == 0.
type Handler struct {
	Start, End, Target, BodyEnd, StackDepth, Register uint32
	Kind                                              HandlerKind
}

// Source identifies bytes for diagnostics; loading never opens Name as a path.
type Source struct {
	Name         string
	Digest       [32]byte
	Bytes, Lines uint32
}

// Position is either all zero (synthetic) or Source is a 1-based source index
// and Line is positive. Byte ranges are half open and checked against Source.
type Position struct{ Source, Start, End, Line uint32 }

// Function describes one frame. Parameter registers are the first Params
// registers and are Value class. Locals are request-owned PHP locations.
// Code has one Position per instruction. ReturnKind governs direct call results.
type Function struct {
	Name                     string
	Params, Locals, MaxStack uint32
	ReturnKind               RegisterKind
	Registers                []RegisterKind
	Code                     []Instruction
	Positions                []Position
	Handlers                 []Handler
}

// Image is builder input, never executable. Callers keep all input unchanged
// during Build. Build owns copies on success; later mutations do not affect it.
type Image struct {
	Identity  Identity
	Required  CapabilitySet
	Entry     uint32
	Constants []Constant
	Sources   []Source
	Functions []Function
}

type Options struct {
	Limits    Limits
	Supported CapabilitySet
}

// LoadOptions binds an artifact to both its expected semantic identity and a
// SHA-256 digest obtained from a trusted source. A digest supplied by the same
// untrusted artifact is not provenance. Native artifacts are not supported.
type LoadOptions struct {
	Options
	Expected      Identity
	TrustedDigest [32]byte
}

// Program owns only immutable compiler data. There is no request environment,
// session, mutable PHP object, static value, inline cache or failure state.
type Program struct {
	image  Image
	limits Limits // immutable encoding budget captured at admission
}

var (
	ErrInvalid    = errors.New("invalid bytecode")
	ErrLimit      = errors.New("bytecode verification limit exceeded")
	ErrVersion    = errors.New("unsupported bytecode version")
	ErrProfile    = errors.New("bytecode identity mismatch")
	ErrIntegrity  = errors.New("bytecode integrity mismatch")
	ErrTrust      = errors.New("untrusted bytecode artifact")
	ErrCapability = errors.New("unsupported bytecode capability")
)

// Error identifies a bounded diagnostic. Offsets are byte offsets for parsing;
// Function/PC are -1 when no instruction has been identified.
type Error struct {
	Kind         error
	Offset       int
	Function, PC int
	Detail       string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v at byte %d, function %d, instruction %d: %s", e.Kind, e.Offset, e.Function, e.PC, e.Detail)
}
func (e *Error) Unwrap() error { return e.Kind }

// Build validates the full image before publishing an immutable program.
func Build(ctx context.Context, image Image, opts Options) (*Program, error) {
	return build(ctx, image, opts)
}

// Load treats all bytes as untrusted and never publishes a partial program.
// The caller must not concurrently mutate data until Load returns.
func Load(ctx context.Context, data []byte, opts LoadOptions) (*Program, error) {
	return load(ctx, data, opts)
}

// Encode returns an independent canonical encoding of a validated program.
func (p *Program) Encode(ctx context.Context) ([]byte, error) { return encode(ctx, p) }

func (p *Program) Identity() Identity      { return p.image.Identity }
func (p *Program) Entry() uint32           { return p.image.Entry }
func (p *Program) Required() CapabilitySet { return p.image.Required }
func (p *Program) FunctionCount() int      { return len(p.image.Functions) }
func (p *Program) SourceCount() int        { return len(p.image.Sources) }
func (p *Program) Source(index uint32) (Source, bool) {
	if uint64(index) >= uint64(len(p.image.Sources)) {
		return Source{}, false
	}
	return p.image.Sources[index], true
}
func (p *Program) Constant(index uint32) (Constant, bool) {
	if uint64(index) >= uint64(len(p.image.Constants)) {
		return Constant{}, false
	}
	return p.image.Constants[index], true
}

// FunctionView has no mutating operations or exposed slices.
type FunctionView struct{ function *Function }

func (p *Program) Function(index uint32) (FunctionView, bool) {
	if uint64(index) >= uint64(len(p.image.Functions)) {
		return FunctionView{}, false
	}
	return FunctionView{&p.image.Functions[index]}, true
}
func (f FunctionView) Len() int {
	if f.function == nil {
		return 0
	}
	return len(f.function.Code)
}
func (f FunctionView) Name() string {
	if f.function == nil {
		return ""
	}
	return f.function.Name
}
func (f FunctionView) Params() uint32 {
	if f.function == nil {
		return 0
	}
	return f.function.Params
}
func (f FunctionView) ReturnKind() RegisterKind {
	if f.function == nil {
		return 0
	}
	return f.function.ReturnKind
}
func (f FunctionView) RegisterKind(index uint32) (RegisterKind, bool) {
	if f.function == nil || uint64(index) >= uint64(len(f.function.Registers)) {
		return 0, false
	}
	return f.function.Registers[index], true
}
func (f FunctionView) HandlerCount() int {
	if f.function == nil {
		return 0
	}
	return len(f.function.Handlers)
}
func (f FunctionView) Handler(index uint32) (Handler, bool) {
	if f.function == nil || uint64(index) >= uint64(len(f.function.Handlers)) {
		return Handler{}, false
	}
	return f.function.Handlers[index], true
}
func (f FunctionView) Instruction(pc uint32) (Instruction, bool) {
	if f.function == nil || uint64(pc) >= uint64(len(f.function.Code)) {
		return Instruction{}, false
	}
	return f.function.Code[pc], true
}
func (f FunctionView) Frame() (registers, locals, stack uint32) {
	if f.function == nil {
		return 0, 0, 0
	}
	return uint32(len(f.function.Registers)), f.function.Locals, f.function.MaxStack
}
func (f FunctionView) Position(pc uint32) (Position, bool) {
	if f.function == nil || uint64(pc) >= uint64(len(f.function.Positions)) {
		return Position{}, false
	}
	return f.function.Positions[pc], true
}

// Safety is intrinsic to a known opcode and cannot be overridden by input.
// Every semantic step polls cancellation and charges Steps before effects.
// Allocations charge their actual sizes before allocation; calls charge Frames;
// all possible user callbacks use the same policy and bounded execution scope.
type Safety struct {
	Steps                                           uint32
	CheckCancel, MayAllocate, MayThrow, MayCallUser bool
	Capabilities                                    CapabilitySet
}

func (op Opcode) Safety() (Safety, bool) { return opcodeSafety(op) }
