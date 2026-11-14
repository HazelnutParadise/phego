package bytecode

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Format 1.4 (W-17 stage 4, destructuring): REQ-025; T-049, T-050. The
// frame is format12Image's: r0 Value (a defined parameter), r1 Value
// (defined, an array), r2 Location (defined), r3 Value (undefined), r4
// Location (undefined). The body starts at pc 3.

// makeRefThenFetch is "MakeRef r3, L2; FetchDim L4 = r3[r0]": the fetch of
// a destructuring assignment by reference from the reference its right-hand
// side became.
func makeRefThenFetch(flags FetchFlags, c uint32) []Instruction {
	return []Instruction{
		{Op: OpMakeRef, A: 3, B: 2},
		{Op: OpFetchDim, Aux: uint16(flags), A: 4, B: 3, C: c},
	}
}

func format14ValidBodies() map[string][]Instruction {
	ref := FetchWrite | FetchForRef | FetchContainerValue
	return map[string][]Instruction{
		"list read":                       {{Op: OpArrayGet, Aux: ArrayGetList, A: 3, B: 1, C: 0}},
		"fetch from a value":              append(makeRefThenFetch(ref, 0), Instruction{Op: OpMakeRef, A: 3, B: 4}),
		"fetch from a value, key a local": append(makeRefThenFetch(ref|FetchKeyLocation, 2), Instruction{Op: OpMakeRef, A: 3, B: 4}),
		"fetch from a value, append":      append(makeRefThenFetch(ref|FetchAppend, 0), Instruction{Op: OpMakeRef, A: 3, B: 4}),
		"fetch from a value then bind":    append(makeRefThenFetch(ref, 0), Instruction{Op: OpBind, A: 2, B: 4}),
	}
}

func TestFormat14AcceptsValidOperands(t *testing.T) {
	for name, body := range format14ValidBodies() {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format12Image(body...)) })
	}
}

func TestFormat14RejectsInvalidOperands(t *testing.T) {
	cases := map[string][]Instruction{
		"array get aux 3":                 {{Op: OpArrayGet, Aux: 3, A: 3, B: 1, C: 0}},
		"list read of a location":         {{Op: OpArrayGet, Aux: ArrayGetList, A: 3, B: 2, C: 0}},
		"value container without ref use": makeRefThenFetch(FetchWrite|FetchContainerValue, 0),
		"value container read-write":      makeRefThenFetch(FetchReadWrite|FetchForRef|FetchContainerValue, 0),
		"value container is a location":   {{Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef | FetchContainerValue), A: 4, B: 2, C: 0}},
		"value container undefined":       {{Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef | FetchContainerValue), A: 4, B: 3, C: 0}},
		"location container flagged off":  {{Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef), A: 4, B: 1, C: 0}},
		"unknown fetch flag":              makeRefThenFetch(FetchWrite|FetchForRef|FetchContainerValue|1<<7, 0),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format12Image(body...), ErrInvalid) })
	}
}

// The element fetched from a reference in a register is an element location
// like any other: it may be made a reference or bound right away, and used
// nowhere else.
func TestFormat14ElementLocations(t *testing.T) {
	ref := FetchWrite | FetchForRef | FetchContainerValue
	for name, body := range map[string][]Instruction{
		"made a reference after an output": append(makeRefThenFetch(ref, 0), Instruction{Op: OpOutput, A: 0}, Instruction{Op: OpMakeRef, A: 3, B: 4}),
		"loaded after the sequence":        append(makeRefThenFetch(ref, 0), Instruction{Op: OpMakeRef, A: 3, B: 4}, Instruction{Op: OpLoad, A: 0, B: 4}),
	} {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format12Image(body...), ErrInvalid) })
	}
}

func TestFormat14IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, body := range format14ValidBodies() {
		for _, minor := range []uint16{0, 1, 2, 3} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format12Image(body...)
				if p, err := loadBytes(encodeAtMinor(t, image, 4)); err != nil || p == nil {
					t.Fatalf("minor 4 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				// A body that starts with MakeRef is refused at that
				// instruction below minor 3; a minor 3 file names 1.4.
				need := "format 1.4"
				if minor < 3 && body[0].Op == OpMakeRef {
					need = "format 1.3"
				}
				if !strings.Contains(e.Detail, "needs bytecode "+need) || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want %s and the declared format", minor, e, need)
				}
			})
		}
	}
	t.Run("a format 1.3 file still loads and its instructions keep their meaning", func(t *testing.T) {
		image := format12Image(
			Instruction{Op: OpMakeRef, A: 3, B: 2},
			Instruction{Op: OpFetchDim, Aux: uint16(FetchWrite | FetchForRef), A: 4, B: 2, C: 0},
			Instruction{Op: OpBind, Aux: uint16(BindValue), A: 4, B: 3},
			Instruction{Op: OpArrayGet, Aux: ArrayGetQuiet, A: 3, B: 1, C: 0},
		)
		if p, err := loadBytes(encodeAtMinor(t, image, 3)); err != nil || p == nil {
			t.Fatalf("Load=(%v,%v)", p, err)
		}
	})
}

func TestFormat14AddsNoOpcode(t *testing.T) {
	// Destructuring is expressed with the existing instructions; the first
	// opcode after format 1.3's MakeRef is format 1.5's FetchConst (44).
	if OpFetchConst != 44 || OpMatchError != 48 {
		t.Fatalf("format 1.5 opcodes are %d..%d, want 44..48", OpFetchConst, OpMatchError)
	}
	if _, ok := Opcode(49).Safety(); ok {
		t.Fatalf("opcode 49 is known")
	}
	if s, ok := OpArrayGet.Safety(); !ok || !s.MayThrow {
		t.Fatalf("Safety(ArrayGet) = %+v, %v", s, ok)
	}
}
