<?php
function nullable(?int $x)
{
    return $x === null ? "null" : "int " . $x;
}
function union(int|string|null $x)
{
    return $x === null ? "null" : (($x === (int)$x ? "int " : "string ") . $x);
}
function mixed_param(mixed $x)
{
    return $x === null ? "null" : "set";
}
function default_null(int $x = null)
{
    return $x === null ? "null default" : "int " . $x;
}
function arr_or_bool(array|bool $x)
{
    return $x === true ? "true" : ($x === false ? "false" : "array");
}
function int_float(int|float $x)
{
    return ($x === (int)$x ? "int " : "float ") . $x;
}

echo nullable(null), " | ", nullable(3), " | ", nullable("4"), "\n";
echo union(null), " | ", union(5), " | ", union("5"), " | ", union("abc"), " | ", union(2.0), "\n";
echo union(true), " | ", union(1.5), "\n";
echo mixed_param(null), " ", mixed_param([]), "\n";
echo default_null(), " | ", default_null(null), " | ", default_null(9), "\n";
echo arr_or_bool([]), " ", arr_or_bool(false), " ", arr_or_bool(1), " ", arr_or_bool("0"), "\n";
echo int_float("5"), " | ", int_float("5.5"), " | ", int_float("1e2"), " | ", int_float(true), "\n";

function ret_nullable($v): ?string
{
    return $v;
}
echo "[", ret_nullable(null), "] [", ret_nullable(5), "]\n";
function ret_union($v): int|false
{
    return $v;
}
echo (ret_union(false) === false ? "false" : "other"), " ", ret_union("12"), "\n";
