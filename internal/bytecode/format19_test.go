package bytecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.9 adds no
// instruction: it lets a register that is defined before every instruction
// of a protected range stay defined in the handler, and lets a Leave carry
// the registers that are still defined when the last finally block it runs
// ends. PHP needs both: a foreach whose body has a try keeps its iterator
// across the catch and across a continue that runs a finally block.

// format19Image is {main} with registers r0..r2 (Values) and the handlers
// given. r0 is defined by the first instruction.
func format19Image(handlers []Handler, code ...Instruction) Image {
	code = append([]Instruction{{Op: OpConst, A: 0, B: 0}}, code...)
	return Image{Identity: testIdentity(), Constants: []Constant{{Kind: Null}}, Functions: []Function{{
		Name: "main", ReturnKind: Value, Registers: []RegisterKind{Value, Value, Value},
		Code: code, Positions: make([]Position, len(code)), Handlers: handlers,
	}}}
}

// format19Cases are programs only the 1.9 rules accept.
func format19Cases() map[string]Image {
	return map[string]Image{
		// 1: try body, 2: Leave, 3: catch (r1 = exception), 4: after.
		"register read in a catch block": format19Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 4, Register: 1, Kind: Catch}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 4},
			Instruction{Op: OpLeave, A: 4},
			Instruction{Op: OpReturn, A: 0}),
		// The catch block itself reads r0.
		"register read inside the catch block": format19Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 5, Register: 1, Kind: Catch}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpMove, A: 2, B: 0}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpReturn, A: 0}),
		// 1: try body, 2: Leave, 3: finally, 4: EndFinally, 5: after.
		"register read after a finally block": format19Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 5, Kind: Finally}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 0}),
		"register read inside a finally block": format19Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 5, Kind: Finally}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpMove, A: 2, B: 0}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 0}),
		// Two nested finally blocks; the Leave at 2 runs both.
		"leave through two finally blocks": format19Image([]Handler{
			{Start: 1, End: 5, Target: 5, BodyEnd: 7, Kind: Finally},
			{Start: 1, End: 3, Target: 3, BodyEnd: 5, Kind: Finally}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 7},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 0}),
	}
}

func TestFormat19KeepsRegistersAcrossHandlers(t *testing.T) {
	for name, image := range format19Cases() {
		t.Run(name, func(t *testing.T) {
			p, err := Build(context.Background(), image, testOptions())
			if err != nil || p == nil {
				t.Fatalf("Build=(%v,%v)", p, err)
			}
			if q, err := loadBytes(encodeAtMinor(t, image, 9)); err != nil || q == nil {
				t.Fatalf("minor 9 Load=(%v,%v)", q, err)
			}
		})
	}
}

func TestFormat19IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, image := range format19Cases() {
		for minor := uint16(0); minor < 9; minor++ {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				if !strings.Contains(e.Detail, "needs bytecode format 1.9") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want the needed and the declared format", minor, e)
				}
			})
		}
	}
	// A handler that defines its registers again is valid in every format.
	for minor := uint16(0); minor <= 9; minor++ {
		if p, err := loadBytes(encodeAtMinor(t, validImageWithHandler(), minor)); err != nil || p == nil {
			t.Fatalf("minor %d Load of a 1.0 handler=(%v,%v)", minor, p, err)
		}
	}
}

func TestFormat19RejectsUndefinedRegisters(t *testing.T) {
	bad := map[string]Image{
		// r2 is defined inside the range only: not at every instruction.
		"register defined inside the range": format19Image([]Handler{{Start: 1, End: 4, Target: 4, BodyEnd: 5, Register: 1, Kind: Catch}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpConst, A: 2, B: 0}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpReturn, A: 2},
			Instruction{Op: OpReturn, A: 0}),
		// The exception register of a catch is not defined after a finally.
		"register defined by the finally block's try only": format19Image([]Handler{{Start: 1, End: 4, Target: 4, BodyEnd: 6, Kind: Finally}},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpConst, A: 2, B: 0}, Instruction{Op: OpLeave, A: 6},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 2}),
		// A Drop inside the range clears r0 for the handler.
		"register dropped inside the range": format19Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 4, Register: 1, Kind: Catch}},
			Instruction{Op: OpDrop, A: 0}, Instruction{Op: OpLeave, A: 4},
			Instruction{Op: OpReturn, A: 0},
			Instruction{Op: OpReturn, A: 1}),
	}
	for name, image := range bad {
		t.Run(name, func(t *testing.T) {
			p, err := Build(context.Background(), image, testOptions())
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "read before definition") {
				t.Fatalf("Build=(%v,%v), want a register read before definition", p, err)
			}
		})
	}
}

// An EndFinally goes on like a Leave from its own pc, as the VM runs it.
// One placed inside another finally range that starts later in its body
// continues through that block, whose EndFinally then reaches the target:
// the data flow follows the same path, so the register the second block
// dropped is not defined there (found by the stage 7C review, where the
// continuation was worked out from the handler's target instead).
func TestFormat19FollowsEndFinallyThroughALaterFinallyRange(t *testing.T) {
	image := format19Image([]Handler{
		{Start: 1, End: 3, Target: 3, BodyEnd: 9, Kind: Finally},
		{Start: 4, End: 6, Target: 6, BodyEnd: 8, Kind: Finally}},
		Instruction{Op: OpCheckpoint}, Instruction{Op: OpLeave, A: 9},
		Instruction{Op: OpCheckpoint},
		Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
		Instruction{Op: OpDrop, A: 0}, Instruction{Op: OpEndFinally},
		Instruction{Op: OpReturn, A: 0},
		Instruction{Op: OpReturn, A: 0})
	p, err := Build(context.Background(), image, testOptions())
	var e *Error
	if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || e.PC != 9 || !strings.Contains(e.Detail, "read before definition") {
		t.Fatalf("Build=(%v,%v), want the read of the dropped register at 9 refused", p, err)
	}
}
