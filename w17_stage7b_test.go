package phego

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026; T-009, T-010,
// T-029..T-032. Through the public API: closure objects, their captured
// variables, bound objects and static variables never reach another run of
// the same Program, and a closure held only by a temporary is released at
// PHP's time in functions of any size.

const w17Stage7BProgram = `<?php
function counter() { static $n = 0; return ++$n; }
class Box {
    private $items = [];
    public function __construct(private string $id) {}
    public function __destruct() { echo "<", $this->id, ">"; }
    public function adder() { return function ($v) { $this->items[] = $v; static $calls = 0; return ++$calls; }; }
    public function dump() { $o = $this->id . ":"; foreach ($this->items as $i) { $o .= $i . ","; } return $o; }
    public static function make($id) { return static fn() => new static($id); }
}
$total = 0;
$sum = function ($v) use (&$total, $host) { static $seen = 0; $total += $v; return $host . ++$seen; };
$box = (Box::make($host))();
$add = $box->adder();
echo $sum(1), $sum(2), " ", $total, " ", $add("a"), $add($host), " ", $box->dump(), " ";
$copy = clone $add;
$other = new Box("o" . $host);
$bound = $add->bindTo($other, Box::class);
echo $copy("c"), $bound("d"), $add("e"), " ", $other->dump(), " ";
$c = counter(...);
$twice = fn($f) => $f() + $f();
echo $twice($c), counter(), " ", Closure::fromCallable([$box, 'dump'])(), "\n";
$other = null;
$bound = null;
echo "|";
`

func TestW17Stage7BStateBelongsToTheRun(t *testing.T) {
	e := mustEngine(t, Config{})
	p, err := e.Compile(context.Background(), "/w/stage7b.php", w17Stage7BProgram)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id int) error {
		host := fmt.Sprintf("h%d", id)
		r, err := e.Run(context.Background(), p, WithGlobal("host", Bytes([]byte(host))))
		want := fmt.Sprintf("%[1]s1%[1]s2 3 12 %[1]s:a,%[1]s, 333 o%[1]s:d, 33 %[1]s:a,%[1]s,c,e,\n<o%[1]s>|<%[1]s>", host)
		if err != nil || r.Status != StatusCompleted || string(r.Output) != want {
			return fmt.Errorf("run %d = %+v, %v; want output %q, got %q", id, r, err, want, r.Output)
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

// The closure lifetime fixtures give the same output when {main} is too
// large for the ordinary liveness analysis: a closure in a register, and
// with it the objects it captured or is bound to, is released where PHP
// releases it whatever the size of the function.
func TestW17Stage7BLargeFunctionClosureLifetime(t *testing.T) {
	w17LargeFunctionFixtures(t, []string{"closure_destruct_timing", "closure_bind_this_lifetime", "closure_cycle", "use_objects", "closure_basic", "callable_closure"})
}
