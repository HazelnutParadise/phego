<?php
const A = PHP_INT_MAX + 1;
const B = PHP_EOL . "x";
const C = M_PI * 2;
const D = E_ALL & ~E_NOTICE;
const F = PHP_INT_SIZE << 3;
const G = -PHP_INT_MAX - 1;
echo A, "|", B, "|", C, "|", D, "|", F, "|", G, "\n";
function f($a = PHP_INT_MAX + 1, $b = M_PI / 2, $c = E_ALL ^ E_WARNING, $d = PHP_INT_MIN, $e = PHP_FLOAT_EPSILON * 2)
{
    echo $a, "|", $b, "|", $c, "|", $d, "|", $e, "\n";
}
f();
f(1, 2, 3, 4, 5);
const H = (int) "12abc" + (float) "1.5x", I = (string) PHP_INT_SIZE . (bool) 1, J = (array) PHP_INT_SIZE;
echo H, "|", I, "|", J[0], "\n";
