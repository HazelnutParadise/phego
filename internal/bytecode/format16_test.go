package bytecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Format 1.6 (W-17 stage 6: global, static, variadic parameters, argument
// unpacking and named arguments): REQ-025; T-049, T-050. The frame is
// format12Image's: r0 Value (a defined parameter), r1 Value (defined, an
// array), r2 Location (defined), r3 Value (undefined), r4 Location
// (undefined). The body starts at pc 3. format16Image adds the string
// constant 1 ("x"), a parameter table that makes main variadic, and the
// function 1, callee($x, &$x), which records its parameters.

func format16Image(body ...Instruction) Image {
	i := format12Image(body...)
	i.Constants = append(i.Constants, Constant{Kind: String, Bytes: "x"}, Constant{Kind: Int, Bits: 1})
	i.Functions[0].MaxStack = 8
	i.Functions[0].Parameters = []Parameter{{Name: 1}, {Name: 1, Flags: ParamVariadic}}
	i.Functions = append(i.Functions, Function{
		Name: "callee", Params: 2, ReturnKind: Value, Registers: []RegisterKind{Value, Value},
		Code: []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Positions: make([]Position, 2),
		Parameters: []Parameter{{Name: 1}, {Name: 1, Flags: ParamByRef}},
	})
	return i
}

func format16ValidBodies() map[string][]Instruction {
	return map[string][]Instruction{
		"bind global":   {{Op: OpBindGlobal, A: 2, B: 1}},
		"static":        {{Op: OpStaticInit, A: 2, B: format12Body + 2}, {Op: OpBindStatic, A: 2, B: 1}},
		"recv variadic": {{Op: OpRecvVariadic, A: 3}, {Op: OpOutput, A: 3}},
		"pending call": {
			{Op: OpInitCall, B: 1},
			{Op: OpPush, A: 0},
			{Op: OpPush, A: 0, B: 2},
			{Op: OpPush, Aux: uint16(PushLoad), A: 2, B: 2},
			{Op: OpPush, A: 2, B: 2},
			{Op: OpSendUnpack, A: 1},
			{Op: OpSendUnpack, A: 2},
			{Op: OpCallPending, A: 3},
		},
		"nested pending calls": {
			{Op: OpInitCall, B: 1},
			{Op: OpInitCall, B: 0},
			{Op: OpSendUnpack, A: 1},
			{Op: OpCallPending, A: 3},
			{Op: OpPush, A: 3},
			{Op: OpCallPending, A: 3},
		},
		"push load to a direct call": {
			{Op: OpPush, Aux: uint16(PushLoad), A: 2},
			{Op: OpMakeRef, A: 3, B: 2},
			{Op: OpPush, A: 3},
			{Op: OpCall, A: 3, B: 1, C: 2},
		},
	}
}

func TestFormat16AcceptsValidOperands(t *testing.T) {
	for name, body := range format16ValidBodies() {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format16Image(body...)) })
	}
}

func TestFormat16RejectsInvalidOperands(t *testing.T) {
	cases := map[string][]Instruction{
		"bind global aux":                    {{Op: OpBindGlobal, Aux: 1, A: 2, B: 1}},
		"bind global into a value":           {{Op: OpBindGlobal, A: 3, B: 1}},
		"bind global undefined location":     {{Op: OpBindGlobal, A: 4, B: 1}},
		"bind global name not a string":      {{Op: OpBindGlobal, A: 2, B: 2}},
		"bind global name out of range":      {{Op: OpBindGlobal, A: 2, B: 99}},
		"bind global c":                      {{Op: OpBindGlobal, A: 2, B: 1, C: 1}},
		"static init target out of range":    {{Op: OpStaticInit, A: 2, B: 999}},
		"static init on a value":             {{Op: OpStaticInit, A: 3, B: format12Body + 1}},
		"static init c":                      {{Op: OpStaticInit, A: 2, B: format12Body + 1, C: 1}},
		"bind static undefined value":        {{Op: OpBindStatic, A: 2, B: 3}},
		"bind static value is a location":    {{Op: OpBindStatic, A: 2, B: 2}},
		"bind static empties its value":      {{Op: OpBindStatic, A: 2, B: 1}, {Op: OpOutput, A: 1}},
		"recv variadic into a location":      {{Op: OpRecvVariadic, A: 2}},
		"recv variadic b":                    {{Op: OpRecvVariadic, A: 3, B: 1}},
		"init call function out of range":    {{Op: OpInitCall, B: 9}, {Op: OpCallPending, A: 3}},
		"init call a":                        {{Op: OpInitCall, A: 1, B: 1}, {Op: OpCallPending, A: 3}},
		"init call left pending at return":   {{Op: OpInitCall, B: 1}},
		"send unpack outside a call":         {{Op: OpSendUnpack, A: 1}},
		"send unpack undefined":              {{Op: OpInitCall, B: 1}, {Op: OpSendUnpack, A: 3}, {Op: OpCallPending, A: 3}},
		"send unpack b":                      {{Op: OpInitCall, B: 1}, {Op: OpSendUnpack, A: 1, B: 1}, {Op: OpCallPending, A: 3}},
		"call pending without a call":        {{Op: OpPush, A: 0}, {Op: OpCallPending, A: 3}},
		"call pending into a location":       {{Op: OpInitCall, B: 1}, {Op: OpCallPending, A: 4}},
		"call pending twice":                 {{Op: OpInitCall, B: 1}, {Op: OpCallPending, A: 3}, {Op: OpCallPending, A: 3}},
		"direct call takes a started call":   {{Op: OpInitCall, B: 1}, {Op: OpPush, A: 0}, {Op: OpCall, A: 3, B: 1, C: 2}},
		"direct call takes unpacked":         {{Op: OpInitCall, B: 1}, {Op: OpSendUnpack, A: 1}, {Op: OpConst, A: 3, B: 0}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2}},
		"dynamic call takes a started call":  {{Op: OpInitCall, B: 1}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"dynamic call takes unpacked":        {{Op: OpInitCall, B: 1}, {Op: OpSendUnpack, A: 1}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"push aux 2":                         {{Op: OpPush, Aux: 2, A: 2}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"push load of a value":               {{Op: OpPush, Aux: uint16(PushLoad), A: 0}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"push load of an undefined location": {{Op: OpPush, Aux: uint16(PushLoad), A: 4}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"reference to a by-value parameter": {
			{Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2},
		},
		"moved reference to a by-value parameter": {
			{Op: OpMakeRef, A: 3, B: 2}, {Op: OpMove, A: 1, B: 3}, {Op: OpPush, A: 1}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2},
		},
		"by-reference iterator to a by-value parameter": {
			{Op: OpIterInit, Aux: uint16(IterByRef), A: 3, B: 2, C: format12Body + 4}, {Op: OpPush, A: 3}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2},
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format16Image(body...), ErrInvalid) })
	}
	t.Run("the calls started on two paths must be the same", func(t *testing.T) {
		// 3 Branch r0, 4, 6; 4 InitCall 1; 5 Jump 7; 6 InitCall 0; 7 CallPending r3.
		requireBuildError(t, format16Image(
			Instruction{Op: OpBranch, A: 0, B: format12Body + 1, C: format12Body + 3},
			Instruction{Op: OpInitCall, B: 1},
			Instruction{Op: OpJump, A: format12Body + 4},
			Instruction{Op: OpInitCall, B: 0},
			Instruction{Op: OpCallPending, A: 3},
		), ErrInvalid)
	})
	t.Run("a reference on one path only is still a reference", func(t *testing.T) {
		// 3 Branch r0, 4, 6; 4 MakeRef r3; 5 Jump 7; 6 Const r3; 7 Push r3;
		// 8 Push r3; 9 Call callee.
		requireBuildError(t, format16Image(
			Instruction{Op: OpBranch, A: 0, B: format12Body + 1, C: format12Body + 3},
			Instruction{Op: OpMakeRef, A: 3, B: 2},
			Instruction{Op: OpJump, A: format12Body + 4},
			Instruction{Op: OpConst, A: 3, B: 0},
			Instruction{Op: OpPush, A: 3},
			Instruction{Op: OpPush, A: 3},
			Instruction{Op: OpCall, A: 3, B: 1, C: 2},
		), ErrInvalid)
	})
	t.Run("recv variadic needs a variadic parameter", func(t *testing.T) {
		i := format16Image(Instruction{Op: OpRecvVariadic, A: 3})
		i.Functions[0].Parameters = i.Functions[0].Parameters[:1]
		requireBuildError(t, i, ErrInvalid)
	})
}

// A register that held a reference is one no longer once it is overwritten
// or pushed, and a by-reference parameter takes a reference.
func TestFormat16ReferenceTrackingFollowsTheRegister(t *testing.T) {
	cases := map[string][]Instruction{
		"overwritten": {
			{Op: OpMakeRef, A: 3, B: 2}, {Op: OpConst, A: 3, B: 0}, {Op: OpPush, A: 3}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2},
		},
		"moved to the stack by Push": {
			{Op: OpPush, A: 0}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 1, B: 1, C: 2},
			{Op: OpPush, A: 3}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 1, B: 1, C: 2},
		},
		"a call result is a value": {
			{Op: OpPush, A: 0}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2},
			{Op: OpPush, A: 3}, {Op: OpMakeRef, A: 1, B: 2}, {Op: OpPush, A: 1}, {Op: OpCall, A: 3, B: 1, C: 2},
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format16Image(body...)) })
	}
	t.Run("a by-reference parameter register may hold a reference", func(t *testing.T) {
		body := []Instruction{{Op: OpPush, A: 0}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2}}
		requireValidBuild(t, format16Image(body...))
		i := format16Image(body...)
		// The body's prologue overwrites r0 with a constant, so pass r0
		// before that: a function whose first instruction pushes it.
		i.Functions[0].Parameters[0].Flags = ParamByRef
		i.Functions[0].Code = append([]Instruction{{Op: OpLocal, A: 2, B: 0}, {Op: OpPush, A: 0}, {Op: OpMakeRef, A: 3, B: 2}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2}}, i.Functions[0].Code...)
		i.Functions[0].Positions = make([]Position, len(i.Functions[0].Code))
		requireBuildError(t, i, ErrInvalid)
		i.Functions[0].Parameters[0].Flags = 0
		requireValidBuild(t, i)
		// With no parameter table nothing is known about the register.
		i.Functions[0].Parameters = nil
		requireBuildError(t, i, ErrInvalid)
	})
	t.Run("a callee without a parameter table is not checked", func(t *testing.T) {
		i := format16Image(Instruction{Op: OpMakeRef, A: 3, B: 2}, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2})
		requireBuildError(t, i, ErrInvalid)
		i.Functions[1].Parameters = nil
		requireValidBuild(t, i)
	})
}

func TestFormat16ParameterTable(t *testing.T) {
	valid := map[string]func(*Image){
		"none":             func(i *Image) { i.Functions[1].Parameters = nil },
		"empty":            func(i *Image) { i.Functions[1].Parameters = []Parameter{} },
		"one per register": func(*Image) {},
		"with a variadic": func(i *Image) {
			i.Functions[1].Parameters = append(i.Functions[1].Parameters, Parameter{Name: 1, Flags: ParamVariadic | ParamByRef})
		},
		"variadic only": func(i *Image) {
			i.Functions[1].Params = 0
			i.Functions[1].Parameters = []Parameter{{Name: 1, Flags: ParamVariadic}}
		},
		"no parameters":     func(i *Image) { i.Functions[1].Params = 0; i.Functions[1].Parameters = nil },
		"main has no table": func(i *Image) { i.Functions[0].Parameters = nil },
	}
	for name, edit := range valid {
		t.Run(name, func(t *testing.T) {
			i := format16Image(Instruction{Op: OpCheckpoint})
			edit(&i)
			requireValidBuild(t, i)
		})
	}
	invalid := map[string]struct {
		edit   func(*Image)
		detail string
	}{
		"too few":                  {func(i *Image) { i.Functions[1].Parameters = i.Functions[1].Parameters[:1] }, "parameter table has 1 entries"},
		"too many":                 {func(i *Image) { i.Functions[1].Parameters = make([]Parameter, 4) }, "parameter table has 4 entries"},
		"variadic on a register":   {func(i *Image) { i.Functions[1].Parameters[0].Flags = ParamVariadic }, "ParamVariadic marks exactly"},
		"last is not variadic":     {func(i *Image) { i.Functions[1].Parameters = append(i.Functions[1].Parameters, Parameter{Name: 1}) }, "ParamVariadic marks exactly"},
		"unknown flag":             {func(i *Image) { i.Functions[1].Parameters[1].Flags = 4 }, "unknown flags"},
		"name is not a string":     {func(i *Image) { i.Functions[1].Parameters[1].Name = 0 }, "not a string constant"},
		"name is out of range":     {func(i *Image) { i.Functions[1].Parameters[0].Name = 99 }, "not a string constant"},
		"name is far out of range": {func(i *Image) { i.Functions[1].Parameters[0].Name = 0xffffffff }, "not a string constant"},
	}
	for name, tc := range invalid {
		t.Run(name, func(t *testing.T) {
			i := format16Image(Instruction{Op: OpCheckpoint})
			tc.edit(&i)
			requireBuildError(t, i, ErrInvalid)
			_, err := Build(context.Background(), i, testOptions())
			var e *Error
			if !errors.As(err, &e) || e.Function != 1 || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("error = %+v, want function 1 and detail containing %q", e, tc.detail)
			}
		})
	}
}

func TestFormat16ParameterTableRoundTrip(t *testing.T) {
	image := format16Image(format16ValidBodies()["pending call"]...)
	data := encodeAtMinor(t, image, FormatMinor)
	p, err := loadBytes(data)
	if err != nil || p == nil {
		t.Fatalf("Load=(%v,%v)", p, err)
	}
	main, _ := p.Function(0)
	callee, _ := p.Function(1)
	if main.ParameterCount() != 2 || callee.ParameterCount() != 2 {
		t.Fatalf("parameter counts = %d, %d; want 2, 2", main.ParameterCount(), callee.ParameterCount())
	}
	for i, want := range []ParameterFlags{0, ParamVariadic} {
		if name, flags, ok := main.Parameter(uint32(i)); !ok || name != "x" || flags != want {
			t.Fatalf("main parameter %d = %q, %d, %v", i, name, flags, ok)
		}
	}
	for i, want := range []ParameterFlags{0, ParamByRef} {
		if name, flags, ok := callee.Parameter(uint32(i)); !ok || name != "x" || flags != want {
			t.Fatalf("callee parameter %d = %q, %d, %v", i, name, flags, ok)
		}
	}
	if _, _, ok := callee.Parameter(2); ok {
		t.Fatal("parameter 2 of callee exists")
	}
	again, err := p.Encode(context.Background())
	if err != nil || string(again) != string(data) {
		t.Fatalf("canonical re-encode differs: err=%v", err)
	}
	// The payload ends with the callee's table: count 2, then two entries
	// of a u32 name and a u8 of flags.
	tail := data[len(data)-14:]
	if want := []byte{2, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, byte(ParamByRef)}; string(tail) != string(want) {
		t.Fatalf("payload ends with % x, want % x", tail, want)
	}
	t.Run("a table in the payload is verified", func(t *testing.T) {
		b := append([]byte(nil), data...)
		b[len(b)-1] = 0x80
		repairWireIntegrity(b)
		p, err := loadBytes(b)
		var e *Error
		if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || e.Function != 1 || !strings.Contains(e.Detail, "unknown flags") {
			t.Fatalf("Load=(%v,%+v), want ErrInvalid for unknown flags in function 1", p, e)
		}
	})
	t.Run("a truncated table is refused", func(t *testing.T) {
		b := append([]byte(nil), data[:len(data)-3]...)
		repairWireIntegrity(b)
		if p, err := loadBytes(b); p != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("Load=(%v,%v), want ErrInvalid", p, err)
		}
	})
	t.Run("an older file has no table", func(t *testing.T) {
		old := format16Image(Instruction{Op: OpCheckpoint})
		p, err := loadBytes(encodeAtMinor(t, old, 5))
		if err != nil || p == nil {
			t.Fatalf("minor 5 Load=(%v,%v)", p, err)
		}
		if v, _ := p.Function(1); v.ParameterCount() != 0 {
			t.Fatalf("minor 5 callee has %d parameter entries", v.ParameterCount())
		}
	})
}

func TestFormat16IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, body := range format16ValidBodies() {
		for _, minor := range []uint16{0, 1, 2, 3, 4, 5} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format16Image(body...)
				if p, err := loadBytes(encodeAtMinor(t, image, 6)); err != nil || p == nil {
					t.Fatalf("minor 6 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				// Older formats refuse the instructions the body shares
				// with them first (MakeRef needs 1.3); every body has one
				// that needs 1.6 and nothing newer.
				if !strings.Contains(e.Detail, "needs bytecode format 1.") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want the needed and the declared format", minor, e)
				}
				if minor == 5 && !strings.Contains(e.Detail, "needs bytecode format 1.6") {
					t.Fatalf("minor 5 error = %+v, want format 1.6", e)
				}
			})
		}
	}
	t.Run("a format 1.5 file still loads and a reference to a by-value parameter is not its concern", func(t *testing.T) {
		image := format16Image(
			Instruction{Op: OpMakeRef, A: 3, B: 2}, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2},
		)
		image.Functions[1].Parameters = nil // Build verifies as 1.6
		if p, err := loadBytes(encodeAtMinor(t, image, 5)); err != nil || p == nil {
			t.Fatalf("Load=(%v,%v)", p, err)
		}
	})
}

func TestFormat16Opcodes(t *testing.T) {
	// Seven opcodes follow format 1.5's MatchError; 56 is the first unknown one.
	want := []Opcode{OpBindGlobal, OpStaticInit, OpBindStatic, OpRecvVariadic, OpInitCall, OpSendUnpack, OpCallPending}
	for i, op := range want {
		if op != Opcode(49+i) {
			t.Fatalf("format 1.6 opcode %d is %d, want %d", i, op, 49+i)
		}
		if s, ok := op.Safety(); !ok || s.Steps != 1 || !s.CheckCancel || s.Capabilities != 0 || (op != OpInitCall) != s.MayThrow {
			t.Fatalf("Safety(%d) = %+v, %v", op, s, ok)
		}
	}
	if _, ok := Opcode(56).Safety(); ok {
		t.Fatal("opcode 56 is known")
	}
	if FormatMinor != 6 {
		t.Fatalf("FormatMinor = %d, want 6", FormatMinor)
	}
}
