package phego

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/diag/codes"
)

func TestConfigErrorCodes(t *testing.T) {
	ok := func(_ HostContext, _ []Value) (Value, error) { return Null(), nil }
	mfs := fstest.MapFS{}
	cases := []struct {
		name string
		cfg  Config
		code diag.Code
	}{
		{"int size", Config{IntSize: 7}, codes.ConfigIntSize},
		{"release", Config{Release: "9.0.0"}, codes.ConfigRelease},
		{"negative source", Config{MaxSourceBytes: -1}, codes.ConfigNegative},
		{"host name", Config{HostFunctions: []HostFunction{{Name: "1x", Func: ok}}}, codes.HostFuncName},
		{"host nil", Config{HostFunctions: []HostFunction{{Name: "f"}}}, codes.HostFuncNil},
		{"host args", Config{HostFunctions: []HostFunction{{Name: "f", Func: ok, MinArgs: -1}}}, codes.HostFuncArgs},
		{"host capability", Config{HostFunctions: []HostFunction{{Name: "f", Func: ok, Capabilities: []string{""}}}}, codes.HostFuncCapability},
		{"host duplicate", Config{HostFunctions: []HostFunction{{Name: "f", Func: ok}, {Name: "F", Func: ok}}}, codes.HostFuncDuplicate},
		{"mode", Config{Policy: Policy{Mode: 99}}, codes.PolicyMode},
		{"files root", Config{Files: mfs, Policy: Policy{Files: []FileGrant{{Path: "/", FS: mfs}}}}, codes.FilesRootConflict},
		{"negative limit", Config{Limits: Limits{Steps: -1}}, codes.ConfigNegative},
		{"output conflict", Config{OutputLimit: 5, Limits: Limits{Output: 6}}, codes.OutputLimitConflict},
		{"negative active", Config{MaxActive: -1}, codes.ConfigNegative},
		{"queue", Config{MaxQueued: 1}, codes.QueueWithoutActive},
		{"mount path", Config{Policy: Policy{Files: []FileGrant{{Path: "app", FS: mfs}}}}, codes.MountPath},
		{"mount source", Config{Policy: Policy{Files: []FileGrant{{Path: "/app"}}}}, codes.MountSource},
		{"mount root", Config{Policy: Policy{Files: []FileGrant{{Path: "/app", Root: "rel"}}}}, codes.MountRoot},
		{"mount read-only", Config{Policy: Policy{Files: []FileGrant{{Path: "/app", FS: mfs, Write: true}}}}, codes.MountReadOnly},
		{"mount twice", Config{Policy: Policy{Files: []FileGrant{{Path: "/app", FS: mfs}, {Path: "/app", FS: mfs}}}}, codes.MountTwice},
		{"scheme", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https://", Host: "a.example", Ports: []int{443}}}}}, codes.NetworkScheme},
		{"host", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "bad host", Ports: []int{443}}}}}, codes.NetworkHost},
		{"no ports", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "a.example"}}}}, codes.NetworkPorts},
		{"port range", Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: "a.example", Ports: []int{0}}}}}, codes.NetworkPorts},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewEngine(c.cfg)
			var ce *ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("got %v, want *ConfigError", err)
			}
			if ce.Code != c.code {
				t.Fatalf("code %q, want %q (%v)", ce.Code, c.code, err)
			}
			d, ok := Diagnose(err)
			if !ok || d.Code != c.code || d.Primary.File != ce.Field || d.Message == "" || len(d.Notes) == 0 || len(d.Help) == 0 {
				t.Fatalf("Diagnose gave %+v, %v", d, ok)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Fatalf("Error() is not one line: %q", err.Error())
			}
		})
	}
}

func TestRunOptionErrorCodes(t *testing.T) {
	e, err := NewEngine(Config{})
	must(t, err)
	other, err := NewEngine(Config{})
	must(t, err)
	p, err := e.Compile(context.Background(), "a.php", "<?php")
	must(t, err)
	q, err := other.Compile(context.Background(), "a.php", "<?php")
	must(t, err)
	var zero Engine
	for _, c := range []struct {
		name string
		err  error
		code diag.Code
	}{
		{"output", func() error { _, err := e.Run(context.Background(), p, WithOutput(nil)); return err }(), codes.RunOutputNil},
		{"global", func() error { _, err := e.Run(context.Background(), p, WithGlobal("1x", Null())); return err }(), codes.RunGlobalName},
		{"nil option", func() error { _, err := e.Run(context.Background(), p, nil); return err }(), codes.RunOptionNil},
		{"foreign", func() error { _, err := e.Run(context.Background(), q); return err }(), codes.ProgramForeign},
		{"zero engine", func() error { _, err := zero.Compile(context.Background(), "a.php", ""); return err }(), codes.EngineZero},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, ok := Diagnose(c.err)
			if !ok || d.Code != c.code {
				t.Fatalf("Diagnose(%v) = %q, %v; want %q", c.err, d.Code, ok, c.code)
			}
		})
	}
}

func TestDiagnoseOtherErrors(t *testing.T) {
	if _, ok := Diagnose(nil); ok {
		t.Fatal("nil error diagnosed")
	}
	if _, ok := Diagnose(errors.New("plain")); ok {
		t.Fatal("plain error diagnosed")
	}
	d, ok := Diagnose(&ConfigError{Field: "X", Reason: "is bad"})
	if !ok || d.Code != codes.ConfigOther {
		t.Fatalf("host-built ConfigError gave %q, %v", d.Code, ok)
	}
}

func TestConfigDiagnosticGolden(t *testing.T) {
	_, err := NewEngine(Config{Policy: Policy{Files: []FileGrant{{Path: "app", FS: fstest.MapFS{}}}}})
	d, ok := Diagnose(err)
	if !ok {
		t.Fatal(err)
	}
	var b bytes.Buffer
	must(t, diag.Render(&b, d, diag.Options{}))
	want := `error[PHG1019]: invalid Policy.Files[0].Path: "app" is not a clean absolute path
 --> Policy.Files[0].Path
  = note: mounts are matched against absolute PHP paths
  = help: write the path starting with "/" and without "." or ".." segments
`
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}

// Every ConfigError the engine builds must carry a specific code; PHG1000 is
// only for errors built by host code.
func TestEngineConfigErrorsHaveCodes(t *testing.T) {
	files, err := filepath.Glob("*.go")
	must(t, err)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		must(t, err)
		f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		must(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if id, ok := cl.Type.(*ast.Ident); !ok || id.Name != "ConfigError" {
				return true
			}
			for _, el := range cl.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Code" {
						return true
					}
				}
			}
			t.Errorf("%s: ConfigError literal without Code: %s", path, src[cl.Pos()-1:cl.End()-1])
			return true
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewReservedHostNameNoteAndHostCode(t *testing.T) {
	ok := func(_ HostContext, _ []Value) (Value, error) { return Null(), nil }
	_, err := NewEngine(Config{HostFunctions: []HostFunction{{Name: "echo", Func: ok}}})
	d, _ := Diagnose(err)
	if !strings.Contains(strings.Join(append(d.Notes, d.Help...), " "), "reserved") {
		t.Fatalf("reserved word not explained: %+v", d)
	}
	d, _ = Diagnose(&ConfigError{Field: "X", Reason: "bad", Code: "HOST1"})
	if d.Code != "HOST1" {
		t.Fatalf("host code replaced by %s", d.Code)
	}
}
