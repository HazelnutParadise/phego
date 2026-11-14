<?php
function k($n) { echo "k$n "; return $n; }
function t($n) { echo "t$n "; return $n; }
$x = ['a' => 1, 'b' => 2];
[k('a') => &$p, k('b') => $q] = $x;
$p = 'P';
echo "\n", $x['a'], $q, "\n";
$y = [1, 2];
[&$m[t(1)], $m[t(0)]] = $y;
$m[1] = 'M';
echo "\n", $y[0], $y[1], $m[0], "\n";
$z = ['k' => [1]];
[&$r] = $z['k'];
$r = 'R';
echo $z['k'][0], "\n";
$i = 0;
[$i => &$s, $i => $u] = $y;
$s = 'S';
echo $y[0], $u, "\n";
