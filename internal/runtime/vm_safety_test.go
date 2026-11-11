package runtime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// echoLoop is `for (;;) echo "x";` with a Checkpoint at the loop head.
func echoLoop(t testing.TB, width int) *Program {
	a := newAsm(t, width)
	m := a.fn("{main}", 0, 2)
	head := m.pc()
	m.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	m.echoK("x")
	m.jump(head)
	return a.program()
}

func TestVMStepLimitStopsBeforeEffects(t *testing.T) {
	p := echoLoop(t, 64)
	for _, steps := range []int64{1, 2, 3, 4, 5, 1023, 1024, 1025, 4097} {
		cfg := vmConfig(t, 64)
		cfg.Steps = steps
		res := p.Run(cfg)
		if res.End != LimitExceeded || !errors.Is(res.Err, ErrStepLimit) {
			t.Fatalf("steps %d: %v %v", steps, res.End, res.Err)
		}
		// Each iteration is Checkpoint, Const, Output, Jump; the echo is the
		// third instruction, so exactly the charged Outputs printed.
		want := int((steps + 1) / 4)
		if len(res.Output) != want || res.Steps != steps {
			t.Fatalf("steps %d: %d bytes printed, %d steps charged; want %d bytes", steps, len(res.Output), res.Steps, want)
		}
		if len(res.Diagnostics) != 0 {
			t.Fatalf("a host stop must not be reported as PHP: %v", res.Diagnostics)
		}
	}
}

// cancelWriter cancels the run at its n-th write and records the steps.
type cancelWriter struct {
	cancel context.CancelFunc
	n      int
	writes int
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.n {
		w.cancel()
	}
	return len(p), nil
}

func TestVMCancellationBound(t *testing.T) {
	run := func(t *testing.T, p *Program) RunResult {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg := vmConfig(t, 64)
		cfg.Context = ctx
		cfg.Output = &cancelWriter{cancel: cancel, n: 1}
		return p.Run(cfg)
	}
	t.Run("jump-only loop", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.echoK("go") // step 2: the write cancels
		head := m.pc()
		m.jump(head)
		res := run(t, a.program())
		if res.End != Cancelled || !errors.Is(res.Err, context.Canceled) {
			t.Fatalf("%v %v", res.End, res.Err)
		}
		// The Output's own write path observes the cancellation, or the
		// first back-edge does.
		if res.Steps > 3 {
			t.Fatalf("cancellation observed after %d steps", res.Steps)
		}
	})
	t.Run("straight line", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.echoK("go")
		r := m.reg(bytecode.Value)
		for i := 0; i < 5000; i++ {
			m.emit(bytecode.OpConst, 0, r, a.constant(bytecode.Constant{Kind: bytecode.Int, Bits: uint64(i)}), 0)
		}
		m.retNull()
		res := run(t, a.program())
		if res.End != Cancelled {
			t.Fatalf("%v %v", res.End, res.Err)
		}
		if res.Steps > 2+vmPollInterval {
			t.Fatalf("cancellation observed after %d steps, bound %d", res.Steps, 2+vmPollInterval)
		}
	})
	t.Run("recursion", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.echoK("go")
		m.call(1)
		m.retNull()
		r := a.fn("r", 0, 3)
		r.call(1)
		r.retNull()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg := vmConfig(t, 64)
		cfg.Context = ctx
		cfg.Depth = 1 << 20
		cfg.Output = &cancelWriter{cancel: cancel, n: 1}
		res := a.program().Run(cfg)
		if res.End != Cancelled || res.Steps > 4 {
			t.Fatalf("%v after %d steps: %v", res.End, res.Steps, res.Err)
		}
	})
	t.Run("every instruction candidate", func(t *testing.T) {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.echoK("go")
		for i := 0; i < 50; i++ {
			m.k(i)
		}
		m.retNull()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg := vmConfig(t, 64)
		cfg.Context = ctx
		cfg.variant.polling = 1
		cfg.Output = &cancelWriter{cancel: cancel, n: 1}
		res := a.program().Run(cfg)
		if res.End != Cancelled || res.Steps > 3 {
			t.Fatalf("%v after %d steps", res.End, res.Steps)
		}
	})
}

func TestVMDepthLimit(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.call(1, m.k(0))
	m.retNull()
	r := a.fn("r", 1, 3, "n")
	r.echoK(".")
	r.call(1, r.bin(bytecode.Add, r.load("n"), r.k(1)))
	r.retNull()
	cfg := vmConfig(t, 64)
	cfg.Depth = 10
	res := a.program().Run(cfg)
	// {main} and nine frames of r; the tenth call exceeds the limit.
	if res.End != LimitExceeded || string(res.Output) != strings.Repeat(".", 9) {
		t.Fatalf("%v %q %v", res.End, res.Output, res.Err)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("a depth stop is not a PHP error: %v", res.Diagnostics)
	}
	cfg = vmConfig(t, 64)
	cfg.Callbacks = 5
	res = a.program().Run(cfg)
	if res.End != LimitExceeded || string(res.Output) != strings.Repeat(".", 4) {
		t.Fatalf("call budget: %v %q %v", res.End, res.Output, res.Err)
	}
}

type vmFailingWriter struct{ err error }

func (w vmFailingWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestVMOutputLimitAndWriterFailure(t *testing.T) {
	p := echoLoop(t, 64)
	cfg := vmConfig(t, 64)
	cfg.OutputBytes = 10
	res := p.Run(cfg)
	if res.End != LimitExceeded || string(res.Output) != strings.Repeat("x", 10) {
		t.Fatalf("output limit: %v %q", res.End, res.Output)
	}
	boom := errors.New("disk full")
	cfg = vmConfig(t, 64)
	cfg.Output = vmFailingWriter{boom}
	res = p.Run(cfg)
	if res.End != OutputFailed || !errors.Is(res.Err, boom) || res.Steps != 3 {
		t.Fatalf("writer failure: %v %v after %d steps", res.End, res.Err, res.Steps)
	}
	// A displayed diagnostic that cannot be written stops the run as well.
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "u")
	m.echo(m.load("u"))
	m.echoK("never")
	m.retNull()
	cfg = vmConfig(t, 64)
	cfg.Output = vmFailingWriter{boom}
	res = a.program().Run(cfg)
	if res.End != OutputFailed || !errors.Is(res.Err, boom) {
		t.Fatalf("diagnostic write failure: %v %v", res.End, res.Err)
	}
}

func TestVMProgramReuseAfterFailedRun(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "i", "s")
	m.store("i", m.k(0))
	m.store("s", m.k(""))
	head := m.pc()
	m.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	br := m.branch(m.bin(bytecode.Less, m.load("i"), m.k(5)))
	body := m.pc()
	m.store("s", m.bin(bytecode.Concat, m.load("s"), m.load("i")))
	m.incdec(bytecode.PreInc, "i")
	m.jump(head)
	done := m.pc()
	m.patch(br, body, done)
	m.echo(m.load("s"))
	m.ret(m.load("i"))
	p := a.program()
	fresh := p.Run(vmConfig(t, 64))
	cfg := vmConfig(t, 64)
	cfg.Steps = 20
	if res := p.Run(cfg); res.End != LimitExceeded {
		t.Fatalf("limited run: %v", res.End)
	}
	again := p.Run(vmConfig(t, 64))
	if fresh.End != Completed || string(fresh.Output) != "01234" || fresh.Return != int64(5) ||
		again.End != fresh.End || string(again.Output) != string(fresh.Output) || again.Return != fresh.Return || again.Steps != fresh.Steps {
		t.Fatalf("fresh %v %q %v, again %v %q %v", fresh.End, fresh.Output, fresh.Return, again.End, again.Output, again.Return)
	}
}

// TestVMUndefinedRegistersNeverPanic runs code the verifier never reaches by
// dataflow: it follows a terminal. The decoded form is then edited to make
// it reachable, as a corrupted VM would see it, and must end as a located
// internal error or as PHP's undefined value, never a panic or a value left
// by another frame.
func TestVMUndefinedRegistersNeverPanic(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.echo(m.call(1)) // r() leaves 42 in its register 0, then returns
	m.echo(m.call(2)) // s() reads its register 0 before writing it
	m.retNull()
	r := a.fn("r", 0, 3)
	r.ret(r.k(42))
	s := a.fn("s", 0, 4)
	s.retNull()
	undefinedValue := s.reg(bytecode.Value)
	undefinedLoc := s.reg(bytecode.Location)
	s.echo(undefinedValue)
	s.emit(bytecode.OpLoad, 0, s.reg(bytecode.Value), undefinedLoc, 0)
	s.retNull()
	p := a.program()
	if res := p.Run(vmConfig(t, 64)); res.End != Completed || string(res.Output) != "42" {
		t.Fatalf("verified run: %v %q %v", res.End, res.Output, res.Err)
	}
	// Make the unreachable tail reachable: skip s's first two instructions.
	f := &p.funcs[2]
	f.code[0] = vmInstr{op: bytecode.OpJump, a: 2}
	f.code[1] = vmInstr{op: bytecode.OpJump, a: 2}
	res := p.Run(vmConfig(t, 64))
	var ie *InternalError
	if res.End != Internal || !errors.As(res.Err, &ie) || ie.Function != "s" || ie.PC != 3 {
		t.Fatalf("%v %v", res.End, res.Err)
	}
	// The undefined Value register printed as PHP's undefined value, not as
	// the 42 r() left in the same window.
	if string(res.Output) != "42" {
		t.Fatalf("output %q", res.Output)
	}
	if !strings.Contains(res.Err.Error(), "location register") || strings.Contains(res.Err.Error(), "\n") {
		t.Fatalf("diagnostic %q", res.Err)
	}
}

func TestVMIntegerOverflow32(t *testing.T) {
	for _, width := range []int{32, 64} {
		a := newAsm(t, width)
		m := a.fn("{main}", 0, 2, "i", "j")
		max := int64(2147483647)
		m.store("i", m.k(max))
		m.incdec(bytecode.PreInc, "i")
		m.store("j", m.k(-max-1))
		m.incdec(bytecode.PostDec, "j")
		m.ret(m.arrayOf(m.bin(bytecode.Add, m.k(max), m.k(1)), m.bin(bytecode.Multiply, m.k(max), m.k(2)),
			m.bin(bytecode.Subtract, m.k(-max-1), m.k(1)), m.load("i"), m.load("j"), m.unary(bytecode.Negate, m.k(-max-1)),
			m.bin(bytecode.ShiftLeft, m.k(1), m.k(31))))
		res := runAsm(t, a)
		got := encodeSnapshot(res.Return)
		want := map[int]string{
			32: `a:[["i:0","d:41e0000000000000"],["i:1","d:41efffffffc00000"],["i:2","d:c1e0000000200000"],["i:3","d:41e0000000000000"],["i:4","d:c1e0000000200000"],["i:5","d:41e0000000000000"],["i:6","i:-2147483648"]]`,
			64: `a:[["i:0","i:2147483648"],["i:1","i:4294967294"],["i:2","i:-2147483649"],["i:3","i:2147483648"],["i:4","i:-2147483649"],["i:5","i:2147483648"],["i:6","i:2147483648"]]`,
		}[width]
		if got != want {
			t.Errorf("%d bits:\n got %s\nwant %s", width, got, want)
		}
	}
}

func TestVMUnsupportedConstructs(t *testing.T) {
	cases := map[string]func(a *vmAsm){
		"Throw": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			m.emit(bytecode.OpThrow, 0, m.k(1), 0, 0)
		},
		"HostCall": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			r := m.reg(bytecode.Value)
			a.required = bytecode.HostCallbacks
			m.emit(bytecode.OpHostCall, 0, r, a.str("strlen"), 0)
			m.ret(r)
		},
		"Include": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			r := m.reg(bytecode.Value)
			a.required = bytecode.FileRead
			m.emit(bytecode.OpInclude, 0, r, m.k("x.php"), 0)
			m.ret(r)
		},
		"non-string callable": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			r := m.reg(bytecode.Value)
			m.emit(bytecode.OpCallValue, 0, r, m.arrayOf(), 0)
			m.ret(r)
		},
		"named argument": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			m.emit(bytecode.OpPush, 0, m.k(1), a.str("x")+1, 0)
			m.ret(m.callNameNoPush("f", 1))
			f := a.fn("f", 1, 3, "x")
			f.retNull()
		},
		"string offset read": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2)
			m.ret(m.get(m.k("abc"), m.k(0), false))
		},
		"string offset write": func(a *vmAsm) {
			m := a.fn("{main}", 0, 2, "s")
			m.store("s", m.k("abc"))
			m.set("s", m.k(0), m.k("x"))
			m.retNull()
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAsm(t, 64)
			build(a)
			m := a.funcs[0]
			_ = m
			res := runAsm(t, a)
			var u *UnsupportedError
			if res.End != Unsupported || !errors.As(res.Err, &u) || u.File != vmTestFile || u.Function != "{main}" {
				t.Fatalf("%v %v", res.End, res.Err)
			}
			if len(res.Diagnostics) != 0 || len(res.Output) != 0 {
				t.Fatalf("unsupported must not look like PHP: %v %q", res.Diagnostics, res.Output)
			}
		})
	}
	t.Run("handlers", func(t *testing.T) {
		img := newAsm(t, 64)
		m := img.fn("{main}", 0, 2)
		m.echoK("never")
		m.retNull()
		image := img.image()
		f := &image.Functions[0]
		f.Code = append([]bytecode.Instruction{{Op: bytecode.OpLeave, A: 2}, {Op: bytecode.OpEndFinally}}, f.Code...)
		f.Positions = append([]bytecode.Position{{Source: 1, Line: 2}, {Source: 1, Line: 2}}, f.Positions...)
		f.Handlers = []bytecode.Handler{{Start: 0, End: 1, Target: 1, BodyEnd: 2, Kind: bytecode.Finally}}
		bp, err := bytecode.Build(context.Background(), image, bytecode.Options{Limits: bytecode.DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		p, err := NewProgram(bp)
		if err != nil {
			t.Fatal(err)
		}
		res := p.Run(vmConfig(t, 64))
		if res.End != Unsupported || !strings.Contains(res.Err.Error(), "try, catch and finally") || len(res.Output) != 0 {
			t.Fatalf("%v %v %q", res.End, res.Err, res.Output)
		}
	})
}

func (f *vmFn) callNameNoPush(name string, argc int) uint32 {
	n := f.k(name)
	r := f.reg(bytecode.Value)
	f.emit(bytecode.OpCallValue, 0, r, n, uint32(argc))
	return r
}

func TestVMGlobalsAndErrorReporting(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2, "n", "s", "arr", "u")
	m.echo(m.load("n"))
	m.echo(m.load("s"))
	m.echo(m.get(m.load("arr"), m.k("k"), false))
	m.echo(m.load("u"))                                  // undefined: Warning
	m.echo(m.bin(bytecode.Concat, m.k(""), m.arrayOf())) // Warning: Array to string conversion
	m.ret(m.load("arr"))
	p := a.program()
	cfg := vmConfig(t, 64)
	cfg.Globals = []Global{{Name: "n", Value: int64(7)}, {Name: "s", Value: []byte("-")}, {Name: "missing", Value: true},
		{Name: "arr", Value: []SnapshotEntry{{Key: "k", Value: 1.5}}}}
	cfg.ErrorReporting = 32767 &^ levelWarning
	res := p.Run(cfg)
	if res.End != Completed || string(res.Output) != "7-1.5Array" || len(res.Diagnostics) != 0 {
		t.Fatalf("%v %q %v %v", res.End, res.Output, res.Diagnostics, res.Err)
	}
	if got := encodeSnapshot(res.Return); got != `a:[["s:6b","d:3ff8000000000000"]]` {
		t.Fatalf("return %s", got)
	}
	cfg = vmConfig(t, 64)
	cfg.Globals = []Global{{Name: "n", Value: struct{}{}}}
	if res := p.Run(cfg); res.End != Internal || !strings.Contains(res.Err.Error(), "global $n") {
		t.Fatalf("bad global: %v %v", res.End, res.Err)
	}
	cfg = vmConfig(t, 64)
	cfg.LogErrors, cfg.DisplayErrors = true, false
	res = p.Run(cfg)
	if len(res.Diagnostics) != 7 || res.Diagnostics[0] != (RunDiagnostic{"Warning", "Undefined variable $n", vmTestFile, 2}) || len(res.Output) != len("Array") {
		t.Fatalf("diagnostics %+v", res.Diagnostics)
	}
}

func TestVMRunConfigErrors(t *testing.T) {
	p := echoLoop(t, 64)
	bad := []func(c *RunConfig){
		func(c *RunConfig) { c.Context = nil },
		func(c *RunConfig) { c.Cleanup = nil },
		func(c *RunConfig) { c.Request = nil },
		func(c *RunConfig) { c.IntWidth = 32 },
		func(c *RunConfig) { c.Depth = 0 },
		func(c *RunConfig) { c.Callbacks = 0 },
		func(c *RunConfig) { c.OutputBytes = 0 },
		func(c *RunConfig) { c.Steps = -1 },
		func(c *RunConfig) { c.Cleanup = context.Background() }, // no deadline
	}
	for i, edit := range bad {
		cfg := vmConfig(t, 64)
		edit(&cfg)
		if res := p.Run(cfg); res.End != Internal || res.Err == nil {
			t.Fatalf("case %d: %v %v", i, res.End, res.Err)
		}
	}
	var buf bytes.Buffer
	cfg := vmConfig(t, 64)
	cfg.Output = &buf
	cfg.Steps = 7
	if res := p.Run(cfg); res.End != LimitExceeded || buf.String() != "xx" || res.Output != nil {
		t.Fatalf("writer output: %v %q %q", res.End, buf.String(), res.Output)
	}
}
