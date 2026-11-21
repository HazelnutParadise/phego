<?php
class A { private $p = 2; }
$f = function ($a) { return nope($a, $this->p); };
echo "x\n";
$f->call(new A, 7);
