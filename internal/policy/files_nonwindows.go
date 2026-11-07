//go:build !windows

package policy

// Only Windows reserves device names in every directory.
func reservedDeviceName(string, error) bool { return false }

func reservedNameError(string) error { return nil }

// Other systems open a directory the way their open(2) does.
func directoryOpenError(string) error { return nil }
