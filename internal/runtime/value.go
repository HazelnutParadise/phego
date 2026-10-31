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
	refcount int
	target   value
}

func nullValue() value           { return value{kind: valueNull} }
func boolValue(v bool) value     { return value{kind: valueBool, boolean: v} }
func floatValue(v float64) value { return value{kind: valueFloat, floating: v} }

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

// identical implements PHP === for the kinds modeled here. Comparing distinct
// arrays that re-enter an array already being compared returns
// ErrRecursiveDependency instead of recursing without bound.
func identical(a, b value) (bool, error) {
	return identicalIn(a, b, map[*phpArray]bool{})
}

func identicalIn(a, b value, active map[*phpArray]bool) (bool, error) {
	a, b = a.deref(), b.deref()
	if a.kind != b.kind {
		return false, nil
	}
	switch a.kind {
	case valueUndefined, valueNull:
		return true, nil
	case valueBool:
		return a.boolean == b.boolean, nil
	case valueInt:
		return a.integer == b.integer, nil
	case valueFloat:
		return a.floating == b.floating, nil
	case valueString:
		return a.text == b.text, nil
	case valueObject:
		return a.object == b.object, nil
	case valueArray:
		if a.array == b.array {
			return true, nil
		}
		if a.array.count() != b.array.count() {
			return false, nil
		}
		if active[a.array] || active[b.array] {
			return false, ErrRecursiveDependency
		}
		active[a.array], active[b.array] = true, true
		defer delete(active, a.array)
		defer delete(active, b.array)
		ai, bi := 0, 0
		for {
			ab, aok := a.array.next(&ai)
			bb, bok := b.array.next(&bi)
			if !aok || !bok {
				return aok == bok, nil
			}
			if ab.key != bb.key {
				return false, nil
			}
			if same, err := identicalIn(ab.val, bb.val, active); err != nil || !same {
				return false, err
			}
		}
	}
	return false, nil
}
