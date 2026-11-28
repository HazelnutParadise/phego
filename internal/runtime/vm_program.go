package runtime

import (
	"errors"
	"fmt"
	"math"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/phpname"
)

// W-17 stage 1: REQ-001, REQ-005, REQ-011, REQ-015, REQ-016, REQ-025, REQ-026;
// T-009, T-010, T-029..T-032, T-051, T-052.
//
// Program is a verified bytecode program prepared for execution. It is
// immutable and safe to share between goroutines; all mutable state of a run
// lives in its execution.
type Program struct {
	consts []vmConst
	funcs  []vmFunction
	byName map[string]int // lower-case function name to index
	entry  int
	width  int
	// mainLocal maps a variable name of the entry function to its local:
	// the global variables a global statement binds.
	mainLocal map[string]int
	// classes is the decoded class table (format 1.7) and hoisted the
	// indices of the classes that exist before {main} starts, in order.
	// Both are immutable: a run builds its own classes from them.
	classes []vmClassDecl
	hoisted []int
	// Builtin reports a lower-case name of a function PHP declares itself
	// and Phego does not provide: a callable that names one is refused as
	// unsupported, not reported as an undefined function. Set once, before
	// the first run, by the engine that built the Program; nil knows none.
	Builtin func(lower string) bool
	// memberFunc marks the functions that are a class member's body or
	// initializer.
	memberFunc []bool
	// trampoline is the index of a function that no code calls: the
	// callee of a call __call or __callStatic will receive, which takes
	// any arguments by value.
	trampoline int
	// fnTrait marks the functions that are a trait's methods: each class
	// that uses the trait has its own copy of their static variables.
	fnTrait []bool
	// ncaches is the number of inline caches the program's instructions
	// name; the caches themselves belong to each run.
	ncaches int
}

// vmInstr is a bytecode instruction whose register operands are renumbered
// densely per register class: Value registers index the frame's value window
// and Location registers its location window. kill/nkill name the Value
// registers that may hold an array and are dead after this instruction (see
// vmLiveness).
type vmInstr struct {
	op    bytecode.Opcode
	aux   uint16
	nkill uint16
	a, b  uint32
	c     uint32
	kill  uint32
	// cache is the index plus one of the instruction's inline cache in the
	// run's cache table, or zero (format 1.7 member instructions).
	cache uint32
}

type vmFunction struct {
	name       string
	index      int
	params     int
	nvals      int // Value registers
	nlocs      int // Location registers
	locals     int
	code       []vmInstr
	lines      []int32
	files      []string // per instruction; "" for a synthetic position
	kills      []uint32
	localNames []string
	paramLocal []int32 // local of parameter i, -1 if the prologue does not store it
	paramPC    []int32 // pc of the prologue Store that receives parameter i, or -1
	// paramNames and paramByRef come from the parameter table (format
	// 1.6): nil when the function records nothing, else one entry per
	// parameter register. variadic says a variadic parameter follows them,
	// variadicByRef that it takes references, anyByRef that some parameter
	// does.
	paramNames    []string
	paramByRef    []bool
	variadic      bool
	variadicByRef bool
	anyByRef      bool
	// required is how many arguments a call must pass: every parameter up
	// to the last one no ParamDefault names, as PHP treats an optional
	// parameter before a required one as required.
	required int
	// nelems is the size of a frame's element window, and elemOf maps each
	// Location register to its slot there, -1 when no FetchDim defines it.
	nelems int
	elemOf []int32
	// Format 1.7: the declared types of the parameters ("" for none; one
	// more entry for a variadic parameter), the declared return type, and
	// whether the code was compiled under strict_types. typed says some
	// parameter has a type, which the call checks on entry.
	paramTypes []string
	returnType string
	strict     bool
	typed      bool
	// Format 1.8. closure marks a closure's body, which only a Closure
	// object enters; noThis a static closure or method or a plain
	// function; usesThis a closure body that uses $this. captures is the
	// closure's use list.
	closure  bool
	noThis   bool
	usesThis bool
	captures []vmCapture
	// Format 1.9 (stage 7C). handlers is the exception table; handlerAt,
	// finallyAt and bodyAt index it per pc (nil without handlers).
	// liveAt holds, for each handler target and each Leave target, the
	// registers that are live there, in ascending order, while the
	// function is prepared; the VM reads them from vmHandler.live and,
	// for the Leave whose operand b is an index plus one, from leaveLive.
	// liveKnown says the liveness analysis ran to its end.
	handlers  []vmHandler
	handlerAt []int32
	finallyAt []int32
	bodyAt    []int32
	liveAt    map[int32][]uint32
	leaveLive [][]uint32
	liveKnown bool
	// Format 1.10 (stage 8): the unwind table, with dense registers: the
	// calls in order of start, each with the call it is nested in, and the
	// returns that free loop variables (UnwindSplit). ranges are PHP's live
	// ranges of the composite registers, worked out the first time an
	// exception or exit unwinds the function (vmFunction.rangesFor).
	calls  []vmUnwindCall
	splits []bytecode.Unwind
	ranges *vmRangeCache
	// unsupported names a construct of the function that stage 1 cannot run;
	// entering the function ends the run with an UnsupportedError.
	unsupported string
}

// vmCapture is one variable of a closure's use list: the local that
// receives it when a call enters the closure.
type vmCapture struct {
	local           int32
	byRef, implicit bool
}

// opFusedCompareBranch is an internal opcode outside the format's range. It
// runs a comparison Binary and the Branch after it as one dispatch, keeping
// both steps' charges, polls and locations. NewProgram never emits it; only
// the superinstruction benchmark candidate (vm_bench_test.go) rewrites a
// test program's code to use it.
const opFusedCompareBranch bytecode.Opcode = 0xff00

// UnsupportedError is a construct that compiled but that the stage-1
// interpreter cannot execute. It ends the run as RunEnd Unsupported; it is
// never a PHP error. Error() is a single line.
type UnsupportedError struct {
	Feature  string
	Function string
	PC       int
	File     string
	Line     int
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("unsupported in the stage-1 VM: %s at %s:%d (function %s, pc %d)", e.Feature, vmFileName(e.File), e.Line, e.Function, e.PC)
}

// InternalError is an engine fault located at an instruction, such as a read
// of a location register that no Local defined. It never surfaces as PHP.
type InternalError struct {
	Detail   string
	Function string
	PC       int
	File     string
	Line     int
}

func (e *InternalError) Error() string {
	return fmt.Sprintf("internal VM error: %s at %s:%d (function %s, pc %d)", e.Detail, vmFileName(e.File), e.Line, e.Function, e.PC)
}

func vmFileName(f string) string {
	if f == "" {
		return "Unknown"
	}
	return f
}

// operand roles used to renumber and bounds-check instructions.
const (
	roleNone = iota
	roleValue
	roleLocation
	roleAny // Move/Drop/Push: either class, by the register's declared kind
	roleConst
	roleFunc
	roleLocal
	roleTarget
	roleCount
	roleNameConst // Push B: 0 or a string constant index plus one
	roleClass     // an index into the class table
)

// role17 maps a format 1.7 operand role to the decoder's.
func role17(r bytecode.Role) int {
	switch r {
	case bytecode.RoleUse, bytecode.RoleDef, bytecode.RoleUseDef:
		return roleValue
	case bytecode.RoleLocDef, bytecode.RoleLocUse:
		return roleLocation
	case bytecode.RoleString:
		return roleConst
	case bytecode.RoleClass:
		return roleClass
	case bytecode.RoleTarget:
		return roleTarget
	case bytecode.RoleFunc:
		return roleFunc
	}
	return roleNone
}

func operandRoles(in bytecode.Instruction) (a, b, c int) {
	switch in.Op {
	case bytecode.OpConst:
		return roleValue, roleConst, roleNone
	case bytecode.OpMove:
		return roleAny, roleAny, roleNone
	case bytecode.OpLocal:
		return roleLocation, roleLocal, roleNone
	case bytecode.OpLoad, bytecode.OpIsset, bytecode.OpIncDec:
		return roleValue, roleLocation, roleNone
	case bytecode.OpStore:
		return roleLocation, roleValue, roleNone
	case bytecode.OpArrayUnset:
		if bytecode.ArrayWriteFlags(in.Aux)&bytecode.KeyLocation != 0 {
			return roleLocation, roleLocation, roleNone
		}
		return roleLocation, roleValue, roleNone
	case bytecode.OpArrayAppend:
		if bytecode.ArrayWriteFlags(in.Aux)&bytecode.ValueLocation != 0 {
			return roleLocation, roleLocation, roleNone
		}
		return roleLocation, roleValue, roleNone
	case bytecode.OpBinary, bytecode.OpArrayGet, bytecode.OpArrayIsset:
		return roleValue, roleValue, roleValue
	case bytecode.OpJump, bytecode.OpLeave:
		if in.Op == bytecode.OpLeave && in.Aux == bytecode.LeaveCarry {
			// Format 1.10: C is the value the Leave carries.
			return roleTarget, roleNone, roleValue
		}
		return roleTarget, roleNone, roleNone
	case bytecode.OpBranch:
		return roleValue, roleTarget, roleTarget
	case bytecode.OpPush:
		return roleAny, roleNameConst, roleNone
	case bytecode.OpBindGlobal:
		return roleLocation, roleConst, roleNone
	case bytecode.OpStaticInit:
		return roleLocation, roleTarget, roleNone
	case bytecode.OpBindStatic:
		return roleLocation, roleValue, roleNone
	case bytecode.OpRecvVariadic:
		return roleValue, roleNone, roleNone
	case bytecode.OpInitCall:
		return roleNone, roleFunc, roleNone
	case bytecode.OpSendUnpack, bytecode.OpCallPending:
		return roleAny, roleNone, roleNone
	case bytecode.OpCall:
		return roleAny, roleFunc, roleCount
	case bytecode.OpCallValue:
		return roleValue, roleValue, roleCount
	case bytecode.OpHostCall:
		return roleValue, roleConst, roleCount
	case bytecode.OpReturn, bytecode.OpThrow, bytecode.OpOutput, bytecode.OpNewArray:
		return roleValue, roleNone, roleNone
	case bytecode.OpReturnRef, bytecode.OpUnset:
		return roleLocation, roleNone, roleNone
	case bytecode.OpInclude, bytecode.OpEval, bytecode.OpShell, bytecode.OpUnary, bytecode.OpCast:
		return roleValue, roleValue, roleNone
	case bytecode.OpDrop:
		return roleAny, roleNone, roleNone
	case bytecode.OpArraySet:
		b, c = roleValue, roleValue
		if bytecode.ArrayWriteFlags(in.Aux)&bytecode.KeyLocation != 0 {
			b = roleLocation
		}
		if bytecode.ArrayWriteFlags(in.Aux)&bytecode.ValueLocation != 0 {
			c = roleLocation
		}
		return roleLocation, b, c
	case bytecode.OpArrayAdd:
		// A is read and updated in place, never redefined, so liveness keeps
		// the array alive from NewArray to its last use.
		switch flags := bytecode.ArrayAddFlags(in.Aux); {
		case flags&bytecode.AddKeyLocation != 0:
			return roleValue, roleValue, roleLocation
		case flags&bytecode.AddKey != 0:
			return roleValue, roleValue, roleValue
		}
		return roleValue, roleValue, roleNone
	case bytecode.OpExit:
		if in.Aux == 1 {
			return roleValue, roleNone, roleNone
		}
	case bytecode.OpFetchDim:
		flags := bytecode.FetchFlags(in.Aux)
		container := roleLocation
		if flags&bytecode.FetchContainerValue != 0 {
			container = roleValue // a reference in a Value register (format 1.4)
		}
		switch {
		case flags&bytecode.FetchAppend != 0:
			return roleLocation, container, roleNone
		case flags&bytecode.FetchKeyLocation != 0:
			return roleLocation, container, roleLocation
		}
		return roleLocation, container, roleValue
	case bytecode.OpIterInit:
		if in.Aux&uint16(bytecode.IterByRef) != 0 {
			return roleValue, roleAny, roleTarget
		}
		return roleValue, roleValue, roleTarget
	case bytecode.OpIterNext:
		if bytecode.IterFlags(in.Aux)&bytecode.IterLocation != 0 {
			return roleValue, roleLocation, roleTarget
		}
		return roleValue, roleValue, roleTarget
	case bytecode.OpBind:
		if in.Aux == uint16(bytecode.BindValue) {
			return roleLocation, roleValue, roleNone
		}
		return roleLocation, roleLocation, roleNone
	case bytecode.OpMakeRef:
		return roleValue, roleLocation, roleNone
	case bytecode.OpIterKey:
		if bytecode.IterFlags(in.Aux) == bytecode.IterLocation {
			return roleLocation, roleValue, roleNone
		}
		return roleValue, roleValue, roleNone
	case bytecode.OpParamDefault:
		return roleValue, roleTarget, roleNone
	case bytecode.OpFetchConst:
		return roleValue, roleConst, roleConst
	case bytecode.OpDeclareConst:
		return roleValue, roleConst, roleNone
	case bytecode.OpBeginSilence, bytecode.OpEndSilence, bytecode.OpMatchError:
		return roleValue, roleNone, roleNone
	default:
		if r, ok := bytecode.Roles17(in); ok {
			return role17(r[0]), role17(r[1]), role17(r[2])
		}
	}
	return roleNone, roleNone, roleNone
}

// NewProgram decodes p once into the interpreter's private form. Every
// register, constant, local, function and jump index is checked here again,
// including in code the verifier only shape-checked, so the interpreter's
// indexing never depends on dataflow facts.
func NewProgram(p *bytecode.Program) (*Program, error) {
	if p == nil {
		return nil, errors.New("NewProgram: nil bytecode program")
	}
	width := p.Identity().Profile.IntWidth
	if width != 32 && width != 64 {
		return nil, fmt.Errorf("NewProgram: invalid PHP integer width %d", width)
	}
	consts := p.Constants(nil)
	pool, err := decodeConstants(consts, width)
	if err != nil {
		return nil, err
	}
	n := p.FunctionCount()
	prog := &Program{consts: pool, funcs: make([]vmFunction, n), byName: make(map[string]int, n), entry: int(p.Entry()), width: width, memberFunc: make([]bool, n)}
	if prog.entry < 0 || prog.entry >= n {
		return nil, fmt.Errorf("NewProgram: entry function %d out of range", prog.entry)
	}
	sources := make([]string, p.SourceCount())
	for i := range sources {
		s, _ := p.Source(uint32(i))
		sources[i] = s.Name
	}
	for i := 0; i < n; i++ {
		fv, _ := p.Function(uint32(i))
		code, _ := p.Instructions(uint32(i), nil)
		f, err := decodeFunction(fv, code, i, pool, sources, p)
		if err != nil {
			return nil, err
		}
		// A member access by a constant name on an object in a register
		// gets an inline cache.
		for pc := range f.code {
			in := &f.code[pc]
			switch in.op {
			case bytecode.OpPropGet, bytecode.OpPropSet, bytecode.OpInitMethod:
				if in.aux&(bytecode.MemberNameReg|bytecode.MemberObjLocation) == 0 && in.aux != bytecode.InitCallable {
					prog.ncaches++
					in.cache = uint32(prog.ncaches)
				}
			}
		}
		prog.funcs[i] = f
		if i == prog.entry {
			prog.mainLocal = make(map[string]int, len(f.localNames))
			for l, name := range f.localNames {
				if _, dup := prog.mainLocal[name]; !dup && name != "" {
					prog.mainLocal[name] = l
				}
			}
		}
		if i != prog.entry && !f.closure {
			key := phpname.Lower(f.name)
			if _, dup := prog.byName[key]; !dup {
				prog.byName[key] = i
			}
		}
	}
	if err := prog.decodeClasses(p); err != nil {
		return nil, err
	}
	// A method or an initializer is no function: a callable string that
	// spells its bare name must not reach it.
	for name, i := range prog.byName {
		if prog.memberFunc[i] {
			delete(prog.byName, name)
		}
	}
	prog.trampoline = len(prog.funcs)
	prog.funcs = append(prog.funcs, vmFunction{name: "__call", index: prog.trampoline, variadic: true, paramNames: []string{}, paramByRef: []bool{}, unsupported: "entering the magic call trampoline"})
	// Call operands name callees; their return class must match the destination.
	for i := range prog.funcs {
		f := &prog.funcs[i]
		for pc, in := range f.code {
			if in.op == bytecode.OpCall && int(in.b) >= n {
				return nil, fmt.Errorf("NewProgram: function %s pc %d calls function %d out of range", f.name, pc, in.b)
			}
		}
	}
	return prog, nil
}

// vmConst is the decoded constant pool. Strings are immutable Go strings, so
// sharing the decoded values between executions is safe.
type vmConst = value

func decodeConstants(cs []bytecode.Constant, width int) ([]vmConst, error) {
	out := make([]vmConst, len(cs))
	for i, c := range cs {
		switch c.Kind {
		case bytecode.Null:
			out[i] = nullValue()
		case bytecode.Bool:
			out[i] = boolValue(c.Bits != 0)
		case bytecode.Int:
			v, err := intValue(int64(c.Bits), width)
			if err != nil {
				return nil, fmt.Errorf("NewProgram: constant %d: %w", i, err)
			}
			out[i] = v
		case bytecode.Float:
			out[i] = floatValue(math.Float64frombits(c.Bits))
		case bytecode.String:
			out[i] = stringValue(c.Bytes)
		default:
			return nil, fmt.Errorf("NewProgram: constant %d has unknown kind %d", i, c.Kind)
		}
	}
	return out, nil
}

func decodeFunction(fv bytecode.FunctionView, code []bytecode.Instruction, index int, pool []vmConst, sources []string, p *bytecode.Program) (vmFunction, error) {
	nregs, locals, _ := fv.Frame()
	f := vmFunction{name: fv.Name(), index: index, params: int(fv.Params()), locals: int(locals)}
	fail := func(pc int, format string, args ...any) error {
		return fmt.Errorf("NewProgram: function %s pc %d: %s", f.name, pc, fmt.Sprintf(format, args...))
	}
	if fv.ReturnKind() != bytecode.Value {
		f.unsupported = "functions returning by reference"
	}
	// Dense renumbering per register class.
	dense := make([]uint32, nregs)
	kinds := make([]bytecode.RegisterKind, nregs)
	for r := uint32(0); r < nregs; r++ {
		k, _ := fv.RegisterKind(r)
		kinds[r] = k
		switch k {
		case bytecode.Value:
			dense[r] = uint32(f.nvals)
			f.nvals++
		case bytecode.Location:
			dense[r] = uint32(f.nlocs)
			f.nlocs++
		default:
			return f, fail(-1, "register %d has unknown kind %d", r, k)
		}
	}
	if f.params > int(nregs) {
		return f, fail(-1, "%d parameters but %d registers", f.params, nregs)
	}
	for r := 0; r < f.params; r++ {
		if kinds[r] != bytecode.Value {
			return f, fail(-1, "parameter register %d is not a Value", r)
		}
	}
	f.localNames = make([]string, f.locals)
	for i := range f.localNames {
		f.localNames[i], _ = fv.LocalName(uint32(i))
	}
	if n := fv.ParameterCount(); n > 0 {
		f.paramNames, f.paramByRef = make([]string, f.params), make([]bool, f.params)
		for i := 0; i < n; i++ {
			name, flags, _ := fv.Parameter(uint32(i))
			byRef := flags&bytecode.ParamByRef != 0
			f.anyByRef = f.anyByRef || byRef
			if i < f.params {
				f.paramNames[i], f.paramByRef[i] = name, byRef
			} else {
				f.variadic, f.variadicByRef = true, byRef
			}
		}
	}
	f.returnType = fv.ReturnType()
	f.strict = fv.Flags()&bytecode.FuncStrict != 0
	f.closure = fv.Flags()&bytecode.FuncClosure != 0
	f.noThis = fv.Flags()&bytecode.FuncNoThis != 0
	f.usesThis = fv.Flags()&bytecode.FuncUsesThis != 0
	for i, n := 0, fv.CaptureCount(); i < n; i++ {
		c, _ := fv.CaptureAt(i)
		if int(c.Local) >= f.locals {
			return f, fail(-1, "capture %d names local %d of %d", i, c.Local, f.locals)
		}
		f.captures = append(f.captures, vmCapture{local: int32(c.Local), byRef: c.Flags&bytecode.CaptureByRef != 0, implicit: c.Flags&bytecode.CaptureImplicit != 0})
	}
	if n := fv.ParameterCount(); n > 0 {
		f.paramTypes = make([]string, n)
		for i := range f.paramTypes {
			f.paramTypes[i] = fv.ParameterType(uint32(i))
			f.typed = f.typed || f.paramTypes[i] != ""
		}
	}
	f.code = make([]vmInstr, len(code))
	f.lines = make([]int32, len(code))
	f.files = make([]string, len(code))
	f.required = f.params
	optional := make([]bool, f.params)
	f.elemOf = make([]int32, f.nlocs)
	for i := range f.elemOf {
		f.elemOf[i] = -1
	}
	for pc, in := range code {
		pos, _ := fv.Position(uint32(pc))
		if pos.Source > 0 && int(pos.Source) <= len(sources) {
			f.files[pc] = sources[pos.Source-1]
			f.lines[pc] = int32(min(pos.Line, math.MaxInt32))
		}
		out := vmInstr{op: in.Op, aux: in.Aux}
		ra, rb, rc := operandRoles(in)
		operands := [3]uint32{in.A, in.B, in.C}
		roles := [3]int{ra, rb, rc}
		var decoded [3]uint32
		for i, role := range roles {
			v := operands[i]
			switch role {
			case roleNone, roleCount:
				decoded[i] = v
			case roleValue, roleLocation, roleAny:
				if v >= nregs {
					return f, fail(pc, "register %d out of range", v)
				}
				want := bytecode.Value
				if role == roleLocation {
					want = bytecode.Location
				}
				if role != roleAny && kinds[v] != want {
					return f, fail(pc, "register %d has the wrong class", v)
				}
				decoded[i] = dense[v]
			case roleConst:
				if int(v) >= len(pool) {
					return f, fail(pc, "constant %d out of range", v)
				}
				decoded[i] = v
			case roleNameConst:
				if v != 0 && int(v-1) >= len(pool) {
					return f, fail(pc, "argument name constant %d out of range", v-1)
				}
				decoded[i] = v
			case roleFunc:
				if int(v) >= p.FunctionCount() {
					return f, fail(pc, "function %d out of range", v)
				}
				decoded[i] = v
			case roleClass:
				if int(v) >= p.ClassCount() {
					return f, fail(pc, "class %d out of range", v)
				}
				decoded[i] = v
			case roleLocal:
				if v >= locals {
					return f, fail(pc, "local %d out of range", v)
				}
				decoded[i] = v
			case roleTarget:
				if int(v) >= len(code) {
					return f, fail(pc, "jump target %d out of range", v)
				}
				decoded[i] = v
			}
		}
		// Move and the two-class operands keep their register class in aux bits
		// the interpreter needs: Move copies a location when the source is one.
		switch in.Op {
		case bytecode.OpMove:
			if kinds[in.A] != kinds[in.B] {
				return f, fail(pc, "move between register classes")
			}
			if kinds[in.A] == bytecode.Location {
				out.aux = 1
			}
		case bytecode.OpDrop:
			if kinds[in.A] == bytecode.Location {
				out.aux = 1
			}
		case bytecode.OpPush:
			out.aux = 0
			if kinds[in.A] == bytecode.Location {
				out.aux = vmPushRef
				switch bytecode.PushFlags(in.Aux) {
				case bytecode.PushLoad:
					out.aux = vmPushLoad
				case bytecode.PushAuto:
					out.aux = vmPushAuto
				}
			} else {
				switch bytecode.PushFlags(in.Aux) {
				case bytecode.PushTemp:
					out.aux = vmPushTemp
				case bytecode.PushResult:
					out.aux = vmPushResult
				case bytecode.PushElement:
					out.aux = vmPushElement
				}
			}
		case bytecode.OpSendUnpack:
			if kinds[in.A] == bytecode.Location {
				out.aux = vmPushRef // a plain variable
			}
		case bytecode.OpCallPending:
			if kinds[in.A] != bytecode.Value {
				out.aux = 1 // a Location result: a by-reference callee
			}
		case bytecode.OpIterInit:
			// A by-reference subject that is a variable or an element.
			if in.Aux == uint16(bytecode.IterByRef) && kinds[in.B] == bytecode.Location {
				out.aux |= vmIterSubjectLocation
			}
		case bytecode.OpCall:
			if kinds[in.A] != bytecode.Value {
				out.aux = 1 // a Location result: a by-reference callee
			}
		case bytecode.OpArraySet:
			// The result register is renumbered like an operand.
			if r, ok := bytecode.ArraySetResult(in.Aux); ok {
				if r >= nregs || kinds[r] != bytecode.Value {
					return f, fail(pc, "result register %d is not a Value register", r)
				}
				aux, _ := bytecode.ArraySetAux(bytecode.ArrayWriteFlags(in.Aux)&(bytecode.KeyLocation|bytecode.ValueLocation), dense[r])
				out.aux = aux
			}
		case bytecode.OpFetchDim, bytecode.OpFetchProp, bytecode.OpFetchStatic:
			// Each destination gets a slot in the frame's element window.
			if f.elemOf[decoded[0]] < 0 {
				f.elemOf[decoded[0]] = int32(f.nelems)
				f.nelems++
			}
		case bytecode.OpParamDefault:
			if in.A >= uint32(f.params) {
				return f, fail(pc, "register %d is not a parameter", in.A)
			}
			optional[in.A] = true
		}
		out.a, out.b, out.c = decoded[0], decoded[1], decoded[2]
		f.code[pc] = out
	}
	for f.required > 0 && optional[f.required-1] {
		f.required--
	}
	// The Store a ParamDefault skips to receives that parameter; its
	// decoded c (zero in the image) is the parameter plus one, which tells
	// the Store to cache an evaluated default (vmState.cacheDefault).
	for _, in := range code {
		if in.Op == bytecode.OpParamDefault && uint64(in.B) < uint64(len(code)) {
			if st := code[in.B]; (st.Op == bytecode.OpStore || st.Op == bytecode.OpBind && st.Aux == uint16(bytecode.BindValue)) && st.B == in.A {
				f.code[in.B].c = in.A + 1
			}
		}
	}
	f.paramLocal, f.paramPC = parameterPrologue(f, code)
	for i, c := range f.captures {
		for _, l := range f.paramLocal {
			if l == c.local {
				return f, fail(-1, "capture %d names the local of a parameter", i)
			}
		}
	}
	// The instruction that receives a typed parameter checks it: its
	// decoded c is the parameter plus one, as for a parameter with a
	// default.
	for i, pc := range f.paramPC {
		if pc >= 0 && i < len(f.paramTypes) && f.paramTypes[i] != "" {
			f.code[pc].c = uint32(i) + 1
		}
	}
	decodeHandlers(&f, fv)
	f.ranges = &vmRangeCache{}
	for i := 0; i < fv.UnwindCount(); i++ {
		u, _ := fv.UnwindAt(i)
		if u.Kind != bytecode.UnwindPendingCall {
			if u.Reg >= nregs || kinds[u.Reg] != bytecode.Value {
				return f, fail(-1, "unwind entry %d names register %d, which is not a Value", i, u.Reg)
			}
			u.Reg = dense[u.Reg]
		}
		f.addUnwind(u)
	}
	for i := range f.handlers {
		h, _ := fv.Handler(uint32(i))
		if f.handlers[i].catch {
			if h.Register >= nregs || kinds[h.Register] != bytecode.Value {
				return f, fail(-1, "handler %d names register %d, which is not a Value", i, h.Register)
			}
			f.handlers[i].reg = dense[h.Register]
		}
	}
	if f.nvals > 0 {
		vmLiveness(&f)
	}
	// Hand the facts to where the VM reads them without a lookup.
	for i := range f.handlers {
		h := &f.handlers[i]
		for _, r := range f.liveAt[h.target] {
			// The catch register is live at the target because the
			// handler reads the exception from it. What it held before,
			// such as the iterator of a foreach inside the try block
			// that got the same register, is dead and is released with
			// the other temporaries, before the handler is entered.
			if !h.catch || r != h.reg {
				h.live = append(h.live, r)
			}
		}
	}
	for pc := range f.code {
		if in := &f.code[pc]; in.op == bytecode.OpLeave {
			in.b = 0
			if live := f.liveAt[int32(in.a)]; len(live) > 0 {
				f.leaveLive = append(f.leaveLive, live)
				in.b = uint32(len(f.leaveLive))
			}
		}
	}
	f.liveAt = nil
	return f, nil
}

// parameterPrologue finds, in the leading run of Local and Store
// instructions, which local receives each parameter register and where
// (conventions: "Local Lk, k; Store Lk, r_i", the Store of an optional
// parameter preceded by "ParamDefault r_i, <its Store>" and the code of the
// default, which the scan steps over). Backtraces show a parameter's current
// value once the prologue stored it, as PHP shows the CV, and a missing
// argument is reported at the Store that would have received it.
func parameterPrologue(f vmFunction, code []bytecode.Instruction) (locals, stores []int32) {
	locals, stores = make([]int32, f.params), make([]int32, f.params)
	for i := range locals {
		locals[i], stores[i] = -1, -1
	}
	locOf := map[uint32]uint32{} // raw Location register -> local
	for pc := 0; pc < len(code); {
		in := code[pc]
		switch {
		case in.Op == bytecode.OpLocal:
			locOf[in.A] = in.B
		case (in.Op == bytecode.OpStore || in.Op == bytecode.OpBind && in.Aux == uint16(bytecode.BindValue)) && in.B < uint32(f.params):
			local, ok := locOf[in.A]
			if !ok || locals[in.B] >= 0 {
				return locals, stores
			}
			locals[in.B], stores[in.B] = int32(local), int32(pc)
		case in.Op == bytecode.OpParamDefault && int(in.B) > pc && int(in.B) < len(code) &&
			(code[in.B].Op == bytecode.OpStore || code[in.B].Op == bytecode.OpBind && code[in.B].Aux == uint16(bytecode.BindValue)) && code[in.B].B == in.A:
			pc = int(in.B)
			continue
		default:
			return locals, stores
		}
		pc++
	}
	return locals, stores
}

// vmComposite reports the registers of f that may hold a composite value:
// written by an instruction that can produce one (or a Move from such a
// register).
func vmComposite(f *vmFunction) []bool {
	composite := make([]bool, f.nvals)
	for _, in := range f.code {
		switch in.op {
		case bytecode.OpLoad, bytecode.OpArrayGet, bytecode.OpNewArray, bytecode.OpCall, bytecode.OpCallValue, bytecode.OpHostCall,
			bytecode.OpInclude, bytecode.OpEval, bytecode.OpFetchConst, bytecode.OpRecvVariadic, bytecode.OpCallPending:
			// A constant's value can be an array (const A = [1]).
			if (in.op != bytecode.OpCall && in.op != bytecode.OpCallPending) || in.aux == 0 {
				composite[in.a] = true
			}
		case bytecode.OpBinary:
			if bytecode.BinaryOp(in.aux) == bytecode.Add {
				composite[in.a] = true
			}
		case bytecode.OpCast:
			if k := bytecode.CastKind(in.aux); k == bytecode.CastArray || k == bytecode.CastObject {
				composite[in.a] = true
			}
		case bytecode.OpNew, bytecode.OpThis, bytecode.OpPropGet, bytecode.OpPropSet, bytecode.OpPropRef, bytecode.OpStaticGet, bytecode.OpStaticSet,
			bytecode.OpStaticRef, bytecode.OpClassConst, bytecode.OpClone, bytecode.OpVerifyReturn, bytecode.OpClosure, bytecode.OpCallableConvert:
			// Format 1.7: every register these define or replace can hold
			// an object, whose release runs its destructor.
			roles, _ := bytecode.Roles17(bytecode.Instruction{Op: in.op, Aux: in.aux})
			for i, v := range [3]uint32{in.a, in.b, in.c} {
				if roles[i] == bytecode.RoleDef || roles[i] == bytecode.RoleUseDef {
					composite[v] = true
				}
			}
		case bytecode.OpIterInit, bytecode.OpIterNext, bytecode.OpArraySet, bytecode.OpMakeRef:
			// The iterator, a fetched element, an assignment's result and a
			// reference.
			if d, ok := vmDef(in); ok {
				composite[d] = true
			}
		}
	}
	for r := 0; r < f.params; r++ {
		composite[r] = true
	}
	for _, h := range f.handlers {
		if h.catch {
			composite[h.reg] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, in := range f.code {
			if in.op == bytecode.OpMove && in.aux == 0 && composite[in.b] && !composite[in.a] {
				composite[in.a] = true
				changed = true
			}
		}
	}
	return composite
}

// vmUses visits the registers instruction in reads, for the liveness.
func vmUses(in vmInstr, visit func(uint32)) {
	ra, rb, rc := operandRoles(bytecode.Instruction{Op: in.op, Aux: in.aux})
	if in.op == bytecode.OpExit && in.aux == 1 {
		ra = roleValue
	}
	ops := [3]uint32{in.a, in.b, in.c}
	dest := vmDefOperand(in)
	for i, role := range [3]int{ra, rb, rc} {
		isDest := i == dest
		if role == roleValue && !isDest {
			visit(ops[i])
		}
		if role == roleAny && i == 1 && in.op == bytecode.OpMove && in.aux == 0 {
			visit(ops[i])
		}
		if role == roleAny && i == 0 && (in.op == bytecode.OpPush && (in.aux == 0 || in.aux == vmPushTemp || in.aux == vmPushResult || in.aux == vmPushElement) || (in.op == bytecode.OpDrop || in.op == bytecode.OpSendUnpack) && in.aux == 0) {
			visit(ops[i])
		}
		if role == roleAny && i == 1 && in.op == bytecode.OpIterInit && in.aux&vmIterSubjectLocation == 0 {
			visit(ops[i])
		}
	}
}

// vmLiveness computes, for each instruction, the Value registers that may
// hold an array and are dead after it, so the interpreter releases them at
// once. Holding a dead copy would make the next write to the same array copy
// it (copy on write), turning "$x = $a[$i]; $a[$i] = ..." loops quadratic.
// Release timing is not observable by PHP code here (no objects or
// destructors), except through the owner count of a reference, which decides
// whether copying an array copies its element or shares it; so no register
// keeps a reference past its use without liveness: Push and Bind move the
// references MakeRef and IterNext put in registers. Functions whose dataflow
// would need more than vmLivenessWords bitset words keep registers until
// they are overwritten or the frame ends.
func vmLiveness(f *vmFunction) {
	n := len(f.code)
	composite := vmComposite(f)
	any := false
	for _, c := range composite {
		any = any || c
	}
	if !any {
		f.liveKnown = true
		return
	}
	// conts are, per EndFinally, the targets of the Leaves it completes,
	// worked out as the VM runs them (vmState.endFinally): a finally
	// block's EndFinally goes on like a Leave from its own pc. wanted are
	// the pcs whose live registers the unwinding needs (handler targets
	// and Leave targets).
	conts := map[int][]int{}
	wanted := map[int32]bool{}
	for _, h := range f.handlers {
		wanted[h.target] = true
	}
	if f.finallyAt != nil {
		ends := map[int32][]int{}
		type pending struct {
			h int32
			t int
		}
		var work []pending
		seen := map[pending]bool{}
		add := func(h int32, t int) {
			if p := (pending{h, t}); !seen[p] {
				seen[p] = true
				work = append(work, p)
			}
		}
		for pc, in := range f.code {
			switch in.op {
			case bytecode.OpEndFinally:
				if h := f.bodyAt[pc]; h >= 0 {
					ends[h] = append(ends[h], pc)
				}
			case bytecode.OpLeave:
				wanted[int32(in.a)] = true
				if h := through(f, pc, int(in.a)); h >= 0 {
					add(h, int(in.a))
				}
			}
		}
		for len(work) > 0 {
			p := work[len(work)-1]
			work = work[:len(work)-1]
			for _, e := range ends[p.h] {
				if next := through(f, e, p.t); next >= 0 {
					add(next, p.t)
				} else {
					conts[e] = append(conts[e], p.t)
				}
			}
		}
	} else {
		for _, in := range f.code {
			if in.op == bytecode.OpLeave {
				wanted[int32(in.a)] = true
			}
		}
	}
	// exc visits the handlers an instruction of a protected range can
	// pass control to: the innermost handler (a throw) and the innermost
	// finally block (a Return or a Leave). def is the register the entry
	// itself defines, or -1.
	exc := func(pc int, visit func(target int, def int)) {
		if f.handlerAt == nil {
			return
		}
		hr, hf := f.handlerAt[pc], f.finallyAt[pc]
		targets := [2]int32{hr, hf}
		if hr == hf {
			targets[1] = -1
		}
		for _, hi := range targets {
			if hi < 0 {
				continue
			}
			h := f.handlers[hi]
			def := -1
			if h.catch {
				def = int(h.reg)
			}
			visit(int(h.target), def)
		}
	}
	words := (f.nvals + 63) / 64
	sparse := words*(n+1) > vmLivenessWords
	uses := vmUses
	def := vmDef
	// PHP keeps a temporary until its last use in code order
	// (zend_calc_live_ranges), whichever path control takes: exit, throw
	// and an unmatched match leave an expression with its temporaries
	// still alive, and the unwinding frees them, after exit has printed
	// its message. holds are those registers at each such instruction,
	// used there for the liveness but never killed.
	holds := vmTerminalHolds(f, composite, uses, def)
	// carryAt maps the target of a Leave that carries a value (format
	// 1.10) to the register it carries: the edge from a finally block's
	// EndFinally to that target writes the register, so its liveness does
	// not reach back into the finally block, whose code reuses it.
	var carryAt map[int]uint32
	for _, in := range f.code {
		if in.op == bytecode.OpLeave && in.aux == bytecode.LeaveCarry {
			if carryAt == nil {
				carryAt = map[int]uint32{}
			}
			carryAt[int(in.a)] = in.c
		}
	}
	// edgeDef is the register the edge from pc to t defines, or -1.
	edgeDef := func(pc, t int) int {
		if carryAt != nil && f.code[pc].op == bytecode.OpEndFinally {
			if c, ok := carryAt[t]; ok {
				return int(c)
			}
		}
		return -1
	}
	succ := func(pc int, visit func(int)) {
		in := f.code[pc]
		switch in.op {
		case bytecode.OpJump:
			visit(int(in.a))
		case bytecode.OpBranch:
			visit(int(in.b))
			visit(int(in.c))
		case bytecode.OpIterInit, bytecode.OpIterNext, bytecode.OpNew:
			visit(pc + 1)
			visit(int(in.c))
		case bytecode.OpParamDefault, bytecode.OpStaticInit:
			visit(pc + 1)
			visit(int(in.b))
		case bytecode.OpLeave:
			if through(f, pc, int(in.a)) < 0 {
				visit(int(in.a))
			}
		case bytecode.OpEndFinally:
			for _, t := range conts[pc] {
				visit(t)
			}
		case bytecode.OpReturn, bytecode.OpReturnRef, bytecode.OpExit, bytecode.OpThrow, bytecode.OpMatchError:
		default:
			if pc+1 < n {
				visit(pc + 1)
			}
		}
	}
	if sparse {
		// The bit sets would be too large: the same facts are computed one
		// register at a time, in memory linear in the code.
		vmLivenessSparse(f, composite, uses, def, succ, exc, wanted, holds, edgeDef)
		return
	}
	// live[pc] is the live-in set of pc; iterate to a fixed point in reverse.
	live := make([]uint64, words*(n+1))
	set := func(s []uint64, r uint32) { s[r/64] |= 1 << (r % 64) }
	has := func(s []uint64, r uint32) bool { return s[r/64]&(1<<(r%64)) != 0 }
	out := make([]uint64, words)
	for changed := true; changed; {
		changed = false
		for pc := n - 1; pc >= 0; pc-- {
			clear(out)
			succ(pc, func(s int) {
				dr := edgeDef(pc, s)
				for w := 0; w < words; w++ {
					x := live[s*words+w]
					if dr >= 0 && dr/64 == w {
						x &^= 1 << (dr % 64)
					}
					out[w] |= x
				}
			})
			in := f.code[pc]
			if d, ok := def(in); ok {
				out[d/64] &^= 1 << (d % 64)
				if in.op == bytecode.OpIterNext {
					// The element is written on the fall-through edge only:
					// the exit edge still sees the register's old value.
					exit := live[int(in.c)*words : (int(in.c)+1)*words]
					out[d/64] |= exit[d/64] & (1 << (d % 64))
				}
			}
			exc(pc, func(t, dr int) {
				for w := 0; w < words; w++ {
					x := live[t*words+w]
					if dr >= 0 && dr/64 == w {
						x &^= 1 << (dr % 64)
					}
					out[w] |= x
				}
			})
			uses(in, func(r uint32) { set(out, r) })
			for _, r := range holds[int32(pc)] {
				set(out, r)
			}
			cur := live[pc*words : (pc+1)*words]
			for w := 0; w < words; w++ {
				if cur[w] != out[w] {
					cur[w] = out[w]
					changed = true
				}
			}
		}
	}
	for pc := 0; pc < n; pc++ {
		clear(out)
		succ(pc, func(s int) {
			dr := edgeDef(pc, s)
			for w := 0; w < words; w++ {
				x := live[s*words+w]
				if dr >= 0 && dr/64 == w {
					x &^= 1 << (dr % 64)
				}
				out[w] |= x
			}
		})
		exc(pc, func(t, dr int) {
			for w := 0; w < words; w++ {
				x := live[t*words+w]
				if dr >= 0 && dr/64 == w {
					x &^= 1 << (dr % 64)
				}
				out[w] |= x
			}
		})
		in := f.code[pc]
		start := len(f.kills)
		touched := func(r uint32) {
			if composite[r] && !has(out, r) {
				for _, k := range f.kills[start:] {
					if k == r {
						return
					}
				}
				f.kills = append(f.kills, r)
			}
		}
		uses(in, touched)
		if d, ok := def(in); ok {
			touched(d)
		}
		// Return and Exit end the frame; Call results are written after the
		// callee returns, so neither keeps a kill list.
		switch in.op {
		case bytecode.OpReturn, bytecode.OpExit, bytecode.OpCall, bytecode.OpCallValue, bytecode.OpCallPending:
			f.kills = f.kills[:start]
		}
		if k := len(f.kills) - start; k > 0 && k <= math.MaxUint16 {
			f.code[pc].kill, f.code[pc].nkill = uint32(start), uint16(k)
		} else {
			f.kills = f.kills[:start]
		}
	}
	if len(wanted) > 0 {
		f.liveAt = make(map[int32][]uint32, len(wanted))
		for pc := range wanted {
			var regs []uint32
			for r := 0; r < f.nvals; r++ {
				if composite[r] && has(live[int(pc)*words:(int(pc)+1)*words], uint32(r)) {
					regs = append(regs, uint32(r))
				}
			}
			f.liveAt[pc] = regs
		}
	}
	f.liveKnown = true
}

// vmLivenessWords bounds the liveness bitsets of one function (8 MiB).
const vmLivenessWords = 1 << 20

// vmLivenessSparseWork bounds the steps of the per-register analysis of a
// function too large for the bit sets. Compiled PHP stays far below it:
// its temporaries live for a few instructions each. Past it the function
// keeps its registers until they are overwritten, as before.
const vmLivenessSparseWork = 1 << 27

// vmLivenessSparseLimit is the bound in effect; tests of the path without
// any liveness set it to zero.
var vmLivenessSparseLimit = vmLivenessSparseWork

// vmLivenessSparse computes the same kill lists as the bit-set analysis of
// vmLiveness for a function whose bit sets would exceed vmLivenessWords:
// for each register that may hold a composite value, liveness is
// propagated backwards from its uses to its definitions over the
// predecessor graph. Releasing a dead register at once matters beyond
// copy-on-write here: it is when PHP runs the destructor of a temporary
// object. An IterNext's element, defined on one edge only, is treated as
// not defined there, which only keeps it longer.
func vmLivenessSparse(f *vmFunction, composite []bool, uses func(vmInstr, func(uint32)), def func(vmInstr) (uint32, bool), succ func(int, func(int)), exc func(int, func(int, int)), wanted map[int32]bool, holds map[int32][]uint32, edgeDef func(int, int) int) {
	n := len(f.code)
	// Predecessors in compressed form. via says how each edge is taken:
	// -2 for normal control, else an exceptional edge into a handler,
	// with the register the handler's entry defines or -1.
	start := make([]int32, n+2)
	for pc := 0; pc < n; pc++ {
		succ(pc, func(s int) { start[s+2]++ })
		exc(pc, func(s, _ int) { start[s+2]++ })
	}
	for i := 2; i < len(start); i++ {
		start[i] += start[i-1]
	}
	preds := make([]int32, start[n+1])
	via := make([]int32, start[n+1])
	for pc := 0; pc < n; pc++ {
		succ(pc, func(s int) {
			// An edge that writes a register (a carried value) is taken
			// like a handler's entry for it.
			v := int32(-2)
			if dr := edgeDef(pc, s); dr >= 0 {
				v = int32(dr)
			}
			preds[start[s+1]], via[start[s+1]] = int32(pc), v
			start[s+1]++
		})
		exc(pc, func(s, dr int) {
			preds[start[s+1]], via[start[s+1]] = int32(pc), int32(dr)
			start[s+1]++
		})
	}
	var wantedPCs []int32
	for pc := range wanted {
		wantedPCs = append(wantedPCs, pc)
	}
	liveAt := make(map[int32][]uint32, len(wanted))
	// Where each register is used and defined.
	type touch struct {
		pc  int32
		reg uint32
		use bool
		// hold marks a use that only keeps the register alive (holds).
		hold bool
	}
	var touches []touch
	for pc, in := range f.code {
		uses(in, func(r uint32) {
			if composite[r] {
				touches = append(touches, touch{pc: int32(pc), reg: r, use: true})
			}
		})
		for _, r := range holds[int32(pc)] {
			touches = append(touches, touch{pc: int32(pc), reg: r, use: true, hold: true})
		}
		if d, ok := def(in); ok && composite[d] {
			touches = append(touches, touch{pc: int32(pc), reg: d})
		}
	}
	byReg := make([][]int32, f.nvals) // indexes into touches
	for i, t := range touches {
		byReg[t.reg] = append(byReg[t.reg], int32(i))
	}
	defines := func(pc int32, r uint32) bool {
		in := f.code[pc]
		if in.op == bytecode.OpIterNext {
			return false
		}
		d, ok := def(in)
		return ok && d == r
	}
	stamp := make([]int32, n) // register plus one whose live-in set holds pc
	var stack []int32
	type kill struct {
		pc  int32
		reg uint32
	}
	var kills []kill
	work := 0
	for r, list := range byReg {
		if len(list) == 0 {
			continue
		}
		mark := int32(r + 1)
		stack = stack[:0]
		for _, ti := range list {
			if t := touches[ti]; t.use && stamp[t.pc] != mark {
				stamp[t.pc] = mark
				stack = append(stack, t.pc)
			}
		}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for pi := start[p]; pi < start[p+1]; pi++ {
				q := preds[pi]
				if work++; work > vmLivenessSparseLimit {
					return
				}
				// An exceptional edge leaves q before q wrote anything,
				// and a catch handler's entry defines its register.
				if stamp[q] == mark || via[pi] == int32(r) || via[pi] == -2 && defines(q, uint32(r)) {
					continue
				}
				stamp[q] = mark
				stack = append(stack, q)
			}
		}
		for _, pc := range wantedPCs {
			if stamp[pc] == mark {
				liveAt[pc] = append(liveAt[pc], uint32(r))
			}
		}
		last := int32(-1)
		for _, ti := range list {
			t := touches[ti]
			if t.pc == last || t.hold {
				continue
			}
			last = t.pc
			liveOut := false
			succ(int(t.pc), func(s int) { liveOut = liveOut || stamp[s] == mark && edgeDef(int(t.pc), s) != r })
			exc(int(t.pc), func(s, dr int) { liveOut = liveOut || stamp[s] == mark && dr != r })
			if !liveOut {
				kills = append(kills, kill{t.pc, uint32(r)})
			}
		}
	}
	f.liveAt, f.liveKnown = liveAt, true
	// Kill lists are contiguous per instruction.
	count := make([]int32, n+1)
	for _, k := range kills {
		count[k.pc+1]++
	}
	for i := 1; i <= n; i++ {
		count[i] += count[i-1]
	}
	f.kills = make([]uint32, len(kills))
	next := append([]int32(nil), count[:n]...)
	for _, k := range kills {
		f.kills[next[k.pc]] = k.reg
		next[k.pc]++
	}
	for pc := 0; pc < n; pc++ {
		k := int(count[pc+1] - count[pc])
		switch f.code[pc].op {
		case bytecode.OpReturn, bytecode.OpExit, bytecode.OpCall, bytecode.OpCallValue, bytecode.OpCallPending:
			k = 0
		}
		if k > 0 && k <= math.MaxUint16 {
			f.code[pc].kill, f.code[pc].nkill = uint32(count[pc]), uint16(k)
		}
	}
}

// definesA reports whether operand A is written by the instruction.
func definesA(op bytecode.Opcode) bool {
	switch op {
	case bytecode.OpConst, bytecode.OpMove, bytecode.OpLocal, bytecode.OpLoad, bytecode.OpBinary, bytecode.OpCall, bytecode.OpCallValue,
		bytecode.OpHostCall, bytecode.OpNewArray, bytecode.OpArrayGet, bytecode.OpInclude, bytecode.OpEval, bytecode.OpShell,
		bytecode.OpUnary, bytecode.OpCast, bytecode.OpIncDec, bytecode.OpIsset, bytecode.OpArrayIsset, bytecode.OpIterInit, bytecode.OpMakeRef,
		bytecode.OpFetchConst, bytecode.OpBeginSilence, bytecode.OpRecvVariadic, bytecode.OpCallPending,
		bytecode.OpNew, bytecode.OpThis, bytecode.OpPropGet, bytecode.OpPropRef, bytecode.OpPropIsset, bytecode.OpStaticGet, bytecode.OpStaticRef,
		bytecode.OpClassConst, bytecode.OpClassName, bytecode.OpInstanceof, bytecode.OpClone,
		bytecode.OpClosure, bytecode.OpCallableConvert, bytecode.OpArgByRef:
		return true
	}
	return false
}

// vmDefOperand is the operand (0 for A, 1 for B) that names the Value
// register a decoded instruction writes, or -1 when it writes none or
// names it elsewhere (the result of ArraySet is in its aux).
func vmDefOperand(in vmInstr) int {
	switch in.op {
	case bytecode.OpLocal, bytecode.OpArraySet:
		return -1
	case bytecode.OpMove, bytecode.OpCall, bytecode.OpCallPending:
		if in.aux != 0 {
			return -1 // a Location
		}
	case bytecode.OpIterNext:
		if bytecode.IterFlags(in.aux)&bytecode.IterLocation != 0 {
			return -1
		}
		return 1
	case bytecode.OpIterKey:
		if bytecode.IterFlags(in.aux) == bytecode.IterLocation {
			return -1
		}
		return 0
	}
	if definesA(in.op) {
		return 0
	}
	return -1
}

// vmDef is the Value register a decoded instruction writes.
func vmDef(in vmInstr) (uint32, bool) {
	if in.op == bytecode.OpArraySet {
		return bytecode.ArraySetResult(in.aux)
	}
	switch vmDefOperand(in) {
	case 0:
		return in.a, true
	case 1:
		return in.b, true
	}
	return 0, false
}
