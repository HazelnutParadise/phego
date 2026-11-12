package vm

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// PHP reports a diagnostic on the line of the opline that raises it, which is
// the line of whatever expression was compiled last (CG(zend_lineno)), except
// that a plain assignment takes its target's line and an element write the
// line reached after its key. These tests check positions, not execution: for
// each diagnostic PHP prints, the instruction that raises it carries PHP's line.

var diagLine = regexp.MustCompile(`(?m)^(?:PHP )?(Warning|Deprecated|Fatal error): +(.*?) in (\S+?)(?: on line |:)(\d+)$|^  thrown in (\S+) on line (\d+)$`)

type phpDiag struct {
	msg  string
	line uint32
}

// diagnostics extracts every diagnostic line PHP printed for file.
func diagnostics(t *testing.T, out string) []phpDiag {
	t.Helper()
	var ds []phpDiag
	for _, m := range diagLine.FindAllStringSubmatch(out, -1) {
		msg, line := m[2], m[4]
		if m[5] != "" {
			msg, line = "thrown", m[6]
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, phpDiag{msg, uint32(n)})
	}
	return ds
}

func referenceStdout(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../tests/vm/w17/reference-64.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Case    string `json:"case"`
			Display struct {
				Stdout string `json:"stdout_b64"`
			} `json:"display"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	for _, c := range ref.Cases {
		if c.Case == name {
			b, err := base64.StdEncoding.DecodeString(c.Display.Stdout)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no reference for %s", name)
	return ""
}

// sel picks the instructions of one function that raise a diagnostic.
type sel struct {
	fn    string
	match func(img bytecode.Image, fn, pc int, in bytecode.Instruction) bool
}

func fnByName(img bytecode.Image, name string) int {
	for i, f := range img.Functions {
		if f.Name == name {
			return i
		}
	}
	return -1
}

// localIndexReg is the Location register of the local named name in fn.
func localIndexReg(img bytecode.Image, fn int, name string) (uint32, bool) {
	f := img.Functions[fn]
	for i, c := range f.LocalNames {
		if img.Constants[c].Bytes == name {
			for _, in := range f.Code {
				if in.Op == bytecode.OpLocal && in.B == uint32(i) {
					return in.A, true
				}
			}
		}
	}
	return 0, false
}

// readsVar matches a warning read of $name: Load with Aux 0, IncDec, or an
// array instruction reading the variable's Location itself.
func readsVar(name string) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(img bytecode.Image, fn, pc int, in bytecode.Instruction) bool {
		l, ok := localIndexReg(img, fn, name)
		return ok && ((in.Op == bytecode.OpLoad && in.Aux == 0 && in.B == l) || (in.Op == bytecode.OpIncDec && in.B == l) || locationRead(in, l))
	}
}

// locationRead reports whether an array instruction reads the variable of
// Location register l itself (ArrayWriteFlags, AddKeyLocation).
func locationRead(in bytecode.Instruction, l uint32) bool {
	switch flags := bytecode.ArrayWriteFlags(in.Aux); in.Op {
	case bytecode.OpArraySet:
		return (flags&bytecode.KeyLocation != 0 && in.B == l) || (flags&bytecode.ValueLocation != 0 && in.C == l)
	case bytecode.OpArrayAppend:
		return flags&bytecode.ValueLocation != 0 && in.B == l
	case bytecode.OpArrayUnset:
		return flags&bytecode.KeyLocation != 0 && in.B == l
	case bytecode.OpArrayAdd:
		return bytecode.ArrayAddFlags(in.Aux)&bytecode.AddKeyLocation != 0 && in.C == l
	}
	return false
}

func incDecOf(name string, op bytecode.IncDecOp) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(img bytecode.Image, fn, pc int, in bytecode.Instruction) bool {
		l, ok := localIndexReg(img, fn, name)
		return ok && in.Op == bytecode.OpIncDec && in.B == l && in.Aux == uint16(op)
	}
}

func opIs(op bytecode.Opcode) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(_ bytecode.Image, _, _ int, in bytecode.Instruction) bool { return in.Op == op }
}

func binaryIs(op bytecode.BinaryOp) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(_ bytecode.Image, _, _ int, in bytecode.Instruction) bool {
		return in.Op == bytecode.OpBinary && in.Aux == uint16(op)
	}
}

func callOf(name string) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(img bytecode.Image, _, _ int, in bytecode.Instruction) bool {
		return in.Op == bytecode.OpCall && img.Functions[in.B].Name == name
	}
}

// selectedLines returns the lines of the instructions s selects.
func selectedLines(t *testing.T, img bytecode.Image, s sel) []uint32 {
	t.Helper()
	fn := fnByName(img, s.fn)
	if fn < 0 {
		t.Fatalf("no function %s", s.fn)
	}
	var lines []uint32
	for pc, in := range img.Functions[fn].Code {
		if s.match(img, fn, pc, in) {
			lines = append(lines, img.Functions[fn].Positions[pc].Line)
		}
	}
	return lines
}

// checkDiagnostics pairs each PHP diagnostic, in output order, with the
// selector of the instruction that raises it and requires that instruction
// to carry PHP's line.
func checkDiagnostics(t *testing.T, img bytecode.Image, out string, sels []sel) {
	t.Helper()
	ds := diagnostics(t, out)
	if len(ds) != len(sels) {
		t.Fatalf("PHP printed %d diagnostics, %d selectors: %+v", len(ds), len(sels), ds)
	}
	for i, d := range ds {
		lines := selectedLines(t, img, sels[i])
		if len(lines) == 0 {
			t.Fatalf("diagnostic %d (%s, line %d): no instruction selected\n%s", i, d.msg, d.line, Disassemble(img))
		}
		for _, l := range lines {
			if l != d.line {
				t.Fatalf("diagnostic %d %q: PHP line %d, instruction lines %v\n%s", i, d.msg, d.line, lines, Disassemble(img))
			}
		}
	}
}

func compileFixture(t *testing.T, name string) bytecode.Image {
	t.Helper()
	src, err := os.ReadFile("../../tests/vm/w17/cases/" + name + ".php")
	if err != nil {
		t.Fatal(err)
	}
	img, err := compileWidth(t, string(src), 64, "/w/cases/"+name+".php")
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestLinesMatchReferenceMultilineExpressions(t *testing.T) {
	img := compileFixture(t, "multiline_expression_lines")
	m := "{main}"
	checkDiagnostics(t, img, referenceStdout(t, "multiline_expression_lines"), []sel{
		{m, readsVar("undef1")},
		{m, readsVar("undef2")},
		{m, opIs(bytecode.OpArrayGet)},
		{m, readsVar("undef3")},
		{m, incDecOf("s", bytecode.PostInc)},
		{m, incDecOf("n", bytecode.PreDec)},
		{m, incDecOf("t", bytecode.PostInc)},
		{m, incDecOf("t", bytecode.PostDec)},
		{m, binaryIs(bytecode.Divide)}, // Uncaught DivisionByZeroError ...:36
		{m, binaryIs(bytecode.Divide)}, // thrown in ... on line 36
	})
}

func TestLinesMatchReferenceWarningsInFunctions(t *testing.T) {
	img := compileFixture(t, "warn_lines_in_functions")
	checkDiagnostics(t, img, referenceStdout(t, "warn_lines_in_functions"), []sel{
		{"first", readsVar("u1")},
		{"second", opIs(bytecode.OpArrayGet)},
		{"second", opIs(bytecode.OpArrayGet)},
	})
}

// The remaining cases were settled with the PHP 8.5.12 oracle:
//
//	docker run --rm --network none --cap-drop ALL -v <probes>:/w:ro \
//	  php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 \
//	  php -n -d display_errors=1 -d error_reporting=-1 /w/<probe>.php
//
// Each case is the probe source and the diagnostics PHP printed for it.
var oracleCases = []struct {
	name, src, out string
	sels           []sel
}{
	{"echo argument", "<?php\necho\n    \"a\",\n    $u1,\n    \"b\";\n",
		"Warning: Undefined variable $u1 in /w/x.php on line 4\n", []sel{{"{main}", readsVar("u1")}}},
	{"interpolation parts", "<?php\n$s = \"x $u2\ny $u3\n\";\n",
		"Warning: Undefined variable $u2 in /w/x.php on line 2\nWarning: Undefined variable $u3 in /w/x.php on line 3\n",
		[]sel{{"{main}", readsVar("u2")}, {"{main}", readsVar("u3")}}},
	{"post-increment split", "<?php\n$q\n    ++;\n",
		"Warning: Undefined variable $q in /w/x.php on line 2\n", []sel{{"{main}", readsVar("q")}}},
	{"assignment takes the target line", "<?php\n$z =\n    $u4;\n",
		"Warning: Undefined variable $u4 in /w/x.php on line 2\n", []sel{{"{main}", readsVar("u4")}}},
	{"nested binary", "<?php\n$r = $u5\n    +\n    ($u6\n    *\n    2);\n",
		"Warning: Undefined variable $u6 in /w/x.php on line 6\nWarning: Undefined variable $u5 in /w/x.php on line 6\n",
		[]sel{{"{main}", readsVar("u6")}, {"{main}", readsVar("u5")}}},
	{"swapped compare", "<?php\n$c = $u7\n    >\n    $u8;\n",
		"Warning: Undefined variable $u8 in /w/x.php on line 4\nWarning: Undefined variable $u7 in /w/x.php on line 4\n",
		[]sel{{"{main}", readsVar("u8")}, {"{main}", readsVar("u7")}}},
	{"array literal elements", "<?php\n$a = [\n    $u9,\n    \"k\" =>\n        $u10,\n];\n",
		"Warning: Undefined variable $u9 in /w/x.php on line 3\nWarning: Undefined variable $u10 in /w/x.php on line 5\n",
		[]sel{{"{main}", readsVar("u9")}, {"{main}", readsVar("u10")}}},
	{"array literal reads value before key", "<?php\n$q = [\n    $uk2\n    =>\n    $uv2\n];\n",
		"Warning: Undefined variable $uv2 in /w/x.php on line 5\nWarning: Undefined variable $uk2 in /w/x.php on line 5\n",
		[]sel{{"{main}", readsVar("uv2")}, {"{main}", readsVar("uk2")}}},
	{"element read with split key", "<?php\necho $u11\n    [\n    \"k\"\n    ];\n",
		"Warning: Undefined variable $u11 in /w/x.php on line 4\nWarning: Trying to access array offset on null in /w/x.php on line 4\n",
		[]sel{{"{main}", readsVar("u11")}, {"{main}", opIs(bytecode.OpArrayGet)}}},
	{"return operand", "<?php\nfunction ret()\n{\n    return\n        $u12;\n}\nret();\n",
		"Warning: Undefined variable $u12 in /w/x.php on line 5\n", []sel{{"ret", readsVar("u12")}}},
	{"if condition", "<?php\nif (\n    $u13\n) {\n}\n",
		"Warning: Undefined variable $u13 in /w/x.php on line 3\n", []sel{{"{main}", readsVar("u13")}}},
	{"unary minus", "<?php\n$n = -\n    $u15;\n",
		"Warning: Undefined variable $u15 in /w/x.php on line 3\n", []sel{{"{main}", readsVar("u15")}}},
	{"cast", "<?php\n$i = (int)\n    $u16;\n",
		"Warning: Undefined variable $u16 in /w/x.php on line 3\n", []sel{{"{main}", readsVar("u16")}}},
	{"isset key", "<?php\n$a = [];\n$b = isset($a[\n    $u17\n]);\n",
		"Warning: Undefined variable $u17 in /w/x.php on line 4\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/x.php on line 4\n",
		[]sel{{"{main}", readsVar("u17")}, {"{main}", opIs(bytecode.OpArrayIsset)}}},
	{"compound assignment", "<?php\n$ca .=\n    $u18;\n",
		"Warning: Undefined variable $u18 in /w/x.php on line 3\nWarning: Undefined variable $ca in /w/x.php on line 3\n",
		[]sel{{"{main}", readsVar("u18")}, {"{main}", readsVar("ca")}}},
	{"ternary condition", "<?php\n$t = $u19\n    ?\n    1\n    :\n    2;\n",
		"Warning: Undefined variable $u19 in /w/x.php on line 2\n", []sel{{"{main}", readsVar("u19")}}},
	{"switch subject per case", "<?php\nswitch ($u20) {\n    case 1:\n        break;\n    case\n        2:\n        break;\n}\n",
		"Warning: Undefined variable $u20 in /w/x.php on line 3\nWarning: Undefined variable $u20 in /w/x.php on line 6\n",
		[]sel{{"{main}", lineIs(readsVar("u20"), 3)}, {"{main}", lineIs(readsVar("u20"), 6)}}},
	{"concat chain", "<?php\n$y = \"a\" .\n    $u21 .\n    \"b\";\n",
		"Warning: Undefined variable $u21 in /w/x.php on line 3\n", []sel{{"{main}", readsVar("u21")}}},
	{"element write after key", "<?php\n$w = [];\n$w[\n    $u23\n] =\n    $u24;\n",
		"Warning: Undefined variable $u23 in /w/x.php on line 4\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/x.php on line 4\nWarning: Undefined variable $u24 in /w/x.php on line 4\n",
		[]sel{{"{main}", readsVar("u23")}, {"{main}", opIs(bytecode.OpArraySet)}, {"{main}", readsVar("u24")}}},
	{"element write with split base", "<?php\n$w = [];\n$w\n    [\n    \"k\"\n    ] = $u2;\n",
		"Warning: Undefined variable $u2 in /w/x.php on line 5\n", []sel{{"{main}", readsVar("u2")}}},
	{"append", "<?php\n$w = [];\n$w\n    [] =\n    $u1;\n",
		"Warning: Undefined variable $u1 in /w/x.php on line 3\n", []sel{{"{main}", readsVar("u1")}}},
	{"assignment split around =", "<?php\n$z\n    =\n    $u3;\n",
		"Warning: Undefined variable $u3 in /w/x.php on line 2\n", []sel{{"{main}", readsVar("u3")}}},
	{"coalesce assignment reads a plain value on the target's line", "<?php\n$a\n ??=\n\n $u;\n",
		"Warning: Undefined variable $u in /w/x.php on line 2\n", []sel{{"{main}", readsVar("u")}}},
	{"coalesce assignment of an expression keeps its line", "<?php\n$c\n ??=\n\n -$u;\n",
		"Warning: Undefined variable $u in /w/x.php on line 5\n", []sel{{"{main}", readsVar("u")}}},
	{"heredoc parts", "<?php\n$h = <<<EOT\nfirst $u25\nsecond $u26\nEOT;\n",
		"Warning: Undefined variable $u25 in /w/x.php on line 3\nWarning: Undefined variable $u26 in /w/x.php on line 4\n",
		[]sel{{"{main}", readsVar("u25")}, {"{main}", readsVar("u26")}}},
	{"increment deprecations split", "<?php\n$st = \"a-\";\n$st\n    ++;\n--\n$st;\n",
		"Deprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/x.php on line 3\nDeprecated: Decrement on non-numeric string has no effect and is deprecated in /w/x.php on line 6\n",
		[]sel{{"{main}", incDecOf("st", bytecode.PostInc)}, {"{main}", incDecOf("st", bytecode.PreDec)}}},
	{"non-numeric warning", "<?php\n$v = \"5 apples\"\n    +\n    1;\n",
		"Warning: A non-numeric value encountered in /w/x.php on line 4\n", []sel{{"{main}", binaryIs(bytecode.Add)}}},
	{"coalesce fallback", "<?php\n$c = $u4\n    ??\n    $u5;\n",
		"Warning: Undefined variable $u5 in /w/x.php on line 4\n", []sel{{"{main}", readsVar("u5")}}},
	{"and left operand", "<?php\n$d = $u6\n    &&\n    $u7;\n",
		"Warning: Undefined variable $u6 in /w/x.php on line 2\n", []sel{{"{main}", readsVar("u6")}}},
	{"unset key", "<?php\n$w = [];\nunset($w[\n    $u8\n]);\n",
		"Warning: Undefined variable $u8 in /w/x.php on line 4\n", []sel{{"{main}", readsVar("u8")}}},
	{"call argument and call line", "<?php\nfunction two($a, $b)\n{\n    return 1;\n}\ntwo(\n    $u9\n);\n",
		"Warning: Undefined variable $u9 in /w/x.php on line 7\nFatal error: Uncaught ArgumentCountError: Too few arguments to function two(), 1 passed in /w/x.php on line 6 and exactly 2 expected in /w/x.php:2\n  thrown in /w/x.php on line 2\n",
		[]sel{{"{main}", readsVar("u9")}, {"two", lineIs(opIs(bytecode.OpStore), 2)}, {"two", lineIs(opIs(bytecode.OpStore), 2)}}},
	{"operator line inside a function and the call line", "<?php\nfunction d($a)\n{\n    return 1\n        %\n        $a;\n}\n$r = d(\n    0\n);\n",
		"Fatal error: Uncaught DivisionByZeroError: Modulo by zero in /w/x.php:6\n  thrown in /w/x.php on line 6\n",
		[]sel{{"d", binaryIs(bytecode.Modulo)}, {"d", binaryIs(bytecode.Modulo)}}},
	{"undefined function on its call line", "<?php\necho \"x\";\nmissing_one(\n    1,\n    2\n);\n",
		"Fatal error: Uncaught Error: Call to undefined function missing_one() in /w/x.php:3\n  thrown in /w/x.php on line 3\n",
		[]sel{{"{main}", opIs(bytecode.OpCallValue)}, {"{main}", opIs(bytecode.OpCallValue)}}},
}

// lineIs narrows a selector to the instructions on one line, for diagnostics
// that the same kind of instruction raises on several lines.
func lineIs(match func(bytecode.Image, int, int, bytecode.Instruction) bool, line uint32) func(bytecode.Image, int, int, bytecode.Instruction) bool {
	return func(img bytecode.Image, fn, pc int, in bytecode.Instruction) bool {
		return match(img, fn, pc, in) && img.Functions[fn].Positions[pc].Line == line
	}
}

func TestLinesMatchOracleProbes(t *testing.T) {
	for _, tc := range oracleCases {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, tc.src)
			checkDiagnostics(t, img, tc.out, tc.sels)
		})
	}
}

// A call carries the line of the call, not of its last argument: the stack
// trace and "passed in ... on line N" use it. The oracle printed "1 passed in
// /w/x.php on line 6" for the arity case above and "#0 /w/x.php(8): d(0)" for
// the modulo case.
func TestCallCarriesTheCallLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   bytecode.Opcode
		line uint32
	}{{"call argument and call line", bytecode.OpCallValue, 6}, {"operator line inside a function and the call line", bytecode.OpCall, 8}} {
		c := oracleCases[0]
		for _, o := range oracleCases {
			if o.name == tc.name {
				c = o
			}
		}
		img := compile(t, c.src)
		var lines []uint32
		for pc, in := range img.Functions[0].Code {
			if in.Op == tc.op {
				lines = append(lines, img.Functions[0].Positions[pc].Line)
			}
		}
		if len(lines) != 1 || lines[0] != tc.line {
			t.Fatalf("%s: call lines %v, want [%d]\n%s", c.name, lines, tc.line, Disassemble(img))
		}
	}
}
