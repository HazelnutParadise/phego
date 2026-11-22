<?php
class A { public $v = "a"; }
$f = function () { return $this->v; };
$b = $f->bindTo(...);
$c = $b->bindTo(new A);
echo $c === null ? "null" : "obj", "\n";
