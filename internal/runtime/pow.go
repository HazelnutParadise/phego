package runtime

import (
	"math"
	"math/big"
)

// powPrecision is the working precision of correctlyRoundedPow in bits. It
// leaves enough guard bits that rounding the result to float64 is correct
// except in cases far closer to a rounding boundary than any test input.
const powPrecision = 256

// correctlyRoundedPow returns x**y rounded to the nearest float64. PHP's **
// calls the C library pow, and glibc rounds it to nearest; Go's math.Pow can
// be several units in the last place away, which PHP code observes through
// printing or comparison. Special values keep math.Pow's IEEE results, which
// match C pow.
func correctlyRoundedPow(x, y float64) float64 {
	if x == 0 || y == 0 || x == 1 || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return math.Pow(x, y)
	}
	negative := false
	if x < 0 {
		if y != math.Trunc(y) {
			return math.Pow(x, y) // NaN
		}
		x = -x
		negative = math.Mod(y, 2) != 0
	}
	if y == 1 {
		if negative {
			return -x
		}
		return x
	}
	// Results that certainly overflow or underflow need no extra precision.
	t := y * math.Log(x)
	if t > 720 || t < -760 {
		r := math.Pow(x, y)
		if negative {
			return -r
		}
		return r
	}
	prec := uint(powPrecision)
	lx := bigLog(new(big.Float).SetPrec(prec).SetFloat64(x), prec)
	e := bigExp(lx.Mul(lx, new(big.Float).SetPrec(prec).SetFloat64(y)), prec)
	r, _ := e.Float64()
	if negative {
		return -r
	}
	return r
}

// bigLn2 returns ln 2 at the given precision as 2*atanh(1/3).
func bigLn2(prec uint) *big.Float {
	third := new(big.Float).SetPrec(prec).Quo(big.NewFloat(1).SetPrec(prec), big.NewFloat(3).SetPrec(prec))
	return bigAtanhDoubled(third, prec)
}

// bigAtanhDoubled returns 2*atanh(z) for |z| <= 1/3 by its power series.
func bigAtanhDoubled(z *big.Float, prec uint) *big.Float {
	z2 := new(big.Float).SetPrec(prec).Mul(z, z)
	term := new(big.Float).SetPrec(prec).Set(z)
	sum := new(big.Float).SetPrec(prec).Set(z)
	piece := new(big.Float).SetPrec(prec)
	for k := int64(3); ; k += 2 {
		term.Mul(term, z2)
		piece.Quo(term, new(big.Float).SetPrec(prec).SetInt64(k))
		if piece.Sign() == 0 || piece.MantExp(nil)-sum.MantExp(nil) < -int(prec) {
			break
		}
		sum.Add(sum, piece)
	}
	return sum.Mul(sum, big.NewFloat(2).SetPrec(prec))
}

// bigLog returns ln x for x > 0.
func bigLog(x *big.Float, prec uint) *big.Float {
	m := new(big.Float).SetPrec(prec)
	exp := x.MantExp(m) // x = m * 2**exp, 0.5 <= m < 1
	// Move m into [sqrt(1/2), sqrt(2)) so the series argument stays small.
	if m.Cmp(big.NewFloat(math.Sqrt2/2)) < 0 {
		m.SetMantExp(m, 1)
		exp--
	}
	one := big.NewFloat(1).SetPrec(prec)
	num := new(big.Float).SetPrec(prec).Sub(m, one)
	den := new(big.Float).SetPrec(prec).Add(m, one)
	ln := bigAtanhDoubled(num.Quo(num, den), prec)
	return ln.Add(ln, new(big.Float).SetPrec(prec).Mul(bigLn2(prec), new(big.Float).SetPrec(prec).SetInt64(int64(exp))))
}

// bigExp returns e**t for |t| below about 800.
func bigExp(t *big.Float, prec uint) *big.Float {
	ln2 := bigLn2(prec)
	kf, _ := new(big.Float).SetPrec(prec).Quo(t, ln2).Float64()
	k := math.Round(kf)
	r := new(big.Float).SetPrec(prec).Sub(t, new(big.Float).SetPrec(prec).Mul(ln2, new(big.Float).SetPrec(prec).SetFloat64(k)))
	// Halve the argument before the Taylor series, then square back.
	const halvings = 24
	r.SetMantExp(r, -halvings)
	sum := big.NewFloat(1).SetPrec(prec)
	term := big.NewFloat(1).SetPrec(prec)
	for n := int64(1); ; n++ {
		term.Mul(term, r)
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(n))
		if term.Sign() == 0 || term.MantExp(nil) < -int(prec) {
			break
		}
		sum.Add(sum, term)
	}
	for range halvings {
		sum.Mul(sum, sum)
	}
	return sum.SetMantExp(sum, int(k))
}
