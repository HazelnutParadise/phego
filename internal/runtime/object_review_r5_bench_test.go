package runtime

import (
	"fmt"
	"testing"
)

// REQ-012/018; T-023/024/035/036. Clone with independently held aliases.
func BenchmarkW13AliasedClone(b *testing.B) {
	for _, n := range []int{1, 32, 256} {
		for _, mode := range []string{"typed", "plain", "dynamic"} {
			b.Run(fmt.Sprintf("%s/%d", mode, n), func(b *testing.B) {
				spec := classSpec{name: "AliasedCloneBench", allowDynamic: true}
				names := make([]string, n)
				for i := range names {
					names[i] = fmt.Sprintf("p%d", i)
					p := propertySpec{name: names[i], hasDefault: true, initial: int64(0)}
					if mode != "plain" {
						p.typeName = "int"
					}
					spec.properties = append(spec.properties, p)
				}
				c, err := buildClass(spec)
				if err != nil {
					b.Fatal(err)
				}
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				src, err := h.instantiate(c)
				if err != nil {
					b.Fatal(err)
				}
				retain(src)
				aliases := make([]value, n)
				for i := range aliases {
					name := names[i]
					if mode == "dynamic" {
						name = "d" + name
						if err := h.writeObject(src, name, w13Int(0), nil, false); err != nil {
							b.Fatal(err)
						}
					}
					if err := h.referenceObject(&aliases[i], src, name, nil); err != nil {
						b.Fatal(err)
					}
				}
				b.Cleanup(func() {
					for i := range aliases {
						if err := h.unset(&aliases[i]); err != nil {
							b.Fatal(err)
						}
					}
					if err := h.release(src); err != nil {
						b.Fatal(err)
					}
					if h.liveObjects != 0 {
						b.Fatal("live objects", h.liveObjects)
					}
				})
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					cloned, err := h.cloneObject(src, nil)
					if err != nil {
						b.Fatal(err)
					}
					if err := h.discard(cloned); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
