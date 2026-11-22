<?php
// catch_engine_argument_count: ArgumentCountError for too few arguments to functions, methods, constructors and closures
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

function two($a, $b)
{
    return $a . $b;
}

function opt_one($a, $b = 2)
{
    return $a + $b;
}

class Am
{
    public $p;

    public function __construct($p)
    {
        $this->p = $p;
    }

    public function m($x, $y, $z)
    {
        return $x;
    }

    public static function st($v)
    {
        return $v;
    }
}

$f = function ($q) {
    return $q;
};

try {
    $r = two(1);
} catch (ArgumentCountError $e) {
    report("function", $e);
}

try {
    $r = opt_one();
} catch (ArgumentCountError $e) {
    report("optional_default", $e);
}

try {
    $r = (new Am(1))->m(1);
} catch (ArgumentCountError $e) {
    report("method", $e);
}

try {
    $r = new Am();
} catch (ArgumentCountError $e) {
    report("constructor", $e);
}

try {
    $r = Am::st();
} catch (ArgumentCountError $e) {
    report("static_method", $e);
}

try {
    $r = $f();
} catch (ArgumentCountError $e) {
    report("closure", $e);
}

echo "enough: ", two("a", "b"), "\n";
$r = two("a", "b", "extra");
echo "extra_allowed: ", $r, "\n";
echo "is_error_type: ", (new ArgumentCountError("x")) instanceof TypeError ? "yes" : "no", "\n";
echo "end: ok\n";
