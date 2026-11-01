//go:build phego_native_probe && cgo

package main

import (
	"context"
	"math"
	"runtime"
	"testing"
	"time"
)

const callTimeout = 5 * time.Second

func awaitResult(t *testing.T, result <-chan int32) int32 {
	t.Helper()
	select {
	case status := <-result:
		return status
	case <-time.After(callTimeout):
		t.Fatal("native call did not finish within five seconds")
		return -1
	}
}

func TestRegisterNilContextReturnsZero(t *testing.T) {
	if token := registerControl(nil, nil); token != 0 {
		t.Fatalf("registerControl(nil) = %d, want 0", token)
	}
}

func TestNormalCompletionCountsEveryCheckpoint(t *testing.T) {
	token := registerControl(context.Background(), nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 3) }()
	if got := awaitResult(t, result); got != 0 {
		t.Fatalf("nativeRun status = %d, want 0", got)
	}
	if got, ok := checkpoints(token); !ok || got != 3 {
		t.Fatalf("checkpoints = (%d, %t), want (3, true)", got, ok)
	}
}

func TestZeroIterationsDoNotCheckpoint(t *testing.T) {
	token := registerControl(context.Background(), nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 0) }()
	if got := awaitResult(t, result); got != 0 {
		t.Fatalf("nativeRun status = %d, want 0", got)
	}
	if got, ok := checkpoints(token); !ok || got != 0 {
		t.Fatalf("checkpoints = (%d, %t), want (0, true)", got, ok)
	}
}

func TestInvalidTokenIsRejectedBeforeLimit(t *testing.T) {
	if got := nativeRun(0, 0); got != 2 {
		t.Fatalf("nativeRun(invalid, 0) = %d, want 2", got)
	}
}

func TestReleasedTokenIsRejectedBeforeLimitAndReleaseIsNotRepeated(t *testing.T) {
	token := registerControl(context.Background(), nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	if !releaseControl(token) {
		t.Fatal("first releaseControl returned false")
	}
	if releaseControl(token) {
		t.Fatal("repeated releaseControl returned true")
	}
	if got := nativeRun(token, 0); got != 2 {
		t.Fatalf("nativeRun(released, 0) = %d, want 2", got)
	}
}

func TestAlreadyCanceledContextStopsWithoutCheckpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	token := registerControl(ctx, nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 3) }()
	if got := awaitResult(t, result); got != 1 {
		t.Fatalf("nativeRun status = %d, want 1", got)
	}
	if got, ok := checkpoints(token); !ok || got != 0 {
		t.Fatalf("checkpoints = (%d, %t), want (0, true)", got, ok)
	}
}

func TestCancellationAfterFirstCheckpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	observed := make(chan struct{}, 1)
	token := registerControl(ctx, observed)
	if token == 0 {
		cancel()
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, math.MaxUint64) }()

	select {
	case <-observed:
		cancel()
	case <-time.After(callTimeout):
		cancel()
		select {
		case <-result:
		case <-time.After(callTimeout):
			t.Fatal("native call remained blocked after cancellation")
		}
		t.Fatal("no checkpoint notification arrived within five seconds")
	}

	if got := awaitResult(t, result); got != 1 {
		t.Fatalf("nativeRun status = %d, want 1", got)
	}
	if got, ok := checkpoints(token); !ok || got == 0 || got >= math.MaxUint64 {
		t.Fatalf("checkpoints = (%d, %t), want a positive count below the limit", got, ok)
	}
}

type panicErrContext struct{}

func (panicErrContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (panicErrContext) Done() <-chan struct{}       { return nil }
func (panicErrContext) Err() error                  { panic("Err panic must stay inside Go") }
func (panicErrContext) Value(any) any               { return nil }

func TestErrPanicIsReturnedAsStatusThree(t *testing.T) {
	token := registerControl(panicErrContext{}, nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 1) }()
	if got := awaitResult(t, result); got != 3 {
		t.Fatalf("nativeRun status = %d, want 3", got)
	}
}

func TestRuntimeGCKeepsRegisteredContextAlive(t *testing.T) {
	token := registerControl(context.Background(), nil)
	if token == 0 {
		t.Fatal("registerControl returned invalid token")
	}
	defer releaseControl(token)
	runtime.GC()

	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 1) }()
	if got := awaitResult(t, result); got != 0 {
		t.Fatalf("nativeRun status after runtime.GC = %d, want 0", got)
	}
}
