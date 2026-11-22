<?php
// catch_engine_division_by_zero: DivisionByZeroError from /, %, /= and %= is catchable
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

function divide($a, $b)
{
    return $a / $b;
}

try {
    $x = 1 / 0;
} catch (DivisionByZeroError $e) {
    report("slash_const", $e);
}

try {
    $x = 1 % 0;
} catch (DivisionByZeroError $e) {
    report("mod_const", $e);
}

$a = 10;
try {
    $a /= 0;
} catch (DivisionByZeroError $e) {
    report("slash_assign", $e);
}
echo "a_after_slash: ", $a, "\n";

$b = 10;
try {
    $b %= 0;
} catch (DivisionByZeroError $e) {
    report("mod_assign", $e);
}
echo "b_after_mod: ", $b, "\n";

try {
    $f = 1.0 / 0;
} catch (DivisionByZeroError $e) {
    report("float_slash", $e);
}

try {
    $r = divide(7, 0);
} catch (DivisionByZeroError $e) {
    report("inside_function", $e);
}

$zero = 0;
try {
    $y = 5 / $zero;
} catch (DivisionByZeroError $e) {
    report("variable_zero", $e);
}

$ok = divide(9, 3);
echo "divide_ok: ", $ok, "\n";
echo "end: ok\n";
