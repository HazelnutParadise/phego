<?php
function inner() { try { yield 1; yield 2; } finally { echo "inner finally\n"; } }
function outer() { try { yield from inner(); } catch (Exception $e) { echo "outer caught ", $e->getMessage(), "\n"; } yield 3; }
$g = outer();
echo $g->current(), "\n";
echo $g->throw(new Exception("x")), "\n";
$g->next();
echo $g->valid() ? "v" : "nv", "\n";
