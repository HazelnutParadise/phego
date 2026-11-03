package ir

import (
	"math"
	"strings"
)

// Effect is the set of PHP-observable behaviours an operation may have.
// Sets are conservative: a missing flag is a guarantee, a present flag is
// only a possibility.
type Effect uint16

const (
	// MayWarn: the operation may raise a notice, warning or deprecation,
	// which also runs a user error handler when one is installed.
	MayWarn Effect = 1 << iota
	// MayThrow: the operation may throw an exception or Error.
	MayThrow
	// MayCallUser: the operation may run user code directly: functions,
	// magic methods, property hooks, autoloaders, destructors or iterators.
	// User code can do anything, including the other effects.
	MayCallUser
	// MayOutput: the operation writes output, which may run an output
	// buffer callback.
	MayOutput
	// MayExit: the operation may end the script.
	MayExit
	// MayWrite: the operation writes a variable, element, property or
	// static slot.
	MayWrite
	// MayAlias: the operation may create a reference binding.
	MayAlias
	// MayDynamicScope: the operation may read or write local variables by
	// a run-time name.
	MayDynamicScope
	// MayCompile: the operation compiles and runs new code (include, eval).
	MayCompile
	// MaySuspend: the operation may suspend the function (yield).
	MaySuspend
	// MayExternal: the operation reaches outside the engine (shell).
	MayExternal
)

// Anything is the effect set of a call to unknown user code.
const Anything = MayWarn | MayThrow | MayCallUser | MayOutput | MayExit | MayWrite | MayAlias

// AllEffects is the conservative fallback for unknown operations and callbacks.
const AllEffects = Anything | MayDynamicScope | MayCompile | MaySuspend | MayExternal

// OperationEffects reports intrinsic effects, excluding operands. Malformed
// context-dependent nodes conservatively return every effect, never pure.
func OperationEffects(n *Node) Effect {
	if n == nil {
		return 0
	}
	switch n.Op {
	case OpUnary, OpCast:
		if len(n.Kids) != 1 {
			return AllEffects
		}
	case OpBinary:
		if len(n.Kids) != 2 {
			return AllEffects
		}
	case OpClosure:
		if n.Func == nil {
			return AllEffects
		}
	case OpArray:
		for _, k := range n.Kids {
			if k != nil && k.Op == OpItem && len(k.Kids) != 2 {
				return AllEffects
			}
		}
	case OpMatch:
		if len(n.Kids) == 0 {
			return AllEffects
		}
		for _, k := range n.Kids[1:] {
			if k == nil {
				return AllEffects
			}
		}
	}
	return ownEffects(n)
}

var effectNames = []string{"warn", "throw", "call", "output", "exit", "write", "alias", "dynscope", "compile", "suspend", "external"}

func (e Effect) String() string {
	var parts []string
	for i, name := range effectNames {
		if e&(1<<i) != 0 {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "pure"
	}
	return strings.Join(parts, ",")
}

// opEffects is the effect of each operation itself, before its operands are
// added. Context-dependent cases are adjusted in ownEffects.
var opEffects = map[Op]Effect{
	OpLiteral: 0, OpArray: 0, OpItem: 0, OpThis: MayThrow,
	// Unpacking a Traversable runs its iterator; other values throw.
	OpSpread: MayCallUser | MayThrow,
	// Converting each part to a string may run __toString or warn.
	OpInterp:      MayWarn | MayThrow | MayCallUser,
	OpVar:         MayWarn,
	OpVarVar:      MayWarn | MayThrow | MayCallUser | MayDynamicScope,
	OpSuperGlobal: MayWarn,
	// Reading an element may call ArrayAccess::offsetGet.
	OpDim: MayWarn | MayThrow | MayCallUser,
	// Property access may run __get, __set, hooks or lazy initializers.
	OpProp: MayWarn | MayThrow | MayCallUser, OpNullsafe: MayWarn | MayThrow | MayCallUser,
	// Class lookups may run an autoloader.
	OpStaticProp: MayWarn | MayThrow | MayCallUser, OpClassConst: MayWarn | MayThrow | MayCallUser,
	// A constant may be deprecated, which warns.
	OpClassName: MayThrow, OpConst: MayWarn | MayThrow, OpMagic: 0,
	// Overwriting a value may run its destructor.
	OpAssign:      MayWrite | MayThrow | MayCallUser,
	OpAssignRef:   MayWrite | MayAlias | MayThrow | MayCallUser | MayWarn,
	OpAssignOp:    MayWrite | MayWarn | MayThrow | MayCallUser,
	OpCoalesceSet: MayWrite | MayThrow | MayCallUser,
	OpDestructure: MayWrite | MayWarn | MayThrow | MayCallUser,
	OpPreInc:      MayWrite | MayWarn | MayThrow, OpPostInc: MayWrite | MayWarn | MayThrow,
	OpUnary:   MayWarn | MayThrow,
	OpSilence: 0,
	OpBinary:  MayWarn | MayThrow, OpLogical: MayWarn, OpCoalesce: 0, OpTernary: MayWarn,
	OpCast:  MayWarn | MayThrow | MayCallUser,
	OpIsset: 0, OpEmpty: MayWarn,
	OpCall: Anything, OpCallValue: Anything, OpMethodCall: Anything, OpNullsafeMC: Anything, OpStaticCall: Anything,
	OpCallable:   MayThrow | MayCallUser,
	OpNew:        Anything,
	OpClone:      MayThrow | MayCallUser,
	OpClosure:    MayWarn,
	OpArrowFn:    0,
	OpInstanceof: MayThrow,
	OpInclude:    Anything | MayCompile | MayDynamicScope,
	OpEval:       Anything | MayCompile | MayDynamicScope,
	OpExit:       MayExit | MayOutput | MayCallUser | MayThrow,
	OpPrint:      MayOutput | MayWarn | MayThrow | MayCallUser,
	OpShell:      MayExternal | MayWarn | MayThrow | MayCallUser,
	OpMatch:      MayThrow,
	OpArm:        0,
	OpThrow:      MayThrow,
	OpYield:      MaySuspend | MayCallUser | MayThrow | MayExit | MayOutput | MayWarn | MayWrite,
	OpYieldFrom:  MaySuspend | MayCallUser | MayThrow | MayExit | MayOutput | MayWarn | MayWrite,

	OpEcho: MayOutput | MayWarn | MayThrow | MayCallUser, OpExpr: 0, OpVoid: 0,
	OpHTML: MayOutput, OpIf: MayWarn, OpWhile: MayWarn, OpDo: MayWarn, OpFor: MayWarn,
	// Iteration may run Iterator methods or warn about non-iterable values.
	OpForeach: MayWarn | MayThrow | MayCallUser | MayWrite,
	// Switch compares loosely, which may call __toString.
	OpSwitch: MayWarn | MayThrow | MayCallUser, OpCase: 0,
	OpBreak: 0, OpContinue: 0, OpReturn: 0,
	OpGlobal: MayWrite | MayAlias | MayCallUser | MayThrow, OpStatic: MayWrite | MayAlias | MayCallUser | MayThrow,
	// Unsetting may run a destructor, offsetUnset or __unset.
	OpUnset: MayWrite | MayThrow | MayCallUser,
	OpTry:   0, OpCatch: MayWrite | MayCallUser | MayThrow, OpFinally: 0,
	OpGoto: 0, OpLabel: 0, OpDeclare: 0, OpDirective: 0, OpBlock: 0,
	OpFuncDecl: MayThrow,
	// Declaring a class at run time may autoload its parents.
	// Linking may also warn, for example about an incompatible return type.
	OpClass:    MayWarn | MayThrow | MayCallUser,
	OpConstDef: MayWarn | MayThrow,
	OpHalt:     0,
}

// ownEffects refines the table for operands whose types make an effect
// impossible.
func ownEffects(n *Node) Effect {
	e, known := opEffects[n.Op]
	if !known {
		return Anything | MayDynamicScope | MayCompile | MaySuspend | MayExternal
	}
	if n.Flags&FlagRef != 0 {
		e |= MayAlias | MayWrite // the node is bound by reference
	}
	switch n.Op {
	case OpReturn:
		if n.Flags&FlagRef != 0 {
			e |= MayWarn
		}
		if n.Flags&FlagCoerce != 0 {
			e |= MayWarn | MayThrow | MayCallUser
		}
	case OpVar, OpSuperGlobal:
		if n.Flags&(FlagWrite|FlagQuiet|FlagRef|FlagUnset) != 0 {
			e &^= MayWarn
		}
		if n.Flags&FlagUnset != 0 {
			e |= MayWrite | MayCallUser | MayThrow
		}
	case OpDim:
		if n.Flags&FlagQuiet != 0 {
			e &^= MayWarn
		}
	case OpConst:
		if n.Name != nil && n.Name.Fallback == "" && isBuiltinConstant(n.Name.Resolved) {
			e = 0
		}
	case OpBinary:
		e = binaryEffects(n)
	case OpUnary:
		v := lit(n.Kids[0])
		switch {
		case n.Text == "!" && v != nil && !nanLiteral(v):
			e = 0
		case n.Text == "!":
			e = MayWarn // NaN truthiness warns, including a run-time variable.
		case v != nil && v.Kind == LitInt, v != nil && v.Kind == LitFloat && n.Text != "~":
			e = 0
		case v != nil && v.Kind == LitFloat:
			e = MayWarn // ~ on a fractional float deprecates the implicit conversion
		}
	case OpCast:
		e = castEffects(n)
	case OpInterp:
		allStrings := true
		for _, k := range n.Kids {
			if l := lit(k); l == nil || l.Kind == LitNull || nanLiteral(l) {
				allStrings = false
			}
		}
		if allStrings {
			e = 0
		}
	case OpEcho, OpPrint:
		allScalar := true
		for _, k := range n.Kids {
			if l := lit(k); l == nil || nanLiteral(l) {
				allScalar = false
			}
		}
		if allScalar {
			e = MayOutput
		}
	case OpArray:
		for _, k := range n.Kids {
			if k != nil && k.Op == OpItem && k.Kids[0] != nil && !isArrayKeyLiteral(k.Kids[0]) {
				e |= MayWarn | MayThrow
			}
		}
	case OpMatch:
		for _, arm := range n.Kids[1:] {
			if len(arm.Kids) == 1 {
				e &^= MayThrow // a default arm handles every value
			}
		}
	case OpClosure:
		e = 0
		for _, c := range n.Func.Captures {
			if c == nil {
				return AllEffects
			}
			if c.ByRef {
				e |= MayAlias | MayWrite
			} else {
				e |= MayWarn
			}
		}
	}
	return e
}

func lit(n *Node) *Literal {
	if n != nil && n.Op == OpLiteral {
		return n.Value
	}
	return nil
}

func numeric(l *Literal) bool { return l.Kind == LitInt || l.Kind == LitFloat }

func nanLiteral(l *Literal) bool { return l.Kind == LitFloat && math.IsNaN(l.Float) }

func isArrayKeyLiteral(n *Node) bool {
	l := lit(n)
	// A null key is deprecated in PHP 8.5, so it is not a quiet key.
	return l != nil && (l.Kind == LitInt || l.Kind == LitString || l.Kind == LitBool)
}

// binaryEffects narrows operator effects for literal operands. Integer and
// float arithmetic cannot warn or throw except for division, modulo and
// shifts, which may throw on a zero or negative right operand.
func binaryEffects(n *Node) Effect {
	a, b := lit(n.Kids[0]), lit(n.Kids[1])
	switch n.Text {
	case "===", "!==":
		return 0
	case "|>":
		return Anything
	case "==", "!=", "<>", "<", "<=", ">", ">=", "<=>":
		// Loose comparison of objects may call __toString or recurse too deep.
		if (a != nil || simpleScalar(n.Kids[0])) && (b != nil || simpleScalar(n.Kids[1])) {
			return 0
		}
		return MayThrow | MayCallUser | MayWarn
	case ".":
		if a != nil && b != nil && !nanLiteral(a) && !nanLiteral(b) {
			return 0
		}
		return MayWarn | MayThrow | MayCallUser
	}
	if a == nil || b == nil || !numeric(a) || !numeric(b) {
		return MayWarn | MayThrow
	}
	switch n.Text {
	case "+", "-", "*":
		return 0
	case "**":
		// PHP 8.5 deprecates a zero base with a negative exponent.
		if (a.Kind == LitInt && a.Int == 0 || a.Kind == LitFloat && a.Float == 0) &&
			(b.Kind == LitInt && b.Int < 0 || b.Kind == LitFloat && b.Float < 0) {
			return MayWarn
		}
		return 0
	case "/", "%":
		if (b.Kind == LitInt && b.Int != 0) && n.Text == "/" {
			return 0
		}
		if n.Text == "%" && (a.Kind == LitFloat || b.Kind == LitFloat) {
			return MayThrow | MayWarn // % converts floats to int, deprecating fractions
		}
		return MayThrow
	case "<<", ">>":
		if a.Kind == LitInt && b.Kind == LitInt && b.Int >= 0 {
			return 0
		}
		return MayThrow | MayWarn
	case "&", "|", "^":
		if a.Kind == LitInt && b.Kind == LitInt {
			return 0
		}
		return MayWarn | MayThrow
	}
	return MayWarn | MayThrow
}

// simpleScalar reports operands whose comparison cannot reach user code
// because they are produced by a literal-only computation.
func simpleScalar(n *Node) bool {
	return n != nil && n.Op == OpLiteral
}

func castEffects(n *Node) Effect {
	l := lit(n.Kids[0])
	switch n.Text {
	case "bool":
		if l == nil || nanLiteral(l) {
			return MayWarn
		}
		return 0
	case "array", "object":
		return 0
	case "string":
		if l != nil && !nanLiteral(l) {
			return 0
		}
		return MayWarn | MayThrow | MayCallUser
	}
	if l != nil {
		if l.Kind == LitFloat && n.Text == "int" {
			return MayWarn // non-representable float to int conversion warns
		}
		return 0
	}
	return MayWarn | MayThrow
}

func isBuiltinConstant(name string) bool {
	switch strings.ToLower(name) {
	case "true", "false", "null":
		return true
	}
	return false
}
