package runtime

import (
	"errors"
	"fmt"
	"math"
)

// valueKind keeps every PHP-visible type distinct. Undefined is a missing
// variable or slot, not null.
type valueKind uint8

const (
	valueUndefined valueKind = iota
	valueNull
	valueBool
	valueInt
	valueFloat
	valueString
	valueArray
	valueObject
	valueReference
	valueResource
)

// value is the candidate PHP value cell. Composite kinds point at shared
// request-local storage whose logical lifetime is counted by the heap; Go GC
// only reclaims memory after the heap has released the last logical owner.
type value struct {
	kind     valueKind
	boolean  bool
	integer  int64
	floating float64
	text     string
	array    *phpArray
	object   *object
	ref      *reference
}

// reference is a shared PHP location. Every variable, element, or property
// bound by & holds one count.
type reference struct {
	owner    *heap
	refcount int
	target   value
}

func nullValue() value           { return value{kind: valueNull} }
func boolValue(v bool) value     { return value{kind: valueBool, boolean: v} }
func floatValue(v float64) value { return value{kind: valueFloat, floating: v} }

func (v value) isExplicitlyUnset() bool {
	return v.kind == valueUndefined && v.boolean
}

func explicitlyUnsetValue() value {
	return value{kind: valueUndefined, boolean: true}
}

// intValue checks representability using the selected PHP platform width.
func intValue(v int64, width int) (value, error) {
	switch width {
	case 32:
		if v < math.MinInt32 || v > math.MaxInt32 {
			return value{}, fmt.Errorf("integer %d is outside the signed 32-bit PHP integer range", v)
		}
	case 64:
	default:
		return value{}, fmt.Errorf("invalid PHP integer width %d: expected 32 or 64 bits", width)
	}
	return value{kind: valueInt, integer: v}, nil
}

// bytesValue copies caller bytes; PHP strings are immutable byte sequences, so
// later caller mutation cannot alter stored data.
func bytesValue(v []byte) value { return value{kind: valueString, text: string(v)} }

// stringValue wraps an immutable Go string as a PHP byte string.
func stringValue(s string) value { return value{kind: valueString, text: s} }

func (v value) bytesCopy() []byte { return []byte(v.text) }

// deref returns the value seen through a reference.
func (v value) deref() value {
	if v.kind == valueReference {
		return v.ref.target
	}
	return v
}

// ErrRecursiveDependency mirrors PHP 8.5.12's Error when comparing arrays
// that contain themselves.
var ErrRecursiveDependency = errors.New("Nesting level too deep - recursive dependency?")

// identical implements PHP === for the kinds modeled here. As in PHP 8.5.12,
// only the left operand's arrays are guarded: re-entering a left array that is
// already being compared returns ErrRecursiveDependency instead of recursing
// without bound, while shared but acyclic storage compares normally.
func identical(a, b value) (bool, error) {
	return identicalIn(a, b, map[*phpArray]bool{})
}

func identicalIn(a, b value, active map[*phpArray]bool) (bool, error) {
	a, b = a.deref(), b.deref()
	if a.kind != b.kind {
		return false, nil
	}
	if a.kind == valueArray {
		return identicalArrays(a.array, b.array, active)
	}
	return identicalScalars(a, b), nil
}

// identicalScalars is === for two values of the same kind other than array.
func identicalScalars(a, b value) bool {
	switch a.kind {
	case valueUndefined, valueNull:
		return true
	case valueBool:
		return a.boolean == b.boolean
	case valueInt:
		return a.integer == b.integer
	case valueFloat:
		return a.floating == b.floating
	case valueString:
		return a.text == b.text
	case valueObject, valueResource:
		return a.object == b.object
	}
	return false
}

// MaxComparisonNesting is the array nesting at which comparison and ===
// throw PHP's Error "Maximum call stack size reached during comparison".
// PHP 8.5.12 compares nested arrays recursively and throws that Error when
// the C stack runs low: with php -n and an 8 MiB stack on linux/arm64, ==, <
// and <=> threw between 30,000 and 40,000 levels and === between 60,000 and
// 70,000. The depth depends on PHP's build and platform, so Phego throws at
// one fixed depth above those, which every program PHP compares completes
// within. Phego compares with a work list, not Go recursion, so no depth can
// overflow the goroutine stack.
const MaxComparisonNesting = 100_000

// errComparisonNesting is the Error past MaxComparisonNesting.
func errComparisonNesting() error {
	return &Throwable{"Error", "Maximum call stack size reached during comparison"}
}

// identicalArrays compares a and b in order, element by element, descending
// into nested array pairs with a work list.
func identicalArrays(a, b *phpArray, active map[*phpArray]bool) (bool, error) {
	if active == nil {
		active = map[*phpArray]bool{}
	}
	type pair struct {
		a, b   *phpArray
		ai, bi int
	}
	var stack []pair
	defer func() {
		for _, p := range stack {
			delete(active, p.a)
		}
	}()
	// enter decides a pair without descending, or pushes it.
	enter := func(a, b *phpArray) (same, decided bool, err error) {
		switch {
		case a == b:
			return true, true, nil
		case a.count() != b.count():
			return false, true, nil
		case active[a]:
			return false, true, ErrRecursiveDependency
		case len(active) >= MaxComparisonNesting:
			return false, true, errComparisonNesting()
		}
		active[a] = true
		stack = append(stack, pair{a: a, b: b})
		return false, false, nil
	}
	if same, decided, err := enter(a, b); decided {
		return same, err
	}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		ab, aok := top.a.next(&top.ai)
		bb, bok := top.b.next(&top.bi)
		if !aok || !bok {
			if aok != bok {
				return false, nil
			}
			delete(active, top.a)
			stack = stack[:len(stack)-1]
			continue
		}
		if ab.key != bb.key {
			return false, nil
		}
		x, y := ab.val.deref(), bb.val.deref()
		if x.kind != y.kind {
			return false, nil
		}
		if x.kind != valueArray {
			if !identicalScalars(x, y) {
				return false, nil
			}
			continue
		}
		if same, decided, err := enter(x.array, y.array); decided && (err != nil || !same) {
			return false, err
		}
	}
	return true, nil
}
