package bytecode

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// format11Frame has r0 Value (defined), r1 Value array (defined), r2
// Location (defined), r3 Value (undefined) and r4 Location (undefined).
var format11Registers = []RegisterKind{Value, Value, Location, Value, Location}

var format11Prologue = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpNewArray, A: 1}, {Op: OpLocal, A: 2, B: 0}}

// format11Image runs the prologue, then body. A body that does not end in a
// terminal instruction is followed by Return r0.
func format11Image(body ...Instruction) Image {
	code := append(append([]Instruction(nil), format11Prologue...), body...)
	if last := code[len(code)-1].Op; last != OpExit && last != OpReturn {
		code = append(code, Instruction{Op: OpReturn, A: 0})
	}
	i := validImage()
	i.Functions[0].Registers = append([]RegisterKind(nil), format11Registers...)
	i.Functions[0].Locals = 1
	i.Functions[0].MaxStack = 2
	i.Functions[0].Code = code
	i.Functions[0].Positions = make([]Position, len(code))
	return i
}

func format11ValidInstructions() map[string]Instruction {
	cases := map[string]Instruction{
		"array set":           {Op: OpArraySet, A: 2, B: 0, C: 0},
		"array append":        {Op: OpArrayAppend, A: 2, B: 0},
		"array unset":         {Op: OpArrayUnset, A: 2, B: 0},
		"isset":               {Op: OpIsset, A: 3, B: 2},
		"array isset":         {Op: OpArrayIsset, A: 3, B: 1, C: 0},
		"unset":               {Op: OpUnset, A: 2},
		"exit without status": {Op: OpExit},
		"exit with status":    {Op: OpExit, Aux: 1, A: 0},
		"quiet load":          {Op: OpLoad, Aux: 1, A: 3, B: 2},
		"quiet array get":     {Op: OpArrayGet, Aux: 1, A: 3, B: 1, C: 0},
	}
	for aux := Not; aux <= BitNot; aux++ {
		cases[fmt.Sprintf("unary %d", aux)] = Instruction{Op: OpUnary, Aux: uint16(aux), A: 3, B: 0}
	}
	for aux := CastBool; aux <= CastArray; aux++ {
		cases[fmt.Sprintf("cast %d", aux)] = Instruction{Op: OpCast, Aux: uint16(aux), A: 3, B: 0}
	}
	for aux := PreInc; aux <= PostDec; aux++ {
		cases[fmt.Sprintf("incdec %d", aux)] = Instruction{Op: OpIncDec, Aux: uint16(aux), A: 3, B: 2}
	}
	for op := Modulo; op <= BoolXor; op++ {
		cases[fmt.Sprintf("binary %d", op)] = Instruction{Op: OpBinary, Aux: uint16(op), A: 3, B: 0, C: 0}
	}
	return cases
}

func TestFormat11AcceptsValidOperands(t *testing.T) {
	for name, in := range format11ValidInstructions() {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format11Image(in)) })
	}
	t.Run("unary may overwrite its operand", func(t *testing.T) {
		requireValidBuild(t, format11Image(Instruction{Op: OpUnary, Aux: uint16(Not), A: 0, B: 0}))
	})
	t.Run("unset keeps the location usable", func(t *testing.T) {
		requireValidBuild(t, format11Image(Instruction{Op: OpUnset, A: 2}, Instruction{Op: OpStore, A: 2, B: 0}, Instruction{Op: OpLoad, A: 3, B: 2}, Instruction{Op: OpReturn, A: 3}))
	})
	t.Run("results define their destination", func(t *testing.T) {
		for _, in := range []Instruction{
			{Op: OpUnary, Aux: uint16(Negate), A: 3, B: 0}, {Op: OpCast, Aux: uint16(CastString), A: 3, B: 0},
			{Op: OpIncDec, Aux: uint16(PostInc), A: 3, B: 2}, {Op: OpIsset, A: 3, B: 2}, {Op: OpArrayIsset, A: 3, B: 1, C: 0},
		} {
			requireValidBuild(t, format11Image(in, Instruction{Op: OpReturn, A: 3}))
		}
	})
}

func TestFormat11RejectsInvalidOperands(t *testing.T) {
	cases := map[string]Instruction{
		// Wrong register kind.
		"unary location operand":      {Op: OpUnary, Aux: uint16(Not), A: 3, B: 2},
		"unary location dest":         {Op: OpUnary, Aux: uint16(Not), A: 2, B: 0},
		"cast location dest":          {Op: OpCast, Aux: uint16(CastInt), A: 2, B: 0},
		"cast location operand":       {Op: OpCast, Aux: uint16(CastInt), A: 3, B: 2},
		"incdec value target":         {Op: OpIncDec, Aux: uint16(PreInc), A: 3, B: 0},
		"incdec location dest":        {Op: OpIncDec, Aux: uint16(PreInc), A: 4, B: 2},
		"array set value container":   {Op: OpArraySet, A: 0, B: 0, C: 0},
		"array set location key":      {Op: OpArraySet, A: 2, B: 2, C: 0},
		"array set location value":    {Op: OpArraySet, A: 2, B: 0, C: 2},
		"array append value target":   {Op: OpArrayAppend, A: 1, B: 0},
		"array append location value": {Op: OpArrayAppend, A: 2, B: 2},
		"array unset value container": {Op: OpArrayUnset, A: 1, B: 0},
		"array unset location key":    {Op: OpArrayUnset, A: 2, B: 2},
		"isset value operand":         {Op: OpIsset, A: 3, B: 0},
		"isset location dest":         {Op: OpIsset, A: 4, B: 2},
		"array isset location":        {Op: OpArrayIsset, A: 3, B: 2, C: 0},
		"array isset location key":    {Op: OpArrayIsset, A: 3, B: 1, C: 2},
		"unset value":                 {Op: OpUnset, A: 0},
		"exit location status":        {Op: OpExit, Aux: 1, A: 2},
		"register out of range":       {Op: OpUnary, Aux: uint16(Not), A: 9, B: 0},
		// Read before definition.
		"unary undefined":        {Op: OpUnary, Aux: uint16(Not), A: 0, B: 3},
		"cast undefined":         {Op: OpCast, Aux: uint16(CastBool), A: 0, B: 3},
		"incdec undefined":       {Op: OpIncDec, Aux: uint16(PreDec), A: 3, B: 4},
		"array set undefined":    {Op: OpArraySet, A: 4, B: 0, C: 0},
		"array set key":          {Op: OpArraySet, A: 2, B: 3, C: 0},
		"array set value":        {Op: OpArraySet, A: 2, B: 0, C: 3},
		"array append undefined": {Op: OpArrayAppend, A: 4, B: 0},
		"array append value":     {Op: OpArrayAppend, A: 2, B: 3},
		"array unset undefined":  {Op: OpArrayUnset, A: 4, B: 0},
		"array unset key":        {Op: OpArrayUnset, A: 2, B: 3},
		"isset undefined":        {Op: OpIsset, A: 3, B: 4},
		"array isset container":  {Op: OpArrayIsset, A: 0, B: 3, C: 0},
		"array isset key":        {Op: OpArrayIsset, A: 0, B: 1, C: 3},
		"unset undefined":        {Op: OpUnset, A: 4},
		"exit undefined status":  {Op: OpExit, Aux: 1, A: 3},
		// Nonzero unused operands or Aux.
		"unary c":           {Op: OpUnary, Aux: uint16(Not), A: 3, B: 0, C: 1},
		"cast c":            {Op: OpCast, Aux: uint16(CastBool), A: 3, B: 0, C: 1},
		"incdec c":          {Op: OpIncDec, Aux: uint16(PreInc), A: 3, B: 2, C: 1},
		"array set aux":     {Op: OpArraySet, Aux: 1, A: 2, B: 0, C: 0},
		"array append c":    {Op: OpArrayAppend, A: 2, B: 0, C: 1},
		"array append aux":  {Op: OpArrayAppend, Aux: 1, A: 2, B: 0},
		"array unset c":     {Op: OpArrayUnset, A: 2, B: 0, C: 1},
		"array unset aux":   {Op: OpArrayUnset, Aux: 1, A: 2, B: 0},
		"isset c":           {Op: OpIsset, A: 3, B: 2, C: 1},
		"isset aux":         {Op: OpIsset, Aux: 1, A: 3, B: 2},
		"array isset aux":   {Op: OpArrayIsset, Aux: 1, A: 3, B: 1, C: 0},
		"unset b":           {Op: OpUnset, A: 2, B: 1},
		"unset c":           {Op: OpUnset, A: 2, C: 1},
		"unset aux":         {Op: OpUnset, Aux: 1, A: 2},
		"exit plain a":      {Op: OpExit, A: 1},
		"exit plain b":      {Op: OpExit, B: 1},
		"exit plain c":      {Op: OpExit, C: 1},
		"exit status b":     {Op: OpExit, Aux: 1, A: 0, B: 1},
		"exit status c":     {Op: OpExit, Aux: 1, A: 0, C: 1},
		"quiet load c":      {Op: OpLoad, Aux: 1, A: 3, B: 2, C: 1},
		"store aux":         {Op: OpStore, Aux: 1, A: 2, B: 0},
		"new array aux":     {Op: OpNewArray, Aux: 1, A: 3},
		"output aux":        {Op: OpOutput, Aux: 1, A: 0},
		"move aux":          {Op: OpMove, Aux: 1, A: 3, B: 0},
		"checkpoint aux":    {Op: OpCheckpoint, Aux: 1},
		"unary aux zero":    {Op: OpUnary, A: 3, B: 0},
		"cast aux zero":     {Op: OpCast, A: 3, B: 0},
		"incdec aux zero":   {Op: OpIncDec, A: 3, B: 2},
		"binary aux zero":   {Op: OpBinary, A: 3, B: 0, C: 0},
		"unary aux high":    {Op: OpUnary, Aux: uint16(BitNot) + 1, A: 3, B: 0},
		"cast aux high":     {Op: OpCast, Aux: uint16(CastArray) + 1, A: 3, B: 0},
		"incdec aux high":   {Op: OpIncDec, Aux: uint16(PostDec) + 1, A: 3, B: 2},
		"binary aux high":   {Op: OpBinary, Aux: uint16(BoolXor) + 1, A: 3, B: 0, C: 0},
		"exit aux high":     {Op: OpExit, Aux: 2, A: 0},
		"load aux high":     {Op: OpLoad, Aux: 2, A: 3, B: 2},
		"array get aux 2":   {Op: OpArrayGet, Aux: 2, A: 3, B: 1, C: 0},
		"first unknown op":  {Op: OpExit + 1},
		"unknown op status": {Op: OpExit + 1, A: 0},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format11Image(in), ErrInvalid) })
	}
}

func TestExitIsTerminalAndMayLeavePendingArguments(t *testing.T) {
	push := Instruction{Op: OpPush, A: 0}
	t.Run("exit is the last instruction", func(t *testing.T) {
		requireValidBuild(t, format11Image(Instruction{Op: OpExit, Aux: 1, A: 0}))
	})
	t.Run("exit has no successor", func(t *testing.T) {
		// Return r3 is unreachable, so its undefined operand is not read.
		requireValidBuild(t, format11Image(Instruction{Op: OpExit}, Instruction{Op: OpReturn, A: 3}))
	})
	t.Run("exit during argument construction", func(t *testing.T) {
		requireValidBuild(t, format11Image(push, Instruction{Op: OpExit}))
		requireValidBuild(t, format11Image(push, push, Instruction{Op: OpExit, Aux: 1, A: 0}))
	})
	t.Run("return still requires an empty argument stack", func(t *testing.T) {
		requireBuildError(t, format11Image(push, Instruction{Op: OpReturn, A: 0}), ErrInvalid)
	})
}

// encodeAtMinor encodes image with the payload layout of the given minor
// version (minor 0 has no local names tables). A minor above FormatMinor
// keeps the current layout and only rewrites the header.
func encodeAtMinor(t *testing.T, image Image, minor uint16) []byte {
	t.Helper()
	p, err := Build(context.Background(), image, testOptions())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := encodeAt(context.Background(), p, min(minor, FormatMinor))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	binary.LittleEndian.PutUint16(data[10:12], minor)
	repairWireIntegrity(data)
	return data
}

func loadBytes(data []byte) (*Program, error) {
	return Load(context.Background(), data, LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(data)})
}

func TestLoadAcceptsMinorZeroAndOneAndGatesFormat11(t *testing.T) {
	if FormatVersion != 1 || FormatMinor != 1 {
		t.Fatalf("format version = %d.%d, want 1.1", FormatVersion, FormatMinor)
	}
	encoded, err := requireValidBuild(t, validImage()).Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint16(encoded[10:12]); got != FormatMinor {
		t.Fatalf("Encode wrote minor %d, want %d", got, FormatMinor)
	}
	for _, minor := range []uint16{0, 1} {
		t.Run(fmt.Sprintf("format 1.0 program at minor %d", minor), func(t *testing.T) {
			data := encodeAtMinor(t, validCallAndReferenceImage(), minor)
			p, err := loadBytes(data)
			if err != nil || p == nil {
				t.Fatalf("Load=(%v,%v)", p, err)
			}
			again, err := p.Encode(context.Background())
			if err != nil || binary.LittleEndian.Uint16(again[10:12]) != FormatMinor {
				t.Fatalf("re-encode minor=%v err=%v", again[10:12], err)
			}
		})
	}
	t.Run("minor 2 is unsupported", func(t *testing.T) {
		p, err := loadBytes(encodeAtMinor(t, validImage(), 2))
		if p != nil || !errors.Is(err, ErrVersion) {
			t.Fatalf("Load=(%v,%v), want ErrVersion", p, err)
		}
	})
	for name, in := range format11ValidInstructions() {
		t.Run("minor 0 rejects "+name, func(t *testing.T) {
			image := format11Image(in)
			if p, err := loadBytes(encodeAtMinor(t, image, 1)); err != nil || p == nil {
				t.Fatalf("minor 1 Load=(%v,%v)", p, err)
			}
			p, err := loadBytes(encodeAtMinor(t, image, 0))
			var e *Error
			if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
				t.Fatalf("minor 0 Load=(%v,%v), want ErrInvalid", p, err)
			}
			if e.PC != len(format11Prologue) || !strings.Contains(e.Detail, "format 1.1") {
				t.Fatalf("minor 0 error = %+v, want the instruction and a format 1.1 detail", e)
			}
		})
	}
}

// format11Everything uses every format 1.1 opcode, Aux value class and the
// new binary operator range in one valid function.
func format11Everything() Image {
	return format11Image(
		Instruction{Op: OpStore, A: 2, B: 0},
		Instruction{Op: OpUnary, Aux: uint16(BitNot), A: 3, B: 0},
		Instruction{Op: OpCast, Aux: uint16(CastArray), A: 3, B: 0},
		Instruction{Op: OpIncDec, Aux: uint16(PostDec), A: 3, B: 2},
		Instruction{Op: OpArraySet, A: 2, B: 0, C: 3},
		Instruction{Op: OpArrayAppend, A: 2, B: 3},
		Instruction{Op: OpArrayUnset, A: 2, B: 0},
		Instruction{Op: OpIsset, A: 3, B: 2},
		Instruction{Op: OpArrayIsset, A: 3, B: 1, C: 0},
		Instruction{Op: OpUnset, A: 2},
		Instruction{Op: OpLoad, Aux: 1, A: 3, B: 2},
		Instruction{Op: OpArrayGet, Aux: 1, A: 3, B: 1, C: 0},
		Instruction{Op: OpBinary, Aux: uint16(Modulo), A: 3, B: 3, C: 0},
		Instruction{Op: OpBinary, Aux: uint16(BoolXor), A: 3, B: 3, C: 0},
		Instruction{Op: OpExit, Aux: 1, A: 3},
	)
}

func TestFormat11EncodeLoadRoundTrip(t *testing.T) {
	image := format11Everything()
	data := encodeAtMinor(t, image, FormatMinor)
	p, err := loadBytes(data)
	if err != nil || p == nil {
		t.Fatalf("Load=(%v,%v)", p, err)
	}
	view, _ := p.Function(0)
	want := image.Functions[0].Code
	if view.Len() != len(want) {
		t.Fatalf("len = %d, want %d", view.Len(), len(want))
	}
	for pc, in := range want {
		if got, ok := view.Instruction(uint32(pc)); !ok || got != in {
			t.Fatalf("instruction %d = %+v, want %+v", pc, got, in)
		}
	}
	again, err := p.Encode(context.Background())
	if err != nil || string(again) != string(data) {
		t.Fatalf("canonical re-encode differs: err=%v", err)
	}
}

func TestFormat11Safety(t *testing.T) {
	for op := OpUnary; op <= OpExit; op++ {
		s, ok := op.Safety()
		want := Safety{Steps: 1, CheckCancel: true, MayAllocate: true, MayThrow: true, MayCallUser: true}
		if !ok || s != want {
			t.Errorf("Safety(%d) = %+v, %v; want %+v", op, s, ok, want)
		}
	}
	if _, ok := (OpExit + 1).Safety(); ok {
		t.Errorf("opcode %d reported known", OpExit+1)
	}
}

func TestProgramInstructionsAndConstantsAreCopies(t *testing.T) {
	image := format11Everything()
	image.Constants = append(image.Constants, Constant{Kind: String, Bytes: "name"})
	p := requireValidBuild(t, image)
	prefix := []Instruction{{Op: OpCheckpoint}}
	code, ok := p.Instructions(0, prefix)
	if !ok || len(code) != 1+len(image.Functions[0].Code) || code[0] != prefix[0] {
		t.Fatalf("Instructions = %d, %v", len(code), ok)
	}
	for pc, in := range image.Functions[0].Code {
		if code[pc+1] != in {
			t.Fatalf("instruction %d = %+v, want %+v", pc, code[pc+1], in)
		}
	}
	code[1].Op = OpShell
	code[2].A = 99
	if view, _ := p.Function(0); func() bool { in, _ := view.Instruction(0); return in != image.Functions[0].Code[0] }() {
		t.Fatal("mutating the Instructions copy changed the Program")
	}
	again, _ := p.Instructions(0, nil)
	if again[0] != image.Functions[0].Code[0] || again[1] != image.Functions[0].Code[1] {
		t.Fatal("Program code changed after mutating a returned copy")
	}
	if got, ok := p.Instructions(1, prefix); ok || len(got) != 1 {
		t.Fatalf("out-of-range Instructions = %d, %v", len(got), ok)
	}
	consts := p.Constants(nil)
	if len(consts) != 2 || consts[1] != image.Constants[1] {
		t.Fatalf("Constants = %+v", consts)
	}
	consts[0].Kind = String
	consts[1].Bytes = "changed"
	if c, _ := p.Constant(0); c.Kind != Null {
		t.Fatal("mutating the Constants copy changed the Program")
	}
	if c, _ := p.Constant(1); c.Bytes != "name" {
		t.Fatal("mutating the Constants copy changed the Program")
	}
	if got := p.Constants(consts[:1]); len(got) != 3 || got[0].Kind != String {
		t.Fatalf("Constants did not append to dst: %+v", got)
	}
}
