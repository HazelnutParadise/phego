package vm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/ir"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// gen builds random programs from a tiny grammar of the compiled subset:
// stage 1 and the element writes, foreach, parameter defaults and string
// offsets of stage 2.
type gen struct {
	r     *rand.Rand
	b     strings.Builder
	funcs []int // parameter counts of f0, f1, ...
}

func (g *gen) v() string { return fmt.Sprintf("$v%d", g.r.Intn(4)) }

func (g *gen) expr(d int) string {
	if d <= 0 {
		switch g.r.Intn(6) {
		case 0:
			return fmt.Sprint(g.r.Intn(20) - 5)
		case 1:
			return []string{"1.5", "-0.0", "1e3"}[g.r.Intn(3)]
		case 2:
			return []string{"'s'", "\"t\"", "''", "'0'"}[g.r.Intn(4)]
		case 3:
			return []string{"true", "false", "null"}[g.r.Intn(3)]
		default:
			return g.v()
		}
	}
	e := func() string { return g.expr(d - 1) }
	switch g.r.Intn(24) {
	case 18:
		return "(" + g.v() + "[" + e() + "][" + e() + "] = " + e() + ")"
	case 19:
		return "(" + g.v() + g.dims(d) + " " + []string{"+=", "-=", ".=", "*=", "??=", "|=", "**=", "<<="}[g.r.Intn(8)] + " " + e() + ")"
	case 20:
		t := g.v() + g.dims(d)
		return []string{t + "++", "++" + t, t + "--", "--" + t}[g.r.Intn(4)]
	case 21:
		return "(" + g.v() + "[][" + e() + "] = " + g.v() + ")"
	case 22:
		return "(" + []string{"'abc'", "\"x\"", "''"}[g.r.Intn(3)] + "[" + e() + "] ?? " + e() + ")"
	case 23:
		return "(" + g.v() + "[" + e() + "][] .= " + e() + ")"
	case 0:
		ops := []string{"+", "-", "*", "/", "%", "**", ".", "<<", ">>", "&", "|", "^", "==", "!=", "===", "!==", "<", "<=", ">", ">=", "<=>"}
		return "(" + e() + " " + ops[g.r.Intn(len(ops))] + " " + e() + ")"
	case 1:
		return "(" + e() + " " + []string{"&&", "||", "xor", "and", "or"}[g.r.Intn(5)] + " " + e() + ")"
	case 2:
		return []string{"!", "-", "+", "~"}[g.r.Intn(4)] + "(" + e() + ")"
	case 3:
		return "(" + []string{"int", "float", "string", "bool", "array"}[g.r.Intn(5)] + ")(" + e() + ")"
	case 4:
		return "(" + e() + " ? " + e() + " : " + e() + ")"
	case 5:
		return "(" + e() + " ?: " + e() + ")"
	case 6:
		return "(" + g.v() + " ?? " + e() + ")"
	case 7:
		return "isset(" + g.v() + ", " + g.v() + "[" + e() + "])"
	case 8:
		return g.v() + "[" + e() + "]"
	case 9:
		return "[" + e() + ", 'k' => " + e() + ", " + fmt.Sprint(g.r.Intn(4)) + " => [" + e() + "]]"
	case 10:
		if len(g.funcs) == 0 {
			return e()
		}
		f := g.r.Intn(len(g.funcs))
		n := g.funcs[f] + g.r.Intn(3) - 1
		if n < 0 {
			n = 0
		}
		args := make([]string, n)
		for i := range args {
			args[i] = e()
		}
		name := fmt.Sprintf("f%d", f)
		if g.r.Intn(4) == 0 {
			name = strings.ToUpper(name)
		}
		if g.r.Intn(8) == 0 {
			name = "nope"
		}
		return name + "(" + strings.Join(args, ", ") + ")"
	case 11:
		return "\"x " + g.v() + " {" + g.v() + "['k']} y\""
	case 12:
		return "(print " + e() + ")"
	case 13:
		return "(" + g.v() + " = " + e() + ")"
	case 14:
		return []string{g.v() + "++", "++" + g.v(), g.v() + "--", "--" + g.v()}[g.r.Intn(4)]
	case 15:
		return "(" + g.v() + " " + []string{"+=", "-=", ".=", "*=", "??=", "|="}[g.r.Intn(6)] + " " + e() + ")"
	case 16:
		return "(" + g.v() + "[" + e() + "] = " + e() + ")"
	default:
		return "exit(" + e() + ")"
	}
}

// dims is one to three element levels, the last of which has a key.
func (g *gen) dims(d int) string {
	out := ""
	for i := g.r.Intn(3); i > 0; i-- {
		if g.r.Intn(4) == 0 {
			out += "[]"
		} else {
			out += "[" + g.expr(d-1) + "]"
		}
	}
	return out + "[" + g.expr(d-1) + "]"
}

func (g *gen) stmt(d, loops int) {
	e := func() string { return g.expr(2) }
	if d <= 0 {
		fmt.Fprintf(&g.b, "echo %s, %s;\n", e(), e())
		return
	}
	body := func(loops int) {
		for i := g.r.Intn(3); i >= 0; i-- {
			g.stmt(d-1, loops)
		}
	}
	switch g.r.Intn(18) {
	case 14:
		fmt.Fprintf(&g.b, "foreach (%s as %s) {\n", e(), g.v())
		body(loops + 1)
		g.b.WriteString("}\n")
	case 15:
		fmt.Fprintf(&g.b, "foreach (%s as %s => %s) {\n", g.v(), g.v(), g.v())
		body(loops + 1)
		g.b.WriteString("}\n")
	case 16:
		fmt.Fprintf(&g.b, "foreach (%s as %s%s => %s[]) {\n", e(), g.v(), g.dims(1), g.v())
		body(loops + 1)
		g.b.WriteString("}\n")
	case 17:
		fmt.Fprintf(&g.b, "unset(%s%s, %s[%s][%s]);\n", g.v(), "["+e()+"]["+e()+"]["+e()+"]", g.v(), e(), e())
	case 0:
		fmt.Fprintf(&g.b, "%s = %s;\n", g.v(), e())
	case 1:
		fmt.Fprintf(&g.b, "echo %s;\n", e())
	case 2:
		fmt.Fprintf(&g.b, "if (%s) {\n", e())
		body(loops)
		fmt.Fprintf(&g.b, "} elseif (%s) {\n", e())
		body(loops)
		g.b.WriteString("} else {\n")
		body(loops)
		g.b.WriteString("}\n")
	case 3:
		fmt.Fprintf(&g.b, "while (%s) {\n", e())
		body(loops + 1)
		g.b.WriteString("}\n")
	case 4:
		fmt.Fprintf(&g.b, "for (%s = 0, %s; %s; %s++, %s) {\n", g.v(), e(), e(), g.v(), e())
		body(loops + 1)
		g.b.WriteString("}\n")
	case 5:
		g.b.WriteString("do {\n")
		body(loops + 1)
		fmt.Fprintf(&g.b, "} while (%s);\n", e())
	case 6:
		fmt.Fprintf(&g.b, "switch (%s) {\ncase %s:\n", e(), e())
		body(loops + 1)
		g.b.WriteString("default:\n")
		body(loops + 1)
		fmt.Fprintf(&g.b, "case %s:\nbreak;\n}\n", e())
	case 7:
		if loops > 0 {
			k := []string{"break", "continue"}[g.r.Intn(2)]
			fmt.Fprintf(&g.b, "%s %d;\n", k, 1+g.r.Intn(loops))
			return
		}
		fmt.Fprintf(&g.b, "%s[] = %s;\n", g.v(), e())
	case 8:
		fmt.Fprintf(&g.b, "unset(%s, %s[%s]);\n", g.v(), g.v(), e())
	case 9:
		fmt.Fprintf(&g.b, "return %s;\n", e())
	case 10:
		fmt.Fprintf(&g.b, "%s[%s] = %s;\n", g.v(), e(), e())
	case 11:
		g.b.WriteString("?>html<?php\n")
	case 12:
		g.b.WriteString("exit;\n")
	default:
		fmt.Fprintf(&g.b, "%s;\n", e())
	}
}

func (g *gen) program() string {
	g.b.Reset()
	g.b.WriteString("<?php\n")
	g.funcs = nil
	n := g.r.Intn(3)
	for i := 0; i < n; i++ {
		g.funcs = append(g.funcs, g.r.Intn(3))
	}
	for i, params := range g.funcs {
		ps := make([]string, params)
		optional := false
		for j := range ps {
			ps[j] = fmt.Sprintf("$v%d", j)
			// Once a parameter has a default, the later ones do too.
			if optional = optional || g.r.Intn(3) == 0; optional {
				ps[j] += " = " + []string{"1", "'d'", "null", "[]", "[1, 'k' => [2]]", "1 + 2", "-1.5"}[g.r.Intn(7)]
			}
		}
		fmt.Fprintf(&g.b, "function f%d(%s) {\n", i, strings.Join(ps, ", "))
		for k := g.r.Intn(3); k >= 0; k-- {
			g.stmt(2, 0)
		}
		g.b.WriteString("}\n")
	}
	for k := g.r.Intn(5); k >= 0; k-- {
		g.stmt(3, 0)
	}
	return g.b.String()
}

func TestRandomSubsetProgramsCompileAndBuild(t *testing.T) {
	g := &gen{r: rand.New(rand.NewSource(17))}
	compiled := 0
	const programs = 400
	for i := 0; i < programs; i++ {
		src := g.program()
		unit, err := lowerSource(src, 64, testFile)
		if err != nil {
			var ce *ir.CompileError
			if errors.As(err, &ce) {
				continue // a PHP compile-time fatal, e.g. a folded constant error
			}
			t.Fatalf("program %d does not lower: %v\n%s", i, err, src)
		}
		img, err := Compile(context.Background(), unit, []byte(src))
		var fatal *CompileError
		if errors.As(err, &fatal) && fatal.Message == "Cannot use [] for reading" {
			continue // "$a[] ??= v", PHP's compile-time fatal that the compiler reports
		}
		if err != nil {
			t.Fatalf("program %d: %v\n%s", i, err, src)
		}
		if _, err := bytecode.Build(context.Background(), img, buildOptions()); err != nil {
			t.Fatalf("program %d: Build: %v\n%s\n%s", i, err, src, Disassemble(img))
		}
		compiled++
	}
	if compiled < programs*9/10 {
		t.Fatalf("only %d of %d generated programs lowered", compiled, programs)
	}
}

func FuzzCompileSubset(f *testing.F) {
	files, _ := filepath.Glob("../../tests/vm/w17/cases/*.php")
	for _, p := range files {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(string(b))
		}
	}
	g := &gen{r: rand.New(rand.NewSource(5))}
	for i := 0; i < 20; i++ {
		f.Add(g.program())
	}
	f.Add("<?php foreach ($a as $b) {} class A {} $x = fn() => 1;")
	f.Add("<?php function f($a, $b = [1, 'k' => 2], $c = 'x' . 'y') { foreach ($b as $k => $v[$k][]) { $a[$k][0] .= $v; $a[]++; } unset($a[1][2]); return $a['s'][0] ?? $c[1]; }\necho f(1), $u[1][2] ??= 3;")
	f.Add("<?php $a[] ??= 1;")
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, src string) {
		tree, err := frontend.Parse([]byte(src), frontend.LexOptions{IntWidth: 64, MaxBytes: 1 << 16})
		if err != nil {
			return
		}
		unit, err := ir.Lower(tree, prof, ir.Options{File: "/fuzz.php"})
		if err != nil {
			return
		}
		img, err := Compile(context.Background(), unit, []byte(src))
		var ue *UnsupportedError
		var ce *CompileError
		switch {
		case errors.As(err, &ue):
			if ue.Line < 1 || ue.Feature == "" || ue.File != "/fuzz.php" {
				t.Fatalf("malformed UnsupportedError %+v", ue)
			}
			return
		case errors.As(err, &ce):
			// A PHP compile-time fatal the compiler reports itself, such as
			// "$a[] ??= 1" (Cannot use [] for reading).
			if ce.Line < 1 || ce.Message == "" || ce.File != "/fuzz.php" {
				t.Fatalf("malformed CompileError %+v", ce)
			}
			return
		case err != nil:
			t.Fatalf("unexpected error class %T: %v", err, err)
		}
		if _, err := bytecode.Build(context.Background(), img, buildOptions()); err != nil && !errors.Is(err, bytecode.ErrLimit) {
			t.Fatalf("Build: %v\n%s", err, Disassemble(img))
		}
	})
}
