<?php
function f($a, $b) { echo "f $a $b\n"; }
function g() { $n = 'f'; $n(1, yield 1); }
$x = g(); $x->current(); echo "cur\n"; $x->send(5); echo "done\n";
