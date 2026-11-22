<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "dtor ", $this->n, "\n"; echo (new Exception("t"))->getTraceAsString(), "\n"; } }
function thrower() { throw new Exception("x"); }
function g($a, $b) {}
function f() { $a = new D('cv'); g(new D('arg'), thrower()); }
try { f(); } catch (Exception $e) { echo "caught ", $e->getLine(), "\n"; }
