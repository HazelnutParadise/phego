package phego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/diag/codes"
)

// ranToCompletion reports a run that executed the whole script and returned.
func ranToCompletion(r *Result, err error) bool {
	return err == nil && r != nil && r.Status == StatusCompleted
}

func mustEngine(t *testing.T, cfg Config) *Engine {
	t.Helper()
	e, err := NewEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close(context.Background()) })
	return e
}

func mustCompile(t *testing.T, e *Engine, name, src string) *Program {
	t.Helper()
	p, err := e.Compile(context.Background(), name, src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// One Program serves many goroutines; each run sees only its own globals.
func TestRunSharesProgramAcrossGoroutines(t *testing.T) {
	e := mustEngine(t, Config{})
	p := mustCompile(t, e, "share.php", "<?php\n$s = 0;\nfor ($i = 0; $i < 100; $i++) { $s = $s + $x; }\necho $x;\nreturn [$x, $s];\n")
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.Run(context.Background(), p, WithGlobal("x", Int64(int64(g))))
			want := Array([]Entry{{IntKey(0), Int64(int64(g))}, {IntKey(1), Int64(int64(100 * g))}})
			if !ranToCompletion(r, err) || !reflect.DeepEqual(r.Return, want) || string(r.Output) != fmt.Sprint(g) {
				t.Errorf("goroutine %d: %+v %v", g, r, err)
			}
		}()
	}
	wg.Wait()
}

// Cancelling an endless loop ends the run promptly and frees its slot.
func TestRunCancellationReleasesAdmission(t *testing.T) {
	e := mustEngine(t, Config{MaxActive: 1})
	loop := mustCompile(t, e, "loop.php", "<?php\necho 'started';\nwhile (true) {}\n")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	r, err := e.Run(ctx, loop)
	if !errors.Is(err, context.Canceled) || r.Status != StatusCancelled || string(r.Output) != "started" {
		t.Fatalf("cancelled run: %+v %v", r, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("cancellation took %v", d)
	}
	if n := e.policy.admission.Active(); n != 0 {
		t.Fatalf("%d slots still held after cancellation", n)
	}
	if r, err := e.Run(context.Background(), mustCompile(t, e, "ok.php", "<?php return 1;")); !ranToCompletion(r, err) {
		t.Fatalf("next run: %+v %v", r, err)
	}
}

// Every budget ends the run with a ResourceError naming it, keeps what was
// produced before, and never claims success.
func TestRunLimitsNameTheResource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limits   Limits
		src      string
		opts     []RunOption
		resource Resource
		limit    int64
		check    func(*Result) error
	}{
		{"steps", Limits{Steps: 1000}, "<?php\nwhile (true) {}\n", nil, ResourceSteps, 1000, nil},
		{"frames", Limits{Frames: 5}, "<?php\nfunction f($n) { return f($n + 1); }\nf(0);\n", nil, ResourceFrames, 5, nil},
		{"output", Limits{Output: 10}, "<?php\necho 'abcd';\necho 'efghijklmn';\n", nil, ResourceOutput, 10, func(r *Result) error {
			if string(r.Output) != "abcd" {
				return fmt.Errorf("output %q, want the accepted prefix", r.Output)
			}
			return nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := mustEngine(t, Config{Limits: tc.limits})
			r, err := e.Run(context.Background(), mustCompile(t, e, "limit.php", tc.src), tc.opts...)
			var re *ResourceError
			if !errors.As(err, &re) || re.Resource != tc.resource || re.Limit != tc.limit || r.Status != StatusPolicyDenied {
				t.Fatalf("%+v %v", r, err)
			}
			if tc.check != nil {
				if err := tc.check(r); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	// Function calls are not a budget of their own: a loop of 20000 calls
	// runs under the default limits.
	e := mustEngine(t, Config{})
	r, err := e.Run(context.Background(), mustCompile(t, e, "calls.php", "<?php\nfunction f($n) { return $n + 1; }\n$s = 0;\nfor ($i = 0; $i < 20000; $i++) { $s = f($s); }\nreturn $s;\n"))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 20000 {
		t.Fatalf("calls: %+v %v", r, err)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunReportsWriterErrors(t *testing.T) {
	e := mustEngine(t, Config{})
	boom := errors.New("client went away")
	r, err := e.Run(context.Background(), mustCompile(t, e, "w.php", "<?php echo 'x'; return 1;"), WithOutput(failingWriter{boom}))
	if !errors.Is(err, boom) || r.Status != StatusFailed {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestRunGlobalsRoundTrip(t *testing.T) {
	nested := Array([]Entry{
		{IntKey(0), Int64(-7)},
		{StringKey("k"), Array([]Entry{{IntKey(5), Bool(true)}, {StringKey("deep"), Array([]Entry{{IntKey(0), Null()}})}})},
		{IntKey(-3), Bytes([]byte("\xff\x00raw"))},
		{StringKey(""), Float64(0.5)},
	})
	values := map[string]Value{
		"null": Null(), "false": Bool(false), "true": Bool(true), "int": Int64(math.MaxInt64), "negative": Int64(math.MinInt64),
		"float": Float64(-1.25), "inf": Float64(math.Inf(1)), "bytes": Bytes([]byte("a\x00b\xff")), "empty": Bytes(nil),
		"emptyArray": Array(nil), "nested": nested,
	}
	e := mustEngine(t, Config{})
	p := mustCompile(t, e, "g.php", "<?php return $v;")
	for name, v := range values {
		r, err := e.Run(context.Background(), p, WithGlobal("v", v))
		if !ranToCompletion(r, err) || !reflect.DeepEqual(r.Return, v) {
			t.Errorf("%s: %+v %v", name, r, err)
		}
	}
	// PHP normalizes the numeric string key "1" to the integer 1.
	r, err := e.Run(context.Background(), p, WithGlobal("v", Array([]Entry{{StringKey("1"), Int64(1)}})))
	if want := Array([]Entry{{IntKey(1), Int64(1)}}); !ranToCompletion(r, err) || !reflect.DeepEqual(r.Return, want) {
		t.Errorf("numeric string key: %+v %v", r, err)
	}
	// An unused global is accepted and has no effect.
	if r, err := e.Run(context.Background(), p, WithGlobal("unused", Int64(1))); !ranToCompletion(r, err) || !r.Return.IsNull() {
		t.Errorf("unused global: %+v %v", r, err)
	}
	// Values the profile cannot hold are the host's ConfigError.
	deep := Null()
	for i := 0; i < maxGlobalDepth+1; i++ {
		deep = Array([]Entry{{IntKey(0), deep}})
	}
	e32 := mustEngine(t, Config{IntSize: 32})
	p32 := mustCompile(t, e32, "g.php", "<?php return $v;")
	for name, run := range map[string]func() error{
		"32-bit range": func() error {
			_, err := e32.Run(context.Background(), p32, WithGlobal("v", Int64(math.MaxInt32+1)))
			return err
		},
		"32-bit key": func() error {
			_, err := e32.Run(context.Background(), p32, WithGlobal("v", Array([]Entry{{IntKey(math.MinInt32 - 1), Null()}})))
			return err
		},
		"depth": func() error { _, err := e.Run(context.Background(), p, WithGlobal("v", deep)); return err },
	} {
		var ce *ConfigError
		if err := run(); !errors.As(err, &ce) || ce.Code != codes.RunGlobalValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	if r, err := e32.Run(context.Background(), p32, WithGlobal("v", Int64(math.MinInt32))); !ranToCompletion(r, err) || !reflect.DeepEqual(r.Return, Int64(math.MinInt32)) {
		t.Errorf("32-bit minimum: %+v %v", r, err)
	}
}

func TestRunReturnValuesAndEnds(t *testing.T) {
	e := mustEngine(t, Config{})
	for _, tc := range []struct {
		src    string
		status Status
		exit   int
		ret    Value
		output string
	}{
		{"<?php return 1.5;", StatusCompleted, 0, Float64(1.5), ""},
		{`<?php return "s\x00";`, StatusCompleted, 0, Bytes([]byte("s\x00")), ""},
		{"<?php return true;", StatusCompleted, 0, Bool(true), ""},
		{"<?php return;", StatusCompleted, 0, Null(), ""},
		{"<?php echo 'x';", StatusCompleted, 0, Null(), "x"},
		{"<?php return [2 => 'a', 'b'];", StatusCompleted, 0, Array([]Entry{{IntKey(2), Bytes([]byte("a"))}, {IntKey(3), Bytes([]byte("b"))}}), ""},
		{"<?php echo 'a'; exit(3);", StatusExited, 3, Null(), "a"},
		{"<?php exit('bye');", StatusExited, 0, Null(), "bye"},
	} {
		r, err := e.Run(context.Background(), mustCompile(t, e, "r.php", tc.src))
		if err != nil || r.Status != tc.status || r.ExitCode != tc.exit || !reflect.DeepEqual(r.Return, tc.ret) || string(r.Output) != tc.output {
			t.Errorf("%s: %+v %v", tc.src, r, err)
		}
	}
}

// An uncaught throwable ends the run as PHP does: output so far is kept, the
// fatal error is displayed, and the exit status is 255.
func TestRunUncaughtThrowable(t *testing.T) {
	e := mustEngine(t, Config{})
	r, err := e.Run(context.Background(), mustCompile(t, e, "u.php", "<?php\necho 'before';\necho 1 % 0;\n"), WithINI("log_errors", "1"))
	var pe *PHPError
	if !errors.As(err, &pe) || pe.Kind != UncaughtThrowable || pe.Message != "DivisionByZeroError: Modulo by zero" || pe.File != "u.php" || pe.Line != 3 {
		t.Fatalf("%#v", err)
	}
	want := "before\nFatal error: Uncaught DivisionByZeroError: Modulo by zero in u.php:3\nStack trace:\n#0 {main}\n  thrown in u.php on line 3\n"
	if r.Status != StatusFailed || r.ExitCode != 255 || string(r.Output) != want {
		t.Fatalf("%+v\n%q", r, r.Output)
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Level != "Fatal error" || r.Diagnostics[0].Line != 3 {
		t.Fatalf("log %+v", r.Diagnostics)
	}
}

// Without WithINI a run uses php -n's CLI settings; compile-time warnings
// are reported before the script runs and follow the same settings.
func TestRunINIDefaultsAndCompileWarnings(t *testing.T) {
	e := mustEngine(t, Config{})
	r, err := e.Run(context.Background(), mustCompile(t, e, "d.php", "<?php echo $u;"))
	if !ranToCompletion(r, err) || string(r.Output) != "\nWarning: Undefined variable $u in d.php on line 1\n" || len(r.Diagnostics) != 0 {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	src := "<?php\necho 'run ';\nswitch (1) { case 1: continue; }\n"
	p := mustCompile(t, e, "c.php", src)
	warning := `"continue" targeting switch is equivalent to "break"`
	r, err = e.Run(context.Background(), p, WithINI("display_errors", "off"), WithINI("log_errors", "On"))
	if !ranToCompletion(r, err) || string(r.Output) != "run " || len(r.Diagnostics) != 1 ||
		!strings.HasPrefix(r.Diagnostics[0].Message, warning) || r.Diagnostics[0].File != "c.php" || r.Diagnostics[0].Line != 3 {
		t.Fatalf("logged compile warning: %+v %v", r, err)
	}
	r, err = e.Run(context.Background(), p)
	if !ranToCompletion(r, err) || !strings.HasPrefix(string(r.Output), "\nWarning: "+warning) || !strings.HasSuffix(string(r.Output), " in c.php on line 3\nrun ") {
		t.Fatalf("displayed compile warning: %q %v", r.Output, err)
	}
	r, err = e.Run(context.Background(), p, WithINI("error_reporting", "0"), WithINI("log_errors", "1"))
	if !ranToCompletion(r, err) || string(r.Output) != "run " || len(r.Diagnostics) != 0 {
		t.Fatalf("error_reporting=0: %+v %v", r, err)
	}
	// The last value of a repeated name wins.
	r, err = e.Run(context.Background(), p, WithINI("display_errors", "0"), WithINI("display_errors", "1"))
	if !ranToCompletion(r, err) || !strings.HasPrefix(string(r.Output), "\nWarning: ") {
		t.Fatalf("repeated name: %q %v", r.Output, err)
	}
}

func TestWithINIValidation(t *testing.T) {
	e := mustEngine(t, Config{})
	p := mustCompile(t, e, "i.php", "<?php return 1;")
	for _, tc := range []struct {
		name, value string
		code        diag.Code // empty: accepted
		unsupported bool
	}{
		{"display_errors", "0", "", false}, {"display_errors", "YES", "", false}, {"display_errors", "False", "", false},
		{"log_errors", "on", "", false}, {"log_errors", "no", "", false},
		{"html_errors", "0", "", false}, {"html_errors", "Off", "", false},
		{"error_reporting", "-1", "", false}, {"error_reporting", "32767", "", false}, {"error_reporting", "+2", "", false},
		{"error_reporting", "2147483647", "", false}, {"error_reporting", "-2147483648", "", false},
		{"display_errors", "stderr", codes.RunINIValue, false}, {"display_errors", "2", codes.RunINIValue, false},
		{"log_errors", "", codes.RunINIValue, false}, {"log_errors", " 1", codes.RunINIValue, false},
		{"html_errors", "maybe", codes.RunINIValue, false},
		{"error_reporting", "E_ALL", codes.RunINIValue, false}, {"error_reporting", "8abc", codes.RunINIValue, false},
		{"error_reporting", "", codes.RunINIValue, false}, {"error_reporting", "2147483648", codes.RunINIValue, false},
		{"error_reporting", "+-1", codes.RunINIValue, false}, {"error_reporting", "0x10", codes.RunINIValue, false},
		{"memory_limit", "1", codes.RunININame, false}, {"Display_errors", "1", codes.RunININame, false},
		{"html_errors", "1", "", true}, {"html_errors", "on", "", true},
	} {
		r, err := e.Run(context.Background(), p, WithINI(tc.name, tc.value))
		var ce *ConfigError
		var ue *UnsupportedError
		switch {
		case tc.unsupported:
			if !errors.As(err, &ue) || r.Status != StatusFailed {
				t.Errorf("%s=%q: %v", tc.name, tc.value, err)
			}
		case tc.code == "":
			if !ranToCompletion(r, err) {
				t.Errorf("%s=%q rejected: %v", tc.name, tc.value, err)
			}
		case !errors.As(err, &ce) || ce.Code != tc.code || r.Status != StatusFailed:
			t.Errorf("%s=%q: %v, want %s", tc.name, tc.value, err, tc.code)
		}
	}
}

func TestRunRefusesClosedAndForeignEngines(t *testing.T) {
	a, b := mustEngine(t, Config{}), mustEngine(t, Config{})
	p := mustCompile(t, a, "a.php", "<?php return 1;")
	var ce *ConfigError
	if r, err := b.Run(context.Background(), p); !errors.As(err, &ce) || ce.Code != codes.ProgramForeign || r.Status != StatusFailed {
		t.Fatalf("foreign program: %+v %v", r, err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r, err := a.Run(context.Background(), p); !errors.Is(err, ErrEngineClosed) || r.Status != StatusFailed {
		t.Fatalf("run after Close: %+v %v", r, err)
	}
}

// A construct the first VM stage cannot run is refused at Compile with its
// location and a rustc-style diagnostic.
func TestUnsupportedConstructIsLocated(t *testing.T) {
	e := mustEngine(t, Config{})
	// Stage 2 runs "$a[0] += 1", stage 3 "$a[0] = &$b" and stage 4
	// "[$x] = $a", the constructs this test used before; objects are still
	// outside the VM.
	src := "<?php\n$a = [1];\n$o->p = 1;\n"
	_, err := e.Compile(context.Background(), "app/x.php", src)
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.Feature != "property access" || ue.File != "app/x.php" || ue.Line != 3 || ue.Option != "" {
		t.Fatalf("%#v", err)
	}
	if got := err.Error(); got != "phego: unsupported: property access in app/x.php on line 3" {
		t.Fatalf("Error() = %q", got)
	}
	d, ok := Diagnose(err)
	if !ok || d.Code != codes.UnsupportedConstruct {
		t.Fatalf("diagnostic %+v", d)
	}
	var b bytes.Buffer
	if err := diag.Render(&b, d, diag.Options{Source: func(string) ([]byte, bool) { return []byte(src), true }}); err != nil {
		t.Fatal(err)
	}
	const golden = `error[PHG3001]: unsupported PHP construct: property access
 --> app/x.php:3:1
  |
3 | $o->p = 1;
  | ^^^^^^^^^^ this build's VM cannot run this yet
  |
  = note: this build's first VM stage does not run that construct yet, so the program is refused instead of running partly
  = help: run the program on a Phego build whose VM implements the construct; engineering/decisions/W-17-vm.md lists the stages
`
	if b.String() != golden {
		t.Fatalf("rendered:\n%s\nwant:\n%s", b.String(), golden)
	}
	// A request without a source location keeps its one-line form.
	if d, _ := Diagnose(&UnsupportedError{Feature: "PHP release 8.4.1"}); d.Code != codes.Unsupported || d.Primary.File != "PHP release 8.4.1" {
		t.Fatalf("unlocated diagnostic %+v", d)
	}
}

// RunFile reads, compiles and runs a file from a mounted host directory.
func TestRunFileExecutesMountedScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php\necho \"hello $name\";\nreturn 7;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := mustEngine(t, Config{MaxActive: 1, Policy: Policy{Files: []FileGrant{{Path: "/app", Root: dir}}}})
	r, err := e.RunFile(context.Background(), "/app/index.php", WithGlobal("name", Bytes([]byte("php"))))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 7 || string(r.Output) != "hello php" {
		t.Fatalf("%+v %v", r, err)
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("RunFile kept its slot")
	}
}

// Declaring a function under a builtin's name is PHP's compile-time fatal,
// not an unsupported construct.
func TestCompileRejectsRedeclaringABuiltin(t *testing.T) {
	e, err := NewEngine(Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Compile(context.Background(), "/w/r.php", "<?php\nfunction strlen() {}\n")
	var pe *PHPError
	if !errors.As(err, &pe) || pe.Kind != CompileError || pe.Message != "Cannot redeclare function strlen()" || pe.File != "/w/r.php" || pe.Line != 2 {
		t.Fatalf("Compile error = %#v, want PHP's redeclare compile error on line 2", err)
	}
}
