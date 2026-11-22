<?php
// catch_engine_unsupported_operands: TypeError for arithmetic on arrays, objects and non-numeric strings
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

$arr = [1, 2];
$str = "x";
$obj = new stdClass();
$num = 1;
$bad = "abc";

try {
    $r = $arr + $num;
} catch (TypeError $e) {
    report("array_plus_int", $e);
}

try {
    $r = $arr - $arr;
} catch (TypeError $e) {
    report("array_minus_array", $e);
}

try {
    $r = $str * $arr;
} catch (TypeError $e) {
    report("string_times_array", $e);
}

try {
    $r = $obj + $num;
} catch (TypeError $e) {
    report("object_plus_int", $e);
}

try {
    $r = $arr % 2;
} catch (TypeError $e) {
    report("array_mod", $e);
}

try {
    $r = -$arr;
} catch (TypeError $e) {
    report("unary_minus_array", $e);
}

try {
    $r = $bad * 2;
} catch (TypeError $e) {
    report("non_numeric_string", $e);
}

$ok = "7" * 2;
echo "numeric_string_ok: ", $ok, "\n";
$sum = [1] + [2, 3];
echo "array_union_ok: ", $sum[1], "\n";
echo "end: ok\n";
