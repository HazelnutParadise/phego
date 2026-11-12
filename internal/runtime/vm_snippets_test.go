package runtime

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// Hand compilations of testdata/vm-oracle/snippets/*.php: backtrace argument
// rendering, CallValue errors and exit forms. PHP's stdout in display mode
// and its exit code are in snippets-{64,32}.json.
var vmSnippets = map[string]func(a *vmAsm){
	"trace_args": func(a *vmAsm) {
		m := a.fn("{main}", 0, 8)
		args := []uint32{m.k(1.0), m.k("a\nlong string with more than fifteen"), m.arrayOf(m.k(1)), m.k(nil), m.k(true), m.k(1e100), m.k(math.Copysign(0, -1)),
			m.k("x\\y\x00z"), m.k(7), m.k("exactly15chars!"), m.k("sixteen chars!!!"), m.k(math.Inf(1)), m.k(math.Inf(-1)), m.k(2.5), m.k(100.0), m.k(0.1),
			m.k("\t\r\f\v\x1b\x7f\x80")}
		m.callName("g", args...)
		m.at(9).retNull()
		g := a.fn("g", 3, 2, "a", "b", "c")
		g.at(4).store("a", g.k("changed"))
		g.at(5).unset("b")
		g.at(6).ret(g.bin(bytecode.Modulo, g.k(1), g.k(0)))
	},
	"trace_nested_too_few": func(a *vmAsm) {
		m := a.fn("{main}", 0, 10)
		m.at(10).echoK("start\n")
		m.at(11).call(2, m.k(41))
		m.at(12).retNull()
		in := a.fn("inner", 3, 2, "a", "b", "c")
		in.at(4).ret(in.load("a"))
		out := a.fn("outer", 1, 6, "x")
		out.at(8).ret(out.callName("inner", out.load("x"), out.k("two")))
	},
	"trace_undefined_in_function": func(a *vmAsm) {
		m := a.fn("{main}", 0, 7)
		m.at(7).call(1, m.arrayOf(m.k(1), m.k(2)))
		m.at(8).retNull()
		c := a.fn("caller", 1, 2, "v")
		c.at(4).echoK("in caller\n")
		c.at(5).ret(c.callName("Missing_Thing"))
	},
	"callvalue_extra_args": func(a *vmAsm) {
		m := a.fn("{main}", 0, 7)
		m.at(7).callName("one", m.k(5), m.k("extra"), m.arrayOf(m.k(1)), m.k(nil))
		m.at(8).retNull()
		o := a.fn("One", 1, 2, "a")
		o.at(4).echo(o.load("a"))
		o.echoK("\n")
		o.at(5).ret(o.bin(bytecode.Modulo, o.load("a"), o.k(0)))
	},
	"callvalue_case": func(a *vmAsm) {
		m := a.fn("{main}", 0, 6)
		m.at(6).echo(m.callName("PAIR", m.k("a"), m.k("b"), m.k("c")))
		m.echoK("\n")
		m.at(7).echo(m.callName("\\pair", m.k("c"), m.k("d")))
		m.echoK("\n")
		m.at(8).retNull()
		p := a.fn("Pair", 2, 2, "a", "b")
		p.at(4).ret(p.bin(bytecode.Concat, p.load("a"), p.load("b")))
	},
	"warning_in_function_then_trace": func(a *vmAsm) {
		m := a.fn("{main}", 0, 7)
		m.at(7).echoK("x")
		m.at(8).call(1, m.k(3))
		m.at(9).retNull()
		f := a.fn("f", 1, 2, "n", "undefined")
		f.at(4).echo(f.load("undefined"))
		f.at(5).ret(f.bin(bytecode.Divide, f.load("n"), f.k(0)))
	},
}

func exitSnippet(status func(m *vmFn) uint32) func(a *vmAsm) {
	return func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.at(2).echoK("a")
		m.at(3).exitWith(status(m))
	}
}

func init() {
	for name, v := range map[string]any{"exit_float": 1.5, "exit_true": true, "exit_false": false, "exit_null": nil, "exit_negative": -1,
		"exit_integral_float": 2.0, "exit_empty_string": "", "exit_big": 300, "exit_int_string": "07", "exit_neg_float": -3.7} {
		vmSnippets[name] = exitSnippet(func(m *vmFn) uint32 { return m.k(v) })
	}
	vmSnippets["exit_array"] = exitSnippet(func(m *vmFn) uint32 { return m.arrayOf(m.k(1)) })
}

func TestVMOracleSnippets(t *testing.T) {
	for _, width := range []int{64, 32} {
		raw, err := os.ReadFile(fmt.Sprintf("testdata/vm-oracle/snippets-%d.json", width))
		if err != nil {
			t.Fatal(err)
		}
		var ref struct {
			Snippets map[string]w17Mode `json:"snippets"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil {
			t.Fatal(err)
		}
		for name, want := range ref.Snippets {
			build, ok := vmSnippets[name]
			if !ok {
				t.Errorf("snippet %s has no hand compilation", name)
				continue
			}
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				a := newAsm(t, width)
				a.file = "/w/snippets/" + name + ".php"
				build(a)
				cfg := vmConfig(t, width)
				cfg.LogErrors = false
				res := a.program().Run(cfg)
				if got, w := string(res.Output), b64(t, want.Stdout); got != w {
					t.Errorf("stdout\n got: %q\nwant: %q", got, w)
				}
				if res.ExitCode != want.Exit {
					t.Errorf("exit code %d, want %d (%v %v)", res.ExitCode, want.Exit, res.End, res.Err)
				}
			})
		}
	}
}
