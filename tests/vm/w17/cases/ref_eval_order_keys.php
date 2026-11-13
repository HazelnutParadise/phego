<?php
function k($n) { echo "k$n "; return $n; }
$a = [];
$b = [];
$a[k(1)][k(2)] = &$b[k(3)][k(4)];
echo "\n";
$b[3][4] = "v";
echo $a[1][2], "\n";
$x = &$b[k(5)];
echo "\n";
$a[k(6)] = &$x;
echo "\n";
