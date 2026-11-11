package runtime

import (
	"fmt"
	"testing"
)

func TestW13R9DynamicChurnBounded(t *testing.T) {
	for _, anchors := range []int{0, 1, 32} {
		t.Run(fmt.Sprint(anchors), func(t *testing.T) {
			p := newPropertyTable(nil)
			anchorSlots := make([]*value, anchors)
			for i := range anchors {
				name := fmt.Sprintf("anchor%d", i)
				anchorSlots[i] = p.slot(name)
				*anchorSlots[i] = w13Int(int64(i))
			}

			var detached *value
			for range 100_000 {
				slot := p.slot("x")
				if detached != nil && slot == detached {
					t.Fatal("dynamic insertion reused a detached location")
				}
				*slot = w13Int(9)
				if _, ok := p.remove("x"); !ok {
					t.Fatal("dynamic removal failed")
				}
				if detached == nil {
					detached = slot
					*detached = w13Int(77)
				}
			}
			if detached.integer != 77 || detached.kind != valueInt {
				t.Fatal("detached location was changed by a later insertion")
			}

			used, capacity, blocks := 0, 0, 0
			for block := p.head; block != nil; block = block.next {
				used += block.used
				capacity += len(block.entries)
				blocks++
			}
			if used > 16*anchors+32 || capacity > 16*anchors+32 {
				t.Fatalf("retained storage too large: anchors=%d used=%d capacity=%d", anchors, used, capacity)
			}
			if blocks > anchors+4 {
				t.Fatalf("too many retained blocks: anchors=%d blocks=%d", anchors, blocks)
			}
			if len(p.index) != anchors {
				t.Fatalf("index has %d live names, want %d", len(p.index), anchors)
			}
			if p.index == nil {
				t.Fatal("dynamic materialization state was discarded")
			}
			for i, slot := range anchorSlots {
				if p.slot(fmt.Sprintf("anchor%d", i)) != slot || slot.integer != int64(i) {
					t.Fatalf("live anchor %d moved or changed", i)
				}
			}
		})
	}
}

// Trigger collection while the active cursor is in the last dynamic block.
// A nested iterator must not clear the outer iterator's protection.
func TestW13R9ReentrantTable(t *testing.T) {
	p := newPropertyTable(nil)
	for range 15 {
		*p.slot("dead") = w13Int(1)
		p.remove("dead")
	}
	*p.slot("a") = w13Int(1)
	var outer, nested []string
	p.each(func(name string, _ *value) {
		outer = append(outer, name)
		if name != "a" {
			return
		}
		p.remove("a")
		*p.slot("d") = w13Int(4)
		p.each(func(inner string, _ *value) {
			nested = append(nested, inner)
			if inner == "d" {
				p.remove("d")
				*p.slot("e") = w13Int(5)
			}
		})
		p.remove("e")
		*p.slot("f") = w13Int(6)
		*p.slot("g") = w13Int(7)
	})
	if fmt.Sprint(outer) != "[a f g]" || fmt.Sprint(nested) != "[d e]" {
		t.Fatalf("reentrant order outer=%v nested=%v", outer, nested)
	}
	func() {
		defer func() {
			if recover() != "callback" {
				t.Fatal("iteration callback panic was not propagated")
			}
		}()
		p.each(func(name string, _ *value) {
			if name != "f" {
				return
			}
			p.remove("f")
			for range 32 {
				*p.slot("dead") = w13Int(1)
				p.remove("dead")
			}
			*p.slot("z") = w13Int(9)
			panic("callback")
		})
	}()
	for range 1000 {
		*p.slot("dead") = w13Int(1)
		p.remove("dead")
	}
	var after []string
	p.each(func(name string, _ *value) { after = append(after, name) })
	if fmt.Sprint(after) != "[g z]" {
		t.Fatalf("iteration after panic = %v", after)
	}
	used := 0
	for b := p.head; b != nil; b = b.next {
		used += b.used
	}
	if used > 64 {
		t.Fatalf("panic left reclamation inhibited: used=%d", used)
	}
}

func BenchmarkW13DynamicChurn(b *testing.B) {
	for _, anchors := range []int{0, 1, 32} {
		b.Run(fmt.Sprint(anchors), func(b *testing.B) {
			p := newPropertyTable(nil)
			for i := range anchors {
				*p.slot(fmt.Sprintf("anchor%d", i)) = w13Int(int64(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				*p.slot("x") = w13Int(1)
				if _, ok := p.remove("x"); !ok {
					b.Fatal("dynamic removal failed")
				}
			}
		})
	}
}

func BenchmarkW13ChurnTraversal(b *testing.B) {
	for _, cycles := range []int{0, 10_000, 100_000} {
		b.Run(fmt.Sprint(cycles), func(b *testing.B) {
			p := newPropertyTable(nil)
			for range cycles {
				*p.slot("x") = w13Int(1)
				p.remove("x")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				p.each(func(string, *value) {})
			}
		})
	}
}

// A caller can retain a removed location. It must not retain later blocks or
// its old name through the table's coallocated storage after reclamation.
func TestW13R9DetachedStorage(t *testing.T) {
	p := newPropertyTable(nil)
	old := p.slot("retired-name")
	*old = w13Int(1)
	block := p.head
	p.remove("retired-name")
	*old = w13Int(77)
	for range 64 {
		*p.slot("x") = w13Int(1)
		p.remove("x")
	}
	if block.next != nil || block.entries[0].name != "" {
		t.Fatal("removed location retains later blocks or its old name")
	}
	if old.integer != 77 || old.kind != valueInt {
		t.Fatal("reclamation changed a detached value")
	}
}

func BenchmarkW13ChurnObjectWork(b *testing.B) {
	for _, cycles := range []int{0, 10_000, 100_000} {
		for _, mode := range []string{"clone", "cleanup"} {
			b.Run(fmt.Sprintf("%d/%s", cycles, mode), func(b *testing.B) {
				h, err := newHeap(64)
				if err != nil {
					b.Fatal(err)
				}
				c, err := buildClass(classSpec{name: "ChurnWork", allowDynamic: true})
				if err != nil {
					b.Fatal(err)
				}
				o, err := h.newObject(c)
				if err != nil {
					b.Fatal(err)
				}
				var owner value
				if err := h.assign(&owner, o); err != nil {
					b.Fatal(err)
				}
				for range cycles {
					if err := h.setProperty(o, "x", w13Int(1)); err != nil {
						b.Fatal(err)
					}
					if err := h.unsetObject(o, "x", nil); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if mode == "cleanup" {
						if err := h.freePropertyTable(o.object.props, nil, true); err != nil {
							b.Fatal(err)
						}
					} else {
						v, err := h.cloneObject(o, nil)
						if err != nil {
							b.Fatal(err)
						}
						if err := h.discard(v); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				if err := h.unset(&owner); err != nil {
					b.Fatal(err)
				}
				if h.liveObjects != 0 {
					b.Fatal("retained objects", h.liveObjects)
				}
			})
		}
	}
}

// Internal traversal safety controls; their pointer topology is Go-private,
// separate from the fixed PHP output matrix.
func TestW13R9ComparisonCursorProtection(t *testing.T) {
	h, err := newHeap(64)
	must(t, err)
	c := w13Build(t, classSpec{name: "R9Cursor", allowDynamic: true})
	var left, right, held value
	must(t, h.assign(&left, w13Instantiate(t, h, c)))
	must(t, h.assign(&right, w13Instantiate(t, h, c)))
	defer func() {
		must(t, h.unset(&left))
		must(t, h.unset(&right))
		must(t, h.unset(&held))
		if h.liveObjects != 0 {
			t.Fatal("retained objects", h.liveObjects)
		}
	}()
	for range 15 {
		must(t, h.setProperty(left, "dead", w13Int(1)))
		must(t, h.unsetObject(left, "dead", nil))
	}
	var cursor, next *propertyBlock
	calls := 0
	text := w13Build(t, classSpec{name: "R9CursorText", methods: []methodSpec{{name: "__toString", returnType: "string", call: func(h *heap, _ value, _ []value) (value, error) {
		calls++
		for i := -1; i < 32; i++ {
			name := "a"
			if i >= 0 {
				name = fmt.Sprintf("p%d", i)
			}
			if err := h.unsetObject(left, name, nil); err != nil {
				return value{}, err
			}
		}
		if cursor.next != next || next == nil {
			t.Fatal("comparison invalidated its active block cursor")
		}
		return w13Str("same"), nil
	}}}})
	must(t, h.assign(&held, w13Instantiate(t, h, text)))
	must(t, h.setProperty(left, "a", held))
	must(t, h.setProperty(right, "a", w13Str("same")))
	for i := range 32 {
		name := fmt.Sprintf("p%d", i)
		must(t, h.setProperty(left, name, w13Int(int64(i))))
		must(t, h.setProperty(right, name, w13Int(int64(i))))
	}
	a, _ := left.object.props.lookup("a")
	for b := left.object.props.head; b != nil; b = b.next {
		for i := 0; i < b.used; i++ {
			if &b.entries[i].value == a {
				cursor = b
				next = b.next
			}
		}
	}
	if cursor == nil || next == nil {
		t.Fatal("fixture needs a non-tail comparison cursor")
	}
	_, err = h.compareObjectValues(left, right, nil)
	must(t, err)
	if calls != 1 {
		t.Fatal("Stringable calls", calls)
	}
	if left.object.props.head != nil {
		t.Fatal("completed comparison retained its fully retired chain")
	}
}

func TestW13R9RollbackCursorTransfer(t *testing.T) {
	h, err := newHeap(64)
	must(t, err)
	c := w13Build(t, classSpec{name: "R9Transfer", allowDynamic: true})
	var owner value
	must(t, h.assign(&owner, w13Instantiate(t, h, c)))
	defer func() {
		must(t, h.unset(&owner))
		if h.liveObjects != 0 {
			t.Fatal("retained objects", h.liveObjects)
		}
	}()
	p := owner.object.props
	for range 14 {
		*p.slot("dead") = w13Int(1)
		p.remove("dead")
	}
	*p.slot("a") = w13Int(2)
	*p.slot("tail") = w13Int(3)
	before := copyPropertyTable(p)
	a, _ := p.lookup("a")
	var cursor, next *propertyBlock
	for b := p.head; b != nil; b = b.next {
		for i := 0; i < b.used; i++ {
			if &b.entries[i].value == a {
				cursor = b
				next = b.next
			}
		}
	}
	if cursor == nil || next == nil {
		t.Fatal("fixture needs a non-tail rollback cursor")
	}
	var seen []string
	p.each(func(name string, _ *value) {
		seen = append(seen, name)
		if name != "a" {
			return
		}
		p.remove("a")
		p.remove("tail")
		must(t, h.restoreLazyProperties(owner.object, before))
		if cursor.next != next {
			t.Fatal("temporary rollback wrapper invalidated an outer cursor")
		}
	})
	if fmt.Sprint(seen) != "[a]" {
		t.Fatal("old cursor unexpectedly visited transferred storage", seen)
	}
	for range 1000 {
		*p.slot("dead") = w13Int(1)
		p.remove("dead")
	}
	var names []string
	p.each(func(name string, _ *value) { names = append(names, name) })
	if fmt.Sprint(names) != "[a tail]" {
		t.Fatal("restored dynamic order", names)
	}
	used := 0
	for b := p.head; b != nil; b = b.next {
		used += b.used
	}
	if used > 64 {
		t.Fatal("transfer lost reclamation state", used)
	}
}
