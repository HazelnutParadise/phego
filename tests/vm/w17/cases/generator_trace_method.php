<?php
function gen($a) { yield 1; throw new Exception("e"); }
$g = gen("x");
$g->current();
try { $g->next(); } catch (Exception $e) { echo $e->getTraceAsString(), "\n"; }
function gen2() { yield 1; echo (new Exception("t"))->getTraceAsString(), "\n"; yield 2; }
foreach (gen2() as $v) {}
$g3 = gen2(); $g3->current(); $g3->next();
