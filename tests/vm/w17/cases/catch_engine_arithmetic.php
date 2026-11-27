<?php
// catch_engine_arithmetic: ArithmeticError for negative shift counts; shifts past the width and PHP_INT_MIN % -1 give results
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

$neg = -1;
$two = 2;

try {
    $r = 1 << -1;
} catch (ArithmeticError $e) {
    report("shl_const", $e);
}

try {
    $r = 1 >> $neg;
} catch (ArithmeticError $e) {
    report("shr_variable", $e);
}

try {
    $r = 8 << $neg;
} catch (ArithmeticError $e) {
    report("shl_variable", $e);
}

echo "shl_64: ", 1 << 64, "\n";
echo "shr_neg_64: ", -1 >> 64, "\n";
echo "shl_by_two: ", 3 << $two, "\n";
echo "int_min_mod_minus_one: ", PHP_INT_MIN % -1, "\n";
echo "int_max_plus_one_type: ", (PHP_INT_MAX + 1) > 0 ? "positive" : "other", "\n";

$m = 17;
$d = 5;
echo "mod_positive: ", $m % $d, "\n";
echo "mod_negative_divisor: ", $m % -$d, "\n";

try {
    $r = $m % 0;
} catch (DivisionByZeroError $e) {
    report("mod_zero_is_not_arithmetic", $e);
}
echo "end: ok\n";
