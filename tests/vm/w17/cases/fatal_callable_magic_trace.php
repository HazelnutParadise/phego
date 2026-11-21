<?php
class A { function __call($n, $a) { return nope(); } }
$o = new A;
echo "x\n";
$f = $o->zzz(...);
$f(1, 2);
