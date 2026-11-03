//go:build phego_native_probe && cgo

package main

import (
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
)

func registered(t *testing.T, owner uint64, items []int64) uint64 {
	t.Helper()
	h := register(owner, items)
	if h == 0 {
		t.Fatal("register returned the reserved zero handle")
	}
	t.Cleanup(func() { release(owner, h) })
	return h
}

func TestWeightedSumObservesElementOrder(t *testing.T) {
	const owner = 0x3001
	for _, items := range [][]int64{nil, {7}, {1, 2, 3}, {3, 2, 1}, {-9, 1 << 40, -(1 << 40), 5}} {
		h := registered(t, owner, items)
		got, status := nativeWeightedSum(owner, h)
		if status != statusOK || got != weightedSum(items) {
			t.Fatalf("items %v: got (%d, %d), want (%d, 0)", items, got, status, weightedSum(items))
		}
	}
	a, _ := nativeWeightedSum(owner, registered(t, owner, []int64{1, 2, 3}))
	b, _ := nativeWeightedSum(owner, registered(t, owner, []int64{3, 2, 1}))
	if a == b {
		t.Fatal("reordered elements produced the same result")
	}
}

func TestRegisteredValueIsCopied(t *testing.T) {
	const owner = 0x3002
	items := []int64{1, 2}
	h := registered(t, owner, items)
	items[0] = 100
	if got, _ := nativeWeightedSum(owner, h); got != 5 {
		t.Fatalf("caller mutation reached the handle value: %d", got)
	}
}

func TestInvalidHandlesAreRejectedWithoutWritingResult(t *testing.T) {
	const owner = 0x3003
	h := registered(t, owner, []int64{1})
	for name, c := range map[string]struct{ owner, handle uint64 }{
		"zero owner":     {0, h},
		"foreign owner":  {owner + 1, h},
		"zero handle":    {owner, 0},
		"zero gen":       {owner, h &^ 0xffffffff},
		"wrong gen":      {owner, h + 1},
		"unknown index":  {owner, uint64(1<<31)<<32 | 1},
		"maximum handle": {owner, ^uint64(0)},
	} {
		got, status := nativeWeightedSum(c.owner, c.handle)
		if status != statusInvalid || got != 0 {
			t.Errorf("%s: got (%d, %d), want (0, %d)", name, got, status, statusInvalid)
		}
	}
	if register(0, nil) != 0 {
		t.Fatal("register accepted the zero owner")
	}
}

func TestReleasedSlotReuseInvalidatesOldGeneration(t *testing.T) {
	const owner = 0x3004
	old := register(owner, []int64{1})
	if !release(owner, old) || release(owner, old) {
		t.Fatal("release must succeed once")
	}
	fresh := registered(t, owner, []int64{2})
	if fresh>>32 != old>>32 || fresh == old {
		t.Fatalf("expected the slot to be reused with a new generation: old=%#x new=%#x", old, fresh)
	}
	if _, status := nativeWeightedSum(owner, old); status != statusInvalid {
		t.Fatalf("stale handle status = %d", status)
	}
	if got, status := nativeWeightedSum(owner, fresh); status != statusOK || got != 2 {
		t.Fatalf("fresh handle = (%d, %d)", got, status)
	}
}

func TestExhaustedGenerationRetiresSlot(t *testing.T) {
	const owner = 0x3005
	h := register(owner, nil)
	index := h >> 32
	table.Lock()
	table.slots[index].generation = ^uint32(0)
	table.Unlock()
	last := index<<32 | uint64(^uint32(0))
	if !release(owner, last) {
		t.Fatal("release at the final generation failed")
	}
	next := registered(t, owner, nil)
	if next>>32 == index {
		t.Fatal("retired slot was reused")
	}
}

func TestExceptionStatusPropagatesWithoutResult(t *testing.T) {
	const owner = 0x3006
	h := registered(t, owner, []int64{1, 2, 3})
	if !inject(owner, h, 1, -1) {
		t.Fatal("inject failed")
	}
	got, status := nativeWeightedSum(owner, h)
	if status != statusException || got != 0 {
		t.Fatalf("got (%d, %d), want (0, %d)", got, status, statusException)
	}
}

func TestGoPanicIsContainedAtBoundary(t *testing.T) {
	const owner = 0x3007
	h := registered(t, owner, []int64{1, 2})
	inject(owner, h, -1, 0)
	if got, status := nativeWeightedSum(owner, h); status != statusPanic || got != 0 {
		t.Fatalf("got (%d, %d), want (0, %d)", got, status, statusPanic)
	}
	inject(owner, h, -1, -1)
	if got, status := nativeWeightedSum(owner, h); status != statusOK || got != 5 {
		t.Fatalf("handle unusable after contained panic: (%d, %d)", got, status)
	}
}

// Only the table keeps these values alive. Forced collections and allocation
// churn run while native code reads them; every result must stay exact.
func TestValuesSurviveGCPressureWhileNativeReads(t *testing.T) {
	const (
		owner   = 0x3008
		workers = 8
		rounds  = 200
	)
	defer debug.SetGCPercent(debug.SetGCPercent(1))
	handles := make([]uint64, workers)
	want := make([]int64, workers)
	for w := range handles {
		items := make([]int64, 64)
		for i := range items {
			items[i] = int64(w*1000 + i)
		}
		handles[w] = registered(t, owner, items)
		want[w] = weightedSum(items)
	}
	stop := make(chan struct{})
	var churn sync.WaitGroup
	churn.Add(1)
	go func() {
		defer churn.Done()
		var sink [][]byte
		for {
			select {
			case <-stop:
				return
			default:
				sink = append(sink, make([]byte, 1<<14))
				if len(sink) > 64 {
					sink = nil
					runtime.GC()
				}
			}
		}
	}()
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for w := range handles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if got, status := nativeWeightedSum(owner, handles[w]); status != statusOK || got != want[w] {
					errs <- "unexpected result under GC pressure"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	churn.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func TestConcurrentReleaseNeverReturnsPartialResult(t *testing.T) {
	const owner = 0x3009
	items := []int64{5, 6, 7, 8}
	h := register(owner, items)
	start := make(chan struct{})
	var wg sync.WaitGroup
	bad := make(chan [2]int64, 64)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 64; i++ {
				got, status := nativeWeightedSum(owner, h)
				if !(status == statusOK && got == weightedSum(items)) && !(status == statusInvalid && got == 0) {
					bad <- [2]int64{got, int64(status)}
				}
			}
		}()
	}
	close(start)
	runtime.Gosched()
	if !release(owner, h) {
		t.Fatal("release failed")
	}
	wg.Wait()
	close(bad)
	for b := range bad {
		t.Errorf("result during release = %d status %d", b[0], b[1])
	}
}
