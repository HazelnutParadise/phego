<?php
function g($x) { return "g" . $x; }
class A { function m($x) { return "m" . $x; } static function s($x) { return "s" . $x; } }
function f(callable $c, $x) { return $c($x); }
echo f("g", 1), f(function ($x) { return "c" . $x; }, 2), f([new A, "m"], 3), f("A::s", 4), f(["A", "s"], 5), f(g(...), 6), "\n";
function r(): Closure { return fn() => 1; }
function r2(): callable { return "g"; }
echo r()(), r2()(7), "\n";
f("nope", 1);
