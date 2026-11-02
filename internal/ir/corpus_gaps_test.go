package ir

import (
	"regexp"
	"strings"
)

// knownGapCount is the number of PHP 8.5.12 corpus compile errors and
// files with compile warnings lowering does not reproduce yet. Each one belongs to a later work package; gapPrefix
// names which, and TestLowerMatchesCompileReference fails when the count
// changes in either direction.
const knownGapCount = 328

// objectModelGap matches errors PHP raises while compiling class members,
// inheritance, enums, property hooks, magic methods and attributes, which
// the object model (W-13) owns.
var objectModelGap = regexp.MustCompile(`Declaration of .* must be compatible|#\[|Attribute "|attribute|[Hh]ook|Method .*::__|__[a-zA-Z_]+\(\)|Propert|propert|[Ee]num|Class .* (contains|declares|cannot|constant)|[Rr]eadonly|[Aa]bstract|Access level|override|Override|Interface method|Cannot redeclare .*::|Visibility|Set access|Cannot apply|Case |trait|Type of |as value for class constant|cannot extend|final|Private constant|class constant|Class declarations may not be nested|Anonymous class method|non-backed|virtual`)

// fileGaps are errors whose message is shared with checks lowering does
// make, so they are classified by corpus file.
var fileGaps = map[string]string{
	// Attribute arguments are compiled by the object model (W-13).
	"650f8de6da8b389b.php": "attributes (W-13)",
	"73f08781589236b3.php": "attributes (W-13)",
	"8f8b8419b2b89d05.php": "attributes (W-13)",
	"a5a531412fc5f718.php": "attributes (W-13)",
	"aca48ce851236dda.php": "attributes (W-13)",
	// These need the built-in function table (W-29): PHP compiles strlen()
	// specially and knows strlen is already declared.
	"0f751b921ccaaa2f.php": "built-in functions (W-29)",
	"8394ddee66cb947e.php": "built-in functions (W-29)",
	"84e338ba0be450ac.php": "built-in functions (W-29)",
	"57077f922869ea78.php": "built-in functions (W-29)",
	// PHP knows test() takes $arr by value because it was declared earlier
	// in the file; lowering does not resolve callees at compile time.
	"ff47421ad4f3220e.php": "compile-time function binding (W-29)",
	// (1+1)::class needs compile-time constant folding of operators.
	"77a22b80ed8f0f34.php": "constant folding (W-15)",
}

// gapPrefix returns the work package owning an unraised compile error, or
// "" when lowering is expected to raise it.
func gapPrefix(file, msg string) string {
	if g, ok := fileGaps[file]; ok {
		return g
	}
	if objectModelGap.MatchString(msg) {
		return "object model (W-13)"
	}
	return ""
}

// diagnosticGap returns the work package owning a file's compile warnings
// when lowering does not reproduce them yet.
func diagnosticGap(file string, want []string) string {
	for _, w := range want {
		switch {
		case objectModelDiagnostic.MatchString(w):
			return "object model (W-13)"
		case strings.Contains(w, "Array to string conversion"):
			// PHP folds a constant array concatenation while compiling.
			return "constant folding (W-15)"
		}
	}
	return ""
}

// objectModelDiagnostic matches compile warnings about methods and
// inheritance, which the object model (W-13) raises.
var objectModelDiagnostic = regexp.MustCompile(`The magic method .* must have public visibility|Return type of .* should either be compatible|Private methods cannot be final`)
