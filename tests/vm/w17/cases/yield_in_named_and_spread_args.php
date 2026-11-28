<?php
function f($a, $b) { echo "f $a $b\n"; }
function g() { f(b: yield 1, a: 2); }
$x = g(); $x->current(); $x->send(5); echo "done\n";
function h() { f(...[1], ...[yield 2]); }
$y = h(); $y->current(); $y->send(6); echo "done2\n";
