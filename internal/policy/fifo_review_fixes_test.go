//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package policy

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syscall.Mkfifo is the OS primitive; the Go os package has no Mkfifo.
func TestReviewFixFIFOOpenDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	done := make(chan error, 1)
	go func() {
		f, err := p.OpenFile("/app/pipe", os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		var pe *PolicyError
		if !errors.As(err, &pe) || pe.Target != "/app/pipe" {
			t.Fatalf("want controlled PHP-path denial, got %v", err)
		}
	case <-time.After(time.Second):
		// Unblock the old implementation before failing, so no goroutine leaks.
		fd, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			syscall.Close(fd)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("FIFO open and cleanup blocked")
		}
		t.Fatal("FIFO open blocked without a writer")
	}
}

func TestReviewFixRootRejectsDeviceFiles(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev"}}})
	f, err := p.OpenFile("/devices/null", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
}

// Keep permanent links to both inodes so a timed-out old implementation can
// be released without guessing which renamed path its open is waiting on.
func TestReviewFixFIFOSwapDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(dir+"/pipe", 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/regular", []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(dir+"/regular", dir+"/target"); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}}})
	stop := make(chan struct{})
	swapError := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			source := dir + "/regular"
			if i%2 == 0 {
				source = dir + "/pipe"
			}
			if err := os.Link(source, dir+"/next"); err != nil {
				swapError <- err
				return
			}
			if err := os.Rename(dir+"/next", dir+"/target"); err != nil {
				swapError <- err
				return
			}
		}
	}()
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 500; i++ {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			flags := []int{os.O_RDONLY, os.O_WRONLY, os.O_RDWR | os.O_TRUNC, os.O_RDONLY | os.O_TRUNC}
			f, err := p.OpenFile("/app/target", flags[i%len(flags)], 0600)
			if err != nil {
				continue
			}
			st, err := f.Stat()
			f.Close()
			if err != nil {
				done <- err
				return
			}
			if !st.Mode().IsRegular() {
				done <- errors.New("returned a special file")
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		close(stop)
		wg.Wait()
		fd, err := syscall.Open(dir+"/pipe", syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			syscall.Close(fd)
			t.Fatal("swap cleanup blocked")
		}
		syscall.Close(fd)
		t.Fatal("open blocked on a swapped-in FIFO")
	}
	select {
	case err := <-swapError:
		t.Fatal(err)
	default:
	}
}
