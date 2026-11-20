package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 6: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. What the
// VM does with static and global variables and with started calls in
// hand-written bytecode: the PHP-visible text is compared with the pinned
// images by the fixtures under tests/vm/w17; these tests cover the
// run-local state and the failure modes compiled code never reaches.

// params gives f the parameter table of a function whose parameter
// registers have these names; a name starting with "&" is by reference and
// one starting with "..." is the variadic parameter.
func (f *vmFn) parameters(names ...string) *vmFn {
	for _, n := range names {
		var flags bytecode.ParameterFlags
		if strings.HasPrefix(n, "...") {
			flags, n = flags|bytecode.ParamVariadic, n[3:]
		}
		if strings.HasPrefix(n, "&") {
			flags, n = flags|bytecode.ParamByRef, n[1:]
		}
		f.table = append(f.table, bytecode.Parameter{Name: f.a.str(n), Flags: flags})
	}
	return f
}

// counter adds "function counter() { static $n = 0; $n++; return $n; }".
func counter(a *vmAsm) {
	f := a.fn("counter", 0, 5, "n")
	init := f.emit(bytecode.OpStaticInit, 0, f.loc("n"), 0, 0)
	f.emit(bytecode.OpBindStatic, 0, f.loc("n"), f.k(0), 0)
	f.code[init].B = f.pc()
	f.incdec(bytecode.PreInc, "n")
	f.ret(f.load("n"))
}

// A static variable and a global one a function made belong to the run: the
// next run of the same Program starts from the initializer again, and runs
// at the same time never see each other's values. Run it with -race.
func TestStaticsAndGlobalsBelongToTheRun(t *testing.T) {
	a := newAsm(t, 64)
	// echo counter(), counter(), ":"; bump(); bump(); global $made; echo $made, ":", $n;
	m := a.fn("{main}", 0, 1, "n", "made")
	m.echo(m.call(1))
	m.echo(m.call(1))
	m.echoK(":")
	m.call(2)
	m.call(2)
	m.emit(bytecode.OpBindGlobal, 0, m.loc("made"), a.str("made"), 0)
	m.echo(m.load("made"))
	m.echoK(":")
	m.echo(m.load("n"))
	m.retNull()
	counter(a)
	// function bump() { global $n, $hidden; $n = $n + 1; $hidden = $hidden + $n; global $made; $made = $hidden; }
	b := a.fn("bump", 0, 9, "n", "hidden", "made")
	b.emit(bytecode.OpBindGlobal, 0, b.loc("n"), a.str("n"), 0)
	b.emit(bytecode.OpBindGlobal, 0, b.loc("hidden"), a.str("hidden"), 0)
	b.store("n", b.bin(bytecode.Add, b.load("n"), b.k(1)))
	b.store("hidden", b.bin(bytecode.Add, b.loadQuiet("hidden"), b.load("n")))
	b.emit(bytecode.OpBindGlobal, 0, b.loc("made"), a.str("made"), 0)
	b.store("made", b.load("hidden"))
	b.retNull()
	p := a.program()
	run := func(n int64) error {
		cfg := vmConfig(t, 64)
		// $n is one of {main}'s variables; $hidden is not, so the host's
		// value reaches the function through the global table alone.
		cfg.Globals = []Global{{Name: "n", Value: n}, {Name: "hidden", Value: n * 1000}}
		res := p.Run(cfg)
		want := fmt.Sprintf("12:%d:%d", n*1000+2*n+3, n+2)
		if res.End != Completed || string(res.Output) != want || len(res.Diagnostics) != 0 {
			return fmt.Errorf("n=%d: %v %q %v %v; want %q", n, res.End, res.Output, res.Diagnostics, res.Err, want)
		}
		return nil
	}
	for i := int64(1); i <= 3; i++ {
		if err := run(i); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if err := run(int64(g*1000 + i)); err != nil {
					errs <- fmt.Errorf("goroutine %d: %w", g, err)
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

// An initializer that ends the run leaves the variable uninitialized, and
// the values of the static variables do not outlive a run that was stopped.
func TestStaticStateAfterAStoppedRun(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "stop")
	m.echo(m.call(1))
	m.echo(m.call(1))
	br := m.branch(m.load("stop"))
	m.patch(br, m.pc(), m.pc()+1)
	m.exit()
	m.echo(m.call(1))
	m.retNull()
	counter(a)
	p := a.program()
	for i, tc := range []struct {
		stop bool
		want string
	}{{true, "12"}, {false, "123"}, {true, "12"}, {false, "123"}} {
		cfg := vmConfig(t, 64)
		cfg.Globals = []Global{{Name: "stop", Value: tc.stop}}
		if res := p.Run(cfg); string(res.Output) != tc.want || res.Err != nil {
			t.Fatalf("run %d: %v %q %v; want %q", i, res.End, res.Output, res.Err, tc.want)
		}
	}
}

// BindStatic and the Bind after RecvVariadic move their value out of the
// register, so the variable is the array's only owner whether or not
// liveness runs: an append then needs no copy. The reference a named Push
// sends moves to the call as well. The register reads as null afterwards.
func TestStage6InstructionsEmptyTheirRegisters(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x")
	withoutLiveness(m)
	m.stack = 4
	m.store("x", m.k(5))
	r := m.reg(bytecode.Value)
	m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	m.emit(bytecode.OpMakeRef, 0, r, m.loc("x"), 0)
	m.emit(bytecode.OpPush, 0, r, a.str("p")+1, 0)
	res := m.reg(bytecode.Value)
	m.emit(bytecode.OpCallPending, 0, res, 0, 0)
	m.echoK("[")
	m.echo(r) // the reference moved to the call
	m.echoK("]")
	m.echo(m.load("x"))
	m.retNull()
	g := a.fn("g", 1, 1, "p").parameters("&p")
	g.code[len(g.code)-1] = bytecode.Instruction{Op: bytecode.OpBind, Aux: uint16(bytecode.BindValue), A: g.loc("p"), B: 0}
	g.store("p", g.k(7))
	g.retNull()
	out := runAsm(t, a)
	if out.Err != nil || string(out.Output) != "[]7" {
		t.Fatalf("output %q, err %v; want \"[]7\"", out.Output, out.Err)
	}
}

// A named or unpacked argument needs a call the same frame started: the
// pending call of a caller is not the callee's to fill.
func TestStartedCallBelongsToItsFrame(t *testing.T) {
	cases := map[string]func(m, g *vmFn){
		"named push without a call": func(m, g *vmFn) {
			m.emit(bytecode.OpPush, 0, m.k(1), m.a.str("p")+1, 0)
			m.exit()
		},
		"named push in the callee of a started call": func(m, g *vmFn) {
			m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
			m.emit(bytecode.OpPush, 0, m.call(2), 0, 0)
			m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 1)
			m.stack = 4
			g := a.fn("g", 1, 2, "p").parameters("p")
			g.retNull()
			// function h() { <named Push with no started call>; exit; }
			// Nothing but CallPending may take a named argument, so the
			// verifier accepts the Push only where it stays pending.
			h := a.fn("h", 0, 3)
			h.stack = 2
			h.emit(bytecode.OpPush, 0, h.k(1), a.str("p")+1, 0)
			h.exit()
			build(m, g)
			m.retNull()
			res := runAsm(t, a)
			var u *UnsupportedError
			if res.End != Unsupported || !errors.As(res.Err, &u) || !strings.Contains(u.Feature, "outside a started call") {
				t.Fatalf("End %v, err %v; want Unsupported for an argument outside a started call", res.End, res.Err)
			}
		})
	}
}

// A run that exits, or fails, between the arguments of a started call ends
// normally; the arguments sent so far are released with the run.
func TestExitInsideAStartedCall(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "arr")
	m.stack = 4
	m.store("arr", m.arrayOf(m.k(1), m.k(2)))
	m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	m.emit(bytecode.OpSendUnpack, 0, m.loc("arr"), 0, 0)
	m.emit(bytecode.OpPush, 0, m.arrayOf(m.k("named")), a.str("extra")+1, 0)
	m.echoK("before exit")
	m.exit()
	g := a.fn("g", 0, 2, "rest").parameters("...rest")
	g.retNull()
	res := runAsm(t, a)
	if res.End != Exited || res.Err != nil || string(res.Output) != "before exit" {
		t.Fatalf("End %v, err %v, output %q", res.End, res.Err, res.Output)
	}
}

// A function called by name receives the value of a reference sent to a
// by-value parameter it records, and Return yields the value of a
// reference: no reference reaches a register that takes values.
func TestReferencesStayOutOfValueParameters(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x", "y")
	withoutLiveness(m)
	m.store("x", m.k("x"))
	r := m.reg(bytecode.Value)
	m.emit(bytecode.OpMakeRef, 0, r, m.loc("x"), 0)
	m.emit(bytecode.OpPush, 0, r, 0, 0)
	got := m.reg(bytecode.Value)
	m.emit(bytecode.OpCallValue, 0, got, m.k("id"), 1)
	// $y = id($x) by "reference"; writing $y must not write $x.
	m.store("y", got)
	m.store("y", m.bin(bytecode.Concat, m.load("y"), m.k("+")))
	m.echo(m.load("x"))
	m.echo(m.load("y"))
	m.retNull()
	// function id($p) { return <the parameter register itself>; }
	id := a.fn("id", 1, 2, "p").parameters("p")
	id.emit(bytecode.OpReturn, 0, 0, 0, 0)
	res := runAsm(t, a)
	if res.Err != nil || string(res.Output) != "xx+" {
		t.Fatalf("output %q, err %v; want \"xx+\"", res.Output, res.Err)
	}
}

// Unpacking a large array polls for cancellation, and the step budget
// stops a loop of calls with unpacked arguments.
func TestUnpackLargeArray(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "arr", "i")
	m.stack = 4
	m.store("arr", m.newArray())
	m.store("i", m.k(0))
	top := m.pc()
	m.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	m.push("arr", m.load("i"))
	m.incdec(bytecode.PreInc, "i")
	br := m.branch(m.bin(bytecode.Less, m.load("i"), m.k(200000)))
	m.patch(br, top, m.pc())
	m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	m.emit(bytecode.OpSendUnpack, 0, m.loc("arr"), 0, 0)
	res := m.reg(bytecode.Value)
	m.emit(bytecode.OpCallPending, 0, res, 0, 0)
	m.echo(res)
	m.retNull()
	// function last(...$rest) { return $rest[199999]; }
	g := a.fn("last", 0, 2, "rest").parameters("...rest")
	rv := g.reg(bytecode.Value)
	g.emit(bytecode.OpRecvVariadic, 0, rv, 0, 0)
	g.emit(bytecode.OpBind, uint16(bytecode.BindValue), g.loc("rest"), rv, 0)
	g.ret(g.get(g.load("rest"), g.k(199999), false))
	out := runAsm(t, a)
	if out.Err != nil || string(out.Output) != "199999" {
		t.Fatalf("output %q, err %v", out.Output, out.Err)
	}
}

// A by-value variadic parameter holds values even when hand-written
// bytecode sends it references: positional extras and a collected name
// through a started call, Locations through a call by name, and a MakeRef
// register. Writing the parameter's array must not write the caller's
// variables. Before the fix the three programs printed W0W1WN, W0W1 and W0.
func TestByValueVariadicReceivesValues(t *testing.T) {
	// function vw(...$rest) { $rest[0] = "W0"; $rest[1] = "W1"; $rest["n"] = "WN"; }
	writer := func(a *vmAsm) {
		v := a.fn("vw", 0, 5, "rest").parameters("...rest")
		r := v.reg(bytecode.Value)
		v.emit(bytecode.OpRecvVariadic, 0, r, 0, 0)
		v.emit(bytecode.OpBind, uint16(bytecode.BindValue), v.loc("rest"), r, 0)
		v.set("rest", v.k(0), v.k("W0"))
		v.set("rest", v.k(1), v.k("W1"))
		v.set("rest", v.k("n"), v.k("WN"))
		v.retNull()
	}
	cases := map[string]struct {
		send func(m *vmFn)
		want string
	}{
		"started call": {func(m *vmFn) {
			m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
			m.emit(bytecode.OpPush, 0, m.loc("x"), 0, 0)
			m.emit(bytecode.OpPush, 0, m.loc("y"), 0, 0)
			m.emit(bytecode.OpPush, 0, m.loc("z"), m.a.str("n")+1, 0)
			m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
		}, "xyz"},
		"call by name": {func(m *vmFn) {
			m.emit(bytecode.OpPush, 0, m.loc("x"), 0, 0)
			m.emit(bytecode.OpPush, 0, m.loc("y"), 0, 0)
			m.emit(bytecode.OpCallValue, 0, m.reg(bytecode.Value), m.k("vw"), 2)
		}, "xyz"},
		"reference in a register": {func(m *vmFn) {
			r := m.reg(bytecode.Value)
			m.emit(bytecode.OpMakeRef, 0, r, m.loc("x"), 0)
			m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
			m.emit(bytecode.OpPush, 0, r, 0, 0)
			m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
		}, "xyz"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 1, "x", "y", "z")
			m.stack = 8
			for _, v := range []string{"x", "y", "z"} {
				m.store(v, m.k(v))
			}
			tc.send(m)
			for _, v := range []string{"x", "y", "z"} {
				m.echo(m.load(v))
			}
			m.retNull()
			writer(a)
			res := runAsm(t, a)
			if res.End != Completed || res.Err != nil || string(res.Output) != tc.want {
				t.Fatalf("End %v, err %v, output %q; want %q", res.End, res.Err, res.Output, tc.want)
			}
		})
	}
}

// A by-reference variadic parameter still receives the references.
func TestByReferenceVariadicReceivesReferences(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "x", "y")
	m.stack = 8
	m.store("x", m.k("x"))
	m.store("y", m.k("y"))
	m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	m.emit(bytecode.OpPush, 0, m.loc("x"), 0, 0)
	m.emit(bytecode.OpPush, 0, m.loc("y"), a.str("n")+1, 0)
	m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
	m.echo(m.load("x"))
	m.echo(m.load("y"))
	m.retNull()
	v := a.fn("vw", 0, 5, "rest").parameters("...&rest")
	r := v.reg(bytecode.Value)
	v.emit(bytecode.OpRecvVariadic, 0, r, 0, 0)
	v.emit(bytecode.OpBind, uint16(bytecode.BindValue), v.loc("rest"), r, 0)
	v.set("rest", v.k(0), v.k("W0"))
	v.set("rest", v.k("n"), v.k("WN"))
	v.retNull()
	if res := runAsm(t, a); res.Err != nil || string(res.Output) != "W0WN" {
		t.Fatalf("output %q, err %v; want \"W0WN\"", res.Output, res.Err)
	}
}

// A named argument a variadic parameter collects leaves no pending
// argument, so a call by name after it would take an argument of the
// caller's own started call. The verifier refuses the image; before the
// fix it was accepted and the callee printed the caller's argument.
func TestNamedArgumentIsNotTakenByACallByName(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1)
	m.stack = 8
	m.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	m.emit(bytecode.OpPush, 0, m.k("MAIN-ARG"), 0, 0)
	m.emit(bytecode.OpPush, 0, m.call(2), 0, 0)
	m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
	m.retNull()
	v := a.fn("v", 0, 2, "rest").parameters("...rest")
	v.retNull()
	// function g() { InitCall v; Push 1, name "zz"; CallValue "w", 1 }
	g := a.fn("g", 0, 3)
	g.stack = 4
	g.emit(bytecode.OpInitCall, 0, 0, 1, 0)
	g.emit(bytecode.OpPush, 0, g.k(1), a.str("zz")+1, 0)
	g.emit(bytecode.OpCallValue, 0, g.reg(bytecode.Value), g.k("w"), 1)
	g.emit(bytecode.OpCallPending, 0, g.reg(bytecode.Value), 0, 0)
	g.retNull()
	w := a.fn("w", 1, 4, "p").parameters("p")
	w.echo(w.load("p"))
	w.retNull()
	_, err := bytecode.Build(context.Background(), a.image(), bytecode.Options{Limits: bytecode.DefaultLimits(), Supported: bytecode.AllCapabilities})
	var be *bytecode.Error
	if !errors.Is(err, bytecode.ErrInvalid) || !errors.As(err, &be) || be.Function != 2 || !strings.Contains(be.Detail, "named argument") {
		t.Fatalf("Build = %v, want ErrInvalid for the named argument in function 2", err)
	}
}
