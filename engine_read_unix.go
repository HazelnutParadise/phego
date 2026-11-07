//go:build unix

package phego

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

// readHostFile reads a FIFO or device the host opened, up to limit bytes,
// and stops without leaving a read behind when ctx ends. A file in the
// runtime poller (Linux FIFOs and terminals) takes a read deadline, which
// ends a waiting read. Go cannot poll FIFOs on darwin (Go issue 24164), so
// such a file is read without blocking, waiting for ctx between attempts.
func readHostFile(ctx context.Context, f *os.File, limit int64) ([]byte, error) {
	if f.SetReadDeadline(time.Time{}) == nil {
		stop := context.AfterFunc(ctx, func() { f.SetReadDeadline(time.Now()) })
		data, err := io.ReadAll(io.LimitReader(f, limit))
		if !stop() && errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, ctx.Err()
		}
		return data, err
	}
	raw, err := f.SyscallConn()
	if err != nil {
		return io.ReadAll(io.LimitReader(f, limit))
	}
	var nonblock error
	if err := raw.Control(func(fd uintptr) { nonblock = syscall.SetNonblock(int(fd), true) }); err != nil || nonblock != nil {
		return io.ReadAll(io.LimitReader(f, limit))
	}
	var out []byte
	buf := make([]byte, 32<<10)
	wait := time.Millisecond
	for int64(len(out)) < limit {
		n, err := f.Read(buf[:min(int64(len(buf)), limit-int64(len(out)))])
		out = append(out, buf[:n]...)
		switch {
		case err == io.EOF:
			return out, nil
		case errors.Is(err, syscall.EAGAIN):
			// Writers are open but have sent nothing yet.
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			wait = min(2*wait, 20*time.Millisecond)
		case err != nil:
			return out, err
		default:
			wait = time.Millisecond
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
	}
	return out, nil
}
