package runtime

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
	"github.com/HazelnutParadise/phego/internal/vm"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. Generators
// and the format 1.10 unwind table, run from PHP source: without liveness
// facts, from many goroutines over one Program, and with their frames kept
// outside the shared stacks.

// phpProgram compiles src (PHP 8.5.12, 64-bit) into a runtime Program.
func phpProgram(t testing.TB, src string) *Program {
	t.Helper()
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: len(src) + 1})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := ir.Lower(tree, prof, ir.Options{File: vmTestFile})
	if err != nil {
		t.Fatal(err)
	}
	image, err := vm.Compile(context.Background(), unit, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	image.Identity = vmIdentity(64)
	bp, err := bytecode.Build(context.Background(), image, bytecode.Options{Limits: bytecode.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProgram(bp)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// dropLiveness removes the liveness facts of every function of p, as for a
// function past the analysis's work bound: registers keep their values
// until they are overwritten, the frame ends or the unwinding frees them.
func dropLiveness(p *Program) {
	for i := range p.funcs {
		f := &p.funcs[i]
		for pc := range f.code {
			f.code[pc].kill, f.code[pc].nkill = 0, 0
		}
		f.kills, f.liveKnown = nil, false
	}
}

const stage8Lifetimes = `<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $d = new D(1); try { $x = yield new D(2); echo "sent $x\n"; yield 2; } finally { echo "finally\n"; } }
$g = gen();
echo $g->current()->n, "\n";
$g->send("a");
unset($g);
echo "--\n";
function inner() { yield 1; return new D(3); }
function outer() { $r = yield from inner(); echo "r ", $r->n, "\n"; }
foreach (outer() as $v) echo $v, "\n";
echo "--\n";
function t() { throw new Exception("t"); }
function g(...$a) { return 1; }
function h() { g(new D(4), g(new D(5), t())); }
try { h(); } catch (Exception $e) { echo "caught\n"; }
echo "end\n";
`

// stage8Want is PHP 8.5.12's output of stage8Lifetimes (measured in the
// pinned arm64 image): the yielded D(2) lives until the next yield replaces
// it, the destroyed generator runs its finally block before its variables
// go, the delegate's return value reaches the delegator, which holds it
// until it ends, and the inner call's arguments go first.
const stage8Want = "2\nsent a\n~D2\nfinally\n~D1\n--\n1\nr 3\n~D3\n--\n~D5\n~D4\ncaught\nend\n"

func TestVMGeneratorsWithoutLiveness(t *testing.T) {
	for _, drop := range []bool{false, true} {
		p := phpProgram(t, stage8Lifetimes)
		if drop {
			dropLiveness(p)
		}
		res := p.Run(vmConfig(t, 64))
		if res.End != Completed {
			t.Fatalf("drop=%v: %v %v", drop, res.End, res.Err)
		}
		got := string(res.Output)
		if !drop && got != stage8Want {
			t.Fatalf("output %q, want %q", got, stage8Want)
		}
		// Without liveness the values outlive their last use, but every
		// one is still freed, once, and the order of the frees the
		// unwinding makes is PHP's.
		for _, want := range []string{"~D1\n", "~D2\n", "~D3\n", "~D4\n", "~D5\n", "finally\n", "r 3\n", "caught\n", "end\n"} {
			if n := countOf(got, want); n != 1 {
				t.Fatalf("drop=%v: %q appears %d times in %q", drop, want, n, got)
			}
		}
		if i, j := indexOf(got, "~D5"), indexOf(got, "~D4"); i > j {
			t.Fatalf("drop=%v: the inner call's argument went after the outer one's: %q", drop, got)
		}
	}
}

func countOf(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestVMGeneratorsSharedProgram runs one Program with generators from 16
// goroutines: every run's Generator objects, suspended frames, sent values
// and delegations are its own. Run it with -race.
func TestVMGeneratorsSharedProgram(t *testing.T) {
	src := `<?php
function counter($start) { $i = $start; while (true) { $cmd = yield $i; if ($cmd === "stop") return $i; $i += $cmd ?? 1; } }
function wrap($g) { $r = yield from $g; return $r * 2; }
$c = counter($n);
$w = wrap($c);
$sum = 0;
foreach ($w as $v) { $sum += $v; if ($v >= $n + 3) { $w->send("stop"); } }
echo $n, ":", $sum, ":", $w->getReturn(), ":", $s, "\n";
`
	p := phpProgram(t, src)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				n := int64(g*1000 + i)
				s := "g" + strconv.Itoa(g) + "i" + strconv.Itoa(i)
				cfg := vmConfig(t, 64)
				cfg.Globals = []Global{{Name: "n", Value: n}, {Name: "s", Value: []byte(s)}}
				res := p.Run(cfg)
				want := fmt.Sprintf("%d:%d:%d:%s\n", n, 4*n+6, 2*(n+3), s)
				if res.End != Completed || string(res.Output) != want {
					errs <- fmt.Errorf("goroutine %d run %d: %v %q %v, want %q", g, i, res.End, res.Output, res.Err, want)
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

// TestVMGeneratorFrameOutsideStacks keeps many generators suspended while
// deep calls reuse the stacks their frames came from: a suspended frame
// holds its own storage, so nothing it holds is overwritten.
func TestVMGeneratorFrameOutsideStacks(t *testing.T) {
	src := `<?php
function gen($k) { $a = [$k, $k * 2]; $s = "s$k"; $x = yield $a[0]; yield $s . $x . $a[1]; }
function deep($n) { $big = [$n, $n, $n, $n, "pad$n"]; return $n == 0 ? 0 : deep($n - 1) + 1; }
$gs = [];
for ($i = 0; $i < 50; $i++) { $g = gen($i); $g->current(); $gs[] = $g; deep(40); }
$out = "";
foreach ($gs as $i => $g) { $out .= $g->send("+") . ","; }
echo $out, "\n";
`
	p := phpProgram(t, src)
	res := p.Run(vmConfig(t, 64))
	want := ""
	for i := 0; i < 50; i++ {
		want += fmt.Sprintf("s%d+%d,", i, 2*i)
	}
	want += "\n"
	if res.End != Completed || string(res.Output) != want {
		t.Fatalf("%v %q %v, want %q", res.End, res.Output, res.Err, want)
	}
}
