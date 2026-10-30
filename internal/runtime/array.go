package runtime

import (
	"errors"
	"fmt"
	"math"
)

// ErrNextElementOccupied mirrors PHP 8.5.12's Error for an append whose next
// integer key is already present or cannot advance.
var ErrNextElementOccupied = errors.New("Cannot add element to the array as the next element is already occupied")

// arrayKey is a normalized PHP array key: an integer or a byte string.
type arrayKey struct {
	isString bool
	integer  int64
	text     string
}

func intKey(i int64) arrayKey     { return arrayKey{integer: i} }
func stringKey(s string) arrayKey { return arrayKey{isString: true, text: s} }

type bucket struct {
	key arrayKey
	val value
}

// phpArray is an insertion-ordered PHP array. While packed, bucket i has key i
// and there are no holes, so no index is needed; any other shape converts to
// the indexed form. Buckets are pointers so a bound element keeps a stable
// address across growth.
type phpArray struct {
	refcount int
	buckets  []*bucket // nil entries are deleted slots
	index    map[arrayKey]int
	live     int
	nextFree int64 // math.MinInt64 until an integer key is inserted
}

func newArray() *phpArray { return &phpArray{nextFree: math.MinInt64} }

func (a *phpArray) packed() bool { return a.index == nil }
func (a *phpArray) count() int   { return a.live }

// normalizeKey applies PHP 8.5.12 key conversion for the platform width.
// Floats and composite keys are outside this prototype and return an error.
func normalizeKey(k value, width int) (arrayKey, error) {
	k = k.deref()
	switch k.kind {
	case valueInt:
		return intKey(k.integer), nil
	case valueBool:
		if k.boolean {
			return intKey(1), nil
		}
		return intKey(0), nil
	case valueNull:
		return stringKey(""), nil
	case valueString:
		if i, ok, err := php8512IntegerStringKey(k.text, width); err != nil {
			return arrayKey{}, err
		} else if ok {
			return intKey(i), nil
		}
		return stringKey(k.text), nil
	}
	return arrayKey{}, fmt.Errorf("array key kind %d is not modeled by this prototype", k.kind)
}

func (a *phpArray) find(k arrayKey) (int, bool) {
	if a.packed() {
		if !k.isString && k.integer >= 0 && k.integer < int64(len(a.buckets)) {
			return int(k.integer), true
		}
		return 0, false
	}
	i, ok := a.index[k]
	return i, ok
}

func (a *phpArray) lookup(k arrayKey) (*bucket, bool) {
	if i, ok := a.find(k); ok {
		return a.buckets[i], true
	}
	return nil, false
}

func (a *phpArray) toIndexed() {
	a.index = make(map[arrayKey]int, len(a.buckets))
	for i, b := range a.buckets {
		if b != nil {
			a.index[b.key] = i
		}
	}
}

// insert adds a new key; callers have checked it is absent. The value is
// stored as given, so ownership transfers to the array.
func (a *phpArray) insert(k arrayKey, v value, width int) *bucket {
	if a.packed() && (k.isString || k.integer != int64(len(a.buckets))) {
		a.toIndexed()
	}
	b := &bucket{key: k, val: v}
	if !a.packed() {
		a.index[k] = len(a.buckets)
	}
	a.buckets = append(a.buckets, b)
	a.live++
	if !k.isString && k.integer >= a.nextFree {
		if k.integer < maxInt(width) {
			a.nextFree = k.integer + 1
		} else {
			a.nextFree = k.integer
		}
	}
	return b
}

func maxInt(width int) int64 {
	if width == 32 {
		return math.MaxInt32
	}
	return math.MaxInt64
}

// appendKey returns the key PHP's $a[] would use.
func (a *phpArray) appendKey() (arrayKey, error) {
	next := a.nextFree
	if next == math.MinInt64 {
		next = 0
	}
	k := intKey(next)
	if _, exists := a.find(k); exists {
		return arrayKey{}, ErrNextElementOccupied
	}
	return k, nil
}

// remove detaches a key and returns its value for the caller to release.
// Removal never resets the next free integer key.
func (a *phpArray) remove(k arrayKey) (value, bool) {
	i, ok := a.find(k)
	if !ok {
		return value{}, false
	}
	if a.packed() && i != len(a.buckets)-1 {
		a.toIndexed()
	}
	old := a.buckets[i].val
	if a.packed() {
		a.buckets = a.buckets[:i]
	} else {
		a.buckets[i] = nil
		delete(a.index, k)
	}
	a.live--
	return old, true
}

// next advances an iteration position over live buckets in insertion order.
// Elements appended during iteration are visited, as PHP foreach by reference does.
func (a *phpArray) next(pos *int) (*bucket, bool) {
	for *pos < len(a.buckets) {
		b := a.buckets[*pos]
		*pos++
		if b != nil {
			return b, true
		}
	}
	return nil, false
}
