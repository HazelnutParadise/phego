//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package phego

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReviewFixRunFileFIFOReleasesAdmission(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "pipe.php")
	if err := syscall.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	e := newPolicyEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir}}}})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := e.RunFile(ctx, "/app/pipe.php"); done <- err }()
	select {
	case err := <-done:
		var pe *PolicyError
		if !errors.As(err, &pe) || pe.Target != "/app/pipe.php" {
			t.Fatalf("want controlled denial, got %v", err)
		}
	case <-time.After(time.Second):
		fd, err := syscall.Open(name, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if err == nil {
			syscall.Close(fd)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("RunFile cleanup blocked")
		}
		t.Error("RunFile remained blocked after cancellation")
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("FIFO retained admission")
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.php"), []byte("<?php return 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := e.RunFile(context.Background(), "/app/ok.php")
	var ue *UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("next file cannot run: %v", err)
	}
}
