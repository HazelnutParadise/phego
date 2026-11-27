<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "dtor ", $this->n, ": ", (new Exception)->getTraceAsString(), "\n"; } }
class C { public $v = 2; }
$f = function ($a) { $d = new D('call'); return $this->v + $a; };
echo $f->call(new C, 5), "\n";
