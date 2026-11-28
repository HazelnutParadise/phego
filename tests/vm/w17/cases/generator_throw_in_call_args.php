<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class O { function m($a, $b, $c) { echo "m\n"; } function __destruct() { echo "~O\n"; } }
function g() { try { (new O)->m(new D(1), yield 1, new D(2)); } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; } yield 2; }
$x = g(); $x->current();
echo $x->throw(new Exception("in")), "\n";
function h() { try { (new O)->m(new D(3), yield 1, 0); } finally { echo "fin\n"; } }
$y = h(); $y->current(); echo "drop\n"; $y = null; echo "end\n";
