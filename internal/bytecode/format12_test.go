package bytecode

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Format 1.2 (W-17 stage 2): REQ-025; T-049, T-050. The frame is the format
// 1.1 one with r0 as the function's parameter: r0 Value (defined), r1 Value
// (defined, an array), r2 Location (defined), r3 Value (undefined), r4
// Location (undefined). The body starts at pc 3; a one-instruction body is
// followed by Return r0 at pc 4.
func format12Image(body ...Instruction) Image {
	i := format11Image(body...)
	i.Functions[0].Params = 1
	return i
}

const (
	format12Body = 3 // pc of the first body instruction
	format12Next = 4 // pc after a one-instruction body
)

func arraySetAux(t *testing.T, flags ArrayWriteFlags, result uint32) uint16 {
	t.Helper()
	aux, ok := ArraySetAux(flags, result)
	if !ok {
		t.Fatalf("ArraySetAux(%d, %d) not encodable", flags, result)
	}
	return aux
}

func format12ValidInstructions(t *testing.T) map[string]Instruction {
	cases := map[string]Instruction{
		"fetch write":                    {Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 2, C: 0},
		"fetch read-write":               {Op: OpFetchDim, Aux: uint16(FetchReadWrite), A: 4, B: 2, C: 0},
		"fetch unset":                    {Op: OpFetchDim, Aux: uint16(FetchUnset), A: 4, B: 2, C: 0},
		"fetch key location":             {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchKeyLocation), A: 4, B: 2, C: 2},
		"fetch unset key location":       {Op: OpFetchDim, Aux: uint16(FetchUnset | FetchKeyLocation), A: 4, B: 2, C: 2},
		"fetch append":                   {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchAppend), A: 4, B: 2},
		"fetch read-write append":        {Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchAppend | FetchForAssignOp), A: 4, B: 2},
		"fetch for incdec":               {Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchForIncDec), A: 4, B: 2, C: 0},
		"fetch for assign-op":            {Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchForAssignOp | FetchKeyLocation), A: 4, B: 2, C: 2},
		"iter init":                      {Op: OpIterInit, A: 3, B: 1, C: format12Next},
		"iter init over itself":          {Op: OpIterInit, A: 1, B: 1, C: format12Next},
		"iter next value":                {Op: OpIterNext, A: 1, B: 3, C: format12Next},
		"iter next location":             {Op: OpIterNext, Aux: uint16(IterLocation), A: 1, B: 2, C: format12Next},
		"iter key value":                 {Op: OpIterKey, A: 3, B: 1},
		"iter key location":              {Op: OpIterKey, Aux: uint16(IterLocation), A: 2, B: 1},
		"param default":                  {Op: OpParamDefault, A: 0, B: format12Next},
		"param default backwards":        {Op: OpParamDefault, A: 0, B: 0},
		"array set result":               {Op: OpArraySet, Aux: arraySetAux(t, 0, 3), A: 2, B: 0, C: 0},
		"array set result over value":    {Op: OpArraySet, Aux: arraySetAux(t, 0, 0), A: 2, B: 0, C: 0},
		"array set result and locations": {Op: OpArraySet, Aux: arraySetAux(t, KeyLocation|ValueLocation, 3), A: 2, B: 2, C: 2},
	}
	return cases
}

func TestFormat12AcceptsValidOperands(t *testing.T) {
	for name, in := range format12ValidInstructions(t) {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, format12Image(in)) })
	}
	t.Run("destinations are defined on the fall-through path", func(t *testing.T) {
		for name, body := range map[string][]Instruction{
			"fetch dim":        {{Op: OpFetchDim, A: 4, B: 2, C: 0}, {Op: OpStore, A: 4, B: 0}},
			"iter init":        {{Op: OpIterInit, A: 3, B: 1, C: format12Body + 2}, {Op: OpIterNext, A: 3, B: 0, C: format12Body + 2}},
			"iter next":        {{Op: OpIterNext, A: 1, B: 3, C: format12Body + 2}, {Op: OpOutput, A: 3}},
			"iter key":         {{Op: OpIterKey, A: 3, B: 1}, {Op: OpOutput, A: 3}},
			"array set result": {{Op: OpArraySet, Aux: arraySetAux(t, 0, 3), A: 2, B: 0, C: 0}, {Op: OpOutput, A: 3}},
		} {
			t.Run(name, func(t *testing.T) { requireValidBuild(t, format12Image(body...)) })
		}
	})
	t.Run("a foreach loop", func(t *testing.T) {
		// 3 IterInit r3, r1, 8; 4 IterNext r3, L2, 7; 5 IterKey r0, r3;
		// 6 Jump 4; 7 Drop r3; 8 Return r0.
		requireValidBuild(t, format12Image(
			Instruction{Op: OpIterInit, A: 3, B: 1, C: 8},
			Instruction{Op: OpIterNext, Aux: uint16(IterLocation), A: 3, B: 2, C: 7},
			Instruction{Op: OpIterKey, A: 0, B: 3},
			Instruction{Op: OpJump, A: 4},
			Instruction{Op: OpDrop, A: 3},
			Instruction{Op: OpReturn, A: 0},
		))
	})
}

func TestFormat12RejectsInvalidOperands(t *testing.T) {
	big, _ := ArraySetAux(0, 4) // names r4, a Location
	cases := map[string]Instruction{
		// Wrong register kind.
		"fetch value dest":              {Op: OpFetchDim, A: 3, B: 2, C: 0},
		"fetch value container":         {Op: OpFetchDim, A: 4, B: 1, C: 0},
		"fetch location key":            {Op: OpFetchDim, A: 4, B: 2, C: 2},
		"fetch key location value":      {Op: OpFetchDim, Aux: uint16(FetchKeyLocation), A: 4, B: 2, C: 0},
		"iter init location iterator":   {Op: OpIterInit, A: 4, B: 1, C: format12Next},
		"iter init location subject":    {Op: OpIterInit, A: 3, B: 2, C: format12Next},
		"iter next location iterator":   {Op: OpIterNext, A: 2, B: 3, C: format12Next},
		"iter next location dest":       {Op: OpIterNext, A: 1, B: 2, C: format12Next},
		"iter next value dest location": {Op: OpIterNext, Aux: uint16(IterLocation), A: 1, B: 3, C: format12Next},
		"iter key location dest":        {Op: OpIterKey, A: 2, B: 1},
		"iter key value dest location":  {Op: OpIterKey, Aux: uint16(IterLocation), A: 3, B: 1},
		"iter key location iterator":    {Op: OpIterKey, A: 3, B: 2},
		"param default not a parameter": {Op: OpParamDefault, A: 1, B: format12Next},
		"param default location":        {Op: OpParamDefault, A: 2, B: format12Next},
		"array set location result":     {Op: OpArraySet, Aux: big, A: 2, B: 0, C: 0},
		"register out of range":         {Op: OpFetchDim, A: 9, B: 2, C: 0},
		"iter register out of range":    {Op: OpIterNext, A: 9, B: 3, C: format12Next},
		// Read before definition.
		"fetch undefined container":     {Op: OpFetchDim, A: 2, B: 4, C: 0},
		"fetch undefined key":           {Op: OpFetchDim, A: 4, B: 2, C: 3},
		"fetch undefined key location":  {Op: OpFetchDim, Aux: uint16(FetchKeyLocation), A: 2, B: 2, C: 4},
		"iter init undefined subject":   {Op: OpIterInit, A: 0, B: 3, C: format12Next},
		"iter next undefined iterator":  {Op: OpIterNext, A: 3, B: 0, C: format12Next},
		"iter next undefined location":  {Op: OpIterNext, Aux: uint16(IterLocation), A: 1, B: 4, C: format12Next},
		"iter key undefined iterator":   {Op: OpIterKey, A: 0, B: 3},
		"iter key undefined location":   {Op: OpIterKey, Aux: uint16(IterLocation), A: 4, B: 1},
		"array set result still checks": {Op: OpArraySet, Aux: arraySetAuxRaw(0, 0), A: 2, B: 3, C: 0},
		// Jump targets.
		"iter init target out of range":     {Op: OpIterInit, A: 3, B: 1, C: 99},
		"iter next target out of range":     {Op: OpIterNext, A: 1, B: 3, C: 99},
		"param default target out of range": {Op: OpParamDefault, A: 0, B: 99},
		// Nonzero unused operands or Aux.
		"fetch append with key":         {Op: OpFetchDim, Aux: uint16(FetchAppend), A: 4, B: 2, C: 1},
		"fetch append key location":     {Op: OpFetchDim, Aux: uint16(FetchAppend | FetchKeyLocation), A: 4, B: 2, C: 2},
		"fetch unset append":            {Op: OpFetchDim, Aux: uint16(FetchUnset | FetchAppend), A: 4, B: 2},
		"fetch unset for incdec":        {Op: OpFetchDim, Aux: uint16(FetchUnset | FetchForIncDec), A: 4, B: 2, C: 0},
		"fetch mode 3":                  {Op: OpFetchDim, Aux: 3, A: 4, B: 2, C: 0},
		"fetch read-write for ref":      {Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchForRef), A: 4, B: 2, C: 0},
		"fetch aux high":                {Op: OpFetchDim, Aux: 1 << 6, A: 4, B: 2, C: 0},
		"iter init aux":                 {Op: OpIterInit, Aux: 1, A: 3, B: 1, C: format12Next},
		"iter next aux high":            {Op: OpIterNext, Aux: 4, A: 1, B: 3, C: format12Next},
		"iter key aux high":             {Op: OpIterKey, Aux: 2, A: 3, B: 1},
		"iter key c":                    {Op: OpIterKey, A: 3, B: 1, C: 1},
		"param default aux":             {Op: OpParamDefault, Aux: 1, A: 0, B: format12Next},
		"param default c":               {Op: OpParamDefault, A: 0, B: format12Next, C: 1},
		"array set aux bit 2":           {Op: OpArraySet, Aux: 1 << 2, A: 2, B: 0, C: 0},
		"array set result out of range": {Op: OpArraySet, Aux: arraySetAuxRaw(0, 9), A: 2, B: 0, C: 0},
		"array append result":           {Op: OpArrayAppend, Aux: arraySetAuxRaw(0, 3), A: 2, B: 0},
		"first unknown op":              {Op: OpMakeRef + 1},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) { requireBuildError(t, format12Image(in), ErrInvalid) })
	}
	t.Run("ArraySetAux refuses a register it cannot name", func(t *testing.T) {
		if _, ok := ArraySetAux(0, ArraySetMaxResult); ok {
			t.Fatal("ArraySetAux accepted the first unencodable register")
		}
		aux, ok := ArraySetAux(KeyLocation, ArraySetMaxResult-1)
		r, has := ArraySetResult(aux)
		if !ok || !has || r != ArraySetMaxResult-1 || ArrayWriteFlags(aux&3) != KeyLocation {
			t.Fatalf("ArraySetAux round trip = %#x, %v; result %d, %v", aux, ok, r, has)
		}
		if _, has := ArraySetResult(uint16(KeyLocation | ValueLocation)); has {
			t.Fatal("a format 1.1 Aux decoded a result register")
		}
	})
}

func arraySetAuxRaw(flags ArrayWriteFlags, result uint32) uint16 {
	return uint16(flags) | uint16(result+1)<<ArraySetResultShift
}

// A destination written on one outgoing edge only is undefined on the other.
func TestFormat12EdgeDefinitions(t *testing.T) {
	for name, body := range map[string][]Instruction{
		// IterInit r3, r1 jumps to pc 5 for a non-iterable subject, where r3
		// was not written.
		"iter init leaves the iterator undefined on the jump": {
			{Op: OpIterInit, A: 3, B: 1, C: format12Body + 2},
			{Op: OpJump, A: format12Body + 2},
			{Op: OpIterNext, A: 3, B: 0, C: format12Body + 3},
		},
		// The jump also discards an iterator register that was defined: the
		// instruction clears it, so a later read is refused.
		"iter init clears a defined iterator on the jump": {
			{Op: OpIterInit, A: 0, B: 1, C: format12Body + 2},
			{Op: OpJump, A: format12Body + 2},
			{Op: OpOutput, A: 0},
		},
		"iter next leaves the element undefined on the jump": {
			{Op: OpIterNext, A: 1, B: 3, C: format12Body + 2},
			{Op: OpJump, A: format12Body + 2},
			{Op: OpOutput, A: 3},
		},
		"fetch does not define its key": {
			{Op: OpFetchDim, A: 4, B: 2, C: 0},
			{Op: OpOutput, A: 3},
		},
		"param default defines nothing": {
			{Op: OpParamDefault, A: 0, B: format12Body + 1},
			{Op: OpOutput, A: 3},
		},
	} {
		t.Run(name, func(t *testing.T) {
			image := format12Image(body...)
			requireBuildError(t, image, ErrInvalid)
			_, err := Build(context.Background(), image, testOptions())
			var e *Error
			if !errors.As(err, &e) || !strings.Contains(e.Detail, "read before definition") {
				t.Fatalf("error = %v, want a read before definition", err)
			}
		})
	}
	t.Run("iter next keeps the element defined when it already was", func(t *testing.T) {
		requireValidBuild(t, format12Image(
			Instruction{Op: OpIterNext, A: 1, B: 0, C: format12Body + 2},
			Instruction{Op: OpJump, A: format12Body + 2},
			Instruction{Op: OpOutput, A: 0},
		))
	})
}

// The new branches are edges like Branch's: they may not enter a handler body.
func TestFormat12BranchesRespectHandlers(t *testing.T) {
	for name, in := range map[string]Instruction{
		"iter init":     {Op: OpIterInit, A: 3, B: 1, C: 6},
		"iter next":     {Op: OpIterNext, A: 1, B: 3, C: 6},
		"param default": {Op: OpParamDefault, A: 0, B: 6},
	} {
		t.Run(name, func(t *testing.T) {
			// 3 in; 4 Leave 8; 5 Return r0; 6 Checkpoint; 7 EndFinally;
			// 8 Return r0. The finally body is [6,8).
			image := format12Image(in,
				Instruction{Op: OpLeave, A: 8},
				Instruction{Op: OpReturn, A: 0},
				Instruction{Op: OpCheckpoint},
				Instruction{Op: OpEndFinally},
				Instruction{Op: OpReturn, A: 0},
			)
			image.Functions[0].Handlers = []Handler{{Start: 3, End: 5, Target: 6, BodyEnd: 8, Kind: Finally}}
			_, err := Build(context.Background(), image, testOptions())
			var e *Error
			if !errors.Is(err, ErrInvalid) || !errors.As(err, &e) || !strings.Contains(e.Detail, "handler body") {
				t.Fatalf("Build error = %v, want normal control entering a handler body", err)
			}
		})
	}
}

func TestLoadAcceptsMinorTwoAndGatesFormat12(t *testing.T) {
	if FormatVersion != 1 || FormatMinor != 3 {
		t.Fatalf("format version = %d.%d, want 1.3", FormatVersion, FormatMinor)
	}
	t.Run("minor 4 is unsupported", func(t *testing.T) {
		p, err := loadBytes(encodeAtMinor(t, validImage(), 4))
		if p != nil || !errors.Is(err, ErrVersion) {
			t.Fatalf("Load=(%v,%v), want ErrVersion", p, err)
		}
	})
	t.Run("a format 1.1 file still loads", func(t *testing.T) {
		data := encodeAtMinor(t, format11Everything(), 1)
		if got := binary.LittleEndian.Uint16(data[10:12]); got != 1 {
			t.Fatalf("test file declares minor %d", got)
		}
		p, err := loadBytes(data)
		if err != nil || p == nil {
			t.Fatalf("Load=(%v,%v)", p, err)
		}
		again, err := p.Encode(context.Background())
		if err != nil || binary.LittleEndian.Uint16(again[10:12]) != FormatMinor {
			t.Fatalf("re-encode minor=%v err=%v", again[10:12], err)
		}
	})
	for name, in := range format12ValidInstructions(t) {
		for _, minor := range []uint16{0, 1} {
			t.Run(fmt.Sprintf("minor %d rejects %s", minor, name), func(t *testing.T) {
				image := format12Image(in)
				if p, err := loadBytes(encodeAtMinor(t, image, 2)); err != nil || p == nil {
					t.Fatalf("minor 2 Load=(%v,%v)", p, err)
				}
				p, err := loadBytes(encodeAtMinor(t, image, minor))
				var e *Error
				if p != nil || !errors.Is(err, ErrInvalid) || !errors.As(err, &e) {
					t.Fatalf("minor %d Load=(%v,%v), want ErrInvalid", minor, p, err)
				}
				if e.PC != format12Body || !strings.Contains(e.Detail, "format 1.2") || !strings.Contains(e.Detail, fmt.Sprintf("format 1.%d", minor)) {
					t.Fatalf("minor %d error = %+v, want the instruction and both formats", minor, e)
				}
			})
		}
	}
}

// format12Everything uses every format 1.2 opcode and Aux class in one
// valid function.
func format12Everything() Image {
	// 3 ParamDefault r0, 5; 4 Const r0; 5 FetchDim L4 = L2[r0];
	// 6 FetchDim L4 = L4[] (read-write, assign-op); 7 FetchDim L4 = L4[L2]
	// (unset); 8 ArraySet L2[r0] = r0 -> r3; 9 IterInit r3, r1, 15;
	// 10 IterNext r3, L2, 14; 11 IterNext r3, r0, 14; 12 IterKey L2, r3;
	// 13 IterKey r0, r3; 14 Drop r3; 15 Return r0.
	return format12Image(
		Instruction{Op: OpParamDefault, A: 0, B: 5},
		Instruction{Op: OpConst, A: 0, B: 0},
		Instruction{Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 2, C: 0},
		Instruction{Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchAppend | FetchForAssignOp), A: 4, B: 4},
		Instruction{Op: OpFetchDim, Aux: uint16(FetchUnset | FetchKeyLocation), A: 4, B: 4, C: 2},
		Instruction{Op: OpArraySet, Aux: arraySetAuxRaw(0, 3), A: 2, B: 0, C: 0},
		Instruction{Op: OpIterInit, A: 3, B: 1, C: 15},
		Instruction{Op: OpIterNext, Aux: uint16(IterLocation), A: 3, B: 2, C: 14},
		Instruction{Op: OpIterNext, A: 3, B: 0, C: 14},
		Instruction{Op: OpIterKey, Aux: uint16(IterLocation), A: 2, B: 3},
		Instruction{Op: OpIterKey, A: 0, B: 3},
		Instruction{Op: OpDrop, A: 3},
		Instruction{Op: OpReturn, A: 0},
	)
}

func TestFormat12EncodeLoadRoundTrip(t *testing.T) {
	image := format12Everything()
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

func TestFormat12Safety(t *testing.T) {
	for op := OpFetchDim; op <= OpParamDefault; op++ {
		s, ok := op.Safety()
		want := Safety{Steps: 1, CheckCancel: true, MayAllocate: true, MayThrow: true, MayCallUser: true}
		if op == OpParamDefault {
			// A pure branch on the frame's argument count.
			want = Safety{Steps: 1, CheckCancel: true}
		}
		if !ok || s != want {
			t.Errorf("Safety(%d) = %+v, %v; want %+v", op, s, ok, want)
		}
	}
	if _, ok := (OpMakeRef + 1).Safety(); ok {
		t.Errorf("opcode %d reported known", OpMakeRef+1)
	}
	if OpFetchDim != 37 || OpIterInit != 38 || OpIterNext != 39 || OpIterKey != 40 || OpParamDefault != 41 {
		t.Errorf("format 1.2 opcodes are renumbered: %d %d %d %d %d", OpFetchDim, OpIterInit, OpIterNext, OpIterKey, OpParamDefault)
	}
	if OpArrayAdd != 36 {
		t.Errorf("OpArrayAdd = %d, want 36: earlier opcodes may not be renumbered", OpArrayAdd)
	}
}
