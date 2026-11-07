//go:build !unix

package policy

import "io/fs"

// Unix FIFO open flags are not available on these platforms. Type checks still
// reject special files; Windows is cross-compiled but not runtime-certified.
const nonblockOpenFlag = 0

// Character devices are opened only on Unix, where they can be identified.
func listedCharDevice(fs.FileInfo) bool { return false }
