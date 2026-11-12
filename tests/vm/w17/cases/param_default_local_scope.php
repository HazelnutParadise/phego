<?php
$a = "global a";
function f($a = "default a")
{
    $b = $a . "!";
    return $b;
}
echo f(), "\n";
echo f("given"), "\n";
echo $a, "\n";
function g($x = 1, $y = 2)
{
    $z = $x + $y;
    $x = "changed";
    return $z . $x;
}
echo g(), g(), g(5), "\n";
echo isset($z) ? "leak" : "no leak", "\n";
function unused($p = "p")
{
    return isset($p) ? "p set" : "p unset";
}
echo unused(), unused(null), "\n";
function unsets($p = "p")
{
    unset($p);
    return isset($p) ? "p set" : "p unset";
}
echo unsets(), "\n";
