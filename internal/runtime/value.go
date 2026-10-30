package runtime

import (
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

// identical implements PHP === for the kinds modeled here.
func identical(a, b value) bool {
	a, b = a.deref(), b.deref()
	if a.kind != b.kind {
		return false
	}
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
	case valueObject:
		return a.object == b.object
	case valueArray:
		if a.array == b.array {
			return true
		}
		if a.array.count() != b.array.count() {
			return false
		}
		ai, bi := 0, 0
		for {
			ab, aok := a.array.next(&ai)
			bb, bok := b.array.next(&bi)
			if !aok || !bok {
				return aok == bok
			}
			if ab.key != bb.key || !identical(ab.val, bb.val) {
				return false
			}
		}
	}
	return false
}
