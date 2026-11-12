package phego

import (
	"testing"
)

// W-17 stage 1: REQ-025, REQ-026; T-009, T-010. Regressions for the
// divergences an independent review of the compiler found against PHP 8.5.12.
// Each expected output is what the pinned PHP image printed for the source,
// saved as /w/cases/<name>.php (the path this test compiles under):
//
//	docker run --rm --network none --cap-drop ALL -v <dir>:/w:ro \
//	  php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 \
//	  php -n -d display_errors=1 -d log_errors=0 -d error_reporting=-1 -d html_errors=0 /w/cases/<name>.php
//
// The PHP_INT_MAX literals assume the 64-bit profile.
var w17ReviewProbes = []struct {
	name, src, stdout string
	exit              int
}{
	{"interp_rope_cv_first",
		"<?php\n$b = [5 => \"x\"];\n$a = 1;\necho \"$a{$b[$a = 5]}\\n\";\n",
		"1x\n", 0},
	{"interp_rope_array_first",
		"<?php\nfunction f() { echo \"[f]\"; return 0; }\n$b = [0 => \"y\"];\n$arr = [1];\necho \"$arr{$b[f()]}!\\n\";\n",
		"\nWarning: Array to string conversion in /w/cases/interp_rope_array_first.php on line 5\n[f]Arrayy!\n", 0},
	{"interp_rope_const_parts",
		"<?php\n$b = [0 => \"y\"];\n$c = 2; echo \"<$c{$b[$c = 0]}>\\n\";\n$c = 2; echo \"$c-{$b[$c = 0]}\\n\";\n$c = 2; echo \"$c{$b[$c = 0]}\\n\";\n",
		"<2y>\n2-y\n2y\n", 0},
	{"interp_fast_concat_two",
		"<?php\n$b = [5 => \"x\"];\n$a = 1;\necho \"$a{$b[$a = 5]}\";\necho \"\\n\";\n",
		"5x\n", 0},
	{"interp_heredoc",
		"<?php\n$b = [5 => \"x\"];\n$a = 1;\n$x = <<<EOT\n  $a{$b[$a = 5]}\n  EOT;\necho $x, \"\\n\";\n$a = 1;\n$y = <<<EOT\n$a\n{$b[$a = 5]}\nEOT;\necho $y, \"\\n\";\n",
		"5x\n1\nx\n", 0},
	{"interp_undefined_parts",
		"<?php\necho \"$u1 $u2\\n\";\necho \"$u3$u4$u5\\n\";\necho \"$u6$u7\";\necho \"\\n\";\n",
		"\nWarning: Undefined variable $u1 in /w/cases/interp_undefined_parts.php on line 2\n\nWarning: Undefined variable $u2 in /w/cases/interp_undefined_parts.php on line 2\n \n\nWarning: Undefined variable $u3 in /w/cases/interp_undefined_parts.php on line 3\n\nWarning: Undefined variable $u4 in /w/cases/interp_undefined_parts.php on line 3\n\nWarning: Undefined variable $u5 in /w/cases/interp_undefined_parts.php on line 3\n\n\nWarning: Undefined variable $u6 in /w/cases/interp_undefined_parts.php on line 4\n\nWarning: Undefined variable $u7 in /w/cases/interp_undefined_parts.php on line 4\n\n", 0},
	{"dim_set_array_key",
		"<?php\n$a = [];\n$a[[]] = $uv;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/dim_set_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_set_array_key.php on line 3\n", 255},
	{"dim_append_string",
		"<?php\n$s = \"abc\";\n$s[] = $uv;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught Error: [] operator not supported for strings in /w/cases/dim_append_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_append_string.php on line 3\n", 255},
	{"dim_set_undefined_key_value",
		"<?php\n$a[$uk] = $uv;\necho \"[\", $a[\"\"], \"]\\n\";\n$b = [];\n$b[$uk] = $uv;\necho \"[\", $b[\"\"], \"]\\n\";\n",
		"\nWarning: Undefined variable $uk in /w/cases/dim_set_undefined_key_value.php on line 2\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/dim_set_undefined_key_value.php on line 2\n\nWarning: Undefined variable $uv in /w/cases/dim_set_undefined_key_value.php on line 2\n[]\n\nWarning: Undefined variable $uk in /w/cases/dim_set_undefined_key_value.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/dim_set_undefined_key_value.php on line 5\n\nWarning: Undefined variable $uv in /w/cases/dim_set_undefined_key_value.php on line 5\n[]\n", 0},
	{"dim_set_false_container",
		"<?php\n$f = false;\n$f[$uk] = $uv;\necho \"[\", $f[\"\"], \"]\\n\";\n$g = false;\n$g[] = $uv;\necho count_is_one($g), \"\\n\";\nfunction count_is_one($x) { return isset($x[0]) ? \"no\" : \"null\"; }\n",
		"\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/dim_set_false_container.php on line 3\n\nWarning: Undefined variable $uk in /w/cases/dim_set_false_container.php on line 3\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/dim_set_false_container.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/dim_set_false_container.php on line 3\n[]\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/dim_set_false_container.php on line 6\n\nWarning: Undefined variable $uv in /w/cases/dim_set_false_container.php on line 6\nnull\n", 0},
	{"dim_set_scalar_container",
		"<?php\n$s = 5;\n$s[$uk] = $uv;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/dim_set_scalar_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_set_scalar_container.php on line 3\n", 255},
	{"dim_append_scalar_container",
		"<?php\n$t = true;\n$t[] = $uv;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/dim_append_scalar_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_append_scalar_container.php on line 3\n", 255},
	{"dim_append_overflow",
		"<?php\n$a = [9223372036854775807 => 1];\n$a[] = $uv;\necho \"end\\n\";\n",
		"\nWarning: Undefined variable $uv in /w/cases/dim_append_overflow.php on line 3\n\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/dim_append_overflow.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_append_overflow.php on line 3\n", 255},
	{"dim_set_float_key",
		"<?php\n$a = [];\n$a[1.5] = $uv;\necho \"[\", $a[1], \"]\\n\";\n",
		"\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/dim_set_float_key.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/dim_set_float_key.php on line 3\n[]\n", 0},
	{"dim_set_false_key_reads_converted",
		"<?php\n$f = false;\n$f[$f] = 1;\necho \"end\\n\";\n",
		"\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/dim_set_false_key_reads_converted.php on line 3\n\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/dim_set_false_key_reads_converted.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_set_false_key_reads_converted.php on line 3\n", 255},
	{"dim_set_null_key_reads_converted",
		"<?php\n$n = null;\n$n[$n] = 1;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/dim_set_null_key_reads_converted.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_set_null_key_reads_converted.php on line 3\n", 255},
	{"dim_set_undefined_key_reads_converted",
		"<?php\n$u[$u] = 1;\necho \"end\\n\";\n",
		"\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/dim_set_undefined_key_reads_converted.php:2\nStack trace:\n#0 {main}\n  thrown in /w/cases/dim_set_undefined_key_reads_converted.php on line 2\n", 255},
	{"dim_set_self_scalar",
		"<?php\n$n = null;\n$n[] = $n;\necho isset($n[0]) ? \"set\" : \"null\", \"\\n\";\n$f = false;\n$f[\"k\"] = $f;\necho $f[\"k\"] === false ? \"false\" : \"other\", \"\\n\";\n",
		"null\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/dim_set_self_scalar.php on line 6\nfalse\n", 0},
	{"dim_set_self",
		"<?php\n$a = [1];\n$a[\"k\"] = $a;\necho isset($a[\"k\"][\"k\"]) ? \"nested\" : \"flat\", \"\\n\";\n$b = [1];\n$b[] = $b;\necho isset($b[1][1]) ? \"nested\" : \"flat\", \"\\n\";\n$d[] = $d;\n$e[\"k\"] = $e;\n$g = [];\n$g[$uk] = $g;\necho \"end\\n\";\n",
		"flat\nflat\n\nWarning: Undefined variable $d in /w/cases/dim_set_self.php on line 8\n\nWarning: Undefined variable $e in /w/cases/dim_set_self.php on line 9\n\nWarning: Undefined variable $uk in /w/cases/dim_set_self.php on line 11\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/dim_set_self.php on line 11\nend\n", 0},
	{"dim_set_key_after_value",
		"<?php\n$k = 1;\n$a = [];\n$a[$k] = $k++;\necho isset($a[2]) ? \"2\" : \"1\", \"\\n\";\n",
		"2\n", 0},
	{"literal_undefined_key",
		"<?php\n$a = [$uk => 1];\necho $a[\"\"], \"\\n\";\n",
		"\nWarning: Undefined variable $uk in /w/cases/literal_undefined_key.php on line 2\n1\n", 0},
	{"literal_null_variable_key",
		"<?php\n$n = null;\n$b = [$n => 2];\necho $b[\"\"], \"\\n\";\n",
		"\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/literal_null_variable_key.php on line 3\n2\n", 0},
	{"literal_constant_keys",
		"<?php\necho \"a\\n\";\n$a = [null => 1, true => 2, false => 3, 1.5 => 4, \"08\" => 5, \"8\" => 6, 2.0 => 7];\necho $a[\"\"], $a[1], $a[0], $a[\"08\"], $a[8], $a[2], \"\\n\";\n",
		"a\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/literal_constant_keys.php on line 3\n\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/literal_constant_keys.php on line 3\n143567\n", 0},
	{"literal_variable_keys",
		"<?php\n$k = 1.5;\n$a = [$k => 1];\necho $a[1], \"\\n\";\n$k = true;\n$a = [$k => 1, \"x\" => 2, \"x\" => 3, \"9\" => 4, \"-0\" => 5];\necho $a[1], $a[\"x\"], $a[9], $a[\"-0\"], \"\\n\";\n",
		"\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/literal_variable_keys.php on line 3\n1\n1345\n", 0},
	{"literal_array_key_variable",
		"<?php\n$k = [];\n$a = [$k => 1];\n",
		"\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/literal_array_key_variable.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/literal_array_key_variable.php on line 3\n", 255},
	{"literal_array_key_constant_dynamic_value",
		"<?php\necho \"a\\n\";\n$x = 1;\n$a = [[] => $x];\n",
		"a\n\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/literal_array_key_constant_dynamic_value.php:4\nStack trace:\n#0 {main}\n  thrown in /w/cases/literal_array_key_constant_dynamic_value.php on line 4\n", 255},
	{"literal_array_key_constant",
		"<?php\necho \"a\\n\";\n$a = [[1] => 1];\n",
		"\nFatal error: Illegal offset type in /w/cases/literal_array_key_constant.php on line 3\nStack trace:\n#0 {main}\n", 255},
	{"literal_overflow_key",
		"<?php\n$k = 9223372036854775807;\n$a = [$k => 1, 2];\n",
		"\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/literal_overflow_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/literal_overflow_key.php on line 3\n", 255},
	{"literal_overflow_constant",
		"<?php\necho \"a\\n\";\n$x = 1;\n$a = [$x, 9223372036854775807 => 1, 2];\n",
		"a\n\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/literal_overflow_constant.php:4\nStack trace:\n#0 {main}\n  thrown in /w/cases/literal_overflow_constant.php on line 4\n", 255},
	{"literal_negative_next",
		"<?php\n$k = -5;\n$a = [$k => 1, 2];\necho $a[-5], $a[-4], \"\\n\";\n",
		"12\n", 0},
	{"literal_value_before_key",
		"<?php\n$a = [$u1 => $u2];\n$b = [$u3, $u4 => $u5];\necho \"end\\n\";\n",
		"\nWarning: Undefined variable $u2 in /w/cases/literal_value_before_key.php on line 2\n\nWarning: Undefined variable $u1 in /w/cases/literal_value_before_key.php on line 2\n\nWarning: Undefined variable $u3 in /w/cases/literal_value_before_key.php on line 3\n\nWarning: Undefined variable $u5 in /w/cases/literal_value_before_key.php on line 3\n\nWarning: Undefined variable $u4 in /w/cases/literal_value_before_key.php on line 3\nend\n", 0},
	{"literal_key_read_late",
		"<?php\n$k = 1;\n$a = [$k => $k++, $k];\necho $a[2], $a[3], \"\\n\";\n",
		"12\n", 0},
	{"literal_nested",
		"<?php\n$a = [[$u], \"k\" => [1, [2, \"z\" => [3]]]];\necho $a[\"k\"][1][\"z\"][0], \"\\n\";\n$b = [[], [[]]];\necho isset($b[1][0]) ? \"y\" : \"n\", \"\\n\";\n",
		"\nWarning: Undefined variable $u in /w/cases/literal_nested.php on line 2\n3\ny\n", 0},
	{"literal_float_big_key",
		"<?php\n$x = 1;\n$b = [1e30 => $x];\necho \"end\\n\";\n",
		"\nWarning: The float 1.0E+30 is not representable as an int, cast occurred in /w/cases/literal_float_big_key.php on line 3\nend\n", 0},
	{"discard_bare_variable",
		"<?php\n$u;\n($u);\n$u[\"k\"];\necho \"|\\n\";\n",
		"\nWarning: Undefined variable $u in /w/cases/discard_bare_variable.php on line 4\n\nWarning: Trying to access array offset on null in /w/cases/discard_bare_variable.php on line 4\n|\n", 0},
	{"discard_other_forms",
		"<?php\n$a + 1;\n-$u;\n(int)$u;\n$u ?: 1;\n$u . \"x\";\n\"x$u\";\n\"$u\";\n$u == 1;\n$u && 1;\n$u ?? 1;\nisset($u);\n[$u];\n$u === null;\nprint $u;\necho \"|\\n\";\n",
		"\nWarning: Undefined variable $a in /w/cases/discard_other_forms.php on line 2\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 3\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 4\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 5\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 6\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 7\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 8\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 9\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 10\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 13\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 14\n\nWarning: Undefined variable $u in /w/cases/discard_other_forms.php on line 15\n|\n", 0},
	{"discard_for_parts",
		"<?php\nfor ($u1; $u2, $i < 1; $u3, $i++) { echo \"b\"; }\necho \"|\\n\";\nfor ($j = 0; $u, $j < 1; $j++) {}\necho \"|\\n\";\n",
		"\nWarning: Undefined variable $i in /w/cases/discard_for_parts.php on line 2\nb\nWarning: Undefined variable $i in /w/cases/discard_for_parts.php on line 2\n|\n|\n", 0},
	{"coalesce_assign_lines",
		"<?php\n$a\n ??=\n\n $u;\necho \"|\\n\";\n$b\n =\n\n $u;\n$c\n ??=\n\n -$u;\n$d = 1;\n$d\n ??=\n $u;\nfunction f() { echo \"[f]\"; }\n$e\n ??=\n f(\n);\necho \"\\n\";\n$g\n ??=\n \"x$u y\n $u\";\n",
		"\nWarning: Undefined variable $u in /w/cases/coalesce_assign_lines.php on line 2\n|\n\nWarning: Undefined variable $u in /w/cases/coalesce_assign_lines.php on line 7\n\nWarning: Undefined variable $u in /w/cases/coalesce_assign_lines.php on line 14\n[f]\n\nWarning: Undefined variable $u in /w/cases/coalesce_assign_lines.php on line 27\n\nWarning: Undefined variable $u in /w/cases/coalesce_assign_lines.php on line 28\n", 0},
}

func TestW17ReviewProbes(t *testing.T) {
	e, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range w17ReviewProbes {
		t.Run(c.name, func(t *testing.T) {
			got := w17Execute(t, e, c.name, c.src, WithINI("display_errors", "1"), WithINI("log_errors", "0"),
				WithINI("html_errors", "0"), WithINI("error_reporting", "-1"))
			if got.err != nil && got.compileFatal == nil && got.exitCode != 255 {
				t.Fatalf("run failed: %v", got.err)
			}
			w17Compare(t, "stdout", []byte(c.stdout), got.stdout)
			if got.exitCode != c.exit {
				t.Errorf("exit code %d, want %d (error %v)", got.exitCode, c.exit, got.err)
			}
		})
	}
}
