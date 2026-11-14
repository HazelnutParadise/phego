package frontend

// Node is one syntax tree node. Kind names the construct, Text holds the
// identifying source text for leaves (names, operators, literals), and Line is
// the line where the construct starts. Children are in source order; an
// omitted optional part is a nil child so positions stay stable per Kind.
// Token is the token kind of a leaf, which tells an integer literal from a
// float literal; whether a number fits the integer type depends on the
// profile's integer width.
type Node struct {
	// FunctionLine is the method function-keyword line, excluding attributes
	// and modifiers. Zero means the metadata is absent.
	FunctionLine int
	// LeadingEmptyLine is the comma line when an array/list starts with an
	// omitted element. Zero means absent; the omitted child remains nil.
	LeadingEmptyLine int
	// CloseLine is the line of the closing bracket of an array/list
	// literal. Zero means absent.
	CloseLine int
	// FoldLine is set on a "." whose operands are both literals, which
	// PHP's parser folds into one string (zend_ast_create_concat_op): the
	// line of the token after the right operand, which the folded value
	// carries. Zero means the concatenation is not folded by the parser.
	FoldLine int
	Kind     string
	Text     string
	Token    Kind
	Line     int
	Children []*Node
}

func node(kind string, line int, children ...*Node) *Node {
	return &Node{Kind: kind, Line: line, Children: children}
}

func leaf(kind string, t Token) *Node {
	return &Node{Kind: kind, Text: t.Text, Token: t.Kind, Line: t.Line}
}

// SyntaxError is a located parse failure. Compile marks errors PHP raises
// as CompileError while building the tree rather than as ParseError.
type SyntaxError struct {
	Line    int
	Message string
	Compile bool
	// CallerLocation means PHP resolves the displayed file and line from its
	// current execution frame. Line remains the source anchor for Go callers.
	CallerLocation bool
}

func (e *SyntaxError) Error() string { return e.Message }
