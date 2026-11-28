<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $v = new D(1); $x = [new D(2), yield 1, new D(3)]; }
$g = gen(); $g->current(); unset($g); echo "--\n";
function g2() { $v = new D(4); echo f(new D(5), yield 1); }
function f($a, $b) { return "f"; }
$g = g2(); $g->current(); unset($g); echo "--\n";
function g3() { foreach ([new D(6)] as $x) { $y = new D(7); yield 1; } }
$g = g3(); $g->current(); unset($g); echo "end\n";
