package vm

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

var opNames = [...]string{
	bytecode.OpCheckpoint: "Checkpoint", bytecode.OpConst: "Const", bytecode.OpMove: "Move",
	bytecode.OpLocal: "Local", bytecode.OpLoad: "Load", bytecode.OpStore: "Store",
	bytecode.OpBinary: "Binary", bytecode.OpJump: "Jump", bytecode.OpBranch: "Branch",
	bytecode.OpPush: "Push", bytecode.OpCall: "Call", bytecode.OpCallValue: "CallValue",
	bytecode.OpHostCall: "HostCall", bytecode.OpReturn: "Return", bytecode.OpReturnRef: "ReturnRef",
	bytecode.OpThrow: "Throw", bytecode.OpOutput: "Output", bytecode.OpNewArray: "NewArray",
	bytecode.OpArrayGet: "ArrayGet", bytecode.OpInclude: "Include", bytecode.OpEval: "Eval",
	bytecode.OpShell: "Shell", bytecode.OpDrop: "Drop", bytecode.OpLeave: "Leave",
	bytecode.OpEndFinally: "EndFinally", bytecode.OpUnary: "Unary", bytecode.OpCast: "Cast",
	bytecode.OpIncDec: "IncDec", bytecode.OpArraySet: "ArraySet", bytecode.OpArrayAppend: "ArrayAppend",
	bytecode.OpArrayUnset: "ArrayUnset", bytecode.OpIsset: "Isset", bytecode.OpArrayIsset: "ArrayIsset",
	bytecode.OpUnset: "Unset", bytecode.OpExit: "Exit", bytecode.OpArrayAdd: "ArrayAdd",
	bytecode.OpFetchDim: "FetchDim", bytecode.OpIterInit: "IterInit", bytecode.OpIterNext: "IterNext",
	bytecode.OpIterKey: "IterKey", bytecode.OpParamDefault: "ParamDefault",
	bytecode.OpBind: "Bind", bytecode.OpMakeRef: "MakeRef",
	bytecode.OpFetchConst: "FetchConst", bytecode.OpDeclareConst: "DeclareConst",
	bytecode.OpBeginSilence: "BeginSilence", bytecode.OpEndSilence: "EndSilence", bytecode.OpMatchError: "MatchError",
	bytecode.OpBindGlobal: "BindGlobal", bytecode.OpStaticInit: "StaticInit", bytecode.OpBindStatic: "BindStatic",
	bytecode.OpRecvVariadic: "RecvVariadic", bytecode.OpInitCall: "InitCall", bytecode.OpSendUnpack: "SendUnpack",
	bytecode.OpCallPending:  "CallPending",
	bytecode.OpDeclareClass: "DeclareClass", bytecode.OpNew: "New", bytecode.OpInitMethod: "InitMethod",
	bytecode.OpInitStatic: "InitStatic", bytecode.OpThis: "This", bytecode.OpPropGet: "PropGet",
	bytecode.OpPropSet: "PropSet", bytecode.OpFetchProp: "FetchProp", bytecode.OpPropRef: "PropRef",
	bytecode.OpPropIsset: "PropIsset", bytecode.OpPropUnset: "PropUnset", bytecode.OpStaticGet: "StaticGet",
	bytecode.OpStaticSet: "StaticSet", bytecode.OpFetchStatic: "FetchStatic", bytecode.OpStaticRef: "StaticRef",
	bytecode.OpClassConst: "ClassConst", bytecode.OpClassName: "ClassName", bytecode.OpInstanceof: "Instanceof",
	bytecode.OpClone: "Clone", bytecode.OpVerifyReturn: "VerifyReturn",
	bytecode.OpClosure: "Closure", bytecode.OpBindLexical: "BindLexical", bytecode.OpCallableConvert: "CallableConvert",
	bytecode.OpArgByRef:        "ArgByRef",
	bytecode.OpGeneratorCreate: "GeneratorCreate", bytecode.OpYield: "Yield", bytecode.OpYieldFrom: "YieldFrom",
}

var binaryNames = [...]string{
	bytecode.Add: "Add", bytecode.Subtract: "Subtract", bytecode.Multiply: "Multiply",
	bytecode.Divide: "Divide", bytecode.Concat: "Concat", bytecode.Equal: "Equal",
	bytecode.Identical: "Identical", bytecode.Less: "Less", bytecode.BitAnd: "BitAnd",
	bytecode.BitOr: "BitOr", bytecode.BitXor: "BitXor", bytecode.Modulo: "Modulo",
	bytecode.Power: "Power", bytecode.ShiftLeft: "ShiftLeft", bytecode.ShiftRight: "ShiftRight",
	bytecode.NotEqual: "NotEqual", bytecode.NotIdentical: "NotIdentical",
	bytecode.LessOrEqual: "LessOrEqual", bytecode.Spaceship: "Spaceship", bytecode.BoolXor: "BoolXor",
}

var unaryNames = [...]string{
	bytecode.Not: "Not", bytecode.Negate: "Negate", bytecode.Plus: "Plus", bytecode.BitNot: "BitNot",
	bytecode.Increment: "Increment", bytecode.Decrement: "Decrement",
}

var castNames = [...]string{
	bytecode.CastBool: "CastBool", bytecode.CastInt: "CastInt", bytecode.CastFloat: "CastFloat",
	bytecode.CastString: "CastString", bytecode.CastArray: "CastArray", bytecode.CastObject: "CastObject",
}

var incDecNames = [...]string{bytecode.PreInc: "PreInc", bytecode.PreDec: "PreDec", bytecode.PostInc: "PostInc", bytecode.PostDec: "PostDec"}

func name(table []string, i int) string {
	if i >= 0 && i < len(table) && table[i] != "" {
		return table[i]
	}
	return strconv.Itoa(i)
}

func auxName(in bytecode.Instruction) string {
	a := int(in.Aux)
	switch in.Op {
	case bytecode.OpBinary:
		return name(binaryNames[:], a)
	case bytecode.OpUnary:
		return name(unaryNames[:], a)
	case bytecode.OpCast:
		return name(castNames[:], a)
	case bytecode.OpIncDec:
		return name(incDecNames[:], a)
	case bytecode.OpFetchDim:
		return fetchName(bytecode.FetchFlags(in.Aux))
	case bytecode.OpYield:
		var parts []string
		for _, f := range [...]struct {
			bit  bytecode.YieldFlags
			name string
		}{{bytecode.YieldValue, "value"}, {bytecode.YieldKey, "key"}, {bytecode.YieldRef, "ref"}, {bytecode.YieldNotice, "notice"}} {
			if bytecode.YieldFlags(in.Aux)&f.bit != 0 {
				parts = append(parts, f.name)
			}
		}
		return joinOrZero(parts)
	case bytecode.OpLeave:
		if in.Aux == bytecode.LeaveCarry {
			return "carry"
		}
	case bytecode.OpArrayGet:
		if in.Aux == bytecode.ArrayGetList {
			return "list"
		}
	case bytecode.OpArrayIsset:
		if in.Aux == bytecode.ArrayIssetEmpty {
			return "empty"
		}
	case bytecode.OpFetchConst:
		if f := bytecode.FetchConstFlags(in.Aux); f != 0 {
			var parts []string
			if f&bytecode.FetchConstUnqualified != 0 {
				parts = append(parts, "unqualified")
			}
			if f&bytecode.FetchConstNoCache != 0 {
				parts = append(parts, "nocache")
			}
			if f&bytecode.FetchConstDefault != 0 {
				parts = append(parts, "default")
			}
			return strings.Join(parts, "+")
		}
	case bytecode.OpBind:
		if bytecode.BindFlags(in.Aux) == bytecode.BindValue {
			return "value"
		}
	case bytecode.OpIterInit, bytecode.OpIterNext:
		if bytecode.IterFlags(in.Aux)&bytecode.IterByRef != 0 {
			return "ref+" + strconv.Itoa(a&int(bytecode.IterLocation))
		}
	case bytecode.OpArraySet:
		// The write flags, then the result register when there is one.
		if r, ok := bytecode.ArraySetResult(in.Aux); ok {
			return strconv.Itoa(a&int(bytecode.KeyLocation|bytecode.ValueLocation)) + "->r" + strconv.FormatUint(uint64(r), 10)
		}
	case bytecode.OpPush:
		// 0 and PushLoad keep their numbers; format 1.7 adds named sends.
		if n := name([]string{bytecode.PushAuto: "auto", bytecode.PushTemp: "temp", bytecode.PushResult: "result", bytecode.PushElement: "element"}, a); n != strconv.Itoa(a) {
			return n
		}
	}
	if _, ok := bytecode.Roles17(in); ok {
		return memberAuxName(in)
	}
	return strconv.Itoa(a)
}

var classRefNames = [...]string{bytecode.ClassSelf: "self", bytecode.ClassParent: "parent", bytecode.ClassStatic: "static", bytecode.ClassValue: "value"}

// memberAuxName spells the Aux of a format 1.7 instruction: the class
// reference when it is not a named class, then the flags and the mode the
// opcode defines, joined by "+"; "0" when there is none of them.
func memberAuxName(in bytecode.Instruction) string {
	aux := in.Aux
	var parts []string
	switch in.Op {
	case bytecode.OpBindLexical:
		return "capture " + strconv.Itoa(int(aux))
	case bytecode.OpVerifyReturn:
		return name([]string{bytecode.ReturnValue: "value", bytecode.ReturnNone: "none", bytecode.ReturnNever: "never"}, int(aux))
	case bytecode.OpInitMethod:
		if aux == bytecode.InitCallable {
			return "callable"
		}
	case bytecode.OpDeclareClass, bytecode.OpThis, bytecode.OpClone:
		return strconv.Itoa(int(aux))
	}
	if r, ok := classOperand(in.Op); ok && r >= 0 {
		if ref := bytecode.AuxClassRef(aux); ref != bytecode.ClassNamed {
			parts = append(parts, name(classRefNames[:], int(ref)))
		}
	}
	if in.Op == bytecode.OpNew {
		if aux&bytecode.NewArgs != 0 {
			parts = append(parts, "args")
		}
		return joinOrZero(parts)
	}
	if aux&bytecode.MemberNameReg != 0 {
		parts = append(parts, "namereg")
	}
	if m := modeName(in.Op, bytecode.AuxMemberMode(aux)); m != "" {
		parts = append(parts, m)
	}
	if aux&bytecode.MemberObjLocation != 0 {
		parts = append(parts, "objloc")
	}
	if aux&bytecode.MemberResult != 0 {
		parts = append(parts, "result")
	}
	return joinOrZero(parts)
}

func joinOrZero(parts []string) string {
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, "+")
}

// modeName spells the member mode as the opcode reads it; the default mode
// (a plain read, or a write for the fetches) is left out.
func modeName(op bytecode.Opcode, m bytecode.MemberMode) string {
	if m == 0 {
		return ""
	}
	switch op {
	case bytecode.OpPropGet:
		return name([]string{bytecode.MemberQuiet: "quiet", bytecode.MemberAssignOp: "assignop", bytecode.MemberIncDec: "incdec"}, int(m))
	case bytecode.OpStaticGet:
		return name([]string{bytecode.MemberQuiet: "quiet", bytecode.MemberIsset: "isset", bytecode.MemberEmpty: "empty"}, int(m))
	case bytecode.OpPropIsset:
		return name([]string{bytecode.MemberIsset: "isset", bytecode.MemberEmpty: "empty"}, int(m))
	case bytecode.OpFetchProp, bytecode.OpFetchStatic:
		return name([]string{bytecode.MemberReadWrite: "readwrite", bytecode.MemberUnset: "unset"}, int(m))
	}
	return strconv.Itoa(int(m))
}

// classOperand is the index of the operand that names a class (0 A, 1 B,
// 2 C) for the format 1.7 opcodes that have one.
func classOperand(op bytecode.Opcode) (int, bool) {
	switch op {
	case bytecode.OpInitStatic:
		return 0, true
	case bytecode.OpNew, bytecode.OpStaticGet, bytecode.OpStaticSet, bytecode.OpFetchStatic, bytecode.OpStaticRef, bytecode.OpClassConst, bytecode.OpClassName:
		return 1, true
	case bytecode.OpInstanceof:
		return 2, true
	}
	return -1, false
}

// nameOperand is the index of the member-name operand.
func nameOperand(op bytecode.Opcode) (int, bool) {
	switch op {
	case bytecode.OpInitMethod, bytecode.OpInitStatic, bytecode.OpPropSet, bytecode.OpPropUnset:
		return 1, true
	case bytecode.OpPropGet, bytecode.OpFetchProp, bytecode.OpPropRef, bytecode.OpPropIsset,
		bytecode.OpStaticGet, bytecode.OpStaticSet, bytecode.OpFetchStatic, bytecode.OpStaticRef:
		return 2, true
	}
	return -1, false
}

// operands17 describes the class, name, constant, class-table and target
// operands of a format 1.7 instruction, from its roles; registers are the
// numbers in the instruction's columns.
func operands17(image bytecode.Image, in bytecode.Instruction) string {
	roles, ok := bytecode.Roles17(in)
	if !ok {
		return ""
	}
	vals := [3]uint32{in.A, in.B, in.C}
	str := func(i uint32) string {
		if int(i) < len(image.Constants) {
			return strconv.Quote(image.Constants[i].Bytes)
		}
		return "?"
	}
	ci, _ := classOperand(in.Op)
	ni, _ := nameOperand(in.Op)
	var parts []string
	for i, role := range roles {
		v := vals[i]
		switch {
		case i == ci:
			switch ref := bytecode.AuxClassRef(in.Aux); ref {
			case bytecode.ClassNamed:
				parts = append(parts, "class "+str(v))
			case bytecode.ClassValue:
				parts = append(parts, "class r"+strconv.FormatUint(uint64(v), 10))
			default:
				parts = append(parts, "class "+name(classRefNames[:], int(ref)))
			}
		case i == ni && role == bytecode.RoleString:
			parts = append(parts, "name "+str(v))
		case i == ni && role == bytecode.RoleUse:
			parts = append(parts, "name r"+strconv.FormatUint(uint64(v), 10))
		case role == bytecode.RoleString:
			parts = append(parts, "const "+str(v))
		case role == bytecode.RoleClass:
			n := "?"
			if int(v) < len(image.Classes) {
				n = str(image.Classes[v].Name)
			}
			parts = append(parts, "class "+strconv.FormatUint(uint64(v), 10)+" "+n)
		case role == bytecode.RoleTarget:
			parts = append(parts, "target "+strconv.FormatUint(uint64(v), 10))
		case role == bytecode.RoleFunc:
			n := "?"
			if int(v) < len(image.Functions) {
				n = strconv.Quote(image.Functions[v].Name)
			}
			parts = append(parts, "func "+strconv.FormatUint(uint64(v), 10)+" "+n)
		}
	}
	return strings.Join(parts, ", ")
}

// fetchName spells FetchFlags: the mode, then +key for a key the instruction
// reads itself, +append for [], and the use.
func fetchName(f bytecode.FetchFlags) string {
	out := name([]string{bytecode.FetchWrite: "Write", bytecode.FetchReadWrite: "ReadWrite", bytecode.FetchUnset: "Unset"}, int(f.Mode()))
	if f&bytecode.FetchKeyLocation != 0 {
		out += "+key"
	}
	if f&bytecode.FetchAppend != 0 {
		out += "+append"
	}
	if f&bytecode.FetchContainerValue != 0 {
		out += "+value"
	}
	switch f.Use() {
	case bytecode.FetchForIncDec:
		out += "+incdec"
	case bytecode.FetchForAssignOp:
		out += "+assignop"
	case bytecode.FetchForRef:
		out += "+ref"
	}
	return out
}

func constantText(c bytecode.Constant) string {
	switch c.Kind {
	case bytecode.Null:
		return "null"
	case bytecode.Bool:
		return "bool " + strconv.FormatBool(c.Bits == 1)
	case bytecode.Int:
		return "int " + strconv.FormatInt(int64(c.Bits), 10)
	case bytecode.Float:
		return fmt.Sprintf("float %s (%#016x)", strconv.FormatFloat(math.Float64frombits(c.Bits), 'g', -1, 64), c.Bits)
	case bytecode.String:
		return "string " + strconv.Quote(c.Bytes)
	}
	return fmt.Sprintf("kind %d bits %#x bytes %q", c.Kind, c.Bits, c.Bytes)
}

var functionFlagNames = []struct {
	flag bytecode.FunctionFlags
	name string
}{{bytecode.FuncStrict, "strict"}, {bytecode.FuncNoThis, "nothis"}, {bytecode.FuncClosure, "closure"}, {bytecode.FuncUsesThis, "usesthis"}}

var classFlagNames = []struct {
	flag bytecode.ClassFlags
	name string
}{
	{bytecode.ClassInterface, "interface"}, {bytecode.ClassTrait, "trait"}, {bytecode.ClassAbstract, "abstract"},
	{bytecode.ClassFinal, "final"}, {bytecode.ClassReadonly, "readonly"}, {bytecode.ClassHoisted, "hoisted"},
}

var memberFlagNames = []struct {
	flag bytecode.MemberFlags
	name string
}{
	{bytecode.MemberStatic, "static"}, {bytecode.MemberFinal, "final"}, {bytecode.MemberAbstract, "abstract"},
	{bytecode.MemberReadonly, "readonly"}, {bytecode.MemberByRef, "byref"},
}

// memberFlagsText is the visibility, always spelled, then the other flags.
func memberFlagsText(f bytecode.MemberFlags) string {
	out := name([]string{bytecode.MemberPublic: "public", bytecode.MemberProtected: "protected", bytecode.MemberPrivate: "private"}, int(f.Visibility()))
	for _, n := range memberFlagNames {
		if f&n.flag != 0 {
			out += "+" + n.name
		}
	}
	return out
}

// Disassemble renders image deterministically: the source, constant and
// class tables, then each function with its flags and return type, its
// parameter table, locals, register classes and one instruction per line as
// "pc: Op aux A B C ; line N". A Const line also names its constant, a Call
// line its callee, and a format 1.7 instruction its class, member name,
// constant, class-table entry and target operands.
func Disassemble(image bytecode.Image) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entry %d\n", image.Entry)
	for i, s := range image.Sources {
		fmt.Fprintf(&b, "source %d %q bytes=%d lines=%d\n", i+1, s.Name, s.Bytes, s.Lines)
	}
	for i, c := range image.Constants {
		fmt.Fprintf(&b, "const %d: %s\n", i, constantText(c))
	}
	str := func(i uint32) string {
		if int(i) < len(image.Constants) {
			return strconv.Quote(image.Constants[i].Bytes)
		}
		return "?"
	}
	// opt renders the index-plus-one form of a String constant.
	opt := func(i uint32) string {
		if i == 0 {
			return "-"
		}
		return str(i - 1)
	}
	fname := func(i uint32) string {
		if int(i) < len(image.Functions) {
			return fmt.Sprintf("func %d %q", i, image.Functions[i].Name)
		}
		return fmt.Sprintf("func %d ?", i)
	}
	// value renders the Value/Init pair of a class constant or property.
	value := func(v, init uint32) string {
		switch {
		case v != 0 && int(v-1) < len(image.Constants):
			return " = " + constantText(image.Constants[v-1])
		case v != 0:
			return " = ?"
		case init != 0:
			return " = init " + fname(init-1)
		}
		return ""
	}
	for ci, c := range image.Classes {
		var flags []string
		for _, n := range classFlagNames {
			if c.Flags&n.flag != 0 {
				flags = append(flags, n.name)
			}
		}
		fmt.Fprintf(&b, "class %d %s line=%d flags=%s", ci, str(c.Name), c.Line, joinOrZero(flags))
		if c.Parent != 0 {
			fmt.Fprintf(&b, " extends %s", opt(c.Parent))
		}
		b.WriteByte('\n')
		for _, l := range []struct {
			label string
			names []uint32
		}{{"implements", c.Interfaces}, {"uses", c.Traits}} {
			if len(l.names) == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %s", l.label)
			for _, n := range l.names {
				fmt.Fprintf(&b, " %s", str(n))
			}
			b.WriteByte('\n')
		}
		for _, r := range c.Rules {
			m := str(r.Method)
			if r.Trait != 0 {
				m = opt(r.Trait) + "::" + m
			}
			fmt.Fprintf(&b, "  rule %s", m)
			if len(r.Insteadof) > 0 {
				b.WriteString(" insteadof")
				for _, n := range r.Insteadof {
					fmt.Fprintf(&b, " %s", str(n))
				}
			} else {
				b.WriteString(" as")
				if r.HasVisibility {
					fmt.Fprintf(&b, " %s", memberFlagsText(r.Flags))
				}
				if r.Alias != 0 {
					fmt.Fprintf(&b, " %s", opt(r.Alias))
				}
			}
			fmt.Fprintf(&b, " ; line %d\n", r.Line)
		}
		for _, k := range c.Constants {
			fmt.Fprintf(&b, "  const %s %s%s\n", str(k.Name), memberFlagsText(k.Flags), value(k.Value, k.Init))
		}
		for _, p := range c.Properties {
			fmt.Fprintf(&b, "  prop %s %s", str(p.Name), memberFlagsText(p.Flags))
			if p.Type != 0 {
				fmt.Fprintf(&b, " type %s", opt(p.Type))
			}
			fmt.Fprintf(&b, "%s\n", value(p.Value, p.Init))
		}
		for _, m := range c.Methods {
			fmt.Fprintf(&b, "  method %s %s %s", str(m.Name), memberFlagsText(m.Flags), fname(m.Function))
			if m.Signature != 0 {
				fmt.Fprintf(&b, " signature %s", opt(m.Signature))
			}
			fmt.Fprintf(&b, " ; line %d\n", m.Line)
		}
	}
	for fi, f := range image.Functions {
		fmt.Fprintf(&b, "func %d %q params=%d locals=%d registers=%d maxstack=%d", fi, f.Name, f.Params, f.Locals, len(f.Registers), f.MaxStack)
		if f.Flags != 0 {
			var flags []string
			for _, n := range functionFlagNames {
				if f.Flags&n.flag != 0 {
					flags = append(flags, n.name)
				}
			}
			if rest := f.Flags &^ (bytecode.FuncStrict | bytecode.FuncNoThis | bytecode.FuncClosure | bytecode.FuncUsesThis); rest != 0 {
				flags = append(flags, strconv.Itoa(int(rest)))
			}
			fmt.Fprintf(&b, " flags=%s", strings.Join(flags, "+"))
		}
		if f.ReturnType != 0 {
			fmt.Fprintf(&b, " returns %s", opt(f.ReturnType))
		}
		b.WriteByte('\n')
		for i, c := range f.Captures {
			fmt.Fprintf(&b, "  capture %d local %d", i, c.Local)
			if c.Flags&bytecode.CaptureByRef != 0 {
				b.WriteString(" byref")
			}
			if c.Flags&bytecode.CaptureImplicit != 0 {
				b.WriteString(" implicit")
			}
			b.WriteByte('\n')
		}
		for i, p := range f.Parameters {
			fmt.Fprintf(&b, "  param %d %s", i, str(p.Name))
			for _, n := range []struct {
				flag bytecode.ParameterFlags
				name string
			}{{bytecode.ParamByRef, "byref"}, {bytecode.ParamVariadic, "variadic"}, {bytecode.ParamOptional, "optional"}} {
				if p.Flags&n.flag != 0 {
					fmt.Fprintf(&b, " %s", n.name)
				}
			}
			if p.Type != 0 {
				fmt.Fprintf(&b, " type %s", opt(p.Type))
			}
			b.WriteByte('\n')
		}
		for i, c := range f.LocalNames {
			n := "?"
			if int(c) < len(image.Constants) {
				n = strconv.Quote(image.Constants[c].Bytes)
			}
			fmt.Fprintf(&b, "  local %d %s\n", i, n)
		}
		var kinds strings.Builder
		for _, k := range f.Registers {
			switch k {
			case bytecode.Value:
				kinds.WriteByte('V')
			case bytecode.Location:
				kinds.WriteByte('L')
			default:
				kinds.WriteByte('?')
			}
		}
		fmt.Fprintf(&b, "  registers %s\n", kinds.String())
		for pc, in := range f.Code {
			line := uint32(0)
			if pc < len(f.Positions) {
				line = f.Positions[pc].Line
			}
			fmt.Fprintf(&b, "  %d: %s %s %d %d %d ; line %d", pc, name(opNames[:], int(in.Op)), auxName(in), in.A, in.B, in.C, line)
			switch {
			case in.Op == bytecode.OpConst && int(in.B) < len(image.Constants):
				fmt.Fprintf(&b, " ; %s", constantText(image.Constants[in.B]))
			case in.Op == bytecode.OpCall && int(in.B) < len(image.Functions):
				fmt.Fprintf(&b, " ; %s", image.Functions[in.B].Name)
			default:
				if d := operands17(image, in); d != "" {
					fmt.Fprintf(&b, " ; %s", d)
				}
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}
