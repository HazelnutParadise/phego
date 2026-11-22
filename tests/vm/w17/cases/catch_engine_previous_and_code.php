<?php
// catch_engine_previous_and_code: engine errors have code 0 and no previous; wrapping one in a user exception links it as previous
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class WrapError extends RuntimeException {}

try {
    $zero = 0;
    $r = 1 % $zero;
} catch (Error $e) {
    report("engine", $e);
    echo "engine_code: ", $e->getCode(), "\n";
    echo "engine_previous: ", $e->getPrevious() === null ? "none" : "present", "\n";
    try {
        throw new WrapError("wrapped", 9, $e);
    } catch (WrapError $w) {
        report("wrapper", $w);
        echo "wrapper_code: ", $w->getCode(), "\n";
        echo "wrapper_previous: ", $w->getPrevious()::class, "\n";
        echo "wrapper_previous_message: ", $w->getPrevious()->getMessage(), "\n";
        echo "wrapper_string: ", $w, "\n";
    }
}
echo "end: ok\n";
