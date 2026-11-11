package phpnum

import (
	"cmp"
	"math"
	"strings"
)

// CompareStrings compares two strings numerically when both are numeric
// and bytewise otherwise.
func CompareStrings(a, b string, width int) int {
	x := Parse(a, width, false)
	y := Parse(b, width, false)
	if x.Kind == NotNumeric || y.Kind == NotNumeric || sameOverflow(x, y, width) {
		return strings.Compare(a, b)
	}
	if x.Kind == Floating || y.Kind == Floating {
		d1, d2 := x.Float, y.Float
		switch {
		case x.Kind != Floating:
			if y.Overflow != 0 {
				return -y.Overflow
			}
			d1 = float64(x.Int)
		case y.Kind != Floating:
			if x.Overflow != 0 {
				return x.Overflow
			}
			d2 = float64(y.Int)
		case d1 == d2 && (math.IsInf(d1, 0) || math.IsNaN(d1)):
			return strings.Compare(a, b)
		}
		d := d1 - d2
		switch {
		case d > 0:
			return 1
		case d < 0:
			return -1
		}
		return 0
	}
	return cmp.Compare(x.Int, y.Int)
}

// EqualStrings is the == fast path for two strings.
func EqualStrings(a, b string, width int) bool {
	x := Parse(a, width, false)
	y := Parse(b, width, false)
	if x.Kind == NotNumeric || y.Kind == NotNumeric || sameOverflow(x, y, width) {
		return a == b
	}
	if x.Kind == Floating || y.Kind == Floating {
		d1, d2 := x.Float, y.Float
		switch {
		case x.Kind != Floating:
			if y.Overflow != 0 {
				return false
			}
			d1 = float64(x.Int)
		case y.Kind != Floating:
			if x.Overflow != 0 {
				return false
			}
			d2 = float64(y.Int)
		case d1 == d2 && (math.IsInf(d1, 0) || math.IsNaN(d1)):
			return a == b
		}
		return d1 == d2
	}
	return x.Int == y.Int
}

// sameOverflow reports two integer strings that overflowed to the same side
// with equal float values, which PHP then compares as strings.
func sameOverflow(x, y Number, width int) bool {
	if x.Overflow == 0 || x.Overflow != y.Overflow || x.Float-y.Float != 0 {
		return false
	}
	if width == 32 {
		return (x.Overflow == 1 && x.Float > 9007199254740991) || (x.Overflow == -1 && x.Float < -9007199254740991)
	}
	return true
}
