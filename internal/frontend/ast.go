package frontend

// Node is one syntax tree node. Kind names the construct, Text holds the
// identifying source text for leaves (names, operators, literals), and Line is
// the line where the construct starts. Children are in source order; an
// omitted optional part is a nil child so positions stay stable per Kind.
type Node struct {
	Kind     string
	Text     string
	Line     int
	Children []*Node
}

func node(kind string, line int, children ...*Node) *Node {
	return &Node{Kind: kind, Line: line, Children: children}
}

func leaf(kind string, t Token) *Node {
	return &Node{Kind: kind, Text: t.Text, Line: t.Line}
}

// SyntaxError is a located parse failure. Compile marks errors PHP raises
// as CompileError while building the tree rather than as ParseError.
type SyntaxError struct {
	Line    int
	Message string
	Compile bool
}

func (e *SyntaxError) Error() string { return e.Message }
