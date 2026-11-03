package bytecode

import (
	"context"
	"strings"
)

func build(ctx context.Context, image Image, opts Options) (*Program, error) {
	b, err := newBudget(ctx, opts.Limits)
	if err != nil {
		return nil, err
	}
	if opts.Supported&^AllCapabilities != 0 {
		return nil, invalid("unknown supported capability")
	}
	b.supported = opts.Supported
	if _, err = verifyImage(b, image, true); err != nil {
		return nil, err
	}
	size, err := encodedSize(b, image)
	if err != nil {
		return nil, err
	}
	if err = b.bytes(size); err != nil {
		return nil, err
	}
	if err = b.poll(); err != nil {
		return nil, err
	}
	owned, err := cloneImage(b, image)
	if err != nil {
		return nil, err
	}
	if err = b.poll(); err != nil {
		return nil, err
	}
	return &Program{image: owned, limits: opts.Limits}, nil
}

func cloneImage(b *budget, src Image) (Image, error) {
	dst := src
	var err error
	if dst.Identity.Profile.Release, err = cloneString(b, src.Identity.Profile.Release); err != nil {
		return Image{}, err
	}
	if dst.Identity.GuestPlatform, err = cloneString(b, src.Identity.GuestPlatform); err != nil {
		return Image{}, err
	}
	if dst.Identity.Target, err = cloneString(b, src.Identity.Target); err != nil {
		return Image{}, err
	}
	if err = b.chargeWork(uint64(len(src.Constants))); err != nil {
		return Image{}, err
	}
	dst.Constants = make([]Constant, len(src.Constants))
	for i, c := range src.Constants {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		dst.Constants[i] = c
		if dst.Constants[i].Bytes, err = cloneString(b, c.Bytes); err != nil {
			return Image{}, err
		}
	}
	if err = b.chargeWork(uint64(len(src.Sources))); err != nil {
		return Image{}, err
	}
	dst.Sources = make([]Source, len(src.Sources))
	for i, s := range src.Sources {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		dst.Sources[i] = s
		if dst.Sources[i].Name, err = cloneString(b, s.Name); err != nil {
			return Image{}, err
		}
	}
	if err = b.chargeWork(uint64(len(src.Functions))); err != nil {
		return Image{}, err
	}
	dst.Functions = make([]Function, len(src.Functions))
	for i, f := range src.Functions {
		if err = b.poll(); err != nil {
			return Image{}, err
		}
		dst.Functions[i] = f
		if dst.Functions[i].Name, err = cloneString(b, f.Name); err != nil {
			return Image{}, err
		}
		var ok bool
		if dst.Functions[i].Registers, ok = cloneKinds(b, f.Registers); !ok {
			return Image{}, b.pollOrLimit()
		}
		if dst.Functions[i].Code, ok = cloneInstructions(b, f.Code); !ok {
			return Image{}, b.pollOrLimit()
		}
		if dst.Functions[i].Positions, ok = clonePositions(b, f.Positions); !ok {
			return Image{}, b.pollOrLimit()
		}
		if dst.Functions[i].Handlers, ok = cloneHandlers(b, f.Handlers); !ok {
			return Image{}, b.pollOrLimit()
		}
	}
	return dst, nil
}

func cloneString(b *budget, s string) (string, error) {
	if err := b.chargeWork(uint64(len(s))); err != nil {
		return "", err
	}
	var out strings.Builder
	out.Grow(len(s))
	for off := 0; off < len(s); {
		if err := b.poll(); err != nil {
			return "", err
		}
		end := chunkEnd(off, len(s), 32*1024)
		_, _ = out.WriteString(s[off:end])
		off = end
	}
	return out.String(), nil
}

func cloneKinds(b *budget, src []RegisterKind) ([]RegisterKind, bool) {
	if b.chargeWork(uint64(len(src))) != nil {
		return nil, false
	}
	dst := make([]RegisterKind, len(src))
	return dst, copyChunks(b, dst, src)
}
func cloneInstructions(b *budget, src []Instruction) ([]Instruction, bool) {
	if b.chargeWork(uint64(len(src))) != nil {
		return nil, false
	}
	dst := make([]Instruction, len(src))
	return dst, copyChunks(b, dst, src)
}
func clonePositions(b *budget, src []Position) ([]Position, bool) {
	if b.chargeWork(uint64(len(src))) != nil {
		return nil, false
	}
	dst := make([]Position, len(src))
	return dst, copyChunks(b, dst, src)
}
func cloneHandlers(b *budget, src []Handler) ([]Handler, bool) {
	if b.chargeWork(uint64(len(src))) != nil {
		return nil, false
	}
	dst := make([]Handler, len(src))
	return dst, copyChunks(b, dst, src)
}
func copyChunks[T any](b *budget, dst, src []T) bool {
	const chunk = 4096
	for off := 0; off < len(src); {
		if b.poll() != nil {
			return false
		}
		end := chunkEnd(off, len(src), chunk)
		copy(dst[off:end], src[off:end])
		off = end
	}
	return true
}
func (b *budget) pollOrLimit() error {
	if err := b.poll(); err != nil {
		return err
	}
	return ErrLimit
}

func encodedSize(b *budget, image Image) (uint64, error) {
	size := uint64(64 + 2 + 4 + 12)
	add := func(n uint64) bool {
		if n > ^uint64(0)-size {
			return false
		}
		size += n
		return true
	}
	str := func(s string) bool { return uint64(len(s)) <= uint64(^uint32(0)) && add(4+uint64(len(s))) }
	if !str(image.Identity.Profile.Release) || !add(4+1) || !str(image.Identity.GuestPlatform) || !str(image.Identity.Target) {
		return 0, ErrLimit
	}
	// Six 32-byte identity digests follow the strings.
	if !add(6 * 32) {
		return 0, ErrLimit
	}
	if uint64(len(image.Constants)) > uint64(^uint32(0)) || uint64(len(image.Sources)) > uint64(^uint32(0)) || uint64(len(image.Functions)) > uint64(^uint32(0)) {
		return 0, ErrLimit
	}
	for _, c := range image.Constants {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		if !str(c.Bytes) || !add(9) {
			return 0, ErrLimit
		}
	}
	for _, s := range image.Sources {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		if !str(s.Name) || !add(40) {
			return 0, ErrLimit
		}
	}
	for _, f := range image.Functions {
		if err := b.chargeWork(1); err != nil {
			return 0, err
		}
		if !str(f.Name) || !add(13) {
			return 0, ErrLimit
		}
		if uint64(len(f.Registers)) > uint64(^uint32(0)) || uint64(len(f.Code)) > uint64(^uint32(0)) || uint64(len(f.Positions)) > uint64(^uint32(0)) || uint64(len(f.Handlers)) > uint64(^uint32(0)) {
			return 0, ErrLimit
		}
		if !add(4 + uint64(len(f.Registers)) + 4 + uint64(len(f.Code))*16 + 4 + uint64(len(f.Positions))*16 + 4 + uint64(len(f.Handlers))*28) {
			return 0, ErrLimit
		}
	}
	return size, nil
}
