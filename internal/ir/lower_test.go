package ir

import (
	"errors"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

func lower(t *testing.T, src string) *Unit {
	t.Helper()
	u, err := lowerSource(src, 64)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return u
}

func lowerSource(src string, width int) (*Unit, error) {
	prof, err := profile.Resolve("8.5.12", width, false)
	if err != nil {
		return nil, err
	}
	tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: width, MaxBytes: len(src) + 1})
	if err != nil {
		return nil, err
	}
	return Lower(tree, prof, Options{File: "/app/t.php"})
}

// firstExpr returns the expression of the first expression statement.
func firstExpr(t *testing.T, u *Unit) *Node {
	t.Helper()
	for _, s := range u.Main.Body {
		if s.Op == OpExpr {
			return s.Kids[0]
		}
	}
	t.Fatal("no expression statement")
	return nil
}

func TestEffectsAreNarrowedOnlyWhenOperandsAllowIt(t *testing.T) {
	cases := []struct {
		src        string
		want, deny Effect
	}{
		{"1 + 2;", 0, MayWarn | MayThrow | MayCallUser},
		{"'a' . 'b';", 0, MayWarn | MayThrow | MayCallUser},
		{"$a === $b;", MayWarn, MayThrow | MayCallUser}, // only reading undefined variables can warn
		{"!$a;", MayWarn, MayThrow | MayCallUser},
		{"isset($a, $b[1]);", MayCallUser, MayWarn}, // offsetExists may run; undefined reads stay quiet
		{"$a ?? 1;", 0, MayWarn},
		{"1 / 0;", MayThrow, MayWarn},
		{"1 << -1;", MayThrow, 0},
		{"10 / 2;", 0, MayThrow},
		{"$a + $b;", MayWarn | MayThrow, MayCallUser},
		{"$a . $b;", MayCallUser, 0},              // __toString
		{"$a == 'x';", MayCallUser | MayThrow, 0}, // __toString in loose comparison
		{"$o->p;", MayCallUser | MayThrow, 0},     // __get, hooks
		{"$o->p = 1;", MayCallUser | MayWrite, 0}, // __set, destructor
		{"$x = 1;", MayWrite | MayCallUser, 0},    // the old value's destructor
		{"f();", Anything, 0},
		{"new A;", Anything, 0},
		{"A::C;", MayCallUser | MayThrow, 0}, // autoload
		{"include 'x.php';", MayCompile | MayDynamicScope, 0},
		{"match (1) { 1 => 2 };", MayThrow, 0},
		{"match (1) { default => 2 };", 0, MayThrow},
		{"(int) 1.5;", MayWarn, MayThrow},
		{"(string) 1;", 0, MayCallUser},
		{"(string) $o;", MayCallUser, 0},
		{"true;", 0, MayThrow},
		{"FOO;", MayThrow, 0},
	}
	for _, c := range cases {
		u := lower(t, "<?php "+c.src)
		e := firstExpr(t, u).Effects
		if e&c.want != c.want || e&c.deny != 0 {
			t.Errorf("%s: effects {%s}, want all of {%s} and none of {%s}", c.src, e, c.want, c.deny)
		}
	}
}

func TestNamesResolveAsPHP(t *testing.T) {
	u := lower(t, `<?php
namespace App;
use Lib\Base, Lib\Sub as S;
use function Lib\fn1;
use const Lib\C1;
strlen('x');
fn1();
\strlen('x');
S\inner();
C1;
OTHER;
true;
new Base;
new S\Deep;
new namespace\Local;
new \Root;
`)
	var got []string
	for _, s := range u.Main.Body {
		n := s.Kids[0]
		switch n.Op {
		case OpCall, OpConst, OpNew:
			got = append(got, string(n.Op)+":"+n.Name.Resolved+"|"+n.Name.Fallback)
		case OpLiteral:
			got = append(got, "literal")
		}
	}
	want := []string{
		"call:App\\strlen|strlen", "call:Lib\\fn1|", "call:strlen|", "call:Lib\\Sub\\inner|",
		"const:Lib\\C1|", "const:App\\OTHER|OTHER", "literal",
		"new:Lib\\Base|", "new:Lib\\Sub\\Deep|", "new:App\\Local|", "new:Root|",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestAliasedVariablesAreMarked(t *testing.T) {
	u := lower(t, `<?php
function f($p, &$r) {
	global $g;
	static $s;
	$a = 1;
	$b = &$a;
	sort($c);
	$d = 2;
	foreach ([1] as &$e) {}
	$fn = function () use (&$h, $i) {};
	return $d;
}`)
	f := u.Functions[0]
	aliased := map[string]bool{}
	for _, v := range f.Vars {
		aliased[v.Name] = v.Flags.Aliased()
	}
	for name, want := range map[string]bool{"p": false, "r": true, "g": true, "s": true, "a": true, "b": true, "c": true, "d": false, "e": true, "h": true, "i": false} {
		if aliased[name] != want {
			t.Errorf("$%s aliased = %v, want %v", name, aliased[name], want)
		}
	}
	call := f.Body[4].Kids[0]
	if call.Op != OpCall || len(call.Args) != 1 || !call.Args[0].MayRef {
		t.Errorf("sort($c) argument should be a possible reference: %+v", call.Args)
	}
}

func TestDynamicScopeAndArgumentUse(t *testing.T) {
	cases := map[string][2]bool{
		"$$name = 1;":         {true, false},
		"extract($a);":        {true, false},
		"\\compact('a');":     {true, false},
		"include 'x.php';":    {true, false},
		"eval('1;');":         {true, false},
		"func_get_args();":    {false, true},
		"$a = 1; return $a;":  {false, false},
		"Other\\extract($a);": {false, false},
	}
	for body, want := range cases {
		u := lower(t, "<?php function f() { "+body+" }")
		f := u.Functions[0]
		if f.DynamicScope != want[0] || f.UsesArgs != want[1] {
			t.Errorf("%s: DynamicScope=%v UsesArgs=%v, want %v", body, f.DynamicScope, f.UsesArgs, want)
		}
	}
}

func TestStrictTypesBelongsToTheFile(t *testing.T) {
	u := lower(t, "<?php declare(strict_types=1); function f() {} $c = fn() => 1;")
	if !u.Strict || !u.Main.Strict || !u.Functions[0].Strict {
		t.Errorf("strict_types=1 must apply to every function of the file")
	}
	u = lower(t, "<?php function f() {}")
	if u.Strict || u.Functions[0].Strict {
		t.Errorf("strict_types defaults to off")
	}
}

func TestHoistingFollowsDeclarationLevel(t *testing.T) {
	u := lower(t, "<?php function top() {} { function block() {} } if (1) { function cond() {} } function outer() { function inner() {} }")
	want := map[string]bool{"top": true, "block": true, "cond": false, "outer": true, "inner": false}
	for _, f := range u.Functions {
		if f.Hoisted != want[f.Name] {
			t.Errorf("%s hoisted = %v, want %v", f.Name, f.Hoisted, want[f.Name])
		}
	}
	if _, err := lowerSource("<?php function a() {} function A() {}", 64); err == nil || !strings.Contains(err.Error(), "Cannot redeclare function A()") {
		t.Errorf("duplicate top-level function: %v", err)
	}
	if _, err := lowerSource("<?php if (1) { function a() {} } else { function a() {} }", 64); err != nil {
		t.Errorf("conditional declarations are checked at run time: %v", err)
	}
}

func TestIntegerLiteralsFollowTheProfileWidth(t *testing.T) {
	for _, c := range []struct {
		width int
		kind  LiteralKind
	}{{64, LitInt}, {32, LitFloat}} {
		u, err := lowerSource("<?php 2147483648;", c.width)
		if err != nil {
			t.Fatal(err)
		}
		if v := firstExpr(t, u).Value; v.Kind != c.kind {
			t.Errorf("%d-bit: 2147483648 is %v, want %v", c.width, v.Kind, c.kind)
		}
	}
}

func TestCompileErrorsCarryPHPLines(t *testing.T) {
	cases := []struct {
		src, msg string
		line     int
	}{
		{"<?php\n$this = 1;", "Cannot re-assign $this", 2},
		{"<?php\nfunction f($a, $a) {}", "Redefinition of parameter $a", 2},
		{"<?php\nf(a: 1,\n 2);", "Cannot use positional argument after named argument", 2},
		{"<?php\nwhile (1) {\n  break 2;\n}", "Cannot 'break' 2 levels", 3},
		{"<?php\ngoto x;\nwhile (1) { x: }", "'goto' into loop or switch statement is disallowed", 2},
		{"<?php\ntry {} finally {\n  goto out;\n}\nout:", "jump out of a finally block is disallowed", 3},
		{"<?php\n$a[];", "Cannot use [] for reading", 2},
		{"<?php\nfunction f(): void { return 1; }", "A void function must not return a value", 2},
		{"<?php\nyield 1;", "The \"yield\" expression can only be used inside a function", 2},
		{"<?php\n$f = function () use ($this) {};", "Cannot use $this as lexical variable", 2},
	}
	for _, c := range cases {
		_, err := lowerSource(c.src, 64)
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Message != c.msg || ce.Line != c.line {
			t.Errorf("%q: got %v, want %q on line %d", c.src, err, c.msg, c.line)
		}
	}
}

func TestLowerRejectsNonFileTrees(t *testing.T) {
	prof, _ := profile.Resolve("8.5.12", 64, false)
	if _, err := Lower(nil, prof, Options{}); err == nil {
		t.Error("nil tree accepted")
	}
	if _, err := Lower(&frontend.Node{Kind: "expr"}, prof, Options{}); err == nil {
		t.Error("non-file tree accepted")
	}
}

func TestEmptyFileLowers(t *testing.T) {
	u := lower(t, "")
	if len(u.Main.Body) != 0 || len(u.Functions) != 0 {
		t.Errorf("empty file produced code: %s", Dump(u))
	}
	u = lower(t, "<html>")
	if len(u.Main.Body) != 1 || u.Main.Body[0].Op != OpHTML || u.Main.Body[0].Effects != MayOutput {
		t.Errorf("inline HTML: %s", Dump(u))
	}
}

func TestUnitIsIndependentOfLaterLowering(t *testing.T) {
	// Units share no mutable state, so lowering another file cannot change
	// one already produced.
	a := lower(t, "<?php function f($x) { return $x; }")
	before := Dump(a)
	lower(t, "<?php function f(&$x) { global $y; }")
	if Dump(a) != before {
		t.Error("lowering a second file changed the first unit")
	}
}
