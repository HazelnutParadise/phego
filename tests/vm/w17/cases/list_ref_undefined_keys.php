<?php
$y = [1];
[&$a, $b] = $y;
echo $a, $b === null ? "null" : "?", "\n";
$z = [];
[&$c, &$d] = $z;
$c = 'C';
$d = 'D';
echo $z[0], $z[1], "\n";
$w = ['x' => 1];
['y' => &$e] = $w;
echo $e === null ? "null" : "?", $w['y'] === null ? "null" : "?", "\n";
$v = [[]];
[[&$f]] = $v;
echo $f === null ? "null" : "?", "\n";
