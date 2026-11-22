<?php
// catch_engine_type_error_argument: TypeError from wrong argument types for typed user parameters
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Box {}
class Crate {}

function takes_int(int $n)
{
    return $n + 1;
}

function takes_string(string $s)
{
    return $s . "!";
}

function takes_box(Box $b)
{
    return "box";
}

function takes_nullable(?int $n)
{
    return "nullable";
}

function takes_union(int|string $v)
{
    return "union";
}

try {
    $r = takes_int("abc");
} catch (TypeError $e) {
    report("int_from_word", $e);
}

try {
    $r = takes_int([1]);
} catch (TypeError $e) {
    report("int_from_array", $e);
}

try {
    $r = takes_string([]);
} catch (TypeError $e) {
    report("string_from_array", $e);
}

try {
    $r = takes_box(new Crate());
} catch (TypeError $e) {
    report("class_wrong", $e);
}

try {
    $r = takes_box(null);
} catch (TypeError $e) {
    report("class_null", $e);
}

try {
    $r = takes_nullable("x");
} catch (TypeError $e) {
    report("nullable_word", $e);
}

$r = takes_nullable(null);
echo "nullable_null: ", $r, "\n";

try {
    $r = takes_union([]);
} catch (TypeError $e) {
    report("union_array", $e);
}

$r = takes_union(2.0);
echo "union_float: ", $r, "\n";

echo "end: ok\n";
