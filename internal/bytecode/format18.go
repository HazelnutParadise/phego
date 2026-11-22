package bytecode

import "fmt"

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.8 adds
// closures. A closure's body is a function marked FuncClosure whose
// Captures list its use variables; both are immutable compiler data. The
// Closure object, the captured values, the bound $this and scope and the
// closure's static variables belong to the run that created them and are
// never part of a Program.

// CaptureFlags describes one captured variable.
type CaptureFlags uint8

const (
	// CaptureByRef is use (&$x): the closure holds a reference to the
	// variable and each call binds the local to it.
	CaptureByRef CaptureFlags = 1 << iota
	// CaptureImplicit is an arrow function's automatic capture: an
	// undefined variable is not reported when the closure is created, and
	// the local stays undefined (ZEND_BIND_IMPLICIT).
	CaptureImplicit

	captureFlagsKnown = CaptureByRef | CaptureImplicit
)

// Capture is one entry of a closure function's use list: the local of the
// closure's frame that receives the captured variable.
type Capture struct {
	Local uint32
	Flags CaptureFlags
}

// ReturnNotice (format 1.8) is the Aux of an OpReturn in a function declared
// to return by reference whose value is not a variable: PHP raises "Only
// variable references should be returned by reference" and returns the
// value (ZEND_RETURN_BY_REF with ZEND_RETURNS_VALUE).
const ReturnNotice uint16 = 1

// captureWire is the encoded size of one Capture: Local:u32, Flags:u8.
const captureWire = 5

// CaptureCount is the number of variables the closure function captures.
func (f FunctionView) CaptureCount() int {
	if f.function == nil {
		return 0
	}
	return len(f.function.Captures)
}

// CaptureAt returns capture index of the function's use list.
func (f FunctionView) CaptureAt(index int) (Capture, bool) {
	if f.function == nil || index < 0 || index >= len(f.function.Captures) {
		return Capture{}, false
	}
	return f.function.Captures[index], true
}

// verifyFunction18 checks the format 1.8 fields of a function: only a
// closure function has a use list, each capture names a distinct local, and
// a by-reference capture is never implicit.
func verifyFunction18(image Image, minor uint16, f *Function) error {
	if minor < 8 {
		if f.Flags&^functionFlagsKnown17 != 0 && f.Flags&^functionFlagsKnown == 0 || len(f.Captures) != 0 {
			return invalid(fmt.Sprintf("a closure function or a capture needs bytecode format 1.8, but the file declares format 1.%d", minor))
		}
		return nil
	}
	if f.Flags&FuncClosure == 0 {
		if len(f.Captures) != 0 {
			return invalid("only a closure function has captures")
		}
		if f.Flags&FuncUsesThis != 0 {
			return invalid("FuncUsesThis marks a closure function only")
		}
		return nil
	}
	if f.ReturnKind != Value {
		return invalid("a closure function returns a value")
	}
	if uint64(len(f.Captures)) > uint64(f.Locals) || len(f.Captures) > 0xffff+1 {
		return invalid("more captures than the closure function has locals")
	}
	// Locals is bounded by Limits.MaxLocals, so a bitmap is cheap.
	seen := make([]uint64, (uint64(f.Locals)+63)/64)
	for i, c := range f.Captures {
		if c.Local >= f.Locals {
			return invalid(fmt.Sprintf("capture %d names local %d of %d", i, c.Local, f.Locals))
		}
		if c.Flags&^captureFlagsKnown != 0 || c.Flags == CaptureByRef|CaptureImplicit {
			return invalid(fmt.Sprintf("capture %d has unknown or contradictory flags", i))
		}
		if seen[c.Local/64]&(1<<(c.Local%64)) != 0 {
			return invalid(fmt.Sprintf("capture %d names local %d twice", i, c.Local))
		}
		seen[c.Local/64] |= 1 << (c.Local % 64)
	}
	return nil
}

// checkOperands18 checks what the operand roles do not: the function an
// OpClosure or OpBindLexical names is a closure function, and the capture
// index is one of its use list.
func checkOperands18(image Image, in Instruction) error {
	switch in.Op {
	case OpClosure:
		if image.Functions[in.B].Flags&FuncClosure == 0 {
			return invalid("closure names a function that is not a closure function")
		}
	case OpBindLexical:
		fn := &image.Functions[in.C]
		if fn.Flags&FuncClosure == 0 || int(in.Aux) >= len(fn.Captures) {
			return invalid("bind-lexical names a capture the closure function does not have")
		}
	}
	return nil
}
