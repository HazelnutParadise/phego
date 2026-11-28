<?php
class O { function m($a, $b) { echo "m $a $b\n"; } }
function g($o) { $o->m(1, yield 1); }
$x = g(new O); $x->current(); echo "cur\n"; $x->send(5); echo "done\n";
