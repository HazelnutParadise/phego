<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "dtor ", $this->n, "\n"; } }
function f() { $l = new D("local"); foreach (new D("iter") as $v) { return 1; } }
f();
echo "end\n";
function g() { $l = new D("local2"); foreach (new D("iter2") as $v) { try { return 1; } finally { echo "fin\n"; } } }
g();
echo "end2\n";
