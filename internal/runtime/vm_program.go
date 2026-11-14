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
	// required is how many arguments a call must pass: every parameter up
	// to the last one no ParamDefault names, as PHP treats an optional
	// parameter before a required one as required.
	required int
	// nelems is the size of a frame's element window, and elemOf maps each
	// Location register to its slot there, -1 when no FetchDim defines it.
	nelems int
	elemOf []int32
	// unsupported names a construct of the function that stage 1 cannot run;
	// entering the function ends the run with an UnsupportedError.
	unsupported string
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
)

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
		return roleTarget, roleNone, roleNone
	case bytecode.OpBranch:
		return roleValue, roleTarget, roleTarget
	case bytecode.OpPush:
		return roleAny, roleNameConst, roleNone
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
	prog := &Program{consts: pool, funcs: make([]vmFunction, n), byName: make(map[string]int, n), entry: int(p.Entry()), width: width}
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
		prog.funcs[i] = f
		if i != prog.entry {
			key := phpname.Lower(f.name)
			if _, dup := prog.byName[key]; !dup {
				prog.byName[key] = i
			}
		}
	}
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
	if fv.HandlerCount() > 0 && f.unsupported == "" {
		f.unsupported = "try, catch and finally"
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
			if kinds[in.A] == bytecode.Location {
				out.aux = 1
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
		case bytecode.OpFetchDim:
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
	if f.nvals > 0 {
		vmLiveness(&f)
	}
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
	// Registers that may hold a composite value: written by an instruction
	// that can produce one (or a Move from such a register).
	composite := make([]bool, f.nvals)
	for _, in := range f.code {
		switch in.op {
		case bytecode.OpLoad, bytecode.OpArrayGet, bytecode.OpNewArray, bytecode.OpCall, bytecode.OpCallValue, bytecode.OpHostCall,
			bytecode.OpInclude, bytecode.OpEval, bytecode.OpFetchConst:
			// A constant's value can be an array (const A = [1]).
			if in.op != bytecode.OpCall || in.aux == 0 {
				composite[in.a] = true
			}
		case bytecode.OpBinary:
			if bytecode.BinaryOp(in.aux) == bytecode.Add {
				composite[in.a] = true
			}
		case bytecode.OpCast:
			if bytecode.CastKind(in.aux) == bytecode.CastArray {
				composite[in.a] = true
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
	for changed := true; changed; {
		changed = false
		for _, in := range f.code {
			if in.op == bytecode.OpMove && in.aux == 0 && composite[in.b] && !composite[in.a] {
				composite[in.a] = true
				changed = true
			}
		}
	}
	any := false
	for _, c := range composite {
		any = any || c
	}
	if !any {
		return
	}
	words := (f.nvals + 63) / 64
	if words*(n+1) > vmLivenessWords {
		return
	}
	uses := func(in vmInstr, visit func(uint32)) {
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
			if role == roleAny && i == 0 && (in.op == bytecode.OpPush || in.op == bytecode.OpDrop) && in.aux == 0 {
				visit(ops[i])
			}
			if role == roleAny && i == 1 && in.op == bytecode.OpIterInit && in.aux&vmIterSubjectLocation == 0 {
				visit(ops[i])
			}
		}
	}
	def := vmDef
	succ := func(pc int, visit func(int)) {
		in := f.code[pc]
		switch in.op {
		case bytecode.OpJump:
			visit(int(in.a))
		case bytecode.OpBranch:
			visit(int(in.b))
			visit(int(in.c))
		case bytecode.OpIterInit, bytecode.OpIterNext:
			visit(pc + 1)
			visit(int(in.c))
		case bytecode.OpParamDefault:
			visit(pc + 1)
			visit(int(in.b))
		case bytecode.OpReturn, bytecode.OpReturnRef, bytecode.OpExit, bytecode.OpThrow, bytecode.OpLeave, bytecode.OpEndFinally, bytecode.OpMatchError:
		default:
			if pc+1 < n {
				visit(pc + 1)
			}
		}
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
				for w := 0; w < words; w++ {
					out[w] |= live[s*words+w]
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
			uses(in, func(r uint32) { set(out, r) })
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
			for w := 0; w < words; w++ {
				out[w] |= live[s*words+w]
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
		case bytecode.OpReturn, bytecode.OpExit, bytecode.OpCall, bytecode.OpCallValue:
			f.kills = f.kills[:start]
		}
		if k := len(f.kills) - start; k > 0 && k <= math.MaxUint16 {
			f.code[pc].kill, f.code[pc].nkill = uint32(start), uint16(k)
		} else {
			f.kills = f.kills[:start]
		}
	}
}

// vmLivenessWords bounds the liveness bitsets of one function (8 MiB).
const vmLivenessWords = 1 << 20

// definesA reports whether operand A is written by the instruction.
func definesA(op bytecode.Opcode) bool {
	switch op {
	case bytecode.OpConst, bytecode.OpMove, bytecode.OpLocal, bytecode.OpLoad, bytecode.OpBinary, bytecode.OpCall, bytecode.OpCallValue,
		bytecode.OpHostCall, bytecode.OpNewArray, bytecode.OpArrayGet, bytecode.OpInclude, bytecode.OpEval, bytecode.OpShell,
		bytecode.OpUnary, bytecode.OpCast, bytecode.OpIncDec, bytecode.OpIsset, bytecode.OpArrayIsset, bytecode.OpIterInit, bytecode.OpMakeRef,
		bytecode.OpFetchConst, bytecode.OpBeginSilence:
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
	case bytecode.OpMove, bytecode.OpCall:
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
