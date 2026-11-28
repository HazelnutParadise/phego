<?php
function &gen(array &$arr) { foreach ($arr as $k => &$v) { yield $k => $v; } }
$a = [1, 2, 3];
foreach (gen($a) as &$v) { $v *= 10; }
unset($v);
echo implode_ish($a), "\n";
function implode_ish($a) { $s = ""; foreach ($a as $x) $s .= $x . ","; return $s; }
function &g2() { $x = 5; yield $x; echo "x=$x\n"; }
foreach (g2() as &$r) { $r = 7; }
function g3() { yield 1; }
try { foreach (g3() as &$r) {} } catch (Error $e) { echo $e::class, ": ", $e->getMessage(), "\n"; }
function &g4() { yield 1; }
foreach (g4() as &$r) { echo $r, "\n"; }
