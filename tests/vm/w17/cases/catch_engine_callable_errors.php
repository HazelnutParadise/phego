<?php
// catch_engine_callable_errors: Errors for calling a missing function or method, a non-callable value, or an object without __invoke
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Obj
{
    public function real()
    {
        return "real";
    }
}

$name = "no_such_fn";
try {
    $r = $name();
} catch (Error $e) {
    report("string_function", $e);
}

$method = "Obj::nope";
try {
    $r = $method();
} catch (Error $e) {
    report("string_static_method", $e);
}

$arr = [new Obj(), "nope"];
try {
    $r = $arr();
} catch (Error $e) {
    report("array_unknown_method", $e);
}

$int = 5;
try {
    $r = $int();
} catch (Error $e) {
    report("int_value", $e);
}

$plain = new stdClass();
try {
    $r = $plain();
} catch (Error $e) {
    report("object_without_invoke", $e);
}

$good = [new Obj(), "real"];
echo "array_callable_ok: ", $good(), "\n";
echo "string_callable_ok: ", (function () { return "closure"; })(), "\n";
echo "end: ok\n";
