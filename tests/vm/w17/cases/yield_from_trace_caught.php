<?php
function inner() { yield 1; echo (new Exception("t"))->getTraceAsString(), "\n"; yield 2; }
function outer() { yield from inner(); }
foreach (outer() as $v) {}
$g = outer(); $g->current(); $g->next();
