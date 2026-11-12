package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// The root package names the budget that ended a run from the LimitError
// cause, so each limit must report its own resource.
func TestVMLimitErrorsNameTheResource(t *testing.T) {
	recursion := func() *vmAsm {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2)
		m.call(1, m.k(0))
		m.retNull()
		r := a.fn("r", 1, 3, "n")
		r.call(1, r.bin(bytecode.Add, r.load("n"), r.k(1)))
		r.retNull()
		return a
	}
	// warnings raises n warnings, each followed by a call when calls is set.
	warnings := func(n int, calls bool) *vmAsm {
		a := newAsm(t, 64)
		m := a.fn("{main}", 0, 2, "u")
		for i := 0; i < n; i++ {
			m.echo(m.load("u"))
			if calls {
				m.call(1)
			}
		}
		m.retNull()
		f := a.fn("f", 0, 3)
		f.retNull()
		return a
	}
	for _, tc := range []struct {
		name string
		a    *vmAsm
		cfg  func(*RunConfig)
		want LimitError
	}{
		{"frames", recursion(), func(c *RunConfig) { c.Depth = 10 }, LimitError{"frames", 10, 10, 1}},
		{"calls", recursion(), func(c *RunConfig) { c.Callbacks = 5 }, LimitError{"calls", 5, 5, 1}},
		{"output", echoLoopAsm(t), func(c *RunConfig) { c.OutputBytes = 10 }, LimitError{"output", 10, 10, 1}},
		{"diagnostics share calls", warnings(5, false), func(c *RunConfig) { c.Callbacks = 3 }, LimitError{"diagnostics", 3, 3, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := vmConfig(t, 64)
			cfg.DisplayErrors = false
			tc.cfg(&cfg)
			res := tc.a.program().Run(cfg)
			var le *LimitError
			if res.End != LimitExceeded || !errors.As(res.Err, &le) || *le != tc.want || !errors.Is(res.Err, errExecutionLimit) {
				t.Fatalf("%v %#v", res.End, res.Err)
			}
		})
	}
	// With its own budget, diagnostics no longer share the call budget.
	cfg := vmConfig(t, 64)
	cfg.Callbacks, cfg.Diagnostics = 2, 10
	if res := warnings(5, false).program().Run(cfg); res.End != Completed || len(res.Diagnostics) != 5 {
		t.Fatalf("%v %v %v", res.End, res.Err, res.Diagnostics)
	}
	// That budget bounds only the record: the run goes on past it, and
	// every diagnostic is still displayed.
	cfg = vmConfig(t, 64)
	cfg.Diagnostics, cfg.DisplayErrors, cfg.LogErrors = 2, true, true
	res := warnings(5, true).program().Run(cfg)
	if res.End != Completed || len(res.Diagnostics) != 2 || strings.Count(string(res.Output), "Warning: Undefined variable $u") != 5 {
		t.Fatalf("%v %v %d recorded, output %q", res.End, res.Err, len(res.Diagnostics), res.Output)
	}
}

// echoLoopAsm writes "x" until a limit stops it.
func echoLoopAsm(t *testing.T) *vmAsm {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	top := m.pc()
	m.emit(bytecode.OpCheckpoint, 0, 0, 0, 0)
	m.echoK("x")
	m.jump(top)
	m.retNull()
	return a
}

// Compile-time diagnostics are reported before {main}, at their own
// location, through display_errors, log_errors and error_reporting.
func TestVMPreludeIsReportedFirst(t *testing.T) {
	a := newAsm(t, 64)
	m := a.fn("{main}", 0, 2)
	m.echoK("main")
	m.retNull()
	prelude := []RunDiagnostic{{Level: "Warning", Message: "compiled", File: vmTestFile, Line: 5}, {Level: "Deprecated", Message: "old", File: vmTestFile, Line: 6}}
	cfg := vmConfig(t, 64)
	cfg.Prelude = prelude
	res := a.program().Run(cfg)
	want := "\nWarning: compiled in /w/t.php on line 5\n\nDeprecated: old in /w/t.php on line 6\nmain"
	if res.End != Completed || string(res.Output) != want || len(res.Diagnostics) != 2 || res.Diagnostics[0] != prelude[0] || res.Diagnostics[1] != prelude[1] {
		t.Fatalf("%v %q %+v", res.End, res.Output, res.Diagnostics)
	}
	cfg = vmConfig(t, 64)
	cfg.Prelude = prelude
	cfg.ErrorReporting = -1 &^ levelWarning
	res = a.program().Run(cfg)
	if res.End != Completed || !strings.HasPrefix(string(res.Output), "\nDeprecated: old") || len(res.Diagnostics) != 1 {
		t.Fatalf("error_reporting: %v %q %+v", res.End, res.Output, res.Diagnostics)
	}
	// A prelude beyond the diagnostic record bound is still displayed and
	// {main} still runs; only the record stops at the bound.
	cfg = vmConfig(t, 64)
	cfg.Prelude, cfg.Diagnostics = prelude, 1
	res = a.program().Run(cfg)
	if res.End != Completed || string(res.Output) != want || len(res.Diagnostics) != 1 || res.Diagnostics[0] != prelude[0] {
		t.Fatalf("budget: %v %q %+v", res.End, res.Output, res.Diagnostics)
	}
}
