package phpnum

import "math"

// Pow evaluates the PHP 8.5.12 profile's floating power operation. The width
// is the guest integer width, independently of the host Go architecture.
// Callers retain responsibility for PHP's zero-base negative-power diagnostic.
func Pow(base, exp float64, width int) float64 {
	if width == 32 {
		return i386Pow(base, exp)
	}
	return glibcPow(base, exp)
}

// MulExact multiplies within the guest's signed integer range.
func MulExact(a, b int64, width int) (int64, bool) {
	p := a * b
	if a != 0 && (p/a != b || a == -1 && b == math.MinInt64 || b == -1 && a == math.MinInt64) {
		return 0, false
	}
	return p, width != 32 || p >= math.MinInt32 && p <= math.MaxInt32
}

// IntPow preserves the integer result until the first overflowing step, then
// continues that same multiplication chain with the profile's floating pow.
// Inputs are representable guest integers. Only one numeric result is valid:
// integer when isInt is true, and floating otherwise.
func IntPow(base, exp int64, width int) (integer int64, floating float64, isInt bool) {
	if exp < 0 {
		return 0, Pow(float64(base), float64(exp), width), false
	}
	if exp == 0 {
		return 1, 0, true
	}
	if base == 0 {
		return 0, 0, true
	}
	l1, l2, i := int64(1), base, exp
	for i >= 1 {
		if i%2 != 0 {
			i--
			p, ok := MulExact(l1, l2, width)
			if !ok {
				return 0, float64(float64(l1)*float64(l2)) * Pow(float64(l2), float64(i), width), false
			}
			l1 = p
		} else {
			i /= 2
			p, ok := MulExact(l2, l2, width)
			if !ok {
				return 0, float64(l1) * Pow(float64(float64(l2)*float64(l2)), float64(i), width), false
			}
			l2 = p
		}
	}
	return l1, 0, true
}
