// Package ir lowers a PHP syntax tree into Phego's high-level intermediate
// representation (HIR).
//
// HIR keeps every PHP-observable operation as its own node: a property read
// stays a property read (it may run __get or a hook), a call keeps its
// argument list and send modes, and a conversion keeps its target type. Names
// are resolved with the file's namespace and imports, variables are bound to
// per-function symbols, and every node carries the conservative set of
// effects it may have. Optimizations may only remove or move an operation
// when its effects allow it.
//
// HIR is immutable after Lower returns and may be shared between executions
// of the same profile; it holds no request state.
package ir

import "github.com/HazelnutParadise/phego/internal/profile"

// Op names a HIR operation.
type Op string

// Expression operations.
const (
	OpLiteral     Op = "literal"      // Value
	OpInterp      Op = "interp"       // Kids are string parts converted and joined in order
	OpArray       Op = "array"        // Kids are OpItem / OpSpread
	OpItem        Op = "item"         // Kids: key (may be nil), value; FlagRef for &$v
	OpSpread      Op = "spread"       // Kids: unpacked operand
	OpVar         Op = "var"          // Var
	OpVarVar      Op = "var_var"      // Kids: name expression; reads or writes the local table by name
	OpSuperGlobal Op = "superglobal"  // Text is the name without $
	OpThis        Op = "this"         // $this
	OpDim         Op = "dim"          // Kids: base, index (nil for [])
	OpProp        Op = "prop"         // Kids: object, name expression; Text for a literal name
	OpNullsafe    Op = "nullsafe"     // like OpProp, short-circuits on null
	OpStaticProp  Op = "static_prop"  // Name or Kids[0] class; Kids[1] name expression; Text literal name
	OpClassConst  Op = "class_const"  // Name or Kids[0] class; Text constant name or Kids[1] expression
	OpClassName   Op = "class_name"   // X::class; Name or Kids[0]
	OpConst       Op = "const"        // Name
	OpMagic       Op = "magic"        // magic constant resolved at run time (Text), e.g. __CLASS__ in traits
	OpAssign      Op = "assign"       // Kids: target, value
	OpAssignRef   Op = "assign_ref"   // Kids: target, source
	OpAssignOp    Op = "assign_op"    // Text: binary operator; Kids: target, value
	OpCoalesceSet Op = "coalesce_set" // ??=; Kids: target, value (value evaluated only when needed)
	OpDestructure Op = "destructure"  // Kids: OpArray pattern, value
	OpPreInc      Op = "pre_inc"      // Text "++" or "--"; Kids: target
	OpPostInc     Op = "post_inc"
	OpUnary       Op = "unary"   // Text: ! - + ~
	OpSilence     Op = "silence" // @expr
	OpBinary      Op = "binary"  // Text: arithmetic, bitwise, string, comparison or |>
	OpLogical     Op = "logical" // Text: && || xor (and/or are normalized to && ||)
	OpCoalesce    Op = "coalesce"
	OpTernary     Op = "ternary" // Kids: cond, then (nil for ?:), else
	OpCast        Op = "cast"    // Text: int float string bool array object
	OpIsset       Op = "isset"   // Kids evaluated in quiet mode
	OpEmpty       Op = "empty"
	OpCall        Op = "call"        // Name is the function; Args
	OpCallValue   Op = "call_value"  // Kids[0] callee expression; Args
	OpMethodCall  Op = "method_call" // Kids: object, name expression; Text literal name; Args
	OpNullsafeMC  Op = "nullsafe_call"
	OpStaticCall  Op = "static_call" // Name or Kids[0] class; Text or Kids[1] method; Args
	OpCallable    Op = "callable"    // first-class callable syntax; Kids[0] is the call without arguments
	OpNew         Op = "new"         // Name or Kids[0] class, or Class for an anonymous class; Args
	OpClone       Op = "clone"
	OpClosure     Op = "closure" // Func
	OpArrowFn     Op = "arrow_fn"
	OpInstanceof  Op = "instanceof" // Kids: value, class expression; or Name
	OpInclude     Op = "include"    // Text: include, include_once, require, require_once
	OpEval        Op = "eval"
	OpExit        Op = "exit"
	OpPrint       Op = "print"
	OpShell       Op = "shell" // backtick execution
	OpMatch       Op = "match" // Kids: subject, OpArm...
	OpArm         Op = "arm"   // Kids: body, conditions...; no conditions for default
	OpThrow       Op = "throw"
	OpYield       Op = "yield" // Kids: key (may be nil), value (may be nil)
	OpYieldFrom   Op = "yield_from"
)

// Statement operations.
const (
	OpEcho      Op = "echo"
	OpExpr      Op = "expr"
	OpVoid      Op = "void" // (void) expr
	OpHTML      Op = "html" // inline HTML; Value is its bytes
	OpIf        Op = "if"   // Kids: cond, OpBlock, then repeated cond/OpBlock pairs, optional trailing else OpBlock
	OpWhile     Op = "while"
	OpDo        Op = "do"
	OpFor       Op = "for"     // Kids: init, cond, step (OpBlock lists of expressions), body
	OpForeach   Op = "foreach" // Kids: subject, key (may be nil), value target, body; FlagRef when by reference
	OpSwitch    Op = "switch"  // Kids: subject, OpCase...
	OpCase      Op = "case"    // Kids: label (nil for default), body OpBlock
	OpBreak     Op = "break"   // Value is the level
	OpContinue  Op = "continue"
	OpReturn    Op = "return"
	OpGlobal    Op = "global" // Var bound to the global of the same name
	OpStatic    Op = "static" // Var bound to the function's static slot; Kids[0] initializer
	OpUnset     Op = "unset"
	OpTry       Op = "try" // Kids: body, OpCatch..., optional OpFinally
	OpCatch     Op = "catch"
	OpFinally   Op = "finally"
	OpGoto      Op = "goto" // Text label
	OpLabel     Op = "label"
	OpDeclare   Op = "declare"   // Kids: one OpDirective per directive, then an optional body OpBlock
	OpDirective Op = "directive" // Text directive name in lower case; Value its literal
	OpBlock     Op = "block"
	OpFuncDecl  Op = "func_decl"  // Func, declared when reached (conditional declaration)
	OpClass     Op = "class_decl" // Class, declared when reached
	OpConstDef  Op = "const_decl" // Name, Kids[0] value
	OpHalt      Op = "halt_compiler"
)

// Node is one HIR operation. Which fields are set depends on Op.
type Node struct {
	Op         Op
	Line       int
	Text       string
	Value      *Literal
	Name       *Name
	Var        *Var
	Func       *Func
	Class      *Class
	Attributes []Attribute
	Args       []*Arg
	Kids       []*Node
	Flags      NodeFlag
	Effects    Effect // effects of this node, including its operands
}

// NodeFlag records how a node is used.
type NodeFlag uint8

const (
	FlagWrite NodeFlag = 1 << iota // the node is written
	FlagRef                        // the node is bound by reference
	FlagQuiet                      // read in isset/empty/?? mode: no undefined warnings
	FlagUnset                      // the node is unset
	// FlagCoerce marks a return converted to the function's declared type,
	// which may call __toString, warn or throw a TypeError.
	FlagCoerce
)

// Arg is one call argument.
type Arg struct {
	Value *Node
	Name  string // named argument, without $
	// Spread marks ...$args.
	Spread bool
	// MayRef marks an argument that is passed by reference when the callee
	// declares the parameter by reference. The callee is not known at
	// compile time, so the variable is treated as possibly aliased.
	MayRef bool
}

// LiteralKind is the PHP type of a literal.
type LiteralKind uint8

const (
	LitNull LiteralKind = iota
	LitBool
	LitInt
	LitFloat
	LitString
)

// Literal is a PHP constant value known at compile time.
type Literal struct {
	Kind  LiteralKind
	Bool  bool
	Int   int64
	Float float64
	Str   string // bytes, not necessarily UTF-8
	// Uninterned marks a string PHP holds in a new, refcounted
	// zend_string rather than an interned one. It matters for strings of
	// at most one byte, which PHP otherwise interns: the scanner allocates
	// a string literal whose source spelling is longer than one byte (one
	// written with an escape sequence) and any non-empty nowdoc, and
	// folding keeps or creates such strings as PHP's operators do
	// (internal/vm defaultMayBeInterned relies on it).
	Uninterned bool
}

// NameKind distinguishes the three PHP symbol tables a name can address.
type NameKind uint8

const (
	ClassName NameKind = iota
	FunctionName
	ConstantName
)

// Name is a resolved PHP symbol reference.
type Name struct {
	Kind NameKind
	// Resolved is the fully qualified name without a leading backslash.
	Resolved string
	// Fallback is the global name PHP tries at run time when Resolved is
	// not defined: unqualified function and constant names in a namespace.
	Fallback string
	// Special is "self", "parent" or "static" for a class reference that is
	// resolved against the calling scope.
	Special string
}

// VarFlag describes how a function uses one of its variables.
type VarFlag uint16

const (
	VarParam       VarFlag = 1 << iota
	VarGlobal              // bound by a global statement
	VarStatic              // bound by a static statement
	VarCaptured            // captured by value from the defining scope
	VarCapturedRef         // captured by reference (use (&$x))
	VarRef                 // bound by reference somewhere: =&, &$x items, foreach by reference
	VarMayRef              // passed where the callee may take it by reference
	VarWritten             // assigned somewhere
	VarRead                // read somewhere
)

// Aliased reports whether the variable may share its slot with another
// location. Such a variable must not be replaced by a register value.
func (f VarFlag) Aliased() bool {
	return f&(VarGlobal|VarStatic|VarCapturedRef|VarRef|VarMayRef) != 0
}

// Var is a compiled variable of one function.
type Var struct {
	Name  string // without $
	Flags VarFlag
}

// FuncKind distinguishes PHP function forms.
type FuncKind uint8

const (
	FuncMain FuncKind = iota // file top-level code
	FuncNamed
	FuncMethod
	FuncClosure
	FuncArrow
	FuncHook
)

// Param is a declared parameter.
type Param struct {
	Name     string // without $
	Line     int
	Type     *Type
	Default  *Node
	ByRef    bool
	Variadic bool
	// Promoted holds the modifiers of a promoted constructor parameter.
	Promoted   []string
	Attributes []Attribute
	Var        *Var
}

// Capture is a variable a closure takes from its defining scope.
type Capture struct {
	Name  string
	ByRef bool
	// Implicit marks an arrow function capture; PHP binds it only when the
	// variable exists in the defining scope.
	Implicit bool
	Line     int
}

// Func is a function body: file code, a function, method, closure or hook.
type Func struct {
	Kind FuncKind
	// Name is the fully qualified name for functions, the method name for
	// methods and the "{closure:...}" name PHP reports for closures.
	Name      string
	Line      int
	Params    []*Param
	Return    *Type
	ByRef     bool // returns by reference
	HasBody   bool
	Static    bool // static method or static closure
	Generator bool
	Variadic  bool // the last parameter is variadic
	// Strict is the strict_types mode of the defining file. It governs the
	// calls this function makes and its own return-value coercion.
	Strict bool
	// Hoisted marks a top-level named function, declared before the file runs.
	Hoisted    bool
	Modifiers  []string
	Attributes []Attribute
	Captures   []*Capture
	// Vars lists the function's variables in order of first appearance.
	Vars []*Var
	// DynamicScope marks code that may read or write local variables by a
	// run-time name: variable variables, include, eval, extract, compact,
	// get_defined_vars and parse_str-like functions.
	DynamicScope bool
	// UsesArgs marks code that inspects its arguments at run time
	// (func_get_args and related functions).
	UsesArgs bool
	Body     []*Node
	// Class is the class whose scope the code runs in, if known at compile
	// time. Closures inherit the scope of their defining method.
	Class  *Class
	Parent *Func // defining function of a closure or arrow function

	vars     map[string]*Var
	statics  map[string]bool
	property string // property of a hook
}

// Var returns the variable named name, or nil.
func (f *Func) Var(name string) *Var { return f.vars[name] }

// Type is a declared type in PHP's canonical spelling.
type Type struct {
	String string // as ReflectionType::__toString prints it
	// Implicit marks a type made nullable by a null default value.
	Implicit bool
	zend     string // as zend_type_to_string prints it in compile errors
}

// ClassKind distinguishes class-like declarations.
type ClassKind uint8

const (
	KindClass ClassKind = iota
	KindInterface
	KindTrait
	KindEnum
)

// Class is a class-like declaration. Inheritance, linking and member rules
// belong to the object model; HIR records declarations and their code.
type Class struct {
	Kind       ClassKind
	Name       string // fully qualified; "class@anonymous" for anonymous classes
	Line       int
	Modifiers  []string
	Parent     *Name
	Interfaces []*Name
	Traits     []*Name
	// TraitRules are the adaptations of the trait uses, in source order.
	TraitRules       []*TraitRule
	Backing          *Type
	Consts           []*ClassConst
	Props            []*Prop
	Cases            []*ClassConst
	Methods          []*Func
	Anonymous        bool
	Attributes       []Attribute
	AttributeTargets int
	IsAttribute      bool
}

// Attribute retains source metadata without keeping compiled argument graphs.
// Empty argument names represent positional arguments.
type Attribute struct {
	Name      *Name
	Line      int
	Arguments []string
}

// TraitRule is one trait adaptation: "T::m insteadof U, V" or
// "[T::]m as [modifier] [alias]". The object model applies and checks them.
type TraitRule struct {
	Line int
	// Trait is the trait named before ::, or nil for a bare method.
	Trait  *Name
	Method string
	// Insteadof lists the excluded traits of an insteadof rule; it is empty
	// for an alias rule.
	Insteadof []*Name
	Modifier  string // visibility of an alias rule, lower case, or ""
	Alias     string // new name of an alias rule, or ""
}

// ClassConst is a class constant or enum case.
type ClassConst struct {
	Name       string
	Line       int
	Modifiers  []string
	Type       *Type
	Value      *Node
	Attributes []Attribute
	Class      *Class
}

// Prop is a declared property.
type Prop struct {
	Name       string
	Line       int
	Modifiers  []string
	Type       *Type
	Default    *Node
	Hooks      []*Func
	HasHooks   bool
	Backed     bool
	Promoted   bool
	Attributes []Attribute
	Class      *Class
}

// Diagnostic is a compile-time warning or deprecation PHP reports without
// stopping compilation.
type Diagnostic struct {
	Line    int
	Level   string // "Deprecated", "Warning"
	Message string
}

// Unit is one compiled file.
type Unit struct {
	File    string
	Profile profile.Profile
	Strict  bool
	Main    *Func
	// Functions lists every named function in source order, including
	// conditional ones; Hoisted tells which are declared before execution.
	Functions   []*Func
	Classes     []*Class
	Diagnostics []Diagnostic
}

// CompileError is a fatal error PHP raises while compiling the file.
type CompileError struct {
	Line    int
	Message string
}

func (e *CompileError) Error() string { return e.Message }
