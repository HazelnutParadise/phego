package ssa

import (
	"context"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/ir"
)

func TestMetadataProfileAndConcurrentReadIsolation(t *testing.T) {
	u, _ := lowerSource(t, "<?php use Foo; return 1+2;", 64)
	p, err := Build(context.Background(), u, Options{Passes: AllPasses, MaxNodes: 1000, MaxDepth: 64})
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Diagnostics) == 0 || p.HIR() != u {
		t.Fatal("original compilation diagnostics lost")
	}
	if err := p.CheckProfile(u.Profile); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []int{0, 1, 2} {
		q := u.Profile
		switch variant {
		case 0:
			q.IntWidth = 32
		case 1:
			q.ShortOpenTag = !q.ShortOpenTag
		case 2:
			q.Release = "8.5.11"
		}
		if p.CheckProfile(q) == nil {
			t.Fatal("wrong profile accepted")
		}
	}
	_, roots, _ := p.Function(0)
	original := roots[0]
	roots[0] = None
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				_, got, _ := p.Function(0)
				if got[0] != original {
					t.Error("roots shared with caller")
				}
				v, _ := p.Value(original)
				v.Constant.Int = 99
				v.Sources = UnknownSource
			}
		})
	}
	wg.Wait()
}

func TestUnknownIntroducedSourcesRemainUnknown(t *testing.T) {
	p := buildSource(t, "<?php $x; foo(); (int)$x; function() use($x) {}; fn()=>$x;", 64, AllPasses)
	for id := ID(1); uint64(id) <= uint64(p.Len()); id++ {
		v, _ := p.Value(id)
		if v.HIR.Op == ir.OpVar || v.HIR.Op == ir.OpCall || v.HIR.Op == ir.OpCast || v.HIR.Op == ir.OpClosure || v.HIR.Op == ir.OpArrowFn {
			if v.Sources&UnknownSource == 0 {
				t.Fatalf("unknown source became trusted: %+v", v)
			}
		}
	}
}
