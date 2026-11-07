//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package phego

import (
	"errors"
	"io/fs"
	"os"
)

// probeOpen opens path for reading and returns the system's error.
func probeOpen(path string) error {
	f, err := os.Open(path)
	if err == nil {
		f.Close()
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
