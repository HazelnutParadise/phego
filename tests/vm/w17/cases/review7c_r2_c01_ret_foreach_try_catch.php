<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "dtor ", $this->n, "\n"; } }
function f() { $a = new D('cv'); foreach ([new D('o')] as $v) { try { return 2; } catch (Exception $e) {} } }
echo f(), "\n";
