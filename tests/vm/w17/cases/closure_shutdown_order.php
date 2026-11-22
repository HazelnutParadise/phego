<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "d", $this->n, "\n"; } }
$a = new D(1);
$f = function () use ($a) { return 1; };
$b = new D(2);
$g = function () use (&$b) { return 1; };
$h = (function () { $o = new D(3); return function () use ($o) { return $o->n; }; })();
unset($a, $b);
echo "end\n";
