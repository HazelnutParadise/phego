//go:build unix

package policy

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"time"
)

// Every open of a host file adds O_NONBLOCK, so opening a FIFO or a device
// never waits inside open(2); specialFile restores the blocking behaviour PHP
// sees, with waits that the open's context can end.
const nonblockOpenFlag = syscall.O_NONBLOCK

// A wait the runtime poller cannot see, such as a FIFO peer that has not
// arrived, is rechecked after a pause that starts at firstPause and doubles
// to maxPause. The pauses bound how late readiness is noticed; the context
// ends a pause at once.
const (
	firstPause = time.Millisecond
	maxPause   = 10 * time.Millisecond
)

// pause sleeps for *d, or until ctx ends, and then doubles *d up to maxPause.
func pause(ctx context.Context, d *time.Duration) error {
	t := time.NewTimer(*d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
	}
	*d = min(2**d, maxPause)
	return nil
}

// specialFile is a FIFO or a device opened with O_NONBLOCK. Read and Write
// wait as they would on a blocking descriptor: through the runtime poller
// when the descriptor supports it, otherwise by pausing between retries.
// The context given to OpenFile ends every wait, and once it has ended Read
// and Write fail with its cause.
//
// It deliberately does not embed *os.File: io.Copy would otherwise reach
// the file's own ReadFrom and WriteTo and bypass these waits.
type specialFile struct {
	file *os.File
	name string // PHP path, for errors the policy itself reports
	ctx  context.Context
	stop func() bool // ends the deadline callback; nil without poller support

	mu      sync.Mutex
	pending []byte // read by the FIFO open while it looked for a writer
}

func newSpecialFile(ctx context.Context, f *os.File, name string) *specialFile {
	s := &specialFile{file: f, name: name, ctx: ctx}
	// Deadlines work exactly when the descriptor is in the runtime poller.
	// A deadline in the past then wakes every waiting Read and Write.
	if f.SetDeadline(time.Time{}) == nil {
		s.stop = context.AfterFunc(ctx, func() { f.SetDeadline(time.Unix(1, 0)) })
	}
	return s
}

func (s *specialFile) cancelled(op string) error {
	return &fs.PathError{Op: op, Path: s.name, Err: context.Cause(s.ctx)}
}

// ioError reports a deadline, which only the context sets, as its cause, and
// names the PHP path instead of the host path that *os.File errors carry.
func (s *specialFile) ioError(op string, err error) error {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return s.cancelled(op)
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return &fs.PathError{Op: pe.Op, Path: s.name, Err: pe.Err}
	}
	return err
}

func (s *specialFile) Read(b []byte) (int, error) {
	if s.ctx.Err() != nil {
		return 0, s.cancelled("read")
	}
	s.mu.Lock()
	if len(s.pending) > 0 && len(b) > 0 {
		n := copy(b, s.pending)
		s.pending = s.pending[n:]
		s.mu.Unlock()
		return n, nil
	}
	s.mu.Unlock()
	d := firstPause
	for {
		n, err := s.file.Read(b)
		if !errors.Is(err, syscall.EAGAIN) {
			return n, s.ioError("read", err)
		}
		if pause(s.ctx, &d) != nil {
			return 0, s.cancelled("read")
		}
	}
}

func (s *specialFile) Write(b []byte) (int, error) {
	if s.ctx.Err() != nil {
		return 0, s.cancelled("write")
	}
	d := firstPause
	n := 0
	for {
		k, err := s.file.Write(b[n:])
		n += k
		if !errors.Is(err, syscall.EAGAIN) {
			return n, s.ioError("write", err)
		}
		if pause(s.ctx, &d) != nil {
			return n, s.cancelled("write")
		}
	}
}

func (s *specialFile) Seek(offset int64, whence int) (int64, error) {
	n, err := s.file.Seek(offset, whence)
	return n, s.ioError("seek", err)
}

func (s *specialFile) Stat() (fs.FileInfo, error) {
	info, err := s.file.Stat()
	return info, s.ioError("stat", err)
}

func (s *specialFile) Close() error {
	if s.stop != nil {
		s.stop()
	}
	return s.ioError("close", s.file.Close())
}

// ReadHostFile reads up to limit bytes of f, a FIFO or device that host code
// opened, such as an fs.FS mount's file. It waits as a blocking read would,
// with the waits of OpenFile's files: ending ctx ends the read with an
// *fs.PathError naming name and carrying context.Cause(ctx), and no read is
// left running. A descriptor the runtime poller cannot wait on is switched
// to nonblocking mode first; if that fails, f is read as it is.
func ReadHostFile(ctx context.Context, f *os.File, name string, limit int64) ([]byte, error) {
	if f.SetDeadline(time.Time{}) != nil {
		rc, err := f.SyscallConn()
		var nonblock error
		if err == nil {
			err = rc.Control(func(fd uintptr) { nonblock = syscall.SetNonblock(int(fd), true) })
		}
		if err != nil || nonblock != nil {
			return io.ReadAll(io.LimitReader(f, limit))
		}
	}
	s := newSpecialFile(ctx, f, name)
	if s.stop != nil {
		defer s.stop()
	}
	return io.ReadAll(io.LimitReader(s, limit))
}

// awaitWriter returns when PHP's blocking open(2) of a FIFO for reading
// would: once a writer is connected, data is waiting, or a writer came and
// left. A connected writer shows as EAGAIN from a one-byte read; a byte that
// read returns is kept for the first Read.
func (s *specialFile) awaitWriter() error {
	rc, err := s.file.SyscallConn()
	if err != nil {
		return err
	}
	var hangup *hangupProbe
	if err := rc.Control(func(fd uintptr) { hangup, err = newHangupProbe(int(fd)) }); err != nil {
		return s.ioError("open", err)
	}
	if err != nil {
		return &fs.PathError{Op: "open", Path: s.name, Err: err}
	}
	defer hangup.close()
	d := firstPause
	for {
		if s.ctx.Err() != nil {
			return s.cancelled("open")
		}
		var b [1]byte
		var n int
		var rerr error
		var left bool
		err := rc.Read(func(fd uintptr) bool {
			for {
				n, rerr = syscall.Read(int(fd), b[:])
				if rerr != syscall.EINTR {
					break
				}
			}
			if n == 0 && rerr == nil {
				left, rerr = hangup.hungUp()
			}
			return true
		})
		switch {
		case err != nil:
			return s.ioError("open", err)
		case n > 0:
			s.pending = append(s.pending, b[0])
			return nil
		case rerr == syscall.EAGAIN || left:
			return nil
		case rerr != nil:
			return &fs.PathError{Op: "open", Path: s.name, Err: rerr}
		}
		if pause(s.ctx, &d) != nil {
			return s.cancelled("open")
		}
	}
}

// wrapSpecial wraps f, already verified as name's node, when it is a FIFO or
// a device. A FIFO opened only for reading first waits for a writer.
func wrapSpecial(ctx context.Context, f *os.File, info fs.FileInfo, name string, flag int) (fs.File, error) {
	if info.Mode()&(fs.ModeNamedPipe|fs.ModeDevice) == 0 {
		return f, nil
	}
	s := newSpecialFile(ctx, f, name)
	if info.Mode()&fs.ModeNamedPipe != 0 && flag&(os.O_WRONLY|os.O_RDWR) == 0 {
		if err := s.awaitWriter(); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

// openWriting opens leaf in parent for writing the way PHP's blocking
// open(2) does: a FIFO opened only for writing waits for a reader, which a
// nonblocking open reports as ENXIO. Each retry first checks that leaf is
// still the FIFO the caller verified.
func openWriting(ctx context.Context, parent *os.Root, leaf string, flag int, perm fs.FileMode, before fs.FileInfo, name string) (*os.File, error) {
	f, err := parent.OpenFile(leaf, flag, perm)
	for d := firstPause; err != nil && before.Mode()&fs.ModeNamedPipe != 0 && errors.Is(err, syscall.ENXIO); {
		if pause(ctx, &d) != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: context.Cause(ctx)}
		}
		st, e := parent.Lstat(leaf)
		if e != nil {
			return nil, e
		}
		if !os.SameFile(before, st) {
			return nil, &PolicyError{Kind: FileWrite, Op: "open", Target: name, Rule: "path-changed"}
		}
		f, err = parent.OpenFile(leaf, flag, perm)
	}
	return f, err
}
