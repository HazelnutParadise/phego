package bytecode

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Format 1.7 (W-17 stage 7A: classes and objects): REQ-001, REQ-016,
// REQ-025, REQ-026. The frame is format16Image's: r0 Value (a defined
// parameter), r1 Value (defined, an array), r2 Location (defined), r3 Value
// (undefined), r4 Location (undefined). The body starts at pc 3.
// format17Image adds the constants and functions the class table names and
// the class table itself: class 0 is C (abstract, extends P, implements I,
// uses T and T2 with an insteadof and an alias rule), class 1 the interface
// I, class 2 the trait T.

// Constant indices of format17Image (0 Null, 1 "x" and 2 Int 1 come from
// format16Image).
const (
	k17C     = 3  // "C"
	k17P     = 4  // "P"
	k17I     = 5  // "I"
	k17T     = 6  // "T"
	k17T2    = 7  // "T2"
	k17m     = 8  // "m"
	k17K     = 9  // "K"
	k17p     = 10 // "p"
	k17int   = 11 // "int"
	k17alias = 12 // "alias"
	k17sm    = 13 // "sm"
	k17am    = 14 // "am"
	k17sig   = 15 // the signature of am
	k17q     = 16 // "q"
	k17K2    = 17 // "K2"
	k17r     = 18 // "r"
	k17empty = 19 // ""
	k17M     = 20 // "M"
	k17Q     = 21 // "Q"
	k17Int   = 2  // Int 1
)

// Function indices of format17Image.
const (
	f17m      = 2 // C::m
	f17sm     = 3 // C::sm, static (FuncNoThis)
	f17am     = 4 // C::am, abstract: am(int $x = 1): int
	f17K2     = 5 // the initializer of C::K2
	f17r      = 6 // the initializer of C::$r
	f17spare  = 7 // a value function no member uses
	f17params = 8 // a function with a parameter
	f17loc    = 9 // a function that returns a Location
)

func valueFunction17(name string) Function {
	return Function{Name: name, ReturnKind: Value, Registers: []RegisterKind{Value},
		Code: []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Positions: make([]Position, 2)}
}

func format17Image(body ...Instruction) Image {
	i := format16Image(body...)
	for _, s := range []string{"C", "P", "I", "T", "T2", "m", "K", "p", "int", "alias", "sm", "am", "public function am(int $x = 1): int", "q", "K2", "r", "", "M", "Q"} {
		i.Constants = append(i.Constants, Constant{Kind: String, Bytes: s})
	}
	i.Functions[0].Flags = FuncStrict
	sm := valueFunction17("C::sm")
	sm.Flags = FuncNoThis | FuncStrict
	am := Function{Name: "C::am", Params: 1, ReturnKind: Value, Registers: []RegisterKind{Value, Value},
		Code: []Instruction{{Op: OpConst, A: 1, B: 0}, {Op: OpReturn, A: 1}}, Positions: make([]Position, 2),
		Parameters: []Parameter{{Name: 1, Flags: ParamOptional, Type: k17int + 1}}, ReturnType: k17int + 1}
	params := Function{Name: "withParam", Params: 1, ReturnKind: Value, Registers: []RegisterKind{Value},
		Code: []Instruction{{Op: OpReturn, A: 0}}, Positions: make([]Position, 1)}
	loc := Function{Name: "returnsLocation", Locals: 1, ReturnKind: Location, Registers: []RegisterKind{Location},
		Code: []Instruction{{Op: OpLocal, A: 0, B: 0}, {Op: OpReturnRef, A: 0}}, Positions: make([]Position, 2)}
	i.Functions = append(i.Functions, valueFunction17("C::m"), sm, am, valueFunction17("C::K2"), valueFunction17("C::$r"), valueFunction17("spare"), params, loc)
	i.Classes = []Class{
		{
			Name: k17C, Flags: ClassAbstract, Parent: k17P + 1, Line: 3,
			Interfaces: []uint32{k17I},
			Traits:     []uint32{k17T, k17T2},
			Rules: []TraitRule{
				{Trait: k17T + 1, Method: k17m, Insteadof: []uint32{k17T2}, Line: 4},
				{Method: k17m, Alias: k17alias + 1, Flags: MemberProtected, HasVisibility: true, Line: 5},
			},
			Constants: []ClassConstant{
				{Name: k17K, Flags: MemberFinal, Value: k17Int + 1},
				{Name: k17K2, Flags: MemberProtected, Init: f17K2 + 1},
			},
			Properties: []Property{
				{Name: k17p, Type: k17int + 1, Value: k17Int + 1},
				{Name: k17q, Flags: MemberPrivate | MemberReadonly},
				{Name: k17r, Flags: MemberStatic, Init: f17r + 1},
			},
			Methods: []Method{
				{Name: k17m, Function: f17m, Line: 6},
				{Name: k17sm, Function: f17sm, Flags: MemberStatic, Line: 7},
				{Name: k17am, Function: f17am, Flags: MemberAbstract, Signature: k17sig + 1, Line: 8},
			},
		},
		{Name: k17I, Flags: ClassInterface, Line: 1},
		{Name: k17T, Flags: ClassTrait | ClassHoisted, Line: 2},
	}
	return i
}

func memberAux(ref ClassRef, nameReg bool, mode MemberMode, objLocation bool) uint16 {
	return MemberAux(ref, nameReg, mode, objLocation)
}

// format17ValidBodies are bodies whose first instruction uses a format 1.7
// opcode or Aux, in a valid form.
func format17ValidBodies() map[string][]Instruction {
	const b = format12Body
	objLoc := MemberObjLocation
	return map[string][]Instruction{
		"declare class":     {{Op: OpDeclareClass, B: 0}},
		"declare interface": {{Op: OpDeclareClass, B: 1}},
		"new without arguments": {
			// 3 New r3 C, 5; 4 CallPending r0; 5 Output r3.
			{Op: OpNew, A: 3, B: k17C, C: b + 2}, {Op: OpCallPending, A: 0}, {Op: OpOutput, A: 3},
		},
		"new self with arguments": {
			// 3 New r3 self, 7; 4 Push auto L2; 5 Push temp r0; 6 CallPending r0; 7 Output r3.
			{Op: OpNew, Aux: uint16(ClassSelf), A: 3, C: b + 4},
			{Op: OpPush, Aux: uint16(PushAuto), A: 2}, {Op: OpPush, Aux: uint16(PushTemp), A: 0},
			{Op: OpCallPending, A: 0}, {Op: OpOutput, A: 3},
		},
		"new from a value": {{Op: OpNew, Aux: uint16(ClassValue), A: 3, B: 0, C: b + 2}, {Op: OpCallPending, A: 0}},
		"new static":       {{Op: OpNew, Aux: uint16(ClassStatic), A: 3, C: b + 2}, {Op: OpCallPending, A: 0}},
		"init method": {
			{Op: OpInitMethod, A: 0, B: k17m},
			{Op: OpPush, Aux: uint16(PushAuto), A: 2},
			{Op: OpPush, Aux: uint16(PushTemp), A: 0},
			{Op: OpPush, Aux: uint16(PushResult), A: 0},
			{Op: OpPush, A: 0},
			{Op: OpPush, Aux: uint16(PushLoad), A: 2},
			{Op: OpPush, Aux: uint16(PushAuto), A: 2, B: 2},
			{Op: OpSendUnpack, A: 1},
			{Op: OpCallPending, A: 3},
		},
		"init method name in a register": {{Op: OpInitMethod, Aux: MemberNameReg, A: 0, B: 1}, {Op: OpCallPending, A: 3}},
		"init static parent":             {{Op: OpInitStatic, Aux: uint16(ClassParent), B: k17sm}, {Op: OpCallPending, A: 3}},
		"init static named":              {{Op: OpInitStatic, A: k17C, B: k17sm}, {Op: OpPush, Aux: uint16(PushResult), A: 0}, {Op: OpCallPending, A: 3}},
		"init static of a value":         {{Op: OpInitStatic, Aux: uint16(ClassValue) | MemberNameReg, A: 0, B: 1}, {Op: OpCallPending, A: 3}},
		"this":                           {{Op: OpThis, A: 3}, {Op: OpOutput, A: 3}},
		"prop get":                       {{Op: OpPropGet, A: 3, B: 0, C: k17p}, {Op: OpOutput, A: 3}},
		"prop get quiet by name":         {{Op: OpPropGet, Aux: memberAux(ClassNamed, true, MemberQuiet, false), A: 3, B: 0, C: 1}},
		"prop set":                       {{Op: OpPropSet, A: 0, B: k17p, C: 1}},
		"prop set by name":               {{Op: OpPropSet, Aux: MemberNameReg, A: 0, B: 1, C: 0}},
		"prop set with its result":       {{Op: OpPropSet, Aux: MemberResult, A: 0, B: k17p, C: 1}, {Op: OpOutput, A: 1}},
		"prop get for an assign-op":      {{Op: OpPropGet, Aux: memberAux(ClassNamed, false, MemberAssignOp, false), A: 3, B: 0, C: k17p}},
		"prop get for an increment":      {{Op: OpPropGet, Aux: memberAux(ClassNamed, true, MemberIncDec, false), A: 3, B: 0, C: 1}},
		"fetch prop":                     {{Op: OpFetchProp, A: 4, B: 0, C: k17p}, {Op: OpStore, A: 4, B: 0}},
		"fetch prop read-write":          {{Op: OpFetchProp, Aux: memberAux(ClassNamed, false, MemberReadWrite, false), A: 4, B: 0, C: k17p}, {Op: OpIncDec, Aux: uint16(PostInc), A: 3, B: 4}},
		"fetch prop unset":               {{Op: OpFetchProp, Aux: memberAux(ClassNamed, true, MemberUnset, false), A: 4, B: 0, C: 1}, {Op: OpConst, A: 3, B: 0}, {Op: OpArrayUnset, A: 4, B: 3}},
		"prop ref":                       {{Op: OpPropRef, A: 3, B: 0, C: k17p}, {Op: OpBind, Aux: uint16(BindValue), A: 2, B: 3}},
		"prop isset":                     {{Op: OpPropIsset, Aux: memberAux(ClassNamed, false, MemberIsset, false), A: 3, B: 0, C: k17p}},
		"prop empty by name":             {{Op: OpPropIsset, Aux: memberAux(ClassNamed, true, MemberEmpty, false), A: 3, B: 0, C: 1}},
		"prop unset":                     {{Op: OpPropUnset, A: 0, B: k17p}},
		"static get":                     {{Op: OpStaticGet, A: 3, B: k17C, C: k17p}},
		"static isset of static":         {{Op: OpStaticGet, Aux: memberAux(ClassStatic, false, MemberIsset, false), A: 3, C: k17p}},
		"static get of a value by name":  {{Op: OpStaticGet, Aux: memberAux(ClassValue, true, MemberEmpty, false), A: 3, B: 0, C: 1}},
		"static set":                     {{Op: OpStaticSet, Aux: uint16(ClassSelf), A: 0, C: k17r}},
		"static set with its result":     {{Op: OpStaticSet, Aux: uint16(ClassNamed) | MemberResult, A: 0, B: k17C, C: k17r}, {Op: OpOutput, A: 0}},
		"fetch static":                   {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpStore, A: 4, B: 0}},
		"fetch static read-write":        {{Op: OpFetchStatic, Aux: memberAux(ClassSelf, false, MemberReadWrite, false), A: 4, C: k17r}, {Op: OpArraySet, A: 4, B: 0, C: 0}},
		"static ref":                     {{Op: OpStaticRef, Aux: uint16(ClassParent), A: 3, C: k17r}, {Op: OpPush, A: 0}, {Op: OpPush, A: 3}, {Op: OpCall, A: 3, B: 1, C: 2}},
		"class const":                    {{Op: OpClassConst, A: 3, B: k17C, C: k17K}},
		"class const of static":          {{Op: OpClassConst, Aux: uint16(ClassStatic), A: 3, C: k17K}},
		"class name":                     {{Op: OpClassName, Aux: uint16(ClassStatic), A: 3}},
		"class name of a value":          {{Op: OpClassName, Aux: uint16(ClassValue), A: 3, B: 0}},
		"instanceof":                     {{Op: OpInstanceof, A: 3, B: 0, C: k17C}},
		"instanceof a value":             {{Op: OpInstanceof, Aux: uint16(ClassValue), A: 3, B: 0, C: 1}},
		"clone":                          {{Op: OpClone, A: 3, B: 0}},
		"verify return":                  {{Op: OpVerifyReturn, A: 0}},
		"verify return none":             {{Op: OpVerifyReturn, Aux: uint16(ReturnNone), A: 0}},
		"verify return never":            {{Op: OpVerifyReturn, Aux: uint16(ReturnNever), A: 0}},
		"cast object":                    {{Op: OpCast, Aux: uint16(CastObject), A: 3, B: 0}},
		"prop set through a fetched prop": {
			{Op: OpFetchProp, A: 4, B: 0, C: k17p}, {Op: OpPropSet, Aux: objLoc, A: 4, B: k17q, C: 0},
		},
	}
}

func TestFormat17Opcodes(t *testing.T) {
	want := []Opcode{OpDeclareClass, OpNew, OpInitMethod, OpInitStatic, OpThis, OpPropGet, OpPropSet, OpFetchProp, OpPropRef, OpPropIsset,
		OpPropUnset, OpStaticGet, OpStaticSet, OpFetchStatic, OpStaticRef, OpClassConst, OpClassName, OpInstanceof, OpClone, OpVerifyReturn}
	for i, op := range want {
		if op != Opcode(56+i) {
			t.Fatalf("format 1.7 opcode %d is %d, want %d", i, op, 56+i)
		}
		if s, ok := op.Safety(); !ok || s != (Safety{Steps: 1, CheckCancel: true, MayAllocate: true, MayThrow: true, MayCallUser: true}) {
			t.Fatalf("Safety(%d) = %+v, %v", op, s, ok)
		}
	}
	if _, ok := Opcode(76).Safety(); ok {
		t.Fatal("opcode 76 is known")
	}
	if FormatMinor != 7 {
		t.Fatalf("FormatMinor = %d, want 7", FormatMinor)
	}
	if CastObject != CastArray+1 || PushAuto != PushLoad+1 || PushTemp != PushAuto+1 || PushResult != PushTemp+1 {
		t.Fatalf("format 1.7 Aux values are renumbered: cast %d, push %d %d %d", CastObject, PushAuto, PushTemp, PushResult)
	}
	// Every format 1.7 opcode has a valid body below.
	seen := map[Opcode]bool{}
	for _, body := range format17ValidBodies() {
		seen[body[0].Op] = true
	}
	for _, op := range want {
		if !seen[op] {
			t.Errorf("no valid body starts with opcode %d", op)
		}
	}
}

func TestFormat17AcceptsAndRoundTripsValidBodies(t *testing.T) {
	for name, body := range format17ValidBodies() {
		t.Run(name, func(t *testing.T) {
			p := requireValidBuild(t, format17Image(body...))
			data, err := p.Encode(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got := binary.LittleEndian.Uint16(data[10:12]); got != 7 {
				t.Fatalf("Encode wrote minor %d", got)
			}
			q, err := loadBytes(data)
			if err != nil || q == nil {
				t.Fatalf("Load=(%v,%v)", q, err)
			}
			got, _ := q.Instructions(0, nil)
			wantCode, _ := p.Instructions(0, nil)
			if fmt.Sprint(got) != fmt.Sprint(wantCode) {
				t.Fatalf("reloaded code %v, want %v", got, wantCode)
			}
			again, err := q.Encode(context.Background())
			if err != nil || string(again) != string(data) {
				t.Fatalf("canonical re-encode differs: err=%v", err)
			}
		})
	}
}

func TestFormat17ClassTableRoundTrip(t *testing.T) {
	image := format17Image(Instruction{Op: OpDeclareClass, B: 0})
	p := requireValidBuild(t, image)
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q, err := loadBytes(data)
	if err != nil || q == nil {
		t.Fatalf("Load=(%v,%v)", q, err)
	}
	for _, prog := range []*Program{p, q} {
		if prog.ClassCount() != len(image.Classes) {
			t.Fatalf("ClassCount = %d, want %d", prog.ClassCount(), len(image.Classes))
		}
		for ci, want := range image.Classes {
			got, ok := prog.Class(uint32(ci))
			// %v prints nil and empty slices alike; the table has no other
			// distinction between them.
			if !ok || fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
				t.Fatalf("Class(%d) = %+v, %v; want %+v", ci, got, ok, want)
			}
		}
		if _, ok := prog.Class(uint32(len(image.Classes))); ok {
			t.Fatal("Class past the table exists")
		}
		if _, ok := prog.Class(0xffffffff); ok {
			t.Fatal("Class(0xffffffff) exists")
		}
		main, _ := prog.Function(0)
		sm, _ := prog.Function(f17sm)
		am, _ := prog.Function(f17am)
		if main.Flags() != FuncStrict || sm.Flags() != FuncNoThis|FuncStrict || am.Flags() != 0 {
			t.Fatalf("flags = %d %d %d", main.Flags(), sm.Flags(), am.Flags())
		}
		if main.ReturnType() != "" || am.ReturnType() != "int" {
			t.Fatalf("return types = %q %q", main.ReturnType(), am.ReturnType())
		}
		if am.ParameterType(0) != "int" || am.ParameterType(1) != "" || main.ParameterType(0) != "" {
			t.Fatalf("parameter types = %q %q %q", am.ParameterType(0), am.ParameterType(1), main.ParameterType(0))
		}
		if name, flags, ok := am.Parameter(0); !ok || name != "x" || flags != ParamOptional {
			t.Fatalf("am parameter 0 = %q, %d, %v", name, flags, ok)
		}
	}
	if (FunctionView{}).ReturnType() != "" || (FunctionView{}).Flags() != 0 || (FunctionView{}).ParameterType(0) != "" {
		t.Fatal("zero FunctionView reports format 1.7 data")
	}
	again, err := q.Encode(context.Background())
	if err != nil || string(again) != string(data) {
		t.Fatalf("canonical re-encode differs: err=%v", err)
	}
	t.Run("the program owns its class table", func(t *testing.T) {
		c, _ := p.Class(0)
		c.Interfaces[0] = 99
		c.Rules[0].Insteadof[0] = 99
		c.Methods[0].Function = 99
		image.Classes[0].Traits[0] = 99
		image.Classes[0].Rules[0].Insteadof[0] = 98
		image.Classes[0].Constants[0].Value = 98
		d, _ := p.Class(0)
		if d.Interfaces[0] != k17I || d.Rules[0].Insteadof[0] != k17T2 || d.Methods[0].Function != f17m || d.Traits[0] != k17T || d.Constants[0].Value != k17Int+1 {
			t.Fatalf("class table changed through a copy: %+v", d)
		}
	})
	t.Run("the payload ends with the class table", func(t *testing.T) {
		plain := format17Image(Instruction{Op: OpCheckpoint})
		plain.Classes = nil
		without := encodeAtMinor(t, plain, FormatMinor)
		if binary.LittleEndian.Uint32(without[len(without)-4:]) != 0 {
			t.Fatalf("an image without classes does not end with an empty class table: % x", without[len(without)-4:])
		}
		// The last function (returnsLocation) has no flags and no return
		// type: five zero bytes before the class count.
		if tail := without[len(without)-9 : len(without)-4]; string(tail) != "\x00\x00\x00\x00\x00" {
			t.Fatalf("last function's format 1.7 record = % x", tail)
		}
	})
}

// classTableRegion returns the encoding of format17Image and the offset of
// its class table (the class count).
func classTableRegion(t *testing.T) ([]byte, int) {
	t.Helper()
	image := format17Image(Instruction{Op: OpCheckpoint})
	data := encodeAtMinor(t, image, FormatMinor)
	image.Classes = nil
	without := encodeAtMinor(t, image, FormatMinor)
	start := len(without) - 4
	if string(data[wireHeader:start]) != string(without[wireHeader:start]) {
		t.Fatal("the encodings differ before the class table")
	}
	return data, start
}

func TestFormat17ClassTableWire(t *testing.T) {
	data, start := classTableRegion(t)
	if binary.LittleEndian.Uint32(data[start:]) != 3 {
		t.Fatalf("class count = %d, want 3", binary.LittleEndian.Uint32(data[start:]))
	}
	cases := map[string]struct {
		edit   func([]byte) []byte
		detail string
	}{
		"class name out of range": {func(b []byte) []byte { binary.LittleEndian.PutUint32(b[start+4:], 999); return b }, "class 0"},
		"class padding":           {func(b []byte) []byte { b[start+4+6] = 1; return b }, "padding"},
		"class flags":             {func(b []byte) []byte { b[start+4+5] = 0x80; return b }, "unknown class flags"},
		"truncated":               {func(b []byte) []byte { return b[:len(b)-1] }, ""},
		"trailing byte":           {func(b []byte) []byte { return append(b, 0) }, "trailing"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b := tc.edit(append([]byte(nil), data...))
			repairWireIntegrity(b)
			p, err := loadBytes(b)
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Load=(%v,%v), want ErrInvalid with %q", p, err, tc.detail)
			}
		})
	}
}

func TestOlderFormatsStillLoad(t *testing.T) {
	images := map[uint16]Image{
		0: validImage(),
		1: format11Everything(),
		2: format12Everything(),
		3: format12Image(format13ValidInstructions()["make ref"]),
		4: format12Image(format14ValidBodies()["fetch from a value"]...),
		5: format15Image(format15ValidBodies()["fetch constant then use"]...),
		6: format16Image(format16ValidBodies()["pending call"]...),
	}
	for minor := uint16(0); minor < 7; minor++ {
		image, ok := images[minor]
		if !ok {
			t.Fatalf("no image for minor %d", minor)
		}
		t.Run(fmt.Sprintf("minor %d", minor), func(t *testing.T) {
			data := encodeAtMinor(t, image, minor)
			if got := binary.LittleEndian.Uint16(data[10:12]); got != minor {
				t.Fatalf("file declares minor %d", got)
			}
			p, err := loadBytes(data)
			if err != nil || p == nil {
				t.Fatalf("Load=(%v,%v)", p, err)
			}
			if p.ClassCount() != 0 {
				t.Fatalf("a format 1.%d file has %d classes", minor, p.ClassCount())
			}
			again, err := p.Encode(context.Background())
			if err != nil || binary.LittleEndian.Uint16(again[10:12]) != FormatMinor {
				t.Fatalf("re-encode err=%v", err)
			}
			if q, err := loadBytes(again); err != nil || q == nil {
				t.Fatalf("reloaded current encoding: Load=(%v,%v)", q, err)
			}
		})
	}
}

func TestFormat17IsGatedByTheDeclaredMinor(t *testing.T) {
	for name, body := range format17ValidBodies() {
		for _, minor := range []uint16{6, 5, 0} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format17Image(body...)
				if p, err := loadBytes(encodeAtMinor(t, image, 7)); err != nil || p == nil {
					t.Fatalf("minor 7 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				if e.PC != format12Body || !strings.Contains(e.Detail, "needs bytecode format 1.7") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want the instruction at pc %d, format 1.7 and the declared format", minor, e, format12Body)
				}
			})
		}
	}
	// A push by the callee's declaration is only valid in a call that a
	// format 1.7 instruction started, so Load meets that instruction first;
	// the instruction check itself refuses the Aux too.
	image := format17Image(Instruction{Op: OpCheckpoint})
	f := &image.Functions[0]
	for _, in := range []Instruction{
		{Op: OpPush, Aux: uint16(PushAuto), A: 2},
		{Op: OpPush, Aux: uint16(PushTemp), A: 0},
		{Op: OpPush, Aux: uint16(PushResult), A: 0},
		{Op: OpCast, Aux: uint16(CastObject), A: 3, B: 0},
	} {
		for _, minor := range []uint16{6, 5, 0} {
			err := verifyInstruction(image, f, minor, in)
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "needs bytecode format 1.7") || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
				t.Errorf("verifyInstruction(%+v, minor %d) = %v, want the format 1.7 gate", in, minor, err)
			}
		}
		if err := verifyInstruction(image, f, 7, in); err != nil {
			t.Errorf("verifyInstruction(%+v, minor 7) = %v", in, err)
		}
	}
}

// The format 1.7 fields of the image tables cannot be written in an older
// layout, so they are checked against the declared minor where the image is
// verified.
func TestFormat17TableFeaturesAreGatedByTheMinor(t *testing.T) {
	base := func() Image {
		i := format17Image(Instruction{Op: OpCheckpoint})
		i.Classes = nil
		for fi := range i.Functions {
			f := &i.Functions[fi]
			f.Flags, f.ReturnType = 0, 0
			for pi := range f.Parameters {
				f.Parameters[pi].Type = 0
				f.Parameters[pi].Flags &^= ParamOptional
			}
		}
		return i
	}
	verify := func(i Image, minor uint16) error {
		b, err := newBudget(context.Background(), DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		_, err = verifyImage(b, i, minor, false)
		return err
	}
	for _, minor := range []uint16{0, 5, 6, 7} {
		if err := verify(base(), minor); err != nil {
			t.Fatalf("the image without format 1.7 fields at minor %d: %v", minor, err)
		}
	}
	cases := map[string]struct {
		edit   func(*Image)
		detail string
	}{
		"class table": {func(i *Image) {
			// base cleared C::sm's FuncNoThis, so the method is not static.
			i.Classes = format17Image().Classes
			i.Classes[0].Methods[1].Flags = 0
		}, "a class table needs bytecode format 1.7"},
		"function flags":  {func(i *Image) { i.Functions[f17sm].Flags = FuncNoThis }, "function flags and return types need bytecode format 1.7"},
		"return type":     {func(i *Image) { i.Functions[f17am].ReturnType = k17int + 1 }, "function flags and return types need bytecode format 1.7"},
		"parameter type":  {func(i *Image) { i.Functions[f17am].Parameters[0].Type = k17int + 1 }, "uses a type or flag of bytecode format 1.7"},
		"optional param":  {func(i *Image) { i.Functions[f17am].Parameters[0].Flags = ParamOptional }, "uses a type or flag of bytecode format 1.7"},
		"strict function": {func(i *Image) { i.Functions[0].Flags = FuncStrict }, "function flags and return types need bytecode format 1.7"},
	}
	for name, tc := range cases {
		for _, minor := range []uint16{6, 5, 0} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				i := base()
				tc.edit(&i)
				if err := verify(i, 7); err != nil {
					t.Fatalf("minor 7: %v", err)
				}
				err := verify(i, minor)
				var e *Error
				if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) || !strings.Contains(e.Detail, fmt.Sprintf("declares format 1.%d", minor)) {
					t.Fatalf("minor %d: %v, want %q and the declared format", minor, err, tc.detail)
				}
			})
		}
	}
}

func TestFormat17FunctionFields(t *testing.T) {
	cases := map[string]struct {
		edit   func(*Image)
		detail string
	}{
		"unknown function flag":        {func(i *Image) { i.Functions[0].Flags = FuncNoThis << 1 }, "unknown function flags"},
		"return type out of range":     {func(i *Image) { i.Functions[f17am].ReturnType = 999 }, "return type"},
		"return type far out of range": {func(i *Image) { i.Functions[f17am].ReturnType = 0xffffffff }, "return type"},
		"return type not a string":     {func(i *Image) { i.Functions[f17am].ReturnType = k17Int + 1 }, "return type"},
		"return type empty":            {func(i *Image) { i.Functions[f17am].ReturnType = k17empty + 1 }, "return type"},
		"parameter type out of range":  {func(i *Image) { i.Functions[f17am].Parameters[0].Type = 999 }, "type of parameter 0"},
		"parameter type not a string":  {func(i *Image) { i.Functions[f17am].Parameters[0].Type = 1 }, "type of parameter 0"},
		"parameter type empty":         {func(i *Image) { i.Functions[f17am].Parameters[0].Type = k17empty + 1 }, "type of parameter 0"},
		"unknown parameter flag":       {func(i *Image) { i.Functions[f17am].Parameters[0].Flags = ParamOptional << 1 }, "unknown flags"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			i := format17Image(Instruction{Op: OpCheckpoint})
			tc.edit(&i)
			_, err := Build(context.Background(), i, testOptions())
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Build = %v, want ErrInvalid with %q", err, tc.detail)
			}
		})
	}
}

func TestFormat17ClassTableIsVerified(t *testing.T) {
	c0 := func(i *Image) *Class { return &i.Classes[0] }
	cases := map[string]struct {
		edit   func(*Image)
		detail string
	}{
		// Names.
		"name out of range":          {func(i *Image) { c0(i).Name = 999 }, "class or parent name"},
		"name far out of range":      {func(i *Image) { c0(i).Name = 0xffffffff }, "class or parent name"},
		"name not a string":          {func(i *Image) { c0(i).Name = k17Int }, "class or parent name"},
		"name empty":                 {func(i *Image) { c0(i).Name = k17empty }, "class or parent name"},
		"parent not a string":        {func(i *Image) { c0(i).Parent = k17Int + 1 }, "class or parent name"},
		"parent empty":               {func(i *Image) { c0(i).Parent = k17empty + 1 }, "class or parent name"},
		"parent out of range":        {func(i *Image) { c0(i).Parent = 999 }, "class or parent name"},
		"interface name not a str":   {func(i *Image) { c0(i).Interfaces = []uint32{k17Int} }, "interface or trait name"},
		"trait name empty":           {func(i *Image) { c0(i).Traits = []uint32{k17empty} }, "interface or trait name"},
		"trait name out of range":    {func(i *Image) { c0(i).Traits = []uint32{999} }, "interface or trait name"},
		"unknown class flags":        {func(i *Image) { c0(i).Flags |= ClassHoisted << 1 }, "unknown class flags"},
		"interface and final":        {func(i *Image) { i.Classes[1].Flags |= ClassFinal }, "contradictory"},
		"interface and trait":        {func(i *Image) { i.Classes[1].Flags |= ClassTrait }, "contradictory"},
		"interface and readonly":     {func(i *Image) { i.Classes[1].Flags |= ClassReadonly }, "contradictory"},
		"trait and abstract":         {func(i *Image) { i.Classes[2].Flags |= ClassAbstract }, "contradictory"},
		"trait and final":            {func(i *Image) { i.Classes[2].Flags |= ClassFinal }, "contradictory"},
		"abstract and final":         {func(i *Image) { c0(i).Flags |= ClassFinal }, "contradictory"},
		"interface with a parent":    {func(i *Image) { i.Classes[1].Parent = k17P + 1 }, "no parent class"},
		"trait with a parent":        {func(i *Image) { i.Classes[2].Parent = k17P + 1 }, "no parent class"},
		"interface with properties":  {func(i *Image) { i.Classes[1].Properties = []Property{{Name: k17p}} }, "no traits or properties"},
		"interface with traits":      {func(i *Image) { i.Classes[1].Traits = []uint32{k17T} }, "no traits or properties"},
		"duplicate constant":         {func(i *Image) { c0(i).Constants = append(c0(i).Constants, ClassConstant{Name: k17K, Value: 1}) }, "duplicate class constant"},
		"duplicate property":         {func(i *Image) { c0(i).Properties = append(c0(i).Properties, Property{Name: k17p}) }, "duplicate property"},
		"duplicate method":           {func(i *Image) { c0(i).Methods = append(c0(i).Methods, Method{Name: k17m, Function: f17spare}) }, "duplicate method"},
		"duplicate method by case":   {func(i *Image) { c0(i).Methods = append(c0(i).Methods, Method{Name: k17M, Function: f17spare}) }, "duplicate method"},
		"constant value and init":    {func(i *Image) { c0(i).Constants[0].Init = f17spare + 1 }, "both a constant value and an initializer"},
		"constant without value":     {func(i *Image) { c0(i).Constants[0].Value = 0 }, "has no value"},
		"property value and init":    {func(i *Image) { c0(i).Properties[0].Init = f17spare + 1 }, "both a constant value and an initializer"},
		"constant value range":       {func(i *Image) { c0(i).Constants[0].Value = uint32(len(i.Constants)) + 1 }, "not in the constant pool"},
		"constant value far range":   {func(i *Image) { c0(i).Constants[0].Value = 0xffffffff }, "not in the constant pool"},
		"property value range":       {func(i *Image) { c0(i).Properties[0].Value = uint32(len(i.Constants)) + 1 }, "not in the constant pool"},
		"init out of range":          {func(i *Image) { c0(i).Constants[1].Init = uint32(len(i.Functions)) + 1 }, "not in the image"},
		"init far out of range":      {func(i *Image) { c0(i).Constants[1].Init = 0xffffffff }, "not in the image"},
		"init is the entry":          {func(i *Image) { c0(i).Constants[1].Init = 1 }, "is the entry"},
		"property init is the entry": {func(i *Image) { c0(i).Properties[2].Init = 1 }, "is the entry"},
		"method out of range":        {func(i *Image) { c0(i).Methods[0].Function = uint32(len(i.Functions)) }, "not in the image"},
		"method far out of range":    {func(i *Image) { c0(i).Methods[0].Function = 0xffffffff }, "not in the image"},
		"method is the entry":        {func(i *Image) { c0(i).Methods[0].Function = 0 }, "is the entry"},
		"two methods share a body":   {func(i *Image) { c0(i).Methods[0].Function = f17am; c0(i).Methods[0].Flags = MemberAbstract }, "more than one class member"},
		"init is a method body":      {func(i *Image) { c0(i).Constants[1].Init = f17m + 1 }, "more than one class member"},
		"two inits share a function": {func(i *Image) { c0(i).Properties[2].Init = f17K2 + 1 }, "more than one class member"},
		"members of two classes share a body": {func(i *Image) {
			i.Classes[2].Methods = []Method{{Name: k17m, Function: f17m}}
		}, "more than one class member"},
		"initializer with parameters": {func(i *Image) { c0(i).Constants[1].Init = f17params + 1 }, "takes no parameters"},
		"initializer with a variadic parameter": {func(i *Image) {
			i.Functions[f17K2].Parameters = []Parameter{{Name: 1, Flags: ParamVariadic}}
		}, "takes no parameters"},
		"method returns a location":      {func(i *Image) { c0(i).Methods[0].Function = f17loc }, "must return a value"},
		"initializer returns a location": {func(i *Image) { c0(i).Properties[2].Init = f17loc + 1 }, "must return a value"},
		"abstract and final method":      {func(i *Image) { c0(i).Methods[2].Flags |= MemberFinal }, "both abstract and final"},
		"interface method not abstract": {func(i *Image) {
			i.Classes[1].Methods = []Method{{Name: k17m, Function: f17spare}}
		}, "public and abstract"},
		"interface method not public": {func(i *Image) {
			i.Classes[1].Methods = []Method{{Name: k17m, Function: f17spare, Flags: MemberAbstract | MemberProtected}}
		}, "public and abstract"},
		"static method without FuncNoThis": {func(i *Image) { c0(i).Methods[0].Flags = MemberStatic }, "static flag differs"},
		"FuncNoThis on an instance method": {func(i *Image) { c0(i).Methods[1].Flags = 0 }, "static flag differs"},
		// Trait rules.
		"insteadof without a trait": {func(i *Image) { c0(i).Rules[0].Trait = 0 }, "insteadof trait rule"},
		"insteadof with an alias":   {func(i *Image) { c0(i).Rules[0].Alias = k17alias + 1 }, "insteadof trait rule"},
		"insteadof with visibility": {func(i *Image) { c0(i).Rules[0].HasVisibility = true }, "insteadof trait rule"},
		"insteadof name not a string": {func(i *Image) {
			c0(i).Rules[0].Insteadof = []uint32{k17Int}
		}, "invalid trait rule"},
		"alias that changes nothing": {func(i *Image) { c0(i).Rules[1].Alias, c0(i).Rules[1].HasVisibility, c0(i).Rules[1].Flags = 0, false, 0 }, "changes nothing"},
		"rule visibility 3":          {func(i *Image) { c0(i).Rules[1].Flags = 3 }, "invalid trait rule"},
		"rule flags not visibility":  {func(i *Image) { c0(i).Rules[1].Flags = MemberStatic }, "invalid trait rule"},
		"rule method not a string":   {func(i *Image) { c0(i).Rules[1].Method = k17Int }, "invalid trait rule"},
		"rule method empty":          {func(i *Image) { c0(i).Rules[1].Method = k17empty }, "invalid trait rule"},
		"rule trait not a string":    {func(i *Image) { c0(i).Rules[1].Trait = k17Int + 1 }, "invalid trait rule"},
		"rule alias empty":           {func(i *Image) { c0(i).Rules[1].Alias = k17empty + 1 }, "invalid trait rule"},
		// Canonical form: Flags is the new visibility only when HasVisibility
		// is set, so it is zero otherwise.
		"alias with visibility bits but no visibility": {func(i *Image) {
			c0(i).Rules[1].HasVisibility, c0(i).Rules[1].Flags = false, MemberPrivate
		}, "trait rule"},
		"insteadof with visibility bits": {func(i *Image) { c0(i).Rules[0].Flags = MemberPrivate }, "trait rule"},
		// Member flags.
		"constant static":            {func(i *Image) { c0(i).Constants[0].Flags = MemberStatic }, "invalid class constant"},
		"constant visibility 3":      {func(i *Image) { c0(i).Constants[0].Flags = 3 }, "invalid class constant"},
		"constant unknown flag":      {func(i *Image) { c0(i).Constants[0].Flags = MemberByRef << 1 }, "invalid class constant"},
		"constant name empty":        {func(i *Image) { c0(i).Constants[0].Name = k17empty }, "invalid class constant"},
		"property abstract":          {func(i *Image) { c0(i).Properties[0].Flags = MemberAbstract }, "invalid property"},
		"property by reference":      {func(i *Image) { c0(i).Properties[0].Flags = MemberByRef }, "invalid property"},
		"property visibility 3":      {func(i *Image) { c0(i).Properties[0].Flags = 3 }, "invalid property"},
		"property unknown flag":      {func(i *Image) { c0(i).Properties[0].Flags = MemberByRef << 1 }, "invalid property"},
		"property type not a string": {func(i *Image) { c0(i).Properties[0].Type = k17Int + 1 }, "invalid property"},
		"property type empty":        {func(i *Image) { c0(i).Properties[0].Type = k17empty + 1 }, "invalid property"},
		"property name out of range": {func(i *Image) { c0(i).Properties[0].Name = 999 }, "invalid property"},
		"method readonly":            {func(i *Image) { c0(i).Methods[0].Flags = MemberReadonly }, "invalid method"},
		"method visibility 3":        {func(i *Image) { c0(i).Methods[0].Flags = 3 }, "invalid method"},
		"method unknown flag":        {func(i *Image) { c0(i).Methods[0].Flags = MemberByRef << 1 }, "invalid method"},
		"method signature not a str": {func(i *Image) { c0(i).Methods[2].Signature = k17Int + 1 }, "invalid method"},
		"method name not a string":   {func(i *Image) { c0(i).Methods[0].Name = k17Int }, "invalid method"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			i := format17Image(Instruction{Op: OpCheckpoint})
			tc.edit(&i)
			_, err := Build(context.Background(), i, testOptions())
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, tc.detail) {
				t.Fatalf("Build = %v, want ErrInvalid with %q", err, tc.detail)
			}
		})
	}
	valid := map[string]func(*Image){
		"properties are case-sensitive": func(i *Image) { c0(i).Properties = append(c0(i).Properties, Property{Name: k17Q}) },
		"constants are case-sensitive": func(i *Image) {
			c0(i).Constants = append(c0(i).Constants, ClassConstant{Name: k17M, Value: 1}, ClassConstant{Name: k17m, Value: 1})
		},
		"members of each kind have a scope": func(i *Image) { c0(i).Properties = append(c0(i).Properties, Property{Name: k17m}) },
		"an interface method": func(i *Image) {
			i.Classes[1].Methods = []Method{{Name: k17m, Function: f17spare, Flags: MemberAbstract}}
		},
		"an interface constant":             func(i *Image) { i.Classes[1].Constants = []ClassConstant{{Name: k17K, Value: 1}} },
		"an interface extending interfaces": func(i *Image) { i.Classes[1].Interfaces = []uint32{k17I, k17P} },
		"a final readonly class":            func(i *Image) { c0(i).Flags = ClassFinal | ClassReadonly; c0(i).Methods[2].Flags = 0 },
		"an alias with visibility only":     func(i *Image) { c0(i).Rules[1].Alias = 0 },
		"an alias without visibility":       func(i *Image) { c0(i).Rules[1].HasVisibility, c0(i).Rules[1].Flags = false, 0 },
		"no class table":                    func(i *Image) { i.Classes = nil },
	}
	for name, edit := range valid {
		t.Run(name, func(t *testing.T) {
			i := format17Image(Instruction{Op: OpCheckpoint})
			edit(&i)
			requireValidBuild(t, i)
		})
	}
}

// operandMutations returns invalid variants of in: for every operand, one of
// the wrong register class, one out of range, a wrong constant kind, a
// non-zero unused operand; an Aux bit no opcode defines; and, for an
// instruction with a class operand, every out-of-range ClassRef.
func operandMutations(in Instruction) map[string]Instruction {
	out := map[string]Instruction{}
	roles, ok := Roles17(in)
	if !ok {
		return out
	}
	for i, role := range roles {
		set := func(name string, v uint32) {
			m := in
			switch i {
			case 0:
				m.A = v
			case 1:
				m.B = v
			case 2:
				m.C = v
			}
			out[fmt.Sprintf("operand %c %s", 'A'+i, name)] = m
		}
		switch role {
		case RoleNone:
			set("unused but not zero", 1)
		case RoleUse, RoleDef, RoleUseDef:
			set("is a location", 2)
			set("out of range", 99)
			set("far out of range", 0xffffffff)
		case RoleLocDef, RoleLocUse:
			set("is a value", 0)
			set("out of range", 99)
		case RoleString:
			set("is not a string", k17Int)
			set("out of range", 999)
			set("far out of range", 0xffffffff)
		case RoleClass:
			set("out of range", 3)
			set("far out of range", 0xffffffff)
		case RoleTarget:
			set("out of range", 999)
		}
	}
	m := in
	m.Aux |= 1 << 15
	out["aux bit 15"] = m
	m = in
	m.Aux |= 1 << 8
	out["aux bit 8"] = m
	if in.Op != OpPropSet && in.Op != OpStaticSet {
		// MemberResult is bit 7 of these two only.
		m = in
		m.Aux |= MemberResult
		out["aux bit 7"] = m
	}
	switch in.Op {
	case OpNew, OpInitStatic, OpStaticGet, OpStaticSet, OpFetchStatic, OpStaticRef, OpClassConst, OpClassName, OpInstanceof:
		for ref := uint16(ClassValue) + 1; ref <= 7; ref++ {
			m = in
			m.Aux = m.Aux&^7 | ref
			out[fmt.Sprintf("class ref %d", ref)] = m
		}
	}
	return out
}

func TestFormat17RejectsInvalidOperands(t *testing.T) {
	for name, body := range format17ValidBodies() {
		if !is17(body[0].Op) {
			continue
		}
		for mname, m := range operandMutations(body[0]) {
			t.Run(name+": "+mname, func(t *testing.T) {
				b := append([]Instruction{m}, body[1:]...)
				image := format17Image(b...)
				if err := checkOperands17(image, &image.Functions[0], m); err == nil {
					t.Fatalf("checkOperands17 accepted %+v", m)
				}
				requireBuildError(t, image, ErrInvalid)
			})
		}
	}
	// Aux values each opcode does not define.
	objLoc := MemberObjLocation
	mode := func(m MemberMode) uint16 { return uint16(m) << 4 }
	cases := map[string]Instruction{
		"declare class aux":              {Op: OpDeclareClass, Aux: 1, B: 0},
		"new name reg":                   {Op: OpNew, Aux: MemberNameReg, A: 3, B: k17C, C: format12Body + 1},
		"new mode":                       {Op: OpNew, Aux: mode(1), A: 3, B: k17C, C: format12Body + 1},
		"new object location":            {Op: OpNew, Aux: objLoc, A: 3, B: k17C, C: format12Body + 1},
		"init method class ref":          {Op: OpInitMethod, Aux: uint16(ClassSelf), A: 0, B: k17m},
		"init method mode":               {Op: OpInitMethod, Aux: mode(1), A: 0, B: k17m},
		"init method object location":    {Op: OpInitMethod, Aux: objLoc, A: 0, B: k17m},
		"init static mode":               {Op: OpInitStatic, Aux: mode(1), A: k17C, B: k17m},
		"init static object location":    {Op: OpInitStatic, Aux: objLoc, A: k17C, B: k17m},
		"this aux":                       {Op: OpThis, Aux: 1, A: 3},
		"prop get result":                {Op: OpPropGet, Aux: MemberResult, A: 3, B: 0, C: k17p},
		"prop get class ref":             {Op: OpPropGet, Aux: uint16(ClassSelf), A: 3, B: 0, C: k17p},
		"prop set mode":                  {Op: OpPropSet, Aux: mode(1), A: 0, B: k17p, C: 0},
		"prop set class ref":             {Op: OpPropSet, Aux: uint16(ClassStatic), A: 0, B: k17p, C: 0},
		"fetch prop mode 3":              {Op: OpFetchProp, Aux: mode(3), A: 4, B: 0, C: k17p},
		"fetch prop class ref":           {Op: OpFetchProp, Aux: uint16(ClassSelf), A: 4, B: 0, C: k17p},
		"prop ref object location":       {Op: OpPropRef, Aux: objLoc, A: 3, B: 4, C: k17p},
		"prop ref mode":                  {Op: OpPropRef, Aux: mode(1), A: 3, B: 0, C: k17p},
		"prop isset read mode":           {Op: OpPropIsset, A: 3, B: 0, C: k17p},
		"prop isset quiet mode":          {Op: OpPropIsset, Aux: mode(MemberQuiet), A: 3, B: 0, C: k17p},
		"prop isset object location":     {Op: OpPropIsset, Aux: mode(MemberIsset) | objLoc, A: 3, B: 4, C: k17p},
		"prop unset mode":                {Op: OpPropUnset, Aux: mode(1), A: 0, B: k17p},
		"prop unset class ref":           {Op: OpPropUnset, Aux: uint16(ClassSelf), A: 0, B: k17p},
		"static get object location":     {Op: OpStaticGet, Aux: objLoc, A: 3, B: k17C, C: k17p},
		"static set mode":                {Op: OpStaticSet, Aux: mode(1), A: 0, B: k17C, C: k17p},
		"static set object location":     {Op: OpStaticSet, Aux: objLoc, A: 0, B: k17C, C: k17p},
		"fetch static unset mode":        {Op: OpFetchStatic, Aux: mode(MemberUnset), A: 4, B: k17C, C: k17p},
		"fetch static object location":   {Op: OpFetchStatic, Aux: objLoc, A: 4, B: k17C, C: k17p},
		"static ref mode":                {Op: OpStaticRef, Aux: mode(1), A: 3, B: k17C, C: k17p},
		"static ref object location":     {Op: OpStaticRef, Aux: objLoc, A: 3, B: k17C, C: k17p},
		"class const name reg":           {Op: OpClassConst, Aux: MemberNameReg, A: 3, B: k17C, C: 0},
		"class const mode":               {Op: OpClassConst, Aux: mode(1), A: 3, B: k17C, C: k17K},
		"class name name reg":            {Op: OpClassName, Aux: MemberNameReg, A: 3, B: k17C},
		"instanceof name reg":            {Op: OpInstanceof, Aux: MemberNameReg, A: 3, B: 0, C: k17C},
		"clone aux":                      {Op: OpClone, Aux: 1, A: 3, B: 0},
		"verify return aux 3":            {Op: OpVerifyReturn, Aux: uint16(ReturnNever) + 1, A: 0},
		"fetch prop object location A!B": {Op: OpFetchProp, Aux: objLoc, A: 4, B: 2, C: k17p},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			image := format17Image(in)
			if err := checkOperands17(image, &image.Functions[0], in); err == nil {
				t.Fatalf("checkOperands17 accepted %+v", in)
			}
			requireBuildError(t, image, ErrInvalid)
		})
	}
	t.Run("a fetch from a location replaces it", func(t *testing.T) {
		in := Instruction{Op: OpFetchProp, Aux: objLoc, A: 4, B: 2, C: k17p}
		image := format17Image(in)
		err := checkOperands17(image, &image.Functions[0], in)
		var e *Error
		if !errors.As(err, &e) || !strings.Contains(e.Detail, "must replace that location") {
			t.Fatalf("checkOperands17 = %v, want the replace rule", err)
		}
	})
	t.Run("the push and cast Aux values", func(t *testing.T) {
		for name, body := range map[string][]Instruction{
			"push auto of a value":      {{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpPush, Aux: uint16(PushAuto), A: 0}, {Op: OpCallPending, A: 3}},
			"push temp of a location":   {{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpPush, Aux: uint16(PushTemp), A: 2}, {Op: OpCallPending, A: 3}},
			"push result of a location": {{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpPush, Aux: uint16(PushResult), A: 2}, {Op: OpCallPending, A: 3}},
			"push aux 5":                {{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpPush, Aux: uint16(PushResult) + 1, A: 0}, {Op: OpCallPending, A: 3}},
			"cast aux past object":      {{Op: OpCast, Aux: uint16(CastObject) + 1, A: 3, B: 0}},
			"cast object into a loc":    {{Op: OpCast, Aux: uint16(CastObject), A: 4, B: 0}},
		} {
			t.Run(name, func(t *testing.T) { requireBuildError(t, format17Image(body...), ErrInvalid) })
		}
	})
}

func requireBuildDetail(t *testing.T, image Image, detail string) *Error {
	t.Helper()
	_, err := Build(context.Background(), image, testOptions())
	var e *Error
	if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, detail) {
		t.Fatalf("Build = %v, want ErrInvalid with %q", err, detail)
	}
	return e
}

func TestFormat17Flow(t *testing.T) {
	const b = format12Body
	objLoc := MemberObjLocation
	t.Run("a read before definition", func(t *testing.T) {
		for name, in := range map[string]Instruction{
			"init method object":       {Op: OpInitMethod, A: 3, B: k17m},
			"init static class value":  {Op: OpInitStatic, Aux: uint16(ClassValue), A: 3, B: k17m},
			"new class value":          {Op: OpNew, Aux: uint16(ClassValue), A: 0, B: 3, C: b + 1},
			"prop get object":          {Op: OpPropGet, A: 0, B: 3, C: k17p},
			"prop get name":            {Op: OpPropGet, Aux: MemberNameReg, A: 0, B: 0, C: 3},
			"prop get element":         {Op: OpPropGet, Aux: objLoc, A: 0, B: 4, C: k17p},
			"prop set value":           {Op: OpPropSet, A: 0, B: k17p, C: 3},
			"prop set object":          {Op: OpPropSet, A: 3, B: k17p, C: 0},
			"prop set element":         {Op: OpPropSet, Aux: objLoc, A: 4, B: k17p, C: 0},
			"fetch prop object":        {Op: OpFetchProp, A: 4, B: 3, C: k17p},
			"prop ref object":          {Op: OpPropRef, A: 0, B: 3, C: k17p},
			"prop isset object":        {Op: OpPropIsset, Aux: uint16(MemberIsset) << 4, A: 0, B: 3, C: k17p},
			"prop unset object":        {Op: OpPropUnset, A: 3, B: k17p},
			"prop unset element":       {Op: OpPropUnset, Aux: objLoc, A: 4, B: k17p},
			"static get class value":   {Op: OpStaticGet, Aux: uint16(ClassValue), A: 0, B: 3, C: k17p},
			"static set value":         {Op: OpStaticSet, A: 3, B: k17C, C: k17p},
			"fetch static class value": {Op: OpFetchStatic, Aux: uint16(ClassValue), A: 4, B: 3, C: k17p},
			"static ref name":          {Op: OpStaticRef, Aux: MemberNameReg, A: 0, B: k17C, C: 3},
			"class const class value":  {Op: OpClassConst, Aux: uint16(ClassValue), A: 0, B: 3, C: k17K},
			"class name value":         {Op: OpClassName, Aux: uint16(ClassValue), A: 0, B: 3},
			"instanceof value":         {Op: OpInstanceof, A: 0, B: 3, C: k17C},
			"instanceof class value":   {Op: OpInstanceof, Aux: uint16(ClassValue), A: 0, B: 0, C: 3},
			"clone":                    {Op: OpClone, A: 0, B: 3},
			"verify return":            {Op: OpVerifyReturn, A: 3},
		} {
			t.Run(name, func(t *testing.T) {
				body := []Instruction{in}
				switch in.Op {
				case OpInitMethod, OpInitStatic:
					body = append(body, Instruction{Op: OpCallPending, A: 0})
				case OpNew:
					body = append(body, Instruction{Op: OpCallPending, A: 0})
				}
				e := requireBuildDetail(t, format17Image(body...), "before definition")
				if e.PC != b {
					t.Fatalf("error at pc %d, want %d", e.PC, b)
				}
			})
		}
	})
	t.Run("definitions", func(t *testing.T) {
		for name, body := range map[string][]Instruction{
			"this":         {{Op: OpThis, A: 3}, {Op: OpOutput, A: 3}},
			"prop isset":   {{Op: OpPropIsset, Aux: uint16(MemberIsset) << 4, A: 3, B: 0, C: k17p}, {Op: OpOutput, A: 3}},
			"static get":   {{Op: OpStaticGet, A: 3, B: k17C, C: k17p}, {Op: OpOutput, A: 3}},
			"class const":  {{Op: OpClassConst, A: 3, B: k17C, C: k17K}, {Op: OpOutput, A: 3}},
			"instanceof":   {{Op: OpInstanceof, A: 3, B: 0, C: k17C}, {Op: OpOutput, A: 3}},
			"clone":        {{Op: OpClone, A: 3, B: 0}, {Op: OpOutput, A: 3}},
			"fetch static": {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpStore, A: 4, B: 0}},
		} {
			t.Run(name, func(t *testing.T) { requireValidBuild(t, format17Image(body...)) })
		}
	})
	t.Run("new", func(t *testing.T) {
		// 3 New r3, 7; 4 Push r0; 5 Push auto L2; 6 CallPending r0; 7 Output r3.
		valid := format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 4},
			Instruction{Op: OpPush, A: 0},
			Instruction{Op: OpPush, Aux: uint16(PushAuto), A: 2},
			Instruction{Op: OpCallPending, A: 0},
			Instruction{Op: OpOutput, A: 3},
		)
		requireValidBuild(t, valid)
		// 3 New r3, 6; 4 CallPending r0; 5 Return r0; 6 CallPending r0;
		// 7 Return r0: the jump edge started no call.
		e := requireBuildDetail(t, format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 3},
			Instruction{Op: OpCallPending, A: 0},
			Instruction{Op: OpReturn, A: 0},
			Instruction{Op: OpCallPending, A: 0},
		), "without a started call")
		if e.PC != b+3 {
			t.Fatalf("error at pc %d, want %d", e.PC, b+3)
		}
		// The fall-through edge must finish the call before the join.
		requireBuildError(t, format17Image(Instruction{Op: OpNew, A: 3, B: k17C, C: b + 1}), ErrInvalid)
		requireBuildError(t, format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 2}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpOutput, A: 3},
		), ErrInvalid)
		// A started call of the fall-through edge left pending at a return.
		requireBuildDetail(t, format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 2}, Instruction{Op: OpReturn, A: 0}, Instruction{Op: OpReturn, A: 0},
		), "pending arguments")
		// The object is defined on both edges.
		requireValidBuild(t, format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 2}, Instruction{Op: OpCallPending, A: 0}, Instruction{Op: OpReturn, A: 3},
		))
		t.Run("as the last instruction", func(t *testing.T) {
			i := format17Image()
			f := &i.Functions[0]
			f.Code = append(f.Code[:format12Body:format12Body], Instruction{Op: OpNew, A: 3, B: k17C, C: 0})
			f.Positions = make([]Position, len(f.Code))
			requireBuildError(t, i, ErrInvalid)
		})
	})
	t.Run("an unknown callee returns a value", func(t *testing.T) {
		for name, start := range map[string]Instruction{
			"init method": {Op: OpInitMethod, A: 0, B: k17m},
			"init static": {Op: OpInitStatic, A: k17C, B: k17sm},
		} {
			t.Run(name, func(t *testing.T) {
				requireBuildDetail(t, format17Image(start, Instruction{Op: OpCallPending, A: 4}), "return kind")
				requireValidBuild(t, format17Image(start, Instruction{Op: OpCallPending, A: 3}))
			})
		}
		requireBuildDetail(t, format17Image(
			Instruction{Op: OpNew, A: 3, B: k17C, C: b + 2}, Instruction{Op: OpCallPending, A: 4},
		), "return kind")
	})
	t.Run("a push by the callee's declaration needs an unknown callee", func(t *testing.T) {
		for _, push := range []Instruction{
			{Op: OpPush, Aux: uint16(PushAuto), A: 2},
			{Op: OpPush, Aux: uint16(PushTemp), A: 0},
			{Op: OpPush, Aux: uint16(PushResult), A: 0},
		} {
			for name, body := range map[string][]Instruction{
				"after a call of a known function": {{Op: OpInitCall, B: 1}, push, {Op: OpCallPending, A: 3}},
				"with no started call":             {push, {Op: OpCallValue, A: 3, B: 0, C: 1}},
				"in a known call inside an unknown one": {
					{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpInitCall, B: 1}, push, {Op: OpCallPending, A: 3}, {Op: OpCallPending, A: 3},
				},
				"after the unknown call ended": {{Op: OpInitMethod, A: 0, B: k17m}, {Op: OpCallPending, A: 3}, push, {Op: OpCallValue, A: 3, B: 0, C: 1}},
			} {
				t.Run(fmt.Sprintf("aux %d %s", push.Aux, name), func(t *testing.T) {
					requireBuildDetail(t, format17Image(body...), "resolved at run time")
				})
			}
			t.Run(fmt.Sprintf("aux %d in an unknown call inside a known one", push.Aux), func(t *testing.T) {
				requireValidBuild(t, format17Image(
					Instruction{Op: OpInitCall, B: 1}, Instruction{Op: OpInitMethod, A: 0, B: k17m}, push,
					Instruction{Op: OpCallPending, A: 3}, Instruction{Op: OpCallPending, A: 3},
				))
			})
		}
	})
	t.Run("an argument pushed by the callee's declaration belongs to that call", func(t *testing.T) {
		// A direct call that took the arguments PushAuto sent for the
		// method call would receive what the method declares, a reference
		// for a by-reference parameter, where its own table says by value.
		for name, body := range map[string][]Instruction{
			"push auto to a direct call": {
				{Op: OpInitMethod, A: 0, B: k17m},
				{Op: OpPush, Aux: uint16(PushAuto), A: 2}, {Op: OpPush, Aux: uint16(PushAuto), A: 2},
				{Op: OpCall, A: 3, B: 1, C: 2}, {Op: OpCallPending, A: 3},
			},
			"push auto to a dynamic call": {
				{Op: OpInitMethod, A: 0, B: k17m},
				{Op: OpPush, Aux: uint16(PushAuto), A: 2}, {Op: OpCallValue, A: 3, B: 0, C: 1}, {Op: OpCallPending, A: 3},
			},
			"push temp to a direct call": {
				{Op: OpInitMethod, A: 0, B: k17m},
				{Op: OpPush, Aux: uint16(PushTemp), A: 0}, {Op: OpPush, Aux: uint16(PushTemp), A: 0},
				{Op: OpCall, A: 3, B: 1, C: 2}, {Op: OpCallPending, A: 3},
			},
		} {
			t.Run(name, func(t *testing.T) { requireBuildError(t, format17Image(body...), ErrInvalid) })
		}
		// A direct call nested in the method call's arguments is valid
		// with its own plain pushes.
		requireValidBuild(t, format17Image(
			Instruction{Op: OpInitMethod, A: 0, B: k17m},
			Instruction{Op: OpPush, A: 0}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2},
			Instruction{Op: OpPush, Aux: uint16(PushResult), A: 3}, Instruction{Op: OpCallPending, A: 3},
		))
	})
	t.Run("a property reference is a reference", func(t *testing.T) {
		for name, ref := range map[string]Instruction{
			"prop ref":   {Op: OpPropRef, A: 3, B: 0, C: k17p},
			"static ref": {Op: OpStaticRef, A: 3, B: k17C, C: k17r},
		} {
			t.Run(name, func(t *testing.T) {
				requireBuildDetail(t, format17Image(ref, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2}),
					"reference to by-value parameter 0")
				requireValidBuild(t, format17Image(Instruction{Op: OpPush, A: 0}, ref, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpCall, A: 3, B: 1, C: 2}))
				// Moved to another register, it is still one.
				requireBuildDetail(t, format17Image(ref, Instruction{Op: OpMove, A: 1, B: 3}, Instruction{Op: OpPush, A: 1}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2}),
					"reference to by-value parameter 0")
				// Overwritten, it is not.
				requireValidBuild(t, format17Image(ref, Instruction{Op: OpConst, A: 3, B: 0}, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2}))
			})
		}
		// A property read is not a reference.
		requireValidBuild(t, format17Image(Instruction{Op: OpPropGet, A: 3, B: 0, C: k17p}, Instruction{Op: OpPush, A: 3}, Instruction{Op: OpPush, A: 0}, Instruction{Op: OpCall, A: 3, B: 1, C: 2}))
	})
}

func TestFormat17ElementWindow(t *testing.T) {
	const b = format12Body
	objLoc := MemberObjLocation
	fetchP := Instruction{Op: OpFetchProp, A: 4, B: 0, C: k17p}
	fetchRW := Instruction{Op: OpFetchProp, Aux: uint16(MemberReadWrite) << 4, A: 4, B: 0, C: k17p}
	setThrough := Instruction{Op: OpPropSet, Aux: objLoc, A: 4, B: k17q, C: 0}
	for name, body := range map[string][]Instruction{
		"prop set through a fetched prop":   {fetchP, setThrough},
		"prop unset through a fetched prop": {{Op: OpFetchProp, Aux: uint16(MemberUnset) << 4, A: 4, B: 0, C: k17p}, {Op: OpPropUnset, Aux: objLoc, A: 4, B: k17q}},
		"a chain of fetched props":          {fetchP, {Op: OpFetchProp, Aux: objLoc, A: 4, B: 4, C: k17q}, {Op: OpFetchProp, Aux: objLoc, A: 4, B: 4, C: k17p}, setThrough},
		"a dimension of a fetched prop":     {fetchP, {Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 4, C: 0}, {Op: OpArraySet, A: 4, B: 0, C: 0}},
		"a prop of a fetched dimension":     {{Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 2, C: 0}, {Op: OpFetchProp, Aux: objLoc, A: 4, B: 4, C: k17p}, setThrough},
		"a prop set on a dimension":         {{Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 2, C: 0}, setThrough},
		"an assign-op through a prop": {
			fetchRW, {Op: OpPropGet, Aux: objLoc, A: 3, B: 4, C: k17q}, {Op: OpConst, A: 1, B: 0},
			{Op: OpBinary, Aux: uint16(Add), A: 3, B: 3, C: 1}, {Op: OpPropSet, Aux: objLoc, A: 4, B: k17q, C: 3},
		},
		"a store to a fetched prop":          {fetchP, {Op: OpConst, A: 3, B: 0}, {Op: OpStore, A: 4, B: 3}},
		"fetch static then store":            {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpStore, A: 4, B: 0}},
		"fetch static then array set":        {{Op: OpFetchStatic, Aux: uint16(ClassSelf), A: 4, C: k17r}, {Op: OpArraySet, A: 4, B: 0, C: 0}},
		"fetch static then a dimension":      {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 4, C: 0}, {Op: OpStore, A: 4, B: 0}},
		"fetch static then a prop set":       {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, setThrough},
		"a new window after the last":        {fetchP, setThrough, fetchP, setThrough},
		"a fetched prop at a jump target":    {{Op: OpJump, A: b + 1}, fetchP, setThrough},
		"a fetched prop the code never uses": {fetchP},
	} {
		t.Run("valid "+name, func(t *testing.T) { requireValidBuild(t, format17Image(body...)) })
	}
	for name, body := range map[string][]Instruction{
		"a prop set after a checkpoint":         {fetchP, {Op: OpCheckpoint}, setThrough},
		"a prop set at a jump target":           {fetchP, {Op: OpJump, A: b + 2}, setThrough},
		"a prop set after output":               {fetchP, {Op: OpOutput, A: 0}, setThrough},
		"a prop set after a method call":        {fetchP, {Op: OpInitMethod, A: 0, B: k17m}, {Op: OpCallPending, A: 3}, setThrough},
		"a prop set after a prop get":           {fetchP, {Op: OpPropGet, A: 3, B: 0, C: k17p}, setThrough},
		"a second use after the write":          {fetchP, setThrough, setThrough},
		"a store after a prop set":              {fetchP, setThrough, {Op: OpStore, A: 4, B: 0}},
		"a fetch from the element after a load": {fetchRW, {Op: OpPropGet, Aux: objLoc, A: 3, B: 4, C: k17q}, {Op: OpFetchProp, Aux: objLoc, A: 4, B: 4, C: k17q}},
		"two loads":                             {fetchRW, {Op: OpPropGet, Aux: objLoc, A: 3, B: 4, C: k17q}, {Op: OpPropGet, Aux: objLoc, A: 3, B: 4, C: k17q}, setThrough},
		"a fetch from a closed element":         {fetchP, setThrough, {Op: OpFetchProp, Aux: objLoc, A: 4, B: 4, C: k17q}},
		"a prop of a variable's location":       {{Op: OpFetchProp, Aux: objLoc, A: 2, B: 2, C: k17p}, {Op: OpStore, A: 2, B: 0}},
		"the element pushed":                    {fetchP, {Op: OpPush, A: 4}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"the element loaded twice by a store":   {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpStore, A: 4, B: 0}, {Op: OpStore, A: 4, B: 0}},
		"a fetched static used after a jump":    {{Op: OpFetchStatic, A: 4, B: k17C, C: k17r}, {Op: OpJump, A: b + 2}, {Op: OpStore, A: 4, B: 0}},
	} {
		t.Run("invalid "+name, func(t *testing.T) {
			e := requireBuildDetail(t, format17Image(body...), "element location")
			if e.PC < 0 || e.Function != 0 {
				t.Fatalf("error = %+v, want the instruction", e)
			}
		})
	}
}

// Flipping any byte of the class table gives a Program or an error, never a
// panic, quickly.
func TestFormat17ClassTableCorruption(t *testing.T) {
	data, start := classTableRegion(t)
	began := time.Now()
	loaded, refused := 0, 0
	for off := start; off < len(data); off++ {
		for _, mutate := range []func(byte) byte{func(v byte) byte { return v ^ 0xff }, func(v byte) byte { return v + 1 }} {
			b := append([]byte(nil), data...)
			b[off] = mutate(b[off])
			repairWireIntegrity(b)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Load panicked at offset %d: %v", off, r)
					}
				}()
				p, err := loadBytes(b)
				switch {
				case err == nil && p != nil:
					loaded++
					if _, err := p.Encode(context.Background()); err != nil {
						t.Fatalf("offset %d: a loaded Program does not encode: %v", off, err)
					}
				case err != nil && p == nil:
					refused++
				default:
					t.Fatalf("offset %d: Load=(%v,%v)", off, p, err)
				}
			}()
		}
	}
	if refused == 0 {
		t.Fatal("no corruption of the class table was refused")
	}
	if d := time.Since(began); d > 20*time.Second {
		t.Fatalf("%d loads took %v", loaded+refused, d)
	}
	t.Logf("%d bytes of class table: %d loaded, %d refused", len(data)-start, loaded, refused)
}
