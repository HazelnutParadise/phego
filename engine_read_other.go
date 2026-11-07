//go:build !unix

package phego

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// readHostFile reads a FIFO or device the host opened, up to limit bytes.
// A file that takes read deadlines stops when ctx ends; any other is read
// as it is.
func readHostFile(ctx context.Context, f *os.File, limit int64) ([]byte, error) {
	if f.SetReadDeadline(time.Time{}) == nil {
		stop := context.AfterFunc(ctx, func() { f.SetReadDeadline(time.Now()) })
		data, err := io.ReadAll(io.LimitReader(f, limit))
		if !stop() && errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, ctx.Err()
		}
		return data, err
	}
	return io.ReadAll(io.LimitReader(f, limit))
}
