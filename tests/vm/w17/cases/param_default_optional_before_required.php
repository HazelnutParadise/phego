<?php
echo "start\n";
function f($a = 1, $b)
{
    return $a . $b;
}
function g($a = null, $b)
{
    return ($a === null ? "null" : $a) . $b;
}
function h($a = 1, $b = 2, $c)
{
    return $a . $b . $c;
}
function ok($a, $b = 2, $c = 3)
{
    return $a . $b . $c;
}
function mixed($a = 1, $b, $c = 3, $d)
{
    return $a . $b . $c . $d;
}
echo f(1, 2), "\n";
echo g(null, 2), "\n";
echo h(1, 2, 3), "\n";
echo ok(1), "\n";
echo mixed(1, 2, 3, 4), "\n";
