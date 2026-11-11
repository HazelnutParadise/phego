package ssa

import (
	"context"
	"errors"
	"testing"

	"github.com/HazelnutParadise/phego/internal/ir"
)

func TestContainerSlotsConsumeCompilerBudget(t *testing.T) {
	f := &ir.Func{Params: make([]*ir.Param, 32)}
	for i := range f.Params {
		f.Params[i] = &ir.Param{}
	}
	for _, u := range []*ir.Unit{
		{Profile: testProfile, Main: f},
		{Profile: testProfile, Main: &ir.Func{Body: make([]*ir.Node, 32)}},
		{Profile: testProfile, Main: &ir.Func{}, Functions: []*ir.Func{f, f, f, f}},
		{Profile: testProfile, Main: &ir.Func{}, Classes: []*ir.Class{{Props: []*ir.Prop{{}, {}, {}, {}}}}},
	} {
		p, err := Build(context.Background(), u, Options{MaxNodes: 2, MaxDepth: 16})
		if p != nil || !errors.Is(err, ErrLimit) {
			t.Fatalf("unmetered container slots: p=%v err=%v", p, err)
		}
	}
}

func TestClosureCaptureBudgetAndMalformedCapture(t *testing.T) {
	f := &ir.Func{Captures: make([]*ir.Capture, 32)}
	for i := range f.Captures {
		f.Captures[i] = &ir.Capture{}
	}
	u := &ir.Unit{Profile: testProfile, Main: &ir.Func{Body: []*ir.Node{{Op: ir.OpClosure, Func: f}}}}
	if p, err := Build(context.Background(), u, Options{MaxNodes: 8, MaxDepth: 16}); p != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("capture budget: %v %v", p, err)
	}
	f.Captures = []*ir.Capture{nil}
	if p, err := Build(context.Background(), u, Options{MaxNodes: 100, MaxDepth: 16}); p != nil || err == nil {
		t.Fatalf("nil capture: %v %v", p, err)
	}
}
