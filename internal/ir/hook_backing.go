package ir

import (
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// hookUsesProperty mirrors zend_property_hook_find_property_usage: a hook
// backs its property when it fetches that name from $this, where both the
// variable name and the property name are string values after parsing.
func hookUsesProperty(object, property *frontend.Node, name string) bool {
	if object = unparen(object); object == nil {
		return false
	}
	switch object.Kind {
	case "variable":
		if object.Text != "$this" {
			return false
		}
	case "variable_variable":
		if len(object.Children) != 1 {
			return false
		}
		if v, isString, ok := parseTimeValue(object.Children[0]); !ok || !isString || v != "this" {
			return false
		}
	default:
		return false
	}
	if property = unparen(property); property != nil && property.Kind == "identifier" {
		return property.Text == name
	}
	v, isString, ok := parseTimeValue(property)
	return ok && isString && v == name
}

// parseTimeValue returns the value of a node the parser already holds as a
// zval: a literal, a heredoc without interpolation, or a concatenation of
// such values, which zend_ast_create_concat_op folds. Decoding is silent;
// lowering the expression reports literal diagnostics.
func parseTimeValue(n *frontend.Node) (value string, isString, ok bool) {
	n = unparen(n)
	if n == nil {
		return "", false, false
	}
	switch n.Kind {
	case "literal":
		switch n.Token {
		case frontend.TLNumber, frontend.TDNumber:
			v, _ := constString(numberLiteral(n.Text, n.Token))
			return v, false, true
		case frontend.TConstantEncapsedString:
			return stringDecoder{}.constant(n.Text, n.Line), true, true
		}
	case "heredoc":
		parts := n.Children[:len(n.Children)-1]
		indent := heredocIndent(n.Children[len(n.Children)-1].Text)
		nowdoc := strings.Contains(n.Text, "'")
		var b strings.Builder
		for i, part := range parts {
			if part.Kind != "string_part" {
				return "", false, false
			}
			text := part.Text
			last := i == len(parts)-1
			if last {
				text = trimFinalNewline(text)
			}
			text = stripIndentation(text, indent, i == 0, last)
			if !nowdoc {
				text = stringDecoder{}.escapes(text, 0, part.Line)
			}
			b.WriteString(text)
		}
		return b.String(), true, true
	case "binary":
		if n.Text != "." || len(n.Children) != 2 {
			break
		}
		// Walk the left operands so a long chain costs its length.
		var rights []*frontend.Node
		left := n
		for cur := n; cur != nil && cur.Kind == "binary" && cur.Text == "." && len(cur.Children) == 2; cur = unparen(left) {
			rights = append(rights, cur.Children[1])
			left = cur.Children[0]
		}
		first, _, ok := parseTimeValue(left)
		if !ok {
			break
		}
		var b strings.Builder
		b.WriteString(first)
		for i := len(rights) - 1; i >= 0; i-- {
			v, _, ok := parseTimeValue(rights[i])
			if !ok {
				return "", false, false
			}
			b.WriteString(v)
		}
		return b.String(), true, true
	}
	return "", false, false
}
