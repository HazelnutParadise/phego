<?php
function gen() { static $calls = 0; $calls++; yield $calls; }
foreach (gen() as $v) echo $v, "\n";
foreach (gen() as $v) echo $v, "\n";
function g2(&$x) { $x++; yield $x; $x++; }
$a = 1; foreach (g2($a) as $v) echo $v, "\n"; echo $a, "\n";
function g3() { $arr = [1, 2]; foreach ($arr as &$r) { yield $r; $r *= 10; } echo $arr[0] + $arr[1], "\n"; }
foreach (g3() as $v) echo $v, "\n";
