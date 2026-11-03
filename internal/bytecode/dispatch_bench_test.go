package bytecode

// These synthetic kernels compare Go dispatch shapes and encodings only. They
// are not PHP programs and make no claim about PHP semantics or application speed.
import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

type benchOp uint8

const (
	bAdd benchOp = iota + 1
	bXor
	bDec
	bBranchNZ
	bAppend
	bHalt
)

type benchInst struct {
	op, aux uint8
	a, b, c uint32
}
type benchProgram struct {
	name    string
	code    []benchInst
	initial [8]uint64
	data    string
	pairs   []bool // immutable preparation, indexed by original semantic PC
}
type benchState struct {
	regs              [8]uint64
	output            [8192]byte
	outN              int
	pc                int
	steps, dispatches uint64
}

var errBenchQuota = errors.New("synthetic step quota exceeded")
var dispatchSink uint64
var encodingSink byte

func (s *benchState) checkpoint(ctx context.Context, quota uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.steps >= quota {
		return errBenchQuota
	}
	s.steps++
	return nil
}

func doBenchOp(s *benchState, in benchInst, data string) bool {
	switch benchOp(in.op) {
	case bAdd:
		s.regs[in.a] = s.regs[in.b] + s.regs[in.c]
	case bXor:
		s.regs[in.a] = s.regs[in.b] ^ s.regs[in.c]
	case bDec:
		s.regs[in.a]--
	case bBranchNZ:
		if s.regs[in.a] != 0 {
			s.pc = int(in.b)
			return true
		}
	case bAppend:
		n := int(in.b)
		copy(s.output[s.outN:s.outN+n], data[int(in.a):int(in.a)+n])
		s.outN += n
	case bHalt:
		return false
	}
	s.pc++
	return true
}

func runSwitch(ctx context.Context, s *benchState, p benchProgram, quota uint64) error {
	s.regs = p.initial
	for s.pc < len(p.code) {
		in := p.code[s.pc]
		if err := s.checkpoint(ctx, quota); err != nil {
			return err
		}
		s.dispatches++
		if !doBenchOp(s, in, p.data) {
			return nil
		}
	}
	return nil
}

type benchHandler func(*benchState, benchInst, string) bool

func hAdd(s *benchState, i benchInst, _ string) bool {
	s.regs[i.a] = s.regs[i.b] + s.regs[i.c]
	s.pc++
	return true
}
func hXor(s *benchState, i benchInst, _ string) bool {
	s.regs[i.a] = s.regs[i.b] ^ s.regs[i.c]
	s.pc++
	return true
}
func hDec(s *benchState, i benchInst, _ string) bool { s.regs[i.a]--; s.pc++; return true }
func hBranch(s *benchState, i benchInst, _ string) bool {
	if s.regs[i.a] != 0 {
		s.pc = int(i.b)
	} else {
		s.pc++
	}
	return true
}
func hAppend(s *benchState, i benchInst, d string) bool {
	n := int(i.b)
	copy(s.output[s.outN:s.outN+n], d[int(i.a):int(i.a)+n])
	s.outN += n
	s.pc++
	return true
}
func hHalt(_ *benchState, _ benchInst, _ string) bool { return false }

var benchHandlers = [...]benchHandler{nil, hAdd, hXor, hDec, hBranch, hAppend, hHalt}

func runTable(ctx context.Context, s *benchState, p benchProgram, quota uint64) error {
	s.regs = p.initial
	for s.pc < len(p.code) {
		in := p.code[s.pc]
		if err := s.checkpoint(ctx, quota); err != nil {
			return err
		}
		s.dispatches++
		if !benchHandlers[in.op](s, in, p.data) {
			return nil
		}
	}
	return nil
}

type decodedOp func(*benchState) bool

func predecode(p benchProgram) []decodedOp {
	ops := make([]decodedOp, len(p.code))
	for pc, in := range p.code {
		a, b, c, data := in.a, in.b, in.c, p.data
		switch benchOp(in.op) {
		case bAdd:
			ops[pc] = func(s *benchState) bool { s.regs[a] = s.regs[b] + s.regs[c]; s.pc++; return true }
		case bXor:
			ops[pc] = func(s *benchState) bool { s.regs[a] = s.regs[b] ^ s.regs[c]; s.pc++; return true }
		case bDec:
			ops[pc] = func(s *benchState) bool { s.regs[a]--; s.pc++; return true }
		case bBranchNZ:
			ops[pc] = func(s *benchState) bool {
				if s.regs[a] != 0 {
					s.pc = int(b)
				} else {
					s.pc++
				}
				return true
			}
		case bAppend:
			ops[pc] = func(s *benchState) bool {
				n := int(b)
				copy(s.output[s.outN:s.outN+n], data[int(a):int(a)+n])
				s.outN += n
				s.pc++
				return true
			}
		case bHalt:
			ops[pc] = func(_ *benchState) bool { return false }
		}
	}
	return ops
}

func runClosures(ctx context.Context, s *benchState, p benchProgram, quota uint64, ops []decodedOp) error {
	s.regs = p.initial
	for s.pc < len(ops) {
		if err := s.checkpoint(ctx, quota); err != nil {
			return err
		}
		s.dispatches++
		if !ops[s.pc](s) {
			return nil
		}
	}
	return nil
}

// Each trampoline handler returns its continuation PC. The dispatcher invokes
// it from an explicit loop, never through recursive Go calls.
type bounce func(*benchState, benchInst, string) int

var bounces = [...]bounce{nil,
	func(s *benchState, i benchInst, _ string) int {
		s.regs[i.a] = s.regs[i.b] + s.regs[i.c]
		return s.pc + 1
	},
	func(s *benchState, i benchInst, _ string) int {
		s.regs[i.a] = s.regs[i.b] ^ s.regs[i.c]
		return s.pc + 1
	},
	func(s *benchState, i benchInst, _ string) int { s.regs[i.a]--; return s.pc + 1 },
	func(s *benchState, i benchInst, _ string) int {
		if s.regs[i.a] != 0 {
			return int(i.b)
		}
		return s.pc + 1
	},
	func(s *benchState, i benchInst, d string) int {
		n := int(i.b)
		copy(s.output[s.outN:s.outN+n], d[int(i.a):int(i.a)+n])
		s.outN += n
		return s.pc + 1
	},
	func(_ *benchState, _ benchInst, _ string) int { return -1 },
}

func runTrampoline(ctx context.Context, s *benchState, p benchProgram, quota uint64) error {
	s.regs = p.initial
	for s.pc < len(p.code) {
		if err := s.checkpoint(ctx, quota); err != nil {
			return err
		}
		s.dispatches++
		i := p.code[s.pc]
		next := bounces[i.op](s, i, p.data)
		if next < 0 {
			return nil
		}
		s.pc = next
	}
	return nil
}
func preparePairs(p benchProgram) []bool {
	pairs := make([]bool, len(p.code))
	for pc := 0; pc+1 < len(p.code); pc++ {
		pairs[pc] = benchOp(p.code[pc].op) == bAdd && benchOp(p.code[pc+1].op) == bAdd
	}
	return pairs
}

func runSuper(ctx context.Context, s *benchState, p benchProgram, quota uint64) error {
	s.regs = p.initial
	for s.pc < len(p.code) {
		if p.pairs[s.pc] {
			a, b := p.code[s.pc], p.code[s.pc+1]
			if err := s.checkpoint(ctx, quota); err != nil {
				return err
			}
			s.dispatches++
			s.regs[a.a] = s.regs[a.b] + s.regs[a.c]
			s.pc++ // the next checkpoint belongs to the second semantic PC
			if err := s.checkpoint(ctx, quota); err != nil {
				return err
			}
			s.regs[b.a] = s.regs[b.b] + s.regs[b.c]
			s.pc++
			continue
		}
		in := p.code[s.pc]
		if err := s.checkpoint(ctx, quota); err != nil {
			return err
		}
		s.dispatches++
		if !doBenchOp(s, in, p.data) {
			return nil
		}
	}
	return nil
}

type runFunc func(context.Context, *benchState, benchProgram, uint64) error
type candidate struct {
	name     string
	run      runFunc
	closures bool
	fused    bool
}

var candidates = []candidate{
	{name: "switch", run: runSwitch}, {name: "function-table", run: runTable},
	{name: "predecoded-closures", closures: true}, {name: "bounded-trampoline", run: runTrampoline},
	{name: "superinstruction-add-pair", run: runSuper, fused: true},
}

func runCandidate(c candidate, ctx context.Context, s *benchState, p benchProgram, quota uint64, ops []decodedOp) error {
	if c.closures {
		return runClosures(ctx, s, p, quota, ops)
	}
	return c.run(ctx, s, p, quota)
}

func workloads() []benchProgram {
	arith := benchProgram{name: "arithmetic", initial: [8]uint64{1, 3, 7}}
	for i := 0; i < 64; i++ {
		arith.code = append(arith.code, benchInst{op: uint8(bAdd), a: 0, b: 0, c: 1}, benchInst{op: uint8(bAdd), a: 1, b: 1, c: 2}, benchInst{op: uint8(bXor), a: 2, b: 2, c: 0})
	}
	arith.code = append(arith.code, benchInst{op: uint8(bHalt)})
	branch := benchProgram{name: "branch-heavy", initial: [8]uint64{127}}
	branch.code = []benchInst{{op: uint8(bDec), a: 0}, {op: uint8(bBranchNZ), a: 0, b: 0}, {op: uint8(bHalt)}}
	mixed := benchProgram{name: "binary-mixed", initial: [8]uint64{0x1234, 0x55, 0x100}, data: "\x00\xffPhego\x80\x01"}
	for i := 0; i < 32; i++ {
		mixed.code = append(mixed.code, benchInst{op: uint8(bAdd), a: 0, b: 0, c: 1}, benchInst{op: uint8(bAppend), a: 0, b: 8}, benchInst{op: uint8(bXor), a: 2, b: 2, c: 0})
	}
	mixed.code = append(mixed.code, benchInst{op: uint8(bHalt)})
	// Held out from dispatch training: alternating branch checks and arithmetic.
	held := benchProgram{name: "heldout-alternating", initial: [8]uint64{27, 1, 0, 7}}
	held.code = []benchInst{
		{op: uint8(bDec), a: 0},
		{op: uint8(bXor), a: 2, b: 2, c: 1},
		{op: uint8(bBranchNZ), a: 2, b: 4},
		{op: uint8(bAdd), a: 3, b: 3, c: 1},
		{op: uint8(bBranchNZ), a: 0, b: 0},
		{op: uint8(bHalt)},
	}
	all := []benchProgram{arith, branch, mixed, held}
	for i := range all {
		all[i].pairs = preparePairs(all[i])
	}
	return all
}

// oracle is intentionally a plain instruction loop used only by correctness checks.
func oracle(ctx context.Context, p benchProgram, quota uint64) (benchState, error) {
	var s benchState
	s.regs = p.initial
	for pc := 0; pc < len(p.code); {
		s.pc = pc
		if err := s.checkpoint(ctx, quota); err != nil {
			return s, err
		}
		in := p.code[pc]
		old := pc
		if !doBenchOp(&s, in, p.data) {
			return s, nil
		}
		pc = s.pc
		if pc == old {
			panic("oracle failed to advance")
		}
	}
	return s, nil
}

type cancelContext struct {
	context.Context
	remaining int
}

func (c *cancelContext) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestDispatchCandidates(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, p := range workloads() {
		t.Run(p.name, func(t *testing.T) {
			want, werr := oracle(context.Background(), p, 100000)
			if werr != nil {
				t.Fatal(werr)
			}
			for _, c := range candidates {
				t.Run(c.name, func(t *testing.T) {
					var got benchState
					ops := predecode(p)
					err := runCandidate(c, context.Background(), &got, p, 100000, ops)
					if err != nil || got.pc != want.pc || got.regs != want.regs || got.steps != want.steps || string(got.output[:got.outN]) != string(want.output[:want.outN]) {
						t.Fatalf("result mismatch: err=%v steps=%d/%d regs=%v/%v output=%x/%x", err, got.steps, want.steps, got.regs, want.regs, got.output[:got.outN], want.output[:want.outN])
					}
					wantDispatches := want.steps
					if c.fused {
						wantDispatches -= uint64(adjacentAddPairs(p.code))
					}
					if got.dispatches != wantDispatches {
						t.Fatalf("dispatches=%d, want %d", got.dispatches, wantDispatches)
					}
					for _, tc := range []struct {
						name      string
						ctx       func() context.Context
						quota     uint64
						wantErr   error
						wantSteps uint64
					}{
						{"cancel-before", func() context.Context { return canceled }, 100, context.Canceled, 0},
						{"cancel-between-fused-steps", func() context.Context { return &cancelContext{Context: context.Background(), remaining: 1} }, 100, context.Canceled, 1},
						{"cancel-after-two", func() context.Context { return &cancelContext{Context: context.Background(), remaining: 2} }, 100, context.Canceled, 2},
						{"quota-between-fused-steps", func() context.Context { return context.Background() }, 1, errBenchQuota, 1},
						{"quota", func() context.Context { return context.Background() }, 3, errBenchQuota, 3},
					} {
						t.Run(tc.name, func(t *testing.T) {
							wantState, wantErr := oracle(tc.ctx(), p, tc.quota)
							if !errors.Is(wantErr, tc.wantErr) || wantState.steps != tc.wantSteps {
								t.Fatalf("oracle err=%v steps=%d, expected %v/%d", wantErr, wantState.steps, tc.wantErr, tc.wantSteps)
							}
							var got benchState
							err := runCandidate(c, tc.ctx(), &got, p, tc.quota, ops)
							if !errors.Is(err, tc.wantErr) || got.pc != wantState.pc || got.steps != wantState.steps || got.regs != wantState.regs || string(got.output[:got.outN]) != string(wantState.output[:wantState.outN]) {
								t.Fatalf("result err=%v steps=%d regs=%v output=%x; want err=%v steps=%d regs=%v output=%x", err, got.steps, got.regs, got.output[:got.outN], wantErr, wantState.steps, wantState.regs, wantState.output[:wantState.outN])
							}
						})
					}
				})
			}
		})
	}
}

func adjacentAddPairs(code []benchInst) int {
	n := 0
	for i := 0; i+1 < len(code); {
		if benchOp(code[i].op) == bAdd && benchOp(code[i+1].op) == bAdd {
			n++
			i += 2
		} else {
			i++
		}
	}
	return n
}

func BenchmarkDispatch(b *testing.B) {
	ctx := context.Background()
	for _, p := range workloads() {
		ops := predecode(p)
		for _, c := range candidates {
			b.Run(p.name+"/"+c.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(p.code)*16), "instruction-B")
				if c.fused {
					b.ReportMetric(float64(len(p.pairs)), "fusion-index-B")
				}
				var steps, dispatches uint64
				started := time.Now()
				var s benchState
				for i := 0; i < b.N; i++ {
					s.pc, s.outN = 0, 0
					s.steps, s.dispatches = 0, 0
					if err := runCandidate(c, ctx, &s, p, 1<<60, ops); err != nil {
						b.Fatal(err)
					}
					steps = s.steps
					dispatches = s.dispatches
					dispatchSink = s.regs[0] ^ uint64(s.outN) ^ s.steps
				}
				b.ReportMetric(float64(steps), "steps/op")
				b.ReportMetric(float64(dispatches), "dispatches/op")
				b.ReportMetric(float64(steps)*float64(b.N)/time.Since(started).Seconds(), "semantic-steps/s")
			})
		}
	}
}

func encodeFixed(code []benchInst) []byte {
	out := make([]byte, len(code)*16)
	for i, x := range code {
		o := out[i*16:]
		binary.LittleEndian.PutUint16(o, uint16(x.op))
		binary.LittleEndian.PutUint16(o[2:], uint16(x.aux))
		binary.LittleEndian.PutUint32(o[4:], x.a)
		binary.LittleEndian.PutUint32(o[8:], x.b)
		binary.LittleEndian.PutUint32(o[12:], x.c)
	}
	return out
}
func decodeFixed(data []byte, dst []benchInst) error {
	if len(data) != len(dst)*16 {
		return errors.New("bad fixed encoding length")
	}
	for i := range dst {
		o := data[i*16:]
		dst[i] = benchInst{uint8(binary.LittleEndian.Uint16(o)), uint8(binary.LittleEndian.Uint16(o[2:])), binary.LittleEndian.Uint32(o[4:]), binary.LittleEndian.Uint32(o[8:]), binary.LittleEndian.Uint32(o[12:])}
	}
	return nil
}
func encodeCompact(code []benchInst) ([]byte, error) {
	out := make([]byte, len(code)*8)
	for i, x := range code {
		if x.a > 65535 || x.b > 65535 || x.c > 65535 {
			return nil, errors.New("operand does not fit compact encoding")
		}
		o := out[i*8:]
		o[0] = x.op
		o[1] = x.aux
		binary.LittleEndian.PutUint16(o[2:], uint16(x.a))
		binary.LittleEndian.PutUint16(o[4:], uint16(x.b))
		binary.LittleEndian.PutUint16(o[6:], uint16(x.c))
	}
	return out, nil
}
func decodeCompact(data []byte, dst []benchInst) error {
	if len(data) != len(dst)*8 {
		return errors.New("bad compact encoding length")
	}
	for i := range dst {
		o := data[i*8:]
		dst[i] = benchInst{o[0], o[1], uint32(binary.LittleEndian.Uint16(o[2:])), uint32(binary.LittleEndian.Uint16(o[4:])), uint32(binary.LittleEndian.Uint16(o[6:]))}
	}
	return nil
}
func encodeVarint(code []benchInst) []byte {
	out := make([]byte, 0, len(code)*7)
	for _, x := range code {
		out = appendUvarint(out, uint64(x.op))
		out = appendUvarint(out, uint64(x.aux))
		out = appendUvarint(out, uint64(x.a))
		out = appendUvarint(out, uint64(x.b))
		out = appendUvarint(out, uint64(x.c))
	}
	return out
}
func appendUvarint(dst []byte, x uint64) []byte {
	for x >= 0x80 {
		dst = append(dst, byte(x)|0x80)
		x >>= 7
	}
	return append(dst, byte(x))
}
func readUvarint(data []byte, pos *int) (uint64, error) {
	var x uint64
	for shift := uint(0); shift < 70; shift += 7 {
		if *pos >= len(data) {
			return 0, errors.New("truncated varint")
		}
		b := data[*pos]
		*pos++
		if shift == 63 && b > 1 {
			return 0, errors.New("varint overflow")
		}
		x |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return x, nil
		}
	}
	return 0, errors.New("varint overflow")
}
func decodeVarint(data []byte, dst []benchInst) error {
	pos := 0
	for i := range dst {
		v := [5]uint64{}
		for j := range v {
			x, e := readUvarint(data, &pos)
			if e != nil {
				return e
			}
			v[j] = x
		}
		if v[0] > 255 || v[1] > 255 || v[2] > 1<<32-1 || v[3] > 1<<32-1 || v[4] > 1<<32-1 {
			return errors.New("field overflow")
		}
		dst[i] = benchInst{uint8(v[0]), uint8(v[1]), uint32(v[2]), uint32(v[3]), uint32(v[4])}
	}
	if pos != len(data) {
		return errors.New("trailing encoding bytes")
	}
	return nil
}

func TestSyntheticEncodings(t *testing.T) {
	for _, p := range workloads() {
		want := p.code
		fixed := encodeFixed(want)
		compact, e := encodeCompact(want)
		if e != nil {
			t.Fatal(e)
		}
		varint := encodeVarint(want)
		for name, encoding := range map[string]struct {
			data []byte
			fn   func([]byte, []benchInst) error
		}{"fixed16": {fixed, decodeFixed}, "compact8": {compact, decodeCompact}, "uvarint": {varint, decodeVarint}} {
			got := make([]benchInst, len(want))
			if e := encoding.fn(encoding.data, got); e != nil || !equalInstructions(got, want) {
				t.Fatalf("%s %s decode mismatch: %v", p.name, name, e)
			}
		}
	}
}
func equalInstructions(a, b []benchInst) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func BenchmarkEncoding(b *testing.B) {
	p := workloads()[2]
	for _, f := range []struct {
		name string
		enc  func([]benchInst) ([]byte, error)
		dec  func([]byte, []benchInst) error
	}{
		{"fixed16", func(x []benchInst) ([]byte, error) { return encodeFixed(x), nil }, decodeFixed},
		{"compact8", encodeCompact, decodeCompact},
		{"uvarint", func(x []benchInst) ([]byte, error) { return encodeVarint(x), nil }, decodeVarint},
	} {
		data, _ := f.enc(p.code)
		dst := make([]benchInst, len(p.code))
		b.Run(f.name+"/encode", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				v, e := f.enc(p.code)
				if e != nil {
					b.Fatal(e)
				}
				encodingSink = v[len(v)-1]
			}
			b.ReportMetric(float64(len(data)), "program-B")
		})
		b.Run(f.name+"/decode", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for i := 0; i < b.N; i++ {
				if e := f.dec(data, dst); e != nil {
					b.Fatal(e)
				}
				encodingSink = byte(dst[len(dst)-1].op)
			}
			b.ReportMetric(float64(len(data)), "program-B")
		})
	}
}

func BenchmarkPreparation(b *testing.B) {
	p := workloads()[2]
	b.Run("predecode-super-pairs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pairs := preparePairs(p)
			dispatchSink = uint64(len(pairs))
		}
	})
	b.Run("predecode-closures", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ops := predecode(p)
			dispatchSink = uint64(len(ops))
		}
	})
	b.Run("compact-decode", func(b *testing.B) {
		data, _ := encodeCompact(p.code)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dst := make([]benchInst, len(p.code))
			if err := decodeCompact(data, dst); err != nil {
				b.Fatal(err)
			}
			dispatchSink = uint64(len(dst))
		}
	})
}

// Fixed independent expectations keep a common operation bug in the oracle from
// making every candidate appear correct.
func TestFixedDispatchResults(t *testing.T) {
	p := benchProgram{initial: [8]uint64{1, 2, 3}, data: "\x00\xff", code: []benchInst{
		{op: uint8(bAdd), a: 0, b: 1, c: 2}, {op: uint8(bXor), a: 1, b: 0, c: 2},
		{op: uint8(bAppend), a: 0, b: 2}, {op: uint8(bHalt)},
	}}
	p.pairs = preparePairs(p)
	for _, c := range candidates {
		var s benchState
		if err := runCandidate(c, context.Background(), &s, p, 100, predecode(p)); err != nil {
			t.Fatal(err)
		}
		if s.regs != ([8]uint64{5, 6, 3}) || s.pc != 3 || s.steps != 4 || s.outN != 2 || s.output[0] != 0 || s.output[1] != 255 {
			t.Fatalf("%s incorrect fixed result: pc=%d regs=%v steps=%d", c.name, s.pc, s.regs, s.steps)
		}
	}
}
