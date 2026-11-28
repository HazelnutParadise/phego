package bytecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.10 adds the
// unwind table and the Leave that carries a value across finally blocks.

// format110Image is {main} with Value registers r0..r3 and the handlers and
// unwind table given. r0 is defined by the first instruction.
func format110Image(handlers []Handler, unwinds []Unwind, code ...Instruction) Image {
	image := format19Image(handlers, code...)
	f := &image.Functions[0]
	f.Registers = []RegisterKind{Value, Value, Value, Value}
	f.Unwinds = unwinds
	return image
}

// carryThroughFinally is a return of r0 that runs a finally block whose
// code reuses r0: 1 Leave carrying r0 to 4, 2-3 the finally block, 4
// Return r0.
func carryThroughFinally() Image {
	return format110Image([]Handler{{Start: 1, End: 2, Target: 2, BodyEnd: 4, Kind: Finally}}, nil,
		Instruction{Op: OpLeave, Aux: LeaveCarry, A: 4, C: 0},
		Instruction{Op: OpConst, A: 0, B: 0}, Instruction{Op: OpEndFinally},
		Instruction{Op: OpReturn, A: 0})
}

// droppedInFinally defines r1 before a try whose finally block drops it,
// and reads it after a Leave with the aux given: 2 Leave to 5, 3 Drop r1,
// 4 EndFinally, 5 Return r1.
func droppedInFinally(aux uint16) Image {
	leave := Instruction{Op: OpLeave, Aux: aux, A: 5}
	if aux == LeaveCarry {
		leave.C = 1
	}
	return format110Image([]Handler{{Start: 2, End: 3, Target: 3, BodyEnd: 5, Kind: Finally}}, nil,
		Instruction{Op: OpConst, A: 1, B: 0}, leave,
		Instruction{Op: OpDrop, A: 1}, Instruction{Op: OpEndFinally},
		Instruction{Op: OpReturn, A: 1})
}

// callTable is a call made at 4 whose arguments r1 and r2 are evaluated at
// 1 and 2, with a pending call started inside it on [2, 3).
func callTable() Image {
	return format110Image(nil, []Unwind{
		{Kind: UnwindCall, Start: 1, End: 4, Reg: 1, Count: 0},
		{Kind: UnwindCall, Start: 1, End: 4, Reg: 2, Count: 1},
		{Kind: UnwindPendingCall, Start: 2, End: 3},
		{Kind: UnwindSplit, Start: 3, Reg: 3, Count: 0},
	},
		Instruction{Op: OpConst, A: 1, B: 0}, Instruction{Op: OpConst, A: 2, B: 0},
		Instruction{Op: OpCheckpoint}, Instruction{Op: OpConst, A: 3, B: 0},
		Instruction{Op: OpReturn, A: 0})
}

func TestFormat110AcceptsAndRoundTrips(t *testing.T) {
	for name, image := range map[string]Image{"carry": carryThroughFinally(), "unwind table": callTable(), "carry over a drop": droppedInFinally(LeaveCarry)} {
		t.Run(name, func(t *testing.T) {
			p, err := Build(context.Background(), image, testOptions())
			if err != nil || p == nil {
				t.Fatalf("Build=(%v,%v)", p, err)
			}
			data := encodeAtMinor(t, image, 10)
			q, err := loadBytes(data)
			if err != nil || q == nil {
				t.Fatalf("minor 10 Load=(%v,%v)", q, err)
			}
			view, _ := q.Function(0)
			if view.UnwindCount() != len(image.Functions[0].Unwinds) {
				t.Fatalf("UnwindCount = %d, want %d", view.UnwindCount(), len(image.Functions[0].Unwinds))
			}
			for i, want := range image.Functions[0].Unwinds {
				if got, ok := view.UnwindAt(i); !ok || got != want {
					t.Fatalf("UnwindAt(%d) = %+v, %v; want %+v", i, got, ok, want)
				}
			}
			again, err := q.Encode(context.Background())
			if err != nil || string(again) != string(data) {
				t.Fatalf("canonical re-encode differs: err=%v", err)
			}
		})
	}
}

func TestFormat110IsGatedByTheDeclaredMinor(t *testing.T) {
	for minor := uint16(0); minor < 10; minor++ {
		// The Aux of a Leave is part of every instruction's encoding, so an
		// older file can carry it.
		t.Run(fmt.Sprintf("minor %d rejects a carrying Leave", minor), func(t *testing.T) {
			p, err := loadBytes(encodeAtMinor(t, carryThroughFinally(), minor))
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "instruction needs bytecode format 1.10") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
				t.Fatalf("Load=(%v,%v), want ErrInvalid naming 1.10 and 1.%d", p, err, minor)
			}
		})
		// An older file has no room for the table; the verifier still
		// refuses one in an image declared older.
		t.Run(fmt.Sprintf("minor %d rejects an unwind table", minor), func(t *testing.T) {
			b, err := newBudget(context.Background(), testOptions().Limits)
			if err != nil {
				t.Fatal(err)
			}
			_, err = verifyImage(b, callTable(), minor, false)
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "an unwind table needs bytecode format 1.10") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
				t.Fatalf("verify = %v, want ErrInvalid naming 1.10 and 1.%d", err, minor)
			}
		})
	}
}

func TestFormat110VerifierRejects(t *testing.T) {
	mod := func(f func(*Image)) Image {
		image := callTable()
		f(&image)
		return image
	}
	cases := map[string]struct {
		image  Image
		detail string
	}{
		"out of order": {mod(func(i *Image) {
			u := i.Functions[0].Unwinds
			u[2], u[3] = u[3], u[2]
		}), "out of order"},
		"empty range":                    {mod(func(i *Image) { i.Functions[0].Unwinds[2].End = 2 }), "invalid range"},
		"range past code":                {mod(func(i *Image) { i.Functions[0].Unwinds[0].End = 99 }), "invalid range"},
		"overlapping calls":              {mod(func(i *Image) { i.Functions[0].Unwinds[2].End = 5 }), "overlaps a call range"},
		"pending call with a register":   {mod(func(i *Image) { i.Functions[0].Unwinds[2].Reg = 1 }), "unused operands"},
		"argument outside the registers": {mod(func(i *Image) { i.Functions[0].Unwinds[1].Reg = 9 }), "not a Value"},
		"argument in a Location": {mod(func(i *Image) {
			i.Functions[0].Registers[2] = Location
			i.Functions[0].Code[1] = Instruction{Op: OpCheckpoint}
		}), "not a Value"},
		"split past code":   {mod(func(i *Image) { i.Functions[0].Unwinds[3].Start = 99 }), "invalid split"},
		"split with an end": {mod(func(i *Image) { i.Functions[0].Unwinds[3].End = 4 }), "invalid split"},
		"unknown kind":      {mod(func(i *Image) { i.Functions[0].Unwinds[3].Kind = 9 }), "unknown kind"},
		"carry of an undefined register": {format110Image([]Handler{{Start: 1, End: 2, Target: 2, BodyEnd: 4, Kind: Finally}}, nil,
			Instruction{Op: OpLeave, Aux: LeaveCarry, A: 4, C: 2},
			Instruction{Op: OpConst, A: 0, B: 0}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 0}), "read before definition"},
		"carry with an operand B": {format110Image(nil, nil,
			Instruction{Op: OpLeave, Aux: LeaveCarry, A: 2, B: 1, C: 0},
			Instruction{Op: OpReturn, A: 0}), "invalid leave operands"},
		"unknown leave aux": {format110Image(nil, nil,
			Instruction{Op: OpLeave, Aux: 2, A: 2},
			Instruction{Op: OpReturn, A: 0}), "invalid leave operands"},
		// Two Leaves to 5, one carrying r0 and one not: the continuation
		// could not tell whether r0 is defined.
		"leaves to one target disagree": {format110Image([]Handler{{Start: 1, End: 3, Target: 3, BodyEnd: 5, Kind: Finally}}, nil,
			Instruction{Op: OpLeave, Aux: LeaveCarry, A: 5, C: 0}, Instruction{Op: OpLeave, A: 5},
			Instruction{Op: OpCheckpoint}, Instruction{Op: OpEndFinally},
			Instruction{Op: OpReturn, A: 0}), "carry different registers"},
		// The finally block drops r1: without the carry the read at the
		// target is rejected (carriedThroughDrop accepts it with one).
		"dropped value read without the carry": {droppedInFinally(0), "read before definition"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Build(context.Background(), tc.image, testOptions())
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Build=(%v,%v), want ErrInvalid with %q", p, err, tc.detail)
			}
		})
	}
}

// genImage is a generator function {main} with Value registers r0..r3.
func genImage(code ...Instruction) Image {
	image := format110Image(nil, nil, code...)
	image.Functions[0].Flags |= FuncGenerator
	return image
}

func TestFormat110GeneratorCreation(t *testing.T) {
	if p, err := Build(context.Background(), genImage(
		Instruction{Op: OpGeneratorCreate},
		Instruction{Op: OpYield, Aux: uint16(YieldValue), A: 1, B: 0},
		Instruction{Op: OpReturn, A: 0}), testOptions()); err != nil || p == nil {
		t.Fatalf("Build = (%v, %v)", p, err)
	}
	cases := map[string]struct {
		image  Image
		detail string
	}{
		"yield before the creation": {genImage(
			Instruction{Op: OpYield, A: 1}, Instruction{Op: OpGeneratorCreate}, Instruction{Op: OpReturn, A: 0}),
			"yields before it creates"},
		"a jump over the creation": {genImage(
			Instruction{Op: OpJump, A: 3}, Instruction{Op: OpGeneratorCreate}, Instruction{Op: OpReturn, A: 0}),
			"bypasses or reenters"},
		"a jump back to the creation": {genImage(
			Instruction{Op: OpGeneratorCreate}, Instruction{Op: OpJump, A: 1}, Instruction{Op: OpReturn, A: 0}),
			"bypasses or reenters"},
		"two creations": {genImage(
			Instruction{Op: OpGeneratorCreate}, Instruction{Op: OpGeneratorCreate}, Instruction{Op: OpReturn, A: 0}),
			"creates its generator 2 times"},
		"a yield outside a generator": {format110Image(nil, nil,
			Instruction{Op: OpYield, A: 1}, Instruction{Op: OpReturn, A: 0}),
			"only a generator function"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Build(context.Background(), tc.image, testOptions())
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Build=(%v,%v), want ErrInvalid with %q", p, err, tc.detail)
			}
		})
	}
}
