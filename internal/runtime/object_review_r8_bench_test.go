package runtime

import (
	"errors"
	"fmt"
	"testing"
)

func BenchmarkW13RollbackSources(b *testing.B) {
	for _, size := range []int{32, 256, 1024} {
		for _, typed := range []bool{true, false} {
			b.Run(fmt.Sprintf("%d/typed=%t", size, typed), func(b *testing.B) {
				propertyType := ""
				if typed {
					propertyType = "int"
				}
				properties := make([]propertySpec, 0, size+1)
				for i := range size {
					properties = append(properties, propertySpec{name: fmt.Sprintf("p%d", i), typeName: propertyType, hasDefault: true, initial: int64(1)})
				}
				properties = append(properties, propertySpec{name: "trigger", typeName: "int", hasDefault: true, initial: int64(1)})
				class, err := buildClass(classSpec{name: fmt.Sprintf("W13R8RollbackBench%d%t", size, typed), properties: properties})
				if err != nil {
					b.Fatal(err)
				}
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				calls := 0
				o, err := h.newLazyObject(class, false, func(*heap, value) (value, error) {
					calls++
					return value{}, &Throwable{"Exception", "init"}
				})
				if err != nil {
					b.Fatal(err)
				}
				var owner value
				if err := h.assign(&owner, o); err != nil {
					b.Fatal(err)
				}
				aliases := make([]value, size)
				for i := range size {
					name := fmt.Sprintf("p%d", i)
					if err := h.writeRawObjectProperty(o, name, w13Int(1), false); err != nil {
						b.Fatal(err)
					}
					if err := h.referenceObject(&aliases[i], o, name, nil); err != nil {
						b.Fatal(err)
					}
				}

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := h.readObject(o, "trigger", nil); err != nil {
						var throwable *Throwable
						if !errors.As(err, &throwable) || throwable.Class != "Exception" || throwable.Message != "init" {
							b.Fatalf("lazy initializer error = %v, want Exception(init)", err)
						}
					} else {
						b.Fatal("lazy initializer unexpectedly succeeded")
					}
				}
				b.StopTimer()
				if calls != b.N {
					b.Fatalf("initializer calls=%d, want %d", calls, b.N)
				}
				for i := range aliases {
					if err := h.unset(&aliases[i]); err != nil {
						b.Fatal(err)
					}
				}
				if err := h.unset(&owner); err != nil {
					b.Fatal(err)
				}
				if h.liveObjects != 0 || len(h.typedSources) != 0 {
					b.Fatalf("benchmark fixture retained values: live=%d typed=%d", h.liveObjects, len(h.typedSources))
				}
			})
		}
	}
}
