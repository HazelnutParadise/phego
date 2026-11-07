package runtime

import (
	"fmt"
	goruntime "runtime"
	"testing"
)

// PHP 8.5.12 (pinned arm64 image) prints key='b' v=2 when the destructor run
// while foreach moves to 'b' unsets 'b': FE_FETCH_RW writes the key before
// the reference assignment that releases the removed value.
func TestReviewForeachRefKeyAfterDestructorRemovesNext(t *testing.T) {
	h := testHeap(t)
	var arr, item value
	o, err := h.newObject(&class{name: "D", destructor: func(h *heap, _ value) error {
		return h.unsetElement(&arr, str("b"))
	}})
	must(t, err)
	must(t, h.setElement(&arr, str("a"), o))
	must(t, h.setElement(&arr, str("b"), num(t, 2)))
	must(t, h.setElement(&arr, str("c"), num(t, 3)))
	var got []string
	must(t, h.foreachRef(&arr, &item, func(k arrayKey) error {
		got = append(got, fmt.Sprintf("%v:%s%d", k.isString, k.text, k.integer))
		if k.isString && k.text == "a" {
			return h.unsetElement(&arr, str("a"))
		}
		return nil
	}))
	if want := []string{"true:a0", "true:b0", "true:c0"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("keys %v, PHP gives %v", got, want)
	}
}

// A freed array's buckets live in chunks shared with other arrays, so each
// bucket must be cleared; otherwise a neighbour keeps the freed values alive.
func TestReviewFreedArrayClearsChunkBuckets(t *testing.T) {
	h := testHeap(t)
	var keep, tmp value
	must(t, h.setElement(&tmp, str("k"), str("large value")))
	must(t, h.setElement(&keep, num(t, 0), num(t, 1)))
	b := tmp.array.buckets[0]
	must(t, h.release(tmp))
	if *b != (bucket{}) {
		t.Fatalf("freed bucket still holds %+v", *b)
	}
	_ = keep
}

// An embedded heap must behave as PHP 8.5.12 does on the same host; amd64 and
// arm64 initialize lazy comparison operands in opposite orders.
func TestReviewDefaultHeapFollowsHostArchitecture(t *testing.T) {
	h, err := newHeap(64)
	must(t, err)
	want := "arm64"
	if goruntime.GOARCH == "amd64" {
		want = "amd64"
	}
	if h.arch != want {
		t.Fatalf("64-bit heap on %s uses %s, want %s", goruntime.GOARCH, h.arch, want)
	}
	if h, err = newHeap(32); err != nil || h.arch != "386" {
		t.Fatalf("32-bit heap arch %q err %v, want 386", h.arch, err)
	}
}
