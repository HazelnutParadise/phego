package phego

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026.

// w17Stage7CProgram throws, catches, runs finally blocks and leaves an
// exception chain in a static variable. Everything it creates (the
// exception objects, their traces, the pending completions of the finally
// blocks, the static counter) must belong to the run.
const w17Stage7CProgram = `<?php
class Tagged extends RuntimeException {
    public $tag;
    function __construct($tag, $previous = null) { parent::__construct("tag " . $tag, 7, $previous); $this->tag = $tag; }
}
function counter() { static $n = 0; $n = $n + 1; return $n; }
function fail($host, $depth) {
    try {
        if ($depth > 0) { return fail($host, $depth - 1); }
        throw new Tagged($host);
    } finally {
        counter();
    }
}
function wrapped($host) {
    try {
        fail($host, 3);
    } catch (Tagged $e) {
        throw new LogicException("wrapped " . $e->tag, 1, $e);
    }
}
function finallyReturn($host) {
    try {
        throw new Tagged($host . "-lost");
    } finally {
        return "kept " . $host;
    }
}
try {
    wrapped($host);
} catch (Exception $e) {
    echo $e::class, ":", $e->getMessage(), ":", $e->getPrevious()->getMessage(), ":", $e->getPrevious()->getCode(), "\n";
}
echo finallyReturn($host), ":", counter(), "\n";
try {
    echo 1 % $zero;
} catch (DivisionByZeroError $e) {
    echo $e->getMessage(), " ", $host, "\n";
}
if ($variant) {
    throw new Tagged($host . "-uncaught");
}
return $host;`

func TestW17Stage7CStateBelongsToTheRun(t *testing.T) {
	e := mustEngine(t, Config{})
	p, err := e.Compile(context.Background(), "/w/stage7c.php", w17Stage7CProgram)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id int) error {
		host := fmt.Sprintf("h%d", id)
		odd := id%2 == 1
		r, err := e.Run(context.Background(), p, WithGlobal("host", Bytes([]byte(host))), WithGlobal("variant", Bool(odd)), WithGlobal("zero", Int64(0)),
			WithINI("display_errors", "0"), WithINI("log_errors", "0"))
		want := fmt.Sprintf("LogicException:wrapped %s:tag %s:7\nkept %s:5\nModulo by zero %s\n", host, host, host, host)
		if string(r.Output) != want {
			return fmt.Errorf("run %d output = %q, %v; want %q", id, r.Output, err, want)
		}
		if odd {
			var pe *PHPError
			if r.Status != StatusFailed || !errors.As(err, &pe) || !strings.Contains(pe.Message, "tag "+host+"-uncaught") {
				return fmt.Errorf("run %d = %+v, %v; want an uncaught Tagged of this run", id, r, err)
			}
			return nil
		}
		if err != nil || r.Status != StatusCompleted {
			return fmt.Errorf("run %d = %+v, %v; want completed", id, r, err)
		}
		return nil
	}
	for id := 1; id <= 4; id++ {
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

// The release of temporaries, pending return values and pending exceptions
// around handlers follows the liveness facts of the function. A function
// too large for the bit-set analysis gets the same facts from the
// per-register analysis: the fixtures that observe destructor timing
// around try, catch and finally print the same as PHP there.
func TestW17Stage7CLargeFunctionExceptionTiming(t *testing.T) {
	w17LargeFunctionFixtures(t, []string{"exception_destructor_timing", "exception_trace_holds_args", "throw_in_argument_evaluation",
		"throw_in_expression_temporaries", "throw_in_loops", "throw_from_destructor", "try_catch_variable_scope", "try_catch_in_loops",
		"finally_break_continue", "finally_return_object_lifetime", "finally_discarded_return", "finally_nested", "finally_loop_inside"})
}
