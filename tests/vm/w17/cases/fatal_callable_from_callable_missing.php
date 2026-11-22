<?php
class A { function __call($n, $a) { return nope(); } function m($x) { return nope2($x); } }
$o = new A;
echo "x\n";
$g = $o->m(...);
$c = Closure::fromCallable("strrev2");
