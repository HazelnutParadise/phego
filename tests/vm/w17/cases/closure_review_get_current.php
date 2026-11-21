<?php
$f = function () { return Closure::getCurrent(); };
echo $f() === $f ? "same" : "diff", "\n";
$fact = fn($n) => $n < 2 ? 1 : $n * Closure::getCurrent()($n - 1);
echo $fact(5), "\n";
class A { function m() { return function () { return Closure::getCurrent() instanceof Closure ? "closure" : "no"; }; } }
echo (new A)->m()(), "\n";
