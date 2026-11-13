<?php
function f()
{
    $a = [1, 2];
    $r = &$a[0];
    $r = 9;
    return $a[0] + $r;
}
echo f(), "\n";
echo f(), "\n";
function g($x)
{
    $y = &$x;
    $y += 1;
    return $x;
}
$v = 1;
echo g($v), " ", $v, "\n";
