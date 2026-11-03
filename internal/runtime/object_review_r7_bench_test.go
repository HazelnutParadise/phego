package runtime

import (
	"fmt"
	"testing"
	"unsafe"
)

func TestW13R7LayoutEvidence(t *testing.T) {
	t.Logf("value=%d object=%d propertyTable=%d lazyObject=%d", unsafe.Sizeof(value{}), unsafe.Sizeof(object{}), unsafe.Sizeof(propertyTable{}), unsafe.Sizeof(lazyObject{}))
}

func BenchmarkW13RawGhost(b *testing.B) {
	for _, size := range []int{2, 32, 256} {
		for _, initialize := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/initialize=%t", size, initialize), func(b *testing.B) {
				spec := classSpec{name: "W13R7Bench"}
				for i := range size {
					spec.properties = append(spec.properties, propertySpec{name: fmt.Sprintf("p%d", i), typeName: "int", hasDefault: true, initial: int64(1)})
				}
				c, err := buildClass(spec)
				if err != nil {
					b.Fatal(err)
				}
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				last := fmt.Sprintf("p%d", size-1)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					o, err := h.newLazyObject(c, false, func(*heap, value) (value, error) { return nullValue(), nil })
					if err != nil {
						b.Fatal(err)
					}
					retain(o)
					if err := h.writeRawObjectProperty(o, "p0", w13Int(42), false); err != nil {
						b.Fatal(err)
					}
					if initialize {
						if _, err := h.readObject(o, last, nil); err != nil {
							b.Fatal(err)
						}
					}
					if err := h.release(o); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
