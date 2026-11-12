<?php
$x = 1 +
    $undef1 +
    2;
echo $x, "\n";
$y = "a"
    . $undef2
    . "b";
echo $y, "\n";
$arr = [
    "k" => 1,
];
echo
    $arr[
        "missing"
    ],
    "|\n";
function f($a, $b)
{
    return $a . $b;
}
echo f(
    $undef3,
    "z"
), "\n";
$s = "";
$s
    ++;
$n = null;
--
$n;
$t = true;
$t++; $t--;
$r = 10
    /
    0;
