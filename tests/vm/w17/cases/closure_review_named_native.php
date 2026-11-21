<?php
class A { public $v = "a"; private $p = "priv"; }
$f = function ($x = "d") { return (isset($this) ? $this->v : "unbound") . $x; };
echo $f->bindTo(newThis: new A)(), "\n";
echo $f->bindTo(newScope: A::class, newThis: null)(), "\n";
echo Closure::bind(newThis: new A, closure: $f)("y"), "\n";
function plain($n) { return "p" . $n; }
echo Closure::fromCallable(callback: "plain")(2), "\n";
echo $f->call(newThis: new A, x: "z"), "\n";
$p = function ($a, $b) { return $this->p . $a . $b; };
echo $p->call(new A, b: 2, a: 1), "\n";
