package ir

import (
	"strings"

	"github.com/HazelnutParadise/phego/internal/phpname"
)

// namespace holds the import tables of the namespace being compiled. PHP
// resets them at each namespace declaration.
type namespace struct {
	name      string            // without leading or trailing backslash
	classes   map[string]string // lower-case alias -> fully qualified name
	functions map[string]string // lower-case alias -> fully qualified name
	constants map[string]string // alias (case-sensitive) -> fully qualified name
}

func newNamespace(name string) *namespace {
	return &namespace{name: name, classes: map[string]string{}, functions: map[string]string{}, constants: map[string]string{}}
}

// prefix joins the current namespace and a relative name.
func (ns *namespace) prefix(name string) string {
	if ns.name == "" {
		return name
	}
	return ns.name + "\\" + name
}

// nameForm classifies a name as written in source.
type nameForm uint8

const (
	unqualified nameForm = iota
	qualified
	fullyQualified
	relative // namespace\Name
)

func formOf(text string) (nameForm, string) {
	switch {
	case strings.HasPrefix(text, "\\"):
		return fullyQualified, text[1:]
	case len(text) > 10 && phpname.EqualFold(text[:10], "namespace\\"):
		return relative, text[10:]
	case strings.Contains(text, "\\"):
		return qualified, text
	}
	return unqualified, text
}

// specialClass returns the lower-case special class name, if any.
func specialClass(name string) string {
	switch l := phpname.Lower(name); l {
	case "self", "parent", "static":
		return l
	}
	return ""
}

// resolveClass resolves a class reference as PHP's zend_resolve_class_name.
func (ns *namespace) resolveClass(text string) *Name {
	form, name := formOf(text)
	switch form {
	case fullyQualified:
		return &Name{Kind: ClassName, Resolved: name}
	case relative:
		return &Name{Kind: ClassName, Resolved: ns.prefix(name)}
	case unqualified:
		if s := specialClass(name); s != "" {
			return &Name{Kind: ClassName, Resolved: s, Special: s}
		}
		if full, ok := ns.classes[phpname.Lower(name)]; ok {
			return &Name{Kind: ClassName, Resolved: full}
		}
		return &Name{Kind: ClassName, Resolved: ns.prefix(name)}
	}
	return &Name{Kind: ClassName, Resolved: ns.qualified(name)}
}

// qualified resolves a qualified name through the class import table of its
// first segment, which PHP applies to every symbol kind.
func (ns *namespace) qualified(name string) string {
	first, rest, _ := strings.Cut(name, "\\")
	if full, ok := ns.classes[phpname.Lower(first)]; ok {
		return full + "\\" + rest
	}
	return ns.prefix(name)
}

// resolveFunction resolves a function name. An unqualified name in a
// namespace falls back to the global function at run time.
func (ns *namespace) resolveFunction(text string) *Name {
	return ns.resolveSymbol(FunctionName, text, func(n string) (string, bool) {
		full, ok := ns.functions[phpname.Lower(n)]
		return full, ok
	})
}

// resolveConstant resolves a constant name; true, false and null are
// always the built-in constants.
func (ns *namespace) resolveConstant(text string) *Name {
	form, name := formOf(text)
	if form == unqualified && isBuiltinConstant(name) {
		return &Name{Kind: ConstantName, Resolved: phpname.Lower(name)}
	}
	return ns.resolveSymbol(ConstantName, text, func(n string) (string, bool) {
		full, ok := ns.constants[n]
		return full, ok
	})
}

func (ns *namespace) resolveSymbol(kind NameKind, text string, imported func(string) (string, bool)) *Name {
	form, name := formOf(text)
	switch form {
	case fullyQualified:
		return &Name{Kind: kind, Resolved: name}
	case relative:
		return &Name{Kind: kind, Resolved: ns.prefix(name)}
	case qualified:
		return &Name{Kind: kind, Resolved: ns.qualified(name)}
	}
	if full, ok := imported(name); ok {
		return &Name{Kind: kind, Resolved: full}
	}
	if ns.name == "" {
		return &Name{Kind: kind, Resolved: name}
	}
	return &Name{Kind: kind, Resolved: ns.prefix(name), Fallback: name}
}

// reservedClassNames are names a class or import alias cannot take.
var reservedClassNames = map[string]bool{
	"bool": true, "false": true, "float": true, "int": true, "null": true, "parent": true,
	"self": true, "static": true, "string": true, "true": true, "void": true, "never": true,
	"iterable": true, "object": true, "mixed": true, "array": true, "callable": true,
}

// isReservedClassName mirrors zend_is_reserved_class_name, which checks the
// last segment of a name.
func isReservedClassName(name string) bool {
	return reservedClassNames[phpname.Lower(lastSegment(name))]
}

func lastSegment(s string) string {
	if i := strings.LastIndexByte(s, '\\'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// builtinTypes are the type names that are not class references.
var builtinTypes = map[string]bool{
	"array": true, "callable": true, "bool": true, "false": true, "true": true, "float": true,
	"int": true, "null": true, "string": true, "void": true, "never": true, "iterable": true,
	"object": true, "mixed": true, "static": true,
}
