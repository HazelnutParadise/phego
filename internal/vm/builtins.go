package vm

import (
	"sort"

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
