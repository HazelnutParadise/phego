// Code generated from internal/runtime/throwables_gen.go by go generate; DO NOT EDIT.

package ir

// BuiltinThrowables are the built-in classes that implement Throwable, by
// lower-cased name: the classes of PHP 8.5.12's Core and SPL extensions the
// runtime provides (internal/runtime/throwables_gen.go). A class that
// extends one is bound while the file is compiled, as PHP binds a class
// whose parent is an internal class.
var BuiltinThrowables = map[string]bool{
	"error":                     true,
	"exception":                 true,
	"arithmeticerror":           true,
	"closedgeneratorexception":  true,
	"compileerror":              true,
	"errorexception":            true,
	"logicexception":            true,
	"requestparsebodyexception": true,
	"runtimeexception":          true,
	"typeerror":                 true,
	"unhandledmatcherror":       true,
	"valueerror":                true,
	"argumentcounterror":        true,
	"badfunctioncallexception":  true,
	"divisionbyzeroerror":       true,
	"domainexception":           true,
	"invalidargumentexception":  true,
	"lengthexception":           true,
	"outofboundsexception":      true,
	"outofrangeexception":       true,
	"overflowexception":         true,
	"parseerror":                true,
	"rangeexception":            true,
	"underflowexception":        true,
	"unexpectedvalueexception":  true,
	"badmethodcallexception":    true,
}
