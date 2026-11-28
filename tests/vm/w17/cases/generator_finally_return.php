<?php
function gen() { try { yield 1; return 5; } finally { echo "fin\n"; } }
$g = gen(); foreach ($g as $v) echo $v, "\n"; echo $g->getReturn(), "\n";
function g2() { try { yield 1; } finally { return 7; } }
$h = g2(); $h->current(); $h->next(); echo $h->getReturn(), "\n";
function g3() { try { yield 1; } finally { echo "f3\n"; } }
$k = g3(); $k->current(); $k = null; echo "--\n";
function g4() { try { try { yield 1; } finally { echo "in\n"; } } finally { echo "out\n"; } }
$m = g4(); $m->current(); unset($m); echo "end\n";
