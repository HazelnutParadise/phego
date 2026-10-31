package runtime

import (
	"math"
	"strconv"
)

// PHP diagnostic levels raised by modeled operations.
const (
	levelWarning    = 2
	levelDeprecated = 8192
)

// diagnostic is one PHP notice, warning, or deprecation in raising order.
type diagnostic struct {
	level   int
	message string
}

// Throwable is a PHP exception or error thrown by an operation, identified by
// its PHP class and message.
type Throwable struct {
	Class   string
	Message string
}

func (t *Throwable) Error() string { return t.Class + ": " + t.Message }

func (h *heap) warn(level int, message string) {
	h.diagnostics = append(h.diagnostics, diagnostic{level, message})
}

func binopError(op string, a, b value) error {
	return &Throwable{"TypeError", "Unsupported operand types: " + typeName(a) + " " + op + " " + typeName(b)}
}

// valueName is PHP's zend_zval_value_name: booleans are spelled by value.
func valueName(v value) string {
	if v = v.deref(); v.kind == valueBool {
		if v.boolean {
			return "true"
		}
		return "false"
	}
	return typeName(v)
}

func (h *heap) number(i int64) value { return value{kind: valueInt, integer: i} }

func (h *heap) inRange(i int64) bool {
	return h.width == 64 || (i >= math.MinInt32 && i <= math.MaxInt32)
}

// floatToInt is zend_dval_to_lval: non-finite and out-of-range floats warn,
// then convert to 0 or by modular arithmetic.
func (h *heap) floatToInt(d float64) int64 {
	if math.IsInf(d, 0) || math.IsNaN(d) {
		h.warn(levelWarning, "The float "+php8512FormatFloat(d, -1)+" is not representable as an int, cast occurred")
		return 0
	}
	if !floatFitsInt(d, h.width) {
		h.warn(levelWarning, "The float "+php8512FormatFloat(d, -1)+" is not representable as an int, cast occurred")
		return floatToIntModular(d, h.width)
	}
	return int64(d)
}

// floatToIntSafe also reports an in-range float that loses precision.
func (h *heap) floatToIntSafe(d float64) int64 {
	i := h.floatToInt(d)
	if float64(i) != d && floatFitsInt(d, h.width) {
		h.warn(levelDeprecated, "Implicit conversion from float "+php8512FormatFloat(d, -1)+" to int loses precision")
	}
	return i
}

// floatToIntCap saturates, as PHP does for numeric strings holding floats.
func (h *heap) floatToIntCap(d float64) int64 {
	switch {
	case math.IsInf(d, 0) || math.IsNaN(d):
		return 0
	case !floatFitsInt(d, h.width):
		if d > 0 {
			return maxInt(h.width)
		}
		return minInt(h.width)
	}
	return int64(d)
}

func minInt(width int) int64 {
	if width == 32 {
		return math.MinInt32
	}
	return math.MinInt64
}

// toNumber is the operand conversion of PHP arithmetic. It fails for arrays
// and non-numeric strings and warns for leading-numeric strings.
func (h *heap) toNumber(v value) (value, bool) {
	switch v = v.deref(); v.kind {
	case valueInt, valueFloat:
		return v, true
	case valueNull, valueUndefined:
		return h.number(0), true
	case valueBool:
		if v.boolean {
			return h.number(1), true
		}
		return h.number(0), true
	case valueString:
		n := php8512NumericString(v.text, h.width, true)
		if n.kind == notNumeric {
			return value{}, false
		}
		if n.trailing {
			h.warn(levelWarning, "A non-numeric value encountered")
		}
		if n.kind == numericInt {
			return h.number(n.integer), true
		}
		return floatValue(n.floating), true
	}
	return value{}, false
}

// toIntOperand is the integer operand conversion of %, <<, >>, and bitwise
// operators.
func (h *heap) toIntOperand(v value) (int64, bool) {
	switch v = v.deref(); v.kind {
	case valueInt:
		return v.integer, true
	case valueNull, valueUndefined:
		return 0, true
	case valueBool:
		if v.boolean {
			return 1, true
		}
		return 0, true
	case valueFloat:
		return h.floatToIntSafe(v.floating), true
	case valueString:
		n := php8512NumericString(v.text, h.width, true)
		if n.kind == notNumeric {
			return 0, false
		}
		if n.trailing {
			h.warn(levelWarning, "A non-numeric value encountered")
		}
		if n.kind == numericInt {
			return n.integer, true
		}
		i := h.floatToIntCap(n.floating)
		if float64(i) != n.floating {
			h.warn(levelDeprecated, "Implicit conversion from float-string \""+cString(v.text)+"\" to int loses precision")
		}
		return i, true
	}
	return 0, false
}

// cString is how PHP's %s prints a byte string: up to the first NUL.
func cString(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}

// ToInt is the (int) cast.
func (h *heap) toInt(v value) int64 {
	switch v = v.deref(); v.kind {
	case valueInt:
		return v.integer
	case valueBool:
		if v.boolean {
			return 1
		}
	case valueFloat:
		return h.floatToInt(v.floating)
	case valueString:
		n := php8512NumericString(v.text, h.width, true)
		switch n.kind {
		case numericInt:
			return n.integer
		case numericFloat:
			return h.floatToIntCap(n.floating)
		}
	case valueArray:
		if v.array.count() > 0 {
			return 1
		}
	}
	return 0
}

// toFloat is the (float) cast.
func (h *heap) toFloat(v value) float64 {
	switch v = v.deref(); v.kind {
	case valueInt:
		return float64(v.integer)
	case valueFloat:
		return v.floating
	case valueBool:
		if v.boolean {
			return 1
		}
	case valueString:
		f, _ := php8512Strtod(cString(v.text))
		return f
	case valueArray:
		if v.array.count() > 0 {
			return 1
		}
	}
	return 0
}

// toBool is PHP truthiness. NaN is true and warns.
func (h *heap) toBool(v value) bool {
	switch v = v.deref(); v.kind {
	case valueBool:
		return v.boolean
	case valueInt:
		return v.integer != 0
	case valueFloat:
		if math.IsNaN(v.floating) {
			h.warn(levelWarning, "unexpected NAN value was coerced to bool")
		}
		return v.floating != 0
	case valueString:
		return len(v.text) > 1 || (len(v.text) == 1 && v.text[0] != '0')
	case valueArray:
		return v.array.count() > 0
	case valueObject:
		return true
	}
	return false
}

// toString is the (string) cast and the conversion used by concatenation.
func (h *heap) toString(v value) (string, error) {
	switch v = v.deref(); v.kind {
	case valueNull, valueUndefined:
		return "", nil
	case valueBool:
		if v.boolean {
			return "1", nil
		}
		return "", nil
	case valueInt:
		return strconv.FormatInt(v.integer, 10), nil
	case valueFloat:
		if math.IsNaN(v.floating) {
			h.warn(levelWarning, "unexpected NAN value was coerced to string")
		}
		return php8512FormatFloat(v.floating, 14), nil
	case valueString:
		return v.text, nil
	case valueArray:
		h.warn(levelWarning, "Array to string conversion")
		return "Array", nil
	}
	return "", &Throwable{"Error", "Object of class " + typeName(v) + " could not be converted to string"}
}

// intOrFloat returns the exact integer result when it is representable on the
// platform and otherwise the float result, as PHP integer overflow does.
func (h *heap) intOrFloat(i int64, ok bool, f float64) value {
	if ok && h.inRange(i) {
		return h.number(i)
	}
	return floatValue(f)
}

func (h *heap) arith(op string, a, b value) (value, error) {
	a, b = a.deref(), b.deref()
	if op == "+" && a.kind == valueArray && b.kind == valueArray {
		return h.arrayUnion(a.array, b.array), nil
	}
	x, ok := h.toNumber(a)
	if !ok {
		return value{}, binopError(op, a, b)
	}
	y, ok := h.toNumber(b)
	if !ok {
		return value{}, binopError(op, a, b)
	}
	if x.kind == valueInt && y.kind == valueInt {
		i, j := x.integer, y.integer
		switch op {
		case "+":
			s := i + j
			return h.intOrFloat(s, (s > i) == (j > 0), float64(i)+float64(j)), nil
		case "-":
			s := i - j
			return h.intOrFloat(s, (s < i) == (j > 0), float64(i)-float64(j)), nil
		case "*":
			p, ok := h.mulExact(i, j)
			return h.intOrFloat(p, ok, float64(i)*float64(j)), nil
		case "/":
			switch {
			case j == 0:
				return value{}, &Throwable{"DivisionByZeroError", "Division by zero"}
			case j == -1 && i == minInt(h.width):
				return floatValue(float64(i) / -1), nil
			case i%j == 0:
				return h.number(i / j), nil
			}
			return floatValue(float64(i) / float64(j)), nil
		case "**":
			return h.intPow(i, j), nil
		}
	}
	f, g := floatOf(x), floatOf(y)
	switch op {
	case "+":
		return floatValue(f + g), nil
	case "-":
		return floatValue(f - g), nil
	case "*":
		return floatValue(f * g), nil
	case "/":
		if g == 0 {
			return value{}, &Throwable{"DivisionByZeroError", "Division by zero"}
		}
		return floatValue(f / g), nil
	case "**":
		return floatValue(h.safePow(f, g)), nil
	}
	panic("unknown arithmetic operator " + op)
}

func floatOf(v value) float64 {
	if v.kind == valueInt {
		return float64(v.integer)
	}
	return v.floating
}

func (h *heap) safePow(base, exp float64) float64 {
	if base == 0 && exp < 0 {
		h.warn(levelDeprecated, "Power of base 0 and negative exponent is deprecated")
	}
	return correctlyRoundedPow(base, exp)
}

// mulExact multiplies within the platform integer range.
func (h *heap) mulExact(a, b int64) (int64, bool) {
	p := a * b
	if a != 0 && (p/a != b || (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64)) {
		return 0, false
	}
	return p, h.inRange(p)
}

// intPow is PHP's integer exponentiation by squaring, which switches to float
// arithmetic at the first overflowing step.
func (h *heap) intPow(base, exp int64) value {
	if exp < 0 {
		return floatValue(h.safePow(float64(base), float64(exp)))
	}
	if exp == 0 {
		return h.number(1)
	}
	if base == 0 {
		return h.number(0)
	}
	l1, l2, i := int64(1), base, exp
	for i >= 1 {
		if i%2 != 0 {
			i--
			p, ok := h.mulExact(l1, l2)
			if !ok {
				return floatValue(float64(l1) * float64(l2) * h.safePow(float64(l2), float64(i)))
			}
			l1 = p
		} else {
			i /= 2
			p, ok := h.mulExact(l2, l2)
			if !ok {
				return floatValue(float64(l1) * h.safePow(float64(l2)*float64(l2), float64(i)))
			}
			l2 = p
		}
	}
	return h.number(l1)
}

func (h *heap) intOperands(op string, a, b value) (int64, int64, error) {
	i, ok := h.toIntOperand(a)
	if !ok {
		return 0, 0, binopError(op, a, b)
	}
	j, ok := h.toIntOperand(b)
	if !ok {
		return 0, 0, binopError(op, a, b)
	}
	return i, j, nil
}

func (h *heap) mod(a, b value) (value, error) {
	i, j, err := h.intOperands("%", a, b)
	switch {
	case err != nil:
		return value{}, err
	case j == 0:
		return value{}, &Throwable{"DivisionByZeroError", "Modulo by zero"}
	case j == -1:
		return h.number(0), nil
	}
	return h.number(i % j), nil
}

func (h *heap) shift(op string, a, b value) (value, error) {
	i, j, err := h.intOperands(op, a, b)
	if err != nil {
		return value{}, err
	}
	if uint64(j) >= uint64(h.width) {
		if j < 0 {
			return value{}, &Throwable{"ArithmeticError", "Bit shift by negative number"}
		}
		if op == ">>" && i < 0 {
			return h.number(-1), nil
		}
		return h.number(0), nil
	}
	if op == ">>" {
		return h.number(i >> j), nil
	}
	if h.width == 32 {
		return h.number(int64(int32(uint32(i) << j))), nil
	}
	return h.number(int64(uint64(i) << j)), nil
}

func (h *heap) bitwise(op string, a, b value) (value, error) {
	a, b = a.deref(), b.deref()
	if a.kind == valueString && b.kind == valueString {
		x, y := a.text, b.text
		if (op == "|") != (len(x) >= len(y)) {
			x, y = y, x
		}
		// For |, x is the longer string; for & and ^, the shorter.
		out := []byte(x)
		for k := 0; k < len(out) && k < len(y); k++ {
			switch op {
			case "&":
				out[k] &= y[k]
			case "|":
				out[k] |= y[k]
			case "^":
				out[k] ^= y[k]
			}
		}
		return value{kind: valueString, text: string(out)}, nil
	}
	i, j, err := h.intOperands(op, a, b)
	if err != nil {
		return value{}, err
	}
	switch op {
	case "&":
		return h.number(i & j), nil
	case "|":
		return h.number(i | j), nil
	}
	return h.number(i ^ j), nil
}

func (h *heap) bitwiseNot(a value) (value, error) {
	switch a = a.deref(); a.kind {
	case valueInt:
		return h.number(^a.integer), nil
	case valueFloat:
		return h.number(^h.floatToIntSafe(a.floating)), nil
	case valueString:
		out := []byte(a.text)
		for k := range out {
			out[k] = ^out[k]
		}
		return value{kind: valueString, text: string(out)}, nil
	}
	return value{}, &Throwable{"TypeError", "Cannot perform bitwise not on " + valueName(a)}
}

func (h *heap) concat(a, b value) (value, error) {
	x, err := h.toString(a)
	if err != nil {
		return value{}, err
	}
	y, err := h.toString(b)
	if err != nil {
		return value{}, err
	}
	return value{kind: valueString, text: x + y}, nil
}

// arrayUnion is array + array: a copy of a with b's absent keys appended.
func (h *heap) arrayUnion(a, b *phpArray) value {
	out := h.duplicate(a)
	out.copiedFrom = nil
	pos := 0
	for e, ok := b.next(&pos); ok; e, ok = b.next(&pos) {
		if _, found := out.find(e.key); found {
			continue
		}
		v := e.val
		if v.kind == valueReference && v.ref.refcount == 1 {
			v = v.ref.target
		}
		retain(v)
		out.insert(e.key, v, h.width)
	}
	return value{kind: valueArray, array: out}
}

// threeWay is ZEND_THREEWAY_COMPARE: unordered floats compare as greater.
func threeWay(a, b float64) int {
	switch {
	case a == b:
		return 0
	case a < b:
		return -1
	}
	return 1
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

func compareInts(a, b int64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	}
	return 0
}

// byteCompare is zend_binary_strcmp normalized to -1, 0, or 1.
func byteCompare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func (h *heap) compareIntToString(i int64, s string) int {
	n := php8512NumericString(s, h.width, false)
	switch n.kind {
	case numericInt:
		return compareInts(i, n.integer)
	case numericFloat:
		return threeWay(float64(i), n.floating)
	}
	return byteCompare(strconv.FormatInt(i, 10), s)
}

func (h *heap) compareFloatToString(f float64, s string) int {
	n := php8512NumericString(s, h.width, false)
	switch n.kind {
	case numericInt:
		return threeWay(f, float64(n.integer))
	case numericFloat:
		return threeWay(f, n.floating)
	}
	str, _ := h.toString(floatValue(f))
	return byteCompare(str, s)
}

// smartStringCompare compares two strings numerically when both are numeric
// and bytewise otherwise.
func (h *heap) smartStringCompare(a, b string) int {
	x := php8512NumericString(a, h.width, false)
	y := php8512NumericString(b, h.width, false)
	if x.kind == notNumeric || y.kind == notNumeric || h.sameOverflow(x, y) {
		return byteCompare(a, b)
	}
	if x.kind == numericFloat || y.kind == numericFloat {
		d1, d2 := x.floating, y.floating
		switch {
		case x.kind != numericFloat:
			if y.overflow != 0 {
				return -y.overflow
			}
			d1 = float64(x.integer)
		case y.kind != numericFloat:
			if x.overflow != 0 {
				return x.overflow
			}
			d2 = float64(y.integer)
		case d1 == d2 && (math.IsInf(d1, 0) || math.IsNaN(d1)):
			return byteCompare(a, b)
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
	return compareInts(x.integer, y.integer)
}

// smartStringEqual is the == fast path for two strings.
func (h *heap) smartStringEqual(a, b string) bool {
	x := php8512NumericString(a, h.width, false)
	y := php8512NumericString(b, h.width, false)
	if x.kind == notNumeric || y.kind == notNumeric || h.sameOverflow(x, y) {
		return a == b
	}
	if x.kind == numericFloat || y.kind == numericFloat {
		d1, d2 := x.floating, y.floating
		switch {
		case x.kind != numericFloat:
			if y.overflow != 0 {
				return false
			}
			d1 = float64(x.integer)
		case y.kind != numericFloat:
			if x.overflow != 0 {
				return false
			}
			d2 = float64(y.integer)
		case d1 == d2 && (math.IsInf(d1, 0) || math.IsNaN(d1)):
			return a == b
		}
		return d1 == d2
	}
	return x.integer == y.integer
}

// sameOverflow reports two integer strings that overflowed to the same side
// with equal float values, which PHP then compares as strings.
func (h *heap) sameOverflow(x, y numericString) bool {
	if x.overflow == 0 || x.overflow != y.overflow || x.floating-y.floating != 0 {
		return false
	}
	if h.width == 32 {
		return (x.overflow == 1 && x.floating > 9007199254740991) || (x.overflow == -1 && x.floating < -9007199254740991)
	}
	return true
}

// silentNumber is the silent scalar-to-number conversion used by comparison.
func (h *heap) silentNumber(v value) value {
	switch v.kind {
	case valueNull, valueUndefined:
		return h.number(0)
	case valueBool:
		if v.boolean {
			return h.number(1)
		}
		return h.number(0)
	case valueString:
		n := php8512NumericString(v.text, h.width, true)
		switch n.kind {
		case numericInt:
			return h.number(n.integer)
		case numericFloat:
			return floatValue(n.floating)
		}
		return h.number(0)
	}
	return v
}

// isNullOrFalse is Z_TYPE < IS_TRUE.
func isNullOrFalse(v value) bool {
	return v.kind == valueNull || v.kind == valueUndefined || (v.kind == valueBool && !v.boolean)
}

func isTrue(v value) bool { return v.kind == valueBool && v.boolean }

func isNaN(v value) bool { return v.kind == valueFloat && math.IsNaN(v.floating) }

// compare is PHP 8.5.12's zend_compare, the basis of <=>, <, and loose ==.
func (h *heap) compare(a, b value) (int, error) {
	return h.compareIn(a, b, map[*phpArray]bool{})
}

func (h *heap) compareIn(a, b value, active map[*phpArray]bool) (int, error) {
	a, b = a.deref(), b.deref()
	converted := false
	for {
		switch {
		case a.kind == valueInt && b.kind == valueInt:
			return compareInts(a.integer, b.integer), nil
		case a.kind == valueFloat && b.kind == valueInt:
			return threeWay(a.floating, float64(b.integer)), nil
		case a.kind == valueInt && b.kind == valueFloat:
			return threeWay(float64(a.integer), b.floating), nil
		case a.kind == valueFloat && b.kind == valueFloat:
			return threeWay(a.floating, b.floating), nil
		case a.kind == valueArray && b.kind == valueArray:
			return h.compareArrays(a.array, b.array, active)
		case isNullOrFalse(a) && isNullOrFalse(b), isTrue(a) && isTrue(b):
			return 0, nil
		case (a.kind == valueNull || a.kind == valueUndefined) && isTrue(b):
			return -1, nil
		case isTrue(a) && (b.kind == valueNull || b.kind == valueUndefined):
			return 1, nil
		case a.kind == valueString && b.kind == valueString:
			return h.smartStringCompare(a.text, b.text), nil
		case (a.kind == valueNull || a.kind == valueUndefined) && b.kind == valueString:
			if b.text == "" {
				return 0, nil
			}
			return -1, nil
		case a.kind == valueString && (b.kind == valueNull || b.kind == valueUndefined):
			if a.text == "" {
				return 0, nil
			}
			return 1, nil
		case a.kind == valueInt && b.kind == valueString:
			return h.compareIntToString(a.integer, b.text), nil
		case a.kind == valueString && b.kind == valueInt:
			return -h.compareIntToString(b.integer, a.text), nil
		case a.kind == valueFloat && b.kind == valueString:
			if math.IsNaN(a.floating) {
				return 1, nil
			}
			return h.compareFloatToString(a.floating, b.text), nil
		case a.kind == valueString && b.kind == valueFloat:
			if math.IsNaN(b.floating) {
				return 1, nil
			}
			return -h.compareFloatToString(b.floating, a.text), nil
		case a.kind == valueObject || b.kind == valueObject:
			if a.kind == valueObject && b.kind == valueObject && a.object == b.object {
				return 0, nil
			}
			return 0, &Throwable{"Error", "object comparison is not modeled"}
		}
		if !converted {
			switch {
			case isNaN(a) || isNaN(b):
				switch {
				case isNullOrFalse(a):
					return -1, nil
				case isTrue(a) || isTrue(b):
					return 0, nil
				case isNullOrFalse(b):
					return 1, nil
				case a.kind != valueFloat:
					a = h.silentNumber(a)
				default:
					b = h.silentNumber(b)
				}
				converted = true
				continue
			case isNullOrFalse(a):
				if h.toBool(b) {
					return -1, nil
				}
				return 0, nil
			case isTrue(a):
				if h.toBool(b) {
					return 0, nil
				}
				return 1, nil
			case isNullOrFalse(b):
				if h.toBool(a) {
					return 1, nil
				}
				return 0, nil
			case isTrue(b):
				if h.toBool(a) {
					return 0, nil
				}
				return -1, nil
			}
			a, b = h.silentNumber(a), h.silentNumber(b)
			converted = true
			continue
		}
		if a.kind == valueArray {
			return 1, nil
		}
		return -1, nil
	}
}

// compareArrays is PHP's unordered array comparison: element count first,
// then each element of a against the same key in b.
func (h *heap) compareArrays(a, b *phpArray, active map[*phpArray]bool) (int, error) {
	if a == b {
		return 0, nil
	}
	if active[a] {
		return 1, &Throwable{"Error", ErrRecursiveDependency.Error()}
	}
	if a.count() != b.count() {
		if a.count() > b.count() {
			return 1, nil
		}
		return -1, nil
	}
	active[a] = true
	defer delete(active, a)
	pos := 0
	for e, ok := a.next(&pos); ok; e, ok = a.next(&pos) {
		other, found := b.lookup(e.key)
		if !found {
			return 1, nil
		}
		if c, err := h.compareIn(e.val, other.val, active); err != nil || c != 0 {
			return c, err
		}
	}
	return 0, nil
}

// looseEqual is PHP ==.
func (h *heap) looseEqual(a, b value) (bool, error) {
	if x, y := a.deref(), b.deref(); x.kind == valueString && y.kind == valueString {
		return h.smartStringEqual(x.text, y.text), nil
	}
	c, err := h.compare(a, b)
	return err == nil && c == 0, err
}
