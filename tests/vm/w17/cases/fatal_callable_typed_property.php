<?php
function g($n) { return "g$n"; }
function f(callable $c) { return $c(1); }
echo f("g"), "\n";
function h(?Closure $c = null, callable|int $d = 3) { return $c ? $c() : "none"; }
echo h(), h(fn() => "c"), "\n";
class A { public Closure $c; public ?Closure $d = null; }
$a = new A; $a->c = fn() => 2; echo ($a->c)(), "\n";
$a->c = "g";
