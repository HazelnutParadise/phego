<?php
$x = 1;
$inc = fn (&$v) => $v++;
$inc($x);
echo "by_ref_after_first: ", $x, "\n";
$inc($x);
echo "by_ref_after_second: ", $x, "\n";

$withDefault = fn ($a, $b = 10, $c = [1, 2]) => $a + $b + $c[1];
echo "defaults_all: ", $withDefault(1), "\n";
echo "defaults_override_b: ", $withDefault(1, 2), "\n";
echo "defaults_override_c: ", $withDefault(1, 2, [0, 5]), "\n";

const BASE_VALUE = 100;
$constDefault = fn ($n = BASE_VALUE * 2) => $n + 1;
echo "const_expr_default: ", $constDefault(), "\n";

$variadic = fn ($first, ...$rest) => $first . "|" . ($rest[0] ?? "-") . "|" . ($rest[1] ?? "-");
echo "variadic_none: ", $variadic("a"), "\n";
echo "variadic_two: ", $variadic("a", "b", "c"), "\n";

$typed = fn (int $n): string => $n * 2;
echo "int_coerced_from_string: ", $typed("21"), "\n";
echo "return_is_string_42: ", $typed("21") === "42" ? "yes" : "no", "\n";

$half = fn (float $f): float => $f / 2;
echo "float_from_int: ", $half(3), "\n";

$named = fn ($a, $b = "B", $c = "C") => $a . $b . $c;
echo "named_out_of_order: ", $named(c: "z", a: "x"), "\n";
echo "named_skip_middle: ", $named("x", c: "z"), "\n";

$assoc = ["b" => "2", "a" => "1"];
echo "unpack_string_keys: ", $named(...$assoc), "\n";
$pos = ["p", "q"];
echo "unpack_positional: ", $named(...$pos), "\n";
echo "unpack_string_keys_partial: ", $named(...["c" => "C2", "a" => "A2"]), "\n";
