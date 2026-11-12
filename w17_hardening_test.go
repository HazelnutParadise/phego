package phego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/HazelnutParadise/phego/diag"
	"github.com/HazelnutParadise/phego/diag/codes"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/runtime"
)

// PHP 8.5.12's compile-time fatal error once its C stack runs low; Phego
// raises it at ir.MaxExpressionNesting.
const w17StackLimit = "Maximum call stack size of 8339456 bytes (zend.max_allowed_stack_size - zend.reserved_stack_size) reached during compilation. Try splitting expression"

// Long operator and member chains compile without recursing once per link,
// and a chain beyond the nesting bound is PHP's compile-time fatal error,
// never a host crash. The stack is lowered so a recursive walk fails fast.
func TestCompileLongChainsDoNotOverflowTheStack(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(256 << 20))
	e := mustEngine(t, Config{})
	beyond := ir.MaxExpressionNesting + 10
	for _, tc := range []struct{ name, src string }{
		{"500k +", "<?php\nreturn 1" + strings.Repeat("+1", 500_000) + ";\n"},
		{"concat", "<?php\n$a = 'x';\nreturn $a" + strings.Repeat(".$a", beyond) + ";\n"},
		{"and", "<?php\n$a = 1;\nreturn $a" + strings.Repeat("&&$a", beyond) + ";\n"},
		{"dim", "<?php\n$a = [];\nreturn $a" + strings.Repeat("[0]", beyond) + ";\n"},
		{"prop", "<?php\n$a = 1;\nreturn $a" + strings.Repeat("->b", beyond) + ";\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Compile(context.Background(), "chain.php", tc.src)
			var pe *PHPError
			if !errors.As(err, &pe) || pe.Kind != CompileError || pe.Message != w17StackLimit || pe.File != "chain.php" {
				t.Fatalf("got %v", err)
			}
		})
	}
	// Below the bound a chain compiles and runs.
	p := mustCompile(t, e, "sum.php", "<?php\nreturn 1"+strings.Repeat("+1", 99_000)+";\n")
	r, err := e.Run(context.Background(), p)
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 99_001 {
		t.Fatalf("%+v %v", r, err)
	}
	p = mustCompile(t, e, "cat.php", "<?php\n$a = 'x';\nreturn $a"+strings.Repeat(".$a", 20_000)+";\n")
	r, err = e.Run(context.Background(), p)
	if s, _ := r.Return.Bytes(); !ranToCompletion(r, err) || len(s) != 20_001 {
		t.Fatalf("%v %v", r.Status, err)
	}
	// Right-nested forms are bounded by the parser at PHP's own limit.
	for _, src := range []string{
		"<?php\n$a = 1;\nreturn " + strings.Repeat("- ", 9_990) + "$a;\n",
		"<?php\n$a = 1;\nreturn " + strings.Repeat("!", 9_990) + "$a;\n",
		"<?php\nreturn " + strings.Repeat("(int)", 9_990) + "1;\n",
		"<?php\nreturn " + strings.Repeat("$a = ", 4_900) + "1;\n",
		"<?php\n$a = 1;\nreturn " + strings.Repeat("$a ?? ", 3_300) + "1;\n",
		"<?php\nfunction f($x) { return $x; }\nreturn " + strings.Repeat("f(", 3_300) + "1" + strings.Repeat(")", 3_300) + ";\n",
		"<?php\n$a = 1;\nreturn " + strings.Repeat("$a ? 1 : (", 9_990) + "2" + strings.Repeat(")", 9_990) + ";\n",
	} {
		if p, err := e.Compile(context.Background(), "right.php", src); err == nil {
			if _, err := e.Run(context.Background(), p); err != nil {
				t.Errorf("%.40q: %v", src, err)
			}
		} else if _, ok := errors.AsType[*PHPError](err); !ok {
			if _, ok := errors.AsType[*UnsupportedError](err); !ok {
				t.Errorf("%.40q: untyped %v", src, err)
			}
		}
	}
}

// w17DeepArrays builds $a and $b, two equal arrays nested depth levels deep.
func w17DeepArrays(depth int, rest string) string {
	return fmt.Sprintf("<?php\n$a = []; $b = [];\nfor ($i = 0; $i < %d; $i++) { $a = [$a]; $b = [$b]; }\n%s", depth, rest)
}

// Freeing, comparing and returning arrays nested deeper than the Go stack
// allows recursion for do not crash the host. Comparison throws PHP's Error
// past runtime.MaxComparisonNesting, as PHP 8.5.12 does past its C stack.
func TestDeepArraysDoNotOverflowTheStack(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(8 << 20))
	e := mustEngine(t, Config{})
	run := func(t *testing.T, src string) (*Result, error) {
		t.Helper()
		return e.Run(context.Background(), mustCompile(t, e, "deep.php", src))
	}
	deep := runtime.MaxComparisonNesting + 1_000
	for _, op := range []string{"==", "===", "<=>"} {
		t.Run(op, func(t *testing.T) {
			r, err := run(t, w17DeepArrays(deep, "return $a "+op+" $b;\n"))
			var pe *PHPError
			if !errors.As(err, &pe) || pe.Kind != UncaughtThrowable || pe.Message != "Error: Maximum call stack size reached during comparison" || pe.Line != 4 || r.ExitCode != 255 {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	for _, tc := range []struct {
		src  string
		want bool
	}{{"return $a == $b;\n", true}, {"return $a === $b;\n", true}, {"return $a < $b;\n", false}, {"$b[0] = 1;\nreturn $a == $b;\n", false}} {
		r, err := run(t, w17DeepArrays(10_000, tc.src))
		if v, _ := r.Return.Bool(); !ranToCompletion(r, err) || v != tc.want {
			t.Fatalf("%q: %+v %v", tc.src, r, err)
		}
	}
	for _, free := range []string{"unset($a, $b);\nreturn 1;\n", "$a = null;\n$b = 0;\nreturn 1;\n"} {
		r, err := run(t, w17DeepArrays(300_000, free))
		if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 1 {
			t.Fatalf("free %q: %+v %v", free, r, err)
		}
	}
	// Result.Return holds at most 256 nested arrays, as WithGlobal does.
	if r, err := run(t, w17DeepArrays(255, "return $a;\n")); !ranToCompletion(r, err) {
		t.Fatalf("256 levels: %+v %v", r, err)
	}
	for _, depth := range []int{256, 300_000} {
		r, err := run(t, w17DeepArrays(depth, "return $a;\n"))
		var re *ResourceError
		if !errors.As(err, &re) || re.Resource != ResourceReturnDepth || re.Limit != 256 || r.Status != StatusPolicyDenied {
			t.Fatalf("return %d levels: %+v %v", depth, r, err)
		}
	}
}

// An array that shares its parts is copied once per path into
// Result.Return; past runtime.SnapshotMaxElements that is a ResourceError,
// not gigabytes of Go data.
func TestReturnedValueExpansionIsBounded(t *testing.T) {
	e := mustEngine(t, Config{})
	for _, tc := range []struct {
		doublings int
		ok        bool
	}{{18, true}, {20, false}, {40, false}} {
		start := time.Now()
		src := fmt.Sprintf("<?php\n$a = [1];\nfor ($i = 0; $i < %d; $i++) { $a = [$a, $a]; }\nreturn $a;\n", tc.doublings)
		r, err := e.Run(context.Background(), mustCompile(t, e, "share.php", src))
		var re *ResourceError
		switch {
		case tc.ok && !ranToCompletion(r, err):
			t.Fatalf("%d doublings: %+v %v", tc.doublings, r.Status, err)
		case !tc.ok && (!errors.As(err, &re) || re.Resource != ResourceReturnElements || re.Limit != runtime.SnapshotMaxElements):
			t.Fatalf("%d doublings: %v %v", tc.doublings, r.Status, err)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Fatalf("%d doublings took %v", tc.doublings, d)
		}
	}
}

// A memory limit this build cannot meter is refused, not ignored.
func TestMemoryLimitIsRefusedUntilMetered(t *testing.T) {
	e := mustEngine(t, Config{Limits: Limits{Memory: 64 << 20}})
	r, err := e.Run(context.Background(), mustCompile(t, e, "m.php", "<?php return 1;"))
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.Option != "Limits.Memory" || r.Status != StatusFailed || len(r.Output) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if got := err.Error(); got != "phego: unsupported: enforcing a memory limit (Limits.Memory)" {
		t.Fatalf("Error() = %q", got)
	}
	d, _ := Diagnose(err)
	if d.Code != codes.Unsupported || d.Primary.File != "Limits.Memory" {
		t.Fatalf("%+v", d)
	}
}

// Limits.Diagnostics bounds the record only: past it the run goes on and
// displayed diagnostics are unaffected.
func TestDiagnosticsLimitBoundsOnlyTheRecord(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\nfor ($i = 0; $i < 10001; $i++) { echo $u; }\nreturn 7;\n"
	r, err := e.Run(context.Background(), mustCompile(t, e, "d.php", src), WithINI("display_errors", "0"))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 7 || len(r.Diagnostics) != 0 {
		t.Fatalf("nothing recorded: %v %v", r.Status, err)
	}
	r, err = e.Run(context.Background(), mustCompile(t, e, "d.php", src), WithINI("display_errors", "0"), WithINI("log_errors", "1"))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 7 || len(r.Diagnostics) != 10000 {
		t.Fatalf("default record: %v %v %d", r.Status, err, len(r.Diagnostics))
	}
	e = mustEngine(t, Config{Limits: Limits{Diagnostics: 3}})
	src = "<?php\nfor ($i = 0; $i < 10; $i++) { echo $u; }\nreturn 7;\n"
	r, err = e.Run(context.Background(), mustCompile(t, e, "d.php", src), WithINI("log_errors", "1"))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 7 || len(r.Diagnostics) != 3 ||
		strings.Count(string(r.Output), "Warning: Undefined variable $u in d.php on line 2") != 10 {
		t.Fatalf("%v %v %d recorded, output %q", r.Status, err, len(r.Diagnostics), r.Output)
	}
}

type countingWriter struct {
	calls int
	err   error
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}

// After the writer failed, nothing more is written to it, even when the
// same operation raises a second diagnostic.
func TestFailedWriterIsNotWrittenAgain(t *testing.T) {
	e := mustEngine(t, Config{})
	boom := errors.New("client went away")
	w := &countingWriter{err: boom}
	r, err := e.Run(context.Background(), mustCompile(t, e, "w.php", "<?php\n$x = \"5 apples\" + \"3 pears\";\necho 'after';\n"), WithOutput(w))
	if !errors.Is(err, boom) || r.Status != StatusFailed || w.calls != 1 {
		t.Fatalf("%d writes: %+v %v", w.calls, r, err)
	}
}

// Close cancels running work instead of waiting for it forever.
func TestCloseCancelsRunningWork(t *testing.T) {
	e := mustEngine(t, Config{MaxActive: 1})
	p := mustCompile(t, e, "loop.php", "<?php\necho 'x';\nwhile (true) {}\n")
	started := make(chan struct{})
	type outcome struct {
		r   *Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := e.Run(context.Background(), p, WithOutput(writerFunc(func(b []byte) (int, error) {
			close(started)
			return len(b), nil
		})))
		done <- outcome{r, err}
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	begin := time.Now()
	if err := e.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if d := time.Since(begin); d > 5*time.Second {
		t.Fatalf("Close took %v", d)
	}
	got := <-done
	if got.r.Status != StatusCancelled || !errors.Is(got.err, ErrEngineClosed) || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("%+v %v", got.r, got.err)
	}
	if e.policy.admission.Active() != 0 {
		t.Fatal("the cancelled run kept its slot")
	}
	if _, err := e.Compile(context.Background(), "x.php", "<?php"); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("compile after Close: %v", err)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// A run longer than the teardown bound still gets the whole bound for its
// teardown, which starts when the script ends.
func TestCleanupBoundStartsAtTeardown(t *testing.T) {
	saved := cleanupTimeout
	cleanupTimeout = 200 * time.Millisecond
	defer func() { cleanupTimeout = saved }()
	e := mustEngine(t, Config{})
	p := mustCompile(t, e, "slow.php", "<?php\n$s = 0;\nfor ($i = 0; $i < 3000000; $i++) { $s = $s + $i; }\nreturn $s;\n")
	start := time.Now()
	r, err := e.Run(context.Background(), p)
	if time.Since(start) < cleanupTimeout {
		t.Skip("the run was shorter than the teardown bound")
	}
	if !ranToCompletion(r, err) {
		t.Fatalf("%+v %v", r.Status, err)
	}
}

// Compile observes its context in the parser and the lowering pass.
func TestCompileObservesTheContext(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\n" + strings.Repeat("$x = 1;\n", 1_000_000)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := e.Compile(ctx, "big.php", src)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Compile returned %v after its deadline", d)
	}
}

// Faults that are not PHP errors are typed and coded.
func TestCompileAndRunFaultsAreTyped(t *testing.T) {
	e := mustEngine(t, Config{})
	// The constant evaluation budget is this build's limit.
	_, err := e.Compile(context.Background(), "nest.php", "<?php\nreturn "+strings.Repeat("[", 9_000)+strings.Repeat("]", 9_000)+";\n")
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.File != "nest.php" || ue.Line != 2 || !strings.Contains(ue.Feature, "constant expression") {
		t.Fatalf("got %#v", err)
	}
	if d, _ := Diagnose(err); d.Code != codes.UnsupportedConstruct || d.Primary.Length != len("return ")+18_001 {
		t.Fatalf("%+v", d)
	}
	// A run-time fatal error has no PHPErrorKind yet, so it is an engine
	// fault, never success.
	_, err = e.result(context.Background(), runtime.RunResult{End: runtime.Fatal, ExitCode: 255, Err: errors.New("boom")})
	var ie *InternalError
	if !errors.As(err, &ie) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("fatal: %#v", err)
	}
	d, ok := Diagnose(err)
	if !ok || d.Code != codes.Internal {
		t.Fatalf("%+v", d)
	}
	// A teardown failure is reported with the error that ended the script.
	_, err = e.result(context.Background(), runtime.RunResult{End: runtime.UncaughtThrowable, ExitCode: 255,
		Throwable: &runtime.ThrownError{Class: "Error", Message: "m", File: "f.php", Line: 1}, CleanupErr: errors.New("teardown")})
	var pe *PHPError
	if !errors.As(err, &pe) || !errors.As(err, &ie) || !strings.Contains(err.Error(), "teardown") {
		t.Fatalf("cleanup: %v", err)
	}
}

// A run option this build does not implement is named once, as the span.
func TestUnsupportedOptionDiagnosticGolden(t *testing.T) {
	e := mustEngine(t, Config{})
	_, err := e.Run(context.Background(), mustCompile(t, e, "i.php", "<?php return 1;"), WithINI("html_errors", "on"))
	d, ok := Diagnose(err)
	if !ok {
		t.Fatal(err)
	}
	var b bytes.Buffer
	must(t, diag.Render(&b, d, diag.Options{}))
	entry, _ := codes.Lookup(codes.Unsupported)
	want := "error[PHG3000]: HTML error text (html_errors=on) is not supported by this build\n --> WithINI\n  = note: " + entry.Note + "\n  = help: " + entry.Help + "\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
	if d.Primary.Length > 1<<20 {
		t.Fatalf("span length %d", d.Primary.Length)
	}
}

// An unlimited step budget, given as the largest value, keeps the fast
// path and still ends nothing early.
func TestHugeStepBudgetRuns(t *testing.T) {
	e := mustEngine(t, Config{Limits: Limits{Steps: math.MaxInt64}})
	r, err := e.Run(context.Background(), mustCompile(t, e, "s.php", "<?php\n$s = 0;\nfor ($i = 0; $i < 10000; $i++) { $s = $s + 1; }\nreturn $s;\n"))
	if n, _ := r.Return.Int64(); !ranToCompletion(r, err) || n != 10000 {
		t.Fatalf("%+v %v", r, err)
	}
}

// Limits.Frames zero is no limit, as in PHP: recursion far deeper than the
// old 10000 default completes, on the interpreter's own frame stack, with a
// lowered Go stack. An explicit Frames limit is enforced elsewhere
// (TestRunLimitsNameTheResource); Steps and the context still stop runaway
// recursion.
func TestDeepRecursionRunsUnderDefaultLimits(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(8 << 20))
	e := mustEngine(t, Config{})
	src := "<?php\nfunction f($n) {\n    if ($n == 0) {\n        return 0;\n    }\n    return 1 + f($n - 1);\n}\necho f(50000), \"\\n\";\n"
	r, err := e.Run(context.Background(), mustCompile(t, e, "rec.php", src))
	// php -n prints "50000\n" (PHP 8.5.12, linux/arm64).
	if !ranToCompletion(r, err) || string(r.Output) != "50000\n" {
		t.Fatalf("%+v %v", r.Status, err)
	}
	runaway := "<?php\nfunction f($n) { return f($n + 1); }\nf(0);\n"
	e = mustEngine(t, Config{Limits: Limits{Steps: 1_000_000}})
	r, err = e.Run(context.Background(), mustCompile(t, e, "run.php", runaway))
	var re *ResourceError
	if !errors.As(err, &re) || re.Resource != ResourceSteps || r.Status != StatusPolicyDenied {
		t.Fatalf("steps: %+v %v", r.Status, err)
	}
	e = mustEngine(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r, err = e.Run(ctx, mustCompile(t, e, "run.php", runaway))
	if !errors.Is(err, context.DeadlineExceeded) || r.Status != StatusCancelled {
		t.Fatalf("deadline: %+v %v", r.Status, err)
	}
}
