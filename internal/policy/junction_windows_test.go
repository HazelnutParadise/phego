//go:build windows

package policy

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mustJunction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J %s %s: %v\n%s", link, target, err, out)
	}
}

// A junction (mount point) is the Windows way to make a directory name lead
// somewhere else without symlink privileges. os.Root refuses to follow one
// with the same "path escapes from parent" error it gives a reserved device
// name, so the two must be told apart: a junction that leaves the mount is a
// policy refusal, not PHP's "No such file or directory" for <dir>\NUL.
func TestRootWindowsJunctionOutsideMountIsRefused(t *testing.T) {
	base := t.TempDir()
	work, outside := filepath.Join(base, "work"), filepath.Join(base, "outside")
	for _, d := range []string{work, outside} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustJunction(t, filepath.Join(work, "out"), outside)

	root, err := os.OpenRoot(work)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	st, err := root.Lstat("out")
	t.Logf("os.Root.Lstat(out): mode=%v err=%v", modeOf(st), err)
	target, err := root.Readlink("out")
	t.Logf("os.Root.Readlink(out): %q err=%v", target, err)
	_, err = root.OpenRoot("out")
	t.Logf("os.Root.OpenRoot(out): err=%v", err)
	_, err = root.Open("out/secret.txt")
	t.Logf("os.Root.Open(out/secret.txt): err=%v", err)
	_, err = root.Lstat("out/secret.txt")
	t.Logf("os.Root.Lstat(out/secret.txt): err=%v", err)
	_, err = root.Lstat("NUL")
	t.Logf("os.Root.Lstat(NUL): err=%v", err)

	for _, write := range []bool{false, true} {
		p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work, Write: write}}})
		flags := []int{os.O_RDONLY}
		if write {
			flags = append(flags, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		}
		for _, name := range []string{"/app/out/secret.txt", "/app/out/new.txt", "/app/out", "/app/out/NUL"} {
			for _, flag := range flags {
				f, err := p.OpenFile(t.Context(), name, flag, 0o600)
				t.Logf("write=%v OpenFile(%s, %#x): %T %v", write, name, flag, err, err)
				if err == nil {
					f.Close()
					t.Errorf("OpenFile(%s, %#x) opened through a junction that leaves the mount", name, flag)
					continue
				}
				// Exclusive creation never follows the last name, so an
				// existing junction is reported as existing, as a symbolic
				// link is.
				if name == "/app/out" && flag&os.O_EXCL != 0 {
					if !errors.Is(err, fs.ErrExist) {
						t.Errorf("OpenFile(%s, %#x): %v, want file exists", name, flag, err)
					}
					continue
				}
				var pe *PolicyError
				if !errors.As(err, &pe) || pe.Rule != "outside-root" {
					t.Errorf("OpenFile(%s, %#x): %v, want PolicyError outside-root", name, flag, err)
				}
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(outside, "secret.txt")); err != nil || string(b) != "secret" {
		t.Fatalf("file outside the mount changed: %q %v", b, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory outside the mount changed: %v %v", entries, err)
	}
}

// PHP on Windows follows a junction like any directory (special-files
// workflow, windows-2025). One that stays inside the mount leads to a
// granted file, so a Root mount opens it too, however the target's case is
// spelled; the junction itself is a directory and is refused like one.
func TestRootWindowsJunctionInsideMountOpens(t *testing.T) {
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "real", "file.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustJunction(t, filepath.Join(work, "in"), filepath.Join(work, "real"))
	mustJunction(t, filepath.Join(work, "upper"), filepath.Join(work, "REAL"))
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work}}})
	for _, name := range []string{"/app/upper/file.txt", "/app/in/../real/file.txt"} {
		f, err := p.OpenFile(t.Context(), name, os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("OpenFile(%s): %v", name, err)
			continue
		}
		f.Close()
	}
	if _, err := p.OpenFile(t.Context(), "/app/in", os.O_RDONLY, 0); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("OpenFile(/app/in): %v, want Permission denied", err)
	}
	f, err := p.OpenFile(t.Context(), "/app/in/file.txt", os.O_RDONLY, 0)
	t.Logf("OpenFile(/app/in/file.txt): %T %v", err, err)
	if err != nil {
		t.Fatalf("OpenFile through a junction inside the mount: %v", err)
	}
	defer f.Close()
	if b, err := io.ReadAll(f); err != nil || string(b) != "inside" {
		t.Fatalf("read %q %v, want inside", b, err)
	}
}

func modeOf(info fs.FileInfo) any {
	if info == nil {
		return nil
	}
	return info.Mode()
}
