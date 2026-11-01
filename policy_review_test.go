package phego

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

type reviewCountingFS struct {
	opens atomic.Int32
	files fs.FS
}

func (f *reviewCountingFS) Open(name string) (fs.File, error) {
	f.opens.Add(1)
	return f.files.Open(name)
}

func TestReviewRunFileAdmitsBeforeOpening(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		f := &reviewCountingFS{files: fstest.MapFS{"a.php": {Data: []byte("<?php return 1;")}}}
		cfg := Config{MaxActive: 1}
		name := "/app/a.php"
		if legacy {
			cfg.Files = f
			name = "a.php"
		} else {
			cfg.Policy.Files = []FileGrant{{Path: "/app", FS: f}}
		}
		e := newPolicyEngine(t, cfg)
		release, err := e.admit(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.RunFile(context.Background(), name)
		release()
		var re *ResourceError
		if !errors.As(err, &re) || r.Status != StatusPolicyDenied {
			t.Fatalf("full RunFile=%+v %v", r, err)
		}
		if f.opens.Load() != 0 {
			t.Fatalf("source opened %d times before admission", f.opens.Load())
		}
		_, err = e.RunFile(context.Background(), name)
		var ue *UnsupportedError
		if !errors.As(err, &ue) {
			t.Fatalf("one slot must suffice through file execution: %v", err)
		}
		if e.policy.admission.Active() != 0 {
			t.Fatal("RunFile leaked slot")
		}
	}
}

func TestReviewLegacyRunFilePreservesFSPaths(t *testing.T) {
	files := fstest.MapFS{"a.php": {Data: []byte("<?php return 1;")}}
	e := newPolicyEngine(t, Config{Files: files})
	for _, name := range []string{"/a.php", "../a.php", "./a.php", "x/../a.php", "", "missing.php"} {
		_, old := files.Open(name)
		_, got := e.RunFile(context.Background(), name)
		var wantPE, gotPE *fs.PathError
		if !errors.As(old, &wantPE) || !errors.As(got, &gotPE) || wantPE.Op != gotPE.Op || wantPE.Path != gotPE.Path || !errors.Is(gotPE.Err, wantPE.Err) {
			t.Errorf("%q old=%v new=%v", name, old, got)
		}
	}
}

func TestReviewRootErrorUsesPublicField(t *testing.T) {
	_, err := NewEngine(Config{Policy: Policy{Files: []FileGrant{{Path: "/app", Root: "relative"}}}})
	wantConfigField(t, err, "Policy.Files[0].Root")
}

func TestReviewRunFileDoesNotReadShadowedAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "protected"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "hidden.php"), []byte("<?php function HIDDEN_SENTINEL( {"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("protected/hidden.php", filepath.Join(root, "alias.php")); err != nil {
		t.Fatal(err)
	}
	e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{{Path: "/", Root: root}, {Path: "/protected", FS: fstest.MapFS{}}}}})
	for _, name := range []string{"/protected/hidden.php", "/alias.php"} {
		_, err := e.RunFile(context.Background(), name)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("RunFile(%q)=%v, want hidden backend absent", name, err)
		}
	}
}
