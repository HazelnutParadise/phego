<?php
class O { function __construct($a, $b) { echo "ctor $a $b\n"; } }
function g() { new O(1, yield 1); }
$x = g(); $x->current(); echo "cur\n"; $x->send(5); echo "done\n";
