package policy

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
)

// linkTarget reports whether component, with Lstat result st, is a link the
// path resolver follows, and reads its target. Besides symbolic links that
// is a junction (mount point): Lstat reports one as an irregular file whose
// target Readlink returns as an absolute path. PHP follows a junction like a
// directory, so its target is resolved as an absolute link target is: into a
// granted directory, or refused. os.Root would refuse every junction as
// escaping (special-files workflow, windows-2025). Other reparse points
// have no link target and stay with os.Root.
func linkTarget(parent *os.Root, component string, st fs.FileInfo) (string, bool, error) {
	if st.Mode()&fs.ModeSymlink != 0 {
		target, err := parent.Readlink(component)
		return target, true, err
	}
	if st.Mode()&fs.ModeIrregular != 0 {
		if target, err := parent.Readlink(component); err == nil {
			return target, true, nil
		}
	}
	return "", false, nil
}

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

// sameHostName compares one component of two host paths. Windows names are
// case-insensitive, and a junction keeps the spelling it was created with.
// The comparison only selects the mount a link target continues in; the
// rest is still resolved inside that mount's directory.
func sameHostName(a, b string) bool { return strings.EqualFold(a, b) }

// junction reports whether leaf is a junction. Exclusive creation does not
// follow the last name, and PHP reports an existing junction as it reports
// a directory: Permission denied (special-files workflow, windows-2025).
func junction(parent *os.Root, leaf string, st fs.FileInfo) bool {
	if parent == nil || st.Mode()&fs.ModeIrregular == 0 {
		return false
	}
	_, err := parent.Readlink(leaf)
	return err == nil
}
