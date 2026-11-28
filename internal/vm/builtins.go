package vm

import (
	"sort"

	"github.com/HazelnutParadise/phego/internal/ir"

	"github.com/HazelnutParadise/phego/internal/phpname"
)

// builtinFunctions (builtins_gen.go) lists PHP 8.5.12's internal functions
// under php -n in the pinned reference image, lower-cased and sorted. Stage
// 1 has no builtin functions, so a call that resolves to one is reported at
// compile time instead of failing as an undefined function at run time.
//
// Regenerate with Docker from this directory (never run by tests):
//
//go:generate sh -c "docker run --rm --network none --cap-drop ALL -v \"$PWD/gen\":/g:ro php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 php -n /g/builtins.php > builtins_gen.go && gofmt -l builtins_gen.go"

// isBuiltin reports whether name, in any case, is a PHP internal function.
func isBuiltin(name string) bool {
	n := phpname.Lower(name)
	i := sort.SearchStrings(builtinFunctions, n)
	return i < len(builtinFunctions) && builtinFunctions[i] == n
}

// builtinClasses (classes_gen.go) lists the classes, interfaces and traits
// PHP 8.5.12 declares under php -n, lower-cased and sorted. Only a few of
// them exist in Phego yet (supportedBuiltinClasses); naming any other is
// reported at compile time instead of failing as an undefined class at run
// time.
//
//go:generate sh -c "docker run --rm --network none --cap-drop ALL -v \"$PWD/gen\":/g:ro php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 php -n /g/classes.php > classes_gen.go && gofmt -l classes_gen.go"

// isBuiltinClass reports whether name, in any case, is a class-like PHP
// declares itself.
func isBuiltinClass(name string) bool {
	n := phpname.Lower(name)
	i := sort.SearchStrings(builtinClasses, n)
	return i < len(builtinClasses) && builtinClasses[i] == n
}

// supportedBuiltinClasses are the builtin classes the runtime provides.
var supportedBuiltinClasses = func() map[string]bool {
	out := map[string]bool{"stdclass": true, "stringable": true, "closure": true, "throwable": true,
		"traversable": true, "iterator": true, "iteratoraggregate": true, "generator": true}
	for name := range ir.BuiltinThrowables {
		out[name] = true
	}
	return out
}()

// IsBuiltinFunction reports whether lower, a lower-case function name
// without a leading backslash, is a function PHP 8.5.12 declares under
// php -n. The run time uses it to refuse a callable that names one.
func IsBuiltinFunction(lower string) bool { return isBuiltin(lower) }
