//go:build phego_native_probe && cgo

package main

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
)

func failOnUnexpectedPanic(t *testing.T) {
	t.Helper()
	if value := recover(); value != nil {
		t.Errorf("unexpected panic: %v", value)
	}
}

func TestRegisterReturnsUniqueNonzeroToken(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1001)
	first := register(owner, false)
	if first == 0 {
		t.Fatal("register returned the reserved zero token")
	}
	if !release(owner, first) {
		t.Fatal("release of the registered token failed")
	}
	second := register(owner, false)
	if second == 0 {
		t.Fatal("register returned the reserved zero token after release")
	}
	if second == first {
		t.Fatalf("released token was reused: %d", first)
	}
	if !release(owner, second) {
		t.Fatal("release of the second token failed")
	}
}

func TestRegisterRejectsZeroOwner(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	if token := register(0, false); token != 0 {
		t.Fatalf("register(0) = %d, want 0", token)
	}
}

func TestRegisterRejectsTokenExhaustionInSubprocess(t *testing.T) {
	const childEnv = "PHEGO_NATIVE_CALLBACK_EXHAUSTION_CHILD"
	if os.Getenv(childEnv) == "1" {
		callbackRegistry.Lock()
		callbackRegistry.nextToken = ^uint64(0) - 1
		callbackRegistry.exhausted = false
		callbackRegistry.Unlock()

		const owner = uint64(0x1008)
		penultimate := register(owner, false)
		last := register(owner, false)
		if penultimate != ^uint64(0)-1 || last != ^uint64(0) || penultimate == last {
			t.Fatalf("boundary tokens = (%d, %d), want distinct final nonzero tokens (%d, %d)", penultimate, last, ^uint64(0)-1, ^uint64(0))
		}
		if got := register(owner, false); got != 0 {
			t.Fatalf("register after token exhaustion = %d, want 0", got)
		}
		if !release(owner, penultimate) || !release(owner, last) {
			t.Fatal("failed to release final live tokens")
		}
		if got := register(owner, false); got != 0 {
			t.Fatalf("register reused an exhausted token after release: %d", got)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRegisterRejectsTokenExhaustionInSubprocess$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("token exhaustion subprocess failed: %v\n%s", err, output)
	}
}

func TestNativeInvokeCountsValidCalls(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1002)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	if got := nativeInvoke(owner, token); got != 0 {
		t.Fatalf("nativeInvoke = %d, want success (0)", got)
	}
	if got, ok := calls(owner, token); !ok || got != 1 {
		t.Fatalf("calls = (%d, %t), want (1, true)", got, ok)
	}
	if got := nativeInvoke(owner, token); got != 0 {
		t.Fatalf("second nativeInvoke = %d, want success (0)", got)
	}
	if got, ok := calls(owner, token); !ok || got != 2 {
		t.Fatalf("calls after second invocation = (%d, %t), want (2, true)", got, ok)
	}
}

func TestNativeInvokeConcurrentCallsAreCounted(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const (
		owner      = uint64(0x1007)
		workers    = 8
		callsEach  = 16
		totalCalls = workers * callsEach
	)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	type result struct {
		status int32
		panic  any
	}
	results := make(chan result, totalCalls)
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for i := 0; i < callsEach; i++ {
				var result result
				func() {
					defer func() { result.panic = recover() }()
					result.status = nativeInvoke(owner, token)
				}()
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)

	completed := 0
	for result := range results {
		completed++
		if result.panic != nil {
			t.Errorf("nativeInvoke panicked: %v", result.panic)
		}
		if result.status != 0 {
			t.Errorf("nativeInvoke status = %d, want success (0)", result.status)
		}
	}
	if completed != totalCalls {
		t.Fatalf("completed calls = %d, want %d", completed, totalCalls)
	}
	if got, ok := calls(owner, token); !ok || got != totalCalls {
		t.Fatalf("calls = (%d, %t), want (%d, true)", got, ok, totalCalls)
	}
}

func TestConcurrentReleaseAndNativeInvoke(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const (
		owner     = uint64(0x1009)
		workers   = 8
		callsEach = 32
	)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	start := make(chan struct{})
	type result struct {
		status int32
		panic  any
	}
	results := make(chan result, workers*callsEach)
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < callsEach; i++ {
				var result result
				func() {
					defer func() { result.panic = recover() }()
					result.status = nativeInvoke(owner, token)
				}()
				results <- result
			}
		}()
	}
	close(start)
	released := release(owner, token)
	wg.Wait()
	close(results)
	if !released {
		t.Fatal("release of the live token failed")
	}
	for result := range results {
		if result.panic != nil {
			t.Errorf("nativeInvoke panicked during concurrent release: %v", result.panic)
		}
		if result.status != 0 && result.status != 1 {
			t.Errorf("nativeInvoke during concurrent release = %d, want 0 or 1", result.status)
		}
	}
	if got := nativeInvoke(owner, token); got != 1 {
		t.Fatalf("nativeInvoke after release completed = %d, want invalid (1)", got)
	}
}

func TestNativeInvokeRejectsInvalidOwnerAndTokensWithoutCounting(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1003)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	if got := nativeInvoke(owner+1, token); got != 1 {
		t.Errorf("nativeInvoke with wrong owner = %d, want invalid (1)", got)
	}
	if got := nativeInvoke(owner, 0); got != 1 {
		t.Errorf("nativeInvoke with zero token = %d, want invalid (1)", got)
	}
	if got, ok := calls(owner, token); !ok || got != 0 {
		t.Fatalf("invalid calls changed valid token count: calls = (%d, %t), want (0, true)", got, ok)
	}
	if !release(owner, token) {
		t.Fatal("release of registered token failed")
	}
	if got := nativeInvoke(owner, token); got != 1 {
		t.Errorf("nativeInvoke with released token = %d, want invalid (1)", got)
	}
}

func TestReleaseRequiresMatchingOwnerAndRejectsRepeat(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1004)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	if release(owner+1, token) {
		t.Fatal("release succeeded for the wrong owner")
	}
	if got := nativeInvoke(owner, token); got != 0 {
		t.Fatalf("wrong-owner release invalidated token: nativeInvoke = %d, want success (0)", got)
	}
	if !release(owner, token) {
		t.Fatal("release failed for the matching owner")
	}
	if release(owner, token) {
		t.Fatal("repeated release succeeded")
	}
}

func TestNativeInvokeRecoversCallbackPanicAndRemainsUsable(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1005)
	token := register(owner, true)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	if got := nativeInvoke(owner, token); got != 2 {
		t.Fatalf("nativeInvoke for panicking callback = %d, want recovered panic (2)", got)
	}
	if got, ok := calls(owner, token); !ok || got != 0 {
		t.Fatalf("panic changed successful-call count: calls = (%d, %t), want (0, true)", got, ok)
	}
	if got := nativeInvoke(owner, token); got != 2 {
		t.Fatalf("nativeInvoke after recovered panic = %d, want recovered panic (2)", got)
	}
}

func TestNativeInvokeKeepsCallbackAliveAcrossGC(t *testing.T) {
	defer failOnUnexpectedPanic(t)
	const owner = uint64(0x1006)
	token := register(owner, false)
	if token == 0 {
		t.Fatal("register returned zero token")
	}
	t.Cleanup(func() { release(owner, token) })

	runtime.GC()
	if got := nativeInvoke(owner, token); got != 0 {
		t.Fatalf("nativeInvoke after runtime.GC = %d, want success (0)", got)
	}
	if got, ok := calls(owner, token); !ok || got != 1 {
		t.Fatalf("calls after GC invocation = (%d, %t), want (1, true)", got, ok)
	}
}
