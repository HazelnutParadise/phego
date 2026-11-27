<?php
// catch_engine_undefined_symbols: Error for undefined functions, methods, classes, constants and static properties
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Obj
{
    const REAL = 1;
    public static $real_static = 2;

    public function real()
    {
        return 3;
    }

    public static function sreal()
    {
        return 4;
    }
}

$o = new Obj();

try {
    $r = no_such_function(1);
} catch (Error $e) {
    report("function", $e);
}

try {
    $r = $o->nope();
} catch (Error $e) {
    report("method", $e);
}

try {
    $r = Obj::nope();
} catch (Error $e) {
    report("static_method", $e);
}

try {
    $r = new Missing();
} catch (Error $e) {
    report("new_class", $e);
}

try {
    $r = Missing::call();
} catch (Error $e) {
    report("static_call_class", $e);
}

try {
    $r = Obj::NOPE;
} catch (Error $e) {
    report("class_constant", $e);
}

try {
    $r = NOPE_CONSTANT;
} catch (Error $e) {
    report("global_constant", $e);
}

try {
    $r = Obj::$nope;
} catch (Error $e) {
    report("static_property", $e);
}

echo "real_const: ", Obj::REAL, "\n";
echo "real_static: ", Obj::$real_static, "\n";
echo "real_method: ", $o->real(), "\n";
echo "real_static_method: ", Obj::sreal(), "\n";
echo "end: ok\n";
