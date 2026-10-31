//go:build phego_native_probe && cgo

// The native handle probe checks the W-07 handle contract: native code holds
// only an owner and a generation-checked handle, while the Go value stays in a
// Go-owned table. It is an isolated experiment, not a PHP execution backend.
package main

/*
#include <stdint.h>
int32_t phego_native_weighted_sum(uint64_t owner, uint64_t handle, int64_t *result);
*/
import "C"

import (
	"fmt"
	"os"
	"sync"
)

// Status codes crossing the ABI.
const (
	statusOK        int32 = 0
	statusInvalid   int32 = 1 // unknown, released, stale, or foreign handle
	statusException int32 = 2 // the accessor raised a PHP-level error
	statusPanic     int32 = 3 // a Go panic was contained at the boundary
)

type slot struct {
	generation uint32
	owner      uint64
	live       bool
	retired    bool
	items      []int64
	failAt     int // index whose read raises statusException; -1 for none
	panicAt    int // index whose read panics; -1 for none
}

// table keeps every value referenced by a handle reachable from Go, so the Go
// collector cannot reclaim or move it while native code holds the handle.
var table struct {
	sync.Mutex
	slots []slot
	free  []uint32
}

// register stores a copy of items for owner and returns a nonzero handle
// formed from the slot index and its generation.
func register(owner uint64, items []int64) uint64 {
	if owner == 0 {
		return 0
	}
	table.Lock()
	defer table.Unlock()
	var index uint32
	if n := len(table.free); n > 0 {
		index = table.free[n-1]
		table.free = table.free[:n-1]
	} else {
		if len(table.slots) == 1<<32-1 {
			return 0
		}
		table.slots = append(table.slots, slot{})
		index = uint32(len(table.slots) - 1)
	}
	s := &table.slots[index]
	s.generation++
	s.owner, s.live = owner, true
	s.items = append([]int64(nil), items...)
	s.failAt, s.panicAt = -1, -1
	return uint64(index)<<32 | uint64(s.generation)
}

func lookup(owner, handle uint64) (*slot, bool) {
	index, generation := handle>>32, uint32(handle)
	if owner == 0 || generation == 0 || index >= uint64(len(table.slots)) {
		return nil, false
	}
	s := &table.slots[index]
	if !s.live || s.generation != generation || s.owner != owner {
		return nil, false
	}
	return s, true
}

// release invalidates a handle. A slot whose generation would wrap is retired
// instead of reused, so no old handle can become valid again.
func release(owner, handle uint64) bool {
	table.Lock()
	defer table.Unlock()
	s, ok := lookup(owner, handle)
	if !ok {
		return false
	}
	s.live, s.items = false, nil
	if s.generation == ^uint32(0) {
		s.retired = true
	} else {
		table.free = append(table.free, uint32(handle>>32))
	}
	return true
}

func inject(owner, handle uint64, failAt, panicAt int) bool {
	table.Lock()
	defer table.Unlock()
	s, ok := lookup(owner, handle)
	if ok {
		s.failAt, s.panicAt = failAt, panicAt
	}
	return ok
}

func contain(status *C.int32_t) {
	if recover() != nil {
		*status = C.int32_t(statusPanic)
	}
}

//export phegoHandleLen
func phegoHandleLen(owner, handle C.uint64_t, out *C.int64_t) (status C.int32_t) {
	defer contain(&status)
	table.Lock()
	defer table.Unlock()
	s, ok := lookup(uint64(owner), uint64(handle))
	if !ok || out == nil {
		return C.int32_t(statusInvalid)
	}
	*out = C.int64_t(len(s.items))
	return C.int32_t(statusOK)
}

//export phegoHandleAt
func phegoHandleAt(owner, handle, index C.uint64_t, out *C.int64_t) (status C.int32_t) {
	defer contain(&status)
	table.Lock()
	s, ok := lookup(uint64(owner), uint64(handle))
	if !ok || out == nil || uint64(index) >= uint64(len(s.items)) {
		table.Unlock()
		return C.int32_t(statusInvalid)
	}
	i := int(index)
	failAt, panicAt, item := s.failAt, s.panicAt, s.items[i]
	table.Unlock()
	if i == panicAt {
		panic("injected accessor panic")
	}
	if i == failAt {
		return C.int32_t(statusException)
	}
	*out = C.int64_t(item)
	return C.int32_t(statusOK)
}

func nativeWeightedSum(owner, handle uint64) (int64, int32) {
	var result C.int64_t
	status := C.phego_native_weighted_sum(C.uint64_t(owner), C.uint64_t(handle), &result)
	return int64(result), int32(status)
}

func weightedSum(items []int64) int64 {
	var sum int64
	for i, item := range items {
		sum += item * int64(i+1)
	}
	return sum
}

func main() {
	const owner = 0x2001
	items := []int64{3, -1, 4, 1, -5}
	handle := register(owner, items)
	got, status := nativeWeightedSum(owner, handle)
	if status != statusOK || got != weightedSum(items) {
		fmt.Fprintf(os.Stderr, "error: native weighted sum = (%d, %d); expected (%d, 0)\n", got, status, weightedSum(items))
		os.Exit(1)
	}
	release(owner, handle)
	if _, status := nativeWeightedSum(owner, handle); status != statusInvalid {
		fmt.Fprintf(os.Stderr, "error: released handle returned status %d\n", status)
		os.Exit(1)
	}
	fmt.Println("native handle probe: weighted sum and release passed")
}
