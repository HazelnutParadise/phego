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

// FormatVersion is the major format version. FormatMinor is the minor
// version Encode writes. Load accepts minor 0 (format 1.0), minor 1 (format
// 1.1, the first W-17 stage), minor 2 (format 1.2, the second W-17 stage),
// minor 3 (format 1.3, the third) and minor 4 (format 1.4, the fourth); a
// file may not use operations newer than the minor it declares.
const (
	FormatVersion uint16 = 1
	FormatMinor   uint16 = 4
)

// The Aux of OpArrayGet. ArrayGetQuiet (format 1.1) is FETCH_DIM_IS: no
// warning for a missing key or a non-array container. ArrayGetList (format
// 1.4) is FETCH_LIST_R, the element read of a destructuring assignment: an
// array is read as with Aux 0, but a string is not a string container and
// a container that is neither array nor null warns "Cannot use T as array"
// instead of "Trying to access array offset"; the result is then null.
const (
	ArrayGetQuiet uint16 = 1
	ArrayGetList  uint16 = 2
)

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

// DefaultLimits admits, through Build and Load, an image that reaches
// MaxInstructions with straight-line code over a 64-register frame. Such an
// image needs about 107 million work units and 145 MiB of accounted memory to
// load. Wider frames, deep argument stacks and many handlers draw on the
// same MaxWork and MaxMemory budgets, so images combining several maxima
// can still return ErrLimit.
func DefaultLimits() Limits {
	return Limits{64 << 20, 192 << 20, 128 << 20, 4096, 1 << 20, 1 << 18, 4096, 65536, 4096, 65536, 65536}
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
	// Format 1.1 (W-17). Operands are in docs/decisions/W-16-bytecode.md.
	OpUnary       // A dest Value, B operand Value; Aux is a UnaryOp
	OpCast        // A dest Value, B operand Value; Aux is a CastKind
	OpIncDec      // A result Value, B target Location; Aux is an IncDecOp
	OpArraySet    // A container Location, B key, C value: $a[k] = v; Aux is ArrayWriteFlags
	OpArrayAppend // A container Location, B value: $a[] = v; Aux is ArrayWriteFlags (ValueLocation only)
	OpArrayUnset  // A container Location, B key: unset($a[k]); Aux is 0 or KeyLocation
	OpIsset       // A dest Value (bool), B Location: isset($x)
	OpArrayIsset  // A dest Value (bool), B container Value, C key Value: isset($a[k])
	OpUnset       // A Location: unset($x)
	OpExit        // A status Value when Aux is 1; Aux 0 is exit without a status
	OpArrayAdd    // A array Value (updated in place), B value Value, C key when Aux has AddKey; Aux is ArrayAddFlags
	// Format 1.2 (W-17 stage 2). Operands are in docs/decisions/W-16-bytecode.md.
	OpFetchDim     // A dest Location (the element), B container Location, C key; Aux is FetchFlags
	OpIterInit     // A iterator Value, B subject Value, C target taken when the subject is not iterable
	OpIterNext     // A iterator Value, B element dest (Value, or Location with IterLocation), C target taken when exhausted
	OpIterKey      // A key dest (Value, or Location with IterLocation), B iterator Value
	OpParamDefault // A parameter register, B target taken when the call passed that argument
	// Format 1.3 (W-17 stage 3). Operands are in docs/decisions/W-16-bytecode.md.
	OpBind    // A dest Location, B source Location (a Value with BindValue): $a =& $b; Aux is BindFlags
	OpMakeRef // A dest Value, B Location: the location becomes a reference, which A then holds
)

// BindFlags is the Aux of OpBind.
type BindFlags uint16

// BindValue makes the source B a Value register, which the instruction
// empties: a reference OpMakeRef, a by-reference IterNext or a
// by-reference argument put there is bound to A; any other value, a
// by-reference parameter's default, is assigned to A.
const BindValue BindFlags = 1

// FetchFlags is the Aux of OpFetchDim, PHP's FETCH_DIM_W, FETCH_DIM_RW and
// FETCH_DIM_UNSET: it makes A name the element of the container B, so that
// the next instruction can write, increment or unset through it.
//
// The mode is the low two bits. FetchWrite creates a missing element as
// null; FetchReadWrite does the same after an "Undefined array key"
// warning; FetchUnset creates nothing and names a null scratch location
// when the element or the container is missing.
//
// FetchForIncDec and FetchForAssignOp say what consumes the element, which
// PHP shows in the Error for a string container and in when a scalar
// container's key variable is read; without either the element is the
// container of a further element operation.
type FetchFlags uint16

const (
	FetchWrite     FetchFlags = 0
	FetchReadWrite FetchFlags = 1
	FetchUnset     FetchFlags = 2
	fetchModeMask  FetchFlags = 3

	FetchKeyLocation FetchFlags = 1 << 2 // C is a Location the instruction reads itself
	FetchAppend      FetchFlags = 1 << 3 // no key ($a[]): C is zero

	FetchForIncDec   FetchFlags = 1 << 4
	FetchForAssignOp FetchFlags = 2 << 4
	FetchForRef      FetchFlags = 3 << 4 // format 1.3: the element is bound by reference (ZEND_FETCH_DIM_REF)
	fetchUseMask     FetchFlags = 3 << 4

	// FetchContainerValue (format 1.4) makes the container B a Value
	// register that holds a reference (the result of MakeRef or of a
	// by-reference IterNext), as ZEND_FETCH_DIM_W on a VAR operand: the
	// element of a destructuring assignment by reference is fetched from
	// the reference the right-hand side became. Only with FetchWrite and
	// FetchForRef.
	FetchContainerValue FetchFlags = 1 << 6

	fetchKnown = fetchModeMask | FetchKeyLocation | FetchAppend | fetchUseMask | FetchContainerValue
)

// Mode is FetchWrite, FetchReadWrite or FetchUnset.
func (f FetchFlags) Mode() FetchFlags { return f & fetchModeMask }

// Use is zero, FetchForIncDec, FetchForAssignOp or FetchForRef.
func (f FetchFlags) Use() FetchFlags { return f & fetchUseMask }

// IterFlags is the Aux of OpIterInit, OpIterNext and OpIterKey.
type IterFlags uint16

const (
	// IterLocation makes the destination of IterNext or IterKey a Location
	// the instruction assigns to, as FE_FETCH_R writes a plain variable
	// itself.
	IterLocation IterFlags = 1
	// IterByRef (format 1.3) is foreach by reference, FE_RESET_RW and
	// FE_FETCH_RW. On IterInit the subject B is a Location, or a Value
	// register holding a temporary; on IterNext the element is bound to the
	// Location B (with IterLocation) or put as a reference into the Value
	// register B for a following Bind.
	IterByRef IterFlags = 2
)

// ArraySetResultShift is where the Aux of OpArraySet holds its result
// register: (Aux >> ArraySetResultShift) is zero for no result, or the
// index of a Value register plus one. The low bits are ArrayWriteFlags and
// bit 2 is zero. Only registers below ArraySetMaxResult can be named.
const (
	ArraySetResultShift        = 3
	ArraySetMaxResult   uint32 = 1<<(16-ArraySetResultShift) - 1
)

// ArraySetAux is the Aux of an OpArraySet that stores its result, the value
// of the PHP assignment expression, in the Value register result. ok is
// false when the register cannot be named.
func ArraySetAux(flags ArrayWriteFlags, result uint32) (aux uint16, ok bool) {
	if result >= ArraySetMaxResult {
		return 0, false
	}
	return uint16(flags) | uint16(result+1)<<ArraySetResultShift, true
}

// ArraySetResult decodes the result register of an OpArraySet Aux.
func ArraySetResult(aux uint16) (result uint32, ok bool) {
	if r := aux >> ArraySetResultShift; r != 0 {
		return uint32(r - 1), true
	}
	return 0, false
}

// ArrayWriteFlags is the Aux of OpArraySet, OpArrayAppend and OpArrayUnset
// (KeyLocation only; ZEND_UNSET_DIM reads the key after reporting an
// undefined container). Without a flag
// the operand is a Value register read before the instruction; with it, the
// operand is a Location the instruction reads itself, with PHP's undefined
// variable warning, at the point ZEND_ASSIGN_DIM reads that CV: the key after
// the container check, the value (OP_DATA) after the key is converted.
type ArrayWriteFlags uint16

const (
	ValueLocation ArrayWriteFlags = 1 << iota // the value operand is a Location
	KeyLocation                               // OpArraySet: the key operand B is a Location
)

// ArrayAddFlags is the Aux of OpArrayAdd, which adds one element of an
// array literal (ZEND_INIT_ARRAY / ZEND_ADD_ARRAY_ELEMENT).
type ArrayAddFlags uint16

const (
	AddKey         ArrayAddFlags = 1 << iota // C is the key; without it the value is appended
	AddKeyLocation                           // with AddKey: C is a Location read by the instruction; undefined is the key ""
)

// UnaryOp is the Aux of OpUnary.
type UnaryOp uint16

const (
	Not UnaryOp = iota + 1
	Negate
	Plus
	BitNot
)

// CastKind is the Aux of OpCast.
type CastKind uint16

const (
	CastBool CastKind = iota + 1
	CastInt
	CastFloat
	CastString
	CastArray
)

// IncDecOp is the Aux of OpIncDec. The result is the new value for the pre
// forms and the old value for the post forms.
type IncDecOp uint16

const (
	PreInc IncDecOp = iota + 1
	PreDec
	PostInc
	PostDec
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
	// Format 1.1 (W-17).
	Modulo
	Power
	ShiftLeft
	ShiftRight
	NotEqual
	NotIdentical
	LessOrEqual
	Spaceship
	BoolXor
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
// LocalNames is empty (no names, as in every format 1.0 function) or has
// exactly Locals entries, each the index of a String constant holding the
// variable name without "$". Names need not be unique or non-empty.
type Function struct {
	Name                     string
	Params, Locals, MaxStack uint32
	ReturnKind               RegisterKind
	Registers                []RegisterKind
	Code                     []Instruction
	Positions                []Position
	Handlers                 []Handler
	LocalNames               []uint32
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
	// Source and Pos locate the failing instruction in PHP source when Build
	// knows it; Pos is zero otherwise. Error() does not include them.
	Source string
	Pos    Position
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v at byte %d, function %d, instruction %d: %s", e.Kind, e.Offset, e.Function, e.PC, e.Detail)
}
func (e *Error) Unwrap() error { return e.Kind }

// Build validates the full image before publishing an immutable program.
func Build(ctx context.Context, image Image, opts Options) (*Program, error) {
	p, err := build(ctx, image, opts)
	return p, locate(image, err)
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

// Constants appends a copy of the constant table to dst and returns it.
// Changing the result never affects p.
func (p *Program) Constants(dst []Constant) []Constant {
	return append(dst, p.image.Constants...)
}

// Instructions appends a copy of the code of function to dst. It reports
// false, returning dst unchanged, when function is out of range. A VM may use
// it to build its own private decoded form once per Program; changing the
// result never affects p.
func (p *Program) Instructions(function uint32, dst []Instruction) ([]Instruction, bool) {
	if uint64(function) >= uint64(len(p.image.Functions)) {
		return dst, false
	}
	return append(dst, p.image.Functions[function].Code...), true
}

// FunctionView has no mutating operations or exposed slices. constants is
// the Program's own pool, read only to resolve local names.
type FunctionView struct {
	function  *Function
	constants []Constant
}

func (p *Program) Function(index uint32) (FunctionView, bool) {
	if uint64(index) >= uint64(len(p.image.Functions)) {
		return FunctionView{}, false
	}
	return FunctionView{&p.image.Functions[index], p.image.Constants}, true
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

// LocalName is the PHP name, without "$", of local index. ok is false when
// the function has no names or index is out of range.
func (f FunctionView) LocalName(index uint32) (name string, ok bool) {
	if f.function == nil || uint64(index) >= uint64(len(f.function.LocalNames)) {
		return "", false
	}
	c := f.function.LocalNames[index]
	if uint64(c) >= uint64(len(f.constants)) {
		return "", false // unreachable for a verified Program
	}
	return f.constants[c].Bytes, true
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
