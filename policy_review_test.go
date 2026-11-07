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

// Embedding only fs.File deliberately hides optional directory enumeration.
type reviewOpaqueFile struct{ fs.File }
type reviewOpaqueFS struct{ fs.FS }

func (f reviewOpaqueFS) Open(name string) (fs.File, error) {
	v, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return reviewOpaqueFile{v}, nil
}

func TestReviewLegacyFilesMixedMountsRequireVerifiableDirectories(t *testing.T) {
	files := fstest.MapFS{"a.php": {Data: []byte("<?php return 1;")}, "protected/hidden.php": {Data: []byte("<?php function HIDDEN_SENTINEL( {")}}
	opaque := reviewOpaqueFS{files}
	only := newPolicyEngine(t, Config{Files: opaque})
	_, err := only.RunFile(context.Background(), "a.php")
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("standalone legacy Files changed: %v", err)
	}
	mixed := newPolicyEngine(t, Config{Files: opaque, Policy: Policy{Files: []FileGrant{{Path: "/protected", FS: fstest.MapFS{}}}}})
	_, err = mixed.RunFile(context.Background(), "/a.php")
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unverifiable mixed parent=%v, want permission denial", err)
	}
	verified := newPolicyEngine(t, Config{Files: files, Policy: Policy{Files: []FileGrant{{Path: "/protected", FS: fstest.MapFS{}}}}})
	_, err = verified.RunFile(context.Background(), "/a.php")
	if !errors.As(err, &unsupported) {
		t.Fatalf("verified mixed parent refused: %v", err)
	}
	_, err = verified.RunFile(context.Background(), "/protected/hidden.php")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("shadowed legacy source leaked: %v", err)
	}
}

func TestReviewOverlappingDiskFSRequiresRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "protected"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "hidden.php"), []byte("<?php function HIDDEN_SENTINEL( {"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "protected", "hidden.php"), filepath.Join(root, "alias.php")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public.php"), []byte("<?php return 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	disk, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	for _, useRoot := range []bool{false, true} {
		name := "FS"
		grant := FileGrant{Path: "/", FS: disk.FS()}
		if useRoot {
			name = "Root"
			grant.FS = nil
			grant.Root = root
		}
		t.Run(name, func(t *testing.T) {
			e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{grant, {Path: "/protected", FS: fstest.MapFS{}}}}})
			result, err := e.RunFile(context.Background(), "/alias.php")
			var denied *PolicyError
			if !errors.As(err, &denied) || denied.Rule != RuleNotGranted || result.Status != StatusPolicyDenied {
				t.Fatalf("hidden hardlink not denied: %+v %v", result, err)
			}
			_, err = e.RunFile(context.Background(), "/public.php")
			if useRoot {
				var unsupported *UnsupportedError
				if !errors.As(err, &unsupported) {
					t.Fatalf("ordinary Root file refused: %v", err)
				}
			} else if !errors.As(err, &denied) {
				t.Fatalf("overlapping disk FS should require Root: %v", err)
			}
		})
	}
	for _, legacy := range []bool{false, true} {
		cfg := Config{Policy: Policy{Files: []FileGrant{{Path: "/", FS: disk.FS()}}}}
		name := "/public.php"
		if legacy {
			cfg = Config{Files: disk.FS()}
			name = "public.php"
		}
		e := newPolicyEngine(t, cfg)
		_, err := e.RunFile(context.Background(), name)
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("standalone source changed: %v", err)
		}
	}
	virtual := fstest.MapFS{"public.php": {Data: []byte("<?php return 1;"), Sys: struct{}{}}}
	e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{{Path: "/", FS: virtual}, {Path: "/protected", FS: fstest.MapFS{}}}}})
	_, err = e.RunFile(context.Background(), "/public.php")
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("virtual metadata mistaken for disk: %v", err)
	}
}

// A script must be a regular file, with the results main had before
// character devices became readable: under a Root mount the policy refuses
// a device by its PHP path, including through an absolute link from another
// mount; an fs.FS mount or a standalone Files reports the device, like a
// directory, as a non-regular file.
func TestRunFileRefusesDevices(t *testing.T) {
	app := t.TempDir()
	if err := os.Symlink("/dev/null", filepath.Join(app, "null.php")); err != nil {
		t.Fatal(err)
	}
	device := fstest.MapFS{"x.php": {Mode: fs.ModeDevice | fs.ModeCharDevice}}
	root := Config{Policy: Policy{Files: []FileGrant{{Path: "/devices", Root: "/dev"}, {Path: "/app", Root: app}}}}
	for _, tc := range []struct {
		name   string
		cfg    Config
		path   string
		target string // the PolicyError target, or "" for StatusFailed
	}{
		{"root device", root, "/devices/null", "/devices/null"},
		{"root link to device", root, "/app/null.php", "/devices/null"},
		{"root directory", root, "/devices", ""},
		{"fs mount device", Config{Policy: Policy{Files: []FileGrant{{Path: "/", FS: device}}}}, "/x.php", ""},
		{"standalone files device", Config{Files: device}, "x.php", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := newPolicyEngine(t, tc.cfg).RunFile(context.Background(), tc.path)
			if tc.target != "" {
				var pe *PolicyError
				if res == nil || res.Status != StatusPolicyDenied || !errors.As(err, &pe) ||
					*pe != (PolicyError{Capability: CapabilityFileRead, Op: "open", Target: tc.target, Rule: RuleNotGranted}) {
					t.Fatalf("RunFile(%s) = %+v, %#v; want StatusPolicyDenied not-granted on %s", tc.path, res, err, tc.target)
				}
				return
			}
			var pathErr *fs.PathError
			if res == nil || res.Status != StatusFailed || !errors.As(err, &pathErr) || !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("RunFile(%s) = %+v, %#v; want StatusFailed with an fs.ErrInvalid path error", tc.path, res, err)
			}
		})
	}
}
