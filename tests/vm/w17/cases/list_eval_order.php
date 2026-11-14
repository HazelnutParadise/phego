<?php
function k($n) { echo "k$n "; return $n; }
function v($n) { echo "v$n "; return $n; }
function t($n) { echo "t$n "; return $n; }
[k(0) => $a, k(1) => $b] = [v(5), v(6)];
echo "\n", $a, $b, "\n";
[$arr[t(0)], $arr[t(1)]] = [v(1), v(2)];
echo "\n", $arr[0], $arr[1], "\n";
[k(0) => $m[t(1)], k(1) => $m[t(0)]] = [v(7), v(8)];
echo "\n", $m[0], $m[1], "\n";
$i = 0;
[$i => $c, $i => $d] = [10, 20];
echo $c, $d, "\n";
[$i++ => $e, $i++ => $f] = [30, 40];
echo $e, $f, $i, "\n";
$j = 0;
[$j => $g, ++$j => $h] = [50, 60];
echo $g, $h, $j, "\n";
