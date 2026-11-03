package bytecode

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestRejectsMalformedConstantsIdentityAndUnreachableCode(t *testing.T) {
	for name, change := range map[string]func(*Image){
		"null bits":                  func(i *Image) { i.Constants[0].Bits = 1 },
		"null bytes":                 func(i *Image) { i.Constants[0].Bytes = "x" },
		"bool bits":                  func(i *Image) { i.Constants[0] = Constant{Kind: Bool, Bits: 2} },
		"integer bytes":              func(i *Image) { i.Constants[0] = Constant{Kind: Int, Bytes: "x"} },
		"float bytes":                func(i *Image) { i.Constants[0] = Constant{Kind: Float, Bytes: "x"} },
		"string bits":                func(i *Image) { i.Constants[0] = Constant{Kind: String, Bits: 1} },
		"unknown constant":           func(i *Image) { i.Constants[0].Kind = 255 },
		"bad entry":                  func(i *Image) { i.Entry = 99 },
		"no functions":               func(i *Image) { i.Functions = nil },
		"no instructions":            func(i *Image) { i.Functions[0].Code = nil; i.Functions[0].Positions = nil },
		"unknown register kind":      func(i *Image) { i.Functions[0].Registers[0] = 99 },
		"wrong return class":         func(i *Image) { i.Functions[0].ReturnKind = Location },
		"position count":             func(i *Image) { i.Functions[0].Positions = nil },
		"synthetic position garbage": func(i *Image) { i.Functions[0].Positions[0].Line = 1 },
		"falls off end":              func(i *Image) { i.Functions[0].Code[1] = Instruction{Op: OpCheckpoint} },
		"unreachable unknown opcode": func(i *Image) {
			i.Functions[0].Code = append(i.Functions[0].Code, Instruction{Op: 65535})
			i.Functions[0].Positions = append(i.Functions[0].Positions, Position{})
		},
		"unreachable bad constant": func(i *Image) {
			i.Functions[0].Code = append(i.Functions[0].Code, Instruction{Op: OpConst, B: 99}, Instruction{Op: OpReturn})
			i.Functions[0].Positions = append(i.Functions[0].Positions, Position{}, Position{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			i := validImage()
			change(&i)
			requireBuildError(t, i, ErrInvalid)
		})
	}
	for name, change := range map[string]func(*Identity){
		"empty release":   func(i *Identity) { i.Profile.Release = "" },
		"unknown release": func(i *Identity) { i.Profile.Release = "8.5.12-unverified" },
		"empty guest":     func(i *Identity) { i.GuestPlatform = "" },
		"empty target":    func(i *Identity) { i.Target = "" },
		"source digest":   func(i *Identity) { i.Source = [32]byte{} },
		"compiler digest": func(i *Identity) { i.Compiler = [32]byte{} },
		"modules digest":  func(i *Identity) { i.Modules = [32]byte{} },
		"ABI digest":      func(i *Identity) { i.RuntimeABI = [32]byte{} },
		"policy digest":   func(i *Identity) { i.Policy = [32]byte{} },
		"scope digest":    func(i *Identity) { i.Scope = [32]byte{} },
	} {
		t.Run(name, func(t *testing.T) { i := validImage(); change(&i.Identity); requireBuildError(t, i, ErrProfile) })
	}
}

func TestOpcodeCapabilitiesCannotBeUnderdeclared(t *testing.T) {
	for _, tc := range []struct {
		op         Opcode
		capability CapabilitySet
	}{
		{OpHostCall, HostCallbacks}, {OpInclude, FileRead}, {OpEval, DynamicCode}, {OpShell, Shell},
	} {
		i := validImage()
		i.Constants = []Constant{{Kind: String, Bytes: "test-only"}}
		i.Functions[0].Registers = []RegisterKind{Value, Value}
		i.Functions[0].Code = []Instruction{{Op: OpConst}, {Op: tc.op, A: 1, B: 0}, {Op: OpReturn, A: 1}}
		i.Functions[0].Positions = make([]Position, 3)
		i.Required = tc.capability
		p := requireValidBuild(t, i)
		if p.Required()&tc.capability == 0 {
			t.Fatal("lost required capability")
		}
		i.Required = 0
		requireBuildError(t, i, ErrCapability)
	}
}

func TestCancellationNilContextAndZeroProgram(t *testing.T) {
	p := requireValidBuild(t, validImage())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := Build(ctx, validImage(), testOptions()); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Build cancellation: %v %v", got, err)
	}
	if got, err := p.Encode(ctx); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Encode cancellation: %v %v", got, err)
	}
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	opts := LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(data)}
	if got, err := Load(ctx, data, opts); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Load cancellation: %v %v", got, err)
	}
	if got, err := Build(nil, validImage(), testOptions()); got != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil Build context: %v %v", got, err)
	}
	if got, err := Load(nil, data, opts); got != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil Load context: %v %v", got, err)
	}
	if got, err := p.Encode(nil); got != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil Encode context: %v %v", got, err)
	}
	for _, empty := range []*Program{nil, {}} {
		if got, err := empty.Encode(context.Background()); got != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty Program encode: %v %v", got, err)
		}
	}
}

func TestNestedArgumentsAndStackJoin(t *testing.T) {
	i := validImage()
	i.Functions = []Function{
		{Name: "main", MaxStack: 2, ReturnKind: Value, Registers: []RegisterKind{Value, Value},
			Code: []Instruction{{Op: OpConst}, {Op: OpPush}, {Op: OpPush}, {Op: OpCall, A: 1, B: 1, C: 1}, {Op: OpCall, A: 1, B: 1, C: 1}, {Op: OpReturn, A: 1}}, Positions: make([]Position, 6)},
		{Name: "callee", Params: 1, ReturnKind: Value, Registers: []RegisterKind{Value}, Code: []Instruction{{Op: OpReturn}}, Positions: []Position{{}}},
	}
	requireValidBuild(t, i)
	// One branch leaves a pending argument while the other does not.
	i = validImage()
	i.Functions[0].MaxStack = 1
	i.Functions[0].Code = []Instruction{{Op: OpConst}, {Op: OpBranch, B: 2, C: 3}, {Op: OpPush}, {Op: OpReturn}}
	i.Functions[0].Positions = make([]Position, 4)
	requireBuildError(t, i, ErrInvalid)
}

func TestCapabilityProgramsRoundTrip(t *testing.T) {
	for _, cap := range []CapabilitySet{HostCallbacks, DynamicCode, FileRead, Shell, AllCapabilities} {
		i := validImage()
		i.Required = cap
		p := requireValidBuild(t, i)
		data, err := p.Encode(context.Background())
		if err != nil {
			t.Fatalf("capability %d: %v", cap, err)
		}
		loaded, err := Load(context.Background(), data, LoadOptions{Options: testOptions(), Expected: i.Identity, TrustedDigest: sha256.Sum256(data)})
		if err != nil || loaded.Required() != cap {
			t.Fatalf("roundtrip capability %d: %v", cap, err)
		}
	}
}

func TestVerifierAcceptsBoundedValidationOfInfiniteLoop(t *testing.T) {
	i := validImage()
	i.Functions[0].Code = []Instruction{{Op: OpJump, A: 0}}
	i.Functions[0].Positions = []Position{{}}
	requireValidBuild(t, i) // Runtime must stop the loop through per-step quota/cancellation.
}

func TestFlowMemoryTracksReachableStates(t *testing.T) {
	i := validImage()
	f := &i.Functions[0]
	f.MaxStack = 4096
	f.Code = make([]Instruction, 200)
	f.Positions = make([]Position, 200)
	f.Code[0] = Instruction{Op: OpConst}
	for n := 1; n < 199; n++ {
		f.Code[n] = Instruction{Op: OpCheckpoint}
	}
	f.Code[199] = Instruction{Op: OpReturn}
	opts := testOptions()
	opts.Limits.MaxMemory = 256 << 10
	if p, err := Build(context.Background(), i, opts); err != nil || p == nil {
		t.Fatalf("empty reachable stacks should fit 256 KiB: %v", err)
	}
}

func TestFunctionEntryCannotBeAHandlerBody(t *testing.T) {
	i := validImage()
	i.Functions[0].Code = append(i.Functions[0].Code, Instruction{Op: OpThrow})
	i.Functions[0].Positions = append(i.Functions[0].Positions, Position{})
	i.Functions[0].Handlers = []Handler{{Start: 2, End: 3, Target: 0, BodyEnd: 2, Kind: Catch, Register: 0}}
	requireBuildError(t, i, ErrInvalid)
}

func TestOutputDuringArgumentPreparation(t *testing.T) {
	i := validImage()
	i.Functions[0].MaxStack = 1
	i.Functions[0].Code = []Instruction{{Op: OpConst}, {Op: OpPush}, {Op: OpOutput}, {Op: OpCall, B: 1, C: 1}, {Op: OpReturn}}
	i.Functions[0].Positions = make([]Position, 5)
	i.Functions = append(i.Functions, Function{Name: "callee", Params: 1, ReturnKind: Value, Registers: []RegisterKind{Value}, Code: []Instruction{{Op: OpReturn}}, Positions: []Position{{}}})
	requireValidBuild(t, i)
}

func nestedCatchInFinallyImage() Image {
	i := validImage()
	f := &i.Functions[0]
	f.Registers = []RegisterKind{Value, Value}
	f.Code = []Instruction{{Op: OpConst}, {Op: OpLeave, A: 8}, {Op: OpCheckpoint}, {Op: OpConst}, {Op: OpThrow}, {Op: OpOutput, A: 1}, {Op: OpLeave, A: 7}, {Op: OpEndFinally}, {Op: OpConst}, {Op: OpReturn}}
	f.Positions = make([]Position, len(f.Code))
	f.Handlers = []Handler{{Start: 0, End: 2, Target: 2, BodyEnd: 8, Kind: Finally}, {Start: 3, End: 5, Target: 5, BodyEnd: 7, Kind: Catch, Register: 1}}
	return i
}
func TestNestedCatchInsideFinallyRoundTrip(t *testing.T) {
	i := nestedCatchInFinallyImage()
	p := requireValidBuild(t, i)
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), data, LoadOptions{Options: testOptions(), Expected: i.Identity, TrustedDigest: sha256.Sum256(data)}); err != nil {
		t.Fatal(err)
	}
}
func TestEncodingRetainsExplicitVerificationLimits(t *testing.T) {
	i := validImage()
	i.Constants = []Constant{{Kind: String, Bytes: strings.Repeat("x", (32<<20)+1)}}
	opts := testOptions()
	opts.Limits.MaxWork = 256 << 20
	opts.Limits.MaxMemory = 256 << 20
	p, err := Build(context.Background(), i, opts)
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatalf("Encode replaced explicit limits with defaults: %v", err)
	}
	loaded, err := Load(context.Background(), data, LoadOptions{Options: opts, Expected: i.Identity, TrustedDigest: sha256.Sum256(data)})
	if err != nil {
		t.Fatal(err)
	}
	again, err := loaded.Encode(context.Background())
	if err != nil || sha256.Sum256(data) != sha256.Sum256(again) {
		t.Fatalf("Load lost encoding limits: %v", err)
	}
}

func TestEndFinallyCannotTerminateAnInnerCatch(t *testing.T) {
	i := nestedCatchInFinallyImage()
	i.Functions[0].Code[6] = Instruction{Op: OpEndFinally}
	requireBuildError(t, i, ErrInvalid)
}
func TestParserDiagnosticIdentifiesPayloadOffset(t *testing.T) {
	p := requireValidBuild(t, validImage())
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[64:68], ^uint32(0))
	repairWireIntegrity(data)
	_, err = Load(context.Background(), data, LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(data)})
	var detail *Error
	if !errors.As(err, &detail) || detail.Offset != 68 {
		t.Fatalf("expected offset 68 after malformed length, got %v", err)
	}
}

func TestChunkEndCannotOverflowNearMaximumIndex(t *testing.T) {
	limit := maxInt()
	for _, chunk := range []int{4096, 32768} {
		off := limit - 819
		if got := chunkEnd(off, limit, chunk); got != limit {
			t.Fatalf("chunk end overflowed: %d, want %d", got, limit)
		}
	}
}

func TestHeaderDiagnosticsIdentifyFixedFields(t *testing.T) {
	p := requireValidBuild(t, validImage())
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		offset int
		change func([]byte) []byte
	}{
		{"truncated", 7, func(b []byte) []byte { return b[:7] }},
		{"magic", 0, func(b []byte) []byte { b[0] = 0; return b }},
		{"header length", 12, func(b []byte) []byte { binary.LittleEndian.PutUint32(b[12:16], 63); return b }},
		{"total length", 16, func(b []byte) []byte { binary.LittleEndian.PutUint64(b[16:24], uint64(len(b)-1)); return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := tc.change(append([]byte(nil), data...))
			_, err := Load(context.Background(), bad, LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(bad)})
			var detail *Error
			if !errors.As(err, &detail) || detail.Offset != tc.offset {
				t.Fatalf("want offset %d, got %v", tc.offset, err)
			}
		})
	}
}
func TestEncodeEnforcesItsOwnOperationBudget(t *testing.T) {
	i := validImage()
	i.Constants = []Constant{{Kind: String, Bytes: strings.Repeat("x", 64<<10)}}
	opts := testOptions()
	opts.Limits.MaxWork = 70 << 10
	p, err := Build(context.Background(), i, opts)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := p.Encode(context.Background()); data != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("Encode must account both serialization and hashing: len=%d err=%v", len(data), err)
	}
}
func TestHandlerRegionsCannotCrossScopes(t *testing.T) {
	i := nestedCatchInFinallyImage()
	requireValidBuild(t, i)
	i.Functions[0].Handlers[1].Start = 0
	i.Functions[0].Handlers[1].End = 1
	requireBuildError(t, i, ErrInvalid)
}
