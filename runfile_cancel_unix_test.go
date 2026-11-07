//go:build unix

package phego

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// php fifo waits for the writer, as RunFile does for a FIFO an fs.FS mount
// or a standalone Files opens. The read still ends when ctx does: RunFile
// reports the cancellation and frees the admission slot instead of waiting
// for the writer.
func TestRunFileFIFOReadEndsWithContext(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.php")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	for _, legacy := range []bool{false, true} {
		// A writer that stays open without writing keeps a reader waiting.
		writer, err := os.OpenFile(fifo, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		cfg := Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", FS: os.DirFS(dir)}}}}
		name := "/app/pipe.php"
		if legacy {
			cfg = Config{MaxActive: 1, Files: os.DirFS(dir)}
			name = "pipe.php"
		}
		e := newPolicyEngine(t, cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		done := make(chan struct{})
		var res *Result
		go func() {
			defer close(done)
			res, err = e.RunFile(ctx, name)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			writer.Close()
			<-done
			t.Fatalf("legacy=%v: RunFile kept waiting for the FIFO writer after its context ended", legacy)
		}
		cancel()
		writer.Close()
		if res == nil || res.Status != StatusCancelled || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("legacy=%v: RunFile = %+v, %v; want StatusCancelled with the deadline", legacy, res, err)
		}
		if a := e.policy.admission; a != nil && a.Active() != 0 {
			t.Errorf("legacy=%v: admission slot leaked", legacy)
		}
	}
}

// A cancelled FIFO read must not keep reading: the next RunFile on the same
// FIFO gets the whole script the writer sends, here a parse error, and no
// reader goroutine is left behind.
func TestRunFileCancelledFIFOReadLeavesNoReader(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.php")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	writer, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	e := newPolicyEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", FS: os.DirFS(dir)}}}})
	before := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		if res, err := e.RunFile(ctx, "/app/pipe.php"); res == nil || res.Status != StatusCancelled {
			t.Fatalf("cancelled read %d: %+v, %v", i, res, err)
		}
		cancel()
	}
	time.Sleep(50 * time.Millisecond)
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("goroutines %d after cancelled reads, %d before", n, before)
	}
	done := make(chan error, 1)
	go func() {
		_, err := e.RunFile(context.Background(), "/app/pipe.php")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := writer.Write([]byte("<?php if (")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	select {
	case err := <-done:
		var pe *PHPError
		if !errors.As(err, &pe) {
			t.Errorf("second RunFile = %v, want the parse error of the whole script", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second RunFile did not finish after the writer closed")
	}
}
