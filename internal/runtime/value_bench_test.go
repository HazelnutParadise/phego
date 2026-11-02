package runtime

import "testing"

// These benchmarks measure the candidate model's allocation, copy, reference,
// destruction, and cycle-collection costs. They are model measurements, not
// PHP engine performance results.

func benchHeap(b *testing.B) *heap {
	h, err := newHeap(64)
	if err != nil {
		b.Fatal(err)
	}
	return h
}

func BenchmarkPackedAppend1K(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		h := benchHeap(b)
		var arr value
		for i := int64(0); i < 1024; i++ {
			if err := h.appendValue(&arr, value{kind: valueInt, integer: i}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkCopyOnWriteSeparation1K(b *testing.B) {
	h := benchHeap(b)
	var src value
	for i := int64(0); i < 1024; i++ {
		if err := h.appendValue(&src, value{kind: valueInt, integer: i}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		var cp value
		if err := h.assign(&cp, src); err != nil {
			b.Fatal(err)
		}
		if err := h.setElement(&cp, value{kind: valueInt}, value{kind: valueInt, integer: -1}); err != nil {
			b.Fatal(err)
		}
		if err := h.unset(&cp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReferenceBindAndWrite(b *testing.B) {
	h := benchHeap(b)
	b.ReportAllocs()
	for b.Loop() {
		var x, y value
		if err := h.bind(&y, &x); err != nil {
			b.Fatal(err)
		}
		if err := h.assign(&y, value{kind: valueInt, integer: 1}); err != nil {
			b.Fatal(err)
		}
		_ = h.unset(&x)
		_ = h.unset(&y)
	}
}

func BenchmarkObjectDestruct(b *testing.B) {
	h := benchHeap(b)
	c := &class{destructor: func(*heap, value) error { return nil }}
	b.ReportAllocs()
	for b.Loop() {
		o, err := h.newObject(c)
		if err != nil {
			b.Fatal(err)
		}
		var v value
		_ = h.assign(&v, o)
		if err := h.unset(&v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCollect100ObjectCycles(b *testing.B) {
	h := benchHeap(b)
	c := &class{}
	b.ReportAllocs()
	for b.Loop() {
		for i := 0; i < 100; i++ {
			o, _ := h.newObject(c)
			var v value
			_ = h.assign(&v, o)
			_ = h.setProperty(v, "self", v)
			_ = h.unset(&v)
		}
		n, err := h.collectCycles()
		if err != nil || n != 100 {
			b.Fatalf("collected %d: %v", n, err)
		}
	}
}
