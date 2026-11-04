package ir

import (
	"fmt"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// These are host work limits, not PHP diagnostics. Each expression has its own
// budget, sized from its source graph. No request state is shared or cached.
const (
	compileConstMaxWork  = 1 << 20
	compileConstMaxBytes = 32 << 20
)

type constantLimitError struct{ line int }

func (e *constantLimitError) Error() string {
	return fmt.Sprintf("ir: constant evaluation resource limit exceeded at line %d", e.line)
}

type compileBudget struct {
	work, bytes int
	exhausted   bool
	memo        map[*Node]compileMemoEntry
	syntax      map[*frontend.Node]syntaxConstEntry
	depths      []int
}

func (b *compileBudget) spendWork(n int) bool {
	if b.exhausted || n < 0 || n > b.work {
		b.exhausted = true
		return false
	}
	b.work -= n
	return true
}

func (b *compileBudget) spendBytes(n int) bool {
	if b.exhausted || n < 0 || n > b.bytes {
		b.exhausted = true
		return false
	}
	b.bytes -= n
	return true
}

// scan accounts for numeric parsing/comparison and hashing without allocating
// copies. A unit is at most 64 bytes; the multiplier covers repeated scans.
func (b *compileBudget) scan(values ...*Literal) bool {
	for _, v := range values {
		if v != nil && v.Kind == LitString && !b.spendWork(4*(1+len(v.Str)/64)) {
			return false
		}
	}
	return true
}

func (b *compileBudget) depth(depth int) bool {
	if depth >= compileConstMaxDepth {
		b.exhausted = true
	}
	return !b.exhausted
}

// Scan metadata once; the bounded seen map also handles shared HIR subtrees.
// Saturate source size before arithmetic. Large existing literals consume no
// allocation budget until an operation actually creates another payload.
func newCompileBudget(root *Node) *compileBudget {
	b := &compileBudget{}
	seen := make(map[*Node]struct{})
	edges, sourceBytes := 0, 0
	stack := []*Node{root}
	for len(stack) != 0 {
		i := len(stack) - 1
		n := stack[i]
		stack = stack[:i]
		if n == nil {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		if len(seen) > compileConstMaxTreeNodes || len(n.Kids) > compileConstMaxTreeNodes-edges {
			return &compileBudget{work: compileConstMaxWork, bytes: compileConstMaxBytes}
		}
		edges += len(n.Kids)
		if n.Value != nil && n.Value.Kind == LitString {
			sourceBytes = min(compileConstMaxBytes, sourceBytes+min(compileConstMaxBytes, len(n.Value.Str)))
		}
		stack = append(stack, n.Kids...)
	}
	size := len(seen) + edges
	b.work = min(compileConstMaxWork, 8*(size+(sourceBytes+63)/64))
	b.bytes = min(compileConstMaxBytes, 8*sourceBytes+64*size)
	return b
}

// Metadata scanning estimates capacity only. An unselected large subtree must
// not fail compilation. The evaluator independently bounds actual work, memory
// and depth, including memo hits.
func newSyntaxCompileBudget(root *frontend.Node) *compileBudget {
	seen := make(map[*frontend.Node]struct{})
	edges, sourceBytes := 0, 0
	stack := []*frontend.Node{root}
	for len(stack) > 0 {
		i := len(stack) - 1
		n := stack[i]
		stack = stack[:i]
		if n == nil {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		if len(seen) > compileConstMaxTreeNodes || len(n.Children) > compileConstMaxTreeNodes-edges {
			return &compileBudget{work: compileConstMaxWork, bytes: compileConstMaxBytes}
		}
		edges += len(n.Children)
		sourceBytes = min(compileConstMaxBytes, sourceBytes+min(compileConstMaxBytes, len(n.Text)))
		stack = append(stack, n.Children...)
	}
	size := len(seen) + edges
	return &compileBudget{work: min(compileConstMaxWork, 8*(size+(sourceBytes+63)/64)), bytes: min(compileConstMaxBytes, 8*sourceBytes+64*size)}
}

type compileMemoEntry struct {
	value  compileConst
	known  bool
	height int
}

func (l *lowerer) evaluateConstant(n *Node) (compileConst, bool) {
	b := l.constants
	if b == nil {
		b = newCompileBudget(n)
	}
	v, known := l.compileConst(n, 0, b)
	if b.exhausted {
		panic(compileFailure{&constantLimitError{line: n.Line}})
	}
	return v, known
}

// Completed unknown results are memoized too. Height records the deepest
// evaluated descendant so reusing a shallow result cannot bypass the limit.
func (l *lowerer) compileConst(n *Node, depth int, b *compileBudget) (value compileConst, known bool) {
	if n == nil || !b.depth(depth) || !b.spendWork(1) {
		return
	}
	if entry, ok := b.memo[n]; ok {
		if !b.depth(depth + entry.height) {
			return
		}
		if i := len(b.depths) - 1; i >= 0 {
			b.depths[i] = max(b.depths[i], depth+entry.height)
		}
		return entry.value, entry.known
	}
	if len(b.memo)+len(b.syntax)+len(b.depths) >= compileConstMaxTreeNodes {
		b.exhausted = true
		return
	}
	if b.memo == nil {
		b.memo = make(map[*Node]compileMemoEntry)
	}
	i := len(b.depths)
	b.depths = append(b.depths, depth)
	completed := false
	defer func() {
		deepest := b.depths[i]
		b.depths = b.depths[:i]
		if i > 0 {
			b.depths[i-1] = max(b.depths[i-1], deepest)
		}
		if completed && !b.exhausted {
			b.memo[n] = compileMemoEntry{value: value, known: known, height: deepest - depth}
		}
	}()
	value, known = l.compileConstValue(n, depth, b)
	completed = true
	return
}
