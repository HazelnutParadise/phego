package frontend

import "strings"

// Modifier targets, named as PHP 8.5.12 names them in errors.
const (
	targetProperty     = "property"
	targetMethod       = "method"
	targetConstant     = "class constant"
	targetParameter    = "parameter"
	targetPropertyHook = "property hook"
)

var modifierTargets = map[Kind][]string{
	TPublic:       {targetProperty, targetMethod, targetConstant, targetParameter},
	TProtected:    {targetProperty, targetMethod, targetConstant, targetParameter},
	TPrivate:      {targetProperty, targetMethod, targetConstant, targetParameter},
	TStatic:       {targetProperty, targetMethod},
	TAbstract:     {targetProperty, targetMethod},
	TFinal:        {targetProperty, targetMethod, targetConstant, targetParameter, targetPropertyHook},
	TReadonly:     {targetProperty, targetParameter},
	TVar:          {targetProperty},
	TPublicSet:    {targetProperty, targetParameter},
	TProtectedSet: {targetProperty, targetParameter},
	TPrivateSet:   {targetProperty, targetParameter},
}

// checkModifiers applies the modifier checks PHP performs while building the
// tree. They are CompileErrors reported at the line PHP's parser has reached,
// which is the token after the modifier list.
func (p *parser) checkModifiers(mods *Node, target string) {
	if mods == nil || len(mods.Children) == 0 {
		return
	}
	line := p.peek().Line
	seen := map[string]bool{}
	for _, m := range mods.Children {
		k := kindOfModifier(m.Text)
		allowed := false
		for _, t := range modifierTargets[k] {
			allowed = allowed || t == target
		}
		if !allowed {
			p.fail(line, "Cannot use the %s modifier on a %s", strings.ToLower(m.Text), target)
		}
		group := strings.ToLower(m.Text)
		switch k {
		case TPublic, TProtected, TPrivate:
			group = "access"
		case TPublicSet, TProtectedSet, TPrivateSet:
			group = "set access"
		}
		if seen[group] {
			if strings.HasSuffix(group, "access") {
				p.fail(line, "Multiple access type modifiers are not allowed")
			}
			p.fail(line, "Multiple %s modifiers are not allowed", group)
		}
		seen[group] = true
		if seen["abstract"] && seen["final"] {
			p.fail(line, "Cannot use the final modifier on an abstract %s", target)
		}
	}
}

// checkClassModifiers covers named and anonymous class declarations.
func (p *parser) checkClassModifiers(mods *Node, anonymous bool) {
	if mods == nil {
		return
	}
	line := p.peek().Line
	seen := map[string]bool{}
	for _, m := range mods.Children {
		name := strings.ToLower(m.Text)
		if anonymous && name != "readonly" {
			p.fail(line, "Cannot use the %s modifier on an anonymous class", name)
		}
		if seen[name] {
			p.fail(line, "Multiple %s modifiers are not allowed", name)
		}
		seen[name] = true
		if seen["abstract"] && seen["final"] {
			p.fail(line, "Cannot use the final modifier on an abstract class")
		}
	}
}

func kindOfModifier(text string) Kind {
	lower := strings.ToLower(text)
	switch lower {
	case "public(set)":
		return TPublicSet
	case "protected(set)":
		return TProtectedSet
	case "private(set)":
		return TPrivateSet
	}
	return keywords[lower]
}
