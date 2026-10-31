package phego

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
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
		{Release: "8.4.0"}, {IntSize: 16}, {MaxSourceBytes: -1}, {OutputLimit: -1},
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
			if _, err := a.Run(context.Background(), p); errors.As(err, &ce) {
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
