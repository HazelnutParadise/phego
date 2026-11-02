package policy

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestRootPathsResolveSymlinksBeforeParentComponents(t *testing.T) {
	for _, truncate := range []bool{false, true} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "sub", "deep"), 0700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"file": "root", "sub/file": "sub"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink("sub/deep", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}}})
		flags := os.O_RDONLY
		if truncate {
			flags = os.O_WRONLY | os.O_TRUNC
		}
		f, err := p.OpenFile("/link/../file", flags, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if !truncate {
			b, e := io.ReadAll(f)
			if e != nil || string(b) != "sub" {
				f.Close()
				t.Fatalf("read=%q %v, want sub/file", b, e)
			}
		}
		f.Close()
		b, err := os.ReadFile(filepath.Join(root, "file"))
		if err != nil || string(b) != "root" {
			t.Fatalf("wrong file modified=%q %v", b, err)
		}
		if truncate {
			b, err := os.ReadFile(filepath.Join(root, "sub", "file"))
			if err != nil || len(b) != 0 {
				t.Fatalf("target not truncated=%q %v", b, err)
			}
		}
	}
}

func TestRootPathsDoNotTreatFilesAsDirectories(t *testing.T) {
	for _, name := range []string{"/file/.", "/file/", "/missing/../file"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "file"), []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}}})
			f, err := p.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0600)
			if f != nil {
				f.Close()
			}
			if err == nil {
				t.Errorf("opened non-directory path %q", name)
			}
			b, err := os.ReadFile(filepath.Join(root, "file"))
			if err != nil || string(b) != "original" {
				t.Fatalf("wrong file modified=%q %v", b, err)
			}
		})
	}
}

func TestRootPathsTraverseVirtualMountParents(t *testing.T) {
	app := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "file"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: app}, {Path: "/other", Dir: other, Write: true}}})
	f, err := p.OpenFile("/app/../other/file", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(b) != "other" {
		t.Fatalf("virtual parent read=%q %v", b, err)
	}
	f, err = p.OpenFile("/app/../other/file", os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err = os.ReadFile(filepath.Join(other, "file"))
	if err != nil || len(b) != 0 {
		t.Fatalf("virtual parent write=%q %v", b, err)
	}
}

func TestRootPathsResolveIntoVirtualMountWithoutHostDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("virtual", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/virtual", FS: fstest.MapFS{"file": {Data: []byte("mounted")}}}}})
	f, err := p.OpenFile("/alias/file", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "mounted" {
		t.Fatalf("virtual-only mount=%q %v", b, err)
	}
}

func TestFSParentPreservesUnresolvedRootPath(t *testing.T) {
	for _, truncate := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "truncate"}[truncate], func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "sub", "deep"), 0700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{"file": "root", "sub/file": "sub"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("sub/deep", filepath.Join(root, "link")); err != nil {
				t.Fatal(err)
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", FS: fstest.MapFS{}}, {Path: "/other", Dir: root, Write: true}}})
			flag := os.O_RDONLY
			if truncate {
				flag = os.O_WRONLY | os.O_TRUNC
			}
			f, err := p.OpenFile("/app/../other/link/../file", flag, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if !truncate {
				b, e := io.ReadAll(f)
				if e != nil || string(b) != "sub" {
					t.Errorf("wrong file read=%q %v", b, e)
				}
			}
			f.Close()
			b, err := os.ReadFile(filepath.Join(root, "file"))
			if err != nil || string(b) != "root" {
				t.Fatalf("wrong file modified=%q %v", b, err)
			}
		})
	}
}

func TestFSParentRequiresExistingDirectory(t *testing.T) {
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", FS: fstest.MapFS{"file": {Data: []byte("file")}, "other": {Data: []byte("other")}, "dir/x": {Data: []byte("x")}}}}})
	for _, name := range []string{"/app/file/../other", "/app/missing/../other"} {
		f, err := p.OpenFile(name, os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		if err == nil {
			t.Errorf("invalid directory path read through %q", name)
		}
	}
	f, err := p.OpenFile("/app/dir/../other", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestRootPathsResolveNestedVirtualAncestors(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("virtual/deep", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/virtual/deep", FS: fstest.MapFS{"file": {Data: []byte("mounted")}}}}})
	for _, name := range []string{"/virtual/deep/file", "/alias/file"} {
		f, err := p.OpenFile(name, os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(b) != "mounted" {
			t.Errorf("read %q=%q %v", name, b, err)
		}
	}
}

func TestVirtualMountAncestorsDoNotCreateHostFiles(t *testing.T) {
	root := t.TempDir()
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/virtual/deep", FS: fstest.MapFS{}}}})
	f, err := p.OpenFile("/virtual/ghost", os.O_WRONLY|os.O_CREATE, 0600)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("created a file under an unbacked virtual directory")
	}
}

func TestFSPathsResolveSymlinksBeforeParentComponents(t *testing.T) {
	source := fstest.MapFS{"file": {Data: []byte("root")}, "sub/file": {Data: []byte("sub")}, "sub/deep/empty": {}, "link": {Mode: os.ModeSymlink, Data: []byte("sub/deep")}}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", FS: source}}})
	f, err := p.OpenFile("/app/link/../file", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "sub" {
		t.Fatalf("FS symlink parent=%q %v", b, err)
	}
}

func TestReadOnlyParentsResolveWritableChildAliases(t *testing.T) {
	for _, separate := range []bool{false, true} {
		t.Run(fmtBool(separate), func(t *testing.T) {
			root := t.TempDir()
			data := filepath.Join(root, "data")
			if err := os.Mkdir(data, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(data, "file"), []byte("hidden"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("data", filepath.Join(root, "alias")); err != nil {
				t.Fatal(err)
			}
			writable := data
			if separate {
				writable = t.TempDir()
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root}, {Path: "/data", Dir: writable, Write: true}}})
			f, err := p.OpenFile("/alias/file", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.(io.Writer).Write([]byte("new")); err != nil {
				f.Close()
				t.Fatal(err)
			}
			f.Close()
			b, err := os.ReadFile(filepath.Join(writable, "file"))
			if err != nil || string(b) != "new" {
				t.Fatalf("writable child=%q %v", b, err)
			}
			if separate {
				b, err := os.ReadFile(filepath.Join(data, "file"))
				if err != nil || string(b) != "hidden" {
					t.Fatalf("hidden backing changed: %q %v", b, err)
				}
			}
			f, err = p.OpenFile("/ungranted", os.O_CREATE|os.O_WRONLY, 0600)
			if f != nil {
				f.Close()
			}
			wantPolicyError(t, err, "not-granted")
		})
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: fstest.MapFS{"alias": {Mode: os.ModeSymlink, Data: []byte("data")}}}, {Path: "/data", Dir: t.TempDir(), Write: true}}})
	f, err := p.OpenFile("/alias/new", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func fmtBool(b bool) string {
	if b {
		return "separate"
	}
	return "same"
}

type foldingFS struct{ fs.FS }

func (f foldingFS) Open(name string) (fs.File, error) { return f.FS.Open(strings.ToLower(name)) }

type opaqueFS struct{ fs.FS }
type opaqueFile struct{ fs.File }

func (f opaqueFS) Open(name string) (fs.File, error) {
	v, e := f.FS.Open(name)
	if e != nil {
		return nil, e
	}
	return opaqueFile{v}, nil
}

func TestFSMountsRejectUnlistedCaseAliases(t *testing.T) {
	backing := foldingFS{fstest.MapFS{"protected/hidden": {Data: []byte("secret")}, "public/file": {Data: []byte("public")}}}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: backing}, {Path: "/protected", FS: fstest.MapFS{}}}})
	for _, name := range []string{"/Protected/hidden", "/PROTECTED/hidden"} {
		f, err := p.OpenFile(name, os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		if err == nil {
			t.Fatalf("read hidden data via %s", name)
		}
	}
	f, err := p.OpenFile("/public/file", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestFSMountsRequireBoundedDirectoryEnumeration(t *testing.T) {
	backing := opaqueFS{fstest.MapFS{"public": {Data: []byte("ok")}}}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: backing}, {Path: "/protected", FS: fstest.MapFS{}}}})
	f, err := p.OpenFile("/public", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("unverifiable FS accepted beside shadow mount")
	}
	p = mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: backing}}})
	f, err = p.OpenFile("/public", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestFSVirtualAncestorsDoNotEnableCaseAliases(t *testing.T) {
	backing := foldingFS{fstest.MapFS{"protected/hidden": {Data: []byte("secret")}}}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: backing}, {Path: "/protected", FS: fstest.MapFS{}}, {Path: "/Protected/deep", FS: fstest.MapFS{"file": {Data: []byte("ok")}}}}})
	f, err := p.OpenFile("/Protected/hidden", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("virtual ancestor enabled hidden case alias")
	}
	f, err = p.OpenFile("/Protected/deep/file", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestSymlinkTargetsResolveNestedLinksBeforeParents(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmtBool(memory), func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "sub/deep"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "file"), []byte("root"), 0600); err != nil {
				t.Fatal(err)
			}
			for name, target := range map[string]string{"deep": "sub/deep", "link": "deep/../../file", "escape": "../file"} {
				if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			grant := FileGrant{Path: "/app", Dir: root, Write: true}
			if memory {
				grant.Dir = ""
				grant.Write = false
				grant.FS = fstest.MapFS{"sub/deep": {Mode: fs.ModeDir}, "file": {Data: []byte("root")}, "deep": {Mode: os.ModeSymlink, Data: []byte("sub/deep")}, "link": {Mode: os.ModeSymlink, Data: []byte("deep/../../file")}, "escape": {Mode: os.ModeSymlink, Data: []byte("../file")}}
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{grant, {Path: "/", FS: fstest.MapFS{"file": {Data: []byte("outside")}}}}})
			f, err := p.OpenFile("/app/link", os.O_RDONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil || string(b) != "root" {
				t.Fatalf("nested target=%q %v", b, err)
			}
			f, err = p.OpenFile("/app/escape", os.O_RDONLY, 0)
			if f != nil {
				f.Close()
			}
			wantPolicyError(t, err, "outside-root")
		})
	}
}

func TestShadowedEscapingSymlinkDoesNotBlockOtherDirectories(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data/file"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "data/file"), filepath.Join(root, "data/hard")); err != nil {
		t.Fatal(err)
	}
	// Keep the escape target ungranted: absolute links into the authorized
	// /shadow directory are now intentionally supported.
	for name, target := range map[string]string{"shadow": outside, "escape": t.TempDir()} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, {Path: "/shadow", Dir: outside}}})
	for _, name := range []string{"/data/file", "/data/hard"} {
		for _, flag := range []int{os.O_RDONLY, os.O_WRONLY} {
			f, err := p.OpenFile(name, flag, 0600)
			if err != nil {
				t.Fatalf("%s flag%d: %v", name, flag, err)
			}
			f.Close()
		}
	}
	f, err := p.OpenFile("/escape/file", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "outside-root")
}

func TestOverlappingFSRejectsUnresolvableSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "protected"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected/hidden"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected/hidden", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../protected/hidden", filepath.Join(root, "public/alias")); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: struct{ fs.FS }{r.FS()}}, {Path: "/protected", FS: fstest.MapFS{}}}})
	for _, name := range []string{"/alias", "/public/alias"} {
		f, err := p.OpenFile(name, os.O_RDONLY, 0)
		if f != nil {
			f.Close()
		}
		if err == nil {
			t.Fatalf("unresolvable symlink read via %s", name)
		}
	}
}

func TestSymlinkBoundariesSurviveMountRemapping(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmtBool(memory), func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "sub/deep"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "file"), []byte("inside"), 0600); err != nil {
				t.Fatal(err)
			}
			for name, target := range map[string]string{"deep": "sub/deep", "good": "deep/../../file", "bad": "sub/../../file"} {
				if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
			}
			source := FileGrant{Path: "/app", Dir: root}
			child := FileGrant{Path: "/app/sub", Dir: filepath.Join(root, "sub")}
			if memory {
				source.Dir = ""
				source.FS = fstest.MapFS{"deep": {Mode: os.ModeSymlink, Data: []byte("sub/deep")}, "good": {Mode: os.ModeSymlink, Data: []byte("deep/../../file")}, "bad": {Mode: os.ModeSymlink, Data: []byte("sub/../../file")}, "file": {Data: []byte("inside")}}
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", FS: fstest.MapFS{"file": {Data: []byte("outside")}}}, source, child}})
			f, err := p.OpenFile("/app/good", os.O_RDONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil || string(b) != "inside" {
				t.Fatalf("remapped target=%q %v", b, err)
			}
			f, err = p.OpenFile("/app/bad", os.O_RDONLY, 0)
			if f != nil {
				f.Close()
			}
			wantPolicyError(t, err, "outside-root")
			f, err = p.OpenFile("/app/sub/../../file", os.O_RDONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			b, err = io.ReadAll(f)
			f.Close()
			if err != nil || string(b) != "outside" {
				t.Fatalf("requested parent=%q %v", b, err)
			}
		})
	}
}
