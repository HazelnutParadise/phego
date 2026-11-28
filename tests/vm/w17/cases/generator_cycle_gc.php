<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function gen() { $self = yield; $d = new D(1); yield 1; }
$g = gen(); $g->current(); $g->send($g);
unset($g);
echo "after unset\n";
class H { public $g; function __destruct() { echo "~H\n"; } }
function g2($h) { $d = new D(2); yield 1; }
$h = new H; $h->g = g2($h); $h->g->current();
unset($h);
echo "end\n";
