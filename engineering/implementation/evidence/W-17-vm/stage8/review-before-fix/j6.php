<?php
class O { function m($a, $b) { echo "m $a $b\n"; return 0; } }
function g($o) { $o->m("x", yield 1); }
$x = g(new O); $x->current();
function k($p, $q, $r) { echo "k $p $q $r\n"; }
try { k("p", $x->send("s"), "r"); } catch (Throwable $e) { echo "T ", $e->getMessage(), "\n"; }
echo "done\n";
