<?php
namespace A;
function g($d = PHP_OS . "x")
{
    return $d;
}
function h($d = E_ALL + 0)
{
    return $d;
}
function k($d = [E_ALL])
{
    return $d[0];
}
function m($d = PHP_OS)
{
    return $d;
}
function f($d = PHP_EOL . "y")
{
    return $d;
}
const X = PHP_INT_SIZE * 2;
echo g(), " ", h(), " ", k(), " ", m(), " ", X, f();
const PHP_OS = "mine", E_ALL = 7, PHP_EOL = "!";
echo g(), " ", h(), " ", k(), " ", m(), " ", X, f(), "\n";
