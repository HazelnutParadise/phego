//go:build !windows

package policy

import (
	"io/fs"
	"os"
)

// linkTarget reports whether component, with Lstat result st, is a link the
// path resolver follows, and reads its target.
func linkTarget(parent *os.Root, component string, st fs.FileInfo) (string, bool, error) {
	if st.Mode()&fs.ModeSymlink == 0 {
		return "", false, nil
	}
	target, err := parent.Readlink(component)
	return target, true, err
}

// Only Windows reserves device names in every directory.
func reservedDeviceName(string, error) bool { return false }

func reservedNameError(string) error { return nil }

// Other systems open a directory the way their open(2) does.
func directoryOpenError(string) error { return nil }

// sameHostName compares one component of two host paths.
func sameHostName(a, b string) bool { return a == b }

// Only Windows has junctions.
func junction(*os.Root, string, fs.FileInfo) bool { return false }
