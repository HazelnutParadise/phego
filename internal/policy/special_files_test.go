//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package policy

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"
)

// PHP 8.5.12 opens FIFOs, devices and sockets with a plain blocking
// open(2) (measured on the pinned arm64, amd64 and i386 images). Root mounts
// do the same: the kernel decides what opens, a FIFO open waits for its peer,
// and reads and writes wait like blocking I/O. Every wait ends with ctx.

// openSpecial opens name for the tests, with ctx bounding every wait.
func openSpecial(ctx context.Context, p *Policy, name string, flag int) (fs.File, error) {
	return p.OpenFile(ctx, name, flag, 0o600)
}

type openResult struct {
	f   fs.File
	err error
}

func openAsync(ctx context.Context, p *Policy, name string, flag int) <-chan openResult {
	done := make(chan openResult, 1)
	go func() {
		f, err := openSpecial(ctx, p, name, flag)
		done <- openResult{f, err}
	}()
	return done
}

// waiting fails the test when done delivers within d.
func waiting(t *testing.T, done <-chan openResult, d time.Duration, what string) {
	t.Helper()
	select {
	case r := <-done:
		if r.f != nil {
			r.f.Close()
		}
		t.Fatalf("%s returned without waiting: %v", what, r.err)
	case <-time.After(d):
	}
}

func received(t *testing.T, done <-chan openResult, what string) openResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatalf("%s still waiting", what)
		return openResult{}
	}
}

// fifoWriter opens the write end of a FIFO without blocking. It succeeds
// only once a reader exists, so it retries while the reader is opening.
func fifoWriter(t *testing.T, name string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		fd, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err == nil {
			return fd
		}
		if !errors.Is(err, syscall.ENXIO) || time.Now().After(deadline) {
			t.Fatalf("open writer: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func mkfifo(t *testing.T) (dir, name string) {
	t.Helper()
	dir = t.TempDir()
	name = filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, name
}

func readAsync(f fs.File, n int) <-chan openResult {
	done := make(chan openResult, 1)
	go func() {
		b := make([]byte, n)
		k, err := f.Read(b)
		done <- openResult{err: readError{b[:k], err}}
	}()
	return done
}

type readError struct {
	data []byte
	err  error
}

func (r readError) Error() string { return string(r.data) }

func readDone(t *testing.T, done <-chan openResult) ([]byte, error) {
	t.Helper()
	r := received(t, done, "read").err.(readError)
	return r.data, r.err
}

// PHP: with the writer opening at 1 s, fopen(FIFO, "r") returns at 1.0 s; with
// a writer already waiting that writes at 1 s, fopen returns at 0.0 s and fread
// returns the data once written.
func TestRootFIFOReadOpensWhenWriterArrives(t *testing.T) {
	dir, name := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	opened := openAsync(t.Context(), p, "/app/pipe", os.O_RDONLY)
	waiting(t, opened, 200*time.Millisecond, "FIFO open without a writer")
	w := fifoWriter(t, name)
	defer func() {
		if w >= 0 {
			syscall.Close(w)
		}
	}()
	r := received(t, opened, "FIFO open after the writer opened")
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.f.Close()
	read := readAsync(r.f, 16)
	waiting(t, read, 200*time.Millisecond, "FIFO read without data")
	if _, err := syscall.Write(w, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if data, err := readDone(t, read); string(data) != "payload" || err != nil {
		t.Fatalf("read %q, %v", data, err)
	}
	syscall.Close(w)
	w = -1
	if n, err := r.f.Read(make([]byte, 16)); n != 0 || err != io.EOF {
		t.Fatalf("read after the writer closed: %d, %v", n, err)
	}
}

// Data a writer leaves behind is read in full, including any byte the open
// consumed while it checked for a writer.
func TestRootFIFOReadKeepsEarlyData(t *testing.T) {
	dir, name := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	opened := openAsync(t.Context(), p, "/app/pipe", os.O_RDONLY)
	w := fifoWriter(t, name)
	if _, err := syscall.Write(w, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	syscall.Close(w)
	r := received(t, opened, "FIFO open")
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.f.Close()
	if data, err := io.ReadAll(r.f); string(data) != "abc" || err != nil {
		t.Fatalf("read %q, %v", data, err)
	}
}

// PHP: a writer that opens and closes without writing ends the wait, and
// fread then reports EOF. Linux reports that through EPOLLHUP; macOS and the
// BSDs give a reader no trace of it, as W-20.md records.
func TestRootFIFOReadSeesWriterThatLeft(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux reports a FIFO writer that left without data; see W-20.md")
	}
	dir, name := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	opened := openAsync(t.Context(), p, "/app/pipe", os.O_RDONLY)
	syscall.Close(fifoWriter(t, name))
	r := received(t, opened, "FIFO open after a writer left")
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.f.Close()
	if n, err := r.f.Read(make([]byte, 8)); n != 0 || err != io.EOF {
		t.Fatalf("read: %d, %v", n, err)
	}
}

// A host stop ends the wait for a peer at once, with the context's cause.
func TestRootFIFOOpenEndsWithContext(t *testing.T) {
	dir, _ := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}}})
	stopped := errors.New("host stopped the request")
	for _, flag := range []int{os.O_RDONLY, os.O_WRONLY, os.O_WRONLY | os.O_CREATE | os.O_TRUNC, os.O_RDONLY | os.O_TRUNC} {
		ctx, cancel := context.WithCancelCause(t.Context())
		opened := openAsync(ctx, p, "/app/pipe", flag)
		waiting(t, opened, 100*time.Millisecond, "FIFO open without a peer")
		start := time.Now()
		cancel(stopped)
		r := received(t, opened, "cancelled FIFO open")
		if r.f != nil {
			r.f.Close()
		}
		var pe *fs.PathError
		if !errors.Is(r.err, stopped) || !errors.As(r.err, &pe) || pe.Path != "/app/pipe" {
			t.Fatalf("flag %#x: %v, want the cause on the PHP path", flag, r.err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("flag %#x: cancellation took %v", flag, d)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if r := received(t, openAsync(ctx, p, "/app/pipe", os.O_RDONLY), "timed-out FIFO open"); !errors.Is(r.err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", r.err)
	}
}

// A blocked read ends with the context too, and later reads fail the same way.
func TestRootFIFOReadEndsWithContext(t *testing.T) {
	dir, name := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := openAsync(ctx, p, "/app/pipe", os.O_RDONLY)
	w := fifoWriter(t, name)
	defer syscall.Close(w)
	r := received(t, opened, "FIFO open")
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.f.Close()
	read := readAsync(r.f, 8)
	waiting(t, read, 100*time.Millisecond, "FIFO read without data")
	cancel()
	if _, err := readDone(t, read); !errors.Is(err, context.Canceled) {
		t.Fatalf("read: %v", err)
	}
	syscall.Write(w, []byte("late"))
	if _, err := r.f.Read(make([]byte, 8)); !errors.Is(err, context.Canceled) {
		t.Fatalf("read after cancellation: %v", err)
	}
}

// PHP: fopen(FIFO, "w"), "a" and "c" wait for a reader; fwrite after the
// reader closed reports EPIPE ("Broken pipe").
func TestRootFIFOWriteWaitsForReader(t *testing.T) {
	for _, flag := range []int{os.O_WRONLY | os.O_CREATE | os.O_TRUNC, os.O_WRONLY | os.O_CREATE | os.O_APPEND, os.O_WRONLY | os.O_CREATE} {
		dir, name := mkfifo(t)
		p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}}})
		opened := openAsync(t.Context(), p, "/app/pipe", flag)
		waiting(t, opened, 200*time.Millisecond, "FIFO write open without a reader")
		reader, err := syscall.Open(name, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		r := received(t, opened, "FIFO write open after the reader opened")
		if r.err != nil {
			syscall.Close(reader)
			t.Fatalf("flag %#x: %v", flag, r.err)
		}
		w := r.f.(io.Writer)
		if n, err := w.Write([]byte("payload")); n != 7 || err != nil {
			t.Fatalf("write: %d, %v", n, err)
		}
		b := make([]byte, 16)
		n, err := syscall.Read(reader, b)
		if string(b[:max(n, 0)]) != "payload" {
			t.Fatalf("reader got %q, %v", b[:max(n, 0)], err)
		}
		syscall.Close(reader)
		var pe *fs.PathError
		if _, err := w.Write([]byte("again")); !errors.Is(err, syscall.EPIPE) || !errors.As(err, &pe) || pe.Path != "/app/pipe" {
			t.Fatalf("write after the reader closed: %v, want EPIPE on the PHP path", err)
		}
		r.f.Close()
	}
}

// PHP: fopen(FIFO, "r+") returns at once and "x" reports "File exists".
// Read-write still needs a writable grant.
func TestRootFIFOReadWriteOpensAtOnce(t *testing.T) {
	dir, _ := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}}})
	r := received(t, openAsync(t.Context(), p, "/app/pipe", os.O_RDWR), "FIFO read-write open")
	if r.err != nil {
		t.Fatal(r.err)
	}
	// PHP: fseek on a FIFO fails with ESPIPE. Errors name the PHP path, not
	// the host directory.
	var pe *fs.PathError
	if _, err := r.f.(io.Seeker).Seek(0, io.SeekStart); !errors.Is(err, syscall.ESPIPE) || !errors.As(err, &pe) || pe.Path != "/app/pipe" {
		t.Fatalf("seek: %v, want ESPIPE on the PHP path", err)
	}
	r.f.Close()
	r = received(t, openAsync(t.Context(), p, "/app/pipe", os.O_WRONLY|os.O_CREATE|os.O_EXCL), "exclusive FIFO open")
	if !errors.Is(r.err, fs.ErrExist) {
		t.Fatalf("exclusive open: %v", r.err)
	}
	readOnly := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	_, err := openSpecial(t.Context(), readOnly, "/app/pipe", os.O_RDWR)
	wantPolicyError(t, err, "not-granted")
}

// What opens is the kernel's decision, as with PHP's open(2) on the same
// system: a socket and a terminal without a controlling tty fail with ENXIO
// on Linux, a directory opened for writing with EISDIR, and a block device
// with whatever the process may do with it. Each case compares with a direct
// open of the node; for a directory that includes O_CREAT and O_TRUNC, which
// Linux refuses and macOS refuses only for O_TRUNC.
func TestRootOpenMatchesKernelForSpecialFiles(t *testing.T) {
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
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	type check struct {
		phpPath, host string
		flag          int
	}
	checks := []check{
		{"/app/s", dir + "/s", os.O_RDONLY},
		{"/app/s", dir + "/s", os.O_WRONLY},
		{"/app/d", dir + "/d", os.O_WRONLY},
		{"/app/d", dir + "/d", os.O_RDWR},
		{"/app/d", dir + "/d", os.O_WRONLY | os.O_APPEND | os.O_CREATE},
		{"/app/d", dir + "/d", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"/app/d", dir + "/d", os.O_RDONLY | os.O_TRUNC},
		{"/app/d", dir + "/d", os.O_RDONLY | os.O_CREATE},
		{"/devices/tty", "/dev/tty", os.O_RDONLY},
		{"/devices/tty", "/dev/tty", os.O_WRONLY},
	}
	entries, err := os.ReadDir("/dev")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type()&os.ModeDevice != 0 && e.Type()&os.ModeCharDevice == 0 {
			checks = append(checks, check{"/devices/" + e.Name(), "/dev/" + e.Name(), os.O_RDONLY})
			break
		}
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir, Write: true}, {Path: "/devices", Dir: "/dev", Write: true}}})
	for _, c := range checks {
		fd, want := syscall.Open(c.host, c.flag|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if want == nil {
			syscall.Close(fd)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		f, got := openSpecial(ctx, p, c.phpPath, c.flag)
		cancel()
		if f != nil {
			f.Close()
		}
		var pe *PolicyError
		switch {
		case errors.As(got, &pe):
			t.Errorf("%s flag %#x: policy refusal %v, kernel says %v", c.phpPath, c.flag, got, want)
		case want == nil && got != nil:
			t.Errorf("%s flag %#x: %v, kernel opens it", c.phpPath, c.flag, got)
		case want != nil && !errors.Is(got, want):
			t.Errorf("%s flag %#x: %v, kernel says %v", c.phpPath, c.flag, got, want)
		}
	}
}

// PHP reads and writes character devices such as /dev/null and /dev/urandom.
// They open without waiting, so a granted device behaves like PHP's.
func TestRootAllowsCharacterDevices(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev", Write: true}}})
	f, err := p.OpenFile(t.Context(), "/devices/null", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.Read(make([]byte, 8)); n != 0 || err != io.EOF {
		t.Fatalf("read /dev/null: %d, %v", n, err)
	}
	f.Close()
	f, err = p.OpenFile(t.Context(), "/devices/urandom", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := io.ReadFull(f, make([]byte, 16)); n != 16 || err != nil {
		t.Fatalf("read /dev/urandom: %d, %v", n, err)
	}
	f.Close()
	// fopen('/dev/null', 'w') opens with O_CREAT|O_TRUNC; devices ignore O_TRUNC.
	f, err = p.OpenFile(t.Context(), "/devices/null", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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

// A terminal or pseudo-terminal read waits for input like PHP's blocking
// read, and the context ends the wait.
func TestRootDeviceReadEndsWithContext(t *testing.T) {
	if _, err := os.Stat("/dev/ptmx"); err != nil {
		t.Skip(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/devices", Dir: "/dev", Write: true}}})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f, err := openSpecial(ctx, p, "/devices/ptmx", os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	read := readAsync(f, 8)
	waiting(t, read, 100*time.Millisecond, "pseudo-terminal read without input")
	start := time.Now()
	cancel()
	if _, err := readDone(t, read); !errors.Is(err, context.Canceled) {
		t.Fatalf("read: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("cancellation took %v", d)
	}
}

// Closing a file while another goroutine waits in Read ends that wait.
func TestRootSpecialFileCloseEndsRead(t *testing.T) {
	dir, name := mkfifo(t)
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	opened := openAsync(t.Context(), p, "/app/pipe", os.O_RDONLY)
	w := fifoWriter(t, name)
	defer syscall.Close(w)
	r := received(t, opened, "FIFO open")
	if r.err != nil {
		t.Fatal(r.err)
	}
	read := readAsync(r.f, 8)
	waiting(t, read, 100*time.Millisecond, "FIFO read without data")
	r.f.Close()
	if _, err := readDone(t, read); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}

// A FIFO reachable only as a hard link to protected data is refused before
// it is opened: opening it would already release a peer waiting in open(2).
func TestRootRefusedFIFOAliasIsNotOpened(t *testing.T) {
	for _, write := range []bool{false, true} {
		root := t.TempDir()
		protected := filepath.Join(root, "protected")
		if err := os.Mkdir(protected, 0o700); err != nil {
			t.Fatal(err)
		}
		pipe := filepath.Join(protected, "pipe")
		if err := syscall.Mkfifo(pipe, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(pipe, filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
		// The peer blocks in open(2) until this end of the FIFO opens.
		peerFlag, unblockFlag, flag := syscall.O_WRONLY, syscall.O_RDONLY, os.O_RDONLY
		spec := Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/protected", FS: fstest.MapFS{}}}}
		if write {
			peerFlag, unblockFlag, flag = syscall.O_RDONLY, syscall.O_WRONLY, os.O_WRONLY
			spec = Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: protected}}}
		}
		peer := make(chan error, 1)
		go func() {
			// Runtime preemption signals can interrupt the blocking open.
			for {
				fd, err := syscall.Open(pipe, peerFlag|syscall.O_CLOEXEC, 0)
				if err == syscall.EINTR {
					continue
				}
				if err == nil {
					syscall.Close(fd)
				}
				peer <- err
				return
			}
		}()
		time.Sleep(50 * time.Millisecond)
		f, err := openSpecial(t.Context(), mustPolicy(t, spec), "/alias", flag)
		if f != nil {
			f.Close()
		}
		wantPolicyError(t, err, "not-granted")
		select {
		case err := <-peer:
			t.Fatalf("write=%v: the refused open released the FIFO peer (%v)", write, err)
		case <-time.After(100 * time.Millisecond):
		}
		fd, err := syscall.Open(pipe, unblockFlag|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		<-peer
		syscall.Close(fd)
	}
}

// A node swapped between the check and the open is either the file that was
// checked or a refusal, and no open waits past its context.
func TestRootFIFOSwapStaysBounded(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(dir+"/pipe", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/regular", []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(dir+"/regular", dir+"/target"); err != nil {
		t.Fatal(err)
	}
	pipe, err := os.Lstat(dir + "/pipe")
	if err != nil {
		t.Fatal(err)
	}
	regular, err := os.Lstat(dir + "/regular")
	if err != nil {
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
	defer func() {
		close(stop)
		wg.Wait()
		select {
		case err := <-swapError:
			t.Fatal(err)
		default:
		}
	}()
	flags := []int{os.O_RDONLY, os.O_WRONLY, os.O_RDWR | os.O_TRUNC, os.O_RDONLY | os.O_TRUNC}
	for i := 0; i < 400; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
		start := time.Now()
		f, err := openSpecial(ctx, p, "/app/target", flags[i%len(flags)])
		cancel()
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("open took %v", d)
		}
		if err != nil {
			continue
		}
		st, err := f.Stat()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(st, pipe) && !os.SameFile(st, regular) {
			t.Fatalf("returned an unexpected node %v", st.Mode())
		}
	}
}
