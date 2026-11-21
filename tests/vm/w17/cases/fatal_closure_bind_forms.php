<?php
class A { public $x = 1; }
$f = function () { return $this->x; };
$b = Closure::bind($f, new A);
echo $b(), "\n";
$c = $f->bindTo(new A, "A");
echo $c(), "\n";
$s = static fn() => 1;
echo $s->bindTo(null, "A")(), "\n";
$d = $f->bindTo(new A, new A);
echo $d(), "\n";
$e = $b->bindTo(null);
echo $e === null ? "null" : "closure", "\n";
$u = function () { return isset($this) ? "t" : "n"; };
$v = $u->bindTo(new A); echo $v(), $v->bindTo(null)(), "\n";
$f();
