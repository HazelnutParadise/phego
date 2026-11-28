<?php
function f($a, $b) { echo "f $a $b\n"; return 0; }
function g() { f("x", yield 1); }
$x = g(); $x->current();
function k($p, $q, $r) { echo "k $p $q $r\n"; }
k("p", $x->send("s"), "r");
echo "done\n";
