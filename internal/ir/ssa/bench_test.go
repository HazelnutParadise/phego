package ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

var buildSink *Program

func BenchmarkW15Build(b *testing.B) {
	for name, source := range map[string]string{
		"constants": "return " + strings.Repeat("(1+2)*(3+4)+", 64) + "0;",
		"typed":     strings.Repeat("$a=((int)$x)|((int)$y);", 64),
		"opaque":    strings.Repeat("if($x){f($x);}else{g($y);}", 64),
	} {
		prof, _ := profile.Resolve("8.5.12", 64, false)
		src := []byte("<?php " + source)
		tree, err := frontend.Parse(src, frontend.LexOptions{MaxBytes: len(src) + 1, IntWidth: 64})
		if err != nil {
			b.Fatal(err)
		}
		u, err := ir.Lower(tree, prof, ir.Options{File: "benchmark.php"})
		if err != nil {
			b.Fatal(err)
		}
		for label, passes := range map[string]Passes{"off": 0, "on": AllPasses} {
			b.Run(name+"/"+label, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					buildSink, err = Build(context.Background(), u, Options{Passes: passes, MaxNodes: 100000, MaxDepth: 1024, MaxConstantBytes: 1 << 20})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

type pointerCandidate struct {
	value    Value
	operands [2]*pointerCandidate
}

var pointerSink *pointerCandidate

type slabDefinition struct {
	value             Value
	start, kids, args int
}
type slabCandidate struct {
	profile      profile.Profile
	file         string
	values       []slabDefinition
	inputs       []ID
	functions    []function
	initializers []ID
}

var slabSink *slabCandidate

// This compares storage construction only, not complete compiler analyses.
// Both candidates retain 512 values with two backward operand edges per value.
func BenchmarkW15Storage(b *testing.B) {
	b.Run("slab", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p := &slabCandidate{values: make([]slabDefinition, 512), inputs: make([]ID, 1024)}
			for i := range p.values {
				p.values[i] = slabDefinition{value: Value{Type: Int}, start: i * 2, kids: 2}
				if i > 0 {
					p.inputs[i*2] = ID(i)
					p.inputs[i*2+1] = ID(i)
				}
			}
			slabSink = p
		}
	})
	b.Run("dense", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			p := &Program{values: make([]definition, 512)}
			for i := range p.values {
				p.values[i] = definition{value: Value{Type: Int}}
				if i > 0 {
					p.values[i].operands = [2]ID{ID(i), ID(i)}
				}
			}
			buildSink = p
		}
	})
	b.Run("pointers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var last *pointerCandidate
			for range 512 {
				last = &pointerCandidate{value: Value{Type: Int}, operands: [2]*pointerCandidate{last, last}}
			}
			pointerSink = last
		}
	})
}
