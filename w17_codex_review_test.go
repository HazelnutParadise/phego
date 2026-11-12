package phego

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// PHP folds only ASCII letters when it looks a function up: abc() and ABC()
// are one function, É() and é() are two. The expected bytes are PHP
// 8.5.12's (php -n in php@sha256:add6a138…f9d9) for this source saved as
// /w/u.php.
func TestFunctionNamesFoldOnlyASCII(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\nfunction É() { echo \"upper\\n\"; }\nfunction abc() { echo \"abc\\n\"; }\nABC();\né();\n"
	r, err := e.Run(context.Background(), mustCompile(t, e, "/w/u.php", src))
	want := "abc\n\nFatal error: Uncaught Error: Call to undefined function é() in /w/u.php:5\nStack trace:\n#0 {main}\n  thrown in /w/u.php on line 5\n"
	if string(r.Output) != want || r.ExitCode != 255 {
		t.Fatalf("output %q exit %d err %v\nwant %q", r.Output, r.ExitCode, err, want)
	}
}

// A run waiting for a slot when Close is called ends as cancelled by Close,
// like a run that had already started.
func TestCloseCancelsARunWaitingForASlot(t *testing.T) {
	e := mustEngine(t, Config{MaxActive: 1, MaxQueued: 1})
	p := mustCompile(t, e, "loop.php", "<?php\necho 'x';\nwhile (true) {}\n")
	started := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := e.Run(context.Background(), p, WithOutput(writerFunc(func(b []byte) (int, error) {
			close(started)
			return len(b), nil
		})))
		first <- err
	}()
	<-started
	type outcome struct {
		r   *Result
		err error
	}
	queued := make(chan outcome, 1)
	go func() {
		r, err := e.Run(context.Background(), p)
		queued <- outcome{r, err}
	}()
	for e.policy.admission.Queued() == 0 {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got := <-queued
	if got.r.Status != StatusCancelled || !errors.Is(got.err, ErrEngineClosed) || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("queued run: %+v %v", got.r, got.err)
	}
	if err := <-first; !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("running run: %v", err)
	}
}

// openCounter counts the files opened through it.
type openCounter struct {
	fs.FS
	opens int
}

func (c *openCounter) Open(name string) (fs.File, error) {
	c.opens++
	return c.FS.Open(name)
}

// A memory limit that cannot be enforced is refused before RunFile opens
// its source, which could be a FIFO that waits for a writer.
func TestRunFileRefusesMemoryLimitBeforeOpening(t *testing.T) {
	files := &openCounter{FS: fstest.MapFS{"a.php": {Data: []byte("<?php return 1;")}}}
	e := mustEngine(t, Config{Files: files, Limits: Limits{Memory: 1 << 20}})
	r, err := e.RunFile(context.Background(), "a.php")
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.Option != "Limits.Memory" || r.Status != StatusFailed {
		t.Fatalf("%+v %v", r, err)
	}
	if files.opens != 0 {
		t.Fatalf("the source was opened %d times before the refusal", files.opens)
	}
}

// A compile whose context ended does not hand out a program, also when the
// context ended during the last step, which does not observe it.
func TestCancelledCompileReturnsNoProgram(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\n" + strings.Repeat("$x = $x + 1;\n", 60_000)
	begin := time.Now()
	mustCompile(t, e, "big.php", src)
	whole := time.Since(begin)
	for i := 1; i <= 40; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		var at time.Time
		timer := time.AfterFunc(whole*time.Duration(i)/40, func() { at = time.Now(); cancel() })
		p, err := e.Compile(ctx, "big.php", src)
		back := time.Now()
		timer.Stop()
		cancelled := ctx.Err() != nil
		cancel()
		if p != nil && cancelled && back.Sub(at) > 20*time.Millisecond {
			t.Fatalf("delay %d/40: Compile returned a program %v after its context ended", i, back.Sub(at))
		}
		if p == nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("delay %d/40: %v", i, err)
		}
	}
}

// Large strings are joined in chunks; the result is the same bytes.
func TestLargeConcatMatchesSmall(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\n$s = 'ab';\nfor ($i = 0; $i < 22; $i++) { $s = $s . $s; }\n$t = $s . 'tail' . $s;\nreturn $t;\n"
	r, err := e.Run(context.Background(), mustCompile(t, e, "c.php", src))
	if !ranToCompletion(r, err) {
		t.Fatalf("%v %v", r.Status, err)
	}
	half := strings.Repeat("ab", 1<<22)
	if got, _ := r.Return.Bytes(); string(got) != half+"tail"+half {
		t.Fatalf("joined string differs: %d bytes", len(got))
	}
}

// Two functions whose names differ only in the case of a non-ASCII letter
// are two functions. PHP 8.5.12 prints "Ul" for this source.
func TestNonASCIICaseDeclaresTwoFunctions(t *testing.T) {
	e := mustEngine(t, Config{})
	src := "<?php\nfunction É() { echo \"U\"; }\nfunction é() { echo \"l\"; }\nÉ(); é();\n"
	r, err := e.Run(context.Background(), mustCompile(t, e, "/w/v.php", src))
	if !ranToCompletion(r, err) || string(r.Output) != "Ul" {
		t.Fatalf("output %q %v %v", r.Output, r.Status, err)
	}
}
