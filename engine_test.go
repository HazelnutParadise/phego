package phego

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

func TestRunReportsParseErrorsWithLocation(t *testing.T) {
	_, err := Run(context.Background(), "<?php\n\necho 1 < 2 < 3;")
	var pe *PHPError
	if !errors.As(err, &pe) || pe.Kind != ParseError || pe.Line != 3 || pe.File != "Standard input code" {
		t.Fatalf("error = %#v", err)
	}
	_, err = Run(context.Background(), "<?php\nclass A { public public $a; }")
	if !errors.As(err, &pe) || pe.Kind != CompileError || pe.Line != 2 {
		t.Fatalf("compile error = %#v", err)
	}
}

func TestRunNeverClaimsSuccessWithoutExecution(t *testing.T) {
	result, err := Run(context.Background(), `<?php return 10 + 20;`)
	var ue *UnsupportedError
	if !errors.As(err, &ue) || result == nil || result.Status != StatusFailed {
		t.Fatalf("result %+v error %v", result, err)
	}
}

func TestConfigIsValidatedAndSnapshotted(t *testing.T) {
	for _, cfg := range []Config{
		{Release: "8.4.1"}, {Release: "8.5.99"}, {IntSize: 16}, {MaxSourceBytes: -1}, {OutputLimit: -1},
		{HostFunctions: []HostFunction{{Name: "1bad", Func: noop}}},
		{HostFunctions: []HostFunction{{Name: "f"}}},
		{HostFunctions: []HostFunction{{Name: "f", MinArgs: 2, MaxArgs: 1, Func: noop}}},
		{HostFunctions: []HostFunction{{Name: "f", Func: noop}, {Name: "F", Func: noop}}},
	} {
		if _, err := NewEngine(cfg); err == nil {
			t.Errorf("accepted %+v", cfg)
		}
	}
	caps := []string{"net"}
	fns := []HostFunction{{Name: "f", MaxArgs: -1, Capabilities: caps, Func: noop}}
	e, err := NewEngine(Config{HostFunctions: fns})
	if err != nil {
		t.Fatal(err)
	}
	fns[0].Name, caps[0] = "changed", "fs"
	if f, ok := e.host["f"]; !ok || f.Capabilities[0] != "net" {
		t.Fatalf("host functions were not snapshotted: %+v", e.host)
	}
}

func noop(HostContext, []Value) (Value, error) { return Null(), nil }

func TestSourceLimitAppliesBeforeParsing(t *testing.T) {
	e, err := NewEngine(Config{MaxSourceBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Compile(context.Background(), "a.php", "<?php echo 1;")
	var le *LimitError
	if !errors.As(err, &le) || le.Max != 8 || le.Got != 13 {
		t.Fatalf("error = %v", err)
	}
}

func TestProgramsAreBoundToTheirEngineAndShareable(t *testing.T) {
	a, _ := NewEngine(Config{})
	b, _ := NewEngine(Config{})
	p, err := a.Compile(context.Background(), "a.php", "<?php echo 1;")
	if err != nil {
		t.Fatal(err)
	}
	var ce *ConfigError
	if _, err := b.Run(context.Background(), p); !errors.As(err, &ce) {
		t.Fatalf("foreign program: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ue *UnsupportedError
			if _, err := a.Run(context.Background(), p); !errors.As(err, &ue) {
				t.Errorf("concurrent run: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestCancelledContextIsReported(t *testing.T) {
	e, _ := NewEngine(Config{})
	p, _ := e.Compile(context.Background(), "a.php", "<?php echo 1;")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := e.Run(ctx, p)
	if !errors.Is(err, context.Canceled) || r.Status != StatusCancelled {
		t.Fatalf("result %+v err %v", r, err)
	}
	if _, err := e.Compile(ctx, "b.php", "<?php"); !errors.Is(err, context.Canceled) {
		t.Fatalf("compile err %v", err)
	}
}

func TestRunOptionsAreValidated(t *testing.T) {
	e, _ := NewEngine(Config{})
	p, _ := e.Compile(context.Background(), "a.php", "<?php echo 1;")
	for _, opt := range []RunOption{nil, WithOutput(nil), WithGlobal("1x", Null()), WithGlobal("this", Null())} {
		var ce *ConfigError
		if _, err := e.Run(context.Background(), p, opt); !errors.As(err, &ce) {
			t.Errorf("option accepted: %v", err)
		}
	}
	if _, err := e.Run(context.Background(), p, WithOutput(&bytes.Buffer{}), WithGlobal("x", Int64(1))); err == nil {
		t.Fatal("run without execution reported success")
	}
}

func TestRunFileUsesGrantedFilesOnly(t *testing.T) {
	e, _ := NewEngine(Config{})
	if r, err := e.RunFile(context.Background(), "a.php"); err == nil || r.Status != StatusPolicyDenied {
		t.Fatalf("file access without grant: %+v %v", r, err)
	}
	files := fstest.MapFS{"bad.php": {Data: []byte("<?php 1 <")}, "big.php": {Data: []byte(strings.Repeat("x", 64))}}
	e, _ = NewEngine(Config{Files: files, MaxSourceBytes: 32})
	var pe *PHPError
	if _, err := e.RunFile(context.Background(), "bad.php"); !errors.As(err, &pe) || pe.File != "bad.php" {
		t.Fatalf("parse error through RunFile: %v", err)
	}
	var le *LimitError
	if _, err := e.RunFile(context.Background(), "big.php"); !errors.As(err, &le) {
		t.Fatalf("size limit through RunFile: %v", err)
	}
	if _, err := e.RunFile(context.Background(), "missing.php"); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestCloseStopsNewWorkAndHonoursDeadline(t *testing.T) {
	e, _ := NewEngine(Config{})
	if err := e.begin(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := e.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close with running work: %v", err)
	}
	if _, err := e.Compile(context.Background(), "a.php", "<?php"); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("compile after close: %v", err)
	}
	e.runs.Done()
	if err := e.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestZeroEngineIsRejectedWithoutPanicking(t *testing.T) {
	var e Engine
	var ce *ConfigError
	if err := e.Close(context.Background()); !errors.As(err, &ce) {
		t.Fatalf("Close on zero Engine: %v", err)
	}
	if _, err := e.Compile(context.Background(), "a.php", "<?php"); !errors.As(err, &ce) {
		t.Fatalf("Compile on zero Engine: %v", err)
	}
	if _, err := e.RunFile(context.Background(), "a.php"); !errors.As(err, &ce) {
		t.Fatalf("RunFile on zero Engine: %v", err)
	}
}

func TestHostFunctionNamesFollowPHPBytes(t *testing.T) {
	for name, ok := range map[string]bool{
		"\xe5\x87\xbd\xe5\xbc\x8f": true, "\x80abc": true, "f\xff": true, "readonly": true, "f1": true,
		"echo": false, "isset": false, "1f": false, "": false, "a-b": false, "a\\b": false,
	} {
		_, err := NewEngine(Config{HostFunctions: []HostFunction{{Name: name, Func: noop}}})
		if (err == nil) != ok {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, err := NewEngine(Config{HostFunctions: []HostFunction{{Name: "\xc3\x89", Func: noop}, {Name: "\xc3\xa9", Func: noop}}}); err != nil {
		t.Fatalf("non-ASCII names differing in case are distinct in PHP: %v", err)
	}
	for _, caps := range [][]string{{""}, {"net", "net"}} {
		if _, err := NewEngine(Config{HostFunctions: []HostFunction{{Name: "f", Func: noop, Capabilities: caps}}}); err == nil {
			t.Errorf("capabilities %q accepted", caps)
		}
	}
	e, _ := NewEngine(Config{})
	p, _ := e.Compile(context.Background(), "a.php", "<?php")
	if _, err := e.Run(context.Background(), p, WithGlobal("\x80v", Int64(1))); errors.As(err, new(*ConfigError)) {
		t.Fatalf("byte-label global rejected: %v", err)
	}
}

func TestRunFileChecksStateBeforeTouchingFiles(t *testing.T) {
	opened := 0
	files := countingFS{MapFS: fstest.MapFS{"a.php": {Data: []byte("<?php")}}, opened: &opened}
	e, _ := NewEngine(Config{Files: files})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := e.RunFile(ctx, "a.php"); !errors.Is(err, context.Canceled) || r.Status != StatusCancelled {
		t.Fatalf("cancelled RunFile: %+v %v", r, err)
	}
	if err := e.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RunFile(context.Background(), "a.php"); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("RunFile after Close: %v", err)
	}
	if opened != 0 {
		t.Fatalf("files were opened %d times", opened)
	}
}

type countingFS struct {
	fstest.MapFS
	opened *int
}

func (f countingFS) Open(name string) (fs.File, error) {
	*f.opened++
	return f.MapFS.Open(name)
}

func TestReleaseErrorsKeepTheirClass(t *testing.T) {
	var ue *UnsupportedError
	if _, err := NewEngine(Config{Release: "8.4.1"}); !errors.As(err, &ue) {
		t.Errorf("known but unimplemented release: %v", err)
	}
	if _, err := NewEngine(Config{Release: "1.0"}); !errors.As(err, &ue) {
		t.Errorf("first public release: %v", err)
	}
	var ce *ConfigError
	if _, err := NewEngine(Config{Release: "8.5.99"}); !errors.As(err, &ce) || ce.Field != "Release" {
		t.Errorf("unknown release: %v", err)
	}
}

// TestProfilesGiveTheirOwnResults compiles the same source under the 32-bit
// and 64-bit profiles. PHP 8.5.12 lexes 2147483648 as an integer only when it
// fits the platform integer.
func TestProfilesGiveTheirOwnResults(t *testing.T) {
	ctx := context.Background()
	kinds := map[int]string{}
	for _, width := range []int{32, 64} {
		e, err := NewEngine(Config{IntSize: width})
		if err != nil {
			t.Fatal(err)
		}
		p, err := e.Compile(ctx, "w.php", `<?php echo 2147483648;`)
		if err != nil {
			t.Fatal(err)
		}
		if p.profile != e.profile || p.profile.IntWidth != width {
			t.Fatalf("program profile %+v, engine %+v", p.profile, e.profile)
		}
		kinds[width] = literalKind(p.tree)
	}
	if kinds[32] != "float" || kinds[64] != "int" {
		t.Fatalf("literal kinds %v", kinds)
	}
}

// literalKind finds the first numeric literal of a tree.
func literalKind(n *frontend.Node) string {
	if n == nil {
		return ""
	}
	switch n.Token {
	case frontend.TLNumber:
		return "int"
	case frontend.TDNumber:
		return "float"
	}
	for _, c := range n.Children {
		if k := literalKind(c); k != "" {
			return k
		}
	}
	return ""
}

// TestProgramNeverRunsUnderAnotherProfile models a cache that hands a program
// compiled for one profile to an engine of another profile.
func TestProgramNeverRunsUnderAnotherProfile(t *testing.T) {
	ctx := context.Background()
	e32, err := NewEngine(Config{IntSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	e64, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e64.Compile(ctx, "w.php", `<?php echo 1;`)
	if err != nil {
		t.Fatal(err)
	}
	stale := *p
	stale.engine = e32
	var ce *ConfigError
	if res, err := e32.Run(ctx, &stale); !errors.As(err, &ce) || res.Status != StatusFailed || !strings.Contains(ce.Reason, "64-bit") {
		t.Fatalf("result %+v error %v", res, err)
	}
	short, err := NewEngine(Config{ShortOpenTag: true})
	if err != nil {
		t.Fatal(err)
	}
	stale.engine = short
	if res, err := short.Run(ctx, &stale); !errors.As(err, &ce) || res.Status != StatusFailed || !strings.Contains(ce.Reason, "short_open_tag off, not") {
		t.Fatalf("short_open_tag mismatch: result %+v error %v", res, err)
	}
}
