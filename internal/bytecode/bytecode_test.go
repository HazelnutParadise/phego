package bytecode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/profile"
)

func testIdentity() Identity {
	return Identity{Profile: profile.Profile{Release: "8.5.12", IntWidth: 64}, GuestPlatform: "linux-amd64", Target: "portable-go",
		Source: sha256.Sum256([]byte("source")), Compiler: sha256.Sum256([]byte("compiler")), Modules: sha256.Sum256([]byte("modules")),
		RuntimeABI: sha256.Sum256([]byte("abi")), Policy: sha256.Sum256([]byte("policy")), Scope: sha256.Sum256([]byte("scope"))}
}

func testOptions() Options { return Options{Limits: DefaultLimits(), Supported: AllCapabilities} }

func validImage() Image {
	return Image{Identity: testIdentity(), Entry: 0, Constants: []Constant{{Kind: Null}}, Functions: []Function{{
		Name: "main", ReturnKind: Value, Registers: []RegisterKind{Value},
		Code: []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Positions: make([]Position, 2),
	}}}
}

// validImageWithHandler has a finally body that is isolated from normal flow.
// OpLeave exits the protected interval and clears register definitions, so
// the continuation deliberately defines r0 again before returning it.
func validImageWithHandler() Image {
	i := validImage()
	i.Functions[0].Code = []Instruction{
		{Op: OpConst, A: 0, B: 0},
		{Op: OpLeave, A: 4},
		{Op: OpCheckpoint},
		{Op: OpEndFinally},
		{Op: OpConst, A: 0, B: 0},
		{Op: OpReturn, A: 0},
	}
	i.Functions[0].Positions = make([]Position, len(i.Functions[0].Code))
	i.Functions[0].Handlers = []Handler{{Start: 0, End: 2, Target: 2, BodyEnd: 4, Kind: Finally}}
	return i
}

func validCallAndReferenceImage() Image {
	i := validImage()
	i.Functions = []Function{
		{Name: "main", MaxStack: 1, ReturnKind: Location, Registers: []RegisterKind{Location, Value, Value, Location},
			Code: []Instruction{{Op: OpLocal, A: 0, B: 0}, {Op: OpConst, A: 1, B: 0}, {Op: OpStore, A: 0, B: 1}, {Op: OpLoad, A: 2, B: 0}, {Op: OpPush, A: 2}, {Op: OpCall, A: 3, B: 1, C: 1}, {Op: OpReturnRef, A: 3}}, Positions: make([]Position, 7), Locals: 1},
		{Name: "callee", Params: 1, ReturnKind: Location, Registers: []RegisterKind{Value, Location}, Code: []Instruction{{Op: OpLocal, A: 1, B: 0}, {Op: OpReturnRef, A: 1}}, Positions: make([]Position, 2), Locals: 1},
	}
	return i
}

func validCatchImage() Image {
	i := validImage()
	i.Functions[0].Registers = []RegisterKind{Value, Value}
	i.Functions[0].Code = []Instruction{{Op: OpConst, A: 1, B: 0}, {Op: OpThrow, A: 1}, {Op: OpReturn, A: 0}}
	i.Functions[0].Positions = make([]Position, 3)
	i.Functions[0].Handlers = []Handler{{Start: 0, End: 2, Target: 2, BodyEnd: 3, Kind: Catch, Register: 0}}
	return i
}

func requireValidBuild(t *testing.T, image Image) *Program {
	t.Helper()
	p, err := Build(context.Background(), image, testOptions())
	if err != nil {
		t.Fatalf("valid control Build failed: %v", err)
	}
	if p == nil {
		t.Fatal("valid control Build returned nil Program")
	}
	return p
}

func requireBuildError(t *testing.T, image Image, kind error) {
	t.Helper()
	requireValidBuild(t, validImage())
	p, err := Build(context.Background(), image, testOptions())
	if err == nil {
		t.Fatal("Build accepted invalid image")
	}
	if p != nil {
		t.Fatalf("Build returned partial Program with error: %#v", p)
	}
	if !errors.Is(err, kind) {
		t.Fatalf("Build error = %v, want errors.Is(_, %v)", err, kind)
	}
}

func TestBuildEncodeLoadRoundTripAndInputOwnership(t *testing.T) {
	image := validImage()
	p := requireValidBuild(t, image)
	encoded, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 64 {
		t.Fatalf("encoding shorter than fixed header: %d", len(encoded))
	}
	if !bytes.Equal(encoded[:8], []byte{'P', 'H', 'E', 'G', 'O', 'B', 'C', 0}) {
		t.Fatalf("magic = %q", encoded[:8])
	}
	if binary.LittleEndian.Uint16(encoded[8:10]) != 1 || binary.LittleEndian.Uint16(encoded[10:12]) != FormatMinor {
		t.Fatal("unexpected format version")
	}
	if binary.LittleEndian.Uint32(encoded[12:16]) != 64 || binary.LittleEndian.Uint64(encoded[16:24]) != uint64(len(encoded)) {
		t.Fatal("invalid fixed header fields")
	}
	if got := sha256.Sum256(encoded); got == ([32]byte{}) {
		t.Fatal("impossible zero digest")
	}
	loadOpts := LoadOptions{Options: testOptions(), Expected: image.Identity, TrustedDigest: sha256.Sum256(encoded)}
	loaded, err := Load(context.Background(), encoded, loadOpts)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil Program")
	}
	if got := loaded.Identity(); got != image.Identity {
		t.Fatalf("identity mismatch: got %#v", got)
	}
	if loaded.Entry() != image.Entry || loaded.FunctionCount() != 1 {
		t.Fatal("round trip changed entry or function count")
	}
	got, ok := loaded.Constant(0)
	if !ok || got != image.Constants[0] {
		t.Fatalf("constant round trip = %#v, %v", got, ok)
	}
	view, ok := loaded.Function(0)
	if !ok || view.Len() != 2 {
		t.Fatal("function view missing after round trip")
	}
	for i, want := range image.Functions[0].Code {
		got, ok := view.Instruction(uint32(i))
		if !ok || got != want {
			t.Fatalf("instruction %d = %#v, %v; want %#v", i, got, ok, want)
		}
	}
	// Both Build and Load must own their inputs after returning.
	image.Constants[0].Kind = String
	image.Functions[0].Code[0].Op = OpDrop
	encoded[64] ^= 0xff
	if got, _ := p.Constant(0); got.Kind != Null {
		t.Fatal("Build retained mutable constant input")
	}
	if got, _ := view.Instruction(0); got.Op != OpConst {
		t.Fatal("Load retained mutable encoded input")
	}
}

func TestBuildRejectsMalformedSemanticsWithoutPartialProgram(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Image)
		want   error
	}{
		{"bad profile", func(i *Image) { i.Identity.Profile.IntWidth = 16 }, ErrProfile},
		{"bad constant index", func(i *Image) { i.Functions[0].Code[0].B = 4 }, ErrInvalid},
		{"unknown opcode", func(i *Image) { i.Functions[0].Code[0].Op = 0xffff }, ErrInvalid},
		{"unknown binary aux", func(i *Image) { i.Functions[0].Code[0] = Instruction{Op: OpBinary, Aux: 0xffff, A: 0, B: 0, C: 0} }, ErrInvalid},
		{"unused operand", func(i *Image) { i.Functions[0].Code[1].B = 1 }, ErrInvalid},
		{"register index", func(i *Image) { i.Functions[0].Code[1].A = 8 }, ErrInvalid},
		{"parameter count exceeds frame", func(i *Image) { i.Functions[0].Params = 2 }, ErrInvalid},
		{"value location mismatch", func(i *Image) { i.Functions[0].Registers[0] = Location }, ErrInvalid},
		{"invalid bool bits", func(i *Image) { i.Constants[0] = Constant{Kind: Bool, Bits: 2} }, ErrInvalid},
		{"invalid constant kind", func(i *Image) { i.Constants[0] = Constant{Kind: 0} }, ErrInvalid},
		{"undefined read", func(i *Image) {
			i.Functions[0].Code = []Instruction{{Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 1)
		}, ErrInvalid},
		{"jump out of bounds", func(i *Image) { i.Functions[0].Code[0] = Instruction{Op: OpJump, A: 99} }, ErrInvalid},
		{"source position out of bounds", func(i *Image) { i.Functions[0].Positions[0] = Position{Source: 1, Start: 1, End: 2, Line: 1} }, ErrInvalid},
		{"source line zero", func(i *Image) {
			i.Sources = []Source{{Name: "s", Digest: sha256.Sum256([]byte("s")), Bytes: 1, Lines: 1}}
			i.Functions[0].Positions[0] = Position{Source: 1, Start: 0, End: 1, Line: 0}
		}, ErrInvalid},
		{"parameter not value", func(i *Image) { i.Functions[0].Params = 1; i.Functions[0].Registers[0] = Location }, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { i := validImage(); tt.mutate(&i); requireBuildError(t, i, tt.want) })
	}
}

func TestBuildRejectsZeroIndependentLimits(t *testing.T) {
	for _, field := range []string{"MaxBytes", "MaxMemory", "MaxWork", "MaxFunctions", "MaxInstructions", "MaxConstants", "MaxRegisters", "MaxLocals", "MaxStack", "MaxHandlers", "MaxSources"} {
		t.Run(field, func(t *testing.T) {
			requireValidBuild(t, validImage())
			o := testOptions()
			switch field {
			case "MaxBytes":
				o.Limits.MaxBytes = 0
			case "MaxMemory":
				o.Limits.MaxMemory = 0
			case "MaxWork":
				o.Limits.MaxWork = 0
			case "MaxFunctions":
				o.Limits.MaxFunctions = 0
			case "MaxInstructions":
				o.Limits.MaxInstructions = 0
			case "MaxConstants":
				o.Limits.MaxConstants = 0
			case "MaxRegisters":
				o.Limits.MaxRegisters = 0
			case "MaxLocals":
				o.Limits.MaxLocals = 0
			case "MaxStack":
				o.Limits.MaxStack = 0
			case "MaxHandlers":
				o.Limits.MaxHandlers = 0
			case "MaxSources":
				o.Limits.MaxSources = 0
			}
			p, err := Build(context.Background(), validImage(), o)
			if err == nil || p != nil || !errors.Is(err, ErrInvalid) {
				t.Fatalf("Build = (%v,%v), want nil and ErrInvalid", p, err)
			}
		})
	}
}

func TestScalarConstantRoundTripPreservesBitsAndBinaryStrings(t *testing.T) {
	image := validImage()
	image.Constants = []Constant{{Kind: Null}, {Kind: Bool, Bits: 1}, {Kind: Int, Bits: ^uint64(0)}, {Kind: Float, Bits: math.Float64bits(math.Copysign(0, -1))}, {Kind: Float, Bits: 0x7ff8000000000042}, {Kind: String, Bytes: "\x00\xff"}}
	image.Functions[0].Registers = make([]RegisterKind, len(image.Constants))
	image.Functions[0].Code = nil
	for n := range image.Constants {
		image.Functions[0].Registers[n] = Value
		image.Functions[0].Code = append(image.Functions[0].Code, Instruction{Op: OpConst, A: uint32(n), B: uint32(n)})
	}
	image.Functions[0].Code = append(image.Functions[0].Code, Instruction{Op: OpReturn, A: 0})
	image.Functions[0].Positions = make([]Position, len(image.Functions[0].Code))
	p := requireValidBuild(t, image)
	for n, want := range image.Constants {
		got, ok := p.Constant(uint32(n))
		if !ok || got != want {
			t.Fatalf("constant %d = %#v, want %#v", n, got, want)
		}
	}
}

func TestIntegerConstantsRespectSignedGuestWidths(t *testing.T) {
	for _, tc := range []struct {
		name     string
		width    int
		min, max uint64
	}{
		{"32-bit", 32, ^uint64(2147483647), uint64(2147483647)},
		{"64-bit", 64, uint64(1) << 63, (uint64(1) << 63) - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := validImage()
			i.Identity.Profile.IntWidth = tc.width
			i.Constants = []Constant{{Kind: Int, Bits: tc.min}, {Kind: Int, Bits: tc.max}}
			i.Functions[0].Registers = []RegisterKind{Value, Value}
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpConst, A: 1, B: 1}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 3)
			p := requireValidBuild(t, i)
			for n, want := range i.Constants {
				got, ok := p.Constant(uint32(n))
				if !ok || got != want {
					t.Fatalf("integer constant %d=%#v, want %#v", n, got, want)
				}
			}
		})
	}
	i := validImage()
	i.Identity.Profile.IntWidth = 32
	i.Constants = []Constant{{Kind: Int, Bits: uint64(1) << 31}}
	i.Functions[0].Code[0].B = 0
	requireBuildError(t, i, ErrInvalid)
}

func TestIdentityFieldsAreComparedIndividually(t *testing.T) {
	mutations := map[string]func(*Identity){
		"release": func(i *Identity) { i.Profile.Release = "8.5.11" }, "width": func(i *Identity) { i.Profile.IntWidth = 32 }, "short tag": func(i *Identity) { i.Profile.ShortOpenTag = true },
		"guest platform": func(i *Identity) { i.GuestPlatform = "windows-amd64" }, "target": func(i *Identity) { i.Target = "native" },
		"source": func(i *Identity) { i.Source[0] ^= 1 }, "compiler": func(i *Identity) { i.Compiler[0] ^= 1 }, "modules": func(i *Identity) { i.Modules[0] ^= 1 },
		"runtime ABI": func(i *Identity) { i.RuntimeABI[0] ^= 1 }, "policy": func(i *Identity) { i.Policy[0] ^= 1 }, "scope": func(i *Identity) { i.Scope[0] ^= 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			p := requireValidBuild(t, validImage())
			data, err := p.Encode(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			expected := testIdentity()
			mutate(&expected)
			opts := LoadOptions{Options: testOptions(), Expected: expected, TrustedDigest: sha256.Sum256(data)}
			loaded, err := Load(context.Background(), data, opts)
			if err == nil || loaded != nil || !errors.Is(err, ErrProfile) {
				t.Fatalf("Load=(%v,%v), want nil and ErrProfile", loaded, err)
			}
		})
	}
}

func TestLoadRejectsHeaderAndTrustFailures(t *testing.T) {
	p := requireValidBuild(t, validImage())
	original, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	validOpts := func(b []byte) LoadOptions {
		return LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(b)}
	}
	cases := []struct {
		name string
		edit func([]byte)
		want error
	}{
		{"version", func(b []byte) { binary.LittleEndian.PutUint16(b[8:10], 2) }, ErrVersion},
		{"native features", func(b []byte) { binary.LittleEndian.PutUint64(b[56:64], 1) }, ErrVersion},
		{"truncated", func(b []byte) {}, ErrInvalid},
		{"trailing data", func(b []byte) {}, ErrInvalid},
		{"integrity", func(b []byte) { b[24] ^= 1 }, ErrIntegrity},
		{"trusted digest", func(b []byte) {}, ErrTrust},
		{"missing trusted digest", func(b []byte) {}, ErrTrust},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireValidBuild(t, validImage())
			b := append([]byte(nil), original...)
			opts := validOpts(b)
			switch tc.name {
			case "truncated":
				b = b[:len(b)-1]
				opts = validOpts(b)
			case "trailing data":
				b = append(b, 0)
				opts = validOpts(b)
			case "trusted digest":
				opts.TrustedDigest[0] ^= 1
			case "missing trusted digest":
				opts.TrustedDigest = [32]byte{}
			}
			tc.edit(b)
			// Header integrity covers all bytes after the digest, including features.
			if tc.name == "version" || tc.name == "native features" || tc.name == "truncated" || tc.name == "trailing data" {
				repairWireIntegrity(b)
				if tc.name != "trusted digest" {
					opts.TrustedDigest = sha256.Sum256(b)
				}
			}
			loaded, e := Load(context.Background(), b, opts)
			if e == nil || loaded != nil || !errors.Is(e, tc.want) {
				t.Fatalf("Load=(%v,%v), want nil and %v", loaded, e, tc.want)
			}
		})
	}
}

func repairWireIntegrity(b []byte) {
	if len(b) < 64 {
		return
	}
	binary.LittleEndian.PutUint64(b[16:24], uint64(len(b)))
	input := append(append([]byte(nil), b[:24]...), b[56:]...)
	d := sha256.Sum256(input)
	copy(b[24:56], d[:])
}

func TestSafetyMetadataIsIntrinsic(t *testing.T) {
	for op := OpCheckpoint; op <= OpMatchError; op++ {
		s, ok := op.Safety()
		if !ok || s.Steps < 1 || !s.CheckCancel {
			t.Errorf("opcode %d safety=%+v known=%v", op, s, ok)
		}
	}
	for _, op := range []Opcode{0, OpMatchError + 1, 0xffff} {
		if _, ok := op.Safety(); ok {
			t.Errorf("unknown opcode %d reported known", op)
		}
	}
}

func TestCancellationAndRequiredCapability(t *testing.T) {
	image := validImage()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, err := Build(ctx, image, testOptions())
	if !errors.Is(err, context.Canceled) || p != nil {
		t.Fatalf("cancelled Build=(%v,%v)", p, err)
	}
	control := requireValidBuild(t, validImage())
	encoded, err := control.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	partial, err := control.Encode(ctx)
	if !errors.Is(err, context.Canceled) || len(partial) != 0 {
		t.Fatalf("cancelled Encode=(%d bytes,%v)", len(partial), err)
	}
	loaded, err := Load(ctx, encoded, LoadOptions{Options: testOptions(), Expected: testIdentity(), TrustedDigest: sha256.Sum256(encoded)})
	if !errors.Is(err, context.Canceled) || loaded != nil {
		t.Fatalf("cancelled Load=(%v,%v)", loaded, err)
	}
	image.Required = HostCallbacks
	o := testOptions()
	o.Supported = 0
	p, err = Build(context.Background(), image, o)
	if err == nil || p != nil || !errors.Is(err, ErrCapability) {
		t.Fatalf("unsupported capability Build=(%v,%v)", p, err)
	}
}

func TestConcurrentImmutableReaders(t *testing.T) {
	p := requireValidBuild(t, validImage())
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 100; k++ {
				if _, ok := p.Function(0); !ok {
					t.Error("missing function")
				}
				if _, ok := p.Constant(0); !ok {
					t.Error("missing constant")
				}
				if _, e := p.Encode(context.Background()); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if len(data) < 64 {
		t.Fatal("unexpected short encoding")
	}
}

func TestBuildBoundsControlFlowAndFrameEdges(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Image)
	}{
		{"branch target", func(i *Image) { i.Functions[0].Code[0] = Instruction{Op: OpBranch, A: 0, B: 0, C: 99} }},
		{"bad local", func(i *Image) { i.Functions[0].Code[0] = Instruction{Op: OpLocal, A: 0, B: 9} }},
		{"bad source range", func(i *Image) {
			i.Sources = []Source{{Name: "s", Digest: sha256.Sum256([]byte("s")), Bytes: 1, Lines: 1}}
			i.Functions[0].Positions[0] = Position{Source: 1, Start: 0, End: 2, Line: 1}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { i := validImage(); tc.mutate(&i); requireBuildError(t, i, ErrInvalid) })
	}
}

func TestBuildRejectsRegisterClassesAndMustDefineFlow(t *testing.T) {
	cases := []struct {
		name  string
		image func() Image
	}{
		{"load from value", func() Image {
			i := validImage()
			i.Functions[0].Code = []Instruction{{Op: OpLoad, A: 0, B: 0}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 2)
			return i
		}},
		{"store into value", func() Image {
			i := validImage()
			i.Functions[0].Code = []Instruction{{Op: OpStore, A: 0, B: 0}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 2)
			return i
		}},
		{"move across classes", func() Image {
			i := validImage()
			i.Functions[0].Registers = []RegisterKind{Value, Location}
			i.Functions[0].Code = []Instruction{{Op: OpMove, A: 0, B: 1}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 2)
			return i
		}},
		{"read after drop", func() Image {
			i := validImage()
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpDrop, A: 0}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 3)
			return i
		}},
		{"one undefined diamond predecessor", func() Image {
			i := validImage()
			i.Functions[0].Registers = []RegisterKind{Value, Value}
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpBranch, A: 0, B: 2, C: 3}, {Op: OpConst, A: 1, B: 0}, {Op: OpJump, A: 4}, {Op: OpReturn, A: 1}}
			i.Functions[0].Positions = make([]Position, 5)
			return i
		}},
		{"undefined on backedge", func() Image {
			i := validImage()
			i.Functions[0].Registers = []RegisterKind{Value, Value}
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpBranch, A: 0, B: 1, C: 4}, {Op: OpMove, A: 1, B: 0}, {Op: OpJump, A: 1}, {Op: OpReturn, A: 1}}
			i.Functions[0].Positions = make([]Position, 5)
			return i
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { requireBuildError(t, tc.image(), ErrInvalid) })
	}
}

func TestBuildRejectsCallAndHandlerBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		base   func() Image
		mutate func(*Image)
	}{
		{"direct call wrong arity", validCallAndReferenceImage, func(i *Image) {
			i.Functions[1].Params = 2
			i.Functions[1].Registers = []RegisterKind{Value, Value, Location}
			i.Functions[1].Code = []Instruction{{Op: OpLocal, A: 2, B: 0}, {Op: OpReturnRef, A: 2}}
			i.Functions[1].Positions = make([]Position, 2)
		}},
		{"direct call argument is location", validCallAndReferenceImage, func(i *Image) {
			i.Functions[0].Code = []Instruction{{Op: OpLocal, A: 0, B: 0}, {Op: OpPush, A: 0}, {Op: OpCall, A: 3, B: 1, C: 1}, {Op: OpReturnRef, A: 3}}
			i.Functions[0].Positions = make([]Position, 4)
		}},
		{"call stack overflow", validCallAndReferenceImage, func(i *Image) { i.Functions[0].MaxStack = 0 }},
		{"return with pending argument", validImage, func(i *Image) {
			i.Functions[0].MaxStack = 1
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpPush, A: 0}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 3)
		}},
		{"handler nonzero stack depth", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].StackDepth = 1 }},
		{"handler target inside protected interval", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].Target = 1 }},
		{"catch register out of range", validCatchImage, func(i *Image) { i.Functions[0].Handlers[0].Register = 99 }},
		{"catch location register", validCatchImage, func(i *Image) { i.Functions[0].Registers[0] = Location }},
		{"finally has register", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].Register = 1 }},
		{"crossing handlers", validImageWithHandler, func(i *Image) {
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpLeave, A: 6}, {Op: OpCheckpoint}, {Op: OpEndFinally}, {Op: OpLeave, A: 6}, {Op: OpReturn, A: 0}, {Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 8)
			i.Functions[0].Handlers = append(i.Functions[0].Handlers, Handler{Start: 1, End: 3, Target: 5, BodyEnd: 6, Kind: Catch, Register: 0})
		}},
		{"handler endpoint beyond code", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].End = 99 }},
		{"body end missing", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].BodyEnd = 0 }},
		{"body end not after target", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].BodyEnd = 2 }},
		{"body end beyond code", validImageWithHandler, func(i *Image) { i.Functions[0].Handlers[0].BodyEnd = 99 }},
		{"branch into handler body middle", validImageWithHandler, func(i *Image) { i.Functions[0].Code[1] = Instruction{Op: OpJump, A: 3} }},
		{"normal fallthrough into handler body", validImageWithHandler, func(i *Image) { i.Functions[0].Code[1] = Instruction{Op: OpCheckpoint} }},
		{"normal fallthrough out of handler body", validImageWithHandler, func(i *Image) { i.Functions[0].Code[3] = Instruction{Op: OpCheckpoint} }},
		{"finally end outside body", validImageWithHandler, func(i *Image) { i.Functions[0].Code[4] = Instruction{Op: OpEndFinally} }},
		{"catch reads preexisting undefined register", validCatchImage, func(i *Image) {
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 1, B: 0}, {Op: OpThrow, A: 1}, {Op: OpOutput, A: 1}, {Op: OpReturn, A: 0}}
			i.Functions[0].Positions = make([]Position, 4)
			i.Functions[0].Handlers[0].BodyEnd = 4
		}},
		{"leave continuation reads cleared register", validImageWithHandler, func(i *Image) { i.Functions[0].Code[4] = Instruction{Op: OpReturn, A: 0} }},
		{"leave target beyond code", validImageWithHandler, func(i *Image) { i.Functions[0].Code[1] = Instruction{Op: OpLeave, A: 99} }},
		{"end finally operands", validImageWithHandler, func(i *Image) { i.Functions[0].Code[3] = Instruction{Op: OpEndFinally, A: 1} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			control := tc.base()
			requireValidBuild(t, control)
			i := tc.base()
			tc.mutate(&i)
			requireBuildError(t, i, ErrInvalid)
		})
	}
}

func TestBuildAcceptsValidCallsControlFlowAndHandlers(t *testing.T) {
	tests := []struct {
		name  string
		image Image
	}{
		{"value location store load push direct call and ref return", validCallAndReferenceImage()},
		{"finally body and leave continuation", validImageWithHandler()},
		{"catch entry defines only catch register", validCatchImage()},
		{"diamond defines value on both arms", func() Image {
			i := validImage()
			i.Functions[0].Registers = []RegisterKind{Value, Value}
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpBranch, A: 0, B: 2, C: 4}, {Op: OpConst, A: 1, B: 0}, {Op: OpJump, A: 5}, {Op: OpConst, A: 1, B: 0}, {Op: OpReturn, A: 1}}
			i.Functions[0].Positions = make([]Position, 6)
			return i
		}()},
		{"backedge preserves must defined values", func() Image {
			i := validImage()
			i.Functions[0].Registers = []RegisterKind{Value, Value}
			i.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpConst, A: 1, B: 0}, {Op: OpBranch, A: 0, B: 3, C: 4}, {Op: OpJump, A: 2}, {Op: OpReturn, A: 1}}
			i.Functions[0].Positions = make([]Position, 5)
			return i
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build(context.Background(), tc.image, testOptions())
			if err != nil || p == nil {
				t.Fatalf("valid image Build=(%v,%v)", p, err)
			}
		})
	}
}

func TestBuildAcceptsValidPureOpcodeOperands(t *testing.T) {
	build := func(registers []RegisterKind, locals uint32, code []Instruction, kind RegisterKind) Image {
		i := validImage()
		i.Functions[0].Registers = registers
		i.Functions[0].Locals = locals
		i.Functions[0].ReturnKind = kind
		i.Functions[0].Code = code
		i.Functions[0].Positions = make([]Position, len(code))
		return i
	}
	cases := []struct {
		name  string
		image Image
	}{
		{"checkpoint", build([]RegisterKind{Value}, 0, []Instruction{{Op: OpCheckpoint}, {Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Value)},
		{"move", build([]RegisterKind{Value, Value}, 0, []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpMove, A: 1, B: 0}, {Op: OpReturn, A: 1}}, Value)},
		{"local load and store", build([]RegisterKind{Value, Location}, 1, []Instruction{{Op: OpLocal, A: 1, B: 0}, {Op: OpConst, A: 0, B: 0}, {Op: OpStore, A: 1, B: 0}, {Op: OpLoad, A: 0, B: 1}, {Op: OpReturn, A: 0}}, Value)},
		{"output", build([]RegisterKind{Value}, 0, []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpOutput, A: 0}, {Op: OpReturn, A: 0}}, Value)},
		{"new array", build([]RegisterKind{Value, Value}, 0, []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpNewArray, A: 1}, {Op: OpReturn, A: 1}}, Value)},
		{"array get", build([]RegisterKind{Value, Value, Value, Value}, 0, []Instruction{{Op: OpNewArray, A: 1}, {Op: OpConst, A: 2, B: 0}, {Op: OpArrayGet, A: 3, B: 1, C: 2}, {Op: OpReturn, A: 3}}, Value)},
		{"call value", build([]RegisterKind{Value, Value}, 0, []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpCallValue, A: 1, B: 0, C: 0}, {Op: OpReturn, A: 1}}, Value)},
		{"drop then redefine", build([]RegisterKind{Value}, 0, []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpDrop, A: 0}, {Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}, Value)},
	}
	for _, op := range []BinaryOp{Add, Subtract, Multiply, Divide, Concat, Equal, Identical, Less, BitAnd, BitOr, BitXor} {
		code := []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpBinary, Aux: uint16(op), A: 1, B: 0, C: 0}, {Op: OpReturn, A: 1}}
		cases = append(cases, struct {
			name  string
			image Image
		}{name: "binary_" + binaryOpName(op), image: build([]RegisterKind{Value, Value}, 0, code, Value)})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build(context.Background(), tc.image, testOptions())
			if err != nil || p == nil {
				t.Fatalf("valid opcode image Build=(%v,%v)", p, err)
			}
		})
	}
}

func binaryOpName(op BinaryOp) string {
	switch op {
	case Add:
		return "add"
	case Subtract:
		return "subtract"
	case Multiply:
		return "multiply"
	case Divide:
		return "divide"
	case Concat:
		return "concat"
	case Equal:
		return "equal"
	case Identical:
		return "identical"
	case Less:
		return "less"
	case BitAnd:
		return "bitand"
	case BitOr:
		return "bitor"
	case BitXor:
		return "bitxor"
	}
	return "unknown"
}

func TestBuildRejectsResourceExhaustionAndCapabilityMetadata(t *testing.T) {
	for _, field := range []string{"bytes", "memory", "work", "functions", "instructions", "constants", "registers", "locals", "stack", "handlers", "sources"} {
		t.Run("exhausted_"+field, func(t *testing.T) {
			requireValidBuild(t, validImage())
			o := testOptions()
			image := validImage()
			switch field {
			case "bytes":
				o.Limits.MaxBytes = 1
			case "memory":
				o.Limits.MaxMemory = 1
			case "work":
				o.Limits.MaxWork = 1
			case "functions":
				o.Limits.MaxFunctions = 1
				second := image.Functions[0]
				second.Name = "secondary"
				image.Functions = append(image.Functions, second)
			case "instructions":
				o.Limits.MaxInstructions = 1
			case "constants":
				o.Limits.MaxConstants = 1
				image.Constants = append(image.Constants, Constant{Kind: Bool})
			case "registers":
				o.Limits.MaxRegisters = 1
				image.Functions[0].Registers = append(image.Functions[0].Registers, Value)
			case "locals":
				o.Limits.MaxLocals = 1
				image.Functions[0].Locals = 2
			case "stack":
				o.Limits.MaxStack = 1
				image.Functions[0].MaxStack = 2
			case "handlers":
				o.Limits.MaxHandlers = 1
				image = validImageWithHandler()
				image.Functions[0].Code = []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpLeave, A: 4}, {Op: OpCheckpoint}, {Op: OpEndFinally}, {Op: OpLeave, A: 6}, {Op: OpReturn, A: 0}, {Op: OpConst, A: 0, B: 0}, {Op: OpReturn, A: 0}}
				image.Functions[0].Positions = make([]Position, 8)
				image.Functions[0].Handlers = []Handler{{Start: 0, End: 2, Target: 2, BodyEnd: 4, Kind: Finally}, {Start: 4, End: 5, Target: 5, BodyEnd: 6, Kind: Catch, Register: 0}}
			case "sources":
				o.Limits.MaxSources = 1
				image.Sources = []Source{{Name: "one", Digest: sha256.Sum256([]byte("one")), Bytes: 1, Lines: 1}, {Name: "two", Digest: sha256.Sum256([]byte("two")), Bytes: 1, Lines: 1}}
			}
			requireValidBuild(t, image)
			p, err := Build(context.Background(), image, o)
			if err == nil || p != nil || !errors.Is(err, ErrLimit) {
				t.Fatalf("Build=(%v,%v), want nil and ErrLimit", p, err)
			}
		})
	}
	for _, tc := range []struct {
		name     string
		op       Opcode
		required CapabilitySet
	}{
		{"callback", OpHostCall, HostCallbacks}, {"include", OpInclude, FileRead}, {"eval", OpEval, DynamicCode}, {"shell", OpShell, Shell},
	} {
		t.Run("intrinsic_"+tc.name, func(t *testing.T) {
			op, ok := tc.op.Safety()
			if !ok || op.Capabilities&tc.required != tc.required {
				t.Fatalf("Safety(%v)=%+v, %v", tc.op, op, ok)
			}
		})
	}
}

func TestSourceNamesAndBytesAreOpaque(t *testing.T) {
	i := validImage()
	i.Sources = []Source{{Name: "\xff\x00name", Digest: sha256.Sum256([]byte{0xff}), Bytes: 1, Lines: 1}}
	i.Functions[0].Positions[0] = Position{Source: 1, Start: 0, End: 1, Line: 1}
	p := requireValidBuild(t, i)
	v, ok := p.Function(0)
	if !ok {
		t.Fatal("function missing")
	}
	pos, ok := v.Position(0)
	if !ok || pos.Source != 1 {
		t.Fatalf("source position=%+v, %v", pos, ok)
	}
}
