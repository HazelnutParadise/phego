package policy

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestOverlappingMountsProtectAliases(t *testing.T) {
	for _, target := range []string{"protected", "protected/sub", "safe/../protected"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range []string{"protected/sub", "safe"} {
				if err := os.MkdirAll(filepath.Join(root, d), 0700); err != nil {
					t.Fatal(err)
				}
			}
			protected := filepath.Join(root, target, "file")
			if err := os.WriteFile(protected, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(root, "alias")); err != nil {
				t.Fatal(err)
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: filepath.Join(root, "protected")}}})
			for _, flag := range []int{os.O_WRONLY, os.O_RDWR, os.O_WRONLY | os.O_TRUNC, os.O_CREATE | os.O_WRONLY, os.O_WRONLY | os.O_APPEND} {
				f, err := p.OpenFile("/alias/file", flag, 0600)
				if f != nil {
					f.Close()
				}
				wantPolicyError(t, err, "not-granted")
			}
			f, err := p.OpenFile("/alias/new", os.O_CREATE|os.O_WRONLY, 0600)
			if f != nil {
				f.Close()
			}
			wantPolicyError(t, err, "not-granted")
			b, err := os.ReadFile(protected)
			if err != nil || string(b) != "original" {
				t.Fatalf("protected content=%q %v", b, err)
			}
			if _, err := os.Stat(filepath.Join(root, target, "new")); !os.IsNotExist(err) {
				t.Fatalf("created protected file: %v", err)
			}
		})
	}
}

func TestOverlappingMountsPreserveWritableAliases(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"protected", "uploads"} {
		if err := os.Mkdir(filepath.Join(root, d), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("uploads", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "uploads", "file"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: filepath.Join(root, "protected")}}})
	f, err := p.OpenFile("/alias/file", os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := f.(io.Writer)
	if !ok {
		t.Fatal("not writable")
	}
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err := os.ReadFile(filepath.Join(root, "uploads", "file"))
	if err != nil || string(b) != "new" {
		t.Fatalf("writable alias=%q %v", b, err)
	}
}

func TestOverlappingPhysicalRootsProtectDifferentVirtualAliases(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "data")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(protected, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: root, Write: true}, {Path: "/secret", Dir: protected}}})
	f, err := p.OpenFile("/app/data/file", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "original" {
		t.Fatalf("physical alias=%q %v", b, err)
	}
}

func TestOverlappingMountsProtectHardLinkAliases(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(protected, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: protected}}})
	f, err := p.OpenFile("/alias", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "original" {
		t.Fatalf("hard-link alias=%q %v", b, err)
	}
}

func TestOverlappingMountsProtectFinalSymlinkAndHiddenFS(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(protected, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected/file", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(root, "hard")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", FS: fstest.MapFS{}}}})
	for _, name := range []string{"/alias", "/hard"} {
		f, err := p.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0600)
		if f != nil {
			f.Close()
		}
		wantPolicyError(t, err, "not-granted")
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "original" {
		t.Fatalf("hidden data=%q %v", b, err)
	}
}

func TestOverlappingMountsAllowWritableOverride(t *testing.T) {
	for _, separate := range []bool{false, true} {
		t.Run(map[bool]string{false: "same tree", true: "separate backend"}[separate], func(t *testing.T) {
			root := t.TempDir()
			protected := filepath.Join(root, "protected")
			uploads := filepath.Join(protected, "uploads")
			if err := os.MkdirAll(uploads, 0700); err != nil {
				t.Fatal(err)
			}
			if separate {
				uploads = t.TempDir()
			}
			if err := os.Symlink("protected/uploads", filepath.Join(root, "alias")); err != nil {
				t.Fatal(err)
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: protected}, {Path: "/protected/uploads", Dir: uploads, Write: true}}})
			f, err := p.OpenFile("/alias/new", os.O_WRONLY|os.O_CREATE, 0600)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			if _, err := os.Stat(filepath.Join(uploads, "new")); err != nil {
				t.Fatal(err)
			}
			if separate {
				if _, err := os.Stat(filepath.Join(protected, "uploads", "new")); !os.IsNotExist(err) {
					t.Fatalf("wrote hidden backend: %v", err)
				}
			}
		})
	}
}

func TestOverlappingMountsAllowUnprotectedHardLinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "protected"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: filepath.Join(root, "protected")}}})
	f, err := p.OpenFile("/alias", os.O_RDONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err := os.ReadFile(file)
	if err != nil || len(b) != 0 {
		t.Fatalf("legal hard link not truncated: %q %v", b, err)
	}
}

func TestOverlappingMountsSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"protected", "uploads"} {
		if err := os.Mkdir(filepath.Join(root, d), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, d, "file"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink("uploads", alias); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/protected", Dir: filepath.Join(root, "protected")}}})
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 500; i++ {
			target := "uploads"
			if i%2 == 0 {
				target = "protected"
			}
			tmp := filepath.Join(root, "swap")
			if err := os.Symlink(target, tmp); err != nil {
				done <- err
				return
			}
			if err := os.Rename(tmp, alias); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 500; i++ {
		f, _ := p.OpenFile("/alias/file", os.O_WRONLY|os.O_TRUNC, 0600)
		if f != nil {
			f.Close()
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "protected", "file"))
	if err != nil || string(b) != "original" {
		t.Fatalf("replacement corrupted protected data=%q %v", b, err)
	}
}

func TestOverlappingPhysicalRootsProtectWritableDescendantAlias(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/protected", Dir: root}, {Path: "/alias", Dir: child, Write: true}}})
	f, err := p.OpenFile("/alias/file", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "original" {
		t.Fatalf("nested backend=%q %v", b, err)
	}
}

func TestOverlappingPhysicalRootsCaseInsensitiveAncestor(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "Protected")
	child := filepath.Join(root, "sub")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(base, "protected", "sub")
	if _, err := os.Stat(lower); os.IsNotExist(err) {
		t.Skip("requires a case-insensitive filesystem")
	} else if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "file")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/protected", Dir: root}, {Path: "/alias", Dir: lower, Write: true}}})
	f, err := p.OpenFile("/alias/file", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "original" {
		t.Fatalf("case alias=%q %v", b, err)
	}
}

func TestOverlappingMountsRematchReadAliases(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protected, "hidden.php"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected/hidden.php", filepath.Join(root, "alias.php")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/protected", FS: fstest.MapFS{"visible.php": {Data: []byte("visible")}}}}})
	for _, name := range []string{"/protected/hidden.php", "/alias.php", "/alias/hidden.php"} {
		f, err := p.OpenFile(name, os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		if err == nil {
			t.Fatalf("read hidden file through %q", name)
		}
	}
	f, err := p.OpenFile("/alias/visible.php", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "visible" {
		t.Fatalf("resolved mounted read=%q %v", b, err)
	}
}

func TestOverlappingMountsRematchCaseInsensitiveReadAlias(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "Protected")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "protected")); os.IsNotExist(err) {
		t.Skip("requires a case-insensitive filesystem")
	} else if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protected, "hidden.php"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/protected", FS: fstest.MapFS{"visible.php": {Data: []byte("visible")}}}}})
	f, err := p.OpenFile("/Protected/hidden.php", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("case alias read hidden parent file")
	}
	f, err = p.OpenFile("/Protected/visible.php", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "visible" {
		t.Fatalf("case rematch=%q %v", b, err)
	}
}

func TestOverlappingMountsDoNotReadHiddenHardLink(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(protected, "hidden")
	if err := os.WriteFile(file, []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/protected", FS: fstest.MapFS{}}}})
	f, err := p.OpenFile("/alias", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
}

func TestOverlappingMountsPreserveReadableHardLinks(t *testing.T) {
	for _, sameBackend := range []bool{false, true} {
		root := t.TempDir()
		protected := filepath.Join(root, "protected")
		if err := os.Mkdir(protected, 0700); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(root, "visible")
		if sameBackend {
			source = filepath.Join(protected, "visible")
		}
		if err := os.WriteFile(source, []byte("visible"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(source, filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
		mount := FileGrant{Path: "/protected", FS: fstest.MapFS{}}
		if sameBackend {
			mount.FS = nil
			mount.Dir = protected
		}
		p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, mount}})
		f, err := p.OpenFile("/alias", os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(b) != "visible" {
			t.Fatalf("readable hard link=%q %v", b, err)
		}
	}
}
