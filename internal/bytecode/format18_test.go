package bytecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026. Format 1.8: closure
// functions, their captures and the four instructions that use them.

// format18Prefix is the number of instructions before a test body.
const format18Prefix = 3

// format18Image has {main} (r0 and r1 Values holding null, r2 the Location
// of its only local, r3 an undefined Value), closure function 1 with two
// captures, and the plain function 2.
func format18Image(body ...Instruction) Image {
	code := append([]Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpConst, A: 1, B: 0}, {Op: OpLocal, A: 2, B: 0}}, body...)
	code = append(code, Instruction{Op: OpReturn, A: 0})
	leaf := func(name string) Function {
		return Function{Name: name, ReturnKind: Value, Registers: []RegisterKind{Value},
			Code: []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Positions: make([]Position, 2)}
	}
	closure := leaf("{closure:/w/t.php:2}")
	closure.Flags = FuncClosure | FuncUsesThis
	closure.Locals, closure.LocalNames = 2, []uint32{1, 1}
	closure.Captures = []Capture{{Local: 1}, {Local: 0, Flags: CaptureByRef}}
	return Image{Identity: testIdentity(), Constants: []Constant{{Kind: Null}, {Kind: String, Bytes: "v"}}, Functions: []Function{
		{Name: "main", ReturnKind: Value, Locals: 1, LocalNames: []uint32{1}, MaxStack: 2, Flags: FuncNoThis,
			Registers: []RegisterKind{Value, Value, Location, Value}, Code: code, Positions: make([]Position, len(code))},
		closure, leaf("plain"),
	}}
}

func format18ValidBodies() map[string][]Instruction {
	return map[string][]Instruction{
		"closure":            {{Op: OpClosure, A: 3, B: 1}},
		"closure with binds": {{Op: OpClosure, A: 3, B: 1}, {Op: OpBindLexical, Aux: 0, A: 3, B: 2, C: 1}, {Op: OpBindLexical, Aux: 1, A: 3, B: 2, C: 1}},
		"convert a function": {{Op: OpInitCall, B: 2}, {Op: OpCallableConvert, A: 3}},
		"convert a callable": {{Op: OpInitMethod, Aux: InitCallable, A: 0}, {Op: OpCallableConvert, A: 3}},
		"argument mode":      {{Op: OpInitMethod, Aux: InitCallable, A: 0}, {Op: OpArgByRef, A: 3}, {Op: OpPush, A: 0}, {Op: OpCallPending, A: 3}},
	}
}

func TestFormat18AcceptsAndRoundTrips(t *testing.T) {
	for name, body := range format18ValidBodies() {
		t.Run(name, func(t *testing.T) {
			p := requireValidBuild(t, format18Image(body...))
			data, err := p.Encode(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			q, err := loadBytes(data)
			if err != nil || q == nil {
				t.Fatalf("Load=(%v,%v)", q, err)
			}
			again, err := q.Encode(context.Background())
			if err != nil || string(again) != string(data) {
				t.Fatalf("re-encode differs: %v", err)
			}
			view, _ := q.Function(1)
			c0, ok0 := view.CaptureAt(0)
			c1, ok1 := view.CaptureAt(1)
			_, ok2 := view.CaptureAt(2)
			if view.Flags() != FuncClosure|FuncUsesThis || view.CaptureCount() != 2 || !ok0 || !ok1 || ok2 || c0 != (Capture{Local: 1}) || c1 != (Capture{Local: 0, Flags: CaptureByRef}) {
				t.Fatalf("loaded closure function: flags %d captures %d %+v %+v", view.Flags(), view.CaptureCount(), c0, c1)
			}
			if plain, _ := q.Function(2); plain.CaptureCount() != 0 || plain.Flags() != 0 {
				t.Fatalf("plain function has flags %d and %d captures", plain.Flags(), plain.CaptureCount())
			}
		})
	}
	t.Run("return notice", func(t *testing.T) {
		image := format18Image()
		image.Functions[1].Code[1].Aux = ReturnNotice
		requireValidBuild(t, image)
	})
}

func TestFormat18RejectsInvalidInstructions(t *testing.T) {
	cases := map[string][]Instruction{
		"closure of a plain function":      {{Op: OpClosure, A: 3, B: 2}},
		"closure of a missing function":    {{Op: OpClosure, A: 3, B: 9}},
		"closure aux":                      {{Op: OpClosure, Aux: 1, A: 3, B: 1}},
		"closure into a location":          {{Op: OpClosure, A: 2, B: 1}},
		"closure c":                        {{Op: OpClosure, A: 3, B: 1, C: 1}},
		"bind capture out of range":        {{Op: OpClosure, A: 3, B: 1}, {Op: OpBindLexical, Aux: 2, A: 3, B: 2, C: 1}},
		"bind for a plain function":        {{Op: OpClosure, A: 3, B: 1}, {Op: OpBindLexical, A: 3, B: 2, C: 2}},
		"bind for a missing function":      {{Op: OpClosure, A: 3, B: 1}, {Op: OpBindLexical, A: 3, B: 2, C: 9}},
		"bind from a value register":       {{Op: OpClosure, A: 3, B: 1}, {Op: OpBindLexical, A: 3, B: 0, C: 1}},
		"bind on an undefined register":    {{Op: OpBindLexical, A: 3, B: 2, C: 1}},
		"bind on a location":               {{Op: OpBindLexical, A: 2, B: 2, C: 1}},
		"convert without a started call":   {{Op: OpCallableConvert, A: 3}},
		"convert after an argument":        {{Op: OpInitCall, B: 2}, {Op: OpPush, A: 0}, {Op: OpCallableConvert, A: 3}},
		"convert aux":                      {{Op: OpInitCall, B: 2}, {Op: OpCallableConvert, Aux: 1, A: 3}},
		"convert into a location":          {{Op: OpInitCall, B: 2}, {Op: OpCallableConvert, A: 2}},
		"a converted call is gone":         {{Op: OpInitCall, B: 2}, {Op: OpCallableConvert, A: 3}, {Op: OpCallPending, A: 3}},
		"argument mode outside a call":     {{Op: OpArgByRef, A: 3}},
		"argument mode aux":                {{Op: OpInitCall, B: 2}, {Op: OpArgByRef, Aux: 1, A: 3}, {Op: OpCallPending, A: 3}},
		"argument mode b":                  {{Op: OpInitCall, B: 2}, {Op: OpArgByRef, A: 3, B: 1}, {Op: OpCallPending, A: 3}},
		"direct call of a closure":         {{Op: OpCall, A: 3, B: 1, C: 0}},
		"started call of a closure":        {{Op: OpInitCall, B: 1}, {Op: OpCallPending, A: 3}},
		"closure result read as undefined": {{Op: OpOutput, A: 3}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format18Image(body...), ErrInvalid) })
	}
	t.Run("return aux 2", func(t *testing.T) {
		image := format18Image()
		image.Functions[1].Code[1].Aux = ReturnNotice + 1
		requireBuildError(t, image, ErrInvalid)
	})
}

func TestFormat18RejectsInvalidFunctions(t *testing.T) {
	cases := map[string]struct {
		edit   func(*Image)
		detail string
	}{
		"captures of a plain function": {func(i *Image) {
			i.Functions[2].Locals, i.Functions[2].LocalNames, i.Functions[2].Captures = 1, []uint32{1}, []Capture{{}}
		}, "only a closure function has captures"},
		"uses-this on a plain function": {func(i *Image) { i.Functions[2].Flags = FuncUsesThis }, "FuncUsesThis marks a closure function only"},
		"capture local out of range":    {func(i *Image) { i.Functions[1].Captures[0].Local = 2 }, "names local 2 of 2"},
		"capture local twice":           {func(i *Image) { i.Functions[1].Captures[0].Local = 0 }, "names local 0 twice"},
		"more captures than locals":     {func(i *Image) { i.Functions[1].Captures = append(i.Functions[1].Captures, Capture{}) }, "more captures"},
		"unknown capture flag":          {func(i *Image) { i.Functions[1].Captures[0].Flags = CaptureImplicit << 1 }, "unknown or contradictory flags"},
		"implicit by reference":         {func(i *Image) { i.Functions[1].Captures[0].Flags = CaptureByRef | CaptureImplicit }, "unknown or contradictory flags"},
		"unknown function flag":         {func(i *Image) { i.Functions[1].Flags |= FuncUsesThis << 1 }, "unknown function flags"},
		"the entry is a closure":        {func(i *Image) { i.Entry = 1 }, "entry function is a closure function"},
		"a method is a closure": {func(i *Image) {
			i.Classes = []Class{{Name: 1, Methods: []Method{{Name: 1, Function: 1}}}}
		}, "class member's function is a closure function"},
		"a constant initializer is a closure": {func(i *Image) {
			i.Functions[1].Captures, i.Functions[1].Locals, i.Functions[1].LocalNames = nil, 0, nil
			i.Classes = []Class{{Name: 1, Constants: []ClassConstant{{Name: 1, Init: 2}}}}
		}, "class member's function is a closure function"},
		"a property initializer is a closure": {func(i *Image) {
			i.Functions[1].Captures, i.Functions[1].Locals, i.Functions[1].LocalNames = nil, 0, nil
			i.Classes = []Class{{Name: 1, Properties: []Property{{Name: 1, Init: 2}}}}
		}, "class member's function is a closure function"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			image := format18Image()
			tc.edit(&image)
			_, err := Build(context.Background(), image, testOptions())
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Build = %v, want ErrInvalid with %q", err, tc.detail)
			}
		})
	}
}

func TestFormat18IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, body := range format18ValidBodies() {
		for minor := uint16(0); minor < 8; minor++ {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format18Image(body...)
				if p, err := loadBytes(encodeAtMinor(t, image, 8)); err != nil || p == nil {
					t.Fatalf("minor 8 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				// Below 1.6 the Push and InitCall of some bodies are
				// reported first, as their own formats.
				if !strings.Contains(e.Detail, "needs bytecode format 1.") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want the needed and the declared format", minor, e)
				}
				if minor == 7 && !strings.Contains(e.Detail, "needs bytecode format 1.8") {
					t.Fatalf("minor 7 error = %+v, want format 1.8", e)
				}
			})
		}
	}
	t.Run("a by-reference return notice needs 1.8", func(t *testing.T) {
		image := validImage()
		image.Functions[0].Code[1].Aux = ReturnNotice
		p, err := loadBytes(encodeAtMinor(t, image, 7))
		var e *Error
		if p != nil || !errors.As(err, &e) || !strings.Contains(e.Detail, "needs bytecode format 1.8") {
			t.Fatalf("Load=(%v,%v), want format 1.8", p, err)
		}
		if p, err := loadBytes(encodeAtMinor(t, image, 8)); err != nil || p == nil {
			t.Fatalf("minor 8 Load=(%v,%v)", p, err)
		}
	})
	t.Run("a format 1.7 file without closures still loads", func(t *testing.T) {
		image := format18Image()
		image.Functions[1].Flags, image.Functions[1].Captures = 0, nil
		for minor := uint16(1); minor < 8; minor++ {
			if minor < 7 {
				image.Functions[0].Flags = 0
			}
			if p, err := loadBytes(encodeAtMinor(t, image, minor)); err != nil || p == nil {
				t.Fatalf("minor %d Load=(%v,%v)", minor, p, err)
			}
		}
	})
}

func TestFormat18Opcodes(t *testing.T) {
	if OpClosure != 76 || OpBindLexical != 77 || OpCallableConvert != 78 || OpArgByRef != 79 {
		t.Fatalf("format 1.8 opcodes are %d %d %d %d, want 76..79", OpClosure, OpBindLexical, OpCallableConvert, OpArgByRef)
	}
	for _, op := range []Opcode{OpClosure, OpBindLexical, OpCallableConvert, OpArgByRef} {
		if s, ok := op.Safety(); !ok || s != (Safety{Steps: 1, CheckCancel: true, MayAllocate: true, MayThrow: true, MayCallUser: true}) {
			t.Fatalf("Safety(%d) = %+v, %v", op, s, ok)
		}
	}
	if FormatMinor != 9 {
		t.Fatalf("FormatMinor = %d, want 9", FormatMinor)
	}
}
