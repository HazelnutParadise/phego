//go:build !unix

package policy

// Unix FIFO open flags are not available on these platforms. Type checks still
// reject special files; Windows is cross-compiled but not runtime-certified.
const nonblockOpenFlag = 0
