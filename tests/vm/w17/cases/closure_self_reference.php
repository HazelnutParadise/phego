<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "d", $this->n, "\n"; } }
function mk() { $o = new D(1); $f = function () use (&$f, $o) { return 1; }; return 5; }
echo mk(), "\n";
$o2 = new D(2);
$g = function () use (&$g, $o2) {};
unset($g, $o2);
echo "after unset\n";
$t = new D(3);
(function () use ($t) { echo "in\n"; })();
$t = null;
echo "end\n";
