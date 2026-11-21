<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "~", $this->n, "\n"; } }
$d = new D("cap");
$f = function () use ($d) { return $this->n; };
$d = null;
echo $f->call(new D("t1")), $f->call(new D("t2")), "\n";
$f = null;
echo "f dropped\n";
