package policy

import (
	"io/fs"
	"strings"
	"syscall"
)

// reservedDeviceName reports whether os.Root refused component, one path
// name without separators, because Windows opens a device for it in every
// directory (NUL, CON, COM1, CONIN$ and the like). Root refuses those as
// escaping the directory; a colon, which names an alternate data stream,
// stays refused that way.
func reservedDeviceName(component string, err error) bool {
	return !strings.ContainsRune(component, ':') && rootEscapeError(err)
}

// reservedNameError is what PHP on Windows reports for a reserved device
// name inside a directory: its path resolution finds no such file, for
// every open mode, and no device is opened.
func reservedNameError(name string) error {
	return &fs.PathError{Op: "open", Path: name, Err: syscall.ERROR_FILE_NOT_FOUND}
}

// directoryOpenError is PHP's result for opening a directory on Windows:
// CreateFile without FILE_FLAG_BACKUP_SEMANTICS refuses every mode with
// ERROR_ACCESS_DENIED ("Permission denied").
func directoryOpenError(name string) error {
	return &fs.PathError{Op: "open", Path: name, Err: syscall.ERROR_ACCESS_DENIED}
}
