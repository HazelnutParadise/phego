<?php
function k($n) { echo "k$n "; return $n; }
function f(&$a, $b, &$c)
{
    echo "[", $a, $b, $c, "] ";
    $a = "A";
    $c = "C";
}
$v = 1;
f($v, $v = 2, $v);
echo $v, "\n";
$arr = [];
f($arr[k(1)], k(2), $arr[k(3)]);
echo $arr[1], $arr[3], "\n";
$w = 1;
f($w, $w++, $w);
echo $w, "\n";
