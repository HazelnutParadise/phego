//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package phego

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// probeOpen opens path for reading the way PHP's open(2) does, without
// waiting, and returns the kernel's error.
func probeOpen(path string) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err == nil {
		syscall.Close(fd)
	}
	return err
}

type runResult struct {
	res *Result
	err error
}

func runFileAsync(ctx context.Context, e *Engine, path string) <-chan runResult {
	done := make(chan runResult, 1)
	go func() {
		res, err := e.RunFile(ctx, path)
		done <- runResult{res, err}
	}()
	return done
}

func runFinished(t *testing.T, done <-chan runResult) runResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("RunFile still waiting")
		return runResult{}
	}
}

// php pipe.php (PHP 8.5.12 CLI) waits for a writer and runs what it writes.
func TestRunFileFIFORunsWriterSource(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "pipe.php")
	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Fatal(err)
	}
	e := newPolicyEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir}}}})
	done := runFileAsync(t.Context(), e, "/app/pipe.php")
	select {
	case r := <-done:
		t.Fatalf("RunFile returned without a writer: %+v, %v", r.res, r.err)
	case <-time.After(200 * time.Millisecond):
	}
	var w int
	for deadline := time.Now().Add(5 * time.Second); ; {
		fd, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err == nil {
			w = fd
			break
		}
		if !errors.Is(err, syscall.ENXIO) || time.Now().After(deadline) {
			t.Fatalf("open writer: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := syscall.Write(w, []byte("<?php return 1;")); err != nil {
		t.Fatal(err)
	}
	syscall.Close(w)
	r := runFinished(t, done)
	var ue *UnsupportedError
	if !errors.As(r.err, &ue) || ue.Feature != "PHP execution" {
		t.Fatalf("RunFile = %+v, %v; want the FIFO source compiled", r.res, r.err)
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("FIFO retained admission")
	}
}

// A host stop ends a FIFO wait: RunFile reports the cancellation and frees
// its slot for the next run.
func TestRunFileFIFOCancelReleasesAdmission(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.php"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newPolicyEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir}}}})
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	r := runFinished(t, runFileAsync(ctx, e, "/app/pipe.php"))
	if r.res == nil || r.res.Status != StatusCancelled || !errors.Is(r.err, context.DeadlineExceeded) {
		t.Fatalf("RunFile = %+v, %v; want StatusCancelled", r.res, r.err)
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("FIFO retained admission")
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.php"), []byte("<?php return 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := e.RunFile(t.Context(), "/app/ok.php")
	var ue *UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("next file cannot run: %v", err)
	}
}

// php socket (PHP 8.5.12 CLI) prints "Could not open input file": open(2)
// fails. RunFile reports the same kernel error instead of a policy refusal.
func TestRunFileSocketFailsLikeKernel(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "phego-sock")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	l, err := net.Listen("unix", filepath.Join(dir, "s.php"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, want := syscall.Open(filepath.Join(dir, "s.php"), syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if want == nil {
		t.Skip("this kernel opens sockets")
	}
	e := newPolicyEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir}}}})
	res, err := e.RunFile(t.Context(), "/app/s.php")
	var pe *PolicyError
	if res == nil || res.Status != StatusFailed || errors.As(err, &pe) || !errors.Is(err, want) {
		t.Fatalf("RunFile = %+v, %v; want StatusFailed with %v", res, err, want)
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("socket retained admission")
	}
}
