//go:build unix

package policy

import "syscall"

// Nonblocking open prevents a swapped-in FIFO from waiting for a peer before
// the descriptor identity and type checks. Regular-file I/O is unchanged.
const nonblockOpenFlag = syscall.O_NONBLOCK
