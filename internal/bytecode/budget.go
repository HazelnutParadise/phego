package bytecode

import (
	"context"
	"math"
)

type budget struct {
	ctx       context.Context
	lim       Limits
	supported CapabilitySet
	mem       uint64
	work      uint64
}

func newBudget(ctx context.Context, limits Limits) (*budget, error) {
	if ctx == nil || limits.MaxBytes == 0 || limits.MaxMemory == 0 || limits.MaxWork == 0 ||
		limits.MaxFunctions == 0 || limits.MaxInstructions == 0 || limits.MaxConstants == 0 ||
		limits.MaxRegisters == 0 || limits.MaxLocals == 0 || limits.MaxStack == 0 ||
		limits.MaxHandlers == 0 || limits.MaxSources == 0 ||
		uint64(limits.MaxBytes) > uint64(maxInt()) || uint64(limits.MaxMemory) > uint64(maxInt()) {
		return nil, ErrInvalid
	}
	for _, n := range []uint32{limits.MaxFunctions, limits.MaxInstructions, limits.MaxConstants, limits.MaxRegisters, limits.MaxLocals, limits.MaxStack, limits.MaxHandlers, limits.MaxSources} {
		if uint64(n) > uint64(maxInt()) {
			return nil, ErrInvalid
		}
	}
	b := &budget{ctx: ctx, lim: limits}
	if err := b.poll(); err != nil {
		return nil, err
	}
	return b, nil
}

func maxInt() int { return int(^uint(0) >> 1) }

func (b *budget) poll() error {
	if b == nil || b.ctx == nil {
		return ErrInvalid
	}
	return b.ctx.Err()
}

func (b *budget) chargeWork(n uint64) error {
	if err := b.poll(); err != nil {
		return err
	}
	if n > b.lim.MaxWork-b.work {
		return ErrLimit
	}
	b.work += n
	return nil
}

func (b *budget) reserve(n uint64) error {
	if err := b.poll(); err != nil {
		return err
	}
	if n > b.lim.MaxMemory-b.mem {
		return ErrLimit
	}
	b.mem += n
	return nil
}

func (b *budget) reserveMul(count, size uint64) error {
	if size != 0 && count > math.MaxUint64/size {
		return ErrLimit
	}
	return b.reserve(count * size)
}

func (b *budget) bytes(n uint64) error {
	if n > b.lim.MaxBytes {
		return ErrLimit
	}
	return nil
}

func fail(kind error, detail string) error {
	if kind == nil {
		kind = ErrInvalid
	}
	return &Error{Kind: kind, Offset: -1, Function: -1, PC: -1, Detail: detail}
}

func invalid(detail string) error { return fail(ErrInvalid, detail) }

// chunkEnd bounds a chunk without narrowing the index type.
func chunkEnd(off, length, chunk int) int {
	return off + min(chunk, length-off)
}

func invalidAt(offset int, detail string) error {
	return &Error{Kind: ErrInvalid, Offset: offset, Function: -1, PC: -1, Detail: detail}
}
