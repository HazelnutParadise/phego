package runtime

import (
	"fmt"
	"reflect"
	"testing"
)

func TestW13LayoutEvidence(t *testing.T) {
	for _, v := range []any{value{}, reference{}, bucket{}, phpArray{}, object{}, propertyTable{}} {
		typ := reflect.TypeOf(v)
		t.Logf("%s=%d B", typ.Name(), typ.Size())
	}
}

// REQ-018/020/021; T-035/036/039-042. Stable locations are needed by
// references and reentrant destructors; table growth must never move them.
func TestW13PropertyTableStableLocations(t *testing.T) {
	p := newPropertyTable(nil)
	first := p.slot("first")
	*first = str("held")
	for i := range 1024 {
		*p.slot(fmt.Sprint(i)) = w12Int(int64(i))
	}
	if p.slot("first") != first || first.text != "held" {
		t.Fatal("growth moved a property location")
	}
	if v, ok := p.remove("first"); !ok || v.text != "held" {
		t.Fatal("remove lost property")
	}
	*first = str("detached")
	if p.slot("first") == first {
		t.Fatal("reinsertion reused a detached location")
	}
}

func TestW13PropertyTableOrder(t *testing.T) {
	p := newPropertyTable(newPropertyLayout([]string{"a", "b"}))
	*p.slot("a") = w12Int(1)
	*p.slot("b") = w12Int(2)
	*p.slot("c") = w12Int(3)
	p.remove("b")
	*p.slot("b") = w12Int(4)
	var names []string
	p.each(func(name string, _ *value) { names = append(names, name) })
	if !reflect.DeepEqual(names, []string{"a", "b", "c"}) {
		t.Fatal(names)
	}
}

func TestW13PropertyTableReentrantGrowth(t *testing.T) {
	p := newPropertyTable(nil)
	*p.slot("first") = w12Int(1)
	seen := []string{}
	p.each(func(name string, _ *value) {
		seen = append(seen, name)
		if name == "first" {
			for i := range 40 {
				*p.slot(fmt.Sprint(i)) = w12Int(int64(i))
			}
		}
	})
	if len(seen) != 41 || seen[0] != "first" || seen[40] != "39" {
		t.Fatal("iteration lost reentrant appends", seen)
	}
	old := p.slot("first")
	_, _ = p.remove("first")
	*p.slot("first") = w12Int(2)
	if p.slot("first") == old {
		t.Fatal("reinsert reused escaped location")
	}
	seen = nil
	p.each(func(name string, _ *value) { seen = append(seen, name) })
	if len(seen) != 41 || seen[len(seen)-1] != "first" {
		t.Fatal("reinsert lost order", seen)
	}
}

func BenchmarkW13PropertyTable(b *testing.B) {
	names := make([]string, 32)
	for i := range names {
		names[i] = fmt.Sprint("p", i)
	}
	layout := newPropertyLayout(names)
	for _, mode := range []string{"array", "declared", "dynamic"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if mode == "array" {
					a := newArray()
					for _, n := range names {
						a.insert(arrayKey{isString: true, text: n}, w12Int(1), 64)
					}
					continue
				}
				var declared *propertyLayout
				if mode == "declared" {
					declared = layout
				}
				p := newPropertyTable(declared)
				for _, n := range names {
					*p.slot(n) = w12Int(1)
				}
			}
		})
	}
}

// Complete object allocation/store/release, in addition to table-only cost.
func BenchmarkW13ObjectLifecycle(b *testing.B) {
	for _, size := range []int{0, 1, 4, 32, 256} {
		for _, declared := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/declared=%t", size, declared), func(b *testing.B) {
				names := make([]string, size)
				spec := classSpec{name: "Bench", allowDynamic: true}
				for i := range names {
					names[i] = fmt.Sprintf("p%d", i)
					if declared {
						spec.properties = append(spec.properties, propertySpec{name: names[i]})
					}
				}
				c, err := buildClass(spec)
				if err != nil {
					b.Fatal(err)
				}
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					o, err := h.newObject(c)
					if err != nil {
						b.Fatal(err)
					}
					retain(o)
					for _, name := range names {
						if err := h.setProperty(o, name, w12Int(1)); err != nil {
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

func BenchmarkW13MethodDispatch(b *testing.B) {
	c, err := buildClass(classSpec{name: "CallBench", methods: []methodSpec{{name: "f", returnType: "int", call: func(*heap, value, []value) (value, error) { return w12Int(7), nil }}}})
	if err != nil {
		b.Fatal(err)
	}
	h, err := newHeap(64)
	if err != nil {
		b.Fatal(err)
	}
	o, err := h.instantiate(c)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		out, err := h.callMethod(o, "f", nil, nil, true)
		if err != nil || out.integer != 7 {
			b.Fatal(out, err)
		}
	}
}
