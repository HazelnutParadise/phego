package runtime

import "github.com/HazelnutParadise/phego/internal/phpnum"

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
func floatFitsInt(d float64, width int) bool { return phpnum.DoubleFitsLong(d, width) }

// floatToIntModular is zend_dval_to_lval_slow for the platform width.
func floatToIntModular(d float64, width int) int64 { return phpnum.DoubleToLongModular(d, width) }
