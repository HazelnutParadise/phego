//go:build unix

package policy

import (
	"io/fs"
	"os"
	"sync"
	"syscall"
)

// Nonblocking open prevents a swapped-in FIFO from waiting for a peer before
// the descriptor identity and type checks. Regular-file I/O is unchanged.
const nonblockOpenFlag = syscall.O_NONBLOCK

// listedDevices are the device numbers of the host's /dev/null, /dev/zero,
// /dev/full, /dev/random and /dev/urandom, whichever exist.
var listedDevices = sync.OnceValue(func() map[uint64]bool {
	out := make(map[uint64]bool)
	for _, name := range []string{"null", "zero", "full", "random", "urandom"} {
		if fi, err := os.Stat("/dev/" + name); err == nil && isCharDevice(fi.Mode()) {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				out[uint64(st.Rdev)] = true
			}
		}
	}
	return out
})

// listedCharDevice reports a character device that is, by device number, one
// of listedDevices, wherever its node is mounted.
func listedCharDevice(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && listedDevices()[uint64(st.Rdev)]
}
