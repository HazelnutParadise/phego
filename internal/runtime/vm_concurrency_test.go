package runtime

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// TestVMSharedProgramIsolation runs one Program from 16 goroutines, 200 runs
// each, with different globals. Every run must see only its own values; run
// it with -race.
func TestVMSharedProgramIsolation(t *testing.T) {
	a := newAsm(t, 64)
	// function twice($v) { $a = [$v, $v]; $a[] = $v * 2; return $a; }
	// $r = twice($n); echo $n, ":", $s, ":", $r[2], "\n"; $u++; return [$r, $s . "!"];
	m := a.fn("{main}", 0, 2, "n", "s", "r", "u")
	m.store("r", m.call(1, m.load("n")))
	m.echo(m.load("n"))
	m.echoK(":")
	m.echo(m.load("s"))
	m.echoK(":")
	m.echo(m.get(m.load("r"), m.k(2), false))
	m.echoK("\n")
	m.incdec(bytecode.PostInc, "u")
	m.ret(m.arrayOf(m.load("r"), m.bin(bytecode.Concat, m.load("s"), m.k("!"))))
	f := a.fn("twice", 1, 3, "v", "a")
	f.store("a", f.arrayOf(f.load("v"), f.load("v")))
	f.push("a", f.bin(bytecode.Multiply, f.load("v"), f.k(2)))
	f.ret(f.load("a"))
	p := a.program()
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				n := int64(g*1000 + i)
				s := "g" + strconv.Itoa(g) + "i" + strconv.Itoa(i)
				cfg := vmConfig(t, 64)
				cfg.Globals = []Global{{Name: "n", Value: n}, {Name: "s", Value: []byte(s)}}
				res := p.Run(cfg)
				wantOut := fmt.Sprintf("\nWarning: Undefined variable $u in %s on line 2\n", vmTestFile)
				wantOut = fmt.Sprintf("%d:%s:%d\n", n, s, 2*n) + wantOut
				want := fmt.Sprintf(`a:[["i:0","a:[[\"i:0\",\"i:%d\"],[\"i:1\",\"i:%d\"],[\"i:2\",\"i:%d\"]]"],["i:1","s:%x"]]`, n, n, 2*n, s+"!")
				if res.End != Completed || string(res.Output) != wantOut || encodeSnapshot(res.Return) != want {
					errs <- fmt.Errorf("goroutine %d run %d: %v %q %s %v", g, i, res.End, res.Output, encodeSnapshot(res.Return), res.Err)
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
