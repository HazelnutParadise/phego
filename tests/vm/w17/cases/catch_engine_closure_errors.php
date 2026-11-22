<?php
// catch_engine_closure_errors: Errors for writing properties on a Closure, instantiating Closure, and invalid fromCallable targets
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

function shout($s)
{
    return $s . "!";
}

$c = function ($x) {
    return $x * 2;
};

try {
    $c->extra = 1;
} catch (Error $e) {
    report("write_property", $e);
}

try {
    $r = new Closure();
} catch (Error $e) {
    report("instantiate", $e);
}

try {
    $r = Closure::fromCallable("no_such_function");
} catch (TypeError $e) {
    report("from_callable_missing", $e);
}

try {
    $r = Closure::fromCallable([new stdClass(), "nope"]);
} catch (TypeError $e) {
    report("from_callable_method", $e);
}

echo "call_ok: ", $c(4), "\n";
$fromName = Closure::fromCallable("shout");
echo "from_callable_ok: ", $fromName("abc"), "\n";
echo "closure_class: ", $c::class, "\n";
echo "end: ok\n";
