//go:build phego_native_probe && cgo

package main

import (
	"context"
	"testing"
)

type nilPanicContext struct{ context.Context }

func (nilPanicContext) Err() error { panic(nil) }

func TestNilPanicCannotBeReportedAsSuccess(t *testing.T) {
	token := registerControl(nilPanicContext{context.Background()}, nil)
	if token == 0 {
		t.Fatal("registration failed")
	}
	defer releaseControl(token)
	if status := nativeRun(token, 1); status != 3 {
		t.Fatalf("nativeRun after panic(nil) = %d, want 3", status)
	}
}
