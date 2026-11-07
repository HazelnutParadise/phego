//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package policy

import (
	"errors"
	"io"
	"net"
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

// PHP reads and writes character devices such as /dev/null and /dev/urandom.
// They open without waiting, so a granted device behaves like PHP's.
func TestRootAllowsCharacterDevices(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev", Write: true}}})
	f, err := p.OpenFile("/devices/null", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.Read(make([]byte, 8)); n != 0 || err != io.EOF {
		t.Fatalf("read /dev/null: %d, %v", n, err)
	}
	f.Close()
	f, err = p.OpenFile("/devices/urandom", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := io.ReadFull(f, make([]byte, 16)); n != 16 || err != nil {
		t.Fatalf("read /dev/urandom: %d, %v", n, err)
	}
	f.Close()
	// fopen('/dev/null', 'w') opens with O_CREAT|O_TRUNC; devices ignore O_TRUNC.
	f, err = p.OpenFile("/devices/null", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := f.(io.Writer)
	if !ok {
		t.Fatalf("%T is not writable", f)
	}
	if n, err := w.Write([]byte("discarded")); n != 9 || err != nil {
		t.Fatalf("write /dev/null: %d, %v", n, err)
	}
	f.Close()
}

func TestRootRejectsSocketsAndBlockDevices(t *testing.T) {
	// Unix socket paths are short on Darwin; t.TempDir may exceed the limit.
	dir, err := os.MkdirTemp("/tmp", "phego-sock")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	l, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}}})
	for _, flag := range []int{os.O_RDONLY, os.O_WRONLY} {
		f, err := p.OpenFile("/app/s", flag, 0)
		if f != nil {
			f.Close()
		}
		wantPolicyError(t, err, "not-granted")
	}
	entries, err := os.ReadDir("/dev")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type()&os.ModeDevice == 0 || e.Type()&os.ModeCharDevice != 0 {
			continue
		}
		p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev"}}})
		f, err := p.OpenFile("/devices/"+e.Name(), os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		wantPolicyError(t, err, "not-granted")
		return
	}
	t.Log("no block device in /dev; socket rejection checked")
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

// Character devices are opened only when they are, by device number, the
// host's null, zero, full, random or urandom. A terminal or pseudo-terminal
// master blocks a read even with O_NONBLOCK under Go's poller, and other
// devices may have side effects when opened (for example a watchdog).
func TestRootOpensOnlyListedCharacterDevices(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev", Write: true}}})
	for _, name := range []string{"zero", "random", "full"} {
		if _, err := os.Stat("/dev/" + name); err != nil {
			continue // /dev/full is Linux only
		}
		f, err := p.OpenFile("/devices/"+name, os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		f.Close()
	}
	for _, name := range []string{"tty", "ptmx"} {
		if _, err := os.Stat("/dev/" + name); err != nil {
			continue
		}
		for _, flag := range []int{os.O_RDONLY, os.O_RDWR} {
			f, err := p.OpenFile("/devices/"+name, flag, 0)
			var pe *PolicyError
			if !errors.As(err, &pe) || pe.Rule != "not-granted" {
				if f != nil {
					f.Close()
				}
				t.Errorf("%s flag %d: %v, want not-granted", name, flag, err)
			}
		}
	}
}

// OpenSource opens a script's source: like OpenFile, but a host directory
// admits only regular files and directories, so a listed device that
// OpenFile opens is refused before it is opened.
func TestOpenSourceRefusesListedDevices(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev"}}})
	f, err := p.OpenFile("/devices/null", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(/devices/null): %v", err)
	}
	f.Close()
	_, err = p.OpenSource("/devices/null")
	var pe *PolicyError
	if !errors.As(err, &pe) || *pe != (PolicyError{Kind: FileRead, Op: "open", Target: "/devices/null", Rule: "not-granted"}) {
		t.Fatalf("OpenSource(/devices/null) = %#v, want not-granted", err)
	}
	d, err := p.OpenSource("/devices")
	if err != nil {
		t.Fatalf("OpenSource(/devices): %v", err)
	}
	d.Close()
}
