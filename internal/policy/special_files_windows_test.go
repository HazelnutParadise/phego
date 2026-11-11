//go:build windows

package policy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// PHP 8.5.12 on Windows resolves a path before opening it, so a reserved
// device name inside a directory (<dir>\NUL, <dir>\sub\CON, <dir>\COM1) is
// reported as missing for every mode, and no device opens (special-files
// workflow, windows-2025). A Root mount reports the same and creates
// nothing.
func TestRootWindowsReservedNamesAreMissing(t *testing.T) {
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	writable := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work, Write: true}}})
	readOnly := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work}}})
	flags := []struct {
		mode string
		flag int
	}{
		{"r", os.O_RDONLY},
		{"w", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"r+", os.O_RDWR},
		{"a", os.O_WRONLY | os.O_CREATE | os.O_APPEND},
		{"x", os.O_WRONLY | os.O_CREATE | os.O_EXCL},
	}
	names := []string{"/app/NUL", "/app/nul", "/app/sub/NUL", "/app/CON", "/app/COM1", "/app/CONIN$", "/app/CONOUT$", "/app/LPT1", "/app/NUL/x"}
	for _, p := range []*Policy{writable, readOnly} {
		for _, name := range names {
			for _, c := range flags {
				f, err := p.OpenFile(t.Context(), name, c.flag, 0o600)
				if err == nil {
					f.Close()
					t.Errorf("fopen(%s, %q) opened, want No such file or directory", name, c.mode)
					continue
				}
				var pe *PolicyError
				if errors.As(err, &pe) || !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("fopen(%s, %q): %v, want No such file or directory", name, c.mode, err)
				}
			}
		}
	}
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sub" {
		t.Fatalf("mount contents after the opens: %v, want only sub", entries)
	}
}

// PHP 8.5.12 on Windows opens files with CreateFile, which refuses a
// directory, so fopen of a directory fails with Permission denied for r, w,
// r+, a and x (special-files workflow, windows-2025). A Root mount refuses the
// same opens with the same error, including the mount directory itself,
// and leaves the directory untouched. Opens a read-only mount does not
// grant are still refused by the policy.
func TestRootWindowsDirectoriesAreDenied(t *testing.T) {
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "d", "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work, Write: true}}})
	flags := []struct {
		mode string
		flag int
	}{
		{"r", os.O_RDONLY},
		{"w", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"r+", os.O_RDWR},
		{"a", os.O_WRONLY | os.O_CREATE | os.O_APPEND},
		{"x", os.O_WRONLY | os.O_CREATE | os.O_EXCL},
	}
	for _, name := range []string{"/app/d", "/app/d/", "/app"} {
		for _, c := range flags {
			f, err := p.OpenFile(t.Context(), name, c.flag, 0o600)
			if err == nil {
				f.Close()
				t.Errorf("fopen(%s, %q) opened, want Permission denied", name, c.mode)
				continue
			}
			var pe *PolicyError
			if errors.As(err, &pe) || !errors.Is(err, fs.ErrPermission) {
				t.Errorf("fopen(%s, %q): %v, want Permission denied", name, c.mode, err)
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(work, "d", "keep")); err != nil || string(b) != "x" {
		t.Fatalf("directory contents changed: %q %v", b, err)
	}
	readOnly := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work}}})
	_, err := readOnly.OpenFile(t.Context(), "/app/d", os.O_WRONLY, 0)
	wantPolicyError(t, err, "not-granted")
}
