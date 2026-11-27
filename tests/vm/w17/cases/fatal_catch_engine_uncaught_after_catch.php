<?php
// fatal_catch_engine_uncaught_after_catch: engine errors are caught, then a different engine error is uncaught
try {
    $zero = 0;
    $r = 1 % $zero;
} catch (DivisionByZeroError $e) {
    echo "caught: ", $e->getMessage(), " line ", $e->getLine(), "\n";
}
try {
    $r = no_such_function();
} catch (Error $e) {
    echo "caught: ", $e::class, " | ", $e->getMessage(), "\n";
}
echo "third step\n";
$arr = [1];
$total = $arr + 1;
echo "not reached\n";
