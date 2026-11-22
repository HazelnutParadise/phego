<?php
// catch_engine_static_and_const: Errors for undeclared static properties, undefined and private class constants, and unknown classes
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class SC
{
    public static $a = 1;
    const K = 2;
    private const P = 3;
}

try {
    $r = SC::$missing;
} catch (Error $e) {
    report("undeclared_static", $e);
}

try {
    $r = SC::MISSING;
} catch (Error $e) {
    report("undefined_constant", $e);
}

try {
    $r = SC::P;
} catch (Error $e) {
    report("private_constant", $e);
}

try {
    $r = Nope::$x;
} catch (Error $e) {
    report("static_prop_unknown_class", $e);
}

try {
    $r = Nope::K;
} catch (Error $e) {
    report("constant_unknown_class", $e);
}

echo "public_static: ", SC::$a, "\n";
echo "public_const: ", SC::K, "\n";
SC::$a = 5;
echo "after_write: ", SC::$a, "\n";
echo "end: ok\n";
