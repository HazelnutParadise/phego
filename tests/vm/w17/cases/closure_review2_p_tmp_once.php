<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "~", $this->n, "\n"; } }
$d = new D("cap");
$f = function () use ($d) { return 1; };
$d = null;
$f->call(new D("tmp"));
$f = null;
echo "f dropped\n";
