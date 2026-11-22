<?php
$x = 1;
$f = function () use ($x) { $x++; return $x; };
echo $f(), $f(), "\n";
function p(&$r) { return function () use (&$r) { $r++; }; }
$v = 1; $c = p($v); $c(); $c(); echo $v, "\n";
function s() { static $n = 0; return function () use (&$n) { return ++$n; }; }
$a = s(); $b = s(); echo $a(), $b(), $a(), "\n";
$arr = [1, 2];
$m = function () use ($arr) { $arr[] = 3; $c = 0; foreach ($arr as $_) { $c++; } return $c; };
echo $m(), $m(), "\n";
$y = 5; $r = &$y;
$n = function () use ($y) { return $y; };
$y = 6; echo $n(), "\n";
