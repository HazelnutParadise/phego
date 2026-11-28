<?php
class D { function __construct(public $n){} function __destruct(){ echo "d{$this->n}\n"; } }
class O { function m($a, $b) { echo "m\n"; } }
function g($o) { $o->m(new D(1), yield 1); }
$x = g(new O); $x->current(); echo "cur\n"; unset($x); echo "done\n";
