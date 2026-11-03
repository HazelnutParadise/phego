package bytecode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"strings"
)

var wireMagic = [8]byte{'P', 'H', 'E', 'G', 'O', 'B', 'C', 0}

const wireHeader = 64

// Payload order is: profile release string, guest integer width u32, short
// open tag u8, guest platform string, target string, then six 32-byte digests
// in Source/Compiler/Modules/RuntimeABI/Policy/Scope order; required u16 and
// entry u32; constants, sources, and functions as counted sequential records.
// A constant is kind u8, bits u64, and bytes string. A source is name string,
// digest[32], byte length u32, and line count u32. A function is name string,
// params/locals/max-stack u32, return-kind u8, register count plus u8 kinds,
// instruction count plus fixed 16-byte instructions, position count plus
// 16-byte positions, then handler count plus fixed 28-byte handlers. Strings
// and table counts use u32 byte lengths/counts. All integers are little-endian.

func load(ctx context.Context, data []byte, opts LoadOptions) (*Program, error) {
	b, err := newBudget(ctx, opts.Limits)
	if err != nil {
		return nil, err
	}
	b.supported = opts.Supported
	if opts.Supported&^AllCapabilities != 0 {
		return nil, invalid("unknown supported capability")
	}
	if err = b.bytes(uint64(len(data))); err != nil {
		return nil, err
	}
	if len(data) < wireHeader {
		return nil, invalidAt(len(data), "truncated header")
	}
	if !bytes.Equal(data[:8], wireMagic[:]) {
		return nil, invalidAt(0, "bad magic")
	}
	major, minor := binary.LittleEndian.Uint16(data[8:10]), binary.LittleEndian.Uint16(data[10:12])
	if major != FormatVersion || minor != 0 {
		return nil, ErrVersion
	}
	if binary.LittleEndian.Uint32(data[12:16]) != wireHeader {
		return nil, invalidAt(12, "invalid header length")
	}
	if binary.LittleEndian.Uint64(data[16:24]) != uint64(len(data)) {
		return nil, invalidAt(16, "invalid total length")
	}
	if binary.LittleEndian.Uint64(data[56:64]) != 0 {
		return nil, ErrVersion
	}
	if opts.TrustedDigest == ([32]byte{}) {
		return nil, ErrTrust
	}
	if err = b.chargeWork(uint64(len(data))); err != nil {
		return nil, err
	}
	internal, err := digestWire(ctx, data)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(internal[:], data[24:56]) {
		return nil, ErrIntegrity
	}
	if err = b.chargeWork(uint64(len(data))); err != nil {
		return nil, err
	}
	trusted, err := digestAll(ctx, data)
	if err != nil {
		return nil, err
	}
	if trusted != opts.TrustedDigest {
		return nil, ErrTrust
	}
	if err = b.reserve(512); err != nil {
		return nil, err
	}
	image, err := decodePayload(ctx, b, data[wireHeader:])
	if err != nil {
		return nil, err
	}
	if err = validateIdentity(opts.Expected); err != nil || image.Identity != opts.Expected {
		return nil, ErrProfile
	}
	if _, err = verifyImage(b, image, false); err != nil {
		return nil, err
	}
	if err = b.poll(); err != nil {
		return nil, err
	}
	return &Program{image: image, limits: opts.Limits}, nil
}

func encode(ctx context.Context, p *Program) ([]byte, error) {
	if p == nil || len(p.image.Functions) == 0 {
		return nil, ErrInvalid
	}
	b, err := newBudget(ctx, p.limits)
	if err != nil {
		return nil, err
	}
	// Program has already passed verification and exposes no mutable storage.
	size, err := encodedSize(b, p.image)
	if err != nil {
		return nil, err
	}
	if err = b.bytes(size); err != nil {
		return nil, err
	}
	if err = b.reserve(size + 128); err != nil {
		return nil, err
	}
	if err = b.chargeWork(size); err != nil {
		return nil, err
	}
	if size > uint64(maxInt()) {
		return nil, ErrLimit
	}
	out := make([]byte, int(size))
	copy(out[:8], wireMagic[:])
	binary.LittleEndian.PutUint16(out[8:10], FormatVersion)
	binary.LittleEndian.PutUint16(out[10:12], 0)
	binary.LittleEndian.PutUint32(out[12:16], wireHeader)
	binary.LittleEndian.PutUint64(out[16:24], size)
	binary.LittleEndian.PutUint64(out[56:64], 0)
	w := writer{b: b, data: out, pos: wireHeader}
	if err = w.payload(p.image); err != nil {
		return nil, err
	}
	if w.pos != len(out) {
		return nil, invalid("internal encoder size mismatch")
	}
	if err = b.chargeWork(size); err != nil {
		return nil, err
	}
	digest, err := digestWire(ctx, out)
	if err != nil {
		return nil, err
	}
	copy(out[24:56], digest[:])
	if err = b.poll(); err != nil {
		return nil, err
	}
	return out, nil
}

type writer struct {
	b    *budget
	data []byte
	pos  int
}

func (w *writer) put(p []byte) error {
	if w.pos < 0 || w.pos > len(w.data) || len(p) > len(w.data)-w.pos {
		return invalid("internal encoder size mismatch")
	}
	for off := 0; off < len(p); {
		if err := w.b.poll(); err != nil {
			return err
		}
		n := len(p) - off
		if n > 32*1024 {
			n = 32 * 1024
		}
		copy(w.data[w.pos:w.pos+n], p[off:off+n])
		w.pos += n
		off += n
	}
	return nil
}
func (w *writer) u8(v uint8) error { x := [1]byte{v}; return w.put(x[:]) }
func (w *writer) u16(v uint16) error {
	var x [2]byte
	binary.LittleEndian.PutUint16(x[:], v)
	return w.put(x[:])
}
func (w *writer) u32(v uint32) error {
	var x [4]byte
	binary.LittleEndian.PutUint32(x[:], v)
	return w.put(x[:])
}
func (w *writer) u64(v uint64) error {
	var x [8]byte
	binary.LittleEndian.PutUint64(x[:], v)
	return w.put(x[:])
}
func (w *writer) str(s string) error {
	if err := w.u32(uint32(len(s))); err != nil {
		return err
	}
	if len(s) > len(w.data)-w.pos {
		return invalid("internal encoder size mismatch")
	}
	for off := 0; off < len(s); {
		if err := w.b.poll(); err != nil {
			return err
		}
		end := chunkEnd(off, len(s), 32*1024)
		copy(w.data[w.pos:w.pos+end-off], s[off:end])
		w.pos += end - off
		off = end
	}
	return nil
}
func (w *writer) payload(i Image) error {
	if err := w.str(i.Identity.Profile.Release); err != nil {
		return err
	}
	if err := w.u32(uint32(i.Identity.Profile.IntWidth)); err != nil {
		return err
	}
	if err := w.u8(boolByte(i.Identity.Profile.ShortOpenTag)); err != nil {
		return err
	}
	if err := w.str(i.Identity.GuestPlatform); err != nil {
		return err
	}
	if err := w.str(i.Identity.Target); err != nil {
		return err
	}
	for _, d := range [][32]byte{i.Identity.Source, i.Identity.Compiler, i.Identity.Modules, i.Identity.RuntimeABI, i.Identity.Policy, i.Identity.Scope} {
		if err := w.put(d[:]); err != nil {
			return err
		}
	}
	if err := w.u16(uint16(i.Required)); err != nil {
		return err
	}
	if err := w.u32(i.Entry); err != nil {
		return err
	}
	if err := w.u32(uint32(len(i.Constants))); err != nil {
		return err
	}
	for _, c := range i.Constants {
		if err := w.u8(uint8(c.Kind)); err != nil {
			return err
		}
		if err := w.u64(c.Bits); err != nil {
			return err
		}
		if err := w.str(c.Bytes); err != nil {
			return err
		}
	}
	if err := w.u32(uint32(len(i.Sources))); err != nil {
		return err
	}
	for _, s := range i.Sources {
		if err := w.str(s.Name); err != nil {
			return err
		}
		if err := w.put(s.Digest[:]); err != nil {
			return err
		}
		if err := w.u32(s.Bytes); err != nil {
			return err
		}
		if err := w.u32(s.Lines); err != nil {
			return err
		}
	}
	if err := w.u32(uint32(len(i.Functions))); err != nil {
		return err
	}
	for _, f := range i.Functions {
		if err := w.str(f.Name); err != nil {
			return err
		}
		if err := w.u32(f.Params); err != nil {
			return err
		}
		if err := w.u32(f.Locals); err != nil {
			return err
		}
		if err := w.u32(f.MaxStack); err != nil {
			return err
		}
		if err := w.u8(uint8(f.ReturnKind)); err != nil {
			return err
		}
		if err := w.u32(uint32(len(f.Registers))); err != nil {
			return err
		}
		for _, k := range f.Registers {
			if err := w.u8(uint8(k)); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(f.Code))); err != nil {
			return err
		}
		for _, in := range f.Code {
			if err := w.u16(uint16(in.Op)); err != nil {
				return err
			}
			if err := w.u16(in.Aux); err != nil {
				return err
			}
			if err := w.u32(in.A); err != nil {
				return err
			}
			if err := w.u32(in.B); err != nil {
				return err
			}
			if err := w.u32(in.C); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(f.Positions))); err != nil {
			return err
		}
		for _, p := range f.Positions {
			if err := w.u32(p.Source); err != nil {
				return err
			}
			if err := w.u32(p.Start); err != nil {
				return err
			}
			if err := w.u32(p.End); err != nil {
				return err
			}
			if err := w.u32(p.Line); err != nil {
				return err
			}
		}
		if err := w.u32(uint32(len(f.Handlers))); err != nil {
			return err
		}
		for _, h := range f.Handlers {
			for _, v := range []uint32{h.Start, h.End, h.Target, h.BodyEnd, h.StackDepth, h.Register, uint32(h.Kind)} {
				if err := w.u32(v); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func boolByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func digestAll(ctx context.Context, data []byte) ([32]byte, error) {
	var out [32]byte
	h := sha256.New()
	if err := hashChunks(ctx, h, data); err != nil {
		return out, err
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}
func digestWire(ctx context.Context, data []byte) ([32]byte, error) {
	var out [32]byte
	if len(data) < wireHeader {
		return out, invalid("truncated header")
	}
	h := sha256.New()
	if err := hashChunks(ctx, h, data[:24]); err != nil {
		return out, err
	}
	if err := hashChunks(ctx, h, data[56:]); err != nil {
		return out, err
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}
func hashChunks(ctx context.Context, h hash.Hash, data []byte) error {
	for off := 0; off < len(data); {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := len(data) - off
		if n > 32*1024 {
			n = 32 * 1024
		}
		_, _ = h.Write(data[off : off+n])
		off += n
	}
	return nil
}

type reader struct {
	b    *budget
	data []byte
	pos  int
}

func (r *reader) invalid(detail string) error {
	return invalidAt(wireHeader+r.pos, detail)
}

func (r *reader) take(n uint64) ([]byte, error) {
	if n > uint64(len(r.data)-r.pos) {
		return nil, r.invalid("truncated payload")
	}
	if err := r.b.chargeWork(n); err != nil {
		return nil, err
	}
	start := r.pos
	r.pos += int(n)
	return r.data[start:r.pos], nil
}
func (r *reader) u8() (uint8, error) {
	p, e := r.take(1)
	if e != nil {
		return 0, e
	}
	return p[0], nil
}
func (r *reader) u16() (uint16, error) {
	p, e := r.take(2)
	if e != nil {
		return 0, e
	}
	return binary.LittleEndian.Uint16(p), nil
}
func (r *reader) u32() (uint32, error) {
	p, e := r.take(4)
	if e != nil {
		return 0, e
	}
	return binary.LittleEndian.Uint32(p), nil
}
func (r *reader) u64() (uint64, error) {
	p, e := r.take(8)
	if e != nil {
		return 0, e
	}
	return binary.LittleEndian.Uint64(p), nil
}
func (r *reader) str() (string, error) {
	n, e := r.u32()
	if e != nil {
		return "", e
	}
	if uint64(n) > uint64(len(r.data)-r.pos) {
		return "", r.invalid("string length exceeds remaining payload")
	}
	if e = r.b.reserve(uint64(n) + 24); e != nil {
		return "", e
	}
	p, e := r.take(uint64(n))
	if e != nil {
		return "", e
	}
	var out strings.Builder
	out.Grow(len(p))
	for off := 0; off < len(p); {
		if e = r.b.poll(); e != nil {
			return "", e
		}
		end := chunkEnd(off, len(p), 32*1024)
		_, _ = out.Write(p[off:end])
		off = end
	}
	return out.String(), nil
}
func (r *reader) count(min uint64, max uint32) (uint32, error) {
	n, e := r.u32()
	if e != nil {
		return 0, e
	}
	if n > max {
		return 0, ErrLimit
	}
	if min != 0 && uint64(n) > uint64(len(r.data)-r.pos)/min {
		return 0, r.invalid("record count exceeds remaining payload")
	}
	return n, nil
}

func decodePayload(ctx context.Context, b *budget, data []byte) (Image, error) {
	r := reader{b: b, data: data}
	var i Image
	var err error
	if i.Identity.Profile.Release, err = r.str(); err != nil {
		return Image{}, err
	}
	iw, err := r.u32()
	if err != nil {
		return Image{}, err
	}
	if uint64(iw) > uint64(maxInt()) {
		return Image{}, r.invalid("integer width overflow")
	}
	i.Identity.Profile.IntWidth = int(iw)
	short, err := r.u8()
	if err != nil {
		return Image{}, err
	}
	if short > 1 {
		return Image{}, r.invalid("invalid profile boolean")
	}
	i.Identity.Profile.ShortOpenTag = short == 1
	if i.Identity.GuestPlatform, err = r.str(); err != nil {
		return Image{}, err
	}
	if i.Identity.Target, err = r.str(); err != nil {
		return Image{}, err
	}
	digests := []*[32]byte{&i.Identity.Source, &i.Identity.Compiler, &i.Identity.Modules, &i.Identity.RuntimeABI, &i.Identity.Policy, &i.Identity.Scope}
	for _, d := range digests {
		p, e := r.take(32)
		if e != nil {
			return Image{}, e
		}
		copy(d[:], p)
	}
	if i.Required, err = readRequired(&r); err != nil {
		return Image{}, err
	}
	if i.Entry, err = r.u32(); err != nil {
		return Image{}, err
	}
	n, err := r.count(13, b.lim.MaxConstants)
	if err != nil {
		return Image{}, err
	}
	if e := b.reserveMul(uint64(n), 32); e != nil {
		return Image{}, e
	}
	i.Constants = make([]Constant, int(n))
	for x := range i.Constants {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		k, e := r.u8()
		if e != nil {
			return Image{}, e
		}
		v, e := r.u64()
		if e != nil {
			return Image{}, e
		}
		s, e := r.str()
		if e != nil {
			return Image{}, e
		}
		i.Constants[x] = Constant{Kind: ConstantKind(k), Bits: v, Bytes: s}
	}
	n, err = r.count(44, b.lim.MaxSources)
	if err != nil {
		return Image{}, err
	}
	if e := b.reserveMul(uint64(n), 80); e != nil {
		return Image{}, e
	}
	i.Sources = make([]Source, int(n))
	for x := range i.Sources {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		name, e := r.str()
		if e != nil {
			return Image{}, e
		}
		p, e := r.take(32)
		if e != nil {
			return Image{}, e
		}
		copy(i.Sources[x].Digest[:], p)
		sz, e := r.u32()
		if e != nil {
			return Image{}, e
		}
		lines, e := r.u32()
		if e != nil {
			return Image{}, e
		}
		i.Sources[x] = Source{Name: name, Digest: i.Sources[x].Digest, Bytes: sz, Lines: lines}
	}
	n, err = r.count(66, b.lim.MaxFunctions)
	if err != nil {
		return Image{}, err
	}
	if e := b.reserveMul(uint64(n), 160); e != nil {
		return Image{}, e
	}
	i.Functions = make([]Function, int(n))
	var totalIns, totalHandlers uint64
	for x := range i.Functions {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		f := &i.Functions[x]
		if f.Name, err = r.str(); err != nil {
			return Image{}, err
		}
		if f.Params, err = r.u32(); err != nil {
			return Image{}, err
		}
		if f.Locals, err = r.u32(); err != nil {
			return Image{}, err
		}
		if f.MaxStack, err = r.u32(); err != nil {
			return Image{}, err
		}
		if f.Locals > b.lim.MaxLocals || f.MaxStack > b.lim.MaxStack {
			return Image{}, ErrLimit
		}
		rk, e := r.u8()
		if e != nil {
			return Image{}, e
		}
		f.ReturnKind = RegisterKind(rk)
		nr, e := r.count(1, b.lim.MaxRegisters)
		if e != nil {
			return Image{}, e
		}
		if nr == 0 {
			return Image{}, r.invalid("function has no registers")
		}
		if e = b.reserveMul(uint64(nr), 8); e != nil {
			return Image{}, e
		}
		f.Registers = make([]RegisterKind, int(nr))
		for j := range f.Registers {
			k, ee := r.u8()
			if ee != nil {
				return Image{}, ee
			}
			f.Registers[j] = RegisterKind(k)
		}
		ni, e := r.count(16, b.lim.MaxInstructions)
		if e != nil {
			return Image{}, e
		}
		if ni == 0 {
			return Image{}, r.invalid("function has no instructions")
		}
		totalIns += uint64(ni)
		if totalIns > uint64(b.lim.MaxInstructions) {
			return Image{}, ErrLimit
		}
		if e = b.reserveMul(uint64(ni), 32); e != nil {
			return Image{}, e
		}
		f.Code = make([]Instruction, int(ni))
		for j := range f.Code {
			p, ee := r.take(16)
			if ee != nil {
				return Image{}, ee
			}
			f.Code[j] = Instruction{Op: Opcode(binary.LittleEndian.Uint16(p)), Aux: binary.LittleEndian.Uint16(p[2:]), A: binary.LittleEndian.Uint32(p[4:]), B: binary.LittleEndian.Uint32(p[8:]), C: binary.LittleEndian.Uint32(p[12:])}
		}
		np, e := r.count(16, b.lim.MaxInstructions)
		if e != nil {
			return Image{}, e
		}
		if np != ni {
			return Image{}, r.invalid("position count differs from instruction count")
		}
		if e = b.reserveMul(uint64(np), 24); e != nil {
			return Image{}, e
		}
		f.Positions = make([]Position, int(np))
		for j := range f.Positions {
			p, ee := r.take(16)
			if ee != nil {
				return Image{}, ee
			}
			f.Positions[j] = Position{Source: binary.LittleEndian.Uint32(p), Start: binary.LittleEndian.Uint32(p[4:]), End: binary.LittleEndian.Uint32(p[8:]), Line: binary.LittleEndian.Uint32(p[12:])}
		}
		nh, e := r.count(28, b.lim.MaxHandlers)
		if e != nil {
			return Image{}, e
		}
		totalHandlers += uint64(nh)
		if totalHandlers > uint64(b.lim.MaxHandlers) {
			return Image{}, ErrLimit
		}
		if e = b.reserveMul(uint64(nh), 40); e != nil {
			return Image{}, e
		}
		f.Handlers = make([]Handler, int(nh))
		for j := range f.Handlers {
			p, ee := r.take(28)
			if ee != nil {
				return Image{}, ee
			}
			f.Handlers[j] = Handler{Start: binary.LittleEndian.Uint32(p), End: binary.LittleEndian.Uint32(p[4:]), Target: binary.LittleEndian.Uint32(p[8:]), BodyEnd: binary.LittleEndian.Uint32(p[12:]), StackDepth: binary.LittleEndian.Uint32(p[16:]), Register: binary.LittleEndian.Uint32(p[20:]), Kind: HandlerKind(binary.LittleEndian.Uint32(p[24:]))}
		}
	}
	if r.pos != len(data) {
		return Image{}, r.invalid("trailing payload bytes")
	}
	_ = ctx
	return i, nil
}
func readRequired(r *reader) (CapabilitySet, error) { v, e := r.u16(); return CapabilitySet(v), e }
