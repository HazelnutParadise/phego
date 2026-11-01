//go:build phego_native_probe && cgo

// Package main is an isolated native-to-Go callback experiment.
package main

/*
#include <stdint.h>
int32_t phego_native_invoke(uint64_t owner, uint64_t token);
*/
import "C"

import "sync"

type callbackRegistration struct {
	owner       uint64
	panicOnCall bool
	calls       int
}

var callbackRegistry = struct {
	sync.Mutex
	nextToken uint64
	exhausted bool
	entries   map[uint64]callbackRegistration
}{
	nextToken: 1,
	entries:   make(map[uint64]callbackRegistration),
}

// register gives a nonzero token for an owner. Tokens are not reused.
// A zero owner is rejected with token zero.
func register(owner uint64, panicOnCall bool) uint64 {
	if owner == 0 {
		return 0
	}

	callbackRegistry.Lock()
	defer callbackRegistry.Unlock()
	if callbackRegistry.exhausted {
		return 0
	}

	token := callbackRegistry.nextToken
	if token == ^uint64(0) {
		callbackRegistry.exhausted = true
	} else {
		callbackRegistry.nextToken++
	}
	callbackRegistry.entries[token] = callbackRegistration{
		owner:       owner,
		panicOnCall: panicOnCall,
	}
	return token
}

// release succeeds only for the live token and its owner.
func release(owner, token uint64) bool {
	callbackRegistry.Lock()
	defer callbackRegistry.Unlock()
	entry, ok := callbackRegistry.entries[token]
	if !ok || entry.owner != owner {
		return false
	}
	delete(callbackRegistry.entries, token)
	return true
}

// calls reports the number of successful calls while the token is live.
func calls(owner, token uint64) (int, bool) {
	callbackRegistry.Lock()
	defer callbackRegistry.Unlock()
	entry, ok := callbackRegistry.entries[token]
	if !ok || entry.owner != owner {
		return 0, false
	}
	return entry.calls, true
}

// nativeInvoke enters LLVM native code which calls back into Go.
// 0: success, 1: invalid or stale owner/token, 2: recovered callback panic.
func nativeInvoke(owner, token uint64) int32 {
	return int32(C.phego_native_invoke(C.uint64_t(owner), C.uint64_t(token)))
}

//export phego_go_callback
func phego_go_callback(owner C.uint64_t, token C.uint64_t) (status C.int32_t) {
	defer func() {
		if recover() != nil {
			status = 2
		}
	}()

	callbackRegistry.Lock()
	defer callbackRegistry.Unlock()
	key := uint64(token)
	entry, ok := callbackRegistry.entries[key]
	if !ok || entry.owner != uint64(owner) {
		return 1
	}
	if entry.panicOnCall {
		panic("requested callback panic")
	}
	entry.calls++
	callbackRegistry.entries[key] = entry
	return 0
}

func main() {}
