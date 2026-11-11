package runtime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// The W-17 reference programs (tests/vm/w17) record PHP 8.5.12's stdout,
// stderr and exit code in display, log and both modes. These tests compile a
// selection of them by hand, following the compiler conventions, and require
// the same bytes. The root package will run all of them once the compiler
// exists.

type w17Mode struct {
	Stdout string `json:"stdout_b64"`
	Stderr string `json:"stderr_b64"`
	Exit   int    `json:"exit_code"`
}

type w17Case struct {
	Case    string  `json:"case"`
	Display w17Mode `json:"display"`
	Log     w17Mode `json:"log"`
	Both    w17Mode `json:"both"`
	Return  *string `json:"return_json"`
}

func loadW17Reference(t *testing.T, file string) map[string]w17Case {
	t.Helper()
	raw, err := os.ReadFile("../../tests/vm/w17/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []w17Case `json:"cases"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	out := map[string]w17Case{}
	for _, c := range ref.Cases {
		out[c.Case] = c
	}
	return out
}

func b64(t *testing.T, s string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// renderLog is how the CLI writes the log channel to stderr with error_log
// unset: "PHP Level:  message in file on line N".
func renderLog(ds []RunDiagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "PHP %s:  %s in %s on line %d\n", d.Level, d.Message, d.File, d.Line)
	}
	return b.String()
}

// checkW17 runs build (a fresh image per mode) in the three modes and
// compares every byte with the reference.
func checkW17(t *testing.T, ref map[string]w17Case, name string, width int, build func(a *vmAsm)) {
	t.Helper()
	c, ok := ref[name]
	if !ok {
		t.Fatalf("no reference case %s", name)
	}
	for _, mode := range []struct {
		name         string
		display, log bool
		want         w17Mode
	}{{"display", true, false, c.Display}, {"log", false, true, c.Log}, {"both", true, true, c.Both}} {
		a := newAsm(t, width)
		a.file = "/w/cases/" + name + ".php"
		build(a)
		cfg := vmConfig(t, width)
		cfg.DisplayErrors, cfg.LogErrors = mode.display, mode.log
		res := a.program().Run(cfg)
		if res.End == Internal || res.End == Unsupported {
			t.Fatalf("%s/%s: %v %v", name, mode.name, res.End, res.Err)
		}
		if got, want := string(res.Output), b64(t, mode.want.Stdout); got != want {
			t.Errorf("%s/%s stdout\n got: %q\nwant: %q", name, mode.name, got, want)
		}
		if got, want := renderLog(res.Diagnostics), b64(t, mode.want.Stderr); got != want {
			t.Errorf("%s/%s stderr\n got: %q\nwant: %q", name, mode.name, got, want)
		}
		if res.ExitCode != mode.want.Exit {
			t.Errorf("%s/%s exit code %d, want %d (%v %v)", name, mode.name, res.ExitCode, mode.want.Exit, res.End, res.Err)
		}
	}
}

func TestVMReferencePrograms(t *testing.T) {
	for _, width := range []int{64, 32} {
		file := "reference-64.json"
		if width == 32 {
			file = "reference-32.json"
		}
		ref := loadW17Reference(t, file)
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for name, build := range w17Programs {
				t.Run(name, func(t *testing.T) { checkW17(t, ref, name, width, build) })
			}
		})
	}
}

// w17Programs are hand compilations of reference cases.
var w17Programs = map[string]func(a *vmAsm){
	// echo "before\n"; $a = 10; $b = 0; echo $a / $b, "\n"; echo "after\n";
	"fatal_division_by_zero": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "a", "b")
		m.at(2).echoK("before\n")
		m.at(3).store("a", m.k(10))
		m.at(4).store("b", m.k(0))
		m.at(5).echo(m.bin(bytecode.Divide, m.load("a"), m.load("b")))
		m.echoK("\n")
		m.at(6).echoK("after\n")
		m.at(7).retNull()
	},
	"fatal_too_few_arguments": func(a *vmAsm) {
		m := a.fn("{main}", 0, 6)
		m.at(6).echo(m.call(1, m.k("x"), m.k("y")))
		m.echoK("\n")
		m.at(7).echo(m.callName("pair", m.k("only")))
		m.echoK("\n")
		m.at(8).retNull()
		p := a.fn("pair", 2, 2, "a", "b")
		p.at(4).ret(p.bin(bytecode.Concat, p.load("a"), p.load("b")))
	},
	"fatal_uncaught_stack_trace": func(a *vmAsm) {
		m := a.fn("{main}", 0, 14)
		m.at(14).echoK("calling\n")
		m.at(15).echo(m.call(3, m.k(1), m.k(2.5)))
		m.echoK("\n")
		m.at(16).retNull()
		l3 := a.fn("level3", 2, 2, "n", "s")
		l3.at(4).ret(l3.bin(bytecode.Modulo, l3.load("n"), l3.k(0)))
		l2 := a.fn("level2", 1, 6, "n")
		l2.at(8).ret(l2.call(1, l2.load("n"), l2.k("text arg")))
		l1 := a.fn("level1", 2, 10, "a", "b")
		l1.at(12).ret(l1.call(2, l1.bin(bytecode.Add, l1.load("a"), l1.load("b"))))
	},
	"fatal_undefined_function": func(a *vmAsm) {
		m := a.fn("{main}", 0, 6)
		m.at(6).echo(m.call(1))
		m.echoK("\n")
		// PHP resolves the name before evaluating arguments; the compiler
		// emits no arguments for a name the unit cannot resolve.
		m.at(7).echo(m.callName("missing_function"))
		m.echoK("\n")
		m.at(8).retNull()
		e := a.fn("exists", 0, 2)
		e.at(4).ret(e.k("ok"))
	},
	"warn_offset_on_scalar": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "n", "i", "b", "f", "undefinedVar", "arr")
		line := func(l uint32, name string, key any) {
			m.at(l).echo(m.get(m.load(name), m.k(key), false))
			m.echoK("|\n")
		}
		m.at(2).store("n", m.k(nil))
		line(3, "n", "a")
		m.at(4).store("i", m.k(5))
		line(5, "i", 0)
		m.at(6).store("b", m.k(true))
		line(7, "b", "k")
		m.at(8).store("f", m.k(1.5))
		line(9, "f", 1)
		line(10, "undefinedVar", "k")
		m.at(11)
		tmp := m.hidden()
		m.emit(bytecode.OpStore, 0, tmp, m.newArray(), 0)
		m.emit(bytecode.OpArraySet, 0, tmp, m.k("in"), m.k(nil))
		arr := m.reg(bytecode.Value)
		m.emit(bytecode.OpLoad, 0, arr, tmp, 0)
		m.emit(bytecode.OpUnset, 0, tmp, 0, 0)
		m.store("arr", arr)
		m.at(12).echo(m.get(m.get(m.load("arr"), m.k("in"), false), m.k("deep"), false))
		m.echoK("|\n")
		m.at(13).retNull()
	},
	"array_write_null_false": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "n", "u", "v", "e", "f", "g")
		echoGet := func(l uint32, name string, key any) {
			m.at(l).echo(m.get(m.load(name), m.k(key), false))
			m.echoK("\n")
		}
		m.at(2).store("n", m.k(nil))
		m.at(3).set("n", m.k("a"), m.k(1))
		echoGet(4, "n", "a")
		m.at(5).push("u", m.k("created"))
		echoGet(6, "u", 0)
		m.at(7).set("v", m.k("k"), m.k("also created"))
		echoGet(8, "v", "k")
		m.at(9).store("e", m.arrayOf())
		m.at(10).set("e", m.k("x"), m.arrayOf(m.k(1)))
		m.at(11).echo(m.get(m.get(m.load("e"), m.k("x"), false), m.k(0), false))
		m.echoK("\n")
		m.at(12).store("f", m.k(false))
		m.at(13).push("f", m.k("x"))
		echoGet(14, "f", 0)
		m.at(15).store("g", m.k(false))
		m.at(16).set("g", m.k("k"), m.k("y"))
		echoGet(17, "g", "k")
		m.at(18).retNull()
	},
	"fatal_array_append_full": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "a")
		m.at(2)
		tmp := m.hidden()
		m.emit(bytecode.OpStore, 0, tmp, m.newArray(), 0)
		// On 32 bits the literal 9223372036854775807 is a float.
		var key any = int64(math64Max)
		if a.width == 32 {
			key = float64(math64Max)
		}
		m.emit(bytecode.OpArraySet, 0, tmp, m.k(key), m.k("max"))
		arr := m.reg(bytecode.Value)
		m.emit(bytecode.OpLoad, 0, arr, tmp, 0)
		m.store("a", arr)
		m.at(3).echoK("full\n")
		m.at(4).push("a", m.k("overflow"))
		m.at(5).echoK("never\n")
		m.at(6).retNull()
	},
	"exit_status_large": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.at(2).echoK("256\n")
		m.at(3).exitWith(m.bin(bytecode.Add, m.k(256), m.k(4)))
	},
	"exit_in_function": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.at(12).call(1, m.k(2))
		m.at(13).echoK("never\n")
		m.at(14).retNull()
		d := a.fn("deep", 1, 2, "n")
		br := d.at(4).branch(d.bin(bytecode.Equal, d.load("n"), d.k(0)))
		then := d.pc()
		d.at(5).echoK("exiting\n")
		d.at(6).exitWith(d.k(5))
		els := d.pc()
		d.patch(br, then, els)
		d.at(8).echoK("level")
		d.echo(d.load("n"))
		d.echoK("\n")
		d.at(9).call(1, d.bin(bytecode.Subtract, d.load("n"), d.k(1)))
		d.at(10).echoK("never returns\n")
		d.at(11).retNull()
	},
	"die_string": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.at(2).exitWith(m.k("message from die"))
	},
	"exit_plain": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2)
		m.at(2).echoK("a")
		m.at(3).exit()
	},
	"exit_after_warning": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "undefined")
		m.at(2).echo(m.load("undefined"))
		m.at(3).echoK("x\n")
		m.at(4).exitWith(m.k(2))
	},
	"return_array": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "a", "")
		m.at(2)
		inner := m.arrayOf(m.k(true), m.k(nil), m.k(1.5))
		tmp := m.hidden()
		m.emit(bytecode.OpStore, 0, tmp, m.newArray(), 0)
		m.emit(bytecode.OpArrayAppend, 0, tmp, m.k(1), 0)
		m.emit(bytecode.OpArraySet, 0, tmp, m.k("k"), inner)
		m.emit(bytecode.OpArraySet, 0, tmp, m.k(7), m.k("s"))
		arr := m.reg(bytecode.Value)
		m.emit(bytecode.OpLoad, 0, arr, tmp, 0)
		m.store("a", arr)
		m.at(3).echoK("returning\n")
		m.at(4).ret(m.load("a"))
	},
	"fatal_array_unary_minus": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "a")
		m.at(2).store("a", m.arrayOf())
		m.at(3).echoK("neg\n")
		m.at(4).echo(m.unary(bytecode.Negate, m.load("a")))
		m.at(5).retNull()
	},
	"fatal_array_append_string": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "s")
		m.at(2).store("s", m.k("abc"))
		m.at(3).echoK("append\n")
		m.at(4).push("s", m.k("d"))
		m.at(5).retNull()
	},
	"warn_undefined_variable": func(a *vmAsm) {
		m := a.fn("{main}", 0, 2, "nope", "x", "missing", "alsoMissing", "r", "global")
		m.at(2).echo(m.load("nope"))
		m.echoK("|\n")
		m.at(3).store("x", m.bin(bytecode.Add, m.load("missing"), m.k(1)))
		m.at(4).echo(m.load("x"))
		m.echoK("\n")
		m.at(5).echo(m.bin(bytecode.Concat, m.bin(bytecode.Concat, m.k("in string ["), m.load("alsoMissing")), m.k("]\n")))
		m.at(10).store("r", m.call(1))
		m.at(11)
		cond := m.bin(bytecode.Identical, m.load("r"), m.k(nil))
		res := m.reg(bytecode.Value)
		br := m.branch(cond)
		t1 := m.pc()
		m.emit(bytecode.OpMove, 0, res, m.k("null returned"), 0)
		j := m.jump(0)
		t2 := m.pc()
		m.emit(bytecode.OpMove, 0, res, m.k("other"), 0)
		m.patch(br, t1, t2)
		m.patch(j, m.pc(), 0)
		m.echo(res)
		m.echoK("\n")
		m.at(12).store("global", m.k(5))
		m.at(17).call(2)
		m.at(18).retNull()
		in := a.fn("inner", 0, 6, "local")
		in.at(8).ret(in.load("local"))
		ng := a.fn("noGlobals", 0, 13, "global")
		ng.at(15).echo(ng.load("global"))
		ng.echoK("|\n")
		ng.at(16).retNull()
	},
}

const math64Max = 9223372036854775807
