<?php
function inner() { yield 1; throw new LogicException("inner fail"); }
function outer() { try { yield from inner(); } catch (LogicException $e) { echo "outer caught ", $e->getMessage(), "\n"; yield 99; } }
foreach (outer() as $k => $v) echo "$k => $v\n";
function inner2() { try { yield 1; } catch (Exception $e) { echo "inner2 caught ", $e->getMessage(), "\n"; yield 2; } }
function outer2() { yield from inner2(); }
$g = outer2(); $g->current();
echo $g->throw(new Exception("thrown")), "\n";
