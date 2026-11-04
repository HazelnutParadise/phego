package ir

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// Reusing a shared subtree at a greater depth must not evade the work limit.
func TestConstantMemoPreservesDepthLimit(t *testing.T) {
	p, _ := profile.Resolve("8.5.12", 64, false)
	l := &lowerer{unit: &Unit{Profile: p}}
	leaf := &Node{Op: OpLiteral, Value: &Literal{Kind: LitInt, Int: 1}}
	child := &Node{Op: OpUnary, Text: "!", Kids: []*Node{leaf}}
	b := &compileBudget{work: compileConstMaxWork, bytes: compileConstMaxBytes}
	if _, ok := l.compileConst(child, 0, b); !ok || b.exhausted {
		t.Fatal("shallow value failed")
	}
	if _, ok := l.compileConst(child, compileConstMaxDepth-1, b); ok || !b.exhausted {
		t.Fatal("shared deep value bypassed the depth limit")
	}
	syntaxLeaf := &frontend.Node{Kind: "literal", Token: frontend.TLNumber, Text: "1"}
	syntaxChild := &frontend.Node{Kind: "unary", Text: "!", Children: []*frontend.Node{syntaxLeaf}}
	l.constants = &compileBudget{work: compileConstMaxWork, bytes: compileConstMaxBytes}
	if e := l.syntaxConstant(syntaxChild, 0); !e.known || l.constants.exhausted {
		t.Fatal("shallow syntax value failed")
	}
	if e := l.syntaxConstant(syntaxChild, compileConstMaxDepth-1); e.known || !l.constants.exhausted {
		t.Fatal("shared deep syntax value bypassed the depth limit")
	}
}

func TestConstantUnvisitedSubtreeDoesNotExhaustMetadata(t *testing.T) {
	p, _ := profile.Resolve("8.5.12", 64, false)
	selected := &frontend.Node{Kind: "literal", Token: frontend.TLNumber, Text: "1"}
	large := &frontend.Node{Kind: "array", Children: make([]*frontend.Node, compileConstMaxTreeNodes+1)}
	root := &frontend.Node{Kind: "ternary", Children: []*frontend.Node{selected, selected, large}}
	l := &lowerer{unit: &Unit{Profile: p}, constants: newSyntaxCompileBudget(root)}
	e := l.syntaxConstant(root, 0)
	if !e.known || l.constants.exhausted || e.value.value.Int != 1 {
		t.Fatalf("unselected subtree affected constant evaluation: %#v", e)
	}
	// The HIR fallback follows the same actual-visited rule.
	hir := &Node{Op: OpLogical, Text: "&&", Kids: []*Node{
		{Op: OpLiteral, Value: &Literal{Kind: LitBool}},
		{Op: OpArray, Kids: make([]*Node, compileConstMaxTreeNodes+1)},
	}}
	b := newCompileBudget(hir)
	v, known := l.compileConst(hir, 0, b)
	if !known || b.exhausted || v.value.Bool {
		t.Fatalf("unselected HIR subtree affected constant evaluation: %#v", v)
	}
}

func TestConstantSyntaxPendingChainRemainsBounded(t *testing.T) {
	p, _ := profile.Resolve("8.5.12", 64, false)
	leaf := &frontend.Node{Kind: "array"}
	for range compileConstMaxTreeNodes {
		v := &frontend.Node{Kind: "literal", Token: frontend.TLNumber, Text: "1"}
		leaf.Children = append(leaf.Children, &frontend.Node{Kind: "item", Children: []*frontend.Node{nil, v}})
	}
	root := leaf
	for range 128 {
		root = &frontend.Node{Kind: "unary", Text: "!", Children: []*frontend.Node{root}}
	}
	l := &lowerer{unit: &Unit{Profile: p}, constants: newSyntaxCompileBudget(root)}
	e := l.syntaxConstant(root, 0)
	if e.known || !l.constants.exhausted || len(l.constants.syntax) > compileConstMaxTreeNodes || len(l.constants.depths) != 0 {
		t.Fatal("pending unary frames bypassed the aggregate node limit")
	}
}

// A linear constant operation chain needs bounded work, not recursive stack.
func TestConstantSyntaxLongUnaryChain(t *testing.T) {
	for _, width := range []int{32, 64} {
		for _, n := range []int{7500, 7501} {
			want := "1"
			if n%2 != 0 {
				want = "-2"
			}
			_, err := lowerSource("<?php (["+strings.Repeat("~", n)+"1] === ["+want+"])::class;", width)
			var ce *CompileError
			if !errors.As(err, &ce) || ce.Message != `Cannot use "::class" on true` {
				t.Fatalf("width%d chain%d: %v", width, n, err)
			}
		}
	}
}
