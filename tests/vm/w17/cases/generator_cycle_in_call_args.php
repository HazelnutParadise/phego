<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function f($a, $b) {}
function g() { $self = yield; f(new D(1), yield 2); }
$x = g(); $x->current(); $x->send($x);
unset($x);
echo "unset\n";
