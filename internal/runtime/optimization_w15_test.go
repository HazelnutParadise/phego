package runtime

import (
	"context"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/ir/ssa"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// REQ-016/024/034/054; T-031/032/047/048/067/068/107/108.
func TestW15OptimizationRuntime(t *testing.T) {
	for _, width := range []int{32, 64} {
		prof, err := profile.Resolve("8.5.12", width, false)
		if err != nil {
			t.Fatal(err)
		}
		src := []byte("<?php return (1+2)*(3+4);")
		tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: width, MaxBytes: len(src) + 1})
		if err != nil {
			t.Fatal(err)
		}
		u, err := ir.Lower(tree, prof, ir.Options{File: "w15.php"})
		if err != nil {
			t.Fatal(err)
		}
		for _, passes := range []ssa.Passes{0, ssa.FoldConstants, ssa.SpecializeTypes, ssa.AllPasses} {
			p, err := ssa.Build(context.Background(), u, ssa.Options{Passes: passes, MaxNodes: 10000, MaxDepth: 256, MaxConstantBytes: 65536})
			if err != nil {
				t.Fatal(err)
			}
			_, roots, ok := p.Function(0)
			if !ok || len(roots) != 1 {
				t.Fatal("missing main")
			}
			v, _ := p.Value(p.Child(roots[0], 0))
			if passes&ssa.FoldConstants != 0 && (!v.IsConstant || v.Constant.Int != 21) {
				t.Fatalf("fold: %+v", v)
			}
		}
	}
}
