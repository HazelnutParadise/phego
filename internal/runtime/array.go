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

// phpArray is an insertion-ordered PHP array. Packed bucket i has integer key
// i (or is a hole); mixed keys use an ordered slice and lookup index. Stable
// bucket pointers keep element locations valid across growth and compaction.
type phpArray struct {
	refcount int
	buckets  []*bucket // nil entries are deleted slots
	index    map[arrayKey]int
	live     int
	nextFree int64 // math.MinInt64 until an integer key is inserted
	// cursors are active foreach positions; a tail deletion clamps them so an
	// element appended into the freed slot is still visited, as in PHP.
	// REQ-016/017; T-031/032/034. Each iterator tracks a position in every
	// separated copy, since the loop may later select any of those arrays.
	cursors map[*arrayIterator]int
	pointer int // internal pointer; independent of foreach positions
	// PHP 8.5.12's logical position space, independent of Go slice capacity.
	// Growth and compaction can affect observable exhausted internal pointers.
	// Zero is Go's uninitialized-state marker, not PHP's raw table size.
	limit int
}

type arrayIterator struct{ arrays map[*phpArray]struct{} }

func newArray() *phpArray { return &phpArray{nextFree: math.MinInt64} }

func (a *phpArray) packed() bool { return a.index == nil }
func (a *phpArray) count() int   { return a.live }

// normalizeKey handles keys without operation-specific diagnostics. Runtime
// element operations use heap.arrayKey for floats, null diagnostics and errors.
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
			return int(k.integer), a.buckets[int(k.integer)] != nil
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
	a.index = make(map[arrayKey]int, a.live)
	a.compact()
}

// compact remaps positions during representation conversion/rehash. Unlike
// COW copying, it maps an iterator at the old end to the new end. An exhausted
// internal pointer keeps its raw position until insertion reaches it again.
// REQ-016/017; T-031/032/033/034.
func (a *phpArray) compact() {
	used := len(a.buckets)
	if a.live == 0 {
		a.buckets = nil
		a.pointer = 0
		for it := range a.cursors {
			a.cursors[it] = 0
		}
		clear(a.index)
		return
	}
	positions := make(map[int]int, len(a.cursors)+1)
	if a.pointer < used {
		positions[a.pointer] = 0
	}
	for _, p := range a.cursors {
		if p <= used {
			positions[p] = 0
		}
	}
	rank := 0
	for i, b := range a.buckets {
		if _, ok := positions[i]; ok {
			positions[i] = rank
		}
		if b != nil {
			rank++
		}
	}
	positions[used] = rank
	if a.pointer < used {
		a.pointer = positions[a.pointer]
	}
	for it, p := range a.cursors {
		if p <= used {
			a.cursors[it] = positions[p]
		}
	}
	write := 0
	clear(a.index)
	for _, b := range a.buckets {
		if b == nil {
			continue
		}
		a.buckets[write] = b
		a.index[b.key] = write
		write++
	}
	clear(a.buckets[write:])
	a.buckets = a.buckets[:write]
}

func (a *phpArray) growLimit() bool {
	if a.limit < 8 {
		a.limit = 8
	}
	if a.limit > int(^uint(0)>>1)/2 {
		return false
	}
	a.limit *= 2
	return true
}

// insert adds a new key; callers have checked it is absent. The value is
// stored as given, so ownership transfers to the array.
func (a *phpArray) insert(k arrayKey, v value, width int) *bucket {
	if a.limit < 8 {
		a.limit = 8
	}
	if a.packed() {
		if k.isString || (k.integer >= 0 && k.integer < int64(len(a.buckets))) {
			a.toIndexed()
		} else if k.integer < 0 || k.integer >= int64(a.limit) {
			if k.integer >= 0 && k.integer/2 < int64(a.limit) && a.live > a.limit/2 && a.growLimit() {
				// Dense growth keeps the packed position space.
			} else {
				// REQ-017; T-033/034: full numeric conversion grows the
				// position range before compacting, including negative keys.
				// Sort and string-key conversion keep the existing range.
				if len(a.buckets) >= a.limit {
					a.growLimit()
				}
				a.toIndexed()
			}
		}
	}
	b := &bucket{key: k, val: v}
	if a.packed() {
		oldUsed := len(a.buckets)
		for len(a.buckets) <= int(k.integer) {
			a.buckets = append(a.buckets, nil)
		}
		a.buckets[int(k.integer)] = b
		if a.pointer == oldUsed {
			a.pointer = int(k.integer)
		}
	} else {
		if len(a.buckets) >= a.limit {
			if len(a.buckets)-a.live <= a.live>>5 {
				a.growLimit()
			}
			a.compact()
		}
		a.index[k] = len(a.buckets)
		a.buckets = append(a.buckets, b)
	}
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
	old := a.buckets[i].val
	oldUsed := len(a.buckets)
	a.buckets[i] = nil
	if !a.packed() {
		delete(a.index, k)
	}
	for len(a.buckets) > 0 && a.buckets[len(a.buckets)-1] == nil {
		a.buckets = a.buckets[:len(a.buckets)-1]
	}
	for c, pos := range a.cursors {
		a.cursors[c] = min(pos, len(a.buckets))
	}
	// REQ-017; T-034: keep an invalid pointer at the new end after trimming.
	// The next append can make that position valid again.
	if len(a.buckets) < oldUsed {
		a.pointer = min(a.pointer, len(a.buckets))
	}
	a.live--
	return old, true
}

// next advances an iteration position over live buckets in insertion order.
// It visits live entries at or after the stored position, including later appends.
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
