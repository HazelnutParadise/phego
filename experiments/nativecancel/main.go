//go:build phego_native_probe && cgo

// Package main is an isolated cooperative native cancellation experiment.
package main

/*
#include <stdint.h>
int32_t phego_cancel_run(uint64_t token, uint64_t limit);
*/
import "C"

import (
	"context"
	"math"
	"sync"
)

type control struct {
	ctx         context.Context
	observed    chan<- struct{}
	checkpoints uint64
}

var controls = struct {
	sync.Mutex
	byToken   map[uint64]*control
	next      uint64
	exhausted bool
}{byToken: make(map[uint64]*control), next: 1}

// registerControl retains ctx until release; zero indicates an invalid context.
// observed receives a nonblocking notification after an allowed checkpoint.
func registerControl(ctx context.Context, observed chan<- struct{}) uint64 {
	if ctx == nil {
		return 0
	}

	controls.Lock()
	defer controls.Unlock()
	if controls.exhausted {
		return 0
	}
	token := controls.next
	if token == 0 {
		return 0
	}
	if token == math.MaxUint64 {
		controls.exhausted = true
	} else {
		controls.next++
	}
	controls.byToken[token] = &control{ctx: ctx, observed: observed}
	return token
}

// releaseControl invalidates a live token. Tokens are never reused.
func releaseControl(token uint64) bool {
	if token == 0 {
		return false
	}
	controls.Lock()
	defer controls.Unlock()
	if _, ok := controls.byToken[token]; !ok {
		return false
	}
	delete(controls.byToken, token)
	return true
}

// checkpoints counts allowed checkpoints for a live token.
func checkpoints(token uint64) (uint64, bool) {
	controls.Lock()
	defer controls.Unlock()
	entry, ok := controls.byToken[token]
	if !ok {
		return 0, false
	}
	return entry.checkpoints, true
}

// nativeRun returns 0 on completion, 1 on cancellation, 2 for invalid tokens,
// or 3 when a Go checkpoint panics. Native code performs no external effects.
func nativeRun(token, limit uint64) int32 {
	return int32(C.phego_cancel_run(C.uint64_t(token), C.uint64_t(limit)))
}

//export phego_cancel_validate
func phego_cancel_validate(token C.uint64_t) (status C.int32_t) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil || !completed {
			status = 3
		}
	}()

	controls.Lock()
	defer controls.Unlock()
	if token == 0 || controls.byToken[uint64(token)] == nil {
		completed = true
		return 2
	}
	completed = true
	return 0
}

//export phego_cancel_checkpoint
func phego_cancel_checkpoint(token C.uint64_t) (status C.int32_t) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil || !completed {
			status = 3
		}
	}()

	key := uint64(token)
	controls.Lock()
	entry := controls.byToken[key]
	controls.Unlock()
	if key == 0 || entry == nil {
		completed = true
		return 2
	}

	// Context implementations are host code, so call Err without the registry lock.
	canceled := entry.ctx.Err() != nil

	controls.Lock()
	if controls.byToken[key] != entry {
		controls.Unlock()
		completed = true
		return 2
	}
	if canceled {
		controls.Unlock()
		completed = true
		return 1
	}
	if entry.checkpoints == math.MaxUint64 {
		controls.Unlock()
		panic("checkpoint counter overflow")
	}
	entry.checkpoints++
	observed := entry.observed
	controls.Unlock()

	if observed != nil {
		select {
		case observed <- struct{}{}:
		default:
		}
	}

	controls.Lock()
	stillValid := controls.byToken[key] == entry
	controls.Unlock()
	if !stillValid {
		completed = true
		return 2
	}
	completed = true
	return 0
}

func main() {}
