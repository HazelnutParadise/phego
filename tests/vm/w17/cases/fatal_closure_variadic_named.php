<?php
$f = function (int ...$n) { $s = 0; foreach ($n as $v) { $s += $v; } return $s; };
echo $f(1, 2, 3), $f(...[4, 5]), "\n";
$g = fn(int $a, int $b = 2): int => $a * $b;
echo $g(b: 5, a: 2), $g(3), "\n";
$h = function ($a) { return $a; };
echo $h(1, 2, 3), "\n";
$h();
