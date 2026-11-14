package vm

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden from the current compiler")

// goldenPrograms cover every statement and expression form of the stage-1
// subset at least once, plus the evaluation-order cases. Each golden file
// holds the source followed by the disassembly.
var goldenPrograms = []struct{ name, src string }{
	{"inline_html_echo_print", "<p>head</p>\n<?php echo 'a', 1, 2.5; print 'p'; ?>\ntail"},
	{"literals", "<?php\necho null, true, false, 0, -7, 9223372036854775807, 1.5, -0.0, 'str', \"a\\tb\";\n"},
	{"assignments", "<?php\n$a = 1;\n$b = $a;\n$c = $d = 2;\n$e = ($f = 3) + 1;\n(void)$a;\n"},
	{"compound_assignments", "<?php\n$a += 1; $a -= 2; $a *= 3; $a /= 4; $a %= 5; $a **= 2;\n$a .= 'x'; $a <<= 1; $a >>= 1; $a &= 7; $a |= 8; $a ^= 9;\n$b ??= 'default';\n"},
	{"increment_decrement", "<?php\n$i = 0;\n$i++; ++$i; $i--; --$i;\necho $i++ + ++$i;\n"},
	{"unary_and_casts", "<?php\n$x = '5';\necho !$x, -$x, +$x, ~$x;\necho (int)$x, (float)$x, (string)$x, (bool)$x;\n$a = (array)$x;\n"},
	{"arithmetic_and_bitwise", "<?php\n$a = 6; $b = 4;\necho $a + $b, $a - $b, $a * $b, $a / $b, $a % $b, $a ** $b, $a . $b;\necho $a << $b, $a >> $b, $a & $b, $a | $b, $a ^ $b;\n"},
	{"comparisons", "<?php\n$a = 1; $b = 2;\necho $a == $b, $a != $b, $a <> $b, $a === $b, $a !== $b;\necho $a < $b, $a <= $b, $a > $b, $a >= $b, $a <=> $b, $a xor $b;\n"},
	{"logical", "<?php\n$a = 1; $b = 0;\necho $a && $b, $a || $b, $a and $b, $a or $b, !$a;\n"},
	{"ternaries", "<?php\n$a = 1;\necho $a ? 'y' : 'n', $a ?: 'n';\necho $a ? ($a > 1 ? 'big' : 'one') : 'zero';\n"},
	{"coalesce", "<?php\n$a = ['k' => ['j' => 1]];\necho $u ?? 'u', $a['k']['j'] ?? 'n', $a['x'] ?? $u ?? 'chain';\n"},
	{"interpolation", "<?php\n$x = 'v'; $a = ['k' => 1];\necho \"a $x b\", \"$x\", \"{$a['k']}$x\", \"$a[k] tail\";\necho <<<EOT\nH $x\nEOT;\n"},
	{"array_literals", "<?php\n$e = [];\n$a = [1, 'k' => 2, 5 => [3, $e]];\n$b = [$e, [$a]];\n"},
	{"array_read_write", "<?php\n$a = [];\n$a['k'] = 1;\n$a[] = 2;\necho $a['k'], $a[0]['x'];\nunset($a['k']);\n$v = $a[1] = 3;\n"},
	{"isset_unset", "<?php\n$a = ['k' => 1];\necho isset($a), isset($a, $b), isset($a['k']), isset($a['k']['j']);\nunset($a, $b);\n"},
	{"if_elseif_else", "<?php\nif ($a) {\n    echo 1;\n} elseif ($b) {\n    echo 2;\n} else if ($c) {\n    echo 3;\n} else {\n    echo 4;\n}\nif ($d) echo 5;\n"},
	{"while_break_continue", "<?php\n$i = 0;\nwhile ($i < 10) {\n    $i++;\n    if ($i == 2) {\n        continue;\n    }\n    if ($i == 5) {\n        break;\n    }\n    echo $i;\n}\n"},
	{"do_while", "<?php\n$i = 0;\ndo {\n    $i++;\n    if ($i == 1) continue;\n    echo $i;\n} while ($i < 3);\n"},
	{"for_parts", "<?php\nfor ($i = 0, $j = 9; $i < $j; $i++, $j--) {\n    echo $i;\n}\nfor (;;) {\n    break;\n}\n"},
	{"nested_loops_levels", "<?php\nfor ($i = 0; $i < 3; $i++) {\n    while (true) {\n        if ($i) {\n            continue 2;\n        }\n        break 2;\n    }\n}\n"},
	{"switch_default_middle", "<?php\nswitch ($v) {\n    case 1:\n        echo 'one';\n    default:\n        echo 'default';\n    case 2:\n        echo 'two';\n        break;\n}\nswitch (f()) {\n}\nfunction f() { return 1; }\n"},
	{"switch_in_loop_continue", "<?php\nfor ($i = 0; $i < 3; $i++) {\n    switch ($i) {\n        case 1:\n            continue 2;\n        case 2:\n            continue;\n    }\n    echo $i;\n}\n"},
	{"functions", "<?php\nfunction add($a, $b) {\n    $c = $a + $b;\n    return $c;\n}\nfunction none() {\n}\nfunction bare() {\n    return;\n}\necho add(1, 2), none(), bare();\n"},
	{"calls", "<?php\necho later(1), LATER(2), later(), later(1, 2), missing(3);\nfunction later($n) {\n    return $n <= 0 ? 0 : later($n - 1);\n}\n"},
	{"exit_forms", "<?php\nif ($a) exit;\nif ($b) exit(1);\nif ($c) die('bye');\necho 'x' . exit(2);\n"},
	{"top_level_return", "<?php\nif ($a) {\n    return;\n}\nreturn [1, 2];\n"},
	{"order_binary_calls", "<?php\n" + fgh + "echo f() + g() * h();\n"},
	{"order_array_write", "<?php\n" + fgh + "$a[f()] = g();\n"},
	{"order_compound_call", "<?php\n" + fgh + "$x += f();\n"},
	{"order_call_arguments", "<?php\nfunction k($a, $b) { return 1; }\n" + fgh + "echo k(g(), h() ? 1 : 2);\n"},
	{"order_and_or", "<?php\n" + fgh + "$r = f() && g() || h();\n"},
	{"order_coalesce_call", "<?php\n" + fgh + "echo $x ?? f();\n"},
	{"order_nested_ternary", "<?php\n" + fgh + "echo f() ? (g() ? 1 : 2) : (h() ? 3 : 4);\n"},
	{"order_greater_swap", "<?php\n" + fgh + "echo f() > g(), f() >= g(), $a > $b;\n"},
	{"order_variable_read_late", "<?php\n$k = 5;\necho $k + $k++ + $k;\n$a[$k] = $k++;\n"},
	// Stage 2.
	{"element_nested_writes", "<?php\n$a[1][2] = 3;\n$a[][] = $v;\n$a['x'][$k]['z'] = $a;\n$r = $a[1][$k] = $v;\n"},
	{"element_compound_assignments", "<?php\n$a[$k] += $v;\n$a[1]['x'] .= 's';\n$a[] -= 1;\necho $a[0] *= 2;\n"},
	{"element_increment_decrement", "<?php\n$a[$i]++;\n--$a[1][2];\n$a[]++;\necho $a[0]--, ++$a['k'];\n"},
	{"element_coalesce_assignment", "<?php\n$a['k'] ??= 1;\n$a[1][$k] ??= $v;\necho $a[0] ??= 'd';\n"},
	{"element_nested_unset", "<?php\nunset($a[1][2], $a['x'][$k]['z']);\n"},
	{"foreach_forms", "<?php\nforeach ($a as $v) {\n    echo $v;\n}\nforeach ($a as $k => $v) {\n    echo $k;\n}\nforeach ([1, 2] as $out[]) {\n}\nforeach ($a as $ks['k'] => $m[1][2]) {\n}\n"},
	{"foreach_break_continue_levels", "<?php\nforeach ($a as $x) {\n    foreach ($b as $y) {\n        if ($y) {\n            continue 2;\n        }\n        if ($x) {\n            break 2;\n        }\n        continue;\n    }\n    break;\n}\n"},
	{"foreach_in_function", "<?php\nfunction first($xs, $want) {\n    foreach ($xs as $k => $x) {\n        if ($x == $want) {\n            return $k;\n        }\n    }\n    return null;\n}\n"},
	{"parameter_defaults", "<?php\nfunction f(\n    $a,\n    $b = 5,\n    $c = 'x',\n    $d = [1, 'k' => [2]],\n    $e = 1 + 2\n) {\n    return $b;\n}\necho f(1), f(1, 2), f(1, 2, 3, 4, 5), f(1, 2, 3, 4, 5, 6);\n"},
	{"string_offsets", "<?php\n$s = 'abc';\necho $s[0], $s[-1], 'lit'[1], \"$s[2]\", $s[9] ?? 'd', isset($s[1]);\n$s[0] = 'X';\n$r = $s[1] = 'Y';\n"},
	// Stage 5.
	{"constants", "<?php\nnamespace ns;\nconst A = 1, B = A + 1;\necho A, B, \\ns\\A, E_ALL, \\PHP_EOL, PHP_EOL, E_STRICT, UNDEF, \\UNDEF;\n"},
	{"constant_declaration_lines", "<?php\nconst A = 1,\n  B = A\n  . [],\n  C = [1, 'k' => 2];\necho C['k'];\n"},
	{"match_forms", "<?php\necho match ($x) { 1, 2 => 'a', 3 => 'b', default => 'c' };\necho match ($x) { 1 => 'a', $y => 'b' };\necho match (true) { $x > 1 => 'big', default => 'small' };\necho match (f()) {};\nfunction f() { return 1; }\n"},
	{"empty_forms", "<?php\n$a = ['k' => ['j' => 0]];\necho empty($u), empty($a['k']['j']), empty($a['k']['x']['y']), empty(f()), empty(1 + 1);\nfunction f() { return 0; }\n"},
	{"silence", "<?php\necho @$u, @$a['k'], @(1 + $v), @f();\n@$w;\n$r = @$x++;\nfunction f() { return @$y; }\n"},
}

func TestGoldenDisassembly(t *testing.T) {
	for _, p := range goldenPrograms {
		t.Run(p.name, func(t *testing.T) {
			img := compile(t, p.src)
			got := "-- source --\n" + p.src + "\n-- disassembly --\n" + Disassemble(img)
			path := filepath.Join("testdata", p.name+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/vm -run TestGoldenDisassembly -update)", err)
			}
			if got != string(want) {
				t.Fatalf("disassembly differs from %s:\n%s", path, got)
			}
		})
	}
}

func TestDisassemblyListsTablesAndInstructions(t *testing.T) {
	img := compile(t, "<?php\n$x = 'a';\necho $x . 1;\n")
	d := Disassemble(img)
	for _, want := range []string{
		"source 1 \"/w/test.php\"",
		"string \"a\"",
		"int 1",
		"func 0 \"{main}\"",
		"local 0 \"x\"",
		": Binary Concat ",
		"; line 3",
	} {
		if !strings.Contains(d, want) {
			t.Fatalf("disassembly lacks %q:\n%s", want, d)
		}
	}
}
