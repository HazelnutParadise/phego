<?php
namespace A;
function f()
{
    return PHP_OS;
}
function g($d = E_ALL)
{
    return $d;
}
echo f(), " ", g(), "\n";
const PHP_OS = "mine", E_ALL = 7;
echo f(), " ", g(), " ", PHP_OS, " ", E_ALL, "\n";
for ($i = 0; $i < 2; $i++) {
    echo PHP_OS, DIRECTORY_SEPARATOR, E_WARNING;
}
echo "\n";
const DIRECTORY_SEPARATOR = "|";
echo DIRECTORY_SEPARATOR, "\n";
function h()
{
    return E_STRICT;
}
echo h(), "\n";
const E_STRICT = 1;
echo h(), " ", E_STRICT, "\n";
