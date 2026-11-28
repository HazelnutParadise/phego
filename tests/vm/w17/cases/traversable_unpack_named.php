<?php
function f($a, $b = 'B', ...$rest) { echo "a=$a b=$b"; foreach ($rest as $k => $v) echo " $k=$v"; echo "\n"; }
function g1() { yield 1; yield 2; yield 3; }
function g2() { yield 'b' => 'x'; yield 'a' => 'y'; }
function g3() { yield 1; yield 'c' => 'z'; }
function g4() { yield '1' => 'numeric'; yield 'w'; }
f(...g1()); f(...g2()); f(...g3()); f(...g4());
function r(&$x) { $x = 'changed'; }
function g5() { yield 'v'; }
r(...g5());
echo "done\n";
