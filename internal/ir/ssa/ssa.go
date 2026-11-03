// Package ssa builds structured value SSA from immutable PHP HIR.
//
// Each occurrence defines a distinct value. PHP locations remain explicit HIR
// operations, including unaliased locals: no load is cached or promoted. Child
// edges retain HIR evaluation rules; the value table is NOT an eager execution
// schedule. In particular, lazy branches and loops must be entered through their
// controlling operation. This representation needs no location phi nodes.
package ssa

import (
	"context"
	"errors"

	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

type ID uint32

// Zero is the absent operand, never a definition.
const None ID = 0

type Passes uint8

const (
	FoldConstants Passes = 1 << iota
	SpecializeTypes
	AllPasses = FoldConstants | SpecializeTypes
)

type Type uint8

const (
	Unknown Type = iota
	Null
	Bool
	Int
	Float
	String
)

type Specialization uint8

const (
	Generic Specialization = iota
	BoolNot
	IntBitwise
)

// Sources is a bounded union of source categories. Bit 63 represents unknown
// provenance; producers unable to represent a source must include UnknownSource.
// It is metadata, not a substitute for W-21 string-segment provenance.
type Sources uint64

const UnknownSource Sources = 1 << 63

// Effects adds compiler ordering obligations absent from the original HIR table.
type Effects struct {
	PHP ir.Effect
	// Ordered includes location reads, opaque operations, allocation, policy,
	// quota and cancellation checks. Passes never remove or reorder these.
	Ordered bool
}

type Options struct {
	Passes           Passes // zero builds the unoptimized comparison artifact
	MaxNodes         int    // positive definition + operand record limit for the whole unit
	MaxDepth         int    // positive HIR operand nesting limit
	MaxConstantBytes int    // nonnegative total newly folded string bytes
	// Sources is copied, never retained. Missing source introducers become
	// UnknownSource. An explicit zero asserts only that the node's own source
	// is verified trusted; it cannot clear sources inherited from operands.
	Sources map[*ir.Node]Sources
}

// Value is a read-only snapshot. HIR is shared under ir.Lower's immutability
// contract. Constant is copied, so changing a returned snapshot cannot change
// the artifact. Folded values retain their original operand edges and origins.
type Value struct {
	HIR            *ir.Node
	Constant       ir.Literal
	IsConstant     bool
	Type           Type
	Specialization Specialization
	Effects        Effects
	Sources        Sources
}

type definition struct {
	value Value
	// Unary/binary operands stay inline. For larger nodes operands[0] is
	// the side-slab offset. Counts already exist in the immutable HIR.
	operands [2]ID
}

func (d *definition) edge(p *Program, index int) ID {
	if len(d.value.HIR.Kids)+len(d.value.HIR.Args) <= 2 {
		return d.operands[index]
	}
	return p.inputs[int(d.operands[0])+index]
}

type function struct {
	hir      *ir.Func
	roots    []ID
	defaults []ID
}

// Program contains compilation metadata and scalar constants only. There are
// no executions, environments, statics, sessions, heaps, or failure states here.
// All state is private and immutable after Build, permitting concurrent reads.
type Program struct {
	unit         *ir.Unit
	profile      profile.Profile
	file         string
	values       []definition
	inputs       []ID
	functions    []function
	initializers []ID
}

func (p *Program) Profile() profile.Profile { return p.profile }
func (p *Program) File() string             { return p.file }
func (p *Program) Len() int                 { return len(p.values) }
func (p *Program) FunctionCount() int       { return len(p.functions) }

// HIR retains all declaration metadata and compile diagnostics. It is immutable
// under the same contract as the node/function origins returned by this package.
func (p *Program) HIR() *ir.Unit { return p.unit }

// CheckProfile rejects cross-profile cache reuse, including parser INI settings.
func (p *Program) CheckProfile(prof profile.Profile) error {
	if p.profile != prof {
		return errors.New("ssa artifact profile mismatch")
	}
	return nil
}

// Initializers are class declaration templates, not eagerly executed code.
func (p *Program) Initializers() []ID { return append([]ID(nil), p.initializers...) }

func (p *Program) Value(id ID) (Value, bool) {
	if id == None || uint64(id) > uint64(len(p.values)) {
		return Value{}, false
	}
	return p.values[id-1].value, true
}

func (p *Program) Child(id ID, index int) ID {
	if id == None || uint64(id) > uint64(len(p.values)) {
		return None
	}
	d := p.values[id-1]
	if index < 0 || index >= len(d.value.HIR.Kids) {
		return None
	}
	return d.edge(p, index)
}

func (p *Program) Argument(id ID, index int) ID {
	if id == None || uint64(id) > uint64(len(p.values)) {
		return None
	}
	d := p.values[id-1]
	if index < 0 || index >= len(d.value.HIR.Args) {
		return None
	}
	return d.edge(p, len(d.value.HIR.Kids)+index)
}

func (p *Program) Function(index int) (*ir.Func, []ID, bool) {
	if index < 0 || index >= len(p.functions) {
		return nil, nil, false
	}
	f := p.functions[index]
	return f.hir, append([]ID(nil), f.roots...), true
}

func (p *Program) Default(index, parameter int) ID {
	if index < 0 || index >= len(p.functions) || parameter < 0 || parameter >= len(p.functions[index].defaults) {
		return None
	}
	return p.functions[index].defaults[parameter]
}

var ErrLimit = errors.New("ssa compilation limit exceeded")

// Build preserves the input unit. Failure returns no executable partial image.
func Build(ctx context.Context, unit *ir.Unit, opts Options) (*Program, error) {
	return build(ctx, unit, opts)
}
