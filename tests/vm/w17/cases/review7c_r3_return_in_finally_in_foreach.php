<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "dtor ", $this->n, "\n"; } function m() { return $this; } }
function f() { $l = new D('local'); foreach ([new D('it')] as $v) { try { throw new Exception("x"); } finally { return 3; } } }
echo f(), "\n";
