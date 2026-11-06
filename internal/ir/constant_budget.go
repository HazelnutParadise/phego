package ir

import (
	"fmt"
	"math"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// These are host work limits, not PHP diagnostics. Each expression has its own
// budget, linear in its source graph: every evaluated node and memo entry
// costs work and every operation charges the bytes it creates, so evaluation
// that PHP performs in linear time fits. No request state is shared or cached.
func sizedCompileBudget(size, sourceBytes int64) *compileBudget {
	clamp := func(v int64) int { return int(min(v, int64(math.MaxInt))) }
	return &compileBudget{work: clamp(8 * (size + (sourceBytes+63)/64)), bytes: clamp(8*sourceBytes + 64*size)}
}

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

// Scan metadata once; the seen map also handles shared HIR subtrees. Large
// existing literals consume no allocation budget until an operation actually
// creates another payload.
func newCompileBudget(root *Node) *compileBudget {
	seen := make(map[*Node]struct{})
	var edges, sourceBytes int64
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
		edges += int64(len(n.Kids))
		if n.Value != nil && n.Value.Kind == LitString {
			sourceBytes += int64(len(n.Value.Str))
		}
		stack = append(stack, n.Kids...)
	}
	return sizedCompileBudget(int64(len(seen))+edges, sourceBytes)
}

// Metadata scanning estimates capacity only. An unselected large subtree must
// not fail compilation. The evaluator independently bounds actual work, memory
// and depth, including memo hits.
func newSyntaxCompileBudget(root *frontend.Node) *compileBudget {
	seen := make(map[*frontend.Node]struct{})
	var edges, sourceBytes int64
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
		edges += int64(len(n.Children))
		sourceBytes += int64(len(n.Text))
		stack = append(stack, n.Children...)
	}
	return sizedCompileBudget(int64(len(seen))+edges, sourceBytes)
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

// probeConstant answers whether run-time code compiles n to a constant, for
// the conversions PHP applies to such operands (string conversion of a
// member or concatenation operand, method name and class checks). A
// concatenation or interpolation is a string and needs no evaluation. Other
// operands share the expression's memo and budget, so repeated probes of
// nested operands stay linear; exceeding the budget is a host limit.
func (l *lowerer) probeConstant(n *Node) (compileConst, bool) {
	if n == nil || n.Op == OpInterp || n.Op == OpBinary && n.Text == "." {
		return compileConst{}, false
	}
	return l.evaluateConstant(n)
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
