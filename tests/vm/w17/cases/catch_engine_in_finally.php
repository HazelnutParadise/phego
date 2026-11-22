<?php
// catch_engine_in_finally: engine errors passing through finally blocks, and an engine error raised inside finally
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

function divide_guarded($d)
{
    try {
        return 10 / $d;
    } finally {
        echo "finally_divide\n";
    }
}

try {
    $r = divide_guarded(0);
} catch (DivisionByZeroError $e) {
    report("through_finally", $e);
}

function replace_pending()
{
    $arr = [1];
    try {
        throw new LogicException("first");
    } finally {
        $bad = $arr + 1;
        echo "not reached\n";
    }
}

try {
    replace_pending();
} catch (TypeError $e) {
    report("replaced_pending", $e);
    echo "replaced_previous: ", $e->getPrevious()::class, " | ", $e->getPrevious()->getMessage(), "\n";
}

function ok_finally()
{
    try {
        return "value";
    } finally {
        echo "finally_ok\n";
    }
}
echo "returned: ", ok_finally(), "\n";
echo "end: ok\n";
