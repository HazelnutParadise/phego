<?php
class A { public $v = 1; function mk() { return function ($a) { return nope($a); }; } }
echo "x\n";
$f = (new A)->mk();
$f(5);
