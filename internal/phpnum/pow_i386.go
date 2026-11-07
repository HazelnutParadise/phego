package phpnum

import (
	"math"
	"math/big"
	"sync"
)

const (
	i386WorkPrecision = 256
	i386MathPrecision = 64
	i386PHPPrecision  = 53
	i386SeriesTerms   = 128
)

var i386LogConstants = sync.OnceValue(func() (ln2 *big.Float) {
	ln2 = i386AtanhLog(big.NewFloat(1).SetPrec(i386WorkPrecision).SetMode(big.ToNearestEven).Quo(
		big.NewFloat(1).SetPrec(i386WorkPrecision).SetMode(big.ToNearestEven),
		big.NewFloat(3).SetPrec(i386WorkPrecision).SetMode(big.ToNearestEven),
	))
	return ln2
})

// i386Pow models PHP 8.5.12's 32-bit power arithmetic using a fixed amount of
// pure-Go work. The implementation must retain 53-bit intermediate arithmetic
// with an extended exponent range, and the integral-exponent branch order.
// Its fixed-precision transcendental approximations match the measured oracle
// corpus; they are not certified correctly rounded and do not prove bitwise
// equivalence with every x87 implementation.
func i386Pow(x, y float64) float64 {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || x == 0 || y == 0 || math.Abs(x) == 1 {
		return math.Pow(x, y)
	}

	negative := false
	if x < 0 {
		if math.Trunc(y) != y {
			return math.NaN()
		}
		negative = math.Mod(math.Abs(y), 2) == 1
		x = -x
	}

	if y == math.Trunc(y) && math.Abs(y) < 1024 {
		result := i386IntegerPow(x, int(math.Abs(y)), y < 0)
		if negative {
			result.Neg(result)
		}
		value, _ := result.Float64()
		return value
	}

	log2 := i386Log2(x)
	log2.SetPrec(i386MathPrecision)
	z := i386PHPFloat().Mul(i386PHPFloat().SetFloat64(y), log2)
	if z.Cmp(i386PHPFloat().SetInt64(1200)) > 0 {
		return math.Copysign(math.Inf(1), i386Sign(negative))
	}
	if z.Cmp(i386PHPFloat().SetInt64(-1200)) < 0 {
		return math.Copysign(0, i386Sign(negative))
	}

	n := i386NearestEvenInt(z)
	f := i386PHPFloat().Sub(z, i386PHPFloat().SetInt64(n))
	t := i386Exp2m1(f)
	t.SetPrec(i386MathPrecision)
	r := i386PHPFloat().Add(t, i386PHPFloat().SetInt64(1))
	r.SetMantExp(r, int(n))
	if negative {
		r.Neg(r)
	}
	value, _ := r.Float64()
	return value
}

func i386Sign(negative bool) float64 {
	if negative {
		return -1
	}
	return 1
}

func i386PHPFloat() *big.Float {
	return new(big.Float).SetPrec(i386PHPPrecision).SetMode(big.ToNearestEven)
}

func i386WorkFloat() *big.Float {
	return new(big.Float).SetPrec(i386WorkPrecision).SetMode(big.ToNearestEven)
}

// i386IntegerPow performs the measured reciprocal-first, rounded-squaring path.
func i386IntegerPow(base float64, exponent int, reciprocal bool) *big.Float {
	x := i386PHPFloat().SetFloat64(base)
	if reciprocal {
		x = i386PHPFloat().Quo(i386PHPFloat().SetInt64(1), x)
	}
	r := i386PHPFloat().SetInt64(1)
	for exponent != 0 {
		if exponent&1 != 0 {
			r = i386PHPFloat().Mul(r, x)
		}
		exponent >>= 1
		if exponent != 0 {
			x = i386PHPFloat().Mul(x, x)
		}
	}
	return r
}

// i386AtanhLog returns 2*(u+u^3/3+...) with a fixed 128 terms.
func i386AtanhLog(u *big.Float) *big.Float {
	u2 := i386WorkFloat().Mul(u, u)
	power := i386WorkFloat().Set(u)
	sum := i386WorkFloat()
	term, divisor := i386WorkFloat(), i386WorkFloat()
	for odd := int64(1); odd <= 2*i386SeriesTerms-1; odd += 2 {
		term.Quo(power, divisor.SetInt64(odd))
		sum.Add(sum, term)
		power.Mul(power, u2)
	}
	return i386WorkFloat().Mul(sum, i386WorkFloat().SetInt64(2))
}

// i386Log2 evaluates log2(x) from a normalized mantissa without cancellation
// from subtracting a large binary exponent before the logarithm is computed.
func i386Log2(x float64) *big.Float {
	mantissa, exponent := math.Frexp(x)
	m := i386WorkFloat().SetFloat64(mantissa * 2)
	u := i386WorkFloat().Quo(
		i386WorkFloat().Sub(m, i386WorkFloat().SetInt64(1)),
		i386WorkFloat().Add(m, i386WorkFloat().SetInt64(1)),
	)
	logMantissa := i386AtanhLog(u)
	logMantissa.Quo(logMantissa, i386LogConstants())
	logMantissa.Add(logMantissa, i386WorkFloat().SetInt64(int64(exponent-1)))
	return logMantissa
}

// i386Exp2m1 evaluates exp(f*ln(2))-1 with a fixed 128-term Taylor sum.
func i386Exp2m1(f *big.Float) *big.Float {
	a := i386WorkFloat().Mul(f, i386LogConstants())
	term := i386WorkFloat().Set(a)
	sum := i386WorkFloat().Set(a)
	divisor := i386WorkFloat()
	for n := int64(2); n <= i386SeriesTerms; n++ {
		term.Mul(term, a)
		term.Quo(term, divisor.SetInt64(n))
		sum.Add(sum, term)
	}
	return sum
}

func i386NearestEvenInt(x *big.Float) int64 {
	i, _ := x.Int(nil)
	whole := new(big.Float).SetPrec(i386PHPPrecision).SetMode(big.ToNearestEven).SetInt(i)
	fraction := i386PHPFloat().Sub(x, whole)
	absFraction := i386PHPFloat().Abs(fraction)
	half := i386PHPFloat().SetFloat64(0.5)
	comparison := absFraction.Cmp(half)
	if comparison > 0 || (comparison == 0 && i.Bit(0) != 0) {
		if x.Sign() < 0 {
			i.Sub(i, big.NewInt(1))
		} else {
			i.Add(i, big.NewInt(1))
		}
	}
	return i.Int64()
}
