package frontend

import (
	"fmt"
	"iter"
	"strings"

	"github.com/HazelnutParadise/phego/internal/phpname"
)

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
	// `var` is property-only grammar. In other member declarations the
	// original parser reports the following token, before semantic checking.
	if target != targetProperty && target != targetParameter && target != targetPropertyHook {
		for _, m := range mods.Children {
			if kindOfModifier(m.Text) == TVar {
				p.unexpected()
			}
		}
	}
	if err := validateModifiers(modifierTokens(mods), target, p.peek().Line); err != nil {
		panic(parseFailure{err})
	}
}

func modifierTokens(mods *Node) iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for _, m := range mods.Children {
			if !yield(Token{Kind: kindOfModifier(m.Text), Text: m.Text, Line: m.Line}) {
				return
			}
		}
	}
}

// validateModifiers is shared by AST construction and error-only grammar
// replay. The caller supplies the scanner position at the target reduction.
func validateModifiers(tokens iter.Seq[Token], target string, line int) *SyntaxError {
	fail := func(format string, args ...any) *SyntaxError {
		return &SyntaxError{Line: line, Message: fmt.Sprintf(format, args...), Compile: true}
	}
	seen := map[string]bool{}
	for m := range tokens {
		k := m.Kind
		allowed := false
		for _, t := range modifierTargets[k] {
			allowed = allowed || t == target
		}
		if !allowed && k == TVar {
			return &SyntaxError{Line: m.Line, Message: fmt.Sprintf("syntax error, unexpected token %q", m.Text)}
		}
		if !allowed {
			return fail("Cannot use the %s modifier on a %s", phpname.Lower(m.Text), target)
		}
		group := phpname.Lower(m.Text)
		switch k {
		case TPublic, TProtected, TPrivate:
			group = "access"
		case TPublicSet, TProtectedSet, TPrivateSet:
			group = "set access"
		}
		if seen[group] {
			if strings.HasSuffix(group, "access") {
				return fail("Multiple access type modifiers are not allowed")
			}
			return fail("Multiple %s modifiers are not allowed", group)
		}
		seen[group] = true
		if seen["abstract"] && seen["final"] {
			return fail("Cannot use the final modifier on an abstract %s", target)
		}
	}
	return nil
}

// checkClassModifiers covers named and anonymous class declarations.
func (p *parser) checkClassModifiers(mods *Node, anonymous bool) {
	if mods == nil {
		return
	}
	if err := validateClassModifiers(modifierTokens(mods), anonymous, p.peek().Line); err != nil {
		// Class modifier reductions may fail before reading the next line.
		// Replay only the rejected input to preserve that exact error order.
		if detailed, _ := p.diagnoseSyntax(); detailed != nil {
			err = detailed
		}
		panic(parseFailure{err})
	}
}

func validateClassModifiers(tokens iter.Seq[Token], anonymous bool, line int) *SyntaxError {
	fail := func(format string, args ...any) *SyntaxError {
		return &SyntaxError{Line: line, Message: fmt.Sprintf(format, args...), Compile: true}
	}
	seen := map[string]bool{}
	for m := range tokens {
		name := phpname.Lower(m.Text)
		if anonymous && name != "readonly" {
			return fail("Cannot use the %s modifier on an anonymous class", name)
		}
		if seen[name] {
			return fail("Multiple %s modifiers are not allowed", name)
		}
		seen[name] = true
		if seen["abstract"] && seen["final"] {
			return fail("Cannot use the final modifier on an abstract class")
		}
	}
	return nil
}

func kindOfModifier(text string) Kind {
	lower := phpname.Lower(text)
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
