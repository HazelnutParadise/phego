<?php
declare(strict_types=1);
// closure_strict_types: exact types pass, int widens to float, returns are exact
$add = function (int $a, int $b): int {
    return $a + $b;
};
echo "int_exact: ", $add(2, 3), "\n";

$half = function (float $x): float {
    return $x / 2;
};
echo "int_widened_to_float: ", $half(5) === 2.5 ? "2.5" : "other", "\n";
echo "float_exact: ", $half(1.0) === 0.5 ? "0.5" : "other", "\n";

$name = function (string $s): string {
    return "[" . $s . "]";
};
echo "string_exact: ", $name("abc"), "\n";

$flag = function (bool $b): string {
    return $b ? "yes" : "no";
};
echo "bool_exact: ", $flag(false), "\n";

$maybe = function (?int $n): ?int {
    return $n;
};
echo "nullable_null: ", $maybe(null) === null ? "null" : "value", "\n";
echo "nullable_int: ", $maybe(4), "\n";

$union = function (int|string $v): int|string {
    return $v;
};
echo "union_int: ", $union(1) === 1 ? "int" : "other", "\n";
echo "union_string: ", $union("1") === "1" ? "string" : "other", "\n";

$obj = function (stdClass $o): stdClass {
    return $o;
};
$s = new stdClass();
$s->v = "vv";
echo "object_type: ", $obj($s)->v, "\n";

$void = function (): void {
    echo "void ran\n";
};
$void();
$ret = function (): int {
    return 7;
};
echo "return_int: ", $ret(), "\n";

$floatRet = function (int $n): float {
    return $n;
};
echo "int_return_as_float: ", $floatRet(3) === 3.0 ? "float3" : "other", "\n";
