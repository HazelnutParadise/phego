package runtime

import (
	"math"

	"github.com/HazelnutParadise/phego/internal/phpnum"
)

// numericKind is the result class of PHP numeric string recognition.
type numericKind uint8

const (
	notNumeric numericKind = iota
	numericInt
	numericFloat
)

// numericString is a recognized numeric string. Overflow is +1 or -1 when an
// integer string was too large for the platform integer and became a float.
type numericString struct {
	kind     numericKind
	integer  int64
	floating float64
	trailing bool
	overflow int
}

func isPHPSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func php8512NumericString(s string, width int, allowErrors bool) numericString {
	n := phpnum.Parse(s, width, allowErrors)
	return numericString{kind: numericKind(n.Kind), integer: n.Int, floating: n.Float, trailing: n.Trailing, overflow: n.Overflow}
}

func php8512Strtod(s string) (float64, int) { return phpnum.ParseFloat(s) }

func php8512FormatFloat(f float64, precision int) string { return phpnum.FormatFloat(f, precision) }

// floatFitsInt is ZEND_DOUBLE_FITS_LONG for the platform width.
func floatFitsInt(d float64, width int) bool {
	if width == 32 {
		return !(d > math.MaxInt32 || d < math.MinInt32)
	}
	return !(d >= 0x1p63 || d < -0x1p63)
}

// floatToIntModular converts an out-of-range finite float with PHP's modular
// arithmetic for the platform width.
func floatToIntModular(d float64, width int) int64 {
	if width == 32 {
		m := math.Mod(d, 0x1p32)
		if m < 0 {
			m = math.Ceil(m) + 0x1p32
		}
		return int64(int32(uint32(m)))
	}
	m := math.Mod(d, 0x1p64)
	if m < 0 {
		m += 0x1p64
	}
	return int64(uint64(m))
}
