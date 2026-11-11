package runtime

import (
	"math"
	"strconv"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// PHP 8.5.12 operations the earlier runtime helpers did not model. Each one
// follows the Zend function named in its comment; the expectations are in
// testdata/vm-oracle.

// undefinedKeyMessage is the "Undefined array key" warning of zend_execute.c,
// which prints string keys with %s (up to the first NUL).
func undefinedKeyMessage(k arrayKey) string {
	if k.isString {
		return "Undefined array key \"" + cString(k.text) + "\""
	}
	return "Undefined array key " + strconv.FormatInt(k.integer, 10)
}

// readKey converts a read or isset offset; ints need no conversion.
func (s *vmState) readKey(k value) (arrayKey, error) {
	if k.kind == valueInt {
		return intKey(k.integer), nil
	}
	if k.kind == valueUndefined {
		k = nullValue()
	}
	return s.h.arrayKey(k, false)
}

// arrayGet is FETCH_DIM_R (aux 0) and FETCH_DIM_IS (aux 1).
func (s *vmState) arrayGet(regs []value, in *vmInstr) error {
	c, k := regs[in.b].deref(), regs[in.c]
	quiet := in.aux == 1
	switch c.kind {
	case valueArray:
		key, err := s.readKey(k)
		if err != nil || s.stopped() {
			return err
		}
		if b, ok := c.array.lookup(key); ok {
			return s.setReg(regs, in.a, b.val.deref())
		}
		if !quiet {
			s.h.warn(levelWarning, undefinedKeyMessage(key))
		}
		return s.setReg(regs, in.a, nullValue())
	case valueString:
		return s.unsupported("reading a string offset")
	case valueObject, valueResource:
		return s.unsupported("reading an offset of an object or resource")
	}
	// zend_fetch_dimension_address_read: null, bool, int and float containers
	// warn (except in isset mode) and read null; the offset is not converted.
	if !quiet {
		s.h.warn(levelWarning, "Trying to access array offset on "+valueName(c))
	}
	return s.setReg(regs, in.a, nullValue())
}

// writable prepares the container of an element write: undefined and null
// become an array silently, false with PHP 8.1's deprecation; true, numbers
// and strings fail as zend_fetch_dimension_address_W does.
func (s *vmState) writable(slot *value, appending bool) error {
	switch c := slot.deref(); c.kind {
	case valueArray, valueUndefined, valueNull:
		return nil
	case valueBool:
		if c.boolean {
			break
		}
		// zend_false_to_array_deprecated, then array_init.
		s.h.warn(levelDeprecated, "Automatic conversion of false to array is deprecated")
		if s.stopped() {
			return nil
		}
		return s.h.assign(slot, nullValue())
	case valueString:
		if appending {
			return &Throwable{"Error", "[] operator not supported for strings"}
		}
		return s.unsupported("writing a string offset")
	case valueObject, valueResource:
		return s.unsupported("writing an offset of an object or resource")
	}
	return &Throwable{"Error", "Cannot use a scalar value as an array"}
}

func (s *vmState) arraySet(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	if err := s.writable(slot, false); err != nil || s.stopped() {
		return err
	}
	k, v := f.regs[in.b], f.regs[in.c]
	if k.kind == valueUndefined {
		k = nullValue()
	}
	if v.kind == valueUndefined {
		v = nullValue()
	}
	return s.h.setElement(slot, k, v)
}

func (s *vmState) arrayAppend(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	if err := s.writable(slot, true); err != nil || s.stopped() {
		return err
	}
	v := f.regs[in.b]
	if v.kind == valueUndefined {
		v = nullValue()
	}
	if err := s.h.appendValue(slot, v); err != nil {
		if err == ErrNextElementOccupied {
			return &Throwable{"Error", err.Error()}
		}
		return err
	}
	return nil
}

// arrayUnset is ZEND_UNSET_DIM.
func (s *vmState) arrayUnset(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	k := f.regs[in.b]
	if k.kind == valueUndefined {
		k = nullValue()
	}
	switch c := slot.deref(); c.kind {
	case valueArray:
		return s.h.unsetElement(slot, k)
	case valueUndefined:
		s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, in.a))
		return nil
	case valueNull:
		return nil
	case valueBool:
		if !c.boolean {
			s.h.warn(levelDeprecated, "Automatic conversion of false to array is deprecated")
			return nil
		}
	case valueString:
		return &Throwable{"Error", "Cannot unset string offsets"}
	case valueObject, valueResource:
		return s.unsupported("unsetting an offset of an object or resource")
	}
	return &Throwable{"Error", "Cannot unset offset in a non-array variable"}
}

// arrayIsset is ZEND_ISSET_ISEMPTY_DIM_OBJ for isset().
func (s *vmState) arrayIsset(regs []value, in *vmInstr) error {
	c, k := regs[in.b].deref(), regs[in.c].deref()
	result := false
	switch c.kind {
	case valueArray:
		if k.kind == valueArray || k.kind == valueObject {
			return &Throwable{"TypeError", "Cannot access offset of type " + typeName(k) + " in isset or empty"}
		}
		key, err := s.readKey(k)
		if err != nil || s.stopped() {
			return err
		}
		if b, ok := c.array.lookup(key); ok {
			v := b.val.deref()
			result = v.kind != valueNull && v.kind != valueUndefined
		}
	case valueString:
		var err error
		result, err = s.stringOffsetIsset(c.text, k)
		if err != nil || s.stopped() {
			return err
		}
	case valueObject, valueResource:
		return s.unsupported("isset on an offset of an object or resource")
	}
	return s.setReg(regs, in.a, boolValue(result))
}

// stringOffsetIsset is zend_isset_dim_slow for a string container: integer
// offsets, simple scalars and integer numeric strings are positions.
func (s *vmState) stringOffsetIsset(str string, k value) (bool, error) {
	var pos int64
	switch k.kind {
	case valueInt:
		pos = k.integer
	case valueUndefined, valueNull:
		pos = 0
	case valueBool:
		if k.boolean {
			pos = 1
		}
	case valueFloat:
		// zval_get_long_ex(offset, is_strict): zend_dval_to_lval, then the
		// incompatible-float deprecation whenever the value changed.
		pos = s.h.floatToInt(k.floating)
		if float64(pos) != k.floating {
			s.h.warn(levelDeprecated, "Implicit conversion from float "+php8512FormatFloat(k.floating, -1)+" to int loses precision")
		}
	case valueString:
		n := php8512NumericString(k.text, s.h.width, false)
		if n.kind != numericInt {
			return false, nil
		}
		pos = n.integer
	default:
		return false, nil
	}
	if pos < 0 {
		pos += int64(len(str))
	}
	return pos >= 0 && pos < int64(len(str)), nil
}

func (s *vmState) isset(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.b)
	if err != nil {
		return err
	}
	v := slot.deref()
	return s.setReg(f.regs, in.a, boolValue(v.kind != valueUndefined && v.kind != valueNull))
}

func (s *vmState) unset(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	return s.h.unset(slot)
}

// unary is ZEND_BOOL_NOT, the unary minus and plus PHP compiles to
// mul_function(x, -1) and mul_function(x, 1), and ZEND_BW_NOT.
func (s *vmState) unary(op bytecode.UnaryOp, v value) (value, error) {
	switch op {
	case bytecode.Not:
		return boolValue(!s.h.toBool(v)), nil
	case bytecode.Negate:
		if v.kind == valueInt && v.integer != minInt(s.h.width) {
			return value{kind: valueInt, integer: -v.integer}, nil
		}
		return s.h.arith("*", v, s.h.number(-1))
	case bytecode.Plus:
		return s.h.arith("*", v, s.h.number(1))
	case bytecode.BitNot:
		return s.h.bitwiseNot(v)
	}
	return value{}, s.internal("unknown unary operator %d", op)
}

// cast is ZEND_CAST.
func (s *vmState) cast(kind bytecode.CastKind, v value) (value, error) {
	h := s.h
	v = v.deref()
	switch kind {
	case bytecode.CastBool:
		return boolValue(h.toBool(v)), nil
	case bytecode.CastInt:
		return h.number(h.toInt(v)), nil
	case bytecode.CastFloat:
		return floatValue(h.toFloat(v)), nil
	case bytecode.CastString:
		if v.kind == valueString {
			return v, nil
		}
		str, err := h.toString(v)
		return stringValue(str), err
	case bytecode.CastArray:
		switch v.kind {
		case valueArray:
			return v, nil
		case valueUndefined, valueNull:
			return h.newArrayValue(), nil
		case valueObject, valueResource:
			return value{}, s.unsupported("casting an object or resource to array")
		}
		out := h.newArrayValue()
		if err := h.setElement(&out, h.number(0), v); err != nil {
			return value{}, err
		}
		return out, nil
	}
	return value{}, s.internal("unknown cast %d", kind)
}

// incDec is ZEND_PRE_INC and its relatives on a variable.
func (s *vmState) incDec(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.b)
	if err != nil {
		return err
	}
	op := bytecode.IncDecOp(in.aux)
	old := slot.deref()
	if old.kind == valueUndefined {
		// The CV becomes null before the warning, as ZVAL_UNDEFINED_OP1.
		if err := s.h.assign(slot, nullValue()); err != nil {
			return err
		}
		s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, in.b))
		if s.stopped() {
			return nil
		}
		old = nullValue()
	}
	inc := op == bytecode.PreInc || op == bytecode.PostInc
	var next value
	if inc {
		next, err = s.increment(old)
	} else {
		next, err = s.decrement(old)
	}
	if err != nil || s.stopped() {
		return err
	}
	if old.kind < valueArray && next.kind < valueArray && slot.kind < valueArray {
		*slot = next
	} else if err := s.h.assign(slot, next); err != nil {
		return err
	}
	result := next
	if op == bytecode.PostInc || op == bytecode.PostDec {
		result = old
	}
	return s.setReg(f.regs, in.a, result)
}

// increment is increment_function.
func (s *vmState) increment(v value) (value, error) {
	h := s.h
	switch v.kind {
	case valueInt:
		if v.integer == maxInt(h.width) {
			return floatValue(float64(v.integer) + 1), nil
		}
		return h.number(v.integer + 1), nil
	case valueFloat:
		return floatValue(v.floating + 1), nil
	case valueNull:
		return h.number(1), nil
	case valueBool:
		h.warn(levelWarning, "Increment on type bool has no effect, this will change in the next major version of PHP")
		return v, nil
	case valueString:
		n := php8512NumericString(v.text, h.width, false)
		switch n.kind {
		case numericInt:
			if n.integer == maxInt(h.width) {
				return floatValue(float64(n.integer) + 1), nil
			}
			return h.number(n.integer + 1), nil
		case numericFloat:
			return floatValue(n.floating + 1), nil
		}
		h.warn(levelDeprecated, "Increment on non-numeric string is deprecated, use str_increment() instead")
		return stringValue(incrementString(v.text)), nil
	case valueArray:
		return value{}, &Throwable{"TypeError", "Cannot increment array"}
	}
	return value{}, s.unsupported("incrementing an object or resource")
}

// decrement is decrement_function.
func (s *vmState) decrement(v value) (value, error) {
	h := s.h
	switch v.kind {
	case valueInt:
		if v.integer == minInt(h.width) {
			return floatValue(float64(v.integer) - 1), nil
		}
		return h.number(v.integer - 1), nil
	case valueFloat:
		return floatValue(v.floating - 1), nil
	case valueNull:
		h.warn(levelWarning, "Decrement on type null has no effect, this will change in the next major version of PHP")
		return v, nil
	case valueBool:
		h.warn(levelWarning, "Decrement on type bool has no effect, this will change in the next major version of PHP")
		return v, nil
	case valueString:
		if v.text == "" {
			h.warn(levelDeprecated, "Decrement on empty string is deprecated as non-numeric")
			return h.number(-1), nil
		}
		n := php8512NumericString(v.text, h.width, false)
		switch n.kind {
		case numericInt:
			if n.integer == minInt(h.width) {
				return floatValue(float64(n.integer) - 1), nil
			}
			return h.number(n.integer - 1), nil
		case numericFloat:
			return floatValue(n.floating - 1), nil
		}
		h.warn(levelDeprecated, "Decrement on non-numeric string has no effect and is deprecated")
		return v, nil
	case valueArray:
		return value{}, &Throwable{"TypeError", "Cannot decrement array"}
	}
	return value{}, s.unsupported("decrementing an object or resource")
}

// incrementString is increment_string's Perl-style increment.
func incrementString(str string) string {
	if str == "" {
		return "1"
	}
	b := []byte(str)
	const lower, upper, numeric = 1, 2, 3
	last := 0
	pos := len(b) - 1
	carry := false
	for {
		ch := b[pos]
		switch {
		case ch >= 'a' && ch <= 'z':
			carry = ch == 'z'
			if carry {
				b[pos] = 'a'
			} else {
				b[pos]++
			}
			last = lower
		case ch >= 'A' && ch <= 'Z':
			carry = ch == 'Z'
			if carry {
				b[pos] = 'A'
			} else {
				b[pos]++
			}
			last = upper
		case ch >= '0' && ch <= '9':
			carry = ch == '9'
			if carry {
				b[pos] = '0'
			} else {
				b[pos]++
			}
			last = numeric
		default:
			carry = false
		}
		if !carry || pos == 0 {
			break
		}
		pos--
	}
	if carry {
		first := byte('1')
		switch last {
		case upper:
			first = 'A'
		case lower:
			first = 'a'
		}
		return string(first) + string(b)
	}
	return string(b)
}

// exitStatus is exit()'s string|int parameter in coercive mode: strings are
// printed, ints are the status, bool and integral floats coerce to int, a
// fractional float is truncated with the precision deprecation, null is
// deprecated and becomes 0, and arrays are a TypeError raised from an exit()
// frame. It reports the exit code.
func (s *vmState) exitStatus(v value) (int, completion, error) {
	h := s.h
	switch v = v.deref(); v.kind {
	case valueString:
		return 0, s.output(v), nil
	case valueInt:
		return int(v.integer), completion{}, nil
	case valueBool:
		if v.boolean {
			return 1, completion{}, nil
		}
		return 0, completion{}, nil
	case valueUndefined, valueNull:
		h.warn(levelDeprecated, "exit(): Passing null to parameter #1 ($status) of type string|int is deprecated")
		return 0, completion{}, nil
	case valueFloat:
		f := v.floating
		if !math.IsNaN(f) && !math.IsInf(f, 0) && floatFitsInt(f, h.width) {
			if f != math.Trunc(f) {
				h.warn(levelDeprecated, "Implicit conversion from float "+php8512FormatFloat(f, -1)+" to int loses precision")
			}
			return int(int64(f)), completion{}, nil
		}
		str, _ := h.toString(v)
		return 0, s.output(stringValue(str)), nil
	case valueArray:
		return 0, completion{}, &vmInternalFrameThrow{function: "exit", args: []value{v},
			thrown: &Throwable{"TypeError", "exit(): Argument #1 ($status) must be of type string|int, array given"}}
	}
	return 0, completion{}, s.unsupported("exit with an object or resource status")
}

// vmInternalFrameThrow is a throwable raised inside an internal function,
// whose backtrace starts with that function's frame (PHP 8.5's exit()).
type vmInternalFrameThrow struct {
	function string
	args     []value
	thrown   *Throwable
}

func (e *vmInternalFrameThrow) Error() string { return e.thrown.Error() }
func (e *vmInternalFrameThrow) Unwrap() error { return e.thrown }
