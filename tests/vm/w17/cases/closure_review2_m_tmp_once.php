<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "~", $this->n, "\n"; } }
class E extends D { function __call($n, $a) { return $n; } }
$f = (new E("orig"))->zz(...);
$f->call(new E("tmp"));
$f = null;
echo "f dropped\n";
