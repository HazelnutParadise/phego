package runtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 5: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032. What the
// VM does with constants, @ and match errors in hand-written bytecode: the
// PHP-visible text is compared with the pinned images by the fixtures under
// tests/vm/w17; these tests cover the run-local state and the failure modes
// compiled code never reaches.

// fetchConst emits FetchConst into a fresh register.
func (f *vmFn) fetchConst(name, display string, unqualified bool) uint32 {
	r := f.reg(bytecode.Value)
	var aux uint16
	if unqualified {
		aux = uint16(bytecode.FetchConstUnqualified)
	}
	f.emit(bytecode.OpFetchConst, aux, r, f.a.str(name), f.a.str(display))
	return r
}

func (f *vmFn) declareConst(name string, v uint32) {
	f.emit(bytecode.OpDeclareConst, 0, v, f.a.str(name), 0)
}

// A constant declared by one run is not seen by the next run of the same
// Program, nor by other runs at the same time: no "already defined" warning
// and every run reads its own value.
func TestConstantsBelongToTheRun(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "n")
	m.declareConst("N", m.load("n"))
	m.declareConst("LIST", m.arrayOf(m.load("n"), m.k("x")))
	m.echo(m.fetchConst("N", "N", false))
	m.echoK(":")
	m.echo(m.get(m.fetchConst("LIST", "LIST", false), m.k(1), false))
	m.echoK("\n")
	m.retNull()
	p := a.program()
	run := func(n int64) RunResult {
		cfg := vmConfig(t, 64)
		cfg.Globals = []Global{{Name: "n", Value: n}}
		return p.Run(cfg)
	}
	for i := int64(1); i <= 3; i++ {
		res := run(i)
		if res.End != Completed || string(res.Output) != fmt.Sprintf("%d:x\n", i) || len(res.Diagnostics) != 0 {
			t.Fatalf("run %d: %v %q %v %v", i, res.End, res.Output, res.Diagnostics, res.Err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				n := int64(g*1000 + i)
				if res := run(n); res.End != Completed || string(res.Output) != fmt.Sprintf("%d:x\n", n) || len(res.Diagnostics) != 0 {
					errs <- fmt.Errorf("goroutine %d run %d: %v %q %v", g, i, res.End, res.Output, res.Err)
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

// A constant keeps the array it was declared with: a later write to the
// variable separates the variable, also in a function too large for
// liveness, where the DeclareConst operand register keeps its copy.
func TestConstantArrayIsNotChangedThroughItsSource(t *testing.T) {
	for _, liveness := range []bool{true, false} {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "arr")
		if !liveness {
			withoutLiveness(m)
		}
		m.store("arr", m.arrayOf(m.k("a"), m.k("b")))
		m.declareConst("ARR", m.load("arr"))
		m.emit(bytecode.OpArrayAppend, 0, m.loc("arr"), m.k("c"), 0)
		m.emit(bytecode.OpArraySet, 0, m.loc("arr"), m.k(0), m.k("z"))
		m.echo(m.get(m.fetchConst("ARR", "ARR", false), m.k(0), false))
		m.echo(m.get(m.load("arr"), m.k(0), false))
		m.echo(m.get(m.load("arr"), m.k(2), false))
		m.echo(m.get(m.fetchConst("ARR", "ARR", false), m.k(2), true))
		m.retNull()
		res := runAsm(t, a)
		if res.End != Completed || string(res.Output) != "azc" {
			t.Fatalf("liveness %v: %v %q %v", liveness, res.End, res.Output, res.Err)
		}
	}
}

// Predefined constants a compiled program never fetches at run time: a
// resource and a platform-dependent one end the run as unsupported; a
// deprecated one warns on every read; an unqualified name in a namespace
// falls back to the global predefined constant.
func TestFetchConstPredefined(t *testing.T) {
	for _, tc := range []struct {
		name, display string
		unqualified   bool
		want          string
		unsupported   bool
	}{
		{"STDIN", "STDIN", false, "", true},
		{"CHAR_MAX", "CHAR_MAX", false, "", true},
		{"ns\\CHAR_MAX", "ns\\CHAR_MAX", true, "", true},
		{"ns\\E_ALL", "ns\\E_ALL", true, "30719", false},
		{"ns\\E_STRICT", "ns\\E_STRICT", true, "\nDeprecated: Constant E_STRICT is deprecated since 8.4, the error level was removed in /w/t.php on line 1\n2048", false},
		{"dom\\INDEX_SIZE_ERR", "Dom\\INDEX_SIZE_ERR", false, "1", false},
		{"PHP_INT_SIZE", "PHP_INT_SIZE", false, "8", false},
	} {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "")
		m.echo(m.fetchConst(tc.name, tc.display, tc.unqualified))
		m.retNull()
		res := runAsm(t, a)
		var ue *UnsupportedError
		if tc.unsupported {
			if res.End != Unsupported || !errors.As(res.Err, &ue) || !strings.Contains(ue.Feature, tc.name[strings.LastIndexByte(tc.name, '\\')+1:]) {
				t.Errorf("%s: %v %q %v", tc.name, res.End, res.Output, res.Err)
			}
			continue
		}
		if res.End != Completed || string(res.Output) != tc.want {
			t.Errorf("%s: %v %q, want %q (%v)", tc.name, res.End, res.Output, tc.want, res.Err)
		}
	}
	a := newAsm(t, 32)
	m := a.fn("{main}", 0, 1, "")
	m.echo(m.fetchConst("PHP_INT_MAX", "PHP_INT_MAX", false))
	m.retNull()
	if res := runAsm(t, a); res.End != Completed || string(res.Output) != "2147483647" {
		t.Errorf("32-bit PHP_INT_MAX: %v %q %v", res.End, res.Output, res.Err)
	}
}

// Declaring a name a predefined constant holds, in any kind, only warns.
func TestDeclareConstOverPredefinedWarns(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "")
	for _, name := range []string{"STDIN", "CHAR_MAX", "E_STRICT", "TRUE", "nUlL", "__COMPILER_HALT_OFFSET__", "dom\\INDEX_SIZE_ERR"} {
		m.declareConst(name, m.k(1))
	}
	m.echo(m.fetchConst("dom\\INDEX_SIZE_ERR", "Dom\\INDEX_SIZE_ERR", false))
	m.retNull()
	res := runAsm(t, a)
	if res.End != Completed || len(res.Diagnostics) != 7 || string(res.Output) == "" || res.Output[len(res.Output)-1] != '1' {
		t.Fatalf("%v %q %d diagnostics %v", res.End, res.Output, len(res.Diagnostics), res.Err)
	}
	for i, name := range []string{"STDIN", "CHAR_MAX", "E_STRICT", "TRUE", "nUlL", "__COMPILER_HALT_OFFSET__", "dom\\INDEX_SIZE_ERR"} {
		if want := "Constant " + name + " already defined, this will be an error in PHP 9"; res.Diagnostics[i].Message != want || res.Diagnostics[i].Level != "Warning" {
			t.Errorf("diagnostic %d = %+v, want %q", i, res.Diagnostics[i], want)
		}
	}
}

// error_reporting lowered by BeginSilence comes back at EndSilence, also
// for a nested pair, and a run starts from its configuration: a second run
// of a program that ends inside @ is not silenced.
func TestSilenceRestoresAndIsPerRun(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "u")
	outer := m.reg(bytecode.Value)
	m.emit(bytecode.OpBeginSilence, 0, outer, 0, 0)
	m.echo(m.load("u")) // silenced
	inner := m.reg(bytecode.Value)
	m.emit(bytecode.OpBeginSilence, 0, inner, 0, 0)
	m.echo(m.load("u")) // silenced
	m.emit(bytecode.OpEndSilence, 0, inner, 0, 0)
	m.echo(m.load("u")) // still silenced: the inner end restores nothing
	m.emit(bytecode.OpEndSilence, 0, outer, 0, 0)
	m.echo(m.load("u")) // reported
	m.echoK("|")
	m.emit(bytecode.OpBeginSilence, 0, outer, 0, 0)
	m.echo(m.load("u")) // silenced, and the run ends inside @
	m.retNull()
	p := a.program()
	for i := 0; i < 2; i++ {
		res := p.Run(vmConfig(t, 64))
		want := "\nWarning: Undefined variable $u in /w/t.php on line 1\n|"
		if res.End != Completed || string(res.Output) != want || len(res.Diagnostics) != 1 {
			t.Fatalf("run %d: %v %q %v %v", i, res.End, res.Output, res.Diagnostics, res.Err)
		}
	}
	// A reporting mask with only fatal levels is left alone by both ends.
	cfg := vmConfig(t, 64)
	cfg.ErrorReporting = 1
	if res := p.Run(cfg); res.End != Completed || string(res.Output) != "|" || len(res.Diagnostics) != 0 {
		t.Fatalf("fatal-only: %v %q %v", res.End, res.Output, res.Diagnostics)
	}
}

// EndSilence on a register BeginSilence did not write is an engine fault,
// never a PHP error.
func TestEndSilenceNeedsASavedMask(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "")
	m.emit(bytecode.OpEndSilence, 0, m.k("x"), 0, 0)
	m.retNull()
	res := runAsm(t, a)
	var ie *InternalError
	if res.End != Internal || !errors.As(res.Err, &ie) {
		t.Fatalf("%v %v", res.End, res.Err)
	}
}

// MatchError describes the subject as PHP does for the kinds compiled code
// can produce, including an array.
func TestMatchErrorMessages(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{int64(5), "5"},
		{"a long string value here", "'a long string v...'"},
		{2.0, "2.0"},
		{true, "true"},
		{nil, "NULL"},
	} {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 1, "")
		m.emit(bytecode.OpMatchError, 0, m.k(tc.value), 0, 0)
		res := runAsm(t, a)
		if res.End != UncaughtThrowable || res.Throwable == nil || res.Throwable.Class != "UnhandledMatchError" || res.Throwable.Message != "Unhandled match case "+tc.want {
			t.Errorf("%v: %v %+v %v", tc.value, res.End, res.Throwable, res.Err)
		}
	}
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 1, "")
	m.emit(bytecode.OpMatchError, 0, m.arrayOf(m.k(1)), 0, 0)
	res := runAsm(t, a)
	if res.End != UncaughtThrowable || res.Throwable == nil || res.Throwable.Message != "Unhandled match case of type array" {
		t.Fatalf("array: %v %+v %v", res.End, res.Throwable, res.Err)
	}
}
