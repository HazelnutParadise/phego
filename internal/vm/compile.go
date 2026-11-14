// Package vm compiles HIR to Phego bytecode for the first W-17 stage. It
// never executes PHP. The encoding contract shared with the interpreter is
// recorded in engineering/decisions/W-17-vm.md and the W-17 stage-1 conventions.
//
// REQ-025, REQ-026; T-009, T-010.
package vm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// UnsupportedError reports the first construct, in source order, that this
// compiler stage cannot translate.
type UnsupportedError struct {
	Feature string // e.g. "foreach", "class declaration", "parameter default value"
	File    string
	Line    int
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("%s:%d: unsupported PHP construct: %s", e.File, e.Line, e.Feature)
}

// CompileError is an error PHP 8.5.12 raises while compiling the file, such
// as declaring a function whose name is a builtin. PHP prints it as
// "Fatal error: <Message> in <File> on line <Line>" before running anything.
type CompileError struct {
	Message string // e.g. "Cannot redeclare function strlen()"
	File    string
	Line    int
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
}

// Identity placeholders of stage 1 (engineering/decisions/W-17-vm.md); cache keys
// arrive with W-23.
const (
	guestPlatform    = "php-8.5.12"
	target           = "portable-go"
	compilerManifest = "phego/internal/vm stage-5 format-1.5"
	modulesManifest  = "phego/modules/none"
	runtimeManifest  = "phego/runtime-abi/w17-1"
	policyManifest   = "phego/policy/unbound"
	scopeManifest    = "phego/scope/engine"
)

// Static limits of bytecode.DefaultLimits that the compiler itself can
// exceed. Crossing one is reported as an unsupported (too large) program.
var limits = bytecode.DefaultLimits()

// internalError carries an unexpected compiler state out of a panic.
type internalError struct{ msg string }

// ctxError carries a context error out of the compilation.
type ctxError struct{ err error }

// Compile lowers one HIR unit to a bytecode image. source is the file's bytes
// (for the Source record and digests). It never executes PHP.
func Compile(ctx context.Context, unit *ir.Unit, source []byte) (image bytecode.Image, err error) {
	return CompileWith(ctx, unit, source, Options{})
}

// Options are what the compiler needs to know about its host.
type Options struct {
	// HostFunction reports whether the host registered a function under
	// the lower-case name. A call to one is reported as unsupported: it
	// must not be compiled as a call to an undefined function.
	HostFunction func(lower string) bool
}

// CompileWith is Compile with the host's options.
func CompileWith(ctx context.Context, unit *ir.Unit, source []byte, opts Options) (image bytecode.Image, err error) {
	if unit == nil || unit.Main == nil {
		return bytecode.Image{}, errors.New("vm: Compile needs a lowered unit")
	}
	if err := ctx.Err(); err != nil {
		return bytecode.Image{}, err
	}
	if uint64(len(source)) > math.MaxUint32 {
		return bytecode.Image{}, &UnsupportedError{Feature: "source file too large", File: unit.File, Line: 1}
	}
	c := &compiler{
		ctx:        ctx,
		unit:       unit,
		constIndex: map[bytecode.Constant]uint32{},
		funcIndex:  map[string]uint32{},
		host:       opts.HostFunction,
		lines:      sourceLines(source),
	}
	c.declaredConsts = declaredConstants(unit)
	defer func() {
		if r := recover(); r != nil {
			image = bytecode.Image{}
			switch x := r.(type) {
			case *UnsupportedError:
				err = x
			case ctxError:
				err = x.err
			case internalError:
				err = fmt.Errorf("vm: internal compiler error at %s:%d: %s", unit.File, c.line, x.msg)
			default:
				err = fmt.Errorf("vm: internal compiler error at %s:%d: %v", unit.File, c.line, r)
			}
		}
	}()
	return c.compile(source)
}

// sourceLines is the number of lines the frontend counts in source: \n,
// \r\n and a lone \r each end a line, as in PHP's scanner.
func sourceLines(source []byte) uint32 {
	n := uint32(1)
	for i, c := range source {
		if c == '\n' || c == '\r' && (i+1 >= len(source) || source[i+1] != '\n') {
			n++
		}
	}
	return n
}

type compiler struct {
	// host reports a function the host registered; nil when there is none.
	host       func(lower string) bool
	ctx        context.Context
	unit       *ir.Unit
	constants  []bytecode.Constant
	constIndex map[bytecode.Constant]uint32
	// funcIndex maps the lower-case name of a hoisted function to its image
	// index.
	funcIndex map[string]uint32
	// declaredConsts holds the registered names of the constants the unit
	// declares, for the fallback of unqualified names in a namespace.
	declaredConsts map[string]bool
	lines          uint32
	line           int // line of the node being compiled, for internal errors
}

func (c *compiler) compile(source []byte) (bytecode.Image, error) {
	unit := c.unit
	if uint64(len(unit.Functions))+1 > uint64(limits.MaxFunctions) {
		return bytecode.Image{}, &UnsupportedError{Feature: "program too large", File: unit.File, Line: 1}
	}
	for i, f := range unit.Functions {
		if !f.Hoisted {
			continue
		}
		// zend_begin_func_decl: a hoisted declaration of an internal
		// function's name (any case, declared spelling in the message)
		// fails the whole file at compile time, ahead of anything stage 1
		// cannot translate. Functions declared in a namespace never match,
		// as their names carry the namespace.
		if isBuiltin(f.Name) {
			return bytecode.Image{}, &CompileError{Message: "Cannot redeclare function " + f.Name + "()", File: unit.File, Line: positive(f.Line, 1)}
		}
		c.funcIndex[phpname.Lower(f.Name)] = uint32(i + 1)
	}
	funcs := append([]*ir.Func{unit.Main}, unit.Functions...)
	out := make([]bytecode.Function, len(funcs))
	var first *UnsupportedError
	firstRank := 0
	for i, f := range funcs {
		fn, err := c.function(f)
		if err != nil {
			var ue *UnsupportedError
			if !errors.As(err, &ue) {
				return bytecode.Image{}, err
			}
			// Main code competes on its line; a function body that starts on
			// the same line as a main statement comes after it.
			rank := ue.Line*2 + boolInt(i > 0)
			if first == nil || rank < firstRank {
				first, firstRank = ue, rank
			}
			continue
		}
		out[i] = fn
	}
	if first != nil {
		return bytecode.Image{}, first
	}
	for _, f := range unit.Functions {
		if !f.Hoisted {
			// Conditional declarations are reached through OpFuncDecl, which
			// already failed; this is a last guard against a silent omission.
			return bytecode.Image{}, &UnsupportedError{Feature: "conditional function declaration", File: unit.File, Line: f.Line}
		}
	}
	total := 0
	for _, f := range out {
		total += len(f.Code)
	}
	if uint64(total) > uint64(limits.MaxInstructions) || uint64(len(c.constants)) > uint64(limits.MaxConstants) {
		return bytecode.Image{}, &UnsupportedError{Feature: "program too large", File: unit.File, Line: 1}
	}
	digest := sha256.Sum256(source)
	return bytecode.Image{
		Identity: bytecode.Identity{
			Profile:       unit.Profile,
			GuestPlatform: guestPlatform,
			Target:        target,
			Source:        digest,
			Compiler:      sha256.Sum256([]byte(compilerManifest)),
			Modules:       sha256.Sum256([]byte(modulesManifest)),
			RuntimeABI:    sha256.Sum256([]byte(runtimeManifest)),
			Policy:        sha256.Sum256([]byte(policyManifest)),
			Scope:         sha256.Sum256([]byte(scopeManifest)),
		},
		Entry:     0,
		Constants: c.constants,
		Sources:   []bytecode.Source{{Name: unit.File, Digest: digest, Bytes: uint32(len(source)), Lines: c.lines}},
		Functions: out,
	}, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// constant returns the pool index of k, adding it once.
func (c *compiler) constant(k bytecode.Constant) uint32 {
	if i, ok := c.constIndex[k]; ok {
		return i
	}
	i := uint32(len(c.constants))
	c.constants = append(c.constants, k)
	c.constIndex[k] = i
	return i
}

func (c *compiler) stringConst(s string) uint32 {
	return c.constant(bytecode.Constant{Kind: bytecode.String, Bytes: s})
}

// literal converts a HIR literal to a pool constant.
func (c *compiler) literal(v *ir.Literal) uint32 {
	switch v.Kind {
	case ir.LitNull:
		return c.constant(bytecode.Constant{Kind: bytecode.Null})
	case ir.LitBool:
		return c.constant(bytecode.Constant{Kind: bytecode.Bool, Bits: uint64(boolInt(v.Bool))})
	case ir.LitInt:
		if c.unit.Profile.IntWidth == 32 && int64(int32(v.Int)) != v.Int {
			panic(internalError{fmt.Sprintf("integer literal %d outside the 32-bit profile", v.Int)})
		}
		return c.constant(bytecode.Constant{Kind: bytecode.Int, Bits: uint64(v.Int)})
	case ir.LitFloat:
		return c.constant(bytecode.Constant{Kind: bytecode.Float, Bits: math.Float64bits(v.Float)})
	case ir.LitString:
		return c.stringConst(v.Str)
	}
	panic(internalError{fmt.Sprintf("unknown literal kind %d", v.Kind)})
}
