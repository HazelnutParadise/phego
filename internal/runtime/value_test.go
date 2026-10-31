package runtime

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

// The expected output in these tests comes from the PHP 8.5.12 probes recorded
// in docs/implementation/evidence/W-06-php85-lifecycle-linux-386.json.

func testHeap(t *testing.T) *heap {
	t.Helper()
	h, err := newHeap(64)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func same(t *testing.T, a, b value) bool {
	t.Helper()
	ok, err := identical(a, b)
	must(t, err)
	return ok
}

func str(s string) value { return value{kind: valueString, text: s} }

func num(t *testing.T, i int64) value {
	t.Helper()
	v, err := intValue(i, 64)
	must(t, err)
	return v
}

type script struct {
	h   *heap
	out []string
}

func (s *script) echo(format string) { s.out = append(s.out, format) }

// namedClass destructs with "destruct <name>" using the name property.
func (s *script) namedClass() *class {
	return &class{name: "Node", destructor: func(h *heap, this value) error {
		p, err := h.property(this, "name")
		if err != nil {
			return err
		}
		s.echo("destruct " + p.deref().text)
		return nil
	}}
}

func (s *script) object(t *testing.T, c *class, name string) value {
	t.Helper()
	o, err := s.h.newObject(c)
	must(t, err)
	must(t, s.h.setProperty(o, "name", str(name)))
	return o
}

func expectOutput(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("output:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestValueKindsRemainDistinct(t *testing.T) {
	h := testHeap(t)
	o, err := h.newObject(&class{name: "C"})
	must(t, err)
	kinds := []value{{}, nullValue(), boolValue(false), num(t, 0), floatValue(0), bytesValue(nil), h.newArrayValue(), o}
	for i, a := range kinds {
		if a.kind != valueKind(i) {
			t.Fatalf("kind %d = %d", i, a.kind)
		}
		for j, b := range kinds {
			if same(t, a, b) != (i == j) {
				t.Fatalf("identical(%d, %d) = %v", i, j, !(i == j))
			}
		}
	}
}

func TestIntegerRespectsProfileWidth(t *testing.T) {
	for _, c := range []struct {
		n     int64
		width int
		valid bool
	}{
		{math.MinInt32, 32, true}, {math.MaxInt32, 32, true},
		{math.MinInt32 - 1, 32, false}, {math.MaxInt32 + 1, 32, false},
		{math.MinInt64, 64, true}, {math.MaxInt64, 64, true}, {0, 0, false}, {0, 16, false},
	} {
		got, err := intValue(c.n, c.width)
		if (err == nil) != c.valid {
			t.Fatalf("intValue(%d, %d): %v", c.n, c.width, err)
		}
		if err == nil && (got.kind != valueInt || got.integer != c.n) {
			t.Fatalf("wrong integer: %+v", got)
		}
	}
	if _, err := newHeap(16); err == nil {
		t.Fatal("heap accepted an invalid width")
	}
}

func TestByteStringsCopyInputAndOutput(t *testing.T) {
	original := []byte{'a', 0, 'b'}
	v := bytesValue(original)
	original[0] = 'x'
	out := v.bytesCopy()
	if !bytes.Equal(out, []byte{'a', 0, 'b'}) {
		t.Fatalf("bytes = %q", out)
	}
	out[2] = 'x'
	if !bytes.Equal(v.bytesCopy(), []byte{'a', 0, 'b'}) {
		t.Fatal("output aliases value")
	}
}

func TestFloatBitsArePreserved(t *testing.T) {
	for _, f := range []float64{math.Copysign(0, -1), math.Inf(1), math.NaN()} {
		if math.Float64bits(floatValue(f).floating) != math.Float64bits(f) {
			t.Fatalf("float bits changed for %v", f)
		}
	}
}

func TestReferenceSurvivesUnsetOfOriginal(t *testing.T) {
	h := testHeap(t)
	var original, alias value
	must(t, h.assign(&original, num(t, 1)))
	must(t, h.bind(&alias, &original))
	must(t, h.assign(&original, num(t, 2)))
	if alias.deref().integer != 2 {
		t.Fatalf("alias = %+v", alias.deref())
	}
	must(t, h.unset(&original))
	if original.kind != valueUndefined || alias.deref().integer != 2 || alias.ref.refcount != 1 {
		t.Fatalf("after unset original=%+v alias=%+v", original, alias)
	}
	must(t, h.assign(&alias, num(t, 3)))
	if alias.deref().integer != 3 {
		t.Fatal("alias write lost")
	}
}

func TestArrayCopySharesOnlyReferencesWithOtherOwners(t *testing.T) {
	h := testHeap(t)
	// $a = [1, 2]; $r = &$a[0]; unset($r); $b = $a; $b[0] = 9; => 1:9
	var a, r, b value
	must(t, h.appendValue(&a, num(t, 1)))
	must(t, h.appendValue(&a, num(t, 2)))
	e, err := h.element(&a, num(t, 0))
	must(t, err)
	must(t, h.bind(&r, e))
	must(t, h.unset(&r))
	must(t, h.assign(&b, a))
	must(t, h.setElement(&b, num(t, 0), num(t, 9)))
	av, _, _ := h.getElement(a, num(t, 0))
	bv, _, _ := h.getElement(b, num(t, 0))
	if av.integer != 1 || bv.integer != 9 {
		t.Fatalf("got %d:%d, want 1:9", av.integer, bv.integer)
	}
	// $x = 1; $c = [&$x]; $d = $c; $d[0] = 5; => 5:5
	var x, c, d value
	must(t, h.assign(&x, num(t, 1)))
	slot, err := h.appendElement(&c)
	must(t, err)
	must(t, h.bind(slot, &x))
	must(t, h.assign(&d, c))
	must(t, h.setElement(&d, num(t, 0), num(t, 5)))
	cv, _, _ := h.getElement(c, num(t, 0))
	if x.deref().integer != 5 || cv.integer != 5 {
		t.Fatalf("got %d:%d, want 5:5", x.deref().integer, cv.integer)
	}
	// Ordinary elements separate: ['plain' => 10] copied then written.
	var src, cp value
	must(t, h.setElement(&src, str("plain"), num(t, 10)))
	must(t, h.assign(&cp, src))
	must(t, h.setElement(&cp, str("plain"), num(t, 20)))
	sv, _, _ := h.getElement(src, str("plain"))
	pv, _, _ := h.getElement(cp, str("plain"))
	if sv.integer != 10 || pv.integer != 20 || src.array == cp.array {
		t.Fatalf("copy not separated: %d %d", sv.integer, pv.integer)
	}
}

func keys(a value) []arrayKey {
	var out []arrayKey
	pos := 0
	for b, ok := a.deref().array.next(&pos); ok; b, ok = a.deref().array.next(&pos) {
		out = append(out, b.key)
	}
	return out
}

func TestArrayOrderAndNextFreeKey(t *testing.T) {
	h := testHeap(t)
	var n value
	must(t, h.setElement(&n, num(t, -5), str("a")))
	must(t, h.appendValue(&n, str("b")))
	if got := keys(n); !slices.Equal(got, []arrayKey{intKey(-5), intKey(-4)}) {
		t.Fatalf("negative append keys %v", got)
	}
	var p value
	must(t, h.setElement(&p, num(t, 3), str("x")))
	must(t, h.unsetElement(&p, num(t, 3)))
	must(t, h.appendValue(&p, str("y")))
	if got := keys(p); !slices.Equal(got, []arrayKey{intKey(4)}) {
		t.Fatalf("append after unset keys %v", got)
	}
	var m value
	must(t, h.setElement(&m, num(t, math.MaxInt64), num(t, 1)))
	if err := h.appendValue(&m, num(t, 2)); !errors.Is(err, ErrNextElementOccupied) {
		t.Fatalf("append at PHP_INT_MAX: %v", err)
	}
	var o value
	must(t, h.setElement(&o, str("b"), num(t, 1)))
	must(t, h.setElement(&o, str("a"), num(t, 2)))
	must(t, h.setElement(&o, num(t, 10), num(t, 3)))
	must(t, h.setElement(&o, str("7"), num(t, 4)))
	want := []arrayKey{stringKey("b"), stringKey("a"), intKey(10), intKey(7)}
	if got := keys(o); !slices.Equal(got, want) {
		t.Fatalf("order %v want %v", got, want)
	}
}

func TestPackedAndIndexedArraysAgree(t *testing.T) {
	h := testHeap(t)
	var packed, indexed value
	for i := int64(0); i < 4; i++ {
		must(t, h.appendValue(&packed, num(t, i*10)))
		must(t, h.appendValue(&indexed, num(t, i*10)))
	}
	must(t, h.setElement(&indexed, str("k"), num(t, 0)))
	must(t, h.unsetElement(&indexed, str("k")))
	if !packed.array.packed() || indexed.array.packed() {
		t.Fatal("unexpected representations")
	}
	if !same(t, packed, indexed) {
		t.Fatal("packed and indexed arrays differ")
	}
	must(t, h.unsetElement(&packed, num(t, 1)))
	if packed.array.packed() {
		t.Fatal("hole kept packed form")
	}
	for _, k := range []int64{0, 2, 3} {
		if v, ok, _ := h.getElement(packed, num(t, k)); !ok || v.integer != k*10 {
			t.Fatalf("key %d = %+v %v", k, v, ok)
		}
	}
}

func TestForeachByReferenceVisitsAppendedElements(t *testing.T) {
	h := testHeap(t)
	var arr, item value
	for i := int64(1); i <= 3; i++ {
		must(t, h.appendValue(&arr, num(t, i)))
	}
	must(t, h.foreachRef(&arr, &item, func(k arrayKey) error {
		if k == intKey(0) {
			if err := h.appendValue(&arr, num(t, 4)); err != nil {
				return err
			}
		}
		return h.assign(&item, num(t, item.deref().integer*10))
	}))
	must(t, h.unset(&item))
	var got []int64
	pos := 0
	for b, ok := arr.deref().array.next(&pos); ok; b, ok = arr.deref().array.next(&pos) {
		got = append(got, b.val.deref().integer)
	}
	if !slices.Equal(got, []int64{10, 20, 30, 40}) {
		t.Fatalf("got %v", got)
	}
}

func TestDestructorRunsAtLastReference(t *testing.T) {
	s := &script{h: testHeap(t)}
	var first, second value
	must(t, s.h.assign(&first, s.object(t, s.namedClass(), "x")))
	must(t, s.h.assign(&second, first))
	must(t, s.h.unset(&first))
	s.echo("after first")
	must(t, s.h.unset(&second))
	s.echo("after second")
	expectOutput(t, s.out, "after first", "destruct x", "after second")
}

func TestReentrantDestructorsNest(t *testing.T) {
	s := &script{h: testHeap(t)}
	var c *class
	c = &class{destructor: func(h *heap, this value) error {
		p, _ := h.property(this, "name")
		name := p.deref().text
		s.echo("enter " + name)
		if err := h.setProperty(this, "child", nullValue()); err != nil {
			return err
		}
		s.echo("leave " + name)
		return nil
	}}
	leaf := s.object(t, c, "leaf")
	mid := s.object(t, c, "mid")
	must(t, s.h.setProperty(mid, "child", leaf))
	var top value
	must(t, s.h.assign(&top, s.object(t, c, "top")))
	must(t, s.h.setProperty(top, "child", mid))
	must(t, s.h.unset(&top))
	s.echo("done")
	expectOutput(t, s.out, "enter top", "enter mid", "enter leaf", "leave leaf", "leave mid", "leave top", "done")
}

func TestResurrectedObjectIsNotDestructedTwice(t *testing.T) {
	s := &script{h: testHeap(t)}
	var keep, r value
	calls := 0
	c := &class{destructor: func(h *heap, this value) error {
		calls++
		s.echo("destruct")
		return h.assign(&keep, this)
	}}
	o, err := s.h.newObject(c)
	must(t, err)
	must(t, s.h.assign(&r, o))
	must(t, s.h.unset(&r))
	if keep.kind != valueObject || keep.object.freed {
		t.Fatal("resurrected object was freed")
	}
	must(t, s.h.assign(&keep, nullValue()))
	if calls != 1 || !o.object.freed {
		t.Fatalf("calls=%d freed=%v", calls, o.object.freed)
	}
}

func TestCycleCollectionRunsDestructors(t *testing.T) {
	s := &script{h: testHeap(t)}
	c := s.namedClass()
	var a, b value
	must(t, s.h.assign(&a, s.object(t, c, "a")))
	must(t, s.h.assign(&b, s.object(t, c, "b")))
	must(t, s.h.setProperty(a, "peer", b))
	must(t, s.h.setProperty(b, "peer", a))
	must(t, s.h.unset(&a))
	must(t, s.h.unset(&b))
	s.echo("after unset")
	n, err := s.h.collectCycles()
	must(t, err)
	if n != 2 || len(s.h.live) != 0 {
		t.Fatalf("collected %d, live %d", n, len(s.h.live))
	}
	expectOutput(t, s.out, "after unset", "destruct a", "destruct b")
}

func TestCycleResurrectionSurvivesAndIsCollectedLater(t *testing.T) {
	s := &script{h: testHeap(t)}
	var saved value
	c := &class{destructor: func(h *heap, this value) error {
		p, _ := h.property(this, "name")
		s.echo("destruct " + p.deref().text)
		if p.deref().text == "keep" {
			return h.assign(&saved, this)
		}
		return nil
	}}
	for _, name := range []string{"keep", "drop"} {
		var v value
		must(t, s.h.assign(&v, s.object(t, c, name)))
		must(t, s.h.setProperty(v, "self", v))
		must(t, s.h.unset(&v))
	}
	n, err := s.h.collectCycles()
	must(t, err)
	if n != 1 || saved.kind != valueObject {
		t.Fatalf("collected %d", n)
	}
	self, _ := s.h.property(saved, "self")
	if !same(t, *self, saved) {
		t.Fatal("resurrected cycle was damaged")
	}
	must(t, s.h.assign(&saved, nullValue()))
	n, err = s.h.collectCycles()
	must(t, err)
	if n != 1 {
		t.Fatalf("second collection %d", n)
	}
	expectOutput(t, s.out, "destruct keep", "destruct drop")
}

func TestSelfReferencingArrayIsCollected(t *testing.T) {
	h := testHeap(t)
	var arr value
	e, err := h.element(&arr, num(t, 0))
	must(t, err)
	must(t, h.bind(e, &arr))
	must(t, h.unset(&arr))
	n, err := h.collectCycles()
	must(t, err)
	if n != 1 || len(h.roots) != 0 {
		t.Fatalf("collected %d roots %d", n, len(h.roots))
	}
}

func TestLiveValuesAreNotCollected(t *testing.T) {
	h := testHeap(t)
	var a value
	must(t, h.assign(&a, mustObject(t, h)))
	must(t, h.setProperty(a, "self", a))
	n, err := h.collectCycles()
	must(t, err)
	if n != 0 || a.object.freed {
		t.Fatalf("collected live cycle: %d", n)
	}
}

func mustObject(t *testing.T, h *heap) value {
	t.Helper()
	o, err := h.newObject(&class{name: "C"})
	must(t, err)
	return o
}

func TestWeakReferencesAndWeakMaps(t *testing.T) {
	s := &script{h: testHeap(t)}
	var wr *weakReference
	c := &class{destructor: func(h *heap, this value) error {
		if _, ok := wr.get(); ok {
			s.echo("destruct sees object")
		} else {
			s.echo("destruct sees null")
		}
		return nil
	}}
	var o, payload value
	must(t, s.h.assign(&o, s.object(t, c, "w")))
	var err error
	wr, err = s.h.weakReferenceFor(o)
	must(t, err)
	again, err := s.h.weakReferenceFor(o)
	must(t, err)
	if again != wr {
		t.Fatal("WeakReference identity differs")
	}
	if got, ok := wr.get(); !ok || !same(t, got, o) {
		t.Fatal("weak target missing")
	}
	m := newWeakMap()
	must(t, s.h.assign(&payload, s.object(t, &class{}, "payload")))
	must(t, s.h.weakMapSet(m, o, payload))
	must(t, s.h.unset(&payload))
	if m.count() != 1 || o.object.refcount != 1 {
		t.Fatalf("WeakMap count=%d target refcount=%d", m.count(), o.object.refcount)
	}
	payloadObj := m.entries[o.object].object
	must(t, s.h.unset(&o))
	if _, ok := wr.get(); ok || m.count() != 0 || !payloadObj.freed {
		t.Fatal("weak structures kept a freed object")
	}
	expectOutput(t, s.out, "destruct sees object")
}

func TestShutdownOrder(t *testing.T) {
	s := &script{h: testHeap(t)}
	c := s.namedClass()
	var globals value
	set := func(name string, v value) { must(t, s.h.setElement(&globals, str(name), v)) }
	set("first", s.object(t, c, "first-global"))
	set("shared", s.object(t, c, "shared"))
	shared, _, _ := s.h.getElement(globals, str("shared"))
	set("alias", shared)
	set("second", s.object(t, c, "second-global"))
	var holder value
	must(t, s.h.appendValue(&holder, s.object(t, c, "in-array")))
	set("holder", holder)
	must(t, s.h.unset(&holder))
	s.echo("end of script")
	must(t, s.h.shutdown(&globals))
	expectOutput(t, s.out, "end of script", "destruct second-global", "destruct first-global", "destruct shared", "destruct in-array")
	if _, err := s.h.newObject(c); !errors.Is(err, ErrHeapClosed) {
		t.Fatalf("closed heap accepted work: %v", err)
	}
}

func TestDestructorErrorIsReportedAfterCleanup(t *testing.T) {
	h := testHeap(t)
	boom := errors.New("destructor failed")
	inner, err := h.newObject(&class{name: "Inner"})
	must(t, err)
	outer, err := h.newObject(&class{destructor: func(*heap, value) error { return boom }})
	must(t, err)
	must(t, h.setProperty(outer, "inner", inner))
	var v value
	must(t, h.assign(&v, outer))
	if err := h.unset(&v); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if !outer.object.freed || !inner.object.freed || len(h.live) != 0 {
		t.Fatal("failed destructor left objects alive")
	}
}

func TestSnapshotOwnsDataAndRejectsLiveObjects(t *testing.T) {
	h := testHeap(t)
	var arr value
	must(t, h.setElement(&arr, str("s"), bytesValue([]byte("a\x00b"))))
	must(t, h.setElement(&arr, num(t, 7), num(t, -1)))
	snap, err := exportSnapshot(arr)
	must(t, err)
	entries := snap.([]SnapshotEntry)
	if len(entries) != 2 || entries[0].Key != "s" || entries[1].Key != int64(7) {
		t.Fatalf("snapshot %#v", entries)
	}
	b := entries[0].Value.([]byte)
	b[0] = 'x'
	if v, _, _ := h.getElement(arr, str("s")); v.text != "a\x00b" {
		t.Fatal("snapshot aliases heap bytes")
	}
	must(t, h.setElement(&arr, str("o"), mustObject(t, h)))
	if _, err := exportSnapshot(arr); err == nil {
		t.Fatal("snapshot exported a live object")
	}
}

func TestAppendingArrayToItselfCopiesIt(t *testing.T) {
	h := testHeap(t)
	// $x = [1]; $x[] = $x; => [1,[1]]; $y = ['k' => 1]; $y['self'] = $y; => {"k":1,"self":{"k":1}}
	var x, y value
	must(t, h.appendValue(&x, num(t, 1)))
	must(t, h.appendValue(&x, x))
	inner, ok, _ := h.getElement(x, num(t, 1))
	if !ok || inner.kind != valueArray || inner.array == x.array || inner.array.count() != 1 {
		t.Fatalf("self append produced %+v", inner)
	}
	if _, err := exportSnapshot(x); err != nil {
		t.Fatal(err)
	}
	must(t, h.setElement(&y, str("k"), num(t, 1)))
	must(t, h.setElement(&y, str("self"), y))
	self, _, _ := h.getElement(y, str("self"))
	if self.array == y.array || self.array.count() != 1 {
		t.Fatal("self assignment produced a recursive array")
	}
}

func TestShutdownDestructsObjectsCreatedByDestructors(t *testing.T) {
	s := &script{h: testHeap(t)}
	var globals value
	var c *class
	c = &class{destructor: func(h *heap, this value) error {
		p, _ := h.property(this, "name")
		s.echo("destruct " + p.deref().text)
		if p.deref().text == "parent" {
			return h.setElement(&globals, str("child"), s.object(t, c, "child"))
		}
		return nil
	}}
	parent := s.object(t, c, "parent")
	must(t, s.h.setElement(&globals, str("a"), parent))
	must(t, s.h.setElement(&globals, str("b"), parent))
	s.echo("end")
	must(t, s.h.shutdown(&globals))
	expectOutput(t, s.out, "end", "destruct parent", "destruct child")
}

func TestForeachByReferenceFollowsReplacedArray(t *testing.T) {
	// PHP 8.5.12: replacing [1,2,3] with [4,5] at 1 visits 1,4,5; with
	// [7,8,9,10] at 2 visits 1,2,7,8,9,10.
	for _, c := range []struct {
		at          int64
		replacement []int64
		want        []int64
	}{{1, []int64{4, 5}, []int64{1, 4, 5}}, {2, []int64{7, 8, 9, 10}, []int64{1, 2, 7, 8, 9, 10}}} {
		h := testHeap(t)
		var arr, item value
		for i := int64(1); i <= 3; i++ {
			must(t, h.appendValue(&arr, num(t, i)))
		}
		var seen []int64
		must(t, h.foreachRef(&arr, &item, func(arrayKey) error {
			v := item.deref().integer
			seen = append(seen, v)
			if v == c.at {
				var next value
				for _, n := range c.replacement {
					if err := h.appendValue(&next, num(t, n)); err != nil {
						return err
					}
				}
				if err := h.assign(&arr, next); err != nil {
					return err
				}
				return h.unset(&next)
			}
			return nil
		}))
		if !slices.Equal(seen, c.want) {
			t.Fatalf("seen %v, want %v", seen, c.want)
		}
	}
}

func TestForeachByReferenceKeepsCursorWhenSeparatingSharedArray(t *testing.T) {
	h := testHeap(t)
	var arr, copyOf, item value
	for i := int64(1); i <= 3; i++ {
		must(t, h.appendValue(&arr, num(t, i)))
	}
	var seen []int64
	must(t, h.foreachRef(&arr, &item, func(k arrayKey) error {
		seen = append(seen, item.deref().integer)
		if k == intKey(0) {
			// Sharing the inner array forces the next step to separate it.
			return h.assign(&copyOf, arr)
		}
		return nil
	}))
	if !slices.Equal(seen, []int64{1, 2, 3}) {
		t.Fatalf("seen %v", seen)
	}
}

func TestCycleMembersFreedByDestructorsAreNotCounted(t *testing.T) {
	// PHP 8.5.12 returns 1 when each destructor clears its peer.
	s := &script{h: testHeap(t)}
	c := &class{destructor: func(h *heap, this value) error {
		p, _ := h.property(this, "name")
		s.echo("destruct " + p.deref().text)
		return h.setProperty(this, "peer", nullValue())
	}}
	var a, b value
	must(t, s.h.assign(&a, s.object(t, c, "a")))
	must(t, s.h.assign(&b, s.object(t, c, "b")))
	must(t, s.h.setProperty(a, "peer", b))
	must(t, s.h.setProperty(b, "peer", a))
	must(t, s.h.unset(&a))
	must(t, s.h.unset(&b))
	n, err := s.h.collectCycles()
	must(t, err)
	if n != 1 || len(s.h.live) != 0 || len(s.out) != 2 {
		t.Fatalf("collected %d, live %d, output %v", n, len(s.h.live), s.out)
	}
}

func TestForeachByReferenceKeepsCursorAcrossBodySeparation(t *testing.T) {
	// PHP 8.5.12: $c = $arr; $arr[2] = 30 on the first element visits 1,2,30.
	h := testHeap(t)
	var arr, c, item value
	for i := int64(1); i <= 3; i++ {
		must(t, h.appendValue(&arr, num(t, i)))
	}
	done := false
	var seen []int64
	must(t, h.foreachRef(&arr, &item, func(arrayKey) error {
		seen = append(seen, item.deref().integer)
		if done {
			return nil
		}
		done = true
		if err := h.assign(&c, arr); err != nil {
			return err
		}
		return h.setElement(&arr, num(t, 2), num(t, 30))
	}))
	if !slices.Equal(seen, []int64{1, 2, 30}) {
		t.Fatalf("seen %v", seen)
	}
}

func TestForeachByReferenceVisitsElementAppendedAfterTailDelete(t *testing.T) {
	// PHP 8.5.12: iterating [1] with unset($a[0]); $a[] = 2 visits 1,2.
	h := testHeap(t)
	var a, item value
	must(t, h.appendValue(&a, num(t, 1)))
	once := false
	var seen []int64
	must(t, h.foreachRef(&a, &item, func(arrayKey) error {
		seen = append(seen, item.deref().integer)
		if once {
			return nil
		}
		once = true
		if err := h.unsetElement(&a, num(t, 0)); err != nil {
			return err
		}
		return h.appendValue(&a, num(t, 2))
	}))
	if !slices.Equal(seen, []int64{1, 2}) {
		t.Fatalf("seen %v", seen)
	}
}

func TestWeakMapValueReferringToItsKeyIsCollected(t *testing.T) {
	// PHP 8.5.12: $m[$o] = $o; unset($o); gc_collect_cycles() collects 1 and
	// empties the map.
	s := &script{h: testHeap(t)}
	c := &class{destructor: func(*heap, value) error { s.echo("destruct X"); return nil }}
	var o value
	must(t, s.h.assign(&o, s.object(t, c, "x")))
	m := newWeakMap()
	must(t, s.h.weakMapSet(m, o, o))
	w, err := s.h.weakReferenceFor(o)
	must(t, err)
	must(t, s.h.unset(&o))
	n, err := s.h.collectCycles()
	must(t, err)
	if _, alive := w.get(); n != 1 || m.count() != 0 || alive {
		t.Fatalf("collected %d, map %d, alive %v", n, m.count(), alive)
	}
	expectOutput(t, s.out, "destruct X")
}

func TestIdenticalRejectsRecursiveArrays(t *testing.T) {
	// PHP 8.5.12: $a[0] =& $a; $b[0] =& $b; $a === $b throws "Nesting level
	// too deep - recursive dependency?", while $a === $a is true.
	h := testHeap(t)
	var a, b value
	for _, v := range []*value{&a, &b} {
		e, err := h.element(v, num(t, 0))
		must(t, err)
		must(t, h.bind(e, v))
	}
	if _, err := identical(a, b); !errors.Is(err, ErrRecursiveDependency) {
		t.Fatalf("error = %v", err)
	}
	if !same(t, a, a) {
		t.Fatal("an array is not identical to itself")
	}
	var c, d value
	must(t, h.appendValue(&c, num(t, 1)))
	must(t, h.appendValue(&d, num(t, 1)))
	if !same(t, c, d) {
		t.Fatal("equal arrays differ")
	}
}

func TestIdenticalAllowsSharedAcyclicStorage(t *testing.T) {
	// PHP 8.5.12: $x = [1]; $a = [$x]; $b = [$a]; $a === $b is false.
	h := testHeap(t)
	var x, a, b value
	must(t, h.appendValue(&x, num(t, 1)))
	must(t, h.appendValue(&a, x))
	must(t, h.appendValue(&b, a))
	ok, err := identical(a, b)
	if err != nil || ok {
		t.Fatalf("identical = %v, %v", ok, err)
	}
}

func TestForeachCursorSurvivesDeletingCurrentThenSharing(t *testing.T) {
	// PHP 8.5.12: unset($arr[1]); $copy = $arr while visiting 2 visits 1,2,3.
	h := testHeap(t)
	var arr, cp, item value
	for i := int64(1); i <= 3; i++ {
		must(t, h.appendValue(&arr, num(t, i)))
	}
	once := false
	var seen []int64
	must(t, h.foreachRef(&arr, &item, func(arrayKey) error {
		v := item.deref().integer
		seen = append(seen, v)
		if v != 2 || once {
			return nil
		}
		once = true
		if err := h.unsetElement(&arr, num(t, 1)); err != nil {
			return err
		}
		return h.assign(&cp, arr)
	}))
	if !slices.Equal(seen, []int64{1, 2, 3}) {
		t.Fatalf("seen %v", seen)
	}
}

func TestForeachReturnsDestructorErrorFromFinalRelease(t *testing.T) {
	// PHP 8.5.12: unsetting the iterated array inside foreach destroys it
	// when the loop ends, and a destructor exception escapes the loop.
	h := testHeap(t)
	boom := errors.New("from destructor")
	o, err := h.newObject(&class{destructor: func(*heap, value) error { return boom }})
	must(t, err)
	var list, item value
	must(t, h.appendValue(&list, o))
	must(t, h.appendValue(&list, num(t, 1)))
	err = h.foreachRef(&list, &item, func(arrayKey) error { return h.unset(&list) })
	if !errors.Is(err, boom) {
		t.Fatalf("foreach error = %v", err)
	}
}

func TestForeachStopsWhenTargetIsNoLongerAnArray(t *testing.T) {
	// PHP 8.5.12: assigning null or 5 to the iterated variable stops the loop
	// with a warning and keeps the assigned value.
	for _, replacement := range []value{nullValue(), {kind: valueInt, integer: 5}} {
		h := testHeap(t)
		var a, item value
		must(t, h.appendValue(&a, num(t, 1)))
		must(t, h.appendValue(&a, num(t, 2)))
		var seen []int64
		must(t, h.foreachRef(&a, &item, func(arrayKey) error {
			seen = append(seen, item.deref().integer)
			return h.assign(&a, replacement)
		}))
		if !slices.Equal(seen, []int64{1}) || !identicalOrFatal(t, a, replacement) {
			t.Fatalf("seen %v, a %+v", seen, a.deref())
		}
		want := "foreach() argument must be of type array|object, " + typeName(replacement) + " given"
		if !slices.Equal(h.warnings, []string{want}) {
			t.Fatalf("warnings %v", h.warnings)
		}
	}
	h := testHeap(t)
	var notArray, item value
	must(t, h.foreachRef(&notArray, &item, func(arrayKey) error { t.Fatal("body ran"); return nil }))
	if notArray.deref().kind != valueUndefined || len(h.warnings) != 1 {
		t.Fatalf("non-array foreach changed value or missed warning: %+v %v", notArray, h.warnings)
	}
}

func identicalOrFatal(t *testing.T, a, b value) bool {
	t.Helper()
	return same(t, a, b)
}
