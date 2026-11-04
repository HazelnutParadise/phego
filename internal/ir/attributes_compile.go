package ir

import (
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// checkAttributes validates arguments and retains bounded declaration metadata,
// not executable argument graphs.
func (l *lowerer) checkAttributes(attrs *frontend.Node, target string, targetLine int) []Attribute {
	if attrs == nil {
		return nil
	}
	var out []Attribute
	counts := map[string]int{}
	for _, group := range attrs.Children {
		for _, attr := range group.Children {
			name := l.ns.resolveClass(attr.Children[0].Text)
			counts[globalAttributeName(name.Resolved)]++
			if len(attr.Children) > 1 && attr.Children[1] != nil && attr.Children[1].Kind == "callable_args" {
				l.fail(targetLine, "Cannot create Closure as attribute argument")
			}
			meta := Attribute{Name: name, Line: attr.Line}
			if len(attr.Children) > 1 && attr.Children[1] != nil {
				seenNames := map[string]bool{}
				named := false
				for _, arg := range attr.Children[1].Children {
					if arg.Kind == "named_arg" {
						named = true
						if seenNames[arg.Text] {
							l.fail(targetLine, "Duplicate named parameter $%s", arg.Text)
						}
						seenNames[arg.Text] = true
						meta.Arguments = append(meta.Arguments, arg.Text)
						l.attributeConstExpr(arg.Children[0], targetLine)
					} else {
						if named {
							l.fail(targetLine, "Cannot use positional argument after named argument")
						}
						if arg.Kind == "spread" {
							l.fail(targetLine, "Cannot use unpacking in attribute argument list")
						}
						meta.Arguments = append(meta.Arguments, "")
						l.attributeConstExpr(arg, targetLine)
					}
				}
			}
			out = append(out, meta)
		}
	}
	// PHP compiles every argument before checking targets or repetition.
	delayed := hasClassMarker(out, "DelayedTargetValidation")
	for _, attr := range out {
		short := globalAttributeName(attr.Name.Resolved)
		allowed, builtin := builtinAttributeTargets[short]
		if !builtin {
			continue
		}
		targetName := target
		if target == "property hook" {
			targetName = "method"
		}
		if target == "promoted parameter" {
			targetName = "parameter"
		}
		mask := attributeTargetBit(targetName)
		if target == "promoted parameter" {
			mask |= attributeTargetProperty
		}
		if !delayed && allowed.mask&mask == 0 {
			l.fail(targetLine, "Attribute \"%s\" cannot target %s (allowed targets: %s)", attributeDisplay(attr.Name.Resolved), targetName, allowed.labels)
		}
		if counts[short] > 1 {
			l.fail(targetLine, "Attribute \"%s\" must not be repeated", attributeDisplay(attr.Name.Resolved))
		}
		if !delayed && target == "property hook" && short == "nodiscard" {
			l.fail(targetLine, "#[\\NoDiscard] is not supported for property hooks")
		}
		if !delayed && target == "class" && l.class != nil {
			l.validateClassAttributeKinds(l.class, attr)
		}
	}
	return out
}

const (
	attributeTargetClass = 1 << iota
	attributeTargetFunction
	attributeTargetMethod
	attributeTargetProperty
	attributeTargetClassConstant
	attributeTargetParameter
	attributeTargetConstant
)

type attributeTargetInfo struct {
	mask   int
	labels string
}

var builtinAttributeTargets = map[string]attributeTargetInfo{
	"attribute":               {mask: attributeTargetClass, labels: "class"},
	"allowdynamicproperties":  {mask: attributeTargetClass, labels: "class"},
	"nodiscard":               {mask: attributeTargetFunction | attributeTargetMethod, labels: "function, method"},
	"returntypewillchange":    {mask: attributeTargetMethod, labels: "method"},
	"deprecated":              {mask: attributeTargetClass | attributeTargetFunction | attributeTargetMethod | attributeTargetClassConstant | attributeTargetConstant, labels: "class, function, method, class constant, constant"},
	"sensitiveparameter":      {mask: attributeTargetParameter, labels: "parameter"},
	"delayedtargetvalidation": {mask: 127, labels: "class, function, method, property, class constant, parameter, constant"},
	"override":                {mask: attributeTargetMethod | attributeTargetProperty, labels: "method, property"},
}

func globalAttributeName(name string) string {
	name = strings.TrimPrefix(name, "\\")
	if strings.Contains(name, "\\") {
		return ""
	}
	return strings.ToLower(name)
}

func attributeDisplay(name string) string {
	name = strings.TrimPrefix(name, "\\")
	if name == "" {
		return name
	}
	return name
}

func attributeTargetBit(target string) int {
	switch target {
	case "class":
		return attributeTargetClass
	case "function":
		return attributeTargetFunction
	case "method":
		return attributeTargetMethod
	case "property":
		return attributeTargetProperty
	case "class constant":
		return attributeTargetClassConstant
	case "parameter":
		return attributeTargetParameter
	case "constant":
		return attributeTargetConstant
	default:
		return 0
	}
}

func attributeTargetLabels(flags int) string {
	labels := []struct {
		bit  int
		name string
	}{{attributeTargetClass, "class"}, {attributeTargetFunction, "function"}, {attributeTargetMethod, "method"}, {attributeTargetProperty, "property"}, {attributeTargetClassConstant, "class constant"}, {attributeTargetParameter, "parameter"}, {attributeTargetConstant, "constant"}}
	var out []string
	for _, item := range labels {
		if flags&item.bit != 0 {
			out = append(out, item.name)
		}
	}
	return strings.Join(out, ", ")
}

func hasASTAttribute(attrs *frontend.Node, want string) bool {
	if attrs == nil {
		return false
	}
	for _, group := range attrs.Children {
		for _, attr := range group.Children {
			if len(attr.Children) > 0 && strings.EqualFold(strings.TrimPrefix(attr.Children[0].Text, "\\"), want) {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) validateClassAttributeKinds(cl *Class, attr Attribute) {
	name := globalAttributeName(attr.Name.Resolved)
	if name == "attribute" && cl.Kind != KindClass {
		kind := map[ClassKind]string{KindInterface: "interface", KindTrait: "trait", KindEnum: "enum"}[cl.Kind]
		l.fail(cl.Line, "Cannot apply #[\\Attribute] to %s %s", kind, cl.Name)
	}
	if name == "allowdynamicproperties" && cl.Kind != KindClass {
		kind := map[ClassKind]string{KindInterface: "interface", KindTrait: "trait", KindEnum: "enum"}[cl.Kind]
		l.fail(cl.Line, "Cannot apply #[\\AllowDynamicProperties] to %s %s", kind, cl.Name)
	}
	if name == "allowdynamicproperties" && hasModifier(cl.Modifiers, "readonly") {
		l.fail(cl.Line, "Cannot apply #[\\AllowDynamicProperties] to readonly class %s", cl.Name)
	}
	if name == "deprecated" && cl.Kind != KindTrait {
		kind := map[ClassKind]string{KindClass: "class", KindInterface: "interface", KindEnum: "enum"}[cl.Kind]
		l.fail(cl.Line, "Cannot apply #[\\Deprecated] to %s %s", kind, cl.Name)
	}
	if name == "attribute" && hasModifier(cl.Modifiers, "abstract") {
		l.fail(cl.Line, "Cannot apply #[\\Attribute] to abstract class %s", cl.Name)
	}
}

func attributeClassTargets(attrs *frontend.Node) int {
	if attrs == nil {
		return 63
	}
	for _, group := range attrs.Children {
		for _, attr := range group.Children {
			if len(attr.Children) == 0 || !strings.EqualFold(strings.TrimPrefix(attr.Children[0].Text, "\\"), "Attribute") {
				continue
			}
			if len(attr.Children) < 2 || attr.Children[1] == nil || len(attr.Children[1].Children) == 0 {
				return 63
			}
			flags, ok := attributeFlags(attr.Children[1].Children[0])
			if ok {
				return flags & 127
			}
			return 63
		}
	}
	return 63
}

func attributeFlags(n *frontend.Node) (int, bool) {
	if n == nil {
		return 0, false
	}
	switch n.Kind {
	case "literal":
		v, err := strconv.ParseInt(strings.ReplaceAll(n.Text, "_", ""), 0, 64)
		return int(v), err == nil
	case "binary":
		if n.Text != "|" || len(n.Children) < 2 {
			return 0, false
		}
		a, ok := attributeFlags(n.Children[0])
		if !ok {
			return 0, false
		}
		b, ok := attributeFlags(n.Children[1])
		return a | b, ok
	case "class_const":
		if len(n.Children) < 2 {
			return 0, false
		}
		cls, constant := n.Children[0], strings.ToUpper(n.Children[1].Text)
		if cls.Kind != "name" || !strings.EqualFold(strings.TrimPrefix(cls.Text, "\\"), "Attribute") {
			return 0, false
		}
		flags := map[string]int{"TARGET_CLASS": 1, "TARGET_FUNCTION": 2, "TARGET_METHOD": 4, "TARGET_PROPERTY": 8, "TARGET_CLASS_CONSTANT": 16, "TARGET_PARAMETER": 32, "TARGET_ALL": 63, "IS_REPEATABLE": 64}
		v, ok := flags[constant]
		return v, ok
	}
	return 0, false
}

func (l *lowerer) attributeConstExpr(n *frontend.Node, line int) {
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(compileFailure)
			if !ok {
				panic(r)
			}
			if ce, ok := f.err.(*CompileError); ok {
				ce.Line = line
			}
			panic(r)
		}
	}()
	l.checkConstExpr(n, true)
}
