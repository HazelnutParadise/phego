<?php
function noisy($v)
{
    echo "arg", $v, " ";
    return $v;
}
function f($a = "d1", $b = "d2")
{
    echo "body ";
    return $a . $b;
}
echo f(noisy(1)), "\n";
echo f(noisy(1), noisy(2)), "\n";
echo f(), "\n";
echo f(noisy("x") . noisy("y")), "\n";
function g($a, $b = "dd")
{
    return $a . $b;
}
echo g(noisy(1)) . g(noisy(2), noisy(3)), "\n";
function outer($a = 1, $b = 2)
{
    return inner($a, $b) . inner($b);
}
function inner($x, $y = "iy")
{
    return $x . $y;
}
echo outer(), outer(5), "\n";
