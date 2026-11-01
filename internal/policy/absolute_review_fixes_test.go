package policy

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestReviewFixAbsoluteSymlinkMounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures require Unix privileges; Windows compilation is checked separately")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"app/sub/deep", "app2", "other", "outside"} {
		if err := os.MkdirAll(filepath.Join(base, n), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"app/file", "app/sub/file", "app2/file", "other/file", "outside/file"} {
		if err := os.WriteFile(filepath.Join(base, n), []byte(n), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("sub/deep", filepath.Join(base, "app", "deep")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: filepath.Join(base, "app"), Write: true}, {Path: "/other", Dir: filepath.Join(base, "other")}}})
	cases := []struct{ name, target, want, rule string }{
		{"same", base + "/app/file", "app/file", ""},
		{"cross", base + "/other/file", "other/file", ""},
		{"outside", base + "/outside/file", "", "outside-root"},
		{"prefix", base + "/app2/file", "", "outside-root"},
		{"parent", base + "/app/sub/../file", "app/file", ""},
		{"symlink-parent", base + "/app/deep/../file", "app/sub/file", ""},
		{"escape-return", base + "/app/../app/file", "", "outside-root"},
		{"target-boundary", base + "/other/../app/file", "", "outside-root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := os.Symlink(c.target, filepath.Join(base, "app", c.name)); err != nil {
				t.Fatal(err)
			}
			f, err := p.OpenFile("/app/"+c.name, os.O_RDONLY, 0)
			if c.rule != "" {
				if f != nil {
					f.Close()
				}
				wantPolicyError(t, err, c.rule)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			b, err := io.ReadAll(f)
			if err != nil || string(b) != c.want {
				t.Fatalf("read=%q error=%v", b, err)
			}
		})
	}
	f, err := p.OpenFile("/app/cross", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
	b, err := os.ReadFile(base + "/other/file")
	if err != nil || string(b) != "other/file" {
		t.Fatalf("protected=%q %v", b, err)
	}
}

func TestReviewFixAbsoluteSymlinkSwapDoesNotLeak(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink fixture")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(base+"/app", 0700); err != nil {
		t.Fatal(err)
	}
	for n, v := range map[string]string{"/app/safe": "safe", "/secret": "SECRET"} {
		if err := os.WriteFile(base+n, []byte(v), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(base+"/app/safe", base+"/app/link"); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: base + "/app"}}})
	// A successful stable read prevents this race test from passing by denying everything.
	f, err := p.OpenFile("/app/link", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	done := make(chan struct{})
	fail := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			target := base + "/app/safe"
			if i%2 == 0 {
				target = base + "/secret"
			}
			if err := os.Symlink(target, base+"/app/new"); err != nil {
				fail <- err
				return
			}
			if err := os.Rename(base+"/app/new", base+"/app/link"); err != nil {
				fail <- err
				return
			}
		}
	}()
	defer func() {
		close(done)
		wg.Wait()
		select {
		case err := <-fail:
			t.Error(err)
		default:
		}
	}()
	for i := 0; i < 500; i++ {
		f, err := p.OpenFile("/app/link", os.O_RDONLY, 0)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(b) != "safe" {
			t.Fatalf("leaked=%q error=%v", b, err)
		}
	}
}

func TestReviewFixAbsoluteLinkPreservesTrailingSeparator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink fixture")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/file", []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir+"/file/", dir+"/link"); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: dir}}})
	f, err := p.OpenFile("/app/link", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("trailing slash incorrectly treated a regular file as a directory")
	}
}

func TestReviewFixRootHardlinkLimitation(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside"), []byte("host-created-hardlink"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "outside"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: root}}})
	f, err := p.OpenFile("/app/alias", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "host-created-hardlink" {
		t.Fatalf("hardlink limitation changed: %q %v", b, err)
	}
	hostFile, ok := f.(*os.File)
	if !ok || !filepath.IsAbs(hostFile.Name()) {
		t.Fatalf("host-facing file name=%v", f)
	}
}

func TestReviewFixAbsoluteLinkUsesConfiguredRootAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink fixture")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := base + "/real"
	alias := base + "/alias"
	if err := os.MkdirAll(real+"/sub", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"file", "sub/file"} {
		if err := os.WriteFile(real+"/"+n, []byte(n), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for n, target := range map[string]string{"same": alias + "/file", "child": alias + "/sub/file", "escape": alias + "/sub/../file"} {
		if err := os.Symlink(target, real+"/"+n); err != nil {
			t.Fatal(err)
		}
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: alias, Write: true}, {Path: "/locked", Dir: real + "/sub"}}})
	for n, want := range map[string]string{"same": "file", "child": "sub/file"} {
		f, err := p.OpenFile("/app/"+n, os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(b) != want {
			t.Fatalf("got %q %v", b, err)
		}
	}
	f, err := p.OpenFile("/app/escape", os.O_RDONLY, 0)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "outside-root")
	f, err = p.OpenFile("/app/child", os.O_WRONLY|os.O_TRUNC, 0600)
	if f != nil {
		f.Close()
	}
	wantPolicyError(t, err, "not-granted")
}
