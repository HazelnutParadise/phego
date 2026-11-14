<?php
$y = ([$a, $b] = [7, 8]);
echo $y[0], $y[1], $a, $b, "\n";
$z = list($c) = [9];
echo $z[0], $c, "\n";
if ([$d] = [0]) {
    echo "array is truthy\n";
}
if (([$e] = []) === []) {
    echo "empty array\n";
}
function f($x) { return $x; }
echo f([$g] = [3])[0], $g, "\n";
function g() { return [$h] = [4]; }
echo g()[0], "\n";
$w = ([$i] = null);
echo $w === null ? "null" : "?", $i === null ? "null" : "?", "\n";
$v = ([$j] = 5);
echo $v, "\n";
$s = [1, 2];
$t = ([$k, $l] = $s);
$t[] = 3;
echo $s[0], $s[1], isset($s[2]) ? "?" : "-", $t[2], "\n";
$m = ([$n[0]] = ['e']);
echo $m[0], $n[0], "\n";
