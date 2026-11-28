package bytecode

import "fmt"

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.10 tells the
// VM what PHP frees when an exception or exit unwinds a frame, in PHP's
// order (cleanup_unfinished_calls, then cleanup_live_vars), and lets a
// return carry its value across finally blocks the way PHP's FAST_CALL
// keeps it in a temporary (zend_handle_loops_and_finally).

// UnwindKind is the kind of an entry of a function's unwind table.
type UnwindKind uint8

const (
	// UnwindCall is argument Count of a call whose arguments are
	// evaluated into registers before the instruction at End makes it:
	// register Reg holds it. On [Start, End) the call is unfinished, and
	// the unwinding frees its arguments, in order, before the
	// temporaries, innermost call first. The entries of one call share
	// Start and End and come in the order of their arguments.
	UnwindCall UnwindKind = iota + 1
	// UnwindPendingCall is a call an InitCall, InitMethod, InitStatic or
	// New started and whose arguments go to the argument stack: on
	// [Start, End) it is unfinished, and the unwinding frees its arguments
	// in the same order as an UnwindCall's. Reg and Count are zero.
	UnwindPendingCall
	// UnwindSplit is a return at Start that frees the loop variable in
	// register Reg on its way out (an iterator, or the subject of a
	// switch). PHP's live range of that variable starts again after the
	// return (ZEND_FREE_ON_RETURN), so code after Start that unwinds frees
	// it in that order: Count is the variable's position among the ones
	// the return frees, innermost first. End is zero.
	UnwindSplit

	unwindKindLast = UnwindSplit
)

// Unwind is one entry of a function's unwind table (format 1.10).
type Unwind struct {
	Kind       UnwindKind
	Start, End uint32
	Reg, Count uint32
}

// unwindWire is the encoded size of one Unwind: Kind:u8, Start, End, Reg,
// Count:u32.
const unwindWire = 17

// YieldFlags is the Aux of OpYield (format 1.10).
type YieldFlags uint16

const (
	// YieldValue: B holds the value yielded; without it the value is null.
	YieldValue YieldFlags = 1 << iota
	// YieldKey: C holds the key; without it the generator numbers the
	// value with its next integer key.
	YieldKey
	// YieldRef: B holds a reference, which a generator declared to yield
	// by reference hands out (MakeRef or a by-reference fetch made it).
	YieldRef
	// YieldNotice: a generator declared to yield by reference yields a
	// value that is not a variable: PHP raises "Only variable references
	// should be yielded by reference" and yields the value.
	YieldNotice

	yieldFlagsKnown = YieldValue | YieldKey | YieldRef | YieldNotice
)

// LeaveCarry (format 1.10) is the Aux of an OpLeave that carries the value
// of register C to its target: when the Leave runs finally blocks, the
// value waits in the completion they suspend instead of in the register,
// whose number their code may reuse, and is written back to C at the
// target. A finally block that ends the completion otherwise (a return, a
// throw, a jump of its own) frees it. Without finally blocks to run the
// Leave is a jump and the value stays in C.
const LeaveCarry uint16 = 1

// UnwindCount is the number of entries of the function's unwind table.
func (f FunctionView) UnwindCount() int {
	if f.function == nil {
		return 0
	}
	return len(f.function.Unwinds)
}

// UnwindAt returns entry index of the function's unwind table.
func (f FunctionView) UnwindAt(index int) (Unwind, bool) {
	if f.function == nil || index < 0 || index >= len(f.function.Unwinds) {
		return Unwind{}, false
	}
	return f.function.Unwinds[index], true
}

// verifyFunction110 checks a function's unwind table: entries are sorted by
// Start, name instructions and Value registers of the function, and the
// unfinished ranges of calls nest or are disjoint, so that the innermost
// unfinished call is the one that started last.
func verifyFunction110(minor uint16, f *Function) error {
	if minor < 10 {
		if len(f.Unwinds) != 0 {
			return invalid(fmt.Sprintf("an unwind table needs bytecode format 1.10, but the file declares format 1.%d", minor))
		}
		if f.Flags&(FuncGenerator|FuncByRef) != 0 {
			return invalid(fmt.Sprintf("a generator function or a function returning by reference needs bytecode format 1.10, but the file declares format 1.%d", minor))
		}
		return nil
	}
	if f.Flags&FuncGenerator != 0 {
		// The one place the call returns the Generator, outside every
		// protected range: PHP's GENERATOR_CREATE follows the RECVs.
		creates := 0
		for pc, in := range f.Code {
			if in.Op != OpGeneratorCreate {
				continue
			}
			creates++
			for _, h := range f.Handlers {
				if uint32(pc) >= h.Start && uint32(pc) < h.BodyEnd {
					return invalid("a generator is created inside an exception handler's range")
				}
			}
		}
		if creates != 1 {
			return invalid(fmt.Sprintf("a generator function creates its generator %d times, want once", creates))
		}
		// Every path reaches the creation once and only then the body: no
		// edge, handler or yield crosses it except falling through.
		c := uint32(0)
		for pc, in := range f.Code {
			if in.Op == OpGeneratorCreate {
				c = uint32(pc)
			}
		}
		n := uint32(len(f.Code))
		for pc, in := range f.Code {
			p := uint32(pc)
			if (in.Op == OpYield || in.Op == OpYieldFrom) && p < c {
				return invalid("a generator function yields before it creates its generator")
			}
			if (in.Op == OpReturn || in.Op == OpReturnRef) && p < c {
				return invalid("a generator function returns before it creates its generator")
			}
			for k := 0; k < successorCount(in, p, n); k++ {
				t := successor(in, p, n, k).pc
				if p < c && t > c || p > c && t <= c {
					return invalid("control flow bypasses or reenters the creation of the generator")
				}
			}
		}
		for _, h := range f.Handlers {
			before := h.Start < c
			if h.End > c && before || h.Target < c != before || h.BodyEnd > c && h.Target < c {
				return invalid("an exception handler crosses the creation of the generator")
			}
		}
		if f.ReturnKind != Value {
			return invalid("a generator function returns a value")
		}
	}
	n, nr := uint64(len(f.Code)), uint64(len(f.Registers))
	// open holds the ends of the call ranges that contain the current
	// Start, innermost last.
	var open []uint32
	var last uint32
	for i, u := range f.Unwinds {
		if i > 0 && u.Start < last {
			return invalid(fmt.Sprintf("unwind entry %d is out of order", i))
		}
		last = u.Start
		switch u.Kind {
		case UnwindCall, UnwindPendingCall:
			if u.Start >= u.End || uint64(u.End) > n {
				return invalid(fmt.Sprintf("unwind entry %d has an invalid range", i))
			}
			if u.Kind == UnwindPendingCall && (u.Reg != 0 || u.Count != 0) {
				return invalid(fmt.Sprintf("unwind entry %d has unused operands", i))
			}
			if u.Kind == UnwindCall && (uint64(u.Reg) >= nr || f.Registers[u.Reg] != Value) {
				return invalid(fmt.Sprintf("unwind entry %d names a register that is not a Value of the function", i))
			}
			for len(open) > 0 && open[len(open)-1] <= u.Start {
				open = open[:len(open)-1]
			}
			if len(open) > 0 && u.End > open[len(open)-1] {
				return invalid(fmt.Sprintf("unwind entry %d overlaps a call range it does not nest in", i))
			}
			open = append(open, u.End)
		case UnwindSplit:
			if uint64(u.Start) >= n || u.End != 0 || uint64(u.Reg) >= nr || f.Registers[u.Reg] != Value || u.Count > 0xffff {
				return invalid(fmt.Sprintf("unwind entry %d is an invalid split", i))
			}
		default:
			return invalid(fmt.Sprintf("unwind entry %d has unknown kind %d", i, u.Kind))
		}
	}
	return nil
}
