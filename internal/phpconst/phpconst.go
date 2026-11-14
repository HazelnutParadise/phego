// Package phpconst holds the constants PHP 8.5.12 defines under php -n, per
// guest integer width, as the pinned reference images report them. The
// compiler folds them where PHP folds them (zend_try_ct_eval_const) and the
// VM resolves the rest at run time. The tables are generated and read-only;
// a run's own constants live in its execution.
//
// REQ-001, REQ-016, REQ-025, REQ-026.
//
// Regenerate with Docker from this directory (never run by tests). The
// 64-bit table comes from the arm64 image; the amd64 image of the same
// release defines the same names with the same values except CHAR_MAX and
// PHP_BUILD_DATE, which the generator records as platform-dependent
// (checked on 2026-11-14 by running the generator against
// php@sha256:35fd7ee3ab5af1ed1f23f6bb01a3464c10ad0bb813a020b29b770115b852d06f
// and diffing the output).
//
//go:generate sh -c "docker run --rm --network none --cap-drop ALL -v \"$PWD/gen\":/g:ro php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 php -n /g/constants.php > constants_64_gen.go && gofmt -l constants_64_gen.go"
//go:generate sh -c "docker run --rm --network none --cap-drop ALL --platform linux/386 -v \"$PWD/gen\":/g:ro i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049 php -n /g/constants.php > constants_32_gen.go && gofmt -l constants_32_gen.go"
package phpconst

import (
	"sort"
	"strings"
)

// Kind is the type of a predefined constant's value.
type Kind uint8

const (
	Null Kind = iota + 1
	Bool
	Int
	Float
	String
	// Resource names STDIN, STDOUT and STDERR, whose values are stream
	// resources the table cannot hold.
	Resource
	// Platform names a constant whose value differs between the arm64 and
	// amd64 images of the same width; the profile does not say which.
	Platform
)

// Constant is one predefined constant. Name is the key form: the namespace
// part lower-cased, the constant's own name as written, which is how
// zend_register_constant stores it and how lookups compare it.
type Constant struct {
	Name  string
	Kind  Kind
	Bool  bool
	Int   int64
	Float float64
	Str   string
	// Deprecated is the E_DEPRECATED message a use of the constant raises,
	// or empty. PHP does not fold a deprecated constant at compile time.
	Deprecated string
}

// Key returns the lookup form of a constant name: the namespace part
// lower-cased, the unqualified name unchanged. A leading backslash must
// already be stripped.
func Key(name string) string {
	i := strings.LastIndexByte(name, '\\')
	if i < 0 {
		return name
	}
	return strings.ToLower(name[:i]) + name[i:]
}

// Short returns the unqualified part of a key or name: what an unqualified
// constant name in a namespace falls back to.
func Short(name string) string {
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// Lookup finds the constant whose key form is key for the given integer
// width (32 or 64). true, false and null are not in the table: PHP
// substitutes them while compiling.
func Lookup(width int, key string) (Constant, bool) {
	table := table64
	if width == 32 {
		table = table32
	}
	i := sort.Search(len(table), func(i int) bool { return table[i].Name >= key })
	if i < len(table) && table[i].Name == key {
		return table[i], true
	}
	return Constant{}, false
}

// Count is the number of constants in the table of a width.
func Count(width int) int {
	if width == 32 {
		return len(table32)
	}
	return len(table64)
}
