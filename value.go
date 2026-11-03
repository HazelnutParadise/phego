package phego

// Value is an immutable Go-owned PHP value. Its zero value is PHP null.
// Byte strings and ordered arrays are copied at the boundary.
type Value struct {
	kind     valueKind
	boolean  bool
	integer  int64
	floating float64
	text     string
	entries  []Entry
}

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindInt
	kindFloat
	kindBytes
	kindArray
)

// Key is an integer or byte-string array key.
type Key struct {
	integer  int64
	text     string
	isString bool
}

// Entry is one element of an ordered PHP array value.
type Entry struct {
	Key   Key
	Value Value
}

// IntKey constructs an integer array key.
func IntKey(v int64) Key { return Key{integer: v} }

// StringKey constructs a byte-string array key.
func StringKey(v string) Key { return Key{text: v, isString: true} }

// Int64 returns the integer key and whether the key is an integer.
func (k Key) Int64() (int64, bool) { return k.integer, !k.isString }

// String returns the byte-string key and whether the key is a string.
func (k Key) String() (string, bool) { return k.text, k.isString }

// Null constructs PHP null.
func Null() Value { return Value{} }

// Bool constructs a PHP boolean.
func Bool(v bool) Value { return Value{kind: kindBool, boolean: v} }

// Int64 constructs an integer; the selected PHP platform validates its range
// when the value is supplied to an execution.
func Int64(v int64) Value { return Value{kind: kindInt, integer: v} }

// Float64 constructs a PHP floating-point value.
func Float64(v float64) Value { return Value{kind: kindFloat, floating: v} }

// Bytes constructs a PHP byte string and copies its input. The caller must not
// modify v concurrently during the call; after it returns, v can be changed.
func Bytes(v []byte) Value { return Value{kind: kindBytes, text: string(v)} }

// Array constructs an ordered array input and copies its entries. The caller
// must not modify entries concurrently during the call; after it returns,
// entries can be changed. Key conversion and duplicate-key handling belong
// to the selected PHP profile.
func Array(entries []Entry) Value {
	return Value{kind: kindArray, entries: append([]Entry(nil), entries...)}
}

// IsNull reports whether the value is PHP null.
func (v Value) IsNull() bool { return v.kind == kindNull }

// Bool returns the boolean value when its kind is boolean.
func (v Value) Bool() (bool, bool) { return v.boolean, v.kind == kindBool }

// Int64 returns the integer value when its kind is integer.
func (v Value) Int64() (int64, bool) { return v.integer, v.kind == kindInt }

// Float64 returns the floating-point value when its kind is float.
func (v Value) Float64() (float64, bool) { return v.floating, v.kind == kindFloat }

// Bytes returns a fresh copy when the value is a byte string.
func (v Value) Bytes() ([]byte, bool) {
	if v.kind != kindBytes {
		return nil, false
	}
	return []byte(v.text), true
}

// Entries returns a fresh slice in insertion order when the value is an array.
func (v Value) Entries() ([]Entry, bool) {
	if v.kind != kindArray {
		return nil, false
	}
	return append([]Entry(nil), v.entries...), true
}
