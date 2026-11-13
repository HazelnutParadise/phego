<?php
function f($a, &$b, $c = 3, &$d = 4)
{
    $b = $a + $c;
    $d = $d * 2;
    return $d;
}
$x = 0;
echo f(1, $x), " ", $x, "\n";
$y = 5;
echo f(1, $x, 10, $y), " ", $x, " ", $y, "\n";
function swap(&$p, &$q)
{
    $t = $p;
    $p = $q;
    $q = $t;
}
$i = "i";
$j = "j";
swap($i, $j);
echo $i, $j, "\n";
$arr = [1, 2];
swap($arr[0], $arr[1]);
echo $arr[0], $arr[1], "\n";
swap($i, $i);
echo $i, "\n";
