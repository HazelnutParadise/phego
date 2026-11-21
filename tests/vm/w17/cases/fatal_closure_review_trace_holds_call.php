<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "d", $this->n, "\n"; } }
echo "before\n";
$f = function ($a) { $n = null; $n->q(); };
$f->call(new D("callthrow"), new D("arg"));
