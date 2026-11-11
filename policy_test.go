package phego

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	ipolicy "github.com/HazelnutParadise/phego/internal/policy"
)

func newPolicyEngine(t *testing.T, cfg Config) *Engine {
	t.Helper()
	e, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine(%+v): %v", cfg, err)
	}
	return e
}

func wantConfigField(t *testing.T, err error, field string) {
	t.Helper()
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *ConfigError with field %q", err, field)
	}
	if ce.Field != field {
		t.Fatalf("ConfigError field = %q, want %q (%v)", ce.Field, field, err)
	}
}

func TestPolicyZeroConfigHasNoAdmission(t *testing.T) {
	e := newPolicyEngine(t, Config{})
	if e.policy == nil {
		t.Fatal("engine has no compiled policy")
	}
	if e.policy.admission != nil {
		t.Fatalf("admission = %+v, want nil when MaxActive is zero", e.policy.admission)
	}
}

func TestRunFileDeniedWithoutPolicy(t *testing.T) {
	e := newPolicyEngine(t, Config{})
	r, err := e.RunFile(context.Background(), "a.php")
	if r == nil || r.Status != StatusPolicyDenied {
		t.Fatalf("result = %+v, want StatusPolicyDenied", r)
	}
	var pe *PolicyError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want *PolicyError", err)
	}
	if pe.Capability != CapabilityFileRead || pe.Op != "open" || pe.Rule != RuleNotGranted {
		t.Fatalf("PolicyError = %+v, want file-read open not-granted", pe)
	}
}

func TestRunFileLegacyFilesCompatibility(t *testing.T) {
	ctx := context.Background()
	e := newPolicyEngine(t, Config{Files: fstest.MapFS{
		"a.php":   {Data: []byte("<?php return 1;")},
		"bad.php": {Data: []byte("<?php 1 <")},
	}})
	for _, path := range []string{"a.php"} {
		r, err := e.RunFile(ctx, path)
		if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 1 {
			t.Fatalf("RunFile(%q) = %+v, %v; want the file read, compiled and run", path, r, err)
		}
	}
	for _, path := range []string{"bad.php"} {
		_, err := e.RunFile(ctx, path)
		var pe *PHPError
		if !errors.As(err, &pe) {
			t.Fatalf("RunFile(%q) error = %v, want *PHPError", path, err)
		}
		if pe.File != path {
			t.Fatalf("PHPError.File = %q, want %q", pe.File, path)
		}
	}
}

func TestPolicyFileMounts(t *testing.T) {
	ctx := context.Background()
	e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{
		{Path: "/app", FS: fstest.MapFS{"x.php": {Data: []byte("<?php return 1;")}}},
	}}})
	if r, err := e.RunFile(ctx, "/app/x.php"); !ranToCompletion(r, err) {
		t.Fatalf("mounted file = %+v, %v; want it run", r, err)
	}
	r, err := e.RunFile(ctx, "/etc/x.php")
	if r == nil || r.Status != StatusPolicyDenied {
		t.Fatalf("outside mount result = %+v, want StatusPolicyDenied", r)
	}
	var pe *PolicyError
	if !errors.As(err, &pe) || pe.Rule != RuleOutsideRoot {
		t.Fatalf("outside mount error = %v, want *PolicyError with RuleOutsideRoot", err)
	}
	_, err = e.RunFile(ctx, "x.php")
	var rel *PolicyError
	if !errors.As(err, &rel) {
		t.Fatalf("relative path error = %v, want *PolicyError", err)
	}
	if rel.Rule != RuleNotAbsolute && rel.Rule != RuleOutsideRoot {
		t.Fatalf("relative path rule = %q, want %q or %q", rel.Rule, RuleNotAbsolute, RuleOutsideRoot)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "y.php"), []byte("<?php return 2;"), 0o644); err != nil {
		t.Fatal(err)
	}
	rooted := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{{Path: "/root", Root: dir}}}})
	if r, err := rooted.RunFile(ctx, "/root/y.php"); !ranToCompletion(r, err) {
		t.Fatalf("root mount = %+v, %v; want it run", r, err)
	} else if n, _ := r.Return.Int64(); n != 2 {
		t.Fatalf("root mount returned %+v, want 2", r.Return)
	}
}

func TestPolicyLegacyFilesConflict(t *testing.T) {
	_, err := NewEngine(Config{
		Files:  fstest.MapFS{"a.php": {Data: []byte("<?php")}},
		Policy: Policy{Files: []FileGrant{{Path: "/", FS: fstest.MapFS{"b.php": {Data: []byte("<?php")}}}}},
	})
	wantConfigField(t, err, "Files")
}

func TestPolicyValidationFields(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		cfg   Config
		field string
	}{
		{"mode", Config{Policy: Policy{Mode: Mode(9)}}, "Policy.Mode"},
		{"relative file path", Config{Policy: Policy{Files: []FileGrant{{Path: "app", FS: fstest.MapFS{}}}}}, "Policy.Files[0].Path"},
		{"unclean file path", Config{Policy: Policy{Files: []FileGrant{{Path: "/app/", FS: fstest.MapFS{}}}}}, "Policy.Files[0].Path"},
		{"file root and fs", Config{Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir, FS: fstest.MapFS{}}}}}, "Policy.Files[0]"},
		{"file neither root nor fs", Config{Policy: Policy{Files: []FileGrant{{Path: "/app"}}}}, "Policy.Files[0]"},
		{"file fs write", Config{Policy: Policy{Files: []FileGrant{{Path: "/app", FS: fstest.MapFS{}, Write: true}}}}, "Policy.Files[0].Write"},
		{"network no ports", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "a.com"}}}}, "Policy.Network[0]"},
		{"network port zero", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "a.com", Ports: []int{0}}}}}, "Policy.Network[0]"},
		{"network port high", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "a.com", Ports: []int{65536}}}}}, "Policy.Network[0]"},
		{"limits steps", Config{Limits: Limits{Steps: -1}}, "Limits.Steps"},
		{"limits memory", Config{Limits: Limits{Memory: -1}}, "Limits.Memory"},
		{"limits output", Config{Limits: Limits{Output: -1}}, "Limits.Output"},
		{"limits diagnostics", Config{Limits: Limits{Diagnostics: -1}}, "Limits.Diagnostics"},
		{"limits frames", Config{Limits: Limits{Frames: -1}}, "Limits.Frames"},
		{"limits open files", Config{Limits: Limits{OpenFiles: -1}}, "Limits.OpenFiles"},
		{"limits connections", Config{Limits: Limits{Connections: -1}}, "Limits.Connections"},
		{"limits compile bytes", Config{Limits: Limits{CompileBytes: -1}}, "Limits.CompileBytes"},
		{"max active", Config{MaxActive: -1}, "MaxActive"},
		{"max queued", Config{MaxActive: 1, MaxQueued: -1}, "MaxQueued"},
		{"queue without active", Config{MaxQueued: 1}, "MaxQueued"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEngine(tc.cfg)
			wantConfigField(t, err, tc.field)
		})
	}
}

func TestPolicyLimitFolding(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want int64
	}{
		{"deprecated output limit only", Config{OutputLimit: 1234}, 1234},
		{"limits output only", Config{Limits: Limits{Output: 4321}}, 4321},
		{"both same", Config{OutputLimit: 5000, Limits: Limits{Output: 5000}}, 5000},
		{"neither", Config{}, 8 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPolicyEngine(t, tc.cfg)
			if got := e.policy.limits[ipolicy.Output]; got != tc.want {
				t.Fatalf("Output limit = %d, want %d", got, tc.want)
			}
		})
	}
	_, err := NewEngine(Config{OutputLimit: 1, Limits: Limits{Output: 2}})
	wantConfigField(t, err, "OutputLimit")

	e := newPolicyEngine(t, Config{})
	if got := e.policy.limits[ipolicy.Diagnostics]; got != 10000 {
		t.Fatalf("Diagnostics limit = %d, want 10000", got)
	}
	for _, r := range []ipolicy.Resource{
		ipolicy.Steps, ipolicy.Memory, ipolicy.Frames,
		ipolicy.OpenFiles, ipolicy.Connections, ipolicy.CompileBytes,
	} {
		if got, ok := e.policy.limits[r]; ok && got != 0 {
			t.Errorf("unset limit %s = %d, want absent or zero", r, got)
		}
	}
}

func TestAdmitBoundsAndRunDenies(t *testing.T) {
	ctx := context.Background()
	e := newPolicyEngine(t, Config{MaxActive: 1})
	if e.policy.admission == nil {
		t.Fatal("admission is nil with MaxActive 1")
	}
	release, err := e.admit(ctx)
	if err != nil {
		t.Fatalf("first admit: %v", err)
	}
	_, err = e.admit(ctx)
	var re *ResourceError
	if !errors.As(err, &re) || re.Resource != ResourceExecutions {
		t.Fatalf("second admit error = %v, want *ResourceError for executions", err)
	}
	release()
	second, err := e.admit(ctx)
	if err != nil {
		t.Fatalf("admit after release: %v", err)
	}
	release()
	if _, err := e.admit(ctx); !errors.As(err, &re) || re.Resource != ResourceExecutions {
		t.Fatalf("double release returned a second slot: %v", err)
	}
	second()
	third, err := e.admit(ctx)
	if err != nil {
		t.Fatalf("admit after the second release: %v", err)
	}
	third()

	p, err := e.Compile(ctx, "a.php", "<?php return 1;")
	if err != nil {
		t.Fatal(err)
	}
	hold, err := e.admit(ctx)
	if err != nil {
		t.Fatalf("hold slot: %v", err)
	}
	r, err := e.Run(ctx, p)
	hold()
	if r == nil || r.Status != StatusPolicyDenied {
		t.Fatalf("full engine Run result = %+v, want StatusPolicyDenied", r)
	}
	if !errors.As(err, &re) || re.Resource != ResourceExecutions {
		t.Fatalf("full engine Run error = %v, want *ResourceError for executions", err)
	}

	queued := newPolicyEngine(t, Config{MaxActive: 1, MaxQueued: 1})
	slot, err := queued.admit(ctx)
	if err != nil {
		t.Fatalf("hold queued slot: %v", err)
	}
	qctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := queued.admit(qctx)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued admit error = %v, want context.Canceled", err)
	}
	slot()

	closed := newPolicyEngine(t, Config{})
	p, err = closed.Compile(ctx, "a.php", "<?php return 1;")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = closed.Run(ctx, p)
	if !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("Run after Close error = %v, want ErrEngineClosed", err)
	}
	var pe *PolicyError
	if errors.As(err, &pe) {
		t.Fatalf("Run after Close returned *PolicyError: %v", err)
	}
}

func TestPolicySnapshotIsolation(t *testing.T) {
	ctx := context.Background()
	files := []FileGrant{{Path: "/app", FS: fstest.MapFS{"x.php": {Data: []byte("<?php return 1;")}}}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	ports := []int{port}
	network := []NetworkGrant{{Scheme: "tcp", Host: "127.0.0.1", Ports: ports, AllowPrivate: true}}
	e := newPolicyEngine(t, Config{Policy: Policy{Files: files, Network: network}})
	if e.policy == nil || e.policy.policy == nil {
		t.Fatal("engine has no compiled policy")
	}

	files[0].Path = "/evil"
	ports[0] = 1

	if r, err := e.RunFile(ctx, "/app/x.php"); !ranToCompletion(r, err) {
		t.Fatalf("original mount changed with the caller's slice: %+v %v", r, err)
	}
	if _, err := e.RunFile(ctx, "/evil/x.php"); !errors.As(err, new(*PolicyError)) {
		t.Fatalf("mutated mount path was accepted: %v", err)
	}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := e.policy.policy.DialContext(dctx, "tcp", "tcp", "127.0.0.1", port)
	if err != nil {
		t.Fatalf("original network port changed with the caller's slice: %v", err)
	}
	conn.Close()
}

func TestCallbackRegistrationInPolicy(t *testing.T) {
	e := newPolicyEngine(t, Config{HostFunctions: []HostFunction{{Name: "Lookup", MaxArgs: -1, Func: noop}}})
	if e.policy == nil || e.policy.policy == nil {
		t.Fatal("engine has no compiled policy")
	}
	for _, name := range []string{"lookup", "LOOKUP", "LookUp"} {
		if err := e.policy.policy.Allow(ipolicy.Callback, "call", name); err != nil {
			t.Errorf("registered callback %q denied: %v", name, err)
		}
	}
	var pe *ipolicy.PolicyError
	if err := e.policy.policy.Allow(ipolicy.Callback, "call", "other"); !errors.As(err, &pe) || pe.Rule != "not-granted" {
		t.Fatalf("unregistered callback error = %v, want not-granted", err)
	}
	if _, err := NewEngine(Config{HostFunctions: []HostFunction{{Name: "fetch", MaxArgs: -1, Capabilities: []string{"net"}, Func: noop}}}); err != nil {
		t.Fatalf("host capability without a grant: %v", err)
	}
}

func TestPolicyAndResourceErrorStrings(t *testing.T) {
	e := newPolicyEngine(t, Config{})
	if _, err := e.RunFile(context.Background(), "a.php"); !errors.As(err, new(*PolicyError)) {
		t.Fatalf("RunFile error = %v, want *PolicyError", err)
	}
	pe := &PolicyError{Capability: CapabilityFileRead, Op: "open", Target: "/x", Rule: RuleNotGranted}
	if got, want := pe.Error(), `phego: file-read open "/x" refused (not-granted)`; got != want {
		t.Fatalf("PolicyError.Error() = %q, want %q", got, want)
	}
	re := &ResourceError{Resource: ResourceOutput, Limit: 10, Used: 8, Requested: 3}
	if got, want := re.Error(), "phego: output limit 10 exceeded (used 8, requested 3)"; got != want {
		t.Fatalf("ResourceError.Error() = %q, want %q", got, want)
	}
}
