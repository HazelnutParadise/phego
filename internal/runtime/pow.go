// This file is a Go translation of math/pow.c from ARM optimized-routines at
// commit 2ee4b0d45680a9880412c1d186224b08652c35f6, the algorithm glibc uses
// for pow; it reproduces the results of glibc 2.41 on arm64.
//
// Copyright (c) 2018-2026, Arm Limited.
// SPDX-License-Identifier: MIT OR Apache-2.0 WITH LLVM-exception
// See third_party/optimized-routines/LICENSE.

package runtime

import "math"

// glibcPow returns x**y as glibc 2.41's pow does on arm64 Linux with fused
// multiply-add, so PHP-visible results match bit for bit. PHP's ** calls the
// C library pow, and Go's math.Pow can be several units in the last place
// away from it, which PHP code observes through printing or comparison.
//
// glibc on arm64 is compiled with GCC's default multiply-add contraction, so
// a product that feeds an addition is usually fused; which operations are
// fused was checked against the disassembly of Debian's glibc 2.41 libm. Go fuses only on some
// architectures and by its own rules, even across statements, so every such
// operation is written as math.FMA and every other product is wrapped in
// float64(), which the Go specification requires to round.
func glibcPow(x, y float64) float64 {
	var signBias uint64
	ix, iy := math.Float64bits(x), math.Float64bits(y)
	topx, topy := uint32(ix>>52), uint32(iy>>52)
	if topx-0x001 >= 0x7ff-0x001 || (topy&0x7ff)-0x3be >= 0x43e-0x3be {
		if zeroInfNaN(iy) {
			switch {
			case 2*iy == 0:
				return 1.0
			case ix == math.Float64bits(1.0):
				return 1.0
			case 2*ix > 2*math.Float64bits(math.Inf(1)) || 2*iy > 2*math.Float64bits(math.Inf(1)):
				return x + y
			case 2*ix == 2*math.Float64bits(1.0):
				return 1.0
			case (2*ix < 2*math.Float64bits(1.0)) == (iy>>63 == 0):
				return 0.0
			}
			return y * y
		}
		if zeroInfNaN(ix) {
			x2 := float64(x * x)
			if ix>>63 != 0 && checkInt(iy) == 1 {
				x2 = -x2
			}
			if iy>>63 != 0 {
				return 1 / x2
			}
			return x2
		}
		if ix>>63 != 0 {
			switch checkInt(iy) {
			case 0:
				return math.NaN()
			case 1:
				signBias = expSignBias
			}
			ix &= 0x7fffffffffffffff
			topx &= 0x7ff
		}
		if (topy&0x7ff)-0x3be >= 0x43e-0x3be {
			if ix == math.Float64bits(1.0) {
				return 1.0
			}
			if topy&0x7ff < 0x3be {
				if ix > math.Float64bits(1.0) {
					return 1.0 + y
				}
				return 1.0 - y
			}
			if (ix > math.Float64bits(1.0)) == (topy < 0x800) {
				return math.Inf(1)
			}
			return 0
		}
		if topx == 0 {
			// Normalize a subnormal x so its exponent becomes negative.
			ix = math.Float64bits(math.Float64frombits(ix) * 0x1p52)
			ix &= 0x7fffffffffffffff
			ix -= 52 << 52
		}
	}
	hi, lo := powLog(ix)
	ehi := float64(y * hi)
	elo := math.FMA(y, lo, math.FMA(y, hi, -ehi))
	return powExp(ehi, elo, signBias)
}

// checkInt returns 0 when the finite non-zero value with bits iy is not an
// integer, 1 when it is odd, and 2 when it is even.
func checkInt(iy uint64) int {
	e := int(iy >> 52 & 0x7ff)
	switch {
	case e < 0x3ff:
		return 0
	case e > 0x3ff+52:
		return 2
	case iy&(1<<(0x3ff+52-e)-1) != 0:
		return 0
	case iy&(1<<(0x3ff+52-e)) != 0:
		return 1
	}
	return 2
}

func zeroInfNaN(i uint64) bool { return 2*i-1 >= 2*math.Float64bits(math.Inf(1))-1 }

const (
	powLogOffset = 0x3fe6955500000000
	expSignBias  = 0x800 << 7
)

// powLog returns hi+lo = log(x) for the normalized bits ix, with lo carrying
// about 15 more bits of precision.
func powLog(ix uint64) (float64, float64) {
	tmp := ix - powLogOffset
	i := (tmp >> (52 - 7)) % 128
	k := int64(tmp) >> 52
	iz := ix - (tmp & (0xfff << 52))
	z := math.Float64frombits(iz)
	kd := float64(k)
	t := powLogTable[i]
	r := math.FMA(z, t.invc, -1.0)
	t1 := math.FMA(kd, powLn2hi, t.logc)
	t2 := t1 + r
	lo1 := math.FMA(kd, powLn2lo, t.logctail)
	lo2 := t1 - t2 + r
	A := &powLogPoly
	ar := float64(A[0] * r)
	ar2 := float64(r * ar)
	ar3 := float64(r * ar2)
	hi := t2 + ar2
	lo3 := math.FMA(ar, r, -ar2)
	lo4 := t2 - hi + ar2
	q := math.FMA(ar2, math.FMA(ar2, math.FMA(r, A[6], A[5]), math.FMA(r, A[4], A[3])), math.FMA(r, A[2], A[1]))
	lo := math.FMA(ar3, q, lo1+lo2+lo3+lo4) // lo1+lo2+lo3+lo4 + ar3*q
	y := hi + lo
	return y, hi - y + lo
}

// powExp returns sign*exp(x+xtail) where signBias selects the sign.
func powExp(x, xtail float64, signBias uint64) float64 {
	abstop := uint32(math.Float64bits(x)>>52) & 0x7ff
	if abstop-top12(0x1p-54) >= top12(512.0)-top12(0x1p-54) {
		if abstop-top12(0x1p-54) >= 0x80000000 {
			one := 1.0 + x
			if signBias != 0 {
				return -one
			}
			return one
		}
		if abstop >= top12(1024.0) {
			sign := 1.0
			if signBias != 0 {
				sign = -1.0
			}
			if math.Float64bits(x)>>63 != 0 {
				return sign * 0
			}
			return sign * math.Inf(1)
		}
		abstop = 0
	}
	z := float64(expInvLn2N * x)
	kd := math.Round(z) // glibc on arm64 rounds half away from zero here
	ki := uint64(int64(kd))
	r := math.FMA(kd, expNegLn2loN, math.FMA(kd, expNegLn2hiN, x))
	r += xtail
	idx := 2 * (ki % 128)
	top := (ki + signBias) << (52 - 7)
	tail := math.Float64frombits(expTable[idx])
	sbits := expTable[idx+1] + top
	r2 := float64(r * r)
	C := &expPoly
	tmp := math.FMA(float64(r2*r2), math.FMA(r, C[3], C[2]), math.FMA(r2, math.FMA(r, C[1], C[0]), tail+r))
	if abstop == 0 {
		return powExpSpecial(tmp, sbits, ki)
	}
	scale := math.Float64frombits(sbits)
	return math.FMA(scale, tmp, scale)
}

func top12(x float64) uint32 { return uint32(math.Float64bits(x) >> 52) }

// powExpSpecial computes scale*(1+tmp) when it may overflow or underflow,
// rounding once in the subnormal range.
func powExpSpecial(tmp float64, sbits, ki uint64) float64 {
	if ki&0x80000000 == 0 {
		sbits -= 1009 << 52
		scale := math.Float64frombits(sbits)
		y := math.FMA(scale, tmp, scale)
		return 0x1p1009 * y
	}
	// glibc's arm64 build does not fuse this branch: scale*tmp is rounded
	// once and shared, as its disassembly shows.
	sbits += 1022 << 52
	scale := math.Float64frombits(sbits)
	m := float64(scale * tmp)
	y := scale + m
	if math.Abs(y) < 1.0 {
		one := 1.0
		if y < 0.0 {
			one = -1.0
		}
		lo := scale - y + m
		hi := one + y
		lo = one - hi + y + lo
		y = (hi + lo) - one
		if y == 0 {
			y = math.Float64frombits(sbits & 0x8000000000000000)
		}
	}
	return 0x1p-1022 * y
}
