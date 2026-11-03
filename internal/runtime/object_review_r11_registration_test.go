package runtime

import (
	"fmt"
	"testing"
)

// REQ-012/018; T-023/024/035/036. Related registration keeps atomic publication.
func TestW13R11ClassRegistration(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			h, err := newHeap(width)
			must(t, err)
			chain := w13R11ClassChain(t, 64)
			for i, c := range chain {
				must(t, h.registerClass(c))
				if len(h.classes) != i+1 {
					t.Fatalf("registered %d names after %d", len(h.classes), i+1)
				}
				for _, ancestor := range chain[:i+1] {
					if h.classes[phpNameLower(ancestor.name)] != ancestor {
						t.Fatal("ancestor identity changed")
					}
				}
			}
			must(t, h.registerClass(chain[len(chain)-1]))
			if len(h.classes) != len(chain) {
				t.Fatal("duplicate registration changed registry")
			}
			conflict := w13Build(t, classSpec{name: chain[32].name})
			rejected := w13Build(t, classSpec{name: "W13R11Rejected", parent: conflict})
			for range 2 {
				if h.registerClass(rejected) == nil {
					t.Fatal("accepted conflicting ancestor")
				}
				if h.classes[phpNameLower(rejected.name)] != nil {
					t.Fatal("published child after ancestor failure")
				}
				if h.classes[phpNameLower(chain[32].name)] != chain[32] {
					t.Fatal("replaced registered ancestor")
				}
			}
			parent := w13Build(t, classSpec{name: "W13R11Duplicate"})
			duplicate := w13Build(t, classSpec{name: "w13r11duplicate", parent: parent})
			if h.registerClass(duplicate) == nil {
				t.Fatal("accepted duplicate pending ancestor names")
			}
			if h.classes[phpNameLower(parent.name)] != nil {
				t.Fatal("published a partial pending chain")
			}
		})
	}
}

func w13R11ClassChain(tb testing.TB, size int) []*class {
	tb.Helper()
	chain := make([]*class, size)
	for i := range chain {
		spec := classSpec{name: fmt.Sprintf("W13R11Chain%d", i)}
		if i != 0 {
			spec.parent = chain[i-1]
		}
		c, err := buildClass(spec)
		if err != nil {
			tb.Fatal(err)
		}
		chain[i] = c
	}
	return chain
}

func BenchmarkW13ClassChainRegistration(b *testing.B) {
	for _, size := range []int{1, 16, 256, 1024} {
		chain := w13R11ClassChain(b, size)
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				for _, c := range chain {
					if err := h.registerClass(c); err != nil {
						b.Fatal(err)
					}
				}
				if len(h.classes) != size {
					b.Fatal("incomplete registry")
				}
			}
		})
	}
}
