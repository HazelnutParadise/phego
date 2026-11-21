package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 7B: REQ-001, REQ-016, REQ-025, REQ-026. Closures in
// hand-written bytecode: the paths compiled code never takes, and the
// checks the interpreter makes itself whatever the verifier accepted.

// closureFn declares closure function "{closure}" with parameter p, the
// by-value capture v and the by-reference capture r; it returns p . v and
// appends p to r.
func closureFn(a *vmAsm) *vmFn {
	c := a.fn("{closure}", 1, 2, "p", "v", "r").parameters("p")
	c.flags = bytecode.FuncClosure
	c.captures = []bytecode.Capture{{Local: 1}, {Local: 2, Flags: bytecode.CaptureByRef}}
	out := c.reg(bytecode.Value)
	c.emit(bytecode.OpBinary, uint16(bytecode.Concat), out, c.load("p"), c.load("v"))
	sum := c.reg(bytecode.Value)
	c.emit(bytecode.OpBinary, uint16(bytecode.Concat), sum, c.load("r"), c.load("p"))
	c.store("r", sum)
	c.ret(out)
	return c
}

// A closure called by CallValue with positional arguments enters its
// function with its captures: the value captured when it was bound, and
// the variable itself for the capture by reference.
func TestClosureCalledByValue(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "v", "r", "f")
	m.stack = 2
	closureFn(a)
	m.store("v", m.k("a"))
	m.store("r", m.k("<"))
	cl := m.reg(bytecode.Value)
	m.emit(bytecode.OpClosure, 0, cl, 1, 0)
	m.emit(bytecode.OpBindLexical, 0, cl, m.loc("v"), 1)
	m.emit(bytecode.OpBindLexical, 1, cl, m.loc("r"), 1)
	m.store("f", cl)
	m.store("v", m.k("changed"))
	for _, arg := range []string{"1", "2"} {
		m.emit(bytecode.OpPush, 0, m.k(arg), 0, 0)
		res := m.reg(bytecode.Value)
		m.emit(bytecode.OpCallValue, 0, res, m.load("f"), 1)
		m.echo(res)
	}
	m.echo(m.load("r"))
	m.retNull()
	out := runAsm(t, a)
	if out.Err != nil || string(out.Output) != "1a2a<12" {
		t.Fatalf("output %q, err %v; want \"1a2a<12\"", out.Output, out.Err)
	}
}

// Instructions that name a closure check what they are given: a value that
// is not the closure of the named function, a call with arguments turned
// into a closure, and a closure function reached by name all end the run
// without a Go panic and without entering the closure's code.
func TestClosureInstructionsFailClosed(t *testing.T) {
	cases := map[string]struct {
		build func(m *vmFn)
		want  string
	}{
		"bind-lexical on a string": {func(m *vmFn) {
			m.emit(bytecode.OpBindLexical, 0, m.k("x"), m.loc("v"), 1)
		}, "bind-lexical"},
		"bind-lexical on an array": {func(m *vmFn) {
			r := m.reg(bytecode.Value)
			m.emit(bytecode.OpNewArray, 0, r, 0, 0)
			m.emit(bytecode.OpBindLexical, 0, r, m.loc("v"), 1)
		}, "bind-lexical"},
		"bind-lexical on the closure of another function": {func(m *vmFn) {
			r := m.reg(bytecode.Value)
			m.emit(bytecode.OpClosure, 0, r, 2, 0)
			m.emit(bytecode.OpBindLexical, 0, r, m.loc("v"), 1)
		}, "bind-lexical"},
		"a closure function called by its name": {func(m *vmFn) {
			m.emit(bytecode.OpPush, 0, m.k("1"), 0, 0)
			m.emit(bytecode.OpCallValue, 0, m.reg(bytecode.Value), m.k("{closure}"), 1)
		}, "Call to undefined function {closure}()"},
		"a closure function named by a started call": {func(m *vmFn) {
			m.emit(bytecode.OpInitMethod, bytecode.InitCallable, m.k("{closure}"), 0, 0)
			m.emit(bytecode.OpCallPending, 0, m.reg(bytecode.Value), 0, 0)
		}, "Call to undefined function {closure}()"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := newAsm(t, 64)
			m := a.fn("{main}", 0, 1, "v")
			m.stack = 2
			closureFn(a)
			other := a.fn("{closure:other}", 0, 3, "w")
			other.flags = bytecode.FuncClosure
			other.captures = []bytecode.Capture{{Local: 0}}
			other.echoK("entered")
			other.retNull()
			tc.build(m)
			m.retNull()
			res := runAsm(t, a)
			var ie *InternalError
			if strings.Contains(string(res.Output), "entered") {
				t.Fatalf("the closure's code ran: %q", res.Output)
			}
			if tc.want == "bind-lexical" {
				if !errors.As(res.Err, &ie) || !strings.Contains(ie.Detail, tc.want) {
					t.Fatalf("End %v, err %v; want an internal error for bind-lexical", res.End, res.Err)
				}
				return
			}
			if !strings.Contains(string(res.Output)+errorText(res.Err), tc.want) {
				t.Fatalf("End %v, output %q, err %v; want %q", res.End, res.Output, res.Err, tc.want)
			}
		})
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
