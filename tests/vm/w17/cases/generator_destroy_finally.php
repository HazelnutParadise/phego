<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $d = new D(1); try { echo "before\n"; yield 1; echo "after\n"; } finally { echo "finally\n"; } }
$g = gen();
$g->current();
echo "unset\n";
unset($g);
echo "after unset\n";
function g2() { try { foreach ([1, 2] as $x) { try { yield $x; } finally { echo "inner $x\n"; } } } finally { echo "outer\n"; } }
$h = g2(); $h->current(); $h = null; echo "--\n";
function g3() { try { yield 1; } finally { echo "f3\n"; } }
$n = g3(); unset($n); echo "never started\n";
