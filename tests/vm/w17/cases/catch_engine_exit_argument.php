<?php
// catch_engine_exit_argument: exit with an array or object argument raises a catchable TypeError; the script then continues
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

echo "before\n";

try {
    exit([]);
} catch (TypeError $e) {
    report("exit_array", $e);
}

try {
    exit(new stdClass());
} catch (TypeError $e) {
    report("exit_object", $e);
}

echo "still_running\n";
echo "end: ok\n";
