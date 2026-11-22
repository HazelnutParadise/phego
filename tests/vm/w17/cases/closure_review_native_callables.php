<?php
class A { public $v = "a"; }
$f = function () { return isset($this) ? $this->v : "unbound"; };
echo Closure::bind(...)($f, new A)(), "\n";
$b = [$f, "bindTo"](...);
echo $b(new A)(), "\n";
$m = "bindTo";
$g = [$f, $m];
echo $g(new A)(), "\n";
$x = "Closure::bind";
echo $x($f, new A)(), "\n";
function plain($n) { return "p" . $n; }
$fc = Closure::fromCallable(...);
echo $fc("plain")(1), "\n";
$c = [$f, "call"];
echo $c(new A), "\n";
$i = [$f, "__invoke"](...);
echo $i === $f ? "same" : "different", "\n";
echo $b == [$f, "bindTo"](...) ? "equal" : "unequal", "\n";
