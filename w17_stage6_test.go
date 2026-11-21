package phego

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// W-17 stage 6: REQ-001, REQ-016, REQ-025, REQ-026; T-009, T-010,
// T-029..T-032. Through the public API: what a host's globals look like to
// a global statement, that the static and global state of one run never
// reaches another run of the same Program, and that the constructs stage 6
// still leaves out are refused by Compile with their location.

const w17Stage6Program = `<?php
function counter() {
    static $n = 0, $log = [];
    $n++;
    $log[] = $n;
    return $n . "/" . count_of($log);
}
function count_of($a) {
    $c = 0;
    foreach ($a as $x) { $c++; }
    return $c;
}
function seen() {
    global $host, $only_in_function, $made;
    $made = $host . "+" . $only_in_function;
    $host .= "!";
}
function collect(...$named) {
    static $calls = 0;
    $calls++;
    $out = $calls . ":";
    foreach ($named as $k => $v) { $out .= $k . "=" . $v . ","; }
    return $out;
}
echo counter(), " ", counter(), " ", counter(), "\n";
seen();
seen();
echo $host, " ", $made, "\n";
echo collect(...$args, tail: $host), " ", collect(a: 1), "\n";
`

// Every run starts its static variables from their initializers and sees
// only its own host globals, in sequence and from 16 goroutines at once
// (run with -race). $only_in_function is a global {main} never names: the
// host's value reaches the function through the global statement alone.
func TestW17Stage6StateBelongsToTheRun(t *testing.T) {
	e := mustEngine(t, Config{})
	p, err := e.Compile(context.Background(), "/w/stage6.php", w17Stage6Program)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id int) error {
		host := fmt.Sprintf("h%d", id)
		args := Array([]Entry{{StringKey("x"), Int64(int64(id))}, {StringKey("y"), Bytes([]byte("why"))}})
		r, err := e.Run(context.Background(), p,
			WithGlobal("host", Bytes([]byte(host))), WithGlobal("only_in_function", Int64(int64(id))), WithGlobal("args", args))
		want := fmt.Sprintf("1/1 2/2 3/3\n%s!! %s!+%d\n1:x=%d,y=why,tail=%s!!, 2:a=1,\n", host, host, id, id, host)
		if err != nil || r.Status != StatusCompleted || string(r.Output) != want {
			return fmt.Errorf("run %d = %+v, %v; want output %q", id, r, err, want)
		}
		return nil
	}
	for id := 1; id <= 3; id++ {
		if err := run(id); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := run(g*1000 + i); err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// A run whose static initializer throws leaves nothing behind: the next
// run evaluates the initializer again and fails the same way.
func TestW17Stage6FailedInitializerIsNotRemembered(t *testing.T) {
	e := mustEngine(t, Config{})
	p, err := e.Compile(context.Background(), "/w/stage6.php", "<?php\nfunction f() {\n  echo \"init \";\n  static $v = [1] + MISSING;\n  return $v;\n}\nf();\n")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		r, err := e.Run(context.Background(), p)
		var pe *PHPError
		if !errors.As(err, &pe) || r.Status != StatusFailed || r.ExitCode != 255 || !strings.HasPrefix(string(r.Output), "init \nFatal error: Uncaught Error: Undefined constant \"MISSING\"") {
			t.Fatalf("run %d = %+v, %v", i, r, err)
		}
	}
}

// What stage 6 does not run is refused by Compile, located, before any
// output: nothing is compiled by approximation.
func TestW17Stage6UnsupportedIsRefusedAtCompile(t *testing.T) {
	e := mustEngine(t, Config{})
	for _, c := range []struct {
		name, src, feature string
		line               int
	}{
		{"superglobal read", "<?php\necho 1;\nfunction f() {\n  return $GLOBALS['a'];\n}\n", "superglobal", 4},
		{"superglobal global", "<?php\necho 1;\nfunction f() {\n  global $GLOBALS;\n}\n", "superglobal", 4},
		{"dynamic global", "<?php\necho 1;\nfunction f($n) {\n  global $$n;\n}\n", "variable variables", 4},
		{"func_get_args", "<?php\necho 1;\nfunction f(...$a) {\n  return func_get_args();\n}\n", "builtin function func_get_args()", 4},
		{"unpacking an object", "<?php\necho 1;\nfunction f(...$a) {}\nf(...new ArrayIterator([1]));\n", "builtin class ArrayIterator", 4},
		{"unpacking a generator", "<?php\necho 1;\nfunction f(...$a) {}\nf(...(function () { yield 1; })());\n", "generator", 4},
		{"variadic attribute", "<?php\necho 1;\nfunction f(#[A] int ...$a) {}\n", "parameter attribute", 3},
		{"static closure returning by reference", "<?php\necho 1;\n$f = static function &() {};\n", "return by reference", 3},
		{"enum case", "<?php\necho 1;\nenum A { case B; }\n", "enum declaration", 3},
		{"static with a builtin class", "<?php\necho 1;\nfunction f() {\n  static $o = new ArrayObject;\n}\n", "builtin class ArrayObject", 4},
		{"first-class callable of a builtin function", "<?php\necho 1;\n$f = strlen(...);\n", "a closure of builtin function strlen()", 3},
		{"anonymous class", "<?php\necho 1;\nnew class(a: 1) {};\n", "anonymous class", 3},
		{"by-reference value by name", "<?php\necho 1;\nfunction f(&$r) {}\nf(r: 1 + 2);\n", "passing a value that is not a variable by reference", 4},
		{"by-reference call result by name", "<?php\necho 1;\nfunction f(&$r) {}\nf(r: f($x));\n", "passing a value that is not a variable by reference", 4},
		{"array spread", "<?php\necho 1;\n$a = [...$b];\n", "array spread", 3},
		{"first-class callable of a namespaced builtin name", "<?php\necho 1;\nfunction f() {}\n$g = \\count(...);\n", "a closure of builtin function count()", 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			file := "/w/cases/s6_" + strings.ReplaceAll(c.name, " ", "_") + ".php"
			p, err := e.Compile(context.Background(), file, c.src)
			var ue *UnsupportedError
			if p != nil || !errors.As(err, &ue) {
				t.Fatalf("Compile = (%v, %v), want a located UnsupportedError", p, err)
			}
			if ue.Feature != c.feature || ue.File != file || ue.Line != c.line {
				t.Fatalf("Compile refused %#v, want feature %q at %s:%d", ue, c.feature, file, c.line)
			}
		})
	}
}
