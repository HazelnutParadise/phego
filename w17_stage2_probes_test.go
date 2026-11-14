package phego

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// W-17 stage 2: REQ-025, REQ-026; T-009, T-010. Element writes (compound
// assignment, ??=, ++/--, nested writes and unsets, string offset writes),
// foreach by value, parameter defaults and string offset reads. Every
// expectation is what the pinned PHP 8.5.12 images printed for the source
// saved as /w/cases/<name>.php, the path this test compiles under, with stdin
// closed and /w as the working directory:
//
//	docker run --rm --pull=never --network none --cap-drop ALL -w /w -v <dir>:/w:ro \
//	  php@sha256:add6a138b6bd07edd645088917c081579f6e0d52d3c4361aff9d9309e6ebf9d9 \
//	  php -n -d display_errors=1 -d log_errors=0 -d error_log= -d error_reporting=-1 -d html_errors=0 /w/cases/<name>.php
//
// stdout and exit are that run on linux/arm64 (64-bit integers). logOut and
// logErr are stdout and stderr of the same image with display_errors=0 and
// log_errors=1. stdout32 and exit32 are the display run of
// i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049
// (--platform linux/386), recorded only where differs32 says it differs
// from the 64-bit run.
type w17Stage2Probe struct {
	name, src                        string
	stdout, stdout32, logOut, logErr string
	exit, exit32                     int
	differs32                        bool
}

var w17Stage2Probes = []w17Stage2Probe{
	{name: "s2_assign_dim_result_chain",
		src:    "<?php\n$a = $b[\"x\"] = $c[1][2] = \"v\";\necho $a, $b[\"x\"], $c[1][2], \"\\n\";\n$d = [];\n$e = $d[] = $d[] = 5;\necho $e, $d[0], $d[1], \"\\n\";\n$k = \"kk\";\n$v = \"vv\";\n$r = $g[$k] = $v;\necho $r, $g[\"kk\"], \"\\n\";\n$r2 = $h[$uk] = $uv;\necho $r2 === null ? \"null\" : \"other\", \"\\n\";\nif ($m[\"t\"] = 0) echo \"never\"; else echo \"falsy\\n\";\nfunction id($x) { return $x; }\necho id($n[0] = \"arg\"), $n[0], \"\\n\";\n",
		stdout: "vvv\n555\nvvvv\n\nWarning: Undefined variable $uk in /w/cases/s2_assign_dim_result_chain.php on line 11\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_assign_dim_result_chain.php on line 11\n\nWarning: Undefined variable $uv in /w/cases/s2_assign_dim_result_chain.php on line 11\nnull\nfalsy\nargarg\n",
		logOut: "vvv\n555\nvvvv\nnull\nfalsy\nargarg\n",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_assign_dim_result_chain.php on line 11\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_assign_dim_result_chain.php on line 11\nPHP Warning:  Undefined variable $uv in /w/cases/s2_assign_dim_result_chain.php on line 11\n",
		exit:   0},
	{name: "s2_cas_append",
		src:    "<?php\n$a = [];\n$a[] += 1;\n$a[] .= \"x\";\n$a[] -= $uv;\necho $a[0], $a[1], $a[2], \"\\n\";\n$n = null;\n$n[] .= \"q\";\necho $n[0], \"\\n\";\n$f = false;\n$f[] *= 3;\necho $f[0], \"\\n\";\n",
		stdout: "\nWarning: Undefined variable $uv in /w/cases/s2_cas_append.php on line 5\n1x0\nq\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_cas_append.php on line 11\n0\n",
		logOut: "1x0\nq\n0\n",
		logErr: "PHP Warning:  Undefined variable $uv in /w/cases/s2_cas_append.php on line 5\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_cas_append.php on line 11\n",
		exit:   0},
	{name: "s2_cas_append_overflow",
		src:      "<?php\n$a = [9223372036854775807 => 1];\n$a[] += $uv;\necho \"end\\n\";\n",
		stdout:   "\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/s2_cas_append_overflow.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_append_overflow.php on line 3\n",
		stdout32: "\nWarning: The float 9.223372036854776E+18 is not representable as an int, cast occurred in /w/cases/s2_cas_append_overflow.php on line 2\n\nWarning: Undefined variable $uv in /w/cases/s2_cas_append_overflow.php on line 3\nend\n", exit32: 0, differs32: true,
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/s2_cas_append_overflow.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_append_overflow.php on line 3\n",
		exit:   255},
	{name: "s2_cas_array_key",
		src:    "<?php\n$a = [];\n$a[[]] += $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_cas_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_cas_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_cas_array_key_rw_undefined",
		src:    "<?php\n$a = [];\n$a[1.5] .= $uv;\n$a[true] .= \"t\";\n$a[\"7\"] .= \"s\";\n$a[null] .= \"n\";\n$a[false] += 2;\necho $a[1], $a[7], $a[\"\"], $a[0], \"\\n\";\n",
		stdout: "\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\n\nWarning: Undefined array key 1 in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\n\nWarning: Undefined array key 7 in /w/cases/s2_cas_array_key_rw_undefined.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_array_key_rw_undefined.php on line 6\n\nWarning: Undefined array key \"\" in /w/cases/s2_cas_array_key_rw_undefined.php on line 6\n\nWarning: Undefined array key 0 in /w/cases/s2_cas_array_key_rw_undefined.php on line 7\ntsn2\n",
		logOut: "tsn2\n",
		logErr: "PHP Deprecated:  Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\nPHP Warning:  Undefined array key 1 in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\nPHP Warning:  Undefined variable $uv in /w/cases/s2_cas_array_key_rw_undefined.php on line 3\nPHP Warning:  Undefined array key 7 in /w/cases/s2_cas_array_key_rw_undefined.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_array_key_rw_undefined.php on line 6\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_cas_array_key_rw_undefined.php on line 6\nPHP Warning:  Undefined array key 0 in /w/cases/s2_cas_array_key_rw_undefined.php on line 7\n",
		exit:   0},
	{name: "s2_cas_div_zero",
		src:    "<?php\n$a = [1];\n$a[0] %= 0;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught DivisionByZeroError: Modulo by zero in /w/cases/s2_cas_div_zero.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_div_zero.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught DivisionByZeroError: Modulo by zero in /w/cases/s2_cas_div_zero.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_div_zero.php on line 3\n",
		exit:   255},
	{name: "s2_cas_lines",
		src:    "<?php\n$a = [];\n$a\n[$u1]\n+=\n$u2;\necho \"|\\n\";\n$a[\n\"q\"\n] .=\n\"x\"\n;\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_cas_lines.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_lines.php on line 4\n\nWarning: Undefined array key \"\" in /w/cases/s2_cas_lines.php on line 4\n\nWarning: Undefined variable $u2 in /w/cases/s2_cas_lines.php on line 4\n|\n\nWarning: Undefined array key \"q\" in /w/cases/s2_cas_lines.php on line 9\n|\n",
		logOut: "|\n|\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_cas_lines.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_lines.php on line 4\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_cas_lines.php on line 4\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_cas_lines.php on line 4\nPHP Warning:  Undefined array key \"q\" in /w/cases/s2_cas_lines.php on line 9\n",
		exit:   0},
	{name: "s2_cas_ops",
		src:    "<?php\n$a = [10, \"s\", 7, 2.5, 6, 3, 12, 5, 1, 9];\n$a[0] += 5; $a[0] -= 3; $a[0] *= 2; $a[0] /= 4; $a[0] %= 4;\n$a[1] .= \"t\"; $a[2] **= 2; $a[3] += 1; $a[4] <<= 2; $a[5] >>= 1; $a[6] &= 10; $a[7] |= 8; $a[8] ^= 3;\n$a[\"k\"] = 1; $a[\"k\"] += 1.5;\necho $a[0], \"|\", $a[1], \"|\", $a[2], \"|\", $a[3], \"|\", $a[4], \"|\", $a[5], \"|\", $a[6], \"|\", $a[7], \"|\", $a[8], \"|\", $a[9], \"|\", $a[\"k\"], \"\\n\";\n",
		stdout: "2|st|49|3.5|24|1|8|13|2|9|2.5\n",
		logOut: "2|st|49|3.5|24|1|8|13|2|9|2.5\n",
		logErr: "",
		exit:   0},
	{name: "s2_cas_order_calls",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$a = [1 => 10, 2 => 20];\n$a[f(1)] += f(5);\n$a[f(2)] .= f(\"z\");\necho $a[1], $a[2], \"\\n\";\n$i = 0;\n$b = [0, 0, 0];\n$b[$i++] += $i++;\necho $b[0], $b[1], $b[2], $i, \"\\n\";\n$k = 1;\n$c = [1 => 1, 2 => 2];\n$c[$k] += $k++;\necho $c[1], $c[2], \"\\n\";\n",
		stdout: "[f1][f5][f2][fz]1520z\n1002\n13\n",
		logOut: "[f1][f5][f2][fz]1520z\n1002\n13\n",
		logErr: "",
		exit:   0},
	{name: "s2_cas_order_undefined",
		src:    "<?php\n$a = [];\n$a[$uk] .= $uv;\necho \"|\\n\";\n$u2[$uk2] += $uv2;\necho \"|\\n\";\n$f = false;\n$f[$uk3] -= $uv3;\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_cas_order_undefined.php on line 3\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 3\n\nWarning: Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/s2_cas_order_undefined.php on line 3\n|\n\nWarning: Undefined variable $u2 in /w/cases/s2_cas_order_undefined.php on line 5\n\nWarning: Undefined variable $uk2 in /w/cases/s2_cas_order_undefined.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 5\n\nWarning: Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 5\n\nWarning: Undefined variable $uv2 in /w/cases/s2_cas_order_undefined.php on line 5\n|\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_cas_order_undefined.php on line 8\n\nWarning: Undefined variable $uk3 in /w/cases/s2_cas_order_undefined.php on line 8\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 8\n\nWarning: Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 8\n\nWarning: Undefined variable $uv3 in /w/cases/s2_cas_order_undefined.php on line 8\n|\n",
		logOut: "|\n|\n|\n",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_cas_order_undefined.php on line 3\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 3\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 3\nPHP Warning:  Undefined variable $uv in /w/cases/s2_cas_order_undefined.php on line 3\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_cas_order_undefined.php on line 5\nPHP Warning:  Undefined variable $uk2 in /w/cases/s2_cas_order_undefined.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 5\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 5\nPHP Warning:  Undefined variable $uv2 in /w/cases/s2_cas_order_undefined.php on line 5\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_cas_order_undefined.php on line 8\nPHP Warning:  Undefined variable $uk3 in /w/cases/s2_cas_order_undefined.php on line 8\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_cas_order_undefined.php on line 8\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_cas_order_undefined.php on line 8\nPHP Warning:  Undefined variable $uv3 in /w/cases/s2_cas_order_undefined.php on line 8\n",
		exit:   0},
	{name: "s2_cas_result",
		src:    "<?php\n$a = [1, \"a\"];\n$x = ($a[0] += 5);\necho $x, \"\\n\";\necho ($a[1] .= \"z\"), \"\\n\";\necho ($a[\"new\"] ??= \"n\"), ($a[\"new\"] ??= \"m\"), \"\\n\";\n$y = $a[0] *= 2;\necho $y, $a[0], \"\\n\";\n",
		stdout: "6\naz\nnn\n1212\n",
		logOut: "6\naz\nnn\n1212\n",
		logErr: "",
		exit:   0},
	{name: "s2_cas_scalar_append",
		src:    "<?php\n$i = 5;\n$i[] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_scalar_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_scalar_append.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_scalar_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_scalar_append.php on line 3\n",
		exit:   255},
	{name: "s2_cas_scalar_container",
		src:    "<?php\n$i = 5;\n$i[$uk] .= \"x\";\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_cas_scalar_container.php on line 3\n\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_scalar_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_scalar_container.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_cas_scalar_container.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_scalar_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_scalar_container.php on line 3\n",
		exit:   255},
	{name: "s2_cas_self",
		src:    "<?php\n$a = [[1], 2];\n$a[0] += $a;\necho isset($a[0][1]) ? \"y\" : \"n\", $a[0][0], \"\\n\";\n$b = [1];\n$b[] .= $b[0];\necho $b[1], \"\\n\";\n$c = [\"x\"];\n$c[0] .= $c[0];\necho $c[0], \"\\n\";\n",
		stdout: "y1\n1\nxx\n",
		logOut: "y1\n1\nxx\n",
		logErr: "",
		exit:   0},
	{name: "s2_cas_self_base",
		src:    "<?php\n$d[1] .= $d;\necho \"|\\n\";\n$a = [[1]];\n$a[0] += $a;\necho isset($a[0][1]) ? \"y\" : \"n\", \"\\n\";\n$b = [\"x\" => \"s\"];\n$b[\"x\"] .= $b[\"x\"];\n$b[\"n\"][\"m\"] ??= $b;\nforeach ($b[\"n\"][\"m\"] as $k => $v) echo $k, \",\";\necho $b[\"x\"], \"\\n\";\n$u[\"p\"] ??= $u;\necho $u[\"p\"] === null ? \"null\" : \"other\", \"\\n\";\n",
		stdout: "\nWarning: Undefined variable $d in /w/cases/s2_cas_self_base.php on line 2\n\nWarning: Undefined variable $d in /w/cases/s2_cas_self_base.php on line 2\n\nWarning: Undefined array key 1 in /w/cases/s2_cas_self_base.php on line 2\n|\nn\nx,ss\n\nWarning: Undefined variable $u in /w/cases/s2_cas_self_base.php on line 12\nnull\n",
		logOut: "|\nn\nx,ss\nnull\n",
		logErr: "PHP Warning:  Undefined variable $d in /w/cases/s2_cas_self_base.php on line 2\nPHP Warning:  Undefined variable $d in /w/cases/s2_cas_self_base.php on line 2\nPHP Warning:  Undefined array key 1 in /w/cases/s2_cas_self_base.php on line 2\nPHP Warning:  Undefined variable $u in /w/cases/s2_cas_self_base.php on line 12\n",
		exit:   0},
	{name: "s2_cas_string_append",
		src:    "<?php\n$s = \"abc\";\n$s[] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: [] operator not supported for strings in /w/cases/s2_cas_string_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_append.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: [] operator not supported for strings in /w/cases/s2_cas_string_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_append.php on line 3\n",
		exit:   255},
	{name: "s2_cas_string_container",
		src:    "<?php\n$s = \"abc\";\n$s[$uk] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_cas_string_container.php on line 3\n\nWarning: String offset cast occurred in /w/cases/s2_cas_string_container.php on line 3\n\nFatal error: Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_cas_string_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_container.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_cas_string_container.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_cas_string_container.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_cas_string_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_container.php on line 3\n",
		exit:   255},
	{name: "s2_cas_string_container_badkey",
		src:    "<?php\n$s = \"abc\";\n$s[\"x\"] .= \"y\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_cas_string_container_badkey.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_container_badkey.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_cas_string_container_badkey.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_container_badkey.php on line 3\n",
		exit:   255},
	{name: "s2_cas_string_key_array",
		src:    "<?php\n$s = \"abc\";\n$s[[]] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_cas_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_key_array.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_cas_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_key_array.php on line 3\n",
		exit:   255},
	{name: "s2_cas_string_key_leading",
		src:    "<?php\n$s = \"abc\";\n$s[\"1x\"] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nWarning: Illegal string offset \"1x\" in /w/cases/s2_cas_string_key_leading.php on line 3\n\nFatal error: Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_cas_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_key_leading.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_cas_string_key_leading.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_cas_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_string_key_leading.php on line 3\n",
		exit:   255},
	{name: "s2_cas_true_container",
		src:    "<?php\n$t = true;\n$t[0] += $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_true_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_true_container.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_cas_true_container.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_true_container.php on line 3\n",
		exit:   255},
	{name: "s2_cas_types",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$a = [\"5\", 2, null, true, \"abc\", 1.5];\n$a[0] += 1; $a[1] /= 4; $a[2] .= \"x\"; $a[3] += 1; $a[5] -= 0.5;\necho d($a[0]), d($a[1]), d($a[2]), d($a[3]), d($a[5]), \"\\n\";\n$a[4] += 1;\necho \"end\\n\";\n",
		stdout: "int(6)float(0.5)str(x)int(2)float(1)\n\nFatal error: Uncaught TypeError: Unsupported operand types: string + int in /w/cases/s2_cas_types.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_types.php on line 7\n",
		logOut: "int(6)float(0.5)str(x)int(2)float(1)\n",
		logErr: "PHP Fatal error:  Uncaught TypeError: Unsupported operand types: string + int in /w/cases/s2_cas_types.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_cas_types.php on line 7\n",
		exit:   255},
	{name: "s2_cas_undefined_key",
		src:    "<?php\n$a = [];\n$a[\"k\"] += 5;\n$a[3] .= \"s\";\n$a[-2] -= 1;\necho $a[\"k\"], $a[3], $a[-2], \"\\n\";\n$u[\"x\"] -= 1;\necho $u[\"x\"], \"\\n\";\n$n = null;\n$n[0] *= 2;\necho $n[0], \"\\n\";\n$f = false;\n$f[\"a\"] .= \"z\";\necho $f[\"a\"], \"\\n\";\n",
		stdout: "\nWarning: Undefined array key \"k\" in /w/cases/s2_cas_undefined_key.php on line 3\n\nWarning: Undefined array key 3 in /w/cases/s2_cas_undefined_key.php on line 4\n\nWarning: Undefined array key -2 in /w/cases/s2_cas_undefined_key.php on line 5\n5s-1\n\nWarning: Undefined variable $u in /w/cases/s2_cas_undefined_key.php on line 7\n\nWarning: Undefined array key \"x\" in /w/cases/s2_cas_undefined_key.php on line 7\n-1\n\nWarning: Undefined array key 0 in /w/cases/s2_cas_undefined_key.php on line 10\n0\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_cas_undefined_key.php on line 13\n\nWarning: Undefined array key \"a\" in /w/cases/s2_cas_undefined_key.php on line 13\nz\n",
		logOut: "5s-1\n-1\n0\nz\n",
		logErr: "PHP Warning:  Undefined array key \"k\" in /w/cases/s2_cas_undefined_key.php on line 3\nPHP Warning:  Undefined array key 3 in /w/cases/s2_cas_undefined_key.php on line 4\nPHP Warning:  Undefined array key -2 in /w/cases/s2_cas_undefined_key.php on line 5\nPHP Warning:  Undefined variable $u in /w/cases/s2_cas_undefined_key.php on line 7\nPHP Warning:  Undefined array key \"x\" in /w/cases/s2_cas_undefined_key.php on line 7\nPHP Warning:  Undefined array key 0 in /w/cases/s2_cas_undefined_key.php on line 10\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_cas_undefined_key.php on line 13\nPHP Warning:  Undefined array key \"a\" in /w/cases/s2_cas_undefined_key.php on line 13\n",
		exit:   0},
	{name: "s2_coalesce_append",
		src:    "<?php\necho \"a\\n\";\n$c = [];\n$c[] ??= 4;\n",
		stdout: "\nFatal error: Cannot use [] for reading in /w/cases/s2_coalesce_append.php on line 4\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for reading in /w/cases/s2_coalesce_append.php on line 4\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_coalesce_append_mid",
		src:    "<?php\necho \"a\\n\";\n$c = [];\n$c[][1] ??= 4;\n",
		stdout: "\nFatal error: Cannot use [] for reading in /w/cases/s2_coalesce_append_mid.php on line 4\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for reading in /w/cases/s2_coalesce_append_mid.php on line 4\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_coalesce_dim",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$a = [\"x\" => null, \"y\" => 0];\n$a[\"x\"] ??= 1;\n$a[\"y\"] ??= 2;\n$a[\"z\"] ??= 3;\necho $a[\"x\"], $a[\"y\"], $a[\"z\"], \"\\n\";\n$a[f(\"p\")] ??= f(7);\n$a[f(\"p\")] ??= f(8);\necho $a[\"p\"], \"\\n\";\n$b[\"m\"][\"n\"] ??= f(9);\n$b[\"m\"][\"n\"] ??= f(10);\necho $b[\"m\"][\"n\"], \"\\n\";\n$c = [[1 => null]];\n$c[f(0)][f(1)] ??= f(\"w\");\necho $c[0][1], \"\\n\";\n",
		stdout: "103\n[fp][f7][fp]7\n[f9]9\n[f0][f1][fw]w\n",
		logOut: "103\n[fp][f7][fp]7\n[f9]9\n[f0][f1][fw]w\n",
		logErr: "",
		exit:   0},
	{name: "s2_coalesce_dim_lines",
		src:    "<?php\n$a\n [$u1]\n ??=\n\n $u2;\necho \"|\\n\";\n$b\n [\"k\"]\n ??=\n \"x$u3\";\necho $b[\"k\"], \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_coalesce_dim_lines.php on line 3\n\nWarning: Undefined variable $u1 in /w/cases/s2_coalesce_dim_lines.php on line 3\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_lines.php on line 3\n\nWarning: Undefined variable $u2 in /w/cases/s2_coalesce_dim_lines.php on line 3\n|\n\nWarning: Undefined variable $u3 in /w/cases/s2_coalesce_dim_lines.php on line 11\nx|\n",
		logOut: "|\nx|\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_coalesce_dim_lines.php on line 3\nPHP Warning:  Undefined variable $u1 in /w/cases/s2_coalesce_dim_lines.php on line 3\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_lines.php on line 3\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_coalesce_dim_lines.php on line 3\nPHP Warning:  Undefined variable $u3 in /w/cases/s2_coalesce_dim_lines.php on line 11\n",
		exit:   0},
	{name: "s2_coalesce_dim_scalar",
		src:    "<?php\n$i = 5;\n$i[\"a\"] ??= 1;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_coalesce_dim_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_dim_scalar.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_coalesce_dim_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_dim_scalar.php on line 3\n",
		exit:   255},
	{name: "s2_coalesce_dim_string",
		src:    "<?php\n$s = \"abc\";\n$s[0] ??= \"z\";\necho $s, \"\\n\";\n$s[7] ??= \"q\";\necho $s, \"|\\n\";\n$s[\"x\"] ??= \"w\";\necho \"end\\n\";\n",
		stdout: "abc\nabc    q|\n\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_coalesce_dim_string.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_dim_string.php on line 7\n",
		logOut: "abc\nabc    q|\n",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_coalesce_dim_string.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_dim_string.php on line 7\n",
		exit:   255},
	{name: "s2_coalesce_dim_undefined",
		src:    "<?php\n$u[$uk] ??= $uv;\necho \"|\\n\";\n$a = [];\n$a[$k1][$k2] ??= $v2;\necho \"|\\n\";\n$n = null;\n$n[\"a\"] ??= 1;\n$f = false;\n$f[\"a\"] ??= 2;\necho $n[\"a\"], $f[\"a\"], \"\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_coalesce_dim_undefined.php on line 2\n\nWarning: Undefined variable $uk in /w/cases/s2_coalesce_dim_undefined.php on line 2\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 2\n\nWarning: Undefined variable $uv in /w/cases/s2_coalesce_dim_undefined.php on line 2\n|\n\nWarning: Undefined variable $k1 in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nWarning: Undefined variable $k2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nWarning: Undefined variable $k1 in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nWarning: Undefined variable $k2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\n\nWarning: Undefined variable $v2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\n|\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_coalesce_dim_undefined.php on line 10\n12\n",
		logOut: "|\n|\n12\n",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_coalesce_dim_undefined.php on line 2\nPHP Warning:  Undefined variable $uk in /w/cases/s2_coalesce_dim_undefined.php on line 2\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 2\nPHP Warning:  Undefined variable $uv in /w/cases/s2_coalesce_dim_undefined.php on line 2\nPHP Warning:  Undefined variable $k1 in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Warning:  Undefined variable $k2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Warning:  Undefined variable $k1 in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Warning:  Undefined variable $k2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Warning:  Undefined variable $v2 in /w/cases/s2_coalesce_dim_undefined.php on line 5\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_coalesce_dim_undefined.php on line 10\n",
		exit:   0},
	{name: "s2_coalesce_key_reread",
		src:    "<?php\n$k = \"a\";\n$a = [];\n$a[$k] ??= ($k = \"b\");\necho isset($a[\"a\"]) ? \"a\" : \"-\", isset($a[\"b\"]) ? \"b\" : \"-\", \"\\n\";\n$i = 0;\n$c = [];\n$c[$i++] ??= $i;\necho isset($c[0]) ? \"0\" : \"-\", isset($c[1]) ? \"1\" : \"-\", $i, \"\\n\";\n",
		stdout: "-b\n0-1\n",
		logOut: "-b\n0-1\n",
		logErr: "",
		exit:   0},
	{name: "s2_coalesce_self",
		src:    "<?php\n$a = [1];\n$a[\"k\"] ??= $a;\nforeach ($a[\"k\"] as $kk => $vv) { echo $kk, \",\"; }\necho isset($a[\"k\"][\"k\"]) ? \"rec\" : \"flat\", \"\\n\";\n$b = [1];\n$b[\"x\"][\"y\"] ??= $b;\nforeach ($b[\"x\"][\"y\"] as $kk => $vv) { echo $kk, \",\"; }\necho \"\\n\";\n$c = [1];\n$c[\"n\"] .= $c[0];\nforeach ($c as $kk => $vv) { echo $kk, \"=\", $vv, \",\"; }\necho \"\\n\";\n$d = [[1]];\n$d[\"z\"] += $d;\necho \"never\\n\";\n",
		stdout: "0,flat\n0,\n\nWarning: Undefined array key \"n\" in /w/cases/s2_coalesce_self.php on line 11\n0=1,n=1,\n\nWarning: Undefined array key \"z\" in /w/cases/s2_coalesce_self.php on line 15\n\nFatal error: Uncaught TypeError: Unsupported operand types: null + array in /w/cases/s2_coalesce_self.php:15\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_self.php on line 15\n",
		logOut: "0,flat\n0,\n0=1,n=1,\n",
		logErr: "PHP Warning:  Undefined array key \"n\" in /w/cases/s2_coalesce_self.php on line 11\nPHP Warning:  Undefined array key \"z\" in /w/cases/s2_coalesce_self.php on line 15\nPHP Fatal error:  Uncaught TypeError: Unsupported operand types: null + array in /w/cases/s2_coalesce_self.php:15\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_self.php on line 15\n",
		exit:   255},
	{name: "s2_coalesce_string_nested",
		src:    "<?php\n$s = \"abc\";\n$s[0][0] ??= \"z\";\necho $s, \"\\n\";\n$a = [\"str\"];\n$a[0][1] ??= \"q\";\n$a[0][4] ??= \"w\";\necho $a[0], \"|\\n\";\n$s[9][0] ??= \"z\";\necho \"end\\n\";\n",
		stdout: "abc\nstr w|\n\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_coalesce_string_nested.php:9\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_string_nested.php on line 9\n",
		logOut: "abc\nstr w|\n",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_coalesce_string_nested.php:9\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_coalesce_string_nested.php on line 9\n",
		exit:   255},
	{name: "s2_def_basic",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\nfunction f($a, $b = 5, $c = \"s\", $d = null, $e = true, $g = 1.5, $h = [], $i = [1, \"k\" => [2, 3]], $j = -7) {\n    return d($a) . d($b) . d($c) . d($d) . d($e) . d($g) . d($h) . d($i) . d($j) . (isset($i[\"k\"][1]) ? $i[\"k\"][1] : \"?\");\n}\necho f(1), \"\\n\";\necho f(1, 2), \"\\n\";\necho f(1, null, null, 0, false, 2, \"h\", \"i\", \"j\"), \"\\n\";\necho f(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11), \"\\n\";\nfunction g($x = 1 + 2 * 3, $y = \"a\" . \"b\", $z = !true, $w = 1 ? \"t\" : \"f\", $v = null ?? \"c\", $u = [1, 2][1], $t = -(-4), $s = (int)\"12\") { return d($x) . d($y) . d($z) . d($w) . d($v) . d($u) . d($t) . d($s); }\necho g(), \"\\n\";\nfunction h($a = 1, $b = 2) { return \"$a$b\"; }\necho h(), h(9), h(9, 8), h(9, 8, 7), \"\\n\";\n",
		stdout: "\nWarning: Array to string conversion in /w/cases/s2_def_basic.php on line 2\n\nWarning: Array to string conversion in /w/cases/s2_def_basic.php on line 2\nint(1)int(5)str(s)nulltruefloat(1.5)arrayarrayint(-7)3\n\nWarning: Array to string conversion in /w/cases/s2_def_basic.php on line 2\n\nWarning: Array to string conversion in /w/cases/s2_def_basic.php on line 2\nint(1)int(2)str(s)nulltruefloat(1.5)arrayarrayint(-7)3\nint(1)nullnullint(0)falseint(2)str(h)str(i)str(j)?\nint(1)int(2)int(3)int(4)int(5)int(6)int(7)int(8)int(9)?\nint(7)str(ab)falsestr(t)str(c)int(2)int(4)int(12)\n12929898\n",
		logOut: "int(1)int(5)str(s)nulltruefloat(1.5)arrayarrayint(-7)3\nint(1)int(2)str(s)nulltruefloat(1.5)arrayarrayint(-7)3\nint(1)nullnullint(0)falseint(2)str(h)str(i)str(j)?\nint(1)int(2)int(3)int(4)int(5)int(6)int(7)int(8)int(9)?\nint(7)str(ab)falsestr(t)str(c)int(2)int(4)int(12)\n12929898\n",
		logErr: "PHP Warning:  Array to string conversion in /w/cases/s2_def_basic.php on line 2\nPHP Warning:  Array to string conversion in /w/cases/s2_def_basic.php on line 2\nPHP Warning:  Array to string conversion in /w/cases/s2_def_basic.php on line 2\nPHP Warning:  Array to string conversion in /w/cases/s2_def_basic.php on line 2\n",
		exit:   0},
	{name: "s2_def_case_and_recursion",
		src:    "<?php\nfunction Fact($n, $acc = 1) { return $n <= 1 ? $acc : FACT($n - 1, $acc * $n); }\necho fact(5), Fact(5, 2), \"\\n\";\nfunction cnt($xs = [1, 2, 3]) { $n = 0; foreach ($xs as $x) $n++; return $n; }\necho cnt(), cnt([]), cnt([1]), \"\\n\";\n",
		stdout: "120240\n301\n",
		logOut: "120240\n301\n",
		logErr: "",
		exit:   0},
	{name: "s2_def_cached_first_omitted",
		src:    "<?php\nfunction f($a = 1, $b = [5 => 1][6]) { return $b === null ? \"n\" : $b; }\necho \"start\\n\";\necho f(1, 2), f(1, 3), \"\\n\";\necho f(), f(1), f(), \"\\n\";\n",
		stdout: "start\n23\n\nWarning: Undefined array key 6 in /w/cases/s2_def_cached_first_omitted.php on line 2\nnnn\n",
		logOut: "start\n23\nnnn\n",
		logErr: "PHP Warning:  Undefined array key 6 in /w/cases/s2_def_cached_first_omitted.php on line 2\n",
		exit:   0},
	{name: "s2_def_cached_kinds",
		src:    "<?php\nfunction n($v = \"5x\" + 1) { return $v; }\nfunction e($v = [1][5] ?: []) { $v[] = 1; return count_free($v); }\nfunction count_free($a) { $n = 0; foreach ($a as $x) $n++; return $n; }\nfunction o($v = \"abc\"[7] ?: \"abc\"[1]) { return $v; }\nfunction fl($v = [1][5] ?: 1.5) { return $v; }\nfunction bo($v = !\"abc\"[9]) { return $v ? \"T\" : \"F\"; }\necho n(), n(), \"|\", e(), e(), \"|\", o(), o(), \"|\", fl(), fl(), \"|\", bo(), bo(), \"\\n\";\n",
		stdout: "\nWarning: A non-numeric value encountered in /w/cases/s2_def_cached_kinds.php on line 2\n66|\nWarning: Undefined array key 5 in /w/cases/s2_def_cached_kinds.php on line 3\n11|\nWarning: Uninitialized string offset 7 in /w/cases/s2_def_cached_kinds.php on line 5\nbb|\nWarning: Undefined array key 5 in /w/cases/s2_def_cached_kinds.php on line 6\n1.51.5|\nWarning: Uninitialized string offset 9 in /w/cases/s2_def_cached_kinds.php on line 7\nTT\n",
		logOut: "66|11|bb|1.51.5|TT\n",
		logErr: "PHP Warning:  A non-numeric value encountered in /w/cases/s2_def_cached_kinds.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_cached_kinds.php on line 3\nPHP Warning:  Uninitialized string offset 7 in /w/cases/s2_def_cached_kinds.php on line 5\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_cached_kinds.php on line 6\nPHP Warning:  Uninitialized string offset 9 in /w/cases/s2_def_cached_kinds.php on line 7\n",
		exit:   0},
	{name: "s2_def_cached_null",
		src:    "<?php\nfunction f($a, $b = [1, 2][5]) { return $a . ($b === null ? \"N\" : \"?\"); }\necho f(1), f(2), f(3, 4), \"\\n\";\nfunction g($x = \"abc\"[7]) { return \"[$x]\"; }\necho g(\"y\"), \"\\n\";\necho g(), g(), \"\\n\";\n",
		stdout: "\nWarning: Undefined array key 5 in /w/cases/s2_def_cached_null.php on line 2\n1N2N3?\n[y]\n\nWarning: Uninitialized string offset 7 in /w/cases/s2_def_cached_null.php on line 4\n[][]\n",
		logOut: "1N2N3?\n[y]\n[][]\n",
		logErr: "PHP Warning:  Undefined array key 5 in /w/cases/s2_def_cached_null.php on line 2\nPHP Warning:  Uninitialized string offset 7 in /w/cases/s2_def_cached_null.php on line 4\n",
		exit:   0},
	{name: "s2_def_cached_recursion",
		src:    "<?php\nfunction r($n, $tag = [1][9] ?? \"t\", $w = \"ab\"[5]) {\n    if ($n > 0) return r($n - 1) . $n . $w;\n    return \"<$tag>\";\n}\necho r(3), \"\\n\";\necho r(1, \"x\", \"y\"), r(2), \"\\n\";\n",
		stdout: "\nWarning: Uninitialized string offset 5 in /w/cases/s2_def_cached_recursion.php on line 2\n<t>123\n<t>1y<t>12\n",
		logOut: "<t>123\n<t>1y<t>12\n",
		logErr: "PHP Warning:  Uninitialized string offset 5 in /w/cases/s2_def_cached_recursion.php on line 2\n",
		exit:   0},
	{name: "s2_def_uncached_kinds",
		src:    "<?php\nfunction a($v = [[1], [2][5]]) { return $v[1] === null ? \"n\" : \"?\"; }\nfunction s($v = [1][5] ?: \"ab\") { return $v; }\nfunction t($v = [1][5] ?: [1, 2]) { return $v[1]; }\necho a(), a(), \"|\", s(), s(), \"|\", t(), t(), \"\\n\";\n",
		stdout: "\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 2\nn\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 2\nn|\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 3\nab\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 3\nab|\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 4\n2\nWarning: Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 4\n2\n",
		logOut: "nn|abab|22\n",
		logErr: "PHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 3\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 3\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 4\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_uncached_kinds.php on line 4\n",
		exit:   0},
	{name: "s2_def_cached_concat",
		src:    "<?php\nfunction f($a = \"x\" . [1][5], $b = 7 . \"abc\"[9], $c = \"ab\" . [1][6], $d = true . [2][7] . \"\") {\n    return \"[$a|$b|$c|$d]\";\n}\necho f(), \"\\n\", f(), \"\\n\", f(1, 2), \"\\n\";\n",
		stdout: "\nWarning: Undefined array key 5 in /w/cases/s2_def_cached_concat.php on line 2\n\nWarning: Uninitialized string offset 9 in /w/cases/s2_def_cached_concat.php on line 2\n\nWarning: Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\n\nWarning: Undefined array key 7 in /w/cases/s2_def_cached_concat.php on line 2\n[x|7|ab|1]\n\nWarning: Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\n[x|7|ab|1]\n\nWarning: Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\n[1|2|ab|1]\n",
		logOut: "[x|7|ab|1]\n[x|7|ab|1]\n[1|2|ab|1]\n",
		logErr: "PHP Warning:  Undefined array key 5 in /w/cases/s2_def_cached_concat.php on line 2\nPHP Warning:  Uninitialized string offset 9 in /w/cases/s2_def_cached_concat.php on line 2\nPHP Warning:  Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\nPHP Warning:  Undefined array key 7 in /w/cases/s2_def_cached_concat.php on line 2\nPHP Warning:  Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\nPHP Warning:  Undefined array key 6 in /w/cases/s2_def_cached_concat.php on line 2\n",
		exit:   0},
	{name: "s2_def_const_expr_forms",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\nfunction f($a = 10 % 3, $b = 2 ** 10, $c = 7 <=> 3, $d = \"x\" === \"x\", $e = 1 << 4 | 3, $g = ~5 & 0xff, $h = 5 > 3 && 2 < 1, $i = [1 + 1 => \"a\" . \"b\", \"k\" => 3 * 3], $j = 1.5 + 1, $k = 7 / 2, $l = 6 / 2, $m = \"5\" . 5, $n = +\"3\", $o = 0.1 + 0.2, $p = 1 xor 1, $q = (string)12 . (bool)1 . (float)\"1.5\", $r = [1, 2, 3][1] . \"abc\"[2]) {\n  return d($a) . d($b) . d($c) . d($d) . d($e) . d($g) . d($h) . $i[2] . $i[\"k\"] . d($j) . d($k) . d($l) . d($m) . d($n) . d($o) . d($p) . d($q) . d($r);\n}\necho f(), \"\\n\";\n",
		stdout: "int(1)int(1024)int(1)trueint(19)int(250)falseab9float(2.5)float(3.5)int(3)str(55)int(3)float(0.3)falsestr(1211.5)str(2c)\n",
		logOut: "int(1)int(1024)int(1)trueint(19)int(250)falseab9float(2.5)float(3.5)int(3)str(55)int(3)float(0.3)falsestr(1211.5)str(2c)\n",
		logErr: "",
		exit:   0},
	{name: "s2_def_div_zero",
		src:    "<?php\nfunction f($a = 1 % 0) { return $a; }\necho \"a\\n\";\necho f(5), \"\\n\";\necho f(), \"\\n\";\n",
		stdout: "a\n5\n\nFatal error: Uncaught DivisionByZeroError: Modulo by zero in /w/cases/s2_def_div_zero.php:2\nStack trace:\n#0 /w/cases/s2_def_div_zero.php(5): f()\n#1 {main}\n  thrown in /w/cases/s2_def_div_zero.php on line 2\n",
		logOut: "a\n5\n",
		logErr: "PHP Fatal error:  Uncaught DivisionByZeroError: Modulo by zero in /w/cases/s2_def_div_zero.php:2\nStack trace:\n#0 /w/cases/s2_def_div_zero.php(5): f()\n#1 {main}\n  thrown in /w/cases/s2_def_div_zero.php on line 2\n",
		exit:   255},
	{name: "s2_def_lines",
		src:    "<?php\nfunction f(\n  $a,\n  $b\n   =\n   [1, 2][7],\n  $c = \"x\"\n   . [3][\n   9]\n) { return 1; }\nf(1);\necho \"|\\n\";\nfunction g(\n  $p\n  ,\n  $q\n) {}\ng();\n",
		stdout: "\nWarning: Undefined array key 7 in /w/cases/s2_def_lines.php on line 4\n\nWarning: Undefined array key 9 in /w/cases/s2_def_lines.php on line 7\n|\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function g(), 0 passed in /w/cases/s2_def_lines.php on line 18 and exactly 2 expected in /w/cases/s2_def_lines.php:14\nStack trace:\n#0 /w/cases/s2_def_lines.php(18): g()\n#1 {main}\n  thrown in /w/cases/s2_def_lines.php on line 14\n",
		logOut: "|\n",
		logErr: "PHP Warning:  Undefined array key 7 in /w/cases/s2_def_lines.php on line 4\nPHP Warning:  Undefined array key 9 in /w/cases/s2_def_lines.php on line 7\nPHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function g(), 0 passed in /w/cases/s2_def_lines.php on line 18 and exactly 2 expected in /w/cases/s2_def_lines.php:14\nStack trace:\n#0 /w/cases/s2_def_lines.php(18): g()\n#1 {main}\n  thrown in /w/cases/s2_def_lines.php on line 14\n",
		exit:   255},
	{name: "s2_def_mutation",
		src:    "<?php\nfunction f($a = [1, 2], $s = \"ab\") { $a[] = 3; $a[0]++; $s .= \"c\"; return $a[0] . $a[2] . (isset($a[3]) ? \"leak\" : \"\") . $s; }\necho f(), f(), f([7]), f(), \"\\n\";\nfunction r($n, $acc = []) { if ($n == 0) { $o = \"\"; foreach ($acc as $x) $o .= $x; return $o; } $acc[] = $n; return r($n - 1, $acc); }\necho r(3), r(2), \"\\n\";\n",
		stdout: "23abc23abc\nWarning: Undefined array key 2 in /w/cases/s2_def_mutation.php on line 2\n8abc23abc\n32121\n",
		logOut: "23abc23abc8abc23abc\n32121\n",
		logErr: "PHP Warning:  Undefined array key 2 in /w/cases/s2_def_mutation.php on line 2\n",
		exit:   0},
	{name: "s2_def_null_explicit",
		src:    "<?php\nfunction f($a = \"dflt\", $b = \"d2\") { return ($a === null ? \"null\" : $a) . \"/\" . ($b === null ? \"null\" : $b); }\necho f(null), \" \", f(null, null), \" \", f($undef), \"\\n\";\n",
		stdout: "null/d2 null/null \nWarning: Undefined variable $undef in /w/cases/s2_def_null_explicit.php on line 3\nnull/d2\n",
		logOut: "null/d2 null/null null/d2\n",
		logErr: "PHP Warning:  Undefined variable $undef in /w/cases/s2_def_null_explicit.php on line 3\n",
		exit:   0},
	{name: "s2_def_optional_before_required",
		src:    "<?php\nfunction f($a = 1, $b) { return ($a === null ? \"null\" : $a) . $b; }\necho f(5, 6), \"\\n\";\necho f(7), \"\\n\";\n",
		stdout: "\nDeprecated: f(): Optional parameter $a declared before required parameter $b is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required.php on line 2\n56\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_optional_before_required.php on line 4 and exactly 2 expected in /w/cases/s2_def_optional_before_required.php:2\nStack trace:\n#0 /w/cases/s2_def_optional_before_required.php(4): f(7)\n#1 {main}\n  thrown in /w/cases/s2_def_optional_before_required.php on line 2\n",
		logOut: "56\n",
		logErr: "PHP Deprecated:  f(): Optional parameter $a declared before required parameter $b is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required.php on line 2\nPHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_optional_before_required.php on line 4 and exactly 2 expected in /w/cases/s2_def_optional_before_required.php:2\nStack trace:\n#0 /w/cases/s2_def_optional_before_required.php(4): f(7)\n#1 {main}\n  thrown in /w/cases/s2_def_optional_before_required.php on line 2\n",
		exit:   255},
	{name: "s2_def_optional_before_required2",
		src:    "<?php\nfunction f($a = 1, $b, $c = 3) { return \"$a$b$c\"; }\necho f(5, 6), \"\\n\";\nfunction g(\n  $a = [],\n  $b = 2,\n  $c\n) { return 1; }\necho g(1, 2, 3), \"\\n\";\necho g(1, 2), \"\\n\";\n",
		stdout: "\nDeprecated: f(): Optional parameter $a declared before required parameter $b is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 2\n\nDeprecated: g(): Optional parameter $a declared before required parameter $c is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 5\n\nDeprecated: g(): Optional parameter $b declared before required parameter $c is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 6\n563\n1\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function g(), 2 passed in /w/cases/s2_def_optional_before_required2.php on line 10 and exactly 3 expected in /w/cases/s2_def_optional_before_required2.php:7\nStack trace:\n#0 /w/cases/s2_def_optional_before_required2.php(10): g(1, 2)\n#1 {main}\n  thrown in /w/cases/s2_def_optional_before_required2.php on line 7\n",
		logOut: "563\n1\n",
		logErr: "PHP Deprecated:  f(): Optional parameter $a declared before required parameter $b is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 2\nPHP Deprecated:  g(): Optional parameter $a declared before required parameter $c is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 5\nPHP Deprecated:  g(): Optional parameter $b declared before required parameter $c is implicitly treated as a required parameter in /w/cases/s2_def_optional_before_required2.php on line 6\nPHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function g(), 2 passed in /w/cases/s2_def_optional_before_required2.php on line 10 and exactly 3 expected in /w/cases/s2_def_optional_before_required2.php:7\nStack trace:\n#0 /w/cases/s2_def_optional_before_required2.php(10): g(1, 2)\n#1 {main}\n  thrown in /w/cases/s2_def_optional_before_required2.php on line 7\n",
		exit:   255},
	{name: "s2_def_string_offset_default",
		src:    "<?php\nfunction f($a = \"abc\"[5], $b = \"abc\"[\"x\"] ?? \"dflt\") { return \"[$a$b]\"; }\necho \"a\\n\";\necho f(), \"\\n\";\n",
		stdout: "a\n\nWarning: Uninitialized string offset 5 in /w/cases/s2_def_string_offset_default.php on line 2\n[dflt]\n",
		logOut: "a\n[dflt]\n",
		logErr: "PHP Warning:  Uninitialized string offset 5 in /w/cases/s2_def_string_offset_default.php on line 2\n",
		exit:   0},
	{name: "s2_def_too_few_at_least",
		src:    "<?php\nfunction f($a, $b, $c = 3) { return 1; }\necho \"a\\n\";\nf(1);\n",
		stdout: "a\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_at_least.php on line 4 and at least 2 expected in /w/cases/s2_def_too_few_at_least.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_at_least.php(4): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_at_least.php on line 2\n",
		logOut: "a\n",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_at_least.php on line 4 and at least 2 expected in /w/cases/s2_def_too_few_at_least.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_at_least.php(4): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_at_least.php on line 2\n",
		exit:   255},
	{name: "s2_def_too_few_default_only",
		src:    "<?php\nfunction f($a = 1, $b = 2) { return \"$a$b\"; }\necho f(), \"\\n\";\nfunction g($a, $b = 2) { return 1; }\ng();\n",
		stdout: "12\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function g(), 0 passed in /w/cases/s2_def_too_few_default_only.php on line 5 and at least 1 expected in /w/cases/s2_def_too_few_default_only.php:4\nStack trace:\n#0 /w/cases/s2_def_too_few_default_only.php(5): g()\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_default_only.php on line 4\n",
		logOut: "12\n",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function g(), 0 passed in /w/cases/s2_def_too_few_default_only.php on line 5 and at least 1 expected in /w/cases/s2_def_too_few_default_only.php:4\nStack trace:\n#0 /w/cases/s2_def_too_few_default_only.php(5): g()\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_default_only.php on line 4\n",
		exit:   255},
	{name: "s2_def_too_few_exactly",
		src:    "<?php\nfunction f($a, $b) { return 1; }\necho \"a\\n\";\nf(1);\n",
		stdout: "a\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_exactly.php on line 4 and exactly 2 expected in /w/cases/s2_def_too_few_exactly.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_exactly.php(4): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_exactly.php on line 2\n",
		logOut: "a\n",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_exactly.php on line 4 and exactly 2 expected in /w/cases/s2_def_too_few_exactly.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_exactly.php(4): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_exactly.php on line 2\n",
		exit:   255},
	{name: "s2_def_too_few_lines",
		src:    "<?php\nfunction f(\n  $a,\n  $b,\n  $c = 3\n) {\n  return 1;\n}\necho \"a\\n\";\nf(\n  1\n);\n",
		stdout: "a\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_lines.php on line 10 and at least 2 expected in /w/cases/s2_def_too_few_lines.php:4\nStack trace:\n#0 /w/cases/s2_def_too_few_lines.php(10): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_lines.php on line 4\n",
		logOut: "a\n",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function f(), 1 passed in /w/cases/s2_def_too_few_lines.php on line 10 and at least 2 expected in /w/cases/s2_def_too_few_lines.php:4\nStack trace:\n#0 /w/cases/s2_def_too_few_lines.php(10): f(1)\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_lines.php on line 4\n",
		exit:   255},
	{name: "s2_def_too_few_nested",
		src:    "<?php\nfunction inner($a, $b = 2, $c = 3) { return outer(); }\nfunction outer($x, $y = \"long string argument here\") { return 1; }\nfunction top($p = 1.5, $q = [1]) { return inner(\"arg\", null); }\ntop();\n",
		stdout: "\nFatal error: Uncaught ArgumentCountError: Too few arguments to function outer(), 0 passed in /w/cases/s2_def_too_few_nested.php on line 2 and at least 1 expected in /w/cases/s2_def_too_few_nested.php:3\nStack trace:\n#0 /w/cases/s2_def_too_few_nested.php(2): outer()\n#1 /w/cases/s2_def_too_few_nested.php(4): inner('arg', NULL)\n#2 /w/cases/s2_def_too_few_nested.php(5): top()\n#3 {main}\n  thrown in /w/cases/s2_def_too_few_nested.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function outer(), 0 passed in /w/cases/s2_def_too_few_nested.php on line 2 and at least 1 expected in /w/cases/s2_def_too_few_nested.php:3\nStack trace:\n#0 /w/cases/s2_def_too_few_nested.php(2): outer()\n#1 /w/cases/s2_def_too_few_nested.php(4): inner('arg', NULL)\n#2 /w/cases/s2_def_too_few_nested.php(5): top()\n#3 {main}\n  thrown in /w/cases/s2_def_too_few_nested.php on line 3\n",
		exit:   255},
	{name: "s2_def_too_few_none",
		src:    "<?php\nfunction f($a, $c = 3, $d = 4) { return 1; }\necho \"a\\n\";\nf();\n",
		stdout: "a\n\nFatal error: Uncaught ArgumentCountError: Too few arguments to function f(), 0 passed in /w/cases/s2_def_too_few_none.php on line 4 and at least 1 expected in /w/cases/s2_def_too_few_none.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_none.php(4): f()\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_none.php on line 2\n",
		logOut: "a\n",
		logErr: "PHP Fatal error:  Uncaught ArgumentCountError: Too few arguments to function f(), 0 passed in /w/cases/s2_def_too_few_none.php on line 4 and at least 1 expected in /w/cases/s2_def_too_few_none.php:2\nStack trace:\n#0 /w/cases/s2_def_too_few_none.php(4): f()\n#1 {main}\n  thrown in /w/cases/s2_def_too_few_none.php on line 2\n",
		exit:   255},
	{name: "s2_def_too_many_passed",
		src:    "<?php\nfunction f($a, $b = 2) { $x = 1; $x[0] = 2; }\nf(1, 2, 3, \"extra\");\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_def_too_many_passed.php:2\nStack trace:\n#0 /w/cases/s2_def_too_many_passed.php(3): f(1, 2, 3, 'extra')\n#1 {main}\n  thrown in /w/cases/s2_def_too_many_passed.php on line 2\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_def_too_many_passed.php:2\nStack trace:\n#0 /w/cases/s2_def_too_many_passed.php(3): f(1, 2, 3, 'extra')\n#1 {main}\n  thrown in /w/cases/s2_def_too_many_passed.php on line 2\n",
		exit:   255},
	{name: "s2_def_trace",
		src:    "<?php\nfunction f($a, $b = 5, $c = \"dflt\") { $x = []; $x[[]] = 1; }\nfunction g($p = 1, $q = [2]) { f($p, \"explicit\"); }\ng();\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_def_trace.php:2\nStack trace:\n#0 /w/cases/s2_def_trace.php(3): f(1, 'explicit')\n#1 /w/cases/s2_def_trace.php(4): g()\n#2 {main}\n  thrown in /w/cases/s2_def_trace.php on line 2\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_def_trace.php:2\nStack trace:\n#0 /w/cases/s2_def_trace.php(3): f(1, 'explicit')\n#1 /w/cases/s2_def_trace.php(4): g()\n#2 {main}\n  thrown in /w/cases/s2_def_trace.php on line 2\n",
		exit:   255},
	{name: "s2_def_trace_param_changed",
		src:    "<?php\nfunction f($a = 1, $b = 2) { $a = \"changed\"; $b = \"also\"; $x = 5; $x[0] = 1; }\nf(7);\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_def_trace_param_changed.php:2\nStack trace:\n#0 /w/cases/s2_def_trace_param_changed.php(3): f('changed')\n#1 {main}\n  thrown in /w/cases/s2_def_trace_param_changed.php on line 2\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_def_trace_param_changed.php:2\nStack trace:\n#0 /w/cases/s2_def_trace_param_changed.php(3): f('changed')\n#1 {main}\n  thrown in /w/cases/s2_def_trace_param_changed.php on line 2\n",
		exit:   255},
	{name: "s2_def_undefined_key",
		src:    "<?php\nfunction f(\n  $a = 1,\n  $b = [1, 2][7]\n) { return \"x\" . $b; }\necho \"a\\n\";\necho f(), \"\\n\";\necho f(1, 2), \"\\n\";\n",
		stdout: "a\n\nWarning: Undefined array key 7 in /w/cases/s2_def_undefined_key.php on line 4\nx\nx2\n",
		logOut: "a\nx\nx2\n",
		logErr: "PHP Warning:  Undefined array key 7 in /w/cases/s2_def_undefined_key.php on line 4\n",
		exit:   0},
	{name: "s2_def_warn_expr",
		src:    "<?php\nfunction f($a = \"abc\" . [1][0], $b = [1, 2][5] ?? \"none\", $c = \"5\" + \"5\") { return $a . $b . $c; }\necho \"a\\n\";\necho f(), \"\\n\";\n",
		stdout: "a\nabc1none10\n",
		logOut: "a\nabc1none10\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_basic",
		src:    "<?php\n$a = [10, 20, 30];\nforeach ($a as $v) { echo $v, \",\"; }\necho \"\\n\";\nforeach ($a as $k => $v) { echo $k, \"=\", $v, \",\"; }\necho \"\\n\";\n$m = [\"x\" => 1, 5 => \"five\", \"y\" => null, -3 => 2.5, \"\" => true];\nforeach ($m as $k => $v) echo \"[\", $k, \"]=\", $v, \";\";\necho \"\\n\";\nforeach ([] as $v) { echo \"never\"; }\nforeach ([] as $k => $v) { echo \"never\"; }\necho isset($v) ? \"v=\" : \"unset\", $v, isset($k) ? \" k=\" : \" kunset\", $k, \"\\n\";\nforeach ([1, 2] as $q): echo $q; endforeach;\necho \"\\n\";\n",
		stdout: "10,20,30,\n0=10,1=20,2=30,\n[x]=1;[5]=five;[y]=;[-3]=2.5;[]=1;\nv=1 k=\n12\n",
		logOut: "10,20,30,\n0=10,1=20,2=30,\n[x]=1;[5]=five;[y]=;[-3]=2.5;[]=1;\nv=1 k=\n12\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_break_continue",
		src:    "<?php\n$a = [1, 2, 3, 4, 5, 6];\nforeach ($a as $v) { if ($v == 2) continue; if ($v == 5) break; echo $v; }\necho \"|\", $v, \"\\n\";\nforeach ([1, 2, 3] as $i) {\n    foreach ([\"a\", \"b\", \"c\"] as $j) {\n        if ($j == \"b\") continue 2;\n        if ($i == 3) break 2;\n        echo $i, $j, \" \";\n    }\n    echo \"never\";\n}\necho \"|\", $i, $j, \"\\n\";\nforeach ([1, 2, 3] as $i) {\n    $n = 0;\n    while (true) { $n++; if ($n > $i) continue 2; if ($i == 2 && $n == 2) break 2; echo $i, \":\", $n, \" \"; }\n}\necho \"\\n\";\nforeach ([1, 2, 3, 4] as $i) {\n    switch ($i) { case 2: continue 2; case 3: break; case 4: break 2; default: echo \"d\"; }\n    echo $i;\n}\necho \"\\n\";\nfor ($x = 0; $x < 3; $x++) { foreach ([7, 8, 9] as $y) { if ($y == 8) continue 2; echo $x, $y, \" \"; } }\necho \"\\n\";\n",
		stdout: "134|5\n1a 2a |3a\n1:1 2:1 \nd13\n07 17 27 \n",
		logOut: "134|5\n1a 2a |3a\n1:1 2:1 \nd13\n07 17 27 \n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_copy_semantics",
		src:    "<?php\n$a = [1, 2, 3];\n$b = $a;\nforeach ($a as $k => $v) { $b[$k] = $v + 100; $a[$k] = $v + 200; }\necho $a[0], $a[2], $b[0], $b[2], \"\\n\";\n$c = [1, 2, 3];\nforeach ($c as $v) { if ($v == 1) { $c[] = 4; $c[0] = 99; } echo $v; }\necho \"|\", $c[0], $c[3], \"\\n\";\n$d = [[1], [2]];\nforeach ($d as $i => $row) { $row[] = \"x\"; $d[$i][] = \"y\"; }\necho isset($d[0][2]) ? \"leak\" : \"ok\", $d[0][1], $d[1][1], \"\\n\";\n$e = [3, 2, 1];\nforeach ($e as $v) { foreach ($e as $w) { echo $v, $w, \" \"; } }\necho \"\\n\";\n",
		stdout: "201203101103\n123|994\nokyy\n33 32 31 23 22 21 13 12 11 \n",
		logOut: "201203101103\n123|994\nokyy\n33 32 31 23 22 21 13 12 11 \n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_deep_nesting",
		src:    "<?php\n$m = [[[1, 2], [3]], [[4]]];\n$o = \"\";\nforeach ($m as $i => $a) { foreach ($a as $j => $b) { foreach ($b as $k => $v) { if ($v == 2) continue 3; if ($v == 4) break 2; $o .= \"$i$j$k$v \"; } $o .= \"/\"; } $o .= \"|\"; }\necho $o, \"\\n\";\nfunction walk($t, $depth = 0) { $s = \"\"; foreach ($t as $k => $v) { if ((array)$v === $v) { $s .= $k . \"[\" . walk($v, $depth + 1) . \"]\"; } else { $s .= $k . \":\" . $v . \"@\" . $depth . \" \"; } } return $s; }\necho walk([\"a\" => 1, \"b\" => [\"c\" => 2, \"d\" => [\"e\" => 3]], \"f\" => []]), \"\\n\";\n",
		stdout: "0001 |\na:1@0 b[c:2@1 d[e:3@2 ]]f[]\n",
		logOut: "0001 |\na:1@0 b[c:2@1 d[e:3@2 ]]f[]\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_empty_vars",
		src:    "<?php\nforeach ([] as $kk => $vv) { echo \"never\"; }\necho isset($vv) ? \"set\" : \"unset\", isset($kk) ? \"set\" : \"unset\", \"\\n\";\necho $vv, \"|\\n\";\n",
		stdout: "unsetunset\n\nWarning: Undefined variable $vv in /w/cases/s2_fe_empty_vars.php on line 4\n|\n",
		logOut: "unsetunset\n|\n",
		logErr: "PHP Warning:  Undefined variable $vv in /w/cases/s2_fe_empty_vars.php on line 4\n",
		exit:   0},
	{name: "s2_fe_function",
		src:    "<?php\nfunction sum($xs) { $t = 0; foreach ($xs as $x) { $t += $x; } return $t; }\nfunction first($xs, $want) { foreach ($xs as $k => $x) { if ($x == $want) return $k; } return \"none\"; }\nfunction mut($xs) { foreach ($xs as $k => $x) { $xs[$k] = $x * 2; } return $xs; }\n$a = [1, 2, 3, 4];\necho sum($a), first($a, 3), first($a, 9), \"\\n\";\n$b = mut($a);\necho $a[0], $a[3], $b[0], $b[3], \"\\n\";\nfunction nest($m) { $o = \"\"; foreach ($m as $k => $row) { foreach ($row as $j => $c) { $o .= \"$k$j$c \"; } } return $o; }\necho nest([\"a\" => [1, 2], \"b\" => [\"x\" => 3]]), \"\\n\";\n$big = [];\nfor ($i = 0; $i < 1000; $i++) { $big[] = $i; }\necho sum($big), \"\\n\";\n",
		stdout: "102none\n1428\na01 a12 bx3 \n499500\n",
		logOut: "102none\n1428\na01 a12 bx3 \n499500\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_holes",
		src:    "<?php\n$a = [0, 1, 2, 3, 4, 5];\nunset($a[1], $a[3], $a[5]);\nforeach ($a as $k => $v) echo $k, $v, \" \";\necho \"\\n\";\n$a[] = \"n\";\n$a[1] = \"back\";\nforeach ($a as $k => $v) echo $k, $v, \" \";\necho \"\\n\";\n$m = [\"a\" => 1, \"b\" => 2, \"c\" => 3];\nunset($m[\"b\"]);\n$m[\"b\"] = 9;\nforeach ($m as $k => $v) echo $k, $v, \" \";\necho \"\\n\";\n$t = [5 => \"x\", 2 => \"y\", \"5\" => \"z\", 1.7 => \"f\", true => \"t\"];\nforeach ($t as $k => $v) echo $k, $v, \" \";\necho \"\\n\";\n",
		stdout: "00 22 44 \n00 22 44 6n 1back \na1 c3 b9 \n\nDeprecated: Implicit conversion from float 1.7 to int loses precision in /w/cases/s2_fe_holes.php on line 15\n5z 2y 1t \n",
		logOut: "00 22 44 \n00 22 44 6n 1back \na1 c3 b9 \n5z 2y 1t \n",
		logErr: "PHP Deprecated:  Implicit conversion from float 1.7 to int loses precision in /w/cases/s2_fe_holes.php on line 15\n",
		exit:   0},
	{name: "s2_fe_iter_large_holes",
		src:    "<?php\n$a = [];\nfor ($i = 0; $i < 3000; $i++) { $a[] = $i; }\nfor ($i = 1; $i < 2999; $i++) { unset($a[$i]); }\n$t = 0; $n = 0;\nforeach ($a as $k => $v) { $t += $k + $v; $n++; }\necho $n, \" \", $t, \"\\n\";\n",
		stdout: "2 5998\n",
		logOut: "2 5998\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_key_only_dim",
		src:    "<?php\nforeach ([\"a\" => 1] as $k[\"x\"][\"y\"] => $v[]) {}\necho $k[\"x\"][\"y\"], $v[0], \"\\n\";\nforeach ([5, 6] as $i => $u[$i]) {}\necho isset($u[0]) ? \"0\" : \"-\", isset($u[1]) ? \"1\" : \"-\", isset($u[\"\"]) ? \"e\" : \"-\", \"\\n\";\n",
		stdout: "a1\n\nWarning: Undefined variable $i in /w/cases/s2_fe_key_only_dim.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_key_only_dim.php on line 4\n0-e\n",
		logOut: "a1\n0-e\n",
		logErr: "PHP Warning:  Undefined variable $i in /w/cases/s2_fe_key_only_dim.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_key_only_dim.php on line 4\n",
		exit:   0},
	{name: "s2_fe_lines",
		src:    "<?php\nforeach (\n  $undef\n  as\n  $v\n) {\n  echo \"never\";\n}\necho \"|\\n\";\n$n = 3;\nforeach ($n\n  as $k =>\n  $v) {\n}\necho \"|\\n\";\nforeach ([1] as\n  $arr[$uk]) {\n}\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $undef in /w/cases/s2_fe_lines.php on line 3\n\nWarning: foreach() argument must be of type array|object, null given in /w/cases/s2_fe_lines.php on line 3\n|\n\nWarning: foreach() argument must be of type array|object, int given in /w/cases/s2_fe_lines.php on line 11\n|\n\nWarning: Undefined variable $uk in /w/cases/s2_fe_lines.php on line 17\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_lines.php on line 17\n|\n",
		logOut: "|\n|\n|\n",
		logErr: "PHP Warning:  Undefined variable $undef in /w/cases/s2_fe_lines.php on line 3\nPHP Warning:  foreach() argument must be of type array|object, null given in /w/cases/s2_fe_lines.php on line 3\nPHP Warning:  foreach() argument must be of type array|object, int given in /w/cases/s2_fe_lines.php on line 11\nPHP Warning:  Undefined variable $uk in /w/cases/s2_fe_lines.php on line 17\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_lines.php on line 17\n",
		exit:   0},
	{name: "s2_fe_modify_inside",
		src:    "<?php\n$a = [1, 2, 3];\nforeach ($a as $k => $v) { $a[] = $v * 10; if ($k == 0) { $a[1] = \"changed\"; unset($a[2]); } echo $v, \",\"; }\necho \"|\";\nforeach ($a as $k => $v) echo $k, \"=\", $v, \",\";\necho \"\\n\";\n$b = [1, 2, 3];\nforeach ($b as $v) { $b = \"gone\"; echo $v; }\necho $b, \"\\n\";\n$c = [1, 2, 3];\nforeach ($c as $v) { unset($c); echo $v; }\necho isset($c) ? \"set\" : \"unset\", \"\\n\";\n$e = [1, 2];\nforeach ($e as $e) { echo $e; }\necho $e, \"\\n\";\n$g = [\"a\" => 1, \"b\" => 2];\nforeach ($g as $k => $k) { echo $k; }\necho $k, \"\\n\";\n$h = [\"a\" => 1, \"b\" => 2];\nforeach ($h as $h => $w) { echo $h, $w; }\necho $h, \"\\n\";\n",
		stdout: "1,2,3,|0=1,1=changed,3=10,4=20,5=30,\n123gone\n123unset\n122\nabb\na1b2b\n",
		logOut: "1,2,3,|0=1,1=changed,3=10,4=20,5=30,\n123gone\n123unset\n122\nabb\na1b2b\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_nested_break_cow",
		src:    "<?php\n$a = [1, 2, 3];\nforeach ($a as $x) { foreach ($a as $y) { if ($y == 2) break 2; } }\n$a[] = 4;\nforeach ($a as $x) { foreach ($a as $y) { if ($y == 2) continue 2; echo $x, $y, \" \"; } }\n$a[] = 5;\necho $a[3], $a[4], \"\\n\";\nfunction g($a) { foreach ($a as $x) { foreach ($a as $y) { if ($y == 2) return $x + $y; } } return 0; }\necho g($a), \"\\n\";\n",
		stdout: "11 21 31 41 45\n3\n",
		logOut: "11 21 31 41 45\n3\n",
		logErr: "",
		exit:   0},
	{name: "s2_fe_non_array",
		src:    "<?php\n$n = null; $i = 5; $s = \"str\"; $f = false; $t = true; $fl = 1.5;\nforeach ($n as $v) { echo \"never\"; }\nforeach ($i as $k => $v) { echo \"never\"; }\nforeach ($s as $v) { echo \"never\"; }\nforeach ($f as $v) { echo \"never\"; }\nforeach ($t as $v) { echo \"never\"; }\nforeach ($fl as $v) { echo \"never\"; }\nforeach ($undef as $v) { echo \"never\"; }\nforeach (7 as $v) { echo \"never\"; }\nforeach (\"lit\" as $v) { echo \"never\"; }\nforeach (null as $v) { echo \"never\"; }\necho isset($v) ? \"set\" : \"unset\", \"\\n\";\n",
		stdout: "\nWarning: foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 3\n\nWarning: foreach() argument must be of type array|object, int given in /w/cases/s2_fe_non_array.php on line 4\n\nWarning: foreach() argument must be of type array|object, string given in /w/cases/s2_fe_non_array.php on line 5\n\nWarning: foreach() argument must be of type array|object, false given in /w/cases/s2_fe_non_array.php on line 6\n\nWarning: foreach() argument must be of type array|object, true given in /w/cases/s2_fe_non_array.php on line 7\n\nWarning: foreach() argument must be of type array|object, float given in /w/cases/s2_fe_non_array.php on line 8\n\nWarning: Undefined variable $undef in /w/cases/s2_fe_non_array.php on line 9\n\nWarning: foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 9\n\nWarning: foreach() argument must be of type array|object, int given in /w/cases/s2_fe_non_array.php on line 10\n\nWarning: foreach() argument must be of type array|object, string given in /w/cases/s2_fe_non_array.php on line 11\n\nWarning: foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 12\nunset\n",
		logOut: "unset\n",
		logErr: "PHP Warning:  foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 3\nPHP Warning:  foreach() argument must be of type array|object, int given in /w/cases/s2_fe_non_array.php on line 4\nPHP Warning:  foreach() argument must be of type array|object, string given in /w/cases/s2_fe_non_array.php on line 5\nPHP Warning:  foreach() argument must be of type array|object, false given in /w/cases/s2_fe_non_array.php on line 6\nPHP Warning:  foreach() argument must be of type array|object, true given in /w/cases/s2_fe_non_array.php on line 7\nPHP Warning:  foreach() argument must be of type array|object, float given in /w/cases/s2_fe_non_array.php on line 8\nPHP Warning:  Undefined variable $undef in /w/cases/s2_fe_non_array.php on line 9\nPHP Warning:  foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 9\nPHP Warning:  foreach() argument must be of type array|object, int given in /w/cases/s2_fe_non_array.php on line 10\nPHP Warning:  foreach() argument must be of type array|object, string given in /w/cases/s2_fe_non_array.php on line 11\nPHP Warning:  foreach() argument must be of type array|object, null given in /w/cases/s2_fe_non_array.php on line 12\n",
		exit:   0},
	{name: "s2_fe_subjects",
		src:    "<?php\nfunction arr() { echo \"[arr]\"; return [\"p\" => 1, \"q\" => 2]; }\nforeach (arr() as $k => $v) echo $k, $v;\necho \"\\n\";\n$n = [\"in\" => [3, 4], \"s\" => \"str\"];\nforeach ($n[\"in\"] as $v) echo $v;\nforeach ($n[\"s\"] as $v) echo \"never\";\nforeach ($n[\"missing\"] as $v) echo \"never\";\necho \"\\n\";\nforeach ([[1, 2], [3]] as $row) { foreach ($row as $cell) echo $cell; echo \"/\"; }\necho \"\\n\";\n$i = 0;\nforeach ([$i++, $i++, $i] as $v) echo $v;\necho \"\\n\";\nforeach ((array)\"x\" as $k => $v) echo $k, $v;\necho \"\\n\";\n",
		stdout: "[arr]p1q2\n34\nWarning: foreach() argument must be of type array|object, string given in /w/cases/s2_fe_subjects.php on line 7\n\nWarning: Undefined array key \"missing\" in /w/cases/s2_fe_subjects.php on line 8\n\nWarning: foreach() argument must be of type array|object, null given in /w/cases/s2_fe_subjects.php on line 8\n\n12/3/\n012\n0x\n",
		logOut: "[arr]p1q2\n34\n12/3/\n012\n0x\n",
		logErr: "PHP Warning:  foreach() argument must be of type array|object, string given in /w/cases/s2_fe_subjects.php on line 7\nPHP Warning:  Undefined array key \"missing\" in /w/cases/s2_fe_subjects.php on line 8\nPHP Warning:  foreach() argument must be of type array|object, null given in /w/cases/s2_fe_subjects.php on line 8\n",
		exit:   0},
	{name: "s2_fe_target_error",
		src:    "<?php\n$i = 5;\nforeach ([1, 2] as $i[\"k\"]) { echo \"body\"; }\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_fe_target_error.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fe_target_error.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_fe_target_error.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fe_target_error.php on line 3\n",
		exit:   255},
	{name: "s2_fe_target_self",
		src:    "<?php\nforeach ([[1, 2]] as $d[0]) {}\necho $d[0][1], \"\\n\";\n$e = [5];\nforeach ($e as $e[\"k\"]) {}\necho isset($e[\"k\"]) ? $e[\"k\"] : \"none\", $e[0], \"\\n\";\n$c = [[1]];\n$c[0] += $c;\necho isset($c[0][1]) ? \"y\" : \"n\", isset($c[0][0][0]) ? \"deep\" : \"flat\", \"\\n\";\n$g = [\"a\" => 1, \"b\" => 2];\nforeach ($g as $k => $g[$k]) { $g[\"x\"] = 9; }\necho $g[\"a\"], $g[\"b\"], $g[\"x\"], isset($g[\"\"]) ? \"e\" : \"-\", \"\\n\";\n",
		stdout: "2\n55\nnflat\n\nWarning: Undefined variable $k in /w/cases/s2_fe_target_self.php on line 11\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_target_self.php on line 11\n229e\n",
		logOut: "2\n55\nnflat\n229e\n",
		logErr: "PHP Warning:  Undefined variable $k in /w/cases/s2_fe_target_self.php on line 11\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_target_self.php on line 11\n",
		exit:   0},
	{name: "s2_fe_targets",
		src:    "<?php\n$a = [\"x\" => 1, \"y\" => 2];\nforeach ($a as $out[]) {}\necho $out[0], $out[1], \"\\n\";\nforeach ($a as $ks[] => $vs[\"last\"]) {}\necho $ks[0], $ks[1], $vs[\"last\"], \"\\n\";\nforeach ($a as $k => $deep[$k][\"v\"]) {}\necho $deep[\"x\"][\"v\"], $deep[\"y\"][\"v\"], \"\\n\";\n$s = \"str\";\nforeach ([1] as $s[0]) {}\necho $s, \"\\n\";\n",
		stdout: "12\nxy2\n\nWarning: Undefined variable $k in /w/cases/s2_fe_targets.php on line 7\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_targets.php on line 7\n2\nWarning: Undefined array key \"y\" in /w/cases/s2_fe_targets.php on line 8\n\nWarning: Trying to access array offset on null in /w/cases/s2_fe_targets.php on line 8\n\n1tr\n",
		logOut: "12\nxy2\n2\n1tr\n",
		logErr: "PHP Warning:  Undefined variable $k in /w/cases/s2_fe_targets.php on line 7\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_fe_targets.php on line 7\nPHP Warning:  Undefined array key \"y\" in /w/cases/s2_fe_targets.php on line 8\nPHP Warning:  Trying to access array offset on null in /w/cases/s2_fe_targets.php on line 8\n",
		exit:   0},
	{name: "s2_fe_types",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\nforeach ([\"1\" => \"a\", \"01\" => \"b\", 2 => null, \"k\" => 1.0, 3 => [1], 4 => false] as $k => $v) { echo d($k), \"=>\", d($v), \" \"; }\necho \"\\n\";\n",
		stdout: "int(1)=>str(a) str(01)=>str(b) int(2)=>null str(k)=>float(1) int(3)=>\nWarning: Array to string conversion in /w/cases/s2_fe_types.php on line 2\narray int(4)=>false \n",
		logOut: "int(1)=>str(a) str(01)=>str(b) int(2)=>null str(k)=>float(1) int(3)=>array int(4)=>false \n",
		logErr: "PHP Warning:  Array to string conversion in /w/cases/s2_fe_types.php on line 2\n",
		exit:   0},
	{name: "s2_fe_value_order",
		src:    "<?php\n$a = [\"k1\" => \"v1\"];\nforeach ($a as $x => $x) {}\necho $x, \"\\n\";\nforeach ($a as $t[0] => $t[0]) {}\necho $t[0], \"\\n\";\nforeach ([[1, 2]] as $r) { foreach ($r as $r) { echo $r; } }\necho $r, \"\\n\";\n",
		stdout: "k1\nk1\n122\n",
		logOut: "k1\nk1\n122\n",
		logErr: "",
		exit:   0},
	{name: "s2_fetch_string_key_array",
		src:    "<?php\n$s = \"abc\";\n$s[[]][0] = 1;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_fetch_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_array.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_fetch_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_array.php on line 3\n",
		exit:   255},
	{name: "s2_fetch_string_key_float",
		src:    "<?php\n$s = \"abc\";\n$s[1.5]++;\necho \"end\\n\";\n",
		stdout: "\nWarning: String offset cast occurred in /w/cases/s2_fetch_string_key_float.php on line 3\n\nFatal error: Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_fetch_string_key_float.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_float.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  String offset cast occurred in /w/cases/s2_fetch_string_key_float.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_fetch_string_key_float.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_float.php on line 3\n",
		exit:   255},
	{name: "s2_fetch_string_key_leading",
		src:    "<?php\n$s = \"abc\";\n$s[\"1x\"][0] = 1;\necho \"end\\n\";\n",
		stdout: "\nWarning: Illegal string offset \"1x\" in /w/cases/s2_fetch_string_key_leading.php on line 3\n\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_fetch_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_leading.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_fetch_string_key_leading.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_fetch_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_leading.php on line 3\n",
		exit:   255},
	{name: "s2_fetch_string_key_nonnumeric_incdec",
		src:    "<?php\n$s = \"abc\";\n$s[\"x\"]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_fetch_string_key_nonnumeric_incdec.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_nonnumeric_incdec.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_fetch_string_key_nonnumeric_incdec.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_nonnumeric_incdec.php on line 3\n",
		exit:   255},
	{name: "s2_fetch_string_key_numeric",
		src:    "<?php\n$s = \"abc\";\n$s[\"1\"][0] .= $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_fetch_string_key_numeric.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_numeric.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_fetch_string_key_numeric.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_fetch_string_key_numeric.php on line 3\n",
		exit:   255},
	{name: "s2_incdec_dim",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$a = [5, \"9\", \"az\", 1.5, null, true];\n$a[0]++; ++$a[0]; $a[0]--; --$a[0]; $a[0]++;\n$a[1]++;\n$a[2]++;\n$a[3]--;\necho d($a[0]), d($a[1]), d($a[2]), d($a[3]), \"\\n\";\n$x = $a[0]++;\n$y = ++$a[0];\n$z = $a[0]--;\n$w = --$a[0];\necho $x, $y, $z, $w, $a[0], \"\\n\";\n$a[4]++;\n$a[5]++;\necho d($a[4]), d($a[5]), \"\\n\";\n$a[\"nk\"]++;\n$a[\"nd\"]--;\necho d($a[\"nk\"]), d($a[\"nd\"]), \"\\n\";\n$a[]++;\necho d($a[6]), \"\\n\";\n",
		stdout: "\nDeprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_dim.php on line 7\nint(6)int(10)str(ba)float(0.5)\n68866\n\nWarning: Increment on type bool has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim.php on line 16\nint(1)true\n\nWarning: Undefined array key \"nk\" in /w/cases/s2_incdec_dim.php on line 18\n\nWarning: Undefined array key \"nd\" in /w/cases/s2_incdec_dim.php on line 19\n\nWarning: Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim.php on line 19\nint(1)null\nint(1)\n",
		logOut: "int(6)int(10)str(ba)float(0.5)\n68866\nint(1)true\nint(1)null\nint(1)\n",
		logErr: "PHP Deprecated:  Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_dim.php on line 7\nPHP Warning:  Increment on type bool has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim.php on line 16\nPHP Warning:  Undefined array key \"nk\" in /w/cases/s2_incdec_dim.php on line 18\nPHP Warning:  Undefined array key \"nd\" in /w/cases/s2_incdec_dim.php on line 19\nPHP Warning:  Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim.php on line 19\n",
		exit:   0},
	{name: "s2_incdec_dim_array_elem",
		src:    "<?php\n$a = [[1]];\n$a[0]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot increment array in /w/cases/s2_incdec_dim_array_elem.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_array_elem.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot increment array in /w/cases/s2_incdec_dim_array_elem.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_array_elem.php on line 3\n",
		exit:   255},
	{name: "s2_incdec_dim_containers",
		src:    "<?php\n$u[0]++;\necho $u[0], \"\\n\";\n$n = null;\n++$n[\"a\"];\necho $n[\"a\"], \"\\n\";\n$f = false;\n$f[1]--;\necho isset($f[1]) ? \"set\" : \"null\", \"\\n\";\n$u2[$uk]++;\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u in /w/cases/s2_incdec_dim_containers.php on line 2\n\nWarning: Undefined array key 0 in /w/cases/s2_incdec_dim_containers.php on line 2\n1\n\nWarning: Undefined array key \"a\" in /w/cases/s2_incdec_dim_containers.php on line 5\n1\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_incdec_dim_containers.php on line 8\n\nWarning: Undefined array key 1 in /w/cases/s2_incdec_dim_containers.php on line 8\n\nWarning: Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim_containers.php on line 8\nnull\n\nWarning: Undefined variable $u2 in /w/cases/s2_incdec_dim_containers.php on line 10\n\nWarning: Undefined variable $uk in /w/cases/s2_incdec_dim_containers.php on line 10\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_containers.php on line 10\n\nWarning: Undefined array key \"\" in /w/cases/s2_incdec_dim_containers.php on line 10\n|\n",
		logOut: "1\n1\nnull\n|\n",
		logErr: "PHP Warning:  Undefined variable $u in /w/cases/s2_incdec_dim_containers.php on line 2\nPHP Warning:  Undefined array key 0 in /w/cases/s2_incdec_dim_containers.php on line 2\nPHP Warning:  Undefined array key \"a\" in /w/cases/s2_incdec_dim_containers.php on line 5\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_incdec_dim_containers.php on line 8\nPHP Warning:  Undefined array key 1 in /w/cases/s2_incdec_dim_containers.php on line 8\nPHP Warning:  Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_incdec_dim_containers.php on line 8\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_incdec_dim_containers.php on line 10\nPHP Warning:  Undefined variable $uk in /w/cases/s2_incdec_dim_containers.php on line 10\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_containers.php on line 10\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_incdec_dim_containers.php on line 10\n",
		exit:   0},
	{name: "s2_incdec_dim_lines",
		src:    "<?php\n$a = [];\n$a\n[$u1]\n++;\n--\n$a\n[$u2];\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_incdec_dim_lines.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_lines.php on line 4\n\nWarning: Undefined array key \"\" in /w/cases/s2_incdec_dim_lines.php on line 4\n\nWarning: Undefined variable $u2 in /w/cases/s2_incdec_dim_lines.php on line 8\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_lines.php on line 8\n|\n",
		logOut: "|\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_incdec_dim_lines.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_lines.php on line 4\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_incdec_dim_lines.php on line 4\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_incdec_dim_lines.php on line 8\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_incdec_dim_lines.php on line 8\n",
		exit:   0},
	{name: "s2_incdec_dim_result_order",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$a = [5, 5];\necho $a[f(0)]++ + $a[f(1)]--, \"|\", $a[0], $a[1], \"\\n\";\n$i = 0;\n$b = [10, 20, 30];\necho $b[$i++]++, $b[$i++]--, $i, $b[0], $b[1], \"\\n\";\n$k = 0;\n$c = [1, 1];\n$c[$k]++ + $c[$k = 1]++;\necho $c[0], $c[1], \"\\n\";\n$x = ++$b[2] + $b[2]++;\necho $x, $b[2], \"\\n\";\n",
		stdout: "[f0][f1]10|64\n102021119\n22\n6232\n",
		logOut: "[f0][f1]10|64\n102021119\n22\n6232\n",
		logErr: "",
		exit:   0},
	{name: "s2_incdec_dim_scalar",
		src:    "<?php\n$i = 5;\n$i[$uk]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_incdec_dim_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_scalar.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_incdec_dim_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_scalar.php on line 3\n",
		exit:   255},
	{name: "s2_incdec_dim_string",
		src:    "<?php\n$s = \"abc\";\n$s[0]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_incdec_dim_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_incdec_dim_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_dim_string.php on line 3\n",
		exit:   255},
	{name: "s2_incdec_elem_types",
		src:    "<?php\n$a = [\"\", \"z\", \"Zz\", \"a9\", \"1.5\", \"1e2\", \" 5\", \"5 \"];\n$a[0]--; $a[1]++; $a[2]++; $a[3]++; $a[4]++; $a[5]--; $a[6]++; $a[7]++;\necho $a[0], \"|\", $a[1], \"|\", $a[2], \"|\", $a[3], \"|\", $a[4], \"|\", $a[5], \"|\", $a[6], \"|\", $a[7], \"\\n\";\n$b = [\"\"];\n$b[0]++;\necho $b[0], \"\\n\";\n",
		stdout: "\nDeprecated: Decrement on empty string is deprecated as non-numeric in /w/cases/s2_incdec_elem_types.php on line 3\n\nDeprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\n\nDeprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\n\nDeprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\n-1|aa|AAa|b0|2.5|99|6|6\n\nDeprecated: Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 6\n1\n",
		logOut: "-1|aa|AAa|b0|2.5|99|6|6\n1\n",
		logErr: "PHP Deprecated:  Decrement on empty string is deprecated as non-numeric in /w/cases/s2_incdec_elem_types.php on line 3\nPHP Deprecated:  Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\nPHP Deprecated:  Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\nPHP Deprecated:  Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 3\nPHP Deprecated:  Increment on non-numeric string is deprecated, use str_increment() instead in /w/cases/s2_incdec_elem_types.php on line 6\n",
		exit:   0},
	{name: "s2_incdec_scalar_append",
		src:    "<?php\n$i = 5;\n$i[]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_incdec_scalar_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_scalar_append.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_incdec_scalar_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_scalar_append.php on line 3\n",
		exit:   255},
	{name: "s2_incdec_string_append",
		src:    "<?php\n$s = \"abc\";\n$s[]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: [] operator not supported for strings in /w/cases/s2_incdec_string_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_string_append.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: [] operator not supported for strings in /w/cases/s2_incdec_string_append.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_incdec_string_append.php on line 3\n",
		exit:   255},
	{name: "s2_nested_append_overflow",
		src:      "<?php\n$a = [9223372036854775807 => 1];\n$a[][$uk] = $uv;\necho \"end\\n\";\n",
		stdout:   "\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/s2_nested_append_overflow.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_append_overflow.php on line 3\n",
		stdout32: "\nWarning: The float 9.223372036854776E+18 is not representable as an int, cast occurred in /w/cases/s2_nested_append_overflow.php on line 2\n\nWarning: Undefined variable $uk in /w/cases/s2_nested_append_overflow.php on line 3\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_append_overflow.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/s2_nested_append_overflow.php on line 3\nend\n", exit32: 0, differs32: true,
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/s2_nested_append_overflow.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_append_overflow.php on line 3\n",
		exit:   255},
	{name: "s2_nested_array_key",
		src:    "<?php\n$a = [];\n$a[[]][$uk] = $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_nested_compound",
		src:    "<?php\n$a = [1 => [2 => 10]];\n$a[1][2] += 3;\n$a[1][3] .= \"z\";\n$a[\"x\"][\"y\"] .= \"w\";\necho $a[1][2], $a[1][3], $a[\"x\"][\"y\"], \"\\n\";\n$u[\"p\"][\"q\"] -= 1;\necho $u[\"p\"][\"q\"], \"\\n\";\n$a[1][2]++;\n--$a[1][\"nn\"];\n$a[\"i\"][\"j\"][\"k\"]++;\necho $a[1][2], isset($a[1][\"nn\"]) ? \"set\" : \"null\", $a[\"i\"][\"j\"][\"k\"], \"\\n\";\n",
		stdout: "\nWarning: Undefined array key 3 in /w/cases/s2_nested_compound.php on line 4\n\nWarning: Undefined array key \"x\" in /w/cases/s2_nested_compound.php on line 5\n\nWarning: Undefined array key \"y\" in /w/cases/s2_nested_compound.php on line 5\n13zw\n\nWarning: Undefined variable $u in /w/cases/s2_nested_compound.php on line 7\n\nWarning: Undefined array key \"p\" in /w/cases/s2_nested_compound.php on line 7\n\nWarning: Undefined array key \"q\" in /w/cases/s2_nested_compound.php on line 7\n-1\n\nWarning: Undefined array key \"nn\" in /w/cases/s2_nested_compound.php on line 10\n\nWarning: Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_nested_compound.php on line 10\n\nWarning: Undefined array key \"i\" in /w/cases/s2_nested_compound.php on line 11\n\nWarning: Undefined array key \"j\" in /w/cases/s2_nested_compound.php on line 11\n\nWarning: Undefined array key \"k\" in /w/cases/s2_nested_compound.php on line 11\n14null1\n",
		logOut: "13zw\n-1\n14null1\n",
		logErr: "PHP Warning:  Undefined array key 3 in /w/cases/s2_nested_compound.php on line 4\nPHP Warning:  Undefined array key \"x\" in /w/cases/s2_nested_compound.php on line 5\nPHP Warning:  Undefined array key \"y\" in /w/cases/s2_nested_compound.php on line 5\nPHP Warning:  Undefined variable $u in /w/cases/s2_nested_compound.php on line 7\nPHP Warning:  Undefined array key \"p\" in /w/cases/s2_nested_compound.php on line 7\nPHP Warning:  Undefined array key \"q\" in /w/cases/s2_nested_compound.php on line 7\nPHP Warning:  Undefined array key \"nn\" in /w/cases/s2_nested_compound.php on line 10\nPHP Warning:  Decrement on type null has no effect, this will change in the next major version of PHP in /w/cases/s2_nested_compound.php on line 10\nPHP Warning:  Undefined array key \"i\" in /w/cases/s2_nested_compound.php on line 11\nPHP Warning:  Undefined array key \"j\" in /w/cases/s2_nested_compound.php on line 11\nPHP Warning:  Undefined array key \"k\" in /w/cases/s2_nested_compound.php on line 11\n",
		exit:   0},
	{name: "s2_nested_compound_order",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$a = [];\n$a[f(1)][f(2)] .= f(\"v\");\necho $a[1][2], \"\\n\";\n$b = [];\n$b[$u1][$u2] += $u3;\necho \"|\\n\";\n",
		stdout: "[f1][f2][fv]\nWarning: Undefined array key 1 in /w/cases/s2_nested_compound_order.php on line 4\n\nWarning: Undefined array key 2 in /w/cases/s2_nested_compound_order.php on line 4\nv\n\nWarning: Undefined variable $u1 in /w/cases/s2_nested_compound_order.php on line 7\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_compound_order.php on line 7\n\nWarning: Undefined array key \"\" in /w/cases/s2_nested_compound_order.php on line 7\n\nWarning: Undefined variable $u2 in /w/cases/s2_nested_compound_order.php on line 7\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_compound_order.php on line 7\n\nWarning: Undefined array key \"\" in /w/cases/s2_nested_compound_order.php on line 7\n\nWarning: Undefined variable $u3 in /w/cases/s2_nested_compound_order.php on line 7\n|\n",
		logOut: "[f1][f2][fv]v\n|\n",
		logErr: "PHP Warning:  Undefined array key 1 in /w/cases/s2_nested_compound_order.php on line 4\nPHP Warning:  Undefined array key 2 in /w/cases/s2_nested_compound_order.php on line 4\nPHP Warning:  Undefined variable $u1 in /w/cases/s2_nested_compound_order.php on line 7\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_compound_order.php on line 7\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_nested_compound_order.php on line 7\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_nested_compound_order.php on line 7\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_compound_order.php on line 7\nPHP Warning:  Undefined array key \"\" in /w/cases/s2_nested_compound_order.php on line 7\nPHP Warning:  Undefined variable $u3 in /w/cases/s2_nested_compound_order.php on line 7\n",
		exit:   0},
	{name: "s2_nested_compound_string",
		src:    "<?php\n$a = [\"str\"];\n$a[0][1] .= \"x\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_nested_compound_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_compound_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use assign-op operators with string offsets in /w/cases/s2_nested_compound_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_compound_string.php on line 3\n",
		exit:   255},
	{name: "s2_nested_compound_through_string",
		src:    "<?php\n$s = \"str\";\n$s[0][1] .= \"x\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_compound_through_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_compound_through_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_compound_through_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_compound_through_string.php on line 3\n",
		exit:   255},
	{name: "s2_nested_concat_const_array_key",
		src:    "<?php\necho \"a\\n\";\necho $a . [], \"\\n\";\necho [1, [2]] . $u . [], \"\\n\";\n$s = \"x\";\necho $s . [] . $s, \"\\n\";\n$b = [];\nunset($b['abc'][$a . []]);\n$b[$a . ['k' => 1]][$u . [3]] = 1;\necho $b[\"\"][\"\"] ?? \"none\", \"\\n\";\n$v = [$a] . $a;\necho $v, \"\\n\";\n",
		stdout: "a\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 3\n\nWarning: Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 3\nArray\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 4\n\nWarning: Undefined variable $u in /w/cases/s2_nested_concat_const_array_key.php on line 4\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 4\nArrayArray\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 6\nxArrayx\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 8\n\nWarning: Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 8\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 9\n\nWarning: Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 9\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 9\n\nWarning: Undefined variable $u in /w/cases/s2_nested_concat_const_array_key.php on line 9\nnone\n\nWarning: Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 11\n\nWarning: Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 11\n\nWarning: Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 11\nArray\n",
		logOut: "a\nArray\nArrayArray\nxArrayx\nnone\nArray\n",
		logErr: "PHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 3\nPHP Warning:  Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 3\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 4\nPHP Warning:  Undefined variable $u in /w/cases/s2_nested_concat_const_array_key.php on line 4\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 4\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 6\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 8\nPHP Warning:  Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 8\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 9\nPHP Warning:  Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 9\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 9\nPHP Warning:  Undefined variable $u in /w/cases/s2_nested_concat_const_array_key.php on line 9\nPHP Warning:  Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 11\nPHP Warning:  Undefined variable $a in /w/cases/s2_nested_concat_const_array_key.php on line 11\nPHP Warning:  Array to string conversion in /w/cases/s2_nested_concat_const_array_key.php on line 11\n",
		exit:   0},
	{name: "s2_nested_cow",
		src:    "<?php\n$a = [[1, 2], [3]];\n$b = $a;\n$a[0][0] = 9;\necho $b[0][0], $a[0][0], \"\\n\";\n$inner = $a[1];\n$a[1][0] = 7;\n$a[1][] = 8;\necho $inner[0], $a[1][0], $a[1][1], isset($inner[1]) ? \"leak\" : \"ok\", \"\\n\";\nfunction g($p) { $p[\"x\"][\"y\"] = 1; $p[0][0]++; return $p; }\n$q = [[5]];\n$r = g($q);\necho $q[0][0], $r[0][0], isset($q[\"x\"]) ? \"leak\" : \"ok\", $r[\"x\"][\"y\"], \"\\n\";\n",
		stdout: "19\n378ok\n56ok1\n",
		logOut: "19\n378ok\n56ok1\n",
		logErr: "",
		exit:   0},
	{name: "s2_nested_incdec_string",
		src:    "<?php\n$a = [\"str\"];\n$a[0][1]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_nested_incdec_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_incdec_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot increment/decrement string offsets in /w/cases/s2_nested_incdec_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_incdec_string.php on line 3\n",
		exit:   255},
	{name: "s2_nested_incdec_through_string",
		src:    "<?php\n$s = \"str\";\n$s[0][1]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_incdec_through_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_incdec_through_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_incdec_through_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_incdec_through_string.php on line 3\n",
		exit:   255},
	{name: "s2_nested_lines",
		src:    "<?php\n$a = [];\n$a\n[$u1]\n[$u2]\n=\n$u3;\necho \"|\\n\";\nunset(\n$a\n[$u4]\n[$u5]\n);\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_nested_lines.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 4\n\nWarning: Undefined variable $u2 in /w/cases/s2_nested_lines.php on line 5\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 5\n\nWarning: Undefined variable $u3 in /w/cases/s2_nested_lines.php on line 5\n|\n\nWarning: Undefined variable $u4 in /w/cases/s2_nested_lines.php on line 11\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 11\n\nWarning: Undefined variable $u5 in /w/cases/s2_nested_lines.php on line 12\n|\n",
		logOut: "|\n|\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_nested_lines.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 4\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_nested_lines.php on line 5\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 5\nPHP Warning:  Undefined variable $u3 in /w/cases/s2_nested_lines.php on line 5\nPHP Warning:  Undefined variable $u4 in /w/cases/s2_nested_lines.php on line 11\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_lines.php on line 11\nPHP Warning:  Undefined variable $u5 in /w/cases/s2_nested_lines.php on line 12\n",
		exit:   0},
	{name: "s2_nested_order",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$a = [];\n$a[$u1][$u2] = $u3;\necho \"|\\n\";\n$b[f(1)][f(2)] = f(3);\necho $b[1][2], \"\\n\";\n$k = 1;\n$c = [];\n$c[$k][$k = 2] = $k;\necho isset($c[2][2]) ? \"22\" : \"-\", isset($c[1][2]) ? \"12\" : \"-\", \"\\n\";\n$d = [];\n$d[$k++][$k++] = $k++;\necho isset($d[2][3]) ? \"23\" : \"-\", isset($d[3][4]) ? \"34\" : \"-\", $k, \"\\n\";\n$e = [];\n$e[f(4)][$uq][f(5)] = $uw;\necho \"|\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_nested_order.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 4\n\nWarning: Undefined variable $u2 in /w/cases/s2_nested_order.php on line 4\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 4\n\nWarning: Undefined variable $u3 in /w/cases/s2_nested_order.php on line 4\n|\n[f1][f2][f3]3\n22-\n23-5\n[f4][f5]\nWarning: Undefined variable $uq in /w/cases/s2_nested_order.php on line 16\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 16\n\nWarning: Undefined variable $uw in /w/cases/s2_nested_order.php on line 16\n|\n",
		logOut: "|\n[f1][f2][f3]3\n22-\n23-5\n[f4][f5]|\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_nested_order.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 4\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_nested_order.php on line 4\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 4\nPHP Warning:  Undefined variable $u3 in /w/cases/s2_nested_order.php on line 4\nPHP Warning:  Undefined variable $uq in /w/cases/s2_nested_order.php on line 16\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_order.php on line 16\nPHP Warning:  Undefined variable $uw in /w/cases/s2_nested_order.php on line 16\n",
		exit:   0},
	{name: "s2_nested_rw_missing_array_key",
		src:    "<?php\n$a = [];\n$a[[]][1]++;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_rw_missing_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_rw_missing_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_rw_missing_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_rw_missing_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_nested_self",
		src:    "<?php\n$a = [[1]];\n$a[0][1] = $a;\necho isset($a[0][1][0][1]) ? \"rec\" : \"flat\", $a[0][1][0][0], \"\\n\";\n$b = [];\n$b[\"x\"][\"y\"] = $b;\necho isset($b[\"x\"][\"y\"][\"x\"]) ? \"rec\" : \"flat\", \"\\n\";\n$n[1][2] = $n;\necho $n[1][2] === null ? \"null\" : \"other\", \"\\n\";\n",
		stdout: "flat1\nflat\n\nWarning: Undefined variable $n in /w/cases/s2_nested_self.php on line 8\nnull\n",
		logOut: "flat1\nflat\nnull\n",
		logErr: "PHP Warning:  Undefined variable $n in /w/cases/s2_nested_self.php on line 8\n",
		exit:   0},
	{name: "s2_nested_unset",
		src:    "<?php\n$a = [\"n\" => [\"x\" => 1, \"y\" => 2], \"s\" => [1 => [2 => 3, 4 => 5]]];\nunset($a[\"n\"][\"x\"]);\nunset($a[\"s\"][1][2]);\nunset($a[\"no\"][\"x\"]);\nunset($a[\"n\"][\"zz\"][\"q\"]);\necho isset($a[\"n\"][\"x\"]) ? \"x\" : \"-\", isset($a[\"n\"][\"y\"]) ? \"y\" : \"-\", isset($a[\"s\"][1][2]) ? \"2\" : \"-\", isset($a[\"s\"][1][4]) ? \"4\" : \"-\", isset($a[\"no\"]) ? \"created\" : \"none\", isset($a[\"n\"][\"zz\"]) ? \"created\" : \"none\", \"\\n\";\nunset($u[\"a\"][\"b\"]);\necho isset($u) ? \"set\" : \"unset\", \"\\n\";\n$n = null;\nunset($n[\"a\"][\"b\"]);\n$b = $a;\nunset($b[\"n\"][\"y\"]);\necho isset($a[\"n\"][\"y\"]) ? \"kept\" : \"lost\", \"\\n\";\nunset($a[$k1][$k2]);\necho \"|\\n\";\n",
		stdout: "-y-4nonenone\n\nWarning: Undefined variable $u in /w/cases/s2_nested_unset.php on line 8\nunset\nkept\n\nWarning: Undefined variable $k1 in /w/cases/s2_nested_unset.php on line 15\n\nDeprecated: Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_unset.php on line 15\n\nWarning: Undefined variable $k2 in /w/cases/s2_nested_unset.php on line 15\n|\n",
		logOut: "-y-4nonenone\nunset\nkept\n|\n",
		logErr: "PHP Warning:  Undefined variable $u in /w/cases/s2_nested_unset.php on line 8\nPHP Warning:  Undefined variable $k1 in /w/cases/s2_nested_unset.php on line 15\nPHP Deprecated:  Using null as an array offset is deprecated, use an empty string instead in /w/cases/s2_nested_unset.php on line 15\nPHP Warning:  Undefined variable $k2 in /w/cases/s2_nested_unset.php on line 15\n",
		exit:   0},
	{name: "s2_nested_unset_array_key",
		src:    "<?php\n$a = [1];\nunset($a[[]][1]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_unset_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on array in /w/cases/s2_nested_unset_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_nested_unset_false",
		src:    "<?php\n$f = false;\nunset($f[\"a\"][\"b\"]);\necho $f === false ? \"false\" : \"changed\", \"\\n\";\n$g = [false];\nunset($g[0][1]);\necho $g[0] === false ? \"false\" : \"changed\", \"\\n\";\n",
		stdout: "\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_nested_unset_false.php on line 3\nfalse\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_nested_unset_false.php on line 6\nfalse\n",
		logOut: "false\nfalse\n",
		logErr: "PHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_nested_unset_false.php on line 3\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_nested_unset_false.php on line 6\n",
		exit:   0},
	{name: "s2_nested_unset_scalar",
		src:    "<?php\n$i = [5];\nunset($i[0][1]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot unset offset in a non-array variable in /w/cases/s2_nested_unset_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_scalar.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot unset offset in a non-array variable in /w/cases/s2_nested_unset_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_scalar.php on line 3\n",
		exit:   255},
	{name: "s2_nested_unset_scalar_var",
		src:    "<?php\n$i = 5;\nunset($i[$u1][$u2]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot unset offset in a non-array variable in /w/cases/s2_nested_unset_scalar_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_scalar_var.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot unset offset in a non-array variable in /w/cases/s2_nested_unset_scalar_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_scalar_var.php on line 3\n",
		exit:   255},
	{name: "s2_nested_unset_string",
		src:    "<?php\n$s = [\"str\"];\nunset($s[0][$uk]);\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_nested_unset_string.php on line 3\n\nFatal error: Uncaught Error: Cannot unset string offsets in /w/cases/s2_nested_unset_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_nested_unset_string.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot unset string offsets in /w/cases/s2_nested_unset_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_string.php on line 3\n",
		exit:   255},
	{name: "s2_nested_unset_string_var",
		src:    "<?php\n$s = \"str\";\nunset($s[$u1][$u2]);\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_nested_unset_string_var.php on line 3\n\nWarning: String offset cast occurred in /w/cases/s2_nested_unset_string_var.php on line 3\n\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_unset_string_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_string_var.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_nested_unset_string_var.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_nested_unset_string_var.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_unset_string_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_unset_string_var.php on line 3\n",
		exit:   255},
	{name: "s2_nested_write",
		src:    "<?php\n$a[1][2] = 3;\n$b[\"x\"][\"y\"][\"z\"] = 1;\n$c[][] = 5;\n$c[0][] = 6;\n$c[][] = 7;\n$d[][][] = 1;\n$e = [1 => [2 => \"old\"]];\n$e[1][2] = \"new\";\n$e[1][3] = \"add\";\necho $a[1][2], $b[\"x\"][\"y\"][\"z\"], $c[0][0], $c[0][1], $c[1][0], $d[0][0][0], $e[1][2], $e[1][3], \"\\n\";\n$x = $g[1][2] = \"v\";\necho $x, $g[1][2], \"\\n\";\n",
		stdout: "315671newadd\nvv\n",
		logOut: "315671newadd\nvv\n",
		logErr: "",
		exit:   0},
	{name: "s2_nested_write_false",
		src:    "<?php\n$a = [false, null];\n$a[0][1] = 2;\n$a[1][\"k\"] = 3;\necho $a[0][1], $a[1][\"k\"], \"\\n\";\n$f = false;\n$f[1][2] = 3;\necho $f[1][2], \"\\n\";\n",
		stdout: "\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_nested_write_false.php on line 3\n23\n\nDeprecated: Automatic conversion of false to array is deprecated in /w/cases/s2_nested_write_false.php on line 7\n3\n",
		logOut: "23\n3\n",
		logErr: "PHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_nested_write_false.php on line 3\nPHP Deprecated:  Automatic conversion of false to array is deprecated in /w/cases/s2_nested_write_false.php on line 7\n",
		exit:   0},
	{name: "s2_nested_write_scalar",
		src:    "<?php\n$a = [5];\n$a[0][$uk] = $uv;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_nested_write_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_scalar.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_nested_write_scalar.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_scalar.php on line 3\n",
		exit:   255},
	{name: "s2_nested_write_scalar_outer",
		src:    "<?php\n$i = 5;\n$i[$u1][$u2] = $u3;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_nested_write_scalar_outer.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_scalar_outer.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use a scalar value as an array in /w/cases/s2_nested_write_scalar_outer.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_scalar_outer.php on line 3\n",
		exit:   255},
	{name: "s2_nested_write_string_append_mid",
		src:    "<?php\n$s = \"str\";\n$s[][0] = 1;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: [] operator not supported for strings in /w/cases/s2_nested_write_string_append_mid.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_append_mid.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: [] operator not supported for strings in /w/cases/s2_nested_write_string_append_mid.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_append_mid.php on line 3\n",
		exit:   255},
	{name: "s2_nested_write_string_deep",
		src:    "<?php\n$a = [\"str\"];\n$a[0][1][2] = 1;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_write_string_deep.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_deep.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_write_string_deep.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_deep.php on line 3\n",
		exit:   255},
	{name: "s2_nested_write_string_elem",
		src:    "<?php\n$a = [\"str\"];\n$a[0][1] = \"X\";\necho $a[0], \"\\n\";\n$a[0][5] = \"Y\";\necho $a[0], \"|\\n\";\n$a[0][] = \"Z\";\necho \"end\\n\";\n",
		stdout: "sXr\nsXr  Y|\n\nFatal error: Uncaught Error: [] operator not supported for strings in /w/cases/s2_nested_write_string_elem.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_elem.php on line 7\n",
		logOut: "sXr\nsXr  Y|\n",
		logErr: "PHP Fatal error:  Uncaught Error: [] operator not supported for strings in /w/cases/s2_nested_write_string_elem.php:7\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_elem.php on line 7\n",
		exit:   255},
	{name: "s2_nested_write_string_var",
		src:    "<?php\n$s = \"str\";\n$s[$u1][$u2] = $u3;\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $u1 in /w/cases/s2_nested_write_string_var.php on line 3\n\nWarning: String offset cast occurred in /w/cases/s2_nested_write_string_var.php on line 3\n\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_write_string_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_var.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_nested_write_string_var.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_nested_write_string_var.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_nested_write_string_var.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_nested_write_string_var.php on line 3\n",
		exit:   255},
	{name: "s2_so_basic",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"hello\";\necho $s[0], $s[4], $s[-1], $s[-5], $s[1], \"\\n\";\necho d($s[2]), d(\"abc\"[1]), d(\"abc\"[-1]), \"\\n\";\n$n = \"123\";\necho d($n[1]), $n[0] + $n[2], \"\\n\";\n$i = 2;\necho $s[$i], $s[$i - 1], $s[-$i], \"\\n\";\nfunction at($str, $p) { return $str[$p]; }\necho at(\"world\", 3), at(\"x\", 0), \"\\n\";\n$b = \"a\\0b\";\necho d($b[1] === \"\\0\"), $b[2], \"\\n\";\n",
		stdout: "hoohe\nstr(l)str(b)str(c)\nstr(2)4\nlel\nlx\ntrueb\n",
		logOut: "hoohe\nstr(l)str(b)str(c)\nstr(2)4\nlel\nlx\ntrueb\n",
		logErr: "",
		exit:   0},
	{name: "s2_so_coalesce_array_key",
		src:    "<?php\n$s = \"abc\";\necho $s[[]] ?? \"d\", \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_so_coalesce_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_coalesce_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_so_coalesce_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_coalesce_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_so_empty_unset",
		src:    "<?php\n$s = \"abc\";\nunset($s[0]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot unset string offsets in /w/cases/s2_so_empty_unset.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_empty_unset.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot unset string offsets in /w/cases/s2_so_empty_unset.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_empty_unset.php on line 3\n",
		exit:   255},
	{name: "s2_so_in_ops",
		src:    "<?php\n$s = \"a1b2\";\n$t = $s[1] + $s[3];\n$u = $s[0] . $s[2];\necho $t, $u, $s[1] == \"1\" ? \"eq\" : \"ne\", $s[0] === \"a\" ? \"id\" : \"ni\", \"\\n\";\n$out = \"\";\nfor ($i = 0; isset($s[$i]); $i++) { $out = $s[$i] . $out; }\necho $out, \"\\n\";\n$c = 0;\nforeach ([0, 1, 2, 3] as $p) { if ($s[$p] == \"1\" || $s[$p] == \"2\") $c++; }\necho $c, \"\\n\";\n$arr = [$s[0] => $s[1], $s[2] => $s[3]];\necho $arr[\"a\"], $arr[\"b\"], \"\\n\";\nif ($s[9] ?? false) echo \"never\"; else echo \"else\\n\";\n$w = \"\";\nforeach ([\"x\", \"yz\", \"\"] as $str) { $w .= isset($str[0]) ? $str[0] : \"-\"; $w .= $str[1] ?? \"?\"; }\necho $w, \"\\n\";\n",
		stdout: "3abeqid\n2b1a\n2\n12\nelse\nx?yz-?\n",
		logOut: "3abeqid\n2b1a\n2\n12\nelse\nx?yz-?\n",
		logErr: "",
		exit:   0},
	{name: "s2_so_interp",
		src:    "<?php\n$s = \"hello\";\n$a = [\"k\" => \"str\"];\necho \"$s[0]$s[4]|$s[-1]|{$s[1]}|${s}|{$a[\"k\"][1]}|$a[k]\\n\";\necho \"<$s[7]>\\n\";\necho \"<$s[-9]>\\n\";\n$i = 1;\necho \"$s[$i]|{$s[$i + 1]}\\n\";\n$x = <<<EOT\n$s[0]-{$s[-2]}\nEOT;\necho $x, \"\\n\";\n",
		stdout: "\nDeprecated: Using ${var} in strings is deprecated, use {$var} instead in /w/cases/s2_so_interp.php on line 4\nho|o|e|hello|t|str\n\nWarning: Uninitialized string offset 7 in /w/cases/s2_so_interp.php on line 5\n<>\n\nWarning: Uninitialized string offset -9 in /w/cases/s2_so_interp.php on line 6\n<>\ne|l\nh-l\n",
		logOut: "ho|o|e|hello|t|str\n<>\n<>\ne|l\nh-l\n",
		logErr: "PHP Deprecated:  Using ${var} in strings is deprecated, use {$var} instead in /w/cases/s2_so_interp.php on line 4\nPHP Warning:  Uninitialized string offset 7 in /w/cases/s2_so_interp.php on line 5\nPHP Warning:  Uninitialized string offset -9 in /w/cases/s2_so_interp.php on line 6\n",
		exit:   0},
	{name: "s2_so_interp_bare_key",
		src:    "<?php\n$s = \"hello\";\necho \"a\\n\";\necho \"$s[foo]\\n\";\necho \"end\\n\";\n",
		stdout: "a\n\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_interp_bare_key.php:4\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_interp_bare_key.php on line 4\n",
		logOut: "a\n",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_interp_bare_key.php:4\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_interp_bare_key.php on line 4\n",
		exit:   255},
	{name: "s2_so_isset_coalesce",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abc\";\necho d(isset($s[0])), d(isset($s[3])), d(isset($s[-3])), d(isset($s[-4])), d(isset($s[\"1\"])), d(isset($s[\"x\"])), d(isset($s[\"1x\"])), d(isset($s[1.5])), d(isset($s[true])), d(isset($s[null])), d(isset($s[[]])), d(isset($s[\"1.0\"])), d(isset($s[\" 1\"])), d(isset($s[\"1 \"])), \"\\n\";\necho d($s[0] ?? \"d\"), d($s[3] ?? \"d\"), d($s[-1] ?? \"d\"), d($s[-4] ?? \"d\"), d($s[\"1\"] ?? \"d\"), d($s[\"x\"] ?? \"d\"), d($s[\"1x\"] ?? \"d\"), d($s[1.5] ?? \"d\"), d($s[true] ?? \"d\"), d($s[null] ?? \"d\"), d($s[\"1.0\"] ?? \"d\"), d($s[\" 1\"] ?? \"d\"), d($s[\"\"] ?? \"d\"), d($s[1e19] ?? \"d\"), d($s[$uk] ?? \"d\"), \"\\n\";\necho d(isset($s[0][0])), d(isset($s[0][1])), d(isset($s[5][0])), d($s[0][0] ?? \"d\"), d($s[9][0] ?? \"d\"), d($s[0][0][0][0] ?? \"d\"), \"\\n\";\n$a = [\"k\" => \"str\"];\necho d(isset($a[\"k\"][2])), d(isset($a[\"k\"][3])), d($a[\"k\"][1] ?? \"d\"), d($a[\"k\"][7] ?? \"d\"), d($a[\"z\"][0] ?? \"d\"), \"\\n\";\n",
		stdout: "truefalsetruefalsetruefalsefalse\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_so_isset_coalesce.php on line 5\ntruetruetruefalsefalsetruetrue\nstr(a)str(d)str(c)str(d)str(b)str(d)\nWarning: Illegal string offset \"1x\" in /w/cases/s2_so_isset_coalesce.php on line 6\nstr(b)str(b)str(b)str(a)str(d)str(b)str(d)str(d)\nWarning: Undefined variable $uk in /w/cases/s2_so_isset_coalesce.php on line 6\nstr(a)\ntruefalsefalsestr(a)str(d)str(a)\ntruefalsestr(t)str(d)str(d)\n",
		logOut: "truefalsetruefalsetruefalsefalsetruetruetruefalsefalsetruetrue\nstr(a)str(d)str(c)str(d)str(b)str(d)str(b)str(b)str(b)str(a)str(d)str(b)str(d)str(d)str(a)\ntruefalsefalsestr(a)str(d)str(a)\ntruefalsestr(t)str(d)str(d)\n",
		logErr: "PHP Deprecated:  Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_so_isset_coalesce.php on line 5\nPHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_so_isset_coalesce.php on line 6\nPHP Warning:  Undefined variable $uk in /w/cases/s2_so_isset_coalesce.php on line 6\n",
		exit:   0},
	{name: "s2_so_key_array",
		src:    "<?php\n$s = \"abcdef\";\necho $s[[]], \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_so_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_array.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_so_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_array.php on line 3\n",
		exit:   255},
	{name: "s2_so_key_big_float",
		src:      "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abcdef\";\necho d($s[1e19]), \"\\n\";\necho d($s[-1e19]), \"\\n\";\necho d($s[0.0]), \"\\n\";\necho d($s[\"  1\"]), d($s[\"1  \"]), d($s[\"+1\"]), d($s[\"01\"]), \"\\n\";\necho \"end\\n\";\n",
		stdout:   "\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 5\n\nWarning: Uninitialized string offset -8446744073709551616 in /w/cases/s2_so_key_big_float.php on line 5\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 6\n\nWarning: Uninitialized string offset 8446744073709551616 in /w/cases/s2_so_key_big_float.php on line 6\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 7\nstr(a)\nstr(b)str(b)str(b)str(b)\nend\n",
		stdout32: "\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 5\n\nWarning: Uninitialized string offset -1981284352 in /w/cases/s2_so_key_big_float.php on line 5\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 6\n\nWarning: Uninitialized string offset 1981284352 in /w/cases/s2_so_key_big_float.php on line 6\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 7\nstr(a)\nstr(b)str(b)str(b)str(b)\nend\n", exit32: 0, differs32: true,
		logOut: "str()\nstr()\nstr(a)\nstr(b)str(b)str(b)str(b)\nend\n",
		logErr: "PHP Warning:  String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 5\nPHP Warning:  Uninitialized string offset -8446744073709551616 in /w/cases/s2_so_key_big_float.php on line 5\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 6\nPHP Warning:  Uninitialized string offset 8446744073709551616 in /w/cases/s2_so_key_big_float.php on line 6\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_big_float.php on line 7\n",
		exit:   0},
	{name: "s2_so_key_empty_string",
		src:    "<?php\n$s = \"abcdef\";\necho $s[\"\"], \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_empty_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_empty_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_empty_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_empty_string.php on line 3\n",
		exit:   255},
	{name: "s2_so_key_exp_string",
		src:    "<?php\n$s = \"abcdef\";\necho $s[\"1e1\"], \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_exp_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_exp_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_exp_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_exp_string.php on line 3\n",
		exit:   255},
	{name: "s2_so_key_float_string",
		src:    "<?php\n$s = \"abcdef\";\necho $s[\"1.5\"], \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_float_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_float_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_float_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_float_string.php on line 3\n",
		exit:   255},
	{name: "s2_so_key_huge_float",
		src:      "<?php\n$s = \"abcdef\";\necho $s[1e30], \"|\\n\";\necho $s[-1e30], \"|\\n\";\necho $s[1e30] ?? \"d\", $s[18446744073709551616.0] ?? \"d\", \"\\n\";\n$s[18446744073709551616.0] = \"X\";\necho $s, \"\\n\";\n",
		stdout:   "\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 3\n\nWarning: Uninitialized string offset 5076964154930102272 in /w/cases/s2_so_key_huge_float.php on line 3\n|\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 4\n\nWarning: Uninitialized string offset -5076964154930102272 in /w/cases/s2_so_key_huge_float.php on line 4\n|\nda\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 6\nXbcdef\n",
		stdout32: "\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 3\na|\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 4\na|\naa\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 6\nXbcdef\n", exit32: 0, differs32: true,
		logOut: "|\n|\nda\nXbcdef\n",
		logErr: "PHP Warning:  String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 3\nPHP Warning:  Uninitialized string offset 5076964154930102272 in /w/cases/s2_so_key_huge_float.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 4\nPHP Warning:  Uninitialized string offset -5076964154930102272 in /w/cases/s2_so_key_huge_float.php on line 4\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_huge_float.php on line 6\n",
		exit:   0},
	{name: "s2_so_key_overflow_string",
		src:      "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abc\";\necho d($s[\"2147483648\"] ?? \"d\"), \"\\n\";\necho d($s[2147483648] ?? \"d\"), \"\\n\";\necho d($s[2147483648]), \"\\n\";\necho d($s[-2147483649]), \"\\n\";\necho d($s[\"2147483648\"]), \"\\n\";\necho d($s[\"99999999999999999999\"] ?? \"d\"), \"\\n\";\necho d($s[\"99999999999999999999\"]), \"\\n\";\n",
		stdout:   "str(d)\nstr(d)\n\nWarning: Uninitialized string offset 2147483648 in /w/cases/s2_so_key_overflow_string.php on line 7\nstr()\n\nWarning: Uninitialized string offset -2147483649 in /w/cases/s2_so_key_overflow_string.php on line 8\nstr()\n\nWarning: Uninitialized string offset 2147483648 in /w/cases/s2_so_key_overflow_string.php on line 9\nstr()\nstr(d)\n\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_overflow_string.php:11\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_overflow_string.php on line 11\n",
		stdout32: "str(d)\nstr(d)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_overflow_string.php on line 7\n\nWarning: Uninitialized string offset -2147483648 in /w/cases/s2_so_key_overflow_string.php on line 7\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_overflow_string.php on line 8\n\nWarning: Uninitialized string offset 2147483647 in /w/cases/s2_so_key_overflow_string.php on line 8\nstr()\n\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_overflow_string.php:9\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_overflow_string.php on line 9\n", exit32: 255, differs32: true,
		logOut: "str(d)\nstr(d)\nstr()\nstr()\nstr()\nstr(d)\n",
		logErr: "PHP Warning:  Uninitialized string offset 2147483648 in /w/cases/s2_so_key_overflow_string.php on line 7\nPHP Warning:  Uninitialized string offset -2147483649 in /w/cases/s2_so_key_overflow_string.php on line 8\nPHP Warning:  Uninitialized string offset 2147483648 in /w/cases/s2_so_key_overflow_string.php on line 9\nPHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_overflow_string.php:11\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_overflow_string.php on line 11\n",
		exit:   255},
	{name: "s2_so_key_string",
		src:    "<?php\n$s = \"abcdef\";\necho $s[\"x\"], \"\\n\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_string.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_so_key_string.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_so_key_string.php on line 3\n",
		exit:   255},
	{name: "s2_so_key_types",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abcdef\";\necho d($s[\"1\"]), \"\\n\";\necho d($s[\" 2\"]), \"\\n\";\necho d($s[\"3 \"]), \"\\n\";\necho d($s[\"1x\"]), \"\\n\";\necho d($s[1.7]), \"\\n\";\necho d($s[true]), \"\\n\";\necho d($s[false]), \"\\n\";\necho d($s[null]), \"\\n\";\necho d($s[\"-1\"]), \"\\n\";\necho d($s[\"0x1\"]), \"\\n\";\necho d($s[-0.5]), \"\\n\";\necho d($s[1e3]), \"\\n\";\necho d($s[\"9\"]), \"\\n\";\necho d($s[$uk]), \"\\n\";\n",
		stdout: "str(b)\nstr(c)\nstr(d)\n\nWarning: Illegal string offset \"1x\" in /w/cases/s2_so_key_types.php on line 8\nstr(b)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 9\nstr(b)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 10\nstr(b)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 11\nstr(a)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 12\nstr(a)\nstr(f)\n\nWarning: Illegal string offset \"0x1\" in /w/cases/s2_so_key_types.php on line 14\nstr(a)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 15\nstr(a)\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 16\n\nWarning: Uninitialized string offset 1000 in /w/cases/s2_so_key_types.php on line 16\nstr()\n\nWarning: Uninitialized string offset 9 in /w/cases/s2_so_key_types.php on line 17\nstr()\n\nWarning: Undefined variable $uk in /w/cases/s2_so_key_types.php on line 18\n\nWarning: String offset cast occurred in /w/cases/s2_so_key_types.php on line 18\nstr(a)\n",
		logOut: "str(b)\nstr(c)\nstr(d)\nstr(b)\nstr(b)\nstr(b)\nstr(a)\nstr(a)\nstr(f)\nstr(a)\nstr(a)\nstr()\nstr()\nstr(a)\n",
		logErr: "PHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_so_key_types.php on line 8\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 9\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 10\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 11\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 12\nPHP Warning:  Illegal string offset \"0x1\" in /w/cases/s2_so_key_types.php on line 14\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 15\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 16\nPHP Warning:  Uninitialized string offset 1000 in /w/cases/s2_so_key_types.php on line 16\nPHP Warning:  Uninitialized string offset 9 in /w/cases/s2_so_key_types.php on line 17\nPHP Warning:  Undefined variable $uk in /w/cases/s2_so_key_types.php on line 18\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_key_types.php on line 18\n",
		exit:   0},
	{name: "s2_so_lines",
		src:    "<?php\n$s = \"abc\";\necho $s\n[\n7\n]\n, \"|\\n\";\necho\n$s[\n\"1x\"\n], \"\\n\";\n",
		stdout: "\nWarning: Uninitialized string offset 7 in /w/cases/s2_so_lines.php on line 5\n|\n\nWarning: Illegal string offset \"1x\" in /w/cases/s2_so_lines.php on line 10\nb\n",
		logOut: "|\nb\n",
		logErr: "PHP Warning:  Uninitialized string offset 7 in /w/cases/s2_so_lines.php on line 5\nPHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_so_lines.php on line 10\n",
		exit:   0},
	{name: "s2_so_nested",
		src:    "<?php\n$a = [\"abc\", \"k\" => [\"def\", \"x\" => \"ghi\"]];\necho $a[0][1], $a[\"k\"][0][2], $a[\"k\"][\"x\"][0], \"\\n\";\n$s = \"abc\";\necho $s[0][0], $s[1][0][0], $s[2][-1], \"\\n\";\necho $s[0][1], \"|\\n\";\necho $s[5][0], \"|\\n\";\necho $a[0][9][0], \"|\\n\";\n",
		stdout: "bfg\nabc\n\nWarning: Uninitialized string offset 1 in /w/cases/s2_so_nested.php on line 6\n|\n\nWarning: Uninitialized string offset 5 in /w/cases/s2_so_nested.php on line 7\n\nWarning: Uninitialized string offset 0 in /w/cases/s2_so_nested.php on line 7\n|\n\nWarning: Uninitialized string offset 9 in /w/cases/s2_so_nested.php on line 8\n\nWarning: Uninitialized string offset 0 in /w/cases/s2_so_nested.php on line 8\n|\n",
		logOut: "bfg\nabc\n|\n|\n|\n",
		logErr: "PHP Warning:  Uninitialized string offset 1 in /w/cases/s2_so_nested.php on line 6\nPHP Warning:  Uninitialized string offset 5 in /w/cases/s2_so_nested.php on line 7\nPHP Warning:  Uninitialized string offset 0 in /w/cases/s2_so_nested.php on line 7\nPHP Warning:  Uninitialized string offset 9 in /w/cases/s2_so_nested.php on line 8\nPHP Warning:  Uninitialized string offset 0 in /w/cases/s2_so_nested.php on line 8\n",
		exit:   0},
	{name: "s2_so_order",
		src:    "<?php\nfunction f($x) { echo \"[f$x]\"; return $x; }\n$s = \"abc\";\necho $s[f(1)], f(\"x\")[f(0)], \"\\n\";\necho $s[$u1], \"|\\n\";\necho $u2[$u3][$u4], \"|\\n\";\n$k = 0;\necho $s[$k++], $s[$k++], $s[$k], \"\\n\";\necho $s[$k = 2], $k, \"\\n\";\n",
		stdout: "[f1]b[fx][f0]x\n\nWarning: Undefined variable $u1 in /w/cases/s2_so_order.php on line 5\n\nWarning: String offset cast occurred in /w/cases/s2_so_order.php on line 5\na|\n\nWarning: Undefined variable $u2 in /w/cases/s2_so_order.php on line 6\n\nWarning: Undefined variable $u3 in /w/cases/s2_so_order.php on line 6\n\nWarning: Trying to access array offset on null in /w/cases/s2_so_order.php on line 6\n\nWarning: Undefined variable $u4 in /w/cases/s2_so_order.php on line 6\n\nWarning: Trying to access array offset on null in /w/cases/s2_so_order.php on line 6\n|\nabc\nc2\n",
		logOut: "[f1]b[fx][f0]x\na|\n|\nabc\nc2\n",
		logErr: "PHP Warning:  Undefined variable $u1 in /w/cases/s2_so_order.php on line 5\nPHP Warning:  String offset cast occurred in /w/cases/s2_so_order.php on line 5\nPHP Warning:  Undefined variable $u2 in /w/cases/s2_so_order.php on line 6\nPHP Warning:  Undefined variable $u3 in /w/cases/s2_so_order.php on line 6\nPHP Warning:  Trying to access array offset on null in /w/cases/s2_so_order.php on line 6\nPHP Warning:  Undefined variable $u4 in /w/cases/s2_so_order.php on line 6\nPHP Warning:  Trying to access array offset on null in /w/cases/s2_so_order.php on line 6\n",
		exit:   0},
	{name: "s2_so_out_of_range",
		src:      "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abc\";\necho d($s[3]), \"\\n\";\necho d($s[-4]), \"\\n\";\necho d($s[100]), \"\\n\";\n$e = \"\";\necho d($e[0]), d($e[-1]), \"\\n\";\necho d(\"lit\"[9]), \"\\n\";\necho d($s[9223372036854775807]), \"\\n\";\n",
		stdout:   "\nWarning: Uninitialized string offset 3 in /w/cases/s2_so_out_of_range.php on line 5\nstr()\n\nWarning: Uninitialized string offset -4 in /w/cases/s2_so_out_of_range.php on line 6\nstr()\n\nWarning: Uninitialized string offset 100 in /w/cases/s2_so_out_of_range.php on line 7\nstr()\n\nWarning: Uninitialized string offset 0 in /w/cases/s2_so_out_of_range.php on line 9\nstr()\nWarning: Uninitialized string offset -1 in /w/cases/s2_so_out_of_range.php on line 9\nstr()\n\nWarning: Uninitialized string offset 9 in /w/cases/s2_so_out_of_range.php on line 10\nstr()\n\nWarning: Uninitialized string offset 9223372036854775807 in /w/cases/s2_so_out_of_range.php on line 11\nstr()\n",
		stdout32: "\nWarning: Uninitialized string offset 3 in /w/cases/s2_so_out_of_range.php on line 5\nstr()\n\nWarning: Uninitialized string offset -4 in /w/cases/s2_so_out_of_range.php on line 6\nstr()\n\nWarning: Uninitialized string offset 100 in /w/cases/s2_so_out_of_range.php on line 7\nstr()\n\nWarning: Uninitialized string offset 0 in /w/cases/s2_so_out_of_range.php on line 9\nstr()\nWarning: Uninitialized string offset -1 in /w/cases/s2_so_out_of_range.php on line 9\nstr()\n\nWarning: Uninitialized string offset 9 in /w/cases/s2_so_out_of_range.php on line 10\nstr()\n\nWarning: String offset cast occurred in /w/cases/s2_so_out_of_range.php on line 11\nstr(a)\n", exit32: 0, differs32: true,
		logOut: "str()\nstr()\nstr()\nstr()str()\nstr()\nstr()\n",
		logErr: "PHP Warning:  Uninitialized string offset 3 in /w/cases/s2_so_out_of_range.php on line 5\nPHP Warning:  Uninitialized string offset -4 in /w/cases/s2_so_out_of_range.php on line 6\nPHP Warning:  Uninitialized string offset 100 in /w/cases/s2_so_out_of_range.php on line 7\nPHP Warning:  Uninitialized string offset 0 in /w/cases/s2_so_out_of_range.php on line 9\nPHP Warning:  Uninitialized string offset -1 in /w/cases/s2_so_out_of_range.php on line 9\nPHP Warning:  Uninitialized string offset 9 in /w/cases/s2_so_out_of_range.php on line 10\nPHP Warning:  Uninitialized string offset 9223372036854775807 in /w/cases/s2_so_out_of_range.php on line 11\n",
		exit:   0},
	{name: "s2_so_scalar_base",
		src:    "<?php\n$i = 12345;\n$f = 1.5;\n$t = true;\necho $i[0], $f[0], $t[0], \"|\\n\";\necho \"x\"[0][0][0], \"|\\n\";\n",
		stdout: "\nWarning: Trying to access array offset on int in /w/cases/s2_so_scalar_base.php on line 5\n\nWarning: Trying to access array offset on float in /w/cases/s2_so_scalar_base.php on line 5\n\nWarning: Trying to access array offset on true in /w/cases/s2_so_scalar_base.php on line 5\n|\nx|\n",
		logOut: "|\nx|\n",
		logErr: "PHP Warning:  Trying to access array offset on int in /w/cases/s2_so_scalar_base.php on line 5\nPHP Warning:  Trying to access array offset on float in /w/cases/s2_so_scalar_base.php on line 5\nPHP Warning:  Trying to access array offset on true in /w/cases/s2_so_scalar_base.php on line 5\n",
		exit:   0},
	{name: "s2_str_write",
		src:    "<?php\n$s = \"abc\";\n$s[1] = \"X\";\necho $s, \"\\n\";\n$s[5] = \"!\";\necho $s, \"|\\n\";\n$s[-1] = \"z\";\necho $s, \"|\\n\";\n$s[0] = \"long\";\necho $s, \"|\\n\";\n$s[\"2\"] = \"q\";\necho $s, \"|\\n\";\n$s[0] = 65;\necho $s, \"|\\n\";\n$s[1] = 1.5;\necho $s, \"|\\n\";\n$s[2] = true;\necho $s, \"|\\n\";\n$e = \"\";\n$e[2] = \"c\";\necho $e, \"|\\n\";\n",
		stdout: "aXc\naXc  !|\naXc  z|\n\nWarning: Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 9\nlXc  z|\nlXq  z|\n\nWarning: Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 13\n6Xq  z|\n\nWarning: Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 15\n61q  z|\n611  z|\n  c|\n",
		logOut: "aXc\naXc  !|\naXc  z|\nlXc  z|\nlXq  z|\n6Xq  z|\n61q  z|\n611  z|\n  c|\n",
		logErr: "PHP Warning:  Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 9\nPHP Warning:  Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 13\nPHP Warning:  Only the first byte will be assigned to the string offset in /w/cases/s2_str_write.php on line 15\n",
		exit:   0},
	{name: "s2_str_write_array_key",
		src:    "<?php\n$s = \"abcdef\";\n$s[[]] = $uv;\necho $s, \"\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_str_write_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_array_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type array on string in /w/cases/s2_str_write_array_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_array_key.php on line 3\n",
		exit:   255},
	{name: "s2_str_write_array_value",
		src:    "<?php\n$s = \"abc\";\n$s[1] = [1];\necho $s, \"\\n\";\n",
		stdout: "\nWarning: Array to string conversion in /w/cases/s2_str_write_array_value.php on line 3\n\nWarning: Only the first byte will be assigned to the string offset in /w/cases/s2_str_write_array_value.php on line 3\naAc\n",
		logOut: "aAc\n",
		logErr: "PHP Warning:  Array to string conversion in /w/cases/s2_str_write_array_value.php on line 3\nPHP Warning:  Only the first byte will be assigned to the string offset in /w/cases/s2_str_write_array_value.php on line 3\n",
		exit:   0},
	{name: "s2_str_write_bad_key",
		src:    "<?php\n$s = \"abc\";\n$s[\"x\"] = \"y\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_str_write_bad_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_bad_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_str_write_bad_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_bad_key.php on line 3\n",
		exit:   255},
	{name: "s2_str_write_cow",
		src:    "<?php\n$s = \"abc\";\n$t = $s;\n$s[0] = \"X\";\necho $s, $t, \"\\n\";\nfunction h($p) { $p[1] = \"Q\"; return $p; }\n$u = h($s);\necho $s, $u, \"\\n\";\n",
		stdout: "Xbcabc\nXbcXQc\n",
		logOut: "Xbcabc\nXbcXQc\n",
		logErr: "",
		exit:   0},
	{name: "s2_str_write_empty",
		src:    "<?php\n$s = \"abc\";\n$s[1] = \"\";\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_empty.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_empty.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_empty.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_empty.php on line 3\n",
		exit:   255},
	{name: "s2_str_write_float_string_key",
		src:    "<?php\n$s = \"abcdef\";\n$s[\"1.5\"] = \"A\";\necho $s, \"\\n\";\n",
		stdout: "\nFatal error: Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_str_write_float_string_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_float_string_key.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught TypeError: Cannot access offset of type string on string in /w/cases/s2_str_write_float_string_key.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_float_string_key.php on line 3\n",
		exit:   255},
	{name: "s2_str_write_keys",
		src:    "<?php\n$s = \"abcdef\";\n$s[\"1x\"] = \"A\";\necho $s, \"\\n\";\n$s[\" 2\"] = \"B\";\necho $s, \"\\n\";\n$s[\"3 \"] = \"C\";\necho $s, \"\\n\";\n$s[true] = \"D\";\necho $s, \"\\n\";\n$s[false] = \"E\";\necho $s, \"\\n\";\n$s[4.7] = \"F\";\necho $s, \"\\n\";\n$s[null] = \"G\";\necho $s, \"\\n\";\n$s[-7] = \"H\";\necho $s, \"\\n\";\n",
		stdout: "\nWarning: Illegal string offset \"1x\" in /w/cases/s2_str_write_keys.php on line 3\naAcdef\naABdef\naABCef\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_keys.php on line 9\naDBCef\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_keys.php on line 11\nEDBCef\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_keys.php on line 13\nEDBCFf\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_keys.php on line 15\nGDBCFf\n\nWarning: Illegal string offset -7 in /w/cases/s2_str_write_keys.php on line 17\nGDBCFf\n",
		logOut: "aAcdef\naABdef\naABCef\naDBCef\nEDBCef\nEDBCFf\nGDBCFf\nGDBCFf\n",
		logErr: "PHP Warning:  Illegal string offset \"1x\" in /w/cases/s2_str_write_keys.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_keys.php on line 9\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_keys.php on line 11\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_keys.php on line 13\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_keys.php on line 15\nPHP Warning:  Illegal string offset -7 in /w/cases/s2_str_write_keys.php on line 17\n",
		exit:   0},
	{name: "s2_str_write_null",
		src:    "<?php\n$s = \"abc\";\n$s[1] = null;\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_null.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_null.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_null.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_null.php on line 3\n",
		exit:   255},
	{name: "s2_str_write_order",
		src:    "<?php\n$s = \"abc\";\n$s[-9] = $uv;\necho $s, \"|\\n\";\n$s[$uk] = $uv2;\necho $s, \"|\\n\";\n",
		stdout: "\nWarning: Illegal string offset -9 in /w/cases/s2_str_write_order.php on line 3\nabc|\n\nWarning: Undefined variable $uk in /w/cases/s2_str_write_order.php on line 5\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_order.php on line 5\n\nWarning: Undefined variable $uv2 in /w/cases/s2_str_write_order.php on line 5\n\nFatal error: Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_order.php:5\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_order.php on line 5\n",
		logOut: "abc|\n",
		logErr: "PHP Warning:  Illegal string offset -9 in /w/cases/s2_str_write_order.php on line 3\nPHP Warning:  Undefined variable $uk in /w/cases/s2_str_write_order.php on line 5\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_order.php on line 5\nPHP Warning:  Undefined variable $uv2 in /w/cases/s2_str_write_order.php on line 5\nPHP Fatal error:  Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_order.php:5\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_order.php on line 5\n",
		exit:   255},
	{name: "s2_str_write_result",
		src:    "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$s = \"abc\";\necho d($s[0] = \"xyz\"), \"\\n\";\n$x = $s[1] = 7;\necho d($x), $s, \"\\n\";\n$y = $s[-9] = \"q\";\necho d($y), $s, \"\\n\";\n$v = \"W\";\n$z = $s[2] = $v;\necho d($z), $s, \"\\n\";\n",
		stdout: "\nWarning: Only the first byte will be assigned to the string offset in /w/cases/s2_str_write_result.php on line 5\nstr(x)\nstr(7)x7c\n\nWarning: Illegal string offset -9 in /w/cases/s2_str_write_result.php on line 8\nnullx7c\nstr(W)x7W\n",
		logOut: "str(x)\nstr(7)x7c\nnullx7c\nstr(W)x7W\n",
		logErr: "PHP Warning:  Only the first byte will be assigned to the string offset in /w/cases/s2_str_write_result.php on line 5\nPHP Warning:  Illegal string offset -9 in /w/cases/s2_str_write_result.php on line 8\n",
		exit:   0},
	{name: "s2_str_write_undefined_value",
		src:    "<?php\n$s = \"abc\";\n$s[$uk] = $uv;\necho \"end\\n\";\n",
		stdout: "\nWarning: Undefined variable $uk in /w/cases/s2_str_write_undefined_value.php on line 3\n\nWarning: String offset cast occurred in /w/cases/s2_str_write_undefined_value.php on line 3\n\nWarning: Undefined variable $uv in /w/cases/s2_str_write_undefined_value.php on line 3\n\nFatal error: Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_undefined_value.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_undefined_value.php on line 3\n",
		logOut: "",
		logErr: "PHP Warning:  Undefined variable $uk in /w/cases/s2_str_write_undefined_value.php on line 3\nPHP Warning:  String offset cast occurred in /w/cases/s2_str_write_undefined_value.php on line 3\nPHP Warning:  Undefined variable $uv in /w/cases/s2_str_write_undefined_value.php on line 3\nPHP Fatal error:  Uncaught Error: Cannot assign an empty string to a string offset in /w/cases/s2_str_write_undefined_value.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_str_write_undefined_value.php on line 3\n",
		exit:   255},
	{name: "s2_unset_string_key_array",
		src:    "<?php\n$s = \"abc\";\nunset($s[[]][0]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot unset string offsets in /w/cases/s2_unset_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_array.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot unset string offsets in /w/cases/s2_unset_string_key_array.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_array.php on line 3\n",
		exit:   255},
	{name: "s2_unset_string_key_leading",
		src:    "<?php\n$s = \"abc\";\nunset($s[\"1x\"][0]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot use string offset as an array in /w/cases/s2_unset_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_leading.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot use string offset as an array in /w/cases/s2_unset_string_key_leading.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_leading.php on line 3\n",
		exit:   255},
	{name: "s2_unset_string_key_nonnumeric",
		src:    "<?php\n$s = \"abc\";\nunset($s[\"x\"][0]);\necho \"end\\n\";\n",
		stdout: "\nFatal error: Uncaught Error: Cannot unset string offsets in /w/cases/s2_unset_string_key_nonnumeric.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_nonnumeric.php on line 3\n",
		logOut: "",
		logErr: "PHP Fatal error:  Uncaught Error: Cannot unset string offsets in /w/cases/s2_unset_string_key_nonnumeric.php:3\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_unset_string_key_nonnumeric.php on line 3\n",
		exit:   255},
	{name: "s2_width_elements",
		src:      "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$a = [2147483647, 9223372036854775807, -2147483648, 1 << 30];\n$a[0]++; $a[1]++; $a[2]--; $a[3] *= 4;\necho d($a[0]), d($a[1]), d($a[2]), d($a[3]), \"\\n\";\n$a[0] = 2147483647;\n$a[0] += 1;\n$b = [1];\n$b[0] <<= 31;\n$b[1] = 5; $b[1] <<= 62;\necho d($a[0]), d($b[0]), d($b[1]), \"\\n\";\n$c = [];\n$c[2147483647] = \"m\";\n$c[] = \"n\";\necho \"end\\n\";\n",
		stdout:   "int(2147483648)\nWarning: The float 9.223372036854776E+18 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(9.2233720368548E+18)int(-2147483649)int(4294967296)\nint(2147483648)int(2147483648)int(4611686018427387904)\nend\n",
		stdout32: "\nWarning: The float 2147483648 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(2147483648)\nWarning: The float 9.223372036854776E+18 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(9.2233720368548E+18)\nWarning: The float -2147483649 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(-2147483649)\nWarning: The float 4294967296 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(4294967296)\n\nWarning: The float 2147483648 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\nfloat(2147483648)int(-2147483648)int(0)\n\nFatal error: Uncaught Error: Cannot add element to the array as the next element is already occupied in /w/cases/s2_width_elements.php:15\nStack trace:\n#0 {main}\n  thrown in /w/cases/s2_width_elements.php on line 15\n", exit32: 255, differs32: true,
		logOut: "int(2147483648)float(9.2233720368548E+18)int(-2147483649)int(4294967296)\nint(2147483648)int(2147483648)int(4611686018427387904)\nend\n",
		logErr: "PHP Warning:  The float 9.223372036854776E+18 is not representable as an int, cast occurred in /w/cases/s2_width_elements.php on line 2\n",
		exit:   0},
	{name: "s2_width_foreach_keys",
		src:      "<?php\nfunction d($v) { if ($v === null) return \"null\"; if ($v === true) return \"true\"; if ($v === false) return \"false\"; if ((int)$v === $v) return \"int($v)\"; if ((float)$v === $v) return \"float($v)\"; if ((string)$v === $v) return \"str($v)\"; return \"array\"; }\n\n$a = [2147483647 => \"a\", \"2147483648\" => \"b\", -2147483648 => \"c\", \"-2147483649\" => \"d\", 4294967296 => \"e\"];\nforeach ($a as $k => $v) echo d($k), $v, \" \";\necho \"\\n\";\n$s = \"abc\";\n$s[2147483647 - 2147483647] = \"X\";\necho $s, \"\\n\";\n",
		stdout:   "int(2147483647)a int(2147483648)b int(-2147483648)c int(-2147483649)d int(4294967296)e \nXbc\n",
		stdout32: "\nWarning: The float 4294967296 is not representable as an int, cast occurred in /w/cases/s2_width_foreach_keys.php on line 4\nint(2147483647)a str(2147483648)b int(-2147483648)c str(-2147483649)d int(0)e \nXbc\n", exit32: 0, differs32: true,
		logOut: "int(2147483647)a int(2147483648)b int(-2147483648)c int(-2147483649)d int(4294967296)e \nXbc\n",
		logErr: "",
		exit:   0},
	{name: "s2_def_parse_error",
		src:    "<?php\necho \"a\\n\";\nfunction f($a =\n) {}\n",
		stdout: "\nParse error: syntax error, unexpected token \")\" in /w/cases/s2_def_parse_error.php on line 4\n",
		logOut: "",
		logErr: "PHP Parse error:  syntax error, unexpected token \")\" in /w/cases/s2_def_parse_error.php on line 4\n",
		exit:   255},
	{name: "s2_fe_parse_error",
		src:    "<?php\necho \"a\\n\";\n$a = [1, 2];\nforeach ($a as) {\n  echo \"x\";\n}\n",
		stdout: "\nParse error: syntax error, unexpected token \")\" in /w/cases/s2_fe_parse_error.php on line 4\n",
		logOut: "",
		logErr: "PHP Parse error:  syntax error, unexpected token \")\" in /w/cases/s2_fe_parse_error.php on line 4\n",
		exit:   255},
	{name: "s2_unset_append_both",
		src:    "<?php\necho \"a\\n\";\nunset($b, $a[][]);\n",
		stdout: "\nFatal error: Cannot use [] for unsetting in /w/cases/s2_unset_append_both.php on line 3\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for unsetting in /w/cases/s2_unset_append_both.php on line 3\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_unset_append_inner",
		src:    "<?php\necho \"a\\n\";\nunset($a[][0]);\n",
		stdout: "\nFatal error: Cannot use [] for unsetting in /w/cases/s2_unset_append_inner.php on line 3\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for unsetting in /w/cases/s2_unset_append_inner.php on line 3\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_unset_append_key_lines",
		src:    "<?php\necho \"a\\n\";\nunset($a[\n$k][\n2][]);\n",
		stdout: "\nFatal error: Cannot use [] for unsetting in /w/cases/s2_unset_append_key_lines.php on line 5\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for unsetting in /w/cases/s2_unset_append_key_lines.php on line 5\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_unset_append_lines",
		src:    "<?php\necho \"a\\n\";\nunset(\n$a\n[0]\n[\n]\n[1]);\n",
		stdout: "\nFatal error: Cannot use [] for unsetting in /w/cases/s2_unset_append_lines.php on line 5\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for unsetting in /w/cases/s2_unset_append_lines.php on line 5\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_unset_append_middle",
		src:    "<?php\necho \"a\\n\";\n$a = [[1]];\nunset($a[0][][1]);\n",
		stdout: "\nFatal error: Cannot use [] for unsetting in /w/cases/s2_unset_append_middle.php on line 4\nStack trace:\n#0 {main}\n",
		logOut: "",
		logErr: "PHP Fatal error:  Cannot use [] for unsetting in /w/cases/s2_unset_append_middle.php on line 4\nStack trace:\n#0 {main}\n",
		exit:   255},
	{name: "s2_def_escape_elsewhere",
		src:    "<?php\nfunction f($x = [\"a\", \"b\"][true]) { echo $x, \"\\n\"; }\nf(); f();\nfunction g($x = [1][5] ?: \"b\") { echo $x, \"\\n\"; }\ng(); g(); g();\n",
		stdout: "b\nb\n\nWarning: Undefined array key 5 in /w/cases/s2_def_escape_elsewhere.php on line 4\nb\nb\nb\n",
		logOut: "b\nb\nb\nb\nb\n",
		logErr: "PHP Warning:  Undefined array key 5 in /w/cases/s2_def_escape_elsewhere.php on line 4\n",
		exit:   0},
	{name: "s2_def_int_operators",
		src:    "<?php\nfunction h($x = (string)(1.5 | 0)) { return $x; }\necho \"[\", h(), \"]\", \"[\", h(), \"]\", \"\\n\";\nfunction i($x = (string)([1][5] % 7)) { return $x; }\necho \"[\", i(), \"]\", \"[\", i(), \"]\", \"\\n\";\nfunction j($x = \"x\" . ([1][5] << 1)) { return $x; }\necho \"[\", j(), \"]\", \"[\", j(), \"]\", \"\\n\";\nfunction k($x = (string)([1][5] === null)) { return $x; }\necho \"[\", k(), \"]\", \"[\", k(), \"]\", \"\\n\";\nfunction m($x = (string)(1 | [1][5])) { return $x; }\necho \"[\", m(), \"]\", \"[\", m(), \"]\", \"\\n\";\nfunction n($x = (string)(int)([1][5] . \"5\")) { return $x; }\necho \"[\", n(), \"]\", \"[\", n(), \"]\", \"\\n\";\nfunction o($x = (string)!([1][5])) { return $x; }\necho \"[\", o(), \"]\", \"[\", o(), \"]\", \"\\n\";\nfunction p($x = (string)([1][5] >> 1)) { return $x; }\necho \"[\", p(), \"]\", \"[\", p(), \"]\", \"\\n\";\n",
		stdout: "[\nDeprecated: Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_def_int_operators.php on line 2\n1][1]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 4\n0][0]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 6\nx0][\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 6\nx0]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 8\n1][1]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 10\n1][1]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 12\n5][5]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 14\n1][1]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 16\n0][0]\n",
		logOut: "[1][1]\n[0][0]\n[x0][x0]\n[1][1]\n[1][1]\n[5][5]\n[1][1]\n[0][0]\n",
		logErr: "PHP Deprecated:  Implicit conversion from float 1.5 to int loses precision in /w/cases/s2_def_int_operators.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 4\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 6\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 6\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 8\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 10\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 12\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 14\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_int_operators.php on line 16\n",
		exit:   0},
	{name: "s2_def_interned_strings",
		src:    "<?php\nfunction fn2($x = [1][5] ?: __FUNCTION__) { return $x; }\necho \"[\", fn2(), \"][\", fn2(), \"]\\n\";\nfunction fl($x = [1][5] ?: __FILE__) { return $x; }\necho \"[\", fl(), \"][\", fl(), \"]\\n\";\nfunction dr($x = [1][5] ?: __DIR__) { return $x; }\necho \"[\", dr(), \"][\", dr(), \"]\\n\";\nfunction ns($x = [1][5] ?: __NAMESPACE__) { return $x; }\necho \"[\", ns(), \"][\", ns(), \"]\\n\";\nfunction r($x = [1][5] ?: (\"\" . 7)) { return $x; }\necho \"[\", r(), \"][\", r(), \"]\\n\";\nfunction s($x = \"ab\"[5]) { return $x; }\necho \"[\", s(), \"][\", s(), \"]\\n\";\nfunction t($x = [1][5] ?: (string)7) { return $x; }\necho \"[\", t(), \"][\", t(), \"]\\n\";\nfunction u($x = [1][5] ?: (\"\" . -1)) { return $x; }\necho \"[\", u(), \"][\", u(), \"]\\n\";\nfunction v($x = [1][5] ?: (\"\" . \"\")) { return $x; }\necho \"[\", v(), \"][\", v(), \"]\\n\";\nfunction w($x = [1][5] ?: __LINE__ . \"\") { return $x; }\necho \"[\", w(), \"][\", w(), \"]\\n\";\nfunction e($x = [1][5] ?: \"\\nx\"[0]) { return $x; }\necho \"[\", e(), \"]\", \"[\", e(), \"]\", \"\\n\";\nfunction g($x = [1][5] ?: (\"a\" | \"b\")) { return $x; }\necho \"[\", g(), \"]\", \"[\", g(), \"]\", \"\\n\";\nfunction i($x = [1][5] ?: ~\"a\") { return $x; }\necho \"[\", i(), \"]\", \"[\", i(), \"]\", \"\\n\";\n",
		stdout: "[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 2\nfn2][\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 2\nfn2]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 4\n/w/cases/s2_def_interned_strings.php][\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 4\n/w/cases/s2_def_interned_strings.php]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 6\n/w/cases][\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 6\n/w/cases]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 8\n][]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 10\n7][7]\n[\nWarning: Uninitialized string offset 5 in /w/cases/s2_def_interned_strings.php on line 12\n][]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 14\n7][7]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 16\n-1][\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 16\n-1]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 18\n][]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 20\n20][\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 20\n20]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 22\n\n][\n]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 24\nc][c]\n[\nWarning: Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 26\n\x9e][\x9e]\n",
		logOut: "[fn2][fn2]\n[/w/cases/s2_def_interned_strings.php][/w/cases/s2_def_interned_strings.php]\n[/w/cases][/w/cases]\n[][]\n[7][7]\n[][]\n[7][7]\n[-1][-1]\n[][]\n[20][20]\n[\n][\n]\n[c][c]\n[\x9e][\x9e]\n",
		logErr: "PHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 2\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 4\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 4\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 6\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 6\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 8\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 10\nPHP Warning:  Uninitialized string offset 5 in /w/cases/s2_def_interned_strings.php on line 12\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 14\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 16\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 16\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 18\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 20\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 20\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 22\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 24\nPHP Warning:  Undefined array key 5 in /w/cases/s2_def_interned_strings.php on line 26\n",
		exit:   0},
}

func TestW17Stage2Probes(t *testing.T) {
	display := []RunOption{WithINI("display_errors", "1"), WithINI("log_errors", "0"), WithINI("html_errors", "0"), WithINI("error_reporting", "-1")}
	logged := []RunOption{WithINI("display_errors", "0"), WithINI("log_errors", "1"), WithINI("html_errors", "0"), WithINI("error_reporting", "-1")}
	for _, width := range []int{64, 32} {
		e, err := NewEngine(Config{IntSize: width})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range w17Stage2Probes {
			wantOut, wantExit := c.stdout, c.exit
			if width == 32 && c.differs32 {
				wantOut, wantExit = c.stdout32, c.exit32
			}
			t.Run(strings.TrimPrefix(c.name, "s2_")+"/"+map[int]string{64: "display64", 32: "display32"}[width], func(t *testing.T) {
				got := w17Execute(t, e, c.name, c.src, display...)
				w17Stage2Check(t, got, wantOut, "", wantExit)
			})
			if width == 64 {
				t.Run(strings.TrimPrefix(c.name, "s2_")+"/log64", func(t *testing.T) {
					got := w17Execute(t, e, c.name, c.src, logged...)
					w17Stage2Check(t, got, c.logOut, c.logErr, c.exit)
				})
			}
		}
	}
}

func w17Stage2Check(t *testing.T, got w17Observed, stdout, stderr string, exit int) {
	t.Helper()
	var ue *UnsupportedError
	if errors.As(got.err, &ue) {
		t.Fatalf("unsupported: %v", got.err)
	}
	if got.err != nil && got.compileFatal == nil && got.exitCode != 255 {
		t.Fatalf("run failed: %v", got.err)
	}
	w17Compare(t, "stdout", []byte(stdout), got.stdout)
	w17Compare(t, "stderr", []byte(stderr), got.stderr)
	if got.exitCode != exit {
		t.Errorf("exit code %d, want %d (error %v)", got.exitCode, exit, got.err)
	}
}

// The acceptance floor of the stage: at least 60 recorded cases, spread
// over the four groups.
func TestW17Stage2ProbeCoverage(t *testing.T) {
	groups := map[string][]string{
		"element writes":     {"s2_cas_", "s2_coalesce_", "s2_incdec_", "s2_nested_", "s2_str_write", "s2_assign_dim", "s2_fetch_", "s2_unset_", "s2_width_elements"},
		"foreach":            {"s2_fe_", "s2_width_foreach"},
		"parameter defaults": {"s2_def_"},
		"string offsets":     {"s2_so_"},
	}
	counted := 0
	for group, prefixes := range groups {
		n := 0
		for _, c := range w17Stage2Probes {
			for _, p := range prefixes {
				if strings.HasPrefix(c.name, p) {
					n++
					break
				}
			}
		}
		if n < 12 {
			t.Errorf("%s: %d recorded cases, want at least 12", group, n)
		}
		counted += n
	}
	if counted != len(w17Stage2Probes) || counted < 60 {
		t.Errorf("%d of %d cases belong to a group, want all of at least 60", counted, len(w17Stage2Probes))
	}
	seen := map[string]bool{}
	for _, c := range w17Stage2Probes {
		if seen[c.name] {
			t.Errorf("duplicate case %s", c.name)
		}
		seen[c.name] = true
	}
}

// What stage 2 still cannot run is refused by Compile, with the file and
// line of the construct, before anything is executed or printed. php is
// what PHP 8.5.12 printed for the two sources it runs (same command as
// above); the others are refused for a construct on the given line whatever
// PHP would do with the rest.
var w17Stage2Unsupported = []struct {
	name, src, feature string
	line               int
	php                string
}{
	{"s2_dim_write_non_variable", "<?php\nfunction f() { return [1]; }\necho \"a\\n\";\nf()[0] = 1;\necho \"end\\n\";\n",
		"array write through a non-variable", 4, "a\nend\n"},
	{"s2_u_fe_property_target", "<?php\necho \"a\\n\";\nforeach ([1] as $o->p) {}\n", "property access", 3, ""},
	{"s2_u_fe_body", "<?php\necho \"a\\n\";\nforeach ([1] as $v) {\n  static $s;\n}\n", "static variable", 4, ""},
	{"s2_u_cas_non_variable", "<?php\nfunction f() { return [1]; }\necho \"a\\n\";\nf()[0] .= 1;\n", "array write through a non-variable", 4, ""},
	{"s2_u_incdec_non_variable", "<?php\nfunction f() { return [1]; }\necho \"a\\n\";\n\nf()[0]++;\n", "array write through a non-variable", 5, ""},
	{"s2_u_nested_non_variable", "<?php\nfunction f() { return [1]; }\necho \"a\\n\";\nf()[0][1] = 2;\n", "array write through a non-variable", 4, ""},
	{"s2_u_unset_non_variable", "<?php\necho \"a\\n\";\nunset($o->p[1][2]);\n", "property access", 3, ""},
	{"s2_u_coalesce_non_variable", "<?php\necho \"a\\n\";\n$o->p[1] ??= 2;\n", "property access", 3, ""},
	{"s2_u_default_constant", "<?php\necho \"a\\n\";\nfunction f($a = 1,\n  $b = PHP_EOL) {}\n", "constant lookup", 4, ""},
	{"s2_u_default_typed", "<?php\necho \"a\\n\";\nfunction f(int $a = 1) {}\n", "typed parameter", 3, ""},
	{"s2_u_default_variadic", "<?php\necho \"a\\n\";\nfunction f($a = 1, ...$r) {}\n", "variadic parameter", 3, ""},
	{"s2_u_default_class_constant", "<?php\necho \"a\\n\";\nfunction f($a = self::X) {}\n", "class constant", 3, ""},
	{"s2_u_default_float_concat", "<?php\necho \"a\\n\";\nfunction f($a = 1,\n  $b = 1.5 . [1][5]) {}\n", "parameter default whose string value PHP may intern", 4, ""},
	{"s2_u_default_array_concat", "<?php\necho \"a\\n\";\nfunction f($a = [] . [1][5]) {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_escaped_byte", "<?php\necho \"a\\n\";\nfunction f($a = [1][5] ?: \"\\n\") {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_nowdoc_byte", "<?php\necho \"a\\n\";\nfunction f($a = [1][5] ?: <<<'E'\na\nE) {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_escape_fold", "<?php\necho \"a\\n\";\nfunction f($a = [1][5] ?: (\"\\n\" . \"\")) {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_float_fold", "<?php\necho \"a\\n\";\nfunction f($a = [1][5] ?: (\"\" . 1.0)) {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_string_bitwise", "<?php\necho \"a\\n\";\nfunction f($a = ([1][5] ?: \"a\") | \"\") {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_default_bitnot_empty", "<?php\necho \"a\\n\";\nfunction f($a = [1][5] ?: ~\"\") {}\n", "parameter default whose string value PHP may intern", 3, ""},
	{"s2_u_named_argument", "<?php\necho \"a\\n\";\nfunction f($a = 1, $b = 2) {}\nf(b: 3);\n", "named arguments", 4, ""},
}

// w17Stage3Supported are probes the table above refused until stage 3,
// with the output PHP 8.5.12 gives them (the first recorded there before).
var w17Stage3Supported = []struct{ name, src, php string }{
	{"s2_fe_by_ref_literal", "<?php\necho \"a\\n\";\nforeach ([1, 2] as &$v) { echo $v; }\necho \"\\n\";\n", "a\n12\n"},
	{"s2_u_fe_by_ref", "<?php\necho \"a\\n\";\n$a = [1];\nforeach ($a as $k => &$v) {}\n", "a\n"},
	{"s2_u_dim_reference", "<?php\necho \"a\\n\";\n$a[0][1] = &$b;\n", "a\n"},
	{"s2_u_default_by_ref", "<?php\necho \"a\\n\";\nfunction f(&$a = 1) {}\n", "a\n"},
}

// w17Stage4Supported are probes the table above refused until stage 4
// (destructuring), with the output PHP 8.5.12 gives them.
var w17Stage4Supported = []struct{ name, src, php string }{
	{"s2_u_fe_list", "<?php\necho \"a\\n\";\nforeach ([[1, 2]] as [$x, $y]) {}\necho $x, $y, \"\\n\";\n", "a\n12\n"},
	{"s2_u_fe_list_keyed", "<?php\necho \"a\\n\";\n\nforeach ([[1, 2]] as $k => list($x, $y)) {}\necho $k, $x, $y, \"\\n\";\n", "a\n012\n"},
}

func TestW17Stage3RunsFormerlyUnsupportedProbes(t *testing.T) {
	e := mustEngine(t, Config{})
	for _, c := range append(append([]struct{ name, src, php string }(nil), w17Stage3Supported...), w17Stage4Supported...) {
		t.Run(strings.TrimPrefix(c.name, "s2_"), func(t *testing.T) {
			p, err := e.Compile(context.Background(), "/w/cases/"+c.name+".php", c.src)
			if err != nil {
				t.Fatal(err)
			}
			r, err := e.Run(context.Background(), p)
			if err != nil || r.Status != StatusCompleted || string(r.Output) != c.php {
				t.Fatalf("Run = %+v, %v; want output %q", r, err, c.php)
			}
		})
	}
}

func TestW17Stage2UnsupportedIsRefusedAtCompile(t *testing.T) {
	for _, width := range []int{64, 32} {
		e, err := NewEngine(Config{IntSize: width})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range w17Stage2Unsupported {
			t.Run(strings.TrimPrefix(c.name, "s2_"), func(t *testing.T) {
				file := "/w/cases/" + c.name + ".php"
				p, err := e.Compile(context.Background(), file, c.src)
				var ue *UnsupportedError
				if p != nil || !errors.As(err, &ue) {
					t.Fatalf("Compile = (%v, %v), want a located UnsupportedError", p, err)
				}
				if ue.Feature != c.feature || ue.File != file || ue.Line != c.line || ue.Option != "" {
					t.Fatalf("Compile refused %#v, want feature %q at %s:%d", ue, c.feature, file, c.line)
				}
			})
		}
	}
}

// Every case of this file either compiles or is refused by Compile: none
// ends at run time as unsupported, after output may have been sent.
func TestW17Stage2NothingIsUnsupportedAtRunTime(t *testing.T) {
	e, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range w17Stage2Probes {
		got := w17Execute(t, e, c.name, c.src, WithINI("display_errors", "1"), WithINI("log_errors", "0"))
		var ue *UnsupportedError
		if errors.As(got.err, &ue) {
			t.Errorf("%s: %v", c.name, got.err)
		}
	}
}

// A chain of element levels costs no Go stack in the compiler or the VM,
// and the nested arrays it builds are released without recursion: 20 000
// levels, which PHP 8.5.12 also compiles (its own limit lies between 30 000
// and 50 000 levels, engineering/decisions/W-17-vm.md).
func TestW17Stage2DeepElementChains(t *testing.T) {
	const depth = 20000
	chain := strings.Repeat("[0]", depth)
	src := "<?php\n$a" + chain + " = 1;\n$a" + chain + "++;\n$a" + chain + " .= 'x';\n$b" + chain + "[] = 2;\nunset($b" + chain + "[0]);\nunset($a" + chain + ");\necho 'done';\n"
	e, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	got := w17Execute(t, e, "s2_deep", src, WithINI("display_errors", "1"), WithINI("log_errors", "0"))
	if got.err != nil || string(got.stdout) != "done" || got.exitCode != 0 {
		t.Fatalf("%q exit %d: %v", got.stdout, got.exitCode, got.err)
	}
}

// A string offset write that would build a string past PHP's default
// memory_limit ends the run as a memory limit stop: the output sent before
// it stays, nothing is reported as PHP, and the host is not asked for the
// memory. (The pinned image ends such a script with its memory limit error.)
func TestW17Stage2StringOffsetWritePastTheMemoryBound(t *testing.T) {
	e, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []string{"134217728", "300000000", "9223372036854775807"} {
		p, err := e.Compile(context.Background(), "/w/cases/s2_huge.php", "<?php\n$s = \"abc\";\necho \"a\\n\";\n$s["+offset+"] = \"x\";\necho \"end\\n\";\n")
		if err != nil {
			t.Fatal(err)
		}
		res, err := e.Run(context.Background(), p)
		var re *ResourceError
		if !errors.As(err, &re) || re.Resource != ResourceMemory || re.Limit != 128<<20 || re.Used != 3 {
			t.Fatalf("offset %s: error %v", offset, err)
		}
		if res == nil || string(res.Output) != "a\n" || len(res.Diagnostics) != 0 {
			t.Fatalf("offset %s: result %+v", offset, res)
		}
	}
	// One byte below the bound is written.
	p, err := e.Compile(context.Background(), "/w/cases/s2_large.php", "<?php\n$s = \"abc\";\n$s[134217727] = \"x\";\necho $s[134217727], $s[134217726] === \" \" ? \"pad\" : \"?\";\n")
	if err != nil {
		t.Fatal(err)
	}
	if res, err := e.Run(context.Background(), p); err != nil || string(res.Output) != "xpad" {
		t.Fatalf("%v %q", err, res.Output)
	}
}

// w17Stage2Gen writes random programs from the stage 2 forms over a few
// variables. Every program is valid PHP that the compiler accepts.
type w17Stage2Gen struct {
	state uint64
	b     strings.Builder
}

func (g *w17Stage2Gen) n(max int) int {
	g.state = g.state*6364136223846793005 + 1442695040888963407
	return int((g.state >> 33) % uint64(max))
}

func (g *w17Stage2Gen) pick(xs ...string) string { return xs[g.n(len(xs))] }

func (g *w17Stage2Gen) v() string { return g.pick("$a", "$b", "$c", "$s") }

func (g *w17Stage2Gen) atom() string {
	return g.pick("0", "1", "-1", "2", "7", "1.5", "'x'", "'abc'", "''", "'1x'", "null", "true", "false", "[]", "[1, 2]", "['k' => [3]]", g.v(), g.v(), g.v())
}

func (g *w17Stage2Gen) dims() string {
	out := ""
	for i := g.n(3); i >= 0; i-- {
		out += "[" + g.pick(g.atom(), g.atom(), "") + "]"
	}
	return out
}

func (g *w17Stage2Gen) expr(d int) string {
	if d <= 0 {
		return g.atom()
	}
	e := func() string { return g.expr(d - 1) }
	key := func() string { return "[" + e() + "]" }
	switch g.n(14) {
	case 0:
		return "(" + g.v() + g.dims() + " = " + e() + ")"
	case 1:
		return "(" + g.v() + g.dims() + " " + g.pick("+=", "-=", ".=", "*=", "%=", "|=", "<<=") + " " + e() + ")"
	case 2:
		return "(" + g.v() + key() + key() + " ??= " + e() + ")"
	case 3:
		return g.pick("++", "--") + g.v() + g.dims()
	case 4:
		return g.v() + g.dims() + g.pick("++", "--")
	case 5:
		return g.v() + key()
	case 6:
		return g.v() + key() + key()
	case 7:
		return "(" + g.v() + key() + " ?? " + e() + ")"
	case 8:
		return "isset(" + g.v() + key() + key() + ")"
	case 9:
		return "f(" + g.pick("", e(), e()+", "+e(), e()+", "+e()+", "+e()) + ")"
	case 10:
		return "(" + e() + " " + g.pick(".", "+", "==", "===", "<") + " " + e() + ")"
	case 11:
		return "\"<" + g.v() + "[0]{" + g.v() + "['k']}>\""
	case 12:
		return "'abc'" + key()
	}
	return e()
}

func (g *w17Stage2Gen) stmt(d, loops int) {
	e := func() string { return g.expr(2) }
	body := func(loops int) {
		for i := g.n(3); i >= 0; i-- {
			g.stmt(d-1, loops)
		}
	}
	if d <= 0 {
		g.b.WriteString("echo " + e() + ", \"\\n\";\n")
		return
	}
	switch g.n(10) {
	case 0:
		g.b.WriteString("foreach (" + e() + " as " + g.v() + ") {\n")
		body(loops + 1)
		g.b.WriteString("}\n")
	case 1:
		g.b.WriteString("foreach (" + g.v() + " as " + g.v() + " => " + g.v() + g.pick("", "[]", "[0]", "['k'][]") + ") {\n")
		body(loops + 1)
		g.b.WriteString("}\n")
	case 2:
		g.b.WriteString("unset(" + g.v() + "[" + e() + "][" + e() + "]);\n")
	case 3:
		if loops > 0 {
			g.b.WriteString(g.pick("break", "continue") + " " + strconv.Itoa(1+g.n(loops)) + ";\n")
			return
		}
		g.b.WriteString(g.v() + " = " + e() + ";\n")
	case 4:
		g.b.WriteString("if (" + e() + ") {\n")
		body(loops)
		g.b.WriteString("}\n")
	case 5:
		g.b.WriteString(g.v() + " = " + g.pick("[]", "'string'", "[[1, 2], 'k' => 'v']", "5", "null", "false") + ";\n")
	default:
		g.b.WriteString("echo " + e() + ", \"\\n\";\n")
	}
}

func (g *w17Stage2Gen) program() string {
	g.b.Reset()
	g.b.WriteString("<?php\nfunction f($p, $q = " + g.pick("1", "'d'", "[]", "[1, 'k' => 2]", "null") + ", $r = " + g.pick("2", "'e' . 'f'", "[3][0]") + ") {\n$a = $p; $b = $q; $c = $r; $s = 'str';\n")
	for i := g.n(3); i >= 0; i-- {
		g.stmt(2, 0)
	}
	g.b.WriteString("return " + g.expr(1) + ";\n}\n")
	for i := 2 + g.n(5); i >= 0; i-- {
		g.stmt(3, 0)
	}
	return g.b.String()
}

// Random stage 2 programs compile, and each run ends as PHP would end it
// (completed, exited, an uncaught Error) or at the step budget: never as an
// engine fault, and never as a construct refused only once it ran.
func TestW17Stage2RandomProgramsEndCleanly(t *testing.T) {
	for _, width := range []int{64, 32} {
		e, err := NewEngine(Config{IntSize: width, Limits: Limits{Steps: 20000}})
		if err != nil {
			t.Fatal(err)
		}
		g := &w17Stage2Gen{state: uint64(width)}
		ends := map[string]int{}
		for i := 0; i < 1500; i++ {
			src := g.program()
			p, err := e.Compile(context.Background(), "/w/cases/random.php", src)
			if err != nil {
				t.Fatalf("width %d program %d does not compile: %v\n%s", width, i, err, src)
			}
			res, err := e.Run(context.Background(), p, WithINI("display_errors", "1"), WithINI("log_errors", "0"))
			var ie *InternalError
			var ue *UnsupportedError
			var re *ResourceError
			var pe *PHPError
			switch {
			case errors.As(err, &ie), errors.As(err, &ue):
				t.Fatalf("width %d program %d: %v\n%s", width, i, err, src)
			case errors.As(err, &re):
				if re.Resource != ResourceSteps {
					t.Fatalf("width %d program %d: %v\n%s", width, i, err, src)
				}
				ends["steps"]++
			case errors.As(err, &pe):
				ends["php error"]++
			case err != nil:
				t.Fatalf("width %d program %d: unexpected %T: %v\n%s", width, i, err, err, src)
			default:
				ends["completed"]++
			}
			if res == nil {
				t.Fatalf("width %d program %d: no result", width, i)
			}
		}
		if ends["completed"] < 100 || ends["php error"] < 100 {
			t.Errorf("width %d: runs ended as %v; the generator no longer covers both", width, ends)
		}
		t.Logf("width %d: %v", width, ends)
	}
}

// RECV_INIT's cache of evaluated defaults belongs to one run: a Program
// shared by two runs, sequential or concurrent, warns in each run's first
// call that omits the argument and in no later call of that run.
func TestW17Stage2DefaultCacheIsPerRun(t *testing.T) {
	e, err := NewEngine(Config{IntSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), "/w/cases/s2_cache.php", "<?php\nfunction f($a = [1][5]) { return $a === null ? \"n\" : \"?\"; }\necho f(), f(), f();\n")
	if err != nil {
		t.Fatal(err)
	}
	const want = "\nWarning: Undefined array key 5 in /w/cases/s2_cache.php on line 2\nnnn"
	done := make(chan string, 4)
	for i := 0; i < 4; i++ {
		go func() {
			res, err := e.Run(context.Background(), p, WithINI("display_errors", "1"), WithINI("log_errors", "0"))
			if err != nil {
				done <- err.Error()
				return
			}
			done <- string(res.Output)
		}()
	}
	for i := 0; i < 4; i++ {
		if got := <-done; got != want {
			t.Fatalf("run output %q, want %q", got, want)
		}
	}
}

// TestW17Stage2OracleDump writes programs and Phego's display-mode output
// for a differential run against the pinned PHP image; it does nothing
// unless PHEGO_DUMP names a directory with cases/ and phego/ in it. By
// default it writes 600 programs of w17Stage2Gen (seed PHEGO_DUMP_SEED,
// width PHEGO_DUMP_WIDTH); with PHEGO_DUMP_FILES set it runs the programs
// already in cases/ instead. PHP's output for each case is then compared
// byte for byte with phego/<case>.out and .code (the stage 2 evidence in
// engineering/implementation/evidence/W-17-vm/stage2/random-differential.txt).
func TestW17Stage2OracleDump(t *testing.T) {
	dir := os.Getenv("PHEGO_DUMP")
	if dir == "" {
		t.Skip()
	}
	width, _ := strconv.Atoi(os.Getenv("PHEGO_DUMP_WIDTH"))
	if width == 0 {
		width = 64
	}
	seed, _ := strconv.ParseUint(os.Getenv("PHEGO_DUMP_SEED"), 10, 64)
	if seed == 0 {
		seed = 12345
	}
	e, err := NewEngine(Config{IntSize: width, Limits: Limits{Steps: 2000000}})
	if err != nil {
		t.Fatal(err)
	}
	g := &w17Stage2Gen{state: seed}
	files, err := filepath.Glob(filepath.Join(dir, "cases", "*.php"))
	if err != nil {
		t.Fatal(err)
	}
	fromFiles := os.Getenv("PHEGO_DUMP_FILES") != ""
	count := 600
	if fromFiles {
		count = len(files)
	}
	for i := 0; i < count; i++ {
		var src, name string
		if fromFiles {
			b, err := os.ReadFile(files[i])
			if err != nil {
				t.Fatal(err)
			}
			src, name = string(b), strings.TrimSuffix(filepath.Base(files[i]), ".php")
		} else {
			src, name = g.program(), fmt.Sprintf("r%04d", i)
		}
		got := w17Execute(t, e, name, src, WithINI("display_errors", "1"), WithINI("log_errors", "0"), WithINI("html_errors", "0"), WithINI("error_reporting", "-1"))
		if !fromFiles {
			if err := os.WriteFile(filepath.Join(dir, "cases", name+".php"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		out := string(got.stdout)
		if got.err != nil && got.compileFatal == nil && got.exitCode != 255 {
			out += fmt.Sprintf("\n<<phego error %v>>", got.err)
		}
		if err := os.WriteFile(filepath.Join(dir, "phego", name+".out"), []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "phego", name+".code"), []byte(fmt.Sprintf("%d\n", got.exitCode)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
