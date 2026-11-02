package runtime

import (
	"math"
	"strconv"
	"strings"
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

// at returns s[i], or 0 past the end, matching PHP's NUL-terminated strings.
func at(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// php8512NumericString recognizes a numeric string as PHP 8.5.12 does for the
// platform width. Leading and trailing whitespace is allowed. With
// allowErrors, a leading-numeric string such as "1abc" is accepted and
// reported as having trailing data; otherwise it is not numeric. Hexadecimal,
// INF, and underscore spellings are never numeric.
func php8512NumericString(s string, width int, allowErrors bool) numericString {
	if s == "" || s[0] > '9' {
		return numericString{}
	}
	maxLength, limitDigits := 20, "9223372036854775808"
	if width == 32 {
		maxLength, limitDigits = 11, "2147483648"
	}
	start := 0
	for start < len(s) && isPHPSpace(s[start]) {
		start++
	}
	p := start
	negative := false
	switch at(s, p) {
	case '-':
		negative = true
		p++
	case '+':
		p++
	}
	var n numericString
	digits := 0
	var magnitude uint64
	if isDigit(at(s, p)) {
		for at(s, p) == '0' {
			p++
		}
		isFloat := false
	scan:
		for digits < maxLength {
			c := at(s, p)
			switch {
			case isDigit(c):
				magnitude = magnitude*10 + uint64(c-'0')
				digits++
				p++
			case c == '.':
				isFloat = true
				break scan
			case c == 'e' || c == 'E':
				e := p + 1
				if at(s, e) == '-' || at(s, e) == '+' {
					p = e
					e++
				}
				if isDigit(at(s, e)) {
					isFloat = true
				}
				break scan
			default:
				break scan
			}
		}
		if digits >= maxLength {
			n.overflow = 1
			if s[start] == '-' {
				n.overflow = -1
			}
			isFloat = true
		}
		if isFloat {
			n.kind = numericFloat
			var used int
			n.floating, used = php8512Strtod(s[start:])
			p = start + used
		} else {
			n.kind = numericInt
		}
	} else if at(s, p) == '.' && isDigit(at(s, p+1)) {
		n.kind = numericFloat
		var used int
		n.floating, used = php8512Strtod(s[start:])
		p = start + used
	} else {
		return numericString{}
	}
	if p != len(s) {
		end := p
		for end < len(s) && isPHPSpace(s[end]) {
			end++
		}
		if end != len(s) {
			if !allowErrors {
				return numericString{}
			}
			n.trailing = true
		}
	}
	if n.kind == numericInt {
		if digits == maxLength-1 {
			// PHP compares the digit run, and anything after it, with the
			// magnitude of the integer minimum as a C string.
			rest := s[p-digits:]
			if i := strings.IndexByte(rest, 0); i >= 0 {
				rest = rest[:i]
			}
			if cmp := strings.Compare(rest, limitDigits); !(cmp < 0 || (cmp == 0 && s[start] == '-')) {
				n.kind = numericFloat
				n.floating, _ = php8512Strtod(s[start:])
				n.overflow = 1
				if s[start] == '-' {
					n.overflow = -1
				}
				return n
			}
		}
		// PHP accumulates in an unsigned platform integer and casts, so a
		// digit run that slipped past the overflow check above wraps.
		u := magnitude
		if negative {
			u = -u
		}
		n.integer = int64(u)
		if width == 32 {
			n.integer = int64(int32(uint32(u)))
		}
	}
	return n
}

// php8512Strtod parses the decimal float prefix of s as zend_strtod does:
// leading whitespace, an optional sign, digits with an optional fraction, and
// an optional exponent. It returns the value and the bytes consumed, which is
// zero when s has no such prefix. Hexadecimal and INF/NAN are not accepted.
func php8512Strtod(s string) (float64, int) {
	p := 0
	for isPHPSpace(at(s, p)) {
		p++
	}
	begin := p
	if c := at(s, p); c == '-' || c == '+' {
		p++
	}
	mantissa := 0
	for isDigit(at(s, p)) {
		p++
		mantissa++
	}
	if at(s, p) == '.' {
		q := p + 1
		frac := 0
		for isDigit(at(s, q)) {
			q++
			frac++
		}
		if mantissa+frac > 0 {
			p = q
			mantissa += frac
		}
	}
	if mantissa == 0 {
		return 0, 0
	}
	if c := at(s, p); c == 'e' || c == 'E' {
		q := p + 1
		if c := at(s, q); c == '-' || c == '+' {
			q++
		}
		if isDigit(at(s, q)) {
			for isDigit(at(s, q)) {
				q++
			}
			p = q
		}
	}
	f, err := strconv.ParseFloat(s[begin:p], 64)
	if err != nil && !isRangeError(err) {
		return 0, 0
	}
	return f, p
}

func isRangeError(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// php8512FormatFloat formats f as zend_gcvt does. Precision 14 is PHP's
// default "precision" INI used for string conversion; -1 selects the shortest
// round-trip digits used by "%.*H" with precision -1.
func php8512FormatFloat(f float64, precision int) string {
	switch {
	case math.IsNaN(f):
		return "NAN"
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	}
	ndigit := precision
	if precision < 0 {
		ndigit = 17
	}
	formatted := strconv.FormatFloat(math.Abs(f), 'e', precision-1, 64)
	if precision < 0 {
		formatted = strconv.FormatFloat(math.Abs(f), 'e', -1, 64)
	}
	mant, exp, _ := strings.Cut(formatted, "e")
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	if digits == "" {
		digits = "0"
	}
	e, _ := strconv.Atoi(exp)
	decpt := e + 1
	if f == 0 {
		decpt = 1
	}
	var b strings.Builder
	if math.Signbit(f) {
		b.WriteByte('-')
	}
	switch {
	case (decpt >= 0 && decpt > ndigit) || decpt < -3:
		b.WriteByte(digits[0])
		b.WriteByte('.')
		if len(digits) == 1 {
			b.WriteByte('0')
		} else {
			b.WriteString(digits[1:])
		}
		b.WriteByte('E')
		if decpt-1 < 0 {
			b.WriteByte('-')
			b.WriteString(strconv.Itoa(1 - decpt))
		} else {
			b.WriteByte('+')
			b.WriteString(strconv.Itoa(decpt - 1))
		}
	case decpt < 0:
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", -decpt))
		b.WriteString(digits)
	default:
		if decpt == 0 {
			b.WriteByte('0')
		} else if len(digits) >= decpt {
			b.WriteString(digits[:decpt])
		} else {
			b.WriteString(digits)
			b.WriteString(strings.Repeat("0", decpt-len(digits)))
		}
		if len(digits) > decpt {
			b.WriteByte('.')
			b.WriteString(digits[decpt:])
		}
	}
	return b.String()
}

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
