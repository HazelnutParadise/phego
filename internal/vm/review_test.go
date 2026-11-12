package vm

import (
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// Compiler-level regressions for the review of W-17 stage 1 against PHP
// 8.5.12; the end-to-end versions with the oracle's bytes are in the root
// package's TestW17ReviewProbes. Each "want" below is what the pinned PHP
// image printed for the same program.

func TestReviewEvaluationOrder(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// Three parts are a rope: $a is converted before $a = 5 runs.
		{"rope reads its first part first", `$b = [5 => "x"]; $a = 1; echo "$a{$b[$a = 5]}\n";`, "1x\n"},
		// Two parts are FAST_CONCAT, which reads $a when it runs.
		{"two parts read late", `$b = [5 => "x"]; $a = 1; echo "$a{$b[$a = 5]}";`, "5x"},
		{"indented heredoc has two parts", "$b = [5 => \"x\"]; $a = 1; echo <<<EOT\n  $a{$b[$a = 5]}\n  EOT;", "5x"},
		{"heredoc with three parts", "$b = [5 => \"x\"]; $a = 1; echo <<<EOT\n$a\n{$b[$a = 5]}\nEOT;", "1\nx"},
		{"element write value is the expression", `$v = 3; $a = []; echo $a[0] = $v, $a[] = $v, "\n";`, "33\n"},
		{"element write key read by the write", `$k = 1; $a = []; $a[$k] = $k++; echo isset($a[2]) ? "2" : "1";`, "2"},
		{"literal key read by its element", `$k = 1; $a = [$k => $k++, $k]; echo $a[2], $a[3];`, "12"},
		{"nested literals", `$a = [[1, [2]], "k" => [3, "z" => [4]]]; echo $a[0][1][0], $a["k"]["z"][0];`, "24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			if got, _ := runMini(t, img); got != tc.want {
				t.Fatalf("output %q, want %q\n%s", got, tc.want, Disassemble(img))
			}
		})
	}
}

// PHP prints nothing for "$u; ($u);" or for the discarded parts of a for
// loop; "$u["k"];" still reads $u.
func TestDiscardedVariableIsNotRead(t *testing.T) {
	img := compile(t, "<?php\n$u;\n($u);\nfor ($u; $u, $i < 1; $u, $i++) {}\n$w[\"k\"];\n")
	for _, name := range []string{"u", "w", "i"} {
		lines := selectedLines(t, img, sel{"{main}", readsVar(name)})
		want := map[string]int{"u": 0, "w": 1, "i": 2}[name]
		if len(lines) != want {
			t.Fatalf("$%s is read %d times, want %d\n%s", name, len(lines), want, Disassemble(img))
		}
	}
}

// Array literals are built in their result register: no hidden locals, no
// Store/Load/Unset round trip.
func TestArrayLiteralsNeedNoHiddenLocals(t *testing.T) {
	img := compile(t, "<?php\n$a = [[1, [2]], \"k\" => [3, $u => 4]];\n")
	f := img.Functions[0]
	if f.Locals != 2 || len(f.LocalNames) != 2 {
		t.Fatalf("locals %d, names %d; want only $a and $u\n%s", f.Locals, len(f.LocalNames), Disassemble(img))
	}
	count := map[bytecode.Opcode]int{}
	for _, in := range f.Code {
		count[in.Op]++
	}
	if count[bytecode.OpNewArray] != 4 || count[bytecode.OpArrayAdd] != 7 || count[bytecode.OpStore] != 1 || count[bytecode.OpUnset] != 0 || count[bytecode.OpLoad] != 0 {
		t.Fatalf("instruction counts %v\n%s", count, Disassemble(img))
	}
	// $u => 4 passes the key variable to ArrayAdd.
	found := false
	for _, in := range f.Code {
		if in.Op == bytecode.OpArrayAdd && bytecode.ArrayAddFlags(in.Aux) == bytecode.AddKey|bytecode.AddKeyLocation {
			found = true
		}
	}
	if !found {
		t.Fatalf("no ArrayAdd reads $u itself\n%s", Disassemble(img))
	}
}

// A plain variable key or value of an element write is read by the write;
// the container itself as the value is read first, as PHP copies it.
func TestElementWriteOperands(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		op        bytecode.Opcode
		aux       uint16
		loads     int
	}{
		{"key and value variables", "$a[$k] = $v;", bytecode.OpArraySet, uint16(bytecode.KeyLocation | bytecode.ValueLocation), 0},
		{"value variable appended", "$a[] = $v;", bytecode.OpArrayAppend, uint16(bytecode.ValueLocation), 0},
		{"constant key", "$a[1] = $v;", bytecode.OpArraySet, uint16(bytecode.ValueLocation), 0},
		{"expression value", "$a[$k] = $v + 1;", bytecode.OpArraySet, uint16(bytecode.KeyLocation), 1},
		{"self assignment", "$a[$k] = $a;", bytecode.OpArraySet, uint16(bytecode.KeyLocation), 1},
		{"self append", "$a[] = $a;", bytecode.OpArrayAppend, 0, 1},
		{"used value is read back quietly", "echo $a[$k] = $v;", bytecode.OpArraySet, uint16(bytecode.KeyLocation | bytecode.ValueLocation), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			var writes []bytecode.Instruction
			loads := 0
			for _, in := range img.Functions[0].Code {
				switch in.Op {
				case bytecode.OpArraySet, bytecode.OpArrayAppend:
					writes = append(writes, in)
				case bytecode.OpLoad:
					loads++
				}
			}
			if len(writes) != 1 || writes[0].Op != tc.op || writes[0].Aux != tc.aux || loads != tc.loads {
				t.Fatalf("writes %+v, %d loads; want %v aux %d with %d loads\n%s", writes, loads, tc.op, tc.aux, tc.loads, Disassemble(img))
			}
		})
	}
}

// unset($a[$k]) passes a plain variable key as its Location: ZEND_UNSET_DIM
// reports an undefined container before it reads the key.
func TestArrayUnsetOperands(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		aux       uint16
		loads     int
	}{
		{"variable key", "unset($a[$k]);", uint16(bytecode.KeyLocation), 0},
		{"key is the container", "unset($a[$a]);", uint16(bytecode.KeyLocation), 0},
		{"constant key", `unset($a["x"]);`, 0, 0},
		{"expression key", "unset($a[$k + 1]);", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := compile(t, "<?php\n"+tc.src+"\n")
			var unsets []bytecode.Instruction
			loads := 0
			for _, in := range img.Functions[0].Code {
				switch in.Op {
				case bytecode.OpArrayUnset:
					unsets = append(unsets, in)
				case bytecode.OpLoad:
					loads++
				}
			}
			if len(unsets) != 1 || unsets[0].Aux != tc.aux || loads != tc.loads {
				t.Fatalf("unsets %+v, %d loads; want aux %d with %d loads\n%s", unsets, loads, tc.aux, tc.loads, Disassemble(img))
			}
		})
	}
	img := compile(t, "<?php\n$a = [\"x\" => 1, \"y\" => 2];\n$k = \"x\";\nunset($a[$k]);\necho isset($a[\"x\"]) ? \"kept\" : \"gone\", $a[\"y\"];\n")
	if out, _ := runMini(t, img); out != "gone2" {
		t.Fatalf("output %q, want \"gone2\"\n%s", out, Disassemble(img))
	}
}
