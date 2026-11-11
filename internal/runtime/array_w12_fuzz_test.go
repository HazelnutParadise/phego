package runtime

import (
	"errors"
	"math"
	"math/rand"
	"slices"
	"testing"
)

// REQ-007/016/017; T-013/014/031/032/033/034.
type w12ModelEntry struct {
	key arrayKey
	val int64
}

type w12ModelArray struct {
	entries  []w12ModelEntry
	nextFree int64
}

func newW12ModelArray() *w12ModelArray {
	return &w12ModelArray{nextFree: math.MinInt64}
}

func (a *w12ModelArray) find(k arrayKey) int {
	for i, entry := range a.entries {
		if entry.key == k {
			return i
		}
	}
	return -1
}

func (a *w12ModelArray) put(k arrayKey, v int64, width int) {
	if i := a.find(k); i >= 0 {
		a.entries[i].val = v
		return
	}
	a.entries = append(a.entries, w12ModelEntry{key: k, val: v})
	if !k.isString && k.integer >= a.nextFree {
		max := int64(math.MaxInt64)
		if width == 32 {
			max = math.MaxInt32
		}
		if k.integer < max {
			a.nextFree = k.integer + 1
		} else {
			a.nextFree = k.integer
		}
	}
}

func (a *w12ModelArray) remove(k arrayKey) {
	i := a.find(k)
	if i < 0 {
		return
	}
	a.entries = append(a.entries[:i], a.entries[i+1:]...)
}

func (a *w12ModelArray) append(v int64, width int) bool {
	next := a.nextFree
	if next == math.MinInt64 {
		next = 0
	}
	k := intKey(next)
	if a.find(k) >= 0 {
		return false
	}
	a.put(k, v, width)
	return true
}

func (a *w12ModelArray) clone() *w12ModelArray {
	dup := &w12ModelArray{
		entries:  append([]w12ModelEntry(nil), a.entries...),
		nextFree: a.nextFree,
	}
	return dup
}

func w12PropertyKey(b byte) arrayKey {
	if b < 224 {
		return intKey(int64(int8(b)))
	}
	return stringKey("s" + string(rune('a'+b%4)))
}

func w12PropertyValue(b byte) value {
	return value{kind: valueInt, integer: int64(int8(b))}
}

func checkW12Model(t *testing.T, width int, actual value, want *w12ModelArray) {
	t.Helper()
	if want == nil {
		if actual.kind != valueUndefined {
			t.Fatalf("width %d: array should be undefined, got kind %d", width, actual.kind)
		}
		return
	}
	if actual.deref().kind != valueArray {
		t.Fatalf("width %d: expected array, got kind %d", width, actual.deref().kind)
	}
	a := actual.deref().array
	if a.live != len(want.entries) {
		t.Fatalf("width %d: live count %d, want %d", width, a.live, len(want.entries))
	}
	if a.nextFree != want.nextFree {
		t.Fatalf("width %d: nextFree=%d, want %d", width, a.nextFree, want.nextFree)
	}

	var got []w12ModelEntry
	pos := 0
	for b, ok := a.next(&pos); ok; b, ok = a.next(&pos) {
		v := b.val.deref()
		if v.kind != valueInt {
			t.Fatalf("width %d: key %+v has value kind %d", width, b.key, v.kind)
		}
		got = append(got, w12ModelEntry{key: b.key, val: v.integer})
	}
	if !slices.Equal(got, want.entries) {
		t.Fatalf("width %d: entries %#v, want %#v", width, got, want.entries)
	}

	if a.packed() {
		if a.index != nil || len(a.buckets) < len(want.entries) {
			t.Fatalf("width %d: malformed packed storage: index=%v buckets=%d", width, a.index, len(a.buckets))
		}
		for i, b := range a.buckets {
			if b != nil && b.key != intKey(int64(i)) {
				t.Fatalf("width %d: packed bucket %d = %#v", width, i, b)
			}
		}
		return
	}
	if len(a.index) != a.live {
		t.Fatalf("width %d: index size %d, live count %d", width, len(a.index), a.live)
	}
	for i, b := range a.buckets {
		if b == nil {
			continue
		}
		if indexed, ok := a.index[b.key]; !ok || indexed != i {
			t.Fatalf("width %d: index[%+v]=%d,%v, bucket position %d", width, b.key, indexed, ok, i)
		}
	}
}

func runW12ArraySequence(t *testing.T, width int, data []byte) {
	t.Helper()
	if len(data) > 1024 {
		data = data[:1024]
	}
	h, err := referenceHeap(width)
	if err != nil {
		t.Fatal(err)
	}
	vars := [2]value{}
	models := [2]*w12ModelArray{}
	for offset, step := 0, 0; offset+3 < len(data); offset, step = offset+4, step+1 {
		op, slots, keyByte, valueByte := data[offset]%4, data[offset+1], data[offset+2], data[offset+3]
		dst, src := int(slots&1), int((slots>>1)&1)
		key := w12PropertyKey(keyByte)
		keyValue := value{kind: valueInt, integer: key.integer}
		if key.isString {
			keyValue = value{kind: valueString, text: key.text}
		}
		wantValue := int64(int8(valueByte))
		var gotErr error
		switch op {
		case 0:
			gotErr = h.setElement(&vars[dst], keyValue, w12PropertyValue(valueByte))
			if models[dst] == nil {
				models[dst] = newW12ModelArray()
			}
			models[dst].put(key, wantValue, width)
		case 1:
			gotErr = h.unsetElement(&vars[dst], keyValue)
			if models[dst] != nil {
				models[dst].remove(key)
			}
		case 2:
			gotErr = h.appendValue(&vars[dst], w12PropertyValue(valueByte))
			if models[dst] == nil {
				models[dst] = newW12ModelArray()
			}
			if !models[dst].append(wantValue, width) {
				if !errors.Is(gotErr, ErrNextElementOccupied) {
					t.Fatalf("width %d step %d: append error %v, want ErrNextElementOccupied", width, step, gotErr)
				}
				gotErr = nil
			} else if errors.Is(gotErr, ErrNextElementOccupied) {
				t.Fatalf("width %d step %d: unexpected occupied-next error", width, step)
			}
		case 3:
			gotErr = h.assign(&vars[dst], vars[src])
			if models[src] == nil {
				models[dst] = nil
			} else {
				models[dst] = models[src].clone()
			}
		}
		if gotErr != nil {
			t.Fatalf("width %d step %d op %d: %v", width, step, op, gotErr)
		}
		for i := range vars {
			checkW12Model(t, width, vars[i], models[i])
		}
	}
}

func TestW12ArrayStructureDeterministicProperties(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(map[int]string{32: "32-bit", 64: "64-bit"}[width], func(t *testing.T) {
			rng := rand.New(rand.NewSource(int64(width)))
			data := make([]byte, 1024)
			for i := range data {
				data[i] = byte(rng.Intn(256))
			}
			runW12ArraySequence(t, width, data)
		})
	}
}

func TestW12ArrayNextIndexBoundaries(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(map[int]string{32: "32-bit", 64: "64-bit"}[width], func(t *testing.T) {
			h, err := referenceHeap(width)
			if err != nil {
				t.Fatal(err)
			}
			max := int64(math.MaxInt64)
			if width == 32 {
				max = math.MaxInt32
			}
			var a value
			if err := h.setElement(&a, value{kind: valueInt, integer: -5}, w12PropertyValue(1)); err != nil {
				t.Fatal(err)
			}
			if err := h.appendValue(&a, w12PropertyValue(2)); err != nil {
				t.Fatal(err)
			}
			if got, want := keys(a), []arrayKey{intKey(-5), intKey(-4)}; !slices.Equal(got, want) {
				t.Fatalf("negative-key append order %v, want %v", got, want)
			}
			if err := h.setElement(&a, value{kind: valueInt, integer: max}, w12PropertyValue(3)); err != nil {
				t.Fatal(err)
			}
			if err := h.appendValue(&a, w12PropertyValue(4)); !errors.Is(err, ErrNextElementOccupied) {
				t.Fatalf("append with occupied maximum next index: %v", err)
			}
			if err := h.unsetElement(&a, value{kind: valueInt, integer: max}); err != nil {
				t.Fatal(err)
			}
			if a.array.nextFree != max {
				t.Fatalf("nextFree after deleting maximum key = %d, want %d", a.array.nextFree, max)
			}
			if err := h.appendValue(&a, w12PropertyValue(5)); err != nil {
				t.Fatalf("append into deleted maximum key: %v", err)
			}
			if got, ok, err := h.getElement(a, value{kind: valueInt, integer: max}); err != nil || !ok || got.integer != 5 {
				t.Fatalf("maximum-key value = %+v, present=%v, error=%v", got, ok, err)
			}
			if err := h.appendValue(&a, w12PropertyValue(6)); !errors.Is(err, ErrNextElementOccupied) {
				t.Fatalf("append after maximum key was reused: %v", err)
			}
		})
	}
}

func TestW12CopyMutationPreservesSourceAndRepresentation(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(map[int]string{32: "32-bit", 64: "64-bit"}[width], func(t *testing.T) {
			h, err := referenceHeap(width)
			if err != nil {
				t.Fatal(err)
			}
			var source, copyOf value
			for i := int64(0); i < 3; i++ {
				if err := h.appendValue(&source, value{kind: valueInt, integer: i + 10}); err != nil {
					t.Fatal(err)
				}
			}
			if err := h.setElement(&source, value{kind: valueString, text: "tail"}, w12PropertyValue(30)); err != nil {
				t.Fatal(err)
			}
			if err := h.unsetElement(&source, value{kind: valueString, text: "tail"}); err != nil {
				t.Fatal(err)
			}
			if source.array.packed() {
				t.Fatal("inserting then deleting a string key restored packed representation")
			}
			if err := h.assign(&copyOf, source); err != nil {
				t.Fatal(err)
			}
			if err := h.setElement(&copyOf, value{kind: valueInt, integer: 1}, w12PropertyValue(99)); err != nil {
				t.Fatal(err)
			}
			if err := h.unsetElement(&copyOf, value{kind: valueInt, integer: 0}); err != nil {
				t.Fatal(err)
			}
			if err := h.appendValue(&copyOf, w12PropertyValue(40)); err != nil {
				t.Fatal(err)
			}
			wantSource := &w12ModelArray{
				entries:  []w12ModelEntry{{key: intKey(0), val: 10}, {key: intKey(1), val: 11}, {key: intKey(2), val: 12}},
				nextFree: 3,
			}
			wantCopy := wantSource.clone()
			wantCopy.put(intKey(1), 99, width)
			wantCopy.remove(intKey(0))
			wantCopy.append(40, width)
			checkW12Model(t, width, source, wantSource)
			checkW12Model(t, width, copyOf, wantCopy)
		})
	}
}

func TestW12ReferenceCopySharingAndUnbinding(t *testing.T) {
	t.Run("unique-element-reference-unbinds-on-copy", func(t *testing.T) {
		h, err := newHeap(64)
		if err != nil {
			t.Fatal(err)
		}
		var source, externalAlias, copyOf value
		if err := h.setElement(&source, value{kind: valueInt, integer: 0}, w12PropertyValue(10)); err != nil {
			t.Fatal(err)
		}
		element, err := h.element(&source, value{kind: valueInt, integer: 0})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.bind(&externalAlias, element); err != nil {
			t.Fatal(err)
		}
		if err := h.unset(&externalAlias); err != nil {
			t.Fatal(err)
		}
		if err := h.assign(&copyOf, source); err != nil {
			t.Fatal(err)
		}
		if err := h.setElement(&copyOf, value{kind: valueInt, integer: 0}, w12PropertyValue(20)); err != nil {
			t.Fatal(err)
		}
		if copyOf.array.buckets[0].val.kind != valueInt {
			t.Fatalf("unshared reference remained bound after COW: kind %d", copyOf.array.buckets[0].val.kind)
		}
		sourceValue, sourceOK, sourceErr := h.getElement(source, value{kind: valueInt, integer: 0})
		copyValue, copyOK, copyErr := h.getElement(copyOf, value{kind: valueInt, integer: 0})
		if sourceErr != nil || copyErr != nil || !sourceOK || !copyOK || sourceValue.integer != 10 || copyValue.integer != 20 {
			t.Fatalf("source/copy = %+v/%+v (%v/%v), present=%v/%v", sourceValue, copyValue, sourceErr, copyErr, sourceOK, copyOK)
		}
	})

	t.Run("shared-element-reference-remains-shared", func(t *testing.T) {
		h, err := newHeap(64)
		if err != nil {
			t.Fatal(err)
		}
		var source, external, copyOf value
		if err := h.assign(&external, w12PropertyValue(10)); err != nil {
			t.Fatal(err)
		}
		element, err := h.element(&source, value{kind: valueInt, integer: 0})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.bind(element, &external); err != nil {
			t.Fatal(err)
		}
		if err := h.assign(&copyOf, source); err != nil {
			t.Fatal(err)
		}
		if copyOf.array.buckets[0].val.kind != valueReference {
			t.Fatalf("shared reference was unbound during copy: kind %d", copyOf.array.buckets[0].val.kind)
		}
		if err := h.setElement(&copyOf, value{kind: valueInt, integer: 0}, w12PropertyValue(25)); err != nil {
			t.Fatal(err)
		}
		sourceValue, sourceOK, sourceErr := h.getElement(source, value{kind: valueInt, integer: 0})
		copyValue, copyOK, copyErr := h.getElement(copyOf, value{kind: valueInt, integer: 0})
		if sourceErr != nil || copyErr != nil || !sourceOK || !copyOK ||
			external.deref().integer != 25 || sourceValue.integer != 25 || copyValue.integer != 25 {
			t.Fatalf("reference values external/source/copy = %+v/%+v/%+v, errors=%v/%v, present=%v/%v",
				external.deref(), sourceValue, copyValue, sourceErr, copyErr, sourceOK, copyOK)
		}
	})
}

func FuzzW12ArrayStructure(f *testing.F) {
	// set 0, copy to 1, mutate 1, append to 0, delete 0, append again.
	f.Add([]byte{0, 0, 0, 7, 3, 1, 0, 0, 0, 1, 0, 9, 2, 0, 0, 4, 1, 0, 0, 0, 2, 0, 0, 5})
	f.Add([]byte{0, 0, 251, 1, 2, 0, 0, 2, 0, 0, 0, 3, 3, 1, 0, 4, 0, 0, 255, 8})
	f.Add(make([]byte, 1024))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, width := range []int{32, 64} {
			runW12ArraySequence(t, width, data)
		}
	})
}
