//go:build !unix

package policy

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"time"
)

// Unix FIFO open flags are not available on these platforms; Windows is
// cross-compiled but not runtime-certified.
const nonblockOpenFlag = 0

// os.Root refuses Windows device names, so no FIFO or device reaches here
// through a host directory, and the file is returned as opened.
func wrapSpecial(_ context.Context, f *os.File, _ fs.FileInfo, _ string, _ int) (fs.File, error) {
	return f, nil
}

// openWriting has no FIFO reader to wait for.
func openWriting(_ context.Context, parent *os.Root, leaf string, flag int, perm fs.FileMode, _ fs.FileInfo, _ string) (*os.File, error) {
	return parent.OpenFile(leaf, flag, perm)
}

// ReadHostFile reads up to limit bytes of f, a device or pipe that host code
// opened. A file that takes read deadlines stops when ctx ends, with an
// *fs.PathError naming name and carrying context.Cause(ctx); any other is
// read as it is.
func ReadHostFile(ctx context.Context, f *os.File, name string, limit int64) ([]byte, error) {
	if f.SetReadDeadline(time.Time{}) != nil {
		return io.ReadAll(io.LimitReader(f, limit))
	}
	stop := context.AfterFunc(ctx, func() { f.SetReadDeadline(time.Unix(1, 0)) })
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if !stop() && errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: context.Cause(ctx)}
	}
	return data, err
}
