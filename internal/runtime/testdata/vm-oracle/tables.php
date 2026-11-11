<?php
// PHP 8.5.12 observations for the W-17 VM tests (internal/runtime): operations
// the operator reference (testdata/php8512-ops-*.json) does not cover. Each
// row records the operands, the result, a thrown error and the diagnostics
// (level, message) in raising order. See README.md to regenerate.

$diags = [];
set_error_handler(function ($no, $str) {
    global $diags;
    $diags[] = [$no, $str];
    return true;
});

function enc($v)
{
    if ($v === null) return "N";
    if ($v === true) return "b:1";
    if ($v === false) return "b:0";
    if (is_int($v)) return "i:" . $v;
    if (is_float($v)) return "d:" . bin2hex(pack("E", $v));
    if (is_string($v)) return "s:" . bin2hex($v);
    $pairs = [];
    foreach ($v as $k => $x) {
        $pairs[] = [enc($k), enc($x)];
    }
    return "a:" . json_encode($pairs);
}

function run(callable $f)
{
    global $diags;
    $diags = [];
    try {
        $r = $f();
        $row = ["r" => is_array($r) && array_key_exists("__pair", $r) ? [enc($r[0]), enc($r[1])] : [enc($r)]];
    } catch (Throwable $e) {
        $row = ["e" => [get_class($e), $e->getMessage()]];
    }
    $row["d"] = $diags;
    return $row;
}

function pair($a, $b) { return ["__pair" => 1, 0 => $a, 1 => $b]; }

$values = [null, false, true, 0, 1, -1, 1.5, "1", "1.5", "abc", "", [], [1]];
$incdec = array_merge($values, [
    PHP_INT_MAX, PHP_INT_MIN, -0.0, 1e300, "a", "z", "Az", "zz", "Zz", "a9", "A99", "5", "5.5", "1e2", " 5", "5 ", "-3", "-",
    "a-", "z ", "9223372036854775807", "-9223372036854775808", "2147483647", "-2147483648", "0x1A", "007", ".5", "\0",
]);
$keys = [null, false, true, 0, 1, -1, 1.5, -0.5, 1e20, "1", "1.5", "abc", "", "08", "-0", "0x1", "-1", " 1", "1 ",
    "9223372036854775807", "9223372036854775808", "-9223372036854775808", "2147483648", "-2147483649", PHP_INT_MAX, [], [1]];
$read = [0 => "zero", "" => "empty", 1 => "one", "abc" => "s", -1 => "minus", "1.5" => "str"];

$out = ["php" => PHP_VERSION, "int_size" => PHP_INT_SIZE, "values" => array_map("enc", $values),
    "incdec_values" => array_map("enc", $incdec), "keys" => array_map("enc", $keys), "read_array" => enc($read)];

foreach ($values as $a) {
    foreach ($values as $b) {
        $out["xor"][] = run(fn() => $a xor $b);
    }
    $out["cast_array"][] = run(fn() => (array)$a);
}
foreach ($incdec as $v) {
    $out["pre_inc"][] = run(function () use ($v) { $x = $v; $r = ++$x; return pair($r, $x); });
    $out["pre_dec"][] = run(function () use ($v) { $x = $v; $r = --$x; return pair($r, $x); });
    $out["post_inc"][] = run(function () use ($v) { $x = $v; $r = $x++; return pair($r, $x); });
    $out["post_dec"][] = run(function () use ($v) { $x = $v; $r = $x--; return pair($r, $x); });
}
$out["undefined_inc"] = [
    run(function () { $r = ++$u; return pair($r, $u); }),
    run(function () { $r = --$u; return pair($r, $u); }),
    run(function () { $r = $u++; return pair($r, $u); }),
    run(function () { $r = $u--; return pair($r, $u); }),
];
foreach ($keys as $k) {
    $out["key_write"][] = run(function () use ($k) { $a = []; $a[$k] = "v"; return $a; });
    $out["key_read"][] = run(function () use ($k, $read) { $b = $read; return $b[$k]; });
    $out["key_read_quiet"][] = run(function () use ($k, $read) { $b = $read; return $b[$k] ?? "dflt"; });
    $out["key_isset"][] = run(function () use ($k, $read) { $b = $read; return isset($b[$k]); });
    $out["key_unset"][] = run(function () use ($k, $read) { $b = $read; unset($b[$k]); return $b; });
    $out["string_isset"][] = run(function () use ($k) { $s = "abc"; return isset($s[$k]); });
}
foreach ($values as $c) {
    $out["container_read"][] = run(function () use ($c) { $x = $c; return $x["k"]; });
    $out["container_read_quiet"][] = run(function () use ($c) { $x = $c; return $x["k"] ?? "dflt"; });
    $out["container_read_array_key"][] = run(function () use ($c) { $x = $c; return $x[[]]; });
    $out["container_set"][] = run(function () use ($c) { $x = $c; $x["k"] = "v"; return $x; });
    $out["container_append"][] = run(function () use ($c) { $x = $c; $x[] = "v"; return $x; });
    $out["container_unset"][] = run(function () use ($c) { $x = $c; unset($x["k"]); return $x; });
    $out["container_isset"][] = run(function () use ($c) { $x = $c; return isset($x["k"]); });
    $out["isset_var"][] = run(function () use ($c) { $x = $c; return isset($x); });
}
$out["undefined_container"] = [
    run(function () { return $u["k"]; }),
    run(function () { return $u["k"] ?? "dflt"; }),
    run(function () { $u["k"] = "v"; return $u; }),
    run(function () { $u[] = "v"; return $u; }),
    run(function () { unset($u["k"]); return 1; }),
    run(function () { return isset($u["k"]); }),
];
echo json_encode($out, JSON_PRETTY_PRINT), "\n";
