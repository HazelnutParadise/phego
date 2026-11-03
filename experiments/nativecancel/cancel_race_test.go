//go:build phego_native_probe && cgo

package main

import (
	"context"
	"testing"
	"time"
)

type blockingContext struct {
	context.Context
	entered chan struct{}
	resume  chan struct{}
}

func (c blockingContext) Err() error {
	close(c.entered)
	<-c.resume
	return nil
}

func TestReleaseDuringContextCheckDoesNotDeadlockOrSucceed(t *testing.T) {
	ctx := blockingContext{Context: context.Background(), entered: make(chan struct{}), resume: make(chan struct{})}
	token := registerControl(ctx, nil)
	if token == 0 {
		t.Fatal("registration failed")
	}
	result := make(chan int32, 1)
	go func() { result <- nativeRun(token, 1) }()
	select {
	case <-ctx.entered:
	case <-time.After(callTimeout):
		t.Fatal("context check was not reached")
	}
	released := make(chan bool, 1)
	go func() { released <- releaseControl(token) }()
	select {
	case ok := <-released:
		if !ok {
			t.Fatal("release failed")
		}
	case <-time.After(callTimeout):
		close(ctx.resume)
		t.Fatal("release blocked while context check was running")
	}
	close(ctx.resume)
	if status := awaitResult(t, result); status != 2 {
		t.Fatalf("status after release = %d, want 2", status)
	}
}
