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

var unaryNames = [...]string{bytecode.Not: "Not", bytecode.Negate: "Negate", bytecode.Plus: "Plus", bytecode.BitNot: "BitNot"}

var castNames = [...]string{
	bytecode.CastBool: "CastBool", bytecode.CastInt: "CastInt", bytecode.CastFloat: "CastFloat",
	bytecode.CastString: "CastString", bytecode.CastArray: "CastArray",
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
	}
	return strconv.Itoa(a)
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

// Disassemble renders image deterministically: the source and constant
// tables, then each function with its locals, register classes and one
// instruction per line as "pc: Op aux A B C ; line N". A Const line also
// names its constant and a Call line its callee.
func Disassemble(image bytecode.Image) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entry %d\n", image.Entry)
	for i, s := range image.Sources {
		fmt.Fprintf(&b, "source %d %q bytes=%d lines=%d\n", i+1, s.Name, s.Bytes, s.Lines)
	}
	for i, c := range image.Constants {
		fmt.Fprintf(&b, "const %d: %s\n", i, constantText(c))
	}
	for fi, f := range image.Functions {
		fmt.Fprintf(&b, "func %d %q params=%d locals=%d registers=%d maxstack=%d\n", fi, f.Name, f.Params, f.Locals, len(f.Registers), f.MaxStack)
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
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}
