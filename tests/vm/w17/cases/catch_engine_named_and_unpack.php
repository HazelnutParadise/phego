<?php
// catch_engine_named_and_unpack: Errors for unknown named parameters, overwritten arguments and non-array unpacking
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

function named_two($a, $b = 2)
{
    return $a . "/" . $b;
}

try {
    $r = named_two(c: 1);
} catch (Error $e) {
    report("unknown_named", $e);
}

try {
    $r = named_two(1, a: 2);
} catch (Error $e) {
    report("overwrites_positional", $e);
}

try {
    $r = named_two(...['zz' => 1]);
} catch (Error $e) {
    report("unknown_named_unpack", $e);
}

try {
    $five = 5;
    $r = named_two(...$five);
} catch (Error $e) {
    report("unpack_scalar", $e);
}

$r = named_two(...['b' => 3, 'a' => 1]);
echo "named_unpack_ok: ", $r, "\n";
$r = named_two(b: 9, a: 8);
echo "named_order_ok: ", $r, "\n";
$r = named_two(...[7]);
echo "positional_unpack_ok: ", $r, "\n";
echo "end: ok\n";
