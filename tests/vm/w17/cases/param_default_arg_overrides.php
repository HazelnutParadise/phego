<?php
function f($a = "da", $b = "db")
{
    return "[" . $a . "][" . $b . "]";
}
echo f(), f("x"), f("x", "y"), f("x", "y", "extra"), "\n";
echo f(0, 0), f("", ""), f(false, false), f(null, null), "\n";
echo f(1.5), f(true), "\n";
echo f($u1), f($u2, $u3), "\n";
function g($a = 1)
{
    $a++;
    return $a;
}
echo g(), g(), g(10), g(), "\n";
function h($a = [1])
{
    $a[] = 2;
    return $a[1];
}
echo h(), h([9, 9, 9]), h(), "\n";
function swap($x = 1, $y = 2)
{
    $t = $x;
    $x = $y;
    $y = $t;
    return $x . $y;
}
echo swap(), swap(3), swap(3, 4), "\n";
