package bytecode

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Format 1.5 (W-17 stage 5: constants, @ and match): REQ-025; T-049,
// T-050. The frame is format12Image's: r0 Value (a defined parameter), r1
// Value (defined, an array), r2 Location (defined), r3 Value (undefined),
// r4 Location (undefined). The body starts at pc 3. format15Image adds the
// constants a constant instruction names: 1 is the string "X", 2 the int 1.

func format15Image(body ...Instruction) Image {
	i := format12Image(body...)
	i.Constants = append(i.Constants, Constant{Kind: String, Bytes: "X"}, Constant{Kind: Int, Bits: 1})
	return i
}

func format15ValidBodies() map[string][]Instruction {
	return map[string][]Instruction{
		"fetch constant":             {{Op: OpFetchConst, A: 3, B: 1, C: 1}},
		"fetch constant unqualified": {{Op: OpFetchConst, Aux: FetchConstUnqualified, A: 3, B: 1, C: 1}},
		"fetch constant then use":    {{Op: OpFetchConst, A: 3, B: 1, C: 1}, {Op: OpOutput, A: 3}},
		"declare constant":           {{Op: OpDeclareConst, A: 0, B: 1}},
		"silence":                    {{Op: OpBeginSilence, A: 3}, {Op: OpOutput, A: 0}, {Op: OpEndSilence, A: 3}},
		"match error":                {{Op: OpMatchError, A: 0}},
		"match error ends the flow":  {{Op: OpBranch, A: 0, B: format12Body + 1, C: format12Body + 2}, {Op: OpMatchError, A: 0}, {Op: OpReturn, A: 0}},
		"empty of an element":        {{Op: OpArrayIsset, Aux: ArrayIssetEmpty, A: 3, B: 1, C: 0}},
	}
}

func TestFormat15AcceptsValidOperands(t *testing.T) {
	for name, body := range format15ValidBodies() {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format15Image(body...)) })
	}
}

func TestFormat15RejectsInvalidOperands(t *testing.T) {
	cases := map[string][]Instruction{
		"fetch constant aux 2":                {{Op: OpFetchConst, Aux: 2, A: 3, B: 1, C: 1}},
		"fetch constant into a location":      {{Op: OpFetchConst, A: 2, B: 1, C: 1}},
		"fetch constant name not a string":    {{Op: OpFetchConst, A: 3, B: 2, C: 1}},
		"fetch constant display not a string": {{Op: OpFetchConst, A: 3, B: 1, C: 0}},
		"fetch constant name out of range":    {{Op: OpFetchConst, A: 3, B: 99, C: 1}},
		"declare constant aux":                {{Op: OpDeclareConst, Aux: 1, A: 0, B: 1}},
		"declare constant from a location":    {{Op: OpDeclareConst, A: 2, B: 1}},
		"declare constant undefined value":    {{Op: OpDeclareConst, A: 3, B: 1}},
		"declare constant name not a string":  {{Op: OpDeclareConst, A: 0, B: 2}},
		"declare constant c":                  {{Op: OpDeclareConst, A: 0, B: 1, C: 1}},
		"begin silence aux":                   {{Op: OpBeginSilence, Aux: 1, A: 3}},
		"begin silence location":              {{Op: OpBeginSilence, A: 2}},
		"begin silence b":                     {{Op: OpBeginSilence, A: 3, B: 1}},
		"end silence undefined":               {{Op: OpEndSilence, A: 3}},
		"end silence location":                {{Op: OpEndSilence, A: 2}},
		"match error undefined":               {{Op: OpMatchError, A: 3}},
		"match error aux":                     {{Op: OpMatchError, Aux: 1, A: 0}},
		"match error c":                       {{Op: OpMatchError, A: 0, C: 1}},
		"match error with pending arguments":  {{Op: OpPush, A: 0}, {Op: OpMatchError, A: 0}},
		"empty aux 2":                         {{Op: OpArrayIsset, Aux: 2, A: 3, B: 1, C: 0}},
		"empty into a location":               {{Op: OpArrayIsset, Aux: ArrayIssetEmpty, A: 2, B: 1, C: 0}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format15Image(body...), ErrInvalid) })
	}
	t.Run("a register defined only by a match error path is undefined after it", func(t *testing.T) {
		// 3 Branch r0, 4, 6; 4 FetchConst r3; 5 MatchError r0; 6 Output r3.
		requireBuildError(t, format15Image(
			Instruction{Op: OpBranch, A: 0, B: format12Body + 1, C: format12Body + 3},
			Instruction{Op: OpFetchConst, A: 3, B: 1, C: 1},
			Instruction{Op: OpMatchError, A: 0},
			Instruction{Op: OpOutput, A: 3},
		), ErrInvalid)
	})
}

func TestFormat15IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, body := range format15ValidBodies() {
		for _, minor := range []uint16{0, 1, 2, 3, 4} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format15Image(body...)
				if p, err := loadBytes(encodeAtMinor(t, image, 5)); err != nil || p == nil {
					t.Fatalf("minor 5 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				if !strings.Contains(e.Detail, "needs bytecode format 1.5") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want format 1.5 and the declared format", minor, e)
				}
			})
		}
	}
	t.Run("a format 1.4 file still loads and its instructions keep their meaning", func(t *testing.T) {
		image := format12Image(
			Instruction{Op: OpArrayIsset, A: 3, B: 1, C: 0},
			Instruction{Op: OpArrayGet, Aux: ArrayGetList, A: 3, B: 1, C: 0},
			Instruction{Op: OpMakeRef, A: 3, B: 2},
		)
		if p, err := loadBytes(encodeAtMinor(t, image, 4)); err != nil || p == nil {
			t.Fatalf("Load=(%v,%v)", p, err)
		}
	})
}

func TestFormat15Opcodes(t *testing.T) {
	// Five opcodes follow format 1.3's MakeRef; 49 is the first unknown one.
	if OpFetchConst != 44 || OpDeclareConst != 45 || OpBeginSilence != 46 || OpEndSilence != 47 || OpMatchError != 48 {
		t.Fatalf("format 1.5 opcodes are %d %d %d %d %d, want 44..48", OpFetchConst, OpDeclareConst, OpBeginSilence, OpEndSilence, OpMatchError)
	}
	if _, ok := Opcode(49).Safety(); ok {
		t.Fatal("opcode 49 is known")
	}
	for _, op := range []Opcode{OpFetchConst, OpDeclareConst, OpMatchError} {
		if s, ok := op.Safety(); !ok || !s.MayThrow || s.Capabilities != 0 {
			t.Fatalf("Safety(%d) = %+v, %v", op, s, ok)
		}
	}
	for _, op := range []Opcode{OpBeginSilence, OpEndSilence} {
		if s, ok := op.Safety(); !ok || s.MayThrow || s.Capabilities != 0 {
			t.Fatalf("Safety(%d) = %+v, %v", op, s, ok)
		}
	}
	if FormatMinor != 5 {
		t.Fatalf("FormatMinor = %d, want 5", FormatMinor)
	}
}
