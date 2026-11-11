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

// An embedded heap must behave as PHP 8.5.12 does on the same host; the
// architecture and the system both decide the order in which lazy
// comparison operands are initialized (TestHostProfileLazyOperandOrder
// compares the order itself with PHP on macOS and Windows).
func TestReviewDefaultHeapFollowsHostProfile(t *testing.T) {
	h, err := newHeap(64)
	must(t, err)
	want := "arm64"
	if goruntime.GOARCH == "amd64" {
		want = "amd64"
	}
	if h.arch != want {
		t.Fatalf("64-bit heap on %s uses %s, want %s", goruntime.GOARCH, h.arch, want)
	}
	system := "linux"
	if goruntime.GOOS == "darwin" || goruntime.GOOS == "windows" && want == "amd64" {
		system = goruntime.GOOS
	}
	if h.os != system {
		t.Fatalf("64-bit heap on %s/%s uses %s, want %s", goruntime.GOOS, goruntime.GOARCH, h.os, system)
	}
	if h, err = newHeap(32); err != nil || h.arch != "386" || h.os != "linux" {
		t.Fatalf("32-bit heap profile %s/%s err %v, want linux/386", h.os, h.arch, err)
	}
	for _, c := range []struct {
		arch, system string
		rightFirst   bool
	}{{"arm64", "linux", false}, {"amd64", "linux", true}, {"arm64", "darwin", false}, {"amd64", "darwin", false}, {"amd64", "windows", true}} {
		h, err := newHeapFor(64, c.arch, c.system)
		must(t, err)
		events := w13ReviewCase(t, h, "compare-lazy-operand-owners").Result.([]any)[1].([]any)
		if got := events[0] == "init:B"; got != c.rightFirst {
			t.Errorf("%s/%s: events %v, right operand first = %v, want %v", c.system, c.arch, events, got, c.rightFirst)
		}
	}
	if _, err := newHeapFor(64, "arm64", "windows"); err == nil {
		t.Error("windows/arm64 has no observed PHP 8.5.12 build but got a profile")
	}
}
