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

// arrayGet is FETCH_DIM_R (aux 0), FETCH_DIM_IS (aux ArrayGetQuiet) and
// FETCH_LIST_R (aux ArrayGetList), zend_fetch_dimension_address_read with
// is_list set for the last.
func (s *vmState) arrayGet(regs []value, in *vmInstr) error {
	c, k := regs[in.b].deref(), regs[in.c]
	quiet, list := in.aux == bytecode.ArrayGetQuiet, in.aux == bytecode.ArrayGetList
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
		if !list {
			return s.stringOffsetGet(regs, in, c.text, k)
		}
	case valueObject, valueResource:
		return s.unsupported("reading an offset of an object or resource")
	}
	// zend_fetch_dimension_address_read: null, bool, int and float containers
	// warn (except in isset mode) and read null; the offset is not converted.
	// A list read says "Cannot use T as array" for every container but null,
	// a string included, with the type's name rather than the value's.
	switch {
	case list && c.kind != valueNull && c.kind != valueUndefined:
		s.h.warn(levelWarning, "Cannot use "+typeName(c)+" as array")
	case !list && !quiet:
		s.h.warn(levelWarning, "Trying to access array offset on "+valueName(c))
	}
	return s.setReg(regs, in.a, nullValue())
}

// writable prepares the container of an element write: undefined and null
// become an empty array silently, false with PHP 8.1's deprecation; true,
// numbers and strings fail as zend_fetch_dimension_address_W does. The array
// exists before the key is read ($n = null; $n[$n] = 1 is a TypeError, as the
// pinned PHP 8.5.12 prints "Cannot access offset of type array on array").
func (s *vmState) writable(slot *value, appending bool) error {
	switch c := slot.deref(); c.kind {
	case valueArray:
		return nil
	case valueUndefined, valueNull:
		return s.h.assign(slot, s.h.newArrayValue())
	case valueBool:
		if c.boolean {
			break
		}
		// zend_false_to_array_deprecated, then array_init.
		s.h.warn(levelDeprecated, "Automatic conversion of false to array is deprecated")
		if s.stopped() {
			return nil
		}
		return s.h.assign(slot, s.h.newArrayValue())
	case valueString:
		if appending {
			return &Throwable{"Error", "[] operator not supported for strings"}
		}
		// ArraySet and FetchDim handle a string container before they
		// prepare the container.
		return s.internal("string container reached the array write path")
	case valueObject, valueResource:
		return s.unsupported("writing an offset of an object or resource")
	}
	return &Throwable{"Error", "Cannot use a scalar value as an array"}
}

// readVar reads the variable a Location register names, as a CV operand of a
// Zend opcode: an undefined variable warns and reads as null. undefined
// reports that it was undefined.
func (s *vmState) readVar(f *vmFrame, r uint32) (v value, undefined bool, err error) {
	slot, err := s.slot(f, r)
	if err != nil {
		return value{}, false, err
	}
	if v = slot.deref(); v.kind != valueUndefined {
		return v, false, nil
	}
	s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, r))
	return nullValue(), true, nil
}

// operandValue is a Value register operand; undefined reads as null.
func operandValue(regs []value, r uint32) value {
	if v := regs[r]; v.kind != valueUndefined {
		return v
	}
	return nullValue()
}

// arraySet is ZEND_ASSIGN_DIM on a variable or an element: the container
// check (with PHP
// 8.1's false deprecation), then the key, then the OP_DATA value, then the
// write. A key or value given as a Location (bytecode.KeyLocation,
// bytecode.ValueLocation) is read at that point, so its undefined-variable
// warning follows the container's errors and the key's conversion
// diagnostics exactly as in PHP.
func (s *vmState) arraySet(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	if slot.deref().kind == valueString {
		return s.stringOffsetSet(f, in, slot)
	}
	if err := s.writable(slot, false); err != nil || s.stopped() {
		return err
	}
	flags := bytecode.ArrayWriteFlags(in.aux)
	var k value
	if flags&bytecode.KeyLocation == 0 {
		k = operandValue(f.regs, in.b)
	} else if k, _, err = s.readVar(f, in.b); err != nil || s.stopped() {
		return err
	}
	if flags&bytecode.ValueLocation == 0 {
		v := operandValue(f.regs, in.c)
		if err := s.h.setElement(slot, k, v); err != nil {
			return err
		}
		return s.arraySetResult(f, in, v)
	}
	// The value is taken now but its warning waits for the key conversion,
	// where zend_fetch_dimension_address_inner_W ends and OP_DATA is read.
	// Holding it while the element is created separates an array written
	// into its own element, as storeElement does.
	vs, err := s.slot(f, in.c)
	if err != nil {
		return err
	}
	v := vs.deref()
	undefined := v.kind == valueUndefined
	if undefined {
		v = nullValue()
	}
	if err := s.h.runtimeCheck(); err != nil {
		return err
	}
	if err := s.h.claimValues(*slot, k, v); err != nil {
		return err
	}
	if err := ownArrayLocation(slot); err != nil {
		return err
	}
	retain(v)
	e, err := s.h.element(slot, k)
	if err == nil && undefined {
		s.h.warn(levelWarning, "Undefined variable $"+s.localName(f, in.c))
	}
	if err == nil && !s.stopped() {
		err = s.h.assign(e, v)
	}
	if err == nil && !s.stopped() {
		err = s.arraySetResult(f, in, v)
	}
	if rerr := s.h.release(v); err == nil {
		err = rerr
	}
	return err
}

// arrayAppend is ZEND_ASSIGN_DIM without a key: the container check, then
// the OP_DATA value, then the insertion, which fails when the next index is
// taken.
func (s *vmState) arrayAppend(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	if err := s.writable(slot, true); err != nil || s.stopped() {
		return err
	}
	var v value
	if bytecode.ArrayWriteFlags(in.aux)&bytecode.ValueLocation == 0 {
		v = operandValue(f.regs, in.b)
	} else if v, _, err = s.readVar(f, in.b); err != nil || s.stopped() {
		return err
	}
	return nextElementError(s.h.appendValue(slot, v))
}

// nextElementError turns a full next index into PHP's Error.
func nextElementError(err error) error {
	if err == ErrNextElementOccupied {
		return &Throwable{"Error", err.Error()}
	}
	return err
}

// arrayAdd is ZEND_INIT_ARRAY / ZEND_ADD_ARRAY_ELEMENT: one element of an
// array literal added in place to the array register A, whose value was read
// before the key. Keys follow ADD_ARRAY_ELEMENT rather than ASSIGN_DIM: an
// undefined variable (bytecode.AddKeyLocation) warns and is the key "" with
// no null-offset deprecation; null, floats, bools, numeric strings, arrays
// and objects convert as zend_hash keys do (arrayKey).
func (s *vmState) arrayAdd(f *vmFrame, in *vmInstr) error {
	arr := &f.regs[in.a]
	if arr.kind != valueArray {
		// Compiled code adds only to the NewArray of its literal; the
		// verifier does not track value types, so other images can get here.
		return s.unsupported("adding an array literal element to a non-array value")
	}
	v := operandValue(f.regs, in.b)
	flags := bytecode.ArrayAddFlags(in.aux)
	if flags&bytecode.AddKey == 0 {
		return nextElementError(s.h.appendValue(arr, v))
	}
	if flags&bytecode.AddKeyLocation == 0 {
		return s.h.setElement(arr, operandValue(f.regs, in.c), v)
	}
	k, undefined, err := s.readVar(f, in.c)
	if err != nil || s.stopped() {
		return err
	}
	if undefined {
		k = stringValue("")
	}
	return s.h.setElement(arr, k, v)
}

// arrayUnset is ZEND_UNSET_DIM.
func (s *vmState) arrayUnset(f *vmFrame, in *vmInstr) error {
	slot, err := s.slot(f, in.a)
	if err != nil {
		return err
	}
	// ZEND_UNSET_DIM reports an undefined container, then reads the key
	// (bytecode.KeyLocation), then applies the container's rule. An
	// undefined key reads as null, which unset treats as "" silently.
	c := slot.deref()
	if c.kind == valueUndefined {
		// An element location is never undefined: FETCH_DIM_UNSET reported
		// its variable already.
		if name, ok := s.variableName(f, in.a); ok {
			s.h.warn(levelWarning, "Undefined variable $"+name)
			if s.stopped() {
				return nil
			}
		}
	}
	var k value
	if bytecode.ArrayWriteFlags(in.aux)&bytecode.KeyLocation == 0 {
		k = operandValue(f.regs, in.b)
	} else if k, _, err = s.readVar(f, in.b); err != nil || s.stopped() {
		return err
	}
	switch c.kind {
	case valueArray:
		return s.h.unsetElement(slot, k)
	case valueUndefined, valueNull:
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

// arrayIsset is ZEND_ISSET_ISEMPTY_DIM_OBJ: isset($a[k]) and, with Aux
// bytecode.ArrayIssetEmpty, empty($a[k]). An array element is set when it
// exists and is not null, empty when it is missing or false as a bool. A
// string container is tested by position (zend_isset_dim_slow,
// zend_isempty_dim_slow: an offset in range is set, and empty only when
// the byte is "0"). Any other container is not set and is empty.
func (s *vmState) arrayIsset(regs []value, in *vmInstr) error {
	c, k := regs[in.b].deref(), regs[in.c].deref()
	empty := in.aux == bytecode.ArrayIssetEmpty
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
			if empty {
				result = !s.h.toBool(v)
			} else {
				result = v.kind != valueNull && v.kind != valueUndefined
			}
		} else {
			result = empty
		}
	case valueString:
		pos, ok, err := s.stringOffsetPosition(c.text, k)
		if err != nil || s.stopped() {
			return err
		}
		if empty {
			result = !ok || c.text[pos] == '0'
		} else {
			result = ok
		}
	case valueObject, valueResource:
		return s.unsupported("isset or empty on an offset of an object or resource")
	default:
		result = empty
	}
	return s.setReg(regs, in.a, boolValue(result))
}

// stringOffsetPosition converts the offset of isset or empty on a string
// (zend_isset_dim_slow, zend_isempty_dim_slow): integer offsets, simple
// scalars and integer numeric strings are positions; ok reports that the
// position is within str.
func (s *vmState) stringOffsetPosition(str string, k value) (pos int64, ok bool, err error) {
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
			return 0, false, nil
		}
		pos = n.integer
	default:
		return 0, false, nil
	}
	if pos < 0 {
		pos += int64(len(str))
	}
	return pos, pos >= 0 && pos < int64(len(str)), nil
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
		if name, ok := s.variableName(f, in.b); ok {
			s.h.warn(levelWarning, "Undefined variable $"+name)
			if s.stopped() {
				return nil
			}
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
