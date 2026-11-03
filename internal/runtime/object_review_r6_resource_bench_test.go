package runtime

import (
	"context"
	"testing"
)

// REQ-019; T-037/038. Track the ordinary operation path as well as cleanup.
func BenchmarkW13ResourceValidation(b *testing.B) {
	for _, mode := range []string{"operation", "explicit-close"} {
		b.Run(mode, func(b *testing.B) {
			h, err := newHeap(64)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			close := func(context.Context) error { return nil }
			var owner value
			if mode == "operation" {
				v, err := h.newResource(ctx, "stream", close)
				if err != nil {
					b.Fatal(err)
				}
				if err := h.assign(&owner, v); err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := h.unset(&owner); err != nil {
						b.Fatal(err)
					}
				})
			}
			b.ReportAllocs()
			for b.Loop() {
				if mode == "operation" {
					if err := h.checkResource(owner, "stream"); err != nil {
						b.Fatal(err)
					}
				} else {
					v, err := h.newResource(ctx, "stream", close)
					if err != nil {
						b.Fatal(err)
					}
					retain(v)
					if err := h.closeResource(v, "stream", ctx); err != nil {
						b.Fatal(err)
					}
					if err := h.release(v); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
